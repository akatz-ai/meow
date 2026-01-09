package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/meow-stack/meow-machine/internal/types"
)

// TemplateLoader loads template workflows for expansion.
type TemplateLoader interface {
	// Load retrieves a template by reference.
	// Template references can be:
	// - ".workflow-name" - same file, specific workflow
	// - "main" - same file, workflow named "main"
	// - "file#workflow" - external file, specific workflow
	// - "file" - external file, workflow named "main"
	Load(ctx context.Context, ref string) ([]*types.Step, error)
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
	variables map[string]string,
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

	// Load the template
	templateSteps, err := loader.Load(ctx, cfg.Template)
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
	mergedVars := make(map[string]string)
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

		// Substitute variables in config
		substituteStepVariables(newStep, mergedVars)

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
		Workdir:       src.Workdir,
		ResumeSession: src.ResumeSession,
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
		dst.Variables = make(map[string]string)
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
		dst.Variables = make(map[string]string)
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

// substituteStepVariables replaces {{variable}} placeholders in step configs.
func substituteStepVariables(step *types.Step, vars map[string]string) {
	switch step.Executor {
	case types.ExecutorShell:
		if step.Shell != nil {
			step.Shell.Command = substituteVars(step.Shell.Command, vars)
			step.Shell.Workdir = substituteVars(step.Shell.Workdir, vars)
			for k, v := range step.Shell.Env {
				step.Shell.Env[k] = substituteVars(v, vars)
			}
		}
	case types.ExecutorSpawn:
		if step.Spawn != nil {
			step.Spawn.Agent = substituteVars(step.Spawn.Agent, vars)
			step.Spawn.Workdir = substituteVars(step.Spawn.Workdir, vars)
			step.Spawn.ResumeSession = substituteVars(step.Spawn.ResumeSession, vars)
			for k, v := range step.Spawn.Env {
				step.Spawn.Env[k] = substituteVars(v, vars)
			}
		}
	case types.ExecutorKill:
		if step.Kill != nil {
			step.Kill.Agent = substituteVars(step.Kill.Agent, vars)
		}
	case types.ExecutorExpand:
		if step.Expand != nil {
			step.Expand.Template = substituteVars(step.Expand.Template, vars)
			for k, v := range step.Expand.Variables {
				step.Expand.Variables[k] = substituteVars(v, vars)
			}
		}
	case types.ExecutorBranch:
		if step.Branch != nil {
			step.Branch.Condition = substituteVars(step.Branch.Condition, vars)
			if step.Branch.OnTrue != nil {
				step.Branch.OnTrue.Template = substituteVars(step.Branch.OnTrue.Template, vars)
				for k, v := range step.Branch.OnTrue.Variables {
					step.Branch.OnTrue.Variables[k] = substituteVars(v, vars)
				}
			}
			if step.Branch.OnFalse != nil {
				step.Branch.OnFalse.Template = substituteVars(step.Branch.OnFalse.Template, vars)
				for k, v := range step.Branch.OnFalse.Variables {
					step.Branch.OnFalse.Variables[k] = substituteVars(v, vars)
				}
			}
			if step.Branch.OnTimeout != nil {
				step.Branch.OnTimeout.Template = substituteVars(step.Branch.OnTimeout.Template, vars)
				for k, v := range step.Branch.OnTimeout.Variables {
					step.Branch.OnTimeout.Variables[k] = substituteVars(v, vars)
				}
			}
		}
	case types.ExecutorAgent:
		if step.Agent != nil {
			step.Agent.Agent = substituteVars(step.Agent.Agent, vars)
			step.Agent.Prompt = substituteVars(step.Agent.Prompt, vars)
		}
	}
}

// substituteVars replaces {{variable}} placeholders with values from vars map.
func substituteVars(s string, vars map[string]string) string {
	if !strings.Contains(s, "{{") {
		return s
	}

	result := s
	for k, v := range vars {
		placeholder := "{{" + k + "}}"
		result = strings.ReplaceAll(result, placeholder, v)
	}
	return result
}
