package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestAgentStatus_Valid(t *testing.T) {
	tests := []struct {
		status AgentStatus
		want   bool
	}{
		{AgentStatusActive, true},
		{AgentStatusStopped, true},
		{AgentStatus("invalid"), false},
		{AgentStatus(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.Valid(); got != tt.want {
				t.Errorf("AgentStatus.Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAgent_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	agent := &Agent{
		ID:          "claude-1",
		Name:        "Primary Worker",
		Status:      AgentStatusActive,
		SessionID:   "session-abc123",
		TmuxSession: "meow-claude-1",
		Workdir:     "/path/to/project",
		Env:         map[string]string{"MEOW_AGENT": "claude-1"},
		CreatedAt:   &now,
	}

	data, err := json.Marshal(agent)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Agent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != agent.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, agent.ID)
	}
	if decoded.Status != agent.Status {
		t.Errorf("Status mismatch: got %s, want %s", decoded.Status, agent.Status)
	}
	if decoded.SessionID != agent.SessionID {
		t.Errorf("SessionID mismatch: got %s, want %s", decoded.SessionID, agent.SessionID)
	}
	if decoded.Env["MEOW_AGENT"] != "claude-1" {
		t.Errorf("Env[MEOW_AGENT] mismatch: got %s, want claude-1", decoded.Env["MEOW_AGENT"])
	}
}

func TestAgent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		agent   *Agent
		wantErr bool
	}{
		{
			name: "valid agent",
			agent: &Agent{
				ID:     "claude-1",
				Status: AgentStatusActive,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			agent: &Agent{
				Status: AgentStatusActive,
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			agent: &Agent{
				ID:     "claude-1",
				Status: AgentStatus("invalid"),
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.agent.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAgent_TmuxSessionName(t *testing.T) {
	tests := []struct {
		name        string
		agent       *Agent
		want        string
	}{
		{
			name:  "default name",
			agent: &Agent{ID: "claude-1"},
			want:  "meow-claude-1",
		},
		{
			name:  "custom session",
			agent: &Agent{ID: "claude-1", TmuxSession: "custom-session"},
			want:  "custom-session",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.agent.TmuxSessionName(); got != tt.want {
				t.Errorf("TmuxSessionName() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestAgent_StartStop(t *testing.T) {
	agent := &Agent{
		ID:     "claude-1",
		Status: AgentStatusStopped,
	}

	// Start the agent
	if err := agent.Start("session-123"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if agent.Status != AgentStatusActive {
		t.Errorf("Status = %s, want active", agent.Status)
	}
	if agent.SessionID != "session-123" {
		t.Errorf("SessionID = %s, want session-123", agent.SessionID)
	}
	if agent.LastHeartbeat == nil {
		t.Error("LastHeartbeat should be set")
	}

	// Try to start again
	if err := agent.Start("session-456"); err == nil {
		t.Error("Expected error when starting already active agent")
	}

	// Stop the agent
	if err := agent.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if agent.Status != AgentStatusStopped {
		t.Errorf("Status = %s, want stopped", agent.Status)
	}
	if agent.StoppedAt == nil {
		t.Error("StoppedAt should be set")
	}

	// Try to stop again
	if err := agent.Stop(); err == nil {
		t.Error("Expected error when stopping already stopped agent")
	}
}

func TestAgent_IsStale(t *testing.T) {
	agent := &Agent{
		ID:     "claude-1",
		Status: AgentStatusActive,
	}

	// No heartbeat
	if !agent.IsStale(time.Minute) {
		t.Error("Should be stale with no heartbeat")
	}

	// Recent heartbeat
	agent.UpdateHeartbeat()
	if agent.IsStale(time.Minute) {
		t.Error("Should not be stale with recent heartbeat")
	}

	// Old heartbeat
	old := time.Now().Add(-2 * time.Minute)
	agent.LastHeartbeat = &old
	if !agent.IsStale(time.Minute) {
		t.Error("Should be stale with old heartbeat")
	}
}
