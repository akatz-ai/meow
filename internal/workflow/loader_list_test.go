package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoaderListAvailable(t *testing.T) {
	// Create a temp directory structure for testing
	tmpDir, err := os.MkdirTemp("", "meow-loader-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create project workflows directory
	projectWfDir := filepath.Join(tmpDir, ".meow", "workflows")
	if err := os.MkdirAll(projectWfDir, 0755); err != nil {
		t.Fatalf("failed to create workflows dir: %v", err)
	}

	// Create a test workflow
	testWorkflow := `[main]
name = "test-workflow"
description = "A test workflow"

[[main.steps]]
id = "step-1"
executor = "shell"
command = "echo hello"
`
	if err := os.WriteFile(filepath.Join(projectWfDir, "test.meow.toml"), []byte(testWorkflow), 0644); err != nil {
		t.Fatalf("failed to write test workflow: %v", err)
	}

	// Create an internal workflow (should be excluded)
	internalWorkflow := `[main]
name = "internal-workflow"
description = "An internal workflow"
internal = true

[[main.steps]]
id = "step-1"
executor = "shell"
command = "echo internal"
`
	if err := os.WriteFile(filepath.Join(projectWfDir, "internal.meow.toml"), []byte(internalWorkflow), 0644); err != nil {
		t.Fatalf("failed to write internal workflow: %v", err)
	}

	// Create lib directory with library workflow
	libDir := filepath.Join(projectWfDir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	libWorkflow := `[main]
name = "lib-workflow"
description = "A library workflow"

[[main.steps]]
id = "step-1"
executor = "shell"
command = "echo lib"
`
	if err := os.WriteFile(filepath.Join(libDir, "lib-test.meow.toml"), []byte(libWorkflow), 0644); err != nil {
		t.Fatalf("failed to write lib workflow: %v", err)
	}

	// Create loader and test
	loader := &Loader{
		ProjectDir:  tmpDir,
		UserDir:     "", // No user dir for this test
		EmbeddedDir: "workflows",
	}

	available, err := loader.ListAvailable()
	if err != nil {
		t.Fatalf("ListAvailable failed: %v", err)
	}

	// Check project workflows
	projectWfs := available["project"]
	if len(projectWfs) != 1 {
		t.Errorf("expected 1 project workflow, got %d", len(projectWfs))
	}
	if len(projectWfs) > 0 {
		if projectWfs[0].Name != "test" {
			t.Errorf("expected workflow name 'test', got %q", projectWfs[0].Name)
		}
		if projectWfs[0].Description != "A test workflow" {
			t.Errorf("expected description 'A test workflow', got %q", projectWfs[0].Description)
		}
		if projectWfs[0].Source != "project" {
			t.Errorf("expected source 'project', got %q", projectWfs[0].Source)
		}
	}

	// Check library workflows
	libWfs := available["library"]
	if len(libWfs) != 1 {
		t.Errorf("expected 1 library workflow, got %d", len(libWfs))
	}
	if len(libWfs) > 0 {
		if libWfs[0].Name != "lib-test" {
			t.Errorf("expected workflow name 'lib-test', got %q", libWfs[0].Name)
		}
		if libWfs[0].Source != "library" {
			t.Errorf("expected source 'library', got %q", libWfs[0].Source)
		}
	}
}

func TestLoaderListAvailableEmptyDir(t *testing.T) {
	// Create a temp directory with no workflows
	tmpDir, err := os.MkdirTemp("", "meow-loader-empty-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create empty project workflows directory
	projectWfDir := filepath.Join(tmpDir, ".meow", "workflows")
	if err := os.MkdirAll(projectWfDir, 0755); err != nil {
		t.Fatalf("failed to create workflows dir: %v", err)
	}

	loader := &Loader{
		ProjectDir:  tmpDir,
		UserDir:     "",
		EmbeddedDir: "workflows",
	}

	available, err := loader.ListAvailable()
	if err != nil {
		t.Fatalf("ListAvailable failed: %v", err)
	}

	// Should return empty maps (no workflows)
	if len(available["project"]) != 0 {
		t.Errorf("expected 0 project workflows, got %d", len(available["project"]))
	}
}

func TestLoaderListAvailableNonExistentDir(t *testing.T) {
	// Test with a non-existent project directory
	loader := &Loader{
		ProjectDir:  "/non/existent/path",
		UserDir:     "/also/non/existent",
		EmbeddedDir: "workflows",
	}

	available, err := loader.ListAvailable()
	if err != nil {
		t.Fatalf("ListAvailable failed: %v", err)
	}

	// Should return empty maps without error
	if len(available) != 0 {
		t.Errorf("expected empty result for non-existent dirs, got %d entries", len(available))
	}
}

func TestAvailableWorkflowStruct(t *testing.T) {
	wf := AvailableWorkflow{
		Name:        "test",
		Description: "A test workflow",
		Source:      "project",
		Path:        "/path/to/test.meow.toml",
		Internal:    false,
	}

	if wf.Name != "test" {
		t.Errorf("unexpected Name: %s", wf.Name)
	}
	if wf.Description != "A test workflow" {
		t.Errorf("unexpected Description: %s", wf.Description)
	}
	if wf.Source != "project" {
		t.Errorf("unexpected Source: %s", wf.Source)
	}
	if wf.Internal != false {
		t.Error("Internal should be false")
	}
}
