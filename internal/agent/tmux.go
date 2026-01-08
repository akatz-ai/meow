// Package agent provides agent lifecycle management via tmux.
package agent

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meow-stack/meow-machine/internal/errors"
	"github.com/meow-stack/meow-machine/internal/types"
)

// MinTmuxVersion is the minimum required tmux version.
const MinTmuxVersion = 3.0

// CommandRunner is an interface for running external commands.
// This allows mocking in tests.
type CommandRunner interface {
	LookPath(file string) (string, error)
	Output(name string, args ...string) ([]byte, error)
}

// DefaultCommandRunner uses the real os/exec package.
type DefaultCommandRunner struct{}

// LookPath finds the path to an executable.
func (d *DefaultCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// Output runs a command and returns its output.
func (d *DefaultCommandRunner) Output(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).Output()
}

// defaultRunner is the default command runner.
var defaultRunner CommandRunner = &DefaultCommandRunner{}

// ValidateTmux checks that tmux is installed and meets version requirements.
// Returns nil if tmux is valid, or a MeowError with actionable hints if not.
func ValidateTmux() error {
	return ValidateTmuxWithRunner(defaultRunner)
}

// ValidateTmuxWithRunner validates tmux using a custom command runner.
// This is primarily for testing.
func ValidateTmuxWithRunner(runner CommandRunner) error {
	// Check tmux exists in PATH
	_, err := runner.LookPath("tmux")
	if err != nil {
		return errors.AgentTmuxNotFound()
	}

	// Check version
	output, err := runner.Output("tmux", "-V")
	if err != nil {
		return errors.AgentTmuxNotFound().WithCause(err)
	}

	version := parseTmuxVersion(string(output))
	if version < MinTmuxVersion {
		return errors.AgentTmuxTooOld(version, MinTmuxVersion)
	}

	return nil
}

// parseTmuxVersion extracts the version number from tmux -V output.
// Examples: "tmux 3.3a" -> 3.3, "tmux 2.9" -> 2.9, "tmux next-3.4" -> 3.4
func parseTmuxVersion(output string) float64 {
	output = strings.TrimSpace(output)

	// Match version patterns like "3.3a", "2.9", "next-3.4"
	re := regexp.MustCompile(`(\d+)\.(\d+)`)
	match := re.FindStringSubmatch(output)
	if len(match) < 3 {
		return 0
	}

	major, _ := strconv.Atoi(match[1])
	minor, _ := strconv.Atoi(match[2])

	// Calculate divisor based on number of digits in minor version
	// This handles multi-digit minor versions correctly:
	// "3.3" -> 3 + 3/10 = 3.3
	// "3.10" -> 3 + 10/100 = 3.10
	divisor := 1.0
	for i := 0; i < len(match[2]); i++ {
		divisor *= 10
	}

	return float64(major) + float64(minor)/divisor
}

// TmuxManager manages Claude agents via tmux sessions.
type TmuxManager struct {
	mu sync.RWMutex

	// tmux is the low-level tmux wrapper
	tmux *TmuxWrapper

	// agents tracks known agents
	agents map[string]*types.Agent

	// Config
	defaultPrompt string // Default prompt to inject ("meow prime")
	gracePeriod   time.Duration
}

// NewTmuxManager creates a new tmux-based agent manager.
func NewTmuxManager() *TmuxManager {
	return &TmuxManager{
		tmux:          NewTmuxWrapper(),
		agents:        make(map[string]*types.Agent),
		defaultPrompt: "meow prime",
		gracePeriod:   3 * time.Second,
	}
}

// Start spawns an agent in a tmux session.
// This is idempotent - if the session already exists, it's treated as success.
func (m *TmuxManager) Start(ctx context.Context, spec *types.StartSpec) error {
	if spec == nil {
		return fmt.Errorf("start spec is nil")
	}
	if spec.Agent == "" {
		return fmt.Errorf("agent ID is required")
	}

	sessionName := "meow-" + spec.Agent

	// Check if session already exists - if so, treat as success (idempotent)
	if m.tmux.SessionExists(ctx, sessionName) {
		// Session exists - track the agent and return success
		m.mu.Lock()
		if _, exists := m.agents[spec.Agent]; !exists {
			agent := &types.Agent{
				ID:          spec.Agent,
				Name:        spec.Agent,
				Status:      types.AgentStatusActive,
				TmuxSession: sessionName,
				Workdir:     spec.Workdir,
				Env:         spec.Env,
			}
			now := time.Now()
			agent.CreatedAt = &now
			agent.LastHeartbeat = &now
			m.agents[spec.Agent] = agent
		}
		m.mu.Unlock()
		return nil
	}

	// Build the claude command
	claudeArgs := []string{"--dangerously-skip-permissions"}
	if spec.ResumeSession != "" {
		claudeArgs = append(claudeArgs, "--resume", spec.ResumeSession)
	}
	claudeCmd := "claude " + strings.Join(claudeArgs, " ")

	// Create the session using the wrapper
	err := m.tmux.NewSession(ctx, SessionOptions{
		Name:    sessionName,
		Workdir: spec.Workdir,
		Env:     spec.Env,
		Command: claudeCmd,
	})
	if err != nil {
		return err
	}

	// Wait for Claude to fully initialize before sending prompt
	// Claude startup takes ~4-5 seconds to fully initialize its UI
	time.Sleep(5 * time.Second)

	// Send the initial prompt
	prompt := spec.Prompt
	if prompt == "" {
		prompt = m.defaultPrompt
	}
	if err := m.tmux.SendKeys(ctx, sessionName, prompt); err != nil {
		return fmt.Errorf("sending initial prompt: %w", err)
	}

	// Track the agent
	m.mu.Lock()
	agent := &types.Agent{
		ID:          spec.Agent,
		Name:        spec.Agent,
		Status:      types.AgentStatusActive,
		TmuxSession: sessionName,
		Workdir:     spec.Workdir,
		Env:         spec.Env,
	}
	now := time.Now()
	agent.CreatedAt = &now
	agent.LastHeartbeat = &now
	if spec.ResumeSession != "" {
		agent.SessionID = spec.ResumeSession
	}
	m.agents[spec.Agent] = agent
	m.mu.Unlock()

	return nil
}

// Stop kills an agent's tmux session.
func (m *TmuxManager) Stop(ctx context.Context, spec *types.StopSpec) error {
	if spec == nil {
		return fmt.Errorf("stop spec is nil")
	}
	if spec.Agent == "" {
		return fmt.Errorf("agent ID is required")
	}

	sessionName := "meow-" + spec.Agent

	// Check if session exists
	if !m.tmux.SessionExists(ctx, sessionName) {
		// Session doesn't exist - that's fine, consider it stopped
		m.updateAgentState(spec.Agent, types.AgentStatusStopped)
		return nil
	}

	if spec.Graceful {
		// Try graceful shutdown first: send Ctrl-C (without Enter)
		_ = m.tmux.SendKeysLiteral(ctx, sessionName, "C-c")

		// Wait for graceful shutdown
		timeout := spec.Timeout
		if timeout == 0 {
			timeout = int(m.gracePeriod.Seconds())
		}

		deadline := time.Now().Add(time.Duration(timeout) * time.Second)
		for time.Now().Before(deadline) {
			if !m.tmux.SessionExists(ctx, sessionName) {
				m.updateAgentState(spec.Agent, types.AgentStatusStopped)
				return nil
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	// Force kill the session
	if err := m.tmux.KillSession(ctx, sessionName); err != nil {
		return err
	}

	m.updateAgentState(spec.Agent, types.AgentStatusStopped)
	return nil
}

// IsRunning checks if an agent's tmux session is alive.
func (m *TmuxManager) IsRunning(ctx context.Context, agentID string) (bool, error) {
	sessionName := "meow-" + agentID
	return m.tmux.SessionExists(ctx, sessionName), nil
}

// SendCommand sends a command to an agent's session.
func (m *TmuxManager) SendCommand(ctx context.Context, agentID, command string) error {
	sessionName := "meow-" + agentID
	if !m.tmux.SessionExists(ctx, sessionName) {
		return fmt.Errorf("session %s not found", sessionName)
	}
	return m.tmux.SendKeys(ctx, sessionName, command)
}

// CaptureOutput captures the current pane output from an agent's session.
func (m *TmuxManager) CaptureOutput(ctx context.Context, agentID string) (string, error) {
	sessionName := "meow-" + agentID
	return m.tmux.CapturePane(ctx, sessionName)
}

// GetSessionID retrieves the Claude session ID from an agent.
// This is used for checkpoint/resume composition.
func (m *TmuxManager) GetSessionID(ctx context.Context, agentID string) (string, error) {
	m.mu.RLock()
	agent, exists := m.agents[agentID]
	m.mu.RUnlock()

	if !exists {
		return "", fmt.Errorf("agent %s not found", agentID)
	}

	return agent.SessionID, nil
}

// SetSessionID stores the session ID for an agent.
// Called when meow session-id captures the ID.
func (m *TmuxManager) SetSessionID(agentID, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, exists := m.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	agent.SessionID = sessionID
	return nil
}

// ListAgents returns all known agents.
// Returns copies to prevent external mutation of internal state.
func (m *TmuxManager) ListAgents() []*types.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*types.Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		result = append(result, copyAgent(agent))
	}
	return result
}

// UpdateHeartbeat updates the heartbeat for an agent.
func (m *TmuxManager) UpdateHeartbeat(agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, exists := m.agents[agentID]
	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}

	agent.UpdateHeartbeat()
	return nil
}

// SyncWithTmux updates agent states based on actual tmux sessions.
// This is called during crash recovery.
func (m *TmuxManager) SyncWithTmux() error {
	ctx := context.Background()

	// List all meow-* sessions using the wrapper
	sessions, err := m.tmux.ListSessions(ctx, "meow-")
	if err != nil {
		return err
	}

	sessionSet := make(map[string]bool)
	for _, s := range sessions {
		sessionSet[s] = true
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Mark agents as stopped if their session doesn't exist
	for id, agent := range m.agents {
		expectedSession := "meow-" + id
		if !sessionSet[expectedSession] {
			agent.Status = types.AgentStatusStopped
			now := time.Now()
			agent.StoppedAt = &now
		}
	}

	// Create agent entries for unknown sessions
	for session := range sessionSet {
		agentID := strings.TrimPrefix(session, "meow-")
		if _, exists := m.agents[agentID]; !exists {
			now := time.Now()
			m.agents[agentID] = &types.Agent{
				ID:          agentID,
				Name:        agentID,
				Status:      types.AgentStatusActive,
				TmuxSession: session,
				CreatedAt:   &now,
			}
		}
	}

	return nil
}

// updateAgentState updates an agent's status.
func (m *TmuxManager) updateAgentState(agentID string, status types.AgentStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, exists := m.agents[agentID]
	if !exists {
		return
	}

	agent.Status = status
	if status == types.AgentStatusStopped {
		now := time.Now()
		agent.StoppedAt = &now
	}
}
