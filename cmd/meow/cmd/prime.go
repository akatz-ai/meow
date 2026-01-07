package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var primeCmd = &cobra.Command{
	Use:   "prime",
	Short: "Show the next task for an agent",
	Long: `Display the next task assigned to the current agent.

This command is typically called by Claude Code at the start of a session
to see what work is assigned. It shows:
- The bead ID and title
- Full instructions
- Required outputs (if any)
- Dependencies and context`,
	RunE: runPrime,
}

var (
	primeAgent  string
	primeFormat string
)

func init() {
	primeCmd.Flags().StringVar(&primeAgent, "agent", "", "agent ID (default: from MEOW_AGENT env)")
	primeCmd.Flags().StringVar(&primeFormat, "format", "text", "output format: text, json")
	rootCmd.AddCommand(primeCmd)
}

func runPrime(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	// TODO: Get agent from flag or env
	// TODO: Find assigned bead for this agent
	// TODO: Display bead details

	fmt.Println("=== YOUR CURRENT TASK ===")
	fmt.Println("(not yet implemented)")
	fmt.Println()
	fmt.Println("No tasks assigned to this agent.")

	return nil
}
