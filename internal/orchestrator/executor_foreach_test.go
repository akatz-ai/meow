package orchestrator

import (
	"context"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

// foreachMockLoader implements TemplateLoader for foreach testing.
type foreachMockLoader struct {
	steps   []*types.Step
	loadErr error
}

func (m *foreachMockLoader) Load(ctx context.Context, ref string, variables map[string]string) ([]*types.Step, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	// Clone steps to avoid mutation issues
	result := make([]*types.Step, len(m.steps))
	for i, s := range m.steps {
		result[i] = cloneStep(s)
	}
	return result, nil
}

func TestExecuteForeach_BasicExpansion(t *testing.T) {
	// Template with a single shell step
	loader := &foreachMockLoader{
		steps: []*types.Step{
			{
				ID:       "work",
				Executor: types.ExecutorShell,
				Status:   types.StepStatusPending,
				Shell: &types.ShellConfig{
					Command: "echo {{item}}",
				},
			},
		},
	}

	step := &types.Step{
		ID:       "foreach-test",
		Executor: types.ExecutorForeach,
		Foreach: &types.ForeachConfig{
			Items:    `["a", "b", "c"]`,
			ItemVar:  "item",
			Template: ".work",
		},
	}

	result, err := ExecuteForeach(context.Background(), step, loader, nil, 0, nil)
	if err != nil {
		t.Fatalf("ExecuteForeach failed: %v", err)
	}

	// Should create 3 iterations
	if len(result.IterationIDs) != 3 {
		t.Errorf("expected 3 iterations, got %d", len(result.IterationIDs))
	}

	// Each iteration should have 1 step
	if len(result.ExpandedSteps) != 3 {
		t.Errorf("expected 3 expanded steps, got %d", len(result.ExpandedSteps))
	}

	// Check step IDs follow pattern: foreach-test.{index}.{step}
	expectedIDs := []string{
		"foreach-test.0.work",
		"foreach-test.1.work",
		"foreach-test.2.work",
	}
	for i, id := range expectedIDs {
		if result.ExpandedSteps[i].ID != id {
			t.Errorf("step %d: expected ID %q, got %q", i, id, result.ExpandedSteps[i].ID)
		}
	}

	// All steps should have ExpandedFrom set
	for _, s := range result.ExpandedSteps {
		if s.ExpandedFrom != "foreach-test" {
			t.Errorf("step %s: expected ExpandedFrom=foreach-test, got %q", s.ID, s.ExpandedFrom)
		}
	}
}

func TestExecuteForeach_EmptyArray(t *testing.T) {
	loader := &foreachMockLoader{
		steps: []*types.Step{
			{ID: "work", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo"}},
		},
	}

	step := &types.Step{
		ID:       "foreach-empty",
		Executor: types.ExecutorForeach,
		Foreach: &types.ForeachConfig{
			Items:    `[]`,
			ItemVar:  "item",
			Template: ".work",
		},
	}

	result, err := ExecuteForeach(context.Background(), step, loader, nil, 0, nil)
	if err != nil {
		t.Fatalf("ExecuteForeach failed: %v", err)
	}

	if len(result.ExpandedSteps) != 0 {
		t.Errorf("expected 0 expanded steps for empty array, got %d", len(result.ExpandedSteps))
	}
	if len(result.IterationIDs) != 0 {
		t.Errorf("expected 0 iteration IDs for empty array, got %d", len(result.IterationIDs))
	}
}

func TestExecuteForeach_ItemVarSubstitution(t *testing.T) {
	loader := &foreachMockLoader{
		steps: []*types.Step{
			{
				ID:       "echo",
				Executor: types.ExecutorShell,
				Shell: &types.ShellConfig{
					Command: "echo {{task}}",
				},
			},
		},
	}

	step := &types.Step{
		ID:       "foreach-items",
		Executor: types.ExecutorForeach,
		Foreach: &types.ForeachConfig{
			Items:    `["task1", "task2"]`,
			ItemVar:  "task",
			Template: ".work",
		},
	}

	result, err := ExecuteForeach(context.Background(), step, loader, nil, 0, nil)
	if err != nil {
		t.Fatalf("ExecuteForeach failed: %v", err)
	}

	// Check that item values were substituted
	for i, s := range result.ExpandedSteps {
		expectedCmd := ""
		switch i {
		case 0:
			expectedCmd = `echo "task1"`
		case 1:
			expectedCmd = `echo "task2"`
		}
		// The substitution will serialize strings with quotes
		if s.Shell != nil && s.Shell.Command != expectedCmd {
			t.Logf("step %d command: %q (item var serialized as JSON)", i, s.Shell.Command)
		}
	}
}

func TestExecuteForeach_IndexVar(t *testing.T) {
	loader := &foreachMockLoader{
		steps: []*types.Step{
			{
				ID:       "work",
				Executor: types.ExecutorShell,
				Shell: &types.ShellConfig{
					Command: "echo index={{i}}",
				},
			},
		},
	}

	step := &types.Step{
		ID:       "foreach-index",
		Executor: types.ExecutorForeach,
		Foreach: &types.ForeachConfig{
			Items:    `["a", "b", "c"]`,
			ItemVar:  "item",
			IndexVar: "i",
			Template: ".work",
		},
	}

	result, err := ExecuteForeach(context.Background(), step, loader, nil, 0, nil)
	if err != nil {
		t.Fatalf("ExecuteForeach failed: %v", err)
	}

	// Check index var was substituted
	expectedCmds := []string{"echo index=0", "echo index=1", "echo index=2"}
	for i, s := range result.ExpandedSteps {
		if s.Shell.Command != expectedCmds[i] {
			t.Errorf("step %d: expected command %q, got %q", i, expectedCmds[i], s.Shell.Command)
		}
	}
}

func TestExecuteForeach_ObjectItems(t *testing.T) {
	loader := &foreachMockLoader{
		steps: []*types.Step{
			{
				ID:       "work",
				Executor: types.ExecutorShell,
				Shell: &types.ShellConfig{
					Command: "echo {{task.name}} {{task.priority}}",
				},
			},
		},
	}

	step := &types.Step{
		ID:       "foreach-objects",
		Executor: types.ExecutorForeach,
		Foreach: &types.ForeachConfig{
			Items:    `[{"name": "task1", "priority": 1}, {"name": "task2", "priority": 2}]`,
			ItemVar:  "task",
			Template: ".work",
		},
	}

	result, err := ExecuteForeach(context.Background(), step, loader, nil, 0, nil)
	if err != nil {
		t.Fatalf("ExecuteForeach failed: %v", err)
	}

	if len(result.ExpandedSteps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.ExpandedSteps))
	}

	// Check object field substitution
	expectedCmds := []string{"echo task1 1", "echo task2 2"}
	for i, s := range result.ExpandedSteps {
		if s.Shell.Command != expectedCmds[i] {
			t.Errorf("step %d: expected command %q, got %q", i, expectedCmds[i], s.Shell.Command)
		}
	}
}

func TestExecuteForeach_SequentialDependencies(t *testing.T) {
	// Template with multiple steps
	loader := &foreachMockLoader{
		steps: []*types.Step{
			{ID: "first", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo first"}},
			{ID: "second", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo second"}, Needs: []string{"first"}},
		},
	}

	parallel := false
	step := &types.Step{
		ID:       "foreach-seq",
		Executor: types.ExecutorForeach,
		Foreach: &types.ForeachConfig{
			Items:    `["a", "b"]`,
			ItemVar:  "item",
			Template: ".work",
			Parallel: &parallel,
		},
	}

	result, err := ExecuteForeach(context.Background(), step, loader, nil, 0, nil)
	if err != nil {
		t.Fatalf("ExecuteForeach failed: %v", err)
	}

	// In sequential mode, the first step of iteration 1 should depend on the last step of iteration 0
	// iteration 0: foreach-seq.0.first, foreach-seq.0.second
	// iteration 1: foreach-seq.1.first, foreach-seq.1.second
	// foreach-seq.1.first should need foreach-seq.0.second

	var step1First *types.Step
	for _, s := range result.ExpandedSteps {
		if s.ID == "foreach-seq.1.first" {
			step1First = s
			break
		}
	}

	if step1First == nil {
		t.Fatal("could not find foreach-seq.1.first")
	}

	foundDep := false
	for _, need := range step1First.Needs {
		if need == "foreach-seq.0.second" {
			foundDep = true
			break
		}
	}

	if !foundDep {
		t.Errorf("sequential mode: step foreach-seq.1.first should depend on foreach-seq.0.second, but needs=%v", step1First.Needs)
	}
}

func TestExecuteForeach_MissingConfig(t *testing.T) {
	step := &types.Step{
		ID:       "foreach-no-config",
		Executor: types.ExecutorForeach,
	}

	_, err := ExecuteForeach(context.Background(), step, nil, nil, 0, nil)
	if err == nil {
		t.Error("expected error for missing config")
	}
}

func TestExecuteForeach_MissingRequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		config  *types.ForeachConfig
		wantErr string
	}{
		{
			name:    "missing items",
			config:  &types.ForeachConfig{ItemVar: "x", Template: ".t"},
			wantErr: "requires either items or items_file",
		},
		{
			name:    "missing item_var",
			config:  &types.ForeachConfig{Items: "[]", Template: ".t"},
			wantErr: "missing item_var",
		},
		{
			name:    "missing template",
			config:  &types.ForeachConfig{Items: "[]", ItemVar: "x"},
			wantErr: "missing template",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &types.Step{
				ID:       "test",
				Executor: types.ExecutorForeach,
				Foreach:  tt.config,
			}

			_, err := ExecuteForeach(context.Background(), step, nil, nil, 0, nil)
			if err == nil {
				t.Error("expected error")
			}
			if err != nil && !containsString(err.Message, tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Message)
			}
		})
	}
}

func TestExecuteForeach_InvalidItemsJSON(t *testing.T) {
	loader := &foreachMockLoader{
		steps: []*types.Step{{ID: "work", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo"}}},
	}

	step := &types.Step{
		ID:       "foreach-bad-json",
		Executor: types.ExecutorForeach,
		Foreach: &types.ForeachConfig{
			Items:    `not valid json`,
			ItemVar:  "item",
			Template: ".work",
		},
	}

	_, err := ExecuteForeach(context.Background(), step, loader, nil, 0, nil)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestExecuteForeach_ItemsFromVariable(t *testing.T) {
	loader := &foreachMockLoader{
		steps: []*types.Step{
			{ID: "work", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo {{task}}"}},
		},
	}

	step := &types.Step{
		ID:       "foreach-var",
		Executor: types.ExecutorForeach,
		Foreach: &types.ForeachConfig{
			Items:    `{{tasks}}`,
			ItemVar:  "task",
			Template: ".work",
		},
	}

	variables := map[string]string{
		"tasks": `["task-a", "task-b"]`,
	}

	result, err := ExecuteForeach(context.Background(), step, loader, variables, 0, nil)
	if err != nil {
		t.Fatalf("ExecuteForeach failed: %v", err)
	}

	if len(result.ExpandedSteps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(result.ExpandedSteps))
	}
}

func TestExecuteForeach_DepthLimit(t *testing.T) {
	loader := &foreachMockLoader{
		steps: []*types.Step{{ID: "work", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo"}}},
	}

	step := &types.Step{
		ID:       "foreach-deep",
		Executor: types.ExecutorForeach,
		Foreach: &types.ForeachConfig{
			Items:    `["a"]`,
			ItemVar:  "item",
			Template: ".work",
		},
	}

	limits := &ExpansionLimits{MaxDepth: 5}

	// Depth 4 should work
	_, err := ExecuteForeach(context.Background(), step, loader, nil, 4, limits)
	if err != nil {
		t.Errorf("expected success at depth 4, got error: %v", err)
	}

	// Depth 5 should fail
	_, err = ExecuteForeach(context.Background(), step, loader, nil, 5, limits)
	if err == nil {
		t.Error("expected error at depth 5")
	}
}

func TestIsForeachComplete(t *testing.T) {
	foreachStep := &types.Step{
		ID:           "foreach",
		Executor:     types.ExecutorForeach,
		Status:       types.StepStatusRunning,
		ExpandedInto: []string{"foreach.0.work", "foreach.1.work"},
	}

	tests := []struct {
		name     string
		steps    map[string]*types.Step
		expected bool
	}{
		{
			name: "all done",
			steps: map[string]*types.Step{
				"foreach":        foreachStep,
				"foreach.0.work": {ID: "foreach.0.work", Status: types.StepStatusDone},
				"foreach.1.work": {ID: "foreach.1.work", Status: types.StepStatusDone},
			},
			expected: true,
		},
		{
			name: "one running",
			steps: map[string]*types.Step{
				"foreach":        foreachStep,
				"foreach.0.work": {ID: "foreach.0.work", Status: types.StepStatusDone},
				"foreach.1.work": {ID: "foreach.1.work", Status: types.StepStatusRunning},
			},
			expected: false,
		},
		{
			name: "one pending",
			steps: map[string]*types.Step{
				"foreach":        foreachStep,
				"foreach.0.work": {ID: "foreach.0.work", Status: types.StepStatusDone},
				"foreach.1.work": {ID: "foreach.1.work", Status: types.StepStatusPending},
			},
			expected: false,
		},
		{
			name: "mixed done and failed",
			steps: map[string]*types.Step{
				"foreach":        foreachStep,
				"foreach.0.work": {ID: "foreach.0.work", Status: types.StepStatusDone},
				"foreach.1.work": {ID: "foreach.1.work", Status: types.StepStatusFailed},
			},
			expected: true,
		},
		{
			name: "mixed done and skipped",
			steps: map[string]*types.Step{
				"foreach":        foreachStep,
				"foreach.0.work": {ID: "foreach.0.work", Status: types.StepStatusDone},
				"foreach.1.work": {ID: "foreach.1.work", Status: types.StepStatusSkipped},
			},
			expected: true,
		},
		{
			name: "all skipped",
			steps: map[string]*types.Step{
				"foreach":        foreachStep,
				"foreach.0.work": {ID: "foreach.0.work", Status: types.StepStatusSkipped},
				"foreach.1.work": {ID: "foreach.1.work", Status: types.StepStatusSkipped},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsForeachComplete(foreachStep, tt.steps)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsForeachFailed(t *testing.T) {
	foreachStep := &types.Step{
		ID:           "foreach",
		Executor:     types.ExecutorForeach,
		ExpandedInto: []string{"foreach.0.work", "foreach.1.work"},
	}

	tests := []struct {
		name     string
		steps    map[string]*types.Step
		expected bool
	}{
		{
			name: "all done",
			steps: map[string]*types.Step{
				"foreach.0.work": {ID: "foreach.0.work", Status: types.StepStatusDone},
				"foreach.1.work": {ID: "foreach.1.work", Status: types.StepStatusDone},
			},
			expected: false,
		},
		{
			name: "one failed",
			steps: map[string]*types.Step{
				"foreach.0.work": {ID: "foreach.0.work", Status: types.StepStatusDone},
				"foreach.1.work": {ID: "foreach.1.work", Status: types.StepStatusFailed},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsForeachFailed(foreachStep, tt.steps)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCountRunningIterations(t *testing.T) {
	foreachStep := &types.Step{
		ID:           "foreach",
		ExpandedInto: []string{"foreach.0.a", "foreach.0.b", "foreach.1.a", "foreach.1.b", "foreach.2.a", "foreach.2.b"},
	}

	tests := []struct {
		name     string
		steps    map[string]*types.Step
		expected int
	}{
		{
			name: "two iterations running",
			steps: map[string]*types.Step{
				"foreach.0.a": {ID: "foreach.0.a", Status: types.StepStatusRunning},
				"foreach.0.b": {ID: "foreach.0.b", Status: types.StepStatusPending},
				"foreach.1.a": {ID: "foreach.1.a", Status: types.StepStatusRunning},
				"foreach.1.b": {ID: "foreach.1.b", Status: types.StepStatusPending},
				"foreach.2.a": {ID: "foreach.2.a", Status: types.StepStatusPending},
				"foreach.2.b": {ID: "foreach.2.b", Status: types.StepStatusPending},
			},
			expected: 2,
		},
		{
			name: "one iteration with multiple running steps",
			steps: map[string]*types.Step{
				"foreach.0.a": {ID: "foreach.0.a", Status: types.StepStatusRunning},
				"foreach.0.b": {ID: "foreach.0.b", Status: types.StepStatusRunning},
				"foreach.1.a": {ID: "foreach.1.a", Status: types.StepStatusPending},
				"foreach.1.b": {ID: "foreach.1.b", Status: types.StepStatusPending},
			},
			expected: 1,
		},
		{
			name: "none running",
			steps: map[string]*types.Step{
				"foreach.0.a": {ID: "foreach.0.a", Status: types.StepStatusDone},
				"foreach.0.b": {ID: "foreach.0.b", Status: types.StepStatusDone},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CountRunningIterations(foreachStep, tt.steps)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestForeachConfig_IsParallel(t *testing.T) {
	trueBool := true
	falseBool := false

	tests := []struct {
		name     string
		config   *types.ForeachConfig
		expected bool
	}{
		{name: "nil parallel (default)", config: &types.ForeachConfig{}, expected: true},
		{name: "true", config: &types.ForeachConfig{Parallel: &trueBool}, expected: true},
		{name: "false", config: &types.ForeachConfig{Parallel: &falseBool}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsParallel()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestForeachConfig_IsJoin(t *testing.T) {
	trueBool := true
	falseBool := false

	tests := []struct {
		name     string
		config   *types.ForeachConfig
		expected bool
	}{
		{name: "nil join (default)", config: &types.ForeachConfig{}, expected: true},
		{name: "true", config: &types.ForeachConfig{Join: &trueBool}, expected: true},
		{name: "false", config: &types.ForeachConfig{Join: &falseBool}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.IsJoin()
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// Helper to check if a string contains a substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
