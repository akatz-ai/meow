package orchestrator

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
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
