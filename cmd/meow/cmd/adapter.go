package cmd

import (
	"github.com/spf13/cobra"
)

var adapterCmd = &cobra.Command{
	Use:   "adapter",
	Short: "Manage agent adapters",
	Long: `Manage adapters that define how MEOW interacts with different AI agents.

Adapters encapsulate agent-specific behavior:
  - How to start and stop the agent
  - How to inject prompts into the agent's tmux session
  - Environment variables for the agent

Adapters are installed to ~/.meow/adapters/<name>/.`,
}

func init() {
	rootCmd.AddCommand(adapterCmd)
}
