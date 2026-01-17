package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
)

func TestStatusExitError(t *testing.T) {
	t.Run("Error method returns message", func(t *testing.T) {
		err := &StatusExitError{Code: ExitNoWorkflows, Message: "no active workflows"}
		if err.Error() != "no active workflows" {
			t.Errorf("expected 'no active workflows', got %q", err.Error())
		}
	})

	t.Run("exit codes are distinct", func(t *testing.T) {
		// Ensure exit codes don't overlap
		if ExitSuccess == ExitNoWorkflows {
			t.Error("ExitSuccess and ExitNoWorkflows should be different")
		}
		if ExitNoWorkflows == ExitWorkflowNotFound {
			t.Error("ExitNoWorkflows and ExitWorkflowNotFound should be different")
		}
		if ExitSuccess == ExitWorkflowNotFound {
			t.Error("ExitSuccess and ExitWorkflowNotFound should be different")
		}
	})
}

func TestStatusStrictFlag(t *testing.T) {
	// The --strict flag should be registered on statusCmd
	flag := statusCmd.Flags().Lookup("strict")
	if flag == nil {
		t.Fatal("--strict flag not found on status command")
	}

	if flag.DefValue != "false" {
		t.Errorf("--strict default should be false, got %s", flag.DefValue)
	}

	if flag.Usage == "" {
		t.Error("--strict flag should have usage text")
	}
}

// Tests for meow-lsfn: Add orphaned run detection to meow status

// TestOrphanedRunDetection verifies that the status command can detect orphaned runs.
// An orphaned run is one where status=running but no lock is held (orchestrator crashed).
func TestOrphanedRunDetection(t *testing.T) {
	// Create a temp directory for the run store
	tmpDir := t.TempDir()
	runsDir := filepath.Join(tmpDir, ".meow", "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create a run with status "running" but no lock (simulating an orphaned state)
	orphanedRun := types.NewRun("orphaned-run-1", "test-template.meow.toml", nil)
	orphanedRun.Status = types.RunStatusRunning
	orphanedRun.StartedAt = time.Now().Add(-10 * time.Minute)

	if err := store.Create(ctx, orphanedRun); err != nil {
		t.Fatal(err)
	}

	// Verify the run is NOT locked (no orchestrator holding it)
	if store.IsLocked(orphanedRun.ID) {
		t.Fatal("expected orphaned run to NOT be locked")
	}

	// List all runs with filter for running status
	filter := orchestrator.RunFilter{Status: types.RunStatusRunning}
	runs, err := store.List(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}

	// Should find our orphaned run
	if len(runs) != 1 {
		t.Fatalf("expected 1 running run, got %d", len(runs))
	}

	// The run is running but NOT locked - this is an orphaned state
	isOrphaned := runs[0].Status == types.RunStatusRunning && !store.IsLocked(runs[0].ID)
	if !isOrphaned {
		t.Error("run should be detected as orphaned (running but not locked)")
	}

	// Test that GetOrphanedRuns function exists on the store or as a helper
	// This function should be added as part of meow-lsfn implementation
	// Currently this will fail because the function doesn't exist
	orphanedRuns := GetOrphanedRuns(ctx, store)
	if len(orphanedRuns) != 1 {
		t.Errorf("GetOrphanedRuns should return 1 orphaned run, got %d", len(orphanedRuns))
	}
}

// GetOrphanedRuns returns runs that are in "running" status but have no lock held.
// This needs to be moved to the actual implementation in status.go or a helper package.
// The test will fail because this placeholder returns empty slice.
func GetOrphanedRuns(ctx context.Context, store *orchestrator.YAMLRunStore) []*types.Run {
	// TODO: Implement in meow-lsfn
	// 1. List all runs
	// 2. Filter to status=running
	// 3. Filter to !IsLocked
	_ = ctx
	_ = store
	return nil // Fails the test
}

// TestOrphanedRunsInStatusOutput verifies that orphaned runs appear in status output.
func TestOrphanedRunsInStatusOutput(t *testing.T) {
	t.Run("orphaned runs should be shown prominently", func(t *testing.T) {
		// After meow-lsfn implementation:
		// - When there are orphaned runs, they should appear BEFORE active runs
		// - They should have a warning indicator (⚠)
		// - They should include actionable guidance (resume or stop)

		// This test documents the expected behavior
		// Currently fails because feature is not implemented
		t.Error("meow-lsfn: orphaned runs should be shown with warning indicator in status output")
	})
}

// TestOrphanedRunGuidance verifies that status shows actionable guidance for orphaned runs.
func TestOrphanedRunGuidance(t *testing.T) {
	t.Run("status should show resume and stop commands for orphaned runs", func(t *testing.T) {
		// Expected output should include:
		// "Run meow resume <id> to recover, or meow stop <id> to clean up."

		// This test documents the expected guidance
		t.Error("meow-lsfn: status should show guidance for orphaned runs (resume or stop)")
	})
}

// TestOrphanedRunDetailView verifies that viewing a specific orphaned run shows its status clearly.
func TestOrphanedRunDetailView(t *testing.T) {
	// Create a temp directory for the run store
	tmpDir := t.TempDir()
	runsDir := filepath.Join(tmpDir, ".meow", "runs")
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	// Create an orphaned run
	orphanedRun := types.NewRun("orphan-detail-test", "test.meow.toml", nil)
	orphanedRun.Status = types.RunStatusRunning
	orphanedRun.StartedAt = time.Now().Add(-5 * time.Minute)

	if err := store.Create(ctx, orphanedRun); err != nil {
		t.Fatal(err)
	}

	// When viewing this specific run's detail, the output should indicate
	// that it's orphaned (running but no orchestrator)
	t.Run("detail view indicates orphaned status", func(t *testing.T) {
		// After meow-lsfn implementation, displayWorkflowDetail should
		// check if the run is orphaned and add a warning to the output

		// Check if the run is orphaned
		isOrphaned := orphanedRun.Status == types.RunStatusRunning && !store.IsLocked(orphanedRun.ID)
		if !isOrphaned {
			t.Fatal("run should be orphaned for this test")
		}

		// The detail view should show the orphaned state
		// This test fails because the feature isn't implemented
		t.Error("meow-lsfn: detail view should indicate when a run is orphaned")
	})
}
