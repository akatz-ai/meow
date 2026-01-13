package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/meow-stack/meow-machine/internal/config"
	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume <workflow-id>",
	Short: "Resume a crashed or interrupted workflow",
	Long: `Resume execution of a workflow that was interrupted.

This command handles crash recovery by:
1. Loading the workflow state from disk
2. Recovering interrupted steps (resetting orchestrator steps to pending,
   checking if agents are still alive, etc.)
3. Continuing execution from where it left off

Use this after the orchestrator crashed while running a workflow.`,
	Args: cobra.ExactArgs(1),
	RunE: runResume,
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}

func runResume(cmd *cobra.Command, args []string) error {
	workflowID := args[0]
	ctx := context.Background()

	// Get working directory
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// Load config (defaults + global + project)
	cfg, err := config.LoadFromDir(dir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Determine workflows directory - check MEOW_STATE_DIR env var first (used by E2E tests),
	// then fall back to default .meow/workflows
	workflowsDir := os.Getenv("MEOW_STATE_DIR")
	if workflowsDir != "" {
		workflowsDir = filepath.Join(workflowsDir, "workflows")
	} else {
		workflowsDir = filepath.Join(dir, ".meow", "workflows")
	}

	if _, err := os.Stat(workflowsDir); os.IsNotExist(err) {
		return fmt.Errorf("workflows directory not found: %s", workflowsDir)
	}

	// Create workflow store
	store, err := orchestrator.NewYAMLWorkflowStore(workflowsDir)
	if err != nil {
		return fmt.Errorf("opening workflow store: %w", err)
	}

	// Acquire per-workflow lock before resuming
	lock, err := store.AcquireWorkflowLock(workflowID)
	if err != nil {
		return fmt.Errorf("acquiring workflow lock: %w", err)
	}
	defer lock.Release()

	// Load the workflow
	wf, err := store.Get(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("loading workflow: %w", err)
	}

	// Check workflow status
	if wf.Status.IsTerminal() {
		return fmt.Errorf("workflow %s is already %s, cannot resume", workflowID, wf.Status)
	}

	if wf.DefaultAdapter == "" && cfg.Agent.DefaultAdapter != "" {
		wf.DefaultAdapter = cfg.Agent.DefaultAdapter
		if err := store.Save(ctx, wf); err != nil {
			return fmt.Errorf("saving workflow defaults: %w", err)
		}
	}

	fmt.Printf("Resuming workflow %s (status: %s)\n", workflowID, wf.Status)

	// Create logger
	logLevel := slog.LevelInfo
	if verbose {
		logLevel = slog.LevelDebug
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevel,
	}))

	// Create shell runner
	shellRunner := orchestrator.NewDefaultShellRunner()

	// Create agent manager for tmux sessions
	// Passing nil registry uses the default (project + global adapters)
	agentManager := orchestrator.NewTmuxAgentManager(dir, nil, logger)

	// Create template expander
	expander := orchestrator.NewTemplateExpanderAdapter(dir)

	// Create orchestrator with agent support
	orch := orchestrator.New(cfg, store, agentManager, shellRunner, expander, logger)
	orch.SetWorkflowID(workflowID)

	// Perform crash recovery
	fmt.Println("Performing crash recovery...")
	if err := orch.Recover(ctx); err != nil {
		return fmt.Errorf("crash recovery failed: %w", err)
	}

	// Reload workflow after recovery (state may have changed)
	wf, err = store.Get(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("reloading workflow after recovery: %w", err)
	}

	// If workflow is now terminal (e.g., cleanup was resumed and completed), exit
	if wf.Status.IsTerminal() {
		fmt.Printf("Workflow %s: %s (completed during recovery)\n", workflowID, wf.Status)
		return nil
	}

	// Store orchestrator PID for meow stop
	wf.OrchestratorPID = os.Getpid()
	if err := store.Save(ctx, wf); err != nil {
		return fmt.Errorf("saving workflow with PID: %w", err)
	}

	// Clear PID on exit (clean shutdown)
	defer func() {
		wf.OrchestratorPID = 0
		store.Save(ctx, wf)
	}()

	// Set up signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Println("\nReceived shutdown signal, stopping...")
		cancel()
	}()

	// Create IPC handler
	ipcHandler := orchestrator.NewIPCHandler(orch, store, agentManager, logger)

	// Start IPC server
	ipcServer := ipc.NewServer(workflowID, ipcHandler, logger)
	if err := ipcServer.StartAsync(ctx); err != nil {
		return fmt.Errorf("starting IPC server: %w", err)
	}
	defer ipcServer.Shutdown()

	fmt.Printf("\nResuming workflow execution...\n")
	if verbose {
		fmt.Printf("IPC socket: %s\n", ipcServer.Path())
	}

	// Run the orchestrator
	if err := orch.Run(ctx); err != nil {
		if err == context.Canceled {
			fmt.Println("Workflow cancelled.")
			return nil
		}
		return fmt.Errorf("running workflow: %w", err)
	}

	// Reload workflow to get final state
	wf, err = store.Get(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("getting final workflow state: %w", err)
	}

	// Print final status
	fmt.Printf("\nWorkflow %s: %s\n", workflowID, wf.Status)
	if verbose || wf.Status == types.WorkflowStatusFailed {
		fmt.Println("\nStep results:")
		for _, step := range wf.Steps {
			status := string(step.Status)
			if step.Error != nil {
				status = fmt.Sprintf("%s (error: %s)", status, step.Error.Message)
			}
			fmt.Printf("  %s: %s\n", step.ID, status)
			if verbose && len(step.Outputs) > 0 {
				for k, v := range step.Outputs {
					fmt.Printf("    %s: %v\n", k, v)
				}
			}
		}
	}

	return nil
}
