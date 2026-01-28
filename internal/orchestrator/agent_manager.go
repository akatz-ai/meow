package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/akatz-ai/meow/internal/adapter"
	"github.com/akatz-ai/meow/internal/agent"
	"github.com/akatz-ai/meow/internal/ipc"
	"github.com/akatz-ai/meow/internal/types"
)

// TmuxAgentManager implements AgentManager using tmux sessions.
// It uses the adapter system to remain agent-agnostic - all agent-specific
// behavior (spawn command, prompt injection, graceful stop) comes from adapters.
type TmuxAgentManager struct {
	logger *slog.Logger

	mu              sync.RWMutex
	agents          map[string]*agentState // agentID -> state
	workdir         string                 // Base working directory
	tmuxSocket      string                 // Custom tmux socket path (empty = default)
	sendKeysTimeout time.Duration          // Timeout for send-keys operations
	tmux            *agent.TmuxWrapper     // TmuxWrapper for session management
	registry        *adapter.Registry      // Adapter registry for agent configs

	// Logging configuration (abstracted from backend details)
	loggingEnabled bool   // Whether to capture agent output to log files
	logDir         string // Directory for agent log files (e.g., .meow/logs/<run_id>)
}

type agentState struct {
	tmuxSession   string
	workflowID    string
	currentStepID string
	workdir       string
	adapterName   string // Which adapter this agent uses (for stop/inject)
}

// AgentManagerOptions configures the TmuxAgentManager.
type AgentManagerOptions struct {
	// LoggingEnabled enables per-agent output logging. Default: true.
	LoggingEnabled bool
	// LogDir is the directory for agent log files (e.g., .meow/logs/<run_id>).
	// Required if LoggingEnabled is true.
	LogDir string
}

// NewTmuxAgentManager creates a new TmuxAgentManager.
// If MEOW_TMUX_SOCKET environment variable is set, uses that socket path.
// The registry parameter provides adapter configs; if nil, a default registry is created.
func NewTmuxAgentManager(workdir string, registry *adapter.Registry, logger *slog.Logger) *TmuxAgentManager {
	return NewTmuxAgentManagerWithOptions(workdir, registry, logger, AgentManagerOptions{
		LoggingEnabled: true, // Default enabled
	})
}

// NewTmuxAgentManagerWithOptions creates a new TmuxAgentManager with explicit options.
func NewTmuxAgentManagerWithOptions(workdir string, registry *adapter.Registry, logger *slog.Logger, opts AgentManagerOptions) *TmuxAgentManager {
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

	// Get send-keys timeout from default adapter (or use 30s default)
	sendKeysTimeout := 30 * time.Second
	if registry != nil {
		defaultAdapter := os.Getenv("MEOW_DEFAULT_ADAPTER")
		if defaultAdapter == "" {
			defaultAdapter = "claude"
		}
		if cfg, err := registry.Load(defaultAdapter); err == nil {
			sendKeysTimeout = cfg.GetSendKeysTimeout()
		}
	}

	// Create TmuxWrapper with socket path and timeout if configured
	var tmuxOpts []agent.TmuxOption
	if tmuxSocket != "" {
		tmuxOpts = append(tmuxOpts, agent.WithSocketPath(tmuxSocket))
	}
	tmuxOpts = append(tmuxOpts, agent.WithTimeout(sendKeysTimeout))
	tmuxWrapper := agent.NewTmuxWrapper(tmuxOpts...)

	return &TmuxAgentManager{
		logger:          logger.With("component", "agent-manager"),
		agents:          make(map[string]*agentState),
		workdir:         workdir,
		tmuxSocket:      tmuxSocket,
		sendKeysTimeout: sendKeysTimeout,
		tmux:            tmuxWrapper,
		registry:        registry,
		loggingEnabled:  opts.LoggingEnabled,
		logDir:          opts.LogDir,
	}
}

// SetTmuxSocket sets a custom tmux socket path.
// This is primarily for testing with isolated tmux servers.
func (m *TmuxAgentManager) SetTmuxSocket(socket string) {
	m.tmuxSocket = socket
	// Recreate TmuxWrapper with new socket path and preserved timeout
	var opts []agent.TmuxOption
	if socket != "" {
		opts = append(opts, agent.WithSocketPath(socket))
	}
	if m.sendKeysTimeout > 0 {
		opts = append(opts, agent.WithTimeout(m.sendKeysTimeout))
	}
	m.tmux = agent.NewTmuxWrapper(opts...)
}

// Start spawns an agent in a tmux session using the configured adapter.
// The adapter determines the spawn command, environment, and startup timing.
func (m *TmuxAgentManager) Start(ctx context.Context, wf *types.Run, step *types.Step) error {
	if step.Spawn == nil {
		return fmt.Errorf("spawn step missing config")
	}

	cfg := step.Spawn
	agentID := cfg.Agent

	// Resolve which adapter to use (step > workflow)
	adapterName := m.registry.Resolve(cfg.Adapter, wf.DefaultAdapter)
	if adapterName == "" {
		return fmt.Errorf("no adapter specified for agent %q", agentID)
	}
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
	if m.tmux.SessionExists(ctx, sessionName) {
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
		env["MEOW_ORCH_SOCK"] = ipc.SocketPath(wf.ID)

		// Pass through simulator config for E2E testing (if set)
		if simConfig := os.Getenv("MEOW_SIM_CONFIG"); simConfig != "" {
			env["MEOW_SIM_CONFIG"] = simConfig
		}

		// Create tmux session with bash - we'll start agent via send-keys
		// This ensures the session stays alive and we can inject prompts
		if err := m.tmux.NewSession(ctx, agent.SessionOptions{
			Name:    sessionName,
			Workdir: workdir,
			Env:     env,
			Command: "bash",
		}); err != nil {
			return fmt.Errorf("creating tmux session: %w", err)
		}

		// Set up agent output logging (if enabled)
		if m.loggingEnabled && m.logDir != "" {
			logPath := filepath.Join(m.logDir, agentID+".log")
			if err := m.tmux.PipePaneToFile(ctx, sessionName, logPath); err != nil {
				m.logger.Warn("failed to enable agent logging", "agent", agentID, "error", err)
				// Continue anyway - logging is non-critical
			} else {
				m.logger.Info("agent logging enabled", "agent", agentID, "logPath", logPath)
			}
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
		if err := m.tmux.SendKeysLiteral(ctx, sessionName, agentCmd); err != nil {
			return fmt.Errorf("sending agent command: %w", err)
		}
		if err := m.tmux.SendKeysSpecial(ctx, sessionName, "Enter"); err != nil {
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
func (m *TmuxAgentManager) Stop(ctx context.Context, wf *types.Run, step *types.Step) error {
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
			if err := m.tmux.SendKeysSpecial(ctx, sessionName, "C-c"); err != nil {
				m.logger.Warn("failed to send C-c", "error", err)
			}
			time.Sleep(2 * time.Second)
		} else {
			// Send graceful stop keys from adapter config
			for _, key := range adapterCfg.GracefulStop.Keys {
				if err := m.tmux.SendKeysSpecial(ctx, sessionName, key); err != nil {
					m.logger.Warn("failed to send graceful stop key", "key", key, "error", err)
				}
			}
			// Wait for graceful shutdown using adapter's configured duration
			time.Sleep(adapterCfg.GetGracefulStopWait())
		}
	}

	// Kill the session
	if err := m.tmux.KillSession(ctx, sessionName); err != nil {
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

	return m.tmux.SessionExists(ctx, state.tmuxSession), nil
}

// InjectPromptOpts controls prompt injection behavior.
type InjectPromptOpts struct {
	// Stabilize indicates whether to run the stabilization sequence before injection.
	// Set to true for subsequent prompts (after the agent has completed at least one step).
	// Set to false for the first prompt after spawn and for fire_forget mode.
	Stabilize bool
}

// InjectPrompt sends a prompt to an agent's tmux session using the configured adapter.
// The adapter determines the injection method, pre/post keys, and timing.
// If opts.Stabilize is true, runs the stabilization sequence before injection.
func (m *TmuxAgentManager) InjectPrompt(ctx context.Context, agentID string, prompt string, opts InjectPromptOpts) error {
	m.mu.RLock()
	state, ok := m.agents[agentID]
	m.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not found", agentID)
	}

	sessionName := state.tmuxSession
	m.logger.Info("injecting prompt", "agent", agentID, "session", sessionName, "promptLen", len(prompt), "stabilize", opts.Stabilize)

	// Load adapter config for prompt injection settings
	adapterCfg, err := m.registry.Load(state.adapterName)
	if err != nil {
		return fmt.Errorf("loading adapter %q for prompt injection: %w", state.adapterName, err)
	}

	injection := adapterCfg.PromptInjection

	// Stabilization sequence (for subsequent prompts, not first prompt after spawn)
	// This ensures the agent is fully idle before injecting the next prompt
	if opts.Stabilize && len(injection.StabilizeSequence) > 0 {
		m.logger.Debug("running stabilization sequence", "agent", agentID, "steps", len(injection.StabilizeSequence))

		// Pre-stabilize delay
		if injection.PreStabilizeDelay > 0 {
			m.logger.Debug("pre-stabilize delay", "delay", injection.PreStabilizeDelay)
			time.Sleep(injection.PreStabilizeDelay.Duration())
		}

		// Run stabilization sequence
		for i, step := range injection.StabilizeSequence {
			m.logger.Debug("stabilize step", "index", i, "key", step.Key, "delay", step.Delay)
			if err := m.tmux.SendKeysSpecial(ctx, sessionName, step.Key); err != nil {
				m.logger.Debug("stabilize key failed", "key", step.Key, "error", err)
				// Continue anyway - stabilization is best-effort
			}
			if step.Delay > 0 {
				time.Sleep(step.Delay.Duration())
			}
		}
	}

	// Send pre-keys (e.g., Escape to exit copy mode)
	for _, key := range injection.PreKeys {
		if err := m.tmux.SendKeysSpecial(ctx, sessionName, key); err != nil {
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
	start := time.Now()
	var sendErr error
	if method == "literal" {
		sendErr = m.tmux.SendKeysLiteral(ctx, sessionName, prompt)
	} else {
		sendErr = m.tmux.SendKeysSpecial(ctx, sessionName, prompt)
	}
	elapsed := time.Since(start)
	m.logger.Debug("send-keys completed", "agent", agentID, "method", method, "bytes", len(prompt), "elapsed", elapsed)
	if sendErr != nil {
		return fmt.Errorf("sending prompt (%s): %w (took %v)", method, sendErr, elapsed)
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
			if err := m.tmux.SendKeysSpecial(ctx, sessionName, key); err != nil {
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

	return m.tmux.SendKeysSpecial(ctx, sessionName, "C-c")
}

// KillAll kills all agent sessions for a workflow.
// This is used during cleanup to ensure all agents are stopped.
func (m *TmuxAgentManager) KillAll(ctx context.Context, wf *types.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var lastErr error
	for agentID, state := range m.agents {
		if state.workflowID != wf.ID {
			continue
		}

		m.logger.Info("killing agent during cleanup", "agent", agentID, "session", state.tmuxSession)

		// Send C-c first for graceful shutdown
		if err := m.tmux.SendKeysSpecial(ctx, state.tmuxSession, "C-c"); err != nil {
			m.logger.Warn("failed to send C-c", "agent", agentID, "error", err)
		}

		// Give a brief moment for graceful shutdown
		time.Sleep(500 * time.Millisecond)

		// Kill the session
		if err := m.tmux.KillSession(ctx, state.tmuxSession); err != nil {
			m.logger.Warn("failed to kill session", "agent", agentID, "error", err)
			lastErr = err
		}

		delete(m.agents, agentID)
	}

	return lastErr
}

// Note: Agent-specific prompt detection (like waitForClaudeReady) has been removed.
// Adapters now handle this via startup_delay configuration - the orchestrator
// waits for the configured delay after spawning before sending prompts.
// This keeps the agent manager agent-agnostic.
//
// Tmux operations are now handled via the TmuxWrapper from internal/agent,
// eliminating code duplication and providing better isolation with socket support.
