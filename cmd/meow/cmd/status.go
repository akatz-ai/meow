package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show current workflow status",
	Long: `Display the current state of the MEOW workflow.

Shows:
- Active workflow and template
- Running agents
- Current bead being executed
- Recent activity`,
	RunE: runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)
}

func runStatus(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	// TODO: Load orchestrator state and display
	fmt.Println("MEOW Status")
	fmt.Println("-----------")
	fmt.Println("(not yet implemented)")

	return nil
}
