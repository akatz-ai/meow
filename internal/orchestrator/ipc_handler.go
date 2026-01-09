package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/meow-stack/meow-machine/internal/types"
)

// IPCHandler implements ipc.Handler for the orchestrator.
type IPCHandler struct {
	store  WorkflowStore
	agents *TmuxAgentManager
	logger *slog.Logger

	mu sync.RWMutex
}

// NewIPCHandler creates a new IPC handler.
func NewIPCHandler(store WorkflowStore, agents *TmuxAgentManager, logger *slog.Logger) *IPCHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &IPCHandler{
		store:  store,
		agents: agents,
		logger: logger.With("component", "ipc-handler"),
	}
}

// HandleStepDone processes a step completion signal.
func (h *IPCHandler) HandleStepDone(ctx context.Context, msg *ipc.StepDoneMessage) any {
	h.logger.Info("handling step_done", "workflow", msg.Workflow, "agent", msg.Agent, "step", msg.Step)

	wf, err := h.store.Get(ctx, msg.Workflow)
	if err != nil {
		h.logger.Error("workflow not found", "workflow", msg.Workflow, "error", err)
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("workflow not found: %s", msg.Workflow),
		}
	}

	var step *types.Step
	var ok bool

	// If step ID is provided, use it; otherwise find the running step for this agent
	if msg.Step != "" {
		step, ok = wf.GetStep(msg.Step)
		if !ok {
			h.logger.Error("step not found", "step", msg.Step)
			return &ipc.ErrorMessage{
				Type:    ipc.MsgError,
				Message: fmt.Sprintf("step not found: %s", msg.Step),
			}
		}
	} else {
		// Find the running agent step for this agent
		step = wf.GetRunningStepForAgent(msg.Agent)
		if step == nil {
			h.logger.Error("no running step found for agent", "agent", msg.Agent)
			return &ipc.ErrorMessage{
				Type:    ipc.MsgError,
				Message: fmt.Sprintf("no running step found for agent: %s", msg.Agent),
			}
		}
	}

	// Validate step is in running state
	if step.Status != types.StepStatusRunning {
		h.logger.Warn("step not running", "step", msg.Step, "status", step.Status)
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("step %s is not running (status: %s)", msg.Step, step.Status),
		}
	}

	// Validate agent matches
	if step.Agent != nil && step.Agent.Agent != msg.Agent {
		h.logger.Warn("agent mismatch", "expected", step.Agent.Agent, "got", msg.Agent)
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("step %s is not assigned to agent %s", msg.Step, msg.Agent),
		}
	}

	// Transition to completing
	if err := step.SetCompleting(); err != nil {
		h.logger.Error("failed to set completing", "error", err)
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("failed to transition step: %v", err),
		}
	}

	// Validate outputs if defined
	if step.Agent != nil && len(step.Agent.Outputs) > 0 {
		agentWorkdir := h.agents.GetWorkdir(msg.Agent)
		errs := ValidateAgentOutputs(msg.Outputs, step.Agent.Outputs, agentWorkdir)
		if len(errs) > 0 {
			// Validation failed - keep step running so agent can retry
			step.Status = types.StepStatusRunning
			h.logger.Warn("output validation failed", "step", step.ID, "errors", errs)
			if err := h.store.Save(ctx, wf); err != nil {
				h.logger.Error("failed to save workflow", "error", err)
			}
			return &ipc.ErrorMessage{
				Type:    ipc.MsgError,
				Message: fmt.Sprintf("output validation failed: %v", errs),
			}
		}
	}

	// Mark step complete
	if err := step.Complete(msg.Outputs); err != nil {
		h.logger.Error("failed to complete step", "error", err)
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("failed to complete step: %v", err),
		}
	}

	// Save workflow
	if err := h.store.Save(ctx, wf); err != nil {
		h.logger.Error("failed to save workflow", "error", err)
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("failed to save workflow: %v", err),
		}
	}

	h.logger.Info("step completed", "step", step.ID)
	return &ipc.AckMessage{
		Type:    ipc.MsgAck,
		Success: true,
	}
}

// HandleGetPrompt returns the current prompt for an agent.
func (h *IPCHandler) HandleGetPrompt(ctx context.Context, msg *ipc.GetPromptMessage) any {
	h.logger.Debug("handling get_prompt", "agent", msg.Agent)

	// Find workflows with work for this agent
	workflows, err := h.store.GetByAgent(ctx, msg.Agent)
	if err != nil {
		h.logger.Error("failed to get workflows", "error", err)
		return &ipc.PromptMessage{
			Type:    ipc.MsgPrompt,
			Content: "",
		}
	}

	for _, wf := range workflows {
		// Check for running step
		step := wf.GetRunningStepForAgent(msg.Agent)
		if step != nil {
			// Check if step is completing
			if step.Status == types.StepStatusCompleting {
				// Step is transitioning - return empty
				return &ipc.PromptMessage{
					Type:    ipc.MsgPrompt,
					Content: "",
				}
			}

			// Check mode
			if step.Agent != nil && step.Agent.Mode == "interactive" {
				// Interactive mode - return empty to allow conversation
				return &ipc.PromptMessage{
					Type:    ipc.MsgPrompt,
					Content: "",
				}
			}

			// Autonomous mode - return prompt as nudge
			prompt := GetPromptForStopHook(step)
			return &ipc.PromptMessage{
				Type:    ipc.MsgPrompt,
				Content: prompt,
			}
		}

		// Check for next ready step
		nextStep := wf.GetNextReadyStepForAgent(msg.Agent)
		if nextStep != nil {
			// There's work pending - orchestrator will inject on next tick
			// Return empty to avoid duplicate injection
			return &ipc.PromptMessage{
				Type:    ipc.MsgPrompt,
				Content: "",
			}
		}
	}

	// No work - agent should idle
	return &ipc.PromptMessage{
		Type:    ipc.MsgPrompt,
		Content: "",
	}
}

// HandleGetSessionID returns the Claude session ID for an agent.
func (h *IPCHandler) HandleGetSessionID(ctx context.Context, msg *ipc.GetSessionIDMessage) any {
	h.logger.Debug("handling get_session_id", "agent", msg.Agent)

	// Get workflows for this agent
	workflows, err := h.store.GetByAgent(ctx, msg.Agent)
	if err != nil {
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("failed to get workflows: %v", err),
		}
	}

	for _, wf := range workflows {
		if agent, ok := wf.Agents[msg.Agent]; ok && agent.ClaudeSession != "" {
			return &ipc.SessionIDMessage{
				Type:      ipc.MsgSessionID,
				SessionID: agent.ClaudeSession,
			}
		}
	}

	return &ipc.ErrorMessage{
		Type:    ipc.MsgError,
		Message: "no session ID found for agent",
	}
}

// HandleApproval processes a human approval/rejection signal.
func (h *IPCHandler) HandleApproval(ctx context.Context, msg *ipc.ApprovalMessage) any {
	h.logger.Info("handling approval", "workflow", msg.Workflow, "gate", msg.GateID, "approved", msg.Approved)

	wf, err := h.store.Get(ctx, msg.Workflow)
	if err != nil {
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("workflow not found: %s", msg.Workflow),
		}
	}

	step, ok := wf.GetStep(msg.GateID)
	if !ok {
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("gate step not found: %s", msg.GateID),
		}
	}

	if step.Status != types.StepStatusRunning {
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("gate step is not running: %s", step.Status),
		}
	}

	// Set output for branch condition to check
	if step.Outputs == nil {
		step.Outputs = make(map[string]any)
	}
	step.Outputs["approved"] = msg.Approved
	step.Outputs["notes"] = msg.Notes

	if err := h.store.Save(ctx, wf); err != nil {
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("failed to save workflow: %v", err),
		}
	}

	return &ipc.AckMessage{
		Type:    ipc.MsgAck,
		Success: true,
	}
}
