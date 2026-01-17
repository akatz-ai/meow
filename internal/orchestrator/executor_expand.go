package orchestrator

import (
	"context"
	"fmt"

	"github.com/akatz-ai/meow/internal/types"
	"github.com/akatz-ai/meow/internal/workflow"
)

// TemplateLoader loads template workflows for expansion.
type TemplateLoader interface {
	// Load retrieves a template by reference.
	// Template references can be:
	// - ".workflow-name" - same file, specific workflow
	// - "main" - same file, workflow named "main"
	// - "file#workflow" - external file, specific workflow
	// - "file" - external file, workflow named "main"
	// The variables parameter allows passing values to satisfy required template variables.
	Load(ctx context.Context, ref string, variables map[string]any) ([]*types.Step, error)
}

// ExpansionLimits defines resource limits for expansion.
type ExpansionLimits struct {
	MaxDepth      int // Maximum expansion nesting depth
	MaxTotalSteps int // Maximum total steps in workflow
}

// DefaultExpansionLimits returns reasonable default limits.
func DefaultExpansionLimits() *ExpansionLimits {
	return &ExpansionLimits{
		MaxDepth:      10,
		MaxTotalSteps: 1000,
	}
}

// ExecuteExpandResult contains the results of template expansion.
type ExecuteExpandResult struct {
	ExpandedSteps []*types.Step // The newly created steps
	StepIDs       []string      // IDs of the expanded steps
}

// ExecuteExpand expands a template's steps into new steps.
// It returns the expanded steps without modifying any workflow state.
// The caller is responsible for adding the steps to the workflow.
//
// Parameters:
// - step: The expand step containing the template reference
// - loader: Interface for loading templates
// - variables: Variables to substitute in the template
// - depth: Current expansion depth (for limit checking)
// - limits: Resource limits for expansion
func ExecuteExpand(
	ctx context.Context,
	step *types.Step,
	loader TemplateLoader,
	variables map[string]any,
	depth int,
	limits *ExpansionLimits,
) (*ExecuteExpandResult, *types.StepError) {
	if step.Expand == nil {
		return nil, &types.StepError{Message: "expand step missing config"}
	}

	cfg := step.Expand

	// Validate required field
	if cfg.Template == "" {
		return nil, &types.StepError{Message: "expand step missing template field"}
	}

	// Apply default limits
	if limits == nil {
		limits = DefaultExpansionLimits()
	}

	// Check depth limit
	if depth >= limits.MaxDepth {
		return nil, &types.StepError{
			Message: fmt.Sprintf("expansion depth limit exceeded: %d >= %d", depth, limits.MaxDepth),
		}
	}

	// Load the template, passing any variables from the expand config
	templateSteps, err := loader.Load(ctx, cfg.Template, cfg.Variables)
	if err != nil {
		return nil, &types.StepError{
			Message: fmt.Sprintf("failed to load template %s: %v", cfg.Template, err),
		}
	}

	if len(templateSteps) == 0 {
		// Empty template is valid - no steps to expand
		return &ExecuteExpandResult{
			ExpandedSteps: nil,
			StepIDs:       nil,
		}, nil
	}

	// Merge variables: workflow variables + expand step variables
	mergedVars := make(map[string]any)
	for k, v := range variables {
		mergedVars[k] = v
	}
	for k, v := range cfg.Variables {
		mergedVars[k] = v
	}

	// Build set of template step IDs for dependency resolution
	templateStepIDs := make(map[string]bool)
	for _, ts := range templateSteps {
		templateStepIDs[ts.ID] = true
	}

	// Expand each template step
	result := &ExecuteExpandResult{
		ExpandedSteps: make([]*types.Step, 0, len(templateSteps)),
		StepIDs:       make([]string, 0, len(templateSteps)),
	}

	for _, tmplStep := range templateSteps {
		// Create new step with prefixed ID
		newID := step.ID + "." + tmplStep.ID
		newStep := cloneStep(tmplStep)
		newStep.ID = newID
		newStep.Status = types.StepStatusPending
		newStep.ExpandedFrom = step.ID

		// Substitute variables in config using VarContext
		varCtx := buildVarContext(mergedVars)
		if err := substituteStepVariablesTyped(newStep, varCtx); err != nil {
			return nil, &types.StepError{
				Message: fmt.Sprintf("variable substitution failed for step %s: %v", newID, err),
			}
		}

		// Update dependencies to use prefixed IDs
		newStep.Needs = prefixNeeds(tmplStep.Needs, step.ID, templateStepIDs)

		result.ExpandedSteps = append(result.ExpandedSteps, newStep)
		result.StepIDs = append(result.StepIDs, newID)
	}

	return result, nil
}

// cloneStep creates a shallow copy of a step.
// The caller should update ID, Status, Needs, and ExpandedFrom.
func cloneStep(src *types.Step) *types.Step {
	dst := &types.Step{
		ID:           src.ID,
		Executor:     src.Executor,
		Status:       src.Status,
		Needs:        append([]string(nil), src.Needs...),
		ExpandedFrom: src.ExpandedFrom,
		ExpandedInto: append([]string(nil), src.ExpandedInto...),
		SourceModule: src.SourceModule,
	}

	// Clone executor-specific configs
	if src.Shell != nil {
		dst.Shell = cloneShellConfig(src.Shell)
	}
	if src.Spawn != nil {
		dst.Spawn = cloneSpawnConfig(src.Spawn)
	}
	if src.Kill != nil {
		dst.Kill = cloneKillConfig(src.Kill)
	}
	if src.Expand != nil {
		dst.Expand = cloneExpandConfig(src.Expand)
	}
	if src.Branch != nil {
		dst.Branch = cloneBranchConfig(src.Branch)
	}
	if src.Foreach != nil {
		dst.Foreach = cloneForeachConfig(src.Foreach)
	}
	if src.Agent != nil {
		dst.Agent = cloneAgentConfig(src.Agent)
	}

	return dst
}

func cloneShellConfig(src *types.ShellConfig) *types.ShellConfig {
	dst := &types.ShellConfig{
		Command: src.Command,
		Workdir: src.Workdir,
		OnError: src.OnError,
	}
	if src.Env != nil {
		dst.Env = make(map[string]string)
		for k, v := range src.Env {
			dst.Env[k] = v
		}
	}
	if src.Outputs != nil {
		dst.Outputs = make(map[string]types.OutputSource)
		for k, v := range src.Outputs {
			dst.Outputs[k] = v
		}
	}
	return dst
}

func cloneSpawnConfig(src *types.SpawnConfig) *types.SpawnConfig {
	dst := &types.SpawnConfig{
		Agent:         src.Agent,
		Adapter:       src.Adapter,
		Workdir:       src.Workdir,
		ResumeSession: src.ResumeSession,
		SpawnArgs:     src.SpawnArgs,
	}
	if src.Env != nil {
		dst.Env = make(map[string]string)
		for k, v := range src.Env {
			dst.Env[k] = v
		}
	}
	return dst
}

func cloneKillConfig(src *types.KillConfig) *types.KillConfig {
	return &types.KillConfig{
		Agent:    src.Agent,
		Graceful: src.Graceful,
		Timeout:  src.Timeout,
	}
}

func cloneExpandConfig(src *types.ExpandConfig) *types.ExpandConfig {
	dst := &types.ExpandConfig{
		Template: src.Template,
	}
	if src.Variables != nil {
		dst.Variables = make(map[string]any)
		for k, v := range src.Variables {
			dst.Variables[k] = v
		}
	}
	return dst
}

func cloneBranchConfig(src *types.BranchConfig) *types.BranchConfig {
	dst := &types.BranchConfig{
		Condition: src.Condition,
		Timeout:   src.Timeout,
	}
	if src.OnTrue != nil {
		dst.OnTrue = cloneBranchTarget(src.OnTrue)
	}
	if src.OnFalse != nil {
		dst.OnFalse = cloneBranchTarget(src.OnFalse)
	}
	if src.OnTimeout != nil {
		dst.OnTimeout = cloneBranchTarget(src.OnTimeout)
	}
	return dst
}

func cloneBranchTarget(src *types.BranchTarget) *types.BranchTarget {
	dst := &types.BranchTarget{
		Template: src.Template,
	}
	if src.Variables != nil {
		dst.Variables = make(map[string]any)
		for k, v := range src.Variables {
			dst.Variables[k] = v
		}
	}
	if src.Inline != nil {
		dst.Inline = make([]types.InlineStep, len(src.Inline))
		copy(dst.Inline, src.Inline)
	}
	return dst
}

func cloneAgentConfig(src *types.AgentConfig) *types.AgentConfig {
	dst := &types.AgentConfig{
		Agent:   src.Agent,
		Prompt:  src.Prompt,
		Mode:    src.Mode,
		Timeout: src.Timeout,
	}
	if src.Outputs != nil {
		dst.Outputs = make(map[string]types.AgentOutputDef)
		for k, v := range src.Outputs {
			dst.Outputs[k] = v
		}
	}
	return dst
}

// prefixNeeds updates dependency references for expanded steps.
// Internal dependencies (within the template) get prefixed.
// External dependencies are kept as-is.
// All expanded steps implicitly depend on the expand step itself.
func prefixNeeds(needs []string, parentID string, templateStepIDs map[string]bool) []string {
	result := make([]string, 0, len(needs)+1)

	for _, need := range needs {
		if templateStepIDs[need] {
			// Internal dependency - prefix with parent ID
			result = append(result, parentID+"."+need)
		} else {
			// External dependency - keep as-is
			result = append(result, need)
		}
	}

	// Steps without internal dependencies implicitly depend on the expand step
	// But we only add this if there are no internal deps, to avoid redundancy
	hasInternalDep := false
	for _, need := range needs {
		if templateStepIDs[need] {
			hasInternalDep = true
			break
		}
	}

	if !hasInternalDep {
		// Prepend the parent expand step as a dependency
		result = append([]string{parentID}, result...)
	}

	return result
}

// substituteStepVariablesTyped replaces {{variable}} placeholders in step configs using VarContext.
// String fields use ctx.Render() (always returns string, stringifies embedded values).
// Variables maps use ctx.EvalMap() (preserves typed values for downstream templates).
func substituteStepVariablesTyped(step *types.Step, ctx *workflow.VarContext) error {
	var err error

	switch step.Executor {
	case types.ExecutorShell:
		if step.Shell != nil {
			if step.Shell.Command, err = ctx.Render(step.Shell.Command); err != nil {
				return fmt.Errorf("shell.command: %w", err)
			}
			if step.Shell.Workdir, err = ctx.Render(step.Shell.Workdir); err != nil {
				return fmt.Errorf("shell.workdir: %w", err)
			}
			for k, v := range step.Shell.Env {
				if step.Shell.Env[k], err = ctx.Render(v); err != nil {
					return fmt.Errorf("shell.env.%s: %w", k, err)
				}
			}
		}
	case types.ExecutorSpawn:
		if step.Spawn != nil {
			if step.Spawn.Agent, err = ctx.Render(step.Spawn.Agent); err != nil {
				return fmt.Errorf("spawn.agent: %w", err)
			}
			if step.Spawn.Adapter, err = ctx.Render(step.Spawn.Adapter); err != nil {
				return fmt.Errorf("spawn.adapter: %w", err)
			}
			if step.Spawn.Workdir, err = ctx.Render(step.Spawn.Workdir); err != nil {
				return fmt.Errorf("spawn.workdir: %w", err)
			}
			if step.Spawn.ResumeSession, err = ctx.Render(step.Spawn.ResumeSession); err != nil {
				return fmt.Errorf("spawn.resume_session: %w", err)
			}
			if step.Spawn.SpawnArgs, err = ctx.Render(step.Spawn.SpawnArgs); err != nil {
				return fmt.Errorf("spawn.spawn_args: %w", err)
			}
			for k, v := range step.Spawn.Env {
				if step.Spawn.Env[k], err = ctx.Render(v); err != nil {
					return fmt.Errorf("spawn.env.%s: %w", k, err)
				}
			}
		}
	case types.ExecutorKill:
		if step.Kill != nil {
			if step.Kill.Agent, err = ctx.Render(step.Kill.Agent); err != nil {
				return fmt.Errorf("kill.agent: %w", err)
			}
		}
	case types.ExecutorExpand:
		if step.Expand != nil {
			if step.Expand.Template, err = ctx.Render(step.Expand.Template); err != nil {
				return fmt.Errorf("expand.template: %w", err)
			}
			// Use EvalMap for Variables to preserve types
			if step.Expand.Variables, err = ctx.EvalMap(step.Expand.Variables); err != nil {
				return fmt.Errorf("expand.variables: %w", err)
			}
		}
	case types.ExecutorBranch:
		if step.Branch != nil {
			if step.Branch.Condition, err = ctx.Render(step.Branch.Condition); err != nil {
				return fmt.Errorf("branch.condition: %w", err)
			}
			if step.Branch.OnTrue != nil {
				if step.Branch.OnTrue.Template, err = ctx.Render(step.Branch.OnTrue.Template); err != nil {
					return fmt.Errorf("branch.on_true.template: %w", err)
				}
				if step.Branch.OnTrue.Variables, err = ctx.EvalMap(step.Branch.OnTrue.Variables); err != nil {
					return fmt.Errorf("branch.on_true.variables: %w", err)
				}
			}
			if step.Branch.OnFalse != nil {
				if step.Branch.OnFalse.Template, err = ctx.Render(step.Branch.OnFalse.Template); err != nil {
					return fmt.Errorf("branch.on_false.template: %w", err)
				}
				if step.Branch.OnFalse.Variables, err = ctx.EvalMap(step.Branch.OnFalse.Variables); err != nil {
					return fmt.Errorf("branch.on_false.variables: %w", err)
				}
			}
			if step.Branch.OnTimeout != nil {
				if step.Branch.OnTimeout.Template, err = ctx.Render(step.Branch.OnTimeout.Template); err != nil {
					return fmt.Errorf("branch.on_timeout.template: %w", err)
				}
				if step.Branch.OnTimeout.Variables, err = ctx.EvalMap(step.Branch.OnTimeout.Variables); err != nil {
					return fmt.Errorf("branch.on_timeout.variables: %w", err)
				}
			}
		}
	case types.ExecutorForeach:
		if step.Foreach != nil {
			if step.Foreach.Items, err = ctx.Render(step.Foreach.Items); err != nil {
				return fmt.Errorf("foreach.items: %w", err)
			}
			if step.Foreach.Template, err = ctx.Render(step.Foreach.Template); err != nil {
				return fmt.Errorf("foreach.template: %w", err)
			}
			if step.Foreach.MaxConcurrent, err = ctx.Render(step.Foreach.MaxConcurrent); err != nil {
				return fmt.Errorf("foreach.max_concurrent: %w", err)
			}
			// Use EvalMap for Variables to preserve types
			if step.Foreach.Variables, err = ctx.EvalMap(step.Foreach.Variables); err != nil {
				return fmt.Errorf("foreach.variables: %w", err)
			}
		}
	case types.ExecutorAgent:
		if step.Agent != nil {
			if step.Agent.Agent, err = ctx.Render(step.Agent.Agent); err != nil {
				return fmt.Errorf("agent.agent: %w", err)
			}
			if step.Agent.Prompt, err = ctx.Render(step.Agent.Prompt); err != nil {
				return fmt.Errorf("agent.prompt: %w", err)
			}
		}
	}
	return nil
}

// buildVarContext creates a VarContext from a variables map.
// This is a helper for callers transitioning from the old map-based API.
// It restructures flat dotted keys (e.g., "task.name") into nested maps
// so VarContext can resolve paths like {{task.name}} correctly.
func buildVarContext(vars map[string]any) *workflow.VarContext {
	ctx := workflow.NewVarContext()
	// Enable deferring undefined variables so unresolved refs remain as {{...}}
	// for later substitution (matches old substituteVars behavior)
	ctx.DeferUndefinedVariables = true
	ctx.DeferStepOutputs = true

	// Build nested structures from flattened vars
	// This handles the case where foreach sets both "task" (JSON string) and "task.name"
	nested := make(map[string]any)
	for k, v := range vars {
		setNestedVar(nested, k, v)
	}

	for k, v := range nested {
		ctx.SetVariable(k, v)
	}
	return ctx
}

// setNestedVar sets a value in a map, creating nested maps as needed.
// For "task.name" = "foo", it creates {"task": {"name": "foo"}}.
// If a non-dotted key conflicts (e.g., "task" exists as a string),
// the nested version takes precedence since it provides more structure.
func setNestedVar(m map[string]any, key string, value any) {
	parts := splitKeyParts(key)

	// For non-dotted keys, don't overwrite existing nested maps
	// This handles the case where foreach sets both "task" (JSON string) and "task.name"
	// and map iteration order is non-deterministic
	if len(parts) == 1 {
		if existing, ok := m[key]; ok {
			if _, isMap := existing.(map[string]any); isMap {
				// Don't overwrite an existing nested map with a scalar value
				return
			}
		}
		m[key] = value
		return
	}

	// Navigate/create nested maps for dotted keys
	current := m
	for _, part := range parts[:len(parts)-1] {
		if existing, ok := current[part]; ok {
			if existingMap, ok := existing.(map[string]any); ok {
				current = existingMap
			} else {
				// Existing value is not a map - create new nested map
				// This overwrites non-map values like JSON strings with nested maps
				newMap := make(map[string]any)
				current[part] = newMap
				current = newMap
			}
		} else {
			newMap := make(map[string]any)
			current[part] = newMap
			current = newMap
		}
	}
	current[parts[len(parts)-1]] = value
}

// splitKeyParts splits a key by dots, but returns the whole key as a single part
// if there are no dots.
func splitKeyParts(key string) []string {
	if key == "" {
		return []string{key}
	}
	// Use strings package for splitting
	parts := make([]string, 0)
	start := 0
	for i := 0; i < len(key); i++ {
		if key[i] == '.' {
			parts = append(parts, key[start:i])
			start = i + 1
		}
	}
	parts = append(parts, key[start:])
	return parts
}
