package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akatz-ai/meow/internal/types"
)

func writeWorkflowModule(t *testing.T, path string, workflowName string) {
	t.Helper()

	content := fmt.Sprintf(`
[%s]
name = %q

[[%s.steps]]
id = "step"
executor = "shell"
command = "echo hello"
`, workflowName, workflowName, workflowName)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}
}

func TestFileTemplateExpander_SubdirectoryWorkflow(t *testing.T) {
	baseDir := t.TempDir()
	modulePath := filepath.Join(baseDir, ".meow", "workflows", "lib", "util.meow.toml")
	writeWorkflowModule(t, modulePath, "monitor")

	expander := NewFileTemplateExpander(baseDir)
	config := &types.ExpandConfig{Template: "lib/util#monitor"}

	result, err := expander.Expand(context.Background(), config, "expand", "", "")
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(result.Steps))
	}

	if result.Steps[0].ID != "expand.step" {
		t.Fatalf("Step ID = %s, want expand.step", result.Steps[0].ID)
	}

	if result.Steps[0].SourceModule != modulePath {
		t.Fatalf("SourceModule = %s, want %s", result.Steps[0].SourceModule, modulePath)
	}
}

// Helper to create a workflow module with expand steps
func writeExpandWorkflowModule(t *testing.T, path string, workflowName string, expandTemplate string) {
	t.Helper()

	var content string
	if expandTemplate != "" {
		content = fmt.Sprintf(`
[%s]
name = %q

[[%s.steps]]
id = "expand-step"
executor = "expand"
template = %q
`, workflowName, workflowName, workflowName, expandTemplate)
	} else {
		content = fmt.Sprintf(`
[%s]
name = %q

[[%s.steps]]
id = "step"
executor = "shell"
command = "echo hello"
`, workflowName, workflowName, workflowName)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}
}

// Helper to write a collection manifest
func writeCollectionManifest(t *testing.T, collectionDir, name, entrypoint string) {
	t.Helper()

	manifest := fmt.Sprintf(`{
  "name": %q,
  "description": "Test collection",
  "entrypoint": %q
}`, name, entrypoint)

	metaDir := filepath.Join(collectionDir, ".meow")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	manifestPath := filepath.Join(metaDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}
}

// Test: Expand within collection finds collection-relative template
// When a workflow from a collection uses template = "lib/helper", it should resolve
// to collection/lib/helper.meow.toml, not globally.
func TestFileTemplateExpander_CollectionRelativeTemplate(t *testing.T) {
	baseDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a collection with a main workflow and a lib/ helper
	collectionDir := filepath.Join(baseDir, ".meow", "workflows", "sprint")
	writeCollectionManifest(t, collectionDir, "sprint", "main.meow.toml")

	// Create the lib/helper.meow.toml inside the collection
	helperPath := filepath.Join(collectionDir, "lib", "helper.meow.toml")
	writeWorkflowModule(t, helperPath, "main")

	// Create a global lib/helper.meow.toml that should NOT be found
	globalHelperPath := filepath.Join(baseDir, ".meow", "workflows", "lib", "helper.meow.toml")
	writeWorkflowModule(t, globalHelperPath, "main")

	// Create expander with collection context
	expander := NewFileTemplateExpander(baseDir)
	expander.CollectionDir = collectionDir // Set collection context

	// Expand "lib/helper" should resolve to collection's lib/helper.meow.toml
	config := &types.ExpandConfig{Template: "lib/helper"}
	result, err := expander.Expand(context.Background(), config, "expand", "", "")
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	// Verify it resolved to collection path, not global
	if len(result.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(result.Steps))
	}

	// The SourceModule should be the collection's lib/helper.meow.toml
	if result.Steps[0].SourceModule != helperPath {
		t.Errorf("SourceModule = %s, want %s (collection-relative)", result.Steps[0].SourceModule, helperPath)
	}
}

// Test: Expand outside collection uses global resolution
func TestFileTemplateExpander_GlobalResolutionWithoutCollection(t *testing.T) {
	baseDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a global lib/helper.meow.toml
	globalHelperPath := filepath.Join(baseDir, ".meow", "workflows", "lib", "helper.meow.toml")
	writeWorkflowModule(t, globalHelperPath, "main")

	// Create expander WITHOUT collection context
	expander := NewFileTemplateExpander(baseDir)
	// expander.CollectionDir is empty - no collection context

	// Expand "lib/helper" should resolve to global lib/helper.meow.toml
	config := &types.ExpandConfig{Template: "lib/helper"}
	result, err := expander.Expand(context.Background(), config, "expand", "", "")
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	// Verify it resolved to global path
	if len(result.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(result.Steps))
	}

	if result.Steps[0].SourceModule != globalHelperPath {
		t.Errorf("SourceModule = %s, want %s (global)", result.Steps[0].SourceModule, globalHelperPath)
	}
}

// Test: Nested expand maintains collection context
func TestFileTemplateExpander_NestedExpandMaintainsCollectionContext(t *testing.T) {
	baseDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a collection with nested expand:
	// main.meow.toml -> expands lib/level1 -> expands lib/level2
	collectionDir := filepath.Join(baseDir, ".meow", "workflows", "sprint")
	writeCollectionManifest(t, collectionDir, "sprint", "main.meow.toml")

	// Create lib/level2.meow.toml (leaf)
	level2Path := filepath.Join(collectionDir, "lib", "level2.meow.toml")
	writeWorkflowModule(t, level2Path, "main")

	// Create lib/level1.meow.toml that expands lib/level2
	level1Path := filepath.Join(collectionDir, "lib", "level1.meow.toml")
	writeExpandWorkflowModule(t, level1Path, "main", "lib/level2")

	// Create a global lib/level2.meow.toml that should NOT be found
	globalLevel2Path := filepath.Join(baseDir, ".meow", "workflows", "lib", "level2.meow.toml")
	writeWorkflowModule(t, globalLevel2Path, "main")

	// Create expander with collection context
	expander := NewFileTemplateExpander(baseDir)
	expander.CollectionDir = collectionDir

	// Expand "lib/level1" - which then expands "lib/level2"
	config := &types.ExpandConfig{Template: "lib/level1"}
	result, err := expander.Expand(context.Background(), config, "expand", "", "")
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	// The result should have the expand step from level1
	// When that expand step runs, it should resolve lib/level2 within the collection
	if len(result.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(result.Steps))
	}

	// Verify the step is an expand step pointing to lib/level2
	step := result.Steps[0]
	if step.Executor != types.ExecutorExpand {
		t.Fatalf("Expected expand executor, got %s", step.Executor)
	}
	if step.Expand == nil || step.Expand.Template != "lib/level2" {
		t.Fatalf("Expected expand template lib/level2, got %v", step.Expand)
	}

	// The SourceModule should be set to level1 so nested expansions can resolve
	if step.SourceModule != level1Path {
		t.Errorf("SourceModule = %s, want %s", step.SourceModule, level1Path)
	}

	// Verify CollectionDir is preserved for nested expansion
	if step.CollectionDir != collectionDir {
		t.Errorf("CollectionDir = %s, want %s (should propagate to nested steps)", step.CollectionDir, collectionDir)
	}
}

// Test: Global templates still work when not in collection (fallback)
func TestFileTemplateExpander_CollectionFallsBackToGlobal(t *testing.T) {
	baseDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a collection WITHOUT a lib/helper.meow.toml
	collectionDir := filepath.Join(baseDir, ".meow", "workflows", "sprint")
	writeCollectionManifest(t, collectionDir, "sprint", "main.meow.toml")
	mainPath := filepath.Join(collectionDir, "main.meow.toml")
	writeWorkflowModule(t, mainPath, "main")

	// Create a global lib/helper.meow.toml
	globalHelperPath := filepath.Join(baseDir, ".meow", "workflows", "lib", "helper.meow.toml")
	writeWorkflowModule(t, globalHelperPath, "main")

	// Create expander with collection context
	expander := NewFileTemplateExpander(baseDir)
	expander.CollectionDir = collectionDir

	// Expand "lib/helper" - not in collection, should fall back to global
	config := &types.ExpandConfig{Template: "lib/helper"}
	result, err := expander.Expand(context.Background(), config, "expand", "", "")
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	// Should resolve to global path
	if len(result.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(result.Steps))
	}

	if result.Steps[0].SourceModule != globalHelperPath {
		t.Errorf("SourceModule = %s, want %s (global fallback)", result.Steps[0].SourceModule, globalHelperPath)
	}
}

// Test: Error messages indicate collection-relative search
func TestFileTemplateExpander_CollectionErrorMessage(t *testing.T) {
	baseDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a collection without the template
	collectionDir := filepath.Join(baseDir, ".meow", "workflows", "sprint")
	writeCollectionManifest(t, collectionDir, "sprint", "main.meow.toml")
	mainPath := filepath.Join(collectionDir, "main.meow.toml")
	writeWorkflowModule(t, mainPath, "main")

	// Create expander with collection context
	expander := NewFileTemplateExpander(baseDir)
	expander.CollectionDir = collectionDir

	// Try to expand a template that doesn't exist
	config := &types.ExpandConfig{Template: "lib/missing"}
	_, err := expander.Expand(context.Background(), config, "expand", "", "")
	if err == nil {
		t.Fatal("Expected error for missing template")
	}

	// Error should indicate collection was searched
	errStr := err.Error()
	if !strings.Contains(errStr, collectionDir) && !strings.Contains(errStr, "collection") {
		t.Errorf("Error should indicate collection search path: %s", errStr)
	}
}
