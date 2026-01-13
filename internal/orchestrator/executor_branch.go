package orchestrator

import (
	"context"
	"fmt"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// BranchOutcome indicates which branch was taken.
type BranchOutcome string

const (
	BranchOutcomeTrue    BranchOutcome = "true"
	BranchOutcomeFalse   BranchOutcome = "false"
	BranchOutcomeTimeout BranchOutcome = "timeout"
	BranchOutcomeNone    BranchOutcome = "none" // No target for the outcome
)

// BranchResult contains the results of branch evaluation.
type BranchResult struct {
	Outcome       BranchOutcome   // Which branch was taken
	ExitCode      int             // Condition command exit code
	ExpandedSteps []*types.Step   // Steps from the expanded target (if any)
	StepIDs       []string        // IDs of expanded steps
	Target        *types.BranchTarget // The selected branch target (may be nil)
}

// ConditionExecutor runs shell commands for condition evaluation.
// This is separate from the shell executor to allow for different testing.
type ConditionExecutor interface {
	// Execute runs a command and returns the exit code.
	// Returns error only for execution failures, not non-zero exit codes.
	Execute(ctx context.Context, command string) (exitCode int, stdout, stderr string, err error)
}

// ExecuteBranch evaluates a condition and returns the appropriate branch target.
// It does NOT expand the target - that's done by the caller using ExecuteExpand.
// This separation allows the orchestrator to handle async branch evaluation.
//
// Parameters:
// - step: The branch step
// - condExec: Executor for running the condition command
// - loader: Template loader for expanding branch targets
// - variables: Variables for substitution
// - depth: Current expansion depth
// - limits: Expansion limits
func ExecuteBranch(
	ctx context.Context,
	step *types.Step,
	condExec ConditionExecutor,
	loader TemplateLoader,
	variables map[string]string,
	depth int,
	limits *ExpansionLimits,
) (*BranchResult, *types.StepError) {
	if step.Branch == nil {
		return nil, &types.StepError{Message: "branch step missing config"}
	}

	cfg := step.Branch

	// Validate required field
	if cfg.Condition == "" {
		return nil, &types.StepError{Message: "branch step missing condition field"}
	}

	result := &BranchResult{}

	// Create context with timeout if specified
	execCtx := ctx
	var cancel context.CancelFunc
	if cfg.Timeout != "" {
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, &types.StepError{
				Message: fmt.Sprintf("invalid timeout %q: %v", cfg.Timeout, err),
			}
		}
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Execute the condition command
	exitCode, _, _, execErr := condExec.Execute(execCtx, cfg.Condition)
	result.ExitCode = exitCode

	// Determine which branch to take
	if execErr != nil {
		// Check if it was a timeout
		if execCtx.Err() == context.DeadlineExceeded {
			result.Outcome = BranchOutcomeTimeout
			result.Target = cfg.OnTimeout
			// If no on_timeout, fall back to on_false per spec
			if result.Target == nil {
				result.Target = cfg.OnFalse
			}
		} else {
			// Other execution error - treat as false
			result.Outcome = BranchOutcomeFalse
			result.Target = cfg.OnFalse
		}
	} else if exitCode == 0 {
		result.Outcome = BranchOutcomeTrue
		result.Target = cfg.OnTrue
	} else {
		result.Outcome = BranchOutcomeFalse
		result.Target = cfg.OnFalse
	}

	// If there's no target for this outcome, we're done
	// Note: We preserve the original outcome (true/false/timeout) even if there's no target.
	// BranchOutcomeNone is only set when the outcome itself was none (shouldn't happen).
	if result.Target == nil {
		return result, nil
	}

	// Expand the target
	expandedResult, expandErr := expandBranchTarget(ctx, step.ID, result.Target, loader, variables, depth, limits)
	if expandErr != nil {
		return result, expandErr
	}

	result.ExpandedSteps = expandedResult.ExpandedSteps
	result.StepIDs = expandedResult.StepIDs

	return result, nil
}

// expandBranchTarget expands a branch target (template or inline steps).
func expandBranchTarget(
	ctx context.Context,
	parentID string,
	target *types.BranchTarget,
	loader TemplateLoader,
	variables map[string]string,
	depth int,
	limits *ExpansionLimits,
) (*ExecuteExpandResult, *types.StepError) {
	// Merge target variables with workflow variables
	mergedVars := make(map[string]string)
	for k, v := range variables {
		mergedVars[k] = v
	}
	for k, v := range target.Variables {
		mergedVars[k] = v
	}

	if target.Template != "" {
		// Expand template
		expandStep := &types.Step{
			ID:       parentID,
			Executor: types.ExecutorExpand,
			Expand: &types.ExpandConfig{
				Template:  target.Template,
				Variables: mergedVars,
			},
		}
		return ExecuteExpand(ctx, expandStep, loader, variables, depth, limits)
	}

	if len(target.Inline) > 0 {
		// Expand inline steps
		return expandInlineSteps(parentID, target.Inline, mergedVars)
	}

	// Empty target (valid - just continue without adding steps)
	return &ExecuteExpandResult{}, nil
}

// expandInlineSteps converts inline step definitions to Step objects.
func expandInlineSteps(parentID string, inline []types.InlineStep, vars map[string]string) (*ExecuteExpandResult, *types.StepError) {
	result := &ExecuteExpandResult{
		ExpandedSteps: make([]*types.Step, 0, len(inline)),
		StepIDs:       make([]string, 0, len(inline)),
	}

	// Build set of inline step IDs for dependency resolution
	inlineStepIDs := make(map[string]bool)
	for _, is := range inline {
		inlineStepIDs[is.ID] = true
	}

	for _, is := range inline {
		newID := parentID + "." + is.ID
		newStep := &types.Step{
			ID:           newID,
			Executor:     is.Executor,
			Status:       types.StepStatusPending,
			ExpandedFrom: parentID,
		}

		// Populate executor-specific config from inline step fields
		switch is.Executor {
		case types.ExecutorShell:
			newStep.Shell = &types.ShellConfig{
				Command: is.Command,
			}
		case types.ExecutorAgent:
			newStep.Agent = &types.AgentConfig{
				Agent:  is.Agent,
				Prompt: is.Prompt,
			}
		}

		// Apply variable substitution to all fields
		substituteStepVariables(newStep, vars)

		// Update dependencies
		newStep.Needs = prefixNeeds(is.Needs, parentID, inlineStepIDs)

		result.ExpandedSteps = append(result.ExpandedSteps, newStep)
		result.StepIDs = append(result.StepIDs, newID)
	}

	return result, nil
}

// SimpleConditionExecutor is a basic implementation using the shell executor.
type SimpleConditionExecutor struct {
	// SocketPath is the IPC socket path for MEOW_ORCH_SOCK environment variable.
	// If set, condition commands can use meow event/await-event.
	SocketPath string
}

// Execute runs a command using the shell executor.
func (e *SimpleConditionExecutor) Execute(ctx context.Context, command string) (int, string, string, error) {
	// Build environment - include MEOW_ORCH_SOCK if we have a socket path
	var env map[string]string
	if e.SocketPath != "" {
		env = map[string]string{
			"MEOW_ORCH_SOCK": e.SocketPath,
		}
	}

	step := &types.Step{
		ID:       "condition",
		Executor: types.ExecutorShell,
		Shell: &types.ShellConfig{
			Command: command,
			OnError: "continue", // Don't fail on non-zero exit
			Env:     env,
		},
	}

	result, _ := ExecuteShell(ctx, step)
	if result == nil {
		return 1, "", "", fmt.Errorf("shell execution returned nil result")
	}

	// Check for execution error (not just non-zero exit)
	if ctx.Err() != nil {
		return result.ExitCode, result.Stdout, result.Stderr, ctx.Err()
	}

	return result.ExitCode, result.Stdout, result.Stderr, nil
}
