package status

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/akatz-ai/meow/internal/types"
	"github.com/akatz-ai/meow/internal/workflow"
)

// FormatOptions controls output formatting.
type FormatOptions struct {
	NoColor  bool
	AllSteps bool
	Agents   bool
	Quiet    bool
}

// FormatDetailedWorkflow formats a single workflow with full details.
func FormatDetailedWorkflow(summary *WorkflowSummary, opts FormatOptions) string {
	var b strings.Builder

	// Header
	b.WriteString(formatHeader(summary, opts))
	b.WriteString("\n\n")

	// Progress
	b.WriteString(formatProgress(summary, opts))
	b.WriteString("\n\n")

	// Running steps
	if len(summary.RunningSteps) > 0 {
		b.WriteString(formatRunningSteps(summary, opts))
		b.WriteString("\n\n")
	}

	// Agents
	if len(summary.Agents) > 0 {
		b.WriteString(formatAgents(summary, opts))
		b.WriteString("\n\n")
	}

	// Errors
	if len(summary.Errors) > 0 {
		b.WriteString(formatErrors(summary, opts))
		b.WriteString("\n")
	}

	return b.String()
}

// FormatWorkflowList formats a list of workflows.
func FormatWorkflowList(summaries []*WorkflowSummary, opts FormatOptions) string {
	var b strings.Builder

	// Header
	b.WriteString(fmt.Sprintf("Found %d workflow(s):\n\n", len(summaries)))

	// Sort by started time (newest first)
	sorted := make([]*WorkflowSummary, len(summaries))
	copy(sorted, summaries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartedAt.After(sorted[j].StartedAt)
	})

	// Display each workflow
	for i, summary := range sorted {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(formatWorkflowListItem(summary, opts))
	}

	return b.String()
}

func formatHeader(summary *WorkflowSummary, opts FormatOptions) string {
	var b strings.Builder

	statusIcon := getStatusIcon(summary.Status, opts.NoColor)
	statusColor := getStatusColor(summary.Status, opts.NoColor)

	b.WriteString(fmt.Sprintf("Workflow: %s\n", summary.ID))
	b.WriteString(fmt.Sprintf("Template: %s\n", summary.Template))
	b.WriteString(fmt.Sprintf("Status:   %s%s %s%s\n",
		statusColor, statusIcon, summary.Status, resetColor(opts.NoColor)))
	b.WriteString(fmt.Sprintf("Started:  %s", formatTime(summary.StartedAt)))

	if summary.DoneAt != nil {
		b.WriteString(fmt.Sprintf("\nCompleted: %s", formatTime(*summary.DoneAt)))
		duration := summary.DoneAt.Sub(summary.StartedAt)
		b.WriteString(fmt.Sprintf(" (took %s)", formatDuration(duration)))
	} else {
		elapsed := time.Since(summary.StartedAt)
		b.WriteString(fmt.Sprintf(" (%s ago)", formatDuration(elapsed)))
	}

	// Show variables if present
	if len(summary.Variables) > 0 && !opts.Quiet {
		b.WriteString("\n\nVariables:")
		for k, v := range summary.Variables {
			b.WriteString(fmt.Sprintf("\n  %s = %s", k, workflow.StringifyValue(v)))
		}
	}

	return b.String()
}

func formatProgress(summary *WorkflowSummary, opts FormatOptions) string {
	var b strings.Builder

	stats := summary.StepStats
	completed := stats.Done + stats.Failed + stats.Skipped
	total := stats.Total

	// Progress percentage
	var percentage int
	if total > 0 {
		percentage = (completed * 100) / total
	}

	// Progress bar (25 characters wide)
	barWidth := 25
	filled := (percentage * barWidth) / 100
	empty := barWidth - filled

	progressBar := strings.Repeat("█", filled) + strings.Repeat("░", empty)

	b.WriteString(fmt.Sprintf("Progress: %s %d%% (%d/%d steps)\n",
		progressBar, percentage, completed, total))

	// Status breakdown
	b.WriteString("\nSteps:    ")

	parts := []string{}
	if stats.Done > 0 {
		parts = append(parts, fmt.Sprintf("%s✓ %d done%s",
			getColor("green", opts.NoColor), stats.Done, resetColor(opts.NoColor)))
	}
	if stats.Running > 0 {
		parts = append(parts, fmt.Sprintf("%s● %d running%s",
			getColor("yellow", opts.NoColor), stats.Running, resetColor(opts.NoColor)))
	}
	if stats.Completing > 0 {
		parts = append(parts, fmt.Sprintf("%s◐ %d completing%s",
			getColor("cyan", opts.NoColor), stats.Completing, resetColor(opts.NoColor)))
	}
	if stats.Pending > 0 {
		parts = append(parts, fmt.Sprintf("%s○ %d pending%s",
			getColor("gray", opts.NoColor), stats.Pending, resetColor(opts.NoColor)))
	}
	if stats.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%s✗ %d failed%s",
			getColor("red", opts.NoColor), stats.Failed, resetColor(opts.NoColor)))
	}
	if stats.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%s⊘ %d skipped%s",
			getColor("gray", opts.NoColor), stats.Skipped, resetColor(opts.NoColor)))
	}

	b.WriteString(strings.Join(parts, ", "))

	return b.String()
}

func formatRunningSteps(summary *WorkflowSummary, opts FormatOptions) string {
	var b strings.Builder

	b.WriteString("Running Steps:\n")

	// Sort by start time
	steps := make([]RunningStep, len(summary.RunningSteps))
	copy(steps, summary.RunningSteps)
	sort.Slice(steps, func(i, j int) bool {
		return steps[i].StartedAt.Before(steps[j].StartedAt)
	})

	for _, step := range steps {
		duration := formatDuration(step.Duration)
		if step.AgentID != "" {
			b.WriteString(fmt.Sprintf("  - %s (agent: %s, %s)\n",
				step.ID, step.AgentID, duration))
		} else {
			b.WriteString(fmt.Sprintf("  - %s (%s, %s)\n",
				step.ID, step.Executor, duration))
		}
	}

	return b.String()
}

func formatAgents(summary *WorkflowSummary, opts FormatOptions) string {
	var b strings.Builder

	b.WriteString("Agents:\n")

	// Sort by agent ID
	agents := make([]AgentSummary, len(summary.Agents))
	copy(agents, summary.Agents)
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].ID < agents[j].ID
	})

	for _, agent := range agents {
		statusIcon := "●"
		statusColor := getColor("green", opts.NoColor)

		if agent.Status == "idle" {
			statusIcon = "○"
			statusColor = getColor("gray", opts.NoColor)
		}

		b.WriteString(fmt.Sprintf("  %s%s %s%s: %s\n",
			statusColor, statusIcon, agent.ID, resetColor(opts.NoColor), agent.Status))

		if agent.CurrentStep != "" {
			b.WriteString(fmt.Sprintf("    Current: %s\n", agent.CurrentStep))
		}

		if agent.TmuxSession != "" {
			attachCmd := fmt.Sprintf("tmux attach -t %s", agent.TmuxSession)
			b.WriteString(fmt.Sprintf("    %s\n", attachCmd))
		}
	}

	return b.String()
}

func formatErrors(summary *WorkflowSummary, opts FormatOptions) string {
	var b strings.Builder

	errColor := getColor("red", opts.NoColor)
	reset := resetColor(opts.NoColor)

	b.WriteString(fmt.Sprintf("%sErrors:%s\n", errColor, reset))
	for _, err := range summary.Errors {
		b.WriteString(fmt.Sprintf("  %s✗%s %s\n", errColor, reset, err))
	}

	return b.String()
}

func formatWorkflowListItem(summary *WorkflowSummary, opts FormatOptions) string {
	var b strings.Builder

	statusIcon := getStatusIcon(summary.Status, opts.NoColor)
	statusColor := getStatusColor(summary.Status, opts.NoColor)

	completed := summary.StepStats.Done + summary.StepStats.Failed + summary.StepStats.Skipped
	total := summary.StepStats.Total

	b.WriteString(fmt.Sprintf("%s%s %s%s", statusColor, statusIcon, summary.ID, resetColor(opts.NoColor)))

	if !opts.Quiet {
		b.WriteString(fmt.Sprintf("\n  Template: %s", summary.Template))
		b.WriteString(fmt.Sprintf("\n  Status:   %s%s%s", statusColor, summary.Status, resetColor(opts.NoColor)))
		b.WriteString(fmt.Sprintf("\n  Progress: %d/%d steps", completed, total))

		if summary.DoneAt != nil {
			duration := summary.DoneAt.Sub(summary.StartedAt)
			b.WriteString(fmt.Sprintf("\n  Duration: %s", formatDuration(duration)))
		} else {
			elapsed := time.Since(summary.StartedAt)
			b.WriteString(fmt.Sprintf("\n  Running:  %s", formatDuration(elapsed)))
		}

		if len(summary.Agents) > 0 {
			b.WriteString(fmt.Sprintf("\n  Agents:   %d", len(summary.Agents)))
		}
	}

	return b.String()
}

// Formatting helpers

func getStatusIcon(status types.RunStatus, noColor bool) string {
	switch status {
	case types.RunStatusRunning:
		return "●"
	case types.RunStatusDone:
		return "✓"
	case types.RunStatusFailed:
		return "✗"
	case types.RunStatusStopped:
		return "■"
	case types.RunStatusPending:
		return "○"
	case types.RunStatusCleaningUp:
		return "◐"
	default:
		return "?"
	}
}

func getStatusColor(status types.RunStatus, noColor bool) string {
	if noColor {
		return ""
	}

	switch status {
	case types.RunStatusRunning:
		return "\033[33m" // Yellow
	case types.RunStatusDone:
		return "\033[32m" // Green
	case types.RunStatusFailed:
		return "\033[31m" // Red
	case types.RunStatusStopped:
		return "\033[90m" // Gray
	case types.RunStatusPending:
		return "\033[90m" // Gray
	case types.RunStatusCleaningUp:
		return "\033[36m" // Cyan
	default:
		return ""
	}
}

func getColor(name string, noColor bool) string {
	if noColor {
		return ""
	}

	switch name {
	case "red":
		return "\033[31m"
	case "green":
		return "\033[32m"
	case "yellow":
		return "\033[33m"
	case "cyan":
		return "\033[36m"
	case "gray":
		return "\033[90m"
	default:
		return ""
	}
}

func resetColor(noColor bool) string {
	if noColor {
		return ""
	}
	return "\033[0m"
}

func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
