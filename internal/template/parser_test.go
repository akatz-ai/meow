package template

import (
	"strings"
	"testing"
)

func TestParseString_ValidTemplate(t *testing.T) {
	toml := `
[meta]
name = "test-template"
version = "1.0.0"
description = "A test template"
author = "test"

[variables]
task_id = { required = true, description = "The task ID" }
skip_tests = { required = false, default = false, type = "bool" }

[[steps]]
id = "step-1"
description = "First step"
instructions = "Do something"

[[steps]]
id = "step-2"
description = "Second step"
needs = ["step-1"]
instructions = "Do something else"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	// Verify meta
	if tmpl.Meta.Name != "test-template" {
		t.Errorf("expected name 'test-template', got %q", tmpl.Meta.Name)
	}
	if tmpl.Meta.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", tmpl.Meta.Version)
	}

	// Verify variables
	if len(tmpl.Variables) != 2 {
		t.Errorf("expected 2 variables, got %d", len(tmpl.Variables))
	}
	if v, ok := tmpl.Variables["task_id"]; !ok || !v.Required {
		t.Errorf("expected task_id to be required")
	}
	if v, ok := tmpl.Variables["skip_tests"]; !ok || v.Type != VarTypeBool {
		t.Errorf("expected skip_tests to be bool type")
	}

	// Verify steps
	if len(tmpl.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(tmpl.Steps))
	}
	if tmpl.Steps[0].ID != "step-1" {
		t.Errorf("expected first step id 'step-1', got %q", tmpl.Steps[0].ID)
	}
	if len(tmpl.Steps[1].Needs) != 1 || tmpl.Steps[1].Needs[0] != "step-1" {
		t.Errorf("expected step-2 to need step-1")
	}
}

func TestParseString_MissingName(t *testing.T) {
	toml := `
[meta]
version = "1.0.0"

[[steps]]
id = "step-1"
description = "First step"
`

	_, err := ParseString(toml)
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "meta.name is required") {
		t.Errorf("expected error about missing name, got: %v", err)
	}
}

func TestParseString_NoSteps(t *testing.T) {
	toml := `
[meta]
name = "test"
version = "1.0.0"
`

	_, err := ParseString(toml)
	if err == nil {
		t.Fatal("expected error for no steps")
	}
	if !strings.Contains(err.Error(), "at least one step") {
		t.Errorf("expected error about no steps, got: %v", err)
	}
}

func TestParseString_DuplicateStepID(t *testing.T) {
	toml := `
[meta]
name = "test"
version = "1.0.0"

[[steps]]
id = "step-1"
description = "First"

[[steps]]
id = "step-1"
description = "Duplicate"
`

	_, err := ParseString(toml)
	if err == nil {
		t.Fatal("expected error for duplicate step ID")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Errorf("expected error about duplicate id, got: %v", err)
	}
}

func TestParseString_UnknownDependency(t *testing.T) {
	toml := `
[meta]
name = "test"
version = "1.0.0"

[[steps]]
id = "step-1"
description = "First"
needs = ["nonexistent"]
`

	_, err := ParseString(toml)
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
	if !strings.Contains(err.Error(), "unknown step") {
		t.Errorf("expected error about unknown step, got: %v", err)
	}
}

func TestParseString_InvalidVariableType(t *testing.T) {
	toml := `
[meta]
name = "test"
version = "1.0.0"

[variables]
foo = { required = true, type = "invalid" }

[[steps]]
id = "step-1"
description = "First"
`

	_, err := ParseString(toml)
	if err == nil {
		t.Fatal("expected error for invalid variable type")
	}
	if !strings.Contains(err.Error(), "invalid type") {
		t.Errorf("expected error about invalid type, got: %v", err)
	}
}

func TestParseString_InvalidTOML(t *testing.T) {
	toml := `
[meta
name = "broken"
`

	_, err := ParseString(toml)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestParseString_ConditionBranches(t *testing.T) {
	toml := `
[meta]
name = "test-condition"
version = "1.0.0"

[[steps]]
id = "check"
description = "Check something"
condition = "test -f /tmp/flag"

[steps.on_true]
template = "do-something"
variables = { path = "/tmp/flag" }

[steps.on_false]
template = "do-other"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.Condition != "test -f /tmp/flag" {
		t.Errorf("expected condition, got %q", step.Condition)
	}
	if step.OnTrue == nil {
		t.Fatal("expected on_true")
	}
	if step.OnTrue.Template != "do-something" {
		t.Errorf("expected on_true template 'do-something', got %q", step.OnTrue.Template)
	}
	if step.OnFalse == nil {
		t.Fatal("expected on_false")
	}
	if step.OnFalse.Template != "do-other" {
		t.Errorf("expected on_false template 'do-other', got %q", step.OnFalse.Template)
	}
}

func TestParseString_StepWithTemplate(t *testing.T) {
	toml := `
[meta]
name = "test-expand"
version = "1.0.0"

[[steps]]
id = "run-impl"
description = "Run implementation"
template = "implement"
variables = { task_id = "{{selected_task}}" }
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.Template != "implement" {
		t.Errorf("expected template 'implement', got %q", step.Template)
	}
	if step.Variables["task_id"] != "{{selected_task}}" {
		t.Errorf("expected variable substitution, got %q", step.Variables["task_id"])
	}
}

func TestParseString_LoopTemplate(t *testing.T) {
	toml := `
[meta]
name = "outer-loop"
version = "1.0.0"
type = "loop"
max_iterations = 100
on_error = "inject-gate"

[[steps]]
id = "check-continue"
type = "condition"
condition = "bd list --status=open | grep -q ."

[steps.on_true]
template = "outer-loop"

[steps.on_false]
inline = []
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if tmpl.Meta.Type != "loop" {
		t.Errorf("expected type 'loop', got %q", tmpl.Meta.Type)
	}
	if tmpl.Meta.MaxIterations != 100 {
		t.Errorf("expected max_iterations 100, got %d", tmpl.Meta.MaxIterations)
	}
	if tmpl.Steps[0].Type != "condition" {
		t.Errorf("expected step type 'condition', got %q", tmpl.Steps[0].Type)
	}
}

func TestGetStep(t *testing.T) {
	toml := `
[meta]
name = "test"
version = "1.0.0"

[[steps]]
id = "step-1"
description = "First"

[[steps]]
id = "step-2"
description = "Second"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.GetStep("step-2")
	if step == nil {
		t.Fatal("expected to find step-2")
	}
	if step.Description != "Second" {
		t.Errorf("expected description 'Second', got %q", step.Description)
	}

	missing := tmpl.GetStep("nonexistent")
	if missing != nil {
		t.Error("expected nil for nonexistent step")
	}
}

func TestGetRequiredVariables(t *testing.T) {
	toml := `
[meta]
name = "test"
version = "1.0.0"

[variables]
required_var = { required = true, description = "Required" }
optional_var = { required = false, default = "foo" }
required_with_default = { required = true, default = "bar" }

[[steps]]
id = "step-1"
description = "First"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	required := tmpl.GetRequiredVariables()
	if len(required) != 1 {
		t.Errorf("expected 1 required variable, got %d", len(required))
	}
	if _, ok := required["required_var"]; !ok {
		t.Error("expected required_var in required variables")
	}
}

func TestStepOrder(t *testing.T) {
	toml := `
[meta]
name = "test"
version = "1.0.0"

[[steps]]
id = "c"
description = "Third"
needs = ["a", "b"]

[[steps]]
id = "a"
description = "First"

[[steps]]
id = "b"
description = "Second"
needs = ["a"]
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	order, err := tmpl.StepOrder()
	if err != nil {
		t.Fatalf("StepOrder failed: %v", err)
	}

	// a must come first
	aIdx := indexOf(order, "a")
	bIdx := indexOf(order, "b")
	cIdx := indexOf(order, "c")

	if aIdx == -1 || bIdx == -1 || cIdx == -1 {
		t.Fatalf("missing step in order: %v", order)
	}
	if aIdx > bIdx {
		t.Errorf("a should come before b: %v", order)
	}
	if aIdx > cIdx {
		t.Errorf("a should come before c: %v", order)
	}
	if bIdx > cIdx {
		t.Errorf("b should come before c: %v", order)
	}
}

func TestStepOrder_Cycle(t *testing.T) {
	toml := `
[meta]
name = "test"
version = "1.0.0"

[[steps]]
id = "a"
description = "First"
needs = ["b"]

[[steps]]
id = "b"
description = "Second"
needs = ["a"]
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	_, err = tmpl.StepOrder()
	if err == nil {
		t.Fatal("expected error for cycle")
	}
	if !strings.Contains(err.Error(), "cycle") {
		t.Errorf("expected cycle error, got: %v", err)
	}
}

func indexOf(slice []string, item string) int {
	for i, s := range slice {
		if s == item {
			return i
		}
	}
	return -1
}

func TestParseString_GateStep(t *testing.T) {
	toml := `
[meta]
name = "test-gate"
version = "1.0.0"
requires_human = true

[[steps]]
id = "await"
description = "Wait for approval"
type = "gate"
instructions = "Wait for human"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if !tmpl.Meta.RequiresHuman {
		t.Error("expected requires_human to be true")
	}
	if tmpl.Steps[0].Type != "gate" {
		t.Errorf("expected type 'gate', got %q", tmpl.Steps[0].Type)
	}
}

func TestParseString_EphemeralStep(t *testing.T) {
	toml := `
[meta]
name = "test-ephemeral"
version = "1.0.0"

[[steps]]
id = "temp"
description = "Temporary step"
ephemeral = true
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if !tmpl.Steps[0].Ephemeral {
		t.Error("expected ephemeral to be true")
	}
}

func TestParseString_VariableEnum(t *testing.T) {
	toml := `
[meta]
name = "test-enum"
version = "1.0.0"

[variables]
strategy = { required = false, default = "bv-triage", enum = ["bv-triage", "priority", "fifo"] }

[[steps]]
id = "step-1"
description = "First"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	v := tmpl.Variables["strategy"]
	if len(v.Enum) != 3 {
		t.Errorf("expected 3 enum values, got %d", len(v.Enum))
	}
	if v.Enum[0] != "bv-triage" {
		t.Errorf("expected first enum 'bv-triage', got %q", v.Enum[0])
	}
}

func TestParseString_StepWithID_Empty(t *testing.T) {
	toml := `
[meta]
name = "test"
version = "1.0.0"

[[steps]]
description = "Missing ID"
`

	_, err := ParseString(toml)
	if err == nil {
		t.Fatal("expected error for missing step ID")
	}
	if !strings.Contains(err.Error(), "id is required") {
		t.Errorf("expected ID required error, got: %v", err)
	}
}

func TestParseString_StepWithCode(t *testing.T) {
	toml := `
[meta]
name = "test-code"
version = "1.0.0"

[[steps]]
id = "run-script"
description = "Run a script"
code = "echo 'hello world'"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.Code != "echo 'hello world'" {
		t.Errorf("expected code, got %q", step.Code)
	}
}

func TestParseString_StepWithAction(t *testing.T) {
	toml := `
[meta]
name = "test-action"
version = "1.0.0"

[[steps]]
id = "notify"
description = "Notify user"
action = "send-slack"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.Action != "send-slack" {
		t.Errorf("expected action 'send-slack', got %q", step.Action)
	}
}

func TestParseString_StepWithValidation(t *testing.T) {
	toml := `
[meta]
name = "test-validation"
version = "1.0.0"

[[steps]]
id = "build"
description = "Build project"
validation = "test -f ./build/output"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.Validation != "test -f ./build/output" {
		t.Errorf("expected validation, got %q", step.Validation)
	}
}

func TestParseString_MetaFields(t *testing.T) {
	toml := `
[meta]
name = "full-meta"
version = "1.0.0"
description = "A complete template"
author = "test-author"
type = "loop"
fits_in_context = true
requires_human = true
estimated_minutes = 30
max_iterations = 50
on_error = "inject-gate"
error_gate_template = "error-handler"
max_retries = 3

[[steps]]
id = "step-1"
description = "First"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if tmpl.Meta.Description != "A complete template" {
		t.Errorf("expected description, got %q", tmpl.Meta.Description)
	}
	if tmpl.Meta.Author != "test-author" {
		t.Errorf("expected author 'test-author', got %q", tmpl.Meta.Author)
	}
	if !tmpl.Meta.FitsInContext {
		t.Error("expected fits_in_context to be true")
	}
	if tmpl.Meta.EstimatedMinutes != 30 {
		t.Errorf("expected estimated_minutes 30, got %d", tmpl.Meta.EstimatedMinutes)
	}
	if tmpl.Meta.OnError != "inject-gate" {
		t.Errorf("expected on_error 'inject-gate', got %q", tmpl.Meta.OnError)
	}
	if tmpl.Meta.ErrorGateTemplate != "error-handler" {
		t.Errorf("expected error_gate_template 'error-handler', got %q", tmpl.Meta.ErrorGateTemplate)
	}
	if tmpl.Meta.MaxRetries != 3 {
		t.Errorf("expected max_retries 3, got %d", tmpl.Meta.MaxRetries)
	}
}

func TestParseString_StepWithTimeout(t *testing.T) {
	toml := `
[meta]
name = "test-timeout"
version = "1.0.0"

[[steps]]
id = "wait"
description = "Wait for condition"
condition = "test -f /tmp/ready"
timeout = "10m"

[steps.on_true]
template = "proceed"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.Timeout != "10m" {
		t.Errorf("expected timeout '10m', got %q", step.Timeout)
	}
}

func TestParseString_StepWithAssignee(t *testing.T) {
	toml := `
[meta]
name = "test-assignee"
version = "1.0.0"

[[steps]]
id = "work"
description = "Do work"
assignee = "claude-1"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.Assignee != "claude-1" {
		t.Errorf("expected assignee 'claude-1', got %q", step.Assignee)
	}
}

func TestParseString_StepWithOnTimeout(t *testing.T) {
	toml := `
[meta]
name = "test-on-timeout"
version = "1.0.0"

[[steps]]
id = "check"
description = "Check with timeout handler"
condition = "test -f /tmp/ready"
timeout = "5m"

[steps.on_true]
template = "proceed"

[steps.on_false]
template = "wait"

[steps.on_timeout]
template = "handle-timeout"
variables = { reason = "timed out" }
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.OnTimeout == nil {
		t.Fatal("expected on_timeout")
	}
	if step.OnTimeout.Template != "handle-timeout" {
		t.Errorf("expected on_timeout template, got %q", step.OnTimeout.Template)
	}
	if step.OnTimeout.Variables["reason"] != "timed out" {
		t.Errorf("expected on_timeout variable, got %v", step.OnTimeout.Variables)
	}
}

func TestParseString_InlineStepsInCondition(t *testing.T) {
	toml := `
[meta]
name = "test-inline"
version = "1.0.0"

[[steps]]
id = "check"
description = "Check and do inline work"
condition = "test -f /tmp/flag"

[steps.on_true]
inline = [
	{ id = "action-1", type = "task", description = "First action" },
	{ id = "action-2", type = "task", description = "Second action", needs = ["action-1"] }
]
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.OnTrue == nil {
		t.Fatal("expected on_true")
	}
	if len(step.OnTrue.Inline) != 2 {
		t.Errorf("expected 2 inline steps, got %d", len(step.OnTrue.Inline))
	}
	if step.OnTrue.Inline[0].ID != "action-1" {
		t.Errorf("expected first inline id 'action-1', got %q", step.OnTrue.Inline[0].ID)
	}
	if len(step.OnTrue.Inline[1].Needs) != 1 || step.OnTrue.Inline[1].Needs[0] != "action-1" {
		t.Errorf("expected second inline to need action-1")
	}
}

func TestParseString_IntVariable(t *testing.T) {
	toml := `
[meta]
name = "test-int-var"
version = "1.0.0"

[variables]
max_count = { required = false, default = 100, type = "int" }

[[steps]]
id = "step-1"
description = "First"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	v := tmpl.Variables["max_count"]
	if v.Type != VarTypeInt {
		t.Errorf("expected int type, got %q", v.Type)
	}
}

func TestParseString_VariableWithDescription(t *testing.T) {
	toml := `
[meta]
name = "test-var-desc"
version = "1.0.0"

[variables]
task_id = { required = true, description = "The ID of the task to work on" }

[[steps]]
id = "step-1"
description = "First"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	v := tmpl.Variables["task_id"]
	if v.Description != "The ID of the task to work on" {
		t.Errorf("expected description, got %q", v.Description)
	}
}

// ============================================================================
// Executor Type Tests
// ============================================================================

func TestExecutorType_Valid(t *testing.T) {
	tests := []struct {
		executor ExecutorType
		valid    bool
	}{
		{ExecutorShell, true},
		{ExecutorSpawn, true},
		{ExecutorKill, true},
		{ExecutorExpand, true},
		{ExecutorBranch, true},
		{ExecutorAgent, true},
		{"", true},           // Empty allowed during migration
		{"invalid", false},
		{"task", false},      // Old type, not valid executor
	}

	for _, tc := range tests {
		t.Run(string(tc.executor), func(t *testing.T) {
			if got := tc.executor.Valid(); got != tc.valid {
				t.Errorf("ExecutorType(%q).Valid() = %v, want %v", tc.executor, got, tc.valid)
			}
		})
	}
}

func TestExecutorType_IsOrchestrator(t *testing.T) {
	orchestratorExecutors := []ExecutorType{
		ExecutorShell, ExecutorSpawn, ExecutorKill, ExecutorExpand, ExecutorBranch,
	}
	externalExecutors := []ExecutorType{
		ExecutorAgent, "",
	}

	for _, e := range orchestratorExecutors {
		if !e.IsOrchestrator() {
			t.Errorf("expected %q to be orchestrator executor", e)
		}
	}
	for _, e := range externalExecutors {
		if e.IsOrchestrator() {
			t.Errorf("expected %q to NOT be orchestrator executor", e)
		}
	}
}

func TestStep_Validate_Shell(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr string
	}{
		{
			name: "valid shell step",
			step: Step{
				ID:       "run-tests",
				Executor: ExecutorShell,
				Command:  "npm test",
			},
			wantErr: "",
		},
		{
			name: "shell without command",
			step: Step{
				ID:       "missing-cmd",
				Executor: ExecutorShell,
			},
			wantErr: "shell executor requires command",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

func TestStep_Validate_Spawn(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr string
	}{
		{
			name: "valid spawn step",
			step: Step{
				ID:       "start-worker",
				Executor: ExecutorSpawn,
				Agent:    "worker-1",
			},
			wantErr: "",
		},
		{
			name: "spawn without agent",
			step: Step{
				ID:       "missing-agent",
				Executor: ExecutorSpawn,
			},
			wantErr: "spawn executor requires agent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

func TestStep_Validate_Kill(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr string
	}{
		{
			name: "valid kill step",
			step: Step{
				ID:       "stop-worker",
				Executor: ExecutorKill,
				Agent:    "worker-1",
			},
			wantErr: "",
		},
		{
			name: "kill without agent",
			step: Step{
				ID:       "missing-agent",
				Executor: ExecutorKill,
			},
			wantErr: "kill executor requires agent",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

func TestStep_Validate_Expand(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr string
	}{
		{
			name: "valid expand step",
			step: Step{
				ID:       "do-impl",
				Executor: ExecutorExpand,
				Template: ".tdd",
			},
			wantErr: "",
		},
		{
			name: "expand without template",
			step: Step{
				ID:       "missing-template",
				Executor: ExecutorExpand,
			},
			wantErr: "expand executor requires template",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

func TestStep_Validate_Branch(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr string
	}{
		{
			name: "valid branch step",
			step: Step{
				ID:        "check-tests",
				Executor:  ExecutorBranch,
				Condition: "npm test",
			},
			wantErr: "",
		},
		{
			name: "branch without condition",
			step: Step{
				ID:       "missing-condition",
				Executor: ExecutorBranch,
			},
			wantErr: "branch executor requires condition",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

func TestStep_Validate_Agent(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr string
	}{
		{
			name: "valid agent step",
			step: Step{
				ID:       "implement",
				Executor: ExecutorAgent,
				Agent:    "worker-1",
				Prompt:   "Implement the feature",
			},
			wantErr: "",
		},
		{
			name: "agent without agent field",
			step: Step{
				ID:       "missing-agent",
				Executor: ExecutorAgent,
				Prompt:   "Do something",
			},
			wantErr: "agent executor requires agent",
		},
		{
			name: "agent without prompt",
			step: Step{
				ID:       "missing-prompt",
				Executor: ExecutorAgent,
				Agent:    "worker-1",
			},
			wantErr: "agent executor requires prompt",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

// Note: Gate is NOT an executor. Human approval is implemented via
// branch executor with condition = "meow await-approval <gate-id>"

func TestStep_Validate_Mode(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr string
	}{
		{
			name: "autonomous mode",
			step: Step{
				ID:       "test",
				Executor: ExecutorAgent,
				Agent:    "worker",
				Prompt:   "Do work",
				Mode:     "autonomous",
			},
			wantErr: "",
		},
		{
			name: "interactive mode",
			step: Step{
				ID:       "test",
				Executor: ExecutorAgent,
				Agent:    "worker",
				Prompt:   "Do work",
				Mode:     "interactive",
			},
			wantErr: "",
		},
		{
			name: "invalid mode",
			step: Step{
				ID:       "test",
				Executor: ExecutorAgent,
				Agent:    "worker",
				Prompt:   "Do work",
				Mode:     "invalid",
			},
			wantErr: "invalid mode",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

func TestStep_Validate_OnError(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr string
	}{
		{
			name: "continue on_error",
			step: Step{
				ID:       "test",
				Executor: ExecutorShell,
				Command:  "npm test",
				OnError:  "continue",
			},
			wantErr: "",
		},
		{
			name: "fail on_error",
			step: Step{
				ID:       "test",
				Executor: ExecutorShell,
				Command:  "npm test",
				OnError:  "fail",
			},
			wantErr: "",
		},
		{
			name: "invalid on_error",
			step: Step{
				ID:       "test",
				Executor: ExecutorShell,
				Command:  "npm test",
				OnError:  "invalid",
			},
			wantErr: "invalid on_error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

func TestStep_Validate_EmptyExecutor(t *testing.T) {
	// Empty executor is allowed for backwards compatibility
	step := Step{
		ID:           "legacy-step",
		Type:         "task", // Using legacy type field
		Instructions: "Do something",
	}
	err := step.Validate()
	if err != nil {
		t.Errorf("expected empty executor to be valid (backwards compatibility), got: %v", err)
	}
}

func TestStep_Validate_MissingID(t *testing.T) {
	step := Step{
		Executor: ExecutorShell,
		Command:  "npm test",
	}
	err := step.Validate()
	if err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Errorf("expected error about missing id, got: %v", err)
	}
}

func TestStep_Validate_InvalidExecutor(t *testing.T) {
	step := Step{
		ID:       "test",
		Executor: "notreal",
	}
	err := step.Validate()
	if err == nil || !strings.Contains(err.Error(), "invalid executor") {
		t.Errorf("expected error about invalid executor, got: %v", err)
	}
}

// ============================================================================
// Foreach Executor Tests
// ============================================================================

func TestExecutorType_Foreach(t *testing.T) {
	if !ExecutorForeach.Valid() {
		t.Error("foreach should be a valid executor")
	}
	if !ExecutorForeach.IsOrchestrator() {
		t.Error("foreach should be an orchestrator executor")
	}
}

func TestParseString_ForeachStep(t *testing.T) {
	toml := `
[meta]
name = "test-foreach"
version = "1.0.0"

[[steps]]
id = "parallel-workers"
executor = "foreach"
items = '["task1", "task2", "task3"]'
item_var = "task"
index_var = "i"
template = ".worker-task"
parallel = true
max_concurrent = 5
join = true

[steps.variables]
agent_id = "worker-{{i}}"
task_description = "{{task}}"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.Executor != ExecutorForeach {
		t.Errorf("expected executor 'foreach', got %q", step.Executor)
	}
	if step.Items != `["task1", "task2", "task3"]` {
		t.Errorf("expected items, got %q", step.Items)
	}
	if step.ItemVar != "task" {
		t.Errorf("expected item_var 'task', got %q", step.ItemVar)
	}
	if step.IndexVar != "i" {
		t.Errorf("expected index_var 'i', got %q", step.IndexVar)
	}
	if step.Template != ".worker-task" {
		t.Errorf("expected template '.worker-task', got %q", step.Template)
	}
	if step.Parallel == nil || !*step.Parallel {
		t.Error("expected parallel to be true")
	}
	// MaxConcurrent is any type (can be int64 or string)
	switch v := step.MaxConcurrent.(type) {
	case int64:
		if v != 5 {
			t.Errorf("expected max_concurrent 5, got %d", v)
		}
	case string:
		if v != "5" {
			t.Errorf("expected max_concurrent '5', got %q", v)
		}
	default:
		t.Errorf("expected max_concurrent to be int64 or string, got %T", step.MaxConcurrent)
	}
	if step.Join == nil || !*step.Join {
		t.Error("expected join to be true")
	}
	if step.Variables["agent_id"] != "worker-{{i}}" {
		t.Errorf("expected agent_id variable, got %q", step.Variables["agent_id"])
	}
}

func TestParseString_ForeachMinimalStep(t *testing.T) {
	toml := `
[meta]
name = "test-foreach-minimal"
version = "1.0.0"

[[steps]]
id = "simple-foreach"
executor = "foreach"
items = "{{planner.outputs.tasks}}"
item_var = "task"
template = ".worker"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	step := tmpl.Steps[0]
	if step.Executor != ExecutorForeach {
		t.Errorf("expected executor 'foreach', got %q", step.Executor)
	}
	if step.Items != "{{planner.outputs.tasks}}" {
		t.Errorf("expected items with variable reference, got %q", step.Items)
	}
	if step.ItemVar != "task" {
		t.Errorf("expected item_var 'task', got %q", step.ItemVar)
	}
	if step.Template != ".worker" {
		t.Errorf("expected template '.worker', got %q", step.Template)
	}
}

func TestStep_Validate_Foreach(t *testing.T) {
	tests := []struct {
		name    string
		step    Step
		wantErr string
	}{
		{
			name: "valid foreach step",
			step: Step{
				ID:       "parallel-workers",
				Executor: ExecutorForeach,
				Items:    `["a", "b", "c"]`,
				ItemVar:  "item",
				Template: ".worker",
			},
			wantErr: "",
		},
		{
			name: "foreach without items",
			step: Step{
				ID:       "missing-items",
				Executor: ExecutorForeach,
				ItemVar:  "item",
				Template: ".worker",
			},
			wantErr: "foreach executor requires items",
		},
		{
			name: "foreach without item_var",
			step: Step{
				ID:       "missing-item-var",
				Executor: ExecutorForeach,
				Items:    `["a", "b"]`,
				Template: ".worker",
			},
			wantErr: "foreach executor requires item_var",
		},
		{
			name: "foreach without template",
			step: Step{
				ID:       "missing-template",
				Executor: ExecutorForeach,
				Items:    `["a", "b"]`,
				ItemVar:  "item",
			},
			wantErr: "foreach executor requires template",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.step.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tc.wantErr, err)
				}
			}
		})
	}
}

func TestTemplate_Validate_ForeachDependencies(t *testing.T) {
	// Foreach steps should allow pattern-based dependencies on their children
	toml := `
[meta]
name = "test-foreach-deps"
version = "1.0.0"

[[steps]]
id = "parallel-builds"
executor = "foreach"
items = '["a", "b", "c"]'
item_var = "item"
template = ".build"

[[steps]]
id = "run-integration-tests"
executor = "shell"
command = "npm run test:integration"
needs = ["parallel-builds.*.build"]
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if len(tmpl.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(tmpl.Steps))
	}
}
