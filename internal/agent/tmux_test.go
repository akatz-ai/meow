package agent

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

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
