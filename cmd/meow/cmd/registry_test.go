package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akatz-ai/meow/internal/registry"
)

func TestRegistryList_Empty(t *testing.T) {
	// Create temporary directory for registries store
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "registries.json")

	// Create empty store
	_ = registry.NewRegistriesStoreWithPath(storePath)

	// Run registry list command
	cmd := registryListCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := runRegistryList(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "No registries registered") {
		t.Errorf("expected 'No registries registered' in output, got: %s", output)
	}
	if !strings.Contains(output, "meow registry add") {
		t.Errorf("expected 'meow registry add' in output, got: %s", output)
	}
}

func TestRegistryList_WithRegistries(t *testing.T) {
	// Create temporary directory and populate registries
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "registries.json")

	store := registry.NewRegistriesStoreWithPath(storePath)
	if err := store.Add("test-reg", "github.com/test/reg", "1.0.0"); err != nil {
		t.Fatalf("failed to add test registry: %v", err)
	}
	if err := store.Add("another-reg", "https://example.com/reg.git", "2.1.0"); err != nil {
		t.Fatalf("failed to add another registry: %v", err)
	}

	// Run registry list command
	cmd := registryListCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := runRegistryList(cmd, []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	expectedContents := []string{
		"test-reg", "1.0.0", "github.com/test/reg",
		"another-reg", "2.1.0", "https://example.com/reg.git",
	}
	for _, expected := range expectedContents {
		if !strings.Contains(output, expected) {
			t.Errorf("expected %q in output, got: %s", expected, output)
		}
	}
}

func TestRegistryShow_Success(t *testing.T) {
	// Create temporary registry with collections
	tmpDir := t.TempDir()
	registryDir := filepath.Join(tmpDir, "test-reg")
	meowDir := filepath.Join(registryDir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("failed to create meow dir: %v", err)
	}

	// Create registry.json
	reg := registry.Registry{
		Name:        "test-reg",
		Description: "Test Registry",
		Version:     "1.0.0",
		Owner: registry.Owner{
			Name:  "Test Owner",
			Email: "test@example.com",
		},
		Collections: []registry.CollectionEntry{
			{
				Name:        "sprint",
				Description: "Sprint workflow",
				Source:      registry.Source{Path: "./collections/sprint"},
				Tags:        []string{"workflow", "sprint"},
			},
			{
				Name:        "review",
				Description: "Code review workflow",
				Source:      registry.Source{Path: "./collections/review"},
				Tags:        []string{"workflow", "review"},
			},
		},
	}

	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal registry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(meowDir, "registry.json"), data, 0644); err != nil {
		t.Fatalf("failed to write registry.json: %v", err)
	}

	// Create cache and registries store
	cacheDir := filepath.Join(tmpDir, "cache")
	cachedRegDir := filepath.Join(cacheDir, "test-reg")
	if err := os.MkdirAll(cachedRegDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}
	if err := os.Symlink(registryDir, filepath.Join(cachedRegDir, ".meow")); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	storePath := filepath.Join(tmpDir, "registries.json")
	store := registry.NewRegistriesStoreWithPath(storePath)
	if err := store.Add("test-reg", "github.com/test/reg", "1.0.0"); err != nil {
		t.Fatalf("failed to add registry: %v", err)
	}

	// Run registry show command
	cmd := registryShowCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err = runRegistryShow(cmd, []string{"test-reg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	expectedContents := []string{
		"Registry: test-reg", "v1.0.0", "Test Owner", "test@example.com",
		"sprint", "Sprint workflow", "workflow, sprint",
		"review", "Code review workflow",
	}
	for _, expected := range expectedContents {
		if !strings.Contains(output, expected) {
			t.Errorf("expected %q in output, got: %s", expected, output)
		}
	}
}

func TestRegistryShow_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "registries.json")

	_ = registry.NewRegistriesStoreWithPath(storePath)

	cmd := registryShowCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := runRegistryShow(cmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent registry, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestRegistryUpdate_Success(t *testing.T) {
	// Create temporary directory with git repo
	tmpDir := t.TempDir()
	registryDir := filepath.Join(tmpDir, "test-reg")
	meowDir := filepath.Join(registryDir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("failed to create meow dir: %v", err)
	}

	// Create initial registry.json
	reg := registry.Registry{
		Name:        "test-reg",
		Version:     "1.0.0",
		Owner:       registry.Owner{Name: "Test Owner"},
		Collections: []registry.CollectionEntry{},
	}
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal registry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(meowDir, "registry.json"), data, 0644); err != nil {
		t.Fatalf("failed to write registry.json: %v", err)
	}

	// Create cache directory structure
	cacheDir := filepath.Join(tmpDir, "cache")
	cachedRegDir := filepath.Join(cacheDir, "test-reg")
	gitDir := filepath.Join(cachedRegDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create git dir: %v", err)
	}

	// Copy registry to cache
	cachedMeowDir := filepath.Join(cachedRegDir, ".meow")
	if err := os.MkdirAll(cachedMeowDir, 0755); err != nil {
		t.Fatalf("failed to create cached meow dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cachedMeowDir, "registry.json"), data, 0644); err != nil {
		t.Fatalf("failed to write cached registry.json: %v", err)
	}

	// Create registries store
	storePath := filepath.Join(tmpDir, "registries.json")
	store := registry.NewRegistriesStoreWithPath(storePath)
	if err := store.Add("test-reg", "github.com/test/reg", "1.0.0"); err != nil {
		t.Fatalf("failed to add registry: %v", err)
	}

	// Test will fail until implemented
	cmd := registryUpdateCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	_ = runRegistryUpdate(cmd, []string{"test-reg"})
	// For now, just check the command exists
	// Full implementation will be tested after implementation
}

func TestRegistryUpdate_All(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "registries.json")

	store := registry.NewRegistriesStoreWithPath(storePath)
	if err := store.Add("reg1", "github.com/test/reg1", "1.0.0"); err != nil {
		t.Fatalf("failed to add reg1: %v", err)
	}
	if err := store.Add("reg2", "github.com/test/reg2", "2.0.0"); err != nil {
		t.Fatalf("failed to add reg2: %v", err)
	}

	cmd := registryUpdateCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	// Set --all flag
	if err := cmd.Flags().Set("all", "true"); err != nil {
		t.Fatalf("failed to set all flag: %v", err)
	}

	_ = runRegistryUpdate(cmd, []string{})
	// For now, just check the command exists
}

func TestRegistryUpdate_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "registries.json")

	_ = registry.NewRegistriesStoreWithPath(storePath)

	cmd := registryUpdateCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := runRegistryUpdate(cmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent registry, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestRegistryRemove_Success(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "registries.json")

	// Create cache directory
	cacheDir := filepath.Join(tmpDir, "cache", "test-reg")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}

	// Create registries store with a registry
	store := registry.NewRegistriesStoreWithPath(storePath)
	if err := store.Add("test-reg", "github.com/test/reg", "1.0.0"); err != nil {
		t.Fatalf("failed to add registry: %v", err)
	}

	cmd := registryRemoveCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := runRegistryRemove(cmd, []string{"test-reg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test-reg") {
		t.Errorf("expected 'test-reg' in output, got: %s", output)
	}

	// Verify registry was removed
	_, err = store.Get("test-reg")
	if err == nil {
		t.Fatal("expected error for removed registry, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestRegistryRemove_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "registries.json")

	_ = registry.NewRegistriesStoreWithPath(storePath)

	cmd := registryRemoveCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	err := runRegistryRemove(cmd, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for nonexistent registry, got nil")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestRegistryRemove_WithInstalledCollections(t *testing.T) {
	tmpDir := t.TempDir()
	storePath := filepath.Join(tmpDir, "registries.json")
	installedPath := filepath.Join(tmpDir, "installed.json")

	// Create installed collections
	installedStore := registry.NewInstalledStoreWithPath(installedPath)
	if err := installedStore.Add("collection1", registry.InstalledCollection{
		Registry:        "test-reg",
		RegistryVersion: "1.0.0",
		Scope:           registry.ScopeUser,
		Path:            "/tmp/collection1",
	}); err != nil {
		t.Fatalf("failed to add installed collection: %v", err)
	}

	// Create registries store
	store := registry.NewRegistriesStoreWithPath(storePath)
	if err := store.Add("test-reg", "github.com/test/reg", "1.0.0"); err != nil {
		t.Fatalf("failed to add registry: %v", err)
	}

	cmd := registryRemoveCmd
	var buf bytes.Buffer
	cmd.SetOut(&buf)

	err := runRegistryRemove(cmd, []string{"test-reg"})

	// Should warn about installed collections
	output := buf.String()
	if err == nil {
		// If remove succeeds, should show warning
		if !strings.Contains(strings.ToLower(output), "warning") {
			t.Errorf("expected warning about installed collections in output, got: %s", output)
		}
	} else {
		// Or could error out
		if !strings.Contains(strings.ToLower(err.Error()), "collection") {
			t.Errorf("expected 'collection' in error message, got: %v", err)
		}
	}
}
