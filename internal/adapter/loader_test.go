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

func TestNewRegistryWithBuiltins(t *testing.T) {
	registry, err := NewRegistryWithBuiltins("", "")
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Should be able to load claude without any filesystem adapters
	config, err := registry.Load("claude")
	if err != nil {
		t.Fatalf("failed to load claude: %v", err)
	}

	if config.Adapter.Name != "claude" {
		t.Errorf("expected name 'claude', got %q", config.Adapter.Name)
	}
}

func TestNewDefaultRegistry(t *testing.T) {
	tempDir := t.TempDir()

	registry, err := NewDefaultRegistry(tempDir)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	// Should have claude built-in
	config, err := registry.Load("claude")
	if err != nil {
		t.Fatalf("failed to load claude: %v", err)
	}

	if config.Adapter.Name != "claude" {
		t.Errorf("expected name 'claude', got %q", config.Adapter.Name)
	}
}
