// Package agent provides agent lifecycle management via tmux.
package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// TmuxWrapper provides a low-level interface to tmux commands.
// It wraps tmux CLI operations for session management, key sending, and pane capture.
type TmuxWrapper struct {
	// defaultTimeout is used when context has no deadline
	defaultTimeout time.Duration
}

// NewTmuxWrapper creates a new tmux wrapper with default settings.
func NewTmuxWrapper() *TmuxWrapper {
	return &TmuxWrapper{
		defaultTimeout: 5 * time.Second,
	}
}

// SessionOptions configures session creation.
type SessionOptions struct {
	Name    string            // Session name (required)
	Workdir string            // Working directory for the session
	Env     map[string]string // Environment variables to set
	Width   int               // Terminal width (default: 200)
	Height  int               // Terminal height (default: 50)
	Command string            // Initial command to run in the session
}

// NewSession creates a new tmux session.
// Returns an error if a session with the same name already exists.
func (w *TmuxWrapper) NewSession(ctx context.Context, opts SessionOptions) error {
	if opts.Name == "" {
		return fmt.Errorf("session name is required")
	}

	// Check if session already exists
	if w.SessionExists(ctx, opts.Name) {
		return fmt.Errorf("session %s already exists", opts.Name)
	}

	args := []string{
		"new-session",
		"-d", // Detached
		"-s", opts.Name,
	}

	// Set terminal size
	width := opts.Width
	if width == 0 {
		width = 200
	}
	height := opts.Height
	if height == 0 {
		height = 50
	}
	args = append(args, "-x", fmt.Sprintf("%d", width), "-y", fmt.Sprintf("%d", height))

	// Set working directory
	if opts.Workdir != "" {
		args = append(args, "-c", opts.Workdir)
	}

	// Set environment variables
	for k, v := range opts.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// Add command if specified
	if opts.Command != "" {
		args = append(args, opts.Command)
	}

	output, err := w.runCmd(ctx, args...)
	if err != nil {
		return fmt.Errorf("creating tmux session: %w: %s", err, output)
	}

	return nil
}

// KillSession terminates a tmux session.
// Returns nil if the session doesn't exist (idempotent).
func (w *TmuxWrapper) KillSession(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("session name is required")
	}

	// Check if session exists first
	if !w.SessionExists(ctx, name) {
		return nil // Already gone
	}

	output, err := w.runCmd(ctx, "kill-session", "-t", name)
	if err != nil {
		// Ignore "session not found" errors (race between check and kill)
		if strings.Contains(string(output), "session not found") {
			return nil
		}
		return fmt.Errorf("killing tmux session: %w: %s", err, output)
	}

	return nil
}

// SessionExists checks if a tmux session exists.
func (w *TmuxWrapper) SessionExists(ctx context.Context, name string) bool {
	return w.hasSession(ctx, name)
}

// ListSessions returns all tmux session names, optionally filtered by prefix.
// If prefix is empty, all sessions are returned.
func (w *TmuxWrapper) ListSessions(ctx context.Context, prefix string) ([]string, error) {
	stdout, stderr, err := w.runCmdWithBuffers(ctx, "list-sessions", "-F", "#{session_name}")
	if err != nil {
		stderrStr := stderr.String()
		// Handle "no server running" - normal when tmux isn't started
		if strings.Contains(stderrStr, "no server running") ||
			strings.Contains(stderrStr, "no current session") {
			return nil, nil
		}
		// Handle case where tmux command fails with exit code 1 (no sessions)
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("listing tmux sessions: %w: %s", err, stderrStr)
	}

	var sessions []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if prefix == "" || strings.HasPrefix(line, prefix) {
			sessions = append(sessions, line)
		}
	}

	return sessions, nil
}

// SendKeys sends keystrokes to a tmux session, followed by Enter.
// This is the typical way to send commands to a session.
func (w *TmuxWrapper) SendKeys(ctx context.Context, session, keys string) error {
	return w.sendKeysInternal(ctx, session, keys, true)
}

// SendKeysLiteral sends keystrokes to a tmux session without pressing Enter.
// Use this for control sequences (like "C-c") or partial input.
func (w *TmuxWrapper) SendKeysLiteral(ctx context.Context, session, keys string) error {
	return w.sendKeysInternal(ctx, session, keys, false)
}

// sendKeysInternal sends keystrokes to a tmux session.
func (w *TmuxWrapper) sendKeysInternal(ctx context.Context, session, keys string, pressEnter bool) error {
	if session == "" {
		return fmt.Errorf("session name is required")
	}

	args := []string{"send-keys", "-t", session, keys}
	if pressEnter {
		args = append(args, "Enter")
	}

	output, err := w.runCmd(ctx, args...)
	if err != nil {
		return fmt.Errorf("send-keys: %w: %s", err, output)
	}

	return nil
}

// CapturePane captures the visible content of a tmux pane.
// Returns the text content of the pane.
func (w *TmuxWrapper) CapturePane(ctx context.Context, session string) (string, error) {
	return w.CapturePaneWithOptions(ctx, session, CapturePaneOptions{})
}

// CapturePaneOptions configures pane capture behavior.
type CapturePaneOptions struct {
	Start int  // Start line (negative = from bottom, 0 = visible start). Default: visible area.
	End   int  // End line (negative = from bottom, -1 = last line). Default: visible area.
	Escape bool // Include escape sequences (colors, etc.)
}

// CapturePaneWithOptions captures pane content with options.
func (w *TmuxWrapper) CapturePaneWithOptions(ctx context.Context, session string, opts CapturePaneOptions) (string, error) {
	if session == "" {
		return "", fmt.Errorf("session name is required")
	}

	args := []string{"capture-pane", "-t", session, "-p"} // -p prints to stdout

	// Add start/end if specified
	if opts.Start != 0 || opts.End != 0 {
		args = append(args, "-S", fmt.Sprintf("%d", opts.Start))
		args = append(args, "-E", fmt.Sprintf("%d", opts.End))
	}

	// Include escape sequences if requested
	if opts.Escape {
		args = append(args, "-e")
	}

	output, err := w.runCmd(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("capture-pane: %w: %s", err, output)
	}

	return string(output), nil
}

// SetEnv sets an environment variable in a tmux session.
// The variable will be available to new processes in the session.
func (w *TmuxWrapper) SetEnv(ctx context.Context, session, key, value string) error {
	if session == "" {
		return fmt.Errorf("session name is required")
	}
	if key == "" {
		return fmt.Errorf("environment variable name is required")
	}

	output, err := w.runCmd(ctx, "set-environment", "-t", session, key, value)
	if err != nil {
		return fmt.Errorf("set-environment: %w: %s", err, output)
	}

	return nil
}

// UnsetEnv removes an environment variable from a tmux session.
func (w *TmuxWrapper) UnsetEnv(ctx context.Context, session, key string) error {
	if session == "" {
		return fmt.Errorf("session name is required")
	}
	if key == "" {
		return fmt.Errorf("environment variable name is required")
	}

	output, err := w.runCmd(ctx, "set-environment", "-t", session, "-u", key)
	if err != nil {
		return fmt.Errorf("unset-environment: %w: %s", err, output)
	}

	return nil
}

// runCmd executes a tmux command with proper timeout handling.
// If the context has no deadline, a default timeout is applied.
func (w *TmuxWrapper) runCmd(ctx context.Context, args ...string) ([]byte, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.defaultTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "tmux", args...)
	return cmd.CombinedOutput()
}

// runCmdWithBuffers executes a tmux command with separate stdout/stderr buffers.
func (w *TmuxWrapper) runCmdWithBuffers(ctx context.Context, args ...string) (stdout, stderr *bytes.Buffer, err error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.defaultTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "tmux", args...)
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	return
}

// hasSession checks if a session exists (internal helper with proper timeout).
func (w *TmuxWrapper) hasSession(ctx context.Context, name string) bool {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, w.defaultTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}
