package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/spf13/cobra"
)

var awaitApprovalCmd = &cobra.Command{
	Use:   "await-approval <gate-id>",
	Short: "Wait for gate approval or rejection",
	Long: `Wait for a gate to be approved or rejected.

This command blocks until either:
- gate-approved event is received (exit 0)
- gate-rejected event is received (exit 1)
- timeout occurs (exit 1)

It is typically used in branch conditions for workflow control.

Exit codes:
  0 - Gate approved
  1 - Gate rejected or timeout
  2 - Error (socket not found, invalid args)

Environment variables required:
  MEOW_ORCH_SOCK - Path to orchestrator socket (set by orchestrator)

Examples:
  # Wait for gate approval (24h default timeout)
  meow await-approval deploy-gate

  # Wait with custom timeout
  meow await-approval deploy-gate --timeout 1h

  # Wait silently (only use exit code)
  meow await-approval deploy-gate --quiet`,
	Args: cobra.ExactArgs(1),
	RunE: runAwaitApproval,
}

var (
	awaitApprovalTimeout string
	awaitApprovalQuiet   bool
)

func init() {
	awaitApprovalCmd.Flags().StringVar(&awaitApprovalTimeout, "timeout", "24h", "timeout duration (e.g., 5m, 1h)")
	awaitApprovalCmd.Flags().BoolVar(&awaitApprovalQuiet, "quiet", false, "suppress output (only use exit code)")
	rootCmd.AddCommand(awaitApprovalCmd)
}

// approvalResult represents the outcome of waiting for approval
type approvalResult struct {
	approved bool
	event    *ipc.EventMatchMessage
	err      error
}

func runAwaitApproval(cmd *cobra.Command, args []string) error {
	gateID := args[0]

	// Get orchestrator socket from environment
	// If not set, exit with rejection semantics (exit 1) - allows await-approval
	// to be used in conditions outside of MEOW workflows
	sockPath := os.Getenv("MEOW_ORCH_SOCK")
	if sockPath == "" {
		os.Exit(1) // Rejection semantics when not in workflow
	}

	// Parse timeout for validation
	timeout, err := time.ParseDuration(awaitApprovalTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid timeout format: %s\n", awaitApprovalTimeout)
		os.Exit(2)
	}

	// Create filter for this gate
	filter := map[string]string{
		"gate": gateID,
	}

	// Channel to receive the first result
	resultCh := make(chan approvalResult, 2)

	// Wait for gate-approved in goroutine
	go func() {
		client := ipc.NewClient(sockPath)
		client.SetTimeout(timeout + time.Minute) // Add buffer for network
		response, err := client.AwaitEvent("gate-approved", filter, awaitApprovalTimeout)
		resultCh <- approvalResult{approved: true, event: response, err: err}
	}()

	// Wait for gate-rejected in goroutine
	go func() {
		client := ipc.NewClient(sockPath)
		client.SetTimeout(timeout + time.Minute) // Add buffer for network
		response, err := client.AwaitEvent("gate-rejected", filter, awaitApprovalTimeout)
		resultCh <- approvalResult{approved: false, event: response, err: err}
	}()

	// Wait for first result
	result := <-resultCh

	// Handle result
	if result.err != nil {
		// Check if it's a timeout error
		if result.err.Error() == "timeout waiting for event" {
			if !awaitApprovalQuiet {
				fmt.Fprintln(os.Stderr, "timeout waiting for approval")
			}
			os.Exit(1)
		}
		// Other error
		fmt.Fprintf(os.Stderr, "error: %v\n", result.err)
		os.Exit(2)
	}

	// Got a response
	if result.approved {
		if !awaitApprovalQuiet {
			approver := ""
			if result.event != nil && result.event.Data != nil {
				if a, ok := result.event.Data["approver"].(string); ok {
					approver = a
				}
			}
			if approver != "" {
				fmt.Printf("Gate %s approved by %s\n", gateID, approver)
			} else {
				fmt.Printf("Gate %s approved\n", gateID)
			}
		}
		os.Exit(0)
	} else {
		if !awaitApprovalQuiet {
			reason := ""
			if result.event != nil && result.event.Data != nil {
				if r, ok := result.event.Data["reason"].(string); ok {
					reason = r
				}
			}
			if reason != "" {
				fmt.Printf("Gate %s rejected: %s\n", gateID, reason)
			} else {
				fmt.Printf("Gate %s rejected\n", gateID)
			}
		}
		os.Exit(1)
	}

	return nil // Never reached
}
