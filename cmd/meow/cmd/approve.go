package cmd

import (
	"fmt"
	"os"

	"github.com/akatz-ai/meow/internal/ipc"
	"github.com/spf13/cobra"
)

var approveCmd = &cobra.Command{
	Use:   "approve <gate-id>",
	Short: "Approve a workflow gate",
	Long: `Approve a workflow gate to allow execution to continue.

This command emits a gate-approved event that can be received by await-approval waiters.

Environment variables:
  MEOW_ORCH_SOCK - Path to orchestrator socket (set by orchestrator)
  MEOW_WORKFLOW  - Workflow ID (fallback if socket not set)

Examples:
  # Inside a workflow (env vars set)
  meow approve gate-deploy

  # Outside workflow (manual use)
  meow approve --workflow wf-abc123 gate-deploy

  # With approver name
  meow approve --approver "John Doe" gate-deploy`,
	Args: cobra.ExactArgs(1),
	RunE: runApprove,
}

var (
	approveWorkflow string
	approveApprover string
)

func init() {
	approveCmd.Flags().StringVar(&approveWorkflow, "workflow", "", "workflow ID (required if MEOW_ORCH_SOCK not set)")
	approveCmd.Flags().StringVar(&approveApprover, "approver", "", "approver name for audit trail")
	rootCmd.AddCommand(approveCmd)
}

func runApprove(cmd *cobra.Command, args []string) error {
	gateID := args[0]

	// Get socket path from environment or derive from workflow ID
	sockPath := os.Getenv("MEOW_ORCH_SOCK")
	workflowID := approveWorkflow
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

	// Emit gate-approved event
	data := map[string]any{
		"gate": gateID,
	}
	if approveApprover != "" {
		data["approver"] = approveApprover
	}
	if workflowID != "" {
		data["workflow"] = workflowID
	}

	err := client.SendEvent("gate-approved", data)
	if err != nil {
		return fmt.Errorf("sending approval event: %w", err)
	}

	if verbose {
		fmt.Printf("Gate %s approved\n", gateID)
	}

	return nil
}
