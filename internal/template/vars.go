package template

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// BeadInfo contains the information needed from a bead for output resolution.
type BeadInfo struct {
	ID      string
	Status  string // "open", "in_progress", "closed"
	Outputs map[string]any
}

// BeadLookupFunc retrieves bead information by ID.
// Returns nil, nil if the bead is not found.
type BeadLookupFunc func(beadID string) (*BeadInfo, error)

// VarContext holds the context for variable substitution.
type VarContext struct {
	// Variables are user-defined template variables
	Variables map[string]any

	// Outputs are bead outputs keyed by bead ID
	// e.g., outputs["step-1"]["stdout"] = "hello"
	Outputs map[string]map[string]any

	// Builtins are auto-populated variables
	Builtins map[string]any

	// BeadLookup is an optional function to fetch bead outputs dynamically.
	// When set, resolveOutput will use this to fetch outputs for beads not
	// already in the Outputs map.
	BeadLookup BeadLookupFunc
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

// SetBuiltin sets a builtin variable (e.g., agent, bead_id).
func (c *VarContext) SetBuiltin(name string, value any) {
	c.Builtins[name] = value
}

// SetOutput sets an output value for a bead.
func (c *VarContext) SetOutput(beadID, field string, value any) {
	if c.Outputs[beadID] == nil {
		c.Outputs[beadID] = make(map[string]any)
	}
	c.Outputs[beadID][field] = value
}

// SetOutputs sets all outputs for a bead.
func (c *VarContext) SetOutputs(beadID string, outputs map[string]any) {
	c.Outputs[beadID] = outputs
}

// SetBeadLookup sets the function used to look up bead outputs dynamically.
// When a {{bead_id.outputs.field}} reference is encountered and the bead's outputs
// are not in the cache, this function will be called to fetch them.
func (c *VarContext) SetBeadLookup(fn BeadLookupFunc) {
	c.BeadLookup = fn
}

// varPattern matches {{variable.path}} patterns
var varPattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// Substitute replaces all {{...}} patterns in the input string.
// Returns an error if a required variable is missing.
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

	// Check for remaining unresolved variables
	if varPattern.MatchString(result) && lastErr == nil {
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

	// Check for output reference: bead_id.outputs.field
	if len(parts) >= 3 && parts[1] == "outputs" {
		beadID := parts[0]
		field := strings.Join(parts[2:], ".")
		return c.resolveOutput(beadID, field)
	}

	// Check for special prefixes
	if root == "output" && len(parts) >= 3 {
		// output.step.field format
		beadID := parts[1]
		field := strings.Join(parts[2:], ".")
		return c.resolveOutput(beadID, field)
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

// resolveOutput looks up a bead output.
// If the bead's outputs are not cached and a BeadLookup function is set,
// it will be used to fetch the bead and its outputs.
func (c *VarContext) resolveOutput(beadID, field string) (any, error) {
	outputs, ok := c.Outputs[beadID]
	if !ok {
		// Try to fetch from BeadLookup if available
		if c.BeadLookup != nil {
			info, err := c.BeadLookup(beadID)
			if err != nil {
				return nil, fmt.Errorf("looking up bead %q: %w", beadID, err)
			}
			if info == nil {
				return nil, fmt.Errorf("bead %q not found", beadID)
			}
			// Check if bead is closed (outputs only available after closing)
			if info.Status != "closed" {
				return nil, fmt.Errorf("bead %q is not closed (status: %s), outputs not available", beadID, info.Status)
			}
			if info.Outputs == nil {
				return nil, fmt.Errorf("bead %q has no outputs", beadID)
			}
			// Cache the outputs for future lookups
			c.Outputs[beadID] = info.Outputs
			outputs = info.Outputs
		} else {
			return nil, fmt.Errorf("no outputs for bead %q", beadID)
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
				return nil, fmt.Errorf("output %q not found in bead %q", field, beadID)
			}
		default:
			return nil, fmt.Errorf("cannot access field %q on non-map value in bead %q", part, beadID)
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
			return nil, fmt.Errorf("cannot access field %q on %T", part, val)
		}
	}
	return val, nil
}

// SubstituteMap applies substitution to all string values in a map.
func (c *VarContext) SubstituteMap(m map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(m))
	for k, v := range m {
		subbed, err := c.Substitute(v)
		if err != nil {
			return nil, fmt.Errorf("substitute %q: %w", k, err)
		}
		result[k] = subbed
	}
	return result, nil
}

// ApplyDefaults fills in missing variables from template defaults.
func (c *VarContext) ApplyDefaults(vars map[string]Var) {
	for name, v := range vars {
		if _, ok := c.Variables[name]; !ok && v.Default != nil {
			c.Variables[name] = v.Default
		}
	}
}

// ValidateRequired checks that all required variables are set.
func (c *VarContext) ValidateRequired(vars map[string]Var) error {
	var missing []string
	for name, v := range vars {
		if v.Required {
			if _, ok := c.Variables[name]; !ok {
				// Check if there's a default
				if v.Default == nil {
					missing = append(missing, name)
				}
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required variables: %v", missing)
	}
	return nil
}

// SubstituteTemplate applies variable substitution to all string fields in a template step.
func (c *VarContext) SubstituteStep(step *Step) (*Step, error) {
	// Create a copy
	result := *step

	var err error

	if result.Description != "" {
		result.Description, err = c.Substitute(result.Description)
		if err != nil {
			return nil, fmt.Errorf("substitute description: %w", err)
		}
	}

	if result.Instructions != "" {
		result.Instructions, err = c.Substitute(result.Instructions)
		if err != nil {
			return nil, fmt.Errorf("substitute instructions: %w", err)
		}
	}

	if result.Condition != "" {
		result.Condition, err = c.Substitute(result.Condition)
		if err != nil {
			return nil, fmt.Errorf("substitute condition: %w", err)
		}
	}

	if result.Template != "" {
		result.Template, err = c.Substitute(result.Template)
		if err != nil {
			return nil, fmt.Errorf("substitute template: %w", err)
		}
	}

	if result.Validation != "" {
		result.Validation, err = c.Substitute(result.Validation)
		if err != nil {
			return nil, fmt.Errorf("substitute validation: %w", err)
		}
	}

	if result.Timeout != "" {
		result.Timeout, err = c.Substitute(result.Timeout)
		if err != nil {
			return nil, fmt.Errorf("substitute timeout: %w", err)
		}
	}

	if len(step.Variables) > 0 {
		result.Variables, err = c.SubstituteMap(step.Variables)
		if err != nil {
			return nil, fmt.Errorf("substitute variables: %w", err)
		}
	}

	// Handle expansion targets
	if step.OnTrue != nil {
		result.OnTrue, err = c.substituteTarget(step.OnTrue)
		if err != nil {
			return nil, fmt.Errorf("substitute on_true: %w", err)
		}
	}

	if step.OnFalse != nil {
		result.OnFalse, err = c.substituteTarget(step.OnFalse)
		if err != nil {
			return nil, fmt.Errorf("substitute on_false: %w", err)
		}
	}

	if step.OnTimeout != nil {
		result.OnTimeout, err = c.substituteTarget(step.OnTimeout)
		if err != nil {
			return nil, fmt.Errorf("substitute on_timeout: %w", err)
		}
	}

	return &result, nil
}

func (c *VarContext) substituteTarget(target *ExpansionTarget) (*ExpansionTarget, error) {
	result := *target
	var err error

	if result.Template != "" {
		result.Template, err = c.Substitute(result.Template)
		if err != nil {
			return nil, err
		}
	}

	if len(target.Variables) > 0 {
		result.Variables, err = c.SubstituteMap(target.Variables)
		if err != nil {
			return nil, err
		}
	}

	return &result, nil
}
