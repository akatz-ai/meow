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
	timeout := step.Kill.Timeout
	if timeout == 0 {
		timeout = 10 // Default 10 seconds
	}

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
		// Send C-c first
		if err := m.sendKeys(ctx, sessionName, "C-c"); err != nil {
			m.logger.Warn("failed to send C-c", "error", err)
		}
		// Wait briefly
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

// InjectPrompt sends ESC + prompt to agent's tmux session.
func (m *TmuxAgentManager) InjectPrompt(ctx context.Context, agentID string, prompt string) error {
	m.mu.RLock()
	state, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	sessionName := state.tmuxSession
	m.logger.Debug("injecting prompt", "agent", agentID, "session", sessionName)

	// Send Escape first to ensure we're in command mode
	if err := m.sendKeys(ctx, sessionName, "Escape"); err != nil {
		m.logger.Warn("failed to send Escape", "error", err)
	}

	// Small delay to ensure Escape is processed
	time.Sleep(100 * time.Millisecond)

	// Send the prompt text
	if err := m.sendKeys(ctx, sessionName, prompt); err != nil {
		return fmt.Errorf("sending prompt: %w", err)
	}

	// Send Enter to submit
	if err := m.sendKeys(ctx, sessionName, "Enter"); err != nil {
		return fmt.Errorf("sending Enter: %w", err)
	}

	return nil
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
