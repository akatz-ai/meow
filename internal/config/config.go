package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// EphemeralCleanup specifies when to clean up ephemeral beads.
type EphemeralCleanup string

const (
	EphemeralCleanupOnComplete EphemeralCleanup = "on_complete" // Clean after template completes
	EphemeralCleanupManual     EphemeralCleanup = "manual"      // Only via `meow clean`
	EphemeralCleanupNever      EphemeralCleanup = "never"       // Keep forever
)

// LogLevel specifies the logging verbosity.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogFormat specifies the log output format.
type LogFormat string

const (
	LogFormatJSON LogFormat = "json"
	LogFormatText LogFormat = "text"
)

// AgentConfig holds agent-related settings.
type AgentConfig struct {
	// DefaultAdapter specifies the default adapter to use when spawning agents.
	// Resolution order: step-level > workflow-level > project-level > global-level > built-in default ("claude").
	DefaultAdapter string `toml:"default_adapter"`

	// SetupHooks controls whether to create .claude/settings.json with MEOW hooks
	// when spawning agents. Default: true (agents get hooks for Ralph Wiggum loop).
	// Set to false to disable automatic hook injection.
	SetupHooks bool `toml:"setup_hooks"`
}

// PathsConfig holds path configuration.
type PathsConfig struct {
	TemplateDir string `toml:"template_dir"`
	BeadsDir    string `toml:"beads_dir"`
	StateDir    string `toml:"state_dir"`
}

// DefaultsConfig holds default values.
type DefaultsConfig struct {
	Agent            string        `toml:"agent"`
	StopGracePeriod  int           `toml:"stop_grace_period"` // Seconds
	ConditionTimeout time.Duration `toml:"condition_timeout"`
}

// OrchestratorConfig holds orchestrator settings.
type OrchestratorConfig struct {
	PollInterval      time.Duration `toml:"poll_interval"`
	HeartbeatInterval time.Duration `toml:"heartbeat_interval"`
}

// CleanupConfig holds cleanup settings.
type CleanupConfig struct {
	Ephemeral EphemeralCleanup `toml:"ephemeral"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  LogLevel  `toml:"level"`
	Format LogFormat `toml:"format"`
	File   string    `toml:"file"`
}

// Config is the main configuration struct for MEOW.
type Config struct {
	Version      string             `toml:"version"`
	Paths        PathsConfig        `toml:"paths"`
	Defaults     DefaultsConfig     `toml:"defaults"`
	Orchestrator OrchestratorConfig `toml:"orchestrator"`
	Cleanup      CleanupConfig      `toml:"cleanup"`
	Logging      LoggingConfig      `toml:"logging"`
	Agent        AgentConfig        `toml:"agent"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Version: "1",
		Paths: PathsConfig{
			TemplateDir: ".meow/templates",
			BeadsDir:    ".beads",
			StateDir:    ".meow/state",
		},
		Defaults: DefaultsConfig{
			Agent:            "claude-1",
			StopGracePeriod:  10,
			ConditionTimeout: time.Hour,
		},
		Orchestrator: OrchestratorConfig{
			PollInterval:      100 * time.Millisecond,
			HeartbeatInterval: 30 * time.Second,
		},
		Cleanup: CleanupConfig{
			Ephemeral: EphemeralCleanupOnComplete,
		},
		Logging: LoggingConfig{
			Level:  LogLevelInfo,
			Format: LogFormatJSON,
			File:   ".meow/state/meow.log",
		},
		Agent: AgentConfig{
			DefaultAdapter: "claude",  // Built-in default adapter
			SetupHooks:     true,      // Enable Ralph Wiggum loop by default for agents
		},
	}
}

// Load loads configuration from file, merging with defaults.
func Load(path string) (*Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // Use defaults if no config file
		}
		return nil, fmt.Errorf("reading config: %w", err)
	}

	if _, err := toml.Decode(string(data), cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return cfg, nil
}

// LoadFromDir loads configuration from the standard locations in a directory.
// Applies in order: defaults -> ~/.meow/config.toml -> .meow/config.toml
// Later configs override earlier ones (project-level takes precedence).
func LoadFromDir(dir string) (*Config, error) {
	cfg := Default()

	// Load global config first (if exists)
	home, err := os.UserHomeDir()
	if err == nil {
		globalConfig := filepath.Join(home, ".meow", "config.toml")
		if data, err := os.ReadFile(globalConfig); err == nil {
			if _, err := toml.Decode(string(data), cfg); err != nil {
				return nil, fmt.Errorf("parsing global config: %w", err)
			}
		}
	}

	// Load project config (overrides global)
	projectConfig := filepath.Join(dir, ".meow", "config.toml")
	if data, err := os.ReadFile(projectConfig); err == nil {
		if _, err := toml.Decode(string(data), cfg); err != nil {
			return nil, fmt.Errorf("parsing project config: %w", err)
		}
	}

	return cfg, nil
}

// Validate checks that the configuration is valid.
func (c *Config) Validate() error {
	if c.Version == "" {
		return fmt.Errorf("config version is required")
	}
	if c.Paths.TemplateDir == "" {
		return fmt.Errorf("template_dir is required")
	}
	if c.Paths.BeadsDir == "" {
		return fmt.Errorf("beads_dir is required")
	}
	if c.Orchestrator.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be positive")
	}
	return nil
}

// TemplateDir returns the absolute template directory path.
func (c *Config) TemplateDir(baseDir string) string {
	if filepath.IsAbs(c.Paths.TemplateDir) {
		return c.Paths.TemplateDir
	}
	return filepath.Join(baseDir, c.Paths.TemplateDir)
}

// BeadsDir returns the absolute beads directory path.
func (c *Config) BeadsDir(baseDir string) string {
	if filepath.IsAbs(c.Paths.BeadsDir) {
		return c.Paths.BeadsDir
	}
	return filepath.Join(baseDir, c.Paths.BeadsDir)
}

// StateDir returns the absolute state directory path.
func (c *Config) StateDir(baseDir string) string {
	if filepath.IsAbs(c.Paths.StateDir) {
		return c.Paths.StateDir
	}
	return filepath.Join(baseDir, c.Paths.StateDir)
}

// LogFile returns the absolute log file path.
func (c *Config) LogFile(baseDir string) string {
	if filepath.IsAbs(c.Logging.File) {
		return c.Logging.File
	}
	return filepath.Join(baseDir, c.Logging.File)
}
