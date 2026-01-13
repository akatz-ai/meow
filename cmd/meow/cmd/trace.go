package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/spf13/cobra"
)

var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Show execution trace",
	Long: `Display the execution trace of the current workflow.

Shows the sequence of step executions with timestamps, durations,
and outcomes. Useful for debugging workflow issues.`,
	RunE: runTrace,
}

var (
	traceLimit  int
	traceFormat string
)

func init() {
	traceCmd.Flags().IntVar(&traceLimit, "limit", 50, "maximum entries to show")
	traceCmd.Flags().StringVar(&traceFormat, "format", "text", "output format: text, json")
	// TODO: implement --follow flag for real-time trace output
	rootCmd.AddCommand(traceCmd)
}

func runTrace(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	tracePath := dir + "/.meow/logs/trace.jsonl"

	// Check if trace file exists
	if _, err := os.Stat(tracePath); os.IsNotExist(err) {
		fmt.Println("No trace file found.")
		fmt.Println("")
		fmt.Println("The trace file is created when the orchestrator runs.")
		fmt.Println("Use 'meow run' to start a workflow.")
		return nil
	}

	file, err := os.Open(tracePath)
	if err != nil {
		return fmt.Errorf("opening trace file: %w", err)
	}
	defer file.Close()

	// Read all entries
	var entries []orchestrator.TraceEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry orchestrator.TraceEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			// Skip malformed lines
			continue
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading trace file: %w", err)
	}

	if len(entries) == 0 {
		fmt.Println("Trace file is empty.")
		return nil
	}

	// Apply limit (show last N entries)
	if traceLimit > 0 && len(entries) > traceLimit {
		entries = entries[len(entries)-traceLimit:]
	}

	// Handle JSON output
	if traceFormat == "json" {
		data, err := json.MarshalIndent(entries, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling trace: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Text output
	fmt.Println("Execution Trace")
	fmt.Println("---------------")
	fmt.Println()

	for _, entry := range entries {
		formatTraceEntry(entry)
	}

	fmt.Println()
	fmt.Printf("Showing %d entries", len(entries))
	if traceLimit > 0 {
		fmt.Printf(" (limit: %d)", traceLimit)
	}
	fmt.Println()

	return nil
}

// formatTraceEntry formats a single trace entry for display.
func formatTraceEntry(entry orchestrator.TraceEntry) {
	// Format timestamp
	ts := entry.Timestamp.Format("15:04:05.000")

	// Action icon/symbol
	icon := actionIcon(entry.Action)

	// Build the main line
	var parts []string
	parts = append(parts, fmt.Sprintf("[%s] %s %s", ts, icon, entry.Action))

	// Add context based on action type
	switch entry.Action {
	case orchestrator.TraceActionStart:
		if entry.Template != "" {
			parts = append(parts, fmt.Sprintf("template=%s", entry.Template))
		}
	case orchestrator.TraceActionResume:
		if ticks, ok := entry.Details["tick_count"]; ok {
			parts = append(parts, fmt.Sprintf("ticks=%v", ticks))
		}
	case orchestrator.TraceActionBake:
		if entry.Template != "" {
			parts = append(parts, fmt.Sprintf("template=%s", entry.Template))
		}
		if count, ok := entry.Details["step_count"]; ok {
			parts = append(parts, fmt.Sprintf("steps=%v", count))
		}
	case orchestrator.TraceActionSpawn:
		if entry.AgentID != "" {
			parts = append(parts, fmt.Sprintf("agent=%s", entry.AgentID))
		}
	case orchestrator.TraceActionDispatch:
		if entry.StepID != "" {
			parts = append(parts, fmt.Sprintf("step=%s", entry.StepID))
		}
		if entry.StepType != "" {
			parts = append(parts, fmt.Sprintf("type=%s", entry.StepType))
		}
	case orchestrator.TraceActionConditionEval:
		if entry.StepID != "" {
			parts = append(parts, fmt.Sprintf("step=%s", entry.StepID))
		}
		if result, ok := entry.Details["result"]; ok {
			parts = append(parts, fmt.Sprintf("result=%v", result))
		}
	case orchestrator.TraceActionExpand:
		if entry.StepID != "" {
			parts = append(parts, fmt.Sprintf("step=%s", entry.StepID))
		}
		if entry.Template != "" {
			parts = append(parts, fmt.Sprintf("template=%s", entry.Template))
		}
		if count, ok := entry.Details["child_count"]; ok {
			parts = append(parts, fmt.Sprintf("children=%v", count))
		}
	case orchestrator.TraceActionClose:
		if entry.StepID != "" {
			parts = append(parts, fmt.Sprintf("step=%s", entry.StepID))
		}
		if entry.StepType != "" {
			parts = append(parts, fmt.Sprintf("type=%s", entry.StepType))
		}
	case orchestrator.TraceActionStop:
		if entry.AgentID != "" {
			parts = append(parts, fmt.Sprintf("agent=%s", entry.AgentID))
		}
		if graceful, ok := entry.Details["graceful"]; ok {
			parts = append(parts, fmt.Sprintf("graceful=%v", graceful))
		}
	case orchestrator.TraceActionShutdown:
		if reason, ok := entry.Details["reason"]; ok {
			parts = append(parts, fmt.Sprintf("reason=%v", reason))
		}
	case orchestrator.TraceActionError:
		if entry.StepID != "" {
			parts = append(parts, fmt.Sprintf("step=%s", entry.StepID))
		}
		if entry.Error != "" {
			parts = append(parts, fmt.Sprintf("error=%q", truncate(entry.Error, 50)))
		}
	}

	fmt.Println(strings.Join(parts, " "))
}

// actionIcon returns an icon for the action type.
func actionIcon(action orchestrator.TraceAction) string {
	switch action {
	case orchestrator.TraceActionStart:
		return ">"
	case orchestrator.TraceActionResume:
		return ">>"
	case orchestrator.TraceActionBake:
		return "+"
	case orchestrator.TraceActionSpawn:
		return "*"
	case orchestrator.TraceActionDispatch:
		return "->"
	case orchestrator.TraceActionConditionEval:
		return "?"
	case orchestrator.TraceActionExpand:
		return "^"
	case orchestrator.TraceActionClose:
		return "v"
	case orchestrator.TraceActionStop:
		return "x"
	case orchestrator.TraceActionShutdown:
		return "||"
	case orchestrator.TraceActionError:
		return "!"
	default:
		return " "
	}
}

// truncate shortens a string if it exceeds maxLen.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

