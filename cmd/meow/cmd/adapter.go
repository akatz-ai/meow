package cmd

import (
	"github.com/spf13/cobra"
)

var adapterCmd = &cobra.Command{
	Use:   "adapter",
	Short: "Manage agent adapters",
	Long: `Manage agent adapters for MEOW orchestration.

Adapters define how to spawn, inject prompts into, and stop different
agent types (e.g., Claude Code, Aider).

Available subcommands:
  setup    Run an adapter's setup script for a worktree`,
}

func init() {
	rootCmd.AddCommand(adapterCmd)
}
