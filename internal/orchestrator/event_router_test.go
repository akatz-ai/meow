package orchestrator

import (
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/ipc"
)

func TestEventRouter_RegisterAndRoute(t *testing.T) {
	router := NewEventRouter(nil)

	// Register a waiter
	ch := router.RegisterWaiter("tool-completed", map[string]string{"tool": "Bash"}, 5*time.Second)

	// Create matching event
	event := &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "tool-completed",
		Data: map[string]any{
			"tool":      "Bash",
			"exit_code": 0,
		},
		Agent:     "worker-1",
		Workflow:  "run-123",
		Timestamp: time.Now().Unix(),
	}

	// Route should succeed
	matched := router.Route(event)
	if !matched {
		t.Error("expected event to be matched")
	}

	// Check that waiter received the event
	select {
	case received := <-ch:
		if received.EventType != "tool-completed" {
			t.Errorf("expected event_type 'tool-completed', got '%s'", received.EventType)
		}
		if received.Data["tool"] != "Bash" {
			t.Errorf("expected tool 'Bash', got '%v'", received.Data["tool"])
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestEventRouter_NoMatchingWaiter(t *testing.T) {
	router := NewEventRouter(nil)

	// Register a waiter for different event type
	router.RegisterWaiter("other-event", nil, 5*time.Second)

	// Create non-matching event
	event := &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "tool-completed",
		Data:      map[string]any{"tool": "Bash"},
	}

	// Route should not match
	matched := router.Route(event)
	if matched {
		t.Error("expected event to not be matched")
	}
}

func TestEventRouter_FilterMismatch(t *testing.T) {
	router := NewEventRouter(nil)

	// Register a waiter with specific filter
	ch := router.RegisterWaiter("tool-completed", map[string]string{"tool": "Read"}, 5*time.Second)

	// Create event with different tool
	event := &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "tool-completed",
		Data:      map[string]any{"tool": "Bash"},
	}

	// Route should not match
	matched := router.Route(event)
	if matched {
		t.Error("expected event to not be matched due to filter mismatch")
	}

	// Channel should not receive anything
	select {
	case <-ch:
		t.Error("should not have received event")
	case <-time.After(50 * time.Millisecond):
		// Expected
	}
}

func TestEventRouter_MultipleFilters(t *testing.T) {
	router := NewEventRouter(nil)

	// Register waiter with multiple filters
	ch := router.RegisterWaiter("tool-completed", map[string]string{
		"tool":   "Bash",
		"status": "success",
	}, 5*time.Second)

	// Event missing one filter key
	event1 := &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "tool-completed",
		Data:      map[string]any{"tool": "Bash"},
	}
	if router.Route(event1) {
		t.Error("should not match - missing 'status' in data")
	}

	// Event with all filters matching
	event2 := &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "tool-completed",
		Data: map[string]any{
			"tool":   "Bash",
			"status": "success",
		},
	}
	if !router.Route(event2) {
		t.Error("should match - all filters present")
	}

	// Verify delivery
	select {
	case <-ch:
		// Expected
	case <-time.After(50 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}

func TestEventRouter_FirstMatchWins(t *testing.T) {
	router := NewEventRouter(nil)

	// Register two waiters for same event type
	ch1 := router.RegisterWaiter("test-event", nil, 5*time.Second)
	ch2 := router.RegisterWaiter("test-event", nil, 5*time.Second)

	event := &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "test-event",
		Data:      map[string]any{},
	}

	// First route - should go to first waiter
	if !router.Route(event) {
		t.Error("should match first waiter")
	}

	// Check ch1 received it
	select {
	case <-ch1:
		// Expected
	case <-time.After(50 * time.Millisecond):
		t.Error("first waiter should receive event")
	}

	// Second route - should go to second waiter
	if !router.Route(event) {
		t.Error("should match second waiter")
	}

	// Check ch2 received it
	select {
	case <-ch2:
		// Expected
	case <-time.After(50 * time.Millisecond):
		t.Error("second waiter should receive event")
	}
}

func TestEventRouter_CleanupExpiredWaiters(t *testing.T) {
	router := NewEventRouter(nil)

	// Register waiter with very short timeout
	ch := router.RegisterWaiter("test-event", nil, 10*time.Millisecond)

	// Wait for expiration
	time.Sleep(50 * time.Millisecond)

	// Cleanup
	removed := router.Cleanup()
	if removed != 1 {
		t.Errorf("expected 1 waiter removed, got %d", removed)
	}

	// Channel should be closed
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed")
		}
	default:
		// Channel is closed and empty
	}

	// Route should not match
	event := &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "test-event",
	}
	if router.Route(event) {
		t.Error("should not match expired waiter")
	}
}

func TestEventRouter_WaiterCount(t *testing.T) {
	router := NewEventRouter(nil)

	if router.WaiterCount("") != 0 {
		t.Error("expected 0 total waiters initially")
	}

	router.RegisterWaiter("event-a", nil, 5*time.Second)
	router.RegisterWaiter("event-a", nil, 5*time.Second)
	router.RegisterWaiter("event-b", nil, 5*time.Second)

	if router.WaiterCount("event-a") != 2 {
		t.Errorf("expected 2 waiters for event-a, got %d", router.WaiterCount("event-a"))
	}

	if router.WaiterCount("event-b") != 1 {
		t.Errorf("expected 1 waiter for event-b, got %d", router.WaiterCount("event-b"))
	}

	if router.WaiterCount("") != 3 {
		t.Errorf("expected 3 total waiters, got %d", router.WaiterCount(""))
	}
}

func TestEventRouter_NoFilter(t *testing.T) {
	router := NewEventRouter(nil)

	// Register waiter with no filter - should match any event of the type
	ch := router.RegisterWaiter("catch-all", nil, 5*time.Second)

	event := &ipc.EventMessage{
		Type:      ipc.MsgEvent,
		EventType: "catch-all",
		Data: map[string]any{
			"any": "data",
			"goes": "here",
		},
	}

	if !router.Route(event) {
		t.Error("should match waiter with no filter")
	}

	select {
	case <-ch:
		// Expected
	case <-time.After(50 * time.Millisecond):
		t.Error("timeout waiting for event")
	}
}
