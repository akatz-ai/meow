package workflow

import (
	"fmt"
	"strings"
	"testing"
)

func TestVarContext_BasicSubstitution(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("name", "world")
	ctx.SetVariable("count", 42)

	tests := []struct {
		input    string
		expected string
	}{
		{"Hello, {{name}}!", "Hello, world!"},
		{"Count: {{count}}", "Count: 42"},
		{"{{name}} and {{name}}", "world and world"},
		{"No vars here", "No vars here"},
		{"{{name}}{{count}}", "world42"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ctx.Substitute(tt.input)
			if err != nil {
				t.Fatalf("Substitute failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestVarContext_NestedAccess(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("config", map[string]any{
		"database": map[string]any{
			"host": "localhost",
			"port": 5432,
		},
	})

	tests := []struct {
		input    string
		expected string
	}{
		{"Host: {{config.database.host}}", "Host: localhost"},
		{"Port: {{config.database.port}}", "Port: 5432"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ctx.Substitute(tt.input)
			if err != nil {
				t.Fatalf("Substitute failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestVarContext_OutputReferences(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetOutput("create-worktree", "path", "/tmp/worktree")
	ctx.SetOutput("create-worktree", "branch", "feature-x")
	ctx.SetOutputs("run-build", map[string]any{
		"exit_code": 0,
		"stdout":    "Build successful",
	})

	tests := []struct {
		input    string
		expected string
	}{
		// step.outputs.field format
		{"Path: {{create-worktree.outputs.path}}", "Path: /tmp/worktree"},
		{"Branch: {{create-worktree.outputs.branch}}", "Branch: feature-x"},
		{"{{run-build.outputs.stdout}}", "Build successful"},
		{"Exit: {{run-build.outputs.exit_code}}", "Exit: 0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ctx.Substitute(tt.input)
			if err != nil {
				t.Fatalf("Substitute failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestVarContext_Builtins(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetBuiltin("agent", "claude-1")
	ctx.SetBuiltin("bead_id", "bd-123")

	tests := []struct {
		input    string
		expected string
	}{
		{"Agent: {{agent}}", "Agent: claude-1"},
		{"Bead: {{bead_id}}", "Bead: bd-123"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ctx.Substitute(tt.input)
			if err != nil {
				t.Fatalf("Substitute failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestVarContext_DefaultBuiltins(t *testing.T) {
	ctx := NewVarContext()

	// timestamp, date, time should be set by default
	result, err := ctx.Substitute("Date: {{date}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if !strings.HasPrefix(result, "Date: 20") {
		t.Errorf("expected date format, got %q", result)
	}
}

func TestVarContext_UndefinedVariable(t *testing.T) {
	ctx := NewVarContext()

	_, err := ctx.Substitute("Hello, {{undefined}}!")
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "undefined") {
		t.Errorf("expected error about undefined, got: %v", err)
	}
}

func TestVarContext_MissingOutput(t *testing.T) {
	ctx := NewVarContext()

	_, err := ctx.Substitute("{{missing-bead.outputs.field}}")
	if err == nil {
		t.Fatal("expected error for missing output")
	}
	if !strings.Contains(err.Error(), "no outputs") {
		t.Errorf("expected error about no outputs, got: %v", err)
	}
}

func TestVarContext_MissingOutputField(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetOutput("my-bead", "stdout", "hello")

	_, err := ctx.Substitute("{{my-bead.outputs.missing}}")
	if err == nil {
		t.Fatal("expected error for missing output field")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected error about field not found, got: %v", err)
	}
}

func TestVarContext_ApplyDefaults(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("override", "user-value")

	vars := map[string]Var{
		"override":   {Default: "default-value"},
		"missing":    {Default: "default-missing"},
		"no_default": {Required: true},
	}

	ctx.ApplyDefaults(vars)

	// User value should not be overwritten
	if ctx.Variables["override"] != "user-value" {
		t.Errorf("expected user value, got %v", ctx.Variables["override"])
	}

	// Missing should get default
	if ctx.Variables["missing"] != "default-missing" {
		t.Errorf("expected default value, got %v", ctx.Variables["missing"])
	}

	// No default should remain unset
	if _, ok := ctx.Variables["no_default"]; ok {
		t.Error("expected no_default to remain unset")
	}
}

func TestVarContext_ValidateRequired(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("provided", "value")

	vars := map[string]Var{
		"provided":    {Required: true},
		"missing":     {Required: true},
		"optional":    {Required: false},
		"has_default": {Required: true, Default: "default"},
	}

	err := ctx.ValidateRequired(vars)
	if err == nil {
		t.Fatal("expected error for missing required variable")
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Errorf("expected error to mention 'missing', got: %v", err)
	}
	// Should not complain about 'has_default' since it has a default
	if strings.Contains(err.Error(), "has_default") {
		t.Errorf("should not complain about has_default: %v", err)
	}
}

func TestVarContext_SubstituteMap(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("host", "localhost")
	ctx.SetVariable("port", "8080")

	m := map[string]string{
		"url":    "http://{{host}}:{{port}}",
		"static": "no-vars",
	}

	result, err := ctx.SubstituteMap(m)
	if err != nil {
		t.Fatalf("SubstituteMap failed: %v", err)
	}

	if result["url"] != "http://localhost:8080" {
		t.Errorf("expected url substitution, got %q", result["url"])
	}
	if result["static"] != "no-vars" {
		t.Errorf("expected static to be unchanged, got %q", result["static"])
	}
}

func TestVarContext_SubstituteStep(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("task_id", "bd-42")
	ctx.SetVariable("framework", "pytest")
	ctx.SetOutput("analyze", "selected", "feature-x")

	step := &Step{
		ID:        "test-step",
		Executor:  ExecutorBranch,
		Condition: "{{analyze.outputs.selected}} != ''",
		Prompt:    "Testing {{task_id}} with {{framework}}",
		Template:  "impl-{{task_id}}",
		Variables: map[string]any{
			"target": "{{task_id}}",
		},
	}

	result, err := ctx.SubstituteStep(step)
	if err != nil {
		t.Fatalf("SubstituteStep failed: %v", err)
	}

	if result.Prompt != "Testing bd-42 with pytest" {
		t.Errorf("expected prompt substitution, got %q", result.Prompt)
	}
	if result.Condition != "feature-x != ''" {
		t.Errorf("expected condition substitution, got %q", result.Condition)
	}
	if result.Template != "impl-bd-42" {
		t.Errorf("expected template substitution, got %q", result.Template)
	}
	if result.Variables["target"] != "bd-42" {
		t.Errorf("expected variables substitution, got %q", result.Variables["target"])
	}
}

func TestVarContext_SubstituteStep_Command(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("agent", "claude-1")
	ctx.SetVariable("worktree", "/tmp/wt-123")

	step := &Step{
		ID:       "setup",
		Executor: ExecutorShell,
		Command:  "echo {{agent}} > /tmp/agent.txt && cd {{worktree}}",
	}

	result, err := ctx.SubstituteStep(step)
	if err != nil {
		t.Fatalf("SubstituteStep failed: %v", err)
	}

	expected := "echo claude-1 > /tmp/agent.txt && cd /tmp/wt-123"
	if result.Command != expected {
		t.Errorf("expected command substitution %q, got %q", expected, result.Command)
	}
}

func TestVarContext_SubstituteStep_ErrorInCommand(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID:       "test-step",
		Executor: ExecutorShell,
		Command:  "echo {{undefined}}",
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "command") {
		t.Errorf("expected error about command, got: %v", err)
	}
}

func TestVarContext_SubstituteStep_ExpansionTargets(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("tmpl", "my-template")
	ctx.SetVariable("val", "42")

	step := &Step{
		ID: "test-step",
		OnTrue: &ExpansionTarget{
			Template: "{{tmpl}}",
			Variables: map[string]any{
				"x": "{{val}}",
			},
		},
		OnFalse: &ExpansionTarget{
			Template: "other-{{tmpl}}",
		},
	}

	result, err := ctx.SubstituteStep(step)
	if err != nil {
		t.Fatalf("SubstituteStep failed: %v", err)
	}

	if result.OnTrue.Template != "my-template" {
		t.Errorf("expected on_true.template substitution, got %q", result.OnTrue.Template)
	}
	if result.OnTrue.Variables["x"] != "42" {
		t.Errorf("expected on_true.variables substitution, got %q", result.OnTrue.Variables["x"])
	}
	if result.OnFalse.Template != "other-my-template" {
		t.Errorf("expected on_false.template substitution, got %q", result.OnFalse.Template)
	}
}

func TestVarContext_RecursiveSubstitution(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("a", "{{b}}")
	ctx.SetVariable("b", "final")

	result, err := ctx.Substitute("Start: {{a}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if result != "Start: final" {
		t.Errorf("expected recursive substitution, got %q", result)
	}
}

func TestVarContext_WhitespaceHandling(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("name", "world")

	tests := []struct {
		input    string
		expected string
	}{
		{"{{ name }}", "world"},
		{"{{  name  }}", "world"},
		{"{{	name	}}", "world"}, // tabs
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := ctx.Substitute(tt.input)
			if err != nil {
				t.Fatalf("Substitute failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestVarContext_StringMapVariable(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("env", map[string]string{
		"HOME": "/home/user",
		"PATH": "/usr/bin",
	})

	result, err := ctx.Substitute("Home: {{env.HOME}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if result != "Home: /home/user" {
		t.Errorf("expected substitution, got %q", result)
	}
}

func TestVarContext_Get(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("name", "world")
	ctx.SetVariable("count", 42)

	if ctx.Get("name") != "world" {
		t.Errorf("expected 'world', got %q", ctx.Get("name"))
	}
	if ctx.Get("count") != "42" {
		t.Errorf("expected '42', got %q", ctx.Get("count"))
	}
	// Missing variable returns empty string
	if ctx.Get("missing") != "" {
		t.Errorf("expected empty string for missing, got %q", ctx.Get("missing"))
	}
}

func TestVarContext_EmptyPath(t *testing.T) {
	ctx := NewVarContext()

	// Empty braces don't match the pattern, so they pass through unchanged
	result, err := ctx.Substitute("{{}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if result != "{{}}" {
		t.Errorf("expected unchanged '{{}}', got %q", result)
	}
}

func TestVarContext_NestedOutputAccess(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetOutputs("my-bead", map[string]any{
		"result": map[string]any{
			"nested": map[string]any{
				"value": "deep",
			},
		},
	})

	result, err := ctx.Substitute("{{my-bead.outputs.result.nested.value}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if result != "deep" {
		t.Errorf("expected 'deep', got %q", result)
	}
}

func TestVarContext_AccessFieldOnNonMap(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("scalar", "just-a-string")

	_, err := ctx.Substitute("{{scalar.field}}")
	if err == nil {
		t.Fatal("expected error for accessing field on scalar")
	}
	if !strings.Contains(err.Error(), "cannot access") {
		t.Errorf("expected 'cannot access' error, got: %v", err)
	}
}

func TestVarContext_SubstituteMap_Error(t *testing.T) {
	ctx := NewVarContext()

	m := map[string]string{
		"url": "http://{{undefined}}/api",
	}

	_, err := ctx.SubstituteMap(m)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "undefined") {
		t.Errorf("expected undefined error, got: %v", err)
	}
}

// Note: TestVarContext_SubstituteStep_Validation was removed
// because Step.Validation field was deleted when legacy template support was removed.

func TestVarContext_SubstituteStep_Timeout(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("duration", "10m")

	step := &Step{
		ID:      "test-step",
		Timeout: "{{duration}}",
	}

	result, err := ctx.SubstituteStep(step)
	if err != nil {
		t.Fatalf("SubstituteStep failed: %v", err)
	}

	if result.Timeout != "10m" {
		t.Errorf("expected timeout substitution, got %q", result.Timeout)
	}
}

func TestVarContext_SubstituteStep_OnTimeout(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("handler", "timeout-handler")
	ctx.SetVariable("reason", "timed out")

	step := &Step{
		ID: "test-step",
		OnTimeout: &ExpansionTarget{
			Template: "{{handler}}",
			Variables: map[string]any{
				"msg": "{{reason}}",
			},
		},
	}

	result, err := ctx.SubstituteStep(step)
	if err != nil {
		t.Fatalf("SubstituteStep failed: %v", err)
	}

	if result.OnTimeout == nil {
		t.Fatal("expected OnTimeout")
	}
	if result.OnTimeout.Template != "timeout-handler" {
		t.Errorf("expected timeout template substitution, got %q", result.OnTimeout.Template)
	}
	if result.OnTimeout.Variables["msg"] != "timed out" {
		t.Errorf("expected timeout variable substitution, got %q", result.OnTimeout.Variables["msg"])
	}
}

// Note: TestVarContext_SubstituteStep_ErrorInDescription and
// TestVarContext_SubstituteStep_ErrorInInstructions were removed because
// Step.Description and Step.Instructions fields were deleted when
// legacy template support was removed.

func TestVarContext_SubstituteStep_ErrorInCondition(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID:        "test-step",
		Condition: "{{undefined}}",
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "condition") {
		t.Errorf("expected error about condition, got: %v", err)
	}
}

func TestVarContext_SubstituteStep_ErrorInTemplate(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID:       "test-step",
		Template: "{{undefined}}",
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "template") {
		t.Errorf("expected error about template, got: %v", err)
	}
}

// Note: TestVarContext_SubstituteStep_ErrorInValidation was removed because
// Step.Validation field was deleted when legacy template support was removed.

func TestVarContext_SubstituteStep_ErrorInTimeout(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID:      "test-step",
		Timeout: "{{undefined}}",
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("expected error about timeout, got: %v", err)
	}
}

func TestVarContext_SubstituteStep_ErrorInVariables(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID: "test-step",
		Variables: map[string]any{
			"key": "{{undefined}}",
		},
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "variables") {
		t.Errorf("expected error about variables, got: %v", err)
	}
}

func TestVarContext_SubstituteStep_ErrorInOnTrue(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID: "test-step",
		OnTrue: &ExpansionTarget{
			Template: "{{undefined}}",
		},
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "on_true") {
		t.Errorf("expected error about on_true, got: %v", err)
	}
}

func TestVarContext_SubstituteStep_ErrorInOnFalse(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID: "test-step",
		OnFalse: &ExpansionTarget{
			Template: "{{undefined}}",
		},
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "on_false") {
		t.Errorf("expected error about on_false, got: %v", err)
	}
}

func TestVarContext_SubstituteStep_ErrorInOnTimeout(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID: "test-step",
		OnTimeout: &ExpansionTarget{
			Template: "{{undefined}}",
		},
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "on_timeout") {
		t.Errorf("expected error about on_timeout, got: %v", err)
	}
}

func TestVarContext_MaxDepthRecursion(t *testing.T) {
	ctx := NewVarContext()
	// Create a long chain that stays within depth
	ctx.SetVariable("a", "{{b}}")
	ctx.SetVariable("b", "{{c}}")
	ctx.SetVariable("c", "{{d}}")
	ctx.SetVariable("d", "{{e}}")
	ctx.SetVariable("e", "final")

	result, err := ctx.Substitute("{{a}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if result != "final" {
		t.Errorf("expected 'final', got %q", result)
	}
}

func TestVarContext_NonStringOutput(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetOutputs("my-bead", map[string]any{
		"count": 42,
		"valid": true,
	})

	result, err := ctx.Substitute("Count: {{my-bead.outputs.count}}, Valid: {{my-bead.outputs.valid}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if result != "Count: 42, Valid: true" {
		t.Errorf("expected formatted output, got %q", result)
	}
}

func TestVarContext_OutputAccessOnNonMap(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetOutput("my-bead", "scalar", "just-a-string")

	// Try to access a field on the scalar value
	_, err := ctx.Substitute("{{my-bead.outputs.scalar.field}}")
	if err == nil {
		t.Fatal("expected error for accessing field on non-map output")
	}
	if !strings.Contains(err.Error(), "cannot access") {
		t.Errorf("expected 'cannot access' error, got: %v", err)
	}
}

// Tests for BeadLookup functionality

func TestVarContext_BeadLookup_ResolvesOutputsFromStore(t *testing.T) {
	ctx := NewVarContext()

	// Set up a mock bead lookup function
	beadStore := map[string]*StepInfo{
		"create-worktree": {
			ID:     "create-worktree",
			Status: "done",
			Outputs: map[string]any{
				"path":   "/tmp/worktree",
				"branch": "feature-x",
			},
		},
	}

	ctx.SetStepLookup(func(stepID string) (*StepInfo, error) {
		if info, ok := beadStore[stepID]; ok {
			return info, nil
		}
		return nil, nil
	})

	// Test resolving output reference
	result, err := ctx.Substitute("Path: {{create-worktree.outputs.path}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if result != "Path: /tmp/worktree" {
		t.Errorf("expected 'Path: /tmp/worktree', got %q", result)
	}

	// Test using step.outputs.field format
	result, err = ctx.Substitute("Branch: {{create-worktree.outputs.branch}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if result != "Branch: feature-x" {
		t.Errorf("expected 'Branch: feature-x', got %q", result)
	}
}

func TestVarContext_BeadLookup_CachesOutputs(t *testing.T) {
	ctx := NewVarContext()

	lookupCount := 0
	ctx.SetStepLookup(func(stepID string) (*StepInfo, error) {
		lookupCount++
		return &StepInfo{
			ID:     stepID,
			Status: "done",
			Outputs: map[string]any{
				"value": "cached",
			},
		}, nil
	})

	// First lookup
	_, err := ctx.Substitute("{{my-bead.outputs.value}}")
	if err != nil {
		t.Fatalf("First substitute failed: %v", err)
	}

	// Second lookup should use cache
	_, err = ctx.Substitute("{{my-bead.outputs.value}}")
	if err != nil {
		t.Fatalf("Second substitute failed: %v", err)
	}

	// Should only have called lookup once
	if lookupCount != 1 {
		t.Errorf("expected 1 lookup call (cached), got %d", lookupCount)
	}
}

func TestVarContext_BeadLookup_BeadNotFound(t *testing.T) {
	ctx := NewVarContext()

	ctx.SetStepLookup(func(stepID string) (*StepInfo, error) {
		return nil, nil // Not found
	})

	_, err := ctx.Substitute("{{missing-bead.outputs.field}}")
	if err == nil {
		t.Fatal("expected error for missing bead")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestVarContext_BeadLookup_BeadNotClosed(t *testing.T) {
	ctx := NewVarContext()

	ctx.SetStepLookup(func(stepID string) (*StepInfo, error) {
		return &StepInfo{
			ID:     stepID,
			Status: "in_progress",
			Outputs: map[string]any{
				"value": "incomplete",
			},
		}, nil
	})

	_, err := ctx.Substitute("{{running-bead.outputs.value}}")
	if err == nil {
		t.Fatal("expected error for unclosed bead")
	}
	if !strings.Contains(err.Error(), "not done") {
		t.Errorf("expected 'not closed' error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "in_progress") {
		t.Errorf("expected error to include status, got: %v", err)
	}
}

func TestVarContext_BeadLookup_OpenBeadError(t *testing.T) {
	ctx := NewVarContext()

	ctx.SetStepLookup(func(stepID string) (*StepInfo, error) {
		return &StepInfo{
			ID:     stepID,
			Status: "open",
		}, nil
	})

	_, err := ctx.Substitute("{{open-bead.outputs.value}}")
	if err == nil {
		t.Fatal("expected error for open bead")
	}
	if !strings.Contains(err.Error(), "not done") {
		t.Errorf("expected 'not closed' error, got: %v", err)
	}
}

func TestVarContext_BeadLookup_ClosedButNoOutputs(t *testing.T) {
	ctx := NewVarContext()

	ctx.SetStepLookup(func(stepID string) (*StepInfo, error) {
		return &StepInfo{
			ID:      stepID,
			Status:  "done",
			Outputs: nil, // No outputs
		}, nil
	})

	_, err := ctx.Substitute("{{empty-bead.outputs.value}}")
	if err == nil {
		t.Fatal("expected error for bead with no outputs")
	}
	if !strings.Contains(err.Error(), "has no outputs") {
		t.Errorf("expected 'has no outputs' error, got: %v", err)
	}
}

func TestVarContext_BeadLookup_LookupError(t *testing.T) {
	ctx := NewVarContext()

	ctx.SetStepLookup(func(stepID string) (*StepInfo, error) {
		return nil, fmt.Errorf("database connection failed")
	})

	_, err := ctx.Substitute("{{some-bead.outputs.value}}")
	if err == nil {
		t.Fatal("expected error from lookup")
	}
	if !strings.Contains(err.Error(), "database connection failed") {
		t.Errorf("expected lookup error to be propagated, got: %v", err)
	}
}

func TestVarContext_BeadLookup_NestedOutputAccess(t *testing.T) {
	ctx := NewVarContext()

	ctx.SetStepLookup(func(stepID string) (*StepInfo, error) {
		return &StepInfo{
			ID:     stepID,
			Status: "done",
			Outputs: map[string]any{
				"result": map[string]any{
					"nested": map[string]any{
						"value": "deep",
					},
				},
			},
		}, nil
	})

	result, err := ctx.Substitute("{{my-bead.outputs.result.nested.value}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if result != "deep" {
		t.Errorf("expected 'deep', got %q", result)
	}
}

func TestVarContext_BeadLookup_PrefersCachedOutputs(t *testing.T) {
	ctx := NewVarContext()

	// Manually set outputs (cached)
	ctx.SetOutputs("cached-bead", map[string]any{
		"value": "from-cache",
	})

	// Set up lookup that would return different value
	ctx.SetStepLookup(func(stepID string) (*StepInfo, error) {
		return &StepInfo{
			ID:     stepID,
			Status: "done",
			Outputs: map[string]any{
				"value": "from-lookup",
			},
		}, nil
	})

	// Should use cached value, not lookup
	result, err := ctx.Substitute("{{cached-bead.outputs.value}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}
	if result != "from-cache" {
		t.Errorf("expected 'from-cache', got %q (should prefer cache over lookup)", result)
	}
}

func TestVarContext_BeadLookup_MissingOutputField(t *testing.T) {
	ctx := NewVarContext()

	ctx.SetStepLookup(func(stepID string) (*StepInfo, error) {
		return &StepInfo{
			ID:     stepID,
			Status: "done",
			Outputs: map[string]any{
				"existing": "value",
			},
		}, nil
	})

	_, err := ctx.Substitute("{{my-bead.outputs.nonexistent}}")
	if err == nil {
		t.Fatal("expected error for missing output field")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestVarContext_BeadLookup_WithoutLookupFuncFallsBackToError(t *testing.T) {
	ctx := NewVarContext()
	// No BeadLookup set

	_, err := ctx.Substitute("{{unknown-bead.outputs.value}}")
	if err == nil {
		t.Fatal("expected error when no lookup func and bead not cached")
	}
	if !strings.Contains(err.Error(), "no outputs") {
		t.Errorf("expected 'no outputs' error, got: %v", err)
	}
}

// Shell Escaping Tests (meow-306)

func TestShellEscape(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple string",
			input:    "hello world",
			expected: "'hello world'",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "''",
		},
		{
			name:     "single quote inside",
			input:    "it's a test",
			expected: "'it'\"'\"'s a test'",
		},
		{
			name:     "multiple single quotes",
			input:    "don't won't can't",
			expected: "'don'\"'\"'t won'\"'\"'t can'\"'\"'t'",
		},
		{
			name:     "command injection semicolon",
			input:    "value; rm -rf /",
			expected: "'value; rm -rf /'",
		},
		{
			name:     "command injection pipe",
			input:    "value | cat /etc/passwd",
			expected: "'value | cat /etc/passwd'",
		},
		{
			name:     "backticks",
			input:    "hello `whoami`",
			expected: "'hello `whoami`'",
		},
		{
			name:     "dollar subshell",
			input:    "hello $(whoami)",
			expected: "'hello $(whoami)'",
		},
		{
			name:     "dollar variable",
			input:    "path is $PATH",
			expected: "'path is $PATH'",
		},
		{
			name:     "double quotes",
			input:    `say "hello"`,
			expected: `'say "hello"'`,
		},
		{
			name:     "newlines",
			input:    "line1\nline2",
			expected: "'line1\nline2'",
		},
		{
			name:     "ampersand",
			input:    "cmd1 && cmd2",
			expected: "'cmd1 && cmd2'",
		},
		{
			name:     "redirect",
			input:    "echo foo > /tmp/file",
			expected: "'echo foo > /tmp/file'",
		},
		{
			name:     "complex injection",
			input:    "'; DROP TABLE users; --",
			expected: "''\"'\"'; DROP TABLE users; --'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ShellEscape(tt.input)
			if result != tt.expected {
				t.Errorf("ShellEscape(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestVarContext_SubstituteForShell(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("safe_name", "simple")
	ctx.SetVariable("dangerous_name", "value; rm -rf /")
	ctx.SetVariable("quote_name", "it's mine")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "safe value substitution",
			input:    "echo {{safe_name}}",
			expected: "echo 'simple'",
		},
		{
			name:     "dangerous value gets escaped",
			input:    "echo {{dangerous_name}}",
			expected: "echo 'value; rm -rf /'",
		},
		{
			name:     "quote in value gets escaped",
			input:    "echo {{quote_name}}",
			expected: "echo 'it'\"'\"'s mine'",
		},
		{
			name:     "multiple substitutions",
			input:    "echo {{safe_name}} {{dangerous_name}}",
			expected: "echo 'simple' 'value; rm -rf /'",
		},
		{
			name:     "no substitutions",
			input:    "echo hello",
			expected: "echo hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.SubstituteForShell(tt.input)
			if err != nil {
				t.Fatalf("SubstituteForShell failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("SubstituteForShell(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestVarContext_SubstituteForShell_Errors(t *testing.T) {
	ctx := NewVarContext()

	_, err := ctx.SubstituteForShell("echo {{undefined}}")
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "undefined") {
		t.Errorf("expected error about undefined, got: %v", err)
	}
}

func TestVarContext_SubstituteForShell_NestedValues(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetOutput("build-step", "path", "/tmp/build; rm -rf /")

	result, err := ctx.SubstituteForShell("cd {{build-step.outputs.path}}")
	if err != nil {
		t.Fatalf("SubstituteForShell failed: %v", err)
	}

	expected := "cd '/tmp/build; rm -rf /'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestVarContext_SubstituteForShell_NoRecursiveSubstitution(t *testing.T) {
	ctx := NewVarContext()
	// Set a variable whose value looks like a template reference
	ctx.SetVariable("user_input", "{{malicious}}")
	ctx.SetVariable("malicious", "SHOULD_NOT_APPEAR")

	result, err := ctx.SubstituteForShell("echo {{user_input}}")
	if err != nil {
		t.Fatalf("SubstituteForShell failed: %v", err)
	}

	// The {{malicious}} in the value should be treated as literal text,
	// not interpreted as another variable reference
	expected := "echo '{{malicious}}'"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// Tests for stringifyValue (meow-r8wp)

func TestStringifyValue_String(t *testing.T) {
	result := stringifyValue("hello")
	if result != "hello" {
		t.Errorf("expected 'hello', got %q", result)
	}
}

func TestStringifyValue_Integer(t *testing.T) {
	result := stringifyValue(42)
	if result != "42" {
		t.Errorf("expected '42', got %q", result)
	}
}

func TestStringifyValue_Boolean(t *testing.T) {
	result := stringifyValue(true)
	if result != "true" {
		t.Errorf("expected 'true', got %q", result)
	}
}

func TestStringifyValue_MapStringAny(t *testing.T) {
	input := map[string]any{
		"foo": "bar",
		"baz": 123,
	}
	result := stringifyValue(input)

	// Result should be valid JSON
	if !strings.Contains(result, `"foo"`) || !strings.Contains(result, `"bar"`) {
		t.Errorf("expected valid JSON with foo:bar, got %q", result)
	}
}

func TestStringifyValue_Slice(t *testing.T) {
	input := []any{"a", "b", "c"}
	result := stringifyValue(input)

	// Should be valid JSON array
	expected := `["a","b","c"]`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestStringifyValue_StringSlice(t *testing.T) {
	input := []string{"x", "y", "z"}
	result := stringifyValue(input)

	// Should be valid JSON array
	expected := `["x","y","z"]`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestStringifyValue_IntSlice(t *testing.T) {
	input := []int{1, 2, 3}
	result := stringifyValue(input)

	// Should be valid JSON array
	expected := `[1,2,3]`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestStringifyValue_Nil(t *testing.T) {
	result := stringifyValue(nil)

	// nil should become empty string
	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestStringifyValue_MapStringString(t *testing.T) {
	input := map[string]string{
		"key1": "value1",
		"key2": "value2",
	}
	result := stringifyValue(input)

	// Result should be valid JSON
	if !strings.Contains(result, `"key1"`) || !strings.Contains(result, `"value1"`) {
		t.Errorf("expected valid JSON, got %q", result)
	}
}

func TestStringifyValue_NestedStructure(t *testing.T) {
	input := map[string]any{
		"outer": map[string]any{
			"inner": []any{1, 2, 3},
		},
	}
	result := stringifyValue(input)

	// Should be valid JSON with nested structure
	if !strings.Contains(result, `"outer"`) || !strings.Contains(result, `"inner"`) {
		t.Errorf("expected nested JSON structure, got %q", result)
	}
}

func TestVarContext_Get_StructuredOutput(t *testing.T) {
	ctx := NewVarContext()

	// Test map output
	ctx.SetVariable("config", map[string]any{
		"host": "localhost",
		"port": 8080,
	})

	result := ctx.Get("config")

	// Should be valid JSON, not "map[host:localhost port:8080]"
	if !strings.HasPrefix(result, "{") {
		t.Errorf("expected JSON object, got %q", result)
	}
	if !strings.Contains(result, `"host"`) {
		t.Errorf("expected JSON with host field, got %q", result)
	}
}

func TestVarContext_Substitute_MapOutput(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("data", map[string]any{
		"name":  "test",
		"count": 42,
	})

	result, err := ctx.Substitute("Data: {{data}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}

	// Should contain valid JSON, not Go map literal
	if !strings.Contains(result, `"name"`) || !strings.Contains(result, `"test"`) {
		t.Errorf("expected JSON in result, got %q", result)
	}
	if strings.Contains(result, "map[") {
		t.Errorf("result contains Go map literal instead of JSON: %q", result)
	}
}

func TestVarContext_Substitute_SliceOutput(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("items", []any{"a", "b", "c"})

	result, err := ctx.Substitute("Items: {{items}}")
	if err != nil {
		t.Fatalf("Substitute failed: %v", err)
	}

	// Should contain valid JSON array
	expected := `Items: ["a","b","c"]`
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestVarContext_SubstituteForShell_MapOutput(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("config", map[string]any{
		"key": "value",
	})

	result, err := ctx.SubstituteForShell("echo {{config}}")
	if err != nil {
		t.Fatalf("SubstituteForShell failed: %v", err)
	}

	// Should be escaped JSON, not Go map literal
	if !strings.Contains(result, `"key"`) {
		t.Errorf("expected JSON in shell-escaped result, got %q", result)
	}
	if strings.Contains(result, "map[") {
		t.Errorf("result contains Go map literal instead of JSON: %q", result)
	}
}

// Tests for Has() method (meow-o22f)

func TestVarContext_Has(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("exists", "value")
	ctx.SetVariable("empty", "")

	tests := []struct {
		name     string
		varName  string
		expected bool
	}{
		{"variable exists with value", "exists", true},
		{"variable exists but empty", "empty", true},
		{"variable does not exist", "missing", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ctx.Has(tt.varName)
			if result != tt.expected {
				t.Errorf("Has(%q) = %v, want %v", tt.varName, result, tt.expected)
			}
		})
	}
}

func TestVarContext_Has_EmptyString(t *testing.T) {
	ctx := NewVarContext()

	// Explicitly set a variable to empty string
	ctx.SetVariable("empty", "")

	// Has() should return true (variable is set, even if empty)
	if !ctx.Has("empty") {
		t.Error("Has('empty') should return true for explicitly set empty string")
	}

	// Get() returns empty string
	if ctx.Get("empty") != "" {
		t.Errorf("Get('empty') should return empty string, got %q", ctx.Get("empty"))
	}
}

func TestVarContext_Has_Unset(t *testing.T) {
	ctx := NewVarContext()

	// Has() should return false for unset variable
	if ctx.Has("unset") {
		t.Error("Has('unset') should return false for unset variable")
	}

	// Get() also returns empty string for unset
	if ctx.Get("unset") != "" {
		t.Errorf("Get('unset') should return empty string, got %q", ctx.Get("unset"))
	}
}

// Tests for Eval/Render/EvalMap/EvalSlice methods (meow-n16t)

func TestVarContext_Eval_PureReference_ReturnsTypedValue(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("task", map[string]any{
		"name": "test-task",
		"id":   42,
	})

	result, err := ctx.Eval("{{task}}")
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	// Should return the actual map, not a JSON string
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["name"] != "test-task" {
		t.Errorf("expected name='test-task', got %v", m["name"])
	}
	if m["id"] != 42 {
		t.Errorf("expected id=42, got %v", m["id"])
	}
}

func TestVarContext_Eval_PureReference_WithWhitespace(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("items", []any{"a", "b", "c"})

	// Test with whitespace around the reference
	result, err := ctx.Eval("  {{ items }}  ")
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	// Should return the actual slice
	s, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(s) != 3 {
		t.Errorf("expected 3 items, got %d", len(s))
	}
}

func TestVarContext_Eval_MixedContent_ReturnsString(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("task", map[string]any{
		"name": "test",
	})

	result, err := ctx.Eval("prefix-{{task}}")
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	// Mixed content should return string
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if !strings.HasPrefix(s, "prefix-") {
		t.Errorf("expected prefix, got %q", s)
	}
}

func TestVarContext_Eval_MultipleReferences_ReturnsString(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("a", "hello")
	ctx.SetVariable("b", "world")

	result, err := ctx.Eval("{{a}} {{b}}")
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	// Multiple references should return string
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "hello world" {
		t.Errorf("expected 'hello world', got %q", s)
	}
}

func TestVarContext_Eval_NestedPath_ReturnsTypedValue(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("config", map[string]any{
		"database": map[string]any{
			"host": "localhost",
			"port": 5432,
		},
	})

	result, err := ctx.Eval("{{config.database}}")
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	// Should return the nested map
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["host"] != "localhost" {
		t.Errorf("expected host='localhost', got %v", m["host"])
	}
}

func TestVarContext_Eval_ScalarValue(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("count", 42)
	ctx.SetVariable("enabled", true)
	ctx.SetVariable("name", "test")

	tests := []struct {
		name     string
		input    string
		expected any
	}{
		{"integer", "{{count}}", 42},
		{"boolean", "{{enabled}}", true},
		{"string", "{{name}}", "test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.Eval(tt.input)
			if err != nil {
				t.Fatalf("Eval failed: %v", err)
			}
			if result != tt.expected {
				t.Errorf("expected %v (%T), got %v (%T)", tt.expected, tt.expected, result, result)
			}
		})
	}
}

func TestVarContext_Eval_UndefinedVariable(t *testing.T) {
	ctx := NewVarContext()

	_, err := ctx.Eval("{{undefined}}")
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "undefined") {
		t.Errorf("expected error about undefined, got: %v", err)
	}
}

func TestVarContext_Eval_StepOutput(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetOutputs("build-step", map[string]any{
		"result": map[string]any{
			"status": "success",
			"files":  []any{"a.go", "b.go"},
		},
	})

	result, err := ctx.Eval("{{build-step.outputs.result}}")
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	// Should return the nested map
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", result)
	}
	if m["status"] != "success" {
		t.Errorf("expected status='success', got %v", m["status"])
	}
}

func TestVarContext_Render_ReturnsString(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("name", "world")

	result, err := ctx.Render("Hello, {{name}}!")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	if result != "Hello, world!" {
		t.Errorf("expected 'Hello, world!', got %q", result)
	}
}

func TestVarContext_Render_MapBecomesJSON(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("config", map[string]any{
		"key": "value",
	})

	result, err := ctx.Render("Config: {{config}}")
	if err != nil {
		t.Fatalf("Render failed: %v", err)
	}

	// Map should be JSON-stringified
	if !strings.Contains(result, `"key"`) {
		t.Errorf("expected JSON in result, got %q", result)
	}
}

func TestVarContext_EvalMap_Basic(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("items", []any{"a", "b", "c"})
	ctx.SetVariable("name", "test")

	input := map[string]any{
		"typed":  "{{items}}",
		"string": "prefix-{{name}}",
		"static": 42,
	}

	result, err := ctx.EvalMap(input)
	if err != nil {
		t.Fatalf("EvalMap failed: %v", err)
	}

	// "typed" should be actual slice
	items, ok := result["typed"].([]any)
	if !ok {
		t.Fatalf("expected []any for 'typed', got %T", result["typed"])
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}

	// "string" should be string (mixed content)
	s, ok := result["string"].(string)
	if !ok {
		t.Fatalf("expected string for 'string', got %T", result["string"])
	}
	if s != "prefix-test" {
		t.Errorf("expected 'prefix-test', got %q", s)
	}

	// "static" should pass through unchanged
	if result["static"] != 42 {
		t.Errorf("expected 42 for 'static', got %v", result["static"])
	}
}

func TestVarContext_EvalMap_Nested(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("data", map[string]any{
		"inner": "value",
	})

	input := map[string]any{
		"outer": map[string]any{
			"ref": "{{data}}",
		},
	}

	result, err := ctx.EvalMap(input)
	if err != nil {
		t.Fatalf("EvalMap failed: %v", err)
	}

	outer, ok := result["outer"].(map[string]any)
	if !ok {
		t.Fatalf("expected map for 'outer', got %T", result["outer"])
	}

	// "ref" should be the actual map
	ref, ok := outer["ref"].(map[string]any)
	if !ok {
		t.Fatalf("expected map for 'ref', got %T", outer["ref"])
	}
	if ref["inner"] != "value" {
		t.Errorf("expected inner='value', got %v", ref["inner"])
	}
}

func TestVarContext_EvalMap_Error(t *testing.T) {
	ctx := NewVarContext()

	input := map[string]any{
		"bad": "{{undefined}}",
	}

	_, err := ctx.EvalMap(input)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("expected error to mention key 'bad', got: %v", err)
	}
}

func TestVarContext_EvalSlice_Basic(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("config", map[string]any{
		"host": "localhost",
	})
	ctx.SetVariable("name", "test")

	input := []any{
		"{{config}}",
		"prefix-{{name}}",
		42,
	}

	result, err := ctx.EvalSlice(input)
	if err != nil {
		t.Fatalf("EvalSlice failed: %v", err)
	}

	// First element should be actual map
	m, ok := result[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map for [0], got %T", result[0])
	}
	if m["host"] != "localhost" {
		t.Errorf("expected host='localhost', got %v", m["host"])
	}

	// Second element should be string (mixed content)
	s, ok := result[1].(string)
	if !ok {
		t.Fatalf("expected string for [1], got %T", result[1])
	}
	if s != "prefix-test" {
		t.Errorf("expected 'prefix-test', got %q", s)
	}

	// Third element should pass through unchanged
	if result[2] != 42 {
		t.Errorf("expected 42 for [2], got %v", result[2])
	}
}

func TestVarContext_EvalSlice_NestedMaps(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("items", []any{1, 2, 3})

	input := []any{
		map[string]any{
			"list": "{{items}}",
		},
	}

	result, err := ctx.EvalSlice(input)
	if err != nil {
		t.Fatalf("EvalSlice failed: %v", err)
	}

	m, ok := result[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map for [0], got %T", result[0])
	}

	// "list" should be actual slice
	items, ok := m["list"].([]any)
	if !ok {
		t.Fatalf("expected []any for 'list', got %T", m["list"])
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

func TestVarContext_EvalSlice_NestedSlice(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("val", "resolved")

	input := []any{
		[]any{
			"{{val}}",
			"static",
		},
	}

	result, err := ctx.EvalSlice(input)
	if err != nil {
		t.Fatalf("EvalSlice failed: %v", err)
	}

	inner, ok := result[0].([]any)
	if !ok {
		t.Fatalf("expected []any for [0], got %T", result[0])
	}
	if inner[0] != "resolved" {
		t.Errorf("expected 'resolved' for [0][0], got %v", inner[0])
	}
	if inner[1] != "static" {
		t.Errorf("expected 'static' for [0][1], got %v", inner[1])
	}
}

func TestVarContext_EvalSlice_Error(t *testing.T) {
	ctx := NewVarContext()

	input := []any{
		"{{undefined}}",
	}

	_, err := ctx.EvalSlice(input)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "index 0") {
		t.Errorf("expected error to mention index, got: %v", err)
	}
}

func TestVarContext_EvalMap_WithSlice(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("data", map[string]any{"key": "value"})

	input := map[string]any{
		"array": []any{
			"{{data}}",
		},
	}

	result, err := ctx.EvalMap(input)
	if err != nil {
		t.Fatalf("EvalMap failed: %v", err)
	}

	arr, ok := result["array"].([]any)
	if !ok {
		t.Fatalf("expected []any for 'array', got %T", result["array"])
	}

	m, ok := arr[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map for array[0], got %T", arr[0])
	}
	if m["key"] != "value" {
		t.Errorf("expected key='value', got %v", m["key"])
	}
}

func TestVarContext_Eval_DeferredVariable(t *testing.T) {
	ctx := NewVarContext()
	ctx.DeferUndefinedVariables = true

	// With deferred mode, undefined variables should be left as-is
	result, err := ctx.Eval("{{item}}")
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	// Should return the original expression as string
	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "{{item}}" {
		t.Errorf("expected '{{item}}', got %q", s)
	}
}

func TestVarContext_Eval_EmptyBraces(t *testing.T) {
	ctx := NewVarContext()

	// Empty braces should pass through unchanged (consistent with Substitute)
	result, err := ctx.Eval("{{}}")
	if err != nil {
		t.Fatalf("Eval failed: %v", err)
	}

	s, ok := result.(string)
	if !ok {
		t.Fatalf("expected string, got %T", result)
	}
	if s != "{{}}" {
		t.Errorf("expected '{{}}', got %q", s)
	}
}
