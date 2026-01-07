package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatePersister_SaveLoadState(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	persister := NewStatePersister(stateDir)

	state := &OrchestratorState{
		Version:      "1",
		WorkflowID:   "wf-001",
		TemplateName: "outer-loop",
		StartedAt:    time.Now().Truncate(time.Second),
		TickCount:    42,
		PID:          os.Getpid(),
	}

	// Save
	if err := persister.SaveState(state); err != nil {
		t.Fatalf("SaveState() error = %v", err)
	}

	// Verify file exists
	statePath := filepath.Join(stateDir, "orchestrator.json")
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		t.Error("State file not created")
	}

	// Load
	loaded, err := persister.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}

	if loaded.WorkflowID != state.WorkflowID {
		t.Errorf("WorkflowID = %s, want %s", loaded.WorkflowID, state.WorkflowID)
	}
	if loaded.TickCount != state.TickCount {
		t.Errorf("TickCount = %d, want %d", loaded.TickCount, state.TickCount)
	}
}

func TestStatePersister_LoadState_NonExistent(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	persister := NewStatePersister(stateDir)

	state, err := persister.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v, want nil", err)
	}
	if state != nil {
		t.Errorf("LoadState() = %v, want nil", state)
	}
}

func TestStatePersister_AcquireReleaseLock(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	persister := NewStatePersister(stateDir)

	// Acquire lock
	if err := persister.AcquireLock(); err != nil {
		t.Fatalf("AcquireLock() error = %v", err)
	}

	// Verify lock file exists
	lockPath := filepath.Join(stateDir, "orchestrator.lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("Lock file not created")
	}

	// Try to acquire again (should fail)
	persister2 := NewStatePersister(stateDir)
	if err := persister2.AcquireLock(); err == nil {
		t.Error("AcquireLock() should fail when lock already held")
		persister2.ReleaseLock()
	}

	// Release lock
	if err := persister.ReleaseLock(); err != nil {
		t.Fatalf("ReleaseLock() error = %v", err)
	}

	// Now should be able to acquire
	if err := persister2.AcquireLock(); err != nil {
		t.Fatalf("AcquireLock() after release error = %v", err)
	}
	persister2.ReleaseLock()
}

func TestStatePersister_UpdateHeartbeat(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	persister := NewStatePersister(stateDir)

	if err := persister.UpdateHeartbeat(); err != nil {
		t.Fatalf("UpdateHeartbeat() error = %v", err)
	}

	// Verify file exists
	heartbeatPath := filepath.Join(stateDir, "heartbeat.json")
	if _, err := os.Stat(heartbeatPath); os.IsNotExist(err) {
		t.Error("Heartbeat file not created")
	}

	// Check if stale (should not be stale immediately)
	stale, err := persister.CheckStaleHeartbeat(time.Minute)
	if err != nil {
		t.Fatalf("CheckStaleHeartbeat() error = %v", err)
	}
	if stale {
		t.Error("Heartbeat should not be stale immediately after update")
	}

	// Check with very short duration (should be stale)
	time.Sleep(10 * time.Millisecond)
	stale, err = persister.CheckStaleHeartbeat(time.Millisecond)
	if err != nil {
		t.Fatalf("CheckStaleHeartbeat() error = %v", err)
	}
	if !stale {
		t.Error("Heartbeat should be stale with 1ms threshold")
	}
}

func TestStatePersister_ClearState(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	persister := NewStatePersister(stateDir)

	// Save state
	state := &OrchestratorState{Version: "1", PID: os.Getpid()}
	if err := persister.SaveState(state); err != nil {
		t.Fatal(err)
	}

	// Clear
	if err := persister.ClearState(); err != nil {
		t.Fatalf("ClearState() error = %v", err)
	}

	// Verify file removed
	statePath := filepath.Join(stateDir, "orchestrator.json")
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Error("State file not removed")
	}

	// Clear again should not error
	if err := persister.ClearState(); err != nil {
		t.Fatalf("ClearState() on non-existent error = %v", err)
	}
}

func TestStatePersister_IsLockHeld(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	persister := NewStatePersister(stateDir)

	// No lock yet
	held, pid, err := persister.IsLockHeld()
	if err != nil {
		t.Fatalf("IsLockHeld() error = %v", err)
	}
	if held {
		t.Error("Lock should not be held initially")
	}

	// Acquire lock
	if err := persister.AcquireLock(); err != nil {
		t.Fatal(err)
	}

	// Check from another persister
	persister2 := NewStatePersister(stateDir)
	held, pid, err = persister2.IsLockHeld()
	if err != nil {
		t.Fatalf("IsLockHeld() error = %v", err)
	}
	if !held {
		t.Error("Lock should be held")
	}
	if pid != os.Getpid() {
		t.Errorf("PID = %d, want %d", pid, os.Getpid())
	}

	persister.ReleaseLock()
}

func TestStatePersister_AtomicWrite(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, ".meow", "state")

	persister := NewStatePersister(stateDir)

	// Write multiple times rapidly
	for i := 0; i < 10; i++ {
		state := &OrchestratorState{
			Version:   "1",
			TickCount: int64(i),
			PID:       os.Getpid(),
		}
		if err := persister.SaveState(state); err != nil {
			t.Fatalf("SaveState(%d) error = %v", i, err)
		}
	}

	// Verify final state
	loaded, err := persister.LoadState()
	if err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if loaded.TickCount != 9 {
		t.Errorf("TickCount = %d, want 9", loaded.TickCount)
	}

	// Verify no temp files left behind
	files, _ := os.ReadDir(stateDir)
	for _, f := range files {
		if filepath.Ext(f.Name()) == ".tmp" {
			t.Errorf("Temp file left behind: %s", f.Name())
		}
	}
}
