package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/akatz-ai/meow/internal/config"
	"github.com/akatz-ai/meow/internal/ipc"
	"github.com/akatz-ai/meow/internal/types"
)

// --- Mock Implementations ---

// mockRunStore implements RunStore for testing.
type mockRunStore struct {
	mu        sync.Mutex
	workflows map[string]*types.Run
	calls     []string
}

func newMockRunStore() *mockRunStore {
	return &mockRunStore{
		workflows: make(map[string]*types.Run),
	}
}

func (m *mockRunStore) Create(ctx context.Context, wf *types.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Create:"+wf.ID)
	m.workflows[wf.ID] = wf
	return nil
}

func (m *mockRunStore) Get(ctx context.Context, id string) (*types.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Get:"+id)
	return m.workflows[id], nil
}

func (m *mockRunStore) Save(ctx context.Context, wf *types.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Save:"+wf.ID)
	m.workflows[wf.ID] = wf
	return nil
}

func (m *mockRunStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Delete:"+id)
	delete(m.workflows, id)
	return nil
}

func (m *mockRunStore) List(ctx context.Context, filter RunFilter) ([]*types.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "List")
	var result []*types.Run
	for _, wf := range m.workflows {
		if filter.Status != "" && wf.Status != filter.Status {
			continue
		}
		result = append(result, wf)
	}
	return result, nil
}

func (m *mockRunStore) GetByAgent(ctx context.Context, agentID string) ([]*types.Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "GetByAgent:"+agentID)
	var result []*types.Run
	for _, wf := range m.workflows {
		for _, step := range wf.Steps {
			if step.Agent != nil && step.Agent.Agent == agentID {
				result = append(result, wf)
				break
			}
		}
	}
	return result, nil
}

// mockAgentManager implements AgentManager for testing.
// injectedPromptRecord captures details of an InjectPrompt call.
type injectedPromptRecord struct {
	AgentID   string
	Prompt    string
	Stabilize bool
}

type mockAgentManager struct {
	mu              sync.Mutex
	running         map[string]bool
	started         []string
	stopped         []string
	interrupted     []string
	injectedPrompts []string
	// injections tracks all InjectPrompt calls with their options
	injections []injectedPromptRecord
	// injectErr if set, InjectPrompt returns this error
	injectErr error
}

func newMockAgentManager() *mockAgentManager {
	return &mockAgentManager{
		running: make(map[string]bool),
	}
}

func (m *mockAgentManager) Start(ctx context.Context, wf *types.Run, step *types.Step) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	agentID := step.Spawn.Agent
	m.started = append(m.started, agentID)
	m.running[agentID] = true
	return nil
}

func (m *mockAgentManager) Stop(ctx context.Context, wf *types.Run, step *types.Step) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	agentID := step.Kill.Agent
	m.stopped = append(m.stopped, agentID)
	m.running[agentID] = false
	return nil
}

func (m *mockAgentManager) IsRunning(ctx context.Context, agentID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[agentID], nil
}

func (m *mockAgentManager) InjectPrompt(ctx context.Context, agentID string, prompt string, opts InjectPromptOpts) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.injectedPrompts = append(m.injectedPrompts, agentID+":"+prompt)
	m.injections = append(m.injections, injectedPromptRecord{
		AgentID:   agentID,
		Prompt:    prompt,
		Stabilize: opts.Stabilize,
	})
	if m.injectErr != nil {
		return m.injectErr
	}
	return nil
}

// GetInjections returns a copy of all injection records.
func (m *mockAgentManager) GetInjections() []injectedPromptRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]injectedPromptRecord, len(m.injections))
	copy(result, m.injections)
	return result
}

func (m *mockAgentManager) Interrupt(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interrupted = append(m.interrupted, agentID)
	return nil
}

func (m *mockAgentManager) KillAll(ctx context.Context, wf *types.Run) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for agentID := range m.running {
		m.stopped = append(m.stopped, agentID)
		m.running[agentID] = false
	}
	return nil
}

// mockShellRunner implements ShellRunner for testing.
type mockShellRunner struct {
	mu       sync.Mutex
	executed []string
	results  map[string]map[string]any
	errors   map[string]error
}

func newMockShellRunner() *mockShellRunner {
	return &mockShellRunner{
		results: make(map[string]map[string]any),
		errors:  make(map[string]error),
	}
}

func (m *mockShellRunner) Run(ctx context.Context, cfg *types.ShellConfig) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.executed = append(m.executed, cfg.Command)
	if err, ok := m.errors[cfg.Command]; ok {
		return nil, err
	}
	if result, ok := m.results[cfg.Command]; ok {
		return result, nil
	}
	return map[string]any{"exit_code": 0}, nil
}

// mockTemplateExpander implements TemplateExpander for testing.
type mockTemplateExpander struct {
	mu       sync.Mutex
	expanded []string
}

func (m *mockTemplateExpander) Expand(ctx context.Context, wf *types.Run, step *types.Step) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expanded = append(m.expanded, step.Expand.Template)
	return nil
}

// --- Helper Functions ---

func testConfig() *config.Config {
	cfg := config.Default()
	cfg.Orchestrator.PollInterval = 10 * time.Millisecond
	return cfg
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

// --- Tests ---

func TestOrchestrator_NoWorkflows_CompletesImmediately(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}
}

func TestOrchestrator_SingleWorkflow_CompletesAllSteps(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create a workflow with a single shell step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["step-1"] = &types.Step{
		ID:       "step-1",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell: &types.ShellConfig{
			Command: "echo hello",
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Re-fetch workflow to get latest state (async updates)
	wf, _ = store.Get(ctx, wf.ID)

	// Check step is done (shell now executes via async branch)
	if wf.Steps["step-1"].Status != types.StepStatusDone {
		t.Errorf("Step status = %v, want %v", wf.Steps["step-1"].Status, types.StepStatusDone)
	}

	// Check workflow is done
	if wf.Status != types.RunStatusDone {
		t.Errorf("Workflow status = %v, want %v", wf.Status, types.RunStatusDone)
	}
}

func TestOrchestrator_DependencyOrdering(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create a workflow with dependent steps
	// step-2 depends on step-1
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["step-1"] = &types.Step{
		ID:       "step-1",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell:    &types.ShellConfig{Command: "echo first"},
	}
	wf.Steps["step-2"] = &types.Step{
		ID:       "step-2",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Needs:    []string{"step-1"},
		Shell:    &types.ShellConfig{Command: "echo second"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Re-fetch workflow to get latest state (async updates)
	wf, _ = store.Get(ctx, wf.ID)

	// Verify both steps completed (step-2 depends on step-1, so ordering is enforced)
	if wf.Steps["step-1"].Status != types.StepStatusDone {
		t.Errorf("step-1 status = %v, want %v", wf.Steps["step-1"].Status, types.StepStatusDone)
	}
	if wf.Steps["step-2"].Status != types.StepStatusDone {
		t.Errorf("step-2 status = %v, want %v", wf.Steps["step-2"].Status, types.StepStatusDone)
	}
	// Dependency ordering is enforced by DAG - step-2 can't run until step-1 is done
}

func TestOrchestrator_Dispatch_SixExecutors(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	tests := []struct {
		name     string
		executor types.ExecutorType
		step     *types.Step
	}{
		{
			name:     "shell",
			executor: types.ExecutorShell,
			step: &types.Step{
				ID:       "shell-step",
				Executor: types.ExecutorShell,
				Status:   types.StepStatusPending,
				Shell:    &types.ShellConfig{Command: "echo test"},
			},
		},
		{
			name:     "spawn",
			executor: types.ExecutorSpawn,
			step: &types.Step{
				ID:       "spawn-step",
				Executor: types.ExecutorSpawn,
				Status:   types.StepStatusPending,
				Spawn:    &types.SpawnConfig{Agent: "test-agent"},
			},
		},
		{
			name:     "kill",
			executor: types.ExecutorKill,
			step: &types.Step{
				ID:       "kill-step",
				Executor: types.ExecutorKill,
				Status:   types.StepStatusPending,
				Kill:     &types.KillConfig{Agent: "test-agent"},
			},
		},
		{
			name:     "expand",
			executor: types.ExecutorExpand,
			step: &types.Step{
				ID:       "expand-step",
				Executor: types.ExecutorExpand,
				Status:   types.StepStatusPending,
				Expand:   &types.ExpandConfig{Template: "test-template"},
			},
		},
		{
			name:     "branch",
			executor: types.ExecutorBranch,
			step: &types.Step{
				ID:       "branch-step",
				Executor: types.ExecutorBranch,
				Status:   types.StepStatusPending,
				Branch:   &types.BranchConfig{Condition: "true"},
			},
		},
		{
			name:     "agent",
			executor: types.ExecutorAgent,
			step: &types.Step{
				ID:       "agent-step",
				Executor: types.ExecutorAgent,
				Status:   types.StepStatusPending,
				Agent:    &types.AgentConfig{Agent: "test-agent", Prompt: "Do something"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := orch.dispatch(context.Background(), wf, tc.step)
			// All executors should work now
			if err != nil {
				t.Errorf("dispatch(%s) error = %v", tc.executor, err)
			}
		})
	}
}

func TestOrchestrator_OrchestratorExecutorsFirst(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with both orchestrator and agent steps ready
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Add steps - agent step first (alphabetically), then shell step
	wf.Steps["agent-step"] = &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusPending,
		Agent:    &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	wf.Steps["shell-step"] = &types.Step{
		ID:       "shell-step",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell:    &types.ShellConfig{Command: "echo first"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Process one tick
	ctx := context.Background()
	err := orch.processWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("processWorkflow error = %v", err)
	}

	// Give async shell execution a moment to complete
	time.Sleep(100 * time.Millisecond)
	orch.wg.Wait() // Wait for async goroutines

	// Re-fetch workflow to get latest state
	wf, _ = store.Get(ctx, wf.ID)

	// Shell step should be running or done (async execution)
	shellStatus := wf.Steps["shell-step"].Status
	if shellStatus != types.StepStatusRunning && shellStatus != types.StepStatusDone {
		t.Errorf("Shell step status = %v, want running or done", shellStatus)
	}

	// Agent step should be running (waiting for meow done)
	if wf.Steps["agent-step"].Status != types.StepStatusRunning {
		t.Errorf("Agent step status = %v, want %v", wf.Steps["agent-step"].Status, types.StepStatusRunning)
	}
}

func TestOrchestrator_AgentIdleCheck(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with two agent steps for the same agent
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	wf.Steps["agent-step-1"] = &types.Step{
		ID:       "agent-step-1",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusPending,
		Agent:    &types.AgentConfig{Agent: "test-agent", Prompt: "First task"},
	}
	wf.Steps["agent-step-2"] = &types.Step{
		ID:       "agent-step-2",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusPending,
		Agent:    &types.AgentConfig{Agent: "test-agent", Prompt: "Second task"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Process one tick - only one agent step should be dispatched
	ctx := context.Background()
	err := orch.processWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("processWorkflow error = %v", err)
	}

	// Only one step should be running (agent is busy)
	runningCount := 0
	for _, step := range wf.Steps {
		if step.Status == types.StepStatusRunning {
			runningCount++
		}
	}
	if runningCount != 1 {
		t.Errorf("Expected 1 running step, got %d", runningCount)
	}

	// Only one prompt should have been injected
	if len(agents.injectedPrompts) != 1 {
		t.Errorf("Expected 1 injected prompt, got %d", len(agents.injectedPrompts))
	}
}

func TestOrchestrator_HandleStepDone(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a running agent step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	now := time.Now()
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusRunning,
		StartedAt: &now,
		Agent:     &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Simulate meow done message
	msg := &ipc.StepDoneMessage{
		Type:     ipc.MsgStepDone,
		Workflow: wf.ID,
		Agent:    "test-agent",
		Step:     "agent-step",
		Outputs:  map[string]any{"result": "success"},
	}

	ctx := context.Background()
	err := orch.HandleStepDone(ctx, msg)
	if err != nil {
		t.Fatalf("HandleStepDone error = %v", err)
	}

	// Step should now be done
	if wf.Steps["agent-step"].Status != types.StepStatusDone {
		t.Errorf("Step status = %v, want %v", wf.Steps["agent-step"].Status, types.StepStatusDone)
	}

	// Outputs should be captured
	if wf.Steps["agent-step"].Outputs["result"] != "success" {
		t.Errorf("Step outputs = %v, want result=success", wf.Steps["agent-step"].Outputs)
	}
}

func TestOrchestrator_OutputValidation(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create a temp file for file_path validation
	tmpFile, err := os.CreateTemp("", "test-output-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	tmpFile.Close()
	defer os.Remove(tmpFile.Name())

	// Create workflow with agent step requiring outputs
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	now := time.Now()
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusRunning,
		StartedAt: &now,
		Agent: &types.AgentConfig{
			Agent:  "test-agent",
			Prompt: "Do work",
			Outputs: map[string]types.AgentOutputDef{
				"file_path": {Required: true, Type: "file_path"},
				"count":     {Required: false, Type: "number"},
			},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Test missing required output
	msg := &ipc.StepDoneMessage{
		Type:     ipc.MsgStepDone,
		Workflow: wf.ID,
		Agent:    "test-agent",
		Step:     "agent-step",
		Outputs:  map[string]any{"count": 42}, // Missing required file_path
	}

	ctx := context.Background()
	err = orch.HandleStepDone(ctx, msg)
	// Validation failure returns error now
	if err == nil {
		t.Fatal("Expected validation error for missing required output")
	}

	// Step should still be running (validation failed)
	step, _ := store.workflows[wf.ID].GetStep("agent-step")
	if step.Status != types.StepStatusRunning {
		t.Errorf("Step status = %v, want %v (validation should fail)", step.Status, types.StepStatusRunning)
	}

	// Now provide valid outputs with actual existing file
	msg.Outputs = map[string]any{
		"file_path": tmpFile.Name(),
		"count":     42,
	}

	err = orch.HandleStepDone(ctx, msg)
	if err != nil {
		t.Fatalf("HandleStepDone error = %v", err)
	}

	// Step should now be done
	step, _ = store.workflows[wf.ID].GetStep("agent-step")
	if step.Status != types.StepStatusDone {
		t.Errorf("Step status = %v, want %v", step.Status, types.StepStatusDone)
	}
}

func TestOrchestrator_WorkflowCompletion(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow that will complete
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["step-1"] = &types.Step{
		ID:       "step-1",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusDone,
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Process - should complete workflow
	ctx := context.Background()
	err := orch.processWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("processWorkflow error = %v", err)
	}

	if wf.Status != types.RunStatusDone {
		t.Errorf("Workflow status = %v, want %v", wf.Status, types.RunStatusDone)
	}
	if wf.DoneAt == nil {
		t.Error("Workflow DoneAt should be set")
	}
}

func TestOrchestrator_WorkflowFailure(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a failed step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["step-1"] = &types.Step{
		ID:       "step-1",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusFailed,
		Error:    &types.StepError{Message: "command failed"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Process - should fail workflow
	ctx := context.Background()
	err := orch.processWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("processWorkflow error = %v", err)
	}

	if wf.Status != types.RunStatusFailed {
		t.Errorf("Workflow status = %v, want %v", wf.Status, types.RunStatusFailed)
	}
}

func TestOrchestrator_SpawnAndKill(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with spawn -> kill sequence
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["spawn-step"] = &types.Step{
		ID:       "spawn-step",
		Executor: types.ExecutorSpawn,
		Status:   types.StepStatusPending,
		Spawn:    &types.SpawnConfig{Agent: "test-agent", Workdir: "/tmp"},
	}
	wf.Steps["kill-step"] = &types.Step{
		ID:       "kill-step",
		Executor: types.ExecutorKill,
		Status:   types.StepStatusPending,
		Needs:    []string{"spawn-step"},
		Kill:     &types.KillConfig{Agent: "test-agent"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}

	// Agent should have been started then stopped
	if len(agents.started) != 1 || agents.started[0] != "test-agent" {
		t.Errorf("Expected agent to be started, got %v", agents.started)
	}
	if len(agents.stopped) != 1 || agents.stopped[0] != "test-agent" {
		t.Errorf("Expected agent to be stopped, got %v", agents.stopped)
	}

	// Both steps should be done
	if wf.Steps["spawn-step"].Status != types.StepStatusDone {
		t.Errorf("Spawn step status = %v, want done", wf.Steps["spawn-step"].Status)
	}
	if wf.Steps["kill-step"].Status != types.StepStatusDone {
		t.Errorf("Kill step status = %v, want done", wf.Steps["kill-step"].Status)
	}
}

// TestOrchestrator_ConcurrentStepCompletion tests that multiple concurrent
// HandleStepDone calls do not result in lost updates. This is the critical test
// for the race condition fix (meow-ilr).
func TestOrchestrator_ConcurrentStepCompletion(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with 3 running agent steps (each assigned to different agent)
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	now := time.Now()

	for i := 1; i <= 3; i++ {
		stepID := "agent-step-" + string(rune('0'+i))
		agentID := "agent-" + string(rune('0'+i))
		wf.Steps[stepID] = &types.Step{
			ID:        stepID,
			Executor:  types.ExecutorAgent,
			Status:    types.StepStatusRunning,
			StartedAt: &now,
			Agent:     &types.AgentConfig{Agent: agentID, Prompt: "Work"},
		}
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()

	// Launch 3 goroutines calling HandleStepDone simultaneously
	var wg sync.WaitGroup
	errors := make(chan error, 3)

	for i := 1; i <= 3; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			stepID := "agent-step-" + string(rune('0'+idx))
			agentID := "agent-" + string(rune('0'+idx))
			msg := &ipc.StepDoneMessage{
				Type:     ipc.MsgStepDone,
				Workflow: wf.ID,
				Agent:    agentID,
				Step:     stepID,
				Outputs:  map[string]any{"result": idx},
			}
			if err := orch.HandleStepDone(ctx, msg); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// Check for any errors
	for err := range errors {
		t.Errorf("HandleStepDone error: %v", err)
	}

	// Reload workflow to get final state
	finalWf, _ := store.Get(ctx, wf.ID)

	// Verify all 3 completions are recorded (no lost updates)
	doneCount := 0
	for stepID, step := range finalWf.Steps {
		if step.Status == types.StepStatusDone {
			doneCount++
			// Verify outputs are captured correctly
			if step.Outputs == nil {
				t.Errorf("Step %s has no outputs", stepID)
			}
		}
	}

	if doneCount != 3 {
		t.Errorf("Expected 3 completed steps, got %d (lost updates detected!)", doneCount)
		for stepID, step := range finalWf.Steps {
			t.Logf("  %s: status=%s", stepID, step.Status)
		}
	}
}

// TestOrchestrator_StepTimeout tests that agent steps are interrupted and failed after timeout.
func TestOrchestrator_StepTimeout(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a running agent step that has a very short timeout
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Set started time in the past to simulate elapsed time
	startedAt := time.Now().Add(-2 * time.Second) // Started 2 seconds ago
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusRunning,
		StartedAt: &startedAt,
		Agent: &types.AgentConfig{
			Agent:   "test-agent",
			Prompt:  "Do work",
			Timeout: "1s", // 1 second timeout (already exceeded)
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// First call - should send interrupt
	ctx := context.Background()
	orch.checkStepTimeouts(ctx, wf)

	// Agent should have been interrupted
	if len(agents.interrupted) != 1 || agents.interrupted[0] != "test-agent" {
		t.Errorf("Expected agent to be interrupted, got %v", agents.interrupted)
	}

	// Step should still be running (grace period not elapsed)
	if wf.Steps["agent-step"].Status != types.StepStatusRunning {
		t.Errorf("Step status = %v, want running (grace period not elapsed)", wf.Steps["agent-step"].Status)
	}

	// Step should have InterruptedAt set
	if wf.Steps["agent-step"].InterruptedAt == nil {
		t.Error("Step InterruptedAt should be set")
	}

	// Simulate grace period elapsed by setting InterruptedAt in the past
	interruptedAt := time.Now().Add(-11 * time.Second) // 11 seconds ago
	wf.Steps["agent-step"].InterruptedAt = &interruptedAt

	// Second call - should fail the step
	orch.checkStepTimeouts(ctx, wf)

	// Step should now be failed
	if wf.Steps["agent-step"].Status != types.StepStatusFailed {
		t.Errorf("Step status = %v, want failed", wf.Steps["agent-step"].Status)
	}

	// Check error message
	if wf.Steps["agent-step"].Error == nil {
		t.Error("Step error should be set")
	} else if wf.Steps["agent-step"].Error.Message == "" {
		t.Error("Step error message should not be empty")
	}
}

// TestOrchestrator_StepNoTimeoutIfCompleted tests that steps that complete before timeout are not affected.
func TestOrchestrator_StepNoTimeoutIfCompleted(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a completed agent step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	startedAt := time.Now().Add(-2 * time.Second)
	doneAt := time.Now().Add(-1 * time.Second)
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusDone,
		StartedAt: &startedAt,
		DoneAt:    &doneAt,
		Agent: &types.AgentConfig{
			Agent:   "test-agent",
			Prompt:  "Do work",
			Timeout: "1s",
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	orch.checkStepTimeouts(ctx, wf)

	// Agent should NOT have been interrupted (step already done)
	if len(agents.interrupted) != 0 {
		t.Errorf("Expected no interrupts, got %v", agents.interrupted)
	}

	// Step should still be done
	if wf.Steps["agent-step"].Status != types.StepStatusDone {
		t.Errorf("Step status = %v, want done", wf.Steps["agent-step"].Status)
	}
}

// TestOrchestrator_CleanupOnCompletion tests that cleanup runs when workflow completes.
func TestOrchestrator_CleanupOnCompletion(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with cleanup_on_success script (opt-in cleanup)
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.CleanupOnSuccess = "echo cleanup executed"
	wf.Steps["step-1"] = &types.Step{
		ID:       "step-1",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell:    &types.ShellConfig{Command: "echo hello"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Errorf("Run() error = %v, want nil", err)
	}

	// Workflow should be done (cleanup was successful)
	finalWf, _ := store.Get(ctx, wf.ID)
	if finalWf.Status != types.RunStatusDone {
		t.Errorf("Workflow status = %v, want done", finalWf.Status)
	}
}

// TestOrchestrator_RunCleanup tests the RunCleanup method directly.
func TestOrchestrator_RunCleanup(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with cleanup_on_success script (testing RunCleanup with Done reason)
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.CleanupOnSuccess = "echo cleanup"
	store.workflows[wf.ID] = wf

	// Register some agents
	agents.running["agent-1"] = true
	agents.running["agent-2"] = true

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.RunCleanup(ctx, wf, types.RunStatusDone)
	if err != nil {
		t.Fatalf("RunCleanup error = %v", err)
	}

	// Workflow should be in done state
	if wf.Status != types.RunStatusDone {
		t.Errorf("Workflow status = %v, want done", wf.Status)
	}

	// DoneAt should be set
	if wf.DoneAt == nil {
		t.Error("Workflow DoneAt should be set")
	}

	// All agents should have been killed
	for agentID, running := range agents.running {
		if running {
			t.Errorf("Agent %s should have been killed", agentID)
		}
	}
}

// TestOrchestrator_RunCleanup_FailedWorkflow tests cleanup for failed workflows.
func TestOrchestrator_RunCleanup_FailedWorkflow(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.CleanupOnFailure = "echo cleanup"
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.RunCleanup(ctx, wf, types.RunStatusFailed)
	if err != nil {
		t.Fatalf("RunCleanup error = %v", err)
	}

	// Workflow should be in failed state (final status matches reason)
	if wf.Status != types.RunStatusFailed {
		t.Errorf("Workflow status = %v, want failed", wf.Status)
	}
}

// TestOrchestrator_RunCleanup_Stopped tests cleanup for stopped workflows.
func TestOrchestrator_RunCleanup_Stopped(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.CleanupOnStop = "echo cleanup"
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.RunCleanup(ctx, wf, types.RunStatusStopped)
	if err != nil {
		t.Fatalf("RunCleanup error = %v", err)
	}

	// Workflow should be in stopped state
	if wf.Status != types.RunStatusStopped {
		t.Errorf("Workflow status = %v, want stopped", wf.Status)
	}
}

// --- Crash Recovery Tests ---

// TestOrchestrator_Recover_ResetOrchestratorSteps tests that running orchestrator steps are reset to pending.
func TestOrchestrator_Recover_ResetOrchestratorSteps(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with running orchestrator steps
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	now := time.Now()
	wf.Steps["shell-step"] = &types.Step{
		ID:        "shell-step",
		Executor:  types.ExecutorShell,
		Status:    types.StepStatusRunning,
		StartedAt: &now,
		Shell:     &types.ShellConfig{Command: "echo hello"},
	}
	wf.Steps["branch-step"] = &types.Step{
		ID:        "branch-step",
		Executor:  types.ExecutorBranch,
		Status:    types.StepStatusCompleting, // Completing should also be reset
		StartedAt: &now,
		Branch:    &types.BranchConfig{Condition: "true"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover error = %v", err)
	}

	// Both steps should be reset to pending
	if wf.Steps["shell-step"].Status != types.StepStatusPending {
		t.Errorf("Shell step status = %v, want pending", wf.Steps["shell-step"].Status)
	}
	if wf.Steps["branch-step"].Status != types.StepStatusPending {
		t.Errorf("Branch step status = %v, want pending", wf.Steps["branch-step"].Status)
	}

	// StartedAt should be cleared
	if wf.Steps["shell-step"].StartedAt != nil {
		t.Error("Shell step StartedAt should be cleared")
	}
}

// TestOrchestrator_Recover_PartialExpansion tests cleanup of partial expansions.
func TestOrchestrator_Recover_PartialExpansion(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with partial expansion
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	now := time.Now()
	// Expand step that was running when crash occurred
	wf.Steps["expand-step"] = &types.Step{
		ID:           "expand-step",
		Executor:     types.ExecutorExpand,
		Status:       types.StepStatusRunning,
		StartedAt:    &now,
		Expand:       &types.ExpandConfig{Template: "some-template"},
		ExpandedInto: []string{"expand-step.child-1", "expand-step.child-2"},
	}
	// Partially expanded children
	wf.Steps["expand-step.child-1"] = &types.Step{
		ID:           "expand-step.child-1",
		Executor:     types.ExecutorShell,
		Status:       types.StepStatusPending,
		ExpandedFrom: "expand-step",
		Shell:        &types.ShellConfig{Command: "echo 1"},
	}
	wf.Steps["expand-step.child-2"] = &types.Step{
		ID:           "expand-step.child-2",
		Executor:     types.ExecutorShell,
		Status:       types.StepStatusPending,
		ExpandedFrom: "expand-step",
		Shell:        &types.ShellConfig{Command: "echo 2"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover error = %v", err)
	}

	// Expand step should be reset to pending
	if wf.Steps["expand-step"].Status != types.StepStatusPending {
		t.Errorf("Expand step status = %v, want pending", wf.Steps["expand-step"].Status)
	}

	// ExpandedInto should be cleared
	if len(wf.Steps["expand-step"].ExpandedInto) != 0 {
		t.Errorf("Expand step ExpandedInto should be cleared, got %v", wf.Steps["expand-step"].ExpandedInto)
	}

	// Child steps should be deleted
	if _, ok := wf.Steps["expand-step.child-1"]; ok {
		t.Error("Child step 1 should be deleted")
	}
	if _, ok := wf.Steps["expand-step.child-2"]; ok {
		t.Error("Child step 2 should be deleted")
	}
}

// TestOrchestrator_Recover_AgentStepDeadAgent tests resetting agent steps when agent is dead.
func TestOrchestrator_Recover_AgentStepDeadAgent(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with running agent step, but agent is dead
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	now := time.Now()
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusRunning,
		StartedAt: &now,
		Agent:     &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	store.workflows[wf.ID] = wf

	// Agent is NOT running (dead)
	agents.running["test-agent"] = false

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover error = %v", err)
	}

	// Step should be reset to pending
	if wf.Steps["agent-step"].Status != types.StepStatusPending {
		t.Errorf("Agent step status = %v, want pending", wf.Steps["agent-step"].Status)
	}
}

// TestOrchestrator_Recover_AgentStepAliveAgent tests keeping agent steps running when agent is alive.
func TestOrchestrator_Recover_AgentStepAliveAgent(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with running agent step, agent is alive
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	now := time.Now()
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusRunning,
		StartedAt: &now,
		Agent:     &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	store.workflows[wf.ID] = wf

	// Agent IS running (alive)
	agents.running["test-agent"] = true

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover error = %v", err)
	}

	// Step should remain running
	if wf.Steps["agent-step"].Status != types.StepStatusRunning {
		t.Errorf("Agent step status = %v, want running", wf.Steps["agent-step"].Status)
	}

	// StartedAt should NOT be cleared
	if wf.Steps["agent-step"].StartedAt == nil {
		t.Error("Agent step StartedAt should NOT be cleared")
	}
}

// TestOrchestrator_Recover_AgentStepCompletingToRunning tests that completing agent steps revert to running.
func TestOrchestrator_Recover_AgentStepCompletingToRunning(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with completing agent step, agent is alive
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	now := time.Now()
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusCompleting,
		StartedAt: &now,
		Agent:     &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	store.workflows[wf.ID] = wf

	// Agent IS running (alive)
	agents.running["test-agent"] = true

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover error = %v", err)
	}

	// Step should be reverted to running (from completing)
	if wf.Steps["agent-step"].Status != types.StepStatusRunning {
		t.Errorf("Agent step status = %v, want running", wf.Steps["agent-step"].Status)
	}
}

// TestOrchestrator_Recover_CleaningUpWorkflow tests resuming cleanup for workflows in cleaning_up state.
func TestOrchestrator_Recover_CleaningUpWorkflow(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow that was in cleaning_up state (prior status determines which cleanup script to use)
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusCleaningUp
	wf.PriorStatus = types.RunStatusDone
	wf.CleanupOnSuccess = "echo cleanup"
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover error = %v", err)
	}

	// Workflow should now be in final state (done)
	finalWf, _ := store.Get(ctx, wf.ID)
	if finalWf.Status != types.RunStatusDone {
		t.Errorf("Workflow status = %v, want done", finalWf.Status)
	}

	// DoneAt should be set
	if finalWf.DoneAt == nil {
		t.Error("Workflow DoneAt should be set")
	}
}

// TestOrchestrator_Recover_NoWorkflows tests recovery with no workflows.
func TestOrchestrator_Recover_NoWorkflows(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.Recover(ctx)
	if err != nil {
		t.Fatalf("Recover error = %v", err)
	}

	// Should complete without error
}

// TestOrchestrator_RunCleanup_NoCleanupScript tests cleanup without a script.
func TestOrchestrator_RunCleanup_NoCleanupScript(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	// No cleanup script
	store.workflows[wf.ID] = wf

	agents.running["agent-1"] = true

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.RunCleanup(ctx, wf, types.RunStatusDone)
	if err != nil {
		t.Fatalf("RunCleanup error = %v", err)
	}

	// Workflow should be done
	if wf.Status != types.RunStatusDone {
		t.Errorf("Workflow status = %v, want done", wf.Status)
	}

	// Agent should still have been killed (KillAll called regardless of script)
	for agentID, running := range agents.running {
		if running {
			t.Errorf("Agent %s should have been killed", agentID)
		}
	}
}

// TestOrchestrator_StepNoTimeoutIfNoTimeoutConfigured tests that steps without timeout are not affected.
func TestOrchestrator_StepNoTimeoutIfNoTimeoutConfigured(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a running agent step WITHOUT timeout
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	startedAt := time.Now().Add(-1 * time.Hour) // Running for an hour
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusRunning,
		StartedAt: &startedAt,
		Agent: &types.AgentConfig{
			Agent:  "test-agent",
			Prompt: "Do work",
			// No Timeout set
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	orch.checkStepTimeouts(ctx, wf)

	// Agent should NOT have been interrupted (no timeout configured)
	if len(agents.interrupted) != 0 {
		t.Errorf("Expected no interrupts, got %v", agents.interrupted)
	}

	// Step should still be running
	if wf.Steps["agent-step"].Status != types.StepStatusRunning {
		t.Errorf("Step status = %v, want running", wf.Steps["agent-step"].Status)
	}
}

// TestOrchestrator_ConcurrentStepCompletionWithOrchTick tests that HandleStepDone
// and the orchestrator tick don't race with each other.
func TestOrchestrator_ConcurrentStepCompletionWithOrchTick(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a mix of pending and running steps
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	now := time.Now()

	// 2 running agent steps
	for i := 1; i <= 2; i++ {
		stepID := "running-" + string(rune('0'+i))
		agentID := "agent-" + string(rune('0'+i))
		wf.Steps[stepID] = &types.Step{
			ID:        stepID,
			Executor:  types.ExecutorAgent,
			Status:    types.StepStatusRunning,
			StartedAt: &now,
			Agent:     &types.AgentConfig{Agent: agentID, Prompt: "Work"},
		}
	}

	// 2 pending shell steps
	for i := 1; i <= 2; i++ {
		stepID := "pending-" + string(rune('0'+i))
		wf.Steps[stepID] = &types.Step{
			ID:       stepID,
			Executor: types.ExecutorShell,
			Status:   types.StepStatusPending,
			Shell:    &types.ShellConfig{Command: "echo " + stepID},
		}
	}

	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Start orchestrator in background
	orchDone := make(chan error, 1)
	go func() {
		orchDone <- orch.Run(ctx)
	}()

	// Give orchestrator time to process pending steps
	time.Sleep(50 * time.Millisecond)

	// Concurrently complete the agent steps while orchestrator is running
	var wg sync.WaitGroup
	for i := 1; i <= 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			stepID := "running-" + string(rune('0'+idx))
			agentID := "agent-" + string(rune('0'+idx))
			msg := &ipc.StepDoneMessage{
				Type:     ipc.MsgStepDone,
				Workflow: wf.ID,
				Agent:    agentID,
				Step:     stepID,
				Outputs:  map[string]any{"done": true},
			}
			if err := orch.HandleStepDone(ctx, msg); err != nil {
				t.Errorf("HandleStepDone error for %s: %v", stepID, err)
			}
		}(i)
	}

	wg.Wait()

	// Wait for orchestrator to complete
	select {
	case err := <-orchDone:
		if err != nil && err != context.Canceled {
			t.Errorf("Orchestrator error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Orchestrator did not complete in time")
	}

	// Verify final state - all steps should be done
	finalWf, _ := store.Get(ctx, wf.ID)
	for stepID, step := range finalWf.Steps {
		if step.Status != types.StepStatusDone {
			t.Errorf("Step %s status = %v, want done", stepID, step.Status)
		}
	}
}

// TestOrchestrator_BranchWaitsForChildren tests that a step depending on a branch
// step does NOT become ready until all the branch's expanded children are complete.
// This is a regression test for the bug where branch steps marked themselves as
// "done" immediately after expanding, causing dependent steps to run prematurely.
func TestOrchestrator_BranchWaitsForChildren(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with:
	// - A branch step that has expanded into child steps (simulating post-expansion state)
	// - A "done" step that depends on the branch step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	now := time.Now()

	// Branch step is "running" with expanded children.
	// This simulates the correct state AFTER handleBranch expands the target.
	// The bug is that handleBranch currently marks this as "done" instead of "running".
	wf.Steps["branch-step"] = &types.Step{
		ID:           "branch-step",
		Executor:     types.ExecutorBranch,
		Status:       types.StepStatusRunning,
		StartedAt:    &now,
		ExpandedInto: []string{"branch-step.child-1", "branch-step.child-2"},
		Branch: &types.BranchConfig{
			Condition: "true",
			OnTrue: &types.BranchTarget{
				Template: ".on-true",
			},
		},
	}

	// Child steps are still pending (not completed yet)
	wf.Steps["branch-step.child-1"] = &types.Step{
		ID:           "branch-step.child-1",
		Executor:     types.ExecutorShell,
		Status:       types.StepStatusPending,
		ExpandedFrom: "branch-step",
		Shell:        &types.ShellConfig{Command: "echo child 1"},
	}
	wf.Steps["branch-step.child-2"] = &types.Step{
		ID:           "branch-step.child-2",
		Executor:     types.ExecutorShell,
		Status:       types.StepStatusPending,
		ExpandedFrom: "branch-step",
		Needs:        []string{"branch-step.child-1"},
		Shell:        &types.ShellConfig{Command: "echo child 2"},
	}

	// Done step depends on branch step
	wf.Steps["done-step"] = &types.Step{
		ID:       "done-step",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Needs:    []string{"branch-step"},
		Shell:    &types.ShellConfig{Command: "echo done"},
	}

	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	// Run the orchestrator - it should complete successfully
	// WITHOUT the fix, the orchestrator will hang because:
	// - The branch step is "running" with children
	// - No code completes the branch when children are done
	// - The done-step is blocked waiting for branch to be "done"
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (this means branch step never completed after children finished)", err)
	}

	// Re-fetch workflow to get latest state (async updates)
	wf, _ = store.Get(ctx, wf.ID)

	// Verify all steps completed (dependency ordering is enforced by DAG)
	// Shell now executes via async branch, so we verify status instead of mock
	if wf.Steps["branch-step.child-1"].Status != types.StepStatusDone {
		t.Errorf("child-1 status = %v, want done", wf.Steps["branch-step.child-1"].Status)
	}
	if wf.Steps["branch-step.child-2"].Status != types.StepStatusDone {
		t.Errorf("child-2 status = %v, want done", wf.Steps["branch-step.child-2"].Status)
	}
	if wf.Steps["branch-step"].Status != types.StepStatusDone {
		t.Errorf("branch-step status = %v, want done", wf.Steps["branch-step"].Status)
	}
	if wf.Steps["done-step"].Status != types.StepStatusDone {
		t.Errorf("done-step status = %v, want done", wf.Steps["done-step"].Status)
	}
	// Workflow should be done
	if wf.Status != types.RunStatusDone {
		t.Errorf("Workflow status = %v, want done", wf.Status)
	}
}

// getStepIDs returns a slice of step IDs for debugging output.
func getStepIDs(steps []*types.Step) []string {
	ids := make([]string, len(steps))
	for i, s := range steps {
		ids[i] = s.ID
	}
	return ids
}

// --- Async Dispatch Tests ---

// TestHandleBranch_ReturnsImmediately verifies that handleBranch returns immediately
// even when the condition command takes a long time to execute.
func TestHandleBranch_ReturnsImmediately(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a branch step that has a slow condition (sleeps 5 seconds)
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["branch-step"] = &types.Step{
		ID:       "branch-step",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "sleep 5", // Slow condition
			OnTrue:    &types.BranchTarget{Template: ".on-true"},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()

	// Measure how long handleBranch takes
	start := time.Now()
	err := orch.handleBranch(ctx, wf, wf.Steps["branch-step"])
	elapsed := time.Since(start)

	// handleBranch should return immediately (< 100ms)
	if err != nil {
		t.Errorf("handleBranch error = %v, want nil", err)
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("handleBranch took %v, want < 100ms (async dispatch failed)", elapsed)
	}

	// Clean up: cancel the pending command
	if cancel, ok := orch.pendingCommands.Load("branch-step"); ok {
		if cancelFunc, ok := cancel.(context.CancelFunc); ok {
			cancelFunc()
		}
	}
	orch.wg.Wait() // Wait for goroutine to exit
}

// TestHandleBranch_StepStaysRunning verifies that the step status is "running"
// after handleBranch returns, not "done".
func TestHandleBranch_StepStaysRunning(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a branch step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["branch-step"] = &types.Step{
		ID:       "branch-step",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "sleep 1", // Condition that takes time
			OnTrue:    &types.BranchTarget{Template: ".on-true"},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.handleBranch(ctx, wf, wf.Steps["branch-step"])
	if err != nil {
		t.Fatalf("handleBranch error = %v", err)
	}

	// Step status should be "running" after handleBranch returns
	if wf.Steps["branch-step"].Status != types.StepStatusRunning {
		t.Errorf("Step status = %v, want %v", wf.Steps["branch-step"].Status, types.StepStatusRunning)
	}

	// StartedAt should be set
	if wf.Steps["branch-step"].StartedAt == nil {
		t.Error("Step StartedAt should be set")
	}

	// Clean up
	if cancel, ok := orch.pendingCommands.Load("branch-step"); ok {
		if cancelFunc, ok := cancel.(context.CancelFunc); ok {
			cancelFunc()
		}
	}
	orch.wg.Wait()
}

// TestHandleBranch_TracksPendingCommand verifies that handleBranch stores the
// cancel function in pendingCommands.
func TestHandleBranch_TracksPendingCommand(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a branch step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["branch-step"] = &types.Step{
		ID:       "branch-step",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "sleep 2",
			OnTrue:    &types.BranchTarget{Template: ".on-true"},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.handleBranch(ctx, wf, wf.Steps["branch-step"])
	if err != nil {
		t.Fatalf("handleBranch error = %v", err)
	}

	// pendingCommands should contain the step ID
	cancel, ok := orch.pendingCommands.Load("branch-step")
	if !ok {
		t.Fatal("pendingCommands does not contain branch-step")
	}

	// Value should be a cancel function
	cancelFunc, ok := cancel.(context.CancelFunc)
	if !ok {
		t.Fatalf("pendingCommands value is %T, want context.CancelFunc", cancel)
	}

	// Cancel should be callable (won't panic)
	cancelFunc()
	orch.wg.Wait()
}

// TestParallelBranchSteps verifies that multiple branch steps with the same
// dependencies start in the same orchestrator tick (parallel execution).
func TestParallelBranchSteps(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with two branch steps that both need "init" step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Initial step that's already done
	wf.Steps["init"] = &types.Step{
		ID:       "init",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusDone,
		Shell:    &types.ShellConfig{Command: "echo init"},
	}

	// Two branch steps that both depend on "init"
	wf.Steps["branch-1"] = &types.Step{
		ID:       "branch-1",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Needs:    []string{"init"},
		Branch: &types.BranchConfig{
			Condition: "sleep 1 && true",
			OnTrue:    &types.BranchTarget{Template: ".on-true"},
		},
	}
	wf.Steps["branch-2"] = &types.Step{
		ID:       "branch-2",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Needs:    []string{"init"},
		Branch: &types.BranchConfig{
			Condition: "sleep 1 && true",
			OnTrue:    &types.BranchTarget{Template: ".on-true"},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()

	// Process workflow once - should dispatch both branch steps
	start := time.Now()
	err := orch.processWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("processWorkflow error = %v", err)
	}
	elapsed := time.Since(start)

	// processWorkflow should return quickly (dispatches async, doesn't wait)
	if elapsed > 200*time.Millisecond {
		t.Errorf("processWorkflow took %v, want < 200ms (should not wait for conditions)", elapsed)
	}

	// Both branch steps should be running
	if wf.Steps["branch-1"].Status != types.StepStatusRunning {
		t.Errorf("branch-1 status = %v, want running", wf.Steps["branch-1"].Status)
	}
	if wf.Steps["branch-2"].Status != types.StepStatusRunning {
		t.Errorf("branch-2 status = %v, want running", wf.Steps["branch-2"].Status)
	}

	// Both should be tracked in pendingCommands
	_, ok1 := orch.pendingCommands.Load("branch-1")
	_, ok2 := orch.pendingCommands.Load("branch-2")
	if !ok1 {
		t.Error("branch-1 not tracked in pendingCommands")
	}
	if !ok2 {
		t.Error("branch-2 not tracked in pendingCommands")
	}

	// Clean up
	orch.cancelPendingCommands()
	orch.wg.Wait()
}

// --- Branch Condition Outcome Tests ---

// TestBranchCondition_TrueOutcome tests that exit code 0 results in "true" outcome
// and on_true is expanded.
func TestBranchCondition_TrueOutcome(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with branch step
	// Branch conditions execute real shell commands (not mocked)
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["branch-step"] = &types.Step{
		ID:       "branch-step",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "exit 0",
			OnTrue: &types.BranchTarget{
				Inline: []types.InlineStep{
					{
						ID:       "on-true-step",
						Executor: types.ExecutorShell,
						Command:  "echo on-true",
					},
				},
			},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Re-fetch workflow
	wf, _ = store.Get(ctx, wf.ID)

	// Verify outcome
	if wf.Steps["branch-step"].Outputs["outcome"] != "true" {
		t.Errorf("Branch step outcome = %v, want 'true'", wf.Steps["branch-step"].Outputs["outcome"])
	}

	// Verify on_true was expanded
	if _, ok := wf.Steps["branch-step.on-true-step"]; !ok {
		t.Error("on_true step should be expanded")
	}

	// Verify branch step is done
	if wf.Steps["branch-step"].Status != types.StepStatusDone {
		t.Errorf("Branch step status = %v, want done", wf.Steps["branch-step"].Status)
	}
}

// TestBranchCondition_FalseOutcome tests that non-zero exit code results in
// "false" outcome and on_false is expanded.
func TestBranchCondition_FalseOutcome(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with branch step
	// Branch conditions execute real shell commands (not mocked)
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["branch-step"] = &types.Step{
		ID:       "branch-step",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "exit 1",
			OnFalse: &types.BranchTarget{
				Inline: []types.InlineStep{
					{
						ID:       "on-false-step",
						Executor: types.ExecutorShell,
						Command:  "echo on-false",
					},
				},
			},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Re-fetch workflow
	wf, _ = store.Get(ctx, wf.ID)

	// Verify outcome
	if wf.Steps["branch-step"].Outputs["outcome"] != "false" {
		t.Errorf("Branch step outcome = %v, want 'false'", wf.Steps["branch-step"].Outputs["outcome"])
	}

	// Verify on_false was expanded
	if _, ok := wf.Steps["branch-step.on-false-step"]; !ok {
		t.Error("on_false step should be expanded")
	}

	// Verify branch step is done
	if wf.Steps["branch-step"].Status != types.StepStatusDone {
		t.Errorf("Branch step status = %v, want done", wf.Steps["branch-step"].Status)
	}
}

// TestBranchCondition_TimeoutOutcome tests that timeout results in "timeout" outcome
// and on_timeout is expanded.
func TestBranchCondition_TimeoutOutcome(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with branch step with timeout
	// Use a command that will take longer than the timeout
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["branch-step"] = &types.Step{
		ID:       "branch-step",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "sleep 2",      // Sleep for 2 seconds
			Timeout:   "100ms",        // Timeout after 100ms
			OnTimeout: &types.BranchTarget{
				Inline: []types.InlineStep{
					{
						ID:       "on-timeout-step",
						Executor: types.ExecutorShell,
						Command:  "echo timeout",
					},
				},
			},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Re-fetch workflow
	wf, _ = store.Get(ctx, wf.ID)

	// Verify outcome
	if wf.Steps["branch-step"].Outputs["outcome"] != "timeout" {
		t.Errorf("Branch step outcome = %v, want 'timeout'", wf.Steps["branch-step"].Outputs["outcome"])
	}

	// Verify on_timeout was expanded
	if _, ok := wf.Steps["branch-step.on-timeout-step"]; !ok {
		t.Error("on_timeout step should be expanded")
	}

	// Verify branch step is done
	if wf.Steps["branch-step"].Status != types.StepStatusDone {
		t.Errorf("Branch step status = %v, want done", wf.Steps["branch-step"].Status)
	}
}

// TestBranchCondition_OutputCapture tests that stdout/stderr are captured
// in step outputs.
func TestBranchCondition_OutputCapture(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with branch step that captures outputs
	// Using a real shell command since branch conditions execute actual commands
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["branch-step"] = &types.Step{
		ID:       "branch-step",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "echo hello && echo debug >&2",
			Outputs: map[string]types.OutputSource{
				"out": {Source: "stdout"},
				"err": {Source: "stderr"},
			},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Re-fetch workflow
	wf, _ = store.Get(ctx, wf.ID)

	// Verify outputs
	outputs := wf.Steps["branch-step"].Outputs
	if outputs["outcome"] != "true" {
		t.Errorf("outcome = %v, want 'true'", outputs["outcome"])
	}
	if outputs["exit_code"] != 0 {
		t.Errorf("exit_code = %v, want 0", outputs["exit_code"])
	}
	// Stdout should contain "hello" (may have trailing newline)
	if out, ok := outputs["out"].(string); !ok || !strings.Contains(out, "hello") {
		t.Errorf("out = %v, want to contain 'hello'", outputs["out"])
	}
	// Stderr should contain "debug" (may have trailing newline)
	if err, ok := outputs["err"].(string); !ok || !strings.Contains(err, "debug") {
		t.Errorf("err = %v, want to contain 'debug'", outputs["err"])
	}

	// Verify step is done (no targets to expand)
	if wf.Steps["branch-step"].Status != types.StepStatusDone {
		t.Errorf("Branch step status = %v, want done", wf.Steps["branch-step"].Status)
	}
}

// TestBranchCondition_NoTargets_Completes tests that a branch with no targets
// (shell pattern) completes with outputs.
func TestBranchCondition_NoTargets_Completes(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with branch step with on_true but no on_false
	// Condition will exit 1, so outcome is "false" but no on_false target
	// Use on_error: continue so it doesn't fail the step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["branch-step"] = &types.Step{
		ID:       "branch-step",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "exit 1",
			OnError:   "continue", // Continue on error instead of failing
			OnTrue: &types.BranchTarget{
				Inline: []types.InlineStep{
					{
						ID:       "on-true-step",
						Executor: types.ExecutorShell,
						Command:  "echo on-true",
					},
				},
			},
			// No OnFalse - this is the key point
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	// Re-fetch workflow
	wf, _ = store.Get(ctx, wf.ID)

	// Verify outcome is "false"
	if wf.Steps["branch-step"].Outputs["outcome"] != "false" {
		t.Errorf("Branch step outcome = %v, want 'false'", wf.Steps["branch-step"].Outputs["outcome"])
	}

	// Verify no expansion occurred
	if len(wf.Steps["branch-step"].ExpandedInto) != 0 {
		t.Errorf("Branch step should not have expanded children, got %v", wf.Steps["branch-step"].ExpandedInto)
	}

	// Verify step is done (no target to expand, so completes immediately)
	if wf.Steps["branch-step"].Status != types.StepStatusDone {
		t.Errorf("Branch step status = %v, want done", wf.Steps["branch-step"].Status)
	}

	// Verify workflow is done
	if wf.Status != types.RunStatusDone {
		t.Errorf("Workflow status = %v, want done", wf.Status)
	}
}

// --- Cancellation Tests ---

// TestCancelPendingCommands_CancelsAll tests that cancelling multiple pending commands works correctly.
func TestCancelPendingCommands_CancelsAll(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Track which cancel functions were called
	var cancelledSteps sync.Map

	// Create 3 mock cancel functions
	for i := 1; i <= 3; i++ {
		stepID := fmt.Sprintf("branch-step-%d", i)
		cancelFunc := func(id string) context.CancelFunc {
			return func() {
				cancelledSteps.Store(id, true)
			}
		}(stepID)
		orch.pendingCommands.Store(stepID, cancelFunc)
	}

	// Cancel all pending commands
	orch.cancelPendingCommands()

	// Verify all 3 cancel functions were called
	for i := 1; i <= 3; i++ {
		stepID := fmt.Sprintf("branch-step-%d", i)
		if _, ok := cancelledSteps.Load(stepID); !ok {
			t.Errorf("Cancel function for %s was not called", stepID)
		}
	}
}

// TestCancelPendingCommands_EmptyMap tests that cancelling with no pending commands doesn't panic.
func TestCancelPendingCommands_EmptyMap(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// No pending commands - should not panic
	orch.cancelPendingCommands()

	// Success if we got here without panicking
}

// TestConditionCancelledMidExecution tests that pending commands are tracked and can be cancelled.
func TestConditionCancelledMidExecution(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a branch step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["branch-step"] = &types.Step{
		ID:       "branch-step",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "sleep 5", // Command that will be cancelled
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx := context.Background()

	// Start the branch step (launches goroutine)
	err := orch.dispatch(ctx, wf, wf.Steps["branch-step"])
	if err != nil {
		t.Fatalf("dispatch error = %v", err)
	}

	// Give the goroutine time to start executing
	time.Sleep(100 * time.Millisecond)

	// Step should be running
	if wf.Steps["branch-step"].Status != types.StepStatusRunning {
		t.Errorf("Step status = %v, want running", wf.Steps["branch-step"].Status)
	}

	// Verify there's a pending command being tracked
	cancelFunc, hasPending := orch.pendingCommands.Load("branch-step")
	if !hasPending {
		t.Fatal("Expected pending command to be tracked")
	}

	// Verify the cancel function is actually a function
	if _, ok := cancelFunc.(context.CancelFunc); !ok {
		t.Errorf("Expected context.CancelFunc, got %T", cancelFunc)
	}

	// Call cancel directly (simulating what cancelPendingCommands does)
	cancelFunc.(context.CancelFunc)()

	// Wait a bit for the goroutine to detect cancellation and exit
	// The defer in executeBranchConditionAsync should clean up the map
	time.Sleep(500 * time.Millisecond)

	// After cancellation and cleanup, the pending command should be removed
	// Note: We can't reliably test exact timing, but the defer ensures cleanup
}

// TestSignalTriggersCleanShutdown tests that signal handling cancels commands and waits for goroutines.
func TestSignalTriggersCleanShutdown(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a branch step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.Steps["branch-step"] = &types.Step{
		ID:       "branch-step",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "sleep 10", // Command that will be cancelled
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start orchestrator in background
	orchDone := make(chan error, 1)
	go func() {
		orchDone <- orch.Run(ctx)
	}()

	// Give orchestrator time to start the branch step
	time.Sleep(200 * time.Millisecond)

	// Verify the branch step is running and command is tracked
	wf, _ = store.Get(ctx, wf.ID)
	if wf.Steps["branch-step"].Status != types.StepStatusRunning {
		t.Errorf("Step status = %v, want running", wf.Steps["branch-step"].Status)
	}

	_, hasPending := orch.pendingCommands.Load("branch-step")
	if !hasPending {
		t.Error("Expected pending command to be tracked")
	}

	// Simulate signal by calling Shutdown (which cancels the context)
	// This mimics what happens when a signal is received
	orch.Shutdown()

	// Wait for orchestrator to complete (should be quick with cancellation)
	// Allow up to 3 seconds for process termination
	select {
	case err := <-orchDone:
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Orchestrator did not shut down within 3 seconds after signal")
	}

	// Verify cleanup happened (defer in executeBranchConditionAsync removes from map)
	_, stillPending := orch.pendingCommands.Load("branch-step")
	if stillPending {
		t.Error("pendingCommands should have been cleaned up after shutdown")
	}
}

// --- Shell as Sugar Tests ---

// TestHandleShell_DelegatesToBranch verifies that handleShell converts
// shell config to branch config and delegates to handleBranch.
func TestHandleShell_DelegatesToBranch(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Create shell step
	wf.Steps["shell-step"] = &types.Step{
		ID:       "shell-step",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell: &types.ShellConfig{
			Command: "echo hello",
			Workdir: "/tmp",
			Env:     map[string]string{"FOO": "bar"},
			Outputs: map[string]types.OutputSource{
				"out": {Source: "stdout"},
			},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	step := wf.Steps["shell-step"]

	// Call handleShell
	err := orch.handleShell(ctx, wf, step)
	if err != nil {
		t.Fatalf("handleShell error = %v", err)
	}

	// Verify step.Branch is populated
	if step.Branch == nil {
		t.Fatal("step.Branch should be populated after handleShell")
	}

	// Verify step.Shell is nil (cleared after conversion)
	if step.Shell != nil {
		t.Error("step.Shell should be nil after handleShell")
	}

	// Verify branch config has correct command
	if step.Branch.Condition != "echo hello" {
		t.Errorf("Branch condition = %q, want %q", step.Branch.Condition, "echo hello")
	}

	// Verify step is running (handleBranch marks it as running and launches async)
	if step.Status != types.StepStatusRunning {
		t.Errorf("Step status = %v, want running", step.Status)
	}
}

// TestHandleShell_RunsAsync verifies that handleShell returns immediately
// without waiting for command completion.
func TestHandleShell_RunsAsync(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Create shell step with slow command
	wf.Steps["slow-step"] = &types.Step{
		ID:       "slow-step",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell: &types.ShellConfig{
			Command: "sleep 2", // 2 second delay
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	step := wf.Steps["slow-step"]

	// Measure time for handleShell call
	start := time.Now()
	err := orch.handleShell(ctx, wf, step)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("handleShell error = %v", err)
	}

	// Should return immediately (< 100ms)
	if elapsed > 100*time.Millisecond {
		t.Errorf("handleShell took %v, want < 100ms (should be async)", elapsed)
	}

	// Wait for goroutines to complete
	orch.wg.Wait()

	// After completion, verify step is done
	// Re-fetch to avoid race condition with async updates
	finalWf, _ := store.Get(ctx, wf.ID)
	finalStep := finalWf.Steps["slow-step"]
	if finalStep.Status != types.StepStatusDone {
		t.Errorf("Step status = %v, want done (after async completion)", finalStep.Status)
	}
}

// TestShellConfigConversion verifies that all ShellConfig fields
// are correctly transferred to BranchConfig.
func TestShellConfigConversion(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Create shell step with all fields populated
	wf.Steps["full-step"] = &types.Step{
		ID:       "full-step",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell: &types.ShellConfig{
			Command: "echo test",
			Workdir: "/custom/path",
			Env: map[string]string{
				"VAR1": "value1",
				"VAR2": "value2",
			},
			OnError: "continue",
			Outputs: map[string]types.OutputSource{
				"stdout": {Source: "stdout"},
				"stderr": {Source: "stderr"},
			},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	step := wf.Steps["full-step"]

	err := orch.handleShell(ctx, wf, step)
	if err != nil {
		t.Fatalf("handleShell error = %v", err)
	}

	// Verify all fields transferred
	branch := step.Branch
	if branch == nil {
		t.Fatal("Branch config should be populated")
	}

	if branch.Condition != "echo test" {
		t.Errorf("Condition = %q, want %q", branch.Condition, "echo test")
	}

	if branch.Workdir != "/custom/path" {
		t.Errorf("Workdir = %q, want %q", branch.Workdir, "/custom/path")
	}

	if len(branch.Env) != 2 {
		t.Errorf("Env has %d entries, want 2", len(branch.Env))
	}
	if branch.Env["VAR1"] != "value1" {
		t.Errorf("Env[VAR1] = %q, want %q", branch.Env["VAR1"], "value1")
	}

	if branch.OnError != "continue" {
		t.Errorf("OnError = %q, want %q", branch.OnError, "continue")
	}

	if len(branch.Outputs) != 2 {
		t.Errorf("Outputs has %d entries, want 2", len(branch.Outputs))
	}

	// Verify no expansion targets (shell has no branches)
	if branch.OnTrue != nil {
		t.Error("OnTrue should be nil for shell-as-sugar")
	}
	if branch.OnFalse != nil {
		t.Error("OnFalse should be nil for shell-as-sugar")
	}
	if branch.OnTimeout != nil {
		t.Error("OnTimeout should be nil for shell-as-sugar")
	}

	// Wait for async completion
	orch.wg.Wait()
}

// TestHandleShell_CapturesOutputs verifies that shell step outputs
// are captured correctly after command execution.
func TestHandleShell_CapturesOutputs(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Create shell step with output definitions
	wf.Steps["output-step"] = &types.Step{
		ID:       "output-step",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell: &types.ShellConfig{
			Command: "echo hello",
			Outputs: map[string]types.OutputSource{
				"result": {Source: "stdout"},
			},
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Run the workflow
	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}

	// Re-fetch workflow to get final state
	finalWf, _ := store.Get(ctx, wf.ID)

	// Step should be done
	step := finalWf.Steps["output-step"]
	if step.Status != types.StepStatusDone {
		t.Errorf("Step status = %v, want done", step.Status)
	}

	// Outputs should be captured
	if step.Outputs == nil {
		t.Fatal("Step outputs should be populated")
	}
}

// TestHandleShell_OnErrorFail verifies that shell steps with on_error: fail
// mark the step as failed when the command exits non-zero.
func TestHandleShell_OnErrorFail(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Create shell step with failing command and on_error: fail
	wf.Steps["fail-step"] = &types.Step{
		ID:       "fail-step",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell: &types.ShellConfig{
			Command: "exit 1", // Fails
			OnError: "fail",
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Run the workflow
	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}

	// Re-fetch workflow to get final state
	finalWf, _ := store.Get(ctx, wf.ID)

	// Step should be failed
	step := finalWf.Steps["fail-step"]
	if step.Status != types.StepStatusFailed {
		t.Errorf("Step status = %v, want failed", step.Status)
	}

	// Error should be set
	if step.Error == nil {
		t.Error("Step error should be set")
	}

	// Workflow should be failed
	if finalWf.Status != types.RunStatusFailed {
		t.Errorf("Workflow status = %v, want failed", finalWf.Status)
	}
}

// TestHandleShell_OnErrorContinue verifies that shell steps with on_error: continue
// mark the step as done even when the command fails.
func TestHandleShell_OnErrorContinue(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Create two steps: one that fails with continue, one that depends on it
	wf.Steps["continue-step"] = &types.Step{
		ID:       "continue-step",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell: &types.ShellConfig{
			Command: "exit 1", // Fails
			OnError: "continue",
		},
	}
	wf.Steps["after-step"] = &types.Step{
		ID:       "after-step",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Needs:    []string{"continue-step"},
		Shell: &types.ShellConfig{
			Command: "echo after",
		},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Run the workflow
	err := orch.Run(ctx)
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}

	// Re-fetch workflow to get final state
	finalWf, _ := store.Get(ctx, wf.ID)

	// First step should be done (not failed)
	step1 := finalWf.Steps["continue-step"]
	if step1.Status != types.StepStatusDone {
		t.Errorf("continue-step status = %v, want done", step1.Status)
	}

	// Error info should be in outputs
	if step1.Outputs == nil || step1.Outputs["error"] == nil {
		t.Error("Step outputs should contain error info")
	}

	// Second step should have run (workflow continues)
	step2 := finalWf.Steps["after-step"]
	if step2.Status != types.StepStatusDone {
		t.Errorf("after-step status = %v, want done", step2.Status)
	}

	// Workflow should be done (not failed)
	if finalWf.Status != types.RunStatusDone {
		t.Errorf("Workflow status = %v, want done", finalWf.Status)
	}
}

// TestParallelShellSteps verifies that shell steps with the same dependencies
// run in parallel rather than sequentially.
func TestParallelShellSteps(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Create three steps: one setup, two parallel sleeps
	wf.Steps["setup"] = &types.Step{
		ID:       "setup",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Shell:    &types.ShellConfig{Command: "echo setup"},
	}
	wf.Steps["parallel-1"] = &types.Step{
		ID:       "parallel-1",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Needs:    []string{"setup"},
		Shell:    &types.ShellConfig{Command: "sleep 0.5"},
	}
	wf.Steps["parallel-2"] = &types.Step{
		ID:       "parallel-2",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Needs:    []string{"setup"},
		Shell:    &types.ShellConfig{Command: "sleep 0.5"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)
	orch.SetWorkflowID(wf.ID)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Measure total execution time
	start := time.Now()
	err := orch.Run(ctx)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Run error = %v", err)
	}

	// Re-fetch workflow to verify completion
	finalWf, _ := store.Get(ctx, wf.ID)

	// All steps should be done
	for stepID, step := range finalWf.Steps {
		if step.Status != types.StepStatusDone {
			t.Errorf("Step %s status = %v, want done", stepID, step.Status)
		}
	}

	// Total time should be ~0.5s (parallel) not ~1s (sequential)
	// Allow up to 1s for overhead, but should be much less than 1.5s
	if elapsed > 1200*time.Millisecond {
		t.Errorf("Parallel execution took %v, want < 1.2s (suggesting sequential execution)", elapsed)
	}

	t.Logf("Parallel execution completed in %v", elapsed)
}

// --- Step Output Resolution Tests ---

func TestFindStepWithScopeWalk_ExactMatch(t *testing.T) {
	wf := &types.Run{
		ID:    "test-wf",
		Steps: make(map[string]*types.Step),
	}
	wf.Steps["shell-step"] = &types.Step{
		ID:      "shell-step",
		Status:  types.StepStatusDone,
		Outputs: map[string]any{"result": "exact-value"},
	}

	step, resolvedID, ok := findStepWithScopeWalk(wf, "shell-step", "some-other-step")
	if !ok {
		t.Fatal("expected to find step")
	}
	if resolvedID != "shell-step" {
		t.Errorf("resolvedID = %q, want %q", resolvedID, "shell-step")
	}
	if step.Outputs["result"] != "exact-value" {
		t.Errorf("outputs = %v, want exact-value", step.Outputs["result"])
	}
}

func TestFindStepWithScopeWalk_SingleLevelPrefix(t *testing.T) {
	wf := &types.Run{
		ID:    "test-wf",
		Steps: make(map[string]*types.Step),
	}
	// Step with prefixed ID (as from foreach expansion)
	wf.Steps["agents.0.resolve-protocol"] = &types.Step{
		ID:      "agents.0.resolve-protocol",
		Status:  types.StepStatusDone,
		Outputs: map[string]any{"resolved_protocol": "tdd"},
	}

	// Current step is sibling in same foreach iteration
	step, resolvedID, ok := findStepWithScopeWalk(wf, "resolve-protocol", "agents.0.track")
	if !ok {
		t.Fatal("expected to find step via scope-walk")
	}
	if resolvedID != "agents.0.resolve-protocol" {
		t.Errorf("resolvedID = %q, want %q", resolvedID, "agents.0.resolve-protocol")
	}
	if step.Outputs["resolved_protocol"] != "tdd" {
		t.Errorf("outputs = %v, want tdd", step.Outputs["resolved_protocol"])
	}
}

func TestFindStepWithScopeWalk_MultiLevelPrefix(t *testing.T) {
	wf := &types.Run{
		ID:    "test-wf",
		Steps: make(map[string]*types.Step),
	}
	// Deep nesting: a.b.c.shell-step
	wf.Steps["a.b.c.shell-step"] = &types.Step{
		ID:      "a.b.c.shell-step",
		Status:  types.StepStatusDone,
		Outputs: map[string]any{"value": "deep-nested"},
	}

	// Current step is a.b.c.d.expand-step - should walk up to find a.b.c.shell-step
	step, resolvedID, ok := findStepWithScopeWalk(wf, "shell-step", "a.b.c.d.expand-step")
	if !ok {
		t.Fatal("expected to find step via multi-level scope-walk")
	}
	if resolvedID != "a.b.c.shell-step" {
		t.Errorf("resolvedID = %q, want %q", resolvedID, "a.b.c.shell-step")
	}
	if step.Outputs["value"] != "deep-nested" {
		t.Errorf("outputs = %v, want deep-nested", step.Outputs["value"])
	}
}

func TestFindStepWithScopeWalk_NotFound(t *testing.T) {
	wf := &types.Run{
		ID:    "test-wf",
		Steps: make(map[string]*types.Step),
	}
	wf.Steps["other-step"] = &types.Step{
		ID:     "other-step",
		Status: types.StepStatusDone,
	}

	_, _, ok := findStepWithScopeWalk(wf, "missing-step", "agents.0.track")
	if ok {
		t.Error("expected not to find non-existent step")
	}
}

func TestGetNestedOutputValue_SimpleField(t *testing.T) {
	outputs := map[string]any{
		"result": "simple-value",
		"count":  42,
	}

	val, ok := getNestedOutputValue(outputs, "result")
	if !ok {
		t.Fatal("expected to find field")
	}
	if val != "simple-value" {
		t.Errorf("val = %v, want simple-value", val)
	}
}

func TestGetNestedOutputValue_NestedField(t *testing.T) {
	outputs := map[string]any{
		"config": map[string]any{
			"database": map[string]any{
				"host": "localhost",
				"port": 5432,
			},
		},
	}

	val, ok := getNestedOutputValue(outputs, "config.database.host")
	if !ok {
		t.Fatal("expected to find nested field")
	}
	if val != "localhost" {
		t.Errorf("val = %v, want localhost", val)
	}
}

func TestGetNestedOutputValue_NotFound(t *testing.T) {
	outputs := map[string]any{
		"config": map[string]any{
			"existing": "value",
		},
	}

	_, ok := getNestedOutputValue(outputs, "config.missing")
	if ok {
		t.Error("expected not to find missing field")
	}

	_, ok = getNestedOutputValue(outputs, "missing")
	if ok {
		t.Error("expected not to find top-level missing field")
	}
}

func TestResolveStepOutputRefs_ExecutorExpand(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	wf := &types.Run{
		ID:    "test-wf",
		Steps: make(map[string]*types.Step),
	}

	// Completed shell step with output
	wf.Steps["resolve-protocol"] = &types.Step{
		ID:       "resolve-protocol",
		Status:   types.StepStatusDone,
		Executor: types.ExecutorShell,
		Outputs:  map[string]any{"resolved_protocol": "tdd"},
	}

	// Expand step referencing the shell step's output
	expandStep := &types.Step{
		ID:       "track",
		Status:   types.StepStatusPending,
		Executor: types.ExecutorExpand,
		Expand: &types.ExpandConfig{
			Template: "lib/protocols/{{resolve-protocol.outputs.resolved_protocol}}",
			Variables: map[string]any{
				"protocol": "{{resolve-protocol.outputs.resolved_protocol}}",
			},
		},
	}
	wf.Steps["track"] = expandStep

	store.workflows[wf.ID] = wf
	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Resolve references
	orch.resolveStepOutputRefs(wf, expandStep)

	// Check template was resolved
	if expandStep.Expand.Template != "lib/protocols/tdd" {
		t.Errorf("Template = %q, want %q", expandStep.Expand.Template, "lib/protocols/tdd")
	}

	// Check variables were resolved
	if expandStep.Expand.Variables["protocol"] != "tdd" {
		t.Errorf("Variables[protocol] = %q, want %q", expandStep.Expand.Variables["protocol"], "tdd")
	}
}

func TestResolveStepOutputRefs_WithScopeWalk(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	wf := &types.Run{
		ID:    "test-wf",
		Steps: make(map[string]*types.Step),
	}

	// Foreach-expanded shell step with output
	wf.Steps["agents.0.resolve-protocol"] = &types.Step{
		ID:       "agents.0.resolve-protocol",
		Status:   types.StepStatusDone,
		Executor: types.ExecutorShell,
		Outputs:  map[string]any{"resolved_protocol": "code-review"},
	}

	// Expand step in same foreach iteration, referencing sibling with unprefixed ID
	expandStep := &types.Step{
		ID:       "agents.0.track",
		Status:   types.StepStatusPending,
		Executor: types.ExecutorExpand,
		Expand: &types.ExpandConfig{
			Template: "lib/protocols/{{resolve-protocol.outputs.resolved_protocol}}",
			Variables: map[string]any{
				"protocol": "{{resolve-protocol.outputs.resolved_protocol}}",
			},
		},
	}
	wf.Steps["agents.0.track"] = expandStep

	store.workflows[wf.ID] = wf
	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Resolve references - should use scope-walk to find agents.0.resolve-protocol
	orch.resolveStepOutputRefs(wf, expandStep)

	// Check template was resolved via scope-walk
	if expandStep.Expand.Template != "lib/protocols/code-review" {
		t.Errorf("Template = %q, want %q", expandStep.Expand.Template, "lib/protocols/code-review")
	}

	// Check variables were resolved via scope-walk
	if expandStep.Expand.Variables["protocol"] != "code-review" {
		t.Errorf("Variables[protocol] = %q, want %q", expandStep.Expand.Variables["protocol"], "code-review")
	}
}

func TestResolveStepOutputRefs_ExecutorBranch(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	wf := &types.Run{
		ID:    "test-wf",
		Steps: make(map[string]*types.Step),
	}

	// Completed shell step with output
	wf.Steps["check-env"] = &types.Step{
		ID:       "check-env",
		Status:   types.StepStatusDone,
		Executor: types.ExecutorShell,
		Outputs:  map[string]any{"env_type": "production"},
	}

	// Branch step with variables referencing the shell step's output
	branchStep := &types.Step{
		ID:       "branch-step",
		Status:   types.StepStatusPending,
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "true",
			OnTrue: &types.BranchTarget{
				Template: "deploy-template",
				Variables: map[string]any{
					"environment": "{{check-env.outputs.env_type}}",
				},
			},
			OnFalse: &types.BranchTarget{
				Template: "skip-template",
				Variables: map[string]any{
					"reason": "{{check-env.outputs.env_type}}-not-ready",
				},
			},
		},
	}
	wf.Steps["branch-step"] = branchStep

	store.workflows[wf.ID] = wf
	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Resolve references
	orch.resolveStepOutputRefs(wf, branchStep)

	// Check OnTrue variables were resolved
	if branchStep.Branch.OnTrue.Variables["environment"] != "production" {
		t.Errorf("OnTrue.Variables[environment] = %q, want %q",
			branchStep.Branch.OnTrue.Variables["environment"], "production")
	}

	// Check OnFalse variables were resolved
	if branchStep.Branch.OnFalse.Variables["reason"] != "production-not-ready" {
		t.Errorf("OnFalse.Variables[reason] = %q, want %q",
			branchStep.Branch.OnFalse.Variables["reason"], "production-not-ready")
	}
}

func TestResolveStepOutputRefs_NestedOutputField(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	wf := &types.Run{
		ID:    "test-wf",
		Steps: make(map[string]*types.Step),
	}

	// Shell step with nested output structure
	wf.Steps["config-step"] = &types.Step{
		ID:       "config-step",
		Status:   types.StepStatusDone,
		Executor: types.ExecutorShell,
		Outputs: map[string]any{
			"config": map[string]any{
				"database": map[string]any{
					"host": "db.example.com",
				},
			},
		},
	}

	// Shell step referencing nested output
	shellStep := &types.Step{
		ID:       "use-config",
		Status:   types.StepStatusPending,
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "echo {{config-step.outputs.config.database.host}}",
		},
	}
	wf.Steps["use-config"] = shellStep

	store.workflows[wf.ID] = wf
	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Resolve references
	orch.resolveStepOutputRefs(wf, shellStep)

	// Check nested output was resolved
	if shellStep.Shell.Command != "echo db.example.com" {
		t.Errorf("Command = %q, want %q", shellStep.Shell.Command, "echo db.example.com")
	}
}

func TestResolveOutputRefs_WithScopeWalk(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))

	wf := &types.Run{
		ID:    "test-wf",
		Steps: make(map[string]*types.Step),
	}

	// Foreach-expanded shell step with output
	wf.Steps["agents.0.check-status"] = &types.Step{
		ID:       "agents.0.check-status",
		Status:   types.StepStatusDone,
		Executor: types.ExecutorShell,
		Outputs:  map[string]any{"ready": "true"},
	}

	store.workflows[wf.ID] = wf
	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Resolve a condition string with scope-walk
	condition := "test {{check-status.outputs.ready}} == true"
	resolved := orch.resolveOutputRefs(wf, condition, "agents.0.branch-step")

	if resolved != "test true == true" {
		t.Errorf("resolved = %q, want %q", resolved, "test true == true")
	}
}

// ===========================================================================
// Prompt Acknowledgment Tests
// Spec: agent-lifecycle (prompt acknowledgment tracking)
// ===========================================================================

// TestOrchestrator_SetEventRouter tests that SetEventRouter correctly sets the router.
func TestOrchestrator_SetEventRouter(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Create an event router
	router := NewEventRouter(logger)

	// Set it on the orchestrator
	orch.SetEventRouter(router)

	// Verify it was set (by using it indirectly via waitForPromptAcknowledgment)
	// This test will fail until SetEventRouter is implemented
	if orch.eventRouter == nil {
		t.Error("eventRouter should be set after SetEventRouter()")
	}
}

// TestOrchestrator_WaitForPromptAcknowledgment_Success tests that acknowledgment
// is received when prompt-received event is emitted.
func TestOrchestrator_WaitForPromptAcknowledgment_Success(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Create and set event router
	router := NewEventRouter(logger)
	orch.SetEventRouter(router)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Track if acknowledgment completed
	done := make(chan bool, 1)

	// Start waiting for acknowledgment in goroutine
	go func() {
		orch.waitForPromptAcknowledgment(ctx, "test-agent", "step-1", 2*time.Second)
		done <- true
	}()

	// Give time for waiter to register
	time.Sleep(50 * time.Millisecond)

	// Emit the prompt-received event
	event := &ipc.EventMessage{
		EventType: "prompt-received",
		Agent:     "test-agent",
	}
	router.Route(event)

	// Wait for acknowledgment to complete
	select {
	case <-done:
		// Success - acknowledgment received
	case <-time.After(1 * time.Second):
		t.Error("waitForPromptAcknowledgment did not complete after event was routed")
	}
}

// TestOrchestrator_WaitForPromptAcknowledgment_Timeout tests that timeout is
// handled gracefully when no event is received.
func TestOrchestrator_WaitForPromptAcknowledgment_Timeout(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Create and set event router
	router := NewEventRouter(logger)
	orch.SetEventRouter(router)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Track how long acknowledgment takes
	start := time.Now()

	// Wait for acknowledgment with short timeout (no event will be emitted)
	orch.waitForPromptAcknowledgment(ctx, "test-agent", "step-1", 100*time.Millisecond)

	elapsed := time.Since(start)

	// Should complete around the timeout duration (with some tolerance)
	if elapsed < 90*time.Millisecond {
		t.Errorf("waitForPromptAcknowledgment returned too quickly: %v", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("waitForPromptAcknowledgment took too long: %v", elapsed)
	}
}

// TestOrchestrator_WaitForPromptAcknowledgment_NoRouter tests that nil router
// is handled gracefully (no-op).
func TestOrchestrator_WaitForPromptAcknowledgment_NoRouter(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Do NOT set event router - leave it nil

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Track how long it takes
	start := time.Now()

	// Should return immediately without panic
	orch.waitForPromptAcknowledgment(ctx, "test-agent", "step-1", 5*time.Second)

	elapsed := time.Since(start)

	// Should complete almost immediately (no blocking)
	if elapsed > 100*time.Millisecond {
		t.Errorf("waitForPromptAcknowledgment with nil router took too long: %v", elapsed)
	}
}

// TestOrchestrator_WaitForPromptAcknowledgment_ContextCancelled tests that
// context cancellation is handled gracefully.
func TestOrchestrator_WaitForPromptAcknowledgment_ContextCancelled(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Create and set event router
	router := NewEventRouter(logger)
	orch.SetEventRouter(router)

	ctx, cancel := context.WithCancel(context.Background())

	// Track completion
	done := make(chan bool, 1)

	go func() {
		orch.waitForPromptAcknowledgment(ctx, "test-agent", "step-1", 5*time.Second)
		done <- true
	}()

	// Give time for waiter to register
	time.Sleep(50 * time.Millisecond)

	// Cancel context
	cancel()

	// Should complete quickly after cancel
	select {
	case <-done:
		// Success - returned after cancel
	case <-time.After(500 * time.Millisecond):
		t.Error("waitForPromptAcknowledgment did not return after context cancel")
	}
}

// ===========================================================================
// Prompt Recovery Tests
// Spec: agent-lifecycle (prompt recovery mechanism)
// ===========================================================================

// TestOrchestrator_PromptRecovery_Success tests that recovery succeeds when
// the prompt-received event is emitted after re-injection.
// Spec: agent-lifecycle.prompt-recovery-success
func TestOrchestrator_PromptRecovery_Success(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Create and set event router
	router := NewEventRouter(logger)
	orch.SetEventRouter(router)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Track completion
	done := make(chan bool, 1)

	// Start waiting for acknowledgment with recovery enabled
	// Initial timeout will expire (no event), triggering recovery
	// Recovery will re-inject and second event will be emitted
	go func() {
		orch.waitForPromptAcknowledgmentWithRecovery(ctx, "test-agent", "step-1", "test prompt", 100*time.Millisecond)
		done <- true
	}()

	// Give time for initial timeout to occur
	time.Sleep(150 * time.Millisecond)

	// Now emit the event (simulating successful recovery)
	event := &ipc.EventMessage{
		EventType: "prompt-received",
		Agent:     "test-agent",
	}
	router.Route(event)

	// Wait for completion
	select {
	case <-done:
		// Success - recovery completed
	case <-time.After(3 * time.Second):
		t.Error("recovery did not complete after event was routed")
	}

	// Verify that prompt was re-injected (at least once for recovery)
	injections := agents.GetInjections()
	if len(injections) < 1 {
		t.Errorf("expected at least 1 recovery injection, got %d", len(injections))
	}

	// The recovery injection should have Stabilize=true
	for _, inj := range injections {
		if inj.AgentID == "test-agent" && inj.Stabilize {
			// Found recovery injection with stabilization
			return
		}
	}
	t.Error("expected recovery injection with Stabilize=true")
}

// TestOrchestrator_PromptRecovery_Escalate tests that prompt-swallowed event
// is emitted after max retries fail.
// Spec: agent-lifecycle.prompt-recovery-escalate
func TestOrchestrator_PromptRecovery_Escalate(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Create and set event router
	router := NewEventRouter(logger)
	orch.SetEventRouter(router)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Register a waiter for the prompt-swallowed event to verify it gets emitted
	swallowedCh := router.RegisterWaiter("prompt-swallowed", map[string]string{"agent": "test-agent"}, 5*time.Second)

	// Track completion
	done := make(chan bool, 1)

	// Start waiting for acknowledgment with recovery enabled
	// All attempts will timeout (no events emitted), triggering escalation
	go func() {
		orch.waitForPromptAcknowledgmentWithRecovery(ctx, "test-agent", "step-1", "test prompt", 100*time.Millisecond)
		done <- true
	}()

	// Wait for completion (should timeout after all retries)
	select {
	case <-done:
		// Recovery exhausted
	case <-time.After(5 * time.Second):
		t.Error("recovery did not complete after all retries")
	}

	// Verify that prompt-swallowed event was emitted
	select {
	case event := <-swallowedCh:
		if event == nil {
			t.Error("received nil event from swallowedCh")
		} else if event.Agent != "test-agent" {
			t.Errorf("expected agent 'test-agent', got %q", event.Agent)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("prompt-swallowed event was not emitted after recovery exhausted")
	}

	// Verify that multiple injection attempts were made
	injections := agents.GetInjections()
	if len(injections) < 1 {
		t.Errorf("expected at least 1 recovery injection, got %d", len(injections))
	}
}

// TestOrchestrator_PromptRecovery_NoRouter tests that recovery handles nil router gracefully.
// Spec: agent-lifecycle.prompt-recovery-no-router
func TestOrchestrator_PromptRecovery_NoRouter(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	// Do NOT set event router - leave it nil

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Track how long it takes
	start := time.Now()

	// Should return immediately without panic
	orch.waitForPromptAcknowledgmentWithRecovery(ctx, "test-agent", "step-1", "test prompt", 5*time.Second)

	elapsed := time.Since(start)

	// Should complete almost immediately (no blocking)
	if elapsed > 100*time.Millisecond {
		t.Errorf("waitForPromptAcknowledgmentWithRecovery with nil router took too long: %v", elapsed)
	}
}

// ===========================================================================
// Agent Injection Failure Recovery Tests
// ===========================================================================

// TestOrchestrator_AgentInjectionFailure_SessionAlive tests that when InjectPrompt
// fails but the agent session is still alive, the step is reset to pending for retry.
func TestOrchestrator_AgentInjectionFailure_SessionAlive(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a pending agent step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	wf.Steps["agent-step"] = &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusPending,
		Agent:    &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	store.workflows[wf.ID] = wf

	// Agent is running (alive), but InjectPrompt will fail
	agents.running["test-agent"] = true
	agents.injectErr = fmt.Errorf("send-keys: signal: killed: not in a mode (took 30.022291698s)")

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.processWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("processWorkflow error = %v", err)
	}

	// Step should be reset to pending (transient error, agent alive → retry)
	step := wf.Steps["agent-step"]
	if step.Status != types.StepStatusPending {
		t.Errorf("Agent step status = %v, want %v (should reset to pending when agent alive)",
			step.Status, types.StepStatusPending)
	}

	// StartedAt should be cleared (reset to pending clears it)
	if step.StartedAt != nil {
		t.Error("Agent step StartedAt should be nil after reset to pending")
	}
}

// TestOrchestrator_AgentInjectionFailure_SessionDead tests that when InjectPrompt
// fails and the agent session is dead, the step is marked as failed.
func TestOrchestrator_AgentInjectionFailure_SessionDead(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a pending agent step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	wf.Steps["agent-step"] = &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusPending,
		Agent:    &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	store.workflows[wf.ID] = wf

	// Agent is NOT running (dead), and InjectPrompt will fail
	agents.running["test-agent"] = false
	agents.injectErr = fmt.Errorf("send-keys: signal: killed: not in a mode (took 30.022291698s)")

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.processWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("processWorkflow error = %v", err)
	}

	// Step should be failed (agent session is dead, no recovery possible)
	step := wf.Steps["agent-step"]
	if step.Status != types.StepStatusFailed {
		t.Errorf("Agent step status = %v, want %v (should fail when agent dead)",
			step.Status, types.StepStatusFailed)
	}

	// Error should be recorded
	if step.Error == nil {
		t.Error("Agent step should have an error recorded")
	}
}

// TestOrchestrator_AgentInjectionFailure_DispatchErrorFallback tests that if
// handleAgent returns an error and the step is in running status, the dispatch
// error handler in processWorkflow fails the step (defense-in-depth).
// ===========================================================================
// Bug Fix Tests: Timeout Persistence, Key Collision, Cleanup Mutex Safety
// ===========================================================================

// TestOrchestrator_TimeoutPersistence_SavesWhenReadyStepsSkipped tests that
// processWorkflow saves workflow state when checkStepTimeouts modifies state
// but all ready steps are skipped (e.g., busy agent).
// Bug: meow-86fo - timeout state changes dropped when readySteps > 0 but none dispatch.
func TestOrchestrator_TimeoutPersistence_SavesWhenReadyStepsSkipped(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with two agent steps targeting the same agent:
	// - Step A: running and timed out (will trigger timeout handling)
	// - Step B: pending, ready (no needs), targets same busy agent (will be skipped)
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	// Step A: running, started 2 seconds ago, 1s timeout → already timed out
	startedAt := time.Now().Add(-2 * time.Second)
	wf.Steps["step-a"] = &types.Step{
		ID:        "step-a",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusRunning,
		StartedAt: &startedAt,
		Agent: &types.AgentConfig{
			Agent:   "test-agent",
			Prompt:  "Do work A",
			Timeout: "1s",
		},
	}

	// Step B: pending, ready (no needs), targets same agent → will be skipped because agent is busy
	wf.Steps["step-b"] = &types.Step{
		ID:       "step-b",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusPending,
		Agent: &types.AgentConfig{
			Agent:  "test-agent",
			Prompt: "Do work B",
		},
	}

	store.workflows[wf.ID] = wf

	// Mark agent as running so it's "busy" (step A is running on it)
	agents.running["test-agent"] = true

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.processWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("processWorkflow error = %v", err)
	}

	// Step A should have InterruptedAt set (timeout handling modified state)
	if wf.Steps["step-a"].InterruptedAt == nil {
		t.Fatal("Step A InterruptedAt should be set by timeout check")
	}

	// The critical assertion: state MUST be saved even though no steps were dispatched.
	// Currently (buggy): readySteps=[step-b] is non-empty, dispatchedSteps={} is empty,
	// so the code falls through to `return nil` without saving the timeout state.
	store.mu.Lock()
	saveCalls := 0
	for _, call := range store.calls {
		if strings.HasPrefix(call, "Save:") {
			saveCalls++
		}
	}
	store.mu.Unlock()

	if saveCalls == 0 {
		t.Error("processWorkflow did not save workflow state; timeout modifications " +
			"(InterruptedAt) were dropped because readySteps was non-empty but none dispatched")
	}
}

// TestOrchestrator_PendingCommandsKeyCollision verifies that two concurrent
// workflows with the same step ID do not collide in pendingCommands.
// Bug: meow-0c8l - pendingCommands keyed by stepID alone causes overwrites.
func TestOrchestrator_PendingCommandsKeyCollision(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create two workflows, each with a branch step named "monitor"
	wfA := types.NewRun("workflow-a", "template-a", nil)
	wfA.Status = types.RunStatusRunning
	wfA.Steps["monitor"] = &types.Step{
		ID:       "monitor",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "sleep 2", // Moderate sleep (leaked goroutine won't block long)
			OnTrue:    &types.BranchTarget{Template: ".on-true"},
		},
	}
	store.workflows[wfA.ID] = wfA

	wfB := types.NewRun("workflow-b", "template-b", nil)
	wfB.Status = types.RunStatusRunning
	wfB.Steps["monitor"] = &types.Step{
		ID:       "monitor",
		Executor: types.ExecutorBranch,
		Status:   types.StepStatusPending,
		Branch: &types.BranchConfig{
			Condition: "sleep 2",
			OnTrue:    &types.BranchTarget{Template: ".on-true"},
		},
	}
	store.workflows[wfB.ID] = wfB

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()

	// Launch branch step for workflow A
	err := orch.handleBranch(ctx, wfA, wfA.Steps["monitor"])
	if err != nil {
		t.Fatalf("handleBranch for wfA error = %v", err)
	}

	// Launch branch step for workflow B (same step ID "monitor")
	err = orch.handleBranch(ctx, wfB, wfB.Steps["monitor"])
	if err != nil {
		t.Fatalf("handleBranch for wfB error = %v", err)
	}

	// Count entries in pendingCommands - should be 2 (one per workflow)
	// Currently (buggy): both store under key "monitor", so wfB overwrites wfA's cancel
	entryCount := 0
	orch.pendingCommands.Range(func(key, value any) bool {
		entryCount++
		return true
	})

	if entryCount != 2 {
		t.Errorf("pendingCommands has %d entries, want 2 (one per workflow); "+
			"step ID collision caused overwrite", entryCount)
	}

	// Cleanup: cancel all pending commands and wait
	orch.cancelPendingCommands()
	orch.wg.Wait()
}

// TestOrchestrator_HandleStepDone_IgnoredDuringCleanup verifies that
// HandleStepDone returns early when the workflow is in cleaning_up state.
// Bug: meow-rywz - HandleStepDone can mutate workflow during RunCleanup.
func TestOrchestrator_HandleStepDone_IgnoredDuringCleanup(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow in cleaning_up state with a running agent step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusCleaningUp
	wf.PriorStatus = types.RunStatusDone

	startedAt := time.Now().Add(-1 * time.Second)
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusRunning,
		StartedAt: &startedAt,
		Agent:     &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.HandleStepDone(ctx, &ipc.StepDoneMessage{
		Workflow: wf.ID,
		Step:     "agent-step",
		Agent:    "test-agent",
		Outputs:  map[string]any{},
	})

	// Currently (buggy): HandleStepDone succeeds and mutates the step to complete,
	// racing with RunCleanup which is concurrently saving the workflow.
	// After fix: should return nil (ignored) without modifying step state.
	if err != nil {
		t.Fatalf("HandleStepDone error = %v (should succeed silently when cleaning up)", err)
	}

	// Step should NOT be completed - it should remain running
	step := wf.Steps["agent-step"]
	if step.Status != types.StepStatusRunning {
		t.Errorf("Step status = %v, want %v; HandleStepDone should not mutate "+
			"workflow during cleanup", step.Status, types.StepStatusRunning)
	}
}

// TestOrchestrator_HandleStepDone_IgnoredWhenTerminal verifies that
// HandleStepDone returns early when the workflow is in a terminal state.
// Bug: meow-rywz - late step-done messages after cleanup can cause issues.
func TestOrchestrator_HandleStepDone_IgnoredWhenTerminal(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow in terminal (done) state with a step still marked running
	// (this can happen if cleanup killed the agent before it completed)
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusDone
	now := time.Now()
	wf.DoneAt = &now

	startedAt := time.Now().Add(-1 * time.Second)
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusRunning,
		StartedAt: &startedAt,
		Agent:     &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.HandleStepDone(ctx, &ipc.StepDoneMessage{
		Workflow: wf.ID,
		Step:     "agent-step",
		Agent:    "test-agent",
		Outputs:  map[string]any{},
	})

	// After fix: should return nil (ignored) without modifying step state
	if err != nil {
		t.Fatalf("HandleStepDone error = %v (should succeed silently when terminal)", err)
	}

	// Step should NOT be completed
	step := wf.Steps["agent-step"]
	if step.Status != types.StepStatusRunning {
		t.Errorf("Step status = %v, want %v; HandleStepDone should not mutate "+
			"workflow in terminal state", step.Status, types.StepStatusRunning)
	}
}

// TestOrchestrator_RunCleanup_MutexSafety verifies that RunCleanup persists
// the cleaning_up status so concurrent HandleStepDone calls see it.
// Bug: meow-rywz - RunCleanup doesn't hold mutex, allowing concurrent mutations.
func TestOrchestrator_RunCleanup_MutexSafety(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning
	wf.CleanupOnSuccess = "echo cleanup"

	startedAt := time.Now().Add(-1 * time.Second)
	wf.Steps["agent-step"] = &types.Step{
		ID:        "agent-step",
		Executor:  types.ExecutorAgent,
		Status:    types.StepStatusRunning,
		StartedAt: &startedAt,
		Agent:     &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	store.workflows[wf.ID] = wf
	agents.running["test-agent"] = true

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()

	// Run cleanup in a goroutine (simulating processWorkflow calling it)
	done := make(chan error, 1)
	go func() {
		done <- orch.RunCleanup(ctx, wf, types.RunStatusDone)
	}()

	// Give RunCleanup a moment to start, then try HandleStepDone
	time.Sleep(50 * time.Millisecond)

	// The workflow in the store should now show cleaning_up status
	// (RunCleanup should have persisted it under the mutex before continuing to I/O)
	storedWf, _ := store.Get(ctx, wf.ID)
	if storedWf.Status != types.RunStatusCleaningUp && storedWf.Status != types.RunStatusDone {
		t.Errorf("Workflow status during cleanup = %v, want cleaning_up or done", storedWf.Status)
	}

	err := <-done
	if err != nil {
		t.Fatalf("RunCleanup error = %v", err)
	}

	// Final state should be terminal
	if !wf.Status.IsTerminal() {
		t.Errorf("Final workflow status = %v, want terminal", wf.Status)
	}
}

func TestOrchestrator_AgentInjectionFailure_DispatchErrorFallback(t *testing.T) {
	store := newMockRunStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a pending agent step
	wf := types.NewRun("test-wf", "test-template", nil)
	wf.Status = types.RunStatusRunning

	wf.Steps["agent-step"] = &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusPending,
		Agent:    &types.AgentConfig{Agent: "test-agent", Prompt: "Do work"},
	}
	store.workflows[wf.ID] = wf

	// Agent is dead and InjectPrompt will fail
	// This tests the dispatch error fallback in processWorkflow:
	// when dispatch returns an error and the step is in running state,
	// it should be failed regardless of executor type.
	agents.running["test-agent"] = false
	agents.injectErr = fmt.Errorf("injection failed")

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.processWorkflow(ctx, wf)
	if err != nil {
		t.Fatalf("processWorkflow error = %v", err)
	}

	// After dispatch error, the step that was transitioned to running by handleAgent
	// should be failed by the dispatch error handler (defense-in-depth).
	// Currently, processWorkflow only fails orchestrator executors on dispatch error.
	// Agent steps are NOT orchestrator executors, so they get stuck in running.
	step := wf.Steps["agent-step"]
	if step.Status != types.StepStatusFailed {
		t.Errorf("Agent step status = %v, want %v (dispatch error should fail running agent steps)",
			step.Status, types.StepStatusFailed)
	}

	// Error should be recorded
	if step.Error == nil {
		t.Error("Agent step should have an error recorded after dispatch failure")
	}
}
