package cmd

import (
	"github.com/spf13/cobra"
)

var adapterCmd = &cobra.Command{
	Use:   "adapter",
	Short: "Manage agent adapters",
	Long: `Manage agent adapters for MEOW orchestration.

Adapters encapsulate agent-specific behavior (how to start, stop, and inject prompts)
while keeping the orchestrator agent-agnostic.

Use subcommands to install, remove, list, and show adapter details.`,
}

func init() {
	rootCmd.AddCommand(adapterCmd)
}
