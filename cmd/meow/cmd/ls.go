package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/spf13/cobra"
)

var (
	lsRunning bool
	lsJSON    bool
	lsAll     bool
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List workflows in this project",
	Long: `List all workflows in the .meow/workflows directory.

By default, shows running and recently completed workflows.
Use --all to include older workflows.`,
	RunE: runLs,
}

func init() {
	lsCmd.Flags().BoolVar(&lsRunning, "running", false, "show only running workflows")
	lsCmd.Flags().BoolVar(&lsJSON, "json", false, "output as JSON")
	lsCmd.Flags().BoolVar(&lsAll, "all", false, "include all workflows (not just recent)")
	rootCmd.AddCommand(lsCmd)
}

func runLs(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	workflowsDir := filepath.Join(dir, ".meow", "workflows")

	if _, err := os.Stat(workflowsDir); os.IsNotExist(err) {
		fmt.Println("No workflows found")
		return nil
	}

	store, err := orchestrator.NewYAMLWorkflowStore(workflowsDir)
	if err != nil {
		return fmt.Errorf("opening workflow store: %w", err)
	}

	filter := orchestrator.WorkflowFilter{}
	if lsRunning {
		filter.Status = types.WorkflowStatusRunning
	}

	workflows, err := store.List(context.Background(), filter)
	if err != nil {
		return fmt.Errorf("listing workflows: %w", err)
	}

	// Sort by StartedAt descending (most recent first)
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].StartedAt.After(workflows[j].StartedAt)
	})

	if len(workflows) == 0 {
		if lsRunning {
			fmt.Println("No running workflows")
		} else {
			fmt.Println("No workflows found")
		}
		return nil
	}

	if lsJSON {
		return printWorkflowsJSON(workflows, store)
	}

	return printWorkflowsTable(workflows, store)
}

func printWorkflowsTable(workflows []*types.Workflow, store *orchestrator.YAMLWorkflowStore) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tSTARTED\tTEMPLATE")

	for _, wf := range workflows {
		started := wf.StartedAt.Format("2006-01-02 15:04:05")
		template := filepath.Base(wf.Template)
		status := string(wf.Status)

		// Add stale indicator for non-running "running" workflows
		if wf.Status == types.WorkflowStatusRunning && !store.IsLocked(wf.ID) {
			status = "running (stale)"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", wf.ID, status, started, template)
	}

	return w.Flush()
}

func printWorkflowsJSON(workflows []*types.Workflow, store *orchestrator.YAMLWorkflowStore) error {
	type workflowJSON struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Stale     bool   `json:"stale,omitempty"`
		StartedAt string `json:"started_at"`
		Template  string `json:"template"`
	}

	out := make([]workflowJSON, len(workflows))
	for i, wf := range workflows {
		isStale := wf.Status == types.WorkflowStatusRunning && !store.IsLocked(wf.ID)
		out[i] = workflowJSON{
			ID:        wf.ID,
			Status:    string(wf.Status),
			Stale:     isStale,
			StartedAt: wf.StartedAt.Format(time.RFC3339),
			Template:  filepath.Base(wf.Template),
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
