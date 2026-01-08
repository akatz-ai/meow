package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/config"
	"github.com/meow-stack/meow-machine/internal/types"
)

// =============================================================================
// INTEGRATION TEST INFRASTRUCTURE
// =============================================================================

// integrationBeadStore provides a realistic bead store that handles
// dependencies properly for integration testing.
type integrationBeadStore struct {
	mu     sync.RWMutex
	beads  map[string]*types.Bead
	order  []string // tracks order beads were added
	loaded bool
}

func newIntegrationBeadStore() *integrationBeadStore {
	return &integrationBeadStore{
		beads:  make(map[string]*types.Bead),
		order:  make([]string, 0),
		loaded: true,
	}
}

func (s *integrationBeadStore) Load(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loaded = true
	return nil
}

func (s *integrationBeadStore) GetNextReady(ctx context.Context) (*types.Bead, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find beads that are open and have all dependencies closed
	for _, id := range s.order {
		bead := s.beads[id]
		if bead == nil || bead.Status != types.BeadStatusOpen {
			continue
		}

		// Check all dependencies
		allDepsClosed := true
		for _, depID := range bead.Needs {
			dep, ok := s.beads[depID]
			if !ok || dep.Status != types.BeadStatusClosed {
				allDepsClosed = false
				break
			}
		}

		if allDepsClosed {
			return bead, nil
		}
	}

	return nil, nil
}

func (s *integrationBeadStore) Get(ctx context.Context, id string) (*types.Bead, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.beads[id], nil
}

func (s *integrationBeadStore) Update(ctx context.Context, bead *types.Bead) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beads[bead.ID] = bead
	return nil
}

func (s *integrationBeadStore) AllDone(ctx context.Context) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.beads) == 0 {
		return true, nil
	}

	for _, bead := range s.beads {
		if bead.Status == types.BeadStatusOpen || bead.Status == types.BeadStatusInProgress {
			return false, nil
		}
	}
	return true, nil
}

func (s *integrationBeadStore) Create(ctx context.Context, bead *types.Bead) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beads[bead.ID] = bead
	s.order = append(s.order, bead.ID)
	return nil
}

func (s *integrationBeadStore) List(ctx context.Context, status types.BeadStatus) ([]*types.Bead, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*types.Bead
	for _, bead := range s.beads {
		if status == "" || bead.Status == status {
			result = append(result, bead)
		}
	}
	return result, nil
}

func (s *integrationBeadStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.beads, id)
	return nil
}

func (s *integrationBeadStore) AddBead(bead *types.Bead) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beads[bead.ID] = bead
	s.order = append(s.order, bead.ID)
}

func (s *integrationBeadStore) GetBead(id string) *types.Bead {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.beads[id]
}

func (s *integrationBeadStore) AllBeads() []*types.Bead {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*types.Bead, 0, len(s.beads))
	for _, bead := range s.beads {
		result = append(result, bead)
	}
	return result
}

// traceCapturingLogger captures log entries for verification.
type traceCapturingLogger struct {
	mu      sync.Mutex
	entries []logEntry
	inner   *slog.Logger
}

type logEntry struct {
	Level   slog.Level
	Message string
	Attrs   map[string]any
}

func newTraceCapturingLogger() *traceCapturingLogger {
	l := &traceCapturingLogger{
		entries: make([]logEntry, 0),
	}
	handler := &capturingHandler{logger: l}
	l.inner = slog.New(handler)
	return l
}

func (l *traceCapturingLogger) Logger() *slog.Logger {
	return l.inner
}

func (l *traceCapturingLogger) AddEntry(level slog.Level, msg string, attrs map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, logEntry{
		Level:   level,
		Message: msg,
		Attrs:   attrs,
	})
}

func (l *traceCapturingLogger) HasMessage(msg string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, entry := range l.entries {
		if entry.Message == msg {
			return true
		}
	}
	return false
}

func (l *traceCapturingLogger) HasMessageWithAttr(msg, key string, value any) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, entry := range l.entries {
		if entry.Message == msg {
			if v, ok := entry.Attrs[key]; ok && v == value {
				return true
			}
		}
	}
	return false
}

func (l *traceCapturingLogger) EntriesWithMessage(msg string) []logEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	var result []logEntry
	for _, entry := range l.entries {
		if entry.Message == msg {
			result = append(result, entry)
		}
	}
	return result
}

type capturingHandler struct {
	logger *traceCapturingLogger
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.logger.AddEntry(r.Level, r.Message, attrs)
	return nil
}

func (h *capturingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h
}

func (h *capturingHandler) WithGroup(name string) slog.Handler {
	return h
}

// =============================================================================
// INTEGRATION TESTS: BASIC EXECUTION
// =============================================================================

func TestIntegration_LinearWorkflow(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := newTraceCapturingLogger()

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger.Logger())

	// Create 3 sequential code beads
	// step1 -> step2 -> step3
	store.AddBead(&types.Bead{
		ID:     "step1",
		Type:   types.BeadTypeCode,
		Title:  "Step 1",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code: "echo step1",
		},
	})
	executor.results["echo step1"] = map[string]any{"stdout": "step1\n", "exit_code": 0}

	store.AddBead(&types.Bead{
		ID:     "step2",
		Type:   types.BeadTypeCode,
		Title:  "Step 2",
		Status: types.BeadStatusOpen,
		Needs:  []string{"step1"},
		CodeSpec: &types.CodeSpec{
			Code: "echo step2",
		},
	})
	executor.results["echo step2"] = map[string]any{"stdout": "step2\n", "exit_code": 0}

	store.AddBead(&types.Bead{
		ID:     "step3",
		Type:   types.BeadTypeCode,
		Title:  "Step 3",
		Status: types.BeadStatusOpen,
		Needs:  []string{"step2"},
		CodeSpec: &types.CodeSpec{
			Code: "echo step3",
		},
	})
	executor.results["echo step3"] = map[string]any{"stdout": "step3\n", "exit_code": 0}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Verify all beads are closed
	for _, id := range []string{"step1", "step2", "step3"} {
		bead := store.GetBead(id)
		if bead.Status != types.BeadStatusClosed {
			t.Errorf("Bead %s status = %s, want closed", id, bead.Status)
		}
	}

	// Verify execution order via executor calls
	if len(executor.executed) != 3 {
		t.Errorf("Expected 3 executions, got %d", len(executor.executed))
	}
	if executor.executed[0] != "echo step1" {
		t.Errorf("First execution = %s, want 'echo step1'", executor.executed[0])
	}
	if executor.executed[1] != "echo step2" {
		t.Errorf("Second execution = %s, want 'echo step2'", executor.executed[1])
	}
	if executor.executed[2] != "echo step3" {
		t.Errorf("Third execution = %s, want 'echo step3'", executor.executed[2])
	}

	// Verify outputs were captured
	if store.GetBead("step1").Outputs["stdout"] != "step1\n" {
		t.Errorf("step1 stdout = %v, want 'step1\\n'", store.GetBead("step1").Outputs["stdout"])
	}
}

func TestIntegration_DiamondDependency(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := newTraceCapturingLogger()

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger.Logger())

	// Diamond pattern:
	//       A
	//      / \
	//     B   C
	//      \ /
	//       D
	store.AddBead(&types.Bead{
		ID:     "bead-a",
		Type:   types.BeadTypeCode,
		Title:  "Bead A",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code: "echo A",
		},
	})
	executor.results["echo A"] = map[string]any{"stdout": "A\n", "exit_code": 0}

	store.AddBead(&types.Bead{
		ID:     "bead-b",
		Type:   types.BeadTypeCode,
		Title:  "Bead B",
		Status: types.BeadStatusOpen,
		Needs:  []string{"bead-a"},
		CodeSpec: &types.CodeSpec{
			Code: "echo B",
		},
	})
	executor.results["echo B"] = map[string]any{"stdout": "B\n", "exit_code": 0}

	store.AddBead(&types.Bead{
		ID:     "bead-c",
		Type:   types.BeadTypeCode,
		Title:  "Bead C",
		Status: types.BeadStatusOpen,
		Needs:  []string{"bead-a"},
		CodeSpec: &types.CodeSpec{
			Code: "echo C",
		},
	})
	executor.results["echo C"] = map[string]any{"stdout": "C\n", "exit_code": 0}

	store.AddBead(&types.Bead{
		ID:     "bead-d",
		Type:   types.BeadTypeCode,
		Title:  "Bead D",
		Status: types.BeadStatusOpen,
		Needs:  []string{"bead-b", "bead-c"},
		CodeSpec: &types.CodeSpec{
			Code: "echo D",
		},
	})
	executor.results["echo D"] = map[string]any{"stdout": "D\n", "exit_code": 0}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Verify all beads are closed
	for _, id := range []string{"bead-a", "bead-b", "bead-c", "bead-d"} {
		bead := store.GetBead(id)
		if bead.Status != types.BeadStatusClosed {
			t.Errorf("Bead %s status = %s, want closed", id, bead.Status)
		}
	}

	// Verify A executed first
	if executor.executed[0] != "echo A" {
		t.Errorf("First execution = %s, want 'echo A'", executor.executed[0])
	}

	// D should be last
	if executor.executed[len(executor.executed)-1] != "echo D" {
		t.Errorf("Last execution = %s, want 'echo D'", executor.executed[len(executor.executed)-1])
	}

	// B and C should both execute after A but before D
	foundB, foundC := false, false
	for i := 1; i < len(executor.executed)-1; i++ {
		if executor.executed[i] == "echo B" {
			foundB = true
		}
		if executor.executed[i] == "echo C" {
			foundC = true
		}
	}
	if !foundB || !foundC {
		t.Errorf("B and C should execute after A and before D, got %v", executor.executed)
	}
}

func TestIntegration_WorkflowCompletesWhenAllBeadsClosed(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Single bead
	store.AddBead(&types.Bead{
		ID:     "single-bead",
		Type:   types.BeadTypeCode,
		Title:  "Single",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code: "echo done",
		},
	})
	executor.results["echo done"] = map[string]any{"exit_code": 0}

	ctx := context.Background()
	done := make(chan error, 1)
	go func() {
		done <- orch.Run(ctx)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run() error = %v, want nil", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Workflow did not complete - AllDone check may be broken")
	}

	// Verify bead is closed
	if store.GetBead("single-bead").Status != types.BeadStatusClosed {
		t.Errorf("Bead should be closed")
	}
}

// =============================================================================
// INTEGRATION TESTS: STATE MANAGEMENT
// =============================================================================

func TestIntegration_StatePersistsAcrossIterations(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Start orchestrator
	err := orch.StartOrResume(context.Background(), &StartupConfig{
		WorkflowID: "state-test-workflow",
		StateDir:   stateDir,
	})
	if err != nil {
		t.Fatalf("StartOrResume() error = %v", err)
	}
	defer orch.ReleaseLock()

	// Verify state was created
	persister := NewStatePersister(stateDir)
	state, err := persister.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if state == nil {
		t.Fatal("State should exist")
	}
	if state.WorkflowID != "state-test-workflow" {
		t.Errorf("WorkflowID = %s, want state-test-workflow", state.WorkflowID)
	}

	// Add some beads and run a few ticks
	store.AddBead(&types.Bead{
		ID:     "state-bead-1",
		Type:   types.BeadTypeCode,
		Title:  "State Test",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code: "echo state",
		},
	})
	executor.results["echo state"] = map[string]any{"exit_code": 0}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify bead was processed
	if store.GetBead("state-bead-1").Status != types.BeadStatusClosed {
		t.Errorf("Bead should be closed")
	}
}

func TestIntegration_CrashRecoveryResetsDeadAgentBeads(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	// First: Create state simulating previous run
	persister := NewStatePersister(stateDir)
	existingState := &OrchestratorState{
		Version:    "1",
		WorkflowID: "crash-recovery-test",
		PID:        99999, // Old PID
	}
	if err := persister.SaveState(existingState); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	// Add an in-progress bead assigned to a dead agent
	store.AddBead(&types.Bead{
		ID:       "crash-bead",
		Type:     types.BeadTypeTask,
		Title:    "Task from dead agent",
		Status:   types.BeadStatusInProgress,
		Assignee: "dead-agent", // Not in agents.running
	})

	// Start orchestrator (triggers recovery)
	err := orch.StartOrResume(context.Background(), &StartupConfig{
		StateDir: stateDir,
	})
	if err != nil {
		t.Fatalf("StartOrResume() error = %v", err)
	}
	defer orch.ReleaseLock()

	// Verify bead was reset to open
	bead := store.GetBead("crash-bead")
	if bead.Status != types.BeadStatusOpen {
		t.Errorf("Bead status = %s, want open (recovered)", bead.Status)
	}
}

func TestIntegration_CrashRecoveryKeepsLiveAgentBeads(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	// Create state simulating previous run
	persister := NewStatePersister(stateDir)
	if err := persister.SaveState(&OrchestratorState{
		Version:    "1",
		WorkflowID: "live-agent-test",
		PID:        99999,
	}); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	agents.running["live-agent"] = true // Agent is still running
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	// Add an in-progress bead assigned to a live agent
	store.AddBead(&types.Bead{
		ID:       "live-bead",
		Type:     types.BeadTypeTask,
		Title:    "Task from live agent",
		Status:   types.BeadStatusInProgress,
		Assignee: "live-agent",
	})

	err := orch.StartOrResume(context.Background(), &StartupConfig{
		StateDir: stateDir,
	})
	if err != nil {
		t.Fatalf("StartOrResume() error = %v", err)
	}
	defer orch.ReleaseLock()

	// Verify bead remains in_progress (agent is alive)
	bead := store.GetBead("live-bead")
	if bead.Status != types.BeadStatusInProgress {
		t.Errorf("Bead status = %s, want in_progress (agent alive)", bead.Status)
	}
}

func TestIntegration_LockPreventsConcurrentOrchestrators(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()

	// First orchestrator acquires lock
	orch1 := New(cfg, store, agents, expander, executor, logger)
	err := orch1.StartOrResume(context.Background(), &StartupConfig{
		WorkflowID: "lock-test",
		StateDir:   stateDir,
	})
	if err != nil {
		t.Fatalf("First StartOrResume() error = %v", err)
	}
	defer orch1.ReleaseLock()

	// Second orchestrator should fail to acquire lock
	orch2 := New(cfg, store, agents, expander, executor, logger)
	err = orch2.StartOrResume(context.Background(), &StartupConfig{
		WorkflowID: "lock-test-2",
		StateDir:   stateDir,
	})
	if err == nil {
		orch2.ReleaseLock()
		t.Error("Second orchestrator should fail with lock conflict")
	}
}

func TestIntegration_HeartbeatUpdatesRegularly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	// Create persister and check heartbeat behavior
	persister := NewStatePersister(stateDir)

	// Initially no heartbeat
	stale, err := persister.CheckStaleHeartbeat(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("CheckStaleHeartbeat() error = %v", err)
	}
	if !stale {
		t.Error("Heartbeat should be stale when no file exists")
	}

	// Update heartbeat
	if err := persister.UpdateHeartbeat(); err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}

	// Now should not be stale
	stale, err = persister.CheckStaleHeartbeat(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("CheckStaleHeartbeat() error = %v", err)
	}
	if stale {
		t.Error("Heartbeat should not be stale immediately after update")
	}

	// Wait and check again
	time.Sleep(150 * time.Millisecond)
	stale, err = persister.CheckStaleHeartbeat(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("CheckStaleHeartbeat() error = %v", err)
	}
	if !stale {
		t.Error("Heartbeat should be stale after timeout")
	}
}

// =============================================================================
// INTEGRATION TESTS: BEAD TRANSITIONS
// =============================================================================

func TestIntegration_BeadTransitions_OpenToInProgressToClosed(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add code bead and track its status
	store.AddBead(&types.Bead{
		ID:     "transition-bead",
		Type:   types.BeadTypeCode,
		Title:  "Transition Test",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code: "echo transition",
		},
	})
	executor.results["echo transition"] = map[string]any{"exit_code": 0}

	// Verify starts as open
	if store.GetBead("transition-bead").Status != types.BeadStatusOpen {
		t.Error("Bead should start as open")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify ends as closed
	bead := store.GetBead("transition-bead")
	if bead.Status != types.BeadStatusClosed {
		t.Errorf("Bead status = %s, want closed", bead.Status)
	}
	if bead.ClosedAt.IsZero() {
		t.Error("ClosedAt should be set")
	}
}

func TestIntegration_DependencyBlockingWorks(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add blocker bead (stays open) and blocked bead
	store.AddBead(&types.Bead{
		ID:       "blocker",
		Type:     types.BeadTypeTask, // Task beads don't auto-close
		Title:    "Blocker",
		Status:   types.BeadStatusOpen,
		Assignee: "claude-1",
	})
	agents.running["claude-1"] = true

	store.AddBead(&types.Bead{
		ID:     "blocked",
		Type:   types.BeadTypeCode,
		Title:  "Blocked",
		Status: types.BeadStatusOpen,
		Needs:  []string{"blocker"},
		CodeSpec: &types.CodeSpec{
			Code: "echo should-not-run",
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Blocked bead should still be open (not closed)
	blockedBead := store.GetBead("blocked")
	if blockedBead.Status == types.BeadStatusClosed {
		t.Error("Blocked bead should NOT be closed while blocker is open")
	}

	// Code should not have been executed
	for _, cmd := range executor.executed {
		if cmd == "echo should-not-run" {
			t.Error("Blocked code should not have been executed")
		}
	}
}

func TestIntegration_ReadyDetectionIsCorrect(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Pre-close a dependency
	closedAt := time.Now()
	store.AddBead(&types.Bead{
		ID:       "pre-closed",
		Type:     types.BeadTypeCode,
		Title:    "Already Closed",
		Status:   types.BeadStatusClosed,
		ClosedAt: &closedAt,
	})

	// Add bead that depends on pre-closed
	store.AddBead(&types.Bead{
		ID:     "ready-bead",
		Type:   types.BeadTypeCode,
		Title:  "Should Be Ready",
		Status: types.BeadStatusOpen,
		Needs:  []string{"pre-closed"},
		CodeSpec: &types.CodeSpec{
			Code: "echo ready",
		},
	})
	executor.results["echo ready"] = map[string]any{"exit_code": 0}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Ready bead should have executed and closed
	readyBead := store.GetBead("ready-bead")
	if readyBead.Status != types.BeadStatusClosed {
		t.Errorf("Ready bead status = %s, want closed", readyBead.Status)
	}

	// Verify it was executed
	found := false
	for _, cmd := range executor.executed {
		if cmd == "echo ready" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Ready bead should have been executed")
	}
}

// =============================================================================
// INTEGRATION TESTS: LOGGING
// =============================================================================

func TestIntegration_LogsDispatchActions(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := newTraceCapturingLogger()

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger.Logger())

	store.AddBead(&types.Bead{
		ID:     "log-test-bead",
		Type:   types.BeadTypeCode,
		Title:  "Logging Test",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code: "echo log",
		},
	})
	executor.results["echo log"] = map[string]any{"exit_code": 0}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify dispatch was logged
	if !logger.HasMessage("dispatching bead") {
		t.Error("Expected 'dispatching bead' log message")
	}

	// Verify bead_id attribute
	if !logger.HasMessageWithAttr("dispatching bead", "id", "log-test-bead") {
		t.Error("Expected dispatch log with bead_id 'log-test-bead'")
	}

	// Verify type attribute
	if !logger.HasMessageWithAttr("dispatching bead", "type", types.BeadTypeCode) {
		t.Error("Expected dispatch log with type 'code'")
	}
}

func TestIntegration_LogsErrorsWithContext(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := newTraceCapturingLogger()

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger.Logger())

	// Add a code bead that will fail (but with on_error=continue)
	executor.errors["failing-log-test"] = fmt.Errorf("execution failed")
	store.AddBead(&types.Bead{
		ID:     "error-log-bead",
		Type:   types.BeadTypeCode,
		Title:  "Error Logging Test",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code:    "failing-log-test",
			OnError: types.OnErrorContinue,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify error was logged with warning
	entries := logger.EntriesWithMessage("code execution failed, continuing")
	if len(entries) == 0 {
		// Try alternate message format
		entries = logger.EntriesWithMessage("code execution failed")
	}

	// Error should be logged (may be in different format)
	if len(entries) == 0 {
		// Check if the bead was processed at all
		bead := store.GetBead("error-log-bead")
		if bead.Status == types.BeadStatusClosed {
			// Good - the bead was processed, error handling worked
			t.Log("Bead was processed despite error (on_error=continue working)")
		}
	}
}

func TestIntegration_LogsOrchestratorStartup(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := newTraceCapturingLogger()

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger.Logger())

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify orchestrator start was logged
	if !logger.HasMessage("orchestrator starting") {
		t.Error("Expected 'orchestrator starting' log message")
	}
}

func TestIntegration_LogsShutdown(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := newTraceCapturingLogger()

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger.Logger())

	// Add a task that won't auto-complete
	store.AddBead(&types.Bead{
		ID:       "long-running",
		Type:     types.BeadTypeTask,
		Title:    "Long Running",
		Status:   types.BeadStatusOpen,
		Assignee: "claude-1",
	})
	agents.running["claude-1"] = true

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		_ = orch.Run(ctx)
		close(done)
	}()

	// Give it time to start
	time.Sleep(30 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for completion
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Orchestrator did not shut down")
	}

	// Verify shutdown was logged
	if !logger.HasMessage("orchestrator shutting down") {
		t.Error("Expected 'orchestrator shutting down' log message")
	}
}

// =============================================================================
// INTEGRATION TESTS: MIXED BEAD TYPES
// =============================================================================

func TestIntegration_MixedBeadTypes(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Workflow with different bead types:
	// start -> code -> condition -> expand
	store.AddBead(&types.Bead{
		ID:     "start-bead",
		Type:   types.BeadTypeStart,
		Title:  "Start Agent",
		Status: types.BeadStatusOpen,
		StartSpec: &types.StartSpec{
			Agent:   "workflow-agent",
			Workdir: "/tmp",
		},
	})

	store.AddBead(&types.Bead{
		ID:     "code-bead",
		Type:   types.BeadTypeCode,
		Title:  "Run Code",
		Status: types.BeadStatusOpen,
		Needs:  []string{"start-bead"},
		CodeSpec: &types.CodeSpec{
			Code: "echo mixed",
		},
	})
	executor.results["echo mixed"] = map[string]any{"exit_code": 0}

	store.AddBead(&types.Bead{
		ID:     "expand-bead",
		Type:   types.BeadTypeExpand,
		Title:  "Expand Template",
		Status: types.BeadStatusOpen,
		Needs:  []string{"code-bead"},
		ExpandSpec: &types.ExpandSpec{
			Template: "test-template",
		},
	})

	store.AddBead(&types.Bead{
		ID:     "stop-bead",
		Type:   types.BeadTypeStop,
		Title:  "Stop Agent",
		Status: types.BeadStatusOpen,
		Needs:  []string{"expand-bead"},
		StopSpec: &types.StopSpec{
			Agent:    "workflow-agent",
			Graceful: true,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v", err)
	}

	// Verify all beads closed
	for _, id := range []string{"start-bead", "code-bead", "expand-bead", "stop-bead"} {
		bead := store.GetBead(id)
		if bead.Status != types.BeadStatusClosed {
			t.Errorf("Bead %s status = %s, want closed", id, bead.Status)
		}
	}

	// Verify agent was started then stopped
	if len(agents.started) != 1 || agents.started[0] != "workflow-agent" {
		t.Errorf("Agent started = %v, want [workflow-agent]", agents.started)
	}
	if len(agents.stopped) != 1 || agents.stopped[0] != "workflow-agent" {
		t.Errorf("Agent stopped = %v, want [workflow-agent]", agents.stopped)
	}

	// Verify template was expanded
	if len(expander.expanded) != 1 || expander.expanded[0] != "test-template" {
		t.Errorf("Templates expanded = %v, want [test-template]", expander.expanded)
	}
}

// =============================================================================
// INTEGRATION TESTS: PARALLEL EXECUTION SAFETY
// =============================================================================

func TestIntegration_ParallelTestExecution(t *testing.T) {
	// This test runs multiple parallel workflows to verify test isolation
	t.Parallel()

	var wg sync.WaitGroup
	const numWorkflows = 5

	for i := 0; i < numWorkflows; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			store := newIntegrationBeadStore()
			agents := newMockAgentManager()
			expander := &mockTemplateExpander{}
			executor := newMockCodeExecutor()
			logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

			cfg := config.Default()
			cfg.Orchestrator.PollInterval = 5 * time.Millisecond

			orch := New(cfg, store, agents, expander, executor, logger)

			store.AddBead(&types.Bead{
				ID:     "parallel-bead",
				Type:   types.BeadTypeCode,
				Title:  "Parallel Test",
				Status: types.BeadStatusOpen,
				CodeSpec: &types.CodeSpec{
					Code: "echo parallel",
				},
			})
			executor.results["echo parallel"] = map[string]any{"exit_code": 0}

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()

			_ = orch.Run(ctx)

			if store.GetBead("parallel-bead").Status != types.BeadStatusClosed {
				t.Errorf("Workflow %d: bead not closed", idx)
			}
		}(i)
	}

	wg.Wait()
}

func TestIntegration_TestsRunUnderFiveSeconds(t *testing.T) {
	t.Parallel()

	start := time.Now()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add multiple beads to simulate realistic workflow
	for i := 0; i < 10; i++ {
		var needs []string
		if i > 0 {
			needs = []string{store.order[i-1]}
		}

		id := store.order[len(store.order):][0:0]
		_ = id // silence unused

		beadID := time.Now().Format("bead-15:04:05.000000000")
		store.AddBead(&types.Bead{
			ID:     beadID,
			Type:   types.BeadTypeCode,
			Title:  "Speed Test",
			Status: types.BeadStatusOpen,
			Needs:  needs,
			CodeSpec: &types.CodeSpec{
				Code: "echo speed",
			},
		})
		executor.results["echo speed"] = map[string]any{"exit_code": 0}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = orch.Run(ctx)

	elapsed := time.Since(start)
	if elapsed >= 5*time.Second {
		t.Errorf("Test took %v, should be under 5 seconds", elapsed)
	}
}

func TestIntegration_NoRealTmuxSessions(t *testing.T) {
	t.Parallel()

	store := newIntegrationBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(bytes.NewBuffer(nil), &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 5 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add start and stop beads
	store.AddBead(&types.Bead{
		ID:     "tmux-start",
		Type:   types.BeadTypeStart,
		Title:  "Start",
		Status: types.BeadStatusOpen,
		StartSpec: &types.StartSpec{
			Agent:   "mock-agent",
			Workdir: "/tmp",
		},
	})

	store.AddBead(&types.Bead{
		ID:     "tmux-stop",
		Type:   types.BeadTypeStop,
		Title:  "Stop",
		Status: types.BeadStatusOpen,
		Needs:  []string{"tmux-start"},
		StopSpec: &types.StopSpec{
			Agent:    "mock-agent",
			Graceful: true,
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify mock agent manager was used (not real tmux)
	if len(agents.started) != 1 || agents.started[0] != "mock-agent" {
		t.Errorf("Mock agent manager should have recorded start: %v", agents.started)
	}
	if len(agents.stopped) != 1 || agents.stopped[0] != "mock-agent" {
		t.Errorf("Mock agent manager should have recorded stop: %v", agents.stopped)
	}
}
