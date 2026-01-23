package types

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRunStatus(t *testing.T) {
	t.Run("Valid returns true for valid statuses", func(t *testing.T) {
		valid := []RunStatus{
			RunStatusPending, RunStatusRunning,
			RunStatusDone, RunStatusFailed,
		}
		for _, s := range valid {
			if !s.Valid() {
				t.Errorf("%s should be valid", s)
			}
		}
	})

	t.Run("IsTerminal", func(t *testing.T) {
		if !RunStatusDone.IsTerminal() {
			t.Error("done should be terminal")
		}
		if !RunStatusFailed.IsTerminal() {
			t.Error("failed should be terminal")
		}
		if RunStatusRunning.IsTerminal() {
			t.Error("running should not be terminal")
		}
	})
}

func TestNewRun(t *testing.T) {
	vars := map[string]any{"agent": "claude-1"}
	run := NewRun("run-123", "test.meow.toml", vars)

	if run.ID != "run-123" {
		t.Errorf("ID = %s, want run-123", run.ID)
	}
	if run.Template != "test.meow.toml" {
		t.Errorf("Template = %s, want test.meow.toml", run.Template)
	}
	if run.Status != RunStatusPending {
		t.Errorf("Status = %s, want pending", run.Status)
	}
	if run.Variables["agent"] != "claude-1" {
		t.Error("Variables not set")
	}
	if run.Agents == nil {
		t.Error("Agents map should be initialized")
	}
	if run.Steps == nil {
		t.Error("Steps map should be initialized")
	}
}

func TestRunAddStep(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	step := &Step{
		ID:       "step-1",
		Executor: ExecutorShell,
		Shell:    &ShellConfig{Command: "echo hello"},
	}

	if err := run.AddStep(step); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Try adding duplicate
	if err := run.AddStep(step); err == nil {
		t.Error("expected error for duplicate step")
	}
}

func TestRunRegisterAgent(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	info := &AgentInfo{
		TmuxSession: "meow-run-1-claude",
		Status:      "active",
		Workdir:     "/tmp/work",
	}

	run.RegisterAgent("claude-1", info)

	workdir, ok := run.GetAgentWorkdir("claude-1")
	if !ok {
		t.Error("agent should exist")
	}
	if workdir != "/tmp/work" {
		t.Errorf("workdir = %s, want /tmp/work", workdir)
	}

	_, ok = run.GetAgentWorkdir("nonexistent")
	if ok {
		t.Error("nonexistent agent should not be found")
	}
}

func TestRunGetReadySteps(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	// Add steps with dependencies
	run.Steps["step1"] = &Step{
		ID:     "step1",
		Status: StepStatusDone,
	}
	run.Steps["step2"] = &Step{
		ID:     "step2",
		Status: StepStatusPending,
		Needs:  []string{"step1"},
	}
	run.Steps["step3"] = &Step{
		ID:     "step3",
		Status: StepStatusPending,
		Needs:  []string{"step2"}, // Not ready (step2 not done)
	}

	ready := run.GetReadySteps()
	if len(ready) != 1 {
		t.Errorf("expected 1 ready step, got %d", len(ready))
	}
	if ready[0].ID != "step2" {
		t.Errorf("expected step2 to be ready, got %s", ready[0].ID)
	}
}

func TestRunGetReadyStepsDeterministic(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	// Add multiple ready steps with different IDs
	// Maps in Go have non-deterministic iteration order,
	// so we need to verify GetReadySteps returns consistent order
	run.Steps["zebra"] = &Step{
		ID:     "zebra",
		Status: StepStatusPending,
	}
	run.Steps["apple"] = &Step{
		ID:     "apple",
		Status: StepStatusPending,
	}
	run.Steps["middle"] = &Step{
		ID:     "middle",
		Status: StepStatusPending,
	}
	run.Steps["banana"] = &Step{
		ID:     "banana",
		Status: StepStatusPending,
	}

	// Get ready steps multiple times
	firstCall := run.GetReadySteps()
	secondCall := run.GetReadySteps()
	thirdCall := run.GetReadySteps()

	// Should return same number of steps
	if len(firstCall) != 4 {
		t.Errorf("expected 4 ready steps, got %d", len(firstCall))
	}

	// Should return steps in same order every time
	for i := 0; i < len(firstCall); i++ {
		if firstCall[i].ID != secondCall[i].ID {
			t.Errorf("order changed between calls: first[%d]=%s, second[%d]=%s",
				i, firstCall[i].ID, i, secondCall[i].ID)
		}
		if firstCall[i].ID != thirdCall[i].ID {
			t.Errorf("order changed between calls: first[%d]=%s, third[%d]=%s",
				i, firstCall[i].ID, i, thirdCall[i].ID)
		}
	}

	// Should be sorted alphabetically by ID
	expected := []string{"apple", "banana", "middle", "zebra"}
	for i, step := range firstCall {
		if step.ID != expected[i] {
			t.Errorf("steps not sorted: expected[%d]=%s, got %s",
				i, expected[i], step.ID)
		}
	}
}

func TestRunAllDone(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	// Empty run is done
	if !run.AllDone() {
		t.Error("empty run should be done")
	}

	run.Steps["step1"] = &Step{ID: "step1", Status: StepStatusDone}
	if !run.AllDone() {
		t.Error("all steps done, run should be done")
	}

	run.Steps["step2"] = &Step{ID: "step2", Status: StepStatusRunning}
	if run.AllDone() {
		t.Error("running step, run should not be done")
	}
}

func TestRunHasFailed(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	run.Steps["step1"] = &Step{ID: "step1", Status: StepStatusDone}
	if run.HasFailed() {
		t.Error("no failed steps, should return false")
	}

	run.Steps["step2"] = &Step{ID: "step2", Status: StepStatusFailed}
	if !run.HasFailed() {
		t.Error("failed step exists, should return true")
	}
}

func TestRunLifecycle(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	if err := run.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if run.Status != RunStatusRunning {
		t.Errorf("expected running, got %s", run.Status)
	}

	// Can't start again
	if err := run.Start(); err == nil {
		t.Error("expected error starting running run")
	}

	run.Complete()
	if run.Status != RunStatusDone {
		t.Errorf("expected done, got %s", run.Status)
	}
	if run.DoneAt == nil {
		t.Error("DoneAt should be set")
	}
}

func TestRunGetStepsForAgent(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	run.Steps["step1"] = &Step{
		ID:       "step1",
		Executor: ExecutorAgent,
		Agent:    &AgentConfig{Agent: "claude-1", Prompt: "test"},
	}
	run.Steps["step2"] = &Step{
		ID:       "step2",
		Executor: ExecutorAgent,
		Agent:    &AgentConfig{Agent: "claude-2", Prompt: "test"},
	}
	run.Steps["step3"] = &Step{
		ID:       "step3",
		Executor: ExecutorShell,
		Shell:    &ShellConfig{Command: "echo"},
	}

	steps := run.GetStepsForAgent("claude-1")
	if len(steps) != 1 {
		t.Errorf("expected 1 step for claude-1, got %d", len(steps))
	}
}

func TestRunAgentIsIdle(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	run.Steps["step1"] = &Step{
		ID:       "step1",
		Executor: ExecutorAgent,
		Status:   StepStatusPending,
		Agent:    &AgentConfig{Agent: "claude-1", Prompt: "test"},
	}

	if !run.AgentIsIdle("claude-1") {
		t.Error("agent should be idle (step pending)")
	}

	run.Steps["step1"].Status = StepStatusRunning
	if run.AgentIsIdle("claude-1") {
		t.Error("agent should not be idle (step running)")
	}

	// Critical: completing status should also make agent not idle
	// This prevents injecting a new prompt while orchestrator is processing completion
	run.Steps["step1"].Status = StepStatusCompleting
	if run.AgentIsIdle("claude-1") {
		t.Error("agent should not be idle (step completing)")
	}

	// Done status should make agent idle again
	run.Steps["step1"].Status = StepStatusDone
	if !run.AgentIsIdle("claude-1") {
		t.Error("agent should be idle (step done)")
	}
}

func TestRunGetRunningStepForAgent(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	run.Steps["step1"] = &Step{
		ID:       "step1",
		Executor: ExecutorAgent,
		Status:   StepStatusRunning,
		Agent:    &AgentConfig{Agent: "claude-1", Prompt: "test"},
	}

	step := run.GetRunningStepForAgent("claude-1")
	if step == nil {
		t.Error("should find running step for claude-1")
	}
	if step.ID != "step1" {
		t.Errorf("expected step1, got %s", step.ID)
	}

	step = run.GetRunningStepForAgent("claude-2")
	if step != nil {
		t.Error("should not find running step for claude-2")
	}

	// GetRunningStepForAgent should also return steps in completing status
	// (the orchestrator is still processing the completion)
	run.Steps["step1"].Status = StepStatusCompleting
	step = run.GetRunningStepForAgent("claude-1")
	if step == nil {
		t.Error("should find completing step for claude-1")
	}
	if step.Status != StepStatusCompleting {
		t.Errorf("expected completing status, got %s", step.Status)
	}

	// Done step should not be returned
	run.Steps["step1"].Status = StepStatusDone
	step = run.GetRunningStepForAgent("claude-1")
	if step != nil {
		t.Error("should not find done step")
	}
}

func TestRunOrchestratorPID(t *testing.T) {
	t.Run("marshals and unmarshals with PID", func(t *testing.T) {
		run := NewRun("run-1", "test.meow.toml", nil)
		run.OrchestratorPID = 12345

		// Marshal to YAML
		data, err := yaml.Marshal(run)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		// Unmarshal back
		var run2 Run
		if err := yaml.Unmarshal(data, &run2); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}

		if run2.OrchestratorPID != 12345 {
			t.Errorf("OrchestratorPID = %d, want 12345", run2.OrchestratorPID)
		}
	})

	t.Run("omits PID when zero (backwards compatibility)", func(t *testing.T) {
		run := NewRun("run-1", "test.meow.toml", nil)
		run.OrchestratorPID = 0

		// Marshal to YAML
		data, err := yaml.Marshal(run)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}

		// Should not contain orchestrator_pid in YAML
		yamlStr := string(data)
		if contains(yamlStr, "orchestrator_pid") {
			t.Error("orchestrator_pid should be omitted when zero")
		}
	})

	t.Run("loads legacy runs without PID field", func(t *testing.T) {
		// Simulate legacy run YAML without orchestrator_pid field
		legacyYAML := `
id: run-legacy
template: test.meow.toml
status: running
started_at: 2024-01-01T00:00:00Z
steps: {}
`
		var run Run
		if err := yaml.Unmarshal([]byte(legacyYAML), &run); err != nil {
			t.Fatalf("failed to unmarshal legacy run: %v", err)
		}

		if run.OrchestratorPID != 0 {
			t.Errorf("OrchestratorPID should default to 0, got %d", run.OrchestratorPID)
		}
		if run.ID != "run-legacy" {
			t.Errorf("ID = %s, want run-legacy", run.ID)
		}
	})
}

func TestRunAgentHasCompletedSteps(t *testing.T) {
	run := NewRun("run-1", "test.meow.toml", nil)

	// Add steps for different agents with different statuses
	run.Steps["step1"] = &Step{
		ID:       "step1",
		Executor: ExecutorAgent,
		Status:   StepStatusPending,
		Agent:    &AgentConfig{Agent: "claude-1", Prompt: "test"},
	}
	run.Steps["step2"] = &Step{
		ID:       "step2",
		Executor: ExecutorAgent,
		Status:   StepStatusRunning,
		Agent:    &AgentConfig{Agent: "claude-1", Prompt: "test"},
	}
	run.Steps["step3"] = &Step{
		ID:       "step3",
		Executor: ExecutorAgent,
		Status:   StepStatusDone,
		Agent:    &AgentConfig{Agent: "claude-2", Prompt: "test"},
	}
	run.Steps["step4"] = &Step{
		ID:       "step4",
		Executor: ExecutorShell, // Not an agent step
		Status:   StepStatusDone,
		Shell:    &ShellConfig{Command: "echo"},
	}

	// claude-1 has pending and running steps, but none completed
	if run.AgentHasCompletedSteps("claude-1") {
		t.Error("claude-1 should NOT have completed steps (only pending/running)")
	}

	// claude-2 has a completed step
	if !run.AgentHasCompletedSteps("claude-2") {
		t.Error("claude-2 SHOULD have completed steps")
	}

	// Nonexistent agent should return false
	if run.AgentHasCompletedSteps("nonexistent") {
		t.Error("nonexistent agent should NOT have completed steps")
	}

	// Now mark one of claude-1's steps as done
	run.Steps["step1"].Status = StepStatusDone
	if !run.AgentHasCompletedSteps("claude-1") {
		t.Error("claude-1 SHOULD have completed steps after step1 done")
	}
}

// contains is a helper to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
