package testutil

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// CleanupManager tracks resources that need cleanup after tests.
type CleanupManager struct {
	mu        sync.Mutex
	t         *testing.T
	resources []cleanupResource
	done      bool
}

// cleanupResource represents a resource that needs cleanup.
type cleanupResource struct {
	name    string
	cleanup func() error
}

// NewCleanupManager creates a cleanup manager for a test.
// The manager will automatically clean up resources when the test completes.
func NewCleanupManager(t *testing.T) *CleanupManager {
	t.Helper()

	cm := &CleanupManager{
		t:         t,
		resources: make([]cleanupResource, 0),
	}

	// Register cleanup to run at test end
	t.Cleanup(func() {
		cm.Cleanup()
	})

	return cm
}

// AddFile registers a file for cleanup.
func (cm *CleanupManager) AddFile(path string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.resources = append(cm.resources, cleanupResource{
		name: "file:" + path,
		cleanup: func() error {
			return os.Remove(path)
		},
	})
}

// AddDir registers a directory for recursive cleanup.
func (cm *CleanupManager) AddDir(path string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.resources = append(cm.resources, cleanupResource{
		name: "dir:" + path,
		cleanup: func() error {
			return os.RemoveAll(path)
		},
	})
}

// AddTmuxSession registers a tmux session for cleanup.
func (cm *CleanupManager) AddTmuxSession(sessionName string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.resources = append(cm.resources, cleanupResource{
		name: "tmux:" + sessionName,
		cleanup: func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "tmux", "kill-session", "-t", sessionName)
			// Ignore errors - session may not exist
			_ = cmd.Run()
			return nil
		},
	})
}

// AddFunc registers a custom cleanup function.
func (cm *CleanupManager) AddFunc(name string, fn func() error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.resources = append(cm.resources, cleanupResource{
		name:    name,
		cleanup: fn,
	})
}

// Cleanup runs all cleanup functions in reverse order.
// It's safe to call multiple times - subsequent calls are no-ops.
func (cm *CleanupManager) Cleanup() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	if cm.done {
		return
	}
	cm.done = true

	// Clean up in reverse order (LIFO)
	for i := len(cm.resources) - 1; i >= 0; i-- {
		res := cm.resources[i]
		if err := res.cleanup(); err != nil {
			// Log but don't fail - we want to try all cleanups
			cm.t.Logf("Cleanup warning for %s: %v", res.name, err)
		}
	}
}

// Standalone cleanup functions

// CleanupTmuxSessions kills all tmux sessions with the given prefix.
func CleanupTmuxSessions(t *testing.T, prefix string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// List all sessions
	cmd := exec.CommandContext(ctx, "tmux", "list-sessions", "-F", "#{session_name}")
	output, err := cmd.Output()
	if err != nil {
		// No sessions or tmux not running - that's fine
		return
	}

	// Kill matching sessions
	for _, session := range strings.Split(string(output), "\n") {
		session = strings.TrimSpace(session)
		if session != "" && strings.HasPrefix(session, prefix) {
			killCmd := exec.CommandContext(ctx, "tmux", "kill-session", "-t", session)
			_ = killCmd.Run() // Ignore errors
		}
	}
}

// CleanupMeowSessions kills all meow-* tmux sessions.
func CleanupMeowSessions(t *testing.T) {
	t.Helper()
	CleanupTmuxSessions(t, "meow-")
}

// CleanupTestSessions kills all test-* tmux sessions.
func CleanupTestSessions(t *testing.T) {
	t.Helper()
	CleanupTmuxSessions(t, "test-")
}

// CleanupWorkspace removes a workspace directory and all its contents.
func CleanupWorkspace(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Logf("Warning: failed to clean up workspace %s: %v", path, err)
	}
}

// CleanupPattern removes files matching a glob pattern.
func CleanupPattern(t *testing.T, pattern string) {
	t.Helper()

	matches, err := filepath.Glob(pattern)
	if err != nil {
		t.Logf("Warning: invalid cleanup pattern %s: %v", pattern, err)
		return
	}

	for _, path := range matches {
		if err := os.RemoveAll(path); err != nil {
			t.Logf("Warning: failed to clean up %s: %v", path, err)
		}
	}
}

// EnsureCleanState ensures no leftover state from previous test runs.
// Call this at the beginning of integration tests.
func EnsureCleanState(t *testing.T) {
	t.Helper()

	// Kill any existing meow sessions
	CleanupMeowSessions(t)

	// Remove any temp test files
	patterns := []string{
		"/tmp/meow-test-*",
		"/tmp/test-workspace-*",
	}
	for _, p := range patterns {
		CleanupPattern(t, p)
	}
}

// TempFile creates a temporary file and registers it for cleanup.
func TempFile(t *testing.T, pattern, content string) string {
	t.Helper()

	f, err := os.CreateTemp("", pattern)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	if content != "" {
		if _, err := f.WriteString(content); err != nil {
			f.Close()
			os.Remove(f.Name())
			t.Fatalf("Failed to write temp file: %v", err)
		}
	}

	f.Close()
	t.Cleanup(func() {
		os.Remove(f.Name())
	})

	return f.Name()
}

// TempDir creates a temporary directory and registers it for cleanup.
func TempDir(t *testing.T, pattern string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	t.Cleanup(func() {
		os.RemoveAll(dir)
	})

	return dir
}

// TempDirWithFiles creates a temporary directory with the given files.
// files is a map of relative paths to content.
func TempDirWithFiles(t *testing.T, pattern string, files map[string]string) string {
	t.Helper()

	dir := TempDir(t, pattern)

	for relPath, content := range files {
		fullPath := filepath.Join(dir, relPath)

		// Create parent directories if needed
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create parent dir for %s: %v", relPath, err)
		}

		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write file %s: %v", relPath, err)
		}
	}

	return dir
}

// WithCleanup wraps a function with cleanup on panic.
// Useful for ensuring cleanup runs even if the test panics.
func WithCleanup(t *testing.T, cleanup func(), fn func()) {
	t.Helper()

	defer func() {
		cleanup()
		if r := recover(); r != nil {
			t.Fatalf("Panic during test: %v", r)
		}
	}()

	fn()
}

// MustCleanup runs cleanup and fails the test if it errors.
func MustCleanup(t *testing.T, name string, fn func() error) {
	t.Helper()

	if err := fn(); err != nil {
		t.Errorf("Cleanup failed for %s: %v", name, err)
	}
}

// CleanupOnFailure registers a cleanup function that only runs if the test fails.
func CleanupOnFailure(t *testing.T, name string, fn func() error) {
	t.Helper()

	t.Cleanup(func() {
		if t.Failed() {
			if err := fn(); err != nil {
				t.Logf("Post-failure cleanup error for %s: %v", name, err)
			}
		}
	})
}

// PreserveOnFailure creates a temp directory that is only cleaned up if the test passes.
// Useful for debugging failing tests.
func PreserveOnFailure(t *testing.T, pattern string) string {
	t.Helper()

	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("Test failed - preserving temp directory: %s", dir)
		} else {
			os.RemoveAll(dir)
		}
	})

	return dir
}
