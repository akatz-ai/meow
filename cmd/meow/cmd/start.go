package cmd

import (
	"fmt"
	"os"

	"github.com/akatz-ai/meow/internal/ipc"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Acknowledge step receipt",
	Long: `Signal that the agent has received and understood the current step.

This command should be called by agents when they receive a new task prompt.
It signals to the orchestrator that the agent is actively working on the step.

The acknowledgment creates an explicit state transition:
  idle -> [prompt injected] -> pending_ack -> [meow start] -> working -> [meow done] -> idle

Environment variables required:
  MEOW_AGENT    - Agent ID (set by orchestrator)
  MEOW_WORKFLOW - Workflow ID (set by orchestrator)

Examples:
  meow start`,
	RunE: runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	// Get orchestrator socket from environment
	// If not set, exit silently (no-op) - allows agents to call meow start
	// without being in a MEOW workflow
	sockPath := os.Getenv("MEOW_ORCH_SOCK")
	if sockPath == "" {
		return nil // Silent no-op
	}

	// Get agent ID from environment
	agentID := os.Getenv("MEOW_AGENT")
	if agentID == "" {
		return fmt.Errorf("MEOW_AGENT not set - are you running in a MEOW session?")
	}

	// Get workflow ID from environment
	workflowID := os.Getenv("MEOW_WORKFLOW")
	if workflowID == "" {
		return fmt.Errorf("MEOW_WORKFLOW not set - are you running in a MEOW session?")
	}

	// Try to get step ID from environment or arguments
	stepID := os.Getenv("MEOW_STEP")
	if stepID == "" && len(args) > 0 {
		stepID = args[0]
	}

	// Create IPC client using the socket path from environment
	client := ipc.NewClient(sockPath)

	// Send step start message
	response, err := client.SendStepStart(workflowID, agentID, stepID)
	if err != nil {
		return fmt.Errorf("sending start message: %w", err)
	}

	// Check response
	switch r := response.(type) {
	case *ipc.AckMessage:
		if !r.Success {
			return fmt.Errorf("orchestrator rejected acknowledgment")
		}
		if verbose {
			fmt.Println("Step acknowledged")
		}
	case *ipc.ErrorMessage:
		return fmt.Errorf("orchestrator error: %s", r.Message)
	default:
		return fmt.Errorf("unexpected response type: %T", response)
	}

	return nil
}
