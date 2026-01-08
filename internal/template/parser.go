// Package template provides TOML template parsing and baking for MEOW workflows.
package template

import (
	"fmt"
	"io"
	"os"

	"github.com/BurntSushi/toml"
)

// Template represents a parsed MEOW template.
type Template struct {
	Meta      Meta            `toml:"meta"`
	Variables map[string]Var  `toml:"variables"`
	Steps     []Step          `toml:"steps"`
}

// Meta contains template metadata.
type Meta struct {
	Name              string `toml:"name"`
	Version           string `toml:"version"`
	Description       string `toml:"description"`
	Author            string `toml:"author"`
	Type              string `toml:"type,omitempty"`              // e.g., "loop"
	FitsInContext     bool   `toml:"fits_in_context,omitempty"`
	RequiresHuman     bool   `toml:"requires_human,omitempty"`
	EstimatedMinutes  int    `toml:"estimated_minutes,omitempty"`
	MaxIterations     int    `toml:"max_iterations,omitempty"`
	OnError           string `toml:"on_error,omitempty"`          // inject-gate, abort, continue, retry
	ErrorGateTemplate string `toml:"error_gate_template,omitempty"`
	MaxRetries        int    `toml:"max_retries,omitempty"`
}

// VarType represents the type of a template variable.
type VarType string

const (
	VarTypeString VarType = "string"
	VarTypeInt    VarType = "int"
	VarTypeBool   VarType = "bool"
)

// Var defines a template variable.
type Var struct {
	Required    bool     `toml:"required"`
	Default     any      `toml:"default,omitempty"`
	Type        VarType  `toml:"type,omitempty"`        // string (default), int, bool
	Description string   `toml:"description,omitempty"`
	Enum        []string `toml:"enum,omitempty"`        // Allowed values
}

// Step represents a single step in a template.
type Step struct {
	ID           string            `toml:"id"`
	Title        string            `toml:"title,omitempty"`        // Human-readable title (module format)
	Description  string            `toml:"description,omitempty"`
	Type         string            `toml:"type,omitempty"`         // task, collaborative, gate, condition, etc.
	Assignee     string            `toml:"assignee,omitempty"`     // Agent ID for task/collaborative/start/stop
	Needs        []string          `toml:"needs,omitempty"`        // Dependencies
	Condition    string            `toml:"condition,omitempty"`    // Shell condition for condition beads
	Code         string            `toml:"code,omitempty"`         // Shell code for code beads
	Instructions string            `toml:"instructions,omitempty"` // For task beads
	Action       string            `toml:"action,omitempty"`       // notify, etc.
	Validation   string            `toml:"validation,omitempty"`   // Post-step validation
	Template     string            `toml:"template,omitempty"`     // Child template reference
	Variables    map[string]string `toml:"variables,omitempty"`    // Variables for child template
	Ephemeral    bool              `toml:"ephemeral,omitempty"`    // Auto-cleanup after workflow
	OnTrue       *ExpansionTarget  `toml:"on_true,omitempty"`      // Condition branch
	OnFalse      *ExpansionTarget  `toml:"on_false,omitempty"`     // Condition branch
	OnTimeout    *ExpansionTarget  `toml:"on_timeout,omitempty"`   // Condition timeout branch
	Timeout      string            `toml:"timeout,omitempty"`      // Timeout duration
	Outputs      *TaskOutputSpec   `toml:"outputs,omitempty"`      // Task output specifications
}

// TaskOutputSpec defines the expected outputs from a task step.
type TaskOutputSpec struct {
	Required []TaskOutputDef `toml:"required,omitempty"`
	Optional []TaskOutputDef `toml:"optional,omitempty"`
}

// TaskOutputDef defines a required or optional output from a task step.
type TaskOutputDef struct {
	Name        string `toml:"name"`
	Type        string `toml:"type"`
	Description string `toml:"description,omitempty"`
}

// ExpansionTarget specifies what to expand for condition branches.
type ExpansionTarget struct {
	Template  string            `toml:"template,omitempty"`
	Inline    []InlineStep      `toml:"inline,omitempty"`
	Variables map[string]string `toml:"variables,omitempty"`
}

// InlineStep represents an inline step definition within an expansion target.
type InlineStep struct {
	ID           string   `toml:"id"`
	Type         string   `toml:"type"`
	Description  string   `toml:"description,omitempty"`
	Instructions string   `toml:"instructions,omitempty"`
	Assignee     string   `toml:"assignee,omitempty"`
	Needs        []string `toml:"needs,omitempty"`
}

// ParseFile parses a TOML template file from the given path.
func ParseFile(path string) (*Template, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open template file: %w", err)
	}
	defer f.Close()

	return Parse(f)
}

// Parse parses a TOML template from the given reader.
func Parse(r io.Reader) (*Template, error) {
	var t Template
	meta, err := toml.NewDecoder(r).Decode(&t)
	if err != nil {
		return nil, fmt.Errorf("decode TOML: %w", err)
	}

	// Check for unknown keys (helps catch typos)
	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		// Log a warning but don't fail - allows forward compatibility
		// In strict mode, we could return an error here
		_ = undecoded
	}

	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("validate template: %w", err)
	}

	return &t, nil
}

// ParseString parses a TOML template from a string.
func ParseString(s string) (*Template, error) {
	var t Template
	if _, err := toml.Decode(s, &t); err != nil {
		return nil, fmt.Errorf("decode TOML: %w", err)
	}

	if err := t.Validate(); err != nil {
		return nil, fmt.Errorf("validate template: %w", err)
	}

	return &t, nil
}

// Validate checks that the template is well-formed.
func (t *Template) Validate() error {
	// Meta validation
	if t.Meta.Name == "" {
		return fmt.Errorf("template meta.name is required")
	}

	// Steps validation
	if len(t.Steps) == 0 {
		return fmt.Errorf("template must have at least one step")
	}

	stepIDs := make(map[string]bool)
	for i, step := range t.Steps {
		if step.ID == "" {
			return fmt.Errorf("step[%d]: id is required", i)
		}
		if stepIDs[step.ID] {
			return fmt.Errorf("step[%d]: duplicate id %q", i, step.ID)
		}
		stepIDs[step.ID] = true
	}

	// Validate dependencies reference existing steps
	for i, step := range t.Steps {
		for _, need := range step.Needs {
			if !stepIDs[need] {
				return fmt.Errorf("step[%d] %q: needs references unknown step %q", i, step.ID, need)
			}
		}
	}

	// Validate variables
	for name, v := range t.Variables {
		if v.Type != "" && v.Type != VarTypeString && v.Type != VarTypeInt && v.Type != VarTypeBool {
			return fmt.Errorf("variable %q: invalid type %q (must be string, int, or bool)", name, v.Type)
		}
	}

	return nil
}

// GetStep returns the step with the given ID, or nil if not found.
func (t *Template) GetStep(id string) *Step {
	for i := range t.Steps {
		if t.Steps[i].ID == id {
			return &t.Steps[i]
		}
	}
	return nil
}

// GetRequiredVariables returns all variables that are required (no default).
func (t *Template) GetRequiredVariables() map[string]Var {
	required := make(map[string]Var)
	for name, v := range t.Variables {
		if v.Required && v.Default == nil {
			required[name] = v
		}
	}
	return required
}

// StepOrder returns a topologically sorted list of step IDs.
// Returns an error if there are cycles.
func (t *Template) StepOrder() ([]string, error) {
	// Kahn's algorithm for topological sort
	// In-degree = number of dependencies each step has
	inDegree := make(map[string]int)
	for _, step := range t.Steps {
		inDegree[step.ID] = len(step.Needs)
	}

	// Find all nodes with no dependencies
	var queue []string
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	// Build reverse adjacency (who depends on me)
	dependents := make(map[string][]string)
	for _, step := range t.Steps {
		for _, need := range step.Needs {
			dependents[need] = append(dependents[need], step.ID)
		}
	}

	var order []string
	for len(queue) > 0 {
		// Pop from queue
		id := queue[0]
		queue = queue[1:]
		order = append(order, id)

		// Reduce in-degree of dependents
		for _, dep := range dependents[id] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != len(t.Steps) {
		return nil, fmt.Errorf("cycle detected in step dependencies")
	}

	return order, nil
}
