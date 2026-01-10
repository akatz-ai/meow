package builtin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGetClaudeAdapter(t *testing.T) {
	config, err := GetClaudeAdapter()
	if err != nil {
		t.Fatalf("failed to get claude adapter: %v", err)
	}

	// Verify adapter metadata
	if config.Adapter.Name != "claude" {
		t.Errorf("expected name 'claude', got %q", config.Adapter.Name)
	}
	if config.Adapter.Description == "" {
		t.Error("expected non-empty description")
	}

	// Verify spawn config
	if config.Spawn.Command != "claude --dangerously-skip-permissions" {
		t.Errorf("unexpected spawn command: %q", config.Spawn.Command)
	}
	if !strings.Contains(config.Spawn.ResumeCommand, "{{session_id}}") {
		t.Errorf("resume command should contain {{session_id}} placeholder: %q", config.Spawn.ResumeCommand)
	}
	if config.Spawn.StartupDelay.Duration() != 3*time.Second {
		t.Errorf("expected startup_delay = 3s, got %v", config.Spawn.StartupDelay)
	}

	// Verify environment
	if tmux, ok := config.Environment["TMUX"]; !ok || tmux != "" {
		t.Errorf("expected TMUX='', got %q", tmux)
	}

	// Verify prompt injection
	if len(config.PromptInjection.PreKeys) == 0 || config.PromptInjection.PreKeys[0] != "Escape" {
		t.Errorf("expected pre_keys to include Escape, got %v", config.PromptInjection.PreKeys)
	}
	if config.PromptInjection.Method != "literal" {
		t.Errorf("expected method 'literal', got %q", config.PromptInjection.Method)
	}
	if len(config.PromptInjection.PostKeys) == 0 || config.PromptInjection.PostKeys[0] != "Enter" {
		t.Errorf("expected post_keys to include Enter, got %v", config.PromptInjection.PostKeys)
	}

	// Verify graceful stop
	if len(config.GracefulStop.Keys) == 0 || config.GracefulStop.Keys[0] != "C-c" {
		t.Errorf("expected graceful_stop.keys = [C-c], got %v", config.GracefulStop.Keys)
	}
	if config.GracefulStop.Wait.Duration() != 2*time.Second {
		t.Errorf("expected graceful_stop.wait = 2s, got %v", config.GracefulStop.Wait)
	}

	// Verify events
	if config.Events.Translator != "./event-translator.sh" {
		t.Errorf("expected translator = ./event-translator.sh, got %q", config.Events.Translator)
	}

	// Validate the config
	if err := config.Validate(); err != nil {
		t.Errorf("claude adapter should be valid: %v", err)
	}
}

func TestGetClaudeAdapterTOML(t *testing.T) {
	tomlContent := GetClaudeAdapterTOML()
	if len(tomlContent) == 0 {
		t.Error("expected non-empty TOML content")
	}
	if !strings.Contains(string(tomlContent), "[adapter]") {
		t.Error("TOML should contain [adapter] section")
	}
	if !strings.Contains(string(tomlContent), "name = \"claude\"") {
		t.Error("TOML should define name = claude")
	}
}

func TestGetClaudeEventTranslator(t *testing.T) {
	script := GetClaudeEventTranslator()
	if len(script) == 0 {
		t.Error("expected non-empty script content")
	}
	if !strings.HasPrefix(string(script), "#!/bin/bash") {
		t.Error("script should start with shebang")
	}
	if !strings.Contains(string(script), "meow event") {
		t.Error("script should call 'meow event'")
	}
}

func TestExtractAdapter(t *testing.T) {
	tempDir := t.TempDir()

	// Extract claude adapter
	adapterDir, err := ExtractAdapter("claude", tempDir)
	if err != nil {
		t.Fatalf("failed to extract claude adapter: %v", err)
	}

	expectedDir := filepath.Join(tempDir, "claude")
	if adapterDir != expectedDir {
		t.Errorf("expected adapter dir %q, got %q", expectedDir, adapterDir)
	}

	// Verify adapter.toml was written
	configPath := filepath.Join(adapterDir, "adapter.toml")
	configContent, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read extracted adapter.toml: %v", err)
	}
	if !strings.Contains(string(configContent), "name = \"claude\"") {
		t.Error("extracted adapter.toml should contain claude name")
	}

	// Verify event-translator.sh was written with executable permission
	scriptPath := filepath.Join(adapterDir, "event-translator.sh")
	scriptInfo, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("failed to stat event-translator.sh: %v", err)
	}
	if scriptInfo.Mode()&0111 == 0 {
		t.Error("event-translator.sh should be executable")
	}

	// Unknown adapter should fail
	_, err = ExtractAdapter("unknown", tempDir)
	if err == nil {
		t.Error("expected error for unknown adapter")
	}
}

func TestEnsureExtracted(t *testing.T) {
	tempDir := t.TempDir()
	cacheDir := filepath.Join(tempDir, "cache")

	// First extraction
	dir1, err := EnsureExtracted("claude", cacheDir)
	if err != nil {
		t.Fatalf("first extraction failed: %v", err)
	}

	// Verify files exist
	configPath := filepath.Join(dir1, "adapter.toml")
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("adapter.toml should exist: %v", err)
	}

	// Second call should reuse existing (no error, same path)
	dir2, err := EnsureExtracted("claude", cacheDir)
	if err != nil {
		t.Fatalf("second extraction failed: %v", err)
	}
	if dir1 != dir2 {
		t.Errorf("expected same directory on reuse: %q vs %q", dir1, dir2)
	}

	// Modify the config to simulate staleness
	if err := os.WriteFile(configPath, []byte("stale"), 0644); err != nil {
		t.Fatal(err)
	}

	// Should re-extract due to staleness
	dir3, err := EnsureExtracted("claude", cacheDir)
	if err != nil {
		t.Fatalf("re-extraction failed: %v", err)
	}
	if dir3 != dir1 {
		t.Errorf("expected same directory after re-extract: %q vs %q", dir1, dir3)
	}

	// Verify content was updated
	content, _ := os.ReadFile(configPath)
	if !strings.Contains(string(content), "name = \"claude\"") {
		t.Error("config should be restored after re-extraction")
	}

	// Test missing event-translator.sh triggers re-extraction
	scriptPath := filepath.Join(dir1, "event-translator.sh")
	if err := os.Remove(scriptPath); err != nil {
		t.Fatalf("failed to remove script: %v", err)
	}

	// Should re-extract because script is missing
	dir4, err := EnsureExtracted("claude", cacheDir)
	if err != nil {
		t.Fatalf("re-extraction after script removal failed: %v", err)
	}
	if dir4 != dir1 {
		t.Errorf("expected same directory after script re-extract: %q vs %q", dir1, dir4)
	}

	// Verify script was restored
	scriptInfo, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatalf("script should be restored: %v", err)
	}
	if scriptInfo.Mode()&0111 == 0 {
		t.Error("restored script should be executable")
	}

	// Test corrupted event-translator.sh triggers re-extraction
	if err := os.WriteFile(scriptPath, []byte("corrupted"), 0755); err != nil {
		t.Fatal(err)
	}

	dir5, err := EnsureExtracted("claude", cacheDir)
	if err != nil {
		t.Fatalf("re-extraction after script corruption failed: %v", err)
	}
	if dir5 != dir1 {
		t.Errorf("expected same directory after corrupt script re-extract: %q vs %q", dir1, dir5)
	}

	// Verify script was restored with correct content
	scriptContent, _ := os.ReadFile(scriptPath)
	if !strings.Contains(string(scriptContent), "#!/bin/bash") {
		t.Error("script should be restored with correct content")
	}
}

func TestBuiltinAdapterNames(t *testing.T) {
	names := BuiltinAdapterNames()
	if len(names) == 0 {
		t.Error("expected at least one built-in adapter")
	}

	found := false
	for _, name := range names {
		if name == "claude" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected 'claude' in built-in adapter names")
	}
}
