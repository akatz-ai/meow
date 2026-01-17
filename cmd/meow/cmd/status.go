package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/akatz-ai/meow/internal/orchestrator"
	"github.com/akatz-ai/meow/internal/status"
	"github.com/akatz-ai/meow/internal/types"
	"github.com/spf13/cobra"
)

// Exit codes for status command
const (
	ExitSuccess          = 0
	ExitNoWorkflows      = 1
	ExitWorkflowNotFound = 2
)

// StatusExitError represents an error with a specific exit code.
type StatusExitError struct {
	Code    int
	Message string
}

func (e *StatusExitError) Error() string {
	return e.Message
}

// Status command flags
var (
	statusJSON      bool
	statusWatch     bool
	statusInterval  time.Duration
	statusFilter    string
	statusAll       bool
	statusAllSteps  bool
	statusAgents    bool
	statusQuiet     bool
	statusNoColor   bool
	statusStrict    bool
)

var statusCmd = &cobra.Command{
	Use:   "status [workflow-id]",
	Short: "Show workflow status",
	Long: `Display the current state of MEOW workflows.

By default, shows only actively running workflows (with lock held).
If exactly one workflow is active, shows its detailed status automatically.

With a workflow ID argument, shows detailed status for that workflow.

Examples:
  meow status                       # Show active workflow(s)
  meow status wf-123                # Show detailed status for workflow
  meow status -a                    # Show all workflows
  meow status --filter=done         # List only completed workflows
  meow status --json                # Output as JSON
  meow status --watch               # Refresh every 2s (default)
  meow status --agents              # Focus on agent status`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)

	statusCmd.Flags().BoolVarP(&statusJSON, "json", "j", false, "Output as JSON")
	statusCmd.Flags().BoolVarP(&statusWatch, "watch", "w", false, "Watch mode - refresh periodically")
	statusCmd.Flags().DurationVarP(&statusInterval, "interval", "i", 2*time.Second, "Watch interval")
	statusCmd.Flags().StringVar(&statusFilter, "filter", "", "Filter by status (running, done, failed, stopped)")
	statusCmd.Flags().BoolVarP(&statusAll, "all", "a", false, "Show all workflows (not just active)")
	statusCmd.Flags().BoolVar(&statusAllSteps, "all-steps", false, "Show all steps (not just running)")
	statusCmd.Flags().BoolVar(&statusAgents, "agents", false, "Focus on agent status")
	statusCmd.Flags().BoolVarP(&statusQuiet, "quiet", "q", false, "Minimal output")
	statusCmd.Flags().BoolVar(&statusNoColor, "no-color", false, "Disable colors")
	statusCmd.Flags().BoolVar(&statusStrict, "strict", false, "Exit non-zero when no workflows match (for scripts)")
}

func runStatus(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	ctx := context.Background()
	runsDir := filepath.Join(".meow", "runs")

	// Create run store
	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		return fmt.Errorf("creating workflow store: %w", err)
	}
	defer store.Close()

	// Determine which workflow(s) to show
	var workflowID string
	if len(args) > 0 {
		workflowID = args[0]
	}

	// Watch mode loop
	if statusWatch {
		err := runStatusWatch(ctx, store, workflowID)
		if exitErr, ok := err.(*StatusExitError); ok {
			fmt.Fprintln(os.Stderr, exitErr.Message)
			os.Exit(exitErr.Code)
		}
		return err
	}

	// Single display
	err = displayStatus(ctx, store, workflowID)
	if exitErr, ok := err.(*StatusExitError); ok {
		fmt.Fprintln(os.Stderr, exitErr.Message)
		os.Exit(exitErr.Code)
	}
	return err
}

func runStatusWatch(ctx context.Context, store *orchestrator.YAMLRunStore, workflowID string) error {
	ticker := time.NewTicker(statusInterval)
	defer ticker.Stop()

	// Clear screen before first display
	fmt.Print("\033[H\033[2J")

	for {
		// Move cursor to top
		fmt.Print("\033[H")

		if err := displayStatus(ctx, store, workflowID); err != nil {
			return err
		}

		fmt.Printf("\n[Refreshing every %s, press Ctrl+C to stop]\n", statusInterval)

		select {
		case <-ticker.C:
			continue
		case <-ctx.Done():
			return nil
		}
	}
}

func displayStatus(ctx context.Context, store *orchestrator.YAMLRunStore, workflowID string) error {
	// If workflow ID specified, show detailed view
	if workflowID != "" {
		return displayWorkflowDetail(ctx, store, workflowID)
	}

	// Otherwise show list of workflows
	return displayWorkflowList(ctx, store)
}

func displayWorkflowDetail(ctx context.Context, store *orchestrator.YAMLRunStore, workflowID string) error {
	wf, err := store.Get(ctx, workflowID)
	if err != nil {
		return &StatusExitError{Code: ExitWorkflowNotFound, Message: fmt.Sprintf("workflow not found: %s", workflowID)}
	}

	// Check if this workflow is orphaned (running but no lock)
	isOrphaned := wf.Status == types.RunStatusRunning && !store.IsLocked(wf.ID)

	summary := status.NewWorkflowSummary(wf)
	opts := status.FormatOptions{
		NoColor:  statusNoColor,
		AllSteps: statusAllSteps,
		Agents:   statusAgents,
		Quiet:    statusQuiet,
	}

	if statusJSON {
		// Include orphaned status in JSON
		type jsonOutput struct {
			*status.WorkflowSummary
			Orphaned bool `json:"orphaned,omitempty"`
		}
		output := jsonOutput{WorkflowSummary: summary, Orphaned: isOrphaned}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(data))
	} else {
		// Show orphaned warning before detailed output
		if isOrphaned {
			fmt.Println("⚠ WARNING: This workflow is orphaned (no orchestrator running)")
			fmt.Printf("  Run 'meow resume %s' to recover, or 'meow stop %s' to clean up.\n\n", wf.ID, wf.ID)
		}
		output := status.FormatDetailedWorkflow(summary, opts)
		fmt.Print(output)
	}

	return nil
}

func displayWorkflowList(ctx context.Context, store *orchestrator.YAMLRunStore) error {
	// Build filter
	filter := orchestrator.RunFilter{}
	if statusFilter != "" {
		filter.Status = types.RunStatus(statusFilter)
		if !filter.Status.Valid() {
			return fmt.Errorf("invalid status filter: %s (use: pending, running, done, failed, stopped)", statusFilter)
		}
	}

	workflows, err := store.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("listing workflows: %w", err)
	}

	// Detect orphaned runs (running but no lock) - show these regardless of filters
	var orphaned []*types.Run
	if statusFilter == "" || statusFilter == "running" {
		for _, wf := range workflows {
			if wf.Status == types.RunStatusRunning && !store.IsLocked(wf.ID) {
				orphaned = append(orphaned, wf)
			}
		}
	}

	// Build a set of orphaned IDs for exclusion from main list
	orphanedIDs := make(map[string]bool)
	for _, wf := range orphaned {
		orphanedIDs[wf.ID] = true
	}

	// Apply active filter by default (unless --all or --filter specified)
	// Active = status=running AND lock held
	// Also exclude orphaned runs from the main list to avoid double-counting
	if !statusAll && statusFilter == "" {
		active := make([]*types.Run, 0, len(workflows))
		for _, wf := range workflows {
			if wf.Status == types.RunStatusRunning && store.IsLocked(wf.ID) {
				active = append(active, wf)
			}
		}
		workflows = active
	} else if statusFilter == "running" {
		// When filtering by running, exclude orphaned from main list
		filtered := make([]*types.Run, 0, len(workflows))
		for _, wf := range workflows {
			if !orphanedIDs[wf.ID] {
				filtered = append(filtered, wf)
			}
		}
		workflows = filtered
	}

	// Show orphaned runs first (they need attention)
	if len(orphaned) > 0 && !statusJSON {
		fmt.Println("⚠ Orphaned Workflows (no orchestrator):")
		fmt.Println()
		for _, wf := range orphaned {
			fmt.Printf("  %-12s %-20s RUNNING (orphaned)   Started: %s\n",
				wf.ID, filepath.Base(wf.Template), wf.StartedAt.Format("15:04"))
		}
		fmt.Println()
		if len(orphaned) == 1 {
			fmt.Printf("  Run 'meow resume %s' to recover, or 'meow stop %s' to clean up.\n", orphaned[0].ID, orphaned[0].ID)
		} else {
			fmt.Println("  Run 'meow resume <id>' to recover, or 'meow stop <id>' to clean up.")
		}
		fmt.Println()
	}

	if len(workflows) == 0 && len(orphaned) == 0 {
		if statusStrict {
			// Script mode: exit non-zero when nothing matches
			if statusFilter != "" {
				return &StatusExitError{Code: ExitNoWorkflows, Message: fmt.Sprintf("no workflows with status: %s", statusFilter)}
			} else if statusAll {
				return &StatusExitError{Code: ExitNoWorkflows, Message: "no workflows found"}
			} else {
				return &StatusExitError{Code: ExitNoWorkflows, Message: "no active workflows"}
			}
		}

		// Human-friendly mode: exit 0 with helpful message
		if statusFilter != "" {
			fmt.Printf("No workflows with status: %s\n\n", statusFilter)
			fmt.Println("Run 'meow status -a' to see all workflows.")
		} else if statusAll {
			fmt.Println("No workflows found.")
			fmt.Println()
			fmt.Println("Run 'meow run <template>' to start a workflow.")
		} else {
			fmt.Println("No active workflows.")
			fmt.Println()
			fmt.Println("Run 'meow run <template>' to start a workflow.")
			fmt.Println("Run 'meow status -a' to see all workflows.")
		}
		return nil
	}

	// If no active workflows but we have orphaned ones
	if len(workflows) == 0 && len(orphaned) > 0 {
		if statusJSON {
			// Output orphaned runs as JSON
			type jsonOutput struct {
				Orphaned []*status.WorkflowSummary `json:"orphaned"`
				Active   []*status.WorkflowSummary `json:"active"`
			}
			orphanedSummaries := make([]*status.WorkflowSummary, len(orphaned))
			for i, wf := range orphaned {
				orphanedSummaries[i] = status.NewWorkflowSummary(wf)
			}
			output := jsonOutput{Orphaned: orphanedSummaries, Active: []*status.WorkflowSummary{}}
			data, err := json.MarshalIndent(output, "", "  ")
			if err != nil {
				return fmt.Errorf("marshaling JSON: %w", err)
			}
			fmt.Println(string(data))
		}
		return nil
	}

	// If only one workflow, show detailed view automatically
	if len(workflows) == 1 && len(orphaned) == 0 {
		return displayWorkflowDetail(ctx, store, workflows[0].ID)
	}

	// Show list view
	summaries := make([]*status.WorkflowSummary, len(workflows))
	for i, wf := range workflows {
		summaries[i] = status.NewWorkflowSummary(wf)
	}

	opts := status.FormatOptions{
		NoColor: statusNoColor,
		Quiet:   statusQuiet,
	}

	if statusJSON {
		// Include orphaned runs in JSON output
		type jsonOutput struct {
			Orphaned []*status.WorkflowSummary `json:"orphaned,omitempty"`
			Active   []*status.WorkflowSummary `json:"active"`
		}
		output := jsonOutput{Active: summaries}
		if len(orphaned) > 0 {
			orphanedSummaries := make([]*status.WorkflowSummary, len(orphaned))
			for i, wf := range orphaned {
				orphanedSummaries[i] = status.NewWorkflowSummary(wf)
			}
			output.Orphaned = orphanedSummaries
		}
		data, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(data))
	} else {
		output := status.FormatWorkflowList(summaries, opts)
		fmt.Print(output)
	}

	return nil
}
