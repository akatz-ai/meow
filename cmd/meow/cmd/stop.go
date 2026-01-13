package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
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

	workflowsDir := filepath.Join(dir, ".meow", "workflows")
	store, err := orchestrator.NewYAMLWorkflowStore(workflowsDir)
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

	if wf.OrchestratorPID == 0 {
		return fmt.Errorf("workflow %s has no orchestrator PID (not running or crashed)", workflowID)
	}

	// CRITICAL: Validate this PID is actually our meow process
	if err := validateMeowProcess(wf.OrchestratorPID); err != nil {
		return fmt.Errorf("orchestrator not running: %w", err)
	}

	process, err := os.FindProcess(wf.OrchestratorPID)
	if err != nil {
		return fmt.Errorf("finding process %d: %w", wf.OrchestratorPID, err)
	}

	if err := process.Signal(syscall.SIGTERM); err != nil {
		if err == syscall.ESRCH {
			return fmt.Errorf("orchestrator process %d no longer exists", wf.OrchestratorPID)
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
