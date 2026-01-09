package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/spf13/cobra"
)

var eventCmd = &cobra.Command{
	Use:   "event <type>",
	Short: "Emit an event to the orchestrator",
	Long: `Emit an event that can be received by await-event waiters.

Events are fire-and-forget notifications. The orchestrator will route the event
to any matching waiters, or simply log it if there are no waiters.

Environment variables required:
  MEOW_ORCH_SOCK - Path to orchestrator socket (set by orchestrator)

Examples:
  # Simple event
  meow event agent-stopped

  # Event with data
  meow event tool-completed --data tool=Bash --data exit_code=0

  # Event with JSON data
  meow event custom-event --data payload='{"key": "value"}'`,
	Args: cobra.ExactArgs(1),
	RunE: runEvent,
}

var (
	eventData []string
)

func init() {
	eventCmd.Flags().StringArrayVar(&eventData, "data", nil, "event data (format: key=value)")
	rootCmd.AddCommand(eventCmd)
}

func runEvent(cmd *cobra.Command, args []string) error {
	eventType := args[0]

	// Get orchestrator socket from environment
	sockPath := os.Getenv("MEOW_ORCH_SOCK")
	if sockPath == "" {
		return fmt.Errorf("MEOW_ORCH_SOCK not set - are you running in a MEOW workflow?")
	}

	// Parse --data flags into map
	data := make(map[string]any)
	for _, kv := range eventData {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid data format: %s (expected key=value)", kv)
		}
		key, value := parts[0], parts[1]

		// Try to parse as JSON first, fall back to string
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			parsed = value
		}
		data[key] = parsed
	}

	// Get agent and workflow from environment for context
	agent := os.Getenv("MEOW_AGENT")
	workflow := os.Getenv("MEOW_WORKFLOW")

	// Create IPC client
	client := ipc.NewClient(sockPath)

	// Send event
	msg := &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: eventType,
		Data:      data,
		Agent:     agent,
		Workflow:  workflow,
	}

	response, err := client.Send(msg)
	if err != nil {
		return fmt.Errorf("sending event: %w", err)
	}

	// Check response
	switch r := response.(type) {
	case *ipc.AckMessage:
		if !r.Success {
			return fmt.Errorf("event was not acknowledged")
		}
		if verbose {
			fmt.Printf("Event '%s' emitted\n", eventType)
		}
	case *ipc.ErrorMessage:
		return fmt.Errorf("orchestrator error: %s", r.Message)
	default:
		return fmt.Errorf("unexpected response type: %T", response)
	}

	return nil
}
