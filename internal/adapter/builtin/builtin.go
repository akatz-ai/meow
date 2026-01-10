// Package builtin provides embedded adapter configurations.
// These adapters are compiled into the MEOW binary and available without
// external files.
//
// Note: Event hook configuration (like Claude's Stop/PreToolUse/PostToolUse hooks)
// is handled by library templates (lib/claude-events.meow.toml), not adapters.
// Adapters only define runtime behavior: spawn, inject, stop.
package builtin

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/meow-stack/meow-machine/internal/types"
)

//go:embed claude_adapter.toml
var claudeAdapterTOML []byte

// GetClaudeAdapter returns the built-in Claude adapter configuration.
func GetClaudeAdapter() (*types.AdapterConfig, error) {
	var config types.AdapterConfig
	if _, err := toml.Decode(string(claudeAdapterTOML), &config); err != nil {
		return nil, fmt.Errorf("parsing embedded claude adapter: %w", err)
	}
	return &config, nil
}

// GetClaudeAdapterTOML returns the raw TOML content for the Claude adapter.
func GetClaudeAdapterTOML() []byte {
	return claudeAdapterTOML
}

// ExtractAdapter extracts the built-in Claude adapter files to a directory.
// Returns the path to the extracted adapter directory.
func ExtractAdapter(name, destDir string) (string, error) {
	switch name {
	case "claude":
		return extractClaudeAdapter(destDir)
	default:
		return "", fmt.Errorf("unknown built-in adapter: %s", name)
	}
}

// extractClaudeAdapter extracts the Claude adapter to the destination directory.
func extractClaudeAdapter(destDir string) (string, error) {
	adapterDir := filepath.Join(destDir, "claude")

	// Create directory
	if err := os.MkdirAll(adapterDir, 0755); err != nil {
		return "", fmt.Errorf("creating adapter directory: %w", err)
	}

	// Write adapter.toml
	configPath := filepath.Join(adapterDir, "adapter.toml")
	if err := os.WriteFile(configPath, claudeAdapterTOML, 0644); err != nil {
		return "", fmt.Errorf("writing adapter.toml: %w", err)
	}

	return adapterDir, nil
}

// EnsureExtracted ensures the built-in adapter is extracted to the cache directory.
// If already extracted and up-to-date, returns the existing path.
// Cache directory is typically ~/.meow/cache/adapters/
func EnsureExtracted(name, cacheDir string) (string, error) {
	adapterDir := filepath.Join(cacheDir, name)
	configPath := filepath.Join(adapterDir, "adapter.toml")

	// Check if already extracted and up-to-date
	if _, err := os.Stat(configPath); err == nil {
		configContent, configErr := os.ReadFile(configPath)

		// File must exist and match embedded content
		if configErr == nil && string(configContent) == string(claudeAdapterTOML) {
			// Up to date, reuse
			return adapterDir, nil
		}
		// Stale or corrupted - re-extract
	}

	return ExtractAdapter(name, cacheDir)
}

// BuiltinAdapterNames returns the list of built-in adapter names.
func BuiltinAdapterNames() []string {
	return []string{"claude"}
}
