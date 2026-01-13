package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

// mockAgentStarter is a mock implementation of AgentStarter for testing.
type mockAgentStarter struct {
	startErr     error
	startCalled  bool
	lastStartCfg *AgentStartConfig
}

func (m *mockAgentStarter) Start(ctx context.Context, cfg *AgentStartConfig) error {
	m.startCalled = true
	m.lastStartCfg = cfg
	return m.startErr
}

func TestExecuteSpawn_Basic(t *testing.T) {
	starter := &mockAgentStarter{}
	step := &types.Step{
		ID:       "test-spawn",
		Executor: types.ExecutorSpawn,
		Spawn: &types.SpawnConfig{
			Agent:   "worker-1",
			Workdir: "/path/to/work",
		},
	}

	result, stepErr := ExecuteSpawn(context.Background(), step, "run-123", starter)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if !starter.startCalled {
		t.Error("expected Start to be called")
	}

	if result.TmuxSession != "meow-run-123-worker-1" {
		t.Errorf("expected session 'meow-run-123-worker-1', got %q", result.TmuxSession)
	}

	if starter.lastStartCfg.AgentID != "worker-1" {
		t.Errorf("expected agent 'worker-1', got %q", starter.lastStartCfg.AgentID)
	}

	if starter.lastStartCfg.Workdir != "/path/to/work" {
		t.Errorf("expected workdir '/path/to/work', got %q", starter.lastStartCfg.Workdir)
	}
}

func TestExecuteSpawn_EnvironmentVariables(t *testing.T) {
	starter := &mockAgentStarter{}
	step := &types.Step{
		ID:       "test-env",
		Executor: types.ExecutorSpawn,
		Spawn: &types.SpawnConfig{
			Agent: "worker-1",
			Env: map[string]string{
				"MY_VAR":    "custom_value",
				"TASK_ID":   "task-456",
				"MEOW_TEST": "should-be-kept", // Non-reserved var
			},
		},
	}

	result, stepErr := ExecuteSpawn(context.Background(), step, "run-123", starter)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result == nil {
		t.Fatal("expected result")
	}

	cfg := starter.lastStartCfg

	// User vars should be present
	if cfg.Env["MY_VAR"] != "custom_value" {
		t.Errorf("expected MY_VAR 'custom_value', got %q", cfg.Env["MY_VAR"])
	}
	if cfg.Env["TASK_ID"] != "task-456" {
		t.Errorf("expected TASK_ID 'task-456', got %q", cfg.Env["TASK_ID"])
	}

	// Orchestrator-injected vars must be present
	if cfg.Env["MEOW_AGENT"] != "worker-1" {
		t.Errorf("expected MEOW_AGENT 'worker-1', got %q", cfg.Env["MEOW_AGENT"])
	}
	if cfg.Env["MEOW_WORKFLOW"] != "run-123" {
		t.Errorf("expected MEOW_WORKFLOW 'wf-123', got %q", cfg.Env["MEOW_WORKFLOW"])
	}
}

func TestExecuteSpawn_OrchestratorEnvOverridesUser(t *testing.T) {
	starter := &mockAgentStarter{}
	step := &types.Step{
		ID:       "test-override",
		Executor: types.ExecutorSpawn,
		Spawn: &types.SpawnConfig{
			Agent: "worker-1",
			Env: map[string]string{
				// User tries to set reserved vars - should be overridden
				"MEOW_AGENT":    "hacker",
				"MEOW_WORKFLOW": "malicious",
			},
		},
	}

	_, stepErr := ExecuteSpawn(context.Background(), step, "run-123", starter)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	cfg := starter.lastStartCfg

	// Orchestrator values must override user values
	if cfg.Env["MEOW_AGENT"] != "worker-1" {
		t.Errorf("MEOW_AGENT should be 'worker-1', got %q", cfg.Env["MEOW_AGENT"])
	}
	if cfg.Env["MEOW_WORKFLOW"] != "run-123" {
		t.Errorf("MEOW_WORKFLOW should be 'wf-123', got %q", cfg.Env["MEOW_WORKFLOW"])
	}
}

func TestExecuteSpawn_ResumeSession(t *testing.T) {
	starter := &mockAgentStarter{}
	step := &types.Step{
		ID:       "test-resume",
		Executor: types.ExecutorSpawn,
		Spawn: &types.SpawnConfig{
			Agent:         "worker-1",
			ResumeSession: "sess-xyz789",
		},
	}

	_, stepErr := ExecuteSpawn(context.Background(), step, "run-123", starter)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if starter.lastStartCfg.ResumeSession != "sess-xyz789" {
		t.Errorf("expected resume session 'sess-xyz789', got %q", starter.lastStartCfg.ResumeSession)
	}
}

func TestExecuteSpawn_MissingConfig(t *testing.T) {
	starter := &mockAgentStarter{}
	step := &types.Step{
		ID:       "test-missing",
		Executor: types.ExecutorSpawn,
		Spawn:    nil,
	}

	_, stepErr := ExecuteSpawn(context.Background(), step, "run-123", starter)
	if stepErr == nil {
		t.Fatal("expected error for missing config")
	}

	if stepErr.Message != "spawn step missing config" {
		t.Errorf("unexpected error message: %s", stepErr.Message)
	}

	if starter.startCalled {
		t.Error("Start should not be called when config is missing")
	}
}

func TestExecuteSpawn_MissingAgent(t *testing.T) {
	starter := &mockAgentStarter{}
	step := &types.Step{
		ID:       "test-no-agent",
		Executor: types.ExecutorSpawn,
		Spawn: &types.SpawnConfig{
			Agent: "", // Empty agent
		},
	}

	_, stepErr := ExecuteSpawn(context.Background(), step, "run-123", starter)
	if stepErr == nil {
		t.Fatal("expected error for missing agent")
	}

	if stepErr.Message != "spawn step missing agent field" {
		t.Errorf("unexpected error message: %s", stepErr.Message)
	}
}

func TestExecuteSpawn_StartError(t *testing.T) {
	starter := &mockAgentStarter{
		startErr: errors.New("tmux session creation failed"),
	}
	step := &types.Step{
		ID:       "test-error",
		Executor: types.ExecutorSpawn,
		Spawn: &types.SpawnConfig{
			Agent: "worker-1",
		},
	}

	_, stepErr := ExecuteSpawn(context.Background(), step, "run-123", starter)
	if stepErr == nil {
		t.Fatal("expected error from Start failure")
	}

	if stepErr.Message != "failed to start agent worker-1: tmux session creation failed" {
		t.Errorf("unexpected error message: %s", stepErr.Message)
	}
}

func TestExecuteSpawn_WorkflowIDInConfig(t *testing.T) {
	starter := &mockAgentStarter{}
	step := &types.Step{
		ID:       "test-wfid",
		Executor: types.ExecutorSpawn,
		Spawn: &types.SpawnConfig{
			Agent: "worker-1",
		},
	}

	_, stepErr := ExecuteSpawn(context.Background(), step, "workflow-abc", starter)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if starter.lastStartCfg.WorkflowID != "workflow-abc" {
		t.Errorf("expected workflow ID 'workflow-abc', got %q", starter.lastStartCfg.WorkflowID)
	}
}

func TestExecuteSpawn_SpawnArgs(t *testing.T) {
	starter := &mockAgentStarter{}
	step := &types.Step{
		ID:       "test-spawn-args",
		Executor: types.ExecutorSpawn,
		Spawn: &types.SpawnConfig{
			Agent:     "worker-1",
			SpawnArgs: "--model opus --verbose",
		},
	}

	result, stepErr := ExecuteSpawn(context.Background(), step, "run-123", starter)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result == nil {
		t.Fatal("expected result")
	}

	if !starter.startCalled {
		t.Error("expected Start to be called")
	}

	// Verify SpawnArgs is passed through
	if starter.lastStartCfg.SpawnArgs != "--model opus --verbose" {
		t.Errorf("expected SpawnArgs '--model opus --verbose', got %q", starter.lastStartCfg.SpawnArgs)
	}
}

func TestBuildTmuxSessionName(t *testing.T) {
	tests := []struct {
		workflowID string
		agentID    string
		expected   string
	}{
		{"run-123", "worker-1", "meow-run-123-worker-1"},
		{"abc", "def", "meow-abc-def"},
		{"workflow-with-dashes", "agent_with_underscore", "meow-workflow-with-dashes-agent_with_underscore"},
	}

	for _, tc := range tests {
		result := BuildTmuxSessionName(tc.workflowID, tc.agentID)
		if result != tc.expected {
			t.Errorf("BuildTmuxSessionName(%q, %q) = %q, expected %q",
				tc.workflowID, tc.agentID, result, tc.expected)
		}
	}
}
