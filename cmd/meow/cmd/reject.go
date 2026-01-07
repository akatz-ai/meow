package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var rejectCmd = &cobra.Command{
	Use:   "reject [bead-id]",
	Short: "Reject a gate bead",
	Long: `Reject a blocking gate bead. The workflow will take the on_false branch.

If no bead ID is provided, rejects the current pending gate (if any).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runReject,
}

var rejectNotes string

func init() {
	rejectCmd.Flags().StringVar(&rejectNotes, "notes", "", "rejection reason")
	rootCmd.AddCommand(rejectCmd)
}

func runReject(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	beadID := ""
	if len(args) > 0 {
		beadID = args[0]
	}

	// TODO: Find gate bead and reject it
	if beadID == "" {
		fmt.Println("Looking for pending gate...")
	} else {
		fmt.Printf("Rejecting gate: %s\n", beadID)
	}
	fmt.Println("(not yet implemented)")

	return nil
}
