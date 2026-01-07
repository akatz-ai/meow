package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List agents",
	Long: `Display all agents and their current status.

Shows:
- Agent ID and name
- Status (active/stopped)
- Current bead assignment
- Last heartbeat`,
	RunE: runAgents,
}

var agentsFormat string

func init() {
	agentsCmd.Flags().StringVar(&agentsFormat, "format", "text", "output format: text, json")
	rootCmd.AddCommand(agentsCmd)
}

func runAgents(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	// TODO: Load agents from .meow/agents.json
	fmt.Println("Agents")
	fmt.Println("------")
	fmt.Println("(not yet implemented)")

	return nil
}
