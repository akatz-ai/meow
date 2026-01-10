package main

import (
	"flag"
	"log/slog"
	"os"
	"time"
)

var (
	configPath      string
	resumeSession   string
	skipPermissions bool // Ignored, for Claude compatibility
	logLevel        string
)

func init() {
	flag.StringVar(&configPath, "config", "", "Path to behavior config YAML")
	flag.StringVar(&resumeSession, "resume", "", "Session ID to resume (for compatibility)")
	flag.BoolVar(&skipPermissions, "dangerously-skip-permissions", false, "Claude compatibility flag (ignored)")
	flag.StringVar(&logLevel, "log-level", "info", "Log level (debug/info/warn/error)")
}

func main() {
	flag.Parse()

	// Allow env var override for config path
	if envConfig := os.Getenv("MEOW_SIM_CONFIG"); envConfig != "" && configPath == "" {
		configPath = envConfig
	}

	// Allow env var override for log level
	if envLevel := os.Getenv("MEOW_SIM_LOG_LEVEL"); envLevel != "" {
		logLevel = envLevel
	}

	// Setup logging
	logger := setupLogger(logLevel)

	logger.Debug("simulator starting",
		"config", configPath,
		"resume_session", resumeSession,
		"agent_id", os.Getenv("MEOW_AGENT"),
		"workflow_id", os.Getenv("MEOW_WORKFLOW"),
	)

	// Load config
	var config SimConfig
	if configPath != "" {
		var err error
		config, err = LoadConfig(configPath)
		if err != nil {
			logger.Error("failed to load config", "path", configPath, "error", err)
			os.Exit(1)
		}
	} else {
		config = NewDefaultSimConfig()
	}

	// Create and run simulator
	sim := NewSimulator(config, logger)
	if err := sim.Run(); err != nil {
		logger.Error("simulator error", "error", err)
		os.Exit(1)
	}
}

func setupLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

// NewDefaultSimConfig returns a default simulator configuration.
func NewDefaultSimConfig() SimConfig {
	return SimConfig{
		Timing: TimingConfig{
			StartupDelay:     100 * time.Millisecond,
			DefaultWorkDelay: 100 * time.Millisecond,
			PromptDelay:      10 * time.Millisecond,
		},
		Hooks: HooksConfig{
			FireStopHook:   true,
			FireToolEvents: true,
		},
		Behaviors: []Behavior{},
		Default: DefaultConfig{
			Behavior: Behavior{
				Match: "",
				Type:  "contains",
				Action: Action{
					Type:    ActionComplete,
					Delay:   100 * time.Millisecond,
					Outputs: map[string]any{},
				},
			},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}
