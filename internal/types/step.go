package types

import (
	"fmt"
	"strings"
	"time"
)

// ExecutorType determines who runs a step and how.
// IMPORTANT: There are exactly 6 executors. Gate is NOT an executor -
// human approval is implemented via branch + meow await-approval.
type ExecutorType string

const (
	// Orchestrator executors - run internally, complete synchronously
	ExecutorShell  ExecutorType = "shell"  // Run shell command
	ExecutorSpawn  ExecutorType = "spawn"  // Start agent in tmux
	ExecutorKill   ExecutorType = "kill"   // Stop agent's tmux session
	ExecutorExpand ExecutorType = "expand" // Inline another workflow
	ExecutorBranch ExecutorType = "branch" // Conditional execution

	// External executors - wait for external completion signal
	ExecutorAgent ExecutorType = "agent" // Agent does work, signals meow done
)

// IsOrchestrator returns true if this executor runs internally.
func (e ExecutorType) IsOrchestrator() bool {
	switch e {
	case ExecutorShell, ExecutorSpawn, ExecutorKill, ExecutorExpand, ExecutorBranch:
		return true
	}
	return false
}

// IsExternal returns true if this executor waits for external signal.
func (e ExecutorType) IsExternal() bool {
	return e == ExecutorAgent
}

// Valid returns true if this is a recognized executor type.
func (e ExecutorType) Valid() bool {
	switch e {
	case ExecutorShell, ExecutorSpawn, ExecutorKill, ExecutorExpand, ExecutorBranch, ExecutorAgent:
		return true
	}
	return false
}

// StepStatus represents the lifecycle state of a step.
type StepStatus string

const (
	StepStatusPending    StepStatus = "pending"    // Waiting for dependencies
	StepStatusRunning    StepStatus = "running"    // Currently executing
	StepStatusCompleting StepStatus = "completing" // Agent called meow done, orchestrator handling transition
	StepStatusDone       StepStatus = "done"       // Completed successfully
	StepStatusFailed     StepStatus = "failed"     // Execution failed
)

// Valid returns true if this is a recognized status.
func (s StepStatus) Valid() bool {
	switch s {
	case StepStatusPending, StepStatusRunning, StepStatusCompleting, StepStatusDone, StepStatusFailed:
		return true
	}
	return false
}

// IsTerminal returns true if this status is final (done or failed).
func (s StepStatus) IsTerminal() bool {
	return s == StepStatusDone || s == StepStatusFailed
}

// CanTransitionTo returns true if transitioning from s to target is valid.
func (s StepStatus) CanTransitionTo(target StepStatus) bool {
	switch s {
	case StepStatusPending:
		return target == StepStatusRunning
	case StepStatusRunning:
		return target == StepStatusCompleting || target == StepStatusDone || target == StepStatusFailed || target == StepStatusPending // Reset on crash
	case StepStatusCompleting:
		return target == StepStatusDone || target == StepStatusRunning // Back to running if validation fails
	case StepStatusDone, StepStatusFailed:
		return false // Terminal states
	}
	return false
}

// OutputSource defines where to capture output from shell commands.
type OutputSource struct {
	Source string `yaml:"source" toml:"source"` // stdout | stderr | exit_code | file:/path
}

// ShellConfig for executor: shell
type ShellConfig struct {
	Command string                  `yaml:"command" toml:"command"`
	Workdir string                  `yaml:"workdir,omitempty" toml:"workdir,omitempty"`
	Env     map[string]string       `yaml:"env,omitempty" toml:"env,omitempty"`
	OnError string                  `yaml:"on_error,omitempty" toml:"on_error,omitempty"` // continue | fail (default: fail)
	Outputs map[string]OutputSource `yaml:"outputs,omitempty" toml:"outputs,omitempty"`
}

// SpawnConfig for executor: spawn
type SpawnConfig struct {
	Agent         string            `yaml:"agent" toml:"agent"`
	Adapter       string            `yaml:"adapter,omitempty" toml:"adapter,omitempty"` // Which adapter to use (defaults to config hierarchy)
	Workdir       string            `yaml:"workdir,omitempty" toml:"workdir,omitempty"`
	Env           map[string]string `yaml:"env,omitempty" toml:"env,omitempty"`
	ResumeSession string            `yaml:"resume_session,omitempty" toml:"resume_session,omitempty"`
}

// KillConfig for executor: kill
type KillConfig struct {
	Agent    string `yaml:"agent" toml:"agent"`
	Graceful bool   `yaml:"graceful,omitempty" toml:"graceful,omitempty"` // Default: true
	Timeout  int    `yaml:"timeout,omitempty" toml:"timeout,omitempty"`   // Seconds, default: 10
}

// ExpandConfig for executor: expand
type ExpandConfig struct {
	Template  string            `yaml:"template" toml:"template"`
	Variables map[string]string `yaml:"variables,omitempty" toml:"variables,omitempty"`
}

// BranchTarget defines what to expand for a branch outcome.
type BranchTarget struct {
	Template  string            `yaml:"template,omitempty" toml:"template,omitempty"`
	Variables map[string]string `yaml:"variables,omitempty" toml:"variables,omitempty"`
	Inline    []InlineStep      `yaml:"inline,omitempty" toml:"inline,omitempty"`
}

// InlineStep is used for inline step definitions in branch targets.
type InlineStep struct {
	ID       string       `yaml:"id" toml:"id"`
	Executor ExecutorType `yaml:"executor" toml:"executor"`
	Prompt   string       `yaml:"prompt,omitempty" toml:"prompt,omitempty"`
	Agent    string       `yaml:"agent,omitempty" toml:"agent,omitempty"`
	Needs    []string     `yaml:"needs,omitempty" toml:"needs,omitempty"`
}

// BranchConfig for executor: branch
type BranchConfig struct {
	Condition string        `yaml:"condition" toml:"condition"` // Shell command, exit 0 = true
	OnTrue    *BranchTarget `yaml:"on_true,omitempty" toml:"on_true,omitempty"`
	OnFalse   *BranchTarget `yaml:"on_false,omitempty" toml:"on_false,omitempty"`
	OnTimeout *BranchTarget `yaml:"on_timeout,omitempty" toml:"on_timeout,omitempty"`
	Timeout   string        `yaml:"timeout,omitempty" toml:"timeout,omitempty"` // Duration string
}

// AgentOutputDef defines an expected output from an agent step.
type AgentOutputDef struct {
	Required    bool   `yaml:"required" toml:"required"`
	Type        string `yaml:"type" toml:"type"` // string | number | boolean | json | file_path
	Description string `yaml:"description,omitempty" toml:"description,omitempty"`
}

// AgentConfig for executor: agent
type AgentConfig struct {
	Agent   string                    `yaml:"agent" toml:"agent"`
	Prompt  string                    `yaml:"prompt" toml:"prompt"`
	Mode    string                    `yaml:"mode,omitempty" toml:"mode,omitempty"` // autonomous (default) | interactive
	Outputs map[string]AgentOutputDef `yaml:"outputs,omitempty" toml:"outputs,omitempty"`
	Timeout string                    `yaml:"timeout,omitempty" toml:"timeout,omitempty"` // Max time for step
}

// StepError captures failure information.
type StepError struct {
	Message string `yaml:"message"`
	Code    int    `yaml:"code,omitempty"`   // Exit code for shell
	Output  string `yaml:"output,omitempty"` // stderr or other context
}

// Step is the single primitive in MEOW. Everything is a step.
// IMPORTANT: Only 6 executor configs for 6 executors.
type Step struct {
	// Identity
	ID       string       `yaml:"id"`
	Executor ExecutorType `yaml:"executor"`

	// Lifecycle
	Status    StepStatus `yaml:"status"`
	StartedAt *time.Time `yaml:"started_at,omitempty"`
	DoneAt    *time.Time `yaml:"done_at,omitempty"`

	// Dependencies
	Needs []string `yaml:"needs,omitempty"`

	// Expansion tracking (for crash recovery)
	ExpandedFrom string   `yaml:"expanded_from,omitempty"` // Parent expand step ID
	ExpandedInto []string `yaml:"expanded_into,omitempty"` // Child step IDs (on expand steps)

	// Data
	Outputs map[string]any `yaml:"outputs,omitempty"`
	Error   *StepError     `yaml:"error,omitempty"`

	// Executor-specific config (exactly one populated based on Executor)
	Shell  *ShellConfig  `yaml:"shell,omitempty"`
	Spawn  *SpawnConfig  `yaml:"spawn,omitempty"`
	Kill   *KillConfig   `yaml:"kill,omitempty"`
	Expand *ExpandConfig `yaml:"expand,omitempty"`
	Branch *BranchConfig `yaml:"branch,omitempty"`
	Agent  *AgentConfig  `yaml:"agent,omitempty"`
}

// IsReady returns true if all dependencies are done.
func (s *Step) IsReady(steps map[string]*Step) bool {
	if s.Status != StepStatusPending {
		return false
	}
	for _, depID := range s.Needs {
		dep, ok := steps[depID]
		if !ok || dep.Status != StepStatusDone {
			return false
		}
	}
	return true
}

// Validate checks the step is well-formed.
func (s *Step) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("step ID is required")
	}
	// Dots are reserved for expansion prefixes (e.g., "parent.child").
	// Only reject dots for steps that aren't expanded from another step.
	if strings.Contains(s.ID, ".") && s.ExpandedFrom == "" {
		return fmt.Errorf("step ID cannot contain dots (reserved for expansion prefixes)")
	}
	if !s.Executor.Valid() {
		return fmt.Errorf("invalid executor: %s", s.Executor)
	}
	return s.validateConfig()
}

// validateConfig ensures exactly one config is set matching the executor.
func (s *Step) validateConfig() error {
	configs := map[ExecutorType]bool{
		ExecutorShell:  s.Shell != nil,
		ExecutorSpawn:  s.Spawn != nil,
		ExecutorKill:   s.Kill != nil,
		ExecutorExpand: s.Expand != nil,
		ExecutorBranch: s.Branch != nil,
		ExecutorAgent:  s.Agent != nil,
	}

	if !configs[s.Executor] {
		return fmt.Errorf("step %s: missing config for executor %s", s.ID, s.Executor)
	}

	for exec, hasConfig := range configs {
		if hasConfig && exec != s.Executor {
			return fmt.Errorf("step %s: has config for %s but executor is %s", s.ID, exec, s.Executor)
		}
	}
	return nil
}

// Complete marks the step as done with outputs.
func (s *Step) Complete(outputs map[string]any) error {
	if !s.Status.CanTransitionTo(StepStatusDone) {
		return fmt.Errorf("cannot complete step in status %s", s.Status)
	}
	now := time.Now()
	s.Status = StepStatusDone
	s.DoneAt = &now
	s.Outputs = outputs
	return nil
}

// Fail marks the step as failed with error info.
func (s *Step) Fail(err *StepError) error {
	if !s.Status.CanTransitionTo(StepStatusFailed) {
		return fmt.Errorf("cannot fail step in status %s", s.Status)
	}
	now := time.Now()
	s.Status = StepStatusFailed
	s.DoneAt = &now
	s.Error = err
	return nil
}

// SetCompleting marks the step as transitioning to done.
func (s *Step) SetCompleting() error {
	if !s.Status.CanTransitionTo(StepStatusCompleting) {
		return fmt.Errorf("cannot set completing in status %s", s.Status)
	}
	s.Status = StepStatusCompleting
	return nil
}

// Start marks the step as running.
func (s *Step) Start() error {
	if !s.Status.CanTransitionTo(StepStatusRunning) {
		return fmt.Errorf("cannot start step in status %s", s.Status)
	}
	now := time.Now()
	s.Status = StepStatusRunning
	s.StartedAt = &now
	return nil
}

// ResetToPending resets the step to pending state (for crash recovery).
func (s *Step) ResetToPending() error {
	if s.Status != StepStatusRunning {
		return fmt.Errorf("can only reset running steps to pending, got %s", s.Status)
	}
	s.Status = StepStatusPending
	s.StartedAt = nil
	return nil
}
