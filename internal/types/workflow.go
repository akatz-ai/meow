package types

import (
	"fmt"
	"time"
)

// WorkflowStatus represents the lifecycle state of a workflow.
type WorkflowStatus string

const (
	WorkflowStatusPending WorkflowStatus = "pending" // Created but not started
	WorkflowStatusRunning WorkflowStatus = "running" // Orchestrator is processing
	WorkflowStatusDone    WorkflowStatus = "done"    // All steps completed
	WorkflowStatusFailed  WorkflowStatus = "failed"  // A step failed
)

// Valid returns true if this is a recognized workflow status.
func (s WorkflowStatus) Valid() bool {
	switch s {
	case WorkflowStatusPending, WorkflowStatusRunning, WorkflowStatusDone, WorkflowStatusFailed:
		return true
	}
	return false
}

// IsTerminal returns true if this status is final (done or failed).
func (s WorkflowStatus) IsTerminal() bool {
	return s == WorkflowStatusDone || s == WorkflowStatusFailed
}

// AgentInfo tracks persisted state for an agent.
type AgentInfo struct {
	TmuxSession   string `yaml:"tmux_session"`
	Status        string `yaml:"status"`                    // active, idle
	Workdir       string `yaml:"workdir"`                   // Working directory for file_path validation
	CurrentStep   string `yaml:"current_step,omitempty"`    // Step currently assigned to agent
	ClaudeSession string `yaml:"claude_session,omitempty"`  // Session ID for resume
}

// Workflow represents a running workflow instance.
type Workflow struct {
	// Identity
	ID       string `yaml:"id"`       // Unique identifier (e.g., "wf-abc123")
	Template string `yaml:"template"` // Source template path

	// Lifecycle
	Status    WorkflowStatus `yaml:"status"`
	StartedAt time.Time      `yaml:"started_at"`
	DoneAt    *time.Time     `yaml:"done_at,omitempty"`

	// Configuration
	Variables map[string]string `yaml:"variables,omitempty"`

	// Agent state - tracked for crash recovery and file_path validation
	Agents map[string]*AgentInfo `yaml:"agents,omitempty"`

	// State - all steps with their current state
	Steps map[string]*Step `yaml:"steps"`
}

// NewWorkflow creates a new workflow instance.
func NewWorkflow(id, template string, vars map[string]string) *Workflow {
	return &Workflow{
		ID:        id,
		Template:  template,
		Status:    WorkflowStatusPending,
		StartedAt: time.Now(),
		Variables: vars,
		Agents:    make(map[string]*AgentInfo),
		Steps:     make(map[string]*Step),
	}
}

// AddStep adds a step to the workflow.
func (w *Workflow) AddStep(step *Step) error {
	if _, exists := w.Steps[step.ID]; exists {
		return fmt.Errorf("step %s already exists", step.ID)
	}
	w.Steps[step.ID] = step
	return nil
}

// RegisterAgent adds or updates agent state.
func (w *Workflow) RegisterAgent(id string, info *AgentInfo) {
	w.Agents[id] = info
}

// GetAgentWorkdir returns the working directory for an agent.
// Used for file_path output validation.
func (w *Workflow) GetAgentWorkdir(agentID string) (string, bool) {
	agent, ok := w.Agents[agentID]
	if !ok {
		return "", false
	}
	return agent.Workdir, true
}

// GetReadySteps returns all steps that are ready to execute.
func (w *Workflow) GetReadySteps() []*Step {
	var ready []*Step
	for _, step := range w.Steps {
		if step.IsReady(w.Steps) {
			ready = append(ready, step)
		}
	}
	return ready
}

// AllDone returns true if all steps are in terminal state.
func (w *Workflow) AllDone() bool {
	for _, step := range w.Steps {
		if !step.Status.IsTerminal() {
			return false
		}
	}
	return true
}

// HasFailed returns true if any step has failed.
func (w *Workflow) HasFailed() bool {
	for _, step := range w.Steps {
		if step.Status == StepStatusFailed {
			return true
		}
	}
	return false
}

// Complete marks the workflow as done.
func (w *Workflow) Complete() {
	now := time.Now()
	w.Status = WorkflowStatusDone
	w.DoneAt = &now
}

// Fail marks the workflow as failed.
func (w *Workflow) Fail() {
	now := time.Now()
	w.Status = WorkflowStatusFailed
	w.DoneAt = &now
}

// Start marks the workflow as running.
func (w *Workflow) Start() error {
	if w.Status != WorkflowStatusPending {
		return fmt.Errorf("cannot start workflow in status %s", w.Status)
	}
	w.Status = WorkflowStatusRunning
	return nil
}

// GetStep retrieves a step by ID.
func (w *Workflow) GetStep(id string) (*Step, bool) {
	step, ok := w.Steps[id]
	return step, ok
}

// GetStepsForAgent returns steps assigned to the given agent.
func (w *Workflow) GetStepsForAgent(agentID string) []*Step {
	var result []*Step
	for _, step := range w.Steps {
		if step.Executor == ExecutorAgent && step.Agent != nil && step.Agent.Agent == agentID {
			result = append(result, step)
		}
	}
	return result
}

// GetRunningStepForAgent returns the running step for the given agent, if any.
func (w *Workflow) GetRunningStepForAgent(agentID string) *Step {
	for _, step := range w.Steps {
		if step.Executor == ExecutorAgent &&
			step.Agent != nil &&
			step.Agent.Agent == agentID &&
			step.Status == StepStatusRunning {
			return step
		}
	}
	return nil
}

// GetNextReadyStepForAgent returns the next ready step for the given agent, if any.
func (w *Workflow) GetNextReadyStepForAgent(agentID string) *Step {
	for _, step := range w.Steps {
		if step.Executor == ExecutorAgent &&
			step.Agent != nil &&
			step.Agent.Agent == agentID &&
			step.IsReady(w.Steps) {
			return step
		}
	}
	return nil
}

// AgentIsIdle returns true if no step assigned to the agent is currently running.
func (w *Workflow) AgentIsIdle(agentID string) bool {
	for _, step := range w.Steps {
		if step.Executor == ExecutorAgent &&
			step.Agent != nil &&
			step.Agent.Agent == agentID &&
			step.Status == StepStatusRunning {
			return false
		}
	}
	return true
}
