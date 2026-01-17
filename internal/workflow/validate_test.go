package workflow

import (
	"strings"
	"testing"
)

func TestValidateFull_ValidTemplate(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{
			Name:    "test",
			Version: "1.0.0",
		},
		Variables: map[string]Var{
			"task_id": {Required: true},
		},
		Steps: []Step{
			{ID: "step-1", Executor: ExecutorShell, Command: "echo {{task_id}}"},
			{ID: "step-2", Executor: ExecutorShell, Command: "echo done", Needs: []string{"step-1"}},
		},
	}

	result := ValidateFull(tmpl)
	if result.HasErrors() {
		t.Errorf("expected no errors, got: %v", result.Error())
	}
}

func TestValidateFull_MissingName(t *testing.T) {
	tmpl := &Template{
		Steps: []Step{{ID: "step-1", Executor: ExecutorShell, Command: "echo test"}},
	}

	result := ValidateFull(tmpl)
	if !result.HasErrors() {
		t.Fatal("expected errors for missing name")
	}
	if !containsError(result, "name is required") {
		t.Errorf("expected name error, got: %v", result.Error())
	}
}

func TestValidateFull_InvalidVersion(t *testing.T) {
	tmpl := &Template{
		Meta:  Meta{Name: "test", Version: "bad-version"},
		Steps: []Step{{ID: "step-1", Executor: ExecutorShell, Command: "echo test"}},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "semver format") {
		t.Errorf("expected semver error, got: %v", result.Error())
	}
}

func TestValidateFull_NoSteps(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "at least one step") {
		t.Errorf("expected steps error, got: %v", result.Error())
	}
}

func TestValidateFull_DuplicateStepID(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "step-1", Executor: ExecutorShell, Command: "echo 1"},
			{ID: "step-1", Executor: ExecutorShell, Command: "echo 2"},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "duplicate step id") {
		t.Errorf("expected duplicate error, got: %v", result.Error())
	}
}

func TestValidateFull_UnknownDependency(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "step-1", Executor: ExecutorShell, Command: "echo test", Needs: []string{"nonexistent"}},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "unknown step") {
		t.Errorf("expected unknown step error, got: %v", result.Error())
	}
}

func TestValidateFull_DependencySuggestion(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "load-context", Executor: ExecutorShell, Command: "echo load"},
			{ID: "write-tests", Executor: ExecutorShell, Command: "echo write", Needs: []string{"load-contxt"}}, // typo
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "did you mean") {
		t.Errorf("expected suggestion, got: %v", result.Error())
	}
}

func TestValidateFull_CircularDependency(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "a", Executor: ExecutorShell, Command: "echo a", Needs: []string{"b"}},
			{ID: "b", Executor: ExecutorShell, Command: "echo b", Needs: []string{"c"}},
			{ID: "c", Executor: ExecutorShell, Command: "echo c", Needs: []string{"a"}},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "circular dependency") {
		t.Errorf("expected cycle error, got: %v", result.Error())
	}
}

func TestValidateFull_SelfReference(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "a", Executor: ExecutorShell, Command: "echo a", Needs: []string{"a"}},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "circular dependency") {
		t.Errorf("expected cycle error for self-reference, got: %v", result.Error())
	}
}

func TestValidateFull_UndefinedVariable(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "step-1", Executor: ExecutorShell, Command: "echo {{undefined_var}}"},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "undefined variable") {
		t.Errorf("expected undefined variable error, got: %v", result.Error())
	}
}

func TestValidateFull_DefinedVariable(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Variables: map[string]Var{
			"my_var": {Required: true},
		},
		Steps: []Step{
			{ID: "step-1", Executor: ExecutorShell, Command: "echo {{my_var}}"},
		},
	}

	result := ValidateFull(tmpl)
	if result.HasErrors() {
		t.Errorf("expected no errors for defined variable, got: %v", result.Error())
	}
}

func TestValidateFull_BuiltinVariables(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "step-1", Executor: ExecutorShell, Command: "echo Time: {{timestamp}}, Agent: {{agent}}"},
		},
	}

	result := ValidateFull(tmpl)
	if result.HasErrors() {
		t.Errorf("expected no errors for builtin variables, got: %v", result.Error())
	}
}

func TestValidateFull_OutputReferences(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "step-1", Executor: ExecutorShell, Command: "echo hello"},
			{ID: "step-2", Executor: ExecutorShell, Command: "echo {{output.step-1.result}} and {{step-1.outputs.other}}"},
		},
	}

	// Output references should not trigger undefined variable errors
	// (they're validated at runtime)
	result := ValidateFull(tmpl)
	if result.HasErrors() {
		t.Errorf("output references should not cause errors, got: %v", result.Error())
	}
}

func TestValidateFull_InvalidOnError(t *testing.T) {
	tmpl := &Template{
		Meta:  Meta{Name: "test", OnError: "invalid"},
		Steps: []Step{{ID: "step-1", Executor: ExecutorShell, Command: "echo test"}},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "invalid on_error") {
		t.Errorf("expected on_error error, got: %v", result.Error())
	}
}

func TestValidateFull_ValidOnError(t *testing.T) {
	for _, onErr := range []string{"continue", "abort", "retry", "inject-gate"} {
		tmpl := &Template{
			Meta:  Meta{Name: "test", OnError: onErr},
			Steps: []Step{{ID: "step-1", Executor: ExecutorShell, Command: "echo test"}},
		}

		result := ValidateFull(tmpl)
		if containsError(result, "on_error") {
			t.Errorf("expected no error for on_error=%q, got: %v", onErr, result.Error())
		}
	}
}

func TestValidateFull_ConditionWithoutBranches(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "check", Executor: ExecutorBranch, Condition: "test -f /tmp/flag"},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "on_true or on_false") {
		t.Errorf("expected branch warning, got: %v", result.Error())
	}
}

func TestValidateFull_ConditionWithBranches(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{
				ID:        "check",
				Executor:  ExecutorBranch,
				Condition: "test -f /tmp/flag",
				OnTrue:    &ExpansionTarget{Template: "do-something"},
			},
		},
	}

	result := ValidateFull(tmpl)
	if containsError(result, "on_true or on_false") {
		t.Errorf("expected no branch error, got: %v", result.Error())
	}
}

// Note: Gate tests removed - gates are now implemented via branch executor
// with condition = "meow await-approval <gate-id>" instead of type = "gate"

func TestValidateFull_MultipleErrors(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Version: "bad"}, // Missing name, bad version
		Steps: []Step{
			{ID: "step-1", Executor: ExecutorShell, Command: "echo 1", Needs: []string{"unknown"}}, // Unknown dep
			{ID: "step-1", Executor: ExecutorShell, Command: "echo 2"},                              // Duplicate
		},
	}

	result := ValidateFull(tmpl)
	if len(result.Errors) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %v", len(result.Errors), result.Error())
	}
}

func TestValidateFull_ExpansionTargetVariables(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Variables: map[string]Var{
			"my_var": {},
		},
		Steps: []Step{
			{
				ID:        "check",
				Executor:  ExecutorBranch,
				Condition: "test -f /tmp/flag",
				OnTrue: &ExpansionTarget{
					Template: "{{my_var}}",
					Variables: map[string]any{
						"x": "{{undefined}}",
					},
				},
			},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "undefined variable") {
		t.Errorf("expected undefined variable in expansion target, got: %v", result.Error())
	}
}

func TestValidationError_String(t *testing.T) {
	err := ValidationError{
		Template: "my-template",
		StepID:   "step-1",
		Field:    "needs",
		Message:  "references unknown step",
		Suggest:  "did you mean \"step-2\"?",
	}

	s := err.Error()
	if !strings.Contains(s, "my-template") {
		t.Errorf("expected template name in error: %s", s)
	}
	if !strings.Contains(s, "step-1") {
		t.Errorf("expected step ID in error: %s", s)
	}
	if !strings.Contains(s, "suggestion") {
		t.Errorf("expected suggestion in error: %s", s)
	}
}

func containsError(result *ValidationResult, substr string) bool {
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), substr) {
			return true
		}
	}
	return false
}

func TestValidateFull_InvalidMetaType(t *testing.T) {
	tmpl := &Template{
		Meta:  Meta{Name: "test", Type: "invalid-type"},
		Steps: []Step{{ID: "step-1", Executor: ExecutorShell, Command: "echo test"}},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "invalid type") {
		t.Errorf("expected type error, got: %v", result.Error())
	}
}

func TestValidateFull_ValidMetaType(t *testing.T) {
	for _, typ := range []string{"loop", "linear"} {
		tmpl := &Template{
			Meta:  Meta{Name: "test", Type: typ},
			Steps: []Step{{ID: "step-1", Executor: ExecutorShell, Command: "echo test"}},
		}

		result := ValidateFull(tmpl)
		if containsError(result, "type") {
			t.Errorf("expected no type error for %q, got: %v", typ, result.Error())
		}
	}
}

func TestValidateFull_OnTimeoutVariables(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Variables: map[string]Var{
			"handler": {},
		},
		Steps: []Step{
			{
				ID:        "check",
				Executor:  ExecutorBranch,
				Condition: "test -f /tmp/ready",
				OnTrue:    &ExpansionTarget{Template: "proceed"},
				OnTimeout: &ExpansionTarget{
					Template: "{{handler}}",
					Variables: map[string]any{
						"x": "{{undefined}}",
					},
				},
			},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "undefined") {
		t.Errorf("expected undefined variable error in on_timeout.variables, got: %v", result.Error())
	}
}

func TestValidationResult_NoErrors(t *testing.T) {
	result := &ValidationResult{}
	if result.HasErrors() {
		t.Error("expected no errors")
	}
	if result.Error() != "" {
		t.Errorf("expected empty error string, got %q", result.Error())
	}
}

func TestValidationResult_Add(t *testing.T) {
	result := &ValidationResult{}
	result.Add("tmpl", "step-1", "field", "error message", "suggestion")

	if !result.HasErrors() {
		t.Error("expected errors")
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}

	err := result.Errors[0]
	if err.Template != "tmpl" {
		t.Errorf("expected template 'tmpl', got %q", err.Template)
	}
	if err.StepID != "step-1" {
		t.Errorf("expected step 'step-1', got %q", err.StepID)
	}
	if err.Field != "field" {
		t.Errorf("expected field 'field', got %q", err.Field)
	}
	if err.Message != "error message" {
		t.Errorf("expected message, got %q", err.Message)
	}
	if err.Suggest != "suggestion" {
		t.Errorf("expected suggestion, got %q", err.Suggest)
	}
}

func TestValidationError_NoLocation(t *testing.T) {
	err := ValidationError{
		Message: "plain error",
	}
	s := err.Error()
	if s != "plain error" {
		t.Errorf("expected plain error, got %q", s)
	}
}

func TestValidateFull_VariableInOnTimeoutTemplate(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Variables: map[string]Var{
			"handler": {},
		},
		Steps: []Step{
			{
				ID:        "check",
				Executor:  ExecutorBranch,
				Condition: "test -f /tmp/ready",
				OnTrue:    &ExpansionTarget{Template: "proceed"},
				OnTimeout: &ExpansionTarget{
					Template: "{{handler}}",
				},
			},
		},
	}

	result := ValidateFull(tmpl)
	// handler is defined, so no error expected for that
	if containsError(result, "handler") {
		t.Errorf("expected no error for defined handler variable, got: %v", result.Error())
	}
}

// Note: Step.Type validation tests removed - the Type field was deleted
// when legacy template support was removed. Steps now use the Executor field.

func TestValidateFull_EmptyStepID(t *testing.T) {
	tmpl := &Template{
		Meta:  Meta{Name: "test"},
		Steps: []Step{{ID: "", Executor: ExecutorShell, Command: "echo test"}},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "id is required") {
		t.Errorf("expected ID required error, got: %v", result.Error())
	}
}

// ============================================================================
// Typed Variables Validation Tests
// ============================================================================

func TestValidateFull_TypedVariablesInStep(t *testing.T) {
	// Validation should pass when step.Variables contains non-string values
	// Only string values should be checked for variable references
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Variables: map[string]Var{
			"defined_var": {},
		},
		Steps: []Step{
			{
				ID:       "expand-step",
				Executor: ExecutorExpand,
				Template: ".worker",
				Variables: map[string]any{
					"config":       map[string]any{"debug": true, "level": 3},
					"string_value": "{{defined_var}}",
				},
			},
		},
	}

	result := ValidateFull(tmpl)
	if result.HasErrors() {
		t.Errorf("expected no errors for typed variables, got: %v", result.Error())
	}
}

func TestValidateFull_TypedVariablesUndefinedRef(t *testing.T) {
	// Validation should still catch undefined variable references in string values
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{
				ID:       "expand-step",
				Executor: ExecutorExpand,
				Template: ".worker",
				Variables: map[string]any{
					"config":       map[string]any{"debug": true},
					"string_value": "{{undefined_var}}",
				},
			},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "undefined variable") {
		t.Errorf("expected undefined variable error for string value, got: %v", result.Error())
	}
}

func TestValidateFull_TypedVariablesInExpansionTarget(t *testing.T) {
	// Validation should pass when ExpansionTarget.Variables contains non-string values
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Variables: map[string]Var{
			"my_var": {},
		},
		Steps: []Step{
			{
				ID:        "check",
				Executor:  ExecutorBranch,
				Condition: "test -f /tmp/flag",
				OnTrue: &ExpansionTarget{
					Template: "do-something",
					Variables: map[string]any{
						"nested": map[string]any{"key": "value"},
						"ref":    "{{my_var}}",
					},
				},
			},
		},
	}

	result := ValidateFull(tmpl)
	if result.HasErrors() {
		t.Errorf("expected no errors for typed expansion target variables, got: %v", result.Error())
	}
}
