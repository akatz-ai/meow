package agent

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestNewTmuxWrapper(t *testing.T) {
	w := NewTmuxWrapper()
	if w == nil {
		t.Fatal("NewTmuxWrapper() returned nil")
	}
	if w.defaultTimeout != 5*time.Second {
		t.Errorf("defaultTimeout = %v, want 5s", w.defaultTimeout)
	}
	if w.socketPath != "" {
		t.Errorf("socketPath should be empty by default, got %q", w.socketPath)
	}
}

func TestNewTmuxWrapper_WithOptions(t *testing.T) {
	w := NewTmuxWrapper(
		WithSocketPath("/tmp/test.sock"),
		WithTimeout(10*time.Second),
	)
	if w.socketPath != "/tmp/test.sock" {
		t.Errorf("socketPath = %q, want /tmp/test.sock", w.socketPath)
	}
	if w.defaultTimeout != 10*time.Second {
		t.Errorf("defaultTimeout = %v, want 10s", w.defaultTimeout)
	}
}

func TestTmuxWrapper_BuildArgs_NoSocket(t *testing.T) {
	w := NewTmuxWrapper()
	args := w.buildArgs("list-sessions", "-F", "#{session_name}")
	expected := []string{"list-sessions", "-F", "#{session_name}"}
	if len(args) != len(expected) {
		t.Fatalf("len(args) = %d, want %d", len(args), len(expected))
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, arg, expected[i])
		}
	}
}

func TestTmuxWrapper_BuildArgs_WithSocket(t *testing.T) {
	w := NewTmuxWrapper(WithSocketPath("/tmp/test.sock"))
	args := w.buildArgs("list-sessions", "-F", "#{session_name}")
	expected := []string{"-S", "/tmp/test.sock", "list-sessions", "-F", "#{session_name}"}
	if len(args) != len(expected) {
		t.Fatalf("len(args) = %d, want %d", len(args), len(expected))
	}
	for i, arg := range args {
		if arg != expected[i] {
			t.Errorf("args[%d] = %q, want %q", i, arg, expected[i])
		}
	}
}

func TestTmuxWrapper_NewSession_EmptyName(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.NewSession(context.Background(), SessionOptions{})
	if err == nil {
		t.Error("NewSession() should fail with empty name")
	}
	if !strings.Contains(err.Error(), "name is required") {
		t.Errorf("Error message should mention name is required, got: %v", err)
	}
}

func TestTmuxWrapper_KillSession_EmptyName(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.KillSession(context.Background(), "")
	if err == nil {
		t.Error("KillSession() should fail with empty name")
	}
}

func TestTmuxWrapper_SendKeys_EmptySession(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.SendKeys(context.Background(), "", "test")
	if err == nil {
		t.Error("SendKeys() should fail with empty session")
	}
}

func TestTmuxWrapper_SendKeysLiteral_EmptySession(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.SendKeysLiteral(context.Background(), "", "test")
	if err == nil {
		t.Error("SendKeysLiteral() should fail with empty session")
	}
}

func TestTmuxWrapper_SendKeysSpecial_EmptySession(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.SendKeysSpecial(context.Background(), "", "C-c")
	if err == nil {
		t.Error("SendKeysSpecial() should fail with empty session")
	}
}

func TestTmuxWrapper_CapturePane_EmptySession(t *testing.T) {
	w := NewTmuxWrapper()
	_, err := w.CapturePane(context.Background(), "")
	if err == nil {
		t.Error("CapturePane() should fail with empty session")
	}
}

func TestTmuxWrapper_SetEnv_EmptySession(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.SetEnv(context.Background(), "", "KEY", "VALUE")
	if err == nil {
		t.Error("SetEnv() should fail with empty session")
	}
}

func TestTmuxWrapper_SetEnv_EmptyKey(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.SetEnv(context.Background(), "test-session", "", "VALUE")
	if err == nil {
		t.Error("SetEnv() should fail with empty key")
	}
}

func TestTmuxWrapper_UnsetEnv_EmptySession(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.UnsetEnv(context.Background(), "", "KEY")
	if err == nil {
		t.Error("UnsetEnv() should fail with empty session")
	}
}

func TestTmuxWrapper_UnsetEnv_EmptyKey(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.UnsetEnv(context.Background(), "test-session", "")
	if err == nil {
		t.Error("UnsetEnv() should fail with empty key")
	}
}

// Integration tests (only run if tmux is available)

func tmuxWrapperAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func TestTmuxWrapper_Integration_SessionExists_NonExistent(t *testing.T) {
	if !tmuxWrapperAvailable() {
		t.Skip("tmux not available")
	}

	w := NewTmuxWrapper()
	exists := w.SessionExists(context.Background(), "nonexistent-session-12345")
	if exists {
		t.Error("SessionExists() should return false for non-existent session")
	}
}

func TestTmuxWrapper_Integration_ListSessions_NoServer(t *testing.T) {
	if !tmuxWrapperAvailable() {
		t.Skip("tmux not available")
	}

	w := NewTmuxWrapper()
	// This should not error even if no tmux server is running
	sessions, err := w.ListSessions(context.Background(), "")
	if err != nil {
		t.Errorf("ListSessions() error = %v", err)
	}
	// sessions may or may not be empty depending on system state
	_ = sessions
}

func TestTmuxWrapper_Integration_ListSessions_WithPrefix(t *testing.T) {
	if !tmuxWrapperAvailable() {
		t.Skip("tmux not available")
	}

	w := NewTmuxWrapper()
	sessions, err := w.ListSessions(context.Background(), "meow-")
	if err != nil {
		t.Errorf("ListSessions() error = %v", err)
	}
	// All returned sessions should have the prefix
	for _, s := range sessions {
		if !strings.HasPrefix(s, "meow-") {
			t.Errorf("Session %q doesn't have prefix 'meow-'", s)
		}
	}
}

func TestTmuxWrapper_Integration_KillSession_NonExistent(t *testing.T) {
	if !tmuxWrapperAvailable() {
		t.Skip("tmux not available")
	}

	w := NewTmuxWrapper()
	// Should not error for non-existent session (idempotent)
	err := w.KillSession(context.Background(), "nonexistent-session-12345")
	if err != nil {
		t.Errorf("KillSession() error = %v for non-existent session", err)
	}
}

func TestTmuxWrapper_Integration_FullLifecycle(t *testing.T) {
	if !tmuxWrapperAvailable() {
		t.Skip("tmux not available")
	}

	w := NewTmuxWrapper()
	ctx := context.Background()
	sessionName := "test-wrapper-" + time.Now().Format("150405")

	// Ensure cleanup
	defer func() {
		_ = w.KillSession(ctx, sessionName)
	}()

	// 1. Create session
	err := w.NewSession(ctx, SessionOptions{
		Name:    sessionName,
		Workdir: "/tmp",
		Env: map[string]string{
			"TEST_VAR": "test_value",
		},
	})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	// 2. Verify session exists
	if !w.SessionExists(ctx, sessionName) {
		t.Error("Session should exist after creation")
	}

	// 3. Verify session is in list
	sessions, err := w.ListSessions(ctx, "test-wrapper-")
	if err != nil {
		t.Errorf("ListSessions() error = %v", err)
	}
	found := false
	for _, s := range sessions {
		if s == sessionName {
			found = true
			break
		}
	}
	if !found {
		t.Error("Session should be in list")
	}

	// 4. Send keys
	err = w.SendKeys(ctx, sessionName, "echo hello")
	if err != nil {
		t.Errorf("SendKeys() error = %v", err)
	}

	// 5. Wait a bit for command to execute
	time.Sleep(100 * time.Millisecond)

	// 6. Capture pane
	content, err := w.CapturePane(ctx, sessionName)
	if err != nil {
		t.Errorf("CapturePane() error = %v", err)
	}
	// Should contain something (at minimum the echo command and output)
	if len(content) == 0 {
		t.Error("CapturePane() returned empty content")
	}

	// 7. Set environment variable
	err = w.SetEnv(ctx, sessionName, "NEW_VAR", "new_value")
	if err != nil {
		t.Errorf("SetEnv() error = %v", err)
	}

	// 8. Unset environment variable
	err = w.UnsetEnv(ctx, sessionName, "NEW_VAR")
	if err != nil {
		t.Errorf("UnsetEnv() error = %v", err)
	}

	// 9. Send literal keys (no Enter)
	err = w.SendKeysLiteral(ctx, sessionName, "C-c")
	if err != nil {
		t.Errorf("SendKeysLiteral() error = %v", err)
	}

	// 10. Kill session
	err = w.KillSession(ctx, sessionName)
	if err != nil {
		t.Errorf("KillSession() error = %v", err)
	}

	// 11. Verify session is gone
	if w.SessionExists(ctx, sessionName) {
		t.Error("Session should not exist after kill")
	}
}

func TestTmuxWrapper_Integration_NewSession_AlreadyExists(t *testing.T) {
	if !tmuxWrapperAvailable() {
		t.Skip("tmux not available")
	}

	w := NewTmuxWrapper()
	ctx := context.Background()
	sessionName := "test-duplicate-" + time.Now().Format("150405")

	// Ensure cleanup
	defer func() {
		_ = w.KillSession(ctx, sessionName)
	}()

	// Create first session
	err := w.NewSession(ctx, SessionOptions{Name: sessionName})
	if err != nil {
		t.Fatalf("First NewSession() error = %v", err)
	}

	// Try to create duplicate - should fail
	err = w.NewSession(ctx, SessionOptions{Name: sessionName})
	if err == nil {
		t.Error("Second NewSession() should fail for existing session")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("Error should mention 'already exists', got: %v", err)
	}
}

func TestTmuxWrapper_Integration_CapturePaneWithOptions(t *testing.T) {
	if !tmuxWrapperAvailable() {
		t.Skip("tmux not available")
	}

	w := NewTmuxWrapper()
	ctx := context.Background()
	sessionName := "test-capture-" + time.Now().Format("150405")

	// Ensure cleanup
	defer func() {
		_ = w.KillSession(ctx, sessionName)
	}()

	// Create session
	err := w.NewSession(ctx, SessionOptions{Name: sessionName})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	// Send some output
	_ = w.SendKeys(ctx, sessionName, "echo line1")
	_ = w.SendKeys(ctx, sessionName, "echo line2")
	_ = w.SendKeys(ctx, sessionName, "echo line3")
	time.Sleep(200 * time.Millisecond)

	// Capture with default options
	content, err := w.CapturePane(ctx, sessionName)
	if err != nil {
		t.Errorf("CapturePane() error = %v", err)
	}
	if len(content) == 0 {
		t.Error("CapturePane() returned empty content")
	}

	// Capture with options (just verify it doesn't error)
	_, err = w.CapturePaneWithOptions(ctx, sessionName, CapturePaneOptions{
		Start: -10,
		End:   -1,
	})
	if err != nil {
		t.Errorf("CapturePaneWithOptions() error = %v", err)
	}
}

func TestTmuxWrapper_PipePaneToFile_EmptySession(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.PipePaneToFile(context.Background(), "", "/tmp/test.log")
	if err == nil || !strings.Contains(err.Error(), "session name is required") {
		t.Errorf("Expected 'session name is required' error, got: %v", err)
	}
}

func TestTmuxWrapper_PipePaneToFile_EmptyLogPath(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.PipePaneToFile(context.Background(), "test-session", "")
	if err == nil || !strings.Contains(err.Error(), "log path is required") {
		t.Errorf("Expected 'log path is required' error, got: %v", err)
	}
}

func TestTmuxWrapper_StopPipePane_EmptySession(t *testing.T) {
	w := NewTmuxWrapper()
	err := w.StopPipePane(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "session name is required") {
		t.Errorf("Expected 'session name is required' error, got: %v", err)
	}
}

func TestTmuxWrapper_Integration_PipePaneToFile(t *testing.T) {
	if !tmuxWrapperAvailable() {
		t.Skip("tmux not available")
	}

	w := NewTmuxWrapper()
	ctx := context.Background()
	sessionName := "test-pipepane-" + time.Now().Format("150405")

	// Create temp file for logging
	logFile, err := os.CreateTemp("", "tmux-log-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	logPath := logFile.Name()
	logFile.Close()
	defer os.Remove(logPath)

	// Ensure cleanup
	defer func() {
		_ = w.KillSession(ctx, sessionName)
	}()

	// Create session
	err = w.NewSession(ctx, SessionOptions{Name: sessionName})
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}

	// Set up pipe-pane logging
	err = w.PipePaneToFile(ctx, sessionName, logPath)
	if err != nil {
		t.Fatalf("PipePaneToFile() error = %v", err)
	}

	// Send some commands
	_ = w.SendKeys(ctx, sessionName, "echo MARKER_START")
	_ = w.SendKeys(ctx, sessionName, "echo Hello from pipe-pane test")
	_ = w.SendKeys(ctx, sessionName, "echo MARKER_END")
	time.Sleep(500 * time.Millisecond)

	// Read log file
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	// Verify output was captured
	logContent := string(content)
	if !strings.Contains(logContent, "MARKER_START") {
		t.Error("Log file should contain MARKER_START")
	}
	if !strings.Contains(logContent, "Hello from pipe-pane test") {
		t.Error("Log file should contain test message")
	}
	if !strings.Contains(logContent, "MARKER_END") {
		t.Error("Log file should contain MARKER_END")
	}

	// Stop pipe-pane
	err = w.StopPipePane(ctx, sessionName)
	if err != nil {
		t.Errorf("StopPipePane() error = %v", err)
	}
}
