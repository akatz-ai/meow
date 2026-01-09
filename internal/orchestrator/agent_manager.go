package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// TmuxAgentManager implements AgentManager using tmux sessions.
type TmuxAgentManager struct {
	logger *slog.Logger

	mu      sync.RWMutex
	agents  map[string]*agentState // agentID -> state
	workdir string                 // Base working directory
}

type agentState struct {
	tmuxSession   string
	workflowID    string
	currentStepID string
	workdir       string
}

// NewTmuxAgentManager creates a new TmuxAgentManager.
func NewTmuxAgentManager(workdir string, logger *slog.Logger) *TmuxAgentManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &TmuxAgentManager{
		logger:  logger.With("component", "agent-manager"),
		agents:  make(map[string]*agentState),
		workdir: workdir,
	}
}

// Start spawns an agent in a tmux session with Claude Code.
func (m *TmuxAgentManager) Start(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Spawn == nil {
		return fmt.Errorf("spawn step missing config")
	}

	cfg := step.Spawn
	agentID := cfg.Agent

	// Build tmux session name
	sessionName := BuildTmuxSessionName(wf.ID, agentID)

	// Determine working directory
	workdir := cfg.Workdir
	if workdir == "" {
		workdir = m.workdir
	}
	if !filepath.IsAbs(workdir) {
		workdir = filepath.Join(m.workdir, workdir)
	}

	m.logger.Info("spawning agent", "agent", agentID, "session", sessionName, "workdir", workdir)

	// Check if session already exists
	if m.sessionExists(sessionName) {
		m.logger.Warn("tmux session already exists", "session", sessionName)
		// Attach to existing session instead of creating new
	} else {
		// Build environment variables
		env := make(map[string]string)
		for k, v := range cfg.Env {
			env[k] = v
		}
		// Orchestrator-injected vars (reserved, override user values)
		env["MEOW_AGENT"] = agentID
		env["MEOW_WORKFLOW"] = wf.ID
		// Prevent Claude from detecting it's in an outer tmux session
		// This avoids potential interactions with the parent tmux server
		env["TMUX"] = ""

		// Create tmux session with bash - we'll start claude via send-keys
		// This ensures the session stays alive and we can inject prompts
		if err := m.createSession(ctx, sessionName, workdir, env, "bash"); err != nil {
			return fmt.Errorf("creating tmux session: %w", err)
		}

		// Build and start claude command via send-keys
		// Use --dangerously-skip-permissions to bypass trust dialog in automated context
		claudeCmd := "claude --dangerously-skip-permissions"
		if cfg.ResumeSession != "" {
			claudeCmd = fmt.Sprintf("claude --dangerously-skip-permissions --resume %s", cfg.ResumeSession)
		}

		// Give the session a moment to initialize
		time.Sleep(100 * time.Millisecond)

		// Start claude in the session
		if err := m.sendKeys(ctx, sessionName, claudeCmd); err != nil {
			return fmt.Errorf("sending claude command: %w", err)
		}
		if err := m.sendKeys(ctx, sessionName, "Enter"); err != nil {
			return fmt.Errorf("sending Enter: %w", err)
		}

		// Wait for Claude to fully start up before returning
		// This ensures it's ready to receive prompts
		m.logger.Info("waiting for Claude to start", "agent", agentID)
		time.Sleep(3 * time.Second)
	}

	// Register agent state
	m.mu.Lock()
	m.agents[agentID] = &agentState{
		tmuxSession: sessionName,
		workflowID:  wf.ID,
		workdir:     workdir,
	}
	m.mu.Unlock()

	// Register agent in workflow for file_path validation
	wf.RegisterAgent(agentID, &types.AgentInfo{
		TmuxSession: sessionName,
		Status:      "active",
		Workdir:     workdir,
	})

	return nil
}

// Stop kills an agent's tmux session.
func (m *TmuxAgentManager) Stop(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Kill == nil {
		return fmt.Errorf("kill step missing config")
	}

	agentID := step.Kill.Agent
	graceful := step.Kill.Graceful

	m.mu.RLock()
	state, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		m.logger.Warn("agent not found, assuming already stopped", "agent", agentID)
		return nil
	}

	sessionName := state.tmuxSession
	m.logger.Info("stopping agent", "agent", agentID, "session", sessionName, "graceful", graceful)

	if graceful {
		// Send C-c first to allow Claude to exit cleanly
		if err := m.sendKeys(ctx, sessionName, "C-c"); err != nil {
			m.logger.Warn("failed to send C-c", "error", err)
		}
		// Wait for graceful shutdown
		time.Sleep(2 * time.Second)
	}

	// Kill the session
	if err := m.killSession(ctx, sessionName); err != nil {
		m.logger.Warn("failed to kill session", "error", err)
		// Not fatal - session might already be gone
	}

	// Clean up state
	m.mu.Lock()
	delete(m.agents, agentID)
	m.mu.Unlock()

	return nil
}

// IsRunning checks if an agent is currently running.
func (m *TmuxAgentManager) IsRunning(ctx context.Context, agentID string) (bool, error) {
	m.mu.RLock()
	state, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return false, nil
	}

	return m.sessionExists(state.tmuxSession), nil
}

// InjectPrompt sends a prompt to an agent's tmux session.
// Uses the "nudge" pattern from gastown: literal mode + delay + Enter with retry.
// This is tested and reliable for Claude Code sessions.
func (m *TmuxAgentManager) InjectPrompt(ctx context.Context, agentID string, prompt string) error {
	m.mu.RLock()
	state, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	sessionName := state.tmuxSession
	m.logger.Info("injecting prompt", "agent", agentID, "session", sessionName, "promptLen", len(prompt))

	// Cancel any copy mode first - this prevents "not in a mode" errors
	// when the agent has scrolled back to read output
	m.cancelCopyMode(ctx, sessionName)

	// Wait for Claude to be ready (check for prompt indicator)
	if err := m.waitForClaudeReady(ctx, sessionName, 10*time.Second); err != nil {
		m.logger.Warn("Claude may not be ready", "error", err)
		// Continue anyway - it might still work
	}

	// Cancel copy mode again in case Claude entered it while we were waiting
	m.cancelCopyMode(ctx, sessionName)

	// Send text in literal mode (-l flag handles special chars and newlines)
	if err := m.sendKeysLiteral(ctx, sessionName, prompt); err != nil {
		return fmt.Errorf("sending prompt: %w", err)
	}

	// Wait 500ms for paste to complete (tested, required for Claude Code)
	time.Sleep(500 * time.Millisecond)

	// Send Enter with retry (critical for message submission)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			m.logger.Debug("retrying Enter", "attempt", attempt+1)
			time.Sleep(200 * time.Millisecond)
		}
		if err := m.sendKeys(ctx, sessionName, "Enter"); err != nil {
			lastErr = err
			m.logger.Debug("Enter attempt failed", "attempt", attempt+1, "error", err)
			continue
		}
		return nil
	}
	return fmt.Errorf("failed to send Enter after 3 attempts: %w", lastErr)
}

// SetCurrentStep updates the current step for an agent.
func (m *TmuxAgentManager) SetCurrentStep(agentID, stepID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if state, ok := m.agents[agentID]; ok {
		state.currentStepID = stepID
	}
}

// GetSession returns the tmux session name for an agent.
func (m *TmuxAgentManager) GetSession(agentID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if state, ok := m.agents[agentID]; ok {
		return state.tmuxSession
	}
	return ""
}

// GetWorkdir returns the working directory for an agent.
func (m *TmuxAgentManager) GetWorkdir(agentID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if state, ok := m.agents[agentID]; ok {
		return state.workdir
	}
	return ""
}

// --- tmux helpers ---

func (m *TmuxAgentManager) sessionExists(name string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", name)
	return cmd.Run() == nil
}

func (m *TmuxAgentManager) createSession(ctx context.Context, name, workdir string, env map[string]string, shellCmd string) error {
	// Build environment exports
	var envExports strings.Builder
	for k, v := range env {
		envExports.WriteString(fmt.Sprintf("export %s=%q; ", k, v))
	}

	// Create the tmux session with explicit window size to help terminal detection
	// The command runs exports and then the shell command (typically claude)
	fullCmd := fmt.Sprintf("%s%s", envExports.String(), shellCmd)

	args := []string{
		"new-session",
		"-d",         // detached
		"-s", name,   // session name
		"-c", workdir, // working directory
		"-x", "200",  // width (helps with terminal detection)
		"-y", "50",   // height
		"sh", "-c", fullCmd, // command to run
	}

	m.logger.Debug("creating tmux session", "args", args)

	cmd := exec.CommandContext(ctx, "tmux", args...)
	cmd.Dir = workdir

	// Set environment for the tmux command itself
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux new-session failed: %w (stderr: %s)", err, stderr.String())
	}

	return nil
}

func (m *TmuxAgentManager) killSession(ctx context.Context, name string) error {
	cmd := exec.CommandContext(ctx, "tmux", "kill-session", "-t", name)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux kill-session failed: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

func (m *TmuxAgentManager) sendKeys(ctx context.Context, session string, keys string) error {
	cmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", session, keys)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux send-keys failed: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

// sendKeysLiteral sends text using tmux's literal mode (-l flag).
// This properly handles special characters and multiline text.
func (m *TmuxAgentManager) sendKeysLiteral(ctx context.Context, session string, text string) error {
	cmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", session, "-l", text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux send-keys -l failed: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

// waitForClaudeReady polls until Claude's prompt indicator appears in the pane.
// Claude is ready when we see the input prompt area (indicated by "> " or "❯").
// This helps ensure we don't send input before Claude is ready to receive it.
func (m *TmuxAgentManager) waitForClaudeReady(ctx context.Context, session string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Capture last few lines of the pane
		lines, err := m.capturePane(ctx, session, 15)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		// Look for Claude's prompt indicators
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			// Claude Code shows "> " or "❯" at the start of input line
			if strings.HasPrefix(trimmed, "> ") || strings.HasPrefix(trimmed, "❯") || trimmed == ">" {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for Claude prompt")
}

// capturePane captures the last N lines of a tmux pane.
func (m *TmuxAgentManager) capturePane(ctx context.Context, session string, lines int) ([]string, error) {
	cmd := exec.CommandContext(ctx, "tmux", "capture-pane", "-p", "-t", session, "-S", fmt.Sprintf("-%d", lines))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("tmux capture-pane failed: %w (stderr: %s)", err, stderr.String())
	}
	output := strings.TrimSpace(stdout.String())
	if output == "" {
		return nil, nil
	}
	return strings.Split(output, "\n"), nil
}

// cancelCopyMode cancels any active copy mode in the tmux pane.
// This is needed before sending keys because tmux will reject send-keys -l
// with "not in a mode" error if the pane is in copy mode (scrollback).
func (m *TmuxAgentManager) cancelCopyMode(ctx context.Context, session string) {
	// Send escape to exit any mode, followed by q to exit copy mode
	// These are no-ops if not in copy mode
	exec.CommandContext(ctx, "tmux", "send-keys", "-t", session, "Escape").Run()
	time.Sleep(50 * time.Millisecond)
	exec.CommandContext(ctx, "tmux", "send-keys", "-t", session, "q").Run()
	time.Sleep(50 * time.Millisecond)
}
