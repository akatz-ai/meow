package types

import (
	"fmt"
	"time"
)

// AgentStatus represents the lifecycle state of an agent.
type AgentStatus string

const (
	AgentStatusActive  AgentStatus = "active"
	AgentStatusStopped AgentStatus = "stopped"
)

// Valid returns true if the status is valid.
func (s AgentStatus) Valid() bool {
	switch s {
	case AgentStatusActive, AgentStatusStopped:
		return true
	}
	return false
}

// Agent represents a Claude Code agent managed by MEOW.
type Agent struct {
	// Identity
	ID   string `json:"id" toml:"id"`     // e.g., "claude-1", "claude-2"
	Name string `json:"name" toml:"name"` // Human-readable name

	// State
	Status AgentStatus `json:"status" toml:"status"`

	// Session tracking (for resume composition)
	SessionID     string     `json:"session_id,omitempty" toml:"session_id,omitempty"`
	TmuxSession   string     `json:"tmux_session,omitempty" toml:"tmux_session,omitempty"` // e.g., "meow-claude-1"
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty" toml:"last_heartbeat,omitempty"`

	// Workspace
	Workdir  string `json:"workdir,omitempty" toml:"workdir,omitempty"`
	Worktree string `json:"worktree,omitempty" toml:"worktree,omitempty"` // Git worktree path if using worktrees

	// Environment variables set for this agent
	Env map[string]string `json:"env,omitempty" toml:"env,omitempty"`

	// Current assignment
	CurrentBead string `json:"current_bead,omitempty" toml:"current_bead,omitempty"`

	// Timestamps
	CreatedAt *time.Time `json:"created_at,omitempty" toml:"created_at,omitempty"`
	StoppedAt *time.Time `json:"stopped_at,omitempty" toml:"stopped_at,omitempty"`

	// Metadata
	Labels map[string]string `json:"labels,omitempty" toml:"labels,omitempty"`
}

// TmuxSessionName returns the tmux session name for this agent.
func (a *Agent) TmuxSessionName() string {
	if a.TmuxSession != "" {
		return a.TmuxSession
	}
	return "meow-" + a.ID
}

// Validate checks that the agent is well-formed.
func (a *Agent) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("agent ID is required")
	}
	if !a.Status.Valid() {
		return fmt.Errorf("invalid agent status: %s", a.Status)
	}
	return nil
}

// Start marks the agent as active.
func (a *Agent) Start(sessionID string) error {
	if a.Status == AgentStatusActive {
		return fmt.Errorf("agent %s is already active", a.ID)
	}
	a.Status = AgentStatusActive
	a.SessionID = sessionID
	now := time.Now()
	a.LastHeartbeat = &now
	a.StoppedAt = nil
	return nil
}

// Stop marks the agent as stopped.
func (a *Agent) Stop() error {
	if a.Status == AgentStatusStopped {
		return fmt.Errorf("agent %s is already stopped", a.ID)
	}
	a.Status = AgentStatusStopped
	now := time.Now()
	a.StoppedAt = &now
	a.CurrentBead = ""
	return nil
}

// UpdateHeartbeat updates the last heartbeat time.
func (a *Agent) UpdateHeartbeat() {
	now := time.Now()
	a.LastHeartbeat = &now
}

// IsStale returns true if the agent hasn't sent a heartbeat in the given duration.
func (a *Agent) IsStale(timeout time.Duration) bool {
	if a.LastHeartbeat == nil {
		return true
	}
	return time.Since(*a.LastHeartbeat) > timeout
}
