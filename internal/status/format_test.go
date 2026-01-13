package status

import (
	"strings"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

func TestFormatDetailedWorkflow(t *testing.T) {
	now := time.Now()
	summary := &WorkflowSummary{
		ID:        "wf-test-123",
		Template:  "test.meow.toml",
		Status:    types.WorkflowStatusRunning,
		StartedAt: now,
		Variables: map[string]string{"env": "production"},
		StepStats: StepStats{
			Total:   10,
			Done:    5,
			Running: 2,
			Pending: 3,
		},
		RunningSteps: []RunningStep{
			{
				ID:        "work",
				Executor:  "agent",
				StartedAt: now.Add(-2 * time.Minute),
				Duration:  2 * time.Minute,
				AgentID:   "agent-0",
			},
		},
		Agents: []AgentSummary{
			{
				ID:          "agent-0",
				Status:      "active",
				TmuxSession: "meow-wf-test-123-agent-0",
				CurrentStep: "work",
			},
		},
	}

	opts := FormatOptions{NoColor: true}
	output := FormatDetailedWorkflow(summary, opts)

	// Check key components are present
	if !strings.Contains(output, "wf-test-123") {
		t.Error("output should contain workflow ID")
	}
	if !strings.Contains(output, "test.meow.toml") {
		t.Error("output should contain template name")
	}
	if !strings.Contains(output, "running") {
		t.Error("output should contain status")
	}
	if !strings.Contains(output, "Progress:") {
		t.Error("output should contain progress section")
	}
	if !strings.Contains(output, "5/10") {
		t.Error("output should show completed/total steps")
	}
	if !strings.Contains(output, "Running Steps:") {
		t.Error("output should contain running steps section")
	}
	if !strings.Contains(output, "Agents:") {
		t.Error("output should contain agents section")
	}
	if !strings.Contains(output, "tmux attach -t meow-wf-test-123-agent-0") {
		t.Error("output should contain tmux attach command")
	}
}

func TestFormatWorkflowList(t *testing.T) {
	now := time.Now()
	summaries := []*WorkflowSummary{
		{
			ID:        "wf-001",
			Template:  "sprint.meow.toml",
			Status:    types.WorkflowStatusRunning,
			StartedAt: now.Add(-1 * time.Hour),
			StepStats: StepStats{Total: 10, Done: 5},
		},
		{
			ID:        "wf-002",
			Template:  "build.meow.toml",
			Status:    types.WorkflowStatusDone,
			StartedAt: now.Add(-30 * time.Minute),
			DoneAt:    timePtr(now.Add(-10 * time.Minute)),
			StepStats: StepStats{Total: 5, Done: 5},
		},
	}

	opts := FormatOptions{NoColor: true}
	output := FormatWorkflowList(summaries, opts)

	if !strings.Contains(output, "Found 2 workflow(s)") {
		t.Error("output should show workflow count")
	}
	if !strings.Contains(output, "wf-001") {
		t.Error("output should contain first workflow ID")
	}
	if !strings.Contains(output, "wf-002") {
		t.Error("output should contain second workflow ID")
	}
	if !strings.Contains(output, "sprint.meow.toml") {
		t.Error("output should contain first template")
	}
	if !strings.Contains(output, "build.meow.toml") {
		t.Error("output should contain second template")
	}
}

func TestFormatProgress(t *testing.T) {
	tests := []struct {
		name     string
		stats    StepStats
		wantPercentage string
	}{
		{
			name:     "0%",
			stats:    StepStats{Total: 10, Done: 0},
			wantPercentage: "0%",
		},
		{
			name:     "50%",
			stats:    StepStats{Total: 10, Done: 5},
			wantPercentage: "50%",
		},
		{
			name:     "100%",
			stats:    StepStats{Total: 10, Done: 10},
			wantPercentage: "100%",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := &WorkflowSummary{
				ID:        "wf-test",
				StepStats: tt.stats,
			}
			opts := FormatOptions{NoColor: true}
			output := formatProgress(summary, opts)

			if !strings.Contains(output, tt.wantPercentage) {
				t.Errorf("expected percentage %s in output, got: %s", tt.wantPercentage, output)
			}
		})
	}
}

func TestGetStatusIcon(t *testing.T) {
	tests := []struct {
		status types.WorkflowStatus
		want   string
	}{
		{types.WorkflowStatusRunning, "●"},
		{types.WorkflowStatusDone, "✓"},
		{types.WorkflowStatusFailed, "✗"},
		{types.WorkflowStatusStopped, "■"},
		{types.WorkflowStatusPending, "○"},
		{types.WorkflowStatusCleaningUp, "◐"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := getStatusIcon(tt.status, true)
			if got != tt.want {
				t.Errorf("expected icon %s for status %s, got %s", tt.want, tt.status, got)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		duration time.Duration
		want     string
	}{
		{5 * time.Second, "5s"},
		{90 * time.Second, "1m30s"},
		{3700 * time.Second, "1h1m"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := formatDuration(tt.duration)
			if got != tt.want {
				t.Errorf("expected %s, got %s", tt.want, got)
			}
		})
	}
}

func TestFormatErrors(t *testing.T) {
	summary := &WorkflowSummary{
		Errors: []string{
			"command failed with exit code 1",
			"timeout exceeded",
		},
	}

	opts := FormatOptions{NoColor: true}
	output := formatErrors(summary, opts)

	if !strings.Contains(output, "Errors:") {
		t.Error("output should contain Errors header")
	}
	if !strings.Contains(output, "command failed with exit code 1") {
		t.Error("output should contain first error")
	}
	if !strings.Contains(output, "timeout exceeded") {
		t.Error("output should contain second error")
	}
}

// Helper function
func timePtr(t time.Time) *time.Time {
	return &t
}
