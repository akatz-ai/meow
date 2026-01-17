package types

import (
	"fmt"
	"sort"
	"time"
)

// RunStatus represents the lifecycle state of a run.
type RunStatus string

const (
	RunStatusPending    RunStatus = "pending"     // Created but not started
	RunStatusRunning    RunStatus = "running"     // Orchestrator is processing
	RunStatusCleaningUp RunStatus = "cleaning_up" // Running cleanup script
	RunStatusDone       RunStatus = "done"        // All steps completed
	RunStatusFailed     RunStatus = "failed"      // A step failed
	RunStatusStopped    RunStatus = "stopped"     // Manually stopped via meow stop
)

// Valid returns true if this is a recognized run status.
func (s RunStatus) Valid() bool {
	switch s {
	case RunStatusPending, RunStatusRunning, RunStatusCleaningUp,
		RunStatusDone, RunStatusFailed, RunStatusStopped:
		return true
	}
	return false
}

// IsTerminal returns true if this status is final (done, failed, or stopped).
func (s RunStatus) IsTerminal() bool {
	return s == RunStatusDone || s == RunStatusFailed || s == RunStatusStopped
}

// AgentInfo tracks persisted state for an agent.
type AgentInfo struct {
	TmuxSession   string `yaml:"tmux_session"`
	Status        string `yaml:"status"`                   // active, idle
	Workdir       string `yaml:"workdir"`                  // Working directory for file_path validation
	CurrentStep   string `yaml:"current_step,omitempty"`   // Step currently assigned to agent
	ClaudeSession string `yaml:"claude_session,omitempty"` // Session ID for resume
}

// Run represents a running workflow instance.
type Run struct {
	// Identity
	ID       string `yaml:"id"`       // Unique identifier (e.g., "run-abc123")
	Template string `yaml:"template"` // Source template path
	Scope    string `yaml:"scope,omitempty"` // Resolution scope: "project", "user", or "embedded"

	// Lifecycle
	Status    RunStatus  `yaml:"status"`
	StartedAt time.Time  `yaml:"started_at"`
	DoneAt    *time.Time `yaml:"done_at,omitempty"`

	// Orchestrator tracking
	OrchestratorPID int `yaml:"orchestrator_pid,omitempty"` // Process ID of running orchestrator (0 if not running)

	// Configuration
	Variables      map[string]any `yaml:"variables,omitempty"`
	DefaultAdapter string            `yaml:"default_adapter,omitempty"` // Run-level default adapter

	// Conditional cleanup scripts (from template) - all are opt-in, no cleanup by default
	// Each runs on the specified trigger, kills agents, then executes the script
	CleanupOnSuccess string `yaml:"cleanup_on_success,omitempty"` // Runs when all steps complete successfully
	CleanupOnFailure string `yaml:"cleanup_on_failure,omitempty"` // Runs when a step fails
	CleanupOnStop    string `yaml:"cleanup_on_stop,omitempty"`    // Runs on SIGINT/SIGTERM or meow stop

	// Prior status before cleanup - used to determine final status after cleanup
	PriorStatus RunStatus `yaml:"prior_status,omitempty"`

	// Agent state - tracked for crash recovery and file_path validation
	Agents map[string]*AgentInfo `yaml:"agents,omitempty"`

	// State - all steps with their current state
	Steps map[string]*Step `yaml:"steps"`
}

// NewRun creates a new run instance.
func NewRun(id, template string, vars map[string]any) *Run {
	return &Run{
		ID:        id,
		Template:  template,
		Status:    RunStatusPending,
		StartedAt: time.Now(),
		Variables: vars,
		Agents:    make(map[string]*AgentInfo),
		Steps:     make(map[string]*Step),
	}
}

// AddStep adds a step to the run.
func (r *Run) AddStep(step *Step) error {
	if _, exists := r.Steps[step.ID]; exists {
		return fmt.Errorf("step %s already exists", step.ID)
	}
	r.Steps[step.ID] = step
	return nil
}

// RegisterAgent adds or updates agent state.
func (r *Run) RegisterAgent(id string, info *AgentInfo) {
	if r.Agents == nil {
		r.Agents = make(map[string]*AgentInfo)
	}
	r.Agents[id] = info
}

// GetAgentWorkdir returns the working directory for an agent.
// Used for file_path output validation.
func (r *Run) GetAgentWorkdir(agentID string) (string, bool) {
	agent, ok := r.Agents[agentID]
	if !ok {
		return "", false
	}
	return agent.Workdir, true
}

// GetReadySteps returns all steps that are ready to execute.
// Steps are returned in deterministic order (sorted by ID).
func (r *Run) GetReadySteps() []*Step {
	var ready []*Step
	for _, step := range r.Steps {
		if step.IsReady(r.Steps) {
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
func (r *Run) AllDone() bool {
	for _, step := range r.Steps {
		if !step.Status.IsTerminal() {
			return false
		}
	}
	return true
}

// HasFailed returns true if any step has failed.
func (r *Run) HasFailed() bool {
	for _, step := range r.Steps {
		if step.Status == StepStatusFailed {
			return true
		}
	}
	return false
}

// Complete marks the run as done.
func (r *Run) Complete() {
	now := time.Now()
	r.Status = RunStatusDone
	r.DoneAt = &now
}

// Fail marks the run as failed.
func (r *Run) Fail() {
	now := time.Now()
	r.Status = RunStatusFailed
	r.DoneAt = &now
}

// StartCleanup transitions the run to the cleaning_up state.
// Records the prior status for determining final status after cleanup.
func (r *Run) StartCleanup(reason RunStatus) error {
	if r.Status == RunStatusCleaningUp {
		return nil // Already cleaning up
	}
	if r.Status.IsTerminal() {
		return fmt.Errorf("cannot start cleanup for run in terminal status %s", r.Status)
	}
	r.PriorStatus = reason
	r.Status = RunStatusCleaningUp
	return nil
}

// FinishCleanup transitions from cleaning_up to the final status.
// Uses PriorStatus to determine the appropriate final status.
func (r *Run) FinishCleanup() {
	if r.Status != RunStatusCleaningUp {
		return
	}
	now := time.Now()
	r.DoneAt = &now

	// Determine final status based on prior status
	switch r.PriorStatus {
	case RunStatusStopped:
		r.Status = RunStatusStopped
	case RunStatusFailed:
		r.Status = RunStatusFailed
	default:
		r.Status = RunStatusDone
	}
}

// Stop marks the run as stopped (manual stop via meow stop).
func (r *Run) Stop() {
	now := time.Now()
	r.Status = RunStatusStopped
	r.DoneAt = &now
}

// GetAgentIDs returns all agent IDs registered in this run.
func (r *Run) GetAgentIDs() []string {
	ids := make([]string, 0, len(r.Agents))
	for id := range r.Agents {
		ids = append(ids, id)
	}
	return ids
}

// Start marks the run as running.
func (r *Run) Start() error {
	if r.Status != RunStatusPending {
		return fmt.Errorf("cannot start run in status %s", r.Status)
	}
	r.Status = RunStatusRunning
	return nil
}

// GetStep retrieves a step by ID.
func (r *Run) GetStep(id string) (*Step, bool) {
	step, ok := r.Steps[id]
	return step, ok
}

// GetStepsForAgent returns steps assigned to the given agent.
func (r *Run) GetStepsForAgent(agentID string) []*Step {
	var result []*Step
	for _, step := range r.Steps {
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
func (r *Run) GetRunningStepForAgent(agentID string) *Step {
	for _, step := range r.Steps {
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
func (r *Run) GetNextReadyStepForAgent(agentID string) *Step {
	for _, step := range r.Steps {
		if step.Executor == ExecutorAgent &&
			step.Agent != nil &&
			step.Agent.Agent == agentID &&
			step.IsReady(r.Steps) {
			return step
		}
	}
	return nil
}

// AgentIsIdle returns true if no step assigned to the agent is currently running or completing.
// The completing check prevents injecting a new prompt while the orchestrator is still
// processing the previous step's completion (prevents race conditions).
func (r *Run) AgentIsIdle(agentID string) bool {
	for _, step := range r.Steps {
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
func (r *Run) GetCleanupScript(reason RunStatus) string {
	switch reason {
	case RunStatusDone:
		return r.CleanupOnSuccess
	case RunStatusFailed:
		return r.CleanupOnFailure
	case RunStatusStopped:
		return r.CleanupOnStop
	default:
		return ""
	}
}

// HasCleanup returns true if any cleanup script is defined for the given reason.
func (r *Run) HasCleanup(reason RunStatus) bool {
	return r.GetCleanupScript(reason) != ""
}

// HasAnyCleanup returns true if any cleanup script is defined.
func (r *Run) HasAnyCleanup() bool {
	return r.CleanupOnSuccess != "" || r.CleanupOnFailure != "" || r.CleanupOnStop != ""
}
