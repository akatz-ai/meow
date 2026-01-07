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
	if cfg.Paths.TemplateDir != ".meow/templates" {
		t.Errorf("TemplateDir = %s, want .meow/templates", cfg.Paths.TemplateDir)
	}
	if cfg.Paths.BeadsDir != ".beads" {
		t.Errorf("BeadsDir = %s, want .beads", cfg.Paths.BeadsDir)
	}
	if cfg.Defaults.Agent != "claude-1" {
		t.Errorf("Defaults.Agent = %s, want claude-1", cfg.Defaults.Agent)
	}
	if cfg.Orchestrator.PollInterval != 100*time.Millisecond {
		t.Errorf("PollInterval = %v, want 100ms", cfg.Orchestrator.PollInterval)
	}
	if cfg.Cleanup.Ephemeral != EphemeralCleanupOnComplete {
		t.Errorf("Cleanup.Ephemeral = %s, want on_complete", cfg.Cleanup.Ephemeral)
	}
	if cfg.Logging.Level != LogLevelInfo {
		t.Errorf("Logging.Level = %s, want info", cfg.Logging.Level)
	}
}

func TestLoad(t *testing.T) {
	// Create temp config file
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	content := `
version = "2"

[paths]
template_dir = "custom/templates"
beads_dir = "custom/beads"
state_dir = "custom/state"

[defaults]
agent = "claude-custom"
stop_grace_period = 30

[orchestrator]
poll_interval = "200ms"
heartbeat_interval = "1m"

[cleanup]
ephemeral = "manual"

[logging]
level = "debug"
format = "text"
file = "custom.log"
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
	if cfg.Paths.TemplateDir != "custom/templates" {
		t.Errorf("TemplateDir = %s, want custom/templates", cfg.Paths.TemplateDir)
	}
	if cfg.Defaults.Agent != "claude-custom" {
		t.Errorf("Defaults.Agent = %s, want claude-custom", cfg.Defaults.Agent)
	}
	if cfg.Defaults.StopGracePeriod != 30 {
		t.Errorf("StopGracePeriod = %d, want 30", cfg.Defaults.StopGracePeriod)
	}
	if cfg.Orchestrator.PollInterval != 200*time.Millisecond {
		t.Errorf("PollInterval = %v, want 200ms", cfg.Orchestrator.PollInterval)
	}
	if cfg.Cleanup.Ephemeral != EphemeralCleanupManual {
		t.Errorf("Cleanup.Ephemeral = %s, want manual", cfg.Cleanup.Ephemeral)
	}
	if cfg.Logging.Level != LogLevelDebug {
		t.Errorf("Logging.Level = %s, want debug", cfg.Logging.Level)
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

func TestLoadFromDir(t *testing.T) {
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
				Paths: PathsConfig{TemplateDir: "a", BeadsDir: "b"},
				Orchestrator: OrchestratorConfig{PollInterval: time.Millisecond},
			},
			wantErr: true,
		},
		{
			name: "missing template_dir",
			cfg: &Config{
				Version: "1",
				Paths:   PathsConfig{BeadsDir: "b"},
				Orchestrator: OrchestratorConfig{PollInterval: time.Millisecond},
			},
			wantErr: true,
		},
		{
			name: "missing beads_dir",
			cfg: &Config{
				Version: "1",
				Paths:   PathsConfig{TemplateDir: "a"},
				Orchestrator: OrchestratorConfig{PollInterval: time.Millisecond},
			},
			wantErr: true,
		},
		{
			name: "zero poll_interval",
			cfg: &Config{
				Version: "1",
				Paths:   PathsConfig{TemplateDir: "a", BeadsDir: "b"},
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

func TestConfig_PathHelpers(t *testing.T) {
	cfg := Default()
	baseDir := "/project"

	if got := cfg.TemplateDir(baseDir); got != "/project/.meow/templates" {
		t.Errorf("TemplateDir = %s, want /project/.meow/templates", got)
	}
	if got := cfg.BeadsDir(baseDir); got != "/project/.beads" {
		t.Errorf("BeadsDir = %s, want /project/.beads", got)
	}
	if got := cfg.StateDir(baseDir); got != "/project/.meow/state" {
		t.Errorf("StateDir = %s, want /project/.meow/state", got)
	}
	if got := cfg.LogFile(baseDir); got != "/project/.meow/state/meow.log" {
		t.Errorf("LogFile = %s, want /project/.meow/state/meow.log", got)
	}

	// Test with absolute paths
	cfg.Paths.TemplateDir = "/absolute/templates"
	if got := cfg.TemplateDir(baseDir); got != "/absolute/templates" {
		t.Errorf("TemplateDir (abs) = %s, want /absolute/templates", got)
	}
}
