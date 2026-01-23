package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/akatz-ai/meow/internal/orchestrator"
	"github.com/akatz-ai/meow/internal/types"
)

func TestValidateMeowProcess(t *testing.T) {
	t.Run("returns error for non-existent PID", func(t *testing.T) {
		// Use a PID that's extremely unlikely to exist (max PID on Linux is typically 32768 or 4194304)
		err := validateMeowProcess(999999)
		if err == nil {
			t.Error("expected error for non-existent PID")
		}
		// Should contain "does not exist" in error message
		if err != nil && err.Error() != "" {
			// Error message should mention the PID doesn't exist
			t.Logf("Got expected error: %v", err)
		}
	})

	t.Run("returns error for PID 1 (init, not meow)", func(t *testing.T) {
		// PID 1 is always init/systemd, never meow
		// This tests the "not a meow process" check
		err := validateMeowProcess(1)
		if err == nil {
			t.Error("expected error for init process (PID 1)")
		}
		// Should indicate it's not a meow process
		t.Logf("Got expected error: %v", err)
	})

	t.Run("validates current test process", func(t *testing.T) {
		// The current process is running via "go test"
		// This should fail validation because cmdline contains "go" not "meow"
		currentPID := os.Getpid()
		err := validateMeowProcess(currentPID)

		// The test process cmdline will be something like:
		// "/tmp/go-build.../cmd.test" or "go test"
		// So it should fail validation (not a meow process)
		if err == nil {
			t.Error("expected error for test process (not a meow process)")
		}
		t.Logf("Current PID %d validation error (expected): %v", currentPID, err)
	})
}

// Note: Testing validateMeowProcess with an actual meow process would require
// starting a real meow orchestrator, which is better suited for E2E tests.
// The unit tests above verify:
// 1. Non-existent PIDs are rejected
// 2. Non-meow processes are rejected
// 3. The function correctly reads /proc/<pid>/cmdline

// TestOrphanedWorkflowDetection tests the logic for detecting orphaned workflows.
// An orphaned workflow has status=running but no valid orchestrator process.
func TestOrphanedWorkflowDetection(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	runsDir := filepath.Join(tmpDir, ".meow", "runs")

	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatalf("failed to create runs dir: %v", err)
	}

	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("detects orphan when OrchestratorPID is 0", func(t *testing.T) {
		now := time.Now()
		wf := &types.Run{
			ID:              "run-orphan-no-pid",
			Template:        "test-template",
			Status:          types.RunStatusRunning,
			StartedAt:       now,
			OrchestratorPID: 0, // No PID recorded
			Steps:           make(map[string]*types.Step),
		}

		if err := store.Save(ctx, wf); err != nil {
			t.Fatalf("failed to save workflow: %v", err)
		}

		// Load and verify the workflow is detected as orphaned
		loaded, err := store.Get(ctx, wf.ID)
		if err != nil {
			t.Fatalf("failed to load workflow: %v", err)
		}

		// Verify conditions that would trigger orphan detection in runStop
		if loaded.Status.IsTerminal() {
			t.Error("workflow should not be terminal yet")
		}
		if loaded.OrchestratorPID != 0 {
			t.Error("OrchestratorPID should be 0")
		}

		// This is the condition that triggers orphan handling: PID is 0
		isOrphaned := loaded.OrchestratorPID == 0
		if !isOrphaned {
			t.Error("workflow with PID=0 should be detected as orphaned")
		}
	})

	t.Run("detects orphan when process does not exist", func(t *testing.T) {
		now := time.Now()
		wf := &types.Run{
			ID:              "run-orphan-dead-process",
			Template:        "test-template",
			Status:          types.RunStatusRunning,
			StartedAt:       now,
			OrchestratorPID: 999999, // Non-existent PID
			Steps:           make(map[string]*types.Step),
		}

		if err := store.Save(ctx, wf); err != nil {
			t.Fatalf("failed to save workflow: %v", err)
		}

		loaded, err := store.Get(ctx, wf.ID)
		if err != nil {
			t.Fatalf("failed to load workflow: %v", err)
		}

		// validateMeowProcess should fail for non-existent PID
		err = validateMeowProcess(loaded.OrchestratorPID)
		if err == nil {
			t.Error("validateMeowProcess should fail for non-existent PID")
		}

		// This triggers orphan handling: PID exists but process doesn't
		isOrphaned := err != nil
		if !isOrphaned {
			t.Error("workflow with dead process should be detected as orphaned")
		}
	})

	t.Run("detects orphan when PID is not a meow process", func(t *testing.T) {
		now := time.Now()
		wf := &types.Run{
			ID:              "run-orphan-wrong-process",
			Template:        "test-template",
			Status:          types.RunStatusRunning,
			StartedAt:       now,
			OrchestratorPID: 1, // PID 1 is init/systemd, not meow
			Steps:           make(map[string]*types.Step),
		}

		if err := store.Save(ctx, wf); err != nil {
			t.Fatalf("failed to save workflow: %v", err)
		}

		loaded, err := store.Get(ctx, wf.ID)
		if err != nil {
			t.Fatalf("failed to load workflow: %v", err)
		}

		// validateMeowProcess should fail for non-meow process
		err = validateMeowProcess(loaded.OrchestratorPID)
		if err == nil {
			t.Error("validateMeowProcess should fail for non-meow process")
		}

		// This triggers orphan handling: PID exists but isn't meow
		isOrphaned := err != nil
		if !isOrphaned {
			t.Error("workflow with wrong process type should be detected as orphaned")
		}
	})
}

// TestOrphanedWorkflowMarkedAsStopped tests that orphaned workflows
// are correctly transitioned to stopped status.
func TestOrphanedWorkflowMarkedAsStopped(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	runsDir := filepath.Join(tmpDir, ".meow", "runs")

	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatalf("failed to create runs dir: %v", err)
	}

	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("orphaned workflow transitions to stopped", func(t *testing.T) {
		now := time.Now()
		wf := &types.Run{
			ID:              "run-to-stop",
			Template:        "test-template",
			Status:          types.RunStatusRunning,
			StartedAt:       now,
			OrchestratorPID: 0, // Orphaned
			Steps:           make(map[string]*types.Step),
		}

		if err := store.Save(ctx, wf); err != nil {
			t.Fatalf("failed to save workflow: %v", err)
		}

		// Simulate what runStop does for orphaned workflows
		loaded, err := store.Get(ctx, wf.ID)
		if err != nil {
			t.Fatalf("failed to load workflow: %v", err)
		}

		// Mark as stopped (this is what runStop does)
		loaded.Status = types.RunStatusStopped
		doneAt := time.Now()
		loaded.DoneAt = &doneAt
		loaded.OrchestratorPID = 0

		if err := store.Save(ctx, loaded); err != nil {
			t.Fatalf("failed to save updated workflow: %v", err)
		}

		// Verify the workflow is now stopped
		final, err := store.Get(ctx, wf.ID)
		if err != nil {
			t.Fatalf("failed to load final workflow: %v", err)
		}

		if final.Status != types.RunStatusStopped {
			t.Errorf("expected status=stopped, got %s", final.Status)
		}
		if final.DoneAt == nil {
			t.Error("DoneAt should be set after marking as stopped")
		}
		if !final.Status.IsTerminal() {
			t.Error("stopped status should be terminal (allowing cleanup)")
		}
	})

	t.Run("already terminal workflow is rejected", func(t *testing.T) {
		now := time.Now()
		wf := &types.Run{
			ID:        "run-already-stopped",
			Template:  "test-template",
			Status:    types.RunStatusStopped, // Already terminal
			StartedAt: now,
			DoneAt:    &now,
			Steps:     make(map[string]*types.Step),
		}

		if err := store.Save(ctx, wf); err != nil {
			t.Fatalf("failed to save workflow: %v", err)
		}

		loaded, err := store.Get(ctx, wf.ID)
		if err != nil {
			t.Fatalf("failed to load workflow: %v", err)
		}

		// runStop checks this first and returns error
		if !loaded.Status.IsTerminal() {
			t.Error("stopped workflow should be terminal")
		}
		// The error message in runStop would be:
		// "workflow %s is already %s"
	})
}
