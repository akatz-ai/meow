package template

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
			{ID: "step-1", Description: "First step with {{task_id}}"},
			{ID: "step-2", Description: "Second step", Needs: []string{"step-1"}},
		},
	}

	result := ValidateFull(tmpl)
	if result.HasErrors() {
		t.Errorf("expected no errors, got: %v", result.Error())
	}
}

func TestValidateFull_MissingName(t *testing.T) {
	tmpl := &Template{
		Steps: []Step{{ID: "step-1"}},
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
		Steps: []Step{{ID: "step-1"}},
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
			{ID: "step-1"},
			{ID: "step-1"},
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
			{ID: "step-1", Needs: []string{"nonexistent"}},
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
			{ID: "load-context"},
			{ID: "write-tests", Needs: []string{"load-contxt"}}, // typo
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
			{ID: "a", Needs: []string{"b"}},
			{ID: "b", Needs: []string{"c"}},
			{ID: "c", Needs: []string{"a"}},
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
			{ID: "a", Needs: []string{"a"}},
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
			{ID: "step-1", Description: "Using {{undefined_var}}"},
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
			{ID: "step-1", Description: "Using {{my_var}}"},
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
			{ID: "step-1", Description: "Time: {{timestamp}}, Agent: {{agent}}"},
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
			{ID: "step-1"},
			{ID: "step-2", Description: "Using {{output.step-1.result}} and {{step-1.outputs.other}}"},
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
		Meta: Meta{Name: "test", OnError: "invalid"},
		Steps: []Step{{ID: "step-1"}},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "invalid on_error") {
		t.Errorf("expected on_error error, got: %v", result.Error())
	}
}

func TestValidateFull_ValidOnError(t *testing.T) {
	for _, onErr := range []string{"continue", "abort", "retry", "inject-gate"} {
		tmpl := &Template{
			Meta: Meta{Name: "test", OnError: onErr},
			Steps: []Step{{ID: "step-1"}},
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
			{ID: "check", Condition: "test -f /tmp/flag"},
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

func TestValidateFull_RestartWithoutCondition(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "restart", Type: "restart"},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "restart step without condition") {
		t.Errorf("expected restart error, got: %v", result.Error())
	}
}

func TestValidateFull_RestartWithCondition(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "restart", Type: "restart", Condition: "bd list --status=open | grep -q ."},
		},
	}

	result := ValidateFull(tmpl)
	if containsError(result, "restart step without condition") {
		t.Errorf("expected no restart error, got: %v", result.Error())
	}
}

func TestValidateFull_BlockingGateWithoutInstructions(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "gate", Type: "blocking-gate"},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "blocking-gate without instructions") {
		t.Errorf("expected gate error, got: %v", result.Error())
	}
}

func TestValidateFull_MultipleErrors(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Version: "bad"}, // Missing name, bad version
		Steps: []Step{
			{ID: "step-1", Needs: []string{"unknown"}}, // Unknown dep
			{ID: "step-1"},                              // Duplicate
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
				Condition: "test -f /tmp/flag",
				OnTrue: &ExpansionTarget{
					Template: "{{my_var}}",
					Variables: map[string]string{
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
		Meta: Meta{Name: "test", Type: "invalid-type"},
		Steps: []Step{{ID: "step-1"}},
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
			Steps: []Step{{ID: "step-1"}},
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
				Condition: "test -f /tmp/ready",
				OnTrue:    &ExpansionTarget{Template: "proceed"},
				OnTimeout: &ExpansionTarget{
					Template: "{{handler}}",
					Variables: map[string]string{
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

func TestValidateFull_StepTypeInvalid(t *testing.T) {
	// Note: In legacy format, only blocking-gate and restart are valid types
	// Other types are valid in module format
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "step-1", Type: "unknown-type"},
		},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "invalid step type") {
		t.Errorf("expected invalid step type error, got: %v", result.Error())
	}
}

func TestValidateFull_EmptyStepID(t *testing.T) {
	tmpl := &Template{
		Meta:  Meta{Name: "test"},
		Steps: []Step{{ID: "", Description: "Missing ID"}},
	}

	result := ValidateFull(tmpl)
	if !containsError(result, "id is required") {
		t.Errorf("expected ID required error, got: %v", result.Error())
	}
}
