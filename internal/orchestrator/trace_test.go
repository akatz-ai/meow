package orchestrator

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestNewTracer(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "state")

	tracer, err := NewTracer(stateDir, "wf-001")
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Close()

	// Directory should be created
	if _, err := os.Stat(stateDir); os.IsNotExist(err) {
		t.Error("State directory not created")
	}

	// File should exist
	if _, err := os.Stat(tracer.Path()); os.IsNotExist(err) {
		t.Error("Trace file not created")
	}
}

func TestTracer_Log(t *testing.T) {
	dir := t.TempDir()
	tracer, err := NewTracer(dir, "wf-001")
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Close()

	entry := TraceEntry{
		Action:   TraceActionDispatch,
		BeadID:   "bd-123",
		BeadType: "task",
		Details:  map[string]any{"key": "value"},
	}

	if err := tracer.Log(entry); err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Read and verify
	entries := readTraceFile(t, tracer.Path())
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Action != TraceActionDispatch {
		t.Errorf("Action = %s, want dispatch", e.Action)
	}
	if e.BeadID != "bd-123" {
		t.Errorf("BeadID = %s, want bd-123", e.BeadID)
	}
	if e.WorkflowID != "wf-001" {
		t.Errorf("WorkflowID = %s, want wf-001", e.WorkflowID)
	}
	if e.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

func TestTracer_LogStart(t *testing.T) {
	dir := t.TempDir()
	tracer, err := NewTracer(dir, "wf-001")
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Close()

	if err := tracer.LogStart("my-template"); err != nil {
		t.Fatalf("LogStart failed: %v", err)
	}

	entries := readTraceFile(t, tracer.Path())
	if len(entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(entries))
	}

	if entries[0].Action != TraceActionStart {
		t.Errorf("Action = %s, want start", entries[0].Action)
	}
	if entries[0].Template != "my-template" {
		t.Errorf("Template = %s, want my-template", entries[0].Template)
	}
}

func TestTracer_LogResume(t *testing.T) {
	dir := t.TempDir()
	tracer, err := NewTracer(dir, "wf-001")
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Close()

	if err := tracer.LogResume(42); err != nil {
		t.Fatalf("LogResume failed: %v", err)
	}

	entries := readTraceFile(t, tracer.Path())
	if entries[0].Action != TraceActionResume {
		t.Errorf("Action = %s, want resume", entries[0].Action)
	}
	if entries[0].Details["tick_count"] != float64(42) {
		t.Errorf("tick_count = %v, want 42", entries[0].Details["tick_count"])
	}
}

func TestTracer_LogDispatch(t *testing.T) {
	dir := t.TempDir()
	tracer, err := NewTracer(dir, "wf-001")
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Close()

	if err := tracer.LogDispatch("bd-001", "task", map[string]any{"agent": "claude-1"}); err != nil {
		t.Fatalf("LogDispatch failed: %v", err)
	}

	entries := readTraceFile(t, tracer.Path())
	if entries[0].Action != TraceActionDispatch {
		t.Errorf("Action = %s, want dispatch", entries[0].Action)
	}
	if entries[0].BeadID != "bd-001" {
		t.Errorf("BeadID = %s, want bd-001", entries[0].BeadID)
	}
	if entries[0].BeadType != "task" {
		t.Errorf("BeadType = %s, want task", entries[0].BeadType)
	}
}

func TestTracer_LogConditionEval(t *testing.T) {
	dir := t.TempDir()
	tracer, err := NewTracer(dir, "wf-001")
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Close()

	if err := tracer.LogConditionEval("bd-cond", true, map[string]any{"shell": "test -f foo"}); err != nil {
		t.Fatalf("LogConditionEval failed: %v", err)
	}

	entries := readTraceFile(t, tracer.Path())
	if entries[0].Action != TraceActionConditionEval {
		t.Errorf("Action = %s, want condition_eval", entries[0].Action)
	}
	if entries[0].Details["result"] != true {
		t.Errorf("result = %v, want true", entries[0].Details["result"])
	}
}

func TestTracer_LogExpand(t *testing.T) {
	dir := t.TempDir()
	tracer, err := NewTracer(dir, "wf-001")
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Close()

	if err := tracer.LogExpand("bd-expand", "child-template", 5); err != nil {
		t.Fatalf("LogExpand failed: %v", err)
	}

	entries := readTraceFile(t, tracer.Path())
	if entries[0].Action != TraceActionExpand {
		t.Errorf("Action = %s, want expand", entries[0].Action)
	}
	if entries[0].Details["child_count"] != float64(5) {
		t.Errorf("child_count = %v, want 5", entries[0].Details["child_count"])
	}
}

func TestTracer_LogClose(t *testing.T) {
	dir := t.TempDir()
	tracer, err := NewTracer(dir, "wf-001")
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Close()

	outputs := map[string]any{"result": "success"}
	if err := tracer.LogClose("bd-001", "task", outputs); err != nil {
		t.Fatalf("LogClose failed: %v", err)
	}

	entries := readTraceFile(t, tracer.Path())
	if entries[0].Action != TraceActionClose {
		t.Errorf("Action = %s, want close", entries[0].Action)
	}
}

func TestTracer_LogError(t *testing.T) {
	dir := t.TempDir()
	tracer, err := NewTracer(dir, "wf-001")
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Close()

	if err := tracer.LogError("bd-001", errors.New("test error")); err != nil {
		t.Fatalf("LogError failed: %v", err)
	}

	entries := readTraceFile(t, tracer.Path())
	if entries[0].Action != TraceActionError {
		t.Errorf("Action = %s, want error", entries[0].Action)
	}
	if entries[0].Error != "test error" {
		t.Errorf("Error = %s, want 'test error'", entries[0].Error)
	}
}

func TestTracer_MultipleEntries(t *testing.T) {
	dir := t.TempDir()
	tracer, err := NewTracer(dir, "wf-001")
	if err != nil {
		t.Fatalf("NewTracer failed: %v", err)
	}
	defer tracer.Close()

	// Log multiple entries
	_ = tracer.LogStart("template")
	_ = tracer.LogDispatch("bd-1", "task", nil)
	_ = tracer.LogClose("bd-1", "task", nil)
	_ = tracer.LogShutdown("completed")

	entries := readTraceFile(t, tracer.Path())
	if len(entries) != 4 {
		t.Errorf("Expected 4 entries, got %d", len(entries))
	}

	// Check order
	actions := []TraceAction{TraceActionStart, TraceActionDispatch, TraceActionClose, TraceActionShutdown}
	for i, want := range actions {
		if entries[i].Action != want {
			t.Errorf("Entry %d action = %s, want %s", i, entries[i].Action, want)
		}
	}
}

func TestNullTracer(t *testing.T) {
	tracer := &NullTracer{}

	// All methods should succeed silently
	if err := tracer.LogStart("test"); err != nil {
		t.Errorf("LogStart failed: %v", err)
	}
	if err := tracer.LogDispatch("bd", "task", nil); err != nil {
		t.Errorf("LogDispatch failed: %v", err)
	}
	if err := tracer.LogClose("bd", "task", nil); err != nil {
		t.Errorf("LogClose failed: %v", err)
	}
	if err := tracer.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
	if tracer.Path() != "" {
		t.Errorf("Path should be empty")
	}
}

// Helper to read trace file
func readTraceFile(t *testing.T, path string) []TraceEntry {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("Failed to open trace file: %v", err)
	}
	defer file.Close()

	var entries []TraceEntry
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var entry TraceEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			t.Fatalf("Failed to unmarshal entry: %v", err)
		}
		entries = append(entries, entry)
	}

	if err := scanner.Err(); err != nil {
		t.Fatalf("Scanner error: %v", err)
	}

	return entries
}
