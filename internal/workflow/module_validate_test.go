package workflow

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
					{ID: "step-1", Executor: ExecutorAgent, Agent: "{{agent}}", Prompt: "First task"},
					{ID: "step-2", Executor: ExecutorAgent, Agent: "{{agent}}", Prompt: "Second task", Needs: []string{"step-1"}},
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
				Steps: []*Step{{ID: "step-1", Executor: ExecutorShell, Command: "echo test"}},
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
					{ID: "step-1", Executor: ExecutorShell, Command: "echo 1"},
					{ID: "step-1", Executor: ExecutorShell, Command: "echo 2"}, // Duplicate
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
					{ID: "step-1", Executor: ExecutorShell, Command: "echo test", Needs: []string{"nonexistent"}},
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
					{ID: "load-context", Executor: ExecutorShell, Command: "echo load"},
					{ID: "write-tests", Executor: ExecutorShell, Command: "echo write", Needs: []string{"load-contxt"}}, // Typo
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
					{ID: "a", Executor: ExecutorShell, Command: "echo a", Needs: []string{"b"}},
					{ID: "b", Executor: ExecutorShell, Command: "echo b", Needs: []string{"c"}},
					{ID: "c", Executor: ExecutorShell, Command: "echo c", Needs: []string{"a"}},
				},
			},
		},
	}

	result := ValidateFullModule(module)
	if !containsModuleError(result, "circular dependency") {
		t.Errorf("expected cycle error, got: %v", result.Error())
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
					{ID: "step-1", Executor: ExecutorExpand, Template: ".helper"},
				},
			},
			"helper": {
				Name:  "helper",
				Steps: []*Step{{ID: "h1", Executor: ExecutorShell, Command: "echo helper"}},
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
					{ID: "step-1", Executor: ExecutorExpand, Template: ".nonexistent"},
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
					{ID: "step-1", Executor: ExecutorExpand, Template: ".implemen"}, // Typo
				},
			},
			"implement": {
				Name:  "implement",
				Steps: []*Step{{ID: "i1", Executor: ExecutorShell, Command: "echo impl"}},
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
					{ID: "step-1", Executor: ExecutorExpand, Template: ".internal-helper"},
				},
			},
			"internal-helper": {
				Name:     "internal-helper",
				Internal: true, // Marked as internal - but local refs are OK
				Steps:    []*Step{{ID: "h1", Executor: ExecutorShell, Command: "echo internal"}},
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
					{ID: "step-1", Executor: ExecutorExpand, Template: ".helper.specific-step"},
				},
			},
			"helper": {
				Name: "helper",
				Steps: []*Step{
					{ID: "specific-step", Executor: ExecutorShell, Command: "echo specific"},
					{ID: "other-step", Executor: ExecutorShell, Command: "echo other"},
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
					{ID: "step-1", Executor: ExecutorExpand, Template: ".helper.missing-step"},
				},
			},
			"helper": {
				Name:  "helper",
				Steps: []*Step{{ID: "existing-step", Executor: ExecutorShell, Command: "echo existing"}},
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
						Executor:  ExecutorBranch,
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
					{ID: "step-1", Executor: ExecutorExpand, Template: "{{dynamic_template}}"},
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
					{ID: "step-1", Executor: ExecutorExpand, Template: "other-file#workflow"},
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

// Variable reference tests

func TestValidateFullModule_UndefinedVariable(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				Name: "main",
				Steps: []*Step{
					{ID: "step-1", Executor: ExecutorShell, Command: "echo {{undefined_var}}"},
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
					{ID: "step-1", Executor: ExecutorShell, Command: "echo {{my_var}}"},
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
					{ID: "step-1", Executor: ExecutorShell, Command: "echo Time: {{timestamp}}, Agent: {{agent}}, Bead: {{bead_id}}"},
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
					{ID: "step-1", Executor: ExecutorShell, Command: "echo hello"},
					{ID: "step-2", Executor: ExecutorShell, Command: "echo {{output.step-1.result}} and {{step-1.outputs.value}}"},
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

// Multiple errors test

func TestValidateFullModule_MultipleErrors(t *testing.T) {
	module := &Module{
		Path: "test.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {
				// Missing name
				Steps: []*Step{
					{ID: "step-1", Executor: ExecutorShell, Command: "echo 1"},
					{ID: "step-1", Executor: ExecutorShell, Command: "echo 2"},                                     // Duplicate
					{ID: "step-2", Executor: ExecutorShell, Command: "echo 3", Needs: []string{"unknown"}}, // Unknown dep
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
