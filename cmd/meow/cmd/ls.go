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
	lsAll    bool
	lsStale  bool
	lsStatus string
	lsJSON   bool
)

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List workflows in this project",
	Long: `List runs in the .meow/runs directory.

By default, shows only actively running runs (lock held by orchestrator).
Use --all to see all runs, or --status to filter by status.

Examples:
  meow ls              # Active workflows only
  meow ls -a           # All workflows
  meow ls --stale      # Stale workflows (running but no lock)
  meow ls --status=done  # Completed workflows`,
	RunE: runLs,
}

func init() {
	lsCmd.Flags().BoolVarP(&lsAll, "all", "a", false, "show all workflows (not just active)")
	lsCmd.Flags().BoolVar(&lsStale, "stale", false, "show stale workflows (running but no lock)")
	lsCmd.Flags().StringVar(&lsStatus, "status", "", "filter by status (running, stopped, done, failed, pending)")
	lsCmd.Flags().BoolVar(&lsJSON, "json", false, "output as JSON")
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

	runsDir := filepath.Join(dir, ".meow", "runs")

	if _, err := os.Stat(runsDir); os.IsNotExist(err) {
		fmt.Println("No runs found")
		return nil
	}

	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		return fmt.Errorf("opening workflow store: %w", err)
	}

	// Apply status filter if specified
	filter := orchestrator.RunFilter{}
	if lsStatus != "" {
		filter.Status = types.RunStatus(lsStatus)
	}

	workflows, err := store.List(context.Background(), filter)
	if err != nil {
		return fmt.Errorf("listing workflows: %w", err)
	}

	// Sort by StartedAt descending (most recent first)
	sort.Slice(workflows, func(i, j int) bool {
		return workflows[i].StartedAt.After(workflows[j].StartedAt)
	})

	// Apply post-filter based on flags
	// Default: only actively running (running + locked)
	// --all: show everything
	// --stale: show stale (running but no lock)
	// --status=X: already filtered above
	if !lsAll && lsStatus == "" {
		filtered := make([]*types.Run, 0, len(workflows))
		for _, wf := range workflows {
			isLocked := store.IsLocked(wf.ID)
			isRunning := wf.Status == types.RunStatusRunning
			isStale := isRunning && !isLocked

			if lsStale {
				// --stale: show only stale workflows
				if isStale {
					filtered = append(filtered, wf)
				}
			} else {
				// Default: show only actively running (running + locked)
				if isRunning && isLocked {
					filtered = append(filtered, wf)
				}
			}
		}
		workflows = filtered
	}

	if len(workflows) == 0 {
		if lsStale {
			fmt.Println("No stale workflows")
		} else if lsStatus != "" {
			fmt.Printf("No %s workflows\n", lsStatus)
		} else if lsAll {
			fmt.Println("No workflows found")
		} else {
			fmt.Println("No active workflows")
		}
		return nil
	}

	if lsJSON {
		return printWorkflowsJSON(workflows, store)
	}

	return printWorkflowsTable(workflows, store)
}

func printWorkflowsTable(workflows []*types.Run, store *orchestrator.YAMLRunStore) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tSTATUS\tSTARTED\tTEMPLATE")

	for _, wf := range workflows {
		started := wf.StartedAt.Format("2006-01-02 15:04:05")
		template := filepath.Base(wf.Template)
		status := string(wf.Status)

		// Add stale indicator for non-running "running" workflows
		if wf.Status == types.RunStatusRunning && !store.IsLocked(wf.ID) {
			status = "running (stale)"
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", wf.ID, status, started, template)
	}

	return w.Flush()
}

func printWorkflowsJSON(workflows []*types.Run, store *orchestrator.YAMLRunStore) error {
	type workflowJSON struct {
		ID        string `json:"id"`
		Status    string `json:"status"`
		Stale     bool   `json:"stale,omitempty"`
		StartedAt string `json:"started_at"`
		Template  string `json:"template"`
	}

	out := make([]workflowJSON, len(workflows))
	for i, wf := range workflows {
		isStale := wf.Status == types.RunStatusRunning && !store.IsLocked(wf.ID)
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
