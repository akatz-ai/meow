package cmd

import (
	"fmt"
	"os"

	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/spf13/cobra"
)

var sessionIDCmd = &cobra.Command{
	Use:   "session-id",
	Short: "Get Claude session ID for an agent",
	Long: `Get the Claude session ID for a running agent.

The session ID can be used to resume a Claude session after context limits
or to coordinate parent-child workflows.

Environment variables required:
  MEOW_WORKFLOW - Workflow ID (set by orchestrator)

Examples:
  # Get session ID for current agent
  meow session-id

  # Get session ID for a specific agent
  meow session-id --agent worker-1`,
	RunE: runSessionID,
}

var sessionIDAgent string

func init() {
	sessionIDCmd.Flags().StringVar(&sessionIDAgent, "agent", "", "Agent ID to query (defaults to MEOW_AGENT)")
	rootCmd.AddCommand(sessionIDCmd)
}

func runSessionID(cmd *cobra.Command, args []string) error {
	// Get orchestrator socket from environment
	// If not set, we cannot proceed (this command requires an active workflow)
	sockPath := os.Getenv("MEOW_ORCH_SOCK")
	if sockPath == "" {
		// Fallback: try to derive from workflow ID for manual use
		workflowID := os.Getenv("MEOW_WORKFLOW")
		if workflowID == "" {
			return fmt.Errorf("MEOW_ORCH_SOCK or MEOW_WORKFLOW not set - are you running in a MEOW session?")
		}
		sockPath = ipc.SocketPath(workflowID)
	}

	// Get agent ID from flag or environment
	agentID := sessionIDAgent
	if agentID == "" {
		agentID = os.Getenv("MEOW_AGENT")
	}
	if agentID == "" {
		return fmt.Errorf("agent not specified - use --agent flag or set MEOW_AGENT")
	}

	// Create IPC client using the socket path from environment
	client := ipc.NewClient(sockPath)

	// Get session ID
	sessionID, err := client.GetSessionID(agentID)
	if err != nil {
		return fmt.Errorf("getting session ID: %w", err)
	}

	// Output session ID to stdout (for use in shell outputs)
	fmt.Println(sessionID)

	return nil
}
