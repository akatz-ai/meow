package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/spf13/cobra"
)

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Show the next task for an agent",
	Long: `Display the next task assigned to the current agent.

This command is typically called by Claude Code at the start of a session
to see what work is assigned. It shows:
- The bead ID and title
- Full instructions
- Required outputs (if any)
- Dependencies and context

When called with --format=prompt, outputs only the prompt text suitable
for injection into an agent's context.`,
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

// PrimeOutput represents the output of the prime command.
type PrimeOutput struct {
	Workflow         *WorkflowInfo `json:"workflow,omitempty"`
	WorkBead         *WorkBeadInfo `json:"work_bead,omitempty"`
	CurrentStep      *StepInfo     `json:"current_step,omitempty"`
	ConversationMode bool          `json:"conversation_mode,omitempty"`
}

// WorkflowInfo describes the current workflow.
type WorkflowInfo struct {
	Name      string      `json:"name"`
	ID        string      `json:"id"`
	Total     int         `json:"total"`
	Completed int         `json:"completed"`
	Steps     []*StepInfo `json:"steps"`
}

// WorkBeadInfo describes the linked work bead.
type WorkBeadInfo struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// StepInfo describes a workflow step.
type StepInfo struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	IsCurrent    bool   `json:"is_current,omitempty"`
	Instructions string `json:"instructions,omitempty"`
}

func runPrime(cmd *cobra.Command, args []string) error {
	if err := checkBeadsDir(); err != nil {
		return err
	}

	// Get agent ID from flag or environment
	agent := primeAgent
	if agent == "" {
		agent = os.Getenv("MEOW_AGENT")
	}
	if agent == "" {
		agent = "default"
	}

	// Get working directory
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// Load beads from .beads directory
	beadsDir := filepath.Join(dir, ".beads")
	store := orchestrator.NewFileBeadStore(beadsDir)
	if err := store.Load(context.Background()); err != nil {
		return fmt.Errorf("loading beads: %w", err)
	}

	// Find agent's wisp steps
	output, err := getPrimeOutput(context.Background(), store, agent)
	if err != nil {
		return err
	}

	// Handle empty output
	if output == nil {
		switch primeFormat {
		case "json":
			fmt.Println("{}")
		case "prompt":
			// Empty output for prompt format means no work or conversation mode
		default:
			fmt.Println("No tasks assigned to this agent.")
		}
		return nil
	}

	// Format output
	switch primeFormat {
	case "json":
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling output: %w", err)
		}
		fmt.Println(string(data))

	case "prompt":
		// Conversation mode returns empty to prevent auto-continuation
		if output.ConversationMode {
			return nil
		}
		fmt.Print(formatPrompt(output))

	default:
		fmt.Print(formatText(output))
	}

	return nil
}

func getPrimeOutput(ctx context.Context, store *orchestrator.FileBeadStore, agentID string) (*PrimeOutput, error) {
	// Check for in-progress collaborative steps first
	inProgress, err := store.ListFiltered(ctx, orchestrator.BeadFilter{
		Tier:     types.TierWisp,
		Assignee: agentID,
		Status:   types.BeadStatusInProgress,
	})
	if err != nil {
		return nil, fmt.Errorf("listing in-progress beads: %w", err)
	}

	for _, step := range inProgress {
		if step.Type == types.BeadTypeCollaborative {
			// Collaborative step in progress - conversation mode
			return &PrimeOutput{
				ConversationMode: true,
			}, nil
		}
	}

	// Get ready wisp steps for this agent
	wispSteps, err := store.ListFiltered(ctx, orchestrator.BeadFilter{
		Tier:     types.TierWisp,
		Assignee: agentID,
		Status:   types.BeadStatusOpen,
	})
	if err != nil {
		return nil, fmt.Errorf("listing wisp beads: %w", err)
	}

	// Filter to only ready steps (all dependencies closed)
	var readySteps []*types.Bead
	for _, step := range wispSteps {
		if isReady(ctx, store, step) {
			readySteps = append(readySteps, step)
		}
	}

	if len(readySteps) == 0 {
		return nil, nil // No work
	}

	// Sort by creation time for deterministic behavior
	sort.Slice(readySteps, func(i, j int) bool {
		return readySteps[i].CreatedAt.Before(readySteps[j].CreatedAt)
	})

	// Get the first ready step
	current := readySteps[0]

	// Get all steps in this workflow for progress display
	allSteps, err := store.ListFiltered(ctx, orchestrator.BeadFilter{
		Tier:       types.TierWisp,
		Assignee:   agentID,
		WorkflowID: current.WorkflowID,
	})
	if err != nil {
		return nil, fmt.Errorf("listing workflow beads: %w", err)
	}

	// Sort steps by creation time for consistent display
	sort.Slice(allSteps, func(i, j int) bool {
		return allSteps[i].CreatedAt.Before(allSteps[j].CreatedAt)
	})

	// Get linked work bead
	var workBead *WorkBeadInfo
	if current.HookBead != "" {
		wb, err := store.Get(ctx, current.HookBead)
		if err == nil && wb != nil {
			workBead = &WorkBeadInfo{
				ID:    wb.ID,
				Title: wb.Title,
			}
		}
	}

	// Build workflow info
	completed := 0
	var steps []*StepInfo
	for _, s := range allSteps {
		if s.Status == types.BeadStatusClosed {
			completed++
		}
		step := &StepInfo{
			ID:     s.ID,
			Title:  s.Title,
			Status: string(s.Status),
		}
		if s.ID == current.ID {
			step.IsCurrent = true
			step.Instructions = s.Instructions
		}
		steps = append(steps, step)
	}

	return &PrimeOutput{
		Workflow: &WorkflowInfo{
			Name:      current.SourceWorkflow,
			ID:        current.WorkflowID,
			Total:     len(allSteps),
			Completed: completed,
			Steps:     steps,
		},
		WorkBead: workBead,
		CurrentStep: &StepInfo{
			ID:           current.ID,
			Title:        current.Title,
			Status:       string(current.Status),
			IsCurrent:    true,
			Instructions: current.Instructions,
		},
	}, nil
}

func isReady(ctx context.Context, store *orchestrator.FileBeadStore, bead *types.Bead) bool {
	for _, depID := range bead.Needs {
		dep, err := store.Get(ctx, depID)
		if err != nil || dep == nil {
			return false
		}
		if dep.Status != types.BeadStatusClosed {
			return false
		}
	}
	return true
}

func formatText(output *PrimeOutput) string {
	var sb strings.Builder

	sb.WriteString("═══════════════════════════════════════════════════════════════\n")
	if output.Workflow != nil {
		sb.WriteString(fmt.Sprintf("Your workflow: %s (step %d/%d)\n",
			output.Workflow.Name,
			output.Workflow.Completed+1,
			output.Workflow.Total))
	}
	if output.WorkBead != nil {
		sb.WriteString(fmt.Sprintf("Work bead: %s %q\n", output.WorkBead.ID, output.WorkBead.Title))
	}
	sb.WriteString("═══════════════════════════════════════════════════════════════\n\n")

	// Progress display
	if output.Workflow != nil && len(output.Workflow.Steps) > 0 {
		for _, step := range output.Workflow.Steps {
			var marker string
			switch {
			case step.Status == "closed":
				marker = "  ✓"
			case step.IsCurrent:
				marker = "  →"
			default:
				marker = "  ○"
			}
			label := step.Title
			if step.IsCurrent {
				label = fmt.Sprintf("%s [in_progress] ← YOU ARE HERE", label)
			}
			sb.WriteString(fmt.Sprintf("%s %s\n", marker, label))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("───────────────────────────────────────────────────────────────\n")
	if output.CurrentStep != nil {
		sb.WriteString(fmt.Sprintf("Current step: %s\n\n", output.CurrentStep.ID))
		if output.CurrentStep.Instructions != "" {
			sb.WriteString("Instructions:\n")
			sb.WriteString(fmt.Sprintf("  %s\n", output.CurrentStep.Instructions))
		}
	}
	sb.WriteString("───────────────────────────────────────────────────────────────\n")

	return sb.String()
}

func formatPrompt(output *PrimeOutput) string {
	var sb strings.Builder

	if output.Workflow != nil {
		sb.WriteString(fmt.Sprintf("# Your Workflow: %s (step %d/%d)\n\n",
			output.Workflow.Name,
			output.Workflow.Completed+1,
			output.Workflow.Total))
	}

	if output.WorkBead != nil {
		sb.WriteString(fmt.Sprintf("**Work bead**: %s - %s\n\n", output.WorkBead.ID, output.WorkBead.Title))
	}

	if output.CurrentStep != nil {
		sb.WriteString(fmt.Sprintf("## Current Step: %s\n\n", output.CurrentStep.Title))
		if output.CurrentStep.Instructions != "" {
			sb.WriteString(output.CurrentStep.Instructions)
			sb.WriteString("\n\n")
		}
		sb.WriteString(fmt.Sprintf("When done, run: `meow close %s`\n", output.CurrentStep.ID))
	}

	return sb.String()
}
