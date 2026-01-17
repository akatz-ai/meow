package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/akatz-ai/meow/internal/types"
)

// AgentMode determines how the stop hook behaves for agent steps.
type AgentMode string

const (
	// AgentModeAutonomous is the default - stop hook re-injects prompt.
	AgentModeAutonomous AgentMode = "autonomous"
	// AgentModeInteractive allows human conversation during step.
	AgentModeInteractive AgentMode = "interactive"
	// AgentModeFireForget injects prompt and completes immediately.
	// Does not wait for meow done, cannot have outputs.
	AgentModeFireForget AgentMode = "fire_forget"
)

// PromptInjector injects prompts into agent sessions via tmux.
type PromptInjector interface {
	// SendKeys sends keystrokes to the agent's tmux session.
	SendKeys(ctx context.Context, session string, keys string) error
}

// AgentStateManager tracks agent state for the workflow.
type AgentStateManager interface {
	// GetSession returns the tmux session name for an agent.
	GetSession(agentID string) string
	// SetCurrentStep sets the current step for an agent.
	SetCurrentStep(agentID, stepID string)
	// SetIdle marks an agent as having no current step.
	SetIdle(agentID string)
	// IsIdle returns true if agent has no current step.
	IsIdle(agentID string) bool
}

// AgentStartResult contains the results of starting an agent step.
type AgentStartResult struct {
	Prompt string // The prompt that was injected
}

// AgentCompletionResult contains the results of completing an agent step.
type AgentCompletionResult struct {
	Outputs          map[string]any // Validated outputs
	ValidationErrors []string       // Any validation errors (if any)
}

// OutputValidator validates agent outputs against definitions.
type OutputValidator interface {
	// Validate checks outputs against definitions.
	// Returns list of validation error messages.
	Validate(outputs map[string]any, defs map[string]types.AgentOutputDef, agentWorkdir string) []string
}

// StartAgentStep marks a step as running and builds the prompt for injection.
// The actual injection should be done by the caller using the returned prompt.
//
// Note: This function does NOT inject the prompt - it just prepares it.
// The orchestrator is responsible for injecting via tmux.
func StartAgentStep(step *types.Step) (*AgentStartResult, *types.StepError) {
	if step.Agent == nil {
		return nil, &types.StepError{Message: "agent step missing config"}
	}

	cfg := step.Agent

	// Validate required fields
	if cfg.Agent == "" {
		return nil, &types.StepError{Message: "agent step missing agent field"}
	}
	if cfg.Prompt == "" {
		return nil, &types.StepError{Message: "agent step missing prompt field"}
	}

	// Validate fire_forget mode constraints
	if cfg.Mode == string(AgentModeFireForget) && len(cfg.Outputs) > 0 {
		return nil, &types.StepError{Message: "fire_forget mode cannot have outputs"}
	}

	// Build the full prompt with output expectations
	prompt := buildAgentPrompt(cfg)

	return &AgentStartResult{
		Prompt: prompt,
	}, nil
}

// buildAgentPrompt constructs the full prompt including output expectations.
func buildAgentPrompt(cfg *types.AgentConfig) string {
	var sb strings.Builder

	// Main prompt
	sb.WriteString(cfg.Prompt)

	// Fire-and-forget mode: just the prompt, no meow done instructions
	if cfg.Mode == string(AgentModeFireForget) {
		return sb.String()
	}

	// Add output expectations if defined
	if len(cfg.Outputs) > 0 {
		sb.WriteString("\n\n## Expected Outputs\n\n")
		sb.WriteString("When complete, run: `meow done --output <key>=<value>`\n\n")

		for name, def := range cfg.Outputs {
			required := ""
			if def.Required {
				required = " **(required)**"
			}
			sb.WriteString(fmt.Sprintf("- `%s` (%s)%s", name, def.Type, required))
			if def.Description != "" {
				sb.WriteString(": " + def.Description)
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("\n\nWhen complete, run: `meow done`")
	}

	return sb.String()
}

// CompleteAgentStep validates outputs and completes the step.
// This is called when an agent runs `meow done`.
//
// Parameters:
// - step: The agent step being completed
// - outputs: The outputs provided by the agent
// - agentWorkdir: The agent's working directory (for file_path validation)
//
// Returns validation errors if outputs don't match definitions.
// If validation fails, the step should be returned to "running" status.
func CompleteAgentStep(
	step *types.Step,
	outputs map[string]any,
	agentWorkdir string,
) (*AgentCompletionResult, *types.StepError) {
	if step.Agent == nil {
		return nil, &types.StepError{Message: "agent step missing config"}
	}

	cfg := step.Agent
	result := &AgentCompletionResult{
		Outputs:          outputs,
		ValidationErrors: []string{},
	}

	// Validate outputs if definitions exist
	if cfg.Outputs != nil && len(cfg.Outputs) > 0 {
		errs := ValidateAgentOutputs(outputs, cfg.Outputs, agentWorkdir)
		result.ValidationErrors = errs

		if len(errs) > 0 {
			// Return error with validation failures
			// The orchestrator should return the step to "running" status
			return result, &types.StepError{
				Message: fmt.Sprintf("output validation failed: %s", strings.Join(errs, "; ")),
			}
		}
	}

	return result, nil
}

// ValidateAgentOutputs checks that outputs match their definitions.
func ValidateAgentOutputs(outputs map[string]any, defs map[string]types.AgentOutputDef, agentWorkdir string) []string {
	var errs []string

	// Check for required outputs
	for name, def := range defs {
		val, ok := outputs[name]
		if !ok || val == nil {
			if def.Required {
				errs = append(errs, fmt.Sprintf("missing required output: %s", name))
			}
			continue
		}

		// Validate type
		typeErr := validateOutputType(name, val, def.Type, agentWorkdir)
		if typeErr != "" {
			errs = append(errs, typeErr)
		}
	}

	return errs
}

// validateOutputType checks that a value matches its declared type.
// If the value is a string, it attempts to coerce it to the expected type.
func validateOutputType(name string, val any, declaredType, agentWorkdir string) string {
	// If value is a string, try to coerce to expected type
	if strVal, ok := val.(string); ok && declaredType != "string" && declaredType != "file_path" {
		coerced, err := coerceStringToType(strVal, declaredType)
		if err != "" {
			return fmt.Sprintf("output %s: %s", name, err)
		}
		val = coerced
	}

	switch declaredType {
	case "string":
		if _, ok := val.(string); !ok {
			return fmt.Sprintf("output %s: expected string, got %T", name, val)
		}
	case "number":
		switch val.(type) {
		case int, int64, float64:
			// OK
		default:
			return fmt.Sprintf("output %s: expected number, got %T", name, val)
		}
	case "boolean":
		if _, ok := val.(bool); !ok {
			return fmt.Sprintf("output %s: expected boolean, got %T", name, val)
		}
	case "json":
		// JSON can be any structured type - maps or arrays
		switch val.(type) {
		case map[string]any, []any:
			// OK
		default:
			return fmt.Sprintf("output %s: expected json (object or array), got %T", name, val)
		}
	case "file_path":
		path, ok := val.(string)
		if !ok {
			return fmt.Sprintf("output %s: expected file path string, got %T", name, val)
		}
		// Validate file exists and is within agent's workdir
		if err := validateFilePath(path, agentWorkdir); err != "" {
			return fmt.Sprintf("output %s: %s", name, err)
		}
	default:
		// Unknown type - allow any value
	}

	return ""
}

// coerceStringToType attempts to convert a string value to the expected type.
// Returns the coerced value and an error message (empty string if successful).
func coerceStringToType(s string, targetType string) (any, string) {
	switch targetType {
	case "boolean":
		switch strings.ToLower(s) {
		case "true", "1", "yes":
			return true, ""
		case "false", "0", "no":
			return false, ""
		default:
			return nil, fmt.Sprintf("cannot parse %q as boolean (use true/false/1/0/yes/no)", s)
		}
	case "number":
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f, ""
		}
		return nil, fmt.Sprintf("cannot parse %q as number", s)
	case "json":
		var v any
		if err := json.Unmarshal([]byte(s), &v); err != nil {
			return nil, fmt.Sprintf("cannot parse %q as JSON: %v", s, err)
		}
		// JSON must be object or array
		switch v.(type) {
		case map[string]any, []any:
			return v, ""
		default:
			return nil, fmt.Sprintf("JSON must be an object or array, got %T", v)
		}
	default:
		return s, ""
	}
}

// validateFilePath checks that a file path exists and is within the allowed directory.
func validateFilePath(path, workdir string) string {
	if path == "" {
		return "file path is empty"
	}

	// Resolve the path
	var absPath string
	if filepath.IsAbs(path) {
		absPath = filepath.Clean(path)
	} else {
		absPath = filepath.Clean(filepath.Join(workdir, path))
	}

	// Check file exists
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Sprintf("file does not exist: %s", path)
	}

	// Check path is within workdir (prevent path traversal)
	if workdir != "" {
		absWorkdir := filepath.Clean(workdir)
		// Ensure workdir ends with separator to prevent prefix matching attacks
		// e.g., workdir="/home/user/work" shouldn't match "/home/user/workspace"
		if !strings.HasSuffix(absWorkdir, string(filepath.Separator)) {
			absWorkdir += string(filepath.Separator)
		}
		// The file must be exactly the workdir or within it
		if absPath != filepath.Clean(workdir) && !strings.HasPrefix(absPath, absWorkdir) {
			return fmt.Sprintf("file path must be within agent workdir: %s", path)
		}
	}

	return ""
}

// ParseAgentMode converts a string to AgentMode.
// Returns an error for invalid modes, listing all valid options.
func ParseAgentMode(s string) (AgentMode, error) {
	switch strings.ToLower(s) {
	case "", "autonomous":
		return AgentModeAutonomous, nil
	case "interactive":
		return AgentModeInteractive, nil
	case "fire_forget":
		return AgentModeFireForget, nil
	default:
		return "", fmt.Errorf("invalid agent mode %q: must be autonomous, interactive, or fire_forget", s)
	}
}

// IsFireForget returns true if the agent config is in fire_forget mode.
func IsFireForget(cfg *types.AgentConfig) bool {
	if cfg == nil {
		return false
	}
	return cfg.Mode == string(AgentModeFireForget)
}
