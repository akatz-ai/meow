package template

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// StepInfo contains the information needed from a step for output resolution.
type StepInfo struct {
	ID      string
	Status  string // "pending", "running", "done", "failed"
	Outputs map[string]any
}

// StepLookupFunc retrieves step information by ID.
// Returns nil, nil if the step is not found.
type StepLookupFunc func(stepID string) (*StepInfo, error)

// VarContext holds the context for variable substitution.
type VarContext struct {
	// Variables are user-defined template variables
	Variables map[string]any

	// Outputs are step outputs keyed by step ID
	// e.g., outputs["step-1"]["stdout"] = "hello"
	Outputs map[string]map[string]any

	// Builtins are auto-populated variables
	Builtins map[string]any

	// StepLookup is an optional function to fetch step outputs dynamically.
	// When set, resolveOutput will use this to fetch outputs for steps not
	// already in the Outputs map.
	StepLookup StepLookupFunc

	// DeferStepOutputs controls behavior when step outputs can't be resolved.
	// When true, unresolvable step output references ({{step.outputs.field}})
	// are left as-is for runtime resolution instead of causing an error.
	// This is useful during template baking where step outputs aren't yet available.
	DeferStepOutputs bool
}

// NewVarContext creates a new variable context with default builtins.
func NewVarContext() *VarContext {
	return &VarContext{
		Variables: make(map[string]any),
		Outputs:   make(map[string]map[string]any),
		Builtins:  make(map[string]any),
	}
}

// SetVariable sets a user-defined variable.
func (c *VarContext) SetVariable(name string, value any) {
	c.Variables[name] = value
}

// Set is an alias for SetVariable for convenience.
func (c *VarContext) Set(name string, value any) {
	c.SetVariable(name, value)
}

// Get returns the value of a user-defined variable, or empty string if not set.
func (c *VarContext) Get(name string) string {
	if val, ok := c.Variables[name]; ok {
		return fmt.Sprintf("%v", val)
	}
	return ""
}

// SetBuiltin sets a builtin variable (e.g., agent, step_id).
func (c *VarContext) SetBuiltin(name string, value any) {
	c.Builtins[name] = value
}

// SetOutput sets an output value for a step.
func (c *VarContext) SetOutput(stepID, field string, value any) {
	if c.Outputs[stepID] == nil {
		c.Outputs[stepID] = make(map[string]any)
	}
	c.Outputs[stepID][field] = value
}

// SetOutputs sets all outputs for a step.
func (c *VarContext) SetOutputs(stepID string, outputs map[string]any) {
	c.Outputs[stepID] = outputs
}

// SetStepLookup sets the function used to look up step outputs dynamically.
// When a {{step_id.outputs.field}} reference is encountered and the step's outputs
// are not in the cache, this function will be called to fetch them.
func (c *VarContext) SetStepLookup(fn StepLookupFunc) {
	c.StepLookup = fn
}

// errDeferred is a sentinel error indicating the variable should be left for runtime
var errDeferred = fmt.Errorf("deferred for runtime")

// varPattern matches {{variable.path}} patterns
var varPattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// Substitute replaces all {{...}} patterns in the input string.
// Returns an error if a required variable is missing (unless deferred).
func (c *VarContext) Substitute(input string) (string, error) {
	var lastErr error
	maxDepth := 10 // Prevent infinite recursion

	result := input
	for i := 0; i < maxDepth; i++ {
		newResult := varPattern.ReplaceAllStringFunc(result, func(match string) string {
			// Extract the variable path (remove {{ and }})
			path := strings.TrimSpace(match[2 : len(match)-2])

			value, err := c.resolve(path)
			if err != nil {
				// If deferred, keep the original pattern for runtime resolution
				if err == errDeferred {
					return match
				}
				lastErr = err
				return match // Keep original on error
			}

			return fmt.Sprintf("%v", value)
		})

		if newResult == result {
			// No more substitutions
			break
		}
		result = newResult
	}

	// Check for remaining unresolved variables (only if not in defer mode)
	if !c.DeferStepOutputs && varPattern.MatchString(result) && lastErr == nil {
		matches := varPattern.FindAllString(result, -1)
		lastErr = fmt.Errorf("unresolved variables after max depth: %v", matches)
	}

	return result, lastErr
}

// resolve looks up a variable path and returns its value.
func (c *VarContext) resolve(path string) (any, error) {
	parts := strings.Split(path, ".")

	if len(parts) == 0 {
		return nil, fmt.Errorf("empty variable path")
	}

	root := parts[0]

	// Check for output reference: step_id.outputs.field
	if len(parts) >= 3 && parts[1] == "outputs" {
		stepID := parts[0]
		field := strings.Join(parts[2:], ".")
		return c.resolveOutput(stepID, field)
	}

	// Check for special prefixes
	if root == "output" && len(parts) >= 3 {
		// output.step.field format
		stepID := parts[1]
		field := strings.Join(parts[2:], ".")
		return c.resolveOutput(stepID, field)
	}

	// Check user variables first
	if val, ok := c.Variables[root]; ok {
		return c.resolvePath(val, parts[1:])
	}

	// Check builtins
	if val, ok := c.Builtins[root]; ok {
		return c.resolvePath(val, parts[1:])
	}

	// Dynamic time builtins - computed fresh at resolution time
	switch root {
	case "timestamp":
		return time.Now().Format(time.RFC3339), nil
	case "date":
		return time.Now().Format("2006-01-02"), nil
	case "time":
		return time.Now().Format("15:04:05"), nil
	}

	return nil, fmt.Errorf("undefined variable: %s", root)
}

// resolveOutput looks up a step output.
// If the step's outputs are not cached and a StepLookup function is set,
// it will be used to fetch the step and its outputs.
func (c *VarContext) resolveOutput(stepID, field string) (any, error) {
	outputs, ok := c.Outputs[stepID]
	if !ok {
		// Try to fetch from StepLookup if available
		if c.StepLookup != nil {
			info, err := c.StepLookup(stepID)
			if err != nil {
				return nil, fmt.Errorf("looking up step %q: %w", stepID, err)
			}
			if info == nil {
				// Step not found - defer if enabled
				if c.DeferStepOutputs {
					return nil, errDeferred
				}
				return nil, fmt.Errorf("step %q not found", stepID)
			}
			// Check if step is done (outputs only available after completion)
			if info.Status != "done" {
				if c.DeferStepOutputs {
					return nil, errDeferred
				}
				return nil, fmt.Errorf("step %q is not done (status: %s), outputs not available", stepID, info.Status)
			}
			if info.Outputs == nil {
				return nil, fmt.Errorf("step %q has no outputs", stepID)
			}
			// Cache the outputs for future lookups
			c.Outputs[stepID] = info.Outputs
			outputs = info.Outputs
		} else {
			// No lookup function - defer if enabled, otherwise error
			if c.DeferStepOutputs {
				return nil, errDeferred
			}
			return nil, fmt.Errorf("no outputs for step %q", stepID)
		}
	}

	// Handle nested field access
	parts := strings.Split(field, ".")
	var val any = outputs

	for _, part := range parts {
		switch v := val.(type) {
		case map[string]any:
			var ok bool
			val, ok = v[part]
			if !ok {
				// List available fields to help with debugging
				available := make([]string, 0, len(v))
				for k := range v {
					available = append(available, k)
				}
				return nil, fmt.Errorf("output %q not found in step %q (available: %v)", field, stepID, available)
			}
		default:
			return nil, fmt.Errorf("cannot access field %q on non-map value in step %q", part, stepID)
		}
	}

	return val, nil
}

// resolvePath navigates nested structures.
func (c *VarContext) resolvePath(val any, parts []string) (any, error) {
	for _, part := range parts {
		switch v := val.(type) {
		case map[string]any:
			var ok bool
			val, ok = v[part]
			if !ok {
				return nil, fmt.Errorf("field %q not found", part)
			}
		case map[string]string:
			var ok bool
			val, ok = v[part]
			if !ok {
				return nil, fmt.Errorf("field %q not found", part)
			}
		default:
			return nil, fmt.Errorf("cannot access field %q on non-map value", part)
		}
	}
	return val, nil
}

// ApplyDefaults sets default values for variables that are not already set.
func (c *VarContext) ApplyDefaults(vars map[string]Var) {
	for name, v := range vars {
		if _, ok := c.Variables[name]; !ok && v.Default != nil {
			c.Variables[name] = v.Default
		}
	}
}

// ValidateRequired checks that all required variables are set.
// Variables with defaults are not considered missing even if Required is true.
func (c *VarContext) ValidateRequired(vars map[string]Var) error {
	var missing []string
	for name, v := range vars {
		if v.Required && v.Default == nil {
			if _, ok := c.Variables[name]; !ok {
				missing = append(missing, name)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required variables: %v", missing)
	}
	return nil
}

// SubstituteMap substitutes variables in all values of a string map.
func (c *VarContext) SubstituteMap(m map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(m))
	for k, v := range m {
		substituted, err := c.Substitute(v)
		if err != nil {
			return nil, fmt.Errorf("key %q: %w", k, err)
		}
		result[k] = substituted
	}
	return result, nil
}

// SubstituteStep substitutes variables in all string fields of a Step.
func (c *VarContext) SubstituteStep(step *Step) (*Step, error) {
	if step == nil {
		return nil, nil
	}

	// Create a copy to avoid modifying the original
	result := *step

	var err error

	// Substitute simple string fields
	if result.Description != "" {
		result.Description, err = c.Substitute(result.Description)
		if err != nil {
			return nil, fmt.Errorf("description: %w", err)
		}
	}

	if result.Prompt != "" {
		result.Prompt, err = c.Substitute(result.Prompt)
		if err != nil {
			return nil, fmt.Errorf("prompt: %w", err)
		}
	}

	// Legacy instructions field
	if result.Instructions != "" {
		result.Instructions, err = c.Substitute(result.Instructions)
		if err != nil {
			return nil, fmt.Errorf("instructions: %w", err)
		}
	}

	if result.Condition != "" {
		result.Condition, err = c.Substitute(result.Condition)
		if err != nil {
			return nil, fmt.Errorf("condition: %w", err)
		}
	}

	if result.Template != "" {
		result.Template, err = c.Substitute(result.Template)
		if err != nil {
			return nil, fmt.Errorf("template: %w", err)
		}
	}

	if result.Command != "" {
		result.Command, err = c.Substitute(result.Command)
		if err != nil {
			return nil, fmt.Errorf("command: %w", err)
		}
	}

	if result.Code != "" {
		result.Code, err = c.Substitute(result.Code)
		if err != nil {
			return nil, fmt.Errorf("code: %w", err)
		}
	}

	if result.Validation != "" {
		result.Validation, err = c.Substitute(result.Validation)
		if err != nil {
			return nil, fmt.Errorf("validation: %w", err)
		}
	}

	if result.Timeout != "" {
		result.Timeout, err = c.Substitute(result.Timeout)
		if err != nil {
			return nil, fmt.Errorf("timeout: %w", err)
		}
	}

	// Substitute variables map
	if len(result.Variables) > 0 {
		result.Variables, err = c.SubstituteMap(result.Variables)
		if err != nil {
			return nil, fmt.Errorf("variables: %w", err)
		}
	}

	// Substitute expansion targets
	if result.OnTrue != nil {
		result.OnTrue, err = c.substituteExpansionTarget(result.OnTrue, "on_true")
		if err != nil {
			return nil, err
		}
	}

	if result.OnFalse != nil {
		result.OnFalse, err = c.substituteExpansionTarget(result.OnFalse, "on_false")
		if err != nil {
			return nil, err
		}
	}

	if result.OnTimeout != nil {
		result.OnTimeout, err = c.substituteExpansionTarget(result.OnTimeout, "on_timeout")
		if err != nil {
			return nil, err
		}
	}

	return &result, nil
}

// substituteExpansionTarget substitutes variables in an ExpansionTarget.
func (c *VarContext) substituteExpansionTarget(target *ExpansionTarget, fieldName string) (*ExpansionTarget, error) {
	if target == nil {
		return nil, nil
	}

	result := *target
	var err error

	if result.Template != "" {
		result.Template, err = c.Substitute(result.Template)
		if err != nil {
			return nil, fmt.Errorf("%s.template: %w", fieldName, err)
		}
	}

	if len(result.Variables) > 0 {
		result.Variables, err = c.SubstituteMap(result.Variables)
		if err != nil {
			return nil, fmt.Errorf("%s.variables: %w", fieldName, err)
		}
	}

	return &result, nil
}

// ShellEscape wraps a string in single quotes for safe shell usage.
// Any single quotes in the string are properly escaped using the '"'"' technique.
func ShellEscape(s string) string {
	// Replace single quotes with '"'"' (end single quote, double-quote the single quote, start single quote)
	escaped := strings.ReplaceAll(s, "'", "'\"'\"'")
	return "'" + escaped + "'"
}

// SubstituteForShell substitutes variables and shell-escapes the values.
// This prevents command injection when variable values contain shell metacharacters.
// Unlike Substitute, this does NOT do recursive substitution - values are escaped once.
func (c *VarContext) SubstituteForShell(input string) (string, error) {
	var lastErr error

	result := varPattern.ReplaceAllStringFunc(input, func(match string) string {
		// Extract the variable path (remove {{ and }})
		path := strings.TrimSpace(match[2 : len(match)-2])

		value, err := c.resolve(path)
		if err != nil {
			if err == errDeferred {
				return match
			}
			lastErr = err
			return match
		}

		// Shell-escape the value to prevent injection
		return ShellEscape(fmt.Sprintf("%v", value))
	})

	return result, lastErr
}
