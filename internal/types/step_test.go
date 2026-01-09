package types

import (
	"testing"
)

func TestExecutorType(t *testing.T) {
	t.Run("Valid returns true for valid executors", func(t *testing.T) {
		validExecutors := []ExecutorType{
			ExecutorShell, ExecutorSpawn, ExecutorKill,
			ExecutorExpand, ExecutorBranch, ExecutorAgent,
		}
		for _, e := range validExecutors {
			if !e.Valid() {
				t.Errorf("%s should be valid", e)
			}
		}
	})

	t.Run("Valid returns false for invalid executors", func(t *testing.T) {
		invalid := ExecutorType("gate") // Gate is NOT an executor!
		if invalid.Valid() {
			t.Error("gate should not be a valid executor")
		}
	})

	t.Run("IsOrchestrator for orchestrator executors", func(t *testing.T) {
		orchestratorExecutors := []ExecutorType{
			ExecutorShell, ExecutorSpawn, ExecutorKill,
			ExecutorExpand, ExecutorBranch,
		}
		for _, e := range orchestratorExecutors {
			if !e.IsOrchestrator() {
				t.Errorf("%s should be orchestrator executor", e)
			}
		}
	})

	t.Run("IsExternal for agent executor only", func(t *testing.T) {
		if !ExecutorAgent.IsExternal() {
			t.Error("agent should be external executor")
		}
		if ExecutorShell.IsExternal() {
			t.Error("shell should not be external executor")
		}
	})
}

func TestStepStatus(t *testing.T) {
	t.Run("Valid returns true for valid statuses", func(t *testing.T) {
		validStatuses := []StepStatus{
			StepStatusPending, StepStatusRunning, StepStatusCompleting,
			StepStatusDone, StepStatusFailed,
		}
		for _, s := range validStatuses {
			if !s.Valid() {
				t.Errorf("%s should be valid", s)
			}
		}
	})

	t.Run("IsTerminal", func(t *testing.T) {
		if !StepStatusDone.IsTerminal() {
			t.Error("done should be terminal")
		}
		if !StepStatusFailed.IsTerminal() {
			t.Error("failed should be terminal")
		}
		if StepStatusRunning.IsTerminal() {
			t.Error("running should not be terminal")
		}
	})

	t.Run("CanTransitionTo valid transitions", func(t *testing.T) {
		tests := []struct {
			from, to StepStatus
			ok       bool
		}{
			{StepStatusPending, StepStatusRunning, true},
			{StepStatusPending, StepStatusDone, false},
			{StepStatusRunning, StepStatusCompleting, true},
			{StepStatusRunning, StepStatusDone, true},
			{StepStatusRunning, StepStatusFailed, true},
			{StepStatusRunning, StepStatusPending, true}, // crash recovery
			{StepStatusCompleting, StepStatusDone, true},
			{StepStatusCompleting, StepStatusRunning, true}, // validation failed
			{StepStatusDone, StepStatusPending, false},      // terminal
			{StepStatusFailed, StepStatusPending, false},    // terminal
		}

		for _, tt := range tests {
			got := tt.from.CanTransitionTo(tt.to)
			if got != tt.ok {
				t.Errorf("CanTransitionTo(%s -> %s) = %v, want %v",
					tt.from, tt.to, got, tt.ok)
			}
		}
	})
}

func TestStepValidate(t *testing.T) {
	t.Run("valid shell step", func(t *testing.T) {
		step := &Step{
			ID:       "test-step",
			Executor: ExecutorShell,
			Status:   StepStatusPending,
			Shell: &ShellConfig{
				Command: "echo hello",
			},
		}
		if err := step.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("step ID required", func(t *testing.T) {
		step := &Step{
			Executor: ExecutorShell,
			Shell:    &ShellConfig{Command: "echo"},
		}
		if err := step.Validate(); err == nil {
			t.Error("expected error for missing ID")
		}
	})

	t.Run("step ID cannot contain dots (non-expanded)", func(t *testing.T) {
		step := &Step{
			ID:       "parent.child",
			Executor: ExecutorShell,
			Shell:    &ShellConfig{Command: "echo"},
		}
		if err := step.Validate(); err == nil {
			t.Error("expected error for dot in ID")
		}
	})

	t.Run("expanded step ID can contain dots", func(t *testing.T) {
		step := &Step{
			ID:           "parent.child",
			Executor:     ExecutorShell,
			Shell:        &ShellConfig{Command: "echo"},
			ExpandedFrom: "parent", // This is an expanded step
		}
		if err := step.Validate(); err != nil {
			t.Errorf("expanded step should allow dots: %v", err)
		}
	})

	t.Run("missing config for executor", func(t *testing.T) {
		step := &Step{
			ID:       "test",
			Executor: ExecutorShell,
			// No Shell config
		}
		if err := step.Validate(); err == nil {
			t.Error("expected error for missing config")
		}
	})

	t.Run("wrong config for executor", func(t *testing.T) {
		step := &Step{
			ID:       "test",
			Executor: ExecutorShell,
			Agent: &AgentConfig{
				Agent:  "claude",
				Prompt: "hello",
			},
		}
		if err := step.Validate(); err == nil {
			t.Error("expected error for wrong config type")
		}
	})
}

func TestStepIsReady(t *testing.T) {
	steps := map[string]*Step{
		"dep1": {ID: "dep1", Status: StepStatusDone},
		"dep2": {ID: "dep2", Status: StepStatusDone},
		"dep3": {ID: "dep3", Status: StepStatusRunning},
	}

	t.Run("ready when all deps done", func(t *testing.T) {
		step := &Step{
			ID:     "test",
			Status: StepStatusPending,
			Needs:  []string{"dep1", "dep2"},
		}
		if !step.IsReady(steps) {
			t.Error("step should be ready")
		}
	})

	t.Run("not ready when dep not done", func(t *testing.T) {
		step := &Step{
			ID:     "test",
			Status: StepStatusPending,
			Needs:  []string{"dep1", "dep3"},
		}
		if step.IsReady(steps) {
			t.Error("step should not be ready (dep3 still running)")
		}
	})

	t.Run("not ready when not pending", func(t *testing.T) {
		step := &Step{
			ID:     "test",
			Status: StepStatusRunning,
			Needs:  []string{"dep1"},
		}
		if step.IsReady(steps) {
			t.Error("step should not be ready (already running)")
		}
	})
}

func TestStepLifecycle(t *testing.T) {
	t.Run("Start sets running status", func(t *testing.T) {
		step := &Step{ID: "test", Status: StepStatusPending}
		if err := step.Start(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if step.Status != StepStatusRunning {
			t.Errorf("expected running, got %s", step.Status)
		}
		if step.StartedAt == nil {
			t.Error("StartedAt should be set")
		}
	})

	t.Run("SetCompleting transitions from running", func(t *testing.T) {
		step := &Step{ID: "test", Status: StepStatusRunning}
		if err := step.SetCompleting(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if step.Status != StepStatusCompleting {
			t.Errorf("expected completing, got %s", step.Status)
		}
	})

	t.Run("Complete sets done with outputs", func(t *testing.T) {
		step := &Step{ID: "test", Status: StepStatusRunning}
		outputs := map[string]any{"result": "success"}
		if err := step.Complete(outputs); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if step.Status != StepStatusDone {
			t.Errorf("expected done, got %s", step.Status)
		}
		if step.Outputs["result"] != "success" {
			t.Error("outputs not set")
		}
	})

	t.Run("Fail sets failed with error", func(t *testing.T) {
		step := &Step{ID: "test", Status: StepStatusRunning}
		stepErr := &StepError{Message: "something broke", Code: 1}
		if err := step.Fail(stepErr); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if step.Status != StepStatusFailed {
			t.Errorf("expected failed, got %s", step.Status)
		}
		if step.Error.Message != "something broke" {
			t.Error("error not set")
		}
	})

	t.Run("ResetToPending only from running", func(t *testing.T) {
		step := &Step{ID: "test", Status: StepStatusRunning}
		if err := step.ResetToPending(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if step.Status != StepStatusPending {
			t.Errorf("expected pending, got %s", step.Status)
		}

		// Try from pending (should fail)
		if err := step.ResetToPending(); err == nil {
			t.Error("expected error resetting from pending")
		}
	})
}
