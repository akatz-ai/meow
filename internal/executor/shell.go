// Package executor provides shell command execution with cancellation support.
package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// ShellExecutor executes shell commands with context cancellation.
type ShellExecutor struct {
	// DefaultShell is the shell used to execute commands.
	// Defaults to "/bin/sh".
	DefaultShell string
}

// NewShellExecutor creates a new ShellExecutor with default settings.
func NewShellExecutor() *ShellExecutor {
	return &ShellExecutor{
		DefaultShell: "/bin/sh",
	}
}

// Execute runs the shell command and captures outputs.
// It supports context cancellation - when the context is cancelled,
// the process is gracefully terminated (SIGTERM, then SIGKILL after 3s).
//
// The returned outputs map always contains:
//   - "exit_code": int - the process exit code (-1 if killed)
//   - "_stdout": string - raw stdout for debugging
//   - "_stderr": string - raw stderr for debugging
//   - Named outputs as specified in spec.Outputs
func (e *ShellExecutor) Execute(ctx context.Context, spec *types.CodeSpec) (map[string]any, error) {
	if spec == nil {
		return nil, fmt.Errorf("code spec is nil")
	}
	if spec.Code == "" {
		return nil, fmt.Errorf("code is empty")
	}

	shell := e.DefaultShell
	if shell == "" {
		shell = "/bin/sh"
	}

	// Create command with shell (not CommandContext - we handle cancellation manually
	// to support graceful SIGTERM before SIGKILL)
	cmd := exec.Command(shell, "-c", spec.Code)

	// Set working directory if specified
	if spec.Workdir != "" {
		cmd.Dir = spec.Workdir
	}

	// Set environment variables
	if len(spec.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range spec.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Capture stdout and stderr
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Set process group so we can kill the entire tree
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting command: %w", err)
	}

	// Wait for completion or cancellation
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var exitCode int
	var execErr error

	select {
	case <-ctx.Done():
		// Context cancelled - kill the process group
		if cmd.Process != nil {
			// Try graceful termination first
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)

			// Give it a moment to clean up
			select {
			case <-done:
				// Process exited
			case <-time.After(3 * time.Second):
				// Force kill
				_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
				<-done
			}
		}
		exitCode = -1
		execErr = ctx.Err()

	case err := <-done:
		if err != nil {
			// Extract exit code from error
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
				execErr = err
			}
		} else {
			exitCode = 0
		}
	}

	// Build outputs map
	outputs := make(map[string]any)
	outputs["exit_code"] = exitCode

	// Process output specifications
	for _, outSpec := range spec.Outputs {
		switch outSpec.Source {
		case types.OutputTypeStdout:
			outputs[outSpec.Name] = strings.TrimSuffix(stdout.String(), "\n")
		case types.OutputTypeStderr:
			outputs[outSpec.Name] = strings.TrimSuffix(stderr.String(), "\n")
		case types.OutputTypeExitCode:
			outputs[outSpec.Name] = exitCode
		case types.OutputTypeFile:
			content, err := e.readFile(outSpec.Path)
			if err != nil {
				// File read errors are not fatal, store empty string
				outputs[outSpec.Name] = ""
			} else {
				outputs[outSpec.Name] = content
			}
		}
	}

	// Always include stdout/stderr for debugging
	outputs["_stdout"] = stdout.String()
	outputs["_stderr"] = stderr.String()

	return outputs, execErr
}

// readFile reads a file and returns its contents as a string.
func (e *ShellExecutor) readFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(string(data), "\n"), nil
}
