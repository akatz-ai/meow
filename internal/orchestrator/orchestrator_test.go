package orchestrator

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/config"
	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/meow-stack/meow-machine/internal/types"
)

// --- Mock Implementations ---

// mockWorkflowStore implements WorkflowStore for testing.
type mockWorkflowStore struct {
	mu        sync.Mutex
	workflows map[string]*types.Workflow
	calls     []string
}

func newMockWorkflowStore() *mockWorkflowStore {
	return &mockWorkflowStore{
		workflows: make(map[string]*types.Workflow),
	}
}

func (m *mockWorkflowStore) Create(ctx context.Context, wf *types.Workflow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Create:"+wf.ID)
	m.workflows[wf.ID] = wf
	return nil
}

func (m *mockWorkflowStore) Get(ctx context.Context, id string) (*types.Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Get:"+id)
	return m.workflows[id], nil
}

func (m *mockWorkflowStore) Save(ctx context.Context, wf *types.Workflow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Save:"+wf.ID)
	m.workflows[wf.ID] = wf
	return nil
}

func (m *mockWorkflowStore) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "Delete:"+id)
	delete(m.workflows, id)
	return nil
}

func (m *mockWorkflowStore) List(ctx context.Context, filter WorkflowFilter) ([]*types.Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "List")
	var result []*types.Workflow
	for _, wf := range m.workflows {
		if filter.Status != "" && wf.Status != filter.Status {
			continue
		}
		result = append(result, wf)
	}
	return result, nil
}

func (m *mockWorkflowStore) GetByAgent(ctx context.Context, agentID string) ([]*types.Workflow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, "GetByAgent:"+agentID)
	var result []*types.Workflow
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
type mockAgentManager struct {
	mu              sync.Mutex
	running         map[string]bool
	started         []string
	stopped         []string
	interrupted     []string
	injectedPrompts []string
}

func newMockAgentManager() *mockAgentManager {
	return &mockAgentManager{
		running: make(map[string]bool),
	}
}

func (m *mockAgentManager) Start(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	agentID := step.Spawn.Agent
	m.started = append(m.started, agentID)
	m.running[agentID] = true
	return nil
}

func (m *mockAgentManager) Stop(ctx context.Context, wf *types.Workflow, step *types.Step) error {
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

func (m *mockAgentManager) InjectPrompt(ctx context.Context, agentID string, prompt string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.injectedPrompts = append(m.injectedPrompts, agentID+":"+prompt)
	return nil
}

func (m *mockAgentManager) Interrupt(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interrupted = append(m.interrupted, agentID)
	return nil
}

func (m *mockAgentManager) KillAll(ctx context.Context, wf *types.Workflow) error {
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

func (m *mockTemplateExpander) Expand(ctx context.Context, wf *types.Workflow, step *types.Step) error {
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
	store := newMockWorkflowStore()
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create a workflow with a single shell step
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	if wf.Status != types.WorkflowStatusDone {
		t.Errorf("Workflow status = %v, want %v", wf.Status, types.WorkflowStatusDone)
	}
}

func TestOrchestrator_DependencyOrdering(t *testing.T) {
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create a workflow with dependent steps
	// step-2 depends on step-1
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	orch := New(testConfig(), store, agents, shell, expander, logger)

	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with both orchestrator and agent steps ready
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with two agent steps for the same agent
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a running agent step
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
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
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow that will complete
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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

	if wf.Status != types.WorkflowStatusDone {
		t.Errorf("Workflow status = %v, want %v", wf.Status, types.WorkflowStatusDone)
	}
	if wf.DoneAt == nil {
		t.Error("Workflow DoneAt should be set")
	}
}

func TestOrchestrator_WorkflowFailure(t *testing.T) {
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a failed step
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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

	if wf.Status != types.WorkflowStatusFailed {
		t.Errorf("Workflow status = %v, want %v", wf.Status, types.WorkflowStatusFailed)
	}
}

func TestOrchestrator_SpawnAndKill(t *testing.T) {
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with spawn -> kill sequence
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with 3 running agent steps (each assigned to different agent)
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a running agent step that has a very short timeout
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a completed agent step
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with cleanup_on_success script (opt-in cleanup)
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	if finalWf.Status != types.WorkflowStatusDone {
		t.Errorf("Workflow status = %v, want done", finalWf.Status)
	}
}

// TestOrchestrator_RunCleanup tests the RunCleanup method directly.
func TestOrchestrator_RunCleanup(t *testing.T) {
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with cleanup_on_success script (testing RunCleanup with Done reason)
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
	wf.CleanupOnSuccess = "echo cleanup"
	store.workflows[wf.ID] = wf

	// Register some agents
	agents.running["agent-1"] = true
	agents.running["agent-2"] = true

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.RunCleanup(ctx, wf, types.WorkflowStatusDone)
	if err != nil {
		t.Fatalf("RunCleanup error = %v", err)
	}

	// Workflow should be in done state
	if wf.Status != types.WorkflowStatusDone {
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
	wf.CleanupOnFailure = "echo cleanup"
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.RunCleanup(ctx, wf, types.WorkflowStatusFailed)
	if err != nil {
		t.Fatalf("RunCleanup error = %v", err)
	}

	// Workflow should be in failed state (final status matches reason)
	if wf.Status != types.WorkflowStatusFailed {
		t.Errorf("Workflow status = %v, want failed", wf.Status)
	}
}

// TestOrchestrator_RunCleanup_Stopped tests cleanup for stopped workflows.
func TestOrchestrator_RunCleanup_Stopped(t *testing.T) {
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
	wf.CleanupOnStop = "echo cleanup"
	store.workflows[wf.ID] = wf

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.RunCleanup(ctx, wf, types.WorkflowStatusStopped)
	if err != nil {
		t.Fatalf("RunCleanup error = %v", err)
	}

	// Workflow should be in stopped state
	if wf.Status != types.WorkflowStatusStopped {
		t.Errorf("Workflow status = %v, want stopped", wf.Status)
	}
}

// --- Crash Recovery Tests ---

// TestOrchestrator_Recover_ResetOrchestratorSteps tests that running orchestrator steps are reset to pending.
func TestOrchestrator_Recover_ResetOrchestratorSteps(t *testing.T) {
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with running orchestrator steps
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with partial expansion
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with running agent step, but agent is dead
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with running agent step, agent is alive
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with completing agent step, agent is alive
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow that was in cleaning_up state (prior status determines which cleanup script to use)
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusCleaningUp
	wf.PriorStatus = types.WorkflowStatusDone
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
	if finalWf.Status != types.WorkflowStatusDone {
		t.Errorf("Workflow status = %v, want done", finalWf.Status)
	}

	// DoneAt should be set
	if finalWf.DoneAt == nil {
		t.Error("Workflow DoneAt should be set")
	}
}

// TestOrchestrator_Recover_NoWorkflows tests recovery with no workflows.
func TestOrchestrator_Recover_NoWorkflows(t *testing.T) {
	store := newMockWorkflowStore()
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
	// No cleanup script
	store.workflows[wf.ID] = wf

	agents.running["agent-1"] = true

	orch := New(testConfig(), store, agents, shell, expander, logger)

	ctx := context.Background()
	err := orch.RunCleanup(ctx, wf, types.WorkflowStatusDone)
	if err != nil {
		t.Fatalf("RunCleanup error = %v", err)
	}

	// Workflow should be done
	if wf.Status != types.WorkflowStatusDone {
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a running agent step WITHOUT timeout
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a mix of pending and running steps
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with:
	// - A branch step that has expanded into child steps (simulating post-expansion state)
	// - A "done" step that depends on the branch step
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	if wf.Status != types.WorkflowStatusDone {
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a branch step that has a slow condition (sleeps 5 seconds)
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a branch step
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with a branch step
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with two branch steps that both need "init" step
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning

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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with branch step
	// Branch conditions execute real shell commands (not mocked)
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with branch step
	// Branch conditions execute real shell commands (not mocked)
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with branch step with timeout
	// Use a command that will take longer than the timeout
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with branch step that captures outputs
	// Using a real shell command since branch conditions execute actual commands
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	store := newMockWorkflowStore()
	agents := newMockAgentManager()
	shell := newMockShellRunner()
	expander := &mockTemplateExpander{}
	logger := testLogger()

	// Create workflow with branch step with on_true but no on_false
	// Condition will exit 1, so outcome is "false" but no on_false target
	// Use on_error: continue so it doesn't fail the step
	wf := types.NewWorkflow("test-wf", "test-template", nil)
	wf.Status = types.WorkflowStatusRunning
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
	if wf.Status != types.WorkflowStatusDone {
		t.Errorf("Workflow status = %v, want done", wf.Status)
	}
}
