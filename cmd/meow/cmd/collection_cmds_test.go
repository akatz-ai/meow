package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akatz-ai/meow/internal/registry"
)

// Tests for meow-4wcf: Implement meow collection list/show/remove Commands

// Test data - simple test manifest and workflow
const testManifest = `{
  "name": "test-collection",
  "description": "A test collection",
  "entrypoint": "main.meow.toml"
}`

const testWorkflowSimple = `
[meta]
name = "test-workflow"
version = "1.0.0"

[[steps]]
id = "start"
executor = "shell"
command = "echo test"
`

// setupTestCollection creates a minimal test collection structure
func setupTestCollection(t *testing.T, home, name string) string {
	t.Helper()

	// Create collection directory
	collectionPath := filepath.Join(home, ".meow", "workflows", name)
	if err := os.MkdirAll(collectionPath, 0755); err != nil {
		t.Fatalf("failed to create collection dir: %v", err)
	}

	// Create .meow/manifest.json
	meowDir := filepath.Join(collectionPath, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	manifestPath := filepath.Join(meowDir, "manifest.json")
	manifest := strings.Replace(testManifest, "test-collection", name, 1)
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}

	// Create a workflow file
	workflowPath := filepath.Join(collectionPath, "main.meow.toml")
	if err := os.WriteFile(workflowPath, []byte(testWorkflowSimple), 0644); err != nil {
		t.Fatalf("failed to write workflow: %v", err)
	}

	return collectionPath
}

// TestCollectionListEmpty tests listing when no collections are installed
func TestCollectionListEmpty(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	var buf bytes.Buffer
	collectionListCmd.SetOut(&buf)
	collectionListCmd.SetErr(&buf)

	err := collectionListCmd.RunE(collectionListCmd, []string{})
	if err != nil {
		t.Fatalf("collectionListCmd.RunE() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No collections installed") {
		t.Errorf("expected 'No collections installed' message, got: %q", output)
	}
	if !strings.Contains(output, "meow install") {
		t.Errorf("expected help message with 'meow install', got: %q", output)
	}
}

// TestCollectionListWithCollections tests listing installed collections
func TestCollectionListWithCollections(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Set up two test collections
	setupTestCollection(t, home, "collection-one")
	setupTestCollection(t, home, "collection-two")

	// Track them in installed.json
	store, err := registry.NewInstalledStore()
	if err != nil {
		t.Fatalf("failed to create installed store: %v", err)
	}

	err = store.Add("collection-one", registry.InstalledCollection{
		Registry:        "test-registry",
		RegistryVersion: "1.0.0",
		Path:            filepath.Join(home, ".meow", "workflows", "collection-one"),
		Scope:           registry.ScopeUser,
	})
	if err != nil {
		t.Fatalf("failed to add collection-one: %v", err)
	}

	err = store.Add("collection-two", registry.InstalledCollection{
		Source: "https://github.com/test/collection-two",
		Path:   filepath.Join(home, ".meow", "workflows", "collection-two"),
		Scope:  registry.ScopeUser,
	})
	if err != nil {
		t.Fatalf("failed to add collection-two: %v", err)
	}

	var buf bytes.Buffer
	collectionListCmd.SetOut(&buf)
	collectionListCmd.SetErr(&buf)

	err = collectionListCmd.RunE(collectionListCmd, []string{})
	if err != nil {
		t.Fatalf("collectionListCmd.RunE() error = %v", err)
	}

	output := buf.String()

	// Should show header
	if !strings.Contains(output, "COLLECTION") {
		t.Errorf("expected COLLECTION header, got: %q", output)
	}

	// Should show both collections
	if !strings.Contains(output, "collection-one") {
		t.Errorf("expected collection-one in output, got: %q", output)
	}
	if !strings.Contains(output, "collection-two") {
		t.Errorf("expected collection-two in output, got: %q", output)
	}

	// Should show registry for collection-one
	if !strings.Contains(output, "test-registry") {
		t.Errorf("expected test-registry in output, got: %q", output)
	}

	// Should show source for collection-two
	if !strings.Contains(output, "github.com/test/collection-two") {
		t.Errorf("expected github.com/test/collection-two in output, got: %q", output)
	}
}

// TestCollectionShowWithDetails tests showing collection details
func TestCollectionShowWithDetails(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Set up test collection
	collectionPath := setupTestCollection(t, home, "test-collection")

	// Track it in installed.json
	store, err := registry.NewInstalledStore()
	if err != nil {
		t.Fatalf("failed to create installed store: %v", err)
	}

	err = store.Add("test-collection", registry.InstalledCollection{
		Registry:        "test-registry",
		RegistryVersion: "2.0.0",
		Path:            collectionPath,
		Scope:           registry.ScopeUser,
	})
	if err != nil {
		t.Fatalf("failed to add test-collection: %v", err)
	}

	var buf bytes.Buffer
	collectionShowCmd.SetOut(&buf)
	collectionShowCmd.SetErr(&buf)

	err = collectionShowCmd.RunE(collectionShowCmd, []string{"test-collection"})
	if err != nil {
		t.Fatalf("collectionShowCmd.RunE() error = %v", err)
	}

	output := buf.String()

	// Should show all details
	if !strings.Contains(output, "Collection: test-collection") {
		t.Errorf("expected collection name, got: %q", output)
	}
	if !strings.Contains(output, "Description: A test collection") {
		t.Errorf("expected description, got: %q", output)
	}
	if !strings.Contains(output, "Entrypoint: main.meow.toml") {
		t.Errorf("expected entrypoint, got: %q", output)
	}
	if !strings.Contains(output, "Path:") {
		t.Errorf("expected path, got: %q", output)
	}
	if !strings.Contains(output, "Scope: user") {
		t.Errorf("expected scope, got: %q", output)
	}
	if !strings.Contains(output, "Registry: test-registry") {
		t.Errorf("expected registry, got: %q", output)
	}
	if !strings.Contains(output, "v2.0.0") {
		t.Errorf("expected registry version, got: %q", output)
	}
	if !strings.Contains(output, "Installed:") {
		t.Errorf("expected installed timestamp, got: %q", output)
	}

	// Should list workflows
	if !strings.Contains(output, "Workflows:") {
		t.Errorf("expected workflows section, got: %q", output)
	}
	if !strings.Contains(output, "test-collection:main") {
		t.Errorf("expected workflow listed, got: %q", output)
	}
}

// TestCollectionShowNotFound tests showing a collection that doesn't exist
func TestCollectionShowNotFound(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	var buf bytes.Buffer
	collectionShowCmd.SetOut(&buf)
	collectionShowCmd.SetErr(&buf)

	err := collectionShowCmd.RunE(collectionShowCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent collection")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error should mention 'not installed', got: %v", err)
	}
}

// TestCollectionRemoveSuccess tests successfully removing a collection
func TestCollectionRemoveSuccess(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Set up test collection
	collectionPath := setupTestCollection(t, home, "test-collection")

	// Track it in installed.json
	store, err := registry.NewInstalledStore()
	if err != nil {
		t.Fatalf("failed to create installed store: %v", err)
	}

	err = store.Add("test-collection", registry.InstalledCollection{
		Path:  collectionPath,
		Scope: registry.ScopeUser,
	})
	if err != nil {
		t.Fatalf("failed to add test-collection: %v", err)
	}

	var buf bytes.Buffer
	collectionRemoveCmd.SetOut(&buf)
	collectionRemoveCmd.SetErr(&buf)

	err = collectionRemoveCmd.RunE(collectionRemoveCmd, []string{"test-collection"})
	if err != nil {
		t.Fatalf("collectionRemoveCmd.RunE() error = %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Removed collection") || !strings.Contains(output, "test-collection") {
		t.Errorf("expected success message, got: %q", output)
	}

	// Verify collection directory is deleted
	if _, err := os.Stat(collectionPath); !os.IsNotExist(err) {
		t.Errorf("collection directory should be deleted")
	}

	// Verify collection is removed from tracking
	c, err := store.Get("test-collection")
	if err != nil {
		t.Fatalf("failed to check if collection exists: %v", err)
	}
	if c != nil {
		t.Error("collection should be removed from installed.json")
	}
}

// TestCollectionRemoveNotFound tests removing a collection that doesn't exist
func TestCollectionRemoveNotFound(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	var buf bytes.Buffer
	collectionRemoveCmd.SetOut(&buf)
	collectionRemoveCmd.SetErr(&buf)

	err := collectionRemoveCmd.RunE(collectionRemoveCmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent collection")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error should mention 'not installed', got: %v", err)
	}
}

// TestCollectionShowMissingManifest tests showing a collection with missing manifest
func TestCollectionShowMissingManifest(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create collection directory without manifest
	collectionPath := filepath.Join(home, ".meow", "workflows", "broken-collection")
	if err := os.MkdirAll(collectionPath, 0755); err != nil {
		t.Fatalf("failed to create collection dir: %v", err)
	}

	// Track it in installed.json
	store, err := registry.NewInstalledStore()
	if err != nil {
		t.Fatalf("failed to create installed store: %v", err)
	}

	err = store.Add("broken-collection", registry.InstalledCollection{
		Path:  collectionPath,
		Scope: registry.ScopeUser,
	})
	if err != nil {
		t.Fatalf("failed to add broken-collection: %v", err)
	}

	var buf bytes.Buffer
	collectionShowCmd.SetOut(&buf)
	collectionShowCmd.SetErr(&buf)

	err = collectionShowCmd.RunE(collectionShowCmd, []string{"broken-collection"})
	if err == nil {
		t.Fatal("expected error for missing manifest")
	}
	if !strings.Contains(err.Error(), "manifest") {
		t.Errorf("error should mention manifest, got: %v", err)
	}
}
