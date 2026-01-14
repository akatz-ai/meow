package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetLsFlags() {
	lsAll = false
	lsJSON = false
}

func writeWorkflowFile(t *testing.T, baseDir, name, description string) string {
	t.Helper()

	path := filepath.Join(baseDir, ".meow", "workflows", filepath.FromSlash(name)+".meow.toml")
	content := fmt.Sprintf(`
[main]
name = "main"
description = %q

[[main.steps]]
id = "step1"
executor = "shell"
command = "echo hello"
`, description)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create workflow dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write workflow: %v", err)
	}

	return path
}

func captureOutput(t *testing.T, fn func() error) (string, error) {
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

func TestLsNoWorkflows(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	if err := os.MkdirAll(filepath.Join(tmpDir, ".meow"), 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

	output, err := captureOutput(t, func() error {
		return runLs(lsCmd, nil)
	})
	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	if !strings.Contains(output, "No workflows found") {
		t.Fatalf("Expected 'No workflows found', got: %s", output)
	}
}

func TestLsTopLevelOnly(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)
	writeWorkflowFile(t, tmpDir, "top-level", "Top level workflow")
	writeWorkflowFile(t, tmpDir, "lib/sub", "Sub workflow")

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

	output, err := captureOutput(t, func() error {
		return runLs(lsCmd, nil)
	})
	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	if !strings.Contains(output, "top-level") {
		t.Fatalf("Expected top-level workflow in output: %s", output)
	}
	if strings.Contains(output, "lib/sub") {
		t.Fatalf("Did not expect subdirectory workflow without --all: %s", output)
	}
}

func TestLsAllIncludesSubdirectories(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)
	writeWorkflowFile(t, tmpDir, "top-level", "Top level workflow")
	writeWorkflowFile(t, tmpDir, "lib/sub", "Sub workflow")

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

	lsAll = true

	output, err := captureOutput(t, func() error {
		return runLs(lsCmd, nil)
	})
	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	if !strings.Contains(output, "top-level") || !strings.Contains(output, "lib/sub") {
		t.Fatalf("Expected both workflows with --all, got: %s", output)
	}
}

func TestLsPathArgument(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)
	writeWorkflowFile(t, tmpDir, "top-level", "Top level workflow")
	writeWorkflowFile(t, tmpDir, "lib/sub", "Sub workflow")

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

	output, err := captureOutput(t, func() error {
		return runLs(lsCmd, []string{"lib"})
	})
	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	if !strings.Contains(output, "lib/sub") {
		t.Fatalf("Expected lib/sub workflow, got: %s", output)
	}
	if strings.Contains(output, "top-level") {
		t.Fatalf("Did not expect top-level workflow when listing lib/: %s", output)
	}
}

func TestLsProjectOverridesUser(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)
	writeWorkflowFile(t, tmpDir, "shared", "Project workflow")
	writeWorkflowFile(t, userHome, "shared", "User workflow")

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

	output, err := captureOutput(t, func() error {
		return runLs(lsCmd, nil)
	})
	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	if !strings.Contains(output, "shared") {
		t.Fatalf("Expected shared workflow in output: %s", output)
	}
	if !strings.Contains(output, "project") {
		t.Fatalf("Expected project source for shared workflow: %s", output)
	}
	if strings.Contains(output, "user") {
		t.Fatalf("Did not expect user source for shadowed workflow: %s", output)
	}
}

func TestLsJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)
	writeWorkflowFile(t, tmpDir, "json-workflow", "JSON workflow")

	origWd, _ := os.Getwd()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(origWd)
	defer resetLsFlags()

	lsJSON = true

	output, err := captureOutput(t, func() error {
		return runLs(lsCmd, nil)
	})
	if err != nil {
		t.Fatalf("runLs failed: %v", err)
	}

	var result []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("Output is not valid JSON: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("Expected 1 workflow, got %d", len(result))
	}

	if result[0]["workflow"] != "json-workflow" {
		t.Fatalf("Expected workflow 'json-workflow', got %v", result[0]["workflow"])
	}
}
