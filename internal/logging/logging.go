// Package logging provides structured logging infrastructure for MEOW.
package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/meow-stack/meow-machine/internal/config"
)

// NewFromConfig creates a new slog.Logger based on configuration.
func NewFromConfig(cfg *config.Config, baseDir string) (*slog.Logger, io.Closer, error) {
	level := parseLevel(cfg.Logging.Level)
	handler := newHandler(cfg.Logging.Format, os.Stderr, level)

	// If a file is configured, use a multi-writer
	var closer io.Closer
	if cfg.Logging.File != "" {
		logPath := cfg.LogFile(baseDir)

		// Ensure directory exists
		if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			return nil, nil, err
		}

		// Open log file with append mode
		file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, err
		}
		closer = file

		// Create multi-writer for both stderr and file
		multi := io.MultiWriter(os.Stderr, file)
		handler = newHandler(cfg.Logging.Format, multi, level)
	}

	return slog.New(handler), closer, nil
}

// NewDefault creates a default logger writing to stderr.
func NewDefault() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}

// NewForTest creates a silent logger for tests.
func NewForTest() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))
}

// NewWithLevel creates a logger with the specified level.
func NewWithLevel(level slog.Level) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}

// parseLevel converts config log level to slog.Level.
func parseLevel(level config.LogLevel) slog.Level {
	switch level {
	case config.LogLevelDebug:
		return slog.LevelDebug
	case config.LogLevelInfo:
		return slog.LevelInfo
	case config.LogLevelWarn:
		return slog.LevelWarn
	case config.LogLevelError:
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// newHandler creates a slog.Handler based on format.
func newHandler(format config.LogFormat, w io.Writer, level slog.Level) slog.Handler {
	opts := &slog.HandlerOptions{
		Level: level,
	}

	switch format {
	case config.LogFormatJSON:
		return slog.NewJSONHandler(w, opts)
	case config.LogFormatText:
		return slog.NewTextHandler(w, opts)
	default:
		return slog.NewJSONHandler(w, opts)
	}
}

// WithFields returns a logger with the given fields added.
func WithFields(logger *slog.Logger, fields ...any) *slog.Logger {
	return logger.With(fields...)
}

// WithWorkflow returns a logger with workflow context.
func WithWorkflow(logger *slog.Logger, workflowID string) *slog.Logger {
	return logger.With("workflow_id", workflowID)
}

// WithBead returns a logger with bead context.
func WithBead(logger *slog.Logger, beadID string, beadType string) *slog.Logger {
	return logger.With("bead_id", beadID, "bead_type", beadType)
}

// WithAgent returns a logger with agent context.
func WithAgent(logger *slog.Logger, agentID string) *slog.Logger {
	return logger.With("agent", agentID)
}
