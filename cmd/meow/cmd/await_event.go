package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/spf13/cobra"
)

var awaitEventCmd = &cobra.Command{
	Use:   "await-event <type>",
	Short: "Wait for an event matching the given criteria",
	Long: `Wait for an event that matches the specified type and filters.

This command blocks until a matching event is received or timeout occurs.
It is typically used in branch conditions for workflow control.

Exit codes:
  0 - Event matched
  1 - Timeout
  2 - Error (socket not found, invalid args)

Environment variables required:
  MEOW_ORCH_SOCK - Path to orchestrator socket (set by orchestrator)

Examples:
  # Wait for any tool-completed event
  meow await-event tool-completed --timeout 5m

  # Wait for specific tool completion
  meow await-event tool-completed --filter tool=Bash --timeout 1h

  # Wait for agent stopped (for monitoring)
  meow await-event agent-stopped --filter agent=worker-1`,
	Args: cobra.ExactArgs(1),
	RunE: runAwaitEvent,
}

var (
	awaitEventFilter  []string
	awaitEventTimeout string
	awaitEventQuiet   bool
)

func init() {
	awaitEventCmd.Flags().StringArrayVar(&awaitEventFilter, "filter", nil, "event filters (format: key=value)")
	awaitEventCmd.Flags().StringVar(&awaitEventTimeout, "timeout", "24h", "timeout duration (e.g., 5m, 1h)")
	awaitEventCmd.Flags().BoolVar(&awaitEventQuiet, "quiet", false, "suppress output (only use exit code)")
	rootCmd.AddCommand(awaitEventCmd)
}

func runAwaitEvent(cmd *cobra.Command, args []string) error {
	eventType := args[0]

	// Get orchestrator socket from environment
	// If not set, exit with timeout semantics (exit 1) - allows await-event
	// to be used in conditions outside of MEOW workflows
	sockPath := os.Getenv("MEOW_ORCH_SOCK")
	if sockPath == "" {
		os.Exit(1) // Timeout semantics
	}

	// Parse filters
	filter := make(map[string]string)
	for _, kv := range awaitEventFilter {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			fmt.Fprintf(os.Stderr, "invalid filter format: %s (expected key=value)\n", kv)
			os.Exit(2)
		}
		filter[parts[0]] = parts[1]
	}

	// Create IPC client with a long timeout since we're waiting
	client := ipc.NewClient(sockPath)
	// Set a very long timeout for the connection itself
	// The actual timeout is handled server-side
	client.SetTimeout(25 * time.Hour)

	// Send await request
	response, err := client.AwaitEvent(eventType, filter, awaitEventTimeout)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "timeout") {
			if !awaitEventQuiet {
				fmt.Fprintln(os.Stderr, "timeout waiting for event")
			}
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(2)
	}

	// Output the matched event as JSON
	if !awaitEventQuiet {
		output := map[string]any{
			"event_type": response.EventType,
			"data":       response.Data,
			"timestamp":  response.Timestamp,
		}
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(output); err != nil {
			fmt.Fprintf(os.Stderr, "error encoding output: %v\n", err)
			os.Exit(2)
		}
	}

	return nil
}
