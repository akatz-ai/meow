package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
)

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Show the current prompt for an agent",
	Long: `Display the current prompt or task for an agent.

This command is called by Claude Code's stop-hook to determine what work
is assigned. It returns:
- Empty string if no work or in interactive mode (agent should idle)
- Prompt text if in autonomous mode (agent should continue)

When called with --format=prompt (default for stop-hook), uses IPC to
communicate with the running orchestrator.

When called with --format=text or --format=json, reads workflow state
directly for human-readable output.`,
	RunE: runPrime,
}

var (
	primeAgent  string
	primeFormat string
)

func init() {
	primeCmd.Flags().StringVar(&primeAgent, "agent", "", "agent ID (default: from MEOW_AGENT env)")
	primeCmd.Flags().StringVar(&primeFormat, "format", "text", "output format: text, json, prompt")
	rootCmd.AddCommand(primeCmd)
}

// PrimeOutput represents the structured output of the prime command.
type PrimeOutput struct {
	Workflow    *WorkflowInfo `json:"workflow,omitempty"`
	CurrentStep *StepInfo     `json:"current_step,omitempty"`
	Interactive bool          `json:"interactive,omitempty"`
	NoWork      bool          `json:"no_work,omitempty"`
}

// WorkflowInfo describes the current workflow.
type WorkflowInfo struct {
	ID       string `json:"id"`
	Template string `json:"template"`
	Status   string `json:"status"`
}

// StepInfo describes a workflow step.
type StepInfo struct {
	ID          string                           `json:"id"`
	Status      string                           `json:"status"`
	Prompt      string                           `json:"prompt,omitempty"`
	Mode        string                           `json:"mode,omitempty"`
	Outputs     map[string]types.AgentOutputDef  `json:"outputs,omitempty"`
}

func runPrime(cmd *cobra.Command, args []string) error {
	// Get agent ID from flag or environment
	agent := primeAgent
	if agent == "" {
		agent = os.Getenv("MEOW_AGENT")
	}
	if agent == "" {
		if primeFormat == "prompt" {
			// Stop-hook mode: return empty silently
			return nil
		}
		return fmt.Errorf("agent not specified and MEOW_AGENT not set")
	}

	// Get workflow ID from environment
	workflowID := os.Getenv("MEOW_WORKFLOW")

	// For prompt format, use IPC if workflow is set
	if primeFormat == "prompt" {
		if workflowID == "" {
			// No workflow context - return empty
			return nil
		}
		return runPrimeIPC(agent, workflowID)
	}

	// For other formats, read workflow files directly
	return runPrimeLocal(agent, workflowID)
}

// runPrimeIPC uses IPC to get the prompt from the running orchestrator.
// This is the fast path for stop-hook injection.
func runPrimeIPC(agent, workflowID string) error {
	client := ipc.NewClientForWorkflow(workflowID)
	prompt, err := client.GetPrompt(agent)
	if err != nil {
		// IPC failed - orchestrator might not be running
		// Return empty to avoid blocking the stop-hook
		return nil
	}

	if prompt != "" {
		fmt.Print(prompt)
	}
	return nil
}

// runPrimeLocal reads workflow files directly for human-readable output.
func runPrimeLocal(agent, workflowID string) error {
	// Find workflows directory
	dir, err := getWorkDir()
	if err != nil {
		return err
	}
	workflowsDir := filepath.Join(dir, ".meow", "workflows")

	// Find the running step for this agent
	output, err := getPrimeOutputFromFiles(workflowsDir, agent, workflowID)
	if err != nil {
		return err
	}

	// Format and print output
	switch primeFormat {
	case "json":
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling output: %w", err)
		}
		fmt.Println(string(data))

	default: // "text"
		fmt.Print(formatPrimeText(output))
	}

	return nil
}

// getPrimeOutputFromFiles reads workflow files to build prime output.
func getPrimeOutputFromFiles(workflowsDir, agentID, workflowID string) (*PrimeOutput, error) {
	// If specific workflow ID provided, read just that one
	if workflowID != "" {
		wf, err := readWorkflowFile(filepath.Join(workflowsDir, workflowID+".yaml"))
		if err != nil {
			return &PrimeOutput{NoWork: true}, nil
		}
		return buildPrimeOutput(wf, agentID)
	}

	// Otherwise, scan all running workflows for this agent
	entries, err := os.ReadDir(workflowsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &PrimeOutput{NoWork: true}, nil
		}
		return nil, fmt.Errorf("reading workflows dir: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}

		wf, err := readWorkflowFile(filepath.Join(workflowsDir, entry.Name()))
		if err != nil {
			continue
		}

		// Skip non-running workflows
		if wf.Status != types.WorkflowStatusRunning {
			continue
		}

		// Check if this workflow has work for the agent
		output, err := buildPrimeOutput(wf, agentID)
		if err != nil {
			continue
		}
		if output.CurrentStep != nil || output.Interactive {
			return output, nil
		}
	}

	return &PrimeOutput{NoWork: true}, nil
}

// readWorkflowFile reads a workflow YAML file.
func readWorkflowFile(path string) (*types.Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var wf types.Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}
	return &wf, nil
}

// buildPrimeOutput creates prime output from a workflow.
func buildPrimeOutput(wf *types.Workflow, agentID string) (*PrimeOutput, error) {
	// Find running step for this agent
	step := wf.GetRunningStepForAgent(agentID)
	if step == nil {
		// Check for next ready step
		nextStep := wf.GetNextReadyStepForAgent(agentID)
		if nextStep == nil {
			return &PrimeOutput{NoWork: true}, nil
		}
		step = nextStep
	}

	// Check for completing state
	if step.Status == types.StepStatusCompleting {
		return &PrimeOutput{NoWork: true}, nil
	}

	// Check for interactive mode
	if step.Agent != nil && step.Agent.Mode == "interactive" {
		return &PrimeOutput{
			Workflow: &WorkflowInfo{
				ID:       wf.ID,
				Template: wf.Template,
				Status:   string(wf.Status),
			},
			Interactive: true,
		}, nil
	}

	// Build step info
	stepInfo := &StepInfo{
		ID:     step.ID,
		Status: string(step.Status),
	}
	if step.Agent != nil {
		stepInfo.Prompt = step.Agent.Prompt
		stepInfo.Mode = step.Agent.Mode
		if stepInfo.Mode == "" {
			stepInfo.Mode = "autonomous"
		}
		stepInfo.Outputs = step.Agent.Outputs
	}

	return &PrimeOutput{
		Workflow: &WorkflowInfo{
			ID:       wf.ID,
			Template: wf.Template,
			Status:   string(wf.Status),
		},
		CurrentStep: stepInfo,
	}, nil
}

// formatPrimeText creates human-readable output.
func formatPrimeText(output *PrimeOutput) string {
	var sb strings.Builder

	if output.NoWork {
		sb.WriteString("No work assigned.\n")
		return sb.String()
	}

	if output.Interactive {
		sb.WriteString("Interactive mode - waiting for user input.\n")
		if output.Workflow != nil {
			sb.WriteString(fmt.Sprintf("Workflow: %s\n", output.Workflow.ID))
		}
		return sb.String()
	}

	sb.WriteString("═══════════════════════════════════════════════════════════════\n")
	if output.Workflow != nil {
		sb.WriteString(fmt.Sprintf("Workflow: %s\n", output.Workflow.ID))
		sb.WriteString(fmt.Sprintf("Template: %s\n", output.Workflow.Template))
	}
	sb.WriteString("═══════════════════════════════════════════════════════════════\n\n")

	if output.CurrentStep != nil {
		sb.WriteString(fmt.Sprintf("Current Step: %s (%s mode)\n\n", output.CurrentStep.ID, output.CurrentStep.Mode))

		if output.CurrentStep.Prompt != "" {
			sb.WriteString("Prompt:\n")
			sb.WriteString("───────────────────────────────────────────────────────────────\n")
			sb.WriteString(output.CurrentStep.Prompt)
			if !strings.HasSuffix(output.CurrentStep.Prompt, "\n") {
				sb.WriteString("\n")
			}
			sb.WriteString("───────────────────────────────────────────────────────────────\n\n")
		}

		if len(output.CurrentStep.Outputs) > 0 {
			sb.WriteString("Required Outputs:\n")
			for name, def := range output.CurrentStep.Outputs {
				required := ""
				if def.Required {
					required = " (required)"
				}
				sb.WriteString(fmt.Sprintf("  • %s (%s)%s\n", name, def.Type, required))
				if def.Description != "" {
					sb.WriteString(fmt.Sprintf("    %s\n", def.Description))
				}
			}
			sb.WriteString("\n")
		}

		sb.WriteString("When done, run: meow done")
		if len(output.CurrentStep.Outputs) > 0 {
			sb.WriteString(" --output name=value")
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// formatPrimePrompt creates prompt output for stop-hook injection.
// This is called from GetPromptForStopHook in the orchestrator.
func formatPrimePrompt(step *types.Step) string {
	return orchestrator.GetPromptForStopHook(step)
}
