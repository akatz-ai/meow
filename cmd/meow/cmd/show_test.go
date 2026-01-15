package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetShowFlags() {
	showRaw = false
	showJSON = false
}

func writeTestWorkflowFile(t *testing.T, baseDir, name, content string) string {
	t.Helper()

	path := filepath.Join(baseDir, ".meow", "workflows", filepath.FromSlash(name)+".meow.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create workflow dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write workflow: %v", err)
	}
	return path
}

func captureShowOutput(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := fn()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String(), err
}

func TestShowWorkflowNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .meow dir but no workflows
	if err := os.MkdirAll(filepath.Join(tmpDir, ".meow"), 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetShowFlags()

	err := runShow(showCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent workflow")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected 'not found' error, got: %v", err)
	}
}

func TestShowSingleWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	content := `
[main]
name = "test-workflow"
description = "A test workflow"

[main.variables]
task_id = { required = true, description = "The task ID" }
debug = { default = "false", description = "Enable debug mode" }

[[main.steps]]
id = "step1"
executor = "shell"
command = "echo hello"

[[main.steps]]
id = "step2"
executor = "shell"
command = "echo done"
needs = ["step1"]
`
	writeTestWorkflowFile(t, tmpDir, "test-workflow", content)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetShowFlags()

	output, err := captureShowOutput(t, func() error {
		return runShow(showCmd, []string{"test-workflow"})
	})
	if err != nil {
		t.Fatalf("runShow failed: %v", err)
	}

	// Check expected content
	if !strings.Contains(output, "Workflow: main") {
		t.Errorf("expected workflow name, got: %s", output)
	}
	if !strings.Contains(output, "A test workflow") {
		t.Errorf("expected description, got: %s", output)
	}
	if !strings.Contains(output, "task_id") {
		t.Errorf("expected variable task_id, got: %s", output)
	}
	if !strings.Contains(output, "(required)") {
		t.Errorf("expected required marker, got: %s", output)
	}
	if !strings.Contains(output, "step1") {
		t.Errorf("expected step1, got: %s", output)
	}
	if !strings.Contains(output, "step2") {
		t.Errorf("expected step2, got: %s", output)
	}
	if !strings.Contains(output, "Steps (2)") {
		t.Errorf("expected step count, got: %s", output)
	}
	if !strings.Contains(output, "Use --raw") {
		t.Errorf("expected --raw hint, got: %s", output)
	}
}

func TestShowMultiWorkflowModule(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	content := `
[helper]
name = "helper"
description = "A helper workflow"

[[helper.steps]]
id = "h1"
executor = "shell"
command = "echo helper"

[other]
name = "other"
description = "Another workflow"

[[other.steps]]
id = "o1"
executor = "shell"
command = "echo other"
`
	writeTestWorkflowFile(t, tmpDir, "lib/multi", content)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetShowFlags()

	// Show without specific workflow - should show module overview
	output, err := captureShowOutput(t, func() error {
		return runShow(showCmd, []string{"lib/multi"})
	})
	if err != nil {
		t.Fatalf("runShow failed: %v", err)
	}

	if !strings.Contains(output, "Module: lib/multi") {
		t.Errorf("expected module name, got: %s", output)
	}
	if !strings.Contains(output, "Workflows (2)") {
		t.Errorf("expected workflow count, got: %s", output)
	}
	if !strings.Contains(output, "helper") {
		t.Errorf("expected helper workflow listed, got: %s", output)
	}
	if !strings.Contains(output, "other") {
		t.Errorf("expected other workflow listed, got: %s", output)
	}
}

func TestShowSpecificWorkflowInModule(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	content := `
[helper]
name = "helper"
description = "A helper workflow"

[[helper.steps]]
id = "h1"
executor = "shell"
command = "echo helper"

[other]
name = "other"
description = "Another workflow"

[[other.steps]]
id = "o1"
executor = "shell"
command = "echo other"
`
	writeTestWorkflowFile(t, tmpDir, "lib/multi", content)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetShowFlags()

	// Show specific workflow using #workflow syntax
	output, err := captureShowOutput(t, func() error {
		return runShow(showCmd, []string{"lib/multi#helper"})
	})
	if err != nil {
		t.Fatalf("runShow failed: %v", err)
	}

	if !strings.Contains(output, "Workflow: helper") {
		t.Errorf("expected workflow name, got: %s", output)
	}
	if !strings.Contains(output, "A helper workflow") {
		t.Errorf("expected description, got: %s", output)
	}
	if !strings.Contains(output, "Other workflows in module:") {
		t.Errorf("expected other workflows section, got: %s", output)
	}
}

func TestShowRawOutput(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	content := `# Test workflow file
[main]
name = "raw-test"
description = "Test raw output"

[[main.steps]]
id = "s1"
executor = "shell"
command = "echo raw"
`
	writeTestWorkflowFile(t, tmpDir, "raw-test", content)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetShowFlags()

	showRaw = true

	output, err := captureShowOutput(t, func() error {
		return runShow(showCmd, []string{"raw-test"})
	})
	if err != nil {
		t.Fatalf("runShow failed: %v", err)
	}

	// Raw output should contain original TOML content
	if !strings.Contains(output, "# Test workflow file") {
		t.Errorf("expected comment from original file, got: %s", output)
	}
	if !strings.Contains(output, `name = "raw-test"`) {
		t.Errorf("expected TOML content, got: %s", output)
	}
	// Should NOT contain summary format
	if strings.Contains(output, "Workflow:") {
		t.Errorf("raw output should not contain summary format, got: %s", output)
	}
}

func TestShowJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	content := `
[main]
name = "json-test"
description = "Test JSON output"

[main.variables]
task = { required = true }

[[main.steps]]
id = "j1"
executor = "shell"
command = "echo json"
`
	writeTestWorkflowFile(t, tmpDir, "json-test", content)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetShowFlags()

	showJSON = true

	output, err := captureShowOutput(t, func() error {
		return runShow(showCmd, []string{"json-test"})
	})
	if err != nil {
		t.Fatalf("runShow failed: %v", err)
	}

	// Should be valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\nOutput: %s", err, output)
	}

	// Check JSON structure
	if result["name"] != "main" {
		t.Errorf("expected name 'main', got %v", result["name"])
	}
	if result["source"] != "project" {
		t.Errorf("expected source 'project', got %v", result["source"])
	}

	workflows, ok := result["workflows"].([]interface{})
	if !ok {
		t.Fatalf("expected workflows array, got %T", result["workflows"])
	}
	if len(workflows) != 1 {
		t.Errorf("expected 1 workflow, got %d", len(workflows))
	}
}

func TestShowWithCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Note: cleanup_on_success must be BEFORE [main.variables] section
	// or it gets parsed as part of variables (TOML subsection rules)
	content := `
[main]
name = "cleanup-test"
description = "Test cleanup display"
cleanup_on_success = "echo done"
cleanup_on_failure = "echo failed"

[[main.steps]]
id = "c1"
executor = "shell"
command = "echo cleanup"
`
	writeTestWorkflowFile(t, tmpDir, "cleanup-test", content)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetShowFlags()

	output, err := captureShowOutput(t, func() error {
		return runShow(showCmd, []string{"cleanup-test"})
	})
	if err != nil {
		t.Fatalf("runShow failed: %v", err)
	}

	if !strings.Contains(output, "Cleanup:") {
		t.Errorf("expected Cleanup section, got: %s", output)
	}
	if !strings.Contains(output, "on_success") {
		t.Errorf("expected on_success cleanup, got: %s", output)
	}
	if !strings.Contains(output, "on_failure") {
		t.Errorf("expected on_failure cleanup, got: %s", output)
	}
}

func TestShowTemplateReference(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	content := `
[main]
name = "expand-test"
description = "Test template reference display"

[[main.steps]]
id = "e1"
executor = "expand"
template = "lib/some-template#helper"

[[main.steps]]
id = "e2"
executor = "foreach"
template = "lib/other"
items = '["a", "b"]'
item_var = "item"
needs = ["e1"]
`
	writeTestWorkflowFile(t, tmpDir, "expand-test", content)

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetShowFlags()

	output, err := captureShowOutput(t, func() error {
		return runShow(showCmd, []string{"expand-test"})
	})
	if err != nil {
		t.Fatalf("runShow failed: %v", err)
	}

	// Should show template references with arrow
	if !strings.Contains(output, "lib/some-template#helper") {
		t.Errorf("expected template reference, got: %s", output)
	}
	if !strings.Contains(output, "lib/other") {
		t.Errorf("expected foreach template, got: %s", output)
	}
}
