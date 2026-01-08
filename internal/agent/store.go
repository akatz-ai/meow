package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/meow-stack/meow-machine/internal/types"
)

// Store manages persistent agent state.
type Store struct {
	stateDir string

	mu     sync.RWMutex
	agents map[string]*types.Agent
	loaded bool
}

// NewStore creates a new agent store.
// stateDir is typically .meow in the project root.
func NewStore(stateDir string) *Store {
	return &Store{
		stateDir: stateDir,
		agents:   make(map[string]*types.Agent),
	}
}

// Load reads agent state from disk.
func (s *Store) Load(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadLocked()
}

// loadLocked performs the actual load (caller must hold the lock).
func (s *Store) loadLocked() error {
	agentsPath := filepath.Join(s.stateDir, "agents.json")
	data, err := os.ReadFile(agentsPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.agents = make(map[string]*types.Agent)
			s.loaded = true
			return nil
		}
		return fmt.Errorf("reading agents file: %w", err)
	}

	var agents []*types.Agent
	if err := json.Unmarshal(data, &agents); err != nil {
		return fmt.Errorf("parsing agents file: %w", err)
	}

	s.agents = make(map[string]*types.Agent)
	for _, agent := range agents {
		s.agents[agent.ID] = agent
	}

	s.loaded = true
	return nil
}

// Save writes agent state to disk atomically.
func (s *Store) Save(ctx context.Context) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.saveLocked()
}

// saveLocked performs the actual save (caller must hold the lock).
func (s *Store) saveLocked() error {
	if err := os.MkdirAll(s.stateDir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	agentsPath := filepath.Join(s.stateDir, "agents.json")
	tmpPath := agentsPath + ".tmp"

	// Convert map to slice for JSON
	agents := make([]*types.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		agents = append(agents, agent)
	}

	data, err := json.MarshalIndent(agents, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling agents: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, agentsPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// Get retrieves an agent by ID.
// Returns a copy of the agent to prevent callers from modifying internal state.
func (s *Store) Get(ctx context.Context, id string) (*types.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.loaded {
		return nil, fmt.Errorf("agent store not loaded")
	}

	agent, ok := s.agents[id]
	if !ok {
		return nil, nil
	}
	return copyAgent(agent), nil
}

// Set creates or updates an agent.
// Stores a copy of the agent to prevent callers from modifying internal state.
func (s *Store) Set(ctx context.Context, agent *types.Agent) error {
	if err := agent.Validate(); err != nil {
		return fmt.Errorf("invalid agent: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.loaded {
		return fmt.Errorf("agent store not loaded")
	}

	s.agents[agent.ID] = copyAgent(agent)
	return s.saveLocked()
}

// Update modifies an existing agent.
func (s *Store) Update(ctx context.Context, id string, updateFn func(*types.Agent) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.loaded {
		return fmt.Errorf("agent store not loaded")
	}

	agent, ok := s.agents[id]
	if !ok {
		return fmt.Errorf("agent %s not found", id)
	}

	if err := updateFn(agent); err != nil {
		return err
	}

	return s.saveLocked()
}

// Delete removes an agent by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.loaded {
		return fmt.Errorf("agent store not loaded")
	}

	if _, exists := s.agents[id]; !exists {
		return fmt.Errorf("agent %s not found", id)
	}

	delete(s.agents, id)
	return s.saveLocked()
}

// List returns all agents.
// Returns copies of agents to prevent callers from modifying internal state.
func (s *Store) List(ctx context.Context) ([]*types.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.loaded {
		return nil, fmt.Errorf("agent store not loaded")
	}

	agents := make([]*types.Agent, 0, len(s.agents))
	for _, agent := range s.agents {
		agents = append(agents, copyAgent(agent))
	}
	return agents, nil
}

// ListByStatus returns agents matching the given status.
// Returns copies of agents to prevent callers from modifying internal state.
func (s *Store) ListByStatus(ctx context.Context, status types.AgentStatus) ([]*types.Agent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.loaded {
		return nil, fmt.Errorf("agent store not loaded")
	}

	var agents []*types.Agent
	for _, agent := range s.agents {
		if agent.Status == status {
			agents = append(agents, copyAgent(agent))
		}
	}
	return agents, nil
}

// copyAgent creates a deep copy of an agent to prevent external mutation.
func copyAgent(a *types.Agent) *types.Agent {
	if a == nil {
		return nil
	}
	cp := *a // Shallow copy

	// Deep copy pointer fields
	if a.LastHeartbeat != nil {
		t := *a.LastHeartbeat
		cp.LastHeartbeat = &t
	}
	if a.CreatedAt != nil {
		t := *a.CreatedAt
		cp.CreatedAt = &t
	}
	if a.StoppedAt != nil {
		t := *a.StoppedAt
		cp.StoppedAt = &t
	}

	// Deep copy maps
	if a.Env != nil {
		cp.Env = make(map[string]string, len(a.Env))
		for k, v := range a.Env {
			cp.Env[k] = v
		}
	}
	if a.Labels != nil {
		cp.Labels = make(map[string]string, len(a.Labels))
		for k, v := range a.Labels {
			cp.Labels[k] = v
		}
	}

	return &cp
}
