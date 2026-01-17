package status

import (
	"testing"
	"time"

	"github.com/akatz-ai/meow/internal/types"
)

func TestNewWorkflowSummary(t *testing.T) {
	now := time.Now()

	wf := &types.Run{
		ID:        "run-test",
		Template:  "test.meow.toml",
		Status:    types.RunStatusRunning,
		StartedAt: now,
		Variables: map[string]any{"env": "test"},
		Steps: map[string]*types.Step{
			"step1": {
				ID:       "step1",
				Executor: types.ExecutorShell,
				Status:   types.StepStatusDone,
			},
			"step2": {
				ID:       "step2",
				Executor: types.ExecutorAgent,
				Status:   types.StepStatusRunning,
			},
			"step3": {
				ID:       "step3",
				Executor: types.ExecutorShell,
				Status:   types.StepStatusPending,
			},
		},
		Agents: map[string]*types.AgentInfo{
			"agent-0": {
				TmuxSession: "meow-wf-test-agent-0",
				Status:      "active",
				CurrentStep: "step2",
			},
		},
	}

	summary := NewWorkflowSummary(wf)

	if summary.ID != "run-test" {
		t.Errorf("expected ID wf-test, got %s", summary.ID)
	}

	if summary.Template != "test.meow.toml" {
		t.Errorf("expected template test.meow.toml, got %s", summary.Template)
	}

	if summary.Status != types.RunStatusRunning {
		t.Errorf("expected status running, got %s", summary.Status)
	}

	if summary.StepStats.Total != 3 {
		t.Errorf("expected 3 total steps, got %d", summary.StepStats.Total)
	}

	if summary.StepStats.Done != 1 {
		t.Errorf("expected 1 done step, got %d", summary.StepStats.Done)
	}

	if summary.StepStats.Running != 1 {
		t.Errorf("expected 1 running step, got %d", summary.StepStats.Running)
	}

	if summary.StepStats.Pending != 1 {
		t.Errorf("expected 1 pending step, got %d", summary.StepStats.Pending)
	}

	if len(summary.RunningSteps) != 1 {
		t.Errorf("expected 1 running step, got %d", len(summary.RunningSteps))
	}

	if len(summary.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(summary.Agents))
	}

	if summary.Agents[0].ID != "agent-0" {
		t.Errorf("expected agent ID agent-0, got %s", summary.Agents[0].ID)
	}
}

func TestComputeStepStats(t *testing.T) {
	tests := []struct {
		name     string
		steps    map[string]*types.Step
		expected StepStats
	}{
		{
			name: "all done",
			steps: map[string]*types.Step{
				"s1": {Status: types.StepStatusDone},
				"s2": {Status: types.StepStatusDone},
			},
			expected: StepStats{
				Total: 2,
				Done:  2,
			},
		},
		{
			name: "mixed statuses",
			steps: map[string]*types.Step{
				"s1": {Status: types.StepStatusDone},
				"s2": {Status: types.StepStatusRunning},
				"s3": {Status: types.StepStatusPending},
				"s4": {Status: types.StepStatusFailed},
				"s5": {Status: types.StepStatusSkipped},
			},
			expected: StepStats{
				Total:   5,
				Done:    1,
				Running: 1,
				Pending: 1,
				Failed:  1,
				Skipped: 1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wf := &types.Run{Steps: tt.steps}
			stats := computeStepStats(wf)

			if stats.Total != tt.expected.Total {
				t.Errorf("expected %d total, got %d", tt.expected.Total, stats.Total)
			}
			if stats.Done != tt.expected.Done {
				t.Errorf("expected %d done, got %d", tt.expected.Done, stats.Done)
			}
			if stats.Running != tt.expected.Running {
				t.Errorf("expected %d running, got %d", tt.expected.Running, stats.Running)
			}
			if stats.Pending != tt.expected.Pending {
				t.Errorf("expected %d pending, got %d", tt.expected.Pending, stats.Pending)
			}
			if stats.Failed != tt.expected.Failed {
				t.Errorf("expected %d failed, got %d", tt.expected.Failed, stats.Failed)
			}
			if stats.Skipped != tt.expected.Skipped {
				t.Errorf("expected %d skipped, got %d", tt.expected.Skipped, stats.Skipped)
			}
		})
	}
}

func TestWorkflowSummaryWithErrors(t *testing.T) {
	wf := &types.Run{
		ID:       "run-test",
		Template: "test.meow.toml",
		Status:   types.RunStatusFailed,
		Steps: map[string]*types.Step{
			"step1": {
				ID:     "step1",
				Status: types.StepStatusFailed,
				Error:  &types.StepError{Message: "command failed with exit code 1"},
			},
			"step2": {
				ID:     "step2",
				Status: types.StepStatusFailed,
				Error:  &types.StepError{Message: "timeout exceeded"},
			},
		},
	}

	summary := NewWorkflowSummary(wf)

	if len(summary.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(summary.Errors))
	}

	// Errors should be collected
	foundError1 := false
	foundError2 := false
	for _, err := range summary.Errors {
		if err == "command failed with exit code 1" {
			foundError1 = true
		}
		if err == "timeout exceeded" {
			foundError2 = true
		}
	}

	if !foundError1 || !foundError2 {
		t.Errorf("not all errors were collected: %v", summary.Errors)
	}
}
