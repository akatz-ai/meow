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

	"github.com/meow-stack/meow-machine/internal/adapter"
	"github.com/meow-stack/meow-machine/internal/types"
)

// TmuxAgentManager implements AgentManager using tmux sessions.
// It uses the adapter system to remain agent-agnostic - all agent-specific
// behavior (spawn command, prompt injection, graceful stop) comes from adapters.
type TmuxAgentManager struct {
	logger *slog.Logger

	mu         sync.RWMutex
	agents     map[string]*agentState // agentID -> state
	workdir    string                 // Base working directory
	tmuxSocket string                 // Custom tmux socket path (empty = default)
	registry   *adapter.Registry      // Adapter registry for agent configs
}

type agentState struct {
	tmuxSession   string
	workflowID    string
	currentStepID string
	workdir       string
	adapterName   string // Which adapter this agent uses (for stop/inject)
}

// NewTmuxAgentManager creates a new TmuxAgentManager.
// If MEOW_TMUX_SOCKET environment variable is set, uses that socket path.
// The registry parameter provides adapter configs; if nil, a default registry is created.
func NewTmuxAgentManager(workdir string, registry *adapter.Registry, logger *slog.Logger) *TmuxAgentManager {
	if logger == nil {
		logger = slog.Default()
	}
	tmuxSocket := os.Getenv("MEOW_TMUX_SOCKET")

	// Create default registry if not provided
	if registry == nil {
		var err error
		registry, err = adapter.NewDefaultRegistry(workdir)
		if err != nil {
			logger.Warn("failed to create adapter registry, using empty", "error", err)
			registry = adapter.NewRegistry("", "")
		}
	}

	return &TmuxAgentManager{
		logger:     logger.With("component", "agent-manager"),
		agents:     make(map[string]*agentState),
		workdir:    workdir,
		tmuxSocket: tmuxSocket,
		registry:   registry,
	}
}

// SetTmuxSocket sets a custom tmux socket path.
// This is primarily for testing with isolated tmux servers.
func (m *TmuxAgentManager) SetTmuxSocket(socket string) {
	m.tmuxSocket = socket
}

// Start spawns an agent in a tmux session using the configured adapter.
// The adapter determines the spawn command, environment, and startup timing.
func (m *TmuxAgentManager) Start(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Spawn == nil {
		return fmt.Errorf("spawn step missing config")
	}

	cfg := step.Spawn
	agentID := cfg.Agent

	// Resolve which adapter to use (step > workflow > project > global > "claude")
	adapterName := m.registry.Resolve(cfg.Adapter, "", "", "")
	adapterCfg, err := m.registry.Load(adapterName)
	if err != nil {
		return fmt.Errorf("loading adapter %q: %w", adapterName, err)
	}

	m.logger.Debug("using adapter", "adapter", adapterName, "agent", agentID)

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

	m.logger.Info("spawning agent", "agent", agentID, "adapter", adapterName, "session", sessionName, "workdir", workdir)

	// Check if session already exists
	if m.sessionExists(sessionName) {
		m.logger.Warn("tmux session already exists", "session", sessionName)
		// Attach to existing session instead of creating new
	} else {
		// Build environment variables from multiple sources
		// Priority: orchestrator-injected > step config > adapter defaults
		env := make(map[string]string)

		// Start with adapter environment
		for k, v := range adapterCfg.Environment {
			env[k] = v
		}
		// Add step-level environment (overrides adapter)
		for k, v := range cfg.Env {
			env[k] = v
		}
		// Orchestrator-injected vars (reserved, always override)
		env["MEOW_AGENT"] = agentID
		env["MEOW_WORKFLOW"] = wf.ID
		env["MEOW_ORCH_SOCK"] = fmt.Sprintf("/tmp/meow-%s.sock", wf.ID)

		// Create tmux session with bash - we'll start agent via send-keys
		// This ensures the session stays alive and we can inject prompts
		if err := m.createSession(ctx, sessionName, workdir, env, "bash"); err != nil {
			return fmt.Errorf("creating tmux session: %w", err)
		}

		// Build agent command from adapter config
		var agentCmd string
		if cfg.ResumeSession != "" && adapterCfg.Spawn.ResumeCommand != "" {
			// Use resume command with session ID substitution
			agentCmd = strings.ReplaceAll(adapterCfg.Spawn.ResumeCommand, "{{session_id}}", cfg.ResumeSession)
		} else {
			agentCmd = adapterCfg.Spawn.Command
		}
		// Append any extra spawn args from step config
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
		// Uses the adapter's configured startup delay
		startupDelay := adapterCfg.GetStartupDelay()
		m.logger.Info("waiting for agent to start", "agent", agentID, "delay", startupDelay)
		time.Sleep(startupDelay)
	}

	// Register agent state (including which adapter to use for stop/inject)
	m.mu.Lock()
	m.agents[agentID] = &agentState{
		tmuxSession: sessionName,
		workflowID:  wf.ID,
		workdir:     workdir,
		adapterName: adapterName,
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

// Stop kills an agent's tmux session using the configured adapter.
// The adapter determines the graceful stop keys and wait duration.
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
		// Load adapter config for graceful stop settings
		adapterCfg, err := m.registry.Load(state.adapterName)
		if err != nil {
			m.logger.Warn("failed to load adapter for graceful stop, using defaults", "adapter", state.adapterName, "error", err)
			// Fall back to reasonable defaults
			if err := m.sendKeys(ctx, sessionName, "C-c"); err != nil {
				m.logger.Warn("failed to send C-c", "error", err)
			}
			time.Sleep(2 * time.Second)
		} else {
			// Send graceful stop keys from adapter config
			for _, key := range adapterCfg.GracefulStop.Keys {
				if err := m.sendKeys(ctx, sessionName, key); err != nil {
					m.logger.Warn("failed to send graceful stop key", "key", key, "error", err)
				}
			}
			// Wait for graceful shutdown using adapter's configured duration
			time.Sleep(adapterCfg.GetGracefulStopWait())
		}
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

// InjectPrompt sends a prompt to an agent's tmux session using the configured adapter.
// The adapter determines the injection method, pre/post keys, and timing.
func (m *TmuxAgentManager) InjectPrompt(ctx context.Context, agentID string, prompt string) error {
	m.mu.RLock()
	state, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	sessionName := state.tmuxSession
	m.logger.Info("injecting prompt", "agent", agentID, "session", sessionName, "promptLen", len(prompt))

	// Load adapter config for prompt injection settings
	adapterCfg, err := m.registry.Load(state.adapterName)
	if err != nil {
		return fmt.Errorf("loading adapter %q for prompt injection: %w", state.adapterName, err)
	}

	injection := adapterCfg.PromptInjection

	// Send pre-keys (e.g., Escape to exit copy mode)
	for _, key := range injection.PreKeys {
		if err := m.sendKeys(ctx, sessionName, key); err != nil {
			m.logger.Debug("pre-key failed", "key", key, "error", err)
			// Continue anyway - often not critical
		}
	}

	// Wait pre-delay
	if injection.PreDelay > 0 {
		time.Sleep(injection.PreDelay.Duration())
	}

	// Send prompt using the configured method
	method := adapterCfg.GetPromptInjectionMethod()
	if method == "literal" {
		if err := m.sendKeysLiteral(ctx, sessionName, prompt); err != nil {
			return fmt.Errorf("sending prompt (literal): %w", err)
		}
	} else {
		if err := m.sendKeys(ctx, sessionName, prompt); err != nil {
			return fmt.Errorf("sending prompt (keys): %w", err)
		}
	}

	// Wait post-delay before sending post-keys
	if injection.PostDelay > 0 {
		time.Sleep(injection.PostDelay.Duration())
	}

	// Send post-keys (e.g., Enter to submit) with retry
	var lastErr error
	for _, key := range injection.PostKeys {
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				m.logger.Debug("retrying post-key", "key", key, "attempt", attempt+1)
				time.Sleep(200 * time.Millisecond)
			}
			if err := m.sendKeys(ctx, sessionName, key); err != nil {
				lastErr = err
				m.logger.Debug("post-key attempt failed", "key", key, "attempt", attempt+1, "error", err)
				continue
			}
			lastErr = nil
			break
		}
		if lastErr != nil {
			return fmt.Errorf("failed to send post-key %q after 3 attempts: %w", key, lastErr)
		}
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

// Note: Agent-specific prompt detection (like waitForClaudeReady) has been removed.
// Adapters now handle this via startup_delay configuration - the orchestrator
// waits for the configured delay after spawning before sending prompts.
// This keeps the agent manager agent-agnostic.
