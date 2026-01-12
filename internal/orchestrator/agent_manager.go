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

	mu         sync.RWMutex
	agents     map[string]*agentState // agentID -> state
	workdir    string                 // Base working directory
	tmuxSocket string                 // Custom tmux socket path (empty = default)
}

type agentState struct {
	tmuxSession   string
	workflowID    string
	currentStepID string
	workdir       string
}

// NewTmuxAgentManager creates a new TmuxAgentManager.
// If MEOW_TMUX_SOCKET environment variable is set, uses that socket path.
func NewTmuxAgentManager(workdir string, logger *slog.Logger) *TmuxAgentManager {
	if logger == nil {
		logger = slog.Default()
	}
	tmuxSocket := os.Getenv("MEOW_TMUX_SOCKET")
	return &TmuxAgentManager{
		logger:     logger.With("component", "agent-manager"),
		agents:     make(map[string]*agentState),
		workdir:    workdir,
		tmuxSocket: tmuxSocket,
	}
}

// SetTmuxSocket sets a custom tmux socket path.
// This is primarily for testing with isolated tmux servers.
func (m *TmuxAgentManager) SetTmuxSocket(socket string) {
	m.tmuxSocket = socket
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
		env["MEOW_ORCH_SOCK"] = fmt.Sprintf("/tmp/meow-%s.sock", wf.ID)
		// Prevent Claude from detecting it's in an outer tmux session
		// This avoids potential interactions with the parent tmux server
		env["TMUX"] = ""

		// Create tmux session with bash - we'll start claude via send-keys
		// This ensures the session stays alive and we can inject prompts
		if err := m.createSession(ctx, sessionName, workdir, env, "bash"); err != nil {
			return fmt.Errorf("creating tmux session: %w", err)
		}

		// Build and start agent command via send-keys
		// MEOW_AGENT_COMMAND env var allows overriding for tests (e.g., to use simulator)
		// Default: claude --dangerously-skip-permissions
		baseCmd := os.Getenv("MEOW_AGENT_COMMAND")
		if baseCmd == "" {
			baseCmd = "claude --dangerously-skip-permissions"
		}

		agentCmd := baseCmd
		if cfg.ResumeSession != "" {
			agentCmd = fmt.Sprintf("%s --resume %s", baseCmd, cfg.ResumeSession)
		}
		// Append any extra spawn args
		if cfg.SpawnArgs != "" {
			agentCmd = agentCmd + " " + cfg.SpawnArgs
		}

		// Give the session a moment to initialize
		time.Sleep(100 * time.Millisecond)

		// Start agent in the session
		if err := m.sendKeys(ctx, sessionName, agentCmd); err != nil {
			return fmt.Errorf("sending agent command: %w", err)
		}
		if err := m.sendKeys(ctx, sessionName, "Enter"); err != nil {
			return fmt.Errorf("sending Enter: %w", err)
		}

		// Wait for agent to fully start up before returning
		// This ensures it's ready to receive prompts
		// MEOW_AGENT_STARTUP_DELAY env var allows overriding for tests (simulator starts faster)
		startupDelay := 3 * time.Second
		if delayStr := os.Getenv("MEOW_AGENT_STARTUP_DELAY"); delayStr != "" {
			if d, err := time.ParseDuration(delayStr); err == nil {
				startupDelay = d
			}
		}
		m.logger.Info("waiting for agent to start", "agent", agentID, "delay", startupDelay)
		time.Sleep(startupDelay)
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

// Interrupt sends C-c to an agent's tmux session for graceful cancellation.
// This is used by the timeout enforcement to interrupt running agents.
func (m *TmuxAgentManager) Interrupt(ctx context.Context, agentID string) error {
	m.mu.RLock()
	state, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	sessionName := state.tmuxSession
	m.logger.Info("sending interrupt to agent", "agent", agentID, "session", sessionName)

	return m.sendKeys(ctx, sessionName, "C-c")
}

// KillAll kills all agent sessions for a workflow.
// This is used during cleanup to ensure all agents are stopped.
func (m *TmuxAgentManager) KillAll(ctx context.Context, wf *types.Workflow) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for agentID, state := range m.agents {
		if state.workflowID != wf.ID {
			continue
		}

		m.logger.Info("killing agent during cleanup", "agent", agentID, "session", state.tmuxSession)

		// Send C-c first for graceful shutdown
		if err := m.sendKeys(ctx, state.tmuxSession, "C-c"); err != nil {
			m.logger.Warn("failed to send C-c", "agent", agentID, "error", err)
		}

		// Give a brief moment for graceful shutdown
		time.Sleep(500 * time.Millisecond)

		// Kill the session
		if err := m.killSession(ctx, state.tmuxSession); err != nil {
			m.logger.Warn("failed to kill session", "agent", agentID, "error", err)
			lastErr = err
		}

		delete(m.agents, agentID)
	}

	return lastErr
}

// --- tmux helpers ---

// tmuxArgs builds a tmux command with optional socket argument.
func (m *TmuxAgentManager) tmuxArgs(args ...string) []string {
	if m.tmuxSocket != "" {
		return append([]string{"-S", m.tmuxSocket}, args...)
	}
	return args
}

func (m *TmuxAgentManager) sessionExists(name string) bool {
	args := m.tmuxArgs("has-session", "-t", name)
	cmd := exec.Command("tmux", args...)
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

	baseArgs := []string{
		"new-session",
		"-d",         // detached
		"-s", name,   // session name
		"-c", workdir, // working directory
		"-x", "200",  // width (helps with terminal detection)
		"-y", "50",   // height
		"sh", "-c", fullCmd, // command to run
	}
	args := m.tmuxArgs(baseArgs...)

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
	args := m.tmuxArgs("kill-session", "-t", name)
	cmd := exec.CommandContext(ctx, "tmux", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tmux kill-session failed: %w (stderr: %s)", err, stderr.String())
	}
	return nil
}

func (m *TmuxAgentManager) sendKeys(ctx context.Context, session string, keys string) error {
	args := m.tmuxArgs("send-keys", "-t", session, keys)
	cmd := exec.CommandContext(ctx, "tmux", args...)
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
	args := m.tmuxArgs("send-keys", "-t", session, "-l", text)
	cmd := exec.CommandContext(ctx, "tmux", args...)
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
	args := m.tmuxArgs("capture-pane", "-p", "-t", session, "-S", fmt.Sprintf("-%d", lines))
	cmd := exec.CommandContext(ctx, "tmux", args...)
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
	args := m.tmuxArgs("send-keys", "-t", session, "Escape")
	exec.CommandContext(ctx, "tmux", args...).Run()
	time.Sleep(50 * time.Millisecond)
	args = m.tmuxArgs("send-keys", "-t", session, "q")
	exec.CommandContext(ctx, "tmux", args...).Run()
	time.Sleep(50 * time.Millisecond)
}
