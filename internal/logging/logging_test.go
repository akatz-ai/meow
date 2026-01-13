package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meow-stack/meow-machine/internal/config"
)

func TestNewFromConfig_DefaultsToStderr(t *testing.T) {
	cfg := &config.Config{
		Logging: config.LoggingConfig{
			Level:  config.LogLevelInfo,
			Format: config.LogFormatJSON,
			File:   "", // No file
		},
	}

	logger, closer, err := NewFromConfig(cfg, "/tmp")
	if err != nil {
		t.Fatalf("NewFromConfig failed: %v", err)
	}
	if closer != nil {
		t.Error("Expected no closer when no file configured")
	}
	if logger == nil {
		t.Fatal("Expected logger to be non-nil")
	}
}

func TestNewForRun(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Paths: config.PathsConfig{
			LogsDir: filepath.Join(dir, "logs"),
		},
		Logging: config.LoggingConfig{
			Level:  config.LogLevelDebug,
			Format: config.LogFormatJSON,
		},
	}

	logger, closer, err := NewForRun(cfg, dir, "run-123")
	if err != nil {
		t.Fatalf("NewForRun failed: %v", err)
	}
	if closer == nil {
		t.Fatal("Expected closer for run log")
	}
	defer closer.Close()

	// Log something
	logger.Info("test message", "key", "value")

	// Verify file was created and contains log
	logPath := filepath.Join(dir, "logs", "run-123.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(data), "test message") {
		t.Errorf("Log file does not contain expected message: %s", data)
	}
}

func TestNewForRun_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	logsDir := filepath.Join(dir, "nested", "deep", "logs")
	cfg := &config.Config{
		Paths: config.PathsConfig{
			LogsDir: logsDir,
		},
		Logging: config.LoggingConfig{
			Level:  config.LogLevelInfo,
			Format: config.LogFormatJSON,
		},
	}

	logger, closer, err := NewForRun(cfg, dir, "run-456")
	if err != nil {
		t.Fatalf("NewForRun failed: %v", err)
	}
	if closer != nil {
		closer.Close()
	}
	if logger == nil {
		t.Fatal("Expected logger to be non-nil")
	}

	// Verify directory was created
	info, err := os.Stat(logsDir)
	if err != nil {
		t.Fatalf("Directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected directory, got file")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input config.LogLevel
		want  slog.Level
	}{
		{config.LogLevelDebug, slog.LevelDebug},
		{config.LogLevelInfo, slog.LevelInfo},
		{config.LogLevelWarn, slog.LevelWarn},
		{config.LogLevelError, slog.LevelError},
		{"unknown", slog.LevelInfo}, // default
	}

	for _, tt := range tests {
		t.Run(string(tt.input), func(t *testing.T) {
			if got := parseLevel(tt.input); got != tt.want {
				t.Errorf("parseLevel(%s) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNewHandler_JSON(t *testing.T) {
	var buf bytes.Buffer
	handler := newHandler(config.LogFormatJSON, &buf, slog.LevelInfo)
	logger := slog.New(handler)

	logger.Info("test", "key", "value")

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("JSON unmarshal failed: %v (output: %s)", err, buf.String())
	}

	if result["msg"] != "test" {
		t.Errorf("msg = %v, want test", result["msg"])
	}
	if result["key"] != "value" {
		t.Errorf("key = %v, want value", result["key"])
	}
}

func TestNewHandler_Text(t *testing.T) {
	var buf bytes.Buffer
	handler := newHandler(config.LogFormatText, &buf, slog.LevelInfo)
	logger := slog.New(handler)

	logger.Info("test", "key", "value")

	output := buf.String()
	if !strings.Contains(output, "test") {
		t.Errorf("output should contain 'test': %s", output)
	}
	if !strings.Contains(output, "key=value") {
		t.Errorf("output should contain 'key=value': %s", output)
	}
}

func TestNewDefault(t *testing.T) {
	logger := NewDefault()
	if logger == nil {
		t.Fatal("Expected logger to be non-nil")
	}
}

func TestNewForTest(t *testing.T) {
	logger := NewForTest()
	if logger == nil {
		t.Fatal("Expected logger to be non-nil")
	}
	// Should not panic when logging
	logger.Info("test message")
}

func TestNewWithLevel(t *testing.T) {
	logger := NewWithLevel(slog.LevelDebug)
	if logger == nil {
		t.Fatal("Expected logger to be non-nil")
	}
}

func TestWithFields(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)

	enriched := WithFields(logger, "field1", "value1", "field2", 42)
	enriched.Info("test")

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if result["field1"] != "value1" {
		t.Errorf("field1 = %v, want value1", result["field1"])
	}
	if result["field2"] != float64(42) { // JSON numbers are float64
		t.Errorf("field2 = %v, want 42", result["field2"])
	}
}

func TestWithWorkflow(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)

	enriched := WithWorkflow(logger, "run-001")
	enriched.Info("test")

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if result["workflow_id"] != "run-001" {
		t.Errorf("workflow_id = %v, want wf-001", result["workflow_id"])
	}
}

func TestWithBead(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)

	enriched := WithBead(logger, "bd-123", "task")
	enriched.Info("test")

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if result["bead_id"] != "bd-123" {
		t.Errorf("bead_id = %v, want bd-123", result["bead_id"])
	}
	if result["bead_type"] != "task" {
		t.Errorf("bead_type = %v, want task", result["bead_type"])
	}
}

func TestWithAgent(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, nil)
	logger := slog.New(handler)

	enriched := WithAgent(logger, "claude-1")
	enriched.Info("test")

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if result["agent"] != "claude-1" {
		t.Errorf("agent = %v, want claude-1", result["agent"])
	}
}
