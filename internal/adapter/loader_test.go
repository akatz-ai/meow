package adapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultGlobalDir(t *testing.T) {
	dir := DefaultGlobalDir()
	if dir == "" {
		t.Skip("could not determine home directory")
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".meow", "adapters")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}

func TestDefaultCacheDir(t *testing.T) {
	dir := DefaultCacheDir()
	if dir == "" {
		t.Skip("could not determine home directory")
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".meow", "cache", "adapters")
	if dir != expected {
		t.Errorf("expected %q, got %q", expected, dir)
	}
}

func TestNewDefaultRegistry(t *testing.T) {
	tempDir := t.TempDir()

	registry, err := NewDefaultRegistry(tempDir)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// No filesystem adapters yet; registry should still be created
	if registry == nil {
		t.Fatal("expected registry")
	}
}
