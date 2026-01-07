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
id = "restart"
type = "restart"
condition = "bd list --status=open | grep -q ."
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
	if tmpl.Steps[0].Type != "restart" {
		t.Errorf("expected step type 'restart', got %q", tmpl.Steps[0].Type)
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

func TestParseString_BlockingGate(t *testing.T) {
	toml := `
[meta]
name = "test-gate"
version = "1.0.0"
requires_human = true

[[steps]]
id = "await"
description = "Wait for approval"
type = "blocking-gate"
instructions = "Wait for human"
`

	tmpl, err := ParseString(toml)
	if err != nil {
		t.Fatalf("ParseString failed: %v", err)
	}

	if !tmpl.Meta.RequiresHuman {
		t.Error("expected requires_human to be true")
	}
	if tmpl.Steps[0].Type != "blocking-gate" {
		t.Errorf("expected type 'blocking-gate', got %q", tmpl.Steps[0].Type)
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
