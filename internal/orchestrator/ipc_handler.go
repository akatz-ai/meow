package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/meow-stack/meow-machine/internal/ipc"
)

// IPCHandler implements ipc.Handler for the orchestrator.
// It delegates all state-mutating operations to the Orchestrator to ensure
// proper mutex coordination and avoid race conditions.
type IPCHandler struct {
	orch        *Orchestrator     // Reference to orchestrator for state mutations
	store       WorkflowStore     // Read-only access for queries
	agents      *TmuxAgentManager // For agent workdir lookups
	eventRouter *EventRouter
	logger      *slog.Logger
}

// NewIPCHandler creates a new IPC handler.
// The orchestrator reference is used for all state-mutating operations (HandleStepDone,
// HandleApproval) to ensure proper mutex coordination.
func NewIPCHandler(orch *Orchestrator, store WorkflowStore, agents *TmuxAgentManager, logger *slog.Logger) *IPCHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &IPCHandler{
		orch:        orch,
		store:       store,
		agents:      agents,
		eventRouter: NewEventRouter(logger),
		logger:      logger.With("component", "ipc-handler"),
	}
}

// EventRouter returns the event router for this handler.
func (h *IPCHandler) EventRouter() *EventRouter {
	return h.eventRouter
}

// HandleStepDone processes a step completion signal.
// Delegates to Orchestrator.HandleStepDone for thread-safe state mutation.
func (h *IPCHandler) HandleStepDone(ctx context.Context, msg *ipc.StepDoneMessage) any {
	h.logger.Info("handling step_done", "workflow", msg.Workflow, "agent", msg.Agent, "step", msg.Step)

	// Delegate to orchestrator (which has the mutex)
	err := h.orch.HandleStepDone(ctx, msg)
	if err != nil {
		h.logger.Error("step_done failed", "error", err)
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: err.Error(),
		}
	}

	return &ipc.AckMessage{
		Type:    ipc.MsgAck,
		Success: true,
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
// Delegates to Orchestrator.HandleApproval for thread-safe state mutation.
func (h *IPCHandler) HandleApproval(ctx context.Context, msg *ipc.ApprovalMessage) any {
	h.logger.Info("handling approval", "workflow", msg.Workflow, "gate", msg.GateID, "approved", msg.Approved)

	// Delegate to orchestrator (which has the mutex)
	err := h.orch.HandleApproval(ctx, msg)
	if err != nil {
		h.logger.Error("approval failed", "error", err)
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: err.Error(),
		}
	}

	return &ipc.AckMessage{
		Type:    ipc.MsgAck,
		Success: true,
	}
}

// HandleEvent processes an event emitted by an agent.
func (h *IPCHandler) HandleEvent(ctx context.Context, msg *ipc.EventMessage) any {
	h.logger.Info("handling event", "event_type", msg.EventType, "agent", msg.Agent)

	// Add metadata
	msg.Timestamp = time.Now().Unix()

	// Route to waiters
	matched := h.eventRouter.Route(msg)

	h.logger.Info("event processed",
		"event_type", msg.EventType,
		"matched", matched,
		"agent", msg.Agent,
		"workflow", msg.Workflow,
	)

	return &ipc.AckMessage{
		Type:    ipc.MsgAck,
		Success: true,
	}
}

// HandleAwaitEvent waits for an event matching the given criteria.
func (h *IPCHandler) HandleAwaitEvent(ctx context.Context, msg *ipc.AwaitEventMessage) any {
	h.logger.Debug("handling await_event", "event_type", msg.EventType, "filter", msg.Filter, "timeout", msg.Timeout)

	// Parse timeout
	timeout := 24 * time.Hour // Default timeout
	if msg.Timeout != "" {
		parsed, err := time.ParseDuration(msg.Timeout)
		if err != nil {
			h.logger.Warn("invalid timeout, using default", "timeout", msg.Timeout, "error", err)
		} else {
			timeout = parsed
		}
	}

	// Register waiter
	ch := h.eventRouter.RegisterWaiter(msg.EventType, msg.Filter, timeout)

	// Wait for event or timeout
	select {
	case event, ok := <-ch:
		if !ok {
			// Channel closed = timeout
			return &ipc.ErrorMessage{
				Type:    ipc.MsgError,
				Message: "timeout waiting for event",
			}
		}
		return &ipc.EventMatchMessage{
			Type:      ipc.MsgEventMatch,
			EventType: event.EventType,
			Data:      event.Data,
			Timestamp: event.Timestamp,
		}
	case <-time.After(timeout):
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: "timeout waiting for event",
		}
	case <-ctx.Done():
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: "cancelled",
		}
	}
}

// HandleGetStepStatus returns the status of a step.
func (h *IPCHandler) HandleGetStepStatus(ctx context.Context, msg *ipc.GetStepStatusMessage) any {
	h.logger.Debug("handling get_step_status", "workflow", msg.Workflow, "step_id", msg.StepID)

	wf, err := h.store.Get(ctx, msg.Workflow)
	if err != nil {
		h.logger.Error("workflow not found", "workflow", msg.Workflow, "error", err)
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("workflow not found: %s", msg.Workflow),
		}
	}

	step, ok := wf.GetStep(msg.StepID)
	if !ok {
		h.logger.Error("step not found", "step_id", msg.StepID)
		return &ipc.ErrorMessage{
			Type:    ipc.MsgError,
			Message: fmt.Sprintf("step not found: %s", msg.StepID),
		}
	}

	return &ipc.StepStatusMessage{
		Type:   ipc.MsgStepStatus,
		StepID: step.ID,
		Status: string(step.Status),
	}
}
