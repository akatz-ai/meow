package template

import (
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
		// bead.outputs.field format
		{"Path: {{create-worktree.outputs.path}}", "Path: /tmp/worktree"},
		{"Branch: {{create-worktree.outputs.branch}}", "Branch: feature-x"},
		// output.bead.field format
		{"{{output.run-build.stdout}}", "Build successful"},
		{"Exit: {{output.run-build.exit_code}}", "Exit: 0"},
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
		"override": {Default: "default-value"},
		"missing":  {Default: "default-missing"},
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
		"provided":  {Required: true},
		"missing":   {Required: true},
		"optional":  {Required: false},
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
		"url":     "http://{{host}}:{{port}}",
		"static":  "no-vars",
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
		ID:           "test-step",
		Description:  "Testing {{task_id}}",
		Instructions: "Use {{framework}} to test",
		Condition:    "{{output.analyze.selected}} != ''",
		Template:     "impl-{{task_id}}",
		Variables: map[string]string{
			"target": "{{task_id}}",
		},
	}

	result, err := ctx.SubstituteStep(step)
	if err != nil {
		t.Fatalf("SubstituteStep failed: %v", err)
	}

	if result.Description != "Testing bd-42" {
		t.Errorf("expected description substitution, got %q", result.Description)
	}
	if result.Instructions != "Use pytest to test" {
		t.Errorf("expected instructions substitution, got %q", result.Instructions)
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

func TestVarContext_SubstituteStep_ExpansionTargets(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("tmpl", "my-template")
	ctx.SetVariable("val", "42")

	step := &Step{
		ID: "test-step",
		OnTrue: &ExpansionTarget{
			Template: "{{tmpl}}",
			Variables: map[string]string{
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

func TestVarContext_SubstituteStep_Validation(t *testing.T) {
	ctx := NewVarContext()
	ctx.SetVariable("check", "test -f /tmp/ready")

	step := &Step{
		ID:         "test-step",
		Validation: "{{check}}",
	}

	result, err := ctx.SubstituteStep(step)
	if err != nil {
		t.Fatalf("SubstituteStep failed: %v", err)
	}

	if result.Validation != "test -f /tmp/ready" {
		t.Errorf("expected validation substitution, got %q", result.Validation)
	}
}

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
			Variables: map[string]string{
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

func TestVarContext_SubstituteStep_ErrorInDescription(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID:          "test-step",
		Description: "Working on {{undefined}}",
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("expected error about description, got: %v", err)
	}
}

func TestVarContext_SubstituteStep_ErrorInInstructions(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID:           "test-step",
		Instructions: "Do {{undefined}} work",
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "instructions") {
		t.Errorf("expected error about instructions, got: %v", err)
	}
}

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

func TestVarContext_SubstituteStep_ErrorInValidation(t *testing.T) {
	ctx := NewVarContext()

	step := &Step{
		ID:         "test-step",
		Validation: "{{undefined}}",
	}

	_, err := ctx.SubstituteStep(step)
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
	if !strings.Contains(err.Error(), "validation") {
		t.Errorf("expected error about validation, got: %v", err)
	}
}

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
		Variables: map[string]string{
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
