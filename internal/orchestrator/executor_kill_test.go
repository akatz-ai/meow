package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

// mockAgentStopper is a mock implementation of AgentStopper for testing.
type mockAgentStopper struct {
	stopErr      error
	isRunningVal bool
	isRunningErr error
	stopCalled   bool
	lastStopCfg  *AgentStopConfig
}

func (m *mockAgentStopper) Stop(ctx context.Context, cfg *AgentStopConfig) error {
	m.stopCalled = true
	m.lastStopCfg = cfg
	return m.stopErr
}

func (m *mockAgentStopper) IsRunning(ctx context.Context, agentID string) (bool, error) {
	return m.isRunningVal, m.isRunningErr
}

func TestExecuteKill_Basic(t *testing.T) {
	stopper := &mockAgentStopper{isRunningVal: true}
	step := &types.Step{
		ID:       "test-kill",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent: "worker-1",
		},
	}

	result, stepErr := ExecuteKill(context.Background(), step, "wf-123", stopper)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if !stopper.stopCalled {
		t.Error("expected Stop to be called")
	}

	if stopper.lastStopCfg.AgentID != "worker-1" {
		t.Errorf("expected agent 'worker-1', got %q", stopper.lastStopCfg.AgentID)
	}

	if result.WasRunning != true {
		t.Error("expected WasRunning to be true")
	}
}

func TestExecuteKill_DefaultTimeout(t *testing.T) {
	stopper := &mockAgentStopper{}
	step := &types.Step{
		ID:       "test-timeout",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent:   "worker-1",
			Timeout: 0, // Unset, should default
		},
	}

	_, stepErr := ExecuteKill(context.Background(), step, "wf-123", stopper)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if stopper.lastStopCfg.Timeout != DefaultKillTimeout {
		t.Errorf("expected default timeout %d, got %d", DefaultKillTimeout, stopper.lastStopCfg.Timeout)
	}
}

func TestExecuteKill_CustomTimeout(t *testing.T) {
	stopper := &mockAgentStopper{}
	step := &types.Step{
		ID:       "test-custom-timeout",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent:   "worker-1",
			Timeout: 30,
		},
	}

	_, stepErr := ExecuteKill(context.Background(), step, "wf-123", stopper)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if stopper.lastStopCfg.Timeout != 30 {
		t.Errorf("expected timeout 30, got %d", stopper.lastStopCfg.Timeout)
	}
}

func TestExecuteKill_GracefulTrue(t *testing.T) {
	stopper := &mockAgentStopper{}
	step := &types.Step{
		ID:       "test-graceful-true",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent:    "worker-1",
			Graceful: true,
		},
	}

	_, stepErr := ExecuteKill(context.Background(), step, "wf-123", stopper)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if !stopper.lastStopCfg.Graceful {
		t.Error("expected Graceful to be true")
	}
}

func TestExecuteKill_GracefulFalse(t *testing.T) {
	stopper := &mockAgentStopper{}
	step := &types.Step{
		ID:       "test-graceful-false",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent:    "worker-1",
			Graceful: false, // Explicit non-graceful
		},
	}

	_, stepErr := ExecuteKill(context.Background(), step, "wf-123", stopper)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// The value is passed through as-is
	if stopper.lastStopCfg.Graceful {
		t.Error("expected Graceful to be false when explicitly set")
	}
}

func TestExecuteKill_MissingConfig(t *testing.T) {
	stopper := &mockAgentStopper{}
	step := &types.Step{
		ID:       "test-missing",
		Executor: types.ExecutorKill,
		Kill:     nil,
	}

	_, stepErr := ExecuteKill(context.Background(), step, "wf-123", stopper)
	if stepErr == nil {
		t.Fatal("expected error for missing config")
	}

	if stepErr.Message != "kill step missing config" {
		t.Errorf("unexpected error message: %s", stepErr.Message)
	}

	if stopper.stopCalled {
		t.Error("Stop should not be called when config is missing")
	}
}

func TestExecuteKill_MissingAgent(t *testing.T) {
	stopper := &mockAgentStopper{}
	step := &types.Step{
		ID:       "test-no-agent",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent: "", // Empty agent
		},
	}

	_, stepErr := ExecuteKill(context.Background(), step, "wf-123", stopper)
	if stepErr == nil {
		t.Fatal("expected error for missing agent")
	}

	if stepErr.Message != "kill step missing agent field" {
		t.Errorf("unexpected error message: %s", stepErr.Message)
	}
}

func TestExecuteKill_StopError(t *testing.T) {
	stopper := &mockAgentStopper{
		stopErr: errors.New("tmux session not found"),
	}
	step := &types.Step{
		ID:       "test-error",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent: "worker-1",
		},
	}

	_, stepErr := ExecuteKill(context.Background(), step, "wf-123", stopper)
	if stepErr == nil {
		t.Fatal("expected error from Stop failure")
	}

	if stepErr.Message != "failed to stop agent worker-1: tmux session not found" {
		t.Errorf("unexpected error message: %s", stepErr.Message)
	}
}

func TestExecuteKill_AgentNotRunning(t *testing.T) {
	stopper := &mockAgentStopper{
		isRunningVal: false,
	}
	step := &types.Step{
		ID:       "test-not-running",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent: "worker-1",
		},
	}

	result, stepErr := ExecuteKill(context.Background(), step, "wf-123", stopper)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Should still call Stop (the stopper will handle the "not running" case)
	if !stopper.stopCalled {
		t.Error("expected Stop to be called even if agent not running")
	}

	if result.WasRunning {
		t.Error("expected WasRunning to be false")
	}
}

func TestExecuteKill_IsRunningError(t *testing.T) {
	stopper := &mockAgentStopper{
		isRunningErr: errors.New("could not check status"),
	}
	step := &types.Step{
		ID:       "test-isrunning-err",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent: "worker-1",
		},
	}

	result, stepErr := ExecuteKill(context.Background(), step, "wf-123", stopper)
	if stepErr != nil {
		t.Fatalf("should not fail on IsRunning error: %v", stepErr)
	}

	// IsRunning error is informational, should not prevent kill
	if !stopper.stopCalled {
		t.Error("expected Stop to be called despite IsRunning error")
	}

	// WasRunning should default to false on error
	if result.WasRunning {
		t.Error("expected WasRunning to be false when IsRunning errors")
	}
}

func TestExecuteKill_WorkflowID(t *testing.T) {
	stopper := &mockAgentStopper{}
	step := &types.Step{
		ID:       "test-wfid",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent: "worker-1",
		},
	}

	_, stepErr := ExecuteKill(context.Background(), step, "workflow-xyz", stopper)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if stopper.lastStopCfg.WorkflowID != "workflow-xyz" {
		t.Errorf("expected workflow ID 'workflow-xyz', got %q", stopper.lastStopCfg.WorkflowID)
	}
}

func TestExecuteKill_AllFieldsPassed(t *testing.T) {
	stopper := &mockAgentStopper{isRunningVal: true}
	step := &types.Step{
		ID:       "test-all-fields",
		Executor: types.ExecutorKill,
		Kill: &types.KillConfig{
			Agent:    "worker-1",
			Graceful: true,
			Timeout:  45,
		},
	}

	result, stepErr := ExecuteKill(context.Background(), step, "wf-abc", stopper)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	cfg := stopper.lastStopCfg
	if cfg.AgentID != "worker-1" {
		t.Errorf("AgentID: expected 'worker-1', got %q", cfg.AgentID)
	}
	if cfg.WorkflowID != "wf-abc" {
		t.Errorf("WorkflowID: expected 'wf-abc', got %q", cfg.WorkflowID)
	}
	if cfg.Graceful != true {
		t.Error("Graceful: expected true")
	}
	if cfg.Timeout != 45 {
		t.Errorf("Timeout: expected 45, got %d", cfg.Timeout)
	}
	if !result.WasRunning {
		t.Error("WasRunning: expected true")
	}
}
