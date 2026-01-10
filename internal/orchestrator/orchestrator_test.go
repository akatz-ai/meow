package orchestrator

import (
	"context"
	"log/slog"
	"os"
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

	// Check step was executed
	if len(shell.executed) != 1 || shell.executed[0] != "echo hello" {
		t.Errorf("Expected shell command 'echo hello' to be executed, got %v", shell.executed)
	}

	// Check step is done
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
		Shell:    &types.ShellConfig{Command: "first"},
	}
	wf.Steps["step-2"] = &types.Step{
		ID:       "step-2",
		Executor: types.ExecutorShell,
		Status:   types.StepStatusPending,
		Needs:    []string{"step-1"},
		Shell:    &types.ShellConfig{Command: "second"},
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

	// Check both commands were executed in order
	if len(shell.executed) != 2 {
		t.Fatalf("Expected 2 commands executed, got %d", len(shell.executed))
	}
	if shell.executed[0] != "first" || shell.executed[1] != "second" {
		t.Errorf("Commands executed in wrong order: %v", shell.executed)
	}
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
			// Branch executor is not implemented yet, but all others should work
			if tc.executor == types.ExecutorBranch {
				if err == nil {
					t.Error("Expected branch executor to return not implemented error")
				}
			} else {
				if err != nil {
					t.Errorf("dispatch(%s) error = %v", tc.executor, err)
				}
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

	// Shell (orchestrator executor) should be dispatched before agent
	// Check shell was executed
	if len(shell.executed) != 1 {
		t.Fatalf("Expected 1 shell command, got %d", len(shell.executed))
	}

	// Both steps should now have been dispatched
	// Shell step should be done (orchestrator executors complete immediately)
	if wf.Steps["shell-step"].Status != types.StepStatusDone {
		t.Errorf("Shell step status = %v, want %v", wf.Steps["shell-step"].Status, types.StepStatusDone)
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
