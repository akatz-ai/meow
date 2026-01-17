package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/meow-stack/meow-machine/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	// Version is set at build time via ldflags
	Version = "dev"

	// Global flags
	verbose bool
	workDir string
)

var rootCmd = &cobra.Command{
	Use:   "meow",
	Short: "MEOW Stack - Molecular Expression Of Work",
	Long: `MEOW is a durable, composable workflow system for AI agent orchestration.

The Makefile of agent orchestration. No Python. No cloud. No magic.
Just tmux, TOML, and a binary.

Built on 7 primitive executors (shell, spawn, kill, expand, branch, foreach, agent),
MEOW enables complex multi-agent workflows with crash recovery and context management.

For more information, see: https://github.com/meow-stack/meow-machine`,
	SilenceUsage:  true,
	SilenceErrors: true,
	Run: func(cmd *cobra.Command, args []string) {
		// When no subcommand is given, list available workflows (like `make` with no args)
		if err := listWorkflows(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVarP(&workDir, "workdir", "C", "", "working directory (default: current)")

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
