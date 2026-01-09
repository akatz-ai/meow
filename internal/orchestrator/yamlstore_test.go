package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

func TestYAMLWorkflowStore(t *testing.T) {
	// Create temp directory for tests
	dir := t.TempDir()
	ctx := context.Background()

	store, err := NewYAMLWorkflowStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("Create and Get", func(t *testing.T) {
		wf := types.NewWorkflow("wf-test-1", "test.meow.toml", map[string]string{"key": "value"})
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
		retrieved, err := store.Get(ctx, "wf-test-1")
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
		wf := types.NewWorkflow("wf-dup", "test.meow.toml", nil)
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
		wf := types.NewWorkflow("wf-save", "test.meow.toml", nil)
		store.Create(ctx, wf)

		wf.Status = types.WorkflowStatusRunning
		if err := store.Save(ctx, wf); err != nil {
			t.Fatalf("Save failed: %v", err)
		}

		retrieved, _ := store.Get(ctx, "wf-save")
		if retrieved.Status != types.WorkflowStatusRunning {
			t.Errorf("Status not updated: got %s", retrieved.Status)
		}
	})

	t.Run("Delete removes workflow", func(t *testing.T) {
		wf := types.NewWorkflow("wf-delete", "test.meow.toml", nil)
		store.Create(ctx, wf)

		if err := store.Delete(ctx, "wf-delete"); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		_, err := store.Get(ctx, "wf-delete")
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
		store2, _ := NewYAMLWorkflowStore(t.TempDir())
		defer store2.Close()

		store2.Create(ctx, types.NewWorkflow("wf-list-1", "test.meow.toml", nil))
		store2.Create(ctx, types.NewWorkflow("wf-list-2", "test.meow.toml", nil))

		all, err := store2.List(ctx, WorkflowFilter{})
		if err != nil {
			t.Fatalf("List failed: %v", err)
		}
		if len(all) != 2 {
			t.Errorf("expected 2 workflows, got %d", len(all))
		}
	})

	t.Run("List with status filter", func(t *testing.T) {
		store3, _ := NewYAMLWorkflowStore(t.TempDir())
		defer store3.Close()

		wf1 := types.NewWorkflow("wf-pending", "test.meow.toml", nil)
		wf2 := types.NewWorkflow("wf-running", "test.meow.toml", nil)
		wf2.Status = types.WorkflowStatusRunning

		store3.Create(ctx, wf1)
		store3.Create(ctx, wf2)

		running, _ := store3.List(ctx, WorkflowFilter{Status: types.WorkflowStatusRunning})
		if len(running) != 1 {
			t.Errorf("expected 1 running workflow, got %d", len(running))
		}
		if running[0].ID != "wf-running" {
			t.Errorf("wrong workflow returned: %s", running[0].ID)
		}
	})

	t.Run("GetByAgent", func(t *testing.T) {
		store4, _ := NewYAMLWorkflowStore(t.TempDir())
		defer store4.Close()

		wf := types.NewWorkflow("wf-agent", "test.meow.toml", nil)
		wf.Steps["s1"] = &types.Step{
			ID:       "s1",
			Executor: types.ExecutorAgent,
			Agent:    &types.AgentConfig{Agent: "claude-1", Prompt: "test"},
		}
		store4.Create(ctx, wf)

		wf2 := types.NewWorkflow("wf-other", "test.meow.toml", nil)
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
		if results[0].ID != "wf-agent" {
			t.Errorf("wrong workflow: %s", results[0].ID)
		}
	})
}

func TestYAMLWorkflowStoreLocking(t *testing.T) {
	dir := t.TempDir()

	store1, err := NewYAMLWorkflowStore(dir)
	if err != nil {
		t.Fatalf("first store failed: %v", err)
	}
	defer store1.Close()

	// Second store should fail to acquire lock
	_, err = NewYAMLWorkflowStore(dir)
	if err == nil {
		t.Error("second store should fail due to lock")
	}

	// Close first store, second should work now
	store1.Close()

	store2, err := NewYAMLWorkflowStore(dir)
	if err != nil {
		t.Errorf("store after close should work: %v", err)
	}
	if store2 != nil {
		store2.Close()
	}
}

func TestYAMLWorkflowStoreCrashRecovery(t *testing.T) {
	dir := t.TempDir()

	// Simulate crash: write .tmp file, no main file
	tmpPath := filepath.Join(dir, "wf-crash.yaml.tmp")
	content := []byte("id: wf-crash\ntemplate: test.meow.toml\nstatus: pending\n")
	if err := os.WriteFile(tmpPath, content, 0644); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}

	// Create store - should recover
	store, err := NewYAMLWorkflowStore(dir)
	if err != nil {
		t.Fatalf("store creation should recover: %v", err)
	}
	defer store.Close()

	// Check .tmp was promoted to main
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("tmp file should have been removed")
	}

	mainPath := filepath.Join(dir, "wf-crash.yaml")
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		t.Error("main file should have been created from tmp")
	}
}

func TestYAMLWorkflowStoreOrphanTmpCleanup(t *testing.T) {
	dir := t.TempDir()

	// Create main file
	mainPath := filepath.Join(dir, "wf-orphan.yaml")
	content := []byte("id: wf-orphan\ntemplate: test.meow.toml\nstatus: pending\n")
	if err := os.WriteFile(mainPath, content, 0644); err != nil {
		t.Fatalf("failed to write main file: %v", err)
	}

	// Create orphan tmp (main exists, so tmp is stale)
	tmpPath := filepath.Join(dir, "wf-orphan.yaml.tmp")
	if err := os.WriteFile(tmpPath, []byte("stale"), 0644); err != nil {
		t.Fatalf("failed to write tmp file: %v", err)
	}

	// Create store - should clean up orphan
	store, err := NewYAMLWorkflowStore(dir)
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

func TestYAMLWorkflowStoreAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store, err := NewYAMLWorkflowStore(dir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	wf := types.NewWorkflow("wf-atomic", "test.meow.toml", nil)
	if err := store.Create(ctx, wf); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify no .tmp file left behind
	tmpPath := filepath.Join(dir, "wf-atomic.yaml.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after save")
	}

	// Verify main file exists
	mainPath := filepath.Join(dir, "wf-atomic.yaml")
	if _, err := os.Stat(mainPath); os.IsNotExist(err) {
		t.Error("main file should exist")
	}
}
