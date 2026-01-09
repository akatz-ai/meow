package types

import (
	"testing"
)

func TestWorkflowStatus(t *testing.T) {
	t.Run("Valid returns true for valid statuses", func(t *testing.T) {
		valid := []WorkflowStatus{
			WorkflowStatusPending, WorkflowStatusRunning,
			WorkflowStatusDone, WorkflowStatusFailed,
		}
		for _, s := range valid {
			if !s.Valid() {
				t.Errorf("%s should be valid", s)
			}
		}
	})

	t.Run("IsTerminal", func(t *testing.T) {
		if !WorkflowStatusDone.IsTerminal() {
			t.Error("done should be terminal")
		}
		if !WorkflowStatusFailed.IsTerminal() {
			t.Error("failed should be terminal")
		}
		if WorkflowStatusRunning.IsTerminal() {
			t.Error("running should not be terminal")
		}
	})
}

func TestNewWorkflow(t *testing.T) {
	vars := map[string]string{"agent": "claude-1"}
	wf := NewWorkflow("wf-123", "test.meow.toml", vars)

	if wf.ID != "wf-123" {
		t.Errorf("ID = %s, want wf-123", wf.ID)
	}
	if wf.Template != "test.meow.toml" {
		t.Errorf("Template = %s, want test.meow.toml", wf.Template)
	}
	if wf.Status != WorkflowStatusPending {
		t.Errorf("Status = %s, want pending", wf.Status)
	}
	if wf.Variables["agent"] != "claude-1" {
		t.Error("Variables not set")
	}
	if wf.Agents == nil {
		t.Error("Agents map should be initialized")
	}
	if wf.Steps == nil {
		t.Error("Steps map should be initialized")
	}
}

func TestWorkflowAddStep(t *testing.T) {
	wf := NewWorkflow("wf-1", "test.meow.toml", nil)

	step := &Step{
		ID:       "step-1",
		Executor: ExecutorShell,
		Shell:    &ShellConfig{Command: "echo hello"},
	}

	if err := wf.AddStep(step); err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Try adding duplicate
	if err := wf.AddStep(step); err == nil {
		t.Error("expected error for duplicate step")
	}
}

func TestWorkflowRegisterAgent(t *testing.T) {
	wf := NewWorkflow("wf-1", "test.meow.toml", nil)

	info := &AgentInfo{
		TmuxSession: "meow-wf-1-claude",
		Status:      "active",
		Workdir:     "/tmp/work",
	}

	wf.RegisterAgent("claude-1", info)

	workdir, ok := wf.GetAgentWorkdir("claude-1")
	if !ok {
		t.Error("agent should exist")
	}
	if workdir != "/tmp/work" {
		t.Errorf("workdir = %s, want /tmp/work", workdir)
	}

	_, ok = wf.GetAgentWorkdir("nonexistent")
	if ok {
		t.Error("nonexistent agent should not be found")
	}
}

func TestWorkflowGetReadySteps(t *testing.T) {
	wf := NewWorkflow("wf-1", "test.meow.toml", nil)

	// Add steps with dependencies
	wf.Steps["step1"] = &Step{
		ID:     "step1",
		Status: StepStatusDone,
	}
	wf.Steps["step2"] = &Step{
		ID:     "step2",
		Status: StepStatusPending,
		Needs:  []string{"step1"},
	}
	wf.Steps["step3"] = &Step{
		ID:     "step3",
		Status: StepStatusPending,
		Needs:  []string{"step2"}, // Not ready (step2 not done)
	}

	ready := wf.GetReadySteps()
	if len(ready) != 1 {
		t.Errorf("expected 1 ready step, got %d", len(ready))
	}
	if ready[0].ID != "step2" {
		t.Errorf("expected step2 to be ready, got %s", ready[0].ID)
	}
}

func TestWorkflowAllDone(t *testing.T) {
	wf := NewWorkflow("wf-1", "test.meow.toml", nil)

	// Empty workflow is done
	if !wf.AllDone() {
		t.Error("empty workflow should be done")
	}

	wf.Steps["step1"] = &Step{ID: "step1", Status: StepStatusDone}
	if !wf.AllDone() {
		t.Error("all steps done, workflow should be done")
	}

	wf.Steps["step2"] = &Step{ID: "step2", Status: StepStatusRunning}
	if wf.AllDone() {
		t.Error("running step, workflow should not be done")
	}
}

func TestWorkflowHasFailed(t *testing.T) {
	wf := NewWorkflow("wf-1", "test.meow.toml", nil)

	wf.Steps["step1"] = &Step{ID: "step1", Status: StepStatusDone}
	if wf.HasFailed() {
		t.Error("no failed steps, should return false")
	}

	wf.Steps["step2"] = &Step{ID: "step2", Status: StepStatusFailed}
	if !wf.HasFailed() {
		t.Error("failed step exists, should return true")
	}
}

func TestWorkflowLifecycle(t *testing.T) {
	wf := NewWorkflow("wf-1", "test.meow.toml", nil)

	if err := wf.Start(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if wf.Status != WorkflowStatusRunning {
		t.Errorf("expected running, got %s", wf.Status)
	}

	// Can't start again
	if err := wf.Start(); err == nil {
		t.Error("expected error starting running workflow")
	}

	wf.Complete()
	if wf.Status != WorkflowStatusDone {
		t.Errorf("expected done, got %s", wf.Status)
	}
	if wf.DoneAt == nil {
		t.Error("DoneAt should be set")
	}
}

func TestWorkflowGetStepsForAgent(t *testing.T) {
	wf := NewWorkflow("wf-1", "test.meow.toml", nil)

	wf.Steps["step1"] = &Step{
		ID:       "step1",
		Executor: ExecutorAgent,
		Agent:    &AgentConfig{Agent: "claude-1", Prompt: "test"},
	}
	wf.Steps["step2"] = &Step{
		ID:       "step2",
		Executor: ExecutorAgent,
		Agent:    &AgentConfig{Agent: "claude-2", Prompt: "test"},
	}
	wf.Steps["step3"] = &Step{
		ID:       "step3",
		Executor: ExecutorShell,
		Shell:    &ShellConfig{Command: "echo"},
	}

	steps := wf.GetStepsForAgent("claude-1")
	if len(steps) != 1 {
		t.Errorf("expected 1 step for claude-1, got %d", len(steps))
	}
}

func TestWorkflowAgentIsIdle(t *testing.T) {
	wf := NewWorkflow("wf-1", "test.meow.toml", nil)

	wf.Steps["step1"] = &Step{
		ID:       "step1",
		Executor: ExecutorAgent,
		Status:   StepStatusPending,
		Agent:    &AgentConfig{Agent: "claude-1", Prompt: "test"},
	}

	if !wf.AgentIsIdle("claude-1") {
		t.Error("agent should be idle (step pending)")
	}

	wf.Steps["step1"].Status = StepStatusRunning
	if wf.AgentIsIdle("claude-1") {
		t.Error("agent should not be idle (step running)")
	}
}

func TestWorkflowGetRunningStepForAgent(t *testing.T) {
	wf := NewWorkflow("wf-1", "test.meow.toml", nil)

	wf.Steps["step1"] = &Step{
		ID:       "step1",
		Executor: ExecutorAgent,
		Status:   StepStatusRunning,
		Agent:    &AgentConfig{Agent: "claude-1", Prompt: "test"},
	}

	step := wf.GetRunningStepForAgent("claude-1")
	if step == nil {
		t.Error("should find running step for claude-1")
	}
	if step.ID != "step1" {
		t.Errorf("expected step1, got %s", step.ID)
	}

	step = wf.GetRunningStepForAgent("claude-2")
	if step != nil {
		t.Error("should not find running step for claude-2")
	}
}
