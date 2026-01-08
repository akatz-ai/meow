package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
)

func TestPrimeRequiresBeadsDir(t *testing.T) {
	// Create a temp directory WITHOUT .beads
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Save and restore workDir
	oldWorkDir := workDir
	workDir = tmpDir
	defer func() { workDir = oldWorkDir }()

	// Run prime command
	cmd := primeCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err = cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error when .beads directory is missing")
	}

	if err.Error() != "no .beads directory found. Are you in a beads project?" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrimeShowsWispsNotOrchestratorBeads(t *testing.T) {
	// Create temp directory with .beads
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create bead store and add test beads
	store := orchestrator.NewFileBeadStore(beadsDir)

	ctx := context.Background()

	// Create a work bead
	workBead := &types.Bead{
		ID:     "bd-work-001",
		Type:   types.BeadTypeTask,
		Title:  "Implement feature",
		Status: types.BeadStatusOpen,
		Tier:   types.TierWork,
	}

	// Create a wisp (should be visible to agent)
	wispBead := &types.Bead{
		ID:             "meow-test.step1",
		Type:           types.BeadTypeTask,
		Title:          "Load context",
		Status:         types.BeadStatusOpen,
		Tier:           types.TierWisp,
		Assignee:       "test-agent",
		HookBead:       "bd-work-001",
		SourceWorkflow: "test-workflow",
		WorkflowID:     "meow-test",
		Instructions:   "Read the requirements",
	}

	// Create an orchestrator bead (should NOT be visible to agent)
	orchBead := &types.Bead{
		ID:         "meow-test.condition",
		Type:       types.BeadTypeCondition,
		Title:      "Check precondition",
		Status:     types.BeadStatusOpen,
		Tier:       types.TierOrchestrator,
		WorkflowID: "meow-test",
	}

	// Load empty store first, then create beads
	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	for _, bead := range []*types.Bead{workBead, wispBead, orchBead} {
		if err := store.Create(ctx, bead); err != nil {
			t.Fatal(err)
		}
	}

	// Get prime output
	output, err := getPrimeOutput(ctx, store, "test-agent")
	if err != nil {
		t.Fatalf("getPrimeOutput failed: %v", err)
	}

	if output == nil {
		t.Fatal("expected output, got nil")
	}

	// Should show the wisp step
	if output.CurrentStep == nil {
		t.Fatal("expected current step")
	}
	if output.CurrentStep.ID != "meow-test.step1" {
		t.Errorf("expected current step to be wisp, got %s", output.CurrentStep.ID)
	}

	// Should link to work bead
	if output.WorkBead == nil {
		t.Fatal("expected work bead link")
	}
	if output.WorkBead.ID != "bd-work-001" {
		t.Errorf("expected work bead bd-work-001, got %s", output.WorkBead.ID)
	}

	// Workflow should only contain wisps, not orchestrator beads
	if output.Workflow == nil {
		t.Fatal("expected workflow info")
	}
	for _, step := range output.Workflow.Steps {
		if step.ID == "meow-test.condition" {
			t.Error("orchestrator bead should not be visible in workflow steps")
		}
	}
}

func TestPrimeCollaborativeModeReturnsEmpty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	store := orchestrator.NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// Create a collaborative bead that's in_progress
	collabBead := &types.Bead{
		ID:         "meow-test.review",
		Type:       types.BeadTypeCollaborative,
		Title:      "Design review",
		Status:     types.BeadStatusInProgress, // Key: in_progress
		Tier:       types.TierWisp,
		Assignee:   "test-agent",
		WorkflowID: "meow-test",
	}

	if err := store.Create(ctx, collabBead); err != nil {
		t.Fatal(err)
	}

	output, err := getPrimeOutput(ctx, store, "test-agent")
	if err != nil {
		t.Fatalf("getPrimeOutput failed: %v", err)
	}

	if output == nil {
		t.Fatal("expected output, got nil")
	}

	// Should indicate conversation mode
	if !output.ConversationMode {
		t.Error("expected ConversationMode to be true for in_progress collaborative step")
	}

	// Other fields should be empty in conversation mode
	if output.CurrentStep != nil {
		t.Error("expected no current step in conversation mode")
	}
}

func TestPrimeShowsRequiredOutputs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	store := orchestrator.NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// Create a wisp with required outputs
	wispBead := &types.Bead{
		ID:             "meow-test.implement",
		Type:           types.BeadTypeTask,
		Title:          "Implement feature",
		Status:         types.BeadStatusOpen,
		Tier:           types.TierWisp,
		Assignee:       "test-agent",
		SourceWorkflow: "implement-tdd",
		WorkflowID:     "meow-test",
		Instructions:   "Write the implementation code",
		TaskOutputs: &types.TaskOutputSpec{
			Required: []types.TaskOutputDef{
				{Name: "files_changed", Type: types.TaskOutputTypeStringArr, Description: "List of modified files"},
				{Name: "commit_sha", Type: types.TaskOutputTypeString, Description: "Git commit SHA"},
			},
		},
	}

	if err := store.Create(ctx, wispBead); err != nil {
		t.Fatal(err)
	}

	output, err := getPrimeOutput(ctx, store, "test-agent")
	if err != nil {
		t.Fatalf("getPrimeOutput failed: %v", err)
	}

	if output == nil {
		t.Fatal("expected output, got nil")
	}

	// Check current step has required outputs
	if output.CurrentStep == nil {
		t.Fatal("expected current step")
	}

	if len(output.CurrentStep.RequiredOutputs) != 2 {
		t.Errorf("expected 2 required outputs, got %d", len(output.CurrentStep.RequiredOutputs))
	}

	// Verify first output
	if output.CurrentStep.RequiredOutputs[0].Name != "files_changed" {
		t.Errorf("expected files_changed, got %s", output.CurrentStep.RequiredOutputs[0].Name)
	}
	if output.CurrentStep.RequiredOutputs[0].Type != types.TaskOutputTypeStringArr {
		t.Errorf("expected string[] type, got %s", output.CurrentStep.RequiredOutputs[0].Type)
	}

	// Test formatText includes required outputs
	text := formatText(output)
	if !bytes.Contains([]byte(text), []byte("Required outputs:")) {
		t.Error("formatText should contain 'Required outputs:'")
	}
	if !bytes.Contains([]byte(text), []byte("files_changed")) {
		t.Error("formatText should contain 'files_changed'")
	}
	if !bytes.Contains([]byte(text), []byte("commit_sha")) {
		t.Error("formatText should contain 'commit_sha'")
	}

	// Test formatPrompt includes required outputs
	prompt := formatPrompt(output)
	if !bytes.Contains([]byte(prompt), []byte("**Required outputs:**")) {
		t.Error("formatPrompt should contain '**Required outputs:**'")
	}
	if !bytes.Contains([]byte(prompt), []byte("`files_changed`")) {
		t.Error("formatPrompt should contain '`files_changed`'")
	}
}

func TestPrimeShowsNoRequiredOutputsWhenEmpty(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	store := orchestrator.NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// Create a wisp WITHOUT required outputs
	wispBead := &types.Bead{
		ID:             "meow-test.review",
		Type:           types.BeadTypeTask,
		Title:          "Review code",
		Status:         types.BeadStatusOpen,
		Tier:           types.TierWisp,
		Assignee:       "test-agent",
		SourceWorkflow: "implement-tdd",
		WorkflowID:     "meow-test",
		Instructions:   "Review the code changes",
	}

	if err := store.Create(ctx, wispBead); err != nil {
		t.Fatal(err)
	}

	output, err := getPrimeOutput(ctx, store, "test-agent")
	if err != nil {
		t.Fatalf("getPrimeOutput failed: %v", err)
	}

	if output == nil {
		t.Fatal("expected output, got nil")
	}

	// Check current step exists and has no required outputs
	if output.CurrentStep == nil {
		t.Fatal("expected current step")
	}
	if len(output.CurrentStep.RequiredOutputs) != 0 {
		t.Errorf("expected 0 required outputs, got %d", len(output.CurrentStep.RequiredOutputs))
	}

	// Test formatText shows "(none)" for empty outputs
	text := formatText(output)
	if !bytes.Contains([]byte(text), []byte("Required outputs: (none)")) {
		t.Error("formatText should contain 'Required outputs: (none)' when no outputs required")
	}
}
