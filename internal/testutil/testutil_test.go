package testutil

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

// Tests for fixtures.go

func TestNewTestBead(t *testing.T) {
	tests := []struct {
		name     string
		beadType types.BeadType
	}{
		{"task", types.BeadTypeTask},
		{"condition", types.BeadTypeCondition},
		{"stop", types.BeadTypeStop},
		{"start", types.BeadTypeStart},
		{"code", types.BeadTypeCode},
		{"expand", types.BeadTypeExpand},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bead := NewTestBead(t, tt.beadType)

			if bead == nil {
				t.Fatal("NewTestBead returned nil")
			}
			if bead.ID == "" {
				t.Error("Bead ID should not be empty")
			}
			if bead.Type != tt.beadType {
				t.Errorf("Bead type = %s, want %s", bead.Type, tt.beadType)
			}
			if bead.Status != types.BeadStatusOpen {
				t.Errorf("Bead status = %s, want open", bead.Status)
			}
			if err := bead.Validate(); err != nil {
				t.Errorf("Bead validation failed: %v", err)
			}
		})
	}
}

func TestNewTestTaskBead_WithOptions(t *testing.T) {
	bead := NewTestTaskBead(t,
		WithID("custom-id"),
		WithTitle("Custom Title"),
		WithStatus(types.BeadStatusInProgress),
		WithNeeds("dep-1", "dep-2"),
		WithLabels("test", "priority:high"),
		WithAssignee("claude-1"),
		WithInstructions("Do something"),
	)

	if bead.ID != "custom-id" {
		t.Errorf("ID = %s, want custom-id", bead.ID)
	}
	if bead.Title != "Custom Title" {
		t.Errorf("Title = %s, want Custom Title", bead.Title)
	}
	if bead.Status != types.BeadStatusInProgress {
		t.Errorf("Status = %s, want in_progress", bead.Status)
	}
	if len(bead.Needs) != 2 {
		t.Errorf("Needs length = %d, want 2", len(bead.Needs))
	}
	if len(bead.Labels) != 2 {
		t.Errorf("Labels length = %d, want 2", len(bead.Labels))
	}
	if bead.Assignee != "claude-1" {
		t.Errorf("Assignee = %s, want claude-1", bead.Assignee)
	}
	if bead.Instructions != "Do something" {
		t.Errorf("Instructions = %s, want 'Do something'", bead.Instructions)
	}
}

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

// Tests for mock_tmux.go

func TestMockTmux_StartStop(t *testing.T) {
	mock := NewMockTmux(t)
	ctx := context.Background()

	// Start an agent
	err := mock.Start(ctx, &types.StartSpec{
		Agent:   "test-agent",
		Workdir: "/tmp",
	})
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Check it's running
	running, err := mock.IsRunning(ctx, "test-agent")
	if err != nil {
		t.Fatalf("IsRunning failed: %v", err)
	}
	if !running {
		t.Error("Agent should be running")
	}

	// Check session count
	if mock.SessionCount() != 1 {
		t.Errorf("Session count = %d, want 1", mock.SessionCount())
	}

	// Stop the agent
	err = mock.Stop(ctx, &types.StopSpec{Agent: "test-agent"})
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Check it's not running
	running, err = mock.IsRunning(ctx, "test-agent")
	if err != nil {
		t.Fatalf("IsRunning failed: %v", err)
	}
	if running {
		t.Error("Agent should not be running")
	}
}

func TestMockTmux_SendCommand(t *testing.T) {
	mock := NewMockTmux(t)
	ctx := context.Background()

	// Start an agent
	_ = mock.Start(ctx, &types.StartSpec{Agent: "test-agent"})

	// Send commands
	_ = mock.SendCommand(ctx, "test-agent", "meow prime")
	_ = mock.SendCommand(ctx, "test-agent", "meow close bd-001")

	// Check commands were recorded
	commands := mock.GetCommands("test-agent")
	if len(commands) < 2 {
		t.Errorf("Expected at least 2 commands, got %d", len(commands))
	}
}

func TestMockTmux_FailureMode(t *testing.T) {
	mock := NewMockTmux(t)
	mock.FailStart = true
	ctx := context.Background()

	err := mock.Start(ctx, &types.StartSpec{Agent: "test-agent"})
	if err == nil {
		t.Error("Expected Start to fail")
	}
}

func TestMockTmux_Assertions(t *testing.T) {
	mock := NewMockTmux(t)
	ctx := context.Background()

	_ = mock.Start(ctx, &types.StartSpec{Agent: "agent-1"})
	_ = mock.SendCommand(ctx, "agent-1", "echo hello")

	// AssertCommandSent should work while session is active
	mock.AssertCommandSent(t, "agent-1", "echo hello")

	_ = mock.Stop(ctx, &types.StopSpec{Agent: "agent-1"})

	// These should not fail - events are recorded
	mock.AssertSessionStarted(t, "agent-1")
	mock.AssertSessionStopped(t, "agent-1")
}

// Tests for mock_claude.go

func TestMockClaude_Execute(t *testing.T) {
	mock := NewMockClaude(t)
	mock.SetCurrentBead("bd-001")

	// Test meow prime
	output, exitCode, err := mock.Execute("meow prime --format json")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Exit code = %d, want 0", exitCode)
	}
	if output == "" {
		t.Error("Output should not be empty")
	}

	// Test meow close
	output, exitCode, err = mock.Execute("meow close bd-001")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("Exit code = %d, want 0", exitCode)
	}

	// Check bead was completed
	completed := mock.GetCompletedBeads()
	if len(completed) != 1 || completed[0] != "bd-001" {
		t.Errorf("Completed beads = %v, want [bd-001]", completed)
	}
}

func TestMockClaude_CustomResponse(t *testing.T) {
	mock := NewMockClaude(t)
	mock.SetResponse("custom cmd", "custom output", 0)

	output, exitCode, _ := mock.Execute("custom cmd arg1 arg2")
	if output != "custom output" {
		t.Errorf("Output = %s, want 'custom output'", output)
	}
	if exitCode != 0 {
		t.Errorf("Exit code = %d, want 0", exitCode)
	}
}

func TestMockClaude_FailOnCommand(t *testing.T) {
	mock := NewMockClaude(t)
	mock.FailOnCommand = "bad command"
	mock.FailError = "test failure"

	_, exitCode, err := mock.Execute("bad command")
	if err == nil {
		t.Error("Expected error")
	}
	if exitCode != 1 {
		t.Errorf("Exit code = %d, want 1", exitCode)
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

func TestAssertions_Bead(t *testing.T) {
	bead := NewTestBead(t, types.BeadTypeTask)
	AssertBeadOpen(t, bead)
	AssertBeadType(t, bead, types.BeadTypeTask)
	AssertBeadValid(t, bead)

	bead.Status = types.BeadStatusInProgress
	AssertBeadInProgress(t, bead)

	bead.Close(map[string]any{"result": "ok"})
	AssertBeadClosed(t, bead)
	AssertBeadHasOutput(t, bead, "result", "ok")
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

	logger.Logger.With("bead_id", "bd-001").Info("working on bead")

	entries := logger.GetEntriesWithAttr("bead_id")
	if len(entries) != 1 {
		t.Errorf("Expected 1 entry with bead_id, got %d", len(entries))
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
		"file1.txt":     "content1",
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

// Integration test for all components working together

func TestIntegration_MockWorkflow(t *testing.T) {
	// Setup
	workspace, cleanup := NewTestWorkspace(t)
	defer cleanup()

	mock := NewMockTmux(t)
	logger := NewTestLogger(t)
	ctx := context.Background()

	// Create test beads
	bead1 := NewTestTaskBead(t, WithID("bd-001"), WithTitle("First task"))
	bead2 := NewTestTaskBead(t, WithID("bd-002"), WithTitle("Second task"), WithNeeds("bd-001"))

	// Start agent
	logger.Logger.Info("starting agent", "workspace", workspace)
	err := mock.Start(ctx, &types.StartSpec{
		Agent:   "claude-1",
		Workdir: workspace,
	})
	RequireNoError(t, err)

	// Execute first task
	logger.Logger.Info("executing task", "bead_id", bead1.ID)
	mock.SendCommand(ctx, "claude-1", "meow prime")

	// Complete first task
	bead1.Status = types.BeadStatusInProgress
	_ = bead1.Close(map[string]any{"result": "done"})
	logger.Logger.Info("task completed", "bead_id", bead1.ID)

	// Execute second task
	logger.Logger.Info("executing task", "bead_id", bead2.ID)

	// Complete second task
	bead2.Status = types.BeadStatusInProgress
	_ = bead2.Close(nil)

	// Stop agent
	err = mock.Stop(ctx, &types.StopSpec{Agent: "claude-1"})
	RequireNoError(t, err)

	// Verify state
	AssertBeadClosed(t, bead1)
	AssertBeadClosed(t, bead2)
	mock.AssertSessionStarted(t, "claude-1")
	mock.AssertSessionStopped(t, "claude-1")
	logger.AssertNoErrors(t)
	logger.AssertAttrPresent(t, "bead_id")
}
