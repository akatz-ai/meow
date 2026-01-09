package cmd

import (
	"fmt"
	"os"

	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/spf13/cobra"
)

var stepStatusCmd = &cobra.Command{
	Use:   "step-status <step-id>",
	Short: "Check the status of a step",
	Long: `Query the status of a workflow step.

This command is useful for conditional logic in shell scripts and branch conditions.

Exit codes:
  0 - Status retrieved (without --is) or status matches (with --is)
  1 - Status does not match (with --is)
  2 - Error (step not found, socket error)

Environment variables required:
  MEOW_ORCH_SOCK - Path to orchestrator socket (set by orchestrator)
  MEOW_WORKFLOW  - Workflow ID (set by orchestrator)

Examples:
  # Get step status
  meow step-status setup-step

  # Check if step is done
  if meow step-status setup-step --is done; then
    echo "Setup complete"
  fi

  # Check if step is not running
  meow step-status main-task --is-not running`,
	Args: cobra.ExactArgs(1),
	RunE: runStepStatus,
}

var (
	stepStatusIs    string
	stepStatusIsNot string
)

func init() {
	stepStatusCmd.Flags().StringVar(&stepStatusIs, "is", "", "check if status matches this value")
	stepStatusCmd.Flags().StringVar(&stepStatusIsNot, "is-not", "", "check if status does NOT match this value")
	rootCmd.AddCommand(stepStatusCmd)
}

func runStepStatus(cmd *cobra.Command, args []string) error {
	stepID := args[0]

	// Get orchestrator socket from environment
	sockPath := os.Getenv("MEOW_ORCH_SOCK")
	if sockPath == "" {
		fmt.Fprintln(os.Stderr, "MEOW_ORCH_SOCK not set - are you running in a MEOW workflow?")
		os.Exit(2)
	}

	// Get workflow ID from environment
	workflowID := os.Getenv("MEOW_WORKFLOW")
	if workflowID == "" {
		fmt.Fprintln(os.Stderr, "MEOW_WORKFLOW not set - are you running in a MEOW workflow?")
		os.Exit(2)
	}

	// Create IPC client
	client := ipc.NewClient(sockPath)

	// Get step status
	status, err := client.GetStepStatus(workflowID, stepID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	// Handle --is flag
	if stepStatusIs != "" {
		if status == stepStatusIs {
			os.Exit(0)
		}
		os.Exit(1)
	}

	// Handle --is-not flag
	if stepStatusIsNot != "" {
		if status != stepStatusIsNot {
			os.Exit(0)
		}
		os.Exit(1)
	}

	// Print status
	fmt.Println(status)
	return nil
}
