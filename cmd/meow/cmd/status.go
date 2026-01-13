package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/status"
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/spf13/cobra"
)

// Exit codes for status command
const (
	ExitSuccess         = 0
	ExitNoWorkflows     = 1
	ExitWorkflowNotFound = 2
	ExitError           = 3
)

// Status command flags
var (
	statusJSON      bool
	statusWatch     bool
	statusInterval  time.Duration
	statusFilter    string
	statusAllSteps  bool
	statusAgents    bool
	statusQuiet     bool
	statusNoColor   bool
)

var statusCmd = &cobra.Command{
	Use:   "status [workflow-id]",
	Short: "Show workflow status",
	Long: `Display the current state of MEOW workflows.

With no arguments, shows a list of all workflows.
With a workflow ID, shows detailed status for that workflow.

Examples:
  meow status                       # List all workflows
  meow status wf-123                # Show detailed status for workflow
  meow status --status running      # List only running workflows
  meow status --json                # Output as JSON
  meow status --watch               # Refresh every 2s (default)
  meow status --watch --interval 5s # Refresh every 5s
  meow status --agents              # Focus on agent status`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)

	statusCmd.Flags().BoolVarP(&statusJSON, "json", "j", false, "Output as JSON")
	statusCmd.Flags().BoolVarP(&statusWatch, "watch", "w", false, "Watch mode - refresh periodically")
	statusCmd.Flags().DurationVarP(&statusInterval, "interval", "i", 2*time.Second, "Watch interval")
	statusCmd.Flags().StringVarP(&statusFilter, "status", "s", "", "Filter by status (running, done, failed, stopped)")
	statusCmd.Flags().BoolVar(&statusAllSteps, "all-steps", false, "Show all steps (not just running)")
	statusCmd.Flags().BoolVarP(&statusAgents, "agents", "a", false, "Focus on agent status")
	statusCmd.Flags().BoolVarP(&statusQuiet, "quiet", "q", false, "Minimal output")
	statusCmd.Flags().BoolVar(&statusNoColor, "no-color", false, "Disable colors")
}

func runStatus(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	ctx := context.Background()
	workflowsDir := filepath.Join(".meow", "workflows")

	// Create workflow store
	store, err := orchestrator.NewYAMLWorkflowStore(workflowsDir)
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
		return runStatusWatch(ctx, store, workflowID)
	}

	// Single display
	return displayStatus(ctx, store, workflowID)
}

func runStatusWatch(ctx context.Context, store *orchestrator.YAMLWorkflowStore, workflowID string) error {
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

func displayStatus(ctx context.Context, store *orchestrator.YAMLWorkflowStore, workflowID string) error {
	// If workflow ID specified, show detailed view
	if workflowID != "" {
		return displayWorkflowDetail(ctx, store, workflowID)
	}

	// Otherwise show list of workflows
	return displayWorkflowList(ctx, store)
}

func displayWorkflowDetail(ctx context.Context, store *orchestrator.YAMLWorkflowStore, workflowID string) error {
	wf, err := store.Get(ctx, workflowID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Workflow not found: %s\n", workflowID)
		os.Exit(ExitWorkflowNotFound)
	}

	summary := status.NewWorkflowSummary(wf)
	opts := status.FormatOptions{
		NoColor:  statusNoColor,
		AllSteps: statusAllSteps,
		Agents:   statusAgents,
		Quiet:    statusQuiet,
	}

	if statusJSON {
		data, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling JSON: %w", err)
		}
		fmt.Println(string(data))
	} else {
		output := status.FormatDetailedWorkflow(summary, opts)
		fmt.Print(output)
	}

	return nil
}

func displayWorkflowList(ctx context.Context, store *orchestrator.YAMLWorkflowStore) error {
	// Build filter
	filter := orchestrator.WorkflowFilter{}
	if statusFilter != "" {
		filter.Status = types.WorkflowStatus(statusFilter)
		if !filter.Status.Valid() {
			return fmt.Errorf("invalid status filter: %s (use: pending, running, done, failed, stopped)", statusFilter)
		}
	}

	workflows, err := store.List(ctx, filter)
	if err != nil {
		return fmt.Errorf("listing workflows: %w", err)
	}

	if len(workflows) == 0 {
		if statusFilter != "" {
			fmt.Printf("No workflows with status: %s\n", statusFilter)
		} else {
			fmt.Println("No workflows found.")
			fmt.Println("\nUse 'meow run <template>' to start a workflow.")
		}
		os.Exit(ExitNoWorkflows)
	}

	// If only one workflow and no filter, show detailed view
	if len(workflows) == 1 && statusFilter == "" {
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
		data, err := json.MarshalIndent(summaries, "", "  ")
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
