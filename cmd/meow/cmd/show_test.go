package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
)

func TestShowRequiresBeadsDir(t *testing.T) {
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

	// Run show command
	cmd := showCmd
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err = cmd.RunE(cmd, []string{"bd-test"})
	if err == nil {
		t.Fatal("expected error when .beads directory is missing")
	}

	if err.Error() != "no .beads directory found. Are you in a beads project?" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestShowDisplaysBasicBeadInfo(t *testing.T) {
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

	// Create a test bead
	testBead := &types.Bead{
		ID:          "bd-test-001",
		Type:        types.BeadTypeTask,
		Title:       "Test task",
		Description: "A test description",
		Status:      types.BeadStatusOpen,
		Assignee:    "test-agent",
		CreatedAt:   time.Now(),
	}

	if err := store.Create(ctx, testBead); err != nil {
		t.Fatal(err)
	}

	// Save and restore workDir
	oldWorkDir := workDir
	workDir = tmpDir
	defer func() { workDir = oldWorkDir }()

	// Capture output
	buf := new(bytes.Buffer)
	cmd := showCmd
	cmd.SetOut(buf)

	// Reset JSON flag
	showJSON = false

	err = runShow(cmd, []string{"bd-test-001"})
	if err != nil {
		t.Fatalf("runShow failed: %v", err)
	}

	// Note: output goes to stdout, not the buffer, so we can't easily capture it
	// This test mainly verifies the command runs without error
}

func TestShowDisplaysOutputs(t *testing.T) {
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

	// Create a bead with outputs
	now := time.Now()
	closedAt := now
	testBead := &types.Bead{
		ID:        "bd-code-001",
		Type:      types.BeadTypeCode,
		Title:     "Run build",
		Status:    types.BeadStatusClosed,
		CreatedAt: now,
		ClosedAt:  &closedAt,
		Outputs: map[string]any{
			"stdout":    "Build successful",
			"exit_code": 0,
			"artifact":  "/path/to/output.bin",
		},
		CodeSpec: &types.CodeSpec{
			Code: "make build",
			Outputs: []types.OutputSpec{
				{Name: "stdout", Source: types.OutputTypeStdout},
			},
		},
	}

	if err := store.Create(ctx, testBead); err != nil {
		t.Fatal(err)
	}

	// Save and restore workDir
	oldWorkDir := workDir
	workDir = tmpDir
	defer func() { workDir = oldWorkDir }()

	// Test JSON output which we can capture
	showJSON = true
	defer func() { showJSON = false }()

	// Capture stdout
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runShow(showCmd, []string{"bd-code-001"})
	if err != nil {
		w.Close()
		os.Stdout = old
		t.Fatalf("runShow failed: %v", err)
	}

	w.Close()
	os.Stdout = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Verify JSON contains outputs
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v", err)
	}

	outputs, ok := result["outputs"].(map[string]any)
	if !ok {
		t.Fatal("expected outputs in JSON output")
	}

	if outputs["stdout"] != "Build successful" {
		t.Errorf("expected stdout output 'Build successful', got %v", outputs["stdout"])
	}

	// exit_code is float64 when unmarshaled from JSON
	if outputs["exit_code"].(float64) != 0 {
		t.Errorf("expected exit_code 0, got %v", outputs["exit_code"])
	}
}

func TestShowBeadNotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	beadsDir := filepath.Join(tmpDir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create empty store
	store := orchestrator.NewFileBeadStore(beadsDir)
	ctx := context.Background()
	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}
	// Write empty JSONL file so store can load
	os.WriteFile(filepath.Join(beadsDir, "issues.jsonl"), []byte{}, 0644)

	// Save and restore workDir
	oldWorkDir := workDir
	workDir = tmpDir
	defer func() { workDir = oldWorkDir }()

	err = runShow(showCmd, []string{"nonexistent-bead"})
	if err == nil {
		t.Fatal("expected error for non-existent bead")
	}

	if !strings.Contains(err.Error(), "bead not found") {
		t.Errorf("expected 'bead not found' error, got: %v", err)
	}
}

func TestPrintBeadWithAllFields(t *testing.T) {
	// Test printBead doesn't panic with a fully-populated bead
	now := time.Now()
	closedAt := now
	bead := &types.Bead{
		ID:             "meow-test.step1",
		Type:           types.BeadTypeTask,
		Title:          "Complete task",
		Description:    "A detailed description",
		Status:         types.BeadStatusClosed,
		Assignee:       "test-agent",
		Needs:          []string{"meow-test.step0"},
		Labels:         []string{"test", "example"},
		Notes:          "Some notes here",
		Parent:         "meow-test",
		Tier:           types.TierWisp,
		HookBead:       "bd-work-001",
		SourceWorkflow: "implement-tdd",
		WorkflowID:     "meow-test",
		CreatedAt:      now,
		ClosedAt:       &closedAt,
		Outputs: map[string]any{
			"result":  "success",
			"count":   42,
			"details": map[string]any{"key": "value"},
		},
		TaskOutputs: &types.TaskOutputSpec{
			Required: []types.TaskOutputDef{
				{Name: "result", Type: types.TaskOutputTypeString, Description: "The result"},
			},
			Optional: []types.TaskOutputDef{
				{Name: "count", Type: types.TaskOutputTypeNumber},
			},
		},
		Instructions: "Do the thing",
	}

	// Should not panic
	printBead(bead)
}

func TestTruncateCode(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"short", "short"},
		{"echo hello", "echo hello"},
		{strings.Repeat("a", 100), strings.Repeat("a", 60) + "..."},
		{"line1\nline2\nline3", "line1 line2 line3"},
		{"  spaced   out  ", "spaced out"},
	}

	for _, tt := range tests {
		result := truncateCode(tt.input)
		if result != tt.expected {
			t.Errorf("truncateCode(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
