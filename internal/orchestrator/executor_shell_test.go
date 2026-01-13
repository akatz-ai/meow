package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

func TestExecuteShell_BasicCommand(t *testing.T) {
	step := &types.Step{
		ID:       "test-basic",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "echo hello",
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Stdout != "hello" {
		t.Errorf("expected stdout 'hello', got %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
}

func TestExecuteShell_CaptureStdout(t *testing.T) {
	step := &types.Step{
		ID:       "test-stdout",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "echo 'test output'",
			Outputs: map[string]types.OutputSource{
				"message": {Source: "stdout"},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outputs["message"] != "test output" {
		t.Errorf("expected output 'test output', got %q", result.Outputs["message"])
	}
}

func TestExecuteShell_CaptureStderr(t *testing.T) {
	step := &types.Step{
		ID:       "test-stderr",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "echo 'error message' >&2",
			Outputs: map[string]types.OutputSource{
				"error_msg": {Source: "stderr"},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outputs["error_msg"] != "error message" {
		t.Errorf("expected stderr 'error message', got %q", result.Outputs["error_msg"])
	}
}

func TestExecuteShell_CaptureExitCode(t *testing.T) {
	step := &types.Step{
		ID:       "test-exitcode",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "exit 42",
			OnError: "continue", // Don't fail so we can check the code
			Outputs: map[string]types.OutputSource{
				"code": {Source: "exit_code"},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outputs["code"] != 42 {
		t.Errorf("expected exit code 42, got %v", result.Outputs["code"])
	}
	if result.ExitCode != 42 {
		t.Errorf("expected result.ExitCode 42, got %d", result.ExitCode)
	}
}

func TestExecuteShell_CaptureFile(t *testing.T) {
	// Create a temp file with content
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "output.txt")
	if err := os.WriteFile(filePath, []byte("file content\n"), 0644); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	step := &types.Step{
		ID:       "test-file",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "true",
			Outputs: map[string]types.OutputSource{
				"file_content": {Source: "file:" + filePath},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outputs["file_content"] != "file content" {
		t.Errorf("expected 'file content', got %q", result.Outputs["file_content"])
	}
}

func TestExecuteShell_WorkingDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	step := &types.Step{
		ID:       "test-workdir",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "pwd",
			Workdir: tmpDir,
			Outputs: map[string]types.OutputSource{
				"cwd": {Source: "stdout"},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outputs["cwd"] != tmpDir {
		t.Errorf("expected cwd %q, got %q", tmpDir, result.Outputs["cwd"])
	}
}

func TestExecuteShell_EnvironmentVariables(t *testing.T) {
	step := &types.Step{
		ID:       "test-env",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "echo $MY_VAR",
			Env: map[string]string{
				"MY_VAR": "custom_value",
			},
			Outputs: map[string]types.OutputSource{
				"var_value": {Source: "stdout"},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outputs["var_value"] != "custom_value" {
		t.Errorf("expected 'custom_value', got %q", result.Outputs["var_value"])
	}
}

func TestExecuteShell_OnErrorFail(t *testing.T) {
	step := &types.Step{
		ID:       "test-fail",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "exit 1",
			OnError: "fail", // Explicit fail (default)
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr == nil {
		t.Fatal("expected error but got nil")
	}

	if stepErr.Code != 1 {
		t.Errorf("expected error code 1, got %d", stepErr.Code)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected result exit code 1, got %d", result.ExitCode)
	}
}

func TestExecuteShell_OnErrorContinue(t *testing.T) {
	step := &types.Step{
		ID:       "test-continue",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "exit 1",
			OnError: "continue",
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("expected no StepError with on_error=continue, got: %v", stepErr)
	}

	// Error should be captured in outputs
	if result.Outputs["error"] == nil {
		t.Error("expected error in outputs with on_error=continue")
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1, got %d", result.ExitCode)
	}
}

func TestExecuteShell_MissingConfig(t *testing.T) {
	step := &types.Step{
		ID:       "test-missing",
		Executor: types.ExecutorShell,
		Shell:    nil, // Missing config
	}

	_, stepErr := ExecuteShell(context.Background(), step)
	if stepErr == nil {
		t.Fatal("expected error for missing config")
	}

	if stepErr.Message != "shell step missing config" {
		t.Errorf("unexpected error message: %s", stepErr.Message)
	}
}

func TestExecuteShell_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	step := &types.Step{
		ID:       "test-cancel",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "sleep 10",
		},
	}

	// Cancel immediately
	cancel()

	start := time.Now()
	_, stepErr := ExecuteShell(ctx, step)
	elapsed := time.Since(start)

	// Should fail quickly due to cancellation
	if stepErr == nil {
		t.Fatal("expected error due to context cancellation")
	}

	// Should not take 10 seconds
	if elapsed > 2*time.Second {
		t.Errorf("command took too long (%v), context cancellation may not be working", elapsed)
	}
}

func TestExecuteShell_MultipleOutputs(t *testing.T) {
	step := &types.Step{
		ID:       "test-multi",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "echo 'stdout message'; echo 'stderr message' >&2",
			Outputs: map[string]types.OutputSource{
				"out": {Source: "stdout"},
				"err": {Source: "stderr"},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outputs["out"] != "stdout message" {
		t.Errorf("expected stdout 'stdout message', got %q", result.Outputs["out"])
	}
	if result.Outputs["err"] != "stderr message" {
		t.Errorf("expected stderr 'stderr message', got %q", result.Outputs["err"])
	}
}

func TestExecuteShell_CommandWithNewlines(t *testing.T) {
	step := &types.Step{
		ID:       "test-multiline",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: `
echo "line 1"
echo "line 2"
echo "line 3"
`,
			Outputs: map[string]types.OutputSource{
				"output": {Source: "stdout"},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	expected := "line 1\nline 2\nline 3"
	if result.Outputs["output"] != expected {
		t.Errorf("expected %q, got %q", expected, result.Outputs["output"])
	}
}

func TestExecuteShell_FileOutputCreatedByCommand(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "result.txt")

	step := &types.Step{
		ID:       "test-file-create",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "echo 'dynamic content' > " + outputFile,
			Outputs: map[string]types.OutputSource{
				"result": {Source: "file:" + outputFile},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if result.Outputs["result"] != "dynamic content" {
		t.Errorf("expected 'dynamic content', got %q", result.Outputs["result"])
	}
}

func TestExecuteShell_MissingFileOutput(t *testing.T) {
	step := &types.Step{
		ID:       "test-missing-file",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "true",
			Outputs: map[string]types.OutputSource{
				"missing": {Source: "file:/nonexistent/path/file.txt"},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Missing file should result in nil output, not error
	if result.Outputs["missing"] != nil {
		t.Errorf("expected nil for missing file, got %v", result.Outputs["missing"])
	}
}

func TestExecuteShell_UnknownOutputSource(t *testing.T) {
	step := &types.Step{
		ID:       "test-unknown-source",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "true",
			Outputs: map[string]types.OutputSource{
				"unknown": {Source: "invalid_source"},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Unknown source should result in nil output
	if result.Outputs["unknown"] != nil {
		t.Errorf("expected nil for unknown source, got %v", result.Outputs["unknown"])
	}
}

func TestExecuteShell_StderrOnError(t *testing.T) {
	step := &types.Step{
		ID:       "test-stderr-error",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "echo 'error details' >&2; exit 1",
			OnError: "fail",
		},
	}

	_, stepErr := ExecuteShell(context.Background(), step)
	if stepErr == nil {
		t.Fatal("expected error")
	}

	if stepErr.Output != "error details" {
		t.Errorf("expected stderr in error output, got %q", stepErr.Output)
	}
}

func TestExecuteShell_ExitCodeAlwaysInOutputs(t *testing.T) {
	step := &types.Step{
		ID:       "test-exit-always",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "true",
			// No explicit outputs defined
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// exit_code should always be present
	if result.Outputs["exit_code"] != 0 {
		t.Errorf("expected exit_code 0, got %v", result.Outputs["exit_code"])
	}
}

func TestExecuteShell_MeowEnvVarsInjected(t *testing.T) {
	// Test that MEOW_* environment variables are injected when provided in Env
	step := &types.Step{
		ID:       "test-meow-vars",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: "echo \"MEOW_WORKFLOW=$MEOW_WORKFLOW MEOW_STEP=$MEOW_STEP MEOW_ORCH_SOCK=$MEOW_ORCH_SOCK\"",
			Env: map[string]string{
				"MEOW_WORKFLOW":  "wf-123",
				"MEOW_STEP":      "step-456",
				"MEOW_ORCH_SOCK": "/tmp/meow-test.sock",
			},
			Outputs: map[string]types.OutputSource{
				"env_vars": {Source: "stdout"},
			},
		},
	}

	result, stepErr := ExecuteShell(context.Background(), step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	expected := "MEOW_WORKFLOW=wf-123 MEOW_STEP=step-456 MEOW_ORCH_SOCK=/tmp/meow-test.sock"
	if result.Outputs["env_vars"] != expected {
		t.Errorf("expected %q, got %q", expected, result.Outputs["env_vars"])
	}
}
