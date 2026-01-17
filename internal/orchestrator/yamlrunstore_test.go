package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

func TestYAMLRunStore(t *testing.T) {
	// Create temp directory for tests
	dir := t.TempDir()
	ctx := context.Background()

	store, err := NewYAMLRunStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("Create and Get", func(t *testing.T) {
		wf := types.NewRun("run-test-1", "test.meow.toml", map[string]any{"key": "value"})
		wf.AddStep(&types.Step{
			ID:       "step1",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusPending,
			Shell:    &types.ShellConfig{Command: "echo hello"},
		})

		if err := store.Create(ctx, wf); err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		// Get it back
		retrieved, err := store.Get(ctx, "run-test-1")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if retrieved.ID != wf.ID {
			t.Errorf("ID mismatch: got %s, want %s", retrieved.ID, wf.ID)
		}
		if retrieved.Template != wf.Template {
			t.Errorf("Template mismatch: got %s, want %s", retrieved.Template, wf.Template)
		}
		if retrieved.Variables["key"] != "value" {
			t.Error("Variables not preserved")
		}
		if len(retrieved.Steps) != 1 {
			t.Errorf("Steps count mismatch: got %d, want 1", len(retrieved.Steps))
		}
	})

	t.Run("Create duplicate fails", func(t *testing.T) {
		wf := types.NewRun("run-dup", "test.meow.toml", nil)
		if err := store.Create(ctx, wf); err != nil {
			t.Fatalf("first Create failed: %v", err)
		}
		if err := store.Create(ctx, wf); err == nil {
			t.Error("second Create should fail")
		}
	})

	t.Run("Get nonexistent fails", func(t *testing.T) {
		_, err := store.Get(ctx, "nonexistent")
		if err == nil {
			t.Error("Get should fail for nonexistent workflow")
		}
	})

	t.Run("Save updates workflow", func(t *testing.T) {
		wf := types.NewRun("run-save", "test.meow.toml", nil)
		store.Create(ctx, wf)

		wf.Status = types.RunStatusRunning
		if err := store.Save(ctx, wf); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		retrieved, _ := store.Get(ctx, "run-save")
		if retrieved.Status != types.RunStatusRunning {
			t.Errorf("Status not updated: got %s", retrieved.Status)
		}
	})

	t.Run("Delete removes workflow", func(t *testing.T) {
		wf := types.NewRun("run-delete", "test.meow.toml", nil)
		store.Create(ctx, wf)

		if err := store.Delete(ctx, "run-delete"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err := store.Get(ctx, "run-delete")
		if err == nil {
			t.Error("Get should fail after Delete")
		}
	})

	t.Run("Delete nonexistent fails", func(t *testing.T) {
		if err := store.Delete(ctx, "nonexistent"); err == nil {
			t.Error("Delete should fail for nonexistent workflow")
		}
	})

	t.Run("List all workflows", func(t *testing.T) {
		// Clear previous
		store2, _ := NewYAMLRunStore(t.TempDir())
		defer store2.Close()

		store2.Create(ctx, types.NewRun("run-list-1", "test.meow.toml", nil))
		store2.Create(ctx, types.NewRun("run-list-2", "test.meow.toml", nil))

		all, err := store2.List(ctx, RunFilter{})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(all) != 2 {
			t.Errorf("expected 2 workflows, got %d", len(all))
		}
	})

	t.Run("List with status filter", func(t *testing.T) {
		store3, _ := NewYAMLRunStore(t.TempDir())
		defer store3.Close()

		wf1 := types.NewRun("run-pending", "test.meow.toml", nil)
		wf2 := types.NewRun("run-running", "test.meow.toml", nil)
		wf2.Status = types.RunStatusRunning

		store3.Create(ctx, wf1)
		store3.Create(ctx, wf2)

		running, _ := store3.List(ctx, RunFilter{Status: types.RunStatusRunning})
		if len(running) != 1 {
			t.Errorf("expected 1 running workflow, got %d", len(running))
		}
		if running[0].ID != "run-running" {
			t.Errorf("wrong workflow returned: %s", running[0].ID)
		}
	})

	t.Run("GetByAgent", func(t *testing.T) {
		store4, _ := NewYAMLRunStore(t.TempDir())
		defer store4.Close()

		wf := types.NewRun("run-agent", "test.meow.toml", nil)
		wf.Steps["s1"] = &types.Step{
			ID:       "s1",
			Executor: types.ExecutorAgent,
			Agent:    &types.AgentConfig{Agent: "claude-1", Prompt: "test"},
		}
		store4.Create(ctx, wf)

		wf2 := types.NewRun("run-other", "test.meow.toml", nil)
		wf2.Steps["s2"] = &types.Step{
			ID:       "s2",
			Executor: types.ExecutorAgent,
			Agent:    &types.AgentConfig{Agent: "claude-2", Prompt: "test"},
		}
		store4.Create(ctx, wf2)

		results, _ := store4.GetByAgent(ctx, "claude-1")
		if len(results) != 1 {
			t.Errorf("expected 1 workflow for claude-1, got %d", len(results))
		}
		if results[0].ID != "run-agent" {
			t.Errorf("wrong workflow: %s", results[0].ID)
		}
	})
}

func TestYAMLRunStoreLocking(t *testing.T) {
	dir := t.TempDir()

	// Multiple stores can be created on the same directory (no directory-level lock)
	store1, err := NewYAMLRunStore(dir)
	if err != nil {
		t.Fatalf("first store failed: %v", err)
	}
	defer store1.Close()

	store2, err := NewYAMLRunStore(dir)
	if err != nil {
		t.Fatalf("second store should succeed (per-workflow locking): %v", err)
	}
	defer store2.Close()

	t.Run("same workflow lock conflicts", func(t *testing.T) {
		// First lock on workflow-1
		lock1, err := store1.AcquireWorkflowLock("workflow-1")
		if err != nil {
			t.Fatalf("first lock failed: %v", err)
		}
		defer lock1.Release()

		// Second lock on same workflow should fail
		_, err = store2.AcquireWorkflowLock("workflow-1")
		if err == nil {
			t.Error("second lock on same workflow should fail")
		}

		// Release first lock
		lock1.Release()

		// Now second lock should succeed
		lock3, err := store2.AcquireWorkflowLock("workflow-1")
		if err != nil {
			t.Errorf("lock after release should succeed: %v", err)
		}
		if lock3 != nil {
			lock3.Release()
		}
	})

	t.Run("different workflows can run concurrently", func(t *testing.T) {
		// Lock on workflow-a
		lockA, err := store1.AcquireWorkflowLock("workflow-a")
		if err != nil {
			t.Fatalf("lock on workflow-a failed: %v", err)
		}
		defer lockA.Release()

		// Lock on workflow-b should succeed (different workflow)
		lockB, err := store2.AcquireWorkflowLock("workflow-b")
		if err != nil {
			t.Fatalf("lock on workflow-b should succeed: %v", err)
		}
		defer lockB.Release()

		// Both workflows can be "running" concurrently
		// This is the key test for parallel workflow execution
	})

	t.Run("lock file cleanup on release", func(t *testing.T) {
		lock, err := store1.AcquireWorkflowLock("cleanup-test")
		if err != nil {
			t.Fatalf("lock failed: %v", err)
		}

		lockPath := filepath.Join(dir, "cleanup-test.yaml.lock")
		if _, err := os.Stat(lockPath); os.IsNotExist(err) {
			t.Error("lock file should exist while locked")
		}

		lock.Release()

		if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
			t.Error("lock file should be cleaned up after release")
		}
	})

	t.Run("IsLocked detects active locks", func(t *testing.T) {
		workflowID := "locked-workflow"

		// Before acquiring lock, should not be locked
		if store1.IsLocked(workflowID) {
			t.Error("workflow should not be locked initially")
		}

		// Acquire lock
		lock, err := store1.AcquireWorkflowLock(workflowID)
		if err != nil {
			t.Fatalf("failed to acquire lock: %v", err)
		}
		defer lock.Release()

		// While locked, should return true
		if !store1.IsLocked(workflowID) {
			t.Error("workflow should be locked while lock is held")
		}

		// Another store should also detect the lock
		if !store2.IsLocked(workflowID) {
			t.Error("another store should also detect the lock")
		}

		// Release the lock
		lock.Release()

		// After release, should not be locked
		if store1.IsLocked(workflowID) {
			t.Error("workflow should not be locked after release")
		}
	})

	t.Run("IsLocked returns false for nonexistent lock file", func(t *testing.T) {
		if store1.IsLocked("never-locked-workflow") {
			t.Error("workflow with no lock file should not be locked")
		}
	})
}

func TestYAMLRunStoreCrashRecovery(t *testing.T) {
	dir := t.TempDir()

	// Simulate crash: write .tmp file, no main file
	tmpPath := filepath.Join(dir, "run-crash.yaml.tmp")
	content := []byte("id: wf-crash\ntemplate: test.meow.toml\nstatus: pending\n")
	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}

	// Create store - should recover
	store, err := NewYAMLRunStore(dir)
	if err != nil {
		t.Fatalf("store creation should recover: %v", err)
	}
	defer store.Close()

	// Check .tmp was promoted to main
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("tmp file should have been removed")
	}

	mainPath := filepath.Join(dir, "run-crash.yaml")
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		t.Error("main file should have been created from tmp")
	}
}

func TestYAMLRunStoreOrphanTmpCleanup(t *testing.T) {
	dir := t.TempDir()

	// Create main file
	mainPath := filepath.Join(dir, "run-orphan.yaml")
	content := []byte("id: wf-orphan\ntemplate: test.meow.toml\nstatus: pending\n")
	if err := os.WriteFile(mainPath, content, 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	// Create orphan tmp (main exists, so tmp is stale)
	tmpPath := filepath.Join(dir, "run-orphan.yaml.tmp")
	if err := os.WriteFile(tmpPath, []byte("stale"), 0644); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}

	// Create store - should clean up orphan
	store, err := NewYAMLRunStore(dir)
	if err != nil {
		t.Fatalf("store creation failed: %v", err)
	}
	defer store.Close()

	// Orphan tmp should be deleted
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("orphan tmp should have been deleted")
	}

	// Main should still exist
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		t.Error("main file should still exist")
	}
}

func TestYAMLRunStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store, err := NewYAMLRunStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	wf := types.NewRun("run-atomic", "test.meow.toml", nil)
	if err := store.Create(ctx, wf); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify no .tmp file left behind
	tmpPath := filepath.Join(dir, "run-atomic.yaml.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after save")
	}

	// Verify main file exists
	mainPath := filepath.Join(dir, "run-atomic.yaml")
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		t.Error("main file should exist")
	}
}
