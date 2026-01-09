package template

import (
	"strings"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

func fixedTime() time.Time {
	return time.Date(2026, 1, 7, 12, 0, 0, 0, time.UTC)
}

func TestBaker_SimpleTemplate(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test", Version: "1.0.0"},
		Steps: []Step{
			{ID: "step-1", Description: "First step", Instructions: "Do something"},
			{ID: "step-2", Description: "Second step", Instructions: "Do more", Needs: []string{"step-1"}},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	if len(result.Beads) != 2 {
		t.Fatalf("expected 2 beads, got %d", len(result.Beads))
	}

	// Check first bead
	bead1 := result.Beads[0]
	if bead1.Type != types.BeadTypeTask {
		t.Errorf("expected task type, got %s", bead1.Type)
	}
	if bead1.Title != "First step" {
		t.Errorf("expected title 'First step', got %q", bead1.Title)
	}
	if bead1.Status != types.BeadStatusOpen {
		t.Errorf("expected open status, got %s", bead1.Status)
	}
	if len(bead1.Needs) != 0 {
		t.Errorf("expected no dependencies, got %v", bead1.Needs)
	}

	// Check second bead
	bead2 := result.Beads[1]
	if len(bead2.Needs) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(bead2.Needs))
	}
	// Dependency should be the bead ID, not step ID
	if bead2.Needs[0] != result.StepToID["step-1"] {
		t.Errorf("expected dependency on step-1 bead, got %s", bead2.Needs[0])
	}
}

func TestBaker_BeadIDFormat(t *testing.T) {
	tmpl := &Template{
		Meta:  Meta{Name: "test"},
		Steps: []Step{{ID: "my-step", Description: "Test"}},
	}

	baker := NewBaker("meow-run-42")
	baker.Now = fixedTime

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	beadID := result.StepToID["my-step"]
	if !strings.HasPrefix(beadID, "meow-run-42.my-step-") {
		t.Errorf("expected ID format 'meow-run-42.my-step-XXX', got %q", beadID)
	}
	// ID should have hash suffix
	parts := strings.Split(beadID, "-")
	if len(parts) < 3 {
		t.Errorf("expected hash suffix in ID: %q", beadID)
	}
}

func TestBaker_VariableSubstitution(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Variables: map[string]Var{
			"task_name": {Required: true},
		},
		Steps: []Step{
			{ID: "step-1", Description: "Working on {{task_name}}", Instructions: "Implement {{task_name}}"},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime
	baker.VarContext.SetVariable("task_name", "authentication")

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	bead := result.Beads[0]
	if bead.Title != "Working on authentication" {
		t.Errorf("expected substituted title, got %q", bead.Title)
	}
	if bead.Description != "Implement authentication" {
		t.Errorf("expected substituted description, got %q", bead.Description)
	}
}

func TestBaker_MissingRequiredVariable(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Variables: map[string]Var{
			"required_var": {Required: true},
		},
		Steps: []Step{{ID: "step-1", Description: "Test"}},
	}

	baker := NewBaker("wf-001")
	// Don't set the required variable

	_, err := baker.Bake(tmpl)
	if err == nil {
		t.Fatal("expected error for missing required variable")
	}
	if !strings.Contains(err.Error(), "required_var") {
		t.Errorf("expected error to mention required_var: %v", err)
	}
}

func TestBaker_DefaultVariable(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Variables: map[string]Var{
			"framework": {Required: false, Default: "pytest"},
		},
		Steps: []Step{
			{ID: "step-1", Description: "Using {{framework}}"},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	if result.Beads[0].Title != "Using pytest" {
		t.Errorf("expected default value, got %q", result.Beads[0].Title)
	}
}

func TestBaker_ConditionBead(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{
				ID:          "check",
				Description: "Check condition",
				Condition:   "test -f /tmp/flag",
				OnTrue:      &ExpansionTarget{Template: "do-work"},
				OnFalse:     &ExpansionTarget{Template: "skip"},
			},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	bead := result.Beads[0]
	if bead.Type != types.BeadTypeCondition {
		t.Errorf("expected condition type, got %s", bead.Type)
	}
	if bead.ConditionSpec == nil {
		t.Fatal("expected ConditionSpec")
	}
	if bead.ConditionSpec.Condition != "test -f /tmp/flag" {
		t.Errorf("expected condition command, got %q", bead.ConditionSpec.Condition)
	}
	if bead.ConditionSpec.OnTrue == nil || bead.ConditionSpec.OnTrue.Template != "do-work" {
		t.Errorf("expected on_true template")
	}
	if bead.ConditionSpec.OnFalse == nil || bead.ConditionSpec.OnFalse.Template != "skip" {
		t.Errorf("expected on_false template")
	}
}

func TestBaker_ExpandBead(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Variables: map[string]Var{
			"task_id": {Required: true},
		},
		Steps: []Step{
			{
				ID:          "run-impl",
				Description: "Run implementation",
				Template:    "implement",
				Variables:   map[string]string{"target": "{{task_id}}"},
			},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime
	baker.VarContext.SetVariable("task_id", "bd-42")

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	bead := result.Beads[0]
	if bead.Type != types.BeadTypeExpand {
		t.Errorf("expected expand type, got %s", bead.Type)
	}
	if bead.ExpandSpec == nil {
		t.Fatal("expected ExpandSpec")
	}
	if bead.ExpandSpec.Template != "implement" {
		t.Errorf("expected template 'implement', got %q", bead.ExpandSpec.Template)
	}
	if bead.ExpandSpec.Variables["target"] != "bd-42" {
		t.Errorf("expected substituted variable, got %q", bead.ExpandSpec.Variables["target"])
	}
}

func TestBaker_GateStep(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{
				ID:           "gate",
				Description:  "Wait for approval",
				Type:         "gate",
				Instructions: "Review and approve",
			},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	bead := result.Beads[0]
	if bead.Type != types.BeadTypeGate {
		t.Errorf("expected gate type, got %s", bead.Type)
	}
	// Gate beads don't need a spec - they're closed by humans
	if bead.ConditionSpec != nil {
		t.Errorf("gate should not have ConditionSpec")
	}
}

func TestBaker_EphemeralStep(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "temp", Description: "Temporary step", Ephemeral: true},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	bead := result.Beads[0]
	if !bead.IsEphemeral() {
		t.Error("expected bead to be ephemeral")
	}
}

func TestBaker_Assignee(t *testing.T) {
	tmpl := &Template{
		Meta:  Meta{Name: "test"},
		Steps: []Step{{ID: "step-1", Description: "Test"}},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime
	baker.Assignee = "claude-1"

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	if result.Beads[0].Assignee != "claude-1" {
		t.Errorf("expected assignee 'claude-1', got %q", result.Beads[0].Assignee)
	}
}

func TestBaker_ParentBead(t *testing.T) {
	tmpl := &Template{
		Meta:  Meta{Name: "test"},
		Steps: []Step{{ID: "step-1", Description: "Test"}},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime
	baker.ParentBead = "parent-bead-123"

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	if result.Beads[0].Parent != "parent-bead-123" {
		t.Errorf("expected parent 'parent-bead-123', got %q", result.Beads[0].Parent)
	}
}

func TestBaker_ValidationError(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{}, // Missing name
		Steps: []Step{
			{ID: "a", Needs: []string{"b"}},
			{ID: "b", Needs: []string{"a"}},
		},
	}

	baker := NewBaker("wf-001")

	_, err := baker.Bake(tmpl)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestBaker_TopologicalOrder(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "c", Description: "Third", Needs: []string{"a", "b"}},
			{ID: "a", Description: "First"},
			{ID: "b", Description: "Second", Needs: []string{"a"}},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	// Result should be in topological order
	idxOf := func(id string) int {
		for i, b := range result.Beads {
			if strings.Contains(b.ID, id) {
				return i
			}
		}
		return -1
	}

	aIdx := idxOf("a")
	bIdx := idxOf("b")
	cIdx := idxOf("c")

	if aIdx > bIdx {
		t.Errorf("a should come before b")
	}
	if aIdx > cIdx {
		t.Errorf("a should come before c")
	}
	if bIdx > cIdx {
		t.Errorf("b should come before c")
	}
}

func TestBaker_BakeInline(t *testing.T) {
	steps := []InlineStep{
		{ID: "inline-1", Type: "task", Description: "First inline"},
		{ID: "inline-2", Type: "task", Description: "Second inline", Needs: []string{"inline-1"}},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	beads, err := baker.BakeInline(steps, "parent-123")
	if err != nil {
		t.Fatalf("BakeInline failed: %v", err)
	}

	if len(beads) != 2 {
		t.Fatalf("expected 2 beads, got %d", len(beads))
	}

	// First inline bead should depend on parent
	if len(beads[0].Needs) == 0 || beads[0].Needs[0] != "parent-123" {
		t.Errorf("expected first bead to depend on parent")
	}

	// All beads should have parent set
	for _, b := range beads {
		if b.Parent != "parent-123" {
			t.Errorf("expected parent 'parent-123', got %q", b.Parent)
		}
	}
}

func TestBaker_BuiltinVariables(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{ID: "step-1", Description: "Workflow: {{workflow_id}}", Instructions: "Bead: {{bead_id}}"},
		},
	}

	baker := NewBaker("wf-test-42")
	baker.Now = fixedTime

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	bead := result.Beads[0]
	if bead.Title != "Workflow: wf-test-42" {
		t.Errorf("expected workflow_id substitution, got %q", bead.Title)
	}
	// bead_id is substituted during step processing
	if !strings.Contains(bead.Description, "wf-test-42") {
		t.Errorf("expected bead_id substitution, got %q", bead.Description)
	}
}

func TestBaker_BakeInline_VariableSubstitution(t *testing.T) {
	steps := []InlineStep{
		{ID: "inline-1", Type: "task", Description: "Task for {{task_name}}", Instructions: "Implement {{task_name}}"},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime
	baker.VarContext.SetVariable("task_name", "authentication")

	beads, err := baker.BakeInline(steps, "parent-123")
	if err != nil {
		t.Fatalf("BakeInline failed: %v", err)
	}

	if len(beads) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(beads))
	}

	bead := beads[0]
	if bead.Title != "Task for authentication" {
		t.Errorf("expected variable substitution in title, got %q", bead.Title)
	}
	if bead.Description != "Implement authentication" {
		t.Errorf("expected variable substitution in description, got %q", bead.Description)
	}
}

func TestBaker_BakeInline_UndefinedVariable(t *testing.T) {
	steps := []InlineStep{
		{ID: "inline-1", Type: "task", Description: "Task for {{undefined_var}}"},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	_, err := baker.BakeInline(steps, "parent-123")
	if err == nil {
		t.Fatal("expected error for undefined variable in inline step")
	}
	if !strings.Contains(err.Error(), "undefined") {
		t.Errorf("expected undefined variable error, got: %v", err)
	}
}

func TestBaker_ConditionWithInlineSteps(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{
				ID:          "check",
				Description: "Check condition",
				Condition:   "test -f /tmp/flag",
				OnTrue: &ExpansionTarget{
					Inline: []InlineStep{
						{ID: "action-1", Type: "task", Description: "Do action"},
					},
				},
			},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	bead := result.Beads[0]
	if bead.ConditionSpec == nil {
		t.Fatal("expected ConditionSpec")
	}
	if bead.ConditionSpec.OnTrue == nil {
		t.Fatal("expected OnTrue")
	}
	// Inline steps should be serialized
	if len(bead.ConditionSpec.OnTrue.Inline) != 1 {
		t.Errorf("expected 1 inline step, got %d", len(bead.ConditionSpec.OnTrue.Inline))
	}
}

// Tests for BakeWorkflow with legacy type field (now produces Steps)
func TestBaker_BakeWorkflow_CodeStep(t *testing.T) {
	workflow := &Workflow{
		Name: "code-test",
		Steps: []*Step{
			{
				ID:   "run-code",
				Type: "code",
				Code: "echo 'hello world'",
			},
		},
	}

	baker := NewBaker("wf-code-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	// Legacy "code" type maps to ExecutorShell
	if step.Executor != types.ExecutorShell {
		t.Errorf("expected shell executor, got %s", step.Executor)
	}
	if step.Shell == nil {
		t.Fatal("expected ShellConfig")
	}
	if step.Shell.Command != "echo 'hello world'" {
		t.Errorf("expected command, got %q", step.Shell.Command)
	}
}

func TestBaker_BakeWorkflow_StartStopSteps(t *testing.T) {
	workflow := &Workflow{
		Name: "agent-control",
		Steps: []*Step{
			{
				ID:       "start-agent",
				Type:     "start",
				Assignee: "claude-worker",
			},
			{
				ID:       "stop-agent",
				Type:     "stop",
				Assignee: "claude-worker",
				Needs:    []string{"start-agent"},
			},
		},
	}

	baker := NewBaker("wf-agent-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}

	// Check start step
	startStep := result.Steps[0]
	// Legacy "start" type maps to ExecutorSpawn
	if startStep.Executor != types.ExecutorSpawn {
		t.Errorf("expected spawn executor, got %s", startStep.Executor)
	}
	if startStep.Spawn == nil {
		t.Fatal("expected SpawnConfig")
	}
	if startStep.Spawn.Agent != "claude-worker" {
		t.Errorf("expected agent 'claude-worker', got %q", startStep.Spawn.Agent)
	}

	// Check stop step
	stopStep := result.Steps[1]
	// Legacy "stop" type maps to ExecutorKill
	if stopStep.Executor != types.ExecutorKill {
		t.Errorf("expected kill executor, got %s", stopStep.Executor)
	}
	if stopStep.Kill == nil {
		t.Fatal("expected KillConfig")
	}
	if stopStep.Kill.Agent != "claude-worker" {
		t.Errorf("expected agent 'claude-worker', got %q", stopStep.Kill.Agent)
	}
}

func TestBaker_BakeWorkflow_ExpandStep(t *testing.T) {
	workflow := &Workflow{
		Name: "expand-test",
		Steps: []*Step{
			{
				ID:       "expand-impl",
				Type:     "expand",
				Template: "implement",
				Variables: map[string]string{
					"task": "bd-42",
				},
			},
		},
	}

	baker := NewBaker("wf-expand-001")
	baker.Now = fixedTime
	baker.Assignee = "default-agent"

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	// Legacy "expand" type maps to ExecutorExpand
	if step.Executor != types.ExecutorExpand {
		t.Errorf("expected expand executor, got %s", step.Executor)
	}
	if step.Expand == nil {
		t.Fatal("expected ExpandConfig")
	}
	if step.Expand.Template != "implement" {
		t.Errorf("expected template 'implement', got %q", step.Expand.Template)
	}
	if step.Expand.Variables["task"] != "bd-42" {
		t.Errorf("expected variable, got %v", step.Expand.Variables)
	}
	// Note: Assignee is no longer stored on ExpandConfig in new model
}

func TestBaker_BakeWorkflow_ConditionStep(t *testing.T) {
	workflow := &Workflow{
		Name: "condition-test",
		Steps: []*Step{
			{
				ID:        "check",
				Type:      "condition",
				Condition: "test -f /tmp/ready",
				OnTrue:    &ExpansionTarget{Template: "proceed"},
				OnFalse:   &ExpansionTarget{Template: "wait"},
				Timeout:   "5m",
			},
		},
	}

	baker := NewBaker("wf-cond-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	// Legacy "condition" type maps to ExecutorBranch
	if step.Executor != types.ExecutorBranch {
		t.Errorf("expected branch executor, got %s", step.Executor)
	}
	if step.Branch == nil {
		t.Fatal("expected BranchConfig")
	}
	if step.Branch.Condition != "test -f /tmp/ready" {
		t.Errorf("expected condition, got %q", step.Branch.Condition)
	}
	if step.Branch.Timeout != "5m" {
		t.Errorf("expected timeout '5m', got %q", step.Branch.Timeout)
	}
	if step.Branch.OnTrue == nil || step.Branch.OnTrue.Template != "proceed" {
		t.Errorf("expected on_true template")
	}
	if step.Branch.OnFalse == nil || step.Branch.OnFalse.Template != "wait" {
		t.Errorf("expected on_false template")
	}
}

func TestBaker_BakeWorkflow_NilWorkflow(t *testing.T) {
	baker := NewBaker("wf-nil-001")
	_, err := baker.BakeWorkflow(nil, nil)
	if err == nil {
		t.Fatal("expected error for nil workflow")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Errorf("expected nil error, got: %v", err)
	}
}

func TestBaker_BakeWorkflow_MissingRequiredVariable(t *testing.T) {
	workflow := &Workflow{
		Name: "required-var-test",
		Variables: map[string]*Var{
			"task_id": {Required: true},
		},
		Steps: []*Step{
			{ID: "step-1", Type: "task"},
		},
	}

	baker := NewBaker("wf-var-001")
	baker.Now = fixedTime

	_, err := baker.BakeWorkflow(workflow, nil)
	if err == nil {
		t.Fatal("expected error for missing required variable")
	}
	if !strings.Contains(err.Error(), "task_id") {
		t.Errorf("expected error about task_id, got: %v", err)
	}
}

func TestBaker_BakeWorkflow_DefaultVariable(t *testing.T) {
	workflow := &Workflow{
		Name: "default-var-test",
		Variables: map[string]*Var{
			"framework": {Default: "pytest"},
		},
		Steps: []*Step{
			{ID: "step-1", Type: "task", Prompt: "Using {{framework}}"},
		},
	}

	baker := NewBaker("wf-default-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Agent == nil {
		t.Fatal("expected AgentConfig")
	}
	// Variable substitution should work in Prompt
	if step.Agent.Prompt != "Using pytest" {
		t.Errorf("expected default variable substitution, got %q", step.Agent.Prompt)
	}
}

func TestBaker_BakeWorkflow_VariableOverride(t *testing.T) {
	workflow := &Workflow{
		Name: "var-override-test",
		Variables: map[string]*Var{
			"framework": {Default: "pytest"},
		},
		Steps: []*Step{
			{ID: "step-1", Type: "task", Prompt: "Using {{framework}}"},
		},
	}

	baker := NewBaker("wf-override-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, map[string]string{
		"framework": "jest",
	})
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Agent == nil {
		t.Fatal("expected AgentConfig")
	}
	if step.Agent.Prompt != "Using jest" {
		t.Errorf("expected override variable, got %q", step.Agent.Prompt)
	}
}

// NOTE: Ephemeral is a legacy bead concept - Steps don't track this.
// This test verifies steps are created correctly even with ephemeral flag set.
func TestBaker_BakeWorkflow_EphemeralStep(t *testing.T) {
	workflow := &Workflow{
		Name: "ephemeral-step-test",
		Steps: []*Step{
			{ID: "temp", Type: "task", Ephemeral: true},
		},
	}

	baker := NewBaker("wf-eph-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	// Step should be created - ephemeral flag is ignored in new model
	step := result.Steps[0]
	if step.Executor != types.ExecutorAgent {
		t.Errorf("expected agent executor, got %s", step.Executor)
	}
}

// NOTE: Dependency validation is not done during baking in new model.
// Dependencies are preserved as-is; validation happens at workflow load time.
func TestBaker_BakeWorkflow_UnknownDependency(t *testing.T) {
	workflow := &Workflow{
		Name: "bad-dep-test",
		Steps: []*Step{
			{ID: "step-1", Type: "task", Needs: []string{"nonexistent"}},
		},
	}

	baker := NewBaker("wf-bad-dep-001")
	baker.Now = fixedTime

	// In new model, baking doesn't validate dependencies - they're preserved as-is
	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow should not fail for unknown deps: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	// Dependency should be preserved
	if len(result.Steps[0].Needs) != 1 || result.Steps[0].Needs[0] != "nonexistent" {
		t.Errorf("expected dependency to be preserved, got %v", result.Steps[0].Needs)
	}
}

func TestBaker_determineBeadTypeFromString_AllTypes(t *testing.T) {
	tests := []struct {
		stepType string
		expected types.BeadType
	}{
		{"task", types.BeadTypeTask},
		{"", types.BeadTypeTask},
		{"collaborative", types.BeadTypeCollaborative},
		{"gate", types.BeadTypeGate},
		{"condition", types.BeadTypeCondition},
		{"start", types.BeadTypeStart},
		{"stop", types.BeadTypeStop},
		{"code", types.BeadTypeCode},
		{"expand", types.BeadTypeExpand},
		{"unknown", types.BeadTypeTask}, // Unknown defaults to task
	}

	baker := NewBaker("test")
	for _, tt := range tests {
		t.Run(tt.stepType, func(t *testing.T) {
			result := baker.determineBeadTypeFromString(tt.stepType)
			if result != tt.expected {
				t.Errorf("expected %s for stepType %q, got %s", tt.expected, tt.stepType, result)
			}
		})
	}
}

func TestBaker_BakeInline_InvalidType(t *testing.T) {
	steps := []InlineStep{
		{ID: "inline-1", Type: "invalid_type_xyz", Description: "Test"},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	beads, err := baker.BakeInline(steps, "parent-123")
	if err != nil {
		t.Fatalf("BakeInline failed: %v", err)
	}

	// Invalid type should default to task
	if beads[0].Type != types.BeadTypeTask {
		t.Errorf("expected task type for invalid, got %s", beads[0].Type)
	}
}

func TestBaker_BakeInline_ExternalDependency(t *testing.T) {
	steps := []InlineStep{
		{ID: "inline-1", Type: "task", Description: "Test", Needs: []string{"external-bead"}},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	beads, err := baker.BakeInline(steps, "parent-123")
	if err != nil {
		t.Fatalf("BakeInline failed: %v", err)
	}

	// External dependency should be preserved as-is
	hasExternal := false
	for _, need := range beads[0].Needs {
		if need == "external-bead" {
			hasExternal = true
			break
		}
	}
	if !hasExternal {
		t.Errorf("expected external dependency to be preserved: %v", beads[0].Needs)
	}
}

func TestBaker_ConditionWithOnTimeout(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{
				ID:          "check",
				Description: "Check with timeout",
				Condition:   "test -f /tmp/ready",
				OnTrue:      &ExpansionTarget{Template: "proceed"},
				OnFalse:     &ExpansionTarget{Template: "wait"},
				OnTimeout:   &ExpansionTarget{Template: "timeout-handler"},
				Timeout:     "5m",
			},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	result, err := baker.Bake(tmpl)
	if err != nil {
		t.Fatalf("Bake failed: %v", err)
	}

	bead := result.Beads[0]
	if bead.ConditionSpec == nil {
		t.Fatal("expected ConditionSpec")
	}
	if bead.ConditionSpec.OnTimeout == nil {
		t.Fatal("expected OnTimeout")
	}
	if bead.ConditionSpec.OnTimeout.Template != "timeout-handler" {
		t.Errorf("expected on_timeout template, got %q", bead.ConditionSpec.OnTimeout.Template)
	}
}

// Test for meow-5tm: Start/Stop specs should use substituted assignee, not raw template value
func TestBaker_BakeWorkflow_StartStopWithVariableAssignee(t *testing.T) {
	workflow := &Workflow{
		Name: "agent-control-var",
		Variables: map[string]*Var{
			"agent": {Required: true},
		},
		Steps: []*Step{
			{
				ID:       "start-agent",
				Type:     "start",
				Assignee: "{{agent}}",
			},
			{
				ID:       "stop-agent",
				Type:     "stop",
				Assignee: "{{agent}}",
				Needs:    []string{"start-agent"},
			},
		},
	}

	baker := NewBaker("wf-agent-var-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, map[string]string{
		"agent": "claude-worker-42",
	})
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}

	// Check start step - agent should be substituted value, not raw "{{agent}}"
	startStep := result.Steps[0]
	if startStep.Spawn == nil {
		t.Fatal("expected SpawnConfig")
	}
	if startStep.Spawn.Agent != "claude-worker-42" {
		t.Errorf("expected substituted agent 'claude-worker-42', got %q", startStep.Spawn.Agent)
	}

	// Check stop step - agent should be substituted value, not raw "{{agent}}"
	stopStep := result.Steps[1]
	if stopStep.Kill == nil {
		t.Fatal("expected KillConfig")
	}
	if stopStep.Kill.Agent != "claude-worker-42" {
		t.Errorf("expected substituted agent 'claude-worker-42', got %q", stopStep.Kill.Agent)
	}
}

// Test for meow-a28: BakeInline should set Tier and Instructions fields
func TestBaker_BakeInline_TierAndInstructions(t *testing.T) {
	steps := []InlineStep{
		{ID: "inline-1", Type: "task", Description: "First inline", Instructions: "Do this first"},
		{ID: "inline-2", Type: "task", Description: "Second inline", Instructions: "Do this second", Needs: []string{"inline-1"}},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	beads, err := baker.BakeInline(steps, "parent-123")
	if err != nil {
		t.Fatalf("BakeInline failed: %v", err)
	}

	if len(beads) != 2 {
		t.Fatalf("expected 2 beads, got %d", len(beads))
	}

	// All inline beads should have Tier set to wisp
	for i, bead := range beads {
		if bead.Tier != types.TierWisp {
			t.Errorf("bead %d: expected tier 'wisp', got %q", i, bead.Tier)
		}
	}

	// Check Instructions field is set
	if beads[0].Instructions != "Do this first" {
		t.Errorf("expected instructions 'Do this first', got %q", beads[0].Instructions)
	}
	if beads[1].Instructions != "Do this second" {
		t.Errorf("expected instructions 'Do this second', got %q", beads[1].Instructions)
	}
}

// Test for meow-zva: Code field should be substituted in BakeWorkflow
func TestBaker_BakeWorkflow_CodeWithVariable(t *testing.T) {
	workflow := &Workflow{
		Name: "code-var-test",
		Variables: map[string]*Var{
			"test_name": {Required: true},
		},
		Steps: []*Step{
			{
				ID:   "run-code",
				Type: "code",
				Code: "echo 'Running test: {{test_name}}'",
			},
		},
	}

	baker := NewBaker("wf-code-var-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, map[string]string{
		"test_name": "auth-unit-tests",
	})
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	// Legacy "code" type maps to ExecutorShell
	if step.Executor != types.ExecutorShell {
		t.Errorf("expected shell executor, got %s", step.Executor)
	}
	if step.Shell == nil {
		t.Fatal("expected ShellConfig")
	}
	// Code should be substituted, not contain raw variable reference
	if step.Shell.Command != "echo 'Running test: auth-unit-tests'" {
		t.Errorf("expected substituted code, got %q", step.Shell.Command)
	}
}

// Test for meow-zva: Condition field should be substituted in BakeWorkflow
func TestBaker_BakeWorkflow_ConditionWithVariable(t *testing.T) {
	workflow := &Workflow{
		Name: "cond-var-test",
		Variables: map[string]*Var{
			"file_path": {Required: true},
		},
		Steps: []*Step{
			{
				ID:        "check",
				Type:      "condition",
				Condition: "test -f {{file_path}}",
				OnTrue:    &ExpansionTarget{Template: "proceed"},
				OnFalse:   &ExpansionTarget{Template: "wait"},
			},
		},
	}

	baker := NewBaker("wf-cond-var-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, map[string]string{
		"file_path": "/tmp/ready.flag",
	})
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Branch == nil {
		t.Fatal("expected BranchConfig")
	}
	// Condition should be substituted, not contain raw variable reference
	if step.Branch.Condition != "test -f /tmp/ready.flag" {
		t.Errorf("expected substituted condition, got %q", step.Branch.Condition)
	}
}

// Test for meow-eej: Inline steps should preserve Title and Code fields
func TestBaker_BakeInline_WithTitleAndCode(t *testing.T) {
	steps := []InlineStep{
		{
			ID:          "code-step",
			Type:        "code",
			Title:       "Run verification script",
			Description: "Run the verification",
			Code:        "echo 'verifying {{item}}'",
		},
	}

	baker := NewBaker("wf-inline-001")
	baker.Now = fixedTime
	baker.VarContext.SetVariable("item", "authentication")

	beads, err := baker.BakeInline(steps, "parent-123")
	if err != nil {
		t.Fatalf("BakeInline failed: %v", err)
	}

	if len(beads) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(beads))
	}

	bead := beads[0]

	// Title should be preserved (not defaulting to Description)
	if bead.Title != "Run verification script" {
		t.Errorf("expected title 'Run verification script', got %q", bead.Title)
	}

	// Type should be code
	if bead.Type != types.BeadTypeCode {
		t.Errorf("expected code type, got %s", bead.Type)
	}

	// CodeSpec should be set with substituted value
	if bead.CodeSpec == nil {
		t.Fatal("expected CodeSpec")
	}
	if bead.CodeSpec.Code != "echo 'verifying authentication'" {
		t.Errorf("expected substituted code, got %q", bead.CodeSpec.Code)
	}
}

// Test for meow-eej: Inline steps should handle condition type
func TestBaker_BakeInline_WithCondition(t *testing.T) {
	steps := []InlineStep{
		{
			ID:        "check-ready",
			Type:      "condition",
			Title:     "Check if ready",
			Condition: "test -f {{ready_file}}",
			OnTrue:    &ExpansionTarget{Template: "proceed"},
			OnFalse:   &ExpansionTarget{Template: "wait"},
		},
	}

	baker := NewBaker("wf-inline-cond-001")
	baker.Now = fixedTime
	baker.VarContext.SetVariable("ready_file", "/tmp/ready.flag")

	beads, err := baker.BakeInline(steps, "parent-123")
	if err != nil {
		t.Fatalf("BakeInline failed: %v", err)
	}

	if len(beads) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(beads))
	}

	bead := beads[0]

	// Type should be condition
	if bead.Type != types.BeadTypeCondition {
		t.Errorf("expected condition type, got %s", bead.Type)
	}

	// ConditionSpec should be set with substituted value
	if bead.ConditionSpec == nil {
		t.Fatal("expected ConditionSpec")
	}
	if bead.ConditionSpec.Condition != "test -f /tmp/ready.flag" {
		t.Errorf("expected substituted condition, got %q", bead.ConditionSpec.Condition)
	}
	if bead.ConditionSpec.OnTrue == nil || bead.ConditionSpec.OnTrue.Template != "proceed" {
		t.Errorf("expected on_true template 'proceed'")
	}
}

// ============================================================================
// NEW TESTS FOR STEP-BASED BAKER (pivot-303)
// These tests define the expected behavior for the refactored baker that
// produces types.Step instead of types.Bead.
// ============================================================================

// TestBakeWorkflow_ReturnsSteps verifies that BakeWorkflow now returns Steps
func TestBakeWorkflow_ReturnsSteps(t *testing.T) {
	workflow := &Workflow{
		Name: "test-workflow",
		Steps: []*Step{
			{
				ID:       "task-1",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Do something useful",
			},
		},
	}

	baker := NewBaker("wf-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	// Result should have Steps, not Beads
	if result.Steps == nil {
		t.Fatal("expected Steps in result, got nil")
	}
	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorAgent {
		t.Errorf("expected agent executor, got %s", step.Executor)
	}
	if step.Status != types.StepStatusPending {
		t.Errorf("expected pending status, got %s", step.Status)
	}
}

// TestBakeWorkflow_ShellExecutor tests shell executor step creation
func TestBakeWorkflow_ShellExecutor(t *testing.T) {
	workflow := &Workflow{
		Name: "shell-test",
		Steps: []*Step{
			{
				ID:       "run-cmd",
				Executor: ExecutorShell,
				Command:  "echo 'hello world'",
				Workdir:  "/tmp",
				Env:      map[string]string{"FOO": "bar"},
				OnError:  "continue",
			},
		},
	}

	baker := NewBaker("wf-shell-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.ID != "run-cmd" {
		t.Errorf("expected step ID 'run-cmd', got %q", step.ID)
	}
	if step.Executor != types.ExecutorShell {
		t.Errorf("expected shell executor, got %s", step.Executor)
	}
	if step.Shell == nil {
		t.Fatal("expected ShellConfig, got nil")
	}
	if step.Shell.Command != "echo 'hello world'" {
		t.Errorf("expected command, got %q", step.Shell.Command)
	}
	if step.Shell.Workdir != "/tmp" {
		t.Errorf("expected workdir '/tmp', got %q", step.Shell.Workdir)
	}
	if step.Shell.Env["FOO"] != "bar" {
		t.Errorf("expected env FOO=bar, got %v", step.Shell.Env)
	}
	if step.Shell.OnError != "continue" {
		t.Errorf("expected on_error 'continue', got %q", step.Shell.OnError)
	}
}

// TestBakeWorkflow_SpawnExecutor tests spawn executor step creation
func TestBakeWorkflow_SpawnExecutor(t *testing.T) {
	workflow := &Workflow{
		Name: "spawn-test",
		Steps: []*Step{
			{
				ID:            "start-agent",
				Executor:      ExecutorSpawn,
				Agent:         "claude-worker",
				Workdir:       "/project",
				Env:           map[string]string{"MEOW_WORKFLOW": "test"},
				ResumeSession: "session-123",
			},
		},
	}

	baker := NewBaker("wf-spawn-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorSpawn {
		t.Errorf("expected spawn executor, got %s", step.Executor)
	}
	if step.Spawn == nil {
		t.Fatal("expected SpawnConfig, got nil")
	}
	if step.Spawn.Agent != "claude-worker" {
		t.Errorf("expected agent 'claude-worker', got %q", step.Spawn.Agent)
	}
	if step.Spawn.Workdir != "/project" {
		t.Errorf("expected workdir '/project', got %q", step.Spawn.Workdir)
	}
	if step.Spawn.ResumeSession != "session-123" {
		t.Errorf("expected resume_session 'session-123', got %q", step.Spawn.ResumeSession)
	}
}

// TestBakeWorkflow_KillExecutor tests kill executor step creation
func TestBakeWorkflow_KillExecutor(t *testing.T) {
	graceful := true
	workflow := &Workflow{
		Name: "kill-test",
		Steps: []*Step{
			{
				ID:       "stop-agent",
				Executor: ExecutorKill,
				Agent:    "claude-worker",
				Graceful: &graceful,
				Timeout:  "30s",
			},
		},
	}

	baker := NewBaker("wf-kill-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorKill {
		t.Errorf("expected kill executor, got %s", step.Executor)
	}
	if step.Kill == nil {
		t.Fatal("expected KillConfig, got nil")
	}
	if step.Kill.Agent != "claude-worker" {
		t.Errorf("expected agent 'claude-worker', got %q", step.Kill.Agent)
	}
	if !step.Kill.Graceful {
		t.Error("expected graceful=true")
	}
}

// TestBakeWorkflow_ExpandExecutor tests expand executor step creation
func TestBakeWorkflow_ExpandExecutor(t *testing.T) {
	workflow := &Workflow{
		Name: "expand-test",
		Steps: []*Step{
			{
				ID:        "do-expand",
				Executor:  ExecutorExpand,
				Template:  "sub-workflow",
				Variables: map[string]string{"task": "build"},
			},
		},
	}

	baker := NewBaker("wf-expand-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorExpand {
		t.Errorf("expected expand executor, got %s", step.Executor)
	}
	if step.Expand == nil {
		t.Fatal("expected ExpandConfig, got nil")
	}
	if step.Expand.Template != "sub-workflow" {
		t.Errorf("expected template 'sub-workflow', got %q", step.Expand.Template)
	}
	if step.Expand.Variables["task"] != "build" {
		t.Errorf("expected variable task=build, got %v", step.Expand.Variables)
	}
}

// TestBakeWorkflow_BranchExecutor tests branch executor step creation
func TestBakeWorkflow_BranchExecutor(t *testing.T) {
	workflow := &Workflow{
		Name: "branch-test",
		Steps: []*Step{
			{
				ID:        "check-flag",
				Executor:  ExecutorBranch,
				Condition: "test -f /tmp/ready",
				OnTrue:    &ExpansionTarget{Template: "proceed"},
				OnFalse:   &ExpansionTarget{Template: "wait"},
				Timeout:   "5m",
			},
		},
	}

	baker := NewBaker("wf-branch-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorBranch {
		t.Errorf("expected branch executor, got %s", step.Executor)
	}
	if step.Branch == nil {
		t.Fatal("expected BranchConfig, got nil")
	}
	if step.Branch.Condition != "test -f /tmp/ready" {
		t.Errorf("expected condition, got %q", step.Branch.Condition)
	}
	if step.Branch.OnTrue == nil || step.Branch.OnTrue.Template != "proceed" {
		t.Errorf("expected on_true template 'proceed'")
	}
	if step.Branch.OnFalse == nil || step.Branch.OnFalse.Template != "wait" {
		t.Errorf("expected on_false template 'wait'")
	}
	if step.Branch.Timeout != "5m" {
		t.Errorf("expected timeout '5m', got %q", step.Branch.Timeout)
	}
}

// TestBakeWorkflow_AgentExecutor tests agent executor step creation
func TestBakeWorkflow_AgentExecutor(t *testing.T) {
	workflow := &Workflow{
		Name: "agent-test",
		Steps: []*Step{
			{
				ID:       "do-work",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Implement the feature",
				Mode:     "autonomous",
				Timeout:  "1h",
				Outputs: map[string]AgentOutputDef{
					"result": {Required: true, Type: "string", Description: "The result"},
				},
			},
		},
	}

	baker := NewBaker("wf-agent-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	step := result.Steps[0]
	if step.Executor != types.ExecutorAgent {
		t.Errorf("expected agent executor, got %s", step.Executor)
	}
	if step.Agent == nil {
		t.Fatal("expected AgentConfig, got nil")
	}
	if step.Agent.Agent != "claude" {
		t.Errorf("expected agent 'claude', got %q", step.Agent.Agent)
	}
	if step.Agent.Prompt != "Implement the feature" {
		t.Errorf("expected prompt, got %q", step.Agent.Prompt)
	}
	if step.Agent.Mode != "autonomous" {
		t.Errorf("expected mode 'autonomous', got %q", step.Agent.Mode)
	}
	if step.Agent.Timeout != "1h" {
		t.Errorf("expected timeout '1h', got %q", step.Agent.Timeout)
	}
	if step.Agent.Outputs == nil {
		t.Fatal("expected outputs")
	}
	if step.Agent.Outputs["result"].Required != true {
		t.Error("expected result output to be required")
	}
}

// TestBakeWorkflow_Dependencies tests that step dependencies are preserved
func TestBakeWorkflow_Dependencies(t *testing.T) {
	workflow := &Workflow{
		Name: "deps-test",
		Steps: []*Step{
			{
				ID:       "first",
				Executor: ExecutorShell,
				Command:  "echo first",
			},
			{
				ID:       "second",
				Executor: ExecutorShell,
				Command:  "echo second",
				Needs:    []string{"first"},
			},
			{
				ID:       "third",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Do third",
				Needs:    []string{"first", "second"},
			},
		},
	}

	baker := NewBaker("wf-deps-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 3 {
		t.Fatalf("expected 3 steps, got %d", len(result.Steps))
	}

	// Find steps by ID
	stepByID := make(map[string]*types.Step)
	for _, s := range result.Steps {
		stepByID[s.ID] = s
	}

	first := stepByID["first"]
	second := stepByID["second"]
	third := stepByID["third"]

	if first == nil || second == nil || third == nil {
		t.Fatal("missing expected steps")
	}

	if len(first.Needs) != 0 {
		t.Errorf("first should have no deps, got %v", first.Needs)
	}
	if len(second.Needs) != 1 || second.Needs[0] != "first" {
		t.Errorf("second should depend on first, got %v", second.Needs)
	}
	if len(third.Needs) != 2 {
		t.Errorf("third should have 2 deps, got %v", third.Needs)
	}
}

// TestBakeWorkflow_VariableSubstitution tests variable substitution in step fields
func TestBakeWorkflow_VariableSubstitution(t *testing.T) {
	workflow := &Workflow{
		Name: "var-test",
		Variables: map[string]*Var{
			"target": {Required: true},
			"agent":  {Default: "claude"},
		},
		Steps: []*Step{
			{
				ID:       "task",
				Executor: ExecutorAgent,
				Agent:    "{{agent}}",
				Prompt:   "Work on {{target}}",
			},
			{
				ID:       "check",
				Executor: ExecutorShell,
				Command:  "test -f {{target}}.done",
			},
		},
	}

	baker := NewBaker("wf-var-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, map[string]string{
		"target": "feature-x",
	})
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(result.Steps))
	}

	// Find the agent step
	var agentStep *types.Step
	for _, s := range result.Steps {
		if s.ID == "task" {
			agentStep = s
			break
		}
	}

	if agentStep == nil {
		t.Fatal("agent step not found")
	}
	if agentStep.Agent.Agent != "claude" {
		t.Errorf("expected agent 'claude', got %q", agentStep.Agent.Agent)
	}
	if agentStep.Agent.Prompt != "Work on feature-x" {
		t.Errorf("expected substituted prompt, got %q", agentStep.Agent.Prompt)
	}

	// Find the shell step
	var shellStep *types.Step
	for _, s := range result.Steps {
		if s.ID == "check" {
			shellStep = s
			break
		}
	}

	if shellStep == nil {
		t.Fatal("shell step not found")
	}
	if shellStep.Shell.Command != "test -f feature-x.done" {
		t.Errorf("expected substituted command, got %q", shellStep.Shell.Command)
	}
}

// TestBakeWorkflow_NoBeads verifies the old Beads field is no longer used
func TestBakeWorkflow_NoBeads(t *testing.T) {
	workflow := &Workflow{
		Name: "no-beads-test",
		Steps: []*Step{
			{
				ID:       "task",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Do work",
			},
		},
	}

	baker := NewBaker("wf-no-beads-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	// Beads should be nil or empty (no longer populated)
	if len(result.Beads) != 0 {
		t.Errorf("expected no Beads, got %d", len(result.Beads))
	}

	// Steps should be populated instead
	if len(result.Steps) != 1 {
		t.Errorf("expected 1 Step, got %d", len(result.Steps))
	}
}

// TestBakeWorkflow_LegacyTypeMapping tests that legacy type field maps to executor
func TestBakeWorkflow_LegacyTypeMapping(t *testing.T) {
	tests := []struct {
		legacyType       string
		expectedExecutor types.ExecutorType
	}{
		{"task", types.ExecutorAgent},
		{"collaborative", types.ExecutorAgent},
		{"code", types.ExecutorShell},
		{"condition", types.ExecutorBranch},
		{"start", types.ExecutorSpawn},
		{"stop", types.ExecutorKill},
		{"expand", types.ExecutorExpand},
		{"gate", types.ExecutorBranch}, // Gates become branch with await-approval
	}

	for _, tt := range tests {
		t.Run(tt.legacyType, func(t *testing.T) {
			step := &Step{
				ID:   "test",
				Type: tt.legacyType,
			}
			// Add required fields based on type
			switch tt.legacyType {
			case "code":
				step.Code = "echo test"
			case "condition":
				step.Condition = "test -f /tmp/x"
			case "start", "stop":
				step.Assignee = "claude"
			case "expand":
				step.Template = "sub"
			case "gate":
				step.Instructions = "Approve this"
			case "task", "collaborative":
				step.Instructions = "Do work"
			}

			workflow := &Workflow{
				Name:  "legacy-test",
				Steps: []*Step{step},
			}

			baker := NewBaker("wf-legacy-001")
			baker.Now = fixedTime

			result, err := baker.BakeWorkflow(workflow, nil)
			if err != nil {
				t.Fatalf("BakeWorkflow failed: %v", err)
			}

			if len(result.Steps) != 1 {
				t.Fatalf("expected 1 step, got %d", len(result.Steps))
			}

			if result.Steps[0].Executor != tt.expectedExecutor {
				t.Errorf("expected executor %s for type %q, got %s",
					tt.expectedExecutor, tt.legacyType, result.Steps[0].Executor)
			}
		})
	}
}

// TestBakeWorkflow_StepValidation tests that created steps are valid
func TestBakeWorkflow_StepValidation(t *testing.T) {
	workflow := &Workflow{
		Name: "validation-test",
		Steps: []*Step{
			{
				ID:       "shell-step",
				Executor: ExecutorShell,
				Command:  "echo test",
			},
			{
				ID:       "agent-step",
				Executor: ExecutorAgent,
				Agent:    "claude",
				Prompt:   "Do work",
			},
		},
	}

	baker := NewBaker("wf-valid-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	// All steps should pass validation
	for _, step := range result.Steps {
		if err := step.Validate(); err != nil {
			t.Errorf("step %s failed validation: %v", step.ID, err)
		}
	}
}
