package cmd

import (
	"fmt"
	"os"

	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/spf13/cobra"
)

var rejectCmd = &cobra.Command{
	Use:   "reject <gate-id>",
	Short: "Reject a workflow gate",
	Long: `Reject a workflow gate. The workflow will take the rejection branch.

This command sends a rejection message to the orchestrator for the specified gate.

Environment variables:
  MEOW_ORCH_SOCK - Path to orchestrator socket (set by orchestrator)
  MEOW_WORKFLOW  - Workflow ID (fallback if socket not set)

Examples:
  # Inside a workflow (env vars set)
  meow reject gate-deploy

  # Outside workflow (manual use)
  meow reject --workflow wf-abc123 gate-deploy

  # With reason
  meow reject --reason "Tests failing" gate-deploy`,
	Args: cobra.ExactArgs(1),
	RunE: runReject,
}

var (
	rejectWorkflow string
	rejectReason   string
)

func init() {
	rejectCmd.Flags().StringVar(&rejectWorkflow, "workflow", "", "workflow ID (required if MEOW_ORCH_SOCK not set)")
	rejectCmd.Flags().StringVar(&rejectReason, "reason", "", "rejection reason")
	rootCmd.AddCommand(rejectCmd)
}

func runReject(cmd *cobra.Command, args []string) error {
	gateID := args[0]

	// Get socket path from environment or derive from workflow ID
	sockPath := os.Getenv("MEOW_ORCH_SOCK")
	workflowID := rejectWorkflow
	if workflowID == "" {
		workflowID = os.Getenv("MEOW_WORKFLOW")
	}

	if sockPath == "" {
		// For manual use, derive from workflow ID
		if workflowID == "" {
			return fmt.Errorf("either MEOW_ORCH_SOCK or --workflow required")
		}
		sockPath = ipc.SocketPath(workflowID)
	}

	// Create IPC client
	client := ipc.NewClient(sockPath)

	// Send rejection (approved=false, notes="", reason=rejectReason)
	err := client.SendApproval(workflowID, gateID, false, "", rejectReason)
	if err != nil {
		return fmt.Errorf("sending rejection: %w", err)
	}

	if verbose {
		fmt.Printf("Gate %s rejected\n", gateID)
	}

	return nil
}
