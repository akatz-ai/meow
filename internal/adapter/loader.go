package adapter

import (
	"os"
	"path/filepath"
)

// DefaultGlobalDir returns the default global adapter directory.
// This is typically ~/.meow/adapters/
func DefaultGlobalDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".meow", "adapters")
}

// DefaultCacheDir returns the default cache directory for extracted adapters.
// This is typically ~/.meow/cache/adapters/
func DefaultCacheDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".meow", "cache", "adapters")
}

// NewDefaultRegistry creates a new registry with default directories.
// globalDir defaults to ~/.meow/adapters/ if empty.
// projectDir defaults to .meow/adapters/ relative to workdir if empty.
func NewDefaultRegistry(workdir string) (*Registry, error) {
	globalDir := DefaultGlobalDir()

	projectDir := ""
	if workdir != "" {
		projectDir = filepath.Join(workdir, ".meow", "adapters")
	}

	return NewRegistry(globalDir, projectDir), nil
}
