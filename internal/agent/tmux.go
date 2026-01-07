// Package agent provides agent lifecycle management via tmux.
package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// TmuxManager manages Claude agents via tmux sessions.
type TmuxManager struct {
	mu sync.RWMutex

	// agents tracks known agents
	agents map[string]*types.Agent

	// Config
	defaultPrompt string // Default prompt to inject ("meow prime")
	gracePeriod   time.Duration
}

// NewTmuxManager creates a new tmux-based agent manager.
func NewTmuxManager() *TmuxManager {
	return &TmuxManager{
		agents:        make(map[string]*types.Agent),
		defaultPrompt: "meow prime",
		gracePeriod:   3 * time.Second,
	}
}

// Start spawns an agent in a tmux session.
func (m *TmuxManager) Start(ctx context.Context, spec *types.StartSpec) error {
	if spec == nil {
		return fmt.Errorf("start spec is nil")
	}
	if spec.Agent == "" {
		return fmt.Errorf("agent ID is required")
	}

	sessionName := "meow-" + spec.Agent

	// Check if session already exists
	if m.sessionExists(sessionName) {
		return fmt.Errorf("session %s already exists", sessionName)
	}

	// Build the claude command
	claudeArgs := []string{"--dangerously-skip-permissions"}
	if spec.ResumeSession != "" {
		claudeArgs = append(claudeArgs, "--resume", spec.ResumeSession)
	}

	claudeCmd := "claude " + strings.Join(claudeArgs, " ")

	// Build the tmux new-session command
	args := []string{
		"new-session",
		"-d",                       // Detached
		"-s", sessionName,          // Session name
		"-x", "200", "-y", "50",    // Size
	}

	// Set working directory
	workdir := spec.Workdir
	if workdir != "" {
		args = append(args, "-c", workdir)
	}

	// Set environment variables
	for k, v := range spec.Env {
		args = append(args, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	// The command to run in the session
	args = append(args, claudeCmd)

	cmd := exec.CommandContext(ctx, "tmux", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("creating tmux session: %w: %s", err, output)
	}

	// Wait a moment for the session to initialize
	time.Sleep(500 * time.Millisecond)

	// Send the initial prompt
	prompt := spec.Prompt
	if prompt == "" {
		prompt = m.defaultPrompt
	}
	if err := m.sendKeys(ctx, sessionName, prompt); err != nil {
		return fmt.Errorf("sending initial prompt: %w", err)
	}

	// Track the agent
	m.mu.Lock()
	agent := &types.Agent{
		ID:          spec.Agent,
		Name:        spec.Agent,
		Status:      types.AgentStatusActive,
		TmuxSession: sessionName,
		Workdir:     workdir,
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
	if !m.sessionExists(sessionName) {
		// Session doesn't exist - that's fine, consider it stopped
		m.updateAgentState(spec.Agent, types.AgentStatusStopped)
		return nil
	}

	if spec.Graceful {
		// Try graceful shutdown first: send Ctrl-C
		_ = m.sendKeys(ctx, sessionName, "C-c")

		// Wait for graceful shutdown
		timeout := spec.Timeout
		if timeout == 0 {
			timeout = int(m.gracePeriod.Seconds())
		}

		deadline := time.Now().Add(time.Duration(timeout) * time.Second)
		for time.Now().Before(deadline) {
			if !m.sessionExists(sessionName) {
				m.updateAgentState(spec.Agent, types.AgentStatusStopped)
				return nil
			}
			time.Sleep(200 * time.Millisecond)
		}
	}

	// Force kill the session
	cmd := exec.CommandContext(ctx, "tmux", "kill-session", "-t", sessionName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Ignore errors if session doesn't exist
		if !strings.Contains(string(output), "session not found") {
			return fmt.Errorf("killing tmux session: %w: %s", err, output)
		}
	}

	m.updateAgentState(spec.Agent, types.AgentStatusStopped)
	return nil
}

// IsRunning checks if an agent's tmux session is alive.
func (m *TmuxManager) IsRunning(ctx context.Context, agentID string) (bool, error) {
	sessionName := "meow-" + agentID
	return m.sessionExists(sessionName), nil
}

// SendCommand sends a command to an agent's session.
func (m *TmuxManager) SendCommand(ctx context.Context, agentID, command string) error {
	sessionName := "meow-" + agentID
	if !m.sessionExists(sessionName) {
		return fmt.Errorf("session %s not found", sessionName)
	}
	return m.sendKeys(ctx, sessionName, command)
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
func (m *TmuxManager) ListAgents() []*types.Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*types.Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		result = append(result, agent)
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
	// List all meow-* sessions
	sessions, err := m.listMeowSessions()
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
		if strings.HasPrefix(session, "meow-") {
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
	}

	return nil
}

// sessionExists checks if a tmux session exists.
func (m *TmuxManager) sessionExists(sessionName string) bool {
	cmd := exec.Command("tmux", "has-session", "-t", sessionName)
	return cmd.Run() == nil
}

// sendKeys sends keystrokes to a tmux session.
func (m *TmuxManager) sendKeys(ctx context.Context, sessionName, keys string) error {
	cmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", sessionName, keys, "Enter")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("send-keys: %w: %s", err, output)
	}
	return nil
}

// listMeowSessions returns all tmux sessions with the meow- prefix.
func (m *TmuxManager) listMeowSessions() ([]string, error) {
	cmd := exec.Command("tmux", "list-sessions", "-F", "#{session_name}")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		// No sessions is not an error
		if strings.Contains(err.Error(), "no server running") {
			return nil, nil
		}
		return nil, err
	}

	var sessions []string
	for _, line := range strings.Split(stdout.String(), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "meow-") {
			sessions = append(sessions, line)
		}
	}
	return sessions, nil
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
