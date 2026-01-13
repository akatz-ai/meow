package status

import (
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// WorkflowSummary contains computed information about a workflow for display.
type WorkflowSummary struct {
	ID          string                 `json:"id"`
	Template    string                 `json:"template"`
	Status      types.RunStatus   `json:"status"`
	StartedAt   time.Time              `json:"started_at"`
	DoneAt      *time.Time             `json:"done_at,omitempty"`
	Variables   map[string]string      `json:"variables,omitempty"`
	StepStats   StepStats              `json:"step_stats"`
	RunningSteps []RunningStep         `json:"running_steps,omitempty"`
	Agents      []AgentSummary         `json:"agents,omitempty"`
	Errors      []string               `json:"errors,omitempty"`
}

// StepStats contains step count breakdown.
type StepStats struct {
	Total      int `json:"total"`
	Done       int `json:"done"`
	Running    int `json:"running"`
	Pending    int `json:"pending"`
	Failed     int `json:"failed"`
	Skipped    int `json:"skipped"`
	Completing int `json:"completing"`
}

// RunningStep contains info about a currently running step.
type RunningStep struct {
	ID        string        `json:"id"`
	Executor  string        `json:"executor"`
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration"`
	AgentID   string        `json:"agent_id,omitempty"`
}

// AgentSummary contains info about an agent.
type AgentSummary struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	TmuxSession string `json:"tmux_session"`
	CurrentStep string `json:"current_step,omitempty"`
}

// NewWorkflowSummary creates a summary from a workflow.
func NewWorkflowSummary(wf *types.Run) *WorkflowSummary {
	summary := &WorkflowSummary{
		ID:        wf.ID,
		Template:  wf.Template,
		Status:    wf.Status,
		StartedAt: wf.StartedAt,
		DoneAt:    wf.DoneAt,
		Variables: wf.Variables,
		StepStats: computeStepStats(wf),
	}

	// Collect running steps
	for _, step := range wf.Steps {
		if step.Status == types.StepStatusRunning || step.Status == types.StepStatusCompleting {
			rs := RunningStep{
				ID:       step.ID,
				Executor: string(step.Executor),
			}
			if step.StartedAt != nil {
				rs.StartedAt = *step.StartedAt
				rs.Duration = time.Since(*step.StartedAt)
			}
			if step.Agent != nil {
				rs.AgentID = step.Agent.Agent
			}
			summary.RunningSteps = append(summary.RunningSteps, rs)
		}
	}

	// Collect agent info
	for id, agentInfo := range wf.Agents {
		summary.Agents = append(summary.Agents, AgentSummary{
			ID:          id,
			Status:      agentInfo.Status,
			TmuxSession: agentInfo.TmuxSession,
			CurrentStep: agentInfo.CurrentStep,
		})
	}

	// Collect errors from failed steps
	for _, step := range wf.Steps {
		if step.Status == types.StepStatusFailed && step.Error != nil {
			summary.Errors = append(summary.Errors, step.Error.Message)
		}
	}

	return summary
}

// computeStepStats tallies up step statuses.
func computeStepStats(wf *types.Run) StepStats {
	stats := StepStats{
		Total: len(wf.Steps),
	}

	for _, step := range wf.Steps {
		switch step.Status {
		case types.StepStatusDone:
			stats.Done++
		case types.StepStatusRunning:
			stats.Running++
		case types.StepStatusPending:
			stats.Pending++
		case types.StepStatusFailed:
			stats.Failed++
		case types.StepStatusSkipped:
			stats.Skipped++
		case types.StepStatusCompleting:
			stats.Completing++
		}
	}

	return stats
}
