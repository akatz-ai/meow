package cmd

import (
	"github.com/spf13/cobra"
)

var adapterCmd = &cobra.Command{
	Use:   "adapter",
	Short: "Manage agent adapters",
	Long: `Manage agent adapters that define how to start, stop, and inject prompts into agents.

Adapters encapsulate agent-specific runtime behavior, allowing MEOW to orchestrate
different types of agents (Claude Code, Aider, etc.) through a common interface.

Adapters can come from three locations (in priority order):
  1. Project-local: .meow/adapters/<name>/adapter.toml
  2. Global:        ~/.meow/adapters/<name>/adapter.toml
  3. Built-in:      Compiled into the MEOW binary

Project adapters override global ones, which override built-in ones.`,
}

func init() {
	rootCmd.AddCommand(adapterCmd)
}
