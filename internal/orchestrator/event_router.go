// Package orchestrator provides the MEOW workflow orchestration engine.
package orchestrator

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/akatz-ai/meow/internal/ipc"
)

// EventRouter routes events to registered waiters.
// Events are fire-and-forget; if no waiter matches, the event is logged but not queued.
type EventRouter struct {
	mu      sync.Mutex
	waiters map[string][]*eventWaiter // event_type -> waiters
	logger  *slog.Logger
}

// eventWaiter represents a waiting await-event request.
type eventWaiter struct {
	eventType string
	filter    map[string]string
	response  chan *ipc.EventMessage
	deadline  time.Time
}

// NewEventRouter creates a new event router.
func NewEventRouter(logger *slog.Logger) *EventRouter {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventRouter{
		waiters: make(map[string][]*eventWaiter),
		logger:  logger.With("component", "event-router"),
	}
}

// RegisterWaiter registers a waiter for events of the given type.
// Returns a channel that will receive the matching event or be closed on timeout/cancellation.
func (r *EventRouter) RegisterWaiter(eventType string, filter map[string]string, timeout time.Duration) <-chan *ipc.EventMessage {
	r.mu.Lock()
	defer r.mu.Unlock()

	ch := make(chan *ipc.EventMessage, 1)
	waiter := &eventWaiter{
		eventType: eventType,
		filter:    filter,
		response:  ch,
		deadline:  time.Now().Add(timeout),
	}

	r.waiters[eventType] = append(r.waiters[eventType], waiter)
	r.logger.Debug("waiter registered",
		"event_type", eventType,
		"filter", filter,
		"timeout", timeout,
		"deadline", waiter.deadline,
	)

	return ch
}

// Route routes an event to matching waiters.
// Returns true if the event was delivered to a waiter, false otherwise.
// Uses first-match-wins semantics.
func (r *EventRouter) Route(event *ipc.EventMessage) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	waiters, ok := r.waiters[event.EventType]
	if !ok || len(waiters) == 0 {
		r.logger.Debug("no waiters for event", "event_type", event.EventType)
		return false
	}

	// Find first matching waiter
	for i, waiter := range waiters {
		if r.matchesFilter(event, waiter.filter) {
			// Check if waiter hasn't expired
			if time.Now().After(waiter.deadline) {
				r.logger.Debug("waiter expired", "event_type", event.EventType)
				continue
			}

			// Deliver event to waiter
			select {
			case waiter.response <- event:
				r.logger.Debug("event delivered to waiter",
					"event_type", event.EventType,
					"filter", waiter.filter,
				)
			default:
				// Channel full or closed, skip
				r.logger.Warn("failed to deliver event to waiter", "event_type", event.EventType)
				continue
			}

			// Remove the matched waiter from the list
			r.waiters[event.EventType] = append(waiters[:i], waiters[i+1:]...)
			if len(r.waiters[event.EventType]) == 0 {
				delete(r.waiters, event.EventType)
			}

			return true
		}
	}

	r.logger.Debug("no matching waiter for event", "event_type", event.EventType)
	return false
}

// matchesFilter checks if an event matches all the filter criteria.
// All filter key-value pairs must match the event data exactly.
// Special keys "agent" and "workflow" are matched against the event's
// Agent and Workflow fields respectively.
func (r *EventRouter) matchesFilter(event *ipc.EventMessage, filter map[string]string) bool {
	if len(filter) == 0 {
		return true
	}

	for key, expectedValue := range filter {
		var actualStr string

		// Handle special fields that are on the event struct, not in Data
		switch key {
		case "agent":
			actualStr = event.Agent
		case "workflow":
			actualStr = event.Workflow
		default:
			actualValue, ok := event.Data[key]
			if !ok {
				return false
			}
			// Convert actual value to string for comparison
			actualStr = fmt.Sprintf("%v", actualValue)
		}

		if actualStr != expectedValue {
			return false
		}
	}

	return true
}

// Cleanup removes expired waiters.
// This should be called periodically.
func (r *EventRouter) Cleanup() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	removed := 0

	for eventType, waiters := range r.waiters {
		var active []*eventWaiter
		for _, waiter := range waiters {
			if now.Before(waiter.deadline) {
				active = append(active, waiter)
			} else {
				// Close the channel to signal timeout
				close(waiter.response)
				removed++
				r.logger.Debug("cleaned up expired waiter",
					"event_type", eventType,
					"filter", waiter.filter,
				)
			}
		}

		if len(active) > 0 {
			r.waiters[eventType] = active
		} else {
			delete(r.waiters, eventType)
		}
	}

	if removed > 0 {
		r.logger.Debug("cleaned up expired waiters", "count", removed)
	}

	return removed
}

// StartCleanupLoop starts a background goroutine that periodically cleans up expired waiters.
// Returns a function to stop the cleanup loop.
func (r *EventRouter) StartCleanupLoop(interval time.Duration) func() {
	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				r.Cleanup()
			case <-done:
				return
			}
		}
	}()

	return func() {
		close(done)
	}
}

// WaiterCount returns the number of active waiters for a given event type.
// If eventType is empty, returns total count across all types.
func (r *EventRouter) WaiterCount(eventType string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	if eventType != "" {
		return len(r.waiters[eventType])
	}

	total := 0
	for _, waiters := range r.waiters {
		total += len(waiters)
	}
	return total
}
