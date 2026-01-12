package testutil

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

// Tests for fixtures.go

func TestNewTestAgent(t *testing.T) {
	agent := NewTestAgent(t)

	if agent == nil {
		t.Fatal("NewTestAgent returned nil")
	}
	if agent.ID == "" {
		t.Error("Agent ID should not be empty")
	}
	if agent.Status != types.AgentStatusActive {
		t.Errorf("Agent status = %s, want active", agent.Status)
	}
	if err := agent.Validate(); err != nil {
		t.Errorf("Agent validation failed: %v", err)
	}
}

func TestNewTestConfig(t *testing.T) {
	cfg := NewTestConfig(t)

	if cfg == nil {
		t.Fatal("NewTestConfig returned nil")
	}
	if cfg.Version == "" {
		t.Error("Config version should not be empty")
	}

	// Check that directories were created
	if _, err := os.Stat(cfg.Paths.TemplateDir); os.IsNotExist(err) {
		t.Error("Template directory should exist")
	}
	if _, err := os.Stat(cfg.Paths.BeadsDir); os.IsNotExist(err) {
		t.Error("Beads directory should exist")
	}
	if _, err := os.Stat(cfg.Paths.StateDir); os.IsNotExist(err) {
		t.Error("State directory should exist")
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("Config validation failed: %v", err)
	}
}

func TestNewTestTemplate(t *testing.T) {
	tmpl := NewTestTemplate(t, "my-template")

	if tmpl == nil {
		t.Fatal("NewTestTemplate returned nil")
	}
	if tmpl.Meta.Name != "my-template" {
		t.Errorf("Template name = %s, want my-template", tmpl.Meta.Name)
	}
	if len(tmpl.Steps) == 0 {
		t.Error("Template should have at least one step")
	}
}

func TestNewTestWorkspace(t *testing.T) {
	workspace, cleanup := NewTestWorkspace(t)
	defer cleanup()

	if workspace == "" {
		t.Fatal("NewTestWorkspace returned empty path")
	}

	// Check structure
	requiredDirs := []string{
		".meow",
		".meow/templates",
		".meow/state",
		".beads",
	}
	for _, dir := range requiredDirs {
		path := filepath.Join(workspace, dir)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("Expected directory %s to exist", dir)
		}
	}

	// Check config file
	configPath := filepath.Join(workspace, ".meow", "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Config file should exist")
	}
}

// Tests for assertions.go

func TestAssertions_Basic(t *testing.T) {
	// These should pass
	AssertEqual(t, 1, 1)
	AssertNotEqual(t, 1, 2)
	AssertNil(t, nil)
	AssertNotNil(t, "something")
	AssertTrue(t, true)
	AssertFalse(t, false)
	AssertContains(t, "hello world", "world")
	AssertNotContains(t, "hello world", "foo")
	AssertLen(t, []int{1, 2, 3}, 3)
	AssertEmpty(t, []int{})
	AssertNotEmpty(t, []int{1})
}

func TestAssertions_Error(t *testing.T) {
	err := os.ErrNotExist
	AssertError(t, err)
	AssertErrorContains(t, err, "not exist")

	AssertNoError(t, nil)
}

func TestAssertions_Agent(t *testing.T) {
	agent := NewTestAgent(t)
	AssertAgentActive(t, agent)
	AssertAgentValid(t, agent)

	agent.Stop()
	AssertAgentStopped(t, agent)
}

func TestAssertions_File(t *testing.T) {
	// Create a temp file
	tmpFile := TempFile(t, "test-*.txt", "test content")

	AssertFileExists(t, tmpFile)
	AssertFileContains(t, tmpFile, "test content")
}

// Tests for logger.go

func TestTestLogger_Basic(t *testing.T) {
	logger := NewTestLogger(t)

	// Log some entries
	logger.Logger.Info("info message")
	logger.Logger.Warn("warning message")
	logger.Logger.Error("error message")

	// Check counts
	if logger.Count() != 3 {
		t.Errorf("Count = %d, want 3", logger.Count())
	}
	if logger.CountLevel(slog.LevelInfo) != 1 {
		t.Errorf("Info count = %d, want 1", logger.CountLevel(slog.LevelInfo))
	}
	if logger.CountLevel(slog.LevelError) != 1 {
		t.Errorf("Error count = %d, want 1", logger.CountLevel(slog.LevelError))
	}
}

func TestTestLogger_Assertions(t *testing.T) {
	logger := NewTestLogger(t)

	logger.Logger.Info("test message", "key", "value")
	logger.Logger.Error("error occurred")

	logger.AssertContains(t, "test message")
	logger.AssertLevel(t, slog.LevelInfo, 1)
	logger.AssertLevelAtLeast(t, slog.LevelInfo, 1)
	logger.AssertHasError(t)
	logger.AssertErrorContains(t, "error occurred")
	logger.AssertAttrPresent(t, "key")
	logger.AssertCount(t, 2)
}

func TestTestLogger_WithAttrs(t *testing.T) {
	logger := NewTestLogger(t)

	logger.Logger.With("step_id", "step-001").Info("working on step")

	entries := logger.GetEntriesWithAttr("step_id")
	if len(entries) != 1 {
		t.Errorf("Expected 1 entry with step_id, got %d", len(entries))
	}
}

func TestDiscardLogger(t *testing.T) {
	logger := DiscardLogger()

	// Should not panic
	logger.Info("this goes nowhere")
	logger.Error("neither does this")
}

// Tests for cleanup.go

func TestCleanupManager(t *testing.T) {
	// Create a subtest so we can control cleanup timing
	t.Run("cleanup", func(t *testing.T) {
		cm := NewCleanupManager(t)

		// Create a temp file to track
		tmpFile := TempFile(t, "cleanup-test-*.txt", "test")
		cm.AddFile(tmpFile)

		// File should exist
		if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
			t.Error("File should exist before cleanup")
		}

		// Manually cleanup
		cm.Cleanup()

		// File should be gone
		if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
			t.Error("File should not exist after cleanup")
		}

		// Multiple cleanups should be safe
		cm.Cleanup() // Should not panic
	})
}

func TestTempDirWithFiles(t *testing.T) {
	dir := TempDirWithFiles(t, "test-", map[string]string{
		"file1.txt":        "content1",
		"subdir/file2.txt": "content2",
	})

	// Check files exist
	AssertFileExists(t, filepath.Join(dir, "file1.txt"))
	AssertFileExists(t, filepath.Join(dir, "subdir", "file2.txt"))

	// Check content
	AssertFileContains(t, filepath.Join(dir, "file1.txt"), "content1")
	AssertFileContains(t, filepath.Join(dir, "subdir", "file2.txt"), "content2")
}

func TestPreserveOnFailure(t *testing.T) {
	// This is a passing test, so the dir should be cleaned up
	dir := PreserveOnFailure(t, "preserve-test-")

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("Dir should exist during test")
	}
	// Cleanup happens automatically after test
}
