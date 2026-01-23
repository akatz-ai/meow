package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/akatz-ai/meow/internal/ipc"
)

// TestHandleEvent_FiltersExpectedAgentStopped verifies that agent-stopped events
// are filtered when they occur within the grace period after a step completion.
// This prevents false nudges when the Claude Code Stop hook fires after meow done.
func TestHandleEvent_FiltersExpectedAgentStopped(t *testing.T) {
	// Create handler with minimal dependencies
	// We only need eventRouter for routing verification
	handler := &IPCHandler{
		eventRouter: NewEventRouter(nil),
		logger:      testLogger(),
	}

	// Register a waiter for agent-stopped events
	waiterCh := handler.eventRouter.RegisterWaiter("agent-stopped", nil, 5*time.Second)

	// Simulate what HandleStepDone does after successful completion:
	// Record the completion time for the agent
	handler.recentCompletions.Store("worker-1", time.Now())

	// Immediately send an agent-stopped event (simulates Stop hook firing)
	result := handler.HandleEvent(context.Background(), &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "agent-stopped",
		Agent:     "worker-1",
		Data:      map[string]any{"reason": "normal"},
	})

	// Should return success (ack)
	ack, ok := result.(*ipc.AckMessage)
	if !ok {
		t.Fatalf("expected AckMessage, got %T", result)
	}
	if !ack.Success {
		t.Error("expected success=true")
	}

	// Verify the event was NOT routed to the waiter (filtered)
	select {
	case <-waiterCh:
		t.Error("event should have been filtered, but was routed to waiter")
	case <-time.After(50 * time.Millisecond):
		// Expected: event was filtered
	}

	// Verify waiter is still registered (event was filtered, not consumed)
	if handler.eventRouter.WaiterCount("agent-stopped") != 1 {
		t.Errorf("expected waiter still registered, got count %d", handler.eventRouter.WaiterCount("agent-stopped"))
	}
}

// TestHandleEvent_PassesThroughAfterGracePeriod verifies that agent-stopped events
// are routed normally when they occur after the grace period expires.
func TestHandleEvent_PassesThroughAfterGracePeriod(t *testing.T) {
	handler := &IPCHandler{
		eventRouter: NewEventRouter(nil),
		logger:      testLogger(),
	}

	// Register a waiter for agent-stopped events
	waiterCh := handler.eventRouter.RegisterWaiter("agent-stopped", nil, 5*time.Second)

	// Record completion time in the past (beyond grace period)
	pastTime := time.Now().Add(-AgentStoppedGracePeriod - time.Second)
	handler.recentCompletions.Store("worker-1", pastTime)

	// Send agent-stopped event (should be routed since grace period expired)
	handler.HandleEvent(context.Background(), &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "agent-stopped",
		Agent:     "worker-1",
		Data:      map[string]any{"reason": "unexpected"},
	})

	// Verify the event WAS routed to the waiter
	select {
	case event := <-waiterCh:
		if event.Agent != "worker-1" {
			t.Errorf("expected agent 'worker-1', got '%s'", event.Agent)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("event should have been routed, but was filtered")
	}
}

// TestHandleEvent_DoesNotFilterOtherEvents verifies that non-agent-stopped events
// are always routed normally, even if the agent has a recent completion.
func TestHandleEvent_DoesNotFilterOtherEvents(t *testing.T) {
	handler := &IPCHandler{
		eventRouter: NewEventRouter(nil),
		logger:      testLogger(),
	}

	// Register a waiter for a different event type
	waiterCh := handler.eventRouter.RegisterWaiter("tool-completed", nil, 5*time.Second)

	// Record recent completion (within grace period)
	handler.recentCompletions.Store("worker-1", time.Now())

	// Send a different event type
	handler.HandleEvent(context.Background(), &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "tool-completed",
		Agent:     "worker-1",
		Data:      map[string]any{"tool": "Bash"},
	})

	// Verify the event WAS routed (not filtered)
	select {
	case event := <-waiterCh:
		if event.EventType != "tool-completed" {
			t.Errorf("expected event_type 'tool-completed', got '%s'", event.EventType)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("non-agent-stopped event should not be filtered")
	}
}

// TestHandleEvent_DoesNotFilterDifferentAgent verifies that agent-stopped events
// for a different agent are not filtered, even if another agent recently completed.
func TestHandleEvent_DoesNotFilterDifferentAgent(t *testing.T) {
	handler := &IPCHandler{
		eventRouter: NewEventRouter(nil),
		logger:      testLogger(),
	}

	// Register a waiter
	waiterCh := handler.eventRouter.RegisterWaiter("agent-stopped", nil, 5*time.Second)

	// Record recent completion for worker-1
	handler.recentCompletions.Store("worker-1", time.Now())

	// Send agent-stopped for worker-2 (different agent)
	handler.HandleEvent(context.Background(), &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "agent-stopped",
		Agent:     "worker-2",
		Data:      map[string]any{"reason": "unexpected"},
	})

	// Verify the event WAS routed (not filtered - different agent)
	select {
	case event := <-waiterCh:
		if event.Agent != "worker-2" {
			t.Errorf("expected agent 'worker-2', got '%s'", event.Agent)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("event for different agent should not be filtered")
	}
}

// TestHandleEvent_FiltersWithEmptyAgentInCompletion verifies that agent-stopped
// events with an empty agent field are not filtered (edge case handling).
func TestHandleEvent_NoFilterWhenAgentEmpty(t *testing.T) {
	handler := &IPCHandler{
		eventRouter: NewEventRouter(nil),
		logger:      testLogger(),
	}

	// Register a waiter
	waiterCh := handler.eventRouter.RegisterWaiter("agent-stopped", nil, 5*time.Second)

	// Note: no completion recorded (empty agent)

	// Send agent-stopped with empty agent
	handler.HandleEvent(context.Background(), &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "agent-stopped",
		Agent:     "", // Empty agent
		Data:      map[string]any{},
	})

	// Verify the event WAS routed (empty agent bypasses filtering)
	select {
	case <-waiterCh:
		// Expected: event was routed
	case <-time.After(100 * time.Millisecond):
		t.Error("event with empty agent should be routed")
	}
}
