// Package template provides TOML template parsing and baking for MEOW workflows.
package template

import (
	"fmt"
	"io"
	"os"
	"strings"

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
	VarTypeFile   VarType = "file" // Value is a file path; contents are read and used
)

// Var defines a template variable.
type Var struct {
	Required    bool     `toml:"required"`
	Default     any      `toml:"default,omitempty"`
	Type        VarType  `toml:"type,omitempty"`        // string (default), int, bool, file
	Description string   `toml:"description,omitempty"`
	Enum        []string `toml:"enum,omitempty"`        // Allowed values
}

// ExecutorType represents the type of executor for a step.
type ExecutorType string

const (
	ExecutorShell   ExecutorType = "shell"
	ExecutorSpawn   ExecutorType = "spawn"
	ExecutorKill    ExecutorType = "kill"
	ExecutorExpand  ExecutorType = "expand"
	ExecutorBranch  ExecutorType = "branch"
	ExecutorForeach ExecutorType = "foreach"
	ExecutorAgent   ExecutorType = "agent"
	// Note: Gate is NOT an executor. Human approval is implemented via
	// branch executor with condition = "meow await-approval <gate-id>"
)

// Valid returns true if the executor type is valid.
// Note: Empty executor is allowed during template migration period.
func (e ExecutorType) Valid() bool {
	switch e {
	case ExecutorShell, ExecutorSpawn, ExecutorKill, ExecutorExpand, ExecutorBranch, ExecutorForeach, ExecutorAgent:
		return true
	case "": // Allow empty during migration - templates use type field
		return true
	}
	return false
}

// IsOrchestrator returns true if the executor runs internally (not waiting for external completion).
func (e ExecutorType) IsOrchestrator() bool {
	switch e {
	case ExecutorShell, ExecutorSpawn, ExecutorKill, ExecutorExpand, ExecutorBranch, ExecutorForeach:
		return true
	}
	return false
}

// OutputSource defines where to capture output from for shell executor.
type OutputSource struct {
	Source string `toml:"source"` // stdout | stderr | exit_code | file:/path
}

// AgentOutputDef defines an expected output from an agent step.
type AgentOutputDef struct {
	Required    bool   `toml:"required"`
	Type        string `toml:"type"` // string | number | boolean | json | file_path
	Description string `toml:"description,omitempty"`
}

// Step represents a single step in a template.
type Step struct {
	ID       string       `toml:"id"`
	Executor ExecutorType `toml:"executor,omitempty"` // shell | spawn | kill | expand | branch | foreach | agent

	// Shared fields
	Needs   []string          `toml:"needs,omitempty"` // Step IDs that must complete first
	Timeout string            `toml:"timeout,omitempty"`

	// Agent executor fields
	Agent  string                    `toml:"agent,omitempty"`  // Agent identifier (also used by spawn, kill)
	Prompt string                    `toml:"prompt,omitempty"` // Instructions for agent (also used by gate)
	Mode   string                    `toml:"mode,omitempty"`   // autonomous | interactive

	// Shell executor fields
	Command string                   `toml:"command,omitempty"` // Shell command to execute
	Workdir string                   `toml:"workdir,omitempty"` // Working directory (also used by spawn)
	Env     map[string]string        `toml:"env,omitempty"`     // Environment variables (also used by spawn)
	OnError string                   `toml:"on_error,omitempty"` // continue | fail (default: fail)

	// Shell output capture
	ShellOutputs map[string]OutputSource `toml:"shell_outputs,omitempty"` // For shell executor stdout/stderr/file capture

	// Spawn executor fields (uses Agent, Workdir, Env)
	Adapter       string `toml:"adapter,omitempty"`        // Which adapter to use (defaults to config hierarchy)
	ResumeSession string `toml:"resume_session,omitempty"` // Claude session ID to resume
	SpawnArgs     string `toml:"spawn_args,omitempty"`     // Extra CLI args to append to spawn command

	// Kill executor fields (uses Agent)
	Graceful *bool `toml:"graceful,omitempty"` // Send SIGTERM first (default: true)
	// Timeout already defined above

	// Expand executor fields
	Template  string            `toml:"template,omitempty"`  // Template reference
	Variables map[string]string `toml:"variables,omitempty"` // Variables for template

	// Branch executor fields
	Condition string           `toml:"condition,omitempty"`   // Shell command (exit 0 = true)
	OnTrue    *ExpansionTarget `toml:"on_true,omitempty"`     // Expand if condition true
	OnFalse   *ExpansionTarget `toml:"on_false,omitempty"`    // Expand if condition false
	OnTimeout *ExpansionTarget `toml:"on_timeout,omitempty"`  // Expand if condition times out

	// Foreach executor fields
	Items         string `toml:"items,omitempty"`          // JSON array expression (may contain variable refs)
	ItemsFile     string `toml:"items_file,omitempty"`     // Path to JSON file (alternative to items)
	ItemVar       string `toml:"item_var,omitempty"`       // Variable name for current item
	IndexVar      string `toml:"index_var,omitempty"`      // Variable name for index (optional)
	Parallel      any    `toml:"parallel,omitempty"`       // Run iterations in parallel (bool or string for variables, default true)
	MaxConcurrent any    `toml:"max_concurrent,omitempty"` // Limit concurrent executions (int or string for variables)
	Join          *bool  `toml:"join,omitempty"`           // Wait for all iterations (default true)
	// Template and Variables fields already defined above for expand executor

	// Agent output definitions (for agent executor)
	Outputs map[string]AgentOutputDef `toml:"outputs,omitempty"`

	// === Legacy fields - MIGRATION REQUIRED ===
	// TODO: Migrate existing templates to use executor/prompt/command fields,
	// then remove these fields. See .meow/templates/*.toml for templates to migrate.

	Type         string          `toml:"type,omitempty"`         // DEPRECATED: use executor
	Title        string          `toml:"title,omitempty"`        // Human-readable title
	Description  string          `toml:"description,omitempty"`  // Step description
	Instructions string          `toml:"instructions,omitempty"` // DEPRECATED: use prompt
	Assignee     string          `toml:"assignee,omitempty"`     // DEPRECATED: use agent
	Code         string          `toml:"code,omitempty"`         // DEPRECATED: use command
	Action       string          `toml:"action,omitempty"`       // Legacy action field
	Validation   string          `toml:"validation,omitempty"`   // Legacy validation field
	Ephemeral    bool            `toml:"ephemeral,omitempty"`    // Legacy ephemeral flag
	LegacyOutputs *TaskOutputSpec `toml:"task_outputs,omitempty"` // DEPRECATED: use outputs
}

// Validate checks that the step has required fields for its executor type.
func (s *Step) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("step id is required")
	}

	if !s.Executor.Valid() {
		return fmt.Errorf("invalid executor: %q", s.Executor)
	}

	// If no executor specified, allow for backwards compatibility
	if s.Executor == "" {
		return nil
	}

	switch s.Executor {
	case ExecutorShell:
		if s.Command == "" {
			return fmt.Errorf("shell executor requires command")
		}
	case ExecutorSpawn:
		if s.Agent == "" {
			return fmt.Errorf("spawn executor requires agent")
		}
	case ExecutorKill:
		if s.Agent == "" {
			return fmt.Errorf("kill executor requires agent")
		}
	case ExecutorExpand:
		if s.Template == "" {
			return fmt.Errorf("expand executor requires template")
		}
	case ExecutorBranch:
		if s.Condition == "" {
			return fmt.Errorf("branch executor requires condition")
		}
	case ExecutorForeach:
		// Exactly one of items or items_file must be set
		if s.Items == "" && s.ItemsFile == "" {
			return fmt.Errorf("foreach executor requires items or items_file")
		}
		if s.Items != "" && s.ItemsFile != "" {
			return fmt.Errorf("foreach executor cannot have both items and items_file")
		}
		if s.ItemVar == "" {
			return fmt.Errorf("foreach executor requires item_var")
		}
		if s.Template == "" {
			return fmt.Errorf("foreach executor requires template")
		}
	case ExecutorAgent:
		if s.Agent == "" {
			return fmt.Errorf("agent executor requires agent")
		}
		if s.Prompt == "" {
			return fmt.Errorf("agent executor requires prompt")
		}
	}

	// Validate mode if specified
	if s.Mode != "" && s.Mode != "autonomous" && s.Mode != "interactive" {
		return fmt.Errorf("invalid mode %q: must be autonomous or interactive", s.Mode)
	}

	// Validate on_error if specified
	if s.OnError != "" && s.OnError != "continue" && s.OnError != "fail" {
		return fmt.Errorf("invalid on_error %q: must be continue or fail", s.OnError)
	}

	return nil
}

// ToStep converts an InlineStep to a Step.
func (is *InlineStep) ToStep() *Step {
	return &Step{
		ID:            is.ID,
		Executor:      is.Executor,
		Needs:         is.Needs,
		Timeout:       is.Timeout,
		Agent:         is.Agent,
		Prompt:        is.Prompt,
		Mode:          is.Mode,
		Command:       is.Command,
		Workdir:       is.Workdir,
		Env:           is.Env,
		OnError:       is.OnError,
		ShellOutputs:  is.ShellOutputs,
		Adapter:       is.Adapter,
		ResumeSession: is.ResumeSession,
		SpawnArgs:     is.SpawnArgs,
		Graceful:      is.Graceful,
		Template:      is.Template,
		Variables:     is.Variables,
		Condition:     is.Condition,
		OnTrue:        is.OnTrue,
		OnFalse:       is.OnFalse,
		OnTimeout:     is.OnTimeout,
		// Foreach fields
		Items:         is.Items,
		ItemVar:       is.ItemVar,
		IndexVar:      is.IndexVar,
		Parallel:      is.Parallel,
		MaxConcurrent: is.MaxConcurrent,
		Join:          is.Join,
		Outputs:       is.Outputs,
		// Legacy fields
		Type:          is.Type,
		Title:         is.Title,
		Description:   is.Description,
		Instructions:  is.Instructions,
		Assignee:      is.Assignee,
		Code:          is.Code,
		Action:        is.Action,
		Validation:    is.Validation,
		Ephemeral:     is.Ephemeral,
		LegacyOutputs: is.LegacyOutputs,
	}
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
// It mirrors the Step struct to ensure all fields are preserved when parsing inline steps.
type InlineStep struct {
	ID       string       `toml:"id"`
	Executor ExecutorType `toml:"executor,omitempty"` // shell | spawn | kill | expand | branch | foreach | agent

	// Shared fields
	Needs   []string `toml:"needs,omitempty"`
	Timeout string   `toml:"timeout,omitempty"`

	// Agent executor fields
	Agent  string `toml:"agent,omitempty"`
	Prompt string `toml:"prompt,omitempty"`
	Mode   string `toml:"mode,omitempty"`

	// Shell executor fields
	Command      string                      `toml:"command,omitempty"`
	Workdir      string                      `toml:"workdir,omitempty"`
	Env          map[string]string           `toml:"env,omitempty"`
	OnError      string                      `toml:"on_error,omitempty"`
	ShellOutputs map[string]OutputSource     `toml:"shell_outputs,omitempty"`

	// Spawn executor fields
	Adapter       string `toml:"adapter,omitempty"`        // Which adapter to use (defaults to config hierarchy)
	ResumeSession string `toml:"resume_session,omitempty"`
	SpawnArgs     string `toml:"spawn_args,omitempty"` // Extra CLI args to append to spawn command

	// Kill executor fields
	Graceful *bool `toml:"graceful,omitempty"`

	// Expand executor fields
	Template  string            `toml:"template,omitempty"`
	Variables map[string]string `toml:"variables,omitempty"`

	// Branch executor fields
	Condition string           `toml:"condition,omitempty"`
	OnTrue    *ExpansionTarget `toml:"on_true,omitempty"`
	OnFalse   *ExpansionTarget `toml:"on_false,omitempty"`
	OnTimeout *ExpansionTarget `toml:"on_timeout,omitempty"`

	// Foreach executor fields
	Items         string `toml:"items,omitempty"`
	ItemsFile     string `toml:"items_file,omitempty"`
	ItemVar       string `toml:"item_var,omitempty"`
	IndexVar      string `toml:"index_var,omitempty"`
	Parallel      any    `toml:"parallel,omitempty"`
	MaxConcurrent any    `toml:"max_concurrent,omitempty"`
	Join          *bool  `toml:"join,omitempty"`
	// Template and Variables fields already defined above for expand executor

	// Agent outputs
	Outputs map[string]AgentOutputDef `toml:"outputs,omitempty"`

	// === Legacy fields - MIGRATION REQUIRED ===
	Type         string          `toml:"type,omitempty"`
	Title        string          `toml:"title,omitempty"`
	Description  string          `toml:"description,omitempty"`
	Instructions string          `toml:"instructions,omitempty"`
	Assignee     string          `toml:"assignee,omitempty"`
	Code         string          `toml:"code,omitempty"`
	Action       string          `toml:"action,omitempty"`
	Validation   string          `toml:"validation,omitempty"`
	Ephemeral    bool            `toml:"ephemeral,omitempty"`
	LegacyOutputs *TaskOutputSpec `toml:"task_outputs,omitempty"`
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

	// Track step IDs and which steps are expand/foreach steps
	stepIDs := make(map[string]bool)
	expandingSteps := make(map[string]bool) // Steps that produce child steps (expand, foreach)
	for i, step := range t.Steps {
		if step.ID == "" {
			return fmt.Errorf("step[%d]: id is required", i)
		}
		if stepIDs[step.ID] {
			return fmt.Errorf("step[%d]: duplicate id %q", i, step.ID)
		}
		stepIDs[step.ID] = true
		if step.Executor == ExecutorExpand || step.Executor == ExecutorForeach {
			expandingSteps[step.ID] = true
		}
	}

	// Validate dependencies reference existing steps
	// Allow references to children of expand/foreach steps (e.g., "expand-step.child")
	// Also allow wildcard patterns for foreach children (e.g., "foreach-step.*.build")
	for i, step := range t.Steps {
		for _, need := range step.Needs {
			if !stepIDs[need] {
				// Check if this references a child of an expand/foreach step
				if dotIdx := strings.Index(need, "."); dotIdx > 0 {
					prefix := need[:dotIdx]
					if expandingSteps[prefix] {
						// This references a child of an expand/foreach step - allowed
						// Includes wildcard patterns like "foreach.*.step"
						continue
					}
				}
				// Check if this is a wildcard pattern like "step-prefix-*"
				if strings.Contains(need, "*") {
					// Wildcards are evaluated at runtime, allow them
					continue
				}
				return fmt.Errorf("step[%d] %q: needs references unknown step %q", i, step.ID, need)
			}
		}
	}

	// Validate variables
	for name, v := range t.Variables {
		if v.Type != "" && v.Type != VarTypeString && v.Type != VarTypeInt && v.Type != VarTypeBool && v.Type != VarTypeFile {
			return fmt.Errorf("variable %q: invalid type %q (must be string, int, bool, or file)", name, v.Type)
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
