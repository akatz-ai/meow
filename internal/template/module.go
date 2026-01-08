// Package template provides TOML template parsing and baking for MEOW workflows.
package template

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

// FileFormat indicates the template file format.
type FileFormat int

const (
	FormatLegacy FileFormat = iota // [meta] + [[steps]]
	FormatModule                   // [workflow-name] sections
)

// Module represents a parsed module file containing one or more workflows.
type Module struct {
	Path      string               // File path for error messages
	Workflows map[string]*Workflow // Named workflows
}

// Workflow represents a single workflow within a module.
type Workflow struct {
	Name        string           `toml:"name"`
	Description string           `toml:"description,omitempty"`
	Ephemeral   bool             `toml:"ephemeral,omitempty"`   // All steps become wisps
	Internal    bool             `toml:"internal,omitempty"`    // Cannot be called from outside
	HooksTo     string           `toml:"hooks_to,omitempty"`    // Variable name for HookBead
	Variables   map[string]*Var  `toml:"variables,omitempty"`   // Reuses template.Var
	Steps       []*Step          `toml:"steps"`
}

// GetWorkflow returns the workflow with the given name, or nil if not found.
func (m *Module) GetWorkflow(name string) *Workflow {
	return m.Workflows[name]
}

// DefaultWorkflow returns the "main" workflow if it exists.
func (m *Module) DefaultWorkflow() *Workflow {
	return m.Workflows["main"]
}

// IsInternal returns true if the workflow is marked as internal.
func (w *Workflow) IsInternal() bool {
	return w.Internal
}

// DetectFormat determines if a TOML file uses legacy or module format.
// Legacy format has [meta] section, module format has workflow sections.
func DetectFormat(content string) FileFormat {
	// Simple heuristic: if it starts with [meta], it's legacy
	if strings.Contains(content, "[meta]") {
		return FormatLegacy
	}
	return FormatModule
}

// ParseModuleFile parses a module-format TOML file from the given path.
func ParseModuleFile(path string) (*Module, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read module file: %w", err)
	}

	return ParseModuleString(string(content), path)
}

// ParseModuleReader parses a module-format TOML from the given reader.
func ParseModuleReader(r io.Reader, path string) (*Module, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read module: %w", err)
	}

	return ParseModuleString(string(content), path)
}

// ParseModuleString parses a module-format TOML from a string.
func ParseModuleString(content string, path string) (*Module, error) {
	// Parse into a map first to get workflow names
	var raw map[string]any
	if _, err := toml.Decode(content, &raw); err != nil {
		return nil, fmt.Errorf("decode TOML: %w", err)
	}

	module := &Module{
		Path:      path,
		Workflows: make(map[string]*Workflow),
	}

	// Parse each top-level key as a workflow
	for name, value := range raw {
		// Skip if not a table (workflows are tables)
		workflowMap, ok := value.(map[string]any)
		if !ok {
			continue
		}

		workflow, err := parseWorkflow(name, workflowMap)
		if err != nil {
			return nil, fmt.Errorf("parse workflow %q: %w", name, err)
		}

		module.Workflows[name] = workflow
	}

	if len(module.Workflows) == 0 {
		return nil, fmt.Errorf("module has no workflows")
	}

	// Validate the module
	if err := module.Validate(); err != nil {
		return nil, fmt.Errorf("validate module: %w", err)
	}

	return module, nil
}

// parseWorkflow parses a single workflow from a map.
func parseWorkflow(name string, data map[string]any) (*Workflow, error) {
	w := &Workflow{}

	// Parse simple fields
	if v, ok := data["name"].(string); ok {
		w.Name = v
	} else {
		w.Name = name // Default to section name
	}

	if v, ok := data["description"].(string); ok {
		w.Description = v
	}
	if v, ok := data["ephemeral"].(bool); ok {
		w.Ephemeral = v
	}
	if v, ok := data["internal"].(bool); ok {
		w.Internal = v
	}
	if v, ok := data["hooks_to"].(string); ok {
		w.HooksTo = v
	}

	// Parse variables
	if vars, ok := data["variables"].(map[string]any); ok {
		w.Variables = make(map[string]*Var)
		for varName, varData := range vars {
			if varMap, ok := varData.(map[string]any); ok {
				v := &Var{}
				if req, ok := varMap["required"].(bool); ok {
					v.Required = req
				}
				if def, ok := varMap["default"]; ok {
					v.Default = def
				}
				if typ, ok := varMap["type"].(string); ok {
					v.Type = VarType(typ) // Convert string to VarType
				}
				if desc, ok := varMap["description"].(string); ok {
					v.Description = desc
				}
				w.Variables[varName] = v
			}
		}
	}

	// Parse steps - TOML decoder returns []map[string]any
	if steps, ok := data["steps"].([]map[string]any); ok {
		for i, stepMap := range steps {
			step, err := parseModuleStep(stepMap)
			if err != nil {
				return nil, fmt.Errorf("step[%d]: %w", i, err)
			}
			w.Steps = append(w.Steps, step)
		}
	} else if steps, ok := data["steps"].([]any); ok {
		// Fallback for []any (shouldn't happen but just in case)
		for i, stepData := range steps {
			stepMap, ok := stepData.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("step[%d] is not a table", i)
			}

			step, err := parseModuleStep(stepMap)
			if err != nil {
				return nil, fmt.Errorf("step[%d]: %w", i, err)
			}
			w.Steps = append(w.Steps, step)
		}
	}

	return w, nil
}

// parseModuleStep parses a step from a map into a Step struct.
func parseModuleStep(data map[string]any) (*Step, error) {
	s := &Step{}

	// Parse required fields
	if id, ok := data["id"].(string); ok {
		s.ID = id
	} else {
		return nil, fmt.Errorf("step missing id")
	}

	// Parse optional fields
	if v, ok := data["type"].(string); ok {
		s.Type = v
	}
	if v, ok := data["title"].(string); ok {
		s.Title = v
	}
	if v, ok := data["description"].(string); ok {
		s.Description = v
	}
	if v, ok := data["instructions"].(string); ok {
		s.Instructions = v
	}
	if v, ok := data["assignee"].(string); ok {
		s.Assignee = v
	}
	if v, ok := data["condition"].(string); ok {
		s.Condition = v
	}
	if v, ok := data["code"].(string); ok {
		s.Code = v
	}
	if v, ok := data["timeout"].(string); ok {
		s.Timeout = v
	}
	if v, ok := data["template"].(string); ok {
		s.Template = v
	}
	if v, ok := data["ephemeral"].(bool); ok {
		s.Ephemeral = v
	}

	// Parse needs (dependencies)
	if needs, ok := data["needs"].([]any); ok {
		for _, n := range needs {
			if ns, ok := n.(string); ok {
				s.Needs = append(s.Needs, ns)
			}
		}
	}

	// Parse variables for expand
	if vars, ok := data["variables"].(map[string]any); ok {
		s.Variables = make(map[string]string)
		for k, v := range vars {
			if vs, ok := v.(string); ok {
				s.Variables[k] = vs
			}
		}
	}

	return s, nil
}

// Validate checks that the module is well-formed.
func (m *Module) Validate() error {
	for name, w := range m.Workflows {
		if err := w.Validate(); err != nil {
			return fmt.Errorf("workflow %q: %w", name, err)
		}
	}
	return nil
}

// Validate checks that the workflow is well-formed.
func (w *Workflow) Validate() error {
	if w.Name == "" {
		return fmt.Errorf("workflow name is required")
	}

	if len(w.Steps) == 0 {
		return fmt.Errorf("workflow must have at least one step")
	}

	// Check for duplicate step IDs
	stepIDs := make(map[string]bool)
	for i, step := range w.Steps {
		if step.ID == "" {
			return fmt.Errorf("step[%d]: id is required", i)
		}
		if stepIDs[step.ID] {
			return fmt.Errorf("step[%d]: duplicate id %q", i, step.ID)
		}
		stepIDs[step.ID] = true
	}

	// Validate dependencies reference existing steps
	for i, step := range w.Steps {
		for _, need := range step.Needs {
			if !stepIDs[need] {
				return fmt.Errorf("step[%d] %q: needs references unknown step %q", i, step.ID, need)
			}
		}
	}

	// Validate step types
	validTypes := map[string]bool{
		"":              true, // Default to task
		"task":          true,
		"collaborative": true,
		"gate":          true,
		"condition":     true,
		"code":          true,
		"start":         true,
		"stop":          true,
		"expand":        true,
	}
	for i, step := range w.Steps {
		if !validTypes[step.Type] {
			return fmt.Errorf("step[%d] %q: invalid type %q", i, step.ID, step.Type)
		}
	}

	// Check for dependency cycles
	if cycle := w.findCycle(); len(cycle) > 0 {
		return fmt.Errorf("circular dependency detected: %s", strings.Join(cycle, " → "))
	}

	return nil
}

// findCycle returns the cycle path if one exists, empty slice otherwise.
func (w *Workflow) findCycle() []string {
	// Build adjacency list (step -> its dependencies)
	deps := make(map[string][]string)
	for _, step := range w.Steps {
		deps[step.ID] = step.Needs
	}

	// States: 0 = unvisited, 1 = visiting, 2 = visited
	state := make(map[string]int)
	parent := make(map[string]string)

	var cycle []string

	var dfs func(id string) bool
	dfs = func(id string) bool {
		state[id] = 1 // visiting

		for _, dep := range deps[id] {
			if state[dep] == 1 {
				// Found cycle - reconstruct path
				cycle = []string{dep}
				for cur := id; cur != dep; {
					cycle = append([]string{cur}, cycle...)
					cur = parent[cur]
				}
				cycle = append([]string{dep}, cycle...)
				return true
			}
			if state[dep] == 0 {
				parent[dep] = id
				if dfs(dep) {
					return true
				}
			}
		}

		state[id] = 2 // visited
		return false
	}

	for _, step := range w.Steps {
		if state[step.ID] == 0 {
			if dfs(step.ID) {
				return cycle
			}
		}
	}

	return nil
}
