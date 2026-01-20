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

// createTestRegistry creates a test registry structure at the given directory.
// Returns the registry metadata for verification.
func createTestRegistry(t *testing.T, dir string) *registry.Registry {
	t.Helper()

	reg := &registry.Registry{
		Name:        "test-registry",
		Description: "A test registry",
		Version:     "1.0.0",
		Owner: registry.Owner{
			Name:  "Test Owner",
			Email: "test@example.com",
		},
		Collections: []registry.CollectionEntry{
			{
				Name:        "test-collection",
				Description: "A test collection",
				Source: registry.Source{
					Path: "./collections/test-collection",
				},
			},
			{
				Name:        "another-collection",
				Description: "Another test collection",
				Source: registry.Source{
					Path: "./collections/another",
				},
			},
		},
	}

	// Create .meow directory
	meowDir := filepath.Join(dir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	// Write registry.json
	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal registry: %v", err)
	}

	if err := os.WriteFile(filepath.Join(meowDir, "registry.json"), data, 0644); err != nil {
		t.Fatalf("failed to write registry.json: %v", err)
	}

	// Create a fake .git directory (so it looks like a git repo)
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	return reg
}

// TestRegistryAddSuccess tests successful registry addition.
func TestRegistryAddSuccess(t *testing.T) {
	// Set up test environment with temp HOME
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a test registry
	registryDir := t.TempDir()
	expectedReg := createTestRegistry(t, registryDir)

	// Run registry add (using local path for testing)
	var buf bytes.Buffer
	registryAddCmd.SetOut(&buf)
	registryAddCmd.SetErr(&buf)

	err := runRegistryAdd(registryAddCmd, []string{registryDir})
	if err != nil {
		t.Fatalf("runRegistryAdd() error = %v", err)
	}

	// Verify registry was added to ~/.meow/registries.json
	registriesPath := filepath.Join(home, ".meow", "registries.json")
	data, err := os.ReadFile(registriesPath)
	if err != nil {
		t.Fatalf("failed to read registries.json: %v", err)
	}

	var registriesFile registry.RegistriesFile
	if err := json.Unmarshal(data, &registriesFile); err != nil {
		t.Fatalf("failed to parse registries.json: %v", err)
	}

	// Verify the registry was registered
	reg, exists := registriesFile.Registries[expectedReg.Name]
	if !exists {
		t.Fatalf("registry %q not found in registries.json", expectedReg.Name)
	}

	if reg.Source != registryDir {
		t.Errorf("expected source %q, got %q", registryDir, reg.Source)
	}

	if reg.Version != expectedReg.Version {
		t.Errorf("expected version %q, got %q", expectedReg.Version, reg.Version)
	}

	// Verify registry was cloned to cache
	cacheDir := filepath.Join(home, ".cache", "meow", "registries", expectedReg.Name)
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		t.Errorf("registry cache should exist at %s", cacheDir)
	}

	// Verify registry.json exists in cache
	cachedRegistryPath := filepath.Join(cacheDir, ".meow", "registry.json")
	if _, err := os.Stat(cachedRegistryPath); os.IsNotExist(err) {
		t.Errorf("cached registry.json should exist at %s", cachedRegistryPath)
	}

	// Verify output
	output := buf.String()
	if !strings.Contains(output, "Added registry") {
		t.Errorf("output should mention 'Added registry', got: %q", output)
	}
	if !strings.Contains(output, expectedReg.Name) {
		t.Errorf("output should mention registry name %q, got: %q", expectedReg.Name, output)
	}
	if !strings.Contains(output, expectedReg.Version) {
		t.Errorf("output should mention version %q, got: %q", expectedReg.Version, output)
	}
	if !strings.Contains(output, "2 collections available") {
		t.Errorf("output should mention collection count, got: %q", output)
	}
}

// TestRegistryAddDuplicate tests adding a registry that's already registered.
func TestRegistryAddDuplicate(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a test registry
	registryDir := t.TempDir()
	createTestRegistry(t, registryDir)

	// Add it once
	var buf bytes.Buffer
	registryAddCmd.SetOut(&buf)
	registryAddCmd.SetErr(&buf)

	err := runRegistryAdd(registryAddCmd, []string{registryDir})
	if err != nil {
		t.Fatalf("first runRegistryAdd() error = %v", err)
	}

	// Try to add it again
	buf.Reset()
	err = runRegistryAdd(registryAddCmd, []string{registryDir})
	if err == nil {
		t.Fatal("expected error when adding duplicate registry")
	}

	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("error should mention 'already registered', got: %v", err)
	}

	// Should suggest update command
	if !strings.Contains(err.Error(), "meow registry update") {
		t.Errorf("error should suggest update command, got: %v", err)
	}
}

// TestRegistryAddInvalidRegistryJSON tests adding a registry with invalid registry.json.
func TestRegistryAddInvalidRegistryJSON(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a directory with invalid registry.json
	registryDir := t.TempDir()
	meowDir := filepath.Join(registryDir, ".meow")
	os.MkdirAll(meowDir, 0755)

	// Invalid JSON
	os.WriteFile(filepath.Join(meowDir, "registry.json"), []byte("{ invalid json }"), 0644)

	var buf bytes.Buffer
	registryAddCmd.SetOut(&buf)
	registryAddCmd.SetErr(&buf)

	err := runRegistryAdd(registryAddCmd, []string{registryDir})
	if err == nil {
		t.Fatal("expected error for invalid registry.json")
	}

	if !strings.Contains(err.Error(), "loading registry") && !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error should mention loading/parsing error, got: %v", err)
	}
}

// TestRegistryAddMissingRegistryJSON tests adding a registry without registry.json.
func TestRegistryAddMissingRegistryJSON(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create an empty directory
	registryDir := t.TempDir()

	var buf bytes.Buffer
	registryAddCmd.SetOut(&buf)
	registryAddCmd.SetErr(&buf)

	err := runRegistryAdd(registryAddCmd, []string{registryDir})
	if err == nil {
		t.Fatal("expected error for missing registry.json")
	}

	if !strings.Contains(err.Error(), "loading registry") && !strings.Contains(err.Error(), "registry.json") {
		t.Errorf("error should mention missing registry.json, got: %v", err)
	}
}

// TestRegistryAddValidationError tests adding a registry that fails validation.
func TestRegistryAddValidationError(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a registry with missing required fields
	registryDir := t.TempDir()
	meowDir := filepath.Join(registryDir, ".meow")
	os.MkdirAll(meowDir, 0755)

	invalidReg := &registry.Registry{
		// Missing Name (required)
		Description: "Test",
		Version:     "", // Missing Version (required)
		Owner: registry.Owner{
			Name: "", // Missing Owner.Name (required)
		},
	}

	data, _ := json.Marshal(invalidReg)
	os.WriteFile(filepath.Join(meowDir, "registry.json"), data, 0644)

	var buf bytes.Buffer
	registryAddCmd.SetOut(&buf)
	registryAddCmd.SetErr(&buf)

	err := runRegistryAdd(registryAddCmd, []string{registryDir})
	if err == nil {
		t.Fatal("expected error for invalid registry")
	}

	if !strings.Contains(err.Error(), "invalid registry") {
		t.Errorf("error should mention 'invalid registry', got: %v", err)
	}
}

// TestRegistryAddOutputFormat tests the output format of registry add.
func TestRegistryAddOutputFormat(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	registryDir := t.TempDir()
	expectedReg := createTestRegistry(t, registryDir)

	var buf bytes.Buffer
	registryAddCmd.SetOut(&buf)
	registryAddCmd.SetErr(&buf)

	err := runRegistryAdd(registryAddCmd, []string{registryDir})
	if err != nil {
		t.Fatalf("runRegistryAdd() error = %v", err)
	}

	output := buf.String()

	// Should show fetching message
	if !strings.Contains(output, "Fetching registry") {
		t.Errorf("output should show fetching message, got: %q", output)
	}

	// Should show owner name
	if !strings.Contains(output, expectedReg.Owner.Name) {
		t.Errorf("output should show owner name %q, got: %q", expectedReg.Owner.Name, output)
	}

	// Should list collection names
	for _, col := range expectedReg.Collections {
		if !strings.Contains(output, col.Name) {
			t.Errorf("output should list collection %q, got: %q", col.Name, output)
		}
	}

	// Should show next steps
	if !strings.Contains(output, "meow registry show") {
		t.Errorf("output should suggest 'meow registry show' command, got: %q", output)
	}
	if !strings.Contains(output, "meow install") {
		t.Errorf("output should suggest 'meow install' command, got: %q", output)
	}
}

// TestRegistryAddGitHubShorthand tests adding a registry using GitHub shorthand.
// Note: This test will be skipped in CI or when git is unavailable.
func TestRegistryAddGitHubShorthand(t *testing.T) {
	// Skip if we can't do network calls
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	// This test would require actual git clone, which we can't test in unit tests
	// without a real registry. We'll handle this in integration/E2E tests.
	t.Skip("GitHub shorthand requires network access, tested in E2E")
}

// TestRegistryAddCleansUpOnError tests that failed registry add cleans up cache.
func TestRegistryAddCleansUpOnError(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a registry with validation errors
	registryDir := t.TempDir()
	meowDir := filepath.Join(registryDir, ".meow")
	os.MkdirAll(meowDir, 0755)

	invalidReg := &registry.Registry{
		Name:    "", // Invalid: missing name
		Version: "1.0.0",
		Owner: registry.Owner{
			Name: "Test",
		},
	}

	data, _ := json.Marshal(invalidReg)
	os.WriteFile(filepath.Join(meowDir, "registry.json"), data, 0644)

	var buf bytes.Buffer
	registryAddCmd.SetOut(&buf)
	registryAddCmd.SetErr(&buf)

	err := runRegistryAdd(registryAddCmd, []string{registryDir})
	if err == nil {
		t.Fatal("expected error for invalid registry")
	}

	// Verify temp cache was cleaned up (no _temp_ directories)
	cacheBaseDir := filepath.Join(home, ".cache", "meow", "registries")
	if entries, err := os.ReadDir(cacheBaseDir); err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "_temp_") {
				t.Errorf("temp cache directory should be cleaned up, found: %s", entry.Name())
			}
		}
	}
}
