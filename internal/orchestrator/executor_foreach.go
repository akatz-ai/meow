package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/meow-stack/meow-machine/internal/types"
)

// fileTemplateLoader implements TemplateLoader using FileTemplateExpander.
type fileTemplateLoader struct {
	expander     *FileTemplateExpander
	sourceModule string
	workflowID   string
}

// Load implements TemplateLoader by using the FileTemplateExpander.
// Templates are loaded with DeferUndefinedVariables=true so that foreach
// can substitute item_var/index_var after cloning for each iteration.
func (l *fileTemplateLoader) Load(ctx context.Context, ref string, variables map[string]string) ([]*types.Step, error) {
	// Create expand config with variables to satisfy required template variables
	cfg := &types.ExpandConfig{
		Template:  ref,
		Variables: variables,
	}

	// Use ExpandWithOptions to defer undefined variables for later substitution
	opts := &ExpandOptions{
		DeferUndefinedVariables: true,
	}

	// Use a temporary parent step ID - we'll rename them later
	result, err := l.expander.ExpandWithOptions(ctx, cfg, "_tmp", l.workflowID, l.sourceModule, opts)
	if err != nil {
		return nil, err
	}

	// Strip the temporary prefix from step IDs
	for _, step := range result.Steps {
		step.ID = strings.TrimPrefix(step.ID, "_tmp.")
		// Also fix needs references
		for i, need := range step.Needs {
			step.Needs[i] = strings.TrimPrefix(need, "_tmp.")
		}
		// Clear ExpandedFrom since we're providing raw template steps
		step.ExpandedFrom = ""
	}

	return result.Steps, nil
}

// ExecuteForeachResult contains the results of foreach expansion.
type ExecuteForeachResult struct {
	ExpandedSteps []*types.Step // All newly created steps
	StepIDs       []string      // IDs of the expanded steps (top-level per iteration)
	IterationIDs  []string      // IDs of each iteration prefix (e.g., "foreach.0", "foreach.1")
}

// ExecuteForeach expands a template for each item in a list.
// It returns the expanded steps without modifying any workflow state.
// The caller is responsible for adding the steps to the workflow.
//
// Parameters:
// - step: The foreach step containing the iteration config
// - loader: Interface for loading templates
// - variables: Workflow-level variables for substitution
// - depth: Current expansion depth (for limit checking)
// - limits: Resource limits for expansion
func ExecuteForeach(
	ctx context.Context,
	step *types.Step,
	loader TemplateLoader,
	variables map[string]string,
	depth int,
	limits *ExpansionLimits,
) (*ExecuteForeachResult, *types.StepError) {
	if step.Foreach == nil {
		return nil, &types.StepError{Message: "foreach step missing config"}
	}

	cfg := step.Foreach

	// Validate required fields
	if cfg.Items == "" && cfg.ItemsFile == "" {
		return nil, &types.StepError{Message: "foreach step requires either items or items_file"}
	}
	if cfg.Items != "" && cfg.ItemsFile != "" {
		return nil, &types.StepError{Message: "foreach step cannot have both items and items_file"}
	}
	if cfg.ItemVar == "" {
		return nil, &types.StepError{Message: "foreach step missing item_var field"}
	}
	if cfg.Template == "" {
		return nil, &types.StepError{Message: "foreach step missing template field"}
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

	// Get items either from expression or file
	var items []any
	var err error
	if cfg.ItemsFile != "" {
		// Read items from file - bypasses variable substitution escaping issues
		items, err = readItemsFromFile(cfg.ItemsFile)
		if err != nil {
			return nil, &types.StepError{
				Message: fmt.Sprintf("failed to read items from file %s: %v", cfg.ItemsFile, err),
			}
		}
	} else {
		// Evaluate items expression
		items, err = evaluateItemsExpression(cfg.Items, variables)
		if err != nil {
			return nil, &types.StepError{
				Message: fmt.Sprintf("failed to evaluate items expression: %v", err),
			}
		}
	}

	// Empty array - nothing to expand
	if len(items) == 0 {
		return &ExecuteForeachResult{
			ExpandedSteps: nil,
			StepIDs:       nil,
			IterationIDs:  nil,
		}, nil
	}

	// Load the template once, passing the foreach config's variables
	templateSteps, err := loader.Load(ctx, cfg.Template, cfg.Variables)
	if err != nil {
		return nil, &types.StepError{
			Message: fmt.Sprintf("failed to load template %s: %v", cfg.Template, err),
		}
	}

	if len(templateSteps) == 0 {
		// Empty template - no steps per iteration
		return &ExecuteForeachResult{
			ExpandedSteps: nil,
			StepIDs:       nil,
			IterationIDs:  nil,
		}, nil
	}

	// Build set of template step IDs for dependency resolution
	templateStepIDs := make(map[string]bool)
	for _, ts := range templateSteps {
		templateStepIDs[ts.ID] = true
	}

	result := &ExecuteForeachResult{
		ExpandedSteps: make([]*types.Step, 0, len(items)*len(templateSteps)),
		StepIDs:       make([]string, 0, len(items)*len(templateSteps)),
		IterationIDs:  make([]string, 0, len(items)),
	}

	// Track the last step of the previous iteration (for sequential mode)
	var prevIterationLastStepID string

	// Expand for each item
	for i, item := range items {
		iterationPrefix := fmt.Sprintf("%s.%d", step.ID, i)
		result.IterationIDs = append(result.IterationIDs, iterationPrefix)

		// Build iteration-specific variables
		iterVars := make(map[string]string)
		// Copy workflow variables
		for k, v := range variables {
			iterVars[k] = v
		}
		// Copy foreach step variables
		for k, v := range cfg.Variables {
			iterVars[k] = v
		}
		// Set item_var (serialize item to JSON for object access)
		itemJSON, _ := json.Marshal(item)
		iterVars[cfg.ItemVar] = string(itemJSON)
		// Set index_var if specified
		if cfg.IndexVar != "" {
			iterVars[cfg.IndexVar] = strconv.Itoa(i)
		}

		// Also add flattened item fields for simple object access
		// e.g., item_var = "task" -> {{task.name}} becomes available
		if itemMap, ok := item.(map[string]any); ok {
			addFlattenedFields(iterVars, cfg.ItemVar, itemMap)
		}

		var iterFirstStepID string

		// Expand each template step for this iteration
		for j, tmplStep := range templateSteps {
			// Create new step with prefixed ID: {foreach_id}.{index}.{step_id}
			newID := fmt.Sprintf("%s.%s", iterationPrefix, tmplStep.ID)
			newStep := cloneStep(tmplStep)
			newStep.ID = newID
			newStep.Status = types.StepStatusPending
			newStep.ExpandedFrom = step.ID

			// Substitute variables in config (including item_var and index_var)
			substituteStepVariables(newStep, iterVars)

			// Update dependencies to use prefixed IDs
			newStep.Needs = prefixForeachNeeds(
				tmplStep.Needs,
				iterationPrefix,
				step.ID,
				templateStepIDs,
				prevIterationLastStepID,
				!cfg.IsParallel() && i > 0, // Add dependency on prev iteration if sequential
			)

			// Track first step of this iteration (for sequential dependencies)
			if j == 0 {
				iterFirstStepID = newID
			}

			result.ExpandedSteps = append(result.ExpandedSteps, newStep)
			result.StepIDs = append(result.StepIDs, newID)
		}

		// Track last step of this iteration for sequential mode
		if len(templateSteps) > 0 {
			lastStepID := fmt.Sprintf("%s.%s", iterationPrefix, templateSteps[len(templateSteps)-1].ID)
			prevIterationLastStepID = lastStepID
		}

		// If not used, at least reference to avoid unused variable warning
		_ = iterFirstStepID
	}

	return result, nil
}

// readItemsFromFile reads a JSON array from a file.
// This bypasses variable substitution entirely, avoiding escaping issues
// when JSON contains embedded newlines or special characters.
func readItemsFromFile(path string) ([]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	var items []any
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parsing JSON array: %w", err)
	}

	return items, nil
}

// evaluateItemsExpression parses the items expression and returns the array.
// The expression can be:
// - A JSON array literal: ["a", "b", "c"]
// - A variable reference: {{planner.outputs.tasks}}
// - A variable reference that resolves to JSON
func evaluateItemsExpression(expr string, variables map[string]string) ([]any, error) {
	// First, substitute any variables in the expression
	resolved := substituteVars(expr, variables)

	// Try to parse as JSON array
	var items []any
	if err := json.Unmarshal([]byte(resolved), &items); err != nil {
		// Try to parse as JSON object in case it's a single item
		var singleItem any
		if jsonErr := json.Unmarshal([]byte(resolved), &singleItem); jsonErr == nil {
			// If it's an array at the top level, we already failed
			// If it's something else, wrap it
			if _, isArray := singleItem.([]any); !isArray {
				return nil, fmt.Errorf("items must be a JSON array, got: %s", resolved)
			}
		}
		return nil, fmt.Errorf("invalid JSON array: %w (expression: %s)", err, resolved)
	}

	return items, nil
}

// addFlattenedFields adds flattened object fields to the variables map.
// For item_var="task" and item={name: "foo", priority: 1}:
// - task.name = "foo"
// - task.priority = "1"
func addFlattenedFields(vars map[string]string, prefix string, obj map[string]any) {
	for key, val := range obj {
		fullKey := prefix + "." + key
		switch v := val.(type) {
		case string:
			vars[fullKey] = v
		case float64:
			// JSON numbers are float64
			if v == float64(int(v)) {
				vars[fullKey] = strconv.Itoa(int(v))
			} else {
				vars[fullKey] = strconv.FormatFloat(v, 'f', -1, 64)
			}
		case bool:
			vars[fullKey] = strconv.FormatBool(v)
		case nil:
			vars[fullKey] = ""
		case map[string]any:
			// Nested object - recurse
			addFlattenedFields(vars, fullKey, v)
		case []any:
			// Arrays - serialize as JSON
			jsonBytes, _ := json.Marshal(v)
			vars[fullKey] = string(jsonBytes)
		default:
			// Fallback to JSON serialization
			jsonBytes, _ := json.Marshal(v)
			vars[fullKey] = string(jsonBytes)
		}
	}
}

// prefixForeachNeeds updates dependency references for foreach-expanded steps.
// - Internal template dependencies get the iteration prefix
// - External dependencies are kept as-is
// - In sequential mode, first step of each iteration depends on last step of prev iteration
// NOTE: Child steps do NOT depend on the foreach step itself - the foreach step stays
// "running" until all children complete (implicit join), so adding it as a dependency
// would create a circular wait.
func prefixForeachNeeds(
	needs []string,
	iterationPrefix string, // e.g., "parallel-workers.0"
	foreachStepID string, // e.g., "parallel-workers" (unused now but kept for compatibility)
	templateStepIDs map[string]bool,
	prevIterationLastStepID string,
	addSequentialDep bool,
) []string {
	result := make([]string, 0, len(needs)+1)

	hasInternalDep := false
	for _, need := range needs {
		// Check if need is a direct template step ID
		if templateStepIDs[need] {
			// Internal dependency - prefix with iteration
			result = append(result, iterationPrefix+"."+need)
			hasInternalDep = true
		} else if firstDot := strings.Index(need, "."); firstDot > 0 {
			// Check if it's a dotted reference where the first segment is a template step
			// e.g., "track.done" where "track" is a template step
			firstSegment := need[:firstDot]
			if templateStepIDs[firstSegment] {
				// Internal dependency with sub-reference - prefix with iteration
				result = append(result, iterationPrefix+"."+need)
				hasInternalDep = true
			} else {
				// External dependency - keep as-is
				result = append(result, need)
			}
		} else {
			// External dependency - keep as-is
			result = append(result, need)
		}
	}

	// In sequential mode, add dependency on previous iteration's last step
	// This is the only case where we add dependencies - to chain iterations
	if addSequentialDep && prevIterationLastStepID != "" {
		// Only add if this step has no internal deps (i.e., it's a "first" step)
		if !hasInternalDep {
			result = append(result, prevIterationLastStepID)
		}
	}

	return result
}

// cloneForeachConfig creates a deep copy of ForeachConfig.
func cloneForeachConfig(src *types.ForeachConfig) *types.ForeachConfig {
	if src == nil {
		return nil
	}

	dst := &types.ForeachConfig{
		Items:         src.Items,
		ItemsFile:     src.ItemsFile,
		ItemVar:       src.ItemVar,
		IndexVar:      src.IndexVar,
		Template:      src.Template,
		MaxConcurrent: src.MaxConcurrent,
	}

	if src.Parallel != nil {
		p := *src.Parallel
		dst.Parallel = &p
	}
	if src.Join != nil {
		j := *src.Join
		dst.Join = &j
	}
	if src.Variables != nil {
		dst.Variables = make(map[string]string)
		for k, v := range src.Variables {
			dst.Variables[k] = v
		}
	}

	return dst
}

// IsForeachComplete checks if all children of a foreach step are done.
// Used for implicit join semantics.
func IsForeachComplete(foreachStep *types.Step, allSteps map[string]*types.Step) bool {
	if foreachStep.ExpandedInto == nil || len(foreachStep.ExpandedInto) == 0 {
		return true // No children = complete
	}

	// Check all expanded steps
	for _, childID := range foreachStep.ExpandedInto {
		child, ok := allSteps[childID]
		if !ok {
			continue // Step not found - treat as complete (may have been cleaned up)
		}
		if child.Status != types.StepStatusDone && child.Status != types.StepStatusFailed {
			return false // Still running
		}
	}

	return true
}

// IsForeachFailed checks if any child of a foreach step has failed.
func IsForeachFailed(foreachStep *types.Step, allSteps map[string]*types.Step) bool {
	if foreachStep.ExpandedInto == nil {
		return false
	}

	for _, childID := range foreachStep.ExpandedInto {
		child, ok := allSteps[childID]
		if !ok {
			continue
		}
		if child.Status == types.StepStatusFailed {
			return true
		}
	}

	return false
}

// CountRunningIterations counts how many iterations are currently running.
// Used for max_concurrent limiting.
func CountRunningIterations(foreachStep *types.Step, allSteps map[string]*types.Step) int {
	if foreachStep.ExpandedInto == nil {
		return 0
	}

	// Group steps by iteration prefix
	runningIterations := make(map[string]bool)

	for _, childID := range foreachStep.ExpandedInto {
		child, ok := allSteps[childID]
		if !ok {
			continue
		}
		if child.Status == types.StepStatusRunning || child.Status == types.StepStatusCompleting {
			// Extract iteration prefix (e.g., "foreach.0" from "foreach.0.step-a")
			parts := strings.SplitN(childID, ".", 3)
			if len(parts) >= 2 {
				iterPrefix := parts[0] + "." + parts[1]
				runningIterations[iterPrefix] = true
			}
		}
	}

	return len(runningIterations)
}

// GetIterationStepIDs returns all step IDs for a specific iteration.
func GetIterationStepIDs(foreachStep *types.Step, iterationIndex int) []string {
	if foreachStep.ExpandedInto == nil {
		return nil
	}

	prefix := fmt.Sprintf("%s.%d.", foreachStep.ID, iterationIndex)
	var stepIDs []string

	for _, childID := range foreachStep.ExpandedInto {
		if strings.HasPrefix(childID, prefix) {
			stepIDs = append(stepIDs, childID)
		}
	}

	return stepIDs
}
