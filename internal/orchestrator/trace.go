package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// TraceAction represents the type of action being traced.
type TraceAction string

const (
	TraceActionStart        TraceAction = "start"         // Orchestrator start
	TraceActionBake         TraceAction = "bake"          // Template baked into beads
	TraceActionSpawn        TraceAction = "spawn"         // Agent spawned
	TraceActionDispatch     TraceAction = "dispatch"      // Bead dispatched
	TraceActionConditionEval TraceAction = "condition_eval" // Condition evaluated
	TraceActionExpand       TraceAction = "expand"        // Template expanded
	TraceActionClose        TraceAction = "close"         // Bead closed
	TraceActionStop         TraceAction = "stop"          // Agent stopped
	TraceActionShutdown     TraceAction = "shutdown"      // Orchestrator shutdown
	TraceActionError        TraceAction = "error"         // Error occurred
	TraceActionResume       TraceAction = "resume"        // Orchestrator resumed
)

// TraceEntry represents a single trace log entry.
type TraceEntry struct {
	Timestamp  time.Time      `json:"ts"`
	Action     TraceAction    `json:"action"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	BeadID     string         `json:"bead_id,omitempty"`
	BeadType   string         `json:"bead_type,omitempty"`
	AgentID    string         `json:"agent_id,omitempty"`
	Template   string         `json:"template,omitempty"`
	Details    map[string]any `json:"details,omitempty"`
	Error      string         `json:"error,omitempty"`
}

// TracerInterface defines the interface for execution tracers.
type TracerInterface interface {
	Log(entry TraceEntry) error
	LogStart(template string) error
	LogResume(tickCount int) error
	LogBake(template string, beadCount int) error
	LogSpawn(agentID string, details map[string]any) error
	LogDispatch(beadID, beadType string, details map[string]any) error
	LogConditionEval(beadID string, result bool, details map[string]any) error
	LogExpand(beadID, template string, childCount int) error
	LogClose(beadID, beadType string, outputs map[string]any) error
	LogStop(agentID string, graceful bool) error
	LogShutdown(reason string) error
	LogError(beadID string, err error) error
	Close() error
	Path() string
}

// Tracer logs execution traces to a JSONL file.
type Tracer struct {
	mu         sync.Mutex
	file       *os.File
	path       string
	workflowID string
}

// NewTracer creates a new tracer that writes to the given directory.
func NewTracer(stateDir, workflowID string) (*Tracer, error) {
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}

	path := filepath.Join(stateDir, "trace.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening trace file: %w", err)
	}

	return &Tracer{
		file:       file,
		path:       path,
		workflowID: workflowID,
	}, nil
}

// Close closes the trace file.
func (t *Tracer) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.file != nil {
		return t.file.Close()
	}
	return nil
}

// Path returns the trace file path.
func (t *Tracer) Path() string {
	return t.path
}

// Log writes a trace entry to the file.
func (t *Tracer) Log(entry TraceEntry) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	entry.Timestamp = time.Now()
	if entry.WorkflowID == "" {
		entry.WorkflowID = t.workflowID
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling trace entry: %w", err)
	}

	if _, err := t.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("writing trace entry: %w", err)
	}

	return nil
}

// LogStart traces orchestrator start.
func (t *Tracer) LogStart(template string) error {
	return t.Log(TraceEntry{
		Action:   TraceActionStart,
		Template: template,
		Details:  map[string]any{"resumed": false},
	})
}

// LogResume traces orchestrator resume from crash.
func (t *Tracer) LogResume(tickCount int) error {
	return t.Log(TraceEntry{
		Action:  TraceActionResume,
		Details: map[string]any{"tick_count": tickCount},
	})
}

// LogBake traces template baking.
func (t *Tracer) LogBake(template string, beadCount int) error {
	return t.Log(TraceEntry{
		Action:   TraceActionBake,
		Template: template,
		Details:  map[string]any{"bead_count": beadCount},
	})
}

// LogSpawn traces agent spawn.
func (t *Tracer) LogSpawn(agentID string, details map[string]any) error {
	return t.Log(TraceEntry{
		Action:  TraceActionSpawn,
		AgentID: agentID,
		Details: details,
	})
}

// LogDispatch traces bead dispatch.
func (t *Tracer) LogDispatch(beadID, beadType string, details map[string]any) error {
	return t.Log(TraceEntry{
		Action:   TraceActionDispatch,
		BeadID:   beadID,
		BeadType: beadType,
		Details:  details,
	})
}

// LogConditionEval traces condition evaluation.
func (t *Tracer) LogConditionEval(beadID string, result bool, details map[string]any) error {
	// Copy details to avoid mutating the input
	merged := make(map[string]any)
	for k, v := range details {
		merged[k] = v
	}
	merged["result"] = result
	return t.Log(TraceEntry{
		Action:   TraceActionConditionEval,
		BeadID:   beadID,
		BeadType: "condition",
		Details:  merged,
	})
}

// LogExpand traces template expansion.
func (t *Tracer) LogExpand(beadID, template string, childCount int) error {
	return t.Log(TraceEntry{
		Action:   TraceActionExpand,
		BeadID:   beadID,
		Template: template,
		Details:  map[string]any{"child_count": childCount},
	})
}

// LogClose traces bead close.
func (t *Tracer) LogClose(beadID, beadType string, outputs map[string]any) error {
	return t.Log(TraceEntry{
		Action:   TraceActionClose,
		BeadID:   beadID,
		BeadType: beadType,
		Details:  map[string]any{"outputs": outputs},
	})
}

// LogStop traces agent stop.
func (t *Tracer) LogStop(agentID string, graceful bool) error {
	return t.Log(TraceEntry{
		Action:  TraceActionStop,
		AgentID: agentID,
		Details: map[string]any{"graceful": graceful},
	})
}

// LogShutdown traces orchestrator shutdown.
func (t *Tracer) LogShutdown(reason string) error {
	return t.Log(TraceEntry{
		Action:  TraceActionShutdown,
		Details: map[string]any{"reason": reason},
	})
}

// LogError traces an error.
func (t *Tracer) LogError(beadID string, err error) error {
	return t.Log(TraceEntry{
		Action: TraceActionError,
		BeadID: beadID,
		Error:  err.Error(),
	})
}

// Compile-time interface checks
var (
	_ TracerInterface = (*Tracer)(nil)
	_ TracerInterface = (*NullTracer)(nil)
)

// NullTracer is a tracer that discards all entries.
type NullTracer struct{}

func (n *NullTracer) Log(_ TraceEntry) error               { return nil }
func (n *NullTracer) LogStart(_ string) error              { return nil }
func (n *NullTracer) LogResume(_ int) error                { return nil }
func (n *NullTracer) LogBake(_ string, _ int) error        { return nil }
func (n *NullTracer) LogSpawn(_ string, _ map[string]any) error { return nil }
func (n *NullTracer) LogDispatch(_ string, _ string, _ map[string]any) error { return nil }
func (n *NullTracer) LogConditionEval(_ string, _ bool, _ map[string]any) error { return nil }
func (n *NullTracer) LogExpand(_ string, _ string, _ int) error { return nil }
func (n *NullTracer) LogClose(_ string, _ string, _ map[string]any) error { return nil }
func (n *NullTracer) LogStop(_ string, _ bool) error       { return nil }
func (n *NullTracer) LogShutdown(_ string) error           { return nil }
func (n *NullTracer) LogError(_ string, _ error) error     { return nil }
func (n *NullTracer) Close() error                         { return nil }
func (n *NullTracer) Path() string                         { return "" }
