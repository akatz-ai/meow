package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var handoffCmd = &cobra.Command{
	Use:   "handoff",
	Short: "Request a context refresh",
	Long: `Request the orchestrator to cycle the agent's session.

This is used when the agent is approaching context limits. The orchestrator
will save the session ID, stop the agent, and restart with --resume.`,
	RunE: runHandoff,
}

var handoffNotes string

func init() {
	handoffCmd.Flags().StringVar(&handoffNotes, "notes", "", "handoff notes for the next session")
	rootCmd.AddCommand(handoffCmd)
}

func runHandoff(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	// TODO: Signal orchestrator to initiate handoff
	fmt.Println("Requesting handoff...")
	if handoffNotes != "" {
		fmt.Printf("Notes for next session: %s\n", handoffNotes)
	}
	fmt.Println("(not yet implemented)")

	return nil
}
