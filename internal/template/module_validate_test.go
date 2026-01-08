package template

import (
	"strings"
	"testing"
)

func TestValidateFullModule_ValidModule(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main-workflow",
				Variables: map[string]*Var{
					"agent": {Required: true},
				},
				Steps: []*Step{
					{ID: "step-1", Title: "First step", Assignee: "{{agent}}"},
					{ID: "step-2", Title: "Second step", Assignee: "{{agent}}", Needs: []string{"step-1"}},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if result.HasErrors() {
		t.Errorf("expected no errors, got: %v", result.Error())
	}
}

func TestValidateFullModule_MissingWorkflowName(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				// Name is empty
				Steps: []*Step{{ID: "step-1"}},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "name is required") {
		t.Errorf("expected name error, got: %v", result.Error())
	}
}

func TestValidateFullModule_NoSteps(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name:  "main",
				Steps: []*Step{}, // No steps
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "at least one step") {
		t.Errorf("expected steps error, got: %v", result.Error())
	}
}

func TestValidateFullModule_DuplicateStepID(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1"},
					{ID: "step-1"}, // Duplicate
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "duplicate step id") {
		t.Errorf("expected duplicate error, got: %v", result.Error())
	}
}

func TestValidateFullModule_UnknownDependency(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Needs: []string{"nonexistent"}},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "unknown step") {
		t.Errorf("expected unknown step error, got: %v", result.Error())
	}
}

func TestValidateFullModule_DependencySuggestion(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "load-context"},
					{ID: "write-tests", Needs: []string{"load-contxt"}}, // Typo
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "did you mean") {
		t.Errorf("expected suggestion, got: %v", result.Error())
	}
}

func TestValidateFullModule_CircularDependency(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "a", Needs: []string{"b"}},
					{ID: "b", Needs: []string{"c"}},
					{ID: "c", Needs: []string{"a"}},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "circular dependency") {
		t.Errorf("expected cycle error, got: %v", result.Error())
	}
}

func TestValidateFullModule_InvalidStepType(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Type: "invalid-type"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "invalid step type") {
		t.Errorf("expected type error, got: %v", result.Error())
	}
}

func TestValidateFullModule_AllValidStepTypes(t *testing.T) {
	validTypes := []string{"task", "collaborative", "gate", "condition", "code", "start", "stop", "expand"}
	for _, typ := range validTypes {
		t.Run(typ, func(t *testing.T) {
			step := &Step{ID: "step-1", Type: typ, Instructions: "test"}
			// Add required fields for each type
			switch typ {
			case "condition":
				step.Condition = "test"
				step.OnTrue = &ExpansionTarget{Template: "other"}
			case "code":
				step.Code = "echo hello"
			case "expand":
				step.Template = "other#workflow"
			case "start", "stop":
				step.Assignee = "agent-1"
			}

			module := &Module{
				Path: "test.meow.toml",
				Workflows: map[string]*Workflow{
					"main": {Name: "main", Steps: []*Step{step}},
				},
			}

			result := ValidateFullModule(module)
			if containsModuleError(result, "invalid step type") {
				t.Errorf("expected no type error for %q, got: %v", typ, result.Error())
			}
		})
	}
}

// Local reference tests

func TestValidateFullModule_ValidLocalReference(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Type: "expand", Template: ".helper"},
				},
			},
			"helper": {
				Name:  "helper",
				Steps: []*Step{{ID: "h1"}},
			},
		},
	}

	result := ValidateFullModule(module)
	if containsModuleError(result, "unknown workflow") {
		t.Errorf("expected no error for valid local reference, got: %v", result.Error())
	}
}

func TestValidateFullModule_UnknownLocalReference(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Type: "expand", Template: ".nonexistent"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "references unknown workflow") {
		t.Errorf("expected unknown workflow error, got: %v", result.Error())
	}
}

func TestValidateFullModule_LocalReferenceSuggestion(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Type: "expand", Template: ".implemen"}, // Typo
				},
			},
			"implement": {
				Name:  "implement",
				Steps: []*Step{{ID: "i1"}},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "did you mean") {
		t.Errorf("expected suggestion for typo, got: %v", result.Error())
	}
}

func TestValidateFullModule_InternalWorkflowCanBeReferencedLocally(t *testing.T) {
	// Internal workflows CAN be referenced from within the same module file.
	// The "internal" flag only prevents external file#workflow references.
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Type: "expand", Template: ".internal-helper"},
				},
			},
			"internal-helper": {
				Name:     "internal-helper",
				Internal: true, // Marked as internal - but local refs are OK
				Steps:    []*Step{{ID: "h1"}},
			},
		},
	}

	result := ValidateFullModule(module)
	// Should NOT error - local references to internal workflows are allowed
	if containsModuleError(result, "internal") {
		t.Errorf("local references to internal workflows should be allowed, got: %v", result.Error())
	}
}

func TestValidateFullModule_LocalReferenceToStep(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Type: "expand", Template: ".helper.specific-step"},
				},
			},
			"helper": {
				Name: "helper",
				Steps: []*Step{
					{ID: "specific-step"},
					{ID: "other-step"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	// Should NOT have error - step exists
	if containsModuleError(result, "unknown step") {
		t.Errorf("expected no error for valid step reference, got: %v", result.Error())
	}
}

func TestValidateFullModule_LocalReferenceToNonexistentStep(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Type: "expand", Template: ".helper.missing-step"},
				},
			},
			"helper": {
				Name:  "helper",
				Steps: []*Step{{ID: "existing-step"}},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "references unknown step") {
		t.Errorf("expected unknown step error, got: %v", result.Error())
	}
}

func TestValidateFullModule_LocalReferenceInOnTrue(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{
						ID:        "check",
						Type:      "condition",
						Condition: "test -f /tmp/flag",
						OnTrue:    &ExpansionTarget{Template: ".missing"},
					},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "references unknown workflow") {
		t.Errorf("expected unknown workflow error in on_true, got: %v", result.Error())
	}
}

func TestValidateFullModule_LocalReferenceSkipsVariables(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Type: "expand", Template: "{{dynamic_template}}"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	// Should NOT error on variable reference - validated at runtime
	if containsModuleError(result, "unknown workflow") {
		t.Errorf("should skip variable references, got: %v", result.Error())
	}
}

func TestValidateFullModule_ExternalReferenceSkipped(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Type: "expand", Template: "other-file#workflow"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	// Should NOT error on external reference - validated elsewhere
	if containsModuleError(result, "unknown workflow") {
		t.Errorf("should skip external references, got: %v", result.Error())
	}
}

// hooks_to validation tests

func TestValidateFullModule_ValidHooksTo(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"implement": {
				Name:      "implement",
				Ephemeral: true,
				HooksTo:   "work_bead",
				Variables: map[string]*Var{
					"work_bead": {Required: true},
				},
				Steps: []*Step{{ID: "step-1"}},
			},
		},
	}

	result := ValidateFullModule(module)
	if containsModuleError(result, "hooks_to") {
		t.Errorf("expected no hooks_to error, got: %v", result.Error())
	}
}

func TestValidateFullModule_HooksToUndefinedVariable(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"implement": {
				Name:      "implement",
				Ephemeral: true,
				HooksTo:   "missing_var",
				Variables: map[string]*Var{
					"work_bead": {Required: true},
				},
				Steps: []*Step{{ID: "step-1"}},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "hooks_to references undefined variable") {
		t.Errorf("expected hooks_to error, got: %v", result.Error())
	}
}

func TestValidateFullModule_HooksToNoVariablesDefined(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"implement": {
				Name:      "implement",
				Ephemeral: true,
				HooksTo:   "work_bead",
				// No variables defined
				Steps: []*Step{{ID: "step-1"}},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "no variables are defined") {
		t.Errorf("expected hooks_to error, got: %v", result.Error())
	}
}

// Variable reference tests

func TestValidateFullModule_UndefinedVariable(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Title: "Using {{undefined_var}}"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "undefined variable") {
		t.Errorf("expected undefined variable error, got: %v", result.Error())
	}
}

func TestValidateFullModule_DefinedVariable(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Variables: map[string]*Var{
					"my_var": {Required: true},
				},
				Steps: []*Step{
					{ID: "step-1", Title: "Using {{my_var}}"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if containsModuleError(result, "undefined variable") {
		t.Errorf("expected no undefined variable error, got: %v", result.Error())
	}
}

func TestValidateFullModule_BuiltinVariables(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Title: "Time: {{timestamp}}, Agent: {{agent}}, Bead: {{bead_id}}"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if containsModuleError(result, "undefined variable") {
		t.Errorf("expected no error for builtin variables, got: %v", result.Error())
	}
}

func TestValidateFullModule_OutputReferencesSkipped(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1"},
					{ID: "step-2", Title: "Using {{output.step-1.result}} and {{step-1.outputs.value}}"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	// Output references are validated at runtime
	if containsModuleError(result, "undefined variable") {
		t.Errorf("output references should not cause errors, got: %v", result.Error())
	}
}

// Type-specific validation tests

func TestValidateFullModule_GateWithoutInstructions(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "gate-1", Type: "gate"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "gate without instructions") {
		t.Errorf("expected gate error, got: %v", result.Error())
	}
}

func TestValidateFullModule_GateWithAssignee(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "gate-1", Type: "gate", Instructions: "Approve", Assignee: "agent-1"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "must not have an assignee") {
		t.Errorf("expected gate assignee error, got: %v", result.Error())
	}
}

func TestValidateFullModule_ConditionWithoutCondition(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{
						ID:     "check",
						Type:   "condition",
						OnTrue: &ExpansionTarget{Template: "other"},
					},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "requires a condition expression") {
		t.Errorf("expected condition error, got: %v", result.Error())
	}
}

func TestValidateFullModule_ConditionWithoutBranches(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{
						ID:        "check",
						Type:      "condition",
						Condition: "test -f /tmp/flag",
						// No OnTrue or OnFalse
					},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "on_true or on_false") {
		t.Errorf("expected branch error, got: %v", result.Error())
	}
}

func TestValidateFullModule_ExpandWithoutTemplate(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "expand-1", Type: "expand"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "requires a template reference") {
		t.Errorf("expected template error, got: %v", result.Error())
	}
}

func TestValidateFullModule_CodeWithoutCode(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "code-1", Type: "code"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "requires a code block") {
		t.Errorf("expected code error, got: %v", result.Error())
	}
}

func TestValidateFullModule_StartWithoutAssignee(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "start-1", Type: "start"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "requires an assignee") {
		t.Errorf("expected assignee error, got: %v", result.Error())
	}
}

func TestValidateFullModule_StopWithoutAssignee(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "stop-1", Type: "stop"},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "requires an assignee") {
		t.Errorf("expected assignee error, got: %v", result.Error())
	}
}

// Multiple errors test

func TestValidateFullModule_MultipleErrors(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				// Missing name
				Steps: []*Step{
					{ID: "step-1", Type: "invalid"}, // Invalid type
					{ID: "step-1"},                   // Duplicate
					{ID: "step-2", Needs: []string{"unknown"}}, // Unknown dep
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if len(result.Errors) < 3 {
		t.Errorf("expected at least 3 errors, got %d: %v", len(result.Errors), result.Error())
	}
}

// Result methods tests

func TestModuleValidationResult_NoErrors(t *testing.T) {
	result := &ModuleValidationResult{}
	if result.HasErrors() {
		t.Error("expected no errors")
	}
	if result.Error() != "" {
		t.Errorf("expected empty error string, got %q", result.Error())
	}
}

func TestModuleValidationResult_Add(t *testing.T) {
	result := &ModuleValidationResult{}
	result.Add("workflow", "step-1", "field", "error message", "suggestion")

	if !result.HasErrors() {
		t.Error("expected errors")
	}
	if len(result.Errors) != 1 {
		t.Errorf("expected 1 error, got %d", len(result.Errors))
	}
}

func containsModuleError(result *ModuleValidationResult, substr string) bool {
	for _, e := range result.Errors {
		if strings.Contains(e.Error(), substr) {
			return true
		}
	}
	return false
}
