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

func TestLsShowsConflicts(t *testing.T) {
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

	// Both versions should be shown
	if !strings.Contains(output, "shared") {
		t.Fatalf("Expected shared workflow in output: %s", output)
	}
	if !strings.Contains(output, "project") {
		t.Fatalf("Expected project source for shared workflow: %s", output)
	}
	if !strings.Contains(output, "user") {
		t.Fatalf("Expected user source for shared workflow: %s", output)
	}
	// Both should be marked with conflict indicator
	if !strings.Contains(output, "project *") {
		t.Fatalf("Expected conflict marker on project workflow: %s", output)
	}
	if !strings.Contains(output, "user *") {
		t.Fatalf("Expected conflict marker on user workflow: %s", output)
	}
	// Should show the hint about @scope
	if !strings.Contains(output, "@scope") {
		t.Fatalf("Expected @scope hint in output: %s", output)
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

// writeCollection creates a collection directory with .meow/manifest.json
func writeCollection(t *testing.T, baseDir, name, description, entrypoint string) string {
	t.Helper()

	collectionDir := filepath.Join(baseDir, ".meow", "workflows", name)
	manifestPath := filepath.Join(collectionDir, ".meow", "manifest.json")

	manifest := fmt.Sprintf(`{
  "name": "%s",
  "description": "%s",
  "entrypoint": "%s"
}`, name, description, entrypoint)

	if err := os.MkdirAll(filepath.Dir(manifestPath), 0755); err != nil {
		t.Fatalf("failed to create collection dir: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Create the entrypoint workflow file
	entrypointPath := filepath.Join(collectionDir, entrypoint)
	content := `
[main]
name = "main"
description = "Collection entrypoint"

[[main.steps]]
id = "step1"
executor = "shell"
command = "echo hello"
`
	if err := os.MkdirAll(filepath.Dir(entrypointPath), 0755); err != nil {
		t.Fatalf("failed to create entrypoint dir: %v", err)
	}
	if err := os.WriteFile(entrypointPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write entrypoint: %v", err)
	}

	return collectionDir
}

func TestLsShowsCollections(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)
	writeCollection(t, tmpDir, "my-collection", "My test collection", "main.meow.toml")

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

	if !strings.Contains(output, "my-collection") {
		t.Fatalf("Expected collection name in output: %s", output)
	}
	if !strings.Contains(output, "(collection)") {
		t.Fatalf("Expected (collection) suffix in output: %s", output)
	}
	if !strings.Contains(output, "My test collection") {
		t.Fatalf("Expected collection description in output: %s", output)
	}
}

func TestLsMixedWorkflowsAndCollections(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)
	writeWorkflowFile(t, tmpDir, "standalone", "Standalone workflow")
	writeCollection(t, tmpDir, "my-collection", "My collection", "main.meow.toml")

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

	// Both should appear
	if !strings.Contains(output, "standalone") {
		t.Fatalf("Expected standalone workflow in output: %s", output)
	}
	if !strings.Contains(output, "my-collection") {
		t.Fatalf("Expected collection in output: %s", output)
	}

	// Only collection should have (collection) suffix
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if strings.Contains(line, "standalone") && strings.Contains(line, "(collection)") {
			t.Fatalf("Standalone workflow should not have (collection) suffix: %s", line)
		}
		if strings.Contains(line, "my-collection") && !strings.Contains(line, "(collection)") {
			t.Fatalf("Collection should have (collection) suffix: %s", line)
		}
	}
}

func TestLsCollectionJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)
	writeCollection(t, tmpDir, "json-collection", "JSON collection", "main.meow.toml")

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
		t.Fatalf("Expected 1 collection, got %d", len(result))
	}

	if result[0]["workflow"] != "json-collection" {
		t.Fatalf("Expected workflow 'json-collection', got %v", result[0]["workflow"])
	}

	if result[0]["description"] != "JSON collection" {
		t.Fatalf("Expected description 'JSON collection', got %v", result[0]["description"])
	}

	// Check for isCollection field
	isCollection, ok := result[0]["isCollection"]
	if !ok {
		t.Fatalf("Expected isCollection field in JSON output")
	}
	if isCollection != true {
		t.Fatalf("Expected isCollection to be true, got %v", isCollection)
	}

	// Check for entrypoint field
	entrypoint, ok := result[0]["entrypoint"]
	if !ok {
		t.Fatalf("Expected entrypoint field in JSON output")
	}
	if entrypoint != "main.meow.toml" {
		t.Fatalf("Expected entrypoint 'main.meow.toml', got %v", entrypoint)
	}
}

func TestLsSkipsDirectoriesWithoutManifest(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	// Create a directory without manifest
	regularDir := filepath.Join(tmpDir, ".meow", "workflows", "lib")
	if err := os.MkdirAll(regularDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	// Create a workflow inside it
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

	// Should not show "lib" as a collection
	if strings.Contains(output, "lib") && !strings.Contains(output, "lib/sub") {
		t.Fatalf("Should not show lib as a collection: %s", output)
	}
}

func TestLsCollectionUsesManifestName(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	// Create a collection where dir name differs from manifest name
	collectionDir := filepath.Join(tmpDir, ".meow", "workflows", "dir-name")
	manifestPath := filepath.Join(collectionDir, ".meow", "manifest.json")

	manifest := `{
  "name": "manifest-name",
  "description": "Uses manifest name",
  "entrypoint": "main.meow.toml"
}`

	if err := os.MkdirAll(filepath.Dir(manifestPath), 0755); err != nil {
		t.Fatalf("failed to create collection dir: %v", err)
	}
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Create entrypoint
	entrypointPath := filepath.Join(collectionDir, "main.meow.toml")
	content := `
[main]
name = "main"
description = "Test"

[[main.steps]]
id = "step1"
executor = "shell"
command = "echo hello"
`
	if err := os.WriteFile(entrypointPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write entrypoint: %v", err)
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

	// Should show manifest name, not dir name
	if !strings.Contains(output, "manifest-name") {
		t.Fatalf("Expected manifest name 'manifest-name' in output: %s", output)
	}
	if strings.Contains(output, "dir-name") {
		t.Fatalf("Should not show dir name 'dir-name' in output: %s", output)
	}
}

func TestLsAllWithCollections(t *testing.T) {
	tmpDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	// Create a standalone workflow
	writeWorkflowFile(t, tmpDir, "standalone", "Standalone workflow")

	// Create a collection
	writeCollection(t, tmpDir, "my-collection", "My collection", "main.meow.toml")

	// Create a workflow inside the collection
	collectionDir := filepath.Join(tmpDir, ".meow", "workflows", "my-collection")
	workflowPath := filepath.Join(collectionDir, "sub-workflow.meow.toml")
	content := `
[main]
name = "main"
description = "Sub workflow"

[[main.steps]]
id = "step1"
executor = "shell"
command = "echo hello"
`
	if err := os.WriteFile(workflowPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write sub-workflow: %v", err)
	}

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

	// Should show standalone workflow
	if !strings.Contains(output, "standalone") {
		t.Fatalf("Expected standalone workflow in output: %s", output)
	}

	// Should show the collection
	if !strings.Contains(output, "my-collection") {
		t.Fatalf("Expected collection in output: %s", output)
	}
	if !strings.Contains(output, "(collection)") {
		t.Fatalf("Expected (collection) suffix for collection: %s", output)
	}

	// Should NOT show workflows inside collections in basic ls -a
	// (Collections are opaque - you run the entrypoint, not individual workflows inside)
	if strings.Contains(output, "sub-workflow") {
		t.Fatalf("Should not show workflows inside collection: %s", output)
	}
}
