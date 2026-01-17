package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

// mockTemplateLoader is a mock implementation of TemplateLoader for testing.
type mockTemplateLoader struct {
	steps   []*types.Step
	loadErr error
}

func (m *mockTemplateLoader) Load(ctx context.Context, ref string, variables map[string]any) ([]*types.Step, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	return m.steps, nil
}

func TestExecuteExpand_Basic(t *testing.T) {
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{
				ID:       "task-1",
				Executor: types.ExecutorAgent,
				Agent: &types.AgentConfig{
					Agent:  "worker",
					Prompt: "Do something",
				},
			},
			{
				ID:       "task-2",
				Executor: types.ExecutorAgent,
				Needs:    []string{"task-1"},
				Agent: &types.AgentConfig{
					Agent:  "worker",
					Prompt: "Do something else",
				},
			},
		},
	}

	step := &types.Step{
		ID:       "expand-step",
		Executor: types.ExecutorExpand,
		Expand: &types.ExpandConfig{
			Template: ".my-template",
		},
	}

	result, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if len(result.ExpandedSteps) != 2 {
		t.Fatalf("expected 2 expanded steps, got %d", len(result.ExpandedSteps))
	}

	// Check step IDs are prefixed
	if result.ExpandedSteps[0].ID != "expand-step.task-1" {
		t.Errorf("expected ID 'expand-step.task-1', got %q", result.ExpandedSteps[0].ID)
	}
	if result.ExpandedSteps[1].ID != "expand-step.task-2" {
		t.Errorf("expected ID 'expand-step.task-2', got %q", result.ExpandedSteps[1].ID)
	}

	// Check ExpandedFrom is set
	if result.ExpandedSteps[0].ExpandedFrom != "expand-step" {
		t.Errorf("expected ExpandedFrom 'expand-step', got %q", result.ExpandedSteps[0].ExpandedFrom)
	}

	// Check status is pending
	if result.ExpandedSteps[0].Status != types.StepStatusPending {
		t.Errorf("expected status pending, got %s", result.ExpandedSteps[0].Status)
	}
}

func TestExecuteExpand_DependencyPrefixing(t *testing.T) {
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{
				ID:       "step-a",
				Executor: types.ExecutorShell,
				Shell:    &types.ShellConfig{Command: "echo a"},
			},
			{
				ID:       "step-b",
				Executor: types.ExecutorShell,
				Needs:    []string{"step-a"},
				Shell:    &types.ShellConfig{Command: "echo b"},
			},
			{
				ID:       "step-c",
				Executor: types.ExecutorShell,
				Needs:    []string{"step-a", "step-b", "external-dep"},
				Shell:    &types.ShellConfig{Command: "echo c"},
			},
		},
	}

	step := &types.Step{
		ID:       "do-work",
		Executor: types.ExecutorExpand,
		Expand:   &types.ExpandConfig{Template: ".work"},
	}

	result, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// step-a has no internal deps, should depend on expand step
	if len(result.ExpandedSteps[0].Needs) != 1 || result.ExpandedSteps[0].Needs[0] != "do-work" {
		t.Errorf("step-a should depend on expand step, got %v", result.ExpandedSteps[0].Needs)
	}

	// step-b depends on step-a, should be prefixed
	if len(result.ExpandedSteps[1].Needs) != 1 || result.ExpandedSteps[1].Needs[0] != "do-work.step-a" {
		t.Errorf("step-b should depend on 'do-work.step-a', got %v", result.ExpandedSteps[1].Needs)
	}

	// step-c has mixed deps
	stepC := result.ExpandedSteps[2]
	expectedNeeds := map[string]bool{
		"do-work.step-a": true,
		"do-work.step-b": true,
		"external-dep":   true,
	}
	for _, need := range stepC.Needs {
		if !expectedNeeds[need] {
			t.Errorf("unexpected dependency: %s", need)
		}
		delete(expectedNeeds, need)
	}
	if len(expectedNeeds) > 0 {
		t.Errorf("missing dependencies: %v", expectedNeeds)
	}
}

func TestExecuteExpand_VariableSubstitution(t *testing.T) {
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{
				ID:       "run-task",
				Executor: types.ExecutorShell,
				Shell: &types.ShellConfig{
					Command: "process {{task_id}} for {{agent}}",
					Workdir: "/work/{{agent}}",
					Env: map[string]string{
						"TASK": "{{task_id}}",
					},
				},
			},
		},
	}

	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand: &types.ExpandConfig{
			Template: ".template",
			Variables: map[string]any{
				"task_id": "task-123",
			},
		},
	}

	// Workflow-level variables
	workflowVars := map[string]any{
		"agent": "worker-1",
	}

	result, stepErr := ExecuteExpand(context.Background(), step, loader, workflowVars, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	expanded := result.ExpandedSteps[0]
	if expanded.Shell.Command != "process task-123 for worker-1" {
		t.Errorf("expected command with substituted vars, got %q", expanded.Shell.Command)
	}
	if expanded.Shell.Workdir != "/work/worker-1" {
		t.Errorf("expected workdir '/work/worker-1', got %q", expanded.Shell.Workdir)
	}
	if expanded.Shell.Env["TASK"] != "task-123" {
		t.Errorf("expected env TASK='task-123', got %q", expanded.Shell.Env["TASK"])
	}
}

func TestExecuteExpand_StepVariablesOverrideWorkflow(t *testing.T) {
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{
				ID:       "test",
				Executor: types.ExecutorShell,
				Shell:    &types.ShellConfig{Command: "echo {{value}}"},
			},
		},
	}

	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand: &types.ExpandConfig{
			Template: ".template",
			Variables: map[string]any{
				"value": "from-step",
			},
		},
	}

	// Workflow has same variable with different value
	workflowVars := map[string]any{
		"value": "from-workflow",
	}

	result, stepErr := ExecuteExpand(context.Background(), step, loader, workflowVars, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Step variables should override workflow variables
	if result.ExpandedSteps[0].Shell.Command != "echo from-step" {
		t.Errorf("step variables should override workflow, got %q", result.ExpandedSteps[0].Shell.Command)
	}
}

func TestExecuteExpand_DepthLimit(t *testing.T) {
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{ID: "task", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "true"}},
		},
	}

	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand:   &types.ExpandConfig{Template: ".template"},
	}

	limits := &ExpansionLimits{MaxDepth: 3, MaxTotalSteps: 1000}

	// At depth 2, should succeed
	_, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 2, limits)
	if stepErr != nil {
		t.Fatalf("should succeed at depth 2: %v", stepErr)
	}

	// At depth 3 (at limit), should fail
	_, stepErr = ExecuteExpand(context.Background(), step, loader, nil, 3, limits)
	if stepErr == nil {
		t.Fatal("expected depth limit error at depth 3")
	}
	if stepErr.Message != "expansion depth limit exceeded: 3 >= 3" {
		t.Errorf("unexpected error message: %s", stepErr.Message)
	}
}

func TestExecuteExpand_EmptyTemplate(t *testing.T) {
	loader := &mockTemplateLoader{
		steps: []*types.Step{}, // Empty template
	}

	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand:   &types.ExpandConfig{Template: ".empty"},
	}

	result, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("empty template should not error: %v", stepErr)
	}

	if len(result.ExpandedSteps) != 0 {
		t.Errorf("expected 0 expanded steps, got %d", len(result.ExpandedSteps))
	}
}

func TestExecuteExpand_MissingConfig(t *testing.T) {
	loader := &mockTemplateLoader{}
	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand:   nil,
	}

	_, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr == nil {
		t.Fatal("expected error for missing config")
	}
	if stepErr.Message != "expand step missing config" {
		t.Errorf("unexpected error: %s", stepErr.Message)
	}
}

func TestExecuteExpand_MissingTemplate(t *testing.T) {
	loader := &mockTemplateLoader{}
	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand:   &types.ExpandConfig{Template: ""},
	}

	_, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr == nil {
		t.Fatal("expected error for missing template")
	}
	if stepErr.Message != "expand step missing template field" {
		t.Errorf("unexpected error: %s", stepErr.Message)
	}
}

func TestExecuteExpand_LoaderError(t *testing.T) {
	loader := &mockTemplateLoader{
		loadErr: errors.New("template not found"),
	}

	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand:   &types.ExpandConfig{Template: ".nonexistent"},
	}

	_, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr == nil {
		t.Fatal("expected error from loader")
	}
	if stepErr.Message != "failed to load template .nonexistent: template not found" {
		t.Errorf("unexpected error: %s", stepErr.Message)
	}
}

func TestExecuteExpand_StepIDsReturned(t *testing.T) {
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{ID: "a", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "a"}},
			{ID: "b", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "b"}},
			{ID: "c", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "c"}},
		},
	}

	step := &types.Step{
		ID:       "parent",
		Executor: types.ExecutorExpand,
		Expand:   &types.ExpandConfig{Template: ".template"},
	}

	result, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	expected := []string{"parent.a", "parent.b", "parent.c"}
	if len(result.StepIDs) != len(expected) {
		t.Fatalf("expected %d step IDs, got %d", len(expected), len(result.StepIDs))
	}
	for i, id := range expected {
		if result.StepIDs[i] != id {
			t.Errorf("expected StepIDs[%d]=%q, got %q", i, id, result.StepIDs[i])
		}
	}
}

func TestExecuteExpand_AgentConfigSubstitution(t *testing.T) {
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{
				ID:       "work",
				Executor: types.ExecutorAgent,
				Agent: &types.AgentConfig{
					Agent:  "{{agent}}",
					Prompt: "Work on {{task}}",
				},
			},
		},
	}

	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand: &types.ExpandConfig{
			Template: ".template",
			Variables: map[string]any{
				"agent": "worker-2",
				"task":  "feature-abc",
			},
		},
	}

	result, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	expanded := result.ExpandedSteps[0]
	if expanded.Agent.Agent != "worker-2" {
		t.Errorf("expected agent 'worker-2', got %q", expanded.Agent.Agent)
	}
	if expanded.Agent.Prompt != "Work on feature-abc" {
		t.Errorf("expected prompt 'Work on feature-abc', got %q", expanded.Agent.Prompt)
	}
}

func TestExecuteExpand_SpawnConfigSubstitution(t *testing.T) {
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{
				ID:       "start",
				Executor: types.ExecutorSpawn,
				Spawn: &types.SpawnConfig{
					Agent:   "{{agent}}",
					Workdir: "/work/{{project}}",
					Env: map[string]string{
						"PROJECT": "{{project}}",
					},
				},
			},
		},
	}

	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand: &types.ExpandConfig{
			Template: ".template",
			Variables: map[string]any{
				"agent":   "agent-x",
				"project": "myproject",
			},
		},
	}

	result, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	expanded := result.ExpandedSteps[0]
	if expanded.Spawn.Agent != "agent-x" {
		t.Errorf("expected agent 'agent-x', got %q", expanded.Spawn.Agent)
	}
	if expanded.Spawn.Workdir != "/work/myproject" {
		t.Errorf("expected workdir '/work/myproject', got %q", expanded.Spawn.Workdir)
	}
	if expanded.Spawn.Env["PROJECT"] != "myproject" {
		t.Errorf("expected env PROJECT='myproject', got %q", expanded.Spawn.Env["PROJECT"])
	}
}

func TestExecuteExpand_DefaultLimits(t *testing.T) {
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{ID: "task", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "true"}},
		},
	}

	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand:   &types.ExpandConfig{Template: ".template"},
	}

	// Pass nil limits - should use defaults
	result, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("should succeed with default limits: %v", stepErr)
	}
	if len(result.ExpandedSteps) != 1 {
		t.Errorf("expected 1 step, got %d", len(result.ExpandedSteps))
	}
}

func TestExecuteExpand_SourceModulePropagation(t *testing.T) {
	// Steps with SourceModule set (as if loaded from a template file)
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{
				ID:           "task-1",
				Executor:     types.ExecutorShell,
				Shell:        &types.ShellConfig{Command: "echo 1"},
				SourceModule: "/some/module.meow.toml",
			},
			{
				ID:           "task-2",
				Executor:     types.ExecutorBranch,
				Needs:        []string{"task-1"},
				SourceModule: "/some/module.meow.toml",
				Branch: &types.BranchConfig{
					Condition: "true",
				},
			},
		},
	}

	step := &types.Step{
		ID:       "expand",
		Executor: types.ExecutorExpand,
		Expand:   &types.ExpandConfig{Template: ".template"},
	}

	result, stepErr := ExecuteExpand(context.Background(), step, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Verify SourceModule is preserved on expanded steps
	for _, expanded := range result.ExpandedSteps {
		if expanded.SourceModule != "/some/module.meow.toml" {
			t.Errorf("step %s: expected SourceModule '/some/module.meow.toml', got %q",
				expanded.ID, expanded.SourceModule)
		}
	}
}

func TestBuildVarContextRender(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		vars     map[string]any
		expected string
	}{
		{"no vars", "no vars", nil, "no vars"},
		{"simple substitution", "{{name}}", map[string]any{"name": "value"}, "value"},
		{"prefix and suffix", "pre {{x}} post", map[string]any{"x": "middle"}, "pre middle post"},
		{"multiple vars", "{{a}} and {{b}}", map[string]any{"a": "1", "b": "2"}, "1 and 2"},
		{"missing var deferred", "{{missing}}", map[string]any{}, "{{missing}}"}, // Unmatched vars left as-is
		{"empty input", "", map[string]any{"x": "y"}, ""},
		{"non-string value", "count: {{n}}", map[string]any{"n": 42}, "count: 42"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := buildVarContext(tc.vars)
			result, err := ctx.Render(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tc.expected {
				t.Errorf("buildVarContext(%v).Render(%q) = %q, expected %q",
					tc.vars, tc.input, result, tc.expected)
			}
		})
	}
}

func TestSubstituteStepVariablesTyped(t *testing.T) {
	t.Run("shell step stringifies embedded values", func(t *testing.T) {
		step := &types.Step{
			ID:       "test",
			Executor: types.ExecutorShell,
			Shell: &types.ShellConfig{
				Command: "echo {{task}}",
			},
		}
		ctx := buildVarContext(map[string]any{
			"task": map[string]any{"name": "foo", "priority": 1},
		})
		if err := substituteStepVariablesTyped(step, ctx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Embedded map should be JSON-stringified
		expected := `echo {"name":"foo","priority":1}`
		if step.Shell.Command != expected {
			t.Errorf("shell.command = %q, expected %q", step.Shell.Command, expected)
		}
	})

	t.Run("expand step variables preserves types", func(t *testing.T) {
		step := &types.Step{
			ID:       "test",
			Executor: types.ExecutorExpand,
			Expand: &types.ExpandConfig{
				Template: ".template",
				Variables: map[string]any{
					"task": "{{upstream_task}}", // Pure reference - should preserve type
				},
			},
		}
		taskValue := map[string]any{"name": "foo", "priority": 1}
		ctx := buildVarContext(map[string]any{
			"upstream_task": taskValue,
		})
		if err := substituteStepVariablesTyped(step, ctx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Variables map should preserve the map type, not stringify
		result, ok := step.Expand.Variables["task"].(map[string]any)
		if !ok {
			t.Fatalf("expected task to be map[string]any, got %T", step.Expand.Variables["task"])
		}
		if result["name"] != "foo" {
			t.Errorf("task.name = %v, expected 'foo'", result["name"])
		}
		if result["priority"] != float64(1) && result["priority"] != 1 {
			t.Errorf("task.priority = %v, expected 1", result["priority"])
		}
	})

	t.Run("mixed content stringifies", func(t *testing.T) {
		step := &types.Step{
			ID:       "test",
			Executor: types.ExecutorShell,
			Shell: &types.ShellConfig{
				Command: "prefix-{{task}}-suffix",
			},
		}
		ctx := buildVarContext(map[string]any{
			"task": map[string]any{"name": "foo"},
		})
		if err := substituteStepVariablesTyped(step, ctx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := `prefix-{"name":"foo"}-suffix`
		if step.Shell.Command != expected {
			t.Errorf("shell.command = %q, expected %q", step.Shell.Command, expected)
		}
	})

	t.Run("agent prompt renders correctly", func(t *testing.T) {
		step := &types.Step{
			ID:       "test",
			Executor: types.ExecutorAgent,
			Agent: &types.AgentConfig{
				Agent:  "{{agent_name}}",
				Prompt: "Implement {{task.name}} with priority {{task.priority}}",
			},
		}
		ctx := buildVarContext(map[string]any{
			"agent_name":    "worker-1",
			"task.name":     "authentication",
			"task.priority": 1,
		})
		if err := substituteStepVariablesTyped(step, ctx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if step.Agent.Agent != "worker-1" {
			t.Errorf("agent.agent = %q, expected 'worker-1'", step.Agent.Agent)
		}
		expectedPrompt := "Implement authentication with priority 1"
		if step.Agent.Prompt != expectedPrompt {
			t.Errorf("agent.prompt = %q, expected %q", step.Agent.Prompt, expectedPrompt)
		}
	})

	t.Run("branch step with variables preserves types", func(t *testing.T) {
		step := &types.Step{
			ID:       "test",
			Executor: types.ExecutorBranch,
			Branch: &types.BranchConfig{
				Condition: "test {{flag}}",
				OnTrue: &types.BranchTarget{
					Template: ".success",
					Variables: map[string]any{
						"data": "{{result}}",
					},
				},
			},
		}
		ctx := buildVarContext(map[string]any{
			"flag":   true,
			"result": []any{"a", "b", "c"},
		})
		if err := substituteStepVariablesTyped(step, ctx); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Condition should be stringified
		if step.Branch.Condition != "test true" {
			t.Errorf("branch.condition = %q, expected 'test true'", step.Branch.Condition)
		}
		// Variables should preserve slice type
		result, ok := step.Branch.OnTrue.Variables["data"].([]any)
		if !ok {
			t.Fatalf("expected data to be []any, got %T", step.Branch.OnTrue.Variables["data"])
		}
		if len(result) != 3 {
			t.Errorf("len(data) = %d, expected 3", len(result))
		}
	})
}
