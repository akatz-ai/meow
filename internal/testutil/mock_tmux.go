package testutil

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// MockTmux simulates tmux session management for testing.
// It tracks session state and commands without requiring actual tmux.
type MockTmux struct {
	mu sync.RWMutex
	t  *testing.T

	// sessions tracks active session names
	sessions map[string]*MockSession

	// Behavior configuration
	FailStart     bool   // If true, Start() will fail
	FailStop      bool   // If true, Stop() will fail
	FailSendKeys  bool   // If true, SendKeys() will fail
	StartDelay    time.Duration
	StopDelay     time.Duration
	CustomError   error  // Custom error to return

	// Event recording
	Events []MockTmuxEvent
}

// MockSession represents a mock tmux session.
type MockSession struct {
	Name      string
	AgentID   string
	Workdir   string
	Env       map[string]string
	StartedAt time.Time
	Commands  []string  // Commands sent to this session
	SessionID string    // Claude session ID (for resume)
}

// MockTmuxEvent records an event that occurred on the mock.
type MockTmuxEvent struct {
	Type      string
	SessionID string
	AgentID   string
	Command   string
	Timestamp time.Time
	Error     error
}

// NewMockTmux creates a new mock tmux manager for testing.
func NewMockTmux(t *testing.T) *MockTmux {
	t.Helper()
	return &MockTmux{
		t:        t,
		sessions: make(map[string]*MockSession),
		Events:   make([]MockTmuxEvent, 0),
	}
}

// Start simulates starting a Claude agent in a tmux session.
func (m *MockTmux) Start(ctx context.Context, spec *types.StartSpec) error {
	if spec == nil {
		return fmt.Errorf("start spec is nil")
	}
	if spec.Agent == "" {
		return fmt.Errorf("agent ID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.StartDelay > 0 {
		time.Sleep(m.StartDelay)
	}

	if m.FailStart {
		err := m.CustomError
		if err == nil {
			err = fmt.Errorf("mock start failure")
		}
		m.recordEvent("start_failed", "", spec.Agent, "", err)
		return err
	}

	sessionName := "meow-" + spec.Agent

	if _, exists := m.sessions[sessionName]; exists {
		err := fmt.Errorf("session %s already exists", sessionName)
		m.recordEvent("start_failed", sessionName, spec.Agent, "", err)
		return err
	}

	session := &MockSession{
		Name:      sessionName,
		AgentID:   spec.Agent,
		Workdir:   spec.Workdir,
		Env:       spec.Env,
		StartedAt: time.Now(),
		Commands:  make([]string, 0),
		SessionID: spec.ResumeSession,
	}

	m.sessions[sessionName] = session
	m.recordEvent("started", sessionName, spec.Agent, "", nil)

	// Record the initial prompt if set
	prompt := spec.Prompt
	if prompt == "" {
		prompt = "meow prime"
	}
	session.Commands = append(session.Commands, prompt)

	return nil
}

// Stop simulates stopping an agent's tmux session.
func (m *MockTmux) Stop(ctx context.Context, spec *types.StopSpec) error {
	if spec == nil {
		return fmt.Errorf("stop spec is nil")
	}
	if spec.Agent == "" {
		return fmt.Errorf("agent ID is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.StopDelay > 0 {
		time.Sleep(m.StopDelay)
	}

	if m.FailStop {
		err := m.CustomError
		if err == nil {
			err = fmt.Errorf("mock stop failure")
		}
		m.recordEvent("stop_failed", "", spec.Agent, "", err)
		return err
	}

	sessionName := "meow-" + spec.Agent

	if _, exists := m.sessions[sessionName]; !exists {
		// Session doesn't exist - consider it stopped (matches real behavior)
		m.recordEvent("stopped_nonexistent", sessionName, spec.Agent, "", nil)
		return nil
	}

	delete(m.sessions, sessionName)
	m.recordEvent("stopped", sessionName, spec.Agent, "", nil)
	return nil
}

// IsRunning checks if a session is running.
func (m *MockTmux) IsRunning(ctx context.Context, agentID string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionName := "meow-" + agentID
	_, exists := m.sessions[sessionName]
	return exists, nil
}

// SendCommand sends a command to a session.
func (m *MockTmux) SendCommand(ctx context.Context, agentID, command string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.FailSendKeys {
		err := m.CustomError
		if err == nil {
			err = fmt.Errorf("mock send-keys failure")
		}
		m.recordEvent("send_failed", "", agentID, command, err)
		return err
	}

	sessionName := "meow-" + agentID
	session, exists := m.sessions[sessionName]
	if !exists {
		err := fmt.Errorf("session %s not found", sessionName)
		m.recordEvent("send_failed", sessionName, agentID, command, err)
		return err
	}

	session.Commands = append(session.Commands, command)
	m.recordEvent("command_sent", sessionName, agentID, command, nil)
	return nil
}

// GetSession returns a mock session by agent ID.
func (m *MockTmux) GetSession(agentID string) *MockSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionName := "meow-" + agentID
	return m.sessions[sessionName]
}

// GetSessions returns all mock sessions.
func (m *MockTmux) GetSessions() map[string]*MockSession {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*MockSession)
	for k, v := range m.sessions {
		result[k] = v
	}
	return result
}

// SessionCount returns the number of active sessions.
func (m *MockTmux) SessionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sessions)
}

// GetCommands returns all commands sent to a session.
func (m *MockTmux) GetCommands(agentID string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionName := "meow-" + agentID
	session, exists := m.sessions[sessionName]
	if !exists {
		return nil
	}
	result := make([]string, len(session.Commands))
	copy(result, session.Commands)
	return result
}

// SetSessionID sets the session ID for an agent (simulating Claude session capture).
func (m *MockTmux) SetSessionID(agentID, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sessionName := "meow-" + agentID
	session, exists := m.sessions[sessionName]
	if !exists {
		return fmt.Errorf("session %s not found", sessionName)
	}

	session.SessionID = sessionID
	m.recordEvent("session_id_set", sessionName, agentID, sessionID, nil)
	return nil
}

// GetSessionID gets the session ID for an agent.
func (m *MockTmux) GetSessionID(agentID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessionName := "meow-" + agentID
	session, exists := m.sessions[sessionName]
	if !exists {
		return "", fmt.Errorf("session %s not found", sessionName)
	}

	return session.SessionID, nil
}

// Reset clears all sessions and events.
func (m *MockTmux) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sessions = make(map[string]*MockSession)
	m.Events = make([]MockTmuxEvent, 0)
	m.FailStart = false
	m.FailStop = false
	m.FailSendKeys = false
	m.StartDelay = 0
	m.StopDelay = 0
	m.CustomError = nil
}

// recordEvent records an event (must be called with lock held).
func (m *MockTmux) recordEvent(eventType, sessionID, agentID, command string, err error) {
	m.Events = append(m.Events, MockTmuxEvent{
		Type:      eventType,
		SessionID: sessionID,
		AgentID:   agentID,
		Command:   command,
		Timestamp: time.Now(),
		Error:     err,
	})
}

// GetEvents returns a copy of all recorded events.
func (m *MockTmux) GetEvents() []MockTmuxEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]MockTmuxEvent, len(m.Events))
	copy(result, m.Events)
	return result
}

// GetEventsOfType returns events of a specific type.
func (m *MockTmux) GetEventsOfType(eventType string) []MockTmuxEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []MockTmuxEvent
	for _, e := range m.Events {
		if e.Type == eventType {
			result = append(result, e)
		}
	}
	return result
}

// AssertSessionStarted asserts that a session was started.
func (m *MockTmux) AssertSessionStarted(t *testing.T, agentID string) {
	t.Helper()

	events := m.GetEventsOfType("started")
	for _, e := range events {
		if e.AgentID == agentID {
			return
		}
	}
	t.Errorf("Expected session for agent %q to be started, but it wasn't", agentID)
}

// AssertSessionStopped asserts that a session was stopped.
func (m *MockTmux) AssertSessionStopped(t *testing.T, agentID string) {
	t.Helper()

	events := m.GetEventsOfType("stopped")
	for _, e := range events {
		if e.AgentID == agentID {
			return
		}
	}
	t.Errorf("Expected session for agent %q to be stopped, but it wasn't", agentID)
}

// AssertCommandSent asserts that a command was sent to a session.
func (m *MockTmux) AssertCommandSent(t *testing.T, agentID, expectedCommand string) {
	t.Helper()

	commands := m.GetCommands(agentID)
	for _, cmd := range commands {
		if cmd == expectedCommand {
			return
		}
	}
	t.Errorf("Expected command %q to be sent to agent %q, but it wasn't. Commands: %v",
		expectedCommand, agentID, commands)
}

// AssertNoSessions asserts that no sessions are active.
func (m *MockTmux) AssertNoSessions(t *testing.T) {
	t.Helper()

	count := m.SessionCount()
	if count != 0 {
		t.Errorf("Expected no sessions, but found %d", count)
	}
}
