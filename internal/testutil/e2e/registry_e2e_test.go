package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/akatz-ai/meow/internal/testutil/e2e"
)

// ===========================================================================
// Registry Distribution E2E Tests
// ===========================================================================
//
// These tests verify the complete registry → install → run flow for the
// MEOW distribution system.

// TestE2E_RegistryValidation tests the registry validate command with valid registries.
func TestE2E_RegistryValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)
	registryDir := filepath.Join(h.TempDir, "test-registry")

	// Create a valid registry structure
	createTestRegistry(t, registryDir)

	// Run validation
	stdout, stderr, err := runMeow(h, "registry", "validate", registryDir)
	if err != nil {
		t.Fatalf("registry validate failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify success output
	output := stdout + stderr
	if !strings.Contains(output, "Registry is valid") {
		t.Errorf("expected 'Registry is valid' in output, got:\n%s", output)
	}
}

// TestE2E_RegistryValidationInvalid tests validation catches registry errors.
func TestE2E_RegistryValidationInvalid(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)
	registryDir := filepath.Join(h.TempDir, "invalid-registry")

	// Create registry with missing collection
	createRegistryWithMissingCollection(t, registryDir)

	// Run validation - should fail
	stdout, stderr, err := runMeow(h, "registry", "validate", registryDir)

	// Validation should fail (non-zero exit or error output)
	output := stdout + stderr
	hasError := err != nil || strings.Contains(output, "validation failed") || strings.Contains(output, "Error")

	if !hasError {
		t.Errorf("expected validation to fail for invalid registry, got:\n%s", output)
	}
}

// TestE2E_CollectionValidation tests validating a standalone collection.
func TestE2E_CollectionValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)
	collectionDir := filepath.Join(h.TempDir, "test-collection")

	// Create a valid collection
	createTestCollection(t, collectionDir, "test-workflows")

	// Run validation
	stdout, stderr, err := runMeow(h, "registry", "validate", collectionDir)
	if err != nil {
		t.Fatalf("collection validate failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify success output
	output := stdout + stderr
	if !strings.Contains(output, "is valid") {
		t.Errorf("expected collection validation to succeed, got:\n%s", output)
	}
}

// TestE2E_RunCollectionEntrypoint tests running a collection's main workflow.
func TestE2E_RunCollectionEntrypoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Install collection directly to the workflows directory
	collectionName := "test-workflows"
	collectionDir := filepath.Join(h.TemplateDir, collectionName)
	createTestCollection(t, collectionDir, collectionName)

	// Run the collection by name (should use entrypoint)
	stdout, stderr, err := runMeow(h, "run", collectionName)
	if err != nil {
		t.Fatalf("meow run collection failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify workflow completed
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected 'workflow completed' in output, got:\n%s", stderr)
	}
}

// TestE2E_RunCollectionSubWorkflow tests running a sub-workflow within a collection.
func TestE2E_RunCollectionSubWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Install collection with lib/ subdirectory
	collectionName := "test-workflows"
	collectionDir := filepath.Join(h.TemplateDir, collectionName)
	createTestCollectionWithLib(t, collectionDir, collectionName)

	// Run sub-workflow using collection:path syntax
	stdout, stderr, err := runMeow(h, "run", collectionName+":lib/helper")
	if err != nil {
		t.Fatalf("meow run collection:path failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify workflow completed
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected 'workflow completed' in output, got:\n%s", stderr)
	}
}

// TestE2E_CollectionExpandRelative tests that expand steps within a collection
// resolve templates relative to the collection root.
func TestE2E_CollectionExpandRelative(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Install collection with expand step that references collection-local template
	collectionName := "expand-collection"
	collectionDir := filepath.Join(h.TemplateDir, collectionName)
	createCollectionWithExpand(t, collectionDir, collectionName)

	// Run the collection entrypoint which expands a collection-local template
	stdout, stderr, err := runMeow(h, "run", collectionName)
	if err != nil {
		t.Fatalf("meow run collection with expand failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify workflow completed
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected 'workflow completed' in output, got:\n%s", stderr)
	}
}

// TestE2E_CollectionLsShowsCollections tests that meow ls shows installed collections.
func TestE2E_CollectionLsShowsCollections(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Install a collection
	collectionName := "visible-collection"
	collectionDir := filepath.Join(h.TemplateDir, collectionName)
	createTestCollection(t, collectionDir, collectionName)

	// Run meow ls
	stdout, stderr, err := runMeow(h, "ls")
	if err != nil {
		t.Fatalf("meow ls failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify collection appears in output
	output := stdout + stderr
	if !strings.Contains(output, collectionName) {
		t.Errorf("expected collection %q in ls output, got:\n%s", collectionName, output)
	}

	// Verify it's marked as a collection
	if !strings.Contains(output, "collection") {
		t.Errorf("expected '(collection)' marker in ls output, got:\n%s", output)
	}
}

// TestE2E_CollectionLsJSON tests that meow ls --json includes collection metadata.
func TestE2E_CollectionLsJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Install a collection
	collectionName := "json-collection"
	collectionDir := filepath.Join(h.TemplateDir, collectionName)
	createTestCollection(t, collectionDir, collectionName)

	// Run meow ls --json
	stdout, _, err := runMeow(h, "ls", "--json")
	if err != nil {
		t.Fatalf("meow ls --json failed: %v", err)
	}

	// Parse JSON output
	var entries []struct {
		Workflow     string `json:"workflow"`
		IsCollection bool   `json:"isCollection"`
		Entrypoint   string `json:"entrypoint"`
	}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("failed to parse ls JSON output: %v\noutput: %s", err, stdout)
	}

	// Find our collection
	found := false
	for _, e := range entries {
		if e.Workflow == collectionName {
			found = true
			if !e.IsCollection {
				t.Errorf("expected isCollection=true for %s", collectionName)
			}
			if e.Entrypoint == "" {
				t.Errorf("expected entrypoint to be set for collection")
			}
			break
		}
	}

	if !found {
		t.Errorf("collection %q not found in ls output", collectionName)
	}
}

// TestE2E_CollectionNestedExpand tests deeply nested expand within collections.
func TestE2E_CollectionNestedExpand(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Create collection with nested expand (main -> helper -> inner)
	collectionName := "nested-expand"
	collectionDir := filepath.Join(h.TemplateDir, collectionName)
	createCollectionWithNestedExpand(t, collectionDir, collectionName)

	// Run the collection
	stdout, stderr, err := runMeowWithTimeout(h, 30*time.Second, "run", collectionName)
	if err != nil {
		t.Fatalf("meow run with nested expand failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify workflow completed
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected 'workflow completed' in output, got:\n%s", stderr)
	}
}

// ===========================================================================
// Test Helper Functions
// ===========================================================================

// createTestRegistry creates a valid registry structure.
func createTestRegistry(t *testing.T, dir string) {
	t.Helper()

	// Create .meow/registry.json
	meowDir := filepath.Join(dir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	registryJSON := `{
    "name": "test-registry",
    "description": "Test registry for E2E tests",
    "version": "1.0.0",
    "owner": {
        "name": "Test Author"
    },
    "collectionRoot": "./collections",
    "collections": [
        {
            "name": "test-workflows",
            "source": "test-workflows",
            "description": "Test workflow collection"
        }
    ]
}`
	if err := os.WriteFile(filepath.Join(meowDir, "registry.json"), []byte(registryJSON), 0644); err != nil {
		t.Fatalf("failed to write registry.json: %v", err)
	}

	// Create the collection referenced in registry
	collectionsDir := filepath.Join(dir, "collections")
	if err := os.MkdirAll(collectionsDir, 0755); err != nil {
		t.Fatalf("failed to create collections dir: %v", err)
	}
	createTestCollection(t, filepath.Join(collectionsDir, "test-workflows"), "test-workflows")
}

// createRegistryWithMissingCollection creates a registry referencing a non-existent collection.
func createRegistryWithMissingCollection(t *testing.T, dir string) {
	t.Helper()

	meowDir := filepath.Join(dir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	// Registry references a collection that doesn't exist
	registryJSON := `{
    "name": "invalid-registry",
    "version": "1.0.0",
    "owner": {"name": "Test"},
    "collections": [
        {
            "name": "missing-collection",
            "source": "does-not-exist",
            "description": "This collection doesn't exist"
        }
    ]
}`
	if err := os.WriteFile(filepath.Join(meowDir, "registry.json"), []byte(registryJSON), 0644); err != nil {
		t.Fatalf("failed to write registry.json: %v", err)
	}
}

// createTestCollection creates a simple collection with a main workflow.
func createTestCollection(t *testing.T, dir, name string) {
	t.Helper()

	// Create .meow/manifest.json
	meowDir := filepath.Join(dir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	manifest := map[string]any{
		"name":        name,
		"description": "Test collection for E2E tests",
		"entrypoint":  "main.meow.toml",
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(meowDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatalf("failed to write manifest.json: %v", err)
	}

	// Create main.meow.toml
	mainWorkflow := `[main]
name = "collection-main"
description = "Main workflow for test collection"

[[main.steps]]
id = "test"
executor = "shell"
command = "echo 'Collection entrypoint executed'"
`
	if err := os.WriteFile(filepath.Join(dir, "main.meow.toml"), []byte(mainWorkflow), 0644); err != nil {
		t.Fatalf("failed to write main.meow.toml: %v", err)
	}
}

// createTestCollectionWithLib creates a collection with lib/ subdirectory.
func createTestCollectionWithLib(t *testing.T, dir, name string) {
	t.Helper()

	// Create base collection
	createTestCollection(t, dir, name)

	// Create lib/helper.meow.toml
	libDir := filepath.Join(dir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	helperWorkflow := `[main]
name = "collection-helper"
description = "Helper workflow in lib/"

[[main.steps]]
id = "helper-step"
executor = "shell"
command = "echo 'Helper workflow executed from lib/'"
`
	if err := os.WriteFile(filepath.Join(libDir, "helper.meow.toml"), []byte(helperWorkflow), 0644); err != nil {
		t.Fatalf("failed to write helper.meow.toml: %v", err)
	}
}

// createCollectionWithExpand creates a collection where the main workflow expands a local template.
func createCollectionWithExpand(t *testing.T, dir, name string) {
	t.Helper()

	// Create .meow/manifest.json
	meowDir := filepath.Join(dir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	manifest := map[string]any{
		"name":        name,
		"description": "Collection with expand steps",
		"entrypoint":  "main.meow.toml",
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(meowDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatalf("failed to write manifest.json: %v", err)
	}

	// Create main.meow.toml that expands a collection-local template
	mainWorkflow := `[main]
name = "expand-main"
description = "Main workflow that expands collection-local template"

[[main.steps]]
id = "expand-helper"
executor = "expand"
template = "lib/helper"
`
	if err := os.WriteFile(filepath.Join(dir, "main.meow.toml"), []byte(mainWorkflow), 0644); err != nil {
		t.Fatalf("failed to write main.meow.toml: %v", err)
	}

	// Create lib/helper.meow.toml (the template being expanded)
	libDir := filepath.Join(dir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	helperWorkflow := `[main]
name = "expanded-helper"
description = "Helper workflow expanded from collection"

[[main.steps]]
id = "expanded-step"
executor = "shell"
command = "echo 'Expanded helper executed'"
`
	if err := os.WriteFile(filepath.Join(libDir, "helper.meow.toml"), []byte(helperWorkflow), 0644); err != nil {
		t.Fatalf("failed to write lib/helper.meow.toml: %v", err)
	}
}

// createCollectionWithNestedExpand creates a collection with nested expand steps.
func createCollectionWithNestedExpand(t *testing.T, dir, name string) {
	t.Helper()

	// Create .meow/manifest.json
	meowDir := filepath.Join(dir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	manifest := map[string]any{
		"name":        name,
		"description": "Collection with nested expand",
		"entrypoint":  "main.meow.toml",
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(meowDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatalf("failed to write manifest.json: %v", err)
	}

	// Create main.meow.toml -> expands lib/middle
	mainWorkflow := `[main]
name = "nested-main"
description = "Main with nested expand"

[[main.steps]]
id = "expand-middle"
executor = "expand"
template = "lib/middle"
`
	if err := os.WriteFile(filepath.Join(dir, "main.meow.toml"), []byte(mainWorkflow), 0644); err != nil {
		t.Fatalf("failed to write main.meow.toml: %v", err)
	}

	// Create lib directory
	libDir := filepath.Join(dir, "lib")
	if err := os.MkdirAll(libDir, 0755); err != nil {
		t.Fatalf("failed to create lib dir: %v", err)
	}

	// Create lib/middle.meow.toml -> expands lib/inner
	middleWorkflow := `[main]
name = "middle-workflow"
description = "Middle level that expands inner"

[[main.steps]]
id = "expand-inner"
executor = "expand"
template = "lib/inner"
`
	if err := os.WriteFile(filepath.Join(libDir, "middle.meow.toml"), []byte(middleWorkflow), 0644); err != nil {
		t.Fatalf("failed to write lib/middle.meow.toml: %v", err)
	}

	// Create lib/inner.meow.toml (final shell step)
	innerWorkflow := `[main]
name = "inner-workflow"
description = "Innermost workflow"

[[main.steps]]
id = "inner-step"
executor = "shell"
command = "echo 'Inner step executed'"
`
	if err := os.WriteFile(filepath.Join(libDir, "inner.meow.toml"), []byte(innerWorkflow), 0644); err != nil {
		t.Fatalf("failed to write lib/inner.meow.toml: %v", err)
	}
}
