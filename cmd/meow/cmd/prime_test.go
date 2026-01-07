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
