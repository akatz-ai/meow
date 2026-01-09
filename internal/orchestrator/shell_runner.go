package orchestrator

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/meow-stack/meow-machine/internal/types"
)

// DefaultShellRunner implements ShellRunner using os/exec.
type DefaultShellRunner struct{}

// NewDefaultShellRunner creates a new DefaultShellRunner.
func NewDefaultShellRunner() *DefaultShellRunner {
	return &DefaultShellRunner{}
}

// Run executes a shell command and captures outputs.
func (r *DefaultShellRunner) Run(ctx context.Context, cfg *types.ShellConfig) (map[string]any, error) {
	if cfg == nil {
		return nil, fmt.Errorf("shell config is nil")
	}

	outputs := make(map[string]any)

	// Create command
	cmd := exec.CommandContext(ctx, "sh", "-c", cfg.Command)

	// Set working directory
	if cfg.Workdir != "" {
		cmd.Dir = cfg.Workdir
	}

	// Set environment variables
	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// Capture stdout and stderr separately
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()

	// Capture raw output
	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	// Get exit code
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	// Capture outputs based on config
	if cfg.Outputs != nil {
		for name, source := range cfg.Outputs {
			value, captureErr := r.captureOutput(source.Source, stdoutStr, stderrStr, exitCode)
			if captureErr != nil {
				// Log but don't fail on capture errors
				outputs[name] = nil
				continue
			}
			outputs[name] = value
		}
	}

	// Always include standard outputs for convenience
	outputs["stdout"] = stdoutStr
	outputs["stderr"] = stderrStr
	outputs["exit_code"] = exitCode

	return outputs, err
}

// captureOutput extracts a value based on the source specification.
func (r *DefaultShellRunner) captureOutput(source, stdout, stderr string, exitCode int) (any, error) {
	switch source {
	case "stdout":
		return stdout, nil
	case "stderr":
		return stderr, nil
	case "exit_code":
		return exitCode, nil
	default:
		// Check for file: prefix
		if strings.HasPrefix(source, "file:") {
			filePath := strings.TrimPrefix(source, "file:")
			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("reading output file %s: %w", filePath, err)
			}
			return strings.TrimSpace(string(content)), nil
		}
		return nil, fmt.Errorf("unknown output source: %s", source)
	}
}
