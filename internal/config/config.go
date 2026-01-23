package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
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
	// Resolution order: step-level > workflow-level > project-level > global-level.
	DefaultAdapter string `toml:"default_adapter"`

	// Logging enables per-agent output logging. When enabled, each agent's terminal
	// output is captured to a log file in .meow/logs/<run_id>/<agent_id>.log.
	// Default: true
	Logging *bool `toml:"logging"`
}

// IsLoggingEnabled returns whether agent logging is enabled (default: true).
func (c *AgentConfig) IsLoggingEnabled() bool {
	if c.Logging == nil {
		return true // Default enabled
	}
	return *c.Logging
}

// PathsConfig holds path configuration.
type PathsConfig struct {
	WorkflowDir string `toml:"workflow_dir"`
	RunsDir     string `toml:"runs_dir"`
	LogsDir     string `toml:"logs_dir"`
}

// OrchestratorConfig holds orchestrator settings.
type OrchestratorConfig struct {
	PollInterval time.Duration `toml:"poll_interval"`
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level  LogLevel  `toml:"level"`
	Format LogFormat `toml:"format"`
}

// Config is the main configuration struct for MEOW.
type Config struct {
	Version      string             `toml:"version"`
	Paths        PathsConfig        `toml:"paths"`
	Orchestrator OrchestratorConfig `toml:"orchestrator"`
	Logging      LoggingConfig      `toml:"logging"`
	Agent        AgentConfig        `toml:"agent"`
}

// Default returns a Config with sensible defaults.
func Default() *Config {
	return &Config{
		Version: "1",
		Paths: PathsConfig{
			WorkflowDir: ".meow/workflows",
			RunsDir:     ".meow/runs",
			LogsDir:     ".meow/logs",
		},
		Orchestrator: OrchestratorConfig{
			PollInterval: 100 * time.Millisecond,
		},
		Logging: LoggingConfig{
			Level:  LogLevelInfo,
			Format: LogFormatJSON,
		},
		Agent: AgentConfig{
			DefaultAdapter: "claude",
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
	if c.Paths.WorkflowDir == "" {
		return fmt.Errorf("workflow_dir is required")
	}
	if c.Orchestrator.PollInterval <= 0 {
		return fmt.Errorf("poll_interval must be positive")
	}
	return nil
}

// WorkflowDir returns the absolute workflow directory path.
func (c *Config) WorkflowDir(baseDir string) string {
	if filepath.IsAbs(c.Paths.WorkflowDir) {
		return c.Paths.WorkflowDir
	}
	return filepath.Join(baseDir, c.Paths.WorkflowDir)
}

// RunsDir returns the absolute runs directory path.
func (c *Config) RunsDir(baseDir string) string {
	if filepath.IsAbs(c.Paths.RunsDir) {
		return c.Paths.RunsDir
	}
	return filepath.Join(baseDir, c.Paths.RunsDir)
}

// LogsDir returns the absolute logs directory path.
func (c *Config) LogsDir(baseDir string) string {
	if filepath.IsAbs(c.Paths.LogsDir) {
		return c.Paths.LogsDir
	}
	return filepath.Join(baseDir, c.Paths.LogsDir)
}
