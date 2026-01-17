// Package workflow provides TOML template parsing and baking for MEOW workflows.
package workflow

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
)

// FileFormat indicates the template file format.
type FileFormat int

const (
	FormatModule FileFormat = iota // [workflow-name] sections
)

// Module represents a parsed module file containing one or more workflows.
type Module struct {
	Path      string               // File path for error messages
	Workflows map[string]*Workflow // Named workflows
}

// Workflow represents a single workflow within a module.
type Workflow struct {
	Name        string          `toml:"name"`
	Description string          `toml:"description,omitempty"`
	Internal    bool            `toml:"internal,omitempty"` // Cannot be called from outside
	Variables   map[string]*Var `toml:"variables,omitempty"`
	Steps       []*Step         `toml:"steps"`

	// Conditional cleanup scripts - all opt-in, no cleanup by default
	CleanupOnSuccess string `toml:"cleanup_on_success,omitempty"` // Runs when all steps complete successfully
	CleanupOnFailure string `toml:"cleanup_on_failure,omitempty"` // Runs when a step fails
	CleanupOnStop    string `toml:"cleanup_on_stop,omitempty"`    // Runs on SIGINT/SIGTERM or meow stop
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

		// Strip leading dot from workflow names (the dot prefix is for local
		// reference syntax in templates, not part of the actual workflow name)
		workflowName := name
		if strings.HasPrefix(workflowName, ".") {
			workflowName = workflowName[1:]
		}

		workflow, err := parseWorkflow(workflowName, workflowMap)
		if err != nil {
			return nil, fmt.Errorf("parse workflow %q: %w", name, err)
		}

		module.Workflows[workflowName] = workflow
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
	if v, ok := data["internal"].(bool); ok {
		w.Internal = v
	}

	// Parse conditional cleanup scripts (opt-in)
	if v, ok := data["cleanup_on_success"].(string); ok {
		w.CleanupOnSuccess = v
	}
	if v, ok := data["cleanup_on_failure"].(string); ok {
		w.CleanupOnFailure = v
	}
	if v, ok := data["cleanup_on_stop"].(string); ok {
		w.CleanupOnStop = v
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

	// Parse executor field (new format)
	if v, ok := data["executor"].(string); ok {
		s.Executor = ExecutorType(v)
	}

	// Parse shared fields
	if v, ok := data["timeout"].(string); ok {
		s.Timeout = v
	}

	// Parse needs (dependencies)
	if needs, ok := data["needs"].([]any); ok {
		for _, n := range needs {
			if ns, ok := n.(string); ok {
				s.Needs = append(s.Needs, ns)
			}
		}
	}

	// Parse agent executor fields
	if v, ok := data["agent"].(string); ok {
		s.Agent = v
	}
	if v, ok := data["prompt"].(string); ok {
		s.Prompt = v
	}
	if v, ok := data["mode"].(string); ok {
		s.Mode = v
	}

	// Parse shell executor fields
	if v, ok := data["command"].(string); ok {
		s.Command = v
	}
	if v, ok := data["workdir"].(string); ok {
		s.Workdir = v
	}
	if v, ok := data["on_error"].(string); ok {
		s.OnError = v
	}

	// Parse env (used by shell and spawn)
	if env, ok := data["env"].(map[string]any); ok {
		s.Env = make(map[string]string)
		for k, v := range env {
			if vs, ok := v.(string); ok {
				s.Env[k] = vs
			}
		}
	}

	// Parse shell_outputs for shell executor (also check "outputs" for shell steps)
	shellOutputsData := data["shell_outputs"]
	if shellOutputsData == nil && s.Executor == ExecutorShell {
		// For shell executor, also accept "outputs" as key
		shellOutputsData = data["outputs"]
	}
	if outputs, ok := shellOutputsData.(map[string]any); ok {
		s.ShellOutputs = make(map[string]OutputSource)
		for k, v := range outputs {
			if vm, ok := v.(map[string]any); ok {
				os := OutputSource{}
				if src, ok := vm["source"].(string); ok {
					os.Source = src
				}
				if typ, ok := vm["type"].(string); ok {
					os.Type = typ
				}
				s.ShellOutputs[k] = os
			}
		}
	}

	// Parse spawn executor fields
	if v, ok := data["adapter"].(string); ok {
		s.Adapter = v
	}
	if v, ok := data["resume_session"].(string); ok {
		s.ResumeSession = v
	}
	if v, ok := data["spawn_args"].(string); ok {
		s.SpawnArgs = v
	}

	// Parse kill executor fields
	if v, ok := data["graceful"].(bool); ok {
		s.Graceful = &v
	}

	// Parse expand executor fields
	if v, ok := data["template"].(string); ok {
		s.Template = v
	}
	if vars, ok := data["variables"].(map[string]any); ok {
		s.Variables = make(map[string]any)
		for k, v := range vars {
			s.Variables[k] = v // Preserve typed values
		}
	}

	// Parse branch executor fields
	if v, ok := data["condition"].(string); ok {
		s.Condition = v
	}
	if v, ok := data["on_true"].(map[string]any); ok {
		target, err := parseExpansionTarget(v)
		if err != nil {
			return nil, fmt.Errorf("on_true: %w", err)
		}
		s.OnTrue = target
	}
	if v, ok := data["on_false"].(map[string]any); ok {
		target, err := parseExpansionTarget(v)
		if err != nil {
			return nil, fmt.Errorf("on_false: %w", err)
		}
		s.OnFalse = target
	}
	if v, ok := data["on_timeout"].(map[string]any); ok {
		target, err := parseExpansionTarget(v)
		if err != nil {
			return nil, fmt.Errorf("on_timeout: %w", err)
		}
		s.OnTimeout = target
	}

	// Parse foreach executor fields
	if v, ok := data["items"].(string); ok {
		s.Items = v
	}
	if v, ok := data["items_file"].(string); ok {
		s.ItemsFile = v
	}
	if v, ok := data["item_var"].(string); ok {
		s.ItemVar = v
	}
	if v, ok := data["index_var"].(string); ok {
		s.IndexVar = v
	}
	// Handle parallel as bool (direct) or string (for variable substitution)
	if v, ok := data["parallel"].(bool); ok {
		s.Parallel = v
	} else if v, ok := data["parallel"].(string); ok {
		s.Parallel = v
	}
	// Handle max_concurrent as string (new) or int64 (backwards compat)
	if v, ok := data["max_concurrent"].(string); ok {
		s.MaxConcurrent = v
	} else if v, ok := data["max_concurrent"].(int64); ok {
		s.MaxConcurrent = fmt.Sprintf("%d", v)
	}
	if v, ok := data["join"].(bool); ok {
		s.Join = &v
	}

	// Parse agent output definitions
	if outputs, ok := data["outputs"].(map[string]any); ok {
		s.Outputs = make(map[string]AgentOutputDef)
		for name, def := range outputs {
			if defMap, ok := def.(map[string]any); ok {
				outDef := AgentOutputDef{}
				if req, ok := defMap["required"].(bool); ok {
					outDef.Required = req
				}
				if typ, ok := defMap["type"].(string); ok {
					outDef.Type = typ
				}
				if desc, ok := defMap["description"].(string); ok {
					outDef.Description = desc
				}
				s.Outputs[name] = outDef
			}
		}
	}

	return s, nil
}

// parseExpansionTarget parses an expansion target from a map.
func parseExpansionTarget(data map[string]any) (*ExpansionTarget, error) {
	target := &ExpansionTarget{}

	if v, ok := data["template"].(string); ok {
		target.Template = v
	}

	// Parse variables
	if vars, ok := data["variables"].(map[string]any); ok {
		target.Variables = make(map[string]any)
		for k, v := range vars {
			target.Variables[k] = v // Preserve typed values
		}
	}

	// Parse inline steps
	if inline, ok := data["inline"].([]any); ok {
		for i, stepData := range inline {
			stepMap, ok := stepData.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("inline[%d] is not a table", i)
			}
			inlineStep, err := parseInlineStep(stepMap)
			if err != nil {
				return nil, fmt.Errorf("inline[%d]: %w", i, err)
			}
			target.Inline = append(target.Inline, *inlineStep)
		}
	} else if inline, ok := data["inline"].([]map[string]any); ok {
		for i, stepMap := range inline {
			inlineStep, err := parseInlineStep(stepMap)
			if err != nil {
				return nil, fmt.Errorf("inline[%d]: %w", i, err)
			}
			target.Inline = append(target.Inline, *inlineStep)
		}
	}

	return target, nil
}

// parseInlineStep parses an inline step from a map.
func parseInlineStep(data map[string]any) (*InlineStep, error) {
	step := &InlineStep{}

	// Parse required fields
	if id, ok := data["id"].(string); ok {
		step.ID = id
	} else {
		return nil, fmt.Errorf("inline step missing id")
	}

	// Parse executor field (new format)
	if v, ok := data["executor"].(string); ok {
		step.Executor = ExecutorType(v)
	}

	// Parse shared fields
	if v, ok := data["timeout"].(string); ok {
		step.Timeout = v
	}

	// Parse needs (dependencies)
	if needs, ok := data["needs"].([]any); ok {
		for _, n := range needs {
			if ns, ok := n.(string); ok {
				step.Needs = append(step.Needs, ns)
			}
		}
	}

	// Parse agent executor fields
	if v, ok := data["agent"].(string); ok {
		step.Agent = v
	}
	if v, ok := data["prompt"].(string); ok {
		step.Prompt = v
	}
	if v, ok := data["mode"].(string); ok {
		step.Mode = v
	}

	// Parse shell executor fields
	if v, ok := data["command"].(string); ok {
		step.Command = v
	}
	if v, ok := data["workdir"].(string); ok {
		step.Workdir = v
	}
	if v, ok := data["on_error"].(string); ok {
		step.OnError = v
	}

	// Parse env
	if env, ok := data["env"].(map[string]any); ok {
		step.Env = make(map[string]string)
		for k, v := range env {
			if vs, ok := v.(string); ok {
				step.Env[k] = vs
			}
		}
	}

	// Parse spawn executor fields
	if v, ok := data["adapter"].(string); ok {
		step.Adapter = v
	}
	if v, ok := data["resume_session"].(string); ok {
		step.ResumeSession = v
	}
	if v, ok := data["spawn_args"].(string); ok {
		step.SpawnArgs = v
	}

	// Parse kill executor fields
	if v, ok := data["graceful"].(bool); ok {
		step.Graceful = &v
	}

	// Parse expand executor fields
	if v, ok := data["template"].(string); ok {
		step.Template = v
	}
	if vars, ok := data["variables"].(map[string]any); ok {
		step.Variables = make(map[string]any)
		for k, v := range vars {
			step.Variables[k] = v // Preserve typed values
		}
	}

	// Parse branch executor fields
	if v, ok := data["condition"].(string); ok {
		step.Condition = v
	}
	if v, ok := data["on_true"].(map[string]any); ok {
		target, err := parseExpansionTarget(v)
		if err != nil {
			return nil, fmt.Errorf("on_true: %w", err)
		}
		step.OnTrue = target
	}
	if v, ok := data["on_false"].(map[string]any); ok {
		target, err := parseExpansionTarget(v)
		if err != nil {
			return nil, fmt.Errorf("on_false: %w", err)
		}
		step.OnFalse = target
	}
	if v, ok := data["on_timeout"].(map[string]any); ok {
		target, err := parseExpansionTarget(v)
		if err != nil {
			return nil, fmt.Errorf("on_timeout: %w", err)
		}
		step.OnTimeout = target
	}

	// Parse foreach executor fields
	if v, ok := data["items"].(string); ok {
		step.Items = v
	}
	if v, ok := data["items_file"].(string); ok {
		step.ItemsFile = v
	}
	if v, ok := data["item_var"].(string); ok {
		step.ItemVar = v
	}
	if v, ok := data["index_var"].(string); ok {
		step.IndexVar = v
	}
	// Handle parallel as bool (direct) or string (for variable substitution)
	if v, ok := data["parallel"].(bool); ok {
		step.Parallel = v
	} else if v, ok := data["parallel"].(string); ok {
		step.Parallel = v
	}
	// Handle max_concurrent as string (new) or int64 (backwards compat)
	if v, ok := data["max_concurrent"].(string); ok {
		step.MaxConcurrent = v
	} else if v, ok := data["max_concurrent"].(int64); ok {
		step.MaxConcurrent = fmt.Sprintf("%d", v)
	}
	if v, ok := data["join"].(bool); ok {
		step.Join = &v
	}

	// Parse agent output definitions
	if outputs, ok := data["outputs"].(map[string]any); ok {
		step.Outputs = make(map[string]AgentOutputDef)
		for name, def := range outputs {
			if defMap, ok := def.(map[string]any); ok {
				outDef := AgentOutputDef{}
				if req, ok := defMap["required"].(bool); ok {
					outDef.Required = req
				}
				if typ, ok := defMap["type"].(string); ok {
					outDef.Type = typ
				}
				if desc, ok := defMap["description"].(string); ok {
					outDef.Description = desc
				}
				step.Outputs[name] = outDef
			}
		}
	}

	return step, nil
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

	// Check for duplicate step IDs and track expand steps
	stepIDs := make(map[string]bool)
	expandSteps := make(map[string]bool)
	for i, step := range w.Steps {
		if step.ID == "" {
			return fmt.Errorf("step[%d]: id is required", i)
		}
		if stepIDs[step.ID] {
			return fmt.Errorf("step[%d]: duplicate id %q", i, step.ID)
		}
		stepIDs[step.ID] = true
		if step.Executor == ExecutorExpand {
			expandSteps[step.ID] = true
		}
	}

	// Validate dependencies reference existing steps
	// Allow references to children of expand steps (e.g., "expand-step.child")
	for i, step := range w.Steps {
		for _, need := range step.Needs {
			if !stepIDs[need] {
				// Check if this references a child of an expand step
				if dotIdx := strings.Index(need, "."); dotIdx > 0 {
					prefix := need[:dotIdx]
					if expandSteps[prefix] {
						// This references a child of an expand step - allowed
						continue
					}
				}
				return fmt.Errorf("step[%d] %q: needs references unknown step %q", i, step.ID, need)
			}
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

// ModuleValidationResult holds all validation errors for a module.
type ModuleValidationResult struct {
	Errors []ValidationError
}

// HasErrors returns true if there are any validation errors.
func (r *ModuleValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// Error implements the error interface.
func (r *ModuleValidationResult) Error() string {
	if len(r.Errors) == 0 {
		return ""
	}
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return fmt.Sprintf("module validation failed with %d error(s):\n  - %s",
		len(r.Errors), strings.Join(msgs, "\n  - "))
}

// Add adds a validation error.
func (r *ModuleValidationResult) Add(workflow, stepID, field, message, suggest string) {
	r.Errors = append(r.Errors, ValidationError{
		Template: workflow, // Reusing Template field for workflow name
		StepID:   stepID,
		Field:    field,
		Message:  message,
		Suggest:  suggest,
	})
}

// ValidateFullModule performs comprehensive validation on a module.
// Returns all errors found, not just the first one.
func ValidateFullModule(m *Module) *ModuleValidationResult {
	result := &ModuleValidationResult{}

	// Validate each workflow
	for name, w := range m.Workflows {
		validateModuleWorkflow(m, name, w, result)
	}

	// Validate cross-workflow references
	validateLocalReferences(m, result)

	return result
}

// validateModuleWorkflow validates a single workflow in the module context.
func validateModuleWorkflow(m *Module, name string, w *Workflow, result *ModuleValidationResult) {
	if w.Name == "" {
		result.Add(name, "", "name", "workflow name is required", "add name = \"workflow-name\"")
	}

	if len(w.Steps) == 0 {
		result.Add(name, "", "steps", "workflow must have at least one step", "add [[steps]] section")
		return
	}

	// Check for duplicate step IDs and track expand steps
	stepIDs := make(map[string]int)
	expandSteps := make(map[string]bool)
	for i, step := range w.Steps {
		if step.ID == "" {
			result.Add(name, fmt.Sprintf("steps[%d]", i), "id", "step id is required", "")
			continue
		}

		if prevIdx, exists := stepIDs[step.ID]; exists {
			result.Add(name, step.ID, "id",
				fmt.Sprintf("duplicate step id (first at index %d)", prevIdx),
				"use unique step ids")
		}
		stepIDs[step.ID] = i

		// Track expand steps for dependency validation
		if step.Executor == ExecutorExpand {
			expandSteps[step.ID] = true
		}
	}

	// Validate dependencies
	// Allow references to children of expand steps (e.g., "expand-step.done")
	for _, step := range w.Steps {
		for _, need := range step.Needs {
			if _, exists := stepIDs[need]; !exists {
				// Check if this references a child of an expand step
				if dotIdx := strings.Index(need, "."); dotIdx > 0 {
					prefix := need[:dotIdx]
					if expandSteps[prefix] {
						// This references a child of an expand step - allowed
						continue
					}
				}
				suggest := findSimilarInMap(need, stepIDs)
				result.Add(name, step.ID, "needs",
					fmt.Sprintf("references unknown step %q", need),
					suggest)
			}
		}
	}

	// Check for cycles
	if cycle := w.findCycle(); len(cycle) > 0 {
		result.Add(name, "", "needs",
			fmt.Sprintf("circular dependency detected: %s", strings.Join(cycle, " → ")),
			"remove one of the dependencies to break the cycle")
	}

	// Validate variable references
	validateModuleVariableReferences(m, name, w, result)
}

// validateLocalReferences checks that all local template references (.workflow syntax)
// exist in the module and respects internal visibility.
func validateLocalReferences(m *Module, result *ModuleValidationResult) {
	for workflowName, w := range m.Workflows {
		for _, step := range w.Steps {
			// Check template field
			checkLocalRef(m, workflowName, step.ID, "template", step.Template, result)

			// Check expansion targets
			if step.OnTrue != nil {
				checkLocalRef(m, workflowName, step.ID, "on_true.template", step.OnTrue.Template, result)
			}
			if step.OnFalse != nil {
				checkLocalRef(m, workflowName, step.ID, "on_false.template", step.OnFalse.Template, result)
			}
			if step.OnTimeout != nil {
				checkLocalRef(m, workflowName, step.ID, "on_timeout.template", step.OnTimeout.Template, result)
			}
		}
	}
}

// checkLocalRef validates a single template reference.
func checkLocalRef(m *Module, workflowName, stepID, field, ref string, result *ModuleValidationResult) {
	if ref == "" {
		return
	}

	// Skip if it contains variable references (validated at runtime)
	if strings.Contains(ref, "{{") {
		return
	}

	// Check if it's a local reference (starts with .)
	if !strings.HasPrefix(ref, ".") {
		return // External reference - validated elsewhere
	}

	// Parse local reference: .workflow or .workflow.step
	localRef := strings.TrimPrefix(ref, ".")
	parts := strings.SplitN(localRef, ".", 2)
	targetWorkflow := parts[0]

	// Check workflow exists
	target, exists := m.Workflows[targetWorkflow]
	if !exists {
		// Build suggestion
		suggest := findSimilarWorkflow(targetWorkflow, m.Workflows)
		result.Add(workflowName, stepID, field,
			fmt.Sprintf("references unknown workflow %q", targetWorkflow),
			suggest)
		return
	}

	// Note: We do NOT check internal visibility here because local references
	// (.workflow syntax) are by definition within the same module file.
	// The "internal" flag only prevents external references (file#workflow)
	// from other files, which is validated at runtime by the loader.

	// If referencing a specific step, validate it exists
	if len(parts) > 1 {
		stepRef := parts[1]
		stepExists := false
		for _, s := range target.Steps {
			if s.ID == stepRef {
				stepExists = true
				break
			}
		}
		if !stepExists {
			result.Add(workflowName, stepID, field,
				fmt.Sprintf("references unknown step %q in workflow %q", stepRef, targetWorkflow),
				"")
		}
	}
}

// validateModuleVariableReferences checks that all variable references in a workflow are defined.
func validateModuleVariableReferences(m *Module, workflowName string, w *Workflow, result *ModuleValidationResult) {
	// Collect all defined variables
	defined := make(map[string]bool)
	for varName := range w.Variables {
		defined[varName] = true
	}

	// Add builtins
	builtins := []string{
		"timestamp", "date", "time", "agent", "bead_id", "molecule_id",
		"workflow_id", "step_id",
	}
	for _, b := range builtins {
		defined[b] = true
	}

	// Check all string fields in steps for variable references
	for _, step := range w.Steps {
		checkModuleVarRefs(step.Command, workflowName, step.ID, "command", defined, result)
		checkModuleVarRefs(step.Prompt, workflowName, step.ID, "prompt", defined, result)
		checkModuleVarRefs(step.Condition, workflowName, step.ID, "condition", defined, result)

		for k, v := range step.Variables {
			// Only check string values for variable references (typed values are preserved as-is)
			if vs, ok := v.(string); ok {
				checkModuleVarRefs(vs, workflowName, step.ID, fmt.Sprintf("variables.%s", k), defined, result)
			}
		}

		if step.OnTrue != nil {
			checkModuleExpansionVarRefs(step.OnTrue, workflowName, step.ID, "on_true", defined, result)
		}
		if step.OnFalse != nil {
			checkModuleExpansionVarRefs(step.OnFalse, workflowName, step.ID, "on_false", defined, result)
		}
		if step.OnTimeout != nil {
			checkModuleExpansionVarRefs(step.OnTimeout, workflowName, step.ID, "on_timeout", defined, result)
		}
	}
}

// checkModuleExpansionVarRefs checks variable references in an expansion target.
func checkModuleExpansionVarRefs(target *ExpansionTarget, workflowName, stepID, field string, defined map[string]bool, result *ModuleValidationResult) {
	for k, v := range target.Variables {
		// Only check string values for variable references (typed values are preserved as-is)
		if vs, ok := v.(string); ok {
			checkModuleVarRefs(vs, workflowName, stepID, fmt.Sprintf("%s.variables.%s", field, k), defined, result)
		}
	}
}

// varRefPatternModule matches {{variable}} patterns
var varRefPatternModule = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

// checkModuleVarRefs checks variable references in a string.
func checkModuleVarRefs(text, workflowName, stepID, field string, defined map[string]bool, result *ModuleValidationResult) {
	if text == "" {
		return
	}

	matches := varRefPatternModule.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		path := strings.TrimSpace(match[1])
		parts := strings.Split(path, ".")
		root := parts[0]

		// Skip output references - they're validated at runtime
		// Check for "output" prefix (legacy format) or "outputs" anywhere in path
		// (step IDs can contain dots from expansion prefixes, e.g., "expand-step.child.outputs.field")
		if root == "output" {
			continue
		}
		isOutputRef := false
		for _, part := range parts {
			if part == "outputs" {
				isOutputRef = true
				break
			}
		}
		if isOutputRef {
			continue
		}

		if !defined[root] {
			suggest := findSimilarInBoolMap(root, defined)
			result.Add(workflowName, stepID, field,
				fmt.Sprintf("undefined variable %q", root),
				suggest)
		}
	}
}

// findSimilarInMap finds a similar key for "did you mean" suggestions.
func findSimilarInMap(target string, candidates map[string]int) string {
	var best string
	bestScore := 0

	for candidate := range candidates {
		score := moduleSimilarity(target, candidate)
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}

	if bestScore > len(target)/2 {
		return fmt.Sprintf("did you mean %q?", best)
	}
	return ""
}

// findSimilarInBoolMap finds a similar key for "did you mean" suggestions.
func findSimilarInBoolMap(target string, candidates map[string]bool) string {
	var best string
	bestScore := 0

	for candidate := range candidates {
		score := moduleSimilarity(target, candidate)
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}

	if bestScore > len(target)/2 {
		return fmt.Sprintf("did you mean %q?", best)
	}
	return ""
}

// findSimilarWorkflow finds a similar workflow name for suggestions.
func findSimilarWorkflow(target string, workflows map[string]*Workflow) string {
	var best string
	bestScore := 0

	for name := range workflows {
		score := moduleSimilarity(target, name)
		if score > bestScore {
			bestScore = score
			best = name
		}
	}

	if bestScore > len(target)/2 {
		return fmt.Sprintf("did you mean %q?", best)
	}
	return ""
}

// moduleSimilarity returns a simple score based on common prefix and suffix.
func moduleSimilarity(a, b string) int {
	score := 0

	// Common prefix
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] == b[i] {
			score++
		} else {
			break
		}
	}

	// Common suffix
	for i := 0; i < minLen-score; i++ {
		if a[len(a)-1-i] == b[len(b)-1-i] {
			score++
		} else {
			break
		}
	}

	return score
}
