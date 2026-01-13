package workflow

import (
	"strings"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

func fixedTime() time.Time {
	return time.Date(2026, 1, 7, 12, 0, 0, 0, time.UTC)
}

// TestBakeWorkflow_ReturnsSteps verifies that BakeWorkflow returns Steps
func TestBakeWorkflow_ReturnsSteps(t *testing.T) {
	workflow := &Workflow{
		Name: "test-workflow",
		Steps: []*Step{
			{
				ID:       "task-1",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Do something useful",
			},
		},
	}

	baker := NewBaker("run-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if result.Steps == nil {
		t.Fatal("expected Steps in result, got nil")
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorAgent {
		t.Errorf("expected agent executor, got %s", step.Executor)
	}
	if step.Status != types.StepStatusPending {
		t.Errorf("expected pending status, got %s", step.Status)
	}
}

// TestBakeWorkflow_ShellExecutor tests shell executor step creation
func TestBakeWorkflow_ShellExecutor(t *testing.T) {
	workflow := &Workflow{
		Name: "shell-test",
		Steps: []*Step{
			{
				ID:       "run-cmd",
				Executor: ExecutorShell,
				Command:  "echo 'hello world'",
				Workdir:  "/tmp",
				Env:      map[string]string{"FOO": "bar"},
				OnError:  "continue",
			},
		},
	}

	baker := NewBaker("run-shell-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.ID != "run-cmd" {
		t.Errorf("expected step ID 'run-cmd', got %q", step.ID)
	}
	if step.Executor != types.ExecutorShell {
		t.Errorf("expected shell executor, got %s", step.Executor)
	}
	if step.Shell == nil {
		t.Fatal("expected ShellConfig, got nil")
	}
	if step.Shell.Command != "echo 'hello world'" {
		t.Errorf("expected command, got %q", step.Shell.Command)
	}
	if step.Shell.Workdir != "/tmp" {
		t.Errorf("expected workdir '/tmp', got %q", step.Shell.Workdir)
	}
	if step.Shell.Env["FOO"] != "bar" {
		t.Errorf("expected env FOO=bar, got %v", step.Shell.Env)
	}
	if step.Shell.OnError != "continue" {
		t.Errorf("expected on_error 'continue', got %q", step.Shell.OnError)
	}
}

// TestBakeWorkflow_SpawnExecutor tests spawn executor step creation
func TestBakeWorkflow_SpawnExecutor(t *testing.T) {
	workflow := &Workflow{
		Name: "spawn-test",
		Steps: []*Step{
			{
				ID:            "start-agent",
				Executor:      ExecutorSpawn,
				Agent:         "claude-worker",
				Workdir:       "/project",
				Env:           map[string]string{"MEOW_WORKFLOW": "test"},
				ResumeSession: "session-123",
				SpawnArgs:     "--model opus --verbose",
			},
		},
	}

	baker := NewBaker("run-spawn-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorSpawn {
		t.Errorf("expected spawn executor, got %s", step.Executor)
	}
	if step.Spawn == nil {
		t.Fatal("expected SpawnConfig, got nil")
	}
	if step.Spawn.Agent != "claude-worker" {
		t.Errorf("expected agent 'claude-worker', got %q", step.Spawn.Agent)
	}
	if step.Spawn.Workdir != "/project" {
		t.Errorf("expected workdir '/project', got %q", step.Spawn.Workdir)
	}
	if step.Spawn.ResumeSession != "session-123" {
		t.Errorf("expected resume_session 'session-123', got %q", step.Spawn.ResumeSession)
	}
	if step.Spawn.SpawnArgs != "--model opus --verbose" {
		t.Errorf("expected spawn_args '--model opus --verbose', got %q", step.Spawn.SpawnArgs)
	}
}

// TestBakeWorkflow_KillExecutor tests kill executor step creation
func TestBakeWorkflow_KillExecutor(t *testing.T) {
	graceful := true
	workflow := &Workflow{
		Name: "kill-test",
		Steps: []*Step{
			{
				ID:       "stop-agent",
				Executor: ExecutorKill,
				Agent:    "claude-worker",
				Graceful: &graceful,
				Timeout:  "30s",
			},
		},
	}

	baker := NewBaker("run-kill-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorKill {
		t.Errorf("expected kill executor, got %s", step.Executor)
	}
	if step.Kill == nil {
		t.Fatal("expected KillConfig, got nil")
	}
	if step.Kill.Agent != "claude-worker" {
		t.Errorf("expected agent 'claude-worker', got %q", step.Kill.Agent)
	}
	if !step.Kill.Graceful {
		t.Error("expected graceful=true")
	}
}

// TestBakeWorkflow_ExpandExecutor tests expand executor step creation
func TestBakeWorkflow_ExpandExecutor(t *testing.T) {
	workflow := &Workflow{
		Name: "expand-test",
		Steps: []*Step{
			{
				ID:        "do-expand",
				Executor:  ExecutorExpand,
				Template:  "sub-workflow",
				Variables: map[string]string{"task": "build"},
			},
		},
	}

	baker := NewBaker("run-expand-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorExpand {
		t.Errorf("expected expand executor, got %s", step.Executor)
	}
	if step.Expand == nil {
		t.Fatal("expected ExpandConfig, got nil")
	}
	if step.Expand.Template != "sub-workflow" {
		t.Errorf("expected template 'sub-workflow', got %q", step.Expand.Template)
	}
	if step.Expand.Variables["task"] != "build" {
		t.Errorf("expected variable task=build, got %v", step.Expand.Variables)
	}
}

// TestBakeWorkflow_BranchExecutor tests branch executor step creation
func TestBakeWorkflow_BranchExecutor(t *testing.T) {
	workflow := &Workflow{
		Name: "branch-test",
		Steps: []*Step{
			{
				ID:        "check-flag",
				Executor:  ExecutorBranch,
				Condition: "test -f /tmp/ready",
				OnTrue:    &ExpansionTarget{Template: "proceed"},
				OnFalse:   &ExpansionTarget{Template: "wait"},
				Timeout:   "5m",
			},
		},
	}

	baker := NewBaker("run-branch-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorBranch {
		t.Errorf("expected branch executor, got %s", step.Executor)
	}
	if step.Branch == nil {
		t.Fatal("expected BranchConfig, got nil")
	}
	if step.Branch.Condition != "test -f /tmp/ready" {
		t.Errorf("expected condition, got %q", step.Branch.Condition)
	}
	if step.Branch.OnTrue == nil || step.Branch.OnTrue.Template != "proceed" {
		t.Errorf("expected on_true template 'proceed'")
	}
	if step.Branch.OnFalse == nil || step.Branch.OnFalse.Template != "wait" {
		t.Errorf("expected on_false template 'wait'")
	}
	if step.Branch.Timeout != "5m" {
		t.Errorf("expected timeout '5m', got %q", step.Branch.Timeout)
	}
}

// TestBakeWorkflow_AgentExecutor tests agent executor step creation
func TestBakeWorkflow_AgentExecutor(t *testing.T) {
	workflow := &Workflow{
		Name: "agent-test",
		Steps: []*Step{
			{
				ID:       "do-work",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Implement the feature",
				Mode:     "autonomous",
				Timeout:  "1h",
				Outputs: map[string]AgentOutputDef{
					"result": {Required: true, Type: "string", Description: "The result"},
				},
			},
		},
	}

	baker := NewBaker("run-agent-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorAgent {
		t.Errorf("expected agent executor, got %s", step.Executor)
	}
	if step.Agent == nil {
		t.Fatal("expected AgentConfig, got nil")
	}
	if step.Agent.Agent != "claude" {
		t.Errorf("expected agent 'claude', got %q", step.Agent.Agent)
	}
	if step.Agent.Prompt != "Implement the feature" {
		t.Errorf("expected prompt, got %q", step.Agent.Prompt)
	}
	if step.Agent.Mode != "autonomous" {
		t.Errorf("expected mode 'autonomous', got %q", step.Agent.Mode)
	}
	if step.Agent.Timeout != "1h" {
		t.Errorf("expected timeout '1h', got %q", step.Agent.Timeout)
	}
	if step.Agent.Outputs == nil {
		t.Fatal("expected outputs")
	}
	if step.Agent.Outputs["result"].Required != true {
		t.Error("expected result output to be required")
	}
}

// TestBakeWorkflow_Dependencies tests that step dependencies are preserved
func TestBakeWorkflow_Dependencies(t *testing.T) {
	workflow := &Workflow{
		Name: "deps-test",
		Steps: []*Step{
			{
				ID:       "first",
				Executor: ExecutorShell,
				Command:  "echo first",
			},
			{
				ID:       "second",
				Executor: ExecutorShell,
				Command:  "echo second",
				Needs:    []string{"first"},
			},
			{
				ID:       "third",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Do third",
				Needs:    []string{"first", "second"},
			},
		},
	}

	baker := NewBaker("run-deps-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result.Steps))
	}

	// Find steps by ID
	stepByID := make(map[string]*types.Step)
	for _, s := range result.Steps {
		stepByID[s.ID] = s
	}

	first := stepByID["first"]
	second := stepByID["second"]
	third := stepByID["third"]

	if first == nil || second == nil || third == nil {
		t.Fatal("missing expected steps")
	}

	if len(first.Needs) != 0 {
		t.Errorf("first should have no deps, got %v", first.Needs)
	}
	if len(second.Needs) != 1 || second.Needs[0] != "first" {
		t.Errorf("second should depend on first, got %v", second.Needs)
	}
	if len(third.Needs) != 2 {
		t.Errorf("third should have 2 deps, got %v", third.Needs)
	}
}

// TestBakeWorkflow_VariableSubstitution tests variable substitution in step fields
func TestBakeWorkflow_VariableSubstitution(t *testing.T) {
	workflow := &Workflow{
		Name: "var-test",
		Variables: map[string]*Var{
			"target": {Required: true},
			"agent":  {Default: "claude"},
		},
		Steps: []*Step{
			{
				ID:       "task",
				Executor: ExecutorAgent,
				Agent:    "{{agent}}",
				Prompt:   "Work on {{target}}",
			},
			{
				ID:       "check",
				Executor: ExecutorShell,
				Command:  "test -f {{target}}.done",
			},
		},
	}

	baker := NewBaker("run-var-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, map[string]string{
		"target": "feature-x",
	})
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}

	// Find the agent step
	var agentStep *types.Step
	for _, s := range result.Steps {
		if s.ID == "task" {
			agentStep = s
			break
		}
	}

	if agentStep == nil {
		t.Fatal("agent step not found")
	}
	if agentStep.Agent.Agent != "claude" {
		t.Errorf("expected agent 'claude', got %q", agentStep.Agent.Agent)
	}
	if agentStep.Agent.Prompt != "Work on feature-x" {
		t.Errorf("expected substituted prompt, got %q", agentStep.Agent.Prompt)
	}

	// Find the shell step
	var shellStep *types.Step
	for _, s := range result.Steps {
		if s.ID == "check" {
			shellStep = s
			break
		}
	}

	if shellStep == nil {
		t.Fatal("shell step not found")
	}
	if shellStep.Shell.Command != "test -f feature-x.done" {
		t.Errorf("expected substituted command, got %q", shellStep.Shell.Command)
	}
}

// TestBakeWorkflow_StepValidation tests that created steps are valid
func TestBakeWorkflow_StepValidation(t *testing.T) {
	workflow := &Workflow{
		Name: "validation-test",
		Steps: []*Step{
			{
				ID:       "shell-step",
				Executor: ExecutorShell,
				Command:  "echo test",
			},
			{
				ID:       "agent-step",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Do work",
			},
		},
	}

	baker := NewBaker("run-valid-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	// All steps should pass validation
	for _, step := range result.Steps {
		if err := step.Validate(); err != nil {
			t.Errorf("step %s failed validation: %v", step.ID, err)
		}
	}
}

// TestBakeWorkflow_NilWorkflow tests error handling for nil workflow
func TestBakeWorkflow_NilWorkflow(t *testing.T) {
	baker := NewBaker("run-nil-001")
	_, err := baker.BakeWorkflow(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil workflow")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("expected nil error, got: %v", err)
	}
}

// TestBakeWorkflow_MissingRequiredVariable tests error handling for missing required variables
func TestBakeWorkflow_MissingRequiredVariable(t *testing.T) {
	workflow := &Workflow{
		Name: "required-var-test",
		Variables: map[string]*Var{
			"task_id": {Required: true},
		},
		Steps: []*Step{
			{ID: "step-1", Executor: ExecutorAgent, Prompt: "Test"},
		},
	}

	baker := NewBaker("run-var-001")
	baker.Now = fixedTime

	_, err := baker.BakeWorkflow(workflow, nil)
	if err == nil {
		t.Fatal("expected error for missing required variable")
	}
	if !strings.Contains(err.Error(), "task_id") {
		t.Errorf("expected error about task_id, got: %v", err)
	}
}

// TestBakeWorkflow_DefaultVariable tests that default variables are applied
func TestBakeWorkflow_DefaultVariable(t *testing.T) {
	workflow := &Workflow{
		Name: "default-var-test",
		Variables: map[string]*Var{
			"framework": {Default: "pytest"},
		},
		Steps: []*Step{
			{ID: "step-1", Executor: ExecutorAgent, Prompt: "Using {{framework}}"},
		},
	}

	baker := NewBaker("run-default-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Agent == nil {
		t.Fatal("expected AgentConfig")
	}
	if step.Agent.Prompt != "Using pytest" {
		t.Errorf("expected default variable substitution, got %q", step.Agent.Prompt)
	}
}

// TestBakeWorkflow_VariableOverride tests that provided variables override defaults
func TestBakeWorkflow_VariableOverride(t *testing.T) {
	workflow := &Workflow{
		Name: "var-override-test",
		Variables: map[string]*Var{
			"framework": {Default: "pytest"},
		},
		Steps: []*Step{
			{ID: "step-1", Executor: ExecutorAgent, Prompt: "Using {{framework}}"},
		},
	}

	baker := NewBaker("run-override-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, map[string]string{
		"framework": "jest",
	})
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Agent == nil {
		t.Fatal("expected AgentConfig")
	}
	if step.Agent.Prompt != "Using jest" {
		t.Errorf("expected override variable, got %q", step.Agent.Prompt)
	}
}

// TestBakeWorkflow_ShellStep tests shell executor
func TestBakeWorkflow_ShellStep(t *testing.T) {
	workflow := &Workflow{
		Name: "shell-test",
		Steps: []*Step{
			{
				ID:       "run-shell",
				Executor: ExecutorShell,
				Command:  "echo 'hello world'",
			},
		},
	}

	baker := NewBaker("run-shell-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorShell {
		t.Errorf("expected shell executor, got %s", step.Executor)
	}
	if step.Shell == nil {
		t.Fatal("expected ShellConfig")
	}
	if step.Shell.Command != "echo 'hello world'" {
		t.Errorf("expected command, got %q", step.Shell.Command)
	}
}

// TestBakeWorkflow_SpawnKillSteps tests spawn/kill executors
func TestBakeWorkflow_SpawnKillSteps(t *testing.T) {
	workflow := &Workflow{
		Name: "agent-control",
		Steps: []*Step{
			{
				ID:       "start-agent",
				Executor: ExecutorSpawn,
				Agent:    "claude-worker",
			},
			{
				ID:       "stop-agent",
				Executor: ExecutorKill,
				Agent:    "claude-worker",
				Needs:    []string{"start-agent"},
			},
		},
	}

	baker := NewBaker("run-agent-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}

	// Check start step
	startStep := result.Steps[0]
	if startStep.Executor != types.ExecutorSpawn {
		t.Errorf("expected spawn executor, got %s", startStep.Executor)
	}
	if startStep.Spawn == nil {
		t.Fatal("expected SpawnConfig")
	}
	if startStep.Spawn.Agent != "claude-worker" {
		t.Errorf("expected agent 'claude-worker', got %q", startStep.Spawn.Agent)
	}

	// Check stop step
	stopStep := result.Steps[1]
	if stopStep.Executor != types.ExecutorKill {
		t.Errorf("expected kill executor, got %s", stopStep.Executor)
	}
	if stopStep.Kill == nil {
		t.Fatal("expected KillConfig")
	}
	if stopStep.Kill.Agent != "claude-worker" {
		t.Errorf("expected agent 'claude-worker', got %q", stopStep.Kill.Agent)
	}
}

// TestBakeWorkflow_ExpandStep tests expand executor
func TestBakeWorkflow_ExpandStep(t *testing.T) {
	workflow := &Workflow{
		Name: "expand-test",
		Steps: []*Step{
			{
				ID:       "expand-impl",
				Executor: ExecutorExpand,
				Template: "implement",
				Variables: map[string]string{
					"task": "bd-42",
				},
			},
		},
	}

	baker := NewBaker("run-expand-001")
	baker.Now = fixedTime
	baker.Assignee = "default-agent"

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorExpand {
		t.Errorf("expected expand executor, got %s", step.Executor)
	}
	if step.Expand == nil {
		t.Fatal("expected ExpandConfig")
	}
	if step.Expand.Template != "implement" {
		t.Errorf("expected template 'implement', got %q", step.Expand.Template)
	}
	if step.Expand.Variables["task"] != "bd-42" {
		t.Errorf("expected variable, got %v", step.Expand.Variables)
	}
}

// TestBakeWorkflow_ConditionStep tests branch executor with condition type
func TestBakeWorkflow_ConditionStep(t *testing.T) {
	workflow := &Workflow{
		Name: "condition-test",
		Steps: []*Step{
			{
				ID:        "check",
				Executor:  ExecutorBranch,
				Condition: "test -f /tmp/ready",
				OnTrue:    &ExpansionTarget{Template: "proceed"},
				OnFalse:   &ExpansionTarget{Template: "wait"},
				Timeout:   "5m",
			},
		},
	}

	baker := NewBaker("run-cond-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorBranch {
		t.Errorf("expected branch executor, got %s", step.Executor)
	}
	if step.Branch == nil {
		t.Fatal("expected BranchConfig")
	}
	if step.Branch.Condition != "test -f /tmp/ready" {
		t.Errorf("expected condition, got %q", step.Branch.Condition)
	}
	if step.Branch.Timeout != "5m" {
		t.Errorf("expected timeout '5m', got %q", step.Branch.Timeout)
	}
	if step.Branch.OnTrue == nil || step.Branch.OnTrue.Template != "proceed" {
		t.Errorf("expected on_true template")
	}
	if step.Branch.OnFalse == nil || step.Branch.OnFalse.Template != "wait" {
		t.Errorf("expected on_false template")
	}
}

// TestBakeWorkflow_ConditionWithVariable tests variable substitution in conditions
func TestBakeWorkflow_ConditionWithVariable(t *testing.T) {
	workflow := &Workflow{
		Name: "cond-var-test",
		Variables: map[string]*Var{
			"file_path": {Required: true},
		},
		Steps: []*Step{
			{
				ID:        "check",
				Executor:  ExecutorBranch,
				Condition: "test -f {{file_path}}",
				OnTrue:    &ExpansionTarget{Template: "proceed"},
				OnFalse:   &ExpansionTarget{Template: "wait"},
			},
		},
	}

	baker := NewBaker("run-cond-var-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, map[string]string{
		"file_path": "/tmp/ready.flag",
	})
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Branch == nil {
		t.Fatal("expected BranchConfig")
	}
	if step.Branch.Condition != "test -f /tmp/ready.flag" {
		t.Errorf("expected substituted condition, got %q", step.Branch.Condition)
	}
}

// TestBakeWorkflow_CodeWithVariable tests variable substitution in shell commands
func TestBakeWorkflow_CodeWithVariable(t *testing.T) {
	workflow := &Workflow{
		Name: "code-var-test",
		Variables: map[string]*Var{
			"test_name": {Required: true},
		},
		Steps: []*Step{
			{
				ID:       "run-code",
				Executor: ExecutorShell,
				Command:  "echo 'Running test: {{test_name}}'",
			},
		},
	}

	baker := NewBaker("run-code-var-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, map[string]string{
		"test_name": "auth-unit-tests",
	})
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorShell {
		t.Errorf("expected shell executor, got %s", step.Executor)
	}
	if step.Shell == nil {
		t.Fatal("expected ShellConfig")
	}
	if step.Shell.Command != "echo 'Running test: auth-unit-tests'" {
		t.Errorf("expected substituted code, got %q", step.Shell.Command)
	}
}

// TestBakeWorkflow_StartStopWithVariableAssignee tests variable substitution in spawn/kill
func TestBakeWorkflow_StartStopWithVariableAssignee(t *testing.T) {
	workflow := &Workflow{
		Name: "agent-control-var",
		Variables: map[string]*Var{
			"agent": {Required: true},
		},
		Steps: []*Step{
			{
				ID:       "start-agent",
				Executor: ExecutorSpawn,
				Agent:    "{{agent}}",
			},
			{
				ID:       "stop-agent",
				Executor: ExecutorKill,
				Agent:    "{{agent}}",
				Needs:    []string{"start-agent"},
			},
		},
	}

	baker := NewBaker("run-agent-var-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, map[string]string{
		"agent": "claude-worker-42",
	})
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}

	startStep := result.Steps[0]
	if startStep.Spawn == nil {
		t.Fatal("expected SpawnConfig")
	}
	if startStep.Spawn.Agent != "claude-worker-42" {
		t.Errorf("expected substituted agent 'claude-worker-42', got %q", startStep.Spawn.Agent)
	}

	stopStep := result.Steps[1]
	if stopStep.Kill == nil {
		t.Fatal("expected KillConfig")
	}
	if stopStep.Kill.Agent != "claude-worker-42" {
		t.Errorf("expected substituted agent 'claude-worker-42', got %q", stopStep.Kill.Agent)
	}
}

// TestBakeWorkflow_DependenciesPreserved tests that dependencies are not validated during baking
func TestBakeWorkflow_DependenciesPreserved(t *testing.T) {
	workflow := &Workflow{
		Name: "deps-test",
		Steps: []*Step{
			{ID: "step-1", Executor: ExecutorAgent, Prompt: "Test", Needs: []string{"nonexistent"}},
		},
	}

	baker := NewBaker("run-deps-001")
	baker.Now = fixedTime

	// Baking doesn't validate dependencies - they're preserved as-is
	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow should not fail for unknown deps: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	// Dependency should be preserved
	if len(result.Steps[0].Needs) != 1 || result.Steps[0].Needs[0] != "nonexistent" {
		t.Errorf("expected dependency to be preserved, got %v", result.Steps[0].Needs)
	}
}

func TestBaker_ForeachConfig(t *testing.T) {
	tomlStr := `
[main]
name = "test-foreach"

[[main.steps]]
id = "iterate"
executor = "foreach"
items = '["a", "b", "c"]'
item_var = "item"
template = ".worker"
`
	m, err := ParseModuleString(tomlStr, "test.toml")
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	wf := m.GetWorkflow("main")
	if wf == nil {
		t.Fatal("main workflow not found")
	}

	t.Logf("Workflow has %d steps", len(wf.Steps))
	for _, s := range wf.Steps {
		t.Logf("Step id=%q executor=%q items=%q item_var=%q template=%q",
			s.ID, s.Executor, s.Items, s.ItemVar, s.Template)
	}

	b := NewBaker("test-wf")
	result, err := b.BakeWorkflow(wf, nil)
	if err != nil {
		t.Fatalf("bake error: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	t.Logf("Baked step: id=%q executor=%q", step.ID, step.Executor)
	if step.Foreach == nil {
		t.Fatal("foreach config is nil")
	}
	t.Logf("Foreach: items=%q item_var=%q template=%q",
		step.Foreach.Items, step.Foreach.ItemVar, step.Foreach.Template)

	if step.Foreach.Items != `["a", "b", "c"]` {
		t.Errorf("expected items '[\"a\", \"b\", \"c\"]', got %q", step.Foreach.Items)
	}
	if step.Foreach.ItemVar != "item" {
		t.Errorf("expected item_var 'item', got %q", step.Foreach.ItemVar)
	}
	if step.Foreach.Template != ".worker" {
		t.Errorf("expected template '.worker', got %q", step.Foreach.Template)
	}
}

// Test for meow-o22f: empty string variables should not be overridden by defaults
func TestBakeWorkflow_EmptyStringNotOverriddenByDefault(t *testing.T) {
	workflow := &Workflow{
		Name: "test-workflow",
		Variables: map[string]*Var{
			"with_default": {
				Default: "default-value",
			},
		},
		Steps: []*Step{
			{
				ID:       "task-1",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Variable is: {{with_default}}",
			},
		},
	}

	baker := NewBaker("run-001")

	// Explicitly set the variable to empty string
	vars := map[string]string{
		"with_default": "",
	}

	result, err := baker.BakeWorkflow(workflow, vars)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	// The variable should remain empty, not be overridden by the default
	if baker.VarContext.Get("with_default") != "" {
		t.Errorf("expected empty string, got %q - default should not override empty string", baker.VarContext.Get("with_default"))
	}

	// Verify the prompt was substituted with empty string
	step := result.Steps[0]
	if step.Agent.Prompt != "Variable is: " {
		t.Errorf("expected 'Variable is: ', got %q", step.Agent.Prompt)
	}
}

// Test for meow-o22f: required validation should fail for unset, but not for empty
func TestBakeWorkflow_RequiredValidationWithEmptyString(t *testing.T) {
	workflow := &Workflow{
		Name: "test-workflow",
		Variables: map[string]*Var{
			"required_var": {
				Required: true,
			},
		},
		Steps: []*Step{
			{
				ID:       "task-1",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Do something",
			},
		},
	}

	baker := NewBaker("run-001")

	// Test 1: Required variable set to empty string should be valid
	vars := map[string]string{
		"required_var": "",
	}

	_, err := baker.BakeWorkflow(workflow, vars)
	if err != nil {
		t.Errorf("BakeWorkflow should succeed when required var is set to empty string, got error: %v", err)
	}

	// Test 2: Required variable not provided should fail
	baker2 := NewBaker("run-002")
	_, err = baker2.BakeWorkflow(workflow, map[string]string{})
	if err == nil {
		t.Error("BakeWorkflow should fail when required var is not provided")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("expected error about required variable, got: %v", err)
	}
}
