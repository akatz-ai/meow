package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/meow-stack/meow-machine/internal/cli"
	"github.com/meow-stack/meow-machine/internal/config"
	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/meow-stack/meow-machine/internal/workflow"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <template>",
	Short: "Run a workflow from a template",
	Long: `Start a workflow by baking a template into steps and running the orchestrator.

The template is loaded and baked into steps which are persisted in
.meow/runs/<id>.yaml. The orchestrator then executes the run.

Use -d/--detach to run in background mode. The orchestrator will run
as a daemon and you can use 'meow status' to check progress and
'meow stop' to stop it.

Examples:
  meow run workflow.toml              # Run in foreground
  meow run workflow.toml -d           # Run in background
  meow run workflow.toml --var x=y    # Pass variables`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
}

var (
	runDry           bool
	runDetach        bool
	runDetachedChild bool   // internal flag for child process
	runWorkflowID    string // internal: workflow ID for detached child
	runVars          []string
	runWorkflow      string
	runYes           bool
)

func init() {
	runCmd.Flags().BoolVar(&runDry, "dry-run", false, "validate and show what would be created without executing")
	runCmd.Flags().BoolVarP(&runDetach, "detach", "d", false, "run in background (detached mode)")
	runCmd.Flags().BoolVar(&runDetachedChild, "_detached-child", false, "internal: running as detached child")
	runCmd.Flags().StringVar(&runWorkflowID, "_workflow-id", "", "internal: workflow ID for detached child")
	runCmd.Flags().MarkHidden("_detached-child")
	runCmd.Flags().MarkHidden("_workflow-id")
	runCmd.Flags().StringArrayVar(&runVars, "var", nil, "variable values (format: name=value)")
	runCmd.Flags().StringVar(&runWorkflow, "workflow", "main", "workflow name to run (default: main)")
	runCmd.Flags().BoolVarP(&runYes, "yes", "y", false, "auto-confirm prompts (create .meow/ if missing)")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	templateRef := args[0]
	workflowRef := templateRef
	workflowName := runWorkflow
	ctx := context.Background()

	// Get working directory
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// Check if .meow/ exists, offer to create if missing
	meowDir := filepath.Join(dir, ".meow")
	if _, err := os.Stat(meowDir); os.IsNotExist(err) {
		if !runYes {
			confirmed, err := cli.Confirm("No .meow/ directory found. Create one?", true)
			if err != nil {
				return fmt.Errorf("prompt failed: %w", err)
			}
			if !confirmed {
				return fmt.Errorf("no .meow/ directory. Run 'meow init' or 'meow init --minimal' first")
			}
		}
		// Create minimal structure
		if err := initMinimalFromRun(dir); err != nil {
			return fmt.Errorf("initializing .meow/: %w", err)
		}
		fmt.Println() // blank line before workflow output
	}

	// Load config (defaults + global + project)
	cfg, err := config.LoadFromDir(dir)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Ensure runs directory exists
	runsDir := cfg.RunsDir(dir)
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		return fmt.Errorf("creating runs dir: %w", err)
	}

	fileRef := templateRef
	if strings.Contains(templateRef, "#") {
		parts := strings.SplitN(templateRef, "#", 2)
		fileRef = strings.TrimSpace(parts[0])
		workflowName = strings.TrimSpace(parts[1])
		if fileRef == "" || workflowName == "" {
			return fmt.Errorf("invalid workflow reference: %s", templateRef)
		}
		if cmd.Flags().Changed("workflow") {
			return fmt.Errorf("workflow name provided in both --workflow and %s", templateRef)
		}
	}

	var module *workflow.Module
	var templatePath string
	isExplicitPath := filepath.IsAbs(fileRef) || strings.HasPrefix(fileRef, ".") || strings.HasSuffix(fileRef, ".toml")

	if isExplicitPath {
		templatePath = fileRef
		if !filepath.IsAbs(templatePath) {
			templatePath = filepath.Join(dir, templatePath)
		}
		if !strings.HasSuffix(templatePath, ".toml") {
			if _, err := os.Stat(templatePath); os.IsNotExist(err) {
				templatePath = templatePath + ".meow.toml"
			}
		}
		if _, err := os.Stat(templatePath); err != nil {
			return fmt.Errorf("template file not found: %s", templatePath)
		}

		module, err = workflow.ParseModuleFile(templatePath)
		if err != nil {
			return fmt.Errorf("parsing template: %w", err)
		}
	} else {
		loader := workflow.NewLoader(dir)
		loaded, err := loader.LoadWorkflow(workflowRef)
		if err != nil {
			return fmt.Errorf("resolving workflow %q: %w", workflowRef, err)
		}
		templatePath = loaded.Path
		module = loaded.Module
		if !cmd.Flags().Changed("workflow") {
			workflowName = loaded.Name
		}
	}

	// Parse variables from flags
	vars := make(map[string]string)
	for _, v := range runVars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid variable format: %s (expected name=value)", v)
		}
		vars[parts[0]] = parts[1]
	}

	// Get workflow (use flag or default to "main")
	templateWorkflow := module.GetWorkflow(workflowName)
	if templateWorkflow == nil {
		// List available workflows for better error message
		var available []string
		for name := range module.Workflows {
			available = append(available, name)
		}
		return fmt.Errorf("workflow %q not found in template. Available: %v", workflowName, available)
	}

	// Generate a unique workflow ID (or use passed ID for detached child)
	workflowID := runWorkflowID
	if workflowID == "" {
		workflowID = fmt.Sprintf("run-%d", time.Now().UnixNano())
	}

	// Create baker
	baker := workflow.NewBaker(workflowID)

	// Bake the workflow into steps
	result, err := baker.BakeWorkflow(templateWorkflow, vars)
	if err != nil {
		return fmt.Errorf("baking workflow: %w", err)
	}

	if runDry {
		fmt.Printf("Would create workflow with %d steps from template: %s (workflow: %s)\n", len(result.Steps), templatePath, workflowName)
		fmt.Printf("Workflow ID: %s\n", result.WorkflowID)
		fmt.Println()
		for _, step := range result.Steps {
			fmt.Printf("  %s [%s]\n", step.ID, step.Executor)
			if len(step.Needs) > 0 {
				fmt.Printf("    needs: %v\n", step.Needs)
			}
		}
		return nil
	}

	// Handle detached mode: spawn child process and exit
	if runDetach && !runDetachedChild {
		return spawnDetachedOrchestrator(cfg, dir, templatePath, workflowID, workflowName)
	}

	// Create a Workflow object
	wf := types.NewRun(workflowID, templatePath, vars)
	if wf.DefaultAdapter == "" && cfg.Agent.DefaultAdapter != "" {
		wf.DefaultAdapter = cfg.Agent.DefaultAdapter
	}

	// Copy conditional cleanup scripts from template (opt-in cleanup)
	wf.CleanupOnSuccess = templateWorkflow.CleanupOnSuccess
	wf.CleanupOnFailure = templateWorkflow.CleanupOnFailure
	wf.CleanupOnStop = templateWorkflow.CleanupOnStop

	// Add all steps to the workflow
	for _, step := range result.Steps {
		if err := wf.AddStep(step); err != nil {
			return fmt.Errorf("adding step %s: %w", step.ID, err)
		}
	}

	// Create workflow store
	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		return fmt.Errorf("opening workflow store: %w", err)
	}

	// Acquire per-workflow lock before persisting
	// This prevents multiple orchestrators from running the same workflow
	lock, err := store.AcquireWorkflowLock(workflowID)
	if err != nil {
		return fmt.Errorf("acquiring workflow lock: %w", err)
	}
	defer lock.Release()

	// Persist the workflow
	if err := store.Create(ctx, wf); err != nil {
		return fmt.Errorf("creating workflow: %w", err)
	}

	// Output success
	fmt.Printf("Created workflow with %d steps from template: %s\n", len(result.Steps), filepath.Base(templatePath))
	fmt.Printf("Workflow ID: %s\n", result.WorkflowID)
	if verbose {
		fmt.Println("\nSteps created:")
		for _, step := range result.Steps {
			fmt.Printf("  %s [%s]\n", step.ID, step.Executor)
		}
	}

	// Start the workflow
	wf.Start()
	if err := store.Save(ctx, wf); err != nil {
		return fmt.Errorf("starting workflow: %w", err)
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

	// Create IPC handler (with orchestrator reference for thread-safe state mutations)
	ipcHandler := orchestrator.NewIPCHandler(orch, store, agentManager, logger)

	// Start IPC server
	ipcServer := ipc.NewServer(workflowID, ipcHandler, logger)
	if err := ipcServer.StartAsync(ctx); err != nil {
		return fmt.Errorf("starting IPC server: %w", err)
	}
	defer ipcServer.Shutdown()

	fmt.Printf("\nRunning workflow...\n")
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
	if verbose || wf.Status == types.RunStatusFailed {
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

// spawnDetachedOrchestrator spawns a child process to run the workflow in background
func spawnDetachedOrchestrator(cfg *config.Config, dir, templatePath, workflowID, workflowName string) error {
	// Build command args for the child process
	args := []string{"run", templatePath, "--_detached-child", "--_workflow-id", workflowID, "--workflow", workflowName}
	for _, v := range runVars {
		args = append(args, "--var", v)
	}
	if verbose {
		args = append(args, "--verbose")
	}

	// Find the executable path
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding executable: %w", err)
	}

	// Create log file for output
	logDir := cfg.LogsDir(dir)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("creating log dir: %w", err)
	}
	logPath := filepath.Join(logDir, workflowID+".log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("creating log file: %w", err)
	}

	// Create the child process
	cmd := exec.Command(executable, args...)
	cmd.Dir = dir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil

	// Detach from parent process group
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true, // Create new session (fully detach)
	}

	// Start the child process
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return fmt.Errorf("starting detached process: %w", err)
	}

	// Don't wait for the child - let it run independently
	// The child will manage its own cleanup

	fmt.Printf("Started workflow in background\n")
	fmt.Printf("  ID:  %s\n", workflowID)
	fmt.Printf("  PID: %d\n", cmd.Process.Pid)
	fmt.Printf("  Log: %s\n", logPath)
	fmt.Println()
	fmt.Printf("Use 'meow status %s' to check progress\n", workflowID)
	fmt.Printf("Use 'meow stop %s' to stop the workflow\n", workflowID)

	return nil
}

// initMinimalFromRun creates only the minimal .meow structure needed for running workflows.
func initMinimalFromRun(dir string) error {
	meowDir := filepath.Join(dir, ".meow")
	for _, subdir := range []string{"runs", "logs"} {
		if err := os.MkdirAll(filepath.Join(meowDir, subdir), 0755); err != nil {
			return err
		}
	}
	fmt.Println("Created .meow/runs/ and .meow/logs/")
	return nil
}
