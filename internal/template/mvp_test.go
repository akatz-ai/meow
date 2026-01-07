package template

import (
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
