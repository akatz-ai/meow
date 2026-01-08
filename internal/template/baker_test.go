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

// Tests for BakeWorkflow with code/start/stop step types
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

	if len(result.Beads) != 1 {
		t.Fatalf("expected 1 bead, got %d", len(result.Beads))
	}

	bead := result.Beads[0]
	if bead.Type != types.BeadTypeCode {
		t.Errorf("expected code type, got %s", bead.Type)
	}
	if bead.CodeSpec == nil {
		t.Fatal("expected CodeSpec")
	}
	if bead.CodeSpec.Code != "echo 'hello world'" {
		t.Errorf("expected code, got %q", bead.CodeSpec.Code)
	}
	if bead.Tier != types.TierOrchestrator {
		t.Errorf("expected orchestrator tier for code, got %s", bead.Tier)
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

	if len(result.Beads) != 2 {
		t.Fatalf("expected 2 beads, got %d", len(result.Beads))
	}

	// Check start bead
	startBead := result.Beads[0]
	if startBead.Type != types.BeadTypeStart {
		t.Errorf("expected start type, got %s", startBead.Type)
	}
	if startBead.StartSpec == nil {
		t.Fatal("expected StartSpec")
	}
	if startBead.StartSpec.Agent != "claude-worker" {
		t.Errorf("expected agent 'claude-worker', got %q", startBead.StartSpec.Agent)
	}

	// Check stop bead
	stopBead := result.Beads[1]
	if stopBead.Type != types.BeadTypeStop {
		t.Errorf("expected stop type, got %s", stopBead.Type)
	}
	if stopBead.StopSpec == nil {
		t.Fatal("expected StopSpec")
	}
	if stopBead.StopSpec.Agent != "claude-worker" {
		t.Errorf("expected agent 'claude-worker', got %q", stopBead.StopSpec.Agent)
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
	if bead.ExpandSpec.Variables["task"] != "bd-42" {
		t.Errorf("expected variable, got %v", bead.ExpandSpec.Variables)
	}
	if bead.ExpandSpec.Assignee != "default-agent" {
		t.Errorf("expected default assignee, got %q", bead.ExpandSpec.Assignee)
	}
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

	bead := result.Beads[0]
	if bead.Type != types.BeadTypeCondition {
		t.Errorf("expected condition type, got %s", bead.Type)
	}
	if bead.ConditionSpec == nil {
		t.Fatal("expected ConditionSpec")
	}
	if bead.ConditionSpec.Condition != "test -f /tmp/ready" {
		t.Errorf("expected condition, got %q", bead.ConditionSpec.Condition)
	}
	if bead.ConditionSpec.Timeout != "5m" {
		t.Errorf("expected timeout '5m', got %q", bead.ConditionSpec.Timeout)
	}
	if bead.ConditionSpec.OnTrue == nil || bead.ConditionSpec.OnTrue.Template != "proceed" {
		t.Errorf("expected on_true template")
	}
	if bead.ConditionSpec.OnFalse == nil || bead.ConditionSpec.OnFalse.Template != "wait" {
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
			{ID: "step-1", Type: "task", Title: "Using {{framework}}"},
		},
	}

	baker := NewBaker("wf-default-001")
	baker.Now = fixedTime

	result, err := baker.BakeWorkflow(workflow, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	bead := result.Beads[0]
	if bead.Title != "Using pytest" {
		t.Errorf("expected default variable substitution, got %q", bead.Title)
	}
}

func TestBaker_BakeWorkflow_VariableOverride(t *testing.T) {
	workflow := &Workflow{
		Name: "var-override-test",
		Variables: map[string]*Var{
			"framework": {Default: "pytest"},
		},
		Steps: []*Step{
			{ID: "step-1", Type: "task", Title: "Using {{framework}}"},
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

	bead := result.Beads[0]
	if bead.Title != "Using jest" {
		t.Errorf("expected override variable, got %q", bead.Title)
	}
}

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

	bead := result.Beads[0]
	if !bead.IsEphemeral() {
		t.Error("expected bead to be ephemeral")
	}
}

func TestBaker_BakeWorkflow_UnknownDependency(t *testing.T) {
	workflow := &Workflow{
		Name: "bad-dep-test",
		Steps: []*Step{
			{ID: "step-1", Type: "task", Needs: []string{"nonexistent"}},
		},
	}

	baker := NewBaker("wf-bad-dep-001")
	baker.Now = fixedTime

	_, err := baker.BakeWorkflow(workflow, nil)
	if err == nil {
		t.Fatal("expected error for unknown dependency")
	}
	if !strings.Contains(err.Error(), "unknown dependency") {
		t.Errorf("expected unknown dependency error, got: %v", err)
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
