package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/spf13/cobra"
)

var doneCmd = &cobra.Command{
	Use:   "done",
	Short: "Signal step completion",
	Long: `Signal that the current step is complete.

This command is called by agents when they finish a task. It sends a completion
signal via IPC to the orchestrator.

Environment variables required:
  MEOW_AGENT    - Agent ID (set by orchestrator)
  MEOW_WORKFLOW - Workflow ID (set by orchestrator)

Examples:
  # Simple completion
  meow done

  # With outputs
  meow done --output key=value --output other=value2

  # With JSON outputs
  meow done --output-json '{"key": "value"}'

  # With notes
  meow done --notes "Completed successfully"`,
	RunE: runDone,
}

var (
	doneNotes      string
	doneOutputs    []string
	doneOutputJSON string
)

func init() {
	doneCmd.Flags().StringVar(&doneNotes, "notes", "", "completion notes")
	doneCmd.Flags().StringArrayVar(&doneOutputs, "output", nil, "output values (format: name=value)")
	doneCmd.Flags().StringVar(&doneOutputJSON, "output-json", "", "outputs as JSON object")
	rootCmd.AddCommand(doneCmd)
}

func runDone(cmd *cobra.Command, args []string) error {
	// Check if orchestrator socket is set
	// If not set, exit silently (no-op) - allows agents to call meow done
	// without being in a MEOW workflow
	if os.Getenv("MEOW_ORCH_SOCK") == "" {
		return nil // Silent no-op
	}

	// Get agent ID from environment
	agentID := os.Getenv("MEOW_AGENT")
	if agentID == "" {
		return fmt.Errorf("MEOW_AGENT not set - are you running in a MEOW session?")
	}

	// Get workflow ID from environment
	workflowID := os.Getenv("MEOW_WORKFLOW")
	if workflowID == "" {
		return fmt.Errorf("MEOW_WORKFLOW not set - are you running in a MEOW session?")
	}

	// Try to get step ID from environment or arguments
	// The orchestrator sets MEOW_STEP when it knows the current step
	stepID := os.Getenv("MEOW_STEP")
	if stepID == "" {
		// Allow step ID as argument for manual use
		if len(args) > 0 {
			stepID = args[0]
		}
	}
	// If still empty, the IPC handler will find the running step for this agent

	// Parse outputs from flags
	outputs := make(map[string]any)

	// First, parse --output flags
	for _, o := range doneOutputs {
		parts := strings.SplitN(o, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid output format: %s (expected name=value)", o)
		}
		outputs[parts[0]] = parts[1]
	}

	// Then, parse --output-json if provided
	if doneOutputJSON != "" {
		var jsonOutputs map[string]any
		if err := json.Unmarshal([]byte(doneOutputJSON), &jsonOutputs); err != nil {
			return fmt.Errorf("invalid --output-json: %w", err)
		}
		for k, v := range jsonOutputs {
			outputs[k] = v
		}
	}

	// Create IPC client for this workflow
	client := ipc.NewClientForWorkflow(workflowID)

	// Send step done message
	response, err := client.SendStepDone(workflowID, agentID, stepID, outputs, doneNotes)
	if err != nil {
		return fmt.Errorf("sending done message: %w", err)
	}

	// Check response
	switch r := response.(type) {
	case *ipc.AckMessage:
		if !r.Success {
			return fmt.Errorf("orchestrator rejected completion")
		}
		if verbose {
			fmt.Printf("Step %s completed\n", stepID)
		}
	case *ipc.ErrorMessage:
		return fmt.Errorf("orchestrator error: %s", r.Message)
	default:
		return fmt.Errorf("unexpected response type: %T", response)
	}

	return nil
}
