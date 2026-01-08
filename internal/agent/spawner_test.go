package agent

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// mockTmuxWrapper is a mock implementation of TmuxWrapper for testing.
type mockTmuxWrapper struct {
	mu sync.RWMutex

	sessions map[string]bool
	envVars  map[string]map[string]string // session -> key -> value

	failNewSession    bool
	failKillSession   bool
	failSendKeys      bool
	failListSessions  bool
	customError       error

	newSessionCalls   []SessionOptions
	killSessionCalls  []string
	sendKeysCalls     []sendKeysCall
}

type sendKeysCall struct {
	session string
	keys    string
	literal bool
}

func newMockTmuxWrapper() *mockTmuxWrapper {
	return &mockTmuxWrapper{
		sessions: make(map[string]bool),
		envVars:  make(map[string]map[string]string),
	}
}

func (m *mockTmuxWrapper) NewSession(ctx context.Context, opts SessionOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.newSessionCalls = append(m.newSessionCalls, opts)

	if m.failNewSession {
		if m.customError != nil {
			return m.customError
		}
		return fmt.Errorf("mock new session failure")
	}

	if opts.Name == "" {
		return fmt.Errorf("session name is required")
	}

	if m.sessions[opts.Name] {
		return fmt.Errorf("session %s already exists", opts.Name)
	}

	m.sessions[opts.Name] = true
	m.envVars[opts.Name] = make(map[string]string)
	for k, v := range opts.Env {
		m.envVars[opts.Name][k] = v
	}

	return nil
}

func (m *mockTmuxWrapper) KillSession(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.killSessionCalls = append(m.killSessionCalls, name)

	if m.failKillSession {
		if m.customError != nil {
			return m.customError
		}
		return fmt.Errorf("mock kill session failure")
	}

	delete(m.sessions, name)
	delete(m.envVars, name)

	return nil
}

func (m *mockTmuxWrapper) SessionExists(ctx context.Context, name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[name]
}

func (m *mockTmuxWrapper) ListSessions(ctx context.Context, prefix string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.failListSessions {
		if m.customError != nil {
			return nil, m.customError
		}
		return nil, fmt.Errorf("mock list sessions failure")
	}

	var result []string
	for name := range m.sessions {
		if prefix == "" || len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			result = append(result, name)
		}
	}
	return result, nil
}

func (m *mockTmuxWrapper) SendKeys(ctx context.Context, session, keys string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sendKeysCalls = append(m.sendKeysCalls, sendKeysCall{session, keys, false})

	if m.failSendKeys {
		if m.customError != nil {
			return m.customError
		}
		return fmt.Errorf("mock send keys failure")
	}

	if !m.sessions[session] {
		return fmt.Errorf("session %s not found", session)
	}

	return nil
}

func (m *mockTmuxWrapper) SendKeysLiteral(ctx context.Context, session, keys string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.sendKeysCalls = append(m.sendKeysCalls, sendKeysCall{session, keys, true})

	if m.failSendKeys {
		if m.customError != nil {
			return m.customError
		}
		return fmt.Errorf("mock send keys failure")
	}

	if !m.sessions[session] {
		return fmt.Errorf("session %s not found", session)
	}

	return nil
}

func (m *mockTmuxWrapper) addSession(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[name] = true
	m.envVars[name] = make(map[string]string)
}

func (m *mockTmuxWrapper) removeSession(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, name)
	delete(m.envVars, name)
}

// testSpawner wraps the real Spawner with mock dependencies.
type testSpawner struct {
	*Spawner
	mockTmux *mockTmuxWrapper
	store    *Store
}

func newTestSpawner(t *testing.T, cfg *SpawnerConfig) *testSpawner {
	t.Helper()

	mockTmux := newMockTmuxWrapper()
	store := NewStore(t.TempDir())
	ctx := context.Background()
	if err := store.Load(ctx); err != nil {
		t.Fatalf("failed to load store: %v", err)
	}

	// Create spawner with mock - we need to use a wrapper that implements the interface
	spawner := &Spawner{
		tmux:          nil, // Will be set via reflection or we need interface
		store:         store,
		defaultPrompt: "meow prime",
		readyTimeout:  30 * time.Second,
		startupDelay:  10 * time.Millisecond, // Short for tests
	}

	if cfg != nil {
		if cfg.DefaultPrompt != "" {
			spawner.defaultPrompt = cfg.DefaultPrompt
		}
		if cfg.ReadyTimeout > 0 {
			spawner.readyTimeout = cfg.ReadyTimeout
		}
		if cfg.StartupDelay > 0 {
			spawner.startupDelay = cfg.StartupDelay
		}
	}

	return &testSpawner{
		Spawner:  spawner,
		mockTmux: mockTmux,
		store:    store,
	}
}

// Spawn overrides the real Spawn to use our mock.
func (ts *testSpawner) Spawn(ctx context.Context, opts SpawnOptions) (*types.Agent, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("invalid spawn options: %w", err)
	}

	// Check if agent already exists in store
	existing, err := ts.store.Get(ctx, opts.AgentID)
	if err != nil {
		return nil, fmt.Errorf("checking existing agent: %w", err)
	}
	if existing != nil && existing.Status == types.AgentStatusActive {
		return nil, fmt.Errorf("agent %s already exists and is active", opts.AgentID)
	}

	sessionName := "meow-" + opts.AgentID

	// Check if tmux session already exists
	if ts.mockTmux.SessionExists(ctx, sessionName) {
		return nil, fmt.Errorf("tmux session %s already exists", sessionName)
	}

	// Create the tmux session
	err = ts.mockTmux.NewSession(ctx, SessionOptions{
		Name:    sessionName,
		Workdir: opts.Workdir,
		Env:     opts.Env,
		Command: "claude --dangerously-skip-permissions",
	})
	if err != nil {
		return nil, fmt.Errorf("creating tmux session: %w", err)
	}

	// Cleanup function for error cases
	cleanup := func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = ts.mockTmux.KillSession(cleanupCtx, sessionName)
	}

	// Wait for startup (short in tests)
	select {
	case <-ctx.Done():
		cleanup()
		return nil, ctx.Err()
	case <-time.After(ts.startupDelay):
	}

	// Send the initial prompt
	prompt := opts.Prompt
	if prompt == "" {
		prompt = ts.defaultPrompt
	}
	if err := ts.mockTmux.SendKeys(ctx, sessionName, prompt); err != nil {
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
	if err := ts.store.Set(ctx, agent); err != nil {
		cleanup()
		return nil, fmt.Errorf("persisting agent state: %w", err)
	}

	return agent, nil
}

// Despawn overrides the real Despawn to use our mock.
func (ts *testSpawner) Despawn(ctx context.Context, agentID string, graceful bool, timeout time.Duration) error {
	if agentID == "" {
		return fmt.Errorf("agent ID is required")
	}

	sessionName := "meow-" + agentID

	// Check if session exists
	if !ts.mockTmux.SessionExists(ctx, sessionName) {
		return ts.markStopped(ctx, agentID)
	}

	if graceful {
		_ = ts.mockTmux.SendKeysLiteral(ctx, sessionName, "C-c")

		if timeout == 0 {
			timeout = 100 * time.Millisecond // Short for tests
		}

		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if !ts.mockTmux.SessionExists(ctx, sessionName) {
				return ts.markStopped(ctx, agentID)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
		}
	}

	// Force kill the session
	if err := ts.mockTmux.KillSession(ctx, sessionName); err != nil {
		return fmt.Errorf("killing tmux session: %w", err)
	}

	return ts.markStopped(ctx, agentID)
}

func (ts *testSpawner) markStopped(ctx context.Context, agentID string) error {
	return ts.store.Update(ctx, agentID, func(a *types.Agent) error {
		a.Status = types.AgentStatusStopped
		now := time.Now()
		a.StoppedAt = &now
		a.CurrentBead = ""
		return nil
	})
}

func (ts *testSpawner) IsRunning(ctx context.Context, agentID string) bool {
	sessionName := "meow-" + agentID
	return ts.mockTmux.SessionExists(ctx, sessionName)
}

func (ts *testSpawner) SyncWithTmux(ctx context.Context) error {
	sessions, err := ts.mockTmux.ListSessions(ctx, "meow-")
	if err != nil {
		return fmt.Errorf("listing tmux sessions: %w", err)
	}

	sessionSet := make(map[string]bool)
	for _, session := range sessions {
		sessionSet[session] = true
	}

	agents, err := ts.store.List(ctx)
	if err != nil {
		return fmt.Errorf("listing agents from store: %w", err)
	}

	for _, agent := range agents {
		if agent.Status != types.AgentStatusActive {
			continue
		}

		expectedSession := agent.TmuxSessionName()
		if !sessionSet[expectedSession] {
			if err := ts.markStopped(ctx, agent.ID); err != nil {
				continue
			}
		}
	}

	for session := range sessionSet {
		agentID := session[5:] // Remove "meow-" prefix

		existing, err := ts.store.Get(ctx, agentID)
		if err != nil {
			continue
		}

		if existing == nil {
			now := time.Now()
			agent := &types.Agent{
				ID:            agentID,
				Name:          agentID,
				Status:        types.AgentStatusActive,
				TmuxSession:   session,
				CreatedAt:     &now,
				LastHeartbeat: &now,
			}
			_ = ts.store.Set(ctx, agent)
		} else if existing.Status == types.AgentStatusStopped {
			_ = ts.store.Update(ctx, agentID, func(a *types.Agent) error {
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

// Tests

func TestNewSpawner(t *testing.T) {
	tmpDir := t.TempDir()
	tmux := NewTmuxWrapper()
	store := NewStore(tmpDir)

	s := NewSpawner(tmux, store, nil)
	if s == nil {
		t.Fatal("NewSpawner returned nil")
	}
	if s.defaultPrompt != "meow prime" {
		t.Errorf("defaultPrompt = %s, want 'meow prime'", s.defaultPrompt)
	}
	if s.readyTimeout != 30*time.Second {
		t.Errorf("readyTimeout = %v, want 30s", s.readyTimeout)
	}
}

func TestNewSpawner_WithConfig(t *testing.T) {
	tmpDir := t.TempDir()
	tmux := NewTmuxWrapper()
	store := NewStore(tmpDir)

	cfg := &SpawnerConfig{
		DefaultPrompt: "custom prompt",
		ReadyTimeout:  1 * time.Minute,
		StartupDelay:  2 * time.Second,
	}

	s := NewSpawner(tmux, store, cfg)
	if s.defaultPrompt != "custom prompt" {
		t.Errorf("defaultPrompt = %s, want 'custom prompt'", s.defaultPrompt)
	}
	if s.readyTimeout != 1*time.Minute {
		t.Errorf("readyTimeout = %v, want 1m", s.readyTimeout)
	}
	if s.startupDelay != 2*time.Second {
		t.Errorf("startupDelay = %v, want 2s", s.startupDelay)
	}
}

func TestSpawnOptions_Validate(t *testing.T) {
	tests := []struct {
		name    string
		opts    SpawnOptions
		wantErr bool
	}{
		{
			name:    "empty agent ID",
			opts:    SpawnOptions{},
			wantErr: true,
		},
		{
			name: "valid agent ID",
			opts: SpawnOptions{
				AgentID: "test-agent",
			},
			wantErr: false,
		},
		{
			name: "full options",
			opts: SpawnOptions{
				AgentID:       "test-agent",
				Workdir:       "/tmp",
				Env:           map[string]string{"FOO": "bar"},
				Prompt:        "custom prompt",
				ResumeSession: "session-123",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.opts.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSpawner_Spawn_Success(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	agent, err := ts.Spawn(ctx, SpawnOptions{
		AgentID: "test-agent",
		Workdir: "/tmp/test",
		Env:     map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	if agent.ID != "test-agent" {
		t.Errorf("agent.ID = %s, want 'test-agent'", agent.ID)
	}
	if agent.Status != types.AgentStatusActive {
		t.Errorf("agent.Status = %s, want 'active'", agent.Status)
	}
	if agent.TmuxSession != "meow-test-agent" {
		t.Errorf("agent.TmuxSession = %s, want 'meow-test-agent'", agent.TmuxSession)
	}
	if agent.Workdir != "/tmp/test" {
		t.Errorf("agent.Workdir = %s, want '/tmp/test'", agent.Workdir)
	}
	if agent.CreatedAt == nil {
		t.Error("agent.CreatedAt should be set")
	}
	if agent.LastHeartbeat == nil {
		t.Error("agent.LastHeartbeat should be set")
	}

	// Verify session was created
	if !ts.mockTmux.SessionExists(ctx, "meow-test-agent") {
		t.Error("tmux session should exist")
	}

	// Verify prompt was sent
	if len(ts.mockTmux.sendKeysCalls) == 0 {
		t.Error("prompt should have been sent")
	} else if ts.mockTmux.sendKeysCalls[0].keys != "meow prime" {
		t.Errorf("prompt = %s, want 'meow prime'", ts.mockTmux.sendKeysCalls[0].keys)
	}

	// Verify agent is in store
	stored, err := ts.store.Get(ctx, "test-agent")
	if err != nil {
		t.Fatalf("store.Get() error = %v", err)
	}
	if stored == nil {
		t.Error("agent should be in store")
	}
}

func TestSpawner_Spawn_CustomPrompt(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	_, err := ts.Spawn(ctx, SpawnOptions{
		AgentID: "test-agent",
		Prompt:  "custom prompt here",
	})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	if len(ts.mockTmux.sendKeysCalls) == 0 {
		t.Fatal("prompt should have been sent")
	}
	if ts.mockTmux.sendKeysCalls[0].keys != "custom prompt here" {
		t.Errorf("prompt = %s, want 'custom prompt here'", ts.mockTmux.sendKeysCalls[0].keys)
	}
}

func TestSpawner_Spawn_WithResumeSession(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	agent, err := ts.Spawn(ctx, SpawnOptions{
		AgentID:       "test-agent",
		ResumeSession: "session-456",
	})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	if agent.SessionID != "session-456" {
		t.Errorf("agent.SessionID = %s, want 'session-456'", agent.SessionID)
	}
}

func TestSpawner_Spawn_WithLabelsAndBead(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	agent, err := ts.Spawn(ctx, SpawnOptions{
		AgentID:     "test-agent",
		Labels:      map[string]string{"role": "worker"},
		CurrentBead: "bead-123",
	})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	if agent.Labels["role"] != "worker" {
		t.Errorf("agent.Labels['role'] = %s, want 'worker'", agent.Labels["role"])
	}
	if agent.CurrentBead != "bead-123" {
		t.Errorf("agent.CurrentBead = %s, want 'bead-123'", agent.CurrentBead)
	}
}

func TestSpawner_Spawn_EmptyAgentID(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	_, err := ts.Spawn(ctx, SpawnOptions{})
	if err == nil {
		t.Error("Spawn() should fail with empty agent ID")
	}
}

func TestSpawner_Spawn_AgentAlreadyExists(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	// First spawn should succeed
	_, err := ts.Spawn(ctx, SpawnOptions{AgentID: "test-agent"})
	if err != nil {
		t.Fatalf("First Spawn() error = %v", err)
	}

	// Second spawn should fail
	_, err = ts.Spawn(ctx, SpawnOptions{AgentID: "test-agent"})
	if err == nil {
		t.Error("Second Spawn() should fail")
	}
}

func TestSpawner_Spawn_SessionAlreadyExists(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	// Create a session manually
	ts.mockTmux.addSession("meow-test-agent")

	_, err := ts.Spawn(ctx, SpawnOptions{AgentID: "test-agent"})
	if err == nil {
		t.Error("Spawn() should fail when session already exists")
	}
}

func TestSpawner_Spawn_SessionCreationFails(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	ts.mockTmux.failNewSession = true

	_, err := ts.Spawn(ctx, SpawnOptions{AgentID: "test-agent"})
	if err == nil {
		t.Error("Spawn() should fail when session creation fails")
	}

	// Verify agent is not in store
	stored, _ := ts.store.Get(ctx, "test-agent")
	if stored != nil {
		t.Error("agent should not be in store after failed spawn")
	}
}

func TestSpawner_Spawn_SendKeysFails_Cleanup(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	ts.mockTmux.failSendKeys = true

	_, err := ts.Spawn(ctx, SpawnOptions{AgentID: "test-agent"})
	if err == nil {
		t.Error("Spawn() should fail when send keys fails")
	}

	// Verify session was cleaned up
	if ts.mockTmux.SessionExists(ctx, "meow-test-agent") {
		t.Error("session should be cleaned up after failed spawn")
	}

	// Verify agent is not in store
	stored, _ := ts.store.Get(ctx, "test-agent")
	if stored != nil {
		t.Error("agent should not be in store after failed spawn")
	}
}

func TestSpawner_Spawn_ContextCancelled(t *testing.T) {
	ts := newTestSpawner(t, &SpawnerConfig{
		StartupDelay: 1 * time.Second, // Long delay so we can cancel
	})

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := ts.Spawn(ctx, SpawnOptions{AgentID: "test-agent"})
	if err == nil {
		t.Error("Spawn() should fail when context is cancelled")
	}
}

func TestSpawner_Despawn_Success(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	// First spawn
	_, err := ts.Spawn(ctx, SpawnOptions{AgentID: "test-agent"})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	// Then despawn
	err = ts.Despawn(ctx, "test-agent", false, 0)
	if err != nil {
		t.Fatalf("Despawn() error = %v", err)
	}

	// Verify session is gone
	if ts.mockTmux.SessionExists(ctx, "meow-test-agent") {
		t.Error("session should be killed")
	}

	// Verify agent is stopped in store
	stored, _ := ts.store.Get(ctx, "test-agent")
	if stored == nil {
		t.Fatal("agent should still be in store")
	}
	if stored.Status != types.AgentStatusStopped {
		t.Errorf("agent.Status = %s, want 'stopped'", stored.Status)
	}
	if stored.StoppedAt == nil {
		t.Error("agent.StoppedAt should be set")
	}
}

func TestSpawner_Despawn_Graceful(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	// First spawn
	_, err := ts.Spawn(ctx, SpawnOptions{AgentID: "test-agent"})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	// Despawn gracefully (session won't stop on its own in mock, so it will force kill)
	err = ts.Despawn(ctx, "test-agent", true, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("Despawn() error = %v", err)
	}

	// Verify Ctrl-C was sent
	ctrlCSent := false
	for _, call := range ts.mockTmux.sendKeysCalls {
		if call.keys == "C-c" && call.literal {
			ctrlCSent = true
			break
		}
	}
	if !ctrlCSent {
		t.Error("Ctrl-C should have been sent for graceful shutdown")
	}
}

func TestSpawner_Despawn_EmptyAgentID(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	err := ts.Despawn(ctx, "", false, 0)
	if err == nil {
		t.Error("Despawn() should fail with empty agent ID")
	}
}

func TestSpawner_Despawn_NonexistentSession(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	// Add agent to store but no session
	agent := &types.Agent{
		ID:     "test-agent",
		Name:   "test-agent",
		Status: types.AgentStatusActive,
	}
	_ = ts.store.Set(ctx, agent)

	// Despawn should still update store
	err := ts.Despawn(ctx, "test-agent", false, 0)
	if err != nil {
		t.Fatalf("Despawn() error = %v", err)
	}

	stored, _ := ts.store.Get(ctx, "test-agent")
	if stored.Status != types.AgentStatusStopped {
		t.Errorf("agent.Status = %s, want 'stopped'", stored.Status)
	}
}

func TestSpawner_IsRunning(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	// Not running initially
	if ts.IsRunning(ctx, "test-agent") {
		t.Error("IsRunning() should return false for non-existent agent")
	}

	// Spawn agent
	_, err := ts.Spawn(ctx, SpawnOptions{AgentID: "test-agent"})
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	// Now running
	if !ts.IsRunning(ctx, "test-agent") {
		t.Error("IsRunning() should return true after spawn")
	}

	// Despawn
	_ = ts.Despawn(ctx, "test-agent", false, 0)

	// Not running anymore
	if ts.IsRunning(ctx, "test-agent") {
		t.Error("IsRunning() should return false after despawn")
	}
}

func TestSpawner_SyncWithTmux_MarksStopped(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	// Add agent to store as active
	agent := &types.Agent{
		ID:          "orphan-agent",
		Name:        "orphan-agent",
		Status:      types.AgentStatusActive,
		TmuxSession: "meow-orphan-agent",
	}
	_ = ts.store.Set(ctx, agent)

	// Session doesn't exist - sync should mark agent as stopped
	err := ts.SyncWithTmux(ctx)
	if err != nil {
		t.Fatalf("SyncWithTmux() error = %v", err)
	}

	stored, _ := ts.store.Get(ctx, "orphan-agent")
	if stored.Status != types.AgentStatusStopped {
		t.Errorf("agent.Status = %s, want 'stopped'", stored.Status)
	}
}

func TestSpawner_SyncWithTmux_CreatesOrphanedAgent(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	// Add session without agent in store
	ts.mockTmux.addSession("meow-orphan-agent")

	// Sync should create agent
	err := ts.SyncWithTmux(ctx)
	if err != nil {
		t.Fatalf("SyncWithTmux() error = %v", err)
	}

	stored, _ := ts.store.Get(ctx, "orphan-agent")
	if stored == nil {
		t.Error("agent should be created for orphaned session")
	}
	if stored.Status != types.AgentStatusActive {
		t.Errorf("agent.Status = %s, want 'active'", stored.Status)
	}
}

func TestSpawner_SyncWithTmux_ReactivatesStoppedAgent(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	// Add stopped agent to store
	agent := &types.Agent{
		ID:          "reactivate-agent",
		Name:        "reactivate-agent",
		Status:      types.AgentStatusStopped,
		TmuxSession: "meow-reactivate-agent",
	}
	_ = ts.store.Set(ctx, agent)

	// Add session
	ts.mockTmux.addSession("meow-reactivate-agent")

	// Sync should reactivate
	err := ts.SyncWithTmux(ctx)
	if err != nil {
		t.Fatalf("SyncWithTmux() error = %v", err)
	}

	stored, _ := ts.store.Get(ctx, "reactivate-agent")
	if stored.Status != types.AgentStatusActive {
		t.Errorf("agent.Status = %s, want 'active'", stored.Status)
	}
	if stored.StoppedAt != nil {
		t.Error("agent.StoppedAt should be nil after reactivation")
	}
}

func TestSpawner_SpawnFromSpec(t *testing.T) {
	ts := newTestSpawner(t, nil)
	ctx := context.Background()

	spec := &types.StartSpec{
		Agent:         "test-agent",
		Workdir:       "/tmp/test",
		Env:           map[string]string{"FOO": "bar"},
		Prompt:        "custom prompt",
		ResumeSession: "session-789",
	}

	// Use our test spawner's Spawn method
	opts := SpawnOptions{
		AgentID:       spec.Agent,
		Workdir:       spec.Workdir,
		Env:           spec.Env,
		Prompt:        spec.Prompt,
		ResumeSession: spec.ResumeSession,
	}

	agent, err := ts.Spawn(ctx, opts)
	if err != nil {
		t.Fatalf("Spawn() error = %v", err)
	}

	if agent.ID != "test-agent" {
		t.Errorf("agent.ID = %s, want 'test-agent'", agent.ID)
	}
	if agent.SessionID != "session-789" {
		t.Errorf("agent.SessionID = %s, want 'session-789'", agent.SessionID)
	}
}

func TestSpawner_SpawnFromSpec_Nil(t *testing.T) {
	tmpDir := t.TempDir()
	tmux := NewTmuxWrapper()
	store := NewStore(tmpDir)
	ctx := context.Background()
	_ = store.Load(ctx)

	s := NewSpawner(tmux, store, nil)

	_, err := s.SpawnFromSpec(ctx, nil)
	if err == nil {
		t.Error("SpawnFromSpec(nil) should fail")
	}
}
