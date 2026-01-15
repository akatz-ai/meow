package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.Version != "1" {
		t.Errorf("Version = %s, want 1", cfg.Version)
	}
	if cfg.Paths.WorkflowDir != ".meow/workflows" {
		t.Errorf("WorkflowDir = %s, want .meow/workflows", cfg.Paths.WorkflowDir)
	}
	if cfg.Paths.RunsDir != ".meow/runs" {
		t.Errorf("RunsDir = %s, want .meow/runs", cfg.Paths.RunsDir)
	}
	if cfg.Paths.LogsDir != ".meow/logs" {
		t.Errorf("LogsDir = %s, want .meow/logs", cfg.Paths.LogsDir)
	}
	if cfg.Orchestrator.PollInterval != 100*time.Millisecond {
		t.Errorf("PollInterval = %v, want 100ms", cfg.Orchestrator.PollInterval)
	}
	if cfg.Logging.Level != LogLevelInfo {
		t.Errorf("Logging.Level = %s, want info", cfg.Logging.Level)
	}
	if cfg.Agent.DefaultAdapter != "claude" {
		t.Errorf("Agent.DefaultAdapter = %s, want claude", cfg.Agent.DefaultAdapter)
	}
}

func TestLoad(t *testing.T) {
	// Create temp config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	content := `
version = "2"

[paths]
workflow_dir = "custom/workflows"
runs_dir = "custom/runs"
logs_dir = "custom/logs"

[orchestrator]
poll_interval = "200ms"

[logging]
level = "debug"
format = "text"

[agent]
default_adapter = "aider"
`

	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Version != "2" {
		t.Errorf("Version = %s, want 2", cfg.Version)
	}
	if cfg.Paths.WorkflowDir != "custom/workflows" {
		t.Errorf("WorkflowDir = %s, want custom/workflows", cfg.Paths.WorkflowDir)
	}
	if cfg.Orchestrator.PollInterval != 200*time.Millisecond {
		t.Errorf("PollInterval = %v, want 200ms", cfg.Orchestrator.PollInterval)
	}
	if cfg.Logging.Level != LogLevelDebug {
		t.Errorf("Logging.Level = %s, want debug", cfg.Logging.Level)
	}
	if cfg.Agent.DefaultAdapter != "aider" {
		t.Errorf("Agent.DefaultAdapter = %s, want aider", cfg.Agent.DefaultAdapter)
	}
}

func TestLoad_NonExistent(t *testing.T) {
	cfg, err := Load("/nonexistent/config.toml")
	if err != nil {
		t.Fatalf("Load should not fail for non-existent file: %v", err)
	}

	// Should return defaults
	if cfg.Version != "1" {
		t.Errorf("Should return defaults, got version = %s", cfg.Version)
	}
}

func TestLoad_InvalidTOML(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	content := `invalid = [toml content`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("Load should fail for invalid TOML")
	}
}

func TestLoad_ReadError(t *testing.T) {
	// Try to read a directory - this will fail with a read error, not "not found"
	dir := t.TempDir()
	_, err := Load(dir)
	if err == nil {
		t.Error("Load should fail when trying to read a directory")
	}
}

func TestLoadFromDir(t *testing.T) {
	t.Run("project-local config", func(t *testing.T) {
		// Create temp directory with .meow/config.toml
		dir := t.TempDir()
		meowDir := filepath.Join(dir, ".meow")
		if err := os.MkdirAll(meowDir, 0755); err != nil {
			t.Fatalf("Failed to create .meow dir: %v", err)
		}

		configPath := filepath.Join(meowDir, "config.toml")
		content := `version = "project-local"`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}

		cfg, err := LoadFromDir(dir)
		if err != nil {
			t.Fatalf("LoadFromDir failed: %v", err)
		}

		if cfg.Version != "project-local" {
			t.Errorf("Version = %s, want project-local", cfg.Version)
		}
	})

	t.Run("no config file - uses defaults", func(t *testing.T) {
		dir := t.TempDir()

		cfg, err := LoadFromDir(dir)
		if err != nil {
			t.Fatalf("LoadFromDir failed: %v", err)
		}

		// Should return defaults
		if cfg.Version != "1" {
			t.Errorf("Version = %s, want 1 (default)", cfg.Version)
		}
	})

	t.Run("invalid project config", func(t *testing.T) {
		dir := t.TempDir()
		meowDir := filepath.Join(dir, ".meow")
		if err := os.MkdirAll(meowDir, 0755); err != nil {
			t.Fatalf("Failed to create .meow dir: %v", err)
		}

		configPath := filepath.Join(meowDir, "config.toml")
		content := `invalid = [toml`
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}

		_, err := LoadFromDir(dir)
		if err == nil {
			t.Error("LoadFromDir should fail with invalid TOML")
		}
	})

	t.Run("user global config", func(t *testing.T) {
		// Get home dir and create user global config temporarily
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("Cannot get user home directory")
		}

		// Per spec: ~/.meow/config.toml (not ~/.config/meow/config.toml)
		userConfigDir := filepath.Join(home, ".meow")
		userConfigPath := filepath.Join(userConfigDir, "config.toml")

		// Check if config already exists (don't overwrite)
		if _, err := os.Stat(userConfigPath); err == nil {
			t.Skip("User global config already exists, skipping to avoid modification")
		}

		// Create the config dir
		if err := os.MkdirAll(userConfigDir, 0755); err != nil {
			t.Fatalf("Failed to create user config dir: %v", err)
		}
		defer os.RemoveAll(userConfigDir)

		// Write a test config
		content := `version = "user-global"`
		if err := os.WriteFile(userConfigPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write user config: %v", err)
		}

		// Use a temp dir that has no project config
		dir := t.TempDir()
		cfg, err := LoadFromDir(dir)
		if err != nil {
			t.Fatalf("LoadFromDir failed: %v", err)
		}

		if cfg.Version != "user-global" {
			t.Errorf("Version = %s, want user-global", cfg.Version)
		}
	})
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name:    "valid default config",
			cfg:     Default(),
			wantErr: false,
		},
		{
			name: "missing version",
			cfg: &Config{
				Paths:        PathsConfig{WorkflowDir: "a"},
				Orchestrator: OrchestratorConfig{PollInterval: time.Millisecond},
			},
			wantErr: true,
		},
		{
			name: "missing workflow_dir",
			cfg: &Config{
				Version:      "1",
				Orchestrator: OrchestratorConfig{PollInterval: time.Millisecond},
			},
			wantErr: true,
		},
		{
			name: "zero poll_interval",
			cfg: &Config{
				Version:      "1",
				Paths:        PathsConfig{WorkflowDir: "a"},
				Orchestrator: OrchestratorConfig{PollInterval: 0},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadFromDir_DefaultAdapter(t *testing.T) {
	t.Run("default adapter from project config", func(t *testing.T) {
		dir := t.TempDir()
		meowDir := filepath.Join(dir, ".meow")
		if err := os.MkdirAll(meowDir, 0755); err != nil {
			t.Fatalf("Failed to create .meow dir: %v", err)
		}

		content := `
[agent]
default_adapter = "aider"
`
		configPath := filepath.Join(meowDir, "config.toml")
		if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}

		cfg, err := LoadFromDir(dir)
		if err != nil {
			t.Fatalf("LoadFromDir failed: %v", err)
		}

		if cfg.Agent.DefaultAdapter != "aider" {
			t.Errorf("Agent.DefaultAdapter = %s, want aider", cfg.Agent.DefaultAdapter)
		}
	})

	t.Run("default adapter uses default value", func(t *testing.T) {
		dir := t.TempDir()
		cfg, err := LoadFromDir(dir)
		if err != nil {
			t.Fatalf("LoadFromDir failed: %v", err)
		}

		if cfg.Agent.DefaultAdapter != "claude" {
			t.Errorf("Agent.DefaultAdapter = %s, want claude (default)", cfg.Agent.DefaultAdapter)
		}
	})
}

func TestConfig_PathHelpers(t *testing.T) {
	cfg := Default()
	baseDir := "/project"

	// Test relative paths
	if got := cfg.WorkflowDir(baseDir); got != "/project/.meow/workflows" {
		t.Errorf("WorkflowDir = %s, want /project/.meow/workflows", got)
	}
	if got := cfg.RunsDir(baseDir); got != "/project/.meow/runs" {
		t.Errorf("RunsDir = %s, want /project/.meow/runs", got)
	}
	if got := cfg.LogsDir(baseDir); got != "/project/.meow/logs" {
		t.Errorf("LogsDir = %s, want /project/.meow/logs", got)
	}

	// Test with absolute paths
	cfg.Paths.WorkflowDir = "/absolute/workflows"
	if got := cfg.WorkflowDir(baseDir); got != "/absolute/workflows" {
		t.Errorf("WorkflowDir (abs) = %s, want /absolute/workflows", got)
	}

	cfg.Paths.RunsDir = "/absolute/runs"
	if got := cfg.RunsDir(baseDir); got != "/absolute/runs" {
		t.Errorf("RunsDir (abs) = %s, want /absolute/runs", got)
	}

	cfg.Paths.LogsDir = "/absolute/logs"
	if got := cfg.LogsDir(baseDir); got != "/absolute/logs" {
		t.Errorf("LogsDir (abs) = %s, want /absolute/logs", got)
	}
}
