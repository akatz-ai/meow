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
	TraceActionStart         TraceAction = "start"          // Orchestrator start
	TraceActionBake          TraceAction = "bake"           // Template baked into steps
	TraceActionSpawn         TraceAction = "spawn"          // Agent spawned
	TraceActionDispatch      TraceAction = "dispatch"       // Step dispatched
	TraceActionConditionEval TraceAction = "condition_eval" // Condition evaluated
	TraceActionExpand        TraceAction = "expand"         // Template expanded
	TraceActionClose         TraceAction = "close"          // Step closed
	TraceActionStop          TraceAction = "stop"           // Agent stopped
	TraceActionShutdown      TraceAction = "shutdown"       // Orchestrator shutdown
	TraceActionError         TraceAction = "error"          // Error occurred
	TraceActionResume        TraceAction = "resume"         // Orchestrator resumed
)

// TraceEntry represents a single trace log entry.
type TraceEntry struct {
	Timestamp  time.Time      `json:"ts"`
	Action     TraceAction    `json:"action"`
	WorkflowID string         `json:"workflow_id,omitempty"`
	StepID     string         `json:"step_id,omitempty"`
	StepType   string         `json:"step_type,omitempty"`
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
	LogBake(template string, stepCount int) error
	LogSpawn(agentID string, details map[string]any) error
	LogDispatch(stepID, stepType string, details map[string]any) error
	LogConditionEval(stepID string, result bool, details map[string]any) error
	LogExpand(stepID, template string, childCount int) error
	LogClose(stepID, stepType string, outputs map[string]any) error
	LogStop(agentID string, graceful bool) error
	LogShutdown(reason string) error
	LogError(stepID string, err error) error
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
func (t *Tracer) LogBake(template string, stepCount int) error {
	return t.Log(TraceEntry{
		Action:   TraceActionBake,
		Template: template,
		Details:  map[string]any{"step_count": stepCount},
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

// LogDispatch traces step dispatch.
func (t *Tracer) LogDispatch(stepID, stepType string, details map[string]any) error {
	return t.Log(TraceEntry{
		Action:   TraceActionDispatch,
		StepID:   stepID,
		StepType: stepType,
		Details:  details,
	})
}

// LogConditionEval traces condition evaluation.
func (t *Tracer) LogConditionEval(stepID string, result bool, details map[string]any) error {
	// Copy details to avoid mutating the input
	merged := make(map[string]any)
	for k, v := range details {
		merged[k] = v
	}
	merged["result"] = result
	return t.Log(TraceEntry{
		Action:   TraceActionConditionEval,
		StepID:   stepID,
		StepType: "condition",
		Details:  merged,
	})
}

// LogExpand traces template expansion.
func (t *Tracer) LogExpand(stepID, template string, childCount int) error {
	return t.Log(TraceEntry{
		Action:   TraceActionExpand,
		StepID:   stepID,
		Template: template,
		Details:  map[string]any{"child_count": childCount},
	})
}

// LogClose traces step close.
func (t *Tracer) LogClose(stepID, stepType string, outputs map[string]any) error {
	return t.Log(TraceEntry{
		Action:   TraceActionClose,
		StepID:   stepID,
		StepType: stepType,
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
func (t *Tracer) LogError(stepID string, err error) error {
	return t.Log(TraceEntry{
		Action: TraceActionError,
		StepID: stepID,
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

func (n *NullTracer) Log(_ TraceEntry) error                                    { return nil }
func (n *NullTracer) LogStart(_ string) error                                   { return nil }
func (n *NullTracer) LogResume(_ int) error                                     { return nil }
func (n *NullTracer) LogBake(_ string, _ int) error                             { return nil }
func (n *NullTracer) LogSpawn(_ string, _ map[string]any) error                 { return nil }
func (n *NullTracer) LogDispatch(_ string, _ string, _ map[string]any) error    { return nil }
func (n *NullTracer) LogConditionEval(_ string, _ bool, _ map[string]any) error { return nil }
func (n *NullTracer) LogExpand(_ string, _ string, _ int) error                 { return nil }
func (n *NullTracer) LogClose(_ string, _ string, _ map[string]any) error       { return nil }
func (n *NullTracer) LogStop(_ string, _ bool) error                            { return nil }
func (n *NullTracer) LogShutdown(_ string) error                                { return nil }
func (n *NullTracer) LogError(_ string, _ error) error                          { return nil }
func (n *NullTracer) Close() error                                              { return nil }
func (n *NullTracer) Path() string                                              { return "" }
