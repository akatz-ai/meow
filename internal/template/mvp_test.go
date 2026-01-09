package template

import (
	"strings"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// TestMinimalViableSlice validates the end-to-end flow:
// Module parsing → Workflow extraction → Baking with tier detection → HookBead linking
func TestMinimalViableSlice(t *testing.T) {
	// A minimal module with an ephemeral workflow (wisps) that hooks to a work bead
	moduleToml := `
# Work selection workflow
[main]
name = "work-loop"
description = "Main work selection and execution loop"

[main.variables]
agent = { required = true, type = "string", description = "Agent ID" }

[[main.steps]]
id = "select-work"
type = "task"
title = "Select next work bead"
assignee = "{{agent}}"

# TDD Implementation workflow (ephemeral = wisps)
[implement]
name = "implement"
description = "TDD implementation workflow"
ephemeral = true
internal = true
hooks_to = "work_bead"

[implement.variables]
work_bead = { required = true, type = "string" }
agent = { required = true, type = "string" }

[[implement.steps]]
id = "load-context"
type = "task"
title = "Load context for {{work_bead}}"
instructions = "Read the bead and understand the requirements"
assignee = "{{agent}}"

[[implement.steps]]
id = "write-tests"
type = "task"
title = "Write failing tests"
instructions = "Write tests that define expected behavior"
assignee = "{{agent}}"
needs = ["load-context"]

[[implement.steps]]
id = "implement"
type = "task"
title = "Implement to pass tests"
instructions = "Write minimum code to pass tests"
assignee = "{{agent}}"
needs = ["write-tests"]

[[implement.steps]]
id = "review"
type = "collaborative"
title = "Design review"
instructions = "Review implementation with user"
assignee = "{{agent}}"
needs = ["implement"]
`

	t.Run("ParseModuleFormat", func(t *testing.T) {
		module, err := ParseModuleString(moduleToml, "test.meow.toml")
		if err != nil {
			t.Fatalf("ParseModuleString failed: %v", err)
		}

		// Should have two workflows
		if len(module.Workflows) != 2 {
			t.Errorf("expected 2 workflows, got %d", len(module.Workflows))
		}

		// Main workflow should exist
		main := module.GetWorkflow("main")
		if main == nil {
			t.Fatal("main workflow not found")
		}
		if main.Name != "work-loop" {
			t.Errorf("expected main name 'work-loop', got %q", main.Name)
		}
		if main.Ephemeral {
			t.Error("main should not be ephemeral")
		}

		// Implement workflow should be ephemeral
		impl := module.GetWorkflow("implement")
		if impl == nil {
			t.Fatal("implement workflow not found")
		}
		if !impl.Ephemeral {
			t.Error("implement should be ephemeral")
		}
		if !impl.Internal {
			t.Error("implement should be internal")
		}
		if impl.HooksTo != "work_bead" {
			t.Errorf("expected hooks_to 'work_bead', got %q", impl.HooksTo)
		}
	})

	t.Run("BakeEphemeralWorkflowAsWisps", func(t *testing.T) {
		module, err := ParseModuleString(moduleToml, "test.meow.toml")
		if err != nil {
			t.Fatalf("ParseModuleString failed: %v", err)
		}

		impl := module.GetWorkflow("implement")
		if impl == nil {
			t.Fatal("implement workflow not found")
		}

		// Create baker
		baker := NewBaker("meow-test-123")
		baker.Assignee = "test-agent" // Default assignee
		baker.Now = func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) }

		// Bake with variables
		result, err := baker.BakeWorkflow(impl, map[string]string{
			"work_bead": "bd-task-456",
			"agent":     "claude-1",
		})
		if err != nil {
			t.Fatalf("BakeWorkflow failed: %v", err)
		}

		// Should produce 4 beads
		if len(result.Beads) != 4 {
			t.Errorf("expected 4 beads, got %d", len(result.Beads))
		}

		// All should be wisps (ephemeral workflow)
		for _, bead := range result.Beads {
			if bead.Tier != types.TierWisp {
				t.Errorf("bead %s: expected tier wisp, got %s", bead.ID, bead.Tier)
			}
		}

		// All should link to the work bead via HookBead
		for _, bead := range result.Beads {
			if bead.HookBead != "bd-task-456" {
				t.Errorf("bead %s: expected HookBead 'bd-task-456', got %q", bead.ID, bead.HookBead)
			}
		}

		// Check assignee substitution
		for _, bead := range result.Beads {
			if bead.Assignee != "claude-1" {
				t.Errorf("bead %s: expected assignee 'claude-1', got %q", bead.ID, bead.Assignee)
			}
		}

		// Find the review step (collaborative)
		var reviewBead *types.Bead
		for _, bead := range result.Beads {
			if bead.Type == types.BeadTypeCollaborative {
				reviewBead = bead
				break
			}
		}
		if reviewBead == nil {
			t.Fatal("review bead (collaborative) not found")
		}
		if reviewBead.Type != types.BeadTypeCollaborative {
			t.Errorf("review bead: expected type collaborative, got %s", reviewBead.Type)
		}
	})

	t.Run("BakeMainWorkflowAsWork", func(t *testing.T) {
		module, err := ParseModuleString(moduleToml, "test.meow.toml")
		if err != nil {
			t.Fatalf("ParseModuleString failed: %v", err)
		}

		main := module.GetWorkflow("main")
		if main == nil {
			t.Fatal("main workflow not found")
		}

		baker := NewBaker("meow-main-123")
		baker.Now = func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) }

		result, err := baker.BakeWorkflow(main, map[string]string{
			"agent": "claude-1",
		})
		if err != nil {
			t.Fatalf("BakeWorkflow failed: %v", err)
		}

		// Main workflow is not ephemeral, so beads should be work tier
		for _, bead := range result.Beads {
			if bead.Tier != types.TierWork {
				t.Errorf("bead %s: expected tier work, got %s", bead.ID, bead.Tier)
			}
		}

		// No HookBead for main workflow
		for _, bead := range result.Beads {
			if bead.HookBead != "" {
				t.Errorf("bead %s: expected no HookBead, got %q", bead.ID, bead.HookBead)
			}
		}
	})

	t.Run("TierDetectionByStepType", func(t *testing.T) {
		// Test that orchestrator types get orchestrator tier even in ephemeral workflow
		orchestratorModule := `
[main]
name = "with-orchestrator"
ephemeral = true

[[main.steps]]
id = "check-ready"
type = "condition"
condition = "test -f /tmp/ready"

[[main.steps]]
id = "do-work"
type = "task"
title = "Do work"
`
		module, err := ParseModuleString(orchestratorModule, "test.meow.toml")
		if err != nil {
			t.Fatalf("ParseModuleString failed: %v", err)
		}

		main := module.GetWorkflow("main")
		baker := NewBaker("meow-tier-test")
		baker.Now = func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) }

		result, err := baker.BakeWorkflow(main, nil)
		if err != nil {
			t.Fatalf("BakeWorkflow failed: %v", err)
		}

		// condition should be orchestrator tier
		// task in ephemeral workflow should be wisp tier
		for _, bead := range result.Beads {
			switch bead.Type {
			case types.BeadTypeCondition:
				if bead.Tier != types.TierOrchestrator {
					t.Errorf("condition bead: expected tier orchestrator, got %s", bead.Tier)
				}
			case types.BeadTypeTask:
				if bead.Tier != types.TierWisp {
					t.Errorf("task bead: expected tier wisp (in ephemeral workflow), got %s", bead.Tier)
				}
			}
		}
	})

	t.Run("SourceWorkflowTracking", func(t *testing.T) {
		module, err := ParseModuleString(moduleToml, "test.meow.toml")
		if err != nil {
			t.Fatalf("ParseModuleString failed: %v", err)
		}

		impl := module.GetWorkflow("implement")
		baker := NewBaker("meow-workflow-123")
		baker.Now = func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) }

		result, err := baker.BakeWorkflow(impl, map[string]string{
			"work_bead": "bd-123",
			"agent":     "test",
		})
		if err != nil {
			t.Fatalf("BakeWorkflow failed: %v", err)
		}

		// All beads should track their source workflow
		for _, bead := range result.Beads {
			if bead.SourceWorkflow != "implement" {
				t.Errorf("bead %s: expected SourceWorkflow 'implement', got %q", bead.ID, bead.SourceWorkflow)
			}
			if bead.WorkflowID != "meow-workflow-123" {
				t.Errorf("bead %s: expected WorkflowID 'meow-workflow-123', got %q", bead.ID, bead.WorkflowID)
			}
		}
	})
}

func TestGateBeadConstraints(t *testing.T) {
	gateModule := `
[main]
name = "with-gate"

[[main.steps]]
id = "await-approval"
type = "gate"
title = "Human approval"
`
	module, err := ParseModuleString(gateModule, "test.meow.toml")
	if err != nil {
		t.Fatalf("ParseModuleString failed: %v", err)
	}

	main := module.GetWorkflow("main")
	baker := NewBaker("meow-gate-test")
	baker.Assignee = "should-be-cleared" // Gate shouldn't have assignee
	baker.Now = func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) }

	result, err := baker.BakeWorkflow(main, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	gateBead := result.Beads[0]
	if gateBead.Type != types.BeadTypeGate {
		t.Errorf("expected gate type, got %s", gateBead.Type)
	}
	if gateBead.Assignee != "" {
		t.Errorf("gate bead should not have assignee, got %q", gateBead.Assignee)
	}
	if gateBead.Tier != types.TierOrchestrator {
		t.Errorf("gate bead should be orchestrator tier, got %s", gateBead.Tier)
	}
}

func TestModule_DefaultWorkflow(t *testing.T) {
	moduleToml := `
[main]
name = "work-loop"

[[main.steps]]
id = "step-1"
type = "task"

[other]
name = "other-workflow"

[[other.steps]]
id = "step-1"
type = "task"
`
	module, err := ParseModuleString(moduleToml, "test.meow.toml")
	if err != nil {
		t.Fatalf("ParseModuleString failed: %v", err)
	}

	// DefaultWorkflow returns "main"
	main := module.DefaultWorkflow()
	if main == nil {
		t.Fatal("expected DefaultWorkflow to return main")
	}
	if main.Name != "work-loop" {
		t.Errorf("expected main workflow name, got %q", main.Name)
	}
}

func TestWorkflow_IsInternal(t *testing.T) {
	moduleToml := `
[internal-workflow]
name = "internal"
internal = true

[[internal-workflow.steps]]
id = "step-1"
type = "task"

[public-workflow]
name = "public"
internal = false

[[public-workflow.steps]]
id = "step-1"
type = "task"
`
	module, err := ParseModuleString(moduleToml, "test.meow.toml")
	if err != nil {
		t.Fatalf("ParseModuleString failed: %v", err)
	}

	internal := module.GetWorkflow("internal-workflow")
	if !internal.IsInternal() {
		t.Error("expected internal-workflow to be internal")
	}

	public := module.GetWorkflow("public-workflow")
	if public.IsInternal() {
		t.Error("expected public-workflow to not be internal")
	}
}

func TestDetectFormat(t *testing.T) {
	legacy := `
[meta]
name = "test"
version = "1.0.0"

[[steps]]
id = "step-1"
`
	if DetectFormat(legacy) != FormatLegacy {
		t.Error("expected FormatLegacy for [meta] content")
	}

	module := `
[main]
name = "work-loop"

[[main.steps]]
id = "step-1"
`
	if DetectFormat(module) != FormatModule {
		t.Error("expected FormatModule for non-[meta] content")
	}
}

func TestParseModuleString_Errors(t *testing.T) {
	// Invalid TOML
	_, err := ParseModuleString("{{invalid toml", "test.toml")
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}

	// No workflows
	_, err = ParseModuleString("", "test.toml")
	if err == nil {
		t.Fatal("expected error for empty module")
	}

	// Workflow validation error - missing steps
	_, err = ParseModuleString(`
[main]
name = "test"
`, "test.toml")
	if err == nil {
		t.Fatal("expected error for workflow with no steps")
	}
}

func TestWorkflow_Validate_Errors(t *testing.T) {
	// Duplicate step ID
	_, err := ParseModuleString(`
[main]
name = "test"

[[main.steps]]
id = "step-1"
type = "task"

[[main.steps]]
id = "step-1"
type = "task"
`, "test.toml")
	if err == nil {
		t.Fatal("expected error for duplicate step ID")
	}

	// Unknown dependency
	_, err = ParseModuleString(`
[main]
name = "test"

[[main.steps]]
id = "step-1"
type = "task"
needs = ["nonexistent"]
`, "test.toml")
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}

	// Invalid step type
	_, err = ParseModuleString(`
[main]
name = "test"

[[main.steps]]
id = "step-1"
type = "invalid-type"
`, "test.toml")
	if err == nil {
		t.Fatal("expected error for invalid step type")
	}
}

func TestParseModuleString_StepMissingID(t *testing.T) {
	_, err := ParseModuleString(`
[main]
name = "test"

[[main.steps]]
type = "task"
`, "test.toml")
	if err == nil {
		t.Fatal("expected error for step missing ID")
	}
}

func TestParseModuleString_AllStepFields(t *testing.T) {
	moduleToml := `
[main]
name = "test"

[[main.steps]]
id = "full-step"
type = "condition"
title = "Check something"
description = "Description here"
instructions = "Instructions here"
assignee = "claude-1"
condition = "test -f /tmp/ready"
code = "echo hello"
timeout = "5m"
template = "child-template"
ephemeral = true
needs = ["other"]
variables = { key = "value" }

[[main.steps]]
id = "other"
type = "task"
`
	module, err := ParseModuleString(moduleToml, "test.toml")
	if err != nil {
		t.Fatalf("ParseModuleString failed: %v", err)
	}

	main := module.GetWorkflow("main")
	step := main.Steps[0]

	if step.Title != "Check something" {
		t.Errorf("expected title, got %q", step.Title)
	}
	if step.Description != "Description here" {
		t.Errorf("expected description, got %q", step.Description)
	}
	if step.Instructions != "Instructions here" {
		t.Errorf("expected instructions, got %q", step.Instructions)
	}
	if step.Assignee != "claude-1" {
		t.Errorf("expected assignee, got %q", step.Assignee)
	}
	if step.Condition != "test -f /tmp/ready" {
		t.Errorf("expected condition, got %q", step.Condition)
	}
	if step.Code != "echo hello" {
		t.Errorf("expected code, got %q", step.Code)
	}
	if step.Timeout != "5m" {
		t.Errorf("expected timeout, got %q", step.Timeout)
	}
	if step.Template != "child-template" {
		t.Errorf("expected template, got %q", step.Template)
	}
	if !step.Ephemeral {
		t.Error("expected ephemeral to be true")
	}
	if len(step.Needs) != 1 || step.Needs[0] != "other" {
		t.Errorf("expected needs ['other'], got %v", step.Needs)
	}
	if step.Variables["key"] != "value" {
		t.Errorf("expected variables, got %v", step.Variables)
	}
}

func TestParseModuleString_WorkflowAllFields(t *testing.T) {
	moduleToml := `
[main]
name = "full-workflow"
description = "A complete workflow"
ephemeral = true
internal = true
hooks_to = "parent_bead"

[main.variables]
task_id = { required = true, type = "string", description = "Task ID", default = "default-id" }

[[main.steps]]
id = "step-1"
type = "task"
`
	module, err := ParseModuleString(moduleToml, "test.toml")
	if err != nil {
		t.Fatalf("ParseModuleString failed: %v", err)
	}

	main := module.GetWorkflow("main")
	if main.Description != "A complete workflow" {
		t.Errorf("expected description, got %q", main.Description)
	}
	if !main.Ephemeral {
		t.Error("expected ephemeral to be true")
	}
	if !main.Internal {
		t.Error("expected internal to be true")
	}
	if main.HooksTo != "parent_bead" {
		t.Errorf("expected hooks_to, got %q", main.HooksTo)
	}

	// Check variable parsing
	v := main.Variables["task_id"]
	if v == nil {
		t.Fatal("expected variable task_id")
	}
	if !v.Required {
		t.Error("expected required to be true")
	}
	if v.Description != "Task ID" {
		t.Errorf("expected description, got %q", v.Description)
	}
	if v.Default != "default-id" {
		t.Errorf("expected default, got %v", v.Default)
	}
}

func TestParseModuleString_CircularDependency(t *testing.T) {
	moduleToml := `
[main]
name = "cycle-test"

[[main.steps]]
id = "a"
type = "task"
needs = ["b"]

[[main.steps]]
id = "b"
type = "task"
needs = ["a"]
`
	_, err := ParseModuleString(moduleToml, "test.toml")
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected circular dependency error, got: %v", err)
	}
}

func TestParseModuleString_SelfReference(t *testing.T) {
	moduleToml := `
[main]
name = "self-ref-test"

[[main.steps]]
id = "self"
type = "task"
needs = ["self"]
`
	_, err := ParseModuleString(moduleToml, "test.toml")
	if err == nil {
		t.Fatal("expected error for self-reference")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected circular dependency error, got: %v", err)
	}
}

func TestParseModuleString_LongerCycle(t *testing.T) {
	moduleToml := `
[main]
name = "long-cycle-test"

[[main.steps]]
id = "a"
type = "task"
needs = ["c"]

[[main.steps]]
id = "b"
type = "task"
needs = ["a"]

[[main.steps]]
id = "c"
type = "task"
needs = ["b"]
`
	_, err := ParseModuleString(moduleToml, "test.toml")
	if err == nil {
		t.Fatal("expected error for longer cycle")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected circular dependency error, got: %v", err)
	}
}

func TestParseModuleString_ConditionBranchTargets(t *testing.T) {
	moduleToml := `
[main]
name = "condition-test"

[[main.steps]]
id = "check"
type = "condition"
condition = "test -f /tmp/ready"
timeout = "30s"

[main.steps.on_true]
template = "do-work"
variables = { x = "42" }

[main.steps.on_false]
template = "wait-more"

[main.steps.on_timeout]
template = "handle-timeout"
variables = { reason = "timed out" }

[[main.steps]]
id = "end"
type = "task"
needs = ["check"]
`
	module, err := ParseModuleString(moduleToml, "test.toml")
	if err != nil {
		t.Fatalf("ParseModuleString failed: %v", err)
	}

	main := module.GetWorkflow("main")
	step := main.Steps[0]

	if step.OnTrue == nil {
		t.Fatal("expected on_true to be parsed")
	}
	if step.OnTrue.Template != "do-work" {
		t.Errorf("expected on_true.template 'do-work', got %q", step.OnTrue.Template)
	}
	if step.OnTrue.Variables["x"] != "42" {
		t.Errorf("expected on_true.variables[x] '42', got %q", step.OnTrue.Variables["x"])
	}

	if step.OnFalse == nil {
		t.Fatal("expected on_false to be parsed")
	}
	if step.OnFalse.Template != "wait-more" {
		t.Errorf("expected on_false.template 'wait-more', got %q", step.OnFalse.Template)
	}

	if step.OnTimeout == nil {
		t.Fatal("expected on_timeout to be parsed")
	}
	if step.OnTimeout.Template != "handle-timeout" {
		t.Errorf("expected on_timeout.template 'handle-timeout', got %q", step.OnTimeout.Template)
	}
	if step.OnTimeout.Variables["reason"] != "timed out" {
		t.Errorf("expected on_timeout.variables[reason] 'timed out', got %q", step.OnTimeout.Variables["reason"])
	}
}

func TestParseModuleString_ConditionWithInlineSteps(t *testing.T) {
	moduleToml := `
[main]
name = "inline-test"

[[main.steps]]
id = "check"
type = "condition"
condition = "test -f /tmp/ready"

[[main.steps.on_true.inline]]
id = "action-1"
type = "task"
description = "First action"

[[main.steps.on_true.inline]]
id = "action-2"
type = "task"
description = "Second action"
needs = ["action-1"]
`
	module, err := ParseModuleString(moduleToml, "test.toml")
	if err != nil {
		t.Fatalf("ParseModuleString failed: %v", err)
	}

	main := module.GetWorkflow("main")
	step := main.Steps[0]

	if step.OnTrue == nil {
		t.Fatal("expected on_true to be parsed")
	}
	if len(step.OnTrue.Inline) != 2 {
		t.Fatalf("expected 2 inline steps, got %d", len(step.OnTrue.Inline))
	}
	if step.OnTrue.Inline[0].ID != "action-1" {
		t.Errorf("expected first inline id 'action-1', got %q", step.OnTrue.Inline[0].ID)
	}
	if step.OnTrue.Inline[1].ID != "action-2" {
		t.Errorf("expected second inline id 'action-2', got %q", step.OnTrue.Inline[1].ID)
	}
	if len(step.OnTrue.Inline[1].Needs) != 1 || step.OnTrue.Inline[1].Needs[0] != "action-1" {
		t.Errorf("expected second inline step to need action-1, got %v", step.OnTrue.Inline[1].Needs)
	}
}

func TestParseModuleString_TaskOutputSpec(t *testing.T) {
	moduleToml := `
[main]
name = "output-test"

[[main.steps]]
id = "select-work"
type = "task"
title = "Select next work bead"
instructions = "Pick a task to work on"

[[main.steps.outputs.required]]
name = "work_bead"
type = "bead_id"
description = "The bead to implement"

[[main.steps.outputs.required]]
name = "reason"
type = "string"
description = "Why this bead was selected"

[[main.steps.outputs.optional]]
name = "notes"
type = "string"
description = "Additional notes"
`
	module, err := ParseModuleString(moduleToml, "test.toml")
	if err != nil {
		t.Fatalf("ParseModuleString failed: %v", err)
	}

	main := module.GetWorkflow("main")
	step := main.Steps[0]

	// This test uses legacy output format (required/optional arrays)
	// which now parses into LegacyOutputs instead of Outputs
	if step.LegacyOutputs == nil {
		t.Fatal("expected legacy outputs to be parsed")
	}
	if len(step.LegacyOutputs.Required) != 2 {
		t.Fatalf("expected 2 required outputs, got %d", len(step.LegacyOutputs.Required))
	}
	if step.LegacyOutputs.Required[0].Name != "work_bead" {
		t.Errorf("expected first required output name 'work_bead', got %q", step.LegacyOutputs.Required[0].Name)
	}
	if step.LegacyOutputs.Required[0].Type != "bead_id" {
		t.Errorf("expected first required output type 'bead_id', got %q", step.LegacyOutputs.Required[0].Type)
	}
	if step.LegacyOutputs.Required[0].Description != "The bead to implement" {
		t.Errorf("expected first required output description, got %q", step.LegacyOutputs.Required[0].Description)
	}
	if step.LegacyOutputs.Required[1].Name != "reason" {
		t.Errorf("expected second required output name 'reason', got %q", step.LegacyOutputs.Required[1].Name)
	}
	if len(step.LegacyOutputs.Optional) != 1 {
		t.Fatalf("expected 1 optional output, got %d", len(step.LegacyOutputs.Optional))
	}
	if step.LegacyOutputs.Optional[0].Name != "notes" {
		t.Errorf("expected optional output name 'notes', got %q", step.LegacyOutputs.Optional[0].Name)
	}
}

func TestBaker_TaskWithOutputSpec(t *testing.T) {
	moduleToml := `
[main]
name = "output-bake-test"

[[main.steps]]
id = "select-work"
type = "task"
title = "Select next work bead"
instructions = "Pick a task to work on"

[[main.steps.outputs.required]]
name = "work_bead"
type = "bead_id"
description = "The bead to implement"

[[main.steps.outputs.optional]]
name = "notes"
type = "string"
`
	module, err := ParseModuleString(moduleToml, "test.toml")
	if err != nil {
		t.Fatalf("ParseModuleString failed: %v", err)
	}

	main := module.GetWorkflow("main")
	baker := NewBaker("meow-output-test")
	baker.Now = func() time.Time { return time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC) }

	result, err := baker.BakeWorkflow(main, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Beads) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(result.Beads))
	}

	bead := result.Beads[0]
	if bead.TaskOutputs == nil {
		t.Fatal("expected TaskOutputs to be set on baked bead")
	}
	if len(bead.TaskOutputs.Required) != 1 {
		t.Fatalf("expected 1 required output, got %d", len(bead.TaskOutputs.Required))
	}
	if bead.TaskOutputs.Required[0].Name != "work_bead" {
		t.Errorf("expected required output name 'work_bead', got %q", bead.TaskOutputs.Required[0].Name)
	}
	if bead.TaskOutputs.Required[0].Type != types.TaskOutputTypeBeadID {
		t.Errorf("expected required output type 'bead_id', got %q", bead.TaskOutputs.Required[0].Type)
	}
	if len(bead.TaskOutputs.Optional) != 1 {
		t.Fatalf("expected 1 optional output, got %d", len(bead.TaskOutputs.Optional))
	}
	if bead.TaskOutputs.Optional[0].Name != "notes" {
		t.Errorf("expected optional output name 'notes', got %q", bead.TaskOutputs.Optional[0].Name)
	}
}
