package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
)

func TestLsNoWorkflows(t *testing.T) {
	// Create temp directory with .meow structure
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, ".meow", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatalf("failed to create workflows dir: %v", err)
	}

	// Change to temp directory
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := runLs(lsCmd, nil)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	if !strings.Contains(output, "No workflows found") {
		t.Errorf("Expected 'No workflows found', got: %s", output)
	}
}

func TestLsWorkflowsSortedByDate(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, ".meow", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatalf("failed to create workflows dir: %v", err)
	}

	// Create workflows with different timestamps
	store, err := orchestrator.NewYAMLWorkflowStore(workflowsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Create workflows with different start times
	wf1 := types.NewWorkflow("wf-old", "test1.meow.toml", nil)
	wf1.StartedAt = time.Now().Add(-2 * time.Hour)
	wf1.Status = types.WorkflowStatusDone

	wf2 := types.NewWorkflow("wf-recent", "test2.meow.toml", nil)
	wf2.StartedAt = time.Now().Add(-1 * time.Hour)
	wf2.Status = types.WorkflowStatusRunning

	wf3 := types.NewWorkflow("wf-newest", "test3.meow.toml", nil)
	wf3.StartedAt = time.Now()
	wf3.Status = types.WorkflowStatusRunning

	store.Create(ctx, wf1)
	store.Create(ctx, wf2)
	store.Create(ctx, wf3)

	// Change to temp directory
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runLs(lsCmd, nil)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	// Verify workflows appear in correct order (newest first)
	lines := strings.Split(output, "\n")
	if len(lines) < 4 { // Header + 3 workflows
		t.Fatalf("Expected at least 4 lines, got %d", len(lines))
	}

	// Check that wf-newest appears before wf-recent before wf-old
	newestIdx := -1
	recentIdx := -1
	oldIdx := -1

	for i, line := range lines {
		if strings.Contains(line, "wf-newest") {
			newestIdx = i
		}
		if strings.Contains(line, "wf-recent") {
			recentIdx = i
		}
		if strings.Contains(line, "wf-old") {
			oldIdx = i
		}
	}

	if newestIdx == -1 || recentIdx == -1 || oldIdx == -1 {
		t.Fatalf("Not all workflows found in output")
	}

	if !(newestIdx < recentIdx && recentIdx < oldIdx) {
		t.Errorf("Workflows not sorted by date (newest first). Order: newest=%d, recent=%d, old=%d", newestIdx, recentIdx, oldIdx)
	}
}

func TestLsRunningFlag(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, ".meow", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatalf("failed to create workflows dir: %v", err)
	}

	store, err := orchestrator.NewYAMLWorkflowStore(workflowsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	wfRunning := types.NewWorkflow("wf-running", "test.meow.toml", nil)
	wfRunning.Status = types.WorkflowStatusRunning

	wfDone := types.NewWorkflow("wf-done", "test.meow.toml", nil)
	wfDone.Status = types.WorkflowStatusDone

	store.Create(ctx, wfRunning)
	store.Create(ctx, wfDone)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)

	// Set the running flag
	lsRunning = true
	defer func() { lsRunning = false }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runLs(lsCmd, nil)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	// Should only show running workflow
	if !strings.Contains(output, "wf-running") {
		t.Error("Expected to see wf-running in output")
	}
	if strings.Contains(output, "wf-done") {
		t.Error("Should not see wf-done in output with --running flag")
	}
}

func TestLsJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, ".meow", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatalf("failed to create workflows dir: %v", err)
	}

	store, err := orchestrator.NewYAMLWorkflowStore(workflowsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	wf := types.NewWorkflow("wf-test", "test.meow.toml", nil)
	wf.Status = types.WorkflowStatusRunning
	store.Create(ctx, wf)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)

	lsJSON = true
	defer func() { lsJSON = false }()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runLs(lsCmd, nil)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	// Verify it's valid JSON
	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("Expected 1 workflow, got %d", len(result))
	}

	if result[0]["id"] != "wf-test" {
		t.Errorf("Expected id 'wf-test', got %v", result[0]["id"])
	}
}

func TestLsStaleDetection(t *testing.T) {
	tmpDir := t.TempDir()
	workflowsDir := filepath.Join(tmpDir, ".meow", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatalf("failed to create workflows dir: %v", err)
	}

	store, err := orchestrator.NewYAMLWorkflowStore(workflowsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Create a workflow with running status but no lock held
	wf := types.NewWorkflow("wf-stale", "test.meow.toml", nil)
	wf.Status = types.WorkflowStatusRunning
	store.Create(ctx, wf)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err = runLs(lsCmd, nil)

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	output := buf.String()

	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	// Should show as stale since no lock is held
	if !strings.Contains(output, "running (stale)") && !strings.Contains(output, "stale") {
		t.Errorf("Expected to see 'stale' indicator, got: %s", output)
	}
}
