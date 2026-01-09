// Package builtin provides embedded adapter configurations.
// These adapters are compiled into the MEOW binary and available without
// external files.
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

//go:embed event_translator.sh
var claudeEventTranslator []byte

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

// GetClaudeEventTranslator returns the raw event translator script.
func GetClaudeEventTranslator() []byte {
	return claudeEventTranslator
}

// ExtractAdapter extracts the built-in Claude adapter files to a directory.
// This is useful when the adapter scripts need to be executed.
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

	// Write event-translator.sh
	scriptPath := filepath.Join(adapterDir, "event-translator.sh")
	if err := os.WriteFile(scriptPath, claudeEventTranslator, 0755); err != nil {
		return "", fmt.Errorf("writing event-translator.sh: %w", err)
	}

	return adapterDir, nil
}

// EnsureExtracted ensures the built-in adapter is extracted to the cache directory.
// If already extracted, returns the existing path.
// Cache directory is typically ~/.meow/cache/adapters/
func EnsureExtracted(name, cacheDir string) (string, error) {
	adapterDir := filepath.Join(cacheDir, name)
	configPath := filepath.Join(adapterDir, "adapter.toml")

	// Check if already extracted
	if _, err := os.Stat(configPath); err == nil {
		// Already exists - verify it's not stale by checking embedded content matches
		existingContent, err := os.ReadFile(configPath)
		if err == nil && string(existingContent) == string(claudeAdapterTOML) {
			// Up to date, reuse
			return adapterDir, nil
		}
		// Stale or unreadable - re-extract
	}

	return ExtractAdapter(name, cacheDir)
}

// BuiltinAdapterNames returns the list of built-in adapter names.
func BuiltinAdapterNames() []string {
	return []string{"claude"}
}
