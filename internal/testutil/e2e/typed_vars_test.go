// Package e2e_test contains end-to-end tests for MEOW.
//
// This file contains typed variable tests.
// Spec: specs/typed-variables.yaml
package e2e_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akatz-ai/meow/internal/testutil/e2e"
	"github.com/akatz-ai/meow/internal/types"
	"gopkg.in/yaml.v3"
)

// TestTypedVariables_ForeachExpandChain reproduces the original failure scenario where:
// - foreach iterates over task objects [{name: "critical", task_ids: "bf-xxx"}]
// - Expands to level1 template with task = "{{task}}"
// - level1 expands to level2 with task = "{{task}}"
// - level2 accesses {{task.task_ids}}
//
// This verifies that typed variables (maps) are preserved through multiple
// expansion layers, not stringified to JSON.
// Spec: foreach-objects.foreach-expand-chain
func TestTypedVariables_ForeachExpandChain(t *testing.T) {
	h := e2e.NewHarness(t)

	// Template that mimics the sprint pattern:
	// foreach with object items → expand → expand → access {{task.field}}
	template := `
[main]
name = "typed-vars-foreach-expand-chain"

# Generate array of task objects
[[main.steps]]
id = "plan"
executor = "shell"
command = "echo '[{\"name\":\"critical\",\"task_ids\":\"bf-xxx\"},{\"name\":\"normal\",\"task_ids\":\"bf-yyy\"}]'"
[main.steps.shell_outputs]
tasks = { source = "stdout", type = "json" }

# Foreach over task objects - this is where typed variables matter
[[main.steps]]
id = "process"
executor = "foreach"
needs = ["plan"]
items = "{{plan.outputs.tasks}}"
template = ".level1"
item_var = "task"

# Verify all iterations completed
[[main.steps]]
id = "verify"
executor = "shell"
needs = ["process"]
command = "echo 'All tasks processed successfully'"

# Level 1 template - passes task object through to level 2
[level1]
name = "level1"
internal = true

[level1.variables]
task = { required = true }

[[level1.steps]]
id = "expand-level2"
executor = "expand"
template = ".level2"
[level1.steps.variables]
task = "{{task}}"

# Level 2 template - actually accesses the field
[level2]
name = "level2"
internal = true

[level2.variables]
task = { required = true }

[[level2.steps]]
id = "use-field"
executor = "shell"
# This is the critical test: accessing {{task.task_ids}} through 2 expansion layers
command = "echo 'Task: {{task.name}}, IDs: {{task.task_ids}}'"
`
	if err := h.WriteTemplate("typed-vars-chain.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "typed-vars-chain.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete\nstderr: %s", stderr)
	}

	// Verify both iterations were dispatched (foreach uses numeric indices)
	for i := 0; i < 2; i++ {
		expectedID := fmt.Sprintf("process.%d", i)
		if !strings.Contains(stderr, expectedID) {
			t.Errorf("expected iteration %d with ID prefix %s in output", i, expectedID)
		}
	}

	// Critical: verify no "cannot access field on non-map" errors
	if strings.Contains(stderr, "cannot access field") {
		t.Errorf("typed variable was stringified, got 'cannot access field' error:\n%s", stderr)
	}
	if strings.Contains(stderr, "non-map") {
		t.Errorf("typed variable was stringified, got 'non-map' error:\n%s", stderr)
	}

	t.Logf("Foreach → expand → expand → field access works correctly")
}

// TestTypedVariables_ForeachDirectFieldAccess tests a simpler case where
// foreach item_var is used directly in the template for field access.
// Spec: foreach-objects.foreach-direct-field-access
func TestTypedVariables_ForeachDirectFieldAccess(t *testing.T) {
	h := e2e.NewHarness(t)

	template := `
[main]
name = "typed-vars-direct"

# Generate array of objects
[[main.steps]]
id = "generate"
executor = "shell"
command = "echo '[{\"id\":\"task-1\",\"priority\":1},{\"id\":\"task-2\",\"priority\":2}]'"
[main.steps.shell_outputs]
items = { source = "stdout", type = "json" }

# Foreach with direct field access
[[main.steps]]
id = "process"
executor = "foreach"
needs = ["generate"]
items = "{{generate.outputs.items}}"
template = ".worker"
item_var = "item"
index_var = "idx"

[worker]
name = "worker"
internal = true

[worker.variables]
item = { required = true }
idx = { required = true }

[[worker.steps]]
id = "work"
executor = "shell"
# Access fields directly on the item variable
command = "echo 'Index={{idx}} ID={{item.id}} Priority={{item.priority}}'"
`
	if err := h.WriteTemplate("typed-vars-direct.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "typed-vars-direct.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete\nstderr: %s", stderr)
	}

	// Critical: verify no field access errors
	if strings.Contains(stderr, "cannot access field") || strings.Contains(stderr, "non-map") {
		t.Errorf("typed variable was stringified, field access failed:\n%s", stderr)
	}

	t.Logf("Foreach with direct field access works correctly")
}

// TestTypedVariables_SurvivesYAMLRoundTrip verifies that typed variables
// survive being saved to YAML and reloaded. This is critical for crash recovery.
// Spec: yaml-persistence.workflow-variables-roundtrip
func TestTypedVariables_SurvivesYAMLRoundTrip(t *testing.T) {
	h := e2e.NewHarness(t)

	// Create a run with typed variables (maps)
	originalVars := map[string]any{
		"task": map[string]any{
			"name":     "critical",
			"task_ids": "bf-xxx",
			"metadata": map[string]any{
				"created_by": "test",
				"priority":   1,
			},
		},
		"items": []any{
			map[string]any{"id": "item-1", "value": 100},
			map[string]any{"id": "item-2", "value": 200},
		},
		"simple_string": "just a string",
		"number":        42,
	}

	// Create a minimal run
	run := &types.Run{
		ID:        "test-typed-vars-roundtrip",
		Template:  "test.toml",
		Status:    types.RunStatusRunning,
		Variables: originalVars,
		Agents:    make(map[string]*types.AgentInfo),
		Steps:     make(map[string]*types.Step),
	}

	// Save to YAML
	if err := h.SaveWorkflow(run); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	// Reload from YAML
	reloaded, err := h.LoadWorkflow(run.ID)
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}

	// Verify the task variable is still a map (not a JSON string)
	task, ok := reloaded.Variables["task"]
	if !ok {
		t.Fatalf("task variable not found after reload")
	}
	taskMap, ok := task.(map[string]any)
	if !ok {
		t.Fatalf("task variable is not a map after reload, got %T", task)
	}

	// Verify nested map is preserved
	metadata, ok := taskMap["metadata"]
	if !ok {
		t.Fatalf("task.metadata not found after reload")
	}
	metadataMap, ok := metadata.(map[string]any)
	if !ok {
		t.Fatalf("task.metadata is not a map after reload, got %T", metadata)
	}

	// Verify values are correct
	if name, ok := taskMap["name"].(string); !ok || name != "critical" {
		t.Errorf("task.name not preserved, got %v", taskMap["name"])
	}
	if priority, ok := metadataMap["priority"].(int); !ok || priority != 1 {
		t.Errorf("task.metadata.priority not preserved, got %v (%T)", metadataMap["priority"], metadataMap["priority"])
	}

	// Verify array of objects is preserved
	items, ok := reloaded.Variables["items"]
	if !ok {
		t.Fatalf("items variable not found after reload")
	}
	itemsSlice, ok := items.([]any)
	if !ok {
		t.Fatalf("items variable is not a slice after reload, got %T", items)
	}
	if len(itemsSlice) != 2 {
		t.Errorf("items slice has wrong length, got %d, want 2", len(itemsSlice))
	}

	// Verify first item is a map
	item0, ok := itemsSlice[0].(map[string]any)
	if !ok {
		t.Fatalf("items[0] is not a map after reload, got %T", itemsSlice[0])
	}
	if id, ok := item0["id"].(string); !ok || id != "item-1" {
		t.Errorf("items[0].id not preserved, got %v", item0["id"])
	}

	// Verify simple values are preserved
	if str, ok := reloaded.Variables["simple_string"].(string); !ok || str != "just a string" {
		t.Errorf("simple_string not preserved, got %v", reloaded.Variables["simple_string"])
	}

	t.Logf("Typed variables survive YAML round-trip correctly")
}

// TestTypedVariables_ExpandPassThrough tests that expand preserves typed variables
// when passing them through via variables = { config = "{{init.outputs.config}}" }
// Spec: expand-pass-through.expand-preserves-objects
//
// DEPENDS ON: meow-c8uh (Update baker to accept map[string]any)
// This test will FAIL until the baker properly handles typed values from shell outputs.
// The error "cannot access field on non-map value" indicates the JSON object was
// stringified instead of being preserved as a map[string]any.
func TestTypedVariables_ExpandPassThrough(t *testing.T) {
	h := e2e.NewHarness(t)

	template := `
[main]
name = "typed-vars-expand"

# Start with a JSON object
[[main.steps]]
id = "init"
executor = "shell"
command = "echo '{\"name\":\"test-obj\",\"data\":{\"nested\":\"value\"}}'"
[main.steps.shell_outputs]
config = { source = "stdout", type = "json" }

# Expand to sub-template, passing the object through
[[main.steps]]
id = "expand-it"
executor = "expand"
template = ".use-config"
needs = ["init"]
[main.steps.variables]
config = "{{init.outputs.config}}"

[use-config]
name = "use-config"
internal = true

[use-config.variables]
config = { required = true }

[[use-config.steps]]
id = "use-nested"
executor = "shell"
command = "echo 'Name: {{config.name}}, Nested: {{config.data.nested}}'"
`
	if err := h.WriteTemplate("typed-vars-expand.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "typed-vars-expand.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Check for success - only log success message if test passes
	workflowCompleted := strings.Contains(stderr, "workflow completed")
	hasFieldAccessError := strings.Contains(stderr, "cannot access field") || strings.Contains(stderr, "non-map")

	if !workflowCompleted {
		t.Errorf("expected workflow to complete\nstderr: %s", stderr)
	}
	if hasFieldAccessError {
		t.Errorf("expand did not preserve typed variable:\n%s", stderr)
	}

	if workflowCompleted && !hasFieldAccessError {
		t.Logf("Expand pass-through preserves typed variables correctly")
	}
}

// TestTypedVariables_StepVariablesInRun verifies that step-level Variables
// map (in ExpandConfig) survives YAML round-trip with typed values.
// Spec: yaml-persistence.step-variables-roundtrip
func TestTypedVariables_StepVariablesInRun(t *testing.T) {
	h := e2e.NewHarness(t)

	// Create a run with a step that has typed variables in its Expand config
	run := &types.Run{
		ID:        "test-step-vars-roundtrip",
		Template:  "test.toml",
		Status:    types.RunStatusRunning,
		Variables: make(map[string]any),
		Agents:    make(map[string]*types.AgentInfo),
		Steps: map[string]*types.Step{
			"expand-step": {
				ID:       "expand-step",
				Executor: types.ExecutorExpand,
				Status:   types.StepStatusPending,
				Expand: &types.ExpandConfig{
					Template: ".sub-template",
					Variables: map[string]any{
						"task": map[string]any{
							"name":     "critical",
							"task_ids": "bf-xxx",
						},
						"count": 5,
					},
				},
			},
		},
	}

	// Save and reload
	if err := h.SaveWorkflow(run); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	reloaded, err := h.LoadWorkflow(run.ID)
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}

	// Verify step variables are preserved as typed values
	step := reloaded.Steps["expand-step"]
	if step == nil {
		t.Fatalf("expand-step not found after reload")
	}
	if step.Expand == nil {
		t.Fatalf("expand config not found after reload")
	}

	task, ok := step.Expand.Variables["task"]
	if !ok {
		t.Fatalf("task variable not found in step.Expand after reload")
	}
	taskMap, ok := task.(map[string]any)
	if !ok {
		t.Fatalf("step.Expand task variable is not a map after reload, got %T", task)
	}
	if taskMap["name"] != "critical" {
		t.Errorf("step.Expand task.name not preserved, got %v", taskMap["name"])
	}

	t.Logf("Step-level typed variables survive YAML round-trip correctly")
}


// TestTypedVariables_YAMLMarshalUnmarshal verifies YAML marshaling/unmarshaling
// preserves types correctly by doing a direct marshal/unmarshal test.
// Spec: yaml-persistence.nested-structures-roundtrip
func TestTypedVariables_YAMLMarshalUnmarshal(t *testing.T) {
	original := map[string]any{
		"task": map[string]any{
			"name":     "test",
			"task_ids": "bf-123",
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(original)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Unmarshal back
	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	// Verify task is still a map
	task, ok := result["task"]
	if !ok {
		t.Fatalf("task not found after round-trip")
	}
	taskMap, ok := task.(map[string]any)
	if !ok {
		t.Fatalf("task is not a map after round-trip, got %T", task)
	}
	if taskMap["name"] != "test" {
		t.Errorf("task.name not preserved, got %v", taskMap["name"])
	}

	t.Logf("Direct YAML marshal/unmarshal preserves types correctly")
}
