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

func TestBaker_BlockingGate(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{
				ID:           "gate",
				Description:  "Wait for approval",
				Type:         "blocking-gate",
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
	if bead.Type != types.BeadTypeCondition {
		t.Errorf("expected condition type for gate, got %s", bead.Type)
	}
	if bead.ConditionSpec == nil {
		t.Fatal("expected ConditionSpec")
	}
	// Blocking gate should have a wait-approve condition
	if !strings.Contains(bead.ConditionSpec.Condition, "wait-approve") {
		t.Errorf("expected wait-approve condition, got %q", bead.ConditionSpec.Condition)
	}
}

func TestBaker_RestartStep(t *testing.T) {
	tmpl := &Template{
		Meta: Meta{Name: "test"},
		Steps: []Step{
			{
				ID:          "restart",
				Description: "Check if loop should continue",
				Type:        "restart",
				Condition:   "bd list --status=open | grep -q .",
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
		t.Errorf("expected condition type for restart, got %s", bead.Type)
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
