// Package agent provides agent lifecycle management via tmux.
package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// Spawner coordinates agent spawning with tmux and persistent state storage.
// It provides a higher-level interface than TmuxManager, integrating with Store
// for durable agent state management.
type Spawner struct {
	tmux  *TmuxWrapper
	store *Store

	// Default configurations
	defaultPrompt     string
	startupDelay      time.Duration
	defaultSetupHooks bool
}

// SpawnerConfig holds configuration for creating a Spawner.
type SpawnerConfig struct {
	// DefaultPrompt is sent to Claude after startup (default: "meow prime")
	DefaultPrompt string
	// StartupDelay is how long to wait after creating the session (default: 500ms)
	StartupDelay time.Duration
	// DefaultSetupHooks controls whether to create .claude/settings.json by default.
	// This enables the Ralph Wiggum loop for autonomous agent operation.
	// Default: true (agents get hooks)
	DefaultSetupHooks bool
}

// NewSpawner creates a new Spawner with the given tmux wrapper and store.
func NewSpawner(tmux *TmuxWrapper, store *Store, cfg *SpawnerConfig) *Spawner {
	s := &Spawner{
		tmux:              tmux,
		store:             store,
		defaultPrompt:     "meow prime",
		startupDelay:      500 * time.Millisecond,
		defaultSetupHooks: true, // Enable Ralph Wiggum loop by default
	}

	if cfg != nil {
		if cfg.DefaultPrompt != "" {
			s.defaultPrompt = cfg.DefaultPrompt
		}
		if cfg.StartupDelay > 0 {
			s.startupDelay = cfg.StartupDelay
		}
		// Note: DefaultSetupHooks is intentionally checked even if false,
		// to allow explicit disabling via config
		s.defaultSetupHooks = cfg.DefaultSetupHooks
	}

	return s
}

// SpawnOptions configures agent spawning behavior.
type SpawnOptions struct {
	// Agent ID (required)
	AgentID string
	// Working directory for the agent
	Workdir string
	// Environment variables to set
	Env map[string]string
	// Initial prompt to send (overrides default)
	Prompt string
	// Resume an existing Claude session
	ResumeSession string
	// Labels for the agent
	Labels map[string]string
	// CurrentBead the agent is working on
	CurrentBead string
	// SetupHooks controls whether to create .claude/settings.json with MEOW hooks.
	// When true, enables the Ralph Wiggum loop for autonomous agent operation.
	SetupHooks bool
}

// Validate checks that the SpawnOptions are valid.
func (o *SpawnOptions) Validate() error {
	if o.AgentID == "" {
		return fmt.Errorf("agent ID is required")
	}
	return nil
}

// Spawn creates and starts a new agent.
// It performs the following steps:
// 1. Validates the spawn options
// 2. Checks if an agent with this ID already exists
// 3. Creates a tmux session
// 4. Starts Claude in the session
// 5. Waits for startup delay
// 6. Sends the initial prompt
// 7. Persists agent state to the store
//
// If any step fails, cleanup is performed to remove partial state.
func (s *Spawner) Spawn(ctx context.Context, opts SpawnOptions) (*types.Agent, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid spawn options: %w", err)
	}

	// Check if agent already exists in store
	existing, err := s.store.Get(ctx, opts.AgentID)
	if err != nil {
		return nil, fmt.Errorf("checking existing agent: %w", err)
	}
	if existing != nil && existing.Status == types.AgentStatusActive {
		return nil, fmt.Errorf("agent %s already exists and is active", opts.AgentID)
	}

	sessionName := "meow-" + opts.AgentID

	// Check if tmux session already exists
	if s.tmux.SessionExists(ctx, sessionName) {
		return nil, fmt.Errorf("tmux session %s already exists", sessionName)
	}

	// Setup hooks in the workdir if enabled
	if opts.SetupHooks && opts.Workdir != "" {
		if err := SetupAgentHooks(opts.Workdir); err != nil {
			// Log but don't fail - hooks are nice-to-have
			// In production, this would use proper logging
			_ = err
		}
	}

	// Build the claude command
	claudeArgs := []string{"--dangerously-skip-permissions"}
	if opts.ResumeSession != "" {
		claudeArgs = append(claudeArgs, "--resume", opts.ResumeSession)
	}
	claudeCmd := "claude " + strings.Join(claudeArgs, " ")

	// Create the tmux session
	err = s.tmux.NewSession(ctx, SessionOptions{
		Name:    sessionName,
		Workdir: opts.Workdir,
		Env:     opts.Env,
		Command: claudeCmd,
	})
	if err != nil {
		return nil, fmt.Errorf("creating tmux session: %w", err)
	}

	// Cleanup function for error cases
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.tmux.KillSession(cleanupCtx, sessionName)
	}

	// Wait for startup
	select {
	case <-ctx.Done():
		cleanup()
		return nil, ctx.Err()
	case <-time.After(s.startupDelay):
	}

	// Send the initial prompt
	prompt := opts.Prompt
	if prompt == "" {
		prompt = s.defaultPrompt
	}
	if err := s.tmux.SendKeys(ctx, sessionName, prompt); err != nil {
		cleanup()
		return nil, fmt.Errorf("sending initial prompt: %w", err)
	}

	// Create the agent record
	now := time.Now()
	agent := &types.Agent{
		ID:            opts.AgentID,
		Name:          opts.AgentID,
		Status:        types.AgentStatusActive,
		TmuxSession:   sessionName,
		Workdir:       opts.Workdir,
		Env:           opts.Env,
		Labels:        opts.Labels,
		CurrentBead:   opts.CurrentBead,
		CreatedAt:     &now,
		LastHeartbeat: &now,
	}
	if opts.ResumeSession != "" {
		agent.SessionID = opts.ResumeSession
	}

	// Persist to store
	if err := s.store.Set(ctx, agent); err != nil {
		cleanup()
		return nil, fmt.Errorf("persisting agent state: %w", err)
	}

	return agent, nil
}

// SpawnFromSpec spawns an agent from a StartSpec.
// This is a convenience method that converts StartSpec to SpawnOptions.
func (s *Spawner) SpawnFromSpec(ctx context.Context, spec *types.StartSpec) (*types.Agent, error) {
	if spec == nil {
		return nil, fmt.Errorf("start spec is nil")
	}

	// Determine SetupHooks: spec overrides default if explicitly set
	setupHooks := s.defaultSetupHooks
	if spec.SetupHooks != nil {
		setupHooks = *spec.SetupHooks
	}

	opts := SpawnOptions{
		AgentID:       spec.Agent,
		Workdir:       spec.Workdir,
		Env:           spec.Env,
		Prompt:        spec.Prompt,
		ResumeSession: spec.ResumeSession,
		SetupHooks:    setupHooks,
	}

	return s.Spawn(ctx, opts)
}

// Despawn stops an agent and updates its state in the store.
// If graceful is true, it tries to send SIGINT first before force-killing.
func (s *Spawner) Despawn(ctx context.Context, agentID string, graceful bool, timeout time.Duration) error {
	if agentID == "" {
		return fmt.Errorf("agent ID is required")
	}

	sessionName := "meow-" + agentID

	// Check if session exists
	if !s.tmux.SessionExists(ctx, sessionName) {
		// Session doesn't exist - just update store
		return s.markStopped(ctx, agentID)
	}

	if graceful {
		// Try graceful shutdown first: send Ctrl-C
		_ = s.tmux.SendKeysLiteral(ctx, sessionName, "C-c")

		// Wait for graceful shutdown
		if timeout == 0 {
			timeout = 3 * time.Second
		}

		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if !s.tmux.SessionExists(ctx, sessionName) {
				return s.markStopped(ctx, agentID)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}

	// Force kill the session
	if err := s.tmux.KillSession(ctx, sessionName); err != nil {
		return fmt.Errorf("killing tmux session: %w", err)
	}

	return s.markStopped(ctx, agentID)
}

// markStopped updates the agent's status to stopped in the store.
// Returns nil if the agent doesn't exist (idempotent).
func (s *Spawner) markStopped(ctx context.Context, agentID string) error {
	err := s.store.Update(ctx, agentID, func(a *types.Agent) error {
		a.Status = types.AgentStatusStopped
		now := time.Now()
		a.StoppedAt = &now
		a.CurrentBead = ""
		return nil
	})
	// Ignore "not found" errors - agent may have been deleted or never existed
	if err != nil && strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

// IsRunning checks if an agent's tmux session is alive.
func (s *Spawner) IsRunning(ctx context.Context, agentID string) bool {
	sessionName := "meow-" + agentID
	return s.tmux.SessionExists(ctx, sessionName)
}

// SyncWithTmux updates agent states based on actual tmux sessions.
// This reconciles the store with the reality of what's running.
func (s *Spawner) SyncWithTmux(ctx context.Context) error {
	// List all meow-* sessions
	sessions, err := s.tmux.ListSessions(ctx, "meow-")
	if err != nil {
		return fmt.Errorf("listing tmux sessions: %w", err)
	}

	sessionSet := make(map[string]bool)
	for _, session := range sessions {
		sessionSet[session] = true
	}

	// Get all agents from store
	agents, err := s.store.List(ctx)
	if err != nil {
		return fmt.Errorf("listing agents from store: %w", err)
	}

	// Mark agents as stopped if their session doesn't exist
	for _, agent := range agents {
		if agent.Status != types.AgentStatusActive {
			continue
		}

		expectedSession := agent.TmuxSessionName()
		if !sessionSet[expectedSession] {
			if err := s.markStopped(ctx, agent.ID); err != nil {
				// Log but continue - don't fail the whole sync
				continue
			}
		}
	}

	// Create agent entries for orphaned sessions
	for session := range sessionSet {
		agentID := strings.TrimPrefix(session, "meow-")

		existing, err := s.store.Get(ctx, agentID)
		if err != nil {
			continue
		}

		if existing == nil {
			// Orphaned session - create agent entry
			now := time.Now()
			agent := &types.Agent{
				ID:            agentID,
				Name:          agentID,
				Status:        types.AgentStatusActive,
				TmuxSession:   session,
				CreatedAt:     &now,
				LastHeartbeat: &now,
			}
			_ = s.store.Set(ctx, agent)
		} else if existing.Status == types.AgentStatusStopped {
			// Session exists but agent marked as stopped - reactivate
			_ = s.store.Update(ctx, agentID, func(a *types.Agent) error {
				a.Status = types.AgentStatusActive
				a.TmuxSession = session
				now := time.Now()
				a.LastHeartbeat = &now
				a.StoppedAt = nil
				return nil
			})
		}
	}

	return nil
}

// MeowHooksJSON is the Claude Code settings.json content for MEOW hooks.
// Uses the new matcher-based format required by Claude Code.
const MeowHooksJSON = `{
  "hooks": {
    "SessionStart": [
      {
        "matcher": {},
        "hooks": [
          {
            "type": "command",
            "command": "meow prime --hook 2>/dev/null || true"
          }
        ]
      }
    ],
    "Stop": [
      {
        "matcher": {},
        "hooks": [
          {
            "type": "command",
            "command": "meow prime --format prompt 2>/dev/null || true"
          }
        ]
      }
    ]
  }
}
`

// SetupAgentHooks creates .claude/settings.json with MEOW hooks in the given directory.
// This enables the Ralph Wiggum loop for autonomous agent operation.
// Returns nil if hooks already exist (does not overwrite).
func SetupAgentHooks(workdir string) error {
	if workdir == "" {
		return fmt.Errorf("workdir is required")
	}

	claudeDir := filepath.Join(workdir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Don't overwrite existing settings (allows user customization)
	if _, err := os.Stat(settingsPath); err == nil {
		return nil
	}

	// Create .claude directory
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return fmt.Errorf("creating .claude directory: %w", err)
	}

	// Write settings with hooks
	if err := os.WriteFile(settingsPath, []byte(MeowHooksJSON), 0644); err != nil {
		return fmt.Errorf("writing settings.json: %w", err)
	}

	return nil
}
