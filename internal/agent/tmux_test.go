package agent

import (
	"context"
	"fmt"
	"os/exec"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/errors"
	"github.com/meow-stack/meow-machine/internal/types"
)

// mockCommandRunner is a mock implementation of CommandRunner for testing.
type mockCommandRunner struct {
	lookPathResult string
	lookPathError  error
	outputResult   []byte
	outputError    error
}

func (m *mockCommandRunner) LookPath(file string) (string, error) {
	return m.lookPathResult, m.lookPathError
}

func (m *mockCommandRunner) Output(name string, args ...string) ([]byte, error) {
	return m.outputResult, m.outputError
}

func TestValidateTmux_NotFound(t *testing.T) {
	runner := &mockCommandRunner{
		lookPathError: fmt.Errorf("executable file not found in $PATH"),
	}

	err := ValidateTmuxWithRunner(runner)
	if err == nil {
		t.Fatal("ValidateTmux should fail when tmux not found")
	}

	if !errors.HasCode(err, errors.CodeAgentTmuxNotFound) {
		t.Errorf("Expected error code %s, got %s", errors.CodeAgentTmuxNotFound, errors.Code(err))
	}

	// Check hint is in details
	meowErr, ok := err.(*errors.MeowError)
	if !ok {
		t.Fatal("Expected *errors.MeowError")
	}
	hint, ok := meowErr.Details["hint"].(string)
	if !ok || hint == "" {
		t.Error("Expected hint in error details")
	}
}

func TestValidateTmux_VersionTooOld(t *testing.T) {
	runner := &mockCommandRunner{
		lookPathResult: "/usr/bin/tmux",
		outputResult:   []byte("tmux 2.9\n"),
	}

	err := ValidateTmuxWithRunner(runner)
	if err == nil {
		t.Fatal("ValidateTmux should fail for old tmux version")
	}

	if !errors.HasCode(err, errors.CodeAgentTmuxTooOld) {
		t.Errorf("Expected error code %s, got %s", errors.CodeAgentTmuxTooOld, errors.Code(err))
	}

	// Check details
	meowErr, ok := err.(*errors.MeowError)
	if !ok {
		t.Fatal("Expected *errors.MeowError")
	}
	if meowErr.Details["current"] != 2.9 {
		t.Errorf("Expected current version 2.9, got %v", meowErr.Details["current"])
	}
	if meowErr.Details["required"] != 3.0 {
		t.Errorf("Expected required version 3.0, got %v", meowErr.Details["required"])
	}
}

func TestValidateTmux_VersionOK(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr bool
	}{
		{"tmux 3.0", "tmux 3.0\n", false},
		{"tmux 3.3a", "tmux 3.3a\n", false},
		{"tmux 3.4", "tmux 3.4\n", false},
		{"tmux next-3.4", "tmux next-3.4\n", false},
		{"tmux 4.0", "tmux 4.0\n", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner := &mockCommandRunner{
				lookPathResult: "/usr/bin/tmux",
				outputResult:   []byte(tt.output),
			}

			err := ValidateTmuxWithRunner(runner)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTmux() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTmux_VersionCommandFails(t *testing.T) {
	runner := &mockCommandRunner{
		lookPathResult: "/usr/bin/tmux",
		outputError:    fmt.Errorf("command failed"),
	}

	err := ValidateTmuxWithRunner(runner)
	if err == nil {
		t.Fatal("ValidateTmux should fail when version command fails")
	}

	// Should return TmuxNotFound with the cause
	if !errors.HasCode(err, errors.CodeAgentTmuxNotFound) {
		t.Errorf("Expected error code %s, got %s", errors.CodeAgentTmuxNotFound, errors.Code(err))
	}
}

func TestParseTmuxVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    float64
	}{
		{"tmux 3.3a", 3.3},
		{"tmux 3.0", 3.0},
		{"tmux 2.9", 2.9},
		{"tmux next-3.4", 3.4},
		{"tmux 4.0", 4.0},
		{"tmux 10.2", 10.2},
		{"invalid", 0},
		{"", 0},
		{"tmux", 0},
		{"3.3a", 3.3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseTmuxVersion(tt.input)
			if got != tt.want {
				t.Errorf("parseTmuxVersion(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateTmux_Integration(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	// Test with real tmux
	err := ValidateTmux()
	if err != nil {
		t.Errorf("ValidateTmux() should pass with real tmux: %v", err)
	}
}

// tmuxAvailable checks if tmux is available for tests.
func tmuxAvailable() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

func TestNewTmuxManager(t *testing.T) {
	m := NewTmuxManager()
	if m == nil {
		t.Fatal("NewTmuxManager() returned nil")
	}
	if m.defaultPrompt != "meow prime" {
		t.Errorf("defaultPrompt = %s, want 'meow prime'", m.defaultPrompt)
	}
}

func TestTmuxManager_Start_NilSpec(t *testing.T) {
	m := NewTmuxManager()
	err := m.Start(context.Background(), nil)
	if err == nil {
		t.Error("Start(nil) should fail")
	}
}

func TestTmuxManager_Start_EmptyAgent(t *testing.T) {
	m := NewTmuxManager()
	err := m.Start(context.Background(), &types.StartSpec{})
	if err == nil {
		t.Error("Start with empty agent should fail")
	}
}

func TestTmuxManager_Stop_NilSpec(t *testing.T) {
	m := NewTmuxManager()
	err := m.Stop(context.Background(), nil)
	if err == nil {
		t.Error("Stop(nil) should fail")
	}
}

func TestTmuxManager_Stop_EmptyAgent(t *testing.T) {
	m := NewTmuxManager()
	err := m.Stop(context.Background(), &types.StopSpec{})
	if err == nil {
		t.Error("Stop with empty agent should fail")
	}
}

func TestTmuxManager_Stop_NonexistentSession(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	m := NewTmuxManager()
	ctx := context.Background()

	// Stopping a non-existent session should succeed
	err := m.Stop(ctx, &types.StopSpec{
		Agent: "nonexistent-agent-" + time.Now().Format("20060102150405"),
	})
	if err != nil {
		t.Errorf("Stop() error = %v, want nil for non-existent session", err)
	}
}

func TestTmuxManager_IsRunning_NonexistentSession(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	m := NewTmuxManager()
	ctx := context.Background()

	running, err := m.IsRunning(ctx, "nonexistent-agent")
	if err != nil {
		t.Errorf("IsRunning() error = %v", err)
	}
	if running {
		t.Error("IsRunning() = true for non-existent session")
	}
}

func TestTmuxManager_SessionIDOperations(t *testing.T) {
	m := NewTmuxManager()

	// Add a fake agent
	m.mu.Lock()
	m.agents["test-agent"] = &types.Agent{
		ID:     "test-agent",
		Status: types.AgentStatusActive,
	}
	m.mu.Unlock()

	// Set session ID
	err := m.SetSessionID("test-agent", "session-123")
	if err != nil {
		t.Errorf("SetSessionID() error = %v", err)
	}

	// Get session ID
	id, err := m.GetSessionID(context.Background(), "test-agent")
	if err != nil {
		t.Errorf("GetSessionID() error = %v", err)
	}
	if id != "session-123" {
		t.Errorf("GetSessionID() = %s, want 'session-123'", id)
	}
}

func TestTmuxManager_SetSessionID_UnknownAgent(t *testing.T) {
	m := NewTmuxManager()

	err := m.SetSessionID("unknown-agent", "session-123")
	if err == nil {
		t.Error("SetSessionID() should fail for unknown agent")
	}
}

func TestTmuxManager_GetSessionID_UnknownAgent(t *testing.T) {
	m := NewTmuxManager()

	_, err := m.GetSessionID(context.Background(), "unknown-agent")
	if err == nil {
		t.Error("GetSessionID() should fail for unknown agent")
	}
}

func TestTmuxManager_ListAgents(t *testing.T) {
	m := NewTmuxManager()

	// Add some agents
	m.mu.Lock()
	m.agents["agent-1"] = &types.Agent{ID: "agent-1", Status: types.AgentStatusActive}
	m.agents["agent-2"] = &types.Agent{ID: "agent-2", Status: types.AgentStatusStopped}
	m.mu.Unlock()

	agents := m.ListAgents()
	if len(agents) != 2 {
		t.Errorf("ListAgents() returned %d agents, want 2", len(agents))
	}
}

func TestTmuxManager_UpdateHeartbeat(t *testing.T) {
	m := NewTmuxManager()

	// Add an agent
	oldTime := time.Now().Add(-1 * time.Hour)
	m.mu.Lock()
	m.agents["test-agent"] = &types.Agent{
		ID:            "test-agent",
		Status:        types.AgentStatusActive,
		LastHeartbeat: &oldTime,
	}
	m.mu.Unlock()

	// Update heartbeat
	err := m.UpdateHeartbeat("test-agent")
	if err != nil {
		t.Errorf("UpdateHeartbeat() error = %v", err)
	}

	// Check heartbeat was updated
	m.mu.RLock()
	agent := m.agents["test-agent"]
	m.mu.RUnlock()

	if agent.LastHeartbeat == nil || time.Since(*agent.LastHeartbeat) > time.Second {
		t.Error("Heartbeat was not updated")
	}
}

func TestTmuxManager_UpdateHeartbeat_UnknownAgent(t *testing.T) {
	m := NewTmuxManager()

	err := m.UpdateHeartbeat("unknown-agent")
	if err == nil {
		t.Error("UpdateHeartbeat() should fail for unknown agent")
	}
}

func TestTmuxManager_updateAgentState(t *testing.T) {
	m := NewTmuxManager()

	// Add an agent
	m.mu.Lock()
	m.agents["test-agent"] = &types.Agent{
		ID:     "test-agent",
		Status: types.AgentStatusActive,
	}
	m.mu.Unlock()

	// Update state
	m.updateAgentState("test-agent", types.AgentStatusStopped)

	// Check state
	m.mu.RLock()
	agent := m.agents["test-agent"]
	m.mu.RUnlock()

	if agent.Status != types.AgentStatusStopped {
		t.Errorf("Status = %s, want stopped", agent.Status)
	}
	if agent.StoppedAt == nil {
		t.Error("StoppedAt should be set")
	}
}

// Integration tests (only run if tmux is available)

func TestTmuxManager_Integration_StartStop(t *testing.T) {
	if !tmuxAvailable() {
		t.Skip("tmux not available")
	}

	// Skip if claude is not available (we don't want to actually run Claude)
	if _, err := exec.LookPath("claude"); err != nil {
		t.Skip("claude not available")
	}

	m := NewTmuxManager()
	ctx := context.Background()
	agentID := "test-" + time.Now().Format("150405")

	// Start
	err := m.Start(ctx, &types.StartSpec{
		Agent:   agentID,
		Workdir: "/tmp",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Wait a moment
	time.Sleep(100 * time.Millisecond)

	// Check running
	running, err := m.IsRunning(ctx, agentID)
	if err != nil {
		t.Errorf("IsRunning() error = %v", err)
	}
	if !running {
		t.Error("Agent should be running after start")
	}

	// Stop
	err = m.Stop(ctx, &types.StopSpec{
		Agent:    agentID,
		Graceful: false, // Force kill for faster test
	})
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Check not running
	running, err = m.IsRunning(ctx, agentID)
	if err != nil {
		t.Errorf("IsRunning() error = %v", err)
	}
	if running {
		t.Error("Agent should not be running after stop")
	}
}
