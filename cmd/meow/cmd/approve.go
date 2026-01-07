package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var approveCmd = &cobra.Command{
	Use:   "approve [bead-id]",
	Short: "Approve a gate bead",
	Long: `Approve a blocking gate bead to continue workflow execution.

If no bead ID is provided, approves the current pending gate (if any).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runApprove,
}

var approveNotes string

func init() {
	approveCmd.Flags().StringVar(&approveNotes, "notes", "", "approval notes")
	rootCmd.AddCommand(approveCmd)
}

func runApprove(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	beadID := ""
	if len(args) > 0 {
		beadID = args[0]
	}

	// TODO: Find gate bead and approve it
	if beadID == "" {
		fmt.Println("Looking for pending gate...")
	} else {
		fmt.Printf("Approving gate: %s\n", beadID)
	}
	fmt.Println("(not yet implemented)")

	return nil
}
