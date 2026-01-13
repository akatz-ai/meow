package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// mockConditionExecutor is a mock implementation of ConditionExecutor.
type mockConditionExecutor struct {
	exitCode int
	stdout   string
	stderr   string
	execErr  error
	delay    time.Duration // Simulate slow execution
}

func (m *mockConditionExecutor) Execute(ctx context.Context, command string) (int, string, string, error) {
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-ctx.Done():
			return 1, "", "", ctx.Err()
		}
	}
	return m.exitCode, m.stdout, m.stderr, m.execErr
}

func TestExecuteBranch_TrueCondition(t *testing.T) {
	condExec := &mockConditionExecutor{exitCode: 0}
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{ID: "success", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo success"}},
		},
	}

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "test -f file.txt",
			OnTrue: &types.BranchTarget{
				Template: ".on-true",
			},
			OnFalse: &types.BranchTarget{
				Template: ".on-false",
			},
		},
	}

	result, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outcome != BranchOutcomeTrue {
		t.Errorf("expected outcome 'true', got %q", result.Outcome)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if len(result.ExpandedSteps) != 1 {
		t.Errorf("expected 1 expanded step, got %d", len(result.ExpandedSteps))
	}
	if result.ExpandedSteps[0].ID != "branch.success" {
		t.Errorf("expected step ID 'branch.success', got %q", result.ExpandedSteps[0].ID)
	}
}

func TestExecuteBranch_FalseCondition(t *testing.T) {
	condExec := &mockConditionExecutor{exitCode: 1}
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{ID: "failure", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo failure"}},
		},
	}

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "test -f nonexistent.txt",
			OnTrue: &types.BranchTarget{
				Template: ".on-true",
			},
			OnFalse: &types.BranchTarget{
				Template: ".on-false",
			},
		},
	}

	result, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outcome != BranchOutcomeFalse {
		t.Errorf("expected outcome 'false', got %q", result.Outcome)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
}

func TestExecuteBranch_Timeout(t *testing.T) {
	condExec := &mockConditionExecutor{
		delay: 100 * time.Millisecond, // Takes 100ms
	}
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{ID: "timeout-step", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo timeout"}},
		},
	}

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "sleep 10",
			Timeout:   "50ms", // 50ms timeout
			OnTrue: &types.BranchTarget{
				Template: ".on-true",
			},
			OnTimeout: &types.BranchTarget{
				Template: ".on-timeout",
			},
		},
	}

	result, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outcome != BranchOutcomeTimeout {
		t.Errorf("expected outcome 'timeout', got %q", result.Outcome)
	}
}

func TestExecuteBranch_TimeoutFallsBackToFalse(t *testing.T) {
	condExec := &mockConditionExecutor{
		delay: 100 * time.Millisecond,
	}
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{ID: "false-step", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo false"}},
		},
	}

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "sleep 10",
			Timeout:   "50ms",
			OnTrue: &types.BranchTarget{
				Template: ".on-true",
			},
			// No OnTimeout - should fall back to OnFalse
			OnFalse: &types.BranchTarget{
				Template: ".on-false",
			},
		},
	}

	result, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Outcome is timeout, but target is on_false
	if result.Outcome != BranchOutcomeTimeout {
		t.Errorf("expected outcome 'timeout', got %q", result.Outcome)
	}
	// Should have expanded the false template
	if len(result.ExpandedSteps) != 1 {
		t.Errorf("expected 1 expanded step (from on_false), got %d", len(result.ExpandedSteps))
	}
}

func TestExecuteBranch_NoTargetForOutcome(t *testing.T) {
	condExec := &mockConditionExecutor{exitCode: 0}
	loader := &mockTemplateLoader{}

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "true",
			// OnTrue is nil - no action on true
			OnFalse: &types.BranchTarget{
				Template: ".on-false",
			},
		},
	}

	result, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Outcome should still be 'true' even though there's no target
	// This preserves information about what the condition evaluated to
	if result.Outcome != BranchOutcomeTrue {
		t.Errorf("expected outcome 'true' (condition passed), got %q", result.Outcome)
	}
	if len(result.ExpandedSteps) != 0 {
		t.Errorf("expected 0 expanded steps, got %d", len(result.ExpandedSteps))
	}
	if result.Target != nil {
		t.Errorf("expected nil target, got %v", result.Target)
	}
}

func TestExecuteBranch_InlineSteps(t *testing.T) {
	condExec := &mockConditionExecutor{exitCode: 0}
	loader := &mockTemplateLoader{} // Not used for inline

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "true",
			OnTrue: &types.BranchTarget{
				Inline: []types.InlineStep{
					{
						ID:       "notify",
						Executor: types.ExecutorAgent,
						Agent:    "worker",
						Prompt:   "Notify success",
					},
					{
						ID:       "cleanup",
						Executor: types.ExecutorAgent,
						Agent:    "worker",
						Prompt:   "Clean up",
						Needs:    []string{"notify"},
					},
				},
			},
		},
	}

	result, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if len(result.ExpandedSteps) != 2 {
		t.Fatalf("expected 2 inline steps, got %d", len(result.ExpandedSteps))
	}

	// Check IDs are prefixed
	if result.ExpandedSteps[0].ID != "branch.notify" {
		t.Errorf("expected 'branch.notify', got %q", result.ExpandedSteps[0].ID)
	}
	if result.ExpandedSteps[1].ID != "branch.cleanup" {
		t.Errorf("expected 'branch.cleanup', got %q", result.ExpandedSteps[1].ID)
	}

	// Check dependencies are prefixed
	if len(result.ExpandedSteps[1].Needs) != 1 || result.ExpandedSteps[1].Needs[0] != "branch.notify" {
		t.Errorf("expected cleanup to depend on 'branch.notify', got %v", result.ExpandedSteps[1].Needs)
	}
}

func TestExecuteBranch_VariableSubstitution(t *testing.T) {
	condExec := &mockConditionExecutor{exitCode: 0}
	loader := &mockTemplateLoader{} // Not used for inline

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "true",
			OnTrue: &types.BranchTarget{
				Variables: map[string]string{
					"target_agent": "worker-2",
				},
				Inline: []types.InlineStep{
					{
						ID:       "work",
						Executor: types.ExecutorAgent,
						Agent:    "{{target_agent}}",
						Prompt:   "Do {{task}}",
					},
				},
			},
		},
	}

	workflowVars := map[string]string{
		"task": "important-work",
	}

	result, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, workflowVars, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	expanded := result.ExpandedSteps[0]
	if expanded.Agent.Agent != "worker-2" {
		t.Errorf("expected agent 'worker-2', got %q", expanded.Agent.Agent)
	}
	if expanded.Agent.Prompt != "Do important-work" {
		t.Errorf("expected prompt 'Do important-work', got %q", expanded.Agent.Prompt)
	}
}

func TestExecuteBranch_MissingConfig(t *testing.T) {
	condExec := &mockConditionExecutor{}
	loader := &mockTemplateLoader{}

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch:   nil,
	}

	_, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr == nil {
		t.Fatal("expected error for missing config")
	}
	if stepErr.Message != "branch step missing config" {
		t.Errorf("unexpected error: %s", stepErr.Message)
	}
}

func TestExecuteBranch_MissingCondition(t *testing.T) {
	condExec := &mockConditionExecutor{}
	loader := &mockTemplateLoader{}

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "",
		},
	}

	_, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr == nil {
		t.Fatal("expected error for missing condition")
	}
	if stepErr.Message != "branch step missing condition field" {
		t.Errorf("unexpected error: %s", stepErr.Message)
	}
}

func TestExecuteBranch_InvalidTimeout(t *testing.T) {
	condExec := &mockConditionExecutor{}
	loader := &mockTemplateLoader{}

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "true",
			Timeout:   "not-a-duration",
		},
	}

	_, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr == nil {
		t.Fatal("expected error for invalid timeout")
	}
	if stepErr.Message == "" {
		t.Error("expected error message")
	}
}

func TestExecuteBranch_ExecutionError(t *testing.T) {
	condExec := &mockConditionExecutor{
		execErr: errors.New("command not found"),
	}
	loader := &mockTemplateLoader{
		steps: []*types.Step{
			{ID: "error-step", Executor: types.ExecutorShell, Shell: &types.ShellConfig{Command: "echo error"}},
		},
	}

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "nonexistent-command",
			OnFalse: &types.BranchTarget{
				Template: ".on-false",
			},
		},
	}

	result, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("execution error should not return step error: %v", stepErr)
	}

	// Execution errors are treated as false
	if result.Outcome != BranchOutcomeFalse {
		t.Errorf("expected outcome 'false' on exec error, got %q", result.Outcome)
	}
}

func TestExecuteBranch_EmptyInline(t *testing.T) {
	condExec := &mockConditionExecutor{exitCode: 0}
	loader := &mockTemplateLoader{}

	step := &types.Step{
		ID:       "branch",
		Executor: types.ExecutorBranch,
		Branch: &types.BranchConfig{
			Condition: "true",
			OnTrue: &types.BranchTarget{
				Inline: []types.InlineStep{}, // Empty inline
			},
		},
	}

	result, stepErr := ExecuteBranch(context.Background(), step, condExec, loader, nil, 0, nil)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Empty inline is valid - just no steps
	if len(result.ExpandedSteps) != 0 {
		t.Errorf("expected 0 steps for empty inline, got %d", len(result.ExpandedSteps))
	}
}

func TestSimpleConditionExecutor_Success(t *testing.T) {
	exec := &SimpleConditionExecutor{}
	exitCode, stdout, _, err := exec.Execute(context.Background(), "echo hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 0 {
		t.Errorf("expected exit code 0, got %d", exitCode)
	}
	if stdout != "hello" {
		t.Errorf("expected stdout 'hello', got %q", stdout)
	}
}

func TestSimpleConditionExecutor_NonZeroExit(t *testing.T) {
	exec := &SimpleConditionExecutor{}
	exitCode, _, _, err := exec.Execute(context.Background(), "exit 42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exitCode != 42 {
		t.Errorf("expected exit code 42, got %d", exitCode)
	}
}

func TestSimpleConditionExecutor_Timeout(t *testing.T) {
	// Use a command that respects context cancellation
	// Note: sleep in a subshell may not respect SIGKILL on all platforms
	exec := &SimpleConditionExecutor{}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Use a bash command that's more likely to be killed
	_, _, _, err := exec.Execute(ctx, "while true; do :; done")

	// The key assertion is that we get an error (context deadline exceeded)
	// Timing may vary by platform
	if err == nil {
		t.Fatal("expected timeout error")
	}
	// Context should be cancelled
	if ctx.Err() != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", ctx.Err())
	}
}

func TestSimpleConditionExecutor_InjectsMeowVars(t *testing.T) {
	exec := &SimpleConditionExecutor{
		SocketPath: "/tmp/meow-test.sock",
		WorkflowID: "run-123",
		StepID:     "step-456",
	}

	// Command that prints specific MEOW_* environment variables we set
	_, stdout, _, err := exec.Execute(context.Background(),
		"echo \"MEOW_WORKFLOW=$MEOW_WORKFLOW MEOW_STEP=$MEOW_STEP MEOW_ORCH_SOCK=$MEOW_ORCH_SOCK\"")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "MEOW_WORKFLOW=run-123 MEOW_STEP=step-456 MEOW_ORCH_SOCK=/tmp/meow-test.sock"
	if stdout != expected {
		t.Errorf("expected MEOW_* vars: %q, got: %q", expected, stdout)
	}
}
