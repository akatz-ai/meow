package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/config"
	"github.com/meow-stack/meow-machine/internal/types"
)

// mockBeadStore implements BeadStore for testing.
type mockBeadStore struct {
	mu     sync.Mutex
	beads  map[string]*types.Bead
	ready  []*types.Bead
	calls  []string
}

func newMockBeadStore() *mockBeadStore {
	return &mockBeadStore{
		beads: make(map[string]*types.Bead),
	}
}

func (m *mockBeadStore) GetNextReady(ctx context.Context) (*types.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "GetNextReady")
	if len(m.ready) == 0 {
		return nil, nil
	}
	bead := m.ready[0]
	m.ready = m.ready[1:]
	return bead, nil
}

func (m *mockBeadStore) Get(ctx context.Context, id string) (*types.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Get:"+id)
	return m.beads[id], nil
}

func (m *mockBeadStore) Update(ctx context.Context, bead *types.Bead) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Update:"+bead.ID)
	m.beads[bead.ID] = bead
	return nil
}

func (m *mockBeadStore) AllDone(ctx context.Context) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "AllDone")
	// Done if no ready beads and all beads are closed
	if len(m.ready) > 0 {
		return false, nil
	}
	for _, b := range m.beads {
		if b.Status != types.BeadStatusClosed {
			return false, nil
		}
	}
	return true, nil
}

func (m *mockBeadStore) addReady(bead *types.Bead) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ready = append(m.ready, bead)
	m.beads[bead.ID] = bead
}

// mockAgentManager implements AgentManager for testing.
type mockAgentManager struct {
	mu       sync.Mutex
	running  map[string]bool
	started  []string
	stopped  []string
}

func newMockAgentManager() *mockAgentManager {
	return &mockAgentManager{
		running: make(map[string]bool),
	}
}

func (m *mockAgentManager) Start(ctx context.Context, spec *types.StartSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = append(m.started, spec.Agent)
	m.running[spec.Agent] = true
	return nil
}

func (m *mockAgentManager) Stop(ctx context.Context, spec *types.StopSpec) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = append(m.stopped, spec.Agent)
	m.running[spec.Agent] = false
	return nil
}

func (m *mockAgentManager) IsRunning(ctx context.Context, agentID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[agentID], nil
}

// mockTemplateExpander implements TemplateExpander for testing.
type mockTemplateExpander struct {
	mu       sync.Mutex
	expanded []string
}

func (m *mockTemplateExpander) Expand(ctx context.Context, spec *types.ExpandSpec, parent *types.Bead) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expanded = append(m.expanded, spec.Template)
	return nil
}

// mockCodeExecutor implements CodeExecutor for testing.
type mockCodeExecutor struct {
	mu       sync.Mutex
	executed []string
	results  map[string]map[string]any
	errors   map[string]error
}

func newMockCodeExecutor() *mockCodeExecutor {
	return &mockCodeExecutor{
		results: make(map[string]map[string]any),
		errors:  make(map[string]error),
	}
}

func (m *mockCodeExecutor) Execute(ctx context.Context, spec *types.CodeSpec) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executed = append(m.executed, spec.Code)
	if err, ok := m.errors[spec.Code]; ok {
		return nil, err
	}
	if result, ok := m.results[spec.Code]; ok {
		return result, nil
	}
	return map[string]any{"exit_code": 0}, nil
}

func TestOrchestrator_RunUntilDone(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// No beads - should complete immediately
	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
}

func TestOrchestrator_DispatchStart(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add a start bead
	startBead := &types.Bead{
		ID:     "bd-start-001",
		Type:   types.BeadTypeStart,
		Title:  "Start agent",
		Status: types.BeadStatusOpen,
		StartSpec: &types.StartSpec{
			Agent:   "claude-1",
			Workdir: "/tmp",
		},
	}
	store.addReady(startBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Verify agent was started
	if len(agents.started) != 1 || agents.started[0] != "claude-1" {
		t.Errorf("Expected claude-1 to be started, got %v", agents.started)
	}

	// Verify bead was closed
	if store.beads["bd-start-001"].Status != types.BeadStatusClosed {
		t.Errorf("Bead status = %s, want closed", store.beads["bd-start-001"].Status)
	}
}

func TestOrchestrator_DispatchStop(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add a stop bead
	stopBead := &types.Bead{
		ID:     "bd-stop-001",
		Type:   types.BeadTypeStop,
		Title:  "Stop agent",
		Status: types.BeadStatusOpen,
		StopSpec: &types.StopSpec{
			Agent:    "claude-1",
			Graceful: true,
		},
	}
	store.addReady(stopBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Verify agent was stopped
	if len(agents.stopped) != 1 || agents.stopped[0] != "claude-1" {
		t.Errorf("Expected claude-1 to be stopped, got %v", agents.stopped)
	}
}

func TestOrchestrator_DispatchCode(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Set up executor result
	executor.results["echo hello"] = map[string]any{
		"stdout":    "hello",
		"exit_code": 0,
	}

	// Add a code bead
	codeBead := &types.Bead{
		ID:     "bd-code-001",
		Type:   types.BeadTypeCode,
		Title:  "Run code",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code: "echo hello",
		},
	}
	store.addReady(codeBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Verify code was executed
	if len(executor.executed) != 1 || executor.executed[0] != "echo hello" {
		t.Errorf("Expected 'echo hello' to be executed, got %v", executor.executed)
	}

	// Verify bead has outputs
	bead := store.beads["bd-code-001"]
	if bead.Outputs["stdout"] != "hello" {
		t.Errorf("Output stdout = %v, want 'hello'", bead.Outputs["stdout"])
	}
}

func TestOrchestrator_DispatchExpand(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add an expand bead
	expandBead := &types.Bead{
		ID:     "bd-expand-001",
		Type:   types.BeadTypeExpand,
		Title:  "Expand template",
		Status: types.BeadStatusOpen,
		ExpandSpec: &types.ExpandSpec{
			Template: "implement-tdd",
		},
	}
	store.addReady(expandBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Verify template was expanded
	if len(expander.expanded) != 1 || expander.expanded[0] != "implement-tdd" {
		t.Errorf("Expected 'implement-tdd' to be expanded, got %v", expander.expanded)
	}
}

func TestOrchestrator_DispatchTask(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add a task bead
	taskBead := &types.Bead{
		ID:       "bd-task-001",
		Type:     types.BeadTypeTask,
		Title:    "Do something",
		Status:   types.BeadStatusOpen,
		Assignee: "claude-1",
	}
	store.addReady(taskBead)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run briefly - task won't complete since we don't simulate agent closing it
	_ = orch.Run(ctx)

	// Verify task was marked in_progress
	if store.beads["bd-task-001"].Status != types.BeadStatusInProgress {
		t.Errorf("Task status = %s, want in_progress", store.beads["bd-task-001"].Status)
	}
}

func TestOrchestrator_ConditionTrue(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Set up executor to return true (exit 0)
	executor.results["test -f exists.txt"] = map[string]any{"exit_code": 0}

	// Add a condition bead
	condBead := &types.Bead{
		ID:     "bd-cond-001",
		Type:   types.BeadTypeCondition,
		Title:  "Check file",
		Status: types.BeadStatusOpen,
		ConditionSpec: &types.ConditionSpec{
			Condition: "test -f exists.txt",
			OnTrue: &types.ExpansionTarget{
				Template: "on-true-template",
			},
			OnFalse: &types.ExpansionTarget{
				Template: "on-false-template",
			},
		},
	}
	store.addReady(condBead)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Wait a bit for goroutine
	time.Sleep(50 * time.Millisecond)

	// Verify on_true template was expanded
	if len(expander.expanded) != 1 || expander.expanded[0] != "on-true-template" {
		t.Errorf("Expected on-true-template to be expanded, got %v", expander.expanded)
	}
}

func TestOrchestrator_Shutdown(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add a task bead that won't auto-complete (keeps the loop running)
	taskBead := &types.Bead{
		ID:       "bd-task-001",
		Type:     types.BeadTypeTask,
		Title:    "Ongoing task",
		Status:   types.BeadStatusInProgress, // Already in progress, won't be picked up again
	}
	store.beads[taskBead.ID] = taskBead

	// Start in goroutine
	done := make(chan error, 1)
	go func() {
		done <- orch.Run(context.Background())
	}()

	// Give it time to start
	time.Sleep(20 * time.Millisecond)

	// Shutdown
	orch.Shutdown()

	select {
	case err := <-done:
		if err != context.Canceled {
			t.Errorf("Run() error = %v, want context.Canceled", err)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Shutdown did not complete in time")
	}
}

// mockBeadStoreWithList extends mockBeadStore with List support for recovery tests.
type mockBeadStoreWithList struct {
	*mockBeadStore
}

func newMockBeadStoreWithList() *mockBeadStoreWithList {
	return &mockBeadStoreWithList{
		mockBeadStore: newMockBeadStore(),
	}
}

func (m *mockBeadStoreWithList) Load(ctx context.Context) error {
	return nil
}

func (m *mockBeadStoreWithList) List(ctx context.Context, status types.BeadStatus) ([]*types.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*types.Bead
	for _, bead := range m.beads {
		if status == "" || bead.Status == status {
			result = append(result, bead)
		}
	}
	return result, nil
}

func TestOrchestrator_StartOrResume_FreshStart(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	store := newMockBeadStoreWithList()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	ctx := context.Background()
	startCfg := &StartupConfig{
		WorkflowID: "test-workflow",
		StateDir:   stateDir,
	}

	err := orch.StartOrResume(ctx, startCfg)
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
		t.Fatal("State should exist after fresh start")
	}
	if state.WorkflowID != "test-workflow" {
		t.Errorf("WorkflowID = %s, want test-workflow", state.WorkflowID)
	}
	if state.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", state.PID, os.Getpid())
	}
}

func TestOrchestrator_StartOrResume_FreshStartWithTemplate(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	store := newMockBeadStoreWithList()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	ctx := context.Background()
	startCfg := &StartupConfig{
		WorkflowID: "test-workflow",
		Template:   "outer-loop",
		StateDir:   stateDir,
	}

	err := orch.StartOrResume(ctx, startCfg)
	if err != nil {
		t.Fatalf("StartOrResume() error = %v", err)
	}
	defer orch.ReleaseLock()

	// Verify template was expanded
	if len(expander.expanded) != 1 || expander.expanded[0] != "outer-loop" {
		t.Errorf("Expected outer-loop to be expanded, got %v", expander.expanded)
	}
}

func TestOrchestrator_StartOrResume_Resume(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	// Create existing state (simulating a previous run)
	persister := NewStatePersister(stateDir)
	existingState := &OrchestratorState{
		Version:      "1",
		WorkflowID:   "existing-workflow",
		TemplateName: "outer-loop",
		TickCount:    100,
		PID:          99999, // Old PID
	}
	if err := persister.SaveState(existingState); err != nil {
		t.Fatal(err)
	}

	store := newMockBeadStoreWithList()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	ctx := context.Background()
	startCfg := &StartupConfig{
		WorkflowID: "new-workflow", // This should be ignored on resume
		StateDir:   stateDir,
	}

	err := orch.StartOrResume(ctx, startCfg)
	if err != nil {
		t.Fatalf("StartOrResume() error = %v", err)
	}
	defer orch.ReleaseLock()

	// Verify template was NOT expanded (resume, not fresh start)
	if len(expander.expanded) > 0 {
		t.Errorf("Template should not be expanded on resume, got %v", expander.expanded)
	}

	// Verify state was loaded with existing workflow ID
	state, _ := persister.LoadState()
	if state.WorkflowID != "existing-workflow" {
		t.Errorf("WorkflowID = %s, want existing-workflow", state.WorkflowID)
	}
	// PID should be updated to current process
	if state.PID != os.Getpid() {
		t.Errorf("PID = %d, want %d", state.PID, os.Getpid())
	}
}

func TestOrchestrator_StartOrResume_RecoverDeadAgent(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	// Create existing state
	persister := NewStatePersister(stateDir)
	existingState := &OrchestratorState{
		Version:    "1",
		WorkflowID: "crash-workflow",
		PID:        99999,
	}
	if err := persister.SaveState(existingState); err != nil {
		t.Fatal(err)
	}

	store := newMockBeadStoreWithList()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	// Add an in-progress task assigned to a dead agent
	deadAgentBead := &types.Bead{
		ID:       "bd-task-dead",
		Type:     types.BeadTypeTask,
		Title:    "Task from dead agent",
		Status:   types.BeadStatusInProgress,
		Assignee: "claude-dead", // Agent is not running
	}
	store.beads[deadAgentBead.ID] = deadAgentBead
	// Note: agents.running["claude-dead"] is not set, so IsRunning returns false

	ctx := context.Background()
	startCfg := &StartupConfig{
		StateDir: stateDir,
	}

	err := orch.StartOrResume(ctx, startCfg)
	if err != nil {
		t.Fatalf("StartOrResume() error = %v", err)
	}
	defer orch.ReleaseLock()

	// Verify bead was reset to open
	if store.beads["bd-task-dead"].Status != types.BeadStatusOpen {
		t.Errorf("Bead status = %s, want open (recovered)", store.beads["bd-task-dead"].Status)
	}
}

func TestOrchestrator_StartOrResume_KeepLiveAgentBead(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	// Create existing state
	persister := NewStatePersister(stateDir)
	existingState := &OrchestratorState{
		Version:    "1",
		WorkflowID: "crash-workflow",
		PID:        99999,
	}
	if err := persister.SaveState(existingState); err != nil {
		t.Fatal(err)
	}

	store := newMockBeadStoreWithList()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	// Add an in-progress task assigned to a live agent
	liveAgentBead := &types.Bead{
		ID:       "bd-task-live",
		Type:     types.BeadTypeTask,
		Title:    "Task from live agent",
		Status:   types.BeadStatusInProgress,
		Assignee: "claude-live",
	}
	store.beads[liveAgentBead.ID] = liveAgentBead
	agents.running["claude-live"] = true // Agent is running

	ctx := context.Background()
	startCfg := &StartupConfig{
		StateDir: stateDir,
	}

	err := orch.StartOrResume(ctx, startCfg)
	if err != nil {
		t.Fatalf("StartOrResume() error = %v", err)
	}
	defer orch.ReleaseLock()

	// Verify bead remains in_progress (agent is still alive)
	if store.beads["bd-task-live"].Status != types.BeadStatusInProgress {
		t.Errorf("Bead status = %s, want in_progress (agent alive)", store.beads["bd-task-live"].Status)
	}
}

func TestOrchestrator_StartOrResume_RecoverOrchestratorBead(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	// Create existing state
	persister := NewStatePersister(stateDir)
	existingState := &OrchestratorState{
		Version:    "1",
		WorkflowID: "crash-workflow",
		PID:        99999,
	}
	if err := persister.SaveState(existingState); err != nil {
		t.Fatal(err)
	}

	store := newMockBeadStoreWithList()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	// Add an in-progress code bead (orchestrator bead, no assignee)
	codeBead := &types.Bead{
		ID:     "bd-code-crash",
		Type:   types.BeadTypeCode,
		Title:  "Code bead that crashed",
		Status: types.BeadStatusInProgress,
		// No assignee - orchestrator was executing this
	}
	store.beads[codeBead.ID] = codeBead

	ctx := context.Background()
	startCfg := &StartupConfig{
		StateDir: stateDir,
	}

	err := orch.StartOrResume(ctx, startCfg)
	if err != nil {
		t.Fatalf("StartOrResume() error = %v", err)
	}
	defer orch.ReleaseLock()

	// Verify code bead was reset to open
	if store.beads["bd-code-crash"].Status != types.BeadStatusOpen {
		t.Errorf("Bead status = %s, want open (orchestrator bead recovered)", store.beads["bd-code-crash"].Status)
	}
}

func TestOrchestrator_StartOrResume_RecoverUnassignedTask(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	// Create existing state
	persister := NewStatePersister(stateDir)
	existingState := &OrchestratorState{
		Version:    "1",
		WorkflowID: "crash-workflow",
		PID:        99999,
	}
	if err := persister.SaveState(existingState); err != nil {
		t.Fatal(err)
	}

	store := newMockBeadStoreWithList()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	// Add an in-progress task bead with NO assignee
	// This could happen if orchestrator crashed after marking in_progress but before agent claimed it
	taskBead := &types.Bead{
		ID:     "bd-task-orphan",
		Type:   types.BeadTypeTask,
		Title:  "Orphaned task with no assignee",
		Status: types.BeadStatusInProgress,
		// No assignee - no agent is working on this
	}
	store.beads[taskBead.ID] = taskBead

	ctx := context.Background()
	startCfg := &StartupConfig{
		StateDir: stateDir,
	}

	err := orch.StartOrResume(ctx, startCfg)
	if err != nil {
		t.Fatalf("StartOrResume() error = %v", err)
	}
	defer orch.ReleaseLock()

	// Verify unassigned task was reset to open
	if store.beads["bd-task-orphan"].Status != types.BeadStatusOpen {
		t.Errorf("Bead status = %s, want open (unassigned task recovered)", store.beads["bd-task-orphan"].Status)
	}
}

func TestOrchestrator_StartOrResume_LockConflict(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	store := newMockBeadStoreWithList()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()

	// First orchestrator acquires lock
	orch1 := New(cfg, store, agents, expander, executor, logger)
	ctx := context.Background()
	startCfg := &StartupConfig{
		WorkflowID: "first-workflow",
		StateDir:   stateDir,
	}

	err := orch1.StartOrResume(ctx, startCfg)
	if err != nil {
		t.Fatalf("First StartOrResume() error = %v", err)
	}
	defer orch1.ReleaseLock()

	// Second orchestrator should fail
	orch2 := New(cfg, store, agents, expander, executor, logger)
	err = orch2.StartOrResume(ctx, startCfg)
	if err == nil {
		t.Error("Second StartOrResume() should fail with lock conflict")
		orch2.ReleaseLock()
	}
}

// retryingCodeExecutor fails N times before succeeding
type retryingCodeExecutor struct {
	mu         sync.Mutex
	failUntil  int
	callCount  int
}

func (e *retryingCodeExecutor) Execute(ctx context.Context, spec *types.CodeSpec) (map[string]any, error) {
	e.mu.Lock()
	e.callCount++
	count := e.callCount
	e.mu.Unlock()

	if count <= e.failUntil {
		return nil, fmt.Errorf("simulated failure %d", count)
	}
	return map[string]any{"exit_code": 0}, nil
}

func TestOrchestrator_CodeRetry(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := &retryingCodeExecutor{failUntil: 2} // Fail twice, succeed on 3rd
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add a code bead with retry
	codeBead := &types.Bead{
		ID:     "bd-retry-001",
		Type:   types.BeadTypeCode,
		Title:  "Retry code",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code:       "echo test",
			OnError:    types.OnErrorRetry,
			MaxRetries: 3,
		},
	}
	store.addReady(codeBead)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Verify bead was closed (retry succeeded on 3rd attempt)
	if store.beads["bd-retry-001"].Status != types.BeadStatusClosed {
		t.Errorf("Bead status = %s, want closed", store.beads["bd-retry-001"].Status)
	}

	// Verify it was called 3 times
	if executor.callCount != 3 {
		t.Errorf("Execute called %d times, want 3", executor.callCount)
	}
}

// blockingCodeExecutor blocks until context is cancelled
type blockingCodeExecutor struct{}

func (e *blockingCodeExecutor) Execute(ctx context.Context, spec *types.CodeSpec) (map[string]any, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestOrchestrator_ConditionTimeout(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := &blockingCodeExecutor{}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add a condition bead with timeout
	condBead := &types.Bead{
		ID:     "bd-timeout-001",
		Type:   types.BeadTypeCondition,
		Title:  "Timeout condition",
		Status: types.BeadStatusOpen,
		ConditionSpec: &types.ConditionSpec{
			Condition: "sleep 10",
			Timeout:   "100ms",
			OnTimeout: &types.ExpansionTarget{
				Template: "on-timeout-template",
			},
		},
	}
	store.addReady(condBead)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Wait for condition goroutine
	time.Sleep(200 * time.Millisecond)

	// Verify on_timeout template was expanded
	if len(expander.expanded) != 1 || expander.expanded[0] != "on-timeout-template" {
		t.Errorf("Expected on-timeout-template to be expanded, got %v", expander.expanded)
	}
}

// =============================================================================
// PRIMITIVE HANDLER UNIT TESTS
// =============================================================================

// --- Task Handler Tests ---

func TestHandler_Task_WaitsForAgentClose(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Mark agent as running
	agents.running["claude-1"] = true

	// Add a task bead with assignee
	taskBead := &types.Bead{
		ID:       "bd-task-wait",
		Type:     types.BeadTypeTask,
		Title:    "Wait for agent",
		Status:   types.BeadStatusOpen,
		Assignee: "claude-1",
	}
	store.addReady(taskBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run briefly - task won't auto-close
	_ = orch.Run(ctx)

	// Verify task is in_progress but NOT closed (must be closed by agent)
	if store.beads["bd-task-wait"].Status != types.BeadStatusInProgress {
		t.Errorf("Task status = %s, want in_progress", store.beads["bd-task-wait"].Status)
	}
}

func TestHandler_Task_WarnsOnNonRunningAgent(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Agent is NOT running
	// agents.running["claude-dead"] is not set

	// Add a task bead with non-running agent
	taskBead := &types.Bead{
		ID:       "bd-task-dead-agent",
		Type:     types.BeadTypeTask,
		Title:    "Task with dead agent",
		Status:   types.BeadStatusOpen,
		Assignee: "claude-dead",
	}
	store.addReady(taskBead)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Should not error - just warns
	_ = orch.Run(ctx)

	// Task should still be marked in_progress
	if store.beads["bd-task-dead-agent"].Status != types.BeadStatusInProgress {
		t.Errorf("Task status = %s, want in_progress", store.beads["bd-task-dead-agent"].Status)
	}
}

func TestHandler_Task_NoAssigneeMarksInProgress(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Task with no assignee
	taskBead := &types.Bead{
		ID:     "bd-task-unassigned",
		Type:   types.BeadTypeTask,
		Title:  "Unassigned task",
		Status: types.BeadStatusOpen,
		// No assignee
	}
	store.addReady(taskBead)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Task should be marked in_progress
	if store.beads["bd-task-unassigned"].Status != types.BeadStatusInProgress {
		t.Errorf("Task status = %s, want in_progress", store.beads["bd-task-unassigned"].Status)
	}
}

// --- Condition Handler Tests ---

func TestHandler_Condition_FalseExpandsOnFalse(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Set up executor to return false (exit non-0)
	executor.results["test -f missing.txt"] = map[string]any{"exit_code": 1}

	// Add a condition bead
	condBead := &types.Bead{
		ID:     "bd-cond-false",
		Type:   types.BeadTypeCondition,
		Title:  "Check missing file",
		Status: types.BeadStatusOpen,
		ConditionSpec: &types.ConditionSpec{
			Condition: "test -f missing.txt",
			OnTrue: &types.ExpansionTarget{
				Template: "on-true-template",
			},
			OnFalse: &types.ExpansionTarget{
				Template: "on-false-template",
			},
		},
	}
	store.addReady(condBead)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Wait for goroutine
	time.Sleep(50 * time.Millisecond)

	// Verify on_false template was expanded
	if len(expander.expanded) != 1 || expander.expanded[0] != "on-false-template" {
		t.Errorf("Expected on-false-template to be expanded, got %v", expander.expanded)
	}
}

func TestHandler_Condition_ErrorExpandsOnFalse(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Set up executor to return error
	executor.errors["command-that-errors"] = fmt.Errorf("execution failed")

	condBead := &types.Bead{
		ID:     "bd-cond-error",
		Type:   types.BeadTypeCondition,
		Title:  "Error condition",
		Status: types.BeadStatusOpen,
		ConditionSpec: &types.ConditionSpec{
			Condition: "command-that-errors",
			OnTrue: &types.ExpansionTarget{
				Template: "on-true-template",
			},
			OnFalse: &types.ExpansionTarget{
				Template: "on-false-template",
			},
		},
	}
	store.addReady(condBead)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Wait for goroutine
	time.Sleep(50 * time.Millisecond)

	// Verify on_false template was expanded (error = false)
	if len(expander.expanded) != 1 || expander.expanded[0] != "on-false-template" {
		t.Errorf("Expected on-false-template on error, got %v", expander.expanded)
	}
}

func TestHandler_Condition_TimeoutFallsBackToOnFalse(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := &blockingCodeExecutor{} // Blocks forever
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Condition with timeout but NO on_timeout (should fall back to on_false)
	condBead := &types.Bead{
		ID:     "bd-cond-timeout-fallback",
		Type:   types.BeadTypeCondition,
		Title:  "Timeout fallback",
		Status: types.BeadStatusOpen,
		ConditionSpec: &types.ConditionSpec{
			Condition: "sleep 10",
			Timeout:   "50ms",
			OnTrue: &types.ExpansionTarget{
				Template: "on-true-template",
			},
			OnFalse: &types.ExpansionTarget{
				Template: "on-false-fallback",
			},
			// No OnTimeout - should fall back to OnFalse
		},
	}
	store.addReady(condBead)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Wait for goroutine
	time.Sleep(150 * time.Millisecond)

	// Verify on_false was used as fallback
	if len(expander.expanded) != 1 || expander.expanded[0] != "on-false-fallback" {
		t.Errorf("Expected on-false-fallback on timeout without on_timeout, got %v", expander.expanded)
	}
}

func TestHandler_Condition_RunsInGoroutine(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Set up executor (will execute in goroutine)
	executor.results["echo test"] = map[string]any{"exit_code": 0}

	condBead := &types.Bead{
		ID:     "bd-cond-goroutine",
		Type:   types.BeadTypeCondition,
		Title:  "Goroutine test",
		Status: types.BeadStatusOpen,
		ConditionSpec: &types.ConditionSpec{
			Condition: "echo test",
			OnTrue: &types.ExpansionTarget{
				Template: "success",
			},
		},
	}
	store.addReady(condBead)

	ctx := context.Background()

	// Call dispatch directly - should NOT block
	err := orch.dispatch(ctx, condBead)
	if err != nil {
		t.Errorf("dispatch() error = %v", err)
	}

	// Bead should be in_progress immediately (goroutine spawned)
	if store.beads["bd-cond-goroutine"].Status != types.BeadStatusInProgress {
		t.Errorf("Condition bead should be in_progress immediately, got %s", store.beads["bd-cond-goroutine"].Status)
	}

	// Wait for goroutine to complete
	time.Sleep(50 * time.Millisecond)
	orch.waitForConditions()

	// Now it should be closed
	if store.beads["bd-cond-goroutine"].Status != types.BeadStatusClosed {
		t.Errorf("Condition bead should be closed after goroutine completes, got %s", store.beads["bd-cond-goroutine"].Status)
	}
}

func TestHandler_Condition_CancellationStopsGoroutine(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := &blockingCodeExecutor{} // Blocks until cancelled
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	condBead := &types.Bead{
		ID:     "bd-cond-cancel",
		Type:   types.BeadTypeCondition,
		Title:  "Cancellable condition",
		Status: types.BeadStatusOpen,
		ConditionSpec: &types.ConditionSpec{
			Condition: "sleep 3600", // Would block for an hour
			OnTrue: &types.ExpansionTarget{
				Template: "never-reached",
			},
		},
	}
	store.addReady(condBead)

	ctx, cancel := context.WithCancel(context.Background())

	// Start the orchestrator
	done := make(chan error, 1)
	go func() {
		done <- orch.Run(ctx)
	}()

	// Give it time to start the condition goroutine
	time.Sleep(50 * time.Millisecond)

	// Cancel the context
	cancel()

	// Should complete quickly
	select {
	case <-done:
		// Good - completed
	case <-time.After(200 * time.Millisecond):
		t.Error("Orchestrator did not stop after cancellation")
	}

	// Template should NOT have been expanded
	if len(expander.expanded) > 0 {
		t.Errorf("No template should be expanded on cancellation, got %v", expander.expanded)
	}
}

func TestHandler_Condition_MissingSpecReturnsError(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	// Condition bead WITHOUT spec
	condBead := &types.Bead{
		ID:     "bd-cond-no-spec",
		Type:   types.BeadTypeCondition,
		Title:  "No spec",
		Status: types.BeadStatusOpen,
		// ConditionSpec is nil
	}

	ctx := context.Background()

	err := orch.dispatch(ctx, condBead)
	if err == nil {
		t.Error("dispatch() should error with missing spec")
	}
}

// --- Code Handler Tests ---

func TestHandler_Code_CapturesStdout(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Set up executor result with stdout
	executor.results["echo hello"] = map[string]any{
		"stdout":    "hello\n",
		"stderr":    "",
		"exit_code": 0,
	}

	codeBead := &types.Bead{
		ID:     "bd-code-stdout",
		Type:   types.BeadTypeCode,
		Title:  "Capture stdout",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code: "echo hello",
		},
	}
	store.addReady(codeBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v", err)
	}

	// Verify stdout was captured in outputs
	bead := store.beads["bd-code-stdout"]
	if bead.Outputs["stdout"] != "hello\n" {
		t.Errorf("stdout = %v, want 'hello\\n'", bead.Outputs["stdout"])
	}
	if bead.Status != types.BeadStatusClosed {
		t.Errorf("Status = %s, want closed", bead.Status)
	}
}

func TestHandler_Code_CapturesStderr(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	executor.results["echo error >&2"] = map[string]any{
		"stdout":    "",
		"stderr":    "error\n",
		"exit_code": 0,
	}

	codeBead := &types.Bead{
		ID:     "bd-code-stderr",
		Type:   types.BeadTypeCode,
		Title:  "Capture stderr",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code: "echo error >&2",
		},
	}
	store.addReady(codeBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	bead := store.beads["bd-code-stderr"]
	if bead.Outputs["stderr"] != "error\n" {
		t.Errorf("stderr = %v, want 'error\\n'", bead.Outputs["stderr"])
	}
}

func TestHandler_Code_OnErrorContinue(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Set up executor to fail
	executor.errors["failing-command"] = fmt.Errorf("command failed")

	codeBead := &types.Bead{
		ID:     "bd-code-continue",
		Type:   types.BeadTypeCode,
		Title:  "Continue on error",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code:    "failing-command",
			OnError: types.OnErrorContinue,
		},
	}
	store.addReady(codeBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() should not error with on_error=continue, got %v", err)
	}

	// Bead should still be closed (continue means proceed despite error)
	bead := store.beads["bd-code-continue"]
	if bead.Status != types.BeadStatusClosed {
		t.Errorf("Bead status = %s, want closed", bead.Status)
	}
}

func TestHandler_Code_OnErrorAbort(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	executor.errors["abort-command"] = fmt.Errorf("critical failure")

	codeBead := &types.Bead{
		ID:     "bd-code-abort",
		Type:   types.BeadTypeCode,
		Title:  "Abort on error",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code:    "abort-command",
			OnError: types.OnErrorAbort,
		},
	}
	store.addReady(codeBead)

	ctx := context.Background()

	// Dispatch directly to see the error
	err := orch.dispatch(ctx, codeBead)
	if err == nil {
		t.Error("dispatch() should return error with on_error=abort")
	}

	// Bead should NOT be closed (aborted)
	bead := store.beads["bd-code-abort"]
	if bead.Status == types.BeadStatusClosed {
		t.Error("Bead should not be closed on abort")
	}
}

func TestHandler_Code_RetryExhausted(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := &retryingCodeExecutor{failUntil: 10} // Fail more than max retries
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	codeBead := &types.Bead{
		ID:     "bd-code-retry-exhausted",
		Type:   types.BeadTypeCode,
		Title:  "Retry exhausted",
		Status: types.BeadStatusOpen,
		CodeSpec: &types.CodeSpec{
			Code:       "always-fails",
			OnError:    types.OnErrorRetry,
			MaxRetries: 3,
		},
	}
	store.addReady(codeBead)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Should have been called exactly MaxRetries times
	if executor.callCount != 3 {
		t.Errorf("Execute called %d times, want 3", executor.callCount)
	}

	// Bead should be closed (retries exhausted, but continues)
	bead := store.beads["bd-code-retry-exhausted"]
	if bead.Status != types.BeadStatusClosed {
		t.Errorf("Bead status = %s, want closed", bead.Status)
	}
}

func TestHandler_Code_MissingSpecReturnsError(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	codeBead := &types.Bead{
		ID:     "bd-code-no-spec",
		Type:   types.BeadTypeCode,
		Title:  "No spec",
		Status: types.BeadStatusOpen,
		// CodeSpec is nil
	}

	ctx := context.Background()

	err := orch.dispatch(ctx, codeBead)
	if err == nil {
		t.Error("dispatch() should error with missing CodeSpec")
	}
}

// --- Expand Handler Tests ---

func TestHandler_Expand_ExpandsTemplate(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	expandBead := &types.Bead{
		ID:     "bd-expand-test",
		Type:   types.BeadTypeExpand,
		Title:  "Expand template",
		Status: types.BeadStatusOpen,
		ExpandSpec: &types.ExpandSpec{
			Template: "tdd-workflow",
		},
	}
	store.addReady(expandBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify template was expanded
	if len(expander.expanded) != 1 || expander.expanded[0] != "tdd-workflow" {
		t.Errorf("Expected tdd-workflow to be expanded, got %v", expander.expanded)
	}

	// Verify bead was closed
	if store.beads["bd-expand-test"].Status != types.BeadStatusClosed {
		t.Errorf("Status = %s, want closed", store.beads["bd-expand-test"].Status)
	}
}

func TestHandler_Expand_PassesVariables(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Custom expander that records variables
	var capturedVars map[string]string
	expander := &variableCapturingExpander{
		onExpand: func(spec *types.ExpandSpec) {
			capturedVars = spec.Variables
		},
	}

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	expandBead := &types.Bead{
		ID:     "bd-expand-vars",
		Type:   types.BeadTypeExpand,
		Title:  "Expand with vars",
		Status: types.BeadStatusOpen,
		ExpandSpec: &types.ExpandSpec{
			Template: "parameterized",
			Variables: map[string]string{
				"task_id":     "bd-123",
				"agent_name":  "claude-1",
				"custom_flag": "true",
			},
		},
	}
	store.addReady(expandBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify variables were passed
	if capturedVars["task_id"] != "bd-123" {
		t.Errorf("task_id = %s, want bd-123", capturedVars["task_id"])
	}
	if capturedVars["agent_name"] != "claude-1" {
		t.Errorf("agent_name = %s, want claude-1", capturedVars["agent_name"])
	}
}

func TestHandler_Expand_MissingSpecReturnsError(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	expandBead := &types.Bead{
		ID:     "bd-expand-no-spec",
		Type:   types.BeadTypeExpand,
		Title:  "No spec",
		Status: types.BeadStatusOpen,
		// ExpandSpec is nil
	}

	ctx := context.Background()

	err := orch.dispatch(ctx, expandBead)
	if err == nil {
		t.Error("dispatch() should error with missing ExpandSpec")
	}
}

func TestHandler_Expand_PassesParentBead(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Custom expander that captures parent
	var capturedParent *types.Bead
	expander := &parentCapturingExpander{
		onExpand: func(parent *types.Bead) {
			capturedParent = parent
		},
	}

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	expandBead := &types.Bead{
		ID:     "bd-expand-parent",
		Type:   types.BeadTypeExpand,
		Title:  "Parent test",
		Status: types.BeadStatusOpen,
		ExpandSpec: &types.ExpandSpec{
			Template: "child-template",
		},
	}
	store.addReady(expandBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify parent was passed
	if capturedParent == nil {
		t.Error("Parent bead should be passed to expander")
	}
	if capturedParent.ID != "bd-expand-parent" {
		t.Errorf("Parent ID = %s, want bd-expand-parent", capturedParent.ID)
	}
}

// --- Start Handler Tests ---

func TestHandler_Start_CreatesSession(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	startBead := &types.Bead{
		ID:     "bd-start-session",
		Type:   types.BeadTypeStart,
		Title:  "Start agent",
		Status: types.BeadStatusOpen,
		StartSpec: &types.StartSpec{
			Agent:   "claude-test",
			Workdir: "/tmp/test",
		},
	}
	store.addReady(startBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify agent was started
	if len(agents.started) != 1 || agents.started[0] != "claude-test" {
		t.Errorf("Expected claude-test to be started, got %v", agents.started)
	}

	// Verify agent is now running
	if !agents.running["claude-test"] {
		t.Error("Agent should be marked as running")
	}

	// Verify bead was closed
	if store.beads["bd-start-session"].Status != types.BeadStatusClosed {
		t.Errorf("Status = %s, want closed", store.beads["bd-start-session"].Status)
	}
}

func TestHandler_Start_SetsWorkdir(t *testing.T) {
	store := newMockBeadStore()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Custom agent manager to capture spec
	var capturedSpec *types.StartSpec
	agents := &specCapturingAgentManager{
		onStart: func(spec *types.StartSpec) {
			capturedSpec = spec
		},
	}

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	startBead := &types.Bead{
		ID:     "bd-start-workdir",
		Type:   types.BeadTypeStart,
		Title:  "Start with workdir",
		Status: types.BeadStatusOpen,
		StartSpec: &types.StartSpec{
			Agent:   "claude-workdir",
			Workdir: "/data/projects/test",
		},
	}
	store.addReady(startBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify workdir was passed
	if capturedSpec.Workdir != "/data/projects/test" {
		t.Errorf("Workdir = %s, want /data/projects/test", capturedSpec.Workdir)
	}
}

func TestHandler_Start_ResumeSession(t *testing.T) {
	store := newMockBeadStore()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	var capturedSpec *types.StartSpec
	agents := &specCapturingAgentManager{
		onStart: func(spec *types.StartSpec) {
			capturedSpec = spec
		},
	}

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	startBead := &types.Bead{
		ID:     "bd-start-resume",
		Type:   types.BeadTypeStart,
		Title:  "Resume session",
		Status: types.BeadStatusOpen,
		StartSpec: &types.StartSpec{
			Agent:         "claude-resume",
			ResumeSession: "session-abc-123",
		},
	}
	store.addReady(startBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify resume session was passed
	if capturedSpec.ResumeSession != "session-abc-123" {
		t.Errorf("ResumeSession = %s, want session-abc-123", capturedSpec.ResumeSession)
	}
}

func TestHandler_Start_CustomPrompt(t *testing.T) {
	store := newMockBeadStore()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	var capturedSpec *types.StartSpec
	agents := &specCapturingAgentManager{
		onStart: func(spec *types.StartSpec) {
			capturedSpec = spec
		},
	}

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	startBead := &types.Bead{
		ID:     "bd-start-prompt",
		Type:   types.BeadTypeStart,
		Title:  "Custom prompt",
		Status: types.BeadStatusOpen,
		StartSpec: &types.StartSpec{
			Agent:  "claude-prompt",
			Prompt: "/start bd-task-123",
		},
	}
	store.addReady(startBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify custom prompt was passed
	if capturedSpec.Prompt != "/start bd-task-123" {
		t.Errorf("Prompt = %s, want /start bd-task-123", capturedSpec.Prompt)
	}
}

func TestHandler_Start_MissingSpecReturnsError(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	startBead := &types.Bead{
		ID:     "bd-start-no-spec",
		Type:   types.BeadTypeStart,
		Title:  "No spec",
		Status: types.BeadStatusOpen,
		// StartSpec is nil
	}

	ctx := context.Background()

	err := orch.dispatch(ctx, startBead)
	if err == nil {
		t.Error("dispatch() should error with missing StartSpec")
	}
}

// --- Stop Handler Tests ---

func TestHandler_Stop_StopsSession(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Start the agent first
	agents.running["claude-stop"] = true

	stopBead := &types.Bead{
		ID:     "bd-stop-session",
		Type:   types.BeadTypeStop,
		Title:  "Stop agent",
		Status: types.BeadStatusOpen,
		StopSpec: &types.StopSpec{
			Agent:    "claude-stop",
			Graceful: true,
		},
	}
	store.addReady(stopBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify agent was stopped
	if len(agents.stopped) != 1 || agents.stopped[0] != "claude-stop" {
		t.Errorf("Expected claude-stop to be stopped, got %v", agents.stopped)
	}

	// Verify agent is no longer running
	if agents.running["claude-stop"] {
		t.Error("Agent should not be running after stop")
	}

	// Verify bead was closed
	if store.beads["bd-stop-session"].Status != types.BeadStatusClosed {
		t.Errorf("Status = %s, want closed", store.beads["bd-stop-session"].Status)
	}
}

func TestHandler_Stop_GracefulFlag(t *testing.T) {
	store := newMockBeadStore()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	var capturedSpec *types.StopSpec
	agents := &stopSpecCapturingAgentManager{
		onStop: func(spec *types.StopSpec) {
			capturedSpec = spec
		},
	}

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	stopBead := &types.Bead{
		ID:     "bd-stop-graceful",
		Type:   types.BeadTypeStop,
		Title:  "Graceful stop",
		Status: types.BeadStatusOpen,
		StopSpec: &types.StopSpec{
			Agent:    "claude-graceful",
			Graceful: true,
			Timeout:  30,
		},
	}
	store.addReady(stopBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify graceful and timeout were passed
	if !capturedSpec.Graceful {
		t.Error("Graceful should be true")
	}
	if capturedSpec.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", capturedSpec.Timeout)
	}
}

func TestHandler_Stop_ForceKill(t *testing.T) {
	store := newMockBeadStore()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	var capturedSpec *types.StopSpec
	agents := &stopSpecCapturingAgentManager{
		onStop: func(spec *types.StopSpec) {
			capturedSpec = spec
		},
	}

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	stopBead := &types.Bead{
		ID:     "bd-stop-force",
		Type:   types.BeadTypeStop,
		Title:  "Force stop",
		Status: types.BeadStatusOpen,
		StopSpec: &types.StopSpec{
			Agent:    "claude-force",
			Graceful: false, // Force kill
		},
	}
	store.addReady(stopBead)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Verify force kill (not graceful)
	if capturedSpec.Graceful {
		t.Error("Graceful should be false for force kill")
	}
}

func TestHandler_Stop_MissingSpecReturnsError(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	stopBead := &types.Bead{
		ID:     "bd-stop-no-spec",
		Type:   types.BeadTypeStop,
		Title:  "No spec",
		Status: types.BeadStatusOpen,
		// StopSpec is nil
	}

	ctx := context.Background()

	err := orch.dispatch(ctx, stopBead)
	if err == nil {
		t.Error("dispatch() should error with missing StopSpec")
	}
}

// --- Dispatch Unknown Type Test ---

func TestHandler_UnknownTypeReturnsError(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	unknownBead := &types.Bead{
		ID:     "bd-unknown",
		Type:   types.BeadType("invalid"),
		Title:  "Unknown type",
		Status: types.BeadStatusOpen,
	}

	ctx := context.Background()

	err := orch.dispatch(ctx, unknownBead)
	if err == nil {
		t.Error("dispatch() should error with unknown bead type")
	}
}

// =============================================================================
// HELPER TYPES FOR TESTS
// =============================================================================

// variableCapturingExpander captures variables passed to Expand.
type variableCapturingExpander struct {
	onExpand func(spec *types.ExpandSpec)
}

func (e *variableCapturingExpander) Expand(ctx context.Context, spec *types.ExpandSpec, parent *types.Bead) error {
	if e.onExpand != nil {
		e.onExpand(spec)
	}
	return nil
}

// parentCapturingExpander captures the parent bead passed to Expand.
type parentCapturingExpander struct {
	onExpand func(parent *types.Bead)
}

func (e *parentCapturingExpander) Expand(ctx context.Context, spec *types.ExpandSpec, parent *types.Bead) error {
	if e.onExpand != nil {
		e.onExpand(parent)
	}
	return nil
}

// specCapturingAgentManager captures StartSpec.
type specCapturingAgentManager struct {
	onStart func(spec *types.StartSpec)
	running map[string]bool
}

func (m *specCapturingAgentManager) Start(ctx context.Context, spec *types.StartSpec) error {
	if m.onStart != nil {
		m.onStart(spec)
	}
	if m.running == nil {
		m.running = make(map[string]bool)
	}
	m.running[spec.Agent] = true
	return nil
}

func (m *specCapturingAgentManager) Stop(ctx context.Context, spec *types.StopSpec) error {
	if m.running != nil {
		m.running[spec.Agent] = false
	}
	return nil
}

func (m *specCapturingAgentManager) IsRunning(ctx context.Context, agentID string) (bool, error) {
	if m.running == nil {
		return false, nil
	}
	return m.running[agentID], nil
}

// stopSpecCapturingAgentManager captures StopSpec.
type stopSpecCapturingAgentManager struct {
	onStop  func(spec *types.StopSpec)
	running map[string]bool
}

func (m *stopSpecCapturingAgentManager) Start(ctx context.Context, spec *types.StartSpec) error {
	if m.running == nil {
		m.running = make(map[string]bool)
	}
	m.running[spec.Agent] = true
	return nil
}

func (m *stopSpecCapturingAgentManager) Stop(ctx context.Context, spec *types.StopSpec) error {
	if m.onStop != nil {
		m.onStop(spec)
	}
	if m.running != nil {
		m.running[spec.Agent] = false
	}
	return nil
}

func (m *stopSpecCapturingAgentManager) IsRunning(ctx context.Context, agentID string) (bool, error) {
	if m.running == nil {
		return false, nil
	}
	return m.running[agentID], nil
}

// =============================================================================
// INLINE STEPS AND EPHEMERAL CLEANUP TESTS
// =============================================================================

// mockBeadStoreWithCreateAndDelete extends mockBeadStore with Create and Delete support.
type mockBeadStoreWithCreateAndDelete struct {
	*mockBeadStore
	created []*types.Bead
}

func newMockBeadStoreWithCreateAndDelete() *mockBeadStoreWithCreateAndDelete {
	return &mockBeadStoreWithCreateAndDelete{
		mockBeadStore: newMockBeadStore(),
		created:       make([]*types.Bead, 0),
	}
}

func (m *mockBeadStoreWithCreateAndDelete) Create(ctx context.Context, bead *types.Bead) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.beads[bead.ID] = bead
	m.created = append(m.created, bead)
	return nil
}

func (m *mockBeadStoreWithCreateAndDelete) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.beads, id)
	return nil
}

func (m *mockBeadStoreWithCreateAndDelete) List(ctx context.Context, status types.BeadStatus) ([]*types.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*types.Bead
	for _, bead := range m.beads {
		if status == "" || bead.Status == status {
			result = append(result, bead)
		}
	}
	return result, nil
}

func TestHandler_Condition_ExpandsInlineSteps(t *testing.T) {
	store := newMockBeadStoreWithCreateAndDelete()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Set up executor to return true (exit 0)
	executor.results["check-something"] = map[string]any{"exit_code": 0}

	// Add a condition bead with inline steps
	condBead := &types.Bead{
		ID:     "bd-cond-inline",
		Type:   types.BeadTypeCondition,
		Title:  "Inline steps",
		Status: types.BeadStatusOpen,
		ConditionSpec: &types.ConditionSpec{
			Condition: "check-something",
			OnTrue: &types.ExpansionTarget{
				Inline: []json.RawMessage{
					json.RawMessage(`{"id": "step-1", "type": "task", "description": "First step"}`),
					json.RawMessage(`{"id": "step-2", "type": "task", "description": "Second step", "needs": ["step-1"]}`),
				},
			},
		},
	}
	store.addReady(condBead)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_ = orch.Run(ctx)

	// Wait for goroutine
	time.Sleep(100 * time.Millisecond)

	// Verify inline beads were created
	if len(store.created) < 2 {
		t.Errorf("Expected at least 2 inline beads created, got %d", len(store.created))
	}

	// Check the first step
	step1 := store.beads["bd-cond-inline.inline.step-1"]
	if step1 == nil {
		t.Error("step-1 bead should be created")
	} else {
		if step1.Type != types.BeadTypeTask {
			t.Errorf("step-1 type = %s, want task", step1.Type)
		}
		if step1.Title != "First step" {
			t.Errorf("step-1 title = %s, want 'First step'", step1.Title)
		}
		if step1.Parent != "bd-cond-inline" {
			t.Errorf("step-1 parent = %s, want bd-cond-inline", step1.Parent)
		}
	}

	// Check the second step with dependency
	step2 := store.beads["bd-cond-inline.inline.step-2"]
	if step2 == nil {
		t.Error("step-2 bead should be created")
	} else {
		// step-2 should depend on step-1
		hasStep1Dep := false
		for _, need := range step2.Needs {
			if need == "bd-cond-inline.inline.step-1" {
				hasStep1Dep = true
				break
			}
		}
		if !hasStep1Dep {
			t.Errorf("step-2 should depend on step-1, needs = %v", step2.Needs)
		}
	}
}

func TestOrchestrator_Run_CleansUpWispsOnCompletion(t *testing.T) {
	store := newMockBeadStoreWithFilteredListing()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond
	cfg.Cleanup.Ephemeral = config.EphemeralCleanupOnComplete

	orch := New(cfg, store, agents, expander, executor, logger)

	// Initialize state (needed for cleanupWorkflow to know the workflow ID)
	workflowID := "wf-run-cleanup"
	orch.state = &OrchestratorState{
		WorkflowID: workflowID,
	}

	// Add a work bead (should NOT be deleted)
	workBead := &types.Bead{
		ID:         "bd-work-001",
		Type:       types.BeadTypeTask,
		Title:      "Normal work bead",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWork,
		WorkflowID: workflowID,
	}
	store.beads[workBead.ID] = workBead

	// Add a wisp bead (should be deleted on completion)
	wispBead := &types.Bead{
		ID:         "wisp-step-001",
		Type:       types.BeadTypeTask,
		Title:      "Wisp step",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWisp,
		WorkflowID: workflowID,
	}
	store.beads[wispBead.ID] = wispBead

	// No ready beads - should complete immediately and cleanup wisps
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v", err)
	}

	// Work bead should still exist
	if store.beads["bd-work-001"] == nil {
		t.Error("Work bead should NOT be deleted")
	}

	// Wisp bead should be deleted
	if store.beads["wisp-step-001"] != nil {
		t.Error("Wisp bead should be deleted on workflow completion")
	}
}

func TestHandler_Start_FailureReturnsError(t *testing.T) {
	store := newMockBeadStore()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Agent manager that fails to start
	agents := &failingAgentManager{
		failStart: true,
		err:       fmt.Errorf("tmux session creation failed"),
	}

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	startBead := &types.Bead{
		ID:     "bd-start-fail",
		Type:   types.BeadTypeStart,
		Title:  "Failing start",
		Status: types.BeadStatusOpen,
		StartSpec: &types.StartSpec{
			Agent: "claude-fail",
		},
	}
	store.beads[startBead.ID] = startBead

	ctx := context.Background()
	err := orch.dispatch(ctx, startBead)

	if err == nil {
		t.Error("dispatch() should error when Start fails")
	}
}

func TestHandler_Stop_FailureReturnsError(t *testing.T) {
	store := newMockBeadStore()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Agent manager that fails to stop
	agents := &failingAgentManager{
		failStop: true,
		err:      fmt.Errorf("tmux kill-session failed"),
	}

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	stopBead := &types.Bead{
		ID:     "bd-stop-fail",
		Type:   types.BeadTypeStop,
		Title:  "Failing stop",
		Status: types.BeadStatusOpen,
		StopSpec: &types.StopSpec{
			Agent: "claude-fail",
		},
	}
	store.beads[stopBead.ID] = stopBead

	ctx := context.Background()
	err := orch.dispatch(ctx, stopBead)

	if err == nil {
		t.Error("dispatch() should error when Stop fails")
	}
}

func TestHandler_Expand_FailureReturnsError(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Expander that fails
	expander := &failingExpander{
		err: fmt.Errorf("template not found"),
	}

	cfg := config.Default()
	orch := New(cfg, store, agents, expander, executor, logger)

	expandBead := &types.Bead{
		ID:     "bd-expand-fail",
		Type:   types.BeadTypeExpand,
		Title:  "Failing expand",
		Status: types.BeadStatusOpen,
		ExpandSpec: &types.ExpandSpec{
			Template: "nonexistent-template",
		},
	}
	store.beads[expandBead.ID] = expandBead

	ctx := context.Background()
	err := orch.dispatch(ctx, expandBead)

	if err == nil {
		t.Error("dispatch() should error when Expand fails")
	}
}

// failingAgentManager is an agent manager that fails operations.
type failingAgentManager struct {
	failStart bool
	failStop  bool
	err       error
}

func (m *failingAgentManager) Start(ctx context.Context, spec *types.StartSpec) error {
	if m.failStart {
		return m.err
	}
	return nil
}

func (m *failingAgentManager) Stop(ctx context.Context, spec *types.StopSpec) error {
	if m.failStop {
		return m.err
	}
	return nil
}

func (m *failingAgentManager) IsRunning(ctx context.Context, agentID string) (bool, error) {
	return false, nil
}

// failingExpander is an expander that always fails.
type failingExpander struct {
	err error
}

func (e *failingExpander) Expand(ctx context.Context, spec *types.ExpandSpec, parent *types.Bead) error {
	return e.err
}

// --- Gate Handler Tests ---

func TestOrchestrator_DispatchGate(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add a gate bead
	gateBead := &types.Bead{
		ID:     "bd-gate-001",
		Type:   types.BeadTypeGate,
		Title:  "Approval gate",
		Status: types.BeadStatusOpen,
	}
	store.addReady(gateBead)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run briefly - gate won't auto-close
	_ = orch.Run(ctx)

	// Verify gate was marked in_progress but NOT closed (must be closed by human)
	if store.beads["bd-gate-001"].Status != types.BeadStatusInProgress {
		t.Errorf("Gate status = %s, want in_progress", store.beads["bd-gate-001"].Status)
	}
}

// --- Collaborative Handler Tests ---

func TestOrchestrator_DispatchCollaborative(t *testing.T) {
	store := newMockBeadStore()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond

	orch := New(cfg, store, agents, expander, executor, logger)

	// Add a collaborative bead
	collabBead := &types.Bead{
		ID:       "bd-collab-001",
		Type:     types.BeadTypeCollaborative,
		Title:    "Collaborative task",
		Status:   types.BeadStatusOpen,
		Assignee: "claude-team",
	}
	store.addReady(collabBead)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run briefly - collaborative beads work like tasks
	_ = orch.Run(ctx)

	// Verify collaborative was marked in_progress (like a task)
	if store.beads["bd-collab-001"].Status != types.BeadStatusInProgress {
		t.Errorf("Collaborative status = %s, want in_progress", store.beads["bd-collab-001"].Status)
	}
}

// =============================================================================
// WISP LIFECYCLE MANAGEMENT TESTS
// =============================================================================

// mockBeadStoreWithFilteredListing extends mockBeadStoreWithCreateAndDelete with ListFiltered support.
type mockBeadStoreWithFilteredListing struct {
	*mockBeadStoreWithCreateAndDelete
}

func newMockBeadStoreWithFilteredListing() *mockBeadStoreWithFilteredListing {
	return &mockBeadStoreWithFilteredListing{
		mockBeadStoreWithCreateAndDelete: newMockBeadStoreWithCreateAndDelete(),
	}
}

func (m *mockBeadStoreWithFilteredListing) ListFiltered(ctx context.Context, filter BeadFilter) ([]*types.Bead, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []*types.Bead
	for _, bead := range m.beads {
		if filter.Status != "" && bead.Status != filter.Status {
			continue
		}
		if filter.Tier != "" && bead.Tier != filter.Tier {
			continue
		}
		if filter.Assignee != "" && bead.Assignee != filter.Assignee {
			continue
		}
		if filter.WorkflowID != "" && bead.WorkflowID != filter.WorkflowID {
			continue
		}
		if filter.HookBead != "" && bead.HookBead != filter.HookBead {
			continue
		}
		result = append(result, bead)
	}
	return result, nil
}

func TestOrchestrator_CleanupWorkflow_DeletesWispsAndOrchestrator(t *testing.T) {
	store := newMockBeadStoreWithFilteredListing()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Cleanup.Ephemeral = config.EphemeralCleanupOnComplete

	orch := New(cfg, store, agents, expander, executor, logger)

	workflowID := "wf-test-001"

	// Add a work bead (should NOT be deleted)
	workBead := &types.Bead{
		ID:         "bd-work-001",
		Type:       types.BeadTypeTask,
		Title:      "Work bead",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWork,
		WorkflowID: workflowID,
	}
	store.beads[workBead.ID] = workBead

	// Add wisp beads (should be deleted)
	wispBead1 := &types.Bead{
		ID:         "wisp-step-1",
		Type:       types.BeadTypeTask,
		Title:      "Wisp step 1",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWisp,
		WorkflowID: workflowID,
	}
	store.beads[wispBead1.ID] = wispBead1

	wispBead2 := &types.Bead{
		ID:         "wisp-step-2",
		Type:       types.BeadTypeTask,
		Title:      "Wisp step 2",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWisp,
		WorkflowID: workflowID,
	}
	store.beads[wispBead2.ID] = wispBead2

	// Add orchestrator bead (should be deleted)
	orchBead := &types.Bead{
		ID:         "orch-cond-1",
		Type:       types.BeadTypeCondition,
		Title:      "Orchestrator condition",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierOrchestrator,
		WorkflowID: workflowID,
	}
	store.beads[orchBead.ID] = orchBead

	ctx := context.Background()
	err := orch.cleanupWorkflow(ctx, workflowID)
	if err != nil {
		t.Fatalf("cleanupWorkflow() error = %v", err)
	}

	// Work bead should still exist
	if store.beads["bd-work-001"] == nil {
		t.Error("Work bead should NOT be deleted")
	}

	// Wisp beads should be deleted
	if store.beads["wisp-step-1"] != nil {
		t.Error("Wisp step 1 should be deleted")
	}
	if store.beads["wisp-step-2"] != nil {
		t.Error("Wisp step 2 should be deleted")
	}

	// Orchestrator bead should be deleted
	if store.beads["orch-cond-1"] != nil {
		t.Error("Orchestrator bead should be deleted")
	}
}

func TestOrchestrator_CleanupWorkflow_SkipsIfNotAllClosed(t *testing.T) {
	store := newMockBeadStoreWithFilteredListing()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Cleanup.Ephemeral = config.EphemeralCleanupOnComplete

	orch := New(cfg, store, agents, expander, executor, logger)

	workflowID := "wf-test-002"

	// Add a closed wisp
	wispClosed := &types.Bead{
		ID:         "wisp-closed",
		Type:       types.BeadTypeTask,
		Title:      "Closed wisp",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWisp,
		WorkflowID: workflowID,
	}
	store.beads[wispClosed.ID] = wispClosed

	// Add an in-progress wisp
	wispInProgress := &types.Bead{
		ID:         "wisp-in-progress",
		Type:       types.BeadTypeTask,
		Title:      "In progress wisp",
		Status:     types.BeadStatusInProgress,
		Tier:       types.TierWisp,
		WorkflowID: workflowID,
	}
	store.beads[wispInProgress.ID] = wispInProgress

	ctx := context.Background()
	err := orch.cleanupWorkflow(ctx, workflowID)
	if err != nil {
		t.Fatalf("cleanupWorkflow() error = %v", err)
	}

	// Both beads should still exist (cleanup skipped)
	if store.beads["wisp-closed"] == nil {
		t.Error("Wisp-closed should NOT be deleted when workflow incomplete")
	}
	if store.beads["wisp-in-progress"] == nil {
		t.Error("Wisp-in-progress should NOT be deleted when workflow incomplete")
	}
}

func TestOrchestrator_CleanupWorkflow_RespectsConfigDisabled(t *testing.T) {
	store := newMockBeadStoreWithFilteredListing()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Cleanup.Ephemeral = config.EphemeralCleanupNever // Disabled

	orch := New(cfg, store, agents, expander, executor, logger)

	workflowID := "wf-test-003"

	// Add a wisp bead
	wispBead := &types.Bead{
		ID:         "wisp-no-delete",
		Type:       types.BeadTypeTask,
		Title:      "Should not delete",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWisp,
		WorkflowID: workflowID,
	}
	store.beads[wispBead.ID] = wispBead

	ctx := context.Background()
	err := orch.cleanupWorkflow(ctx, workflowID)
	if err != nil {
		t.Fatalf("cleanupWorkflow() error = %v", err)
	}

	// Wisp should still exist (cleanup disabled)
	if store.beads["wisp-no-delete"] == nil {
		t.Error("Wisp should NOT be deleted when cleanup disabled")
	}
}

func TestOrchestrator_SquashWisps_CreatesDigestAndBurns(t *testing.T) {
	store := newMockBeadStoreWithFilteredListing()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Cleanup.Ephemeral = config.EphemeralCleanupOnComplete

	orch := New(cfg, store, agents, expander, executor, logger)

	workflowID := "wf-squash-001"
	workBeadID := "bd-work-squash"

	// Add the work bead
	closedAt := time.Now()
	workBead := &types.Bead{
		ID:         workBeadID,
		Type:       types.BeadTypeTask,
		Title:      "Implement feature X",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWork,
		WorkflowID: workflowID,
		Notes:      "Original notes",
	}
	store.beads[workBead.ID] = workBead

	// Add wisps
	wisp1 := &types.Bead{
		ID:         "wisp-1",
		Type:       types.BeadTypeTask,
		Title:      "Write tests",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWisp,
		WorkflowID: workflowID,
		CreatedAt:  time.Now().Add(-10 * time.Minute),
		ClosedAt:   &closedAt,
	}
	store.beads[wisp1.ID] = wisp1

	wisp2 := &types.Bead{
		ID:         "wisp-2",
		Type:       types.BeadTypeCode,
		Title:      "Run build",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWisp,
		WorkflowID: workflowID,
		CreatedAt:  time.Now().Add(-5 * time.Minute),
		ClosedAt:   &closedAt,
	}
	store.beads[wisp2.ID] = wisp2

	ctx := context.Background()
	err := orch.squashWisps(ctx, workflowID, workBeadID)
	if err != nil {
		t.Fatalf("squashWisps() error = %v", err)
	}

	// Work bead should have digest appended
	updatedWorkBead := store.beads[workBeadID]
	if updatedWorkBead == nil {
		t.Fatal("Work bead should still exist")
	}

	if updatedWorkBead.Notes == "Original notes" {
		t.Error("Work bead notes should be updated with digest")
	}

	if !strings.Contains(updatedWorkBead.Notes, "Workflow Execution Digest") {
		t.Error("Notes should contain 'Workflow Execution Digest'")
	}

	if !strings.Contains(updatedWorkBead.Notes, "Write tests") {
		t.Error("Notes should contain wisp title 'Write tests'")
	}

	// Wisps should be deleted
	if store.beads["wisp-1"] != nil {
		t.Error("Wisp-1 should be deleted after squash")
	}
	if store.beads["wisp-2"] != nil {
		t.Error("Wisp-2 should be deleted after squash")
	}
}

func TestOrchestrator_GenerateWispDigest(t *testing.T) {
	orch := &Orchestrator{}

	closedAt := time.Now()
	createdAt := time.Now().Add(-15 * time.Minute)

	wisps := []*types.Bead{
		{
			ID:        "wisp-1",
			Type:      types.BeadTypeTask,
			Title:     "First task",
			Status:    types.BeadStatusClosed,
			CreatedAt: createdAt,
			ClosedAt:  &closedAt,
		},
		{
			ID:        "wisp-2",
			Type:      types.BeadTypeTask,
			Title:     "Second task",
			Status:    types.BeadStatusClosed,
			CreatedAt: createdAt.Add(5 * time.Minute),
			ClosedAt:  &closedAt,
		},
		{
			ID:        "wisp-3",
			Type:      types.BeadTypeCode,
			Title:     "Build step",
			Status:    types.BeadStatusClosed,
			CreatedAt: createdAt.Add(10 * time.Minute),
			ClosedAt:  &closedAt,
		},
	}

	digest := orch.generateWispDigest(wisps)

	// Check structure
	if !strings.Contains(digest, "## Workflow Execution Digest") {
		t.Error("Digest should have header")
	}

	if !strings.Contains(digest, "**Total Steps**: 3") {
		t.Error("Digest should have total steps count")
	}

	if !strings.Contains(digest, "**Steps by Type**") {
		t.Error("Digest should have steps by type section")
	}

	if !strings.Contains(digest, "task: 2") {
		t.Error("Digest should show 2 task types")
	}

	if !strings.Contains(digest, "code: 1") {
		t.Error("Digest should show 1 code type")
	}

	if !strings.Contains(digest, "**Steps Executed**") {
		t.Error("Digest should have steps executed section")
	}

	if !strings.Contains(digest, "[closed] First task") {
		t.Error("Digest should list step titles with status")
	}
}

func TestOrchestrator_GenerateWispDigest_EmptyList(t *testing.T) {
	orch := &Orchestrator{}

	digest := orch.generateWispDigest(nil)
	if digest != "" {
		t.Errorf("Digest should be empty for nil wisps, got: %s", digest)
	}

	digest = orch.generateWispDigest([]*types.Bead{})
	if digest != "" {
		t.Errorf("Digest should be empty for empty wisps, got: %s", digest)
	}
}

func TestOrchestrator_CleanupWorkflow_PreservesEmptyTierAsWork(t *testing.T) {
	store := newMockBeadStoreWithFilteredListing()
	agents := newMockAgentManager()
	expander := &mockTemplateExpander{}
	executor := newMockCodeExecutor()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := config.Default()
	cfg.Cleanup.Ephemeral = config.EphemeralCleanupOnComplete

	orch := New(cfg, store, agents, expander, executor, logger)

	workflowID := "wf-empty-tier"

	// Add a bead with empty tier (should be treated as work and NOT deleted)
	emptyTierBead := &types.Bead{
		ID:         "bd-empty-tier",
		Type:       types.BeadTypeTask,
		Title:      "Bead with empty tier",
		Status:     types.BeadStatusClosed,
		Tier:       "", // Empty tier should default to work behavior
		WorkflowID: workflowID,
	}
	store.beads[emptyTierBead.ID] = emptyTierBead

	// Add a wisp (should be deleted)
	wispBead := &types.Bead{
		ID:         "wisp-to-delete",
		Type:       types.BeadTypeTask,
		Title:      "Wisp to delete",
		Status:     types.BeadStatusClosed,
		Tier:       types.TierWisp,
		WorkflowID: workflowID,
	}
	store.beads[wispBead.ID] = wispBead

	ctx := context.Background()
	err := orch.cleanupWorkflow(ctx, workflowID)
	if err != nil {
		t.Fatalf("cleanupWorkflow() error = %v", err)
	}

	// Empty tier bead should still exist
	if store.beads["bd-empty-tier"] == nil {
		t.Error("Bead with empty tier should NOT be deleted (treated as work)")
	}

	// Wisp should be deleted
	if store.beads["wisp-to-delete"] != nil {
		t.Error("Wisp should be deleted")
	}
}
