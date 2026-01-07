package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

func TestNewShellExecutor(t *testing.T) {
	e := NewShellExecutor()
	if e.DefaultShell != "/bin/sh" {
		t.Errorf("expected default shell /bin/sh, got %s", e.DefaultShell)
	}
}

func TestExecute_SimpleCommand(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	spec := &types.CodeSpec{
		Code: "echo hello",
	}

	outputs, err := e.Execute(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	exitCode, ok := outputs["exit_code"].(int)
	if !ok || exitCode != 0 {
		t.Errorf("expected exit code 0, got %v", outputs["exit_code"])
	}

	stdout, ok := outputs["_stdout"].(string)
	if !ok || !strings.Contains(stdout, "hello") {
		t.Errorf("expected stdout to contain 'hello', got %q", stdout)
	}
}

func TestExecute_CaptureStdout(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	spec := &types.CodeSpec{
		Code: "echo test-output",
		Outputs: []types.OutputSpec{
			{Name: "result", Source: types.OutputTypeStdout},
		},
	}

	outputs, err := e.Execute(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := outputs["result"].(string)
	if !ok || result != "test-output" {
		t.Errorf("expected result 'test-output', got %q", result)
	}
}

func TestExecute_CaptureStderr(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	spec := &types.CodeSpec{
		Code: "echo error-output >&2",
		Outputs: []types.OutputSpec{
			{Name: "error", Source: types.OutputTypeStderr},
		},
	}

	outputs, err := e.Execute(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := outputs["error"].(string)
	if !ok || result != "error-output" {
		t.Errorf("expected error 'error-output', got %q", result)
	}
}

func TestExecute_CaptureExitCode(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	spec := &types.CodeSpec{
		Code: "exit 42",
		Outputs: []types.OutputSpec{
			{Name: "code", Source: types.OutputTypeExitCode},
		},
	}

	// Non-zero exit codes don't return an error - they're just captured in outputs
	outputs, err := e.Execute(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error for non-zero exit: %v", err)
	}

	exitCode, ok := outputs["exit_code"].(int)
	if !ok || exitCode != 42 {
		t.Errorf("expected exit code 42, got %v", outputs["exit_code"])
	}

	code, ok := outputs["code"].(int)
	if !ok || code != 42 {
		t.Errorf("expected code 42, got %v", outputs["code"])
	}
}

func TestExecute_CaptureFile(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	// Create temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "output.txt")

	spec := &types.CodeSpec{
		Code: "echo file-content > " + tmpFile,
		Outputs: []types.OutputSpec{
			{Name: "file_result", Source: types.OutputTypeFile, Path: tmpFile},
		},
	}

	outputs, err := e.Execute(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := outputs["file_result"].(string)
	if !ok || result != "file-content" {
		t.Errorf("expected file_result 'file-content', got %q", result)
	}
}

func TestExecute_WorkingDirectory(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	tmpDir := t.TempDir()

	spec := &types.CodeSpec{
		Code:    "pwd",
		Workdir: tmpDir,
		Outputs: []types.OutputSpec{
			{Name: "cwd", Source: types.OutputTypeStdout},
		},
	}

	outputs, err := e.Execute(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	cwd, ok := outputs["cwd"].(string)
	if !ok || cwd != tmpDir {
		t.Errorf("expected cwd %q, got %q", tmpDir, cwd)
	}
}

func TestExecute_Environment(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	spec := &types.CodeSpec{
		Code: "echo $TEST_VAR",
		Env: map[string]string{
			"TEST_VAR": "test-value",
		},
		Outputs: []types.OutputSpec{
			{Name: "result", Source: types.OutputTypeStdout},
		},
	}

	outputs, err := e.Execute(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, ok := outputs["result"].(string)
	if !ok || result != "test-value" {
		t.Errorf("expected result 'test-value', got %q", result)
	}
}

func TestExecute_ContextCancellation(t *testing.T) {
	e := NewShellExecutor()

	// Create a cancellable context
	ctx, cancel := context.WithCancel(context.Background())

	spec := &types.CodeSpec{
		Code: "sleep 30",
	}

	// Start execution in goroutine
	done := make(chan struct{})
	var outputs map[string]any
	var execErr error

	go func() {
		outputs, execErr = e.Execute(ctx, spec)
		close(done)
	}()

	// Cancel after a short delay
	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for completion with timeout
	select {
	case <-done:
		// Good - execution was cancelled
	case <-time.After(5 * time.Second):
		t.Fatal("execution did not complete after cancellation")
	}

	// Should have context.Canceled error
	if execErr != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", execErr)
	}

	// Exit code should be -1 for cancelled
	exitCode, ok := outputs["exit_code"].(int)
	if !ok || exitCode != -1 {
		t.Errorf("expected exit code -1, got %v", outputs["exit_code"])
	}
}

func TestExecute_ContextTimeout(t *testing.T) {
	e := NewShellExecutor()

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	spec := &types.CodeSpec{
		Code: "sleep 30",
	}

	outputs, err := e.Execute(ctx, spec)

	// Should have context.DeadlineExceeded error
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded error, got %v", err)
	}

	// Exit code should be -1 for timeout
	exitCode, ok := outputs["exit_code"].(int)
	if !ok || exitCode != -1 {
		t.Errorf("expected exit code -1, got %v", outputs["exit_code"])
	}
}

func TestExecute_NilSpec(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	_, err := e.Execute(ctx, nil)
	if err == nil {
		t.Error("expected error for nil spec")
	}
}

func TestExecute_EmptyCode(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	spec := &types.CodeSpec{
		Code: "",
	}

	_, err := e.Execute(ctx, spec)
	if err == nil {
		t.Error("expected error for empty code")
	}
}

func TestExecute_MultipleOutputs(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "output.txt")

	spec := &types.CodeSpec{
		Code: "echo stdout-content && echo stderr-content >&2 && echo file-content > " + tmpFile,
		Outputs: []types.OutputSpec{
			{Name: "out", Source: types.OutputTypeStdout},
			{Name: "err", Source: types.OutputTypeStderr},
			{Name: "file", Source: types.OutputTypeFile, Path: tmpFile},
			{Name: "exit", Source: types.OutputTypeExitCode},
		},
	}

	outputs, err := e.Execute(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if out := outputs["out"].(string); out != "stdout-content" {
		t.Errorf("expected out 'stdout-content', got %q", out)
	}
	if errOut := outputs["err"].(string); errOut != "stderr-content" {
		t.Errorf("expected err 'stderr-content', got %q", errOut)
	}
	if file := outputs["file"].(string); file != "file-content" {
		t.Errorf("expected file 'file-content', got %q", file)
	}
	if exit := outputs["exit"].(int); exit != 0 {
		t.Errorf("expected exit 0, got %d", exit)
	}
}

func TestExecute_MissingFile(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	spec := &types.CodeSpec{
		Code: "echo test",
		Outputs: []types.OutputSpec{
			{Name: "missing", Source: types.OutputTypeFile, Path: "/nonexistent/file"},
		},
	}

	outputs, err := e.Execute(ctx, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Missing file should result in empty string, not error
	missing, ok := outputs["missing"].(string)
	if !ok || missing != "" {
		t.Errorf("expected empty string for missing file, got %q", missing)
	}
}

func TestExecute_BlockingCommand(t *testing.T) {
	e := NewShellExecutor()
	ctx := context.Background()

	// Test a command that could potentially block but completes quickly
	spec := &types.CodeSpec{
		Code: "echo test && sleep 0.1",
	}

	start := time.Now()
	outputs, err := e.Execute(ctx, spec)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should complete in reasonable time
	if duration > 5*time.Second {
		t.Errorf("execution took too long: %v", duration)
	}

	if outputs["exit_code"].(int) != 0 {
		t.Errorf("expected exit code 0, got %v", outputs["exit_code"])
	}
}

func TestReadFile(t *testing.T) {
	e := NewShellExecutor()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")

	// Write test content
	if err := os.WriteFile(tmpFile, []byte("test-content\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	content, err := e.readFile(tmpFile)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should trim trailing newline
	if content != "test-content" {
		t.Errorf("expected 'test-content', got %q", content)
	}
}
