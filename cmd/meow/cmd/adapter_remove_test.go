package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meow-stack/meow-machine/internal/adapter/builtin"
)

func TestAdapterRemoveBuiltinRejectsRemoval(t *testing.T) {
	for _, name := range builtin.BuiltinAdapterNames() {
		err := runAdapterRemove(adapterRemoveCmd, []string{name})
		if err == nil {
			t.Errorf("expected error when removing built-in adapter %q", name)
		}
		if !strings.Contains(err.Error(), "cannot remove built-in adapter") {
			t.Errorf("expected 'cannot remove built-in adapter' error, got: %v", err)
		}
	}
}

func TestAdapterRemoveMutuallyExclusiveFlags(t *testing.T) {
	// Save and restore flag values
	oldProject := adapterRemoveProject
	oldGlobal := adapterRemoveGlobal
	defer func() {
		adapterRemoveProject = oldProject
		adapterRemoveGlobal = oldGlobal
	}()

	adapterRemoveProject = true
	adapterRemoveGlobal = true

	err := runAdapterRemove(adapterRemoveCmd, []string{"test-adapter"})
	if err == nil {
		t.Fatal("expected error when --project and --global are both set")
	}

	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' error, got: %v", err)
	}
}

func TestAdapterRemoveNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore flag values
	oldProject := adapterRemoveProject
	oldGlobal := adapterRemoveGlobal
	oldWorkDir := workDir
	defer func() {
		adapterRemoveProject = oldProject
		adapterRemoveGlobal = oldGlobal
		workDir = oldWorkDir
	}()

	adapterRemoveProject = false
	adapterRemoveGlobal = false
	workDir = tmpDir

	// Create .meow directory structure but no adapters
	meowDir := filepath.Join(tmpDir, ".meow", "adapters")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := runAdapterRemove(adapterRemoveCmd, []string{"nonexistent-adapter"})
	if err == nil {
		t.Fatal("expected error when adapter not found")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestAdapterRemoveProjectNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore flag values
	oldProject := adapterRemoveProject
	oldGlobal := adapterRemoveGlobal
	oldWorkDir := workDir
	defer func() {
		adapterRemoveProject = oldProject
		adapterRemoveGlobal = oldGlobal
		workDir = oldWorkDir
	}()

	adapterRemoveProject = true
	adapterRemoveGlobal = false
	workDir = tmpDir

	// Create .meow directory structure but no adapters
	meowDir := filepath.Join(tmpDir, ".meow", "adapters")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatal(err)
	}

	err := runAdapterRemove(adapterRemoveCmd, []string{"test-adapter"})
	if err == nil {
		t.Fatal("expected error when project adapter not found")
	}

	if !strings.Contains(err.Error(), "not found in project directory") {
		t.Errorf("expected 'not found in project directory' error, got: %v", err)
	}
}

func TestAdapterRemoveForceRemovesProjectAdapter(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore flag values and workDir
	oldForce := adapterRemoveForce
	oldProject := adapterRemoveProject
	oldGlobal := adapterRemoveGlobal
	oldWorkDir := workDir
	defer func() {
		adapterRemoveForce = oldForce
		adapterRemoveProject = oldProject
		adapterRemoveGlobal = oldGlobal
		workDir = oldWorkDir
	}()

	adapterRemoveForce = true
	adapterRemoveProject = false
	adapterRemoveGlobal = false
	workDir = tmpDir

	// Create a project adapter
	adapterDir := filepath.Join(tmpDir, ".meow", "adapters", "test-adapter")
	if err := os.MkdirAll(adapterDir, 0755); err != nil {
		t.Fatal(err)
	}

	adapterConfig := `[adapter]
name = "test-adapter"
description = "Test adapter"

[spawn]
command = "echo test"
`
	if err := os.WriteFile(filepath.Join(adapterDir, "adapter.toml"), []byte(adapterConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify adapter exists
	if !adapterExists(adapterDir) {
		t.Fatal("adapter should exist before removal")
	}

	// Run remove command
	err := runAdapterRemove(adapterRemoveCmd, []string{"test-adapter"})
	if err != nil {
		t.Fatalf("runAdapterRemove failed: %v", err)
	}

	// Verify adapter was removed
	if adapterExists(adapterDir) {
		t.Error("adapter should not exist after removal")
	}
}

func TestAdapterRemoveWithProjectFlag(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore flag values and workDir
	oldForce := adapterRemoveForce
	oldProject := adapterRemoveProject
	oldGlobal := adapterRemoveGlobal
	oldWorkDir := workDir
	defer func() {
		adapterRemoveForce = oldForce
		adapterRemoveProject = oldProject
		adapterRemoveGlobal = oldGlobal
		workDir = oldWorkDir
	}()

	adapterRemoveForce = true
	adapterRemoveProject = true
	adapterRemoveGlobal = false
	workDir = tmpDir

	// Create a project adapter
	adapterDir := filepath.Join(tmpDir, ".meow", "adapters", "test-adapter")
	if err := os.MkdirAll(adapterDir, 0755); err != nil {
		t.Fatal(err)
	}

	adapterConfig := `[adapter]
name = "test-adapter"
description = "Test adapter"

[spawn]
command = "echo test"
`
	if err := os.WriteFile(filepath.Join(adapterDir, "adapter.toml"), []byte(adapterConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Run remove command with --project
	err := runAdapterRemove(adapterRemoveCmd, []string{"test-adapter"})
	if err != nil {
		t.Fatalf("runAdapterRemove failed: %v", err)
	}

	// Verify adapter was removed
	if adapterExists(adapterDir) {
		t.Error("adapter should not exist after removal")
	}
}

func TestAdapterExistsFunction(t *testing.T) {
	tmpDir := t.TempDir()

	// Test non-existent path
	if adapterExists(filepath.Join(tmpDir, "nonexistent")) {
		t.Error("adapterExists should return false for non-existent path")
	}

	// Test directory without adapter.toml
	noTomlDir := filepath.Join(tmpDir, "no-toml")
	if err := os.MkdirAll(noTomlDir, 0755); err != nil {
		t.Fatal(err)
	}
	if adapterExists(noTomlDir) {
		t.Error("adapterExists should return false for directory without adapter.toml")
	}

	// Test valid adapter directory
	validDir := filepath.Join(tmpDir, "valid")
	if err := os.MkdirAll(validDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "adapter.toml"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}
	if !adapterExists(validDir) {
		t.Error("adapterExists should return true for valid adapter directory")
	}
}
