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

func resetLsFlags() {
	lsAll = false
	lsStale = false
	lsStatus = ""
	lsJSON = false
}

func TestLsNoWorkflows(t *testing.T) {
	// Create temp directory with .meow structure
	tmpDir := t.TempDir()
	runsDir := filepath.Join(tmpDir, ".meow", "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatalf("failed to create runs dir: %v", err)
	}

	// Change to temp directory
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

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

	// Default shows "No active workflows" since we filter to active only
	if !strings.Contains(output, "No active workflows") {
		t.Errorf("Expected 'No active workflows', got: %s", output)
	}
}

func TestLsWorkflowsSortedByDate(t *testing.T) {
	tmpDir := t.TempDir()
	runsDir := filepath.Join(tmpDir, ".meow", "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatalf("failed to create runs dir: %v", err)
	}

	// Create workflows with different timestamps
	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Create workflows with different start times
	wf1 := types.NewRun("run-old", "test1.meow.toml", nil)
	wf1.StartedAt = time.Now().Add(-2 * time.Hour)
	wf1.Status = types.RunStatusDone

	wf2 := types.NewRun("run-recent", "test2.meow.toml", nil)
	wf2.StartedAt = time.Now().Add(-1 * time.Hour)
	wf2.Status = types.RunStatusRunning

	wf3 := types.NewRun("run-newest", "test3.meow.toml", nil)
	wf3.StartedAt = time.Now()
	wf3.Status = types.RunStatusRunning

	store.Create(ctx, wf1)
	store.Create(ctx, wf2)
	store.Create(ctx, wf3)

	// Change to temp directory
	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

	// Use --all to see all workflows (not just active)
	lsAll = true

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
		t.Fatalf("Expected at least 4 lines, got %d: %s", len(lines), output)
	}

	// Check that wf-newest appears before wf-recent before wf-old
	newestIdx := -1
	recentIdx := -1
	oldIdx := -1

	for i, line := range lines {
		if strings.Contains(line, "run-newest") {
			newestIdx = i
		}
		if strings.Contains(line, "run-recent") {
			recentIdx = i
		}
		if strings.Contains(line, "run-old") {
			oldIdx = i
		}
	}

	if newestIdx == -1 || recentIdx == -1 || oldIdx == -1 {
		t.Fatalf("Not all workflows found in output: %s", output)
	}

	if !(newestIdx < recentIdx && recentIdx < oldIdx) {
		t.Errorf("Workflows not sorted by date (newest first). Order: newest=%d, recent=%d, old=%d", newestIdx, recentIdx, oldIdx)
	}
}

func TestLsStatusFlag(t *testing.T) {
	tmpDir := t.TempDir()
	runsDir := filepath.Join(tmpDir, ".meow", "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatalf("failed to create runs dir: %v", err)
	}

	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	wfRunning := types.NewRun("run-running", "test.meow.toml", nil)
	wfRunning.Status = types.RunStatusRunning

	wfDone := types.NewRun("run-done", "test.meow.toml", nil)
	wfDone.Status = types.RunStatusDone

	store.Create(ctx, wfRunning)
	store.Create(ctx, wfDone)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

	// Use --status=running to see running workflows (including stale)
	lsStatus = "running"

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

	// Should only show running workflow (note: it will show as stale since no lock)
	if !strings.Contains(output, "run-running") {
		t.Errorf("Expected to see wf-running in output, got: %s", output)
	}
	if strings.Contains(output, "run-done") {
		t.Error("Should not see wf-done in output with --status=running")
	}
}

func TestLsJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	runsDir := filepath.Join(tmpDir, ".meow", "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatalf("failed to create runs dir: %v", err)
	}

	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	wf := types.NewRun("run-test", "test.meow.toml", nil)
	wf.Status = types.RunStatusRunning
	store.Create(ctx, wf)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

	// Use --all and --json to see all workflows as JSON
	lsAll = true
	lsJSON = true

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

	if result[0]["id"] != "run-test" {
		t.Errorf("Expected id 'wf-test', got %v", result[0]["id"])
	}
}

func TestLsStaleDetection(t *testing.T) {
	tmpDir := t.TempDir()
	runsDir := filepath.Join(tmpDir, ".meow", "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatalf("failed to create runs dir: %v", err)
	}

	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Create a workflow with running status but no lock held
	wf := types.NewRun("run-stale", "test.meow.toml", nil)
	wf.Status = types.RunStatusRunning
	store.Create(ctx, wf)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

	// Use --stale to see stale workflows
	lsStale = true

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

	// Should show the stale workflow with "(stale)" indicator
	if !strings.Contains(output, "run-stale") {
		t.Errorf("Expected to see wf-stale in output with --stale flag, got: %s", output)
	}
	if !strings.Contains(output, "running (stale)") {
		t.Errorf("Expected to see 'running (stale)' indicator, got: %s", output)
	}
}
