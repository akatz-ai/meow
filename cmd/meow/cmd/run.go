package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <template>",
	Short: "Run a workflow from a template",
	Long: `Start a workflow by baking a template into beads and running the orchestrator.

The template is loaded from .meow/templates/<name>.toml and expanded into
beads in the .beads directory. The orchestrator then executes the workflow.`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
}

var (
	runDry bool
)

func init() {
	runCmd.Flags().BoolVar(&runDry, "dry-run", false, "validate and show what would be created without executing")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	template := args[0]

	if runDry {
		fmt.Printf("Would run template: %s (dry-run)\n", template)
		// TODO: Load and validate template, show baked beads
		return nil
	}

	fmt.Printf("Starting workflow from template: %s\n", template)
	// TODO: Implement actual workflow execution
	fmt.Println("(not yet implemented)")

	return nil
}
