package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/akatz-ai/meow/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	// Version is set at build time via ldflags
	Version = "dev"

	// Global flags
	verbose bool
	workDir string

	// Workflow shorthand flags (meow <workflow> --var x=y)
	rootVars []string
)

var rootCmd = &cobra.Command{
	Use:   "meow [workflow]",
	Short: "MEOW Stack - Molecular Expression Of Work",
	Long: `MEOW is a durable, composable workflow system for AI agent orchestration.

The Makefile of agent orchestration. No Python. No cloud. No magic.
Just tmux, TOML, and a binary.

Built on 7 primitive executors (shell, spawn, kill, expand, branch, foreach, agent),
MEOW enables complex multi-agent workflows with crash recovery and context management.

For more information, see: https://github.com/akatz-ai/meow`,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// When no args, list available workflows (like `make` with no args)
		if len(args) == 0 {
			return listWorkflows()
		}

		// First arg might be a workflow name - try to run it
		workflowName := args[0]

		// Check if it's a known subcommand (subcommands take precedence)
		for _, sub := range cmd.Commands() {
			if sub.Name() == workflowName {
				// This is a subcommand, let cobra handle it
				// (This shouldn't happen as cobra routes subcommands before RunE)
				return cmd.Help()
			}
			for _, alias := range sub.Aliases {
				if alias == workflowName {
					return cmd.Help()
				}
			}
		}

		// Try to run it as a workflow
		return runWorkflowShorthand(workflowName, args[1:])
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVarP(&workDir, "workdir", "C", "", "working directory (default: current)")

	// Workflow shorthand flags (meow <workflow> --var x=y)
	rootCmd.Flags().StringArrayVar(&rootVars, "var", nil, "variable values for workflow (format: name=value)")

	// Version flag
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("meow {{.Version}}\n")
}

// checkWorkDir ensures we're in a MEOW project directory.
func checkWorkDir() error {
	dir := workDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
	}

	// Check for .meow directory
	meowDir := dir + "/.meow"
	if _, err := os.Stat(meowDir); os.IsNotExist(err) {
		return fmt.Errorf("not a MEOW project (no .meow directory).\n  Run 'meow init' for full setup, or 'meow init --minimal' for just runs/logs")
	}

	return nil
}

// getWorkDir returns the effective working directory.
func getWorkDir() (string, error) {
	if workDir != "" {
		return workDir, nil
	}
	return os.Getwd()
}

// checkBeadsDir ensures we have a .beads directory (for bead-only commands like prime, close).
// This is less strict than checkWorkDir - doesn't require full MEOW orchestrator setup.
func checkBeadsDir() error {
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	beadsDir := dir + "/.beads"
	if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
		return fmt.Errorf("no .beads directory found. Are you in a beads project?")
	}

	return nil
}

// listWorkflows lists all available workflows from all sources.
func listWorkflows() error {
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// Check if we're in a MEOW project - if not, that's OK, we just won't have project workflows
	meowDir := dir + "/.meow"
	inMeowProject := true
	if _, err := os.Stat(meowDir); os.IsNotExist(err) {
		inMeowProject = false
	}

	var projectDir string
	if inMeowProject {
		projectDir = dir
	}

	loader := workflow.NewLoader(projectDir)
	available, err := loader.ListAvailable()
	if err != nil {
		return fmt.Errorf("listing workflows: %w", err)
	}

	// Check if we have any workflows at all
	totalCount := 0
	for _, wfs := range available {
		totalCount += len(wfs)
	}

	if totalCount == 0 {
		if inMeowProject {
			fmt.Println("No workflows found.")
			fmt.Println()
			fmt.Println("Create a workflow in .meow/workflows/ to get started.")
		} else {
			fmt.Println("No workflows found.")
			fmt.Println()
			fmt.Println("Run 'meow init' to create a MEOW project, or")
			fmt.Println("add workflows to ~/.meow/workflows/ for global access.")
		}
		return nil
	}

	fmt.Println("Available workflows:")
	fmt.Println()

	// Define display order
	sourceOrder := []struct {
		key   string
		label string
		path  string
	}{
		{"project", "Project", ".meow/workflows/"},
		{"library", "Library", "lib/"},
		{"user", "User", "~/.meow/workflows/"},
		{"embedded", "Built-in", "<embedded>"},
	}

	for _, source := range sourceOrder {
		workflows := available[source.key]
		if len(workflows) == 0 {
			continue
		}

		// Sort workflows by name
		sort.Slice(workflows, func(i, j int) bool {
			return workflows[i].Name < workflows[j].Name
		})

		fmt.Printf("  %s (%s):\n", source.label, source.path)
		for _, wf := range workflows {
			if wf.Description != "" {
				fmt.Printf("    %-20s %s\n", wf.Name, wf.Description)
			} else {
				fmt.Printf("    %s\n", wf.Name)
			}
		}
		fmt.Println()
	}

	fmt.Println("Run: meow run <workflow> [--var key=value]")

	return nil
}

// runWorkflowShorthand runs a workflow using the shorthand syntax (meow <workflow>).
// This delegates to the run command with the workflow name.
func runWorkflowShorthand(workflowName string, extraArgs []string) error {
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// Check if the workflow exists
	loader := workflow.NewLoader(dir)
	_, err = loader.LoadWorkflow(workflowName)
	if err != nil {
		// Check if it's a WorkflowNotFoundError
		if _, ok := err.(*workflow.WorkflowNotFoundError); ok {
			return fmt.Errorf("unknown command or workflow: %s", workflowName)
		}
		return fmt.Errorf("loading workflow %q: %w", workflowName, err)
	}

	// Build args for run command: run <workflow> --var x=y ...
	args := []string{workflowName}

	// Add --var flags from root command
	for _, v := range rootVars {
		args = append(args, "--var", v)
	}

	// Add any extra args
	args = append(args, extraArgs...)

	// Execute the run command
	runCmd.SetArgs(args)
	return runCmd.Execute()
}
