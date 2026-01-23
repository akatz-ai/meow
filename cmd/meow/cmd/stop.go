package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/akatz-ai/meow/internal/orchestrator"
	"github.com/akatz-ai/meow/internal/types"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop <workflow-id>",
	Short: "Stop a running workflow",
	Long: `Stop a running workflow by sending SIGTERM to its orchestrator.

The orchestrator will run any cleanup_on_stop script defined in the template,
then mark the workflow as stopped.`,
	Args: cobra.ExactArgs(1),
	RunE: runStop,
}

func init() {
	rootCmd.AddCommand(stopCmd)
}

func runStop(cmd *cobra.Command, args []string) error {
	workflowID := args[0]
	ctx := context.Background()

	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	runsDir := filepath.Join(dir, ".meow", "runs")
	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		return fmt.Errorf("opening workflow store: %w", err)
	}

	wf, err := store.Get(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	if wf.Status.IsTerminal() {
		return fmt.Errorf("workflow %s is already %s", workflowID, wf.Status)
	}

	// Check if workflow is orphaned (no PID or process gone)
	isOrphaned := false
	var orphanReason string

	if wf.OrchestratorPID == 0 {
		isOrphaned = true
		orphanReason = "no orchestrator PID recorded"
	} else if err := validateMeowProcess(wf.OrchestratorPID); err != nil {
		isOrphaned = true
		orphanReason = err.Error()
	}

	// Handle orphaned workflow: mark as stopped directly
	if isOrphaned {
		fmt.Printf("Workflow %s is orphaned (%s)\n", workflowID, orphanReason)
		fmt.Println("Marking workflow as stopped...")

		wf.Status = types.RunStatusStopped
		now := time.Now()
		wf.DoneAt = &now
		wf.OrchestratorPID = 0

		if err := store.Save(ctx, wf); err != nil {
			return fmt.Errorf("saving workflow state: %w", err)
		}

		fmt.Printf("✓ Workflow %s marked as stopped\n", workflowID)
		fmt.Println("Run 'meow cleanup' to clean up any remaining resources (tmux sessions, worktrees)")
		return nil
	}

	// Normal case: send SIGTERM to running orchestrator
	process, err := os.FindProcess(wf.OrchestratorPID)
	if err != nil {
		return fmt.Errorf("finding process %d: %w", wf.OrchestratorPID, err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		if err == syscall.ESRCH {
			// Process gone - treat as orphaned
			fmt.Printf("Orchestrator process %d no longer exists\n", wf.OrchestratorPID)
			fmt.Println("Marking workflow as stopped...")

			wf.Status = types.RunStatusStopped
			now := time.Now()
			wf.DoneAt = &now
			wf.OrchestratorPID = 0

			if err := store.Save(ctx, wf); err != nil {
				return fmt.Errorf("saving workflow state: %w", err)
			}

			fmt.Printf("✓ Workflow %s marked as stopped\n", workflowID)
			fmt.Println("Run 'meow cleanup' to clean up any remaining resources (tmux sessions, worktrees)")
			return nil
		}
		if err == syscall.EPERM {
			return fmt.Errorf("permission denied to stop orchestrator (PID %d)", wf.OrchestratorPID)
		}
		return fmt.Errorf("sending signal: %w", err)
	}

	fmt.Printf("Sent stop signal to workflow %s (PID %d)\n", workflowID, wf.OrchestratorPID)
	fmt.Println("Workflow will stop after running any cleanup_on_stop script")
	return nil
}

// validateMeowProcess checks that the PID is actually a meow process.
// This prevents killing wrong processes if PIDs are reused.
func validateMeowProcess(pid int) error {
	cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("process %d does not exist", pid)
		}
		return fmt.Errorf("reading process info: %w", err)
	}

	// cmdline is null-separated, check if "meow" appears in the command
	if !strings.Contains(string(cmdline), "meow") {
		return fmt.Errorf("process %d is not a meow process", pid)
	}

	return nil
}
