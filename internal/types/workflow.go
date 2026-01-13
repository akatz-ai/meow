package types

import (
	"fmt"
	"sort"
	"time"
)

// WorkflowStatus represents the lifecycle state of a workflow.
type WorkflowStatus string

const (
	WorkflowStatusPending    WorkflowStatus = "pending"     // Created but not started
	WorkflowStatusRunning    WorkflowStatus = "running"     // Orchestrator is processing
	WorkflowStatusCleaningUp WorkflowStatus = "cleaning_up" // Running cleanup script
	WorkflowStatusDone       WorkflowStatus = "done"        // All steps completed
	WorkflowStatusFailed     WorkflowStatus = "failed"      // A step failed
	WorkflowStatusStopped    WorkflowStatus = "stopped"     // Manually stopped via meow stop
)

// Valid returns true if this is a recognized workflow status.
func (s WorkflowStatus) Valid() bool {
	switch s {
	case WorkflowStatusPending, WorkflowStatusRunning, WorkflowStatusCleaningUp,
		WorkflowStatusDone, WorkflowStatusFailed, WorkflowStatusStopped:
		return true
	}
	return false
}

// IsTerminal returns true if this status is final (done, failed, or stopped).
func (s WorkflowStatus) IsTerminal() bool {
	return s == WorkflowStatusDone || s == WorkflowStatusFailed || s == WorkflowStatusStopped
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
	Variables      map[string]string `yaml:"variables,omitempty"`
	DefaultAdapter string            `yaml:"default_adapter,omitempty"` // Workflow-level default adapter

	// Conditional cleanup scripts (from template) - all are opt-in, no cleanup by default
	// Each runs on the specified trigger, kills agents, then executes the script
	CleanupOnSuccess string `yaml:"cleanup_on_success,omitempty"` // Runs when all steps complete successfully
	CleanupOnFailure string `yaml:"cleanup_on_failure,omitempty"` // Runs when a step fails
	CleanupOnStop    string `yaml:"cleanup_on_stop,omitempty"`    // Runs on SIGINT/SIGTERM or meow stop

	// Prior status before cleanup - used to determine final status after cleanup
	PriorStatus WorkflowStatus `yaml:"prior_status,omitempty"`

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
	if w.Agents == nil {
		w.Agents = make(map[string]*AgentInfo)
	}
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
// Steps are returned in deterministic order (sorted by ID).
func (w *Workflow) GetReadySteps() []*Step {
	var ready []*Step
	for _, step := range w.Steps {
		if step.IsReady(w.Steps) {
			ready = append(ready, step)
		}
	}

	// Sort by step ID for deterministic ordering
	sort.Slice(ready, func(i, j int) bool {
		return ready[i].ID < ready[j].ID
	})

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

// StartCleanup transitions the workflow to the cleaning_up state.
// Records the prior status for determining final status after cleanup.
func (w *Workflow) StartCleanup(reason WorkflowStatus) error {
	if w.Status == WorkflowStatusCleaningUp {
		return nil // Already cleaning up
	}
	if w.Status.IsTerminal() {
		return fmt.Errorf("cannot start cleanup for workflow in terminal status %s", w.Status)
	}
	w.PriorStatus = reason
	w.Status = WorkflowStatusCleaningUp
	return nil
}

// FinishCleanup transitions from cleaning_up to the final status.
// Uses PriorStatus to determine the appropriate final status.
func (w *Workflow) FinishCleanup() {
	if w.Status != WorkflowStatusCleaningUp {
		return
	}
	now := time.Now()
	w.DoneAt = &now

	// Determine final status based on prior status
	switch w.PriorStatus {
	case WorkflowStatusStopped:
		w.Status = WorkflowStatusStopped
	case WorkflowStatusFailed:
		w.Status = WorkflowStatusFailed
	default:
		w.Status = WorkflowStatusDone
	}
}

// Stop marks the workflow as stopped (manual stop via meow stop).
func (w *Workflow) Stop() {
	now := time.Now()
	w.Status = WorkflowStatusStopped
	w.DoneAt = &now
}

// GetAgentIDs returns all agent IDs registered in this workflow.
func (w *Workflow) GetAgentIDs() []string {
	ids := make([]string, 0, len(w.Agents))
	for id := range w.Agents {
		ids = append(ids, id)
	}
	return ids
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

// GetRunningStepForAgent returns the active step for the given agent, if any.
// This includes steps in both "running" and "completing" states, since a completing
// step is still conceptually "active" (the orchestrator is processing its completion).
// Callers should check the step.Status to determine which state it's in.
func (w *Workflow) GetRunningStepForAgent(agentID string) *Step {
	for _, step := range w.Steps {
		if step.Executor == ExecutorAgent &&
			step.Agent != nil &&
			step.Agent.Agent == agentID &&
			(step.Status == StepStatusRunning || step.Status == StepStatusCompleting) {
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

// AgentIsIdle returns true if no step assigned to the agent is currently running or completing.
// The completing check prevents injecting a new prompt while the orchestrator is still
// processing the previous step's completion (prevents race conditions).
func (w *Workflow) AgentIsIdle(agentID string) bool {
	for _, step := range w.Steps {
		if step.Executor == ExecutorAgent &&
			step.Agent != nil &&
			step.Agent.Agent == agentID &&
			(step.Status == StepStatusRunning || step.Status == StepStatusCompleting) {
			return false
		}
	}
	return true
}

// GetCleanupScript returns the cleanup script for the given reason, or empty string if none defined.
// Cleanup is opt-in: returns empty string unless a cleanup script is explicitly defined for this trigger.
func (w *Workflow) GetCleanupScript(reason WorkflowStatus) string {
	switch reason {
	case WorkflowStatusDone:
		return w.CleanupOnSuccess
	case WorkflowStatusFailed:
		return w.CleanupOnFailure
	case WorkflowStatusStopped:
		return w.CleanupOnStop
	default:
		return ""
	}
}

// HasCleanup returns true if any cleanup script is defined for the given reason.
func (w *Workflow) HasCleanup(reason WorkflowStatus) bool {
	return w.GetCleanupScript(reason) != ""
}

// HasAnyCleanup returns true if any cleanup script is defined.
func (w *Workflow) HasAnyCleanup() bool {
	return w.CleanupOnSuccess != "" || w.CleanupOnFailure != "" || w.CleanupOnStop != ""
}
