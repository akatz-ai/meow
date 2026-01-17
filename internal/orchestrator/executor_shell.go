package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/akatz-ai/meow/internal/types"
)

// ShellResult contains the results of executing a shell command.
type ShellResult struct {
	Outputs  map[string]any
	ExitCode int
	Stdout   string
	Stderr   string
}

// ExecuteShell runs a shell command and captures outputs.
// Returns the captured outputs and any error that occurred.
// If on_error is "continue", errors are captured in outputs rather than returned.
func ExecuteShell(ctx context.Context, step *types.Step) (*ShellResult, *types.StepError) {
	if step.Shell == nil {
		return nil, &types.StepError{Message: "shell step missing config"}
	}

	cfg := step.Shell
	result, err := runShellCommand(ctx, cfg)
	if err != nil {
		// Check on_error handling
		if cfg.OnError == "continue" {
			// Return error info in outputs, no StepError
			result.Outputs["error"] = err.Error()
			return result, nil
		}
		return result, &types.StepError{
			Message: err.Error(),
			Code:    result.ExitCode,
			Output:  result.Stderr,
		}
	}

	return result, nil
}

// runShellCommand executes the shell command and captures output.
func runShellCommand(ctx context.Context, cfg *types.ShellConfig) (*ShellResult, error) {
	result := &ShellResult{
		Outputs: make(map[string]any),
	}

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
	result.Stdout = strings.TrimSpace(stdout.String())
	result.Stderr = strings.TrimSpace(stderr.String())

	// Get exit code
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	// Capture outputs based on config
	// Note: This standalone executor doesn't have workflow context for step output substitution
	if cfg.Outputs != nil {
		for name, source := range cfg.Outputs {
			value, captureErr := captureOutput(source, result, nil)
			if captureErr != nil {
				// Log but don't fail on capture errors
				result.Outputs[name] = nil
				continue
			}
			result.Outputs[name] = value
		}
	}

	// Always include exit_code in outputs for convenience
	result.Outputs["exit_code"] = result.ExitCode

	return result, err
}

// SourceSubstituteFunc substitutes variables in a source path at runtime.
// Used to resolve step output references like {{step.outputs.field}} in output paths.
type SourceSubstituteFunc func(source string) (string, error)

// captureOutput extracts a value based on the source specification.
// If substituteSource is provided, it will be called to substitute any remaining
// variable references in file paths before reading.
func captureOutput(outputSource types.OutputSource, result *ShellResult, substituteSource SourceSubstituteFunc) (any, error) {
	source := outputSource.Source
	var value string

	switch source {
	case "stdout":
		value = result.Stdout
	case "stderr":
		value = result.Stderr
	case "exit_code":
		return result.ExitCode, nil // exit_code is always int, ignore type
	default:
		// Check for file: prefix
		if strings.HasPrefix(source, "file:") {
			filePath := strings.TrimPrefix(source, "file:")
			// Substitute any remaining variable references (e.g., step outputs)
			if substituteSource != nil && strings.Contains(filePath, "{{") {
				substituted, err := substituteSource(filePath)
				if err != nil {
					return nil, fmt.Errorf("substituting output path: %w", err)
				}
				filePath = substituted
			}
			content, err := os.ReadFile(filePath)
			if err != nil {
				return nil, fmt.Errorf("reading output file %s: %w", filePath, err)
			}
			value = strings.TrimSpace(string(content))
		} else {
			return nil, fmt.Errorf("unknown output source: %s", source)
		}
	}

	// Handle type conversion
	if outputSource.Type == "json" {
		var parsed any
		if err := json.Unmarshal([]byte(value), &parsed); err != nil {
			return nil, fmt.Errorf("parsing JSON from %s: %w", source, err)
		}
		return parsed, nil
	}

	return value, nil
}
