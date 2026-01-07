package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

func TestFileBeadStore_Load(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write test data
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-001","type":"task","title":"Task 1","status":"open","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-002","type":"code","title":"Code 1","status":"closed","created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify beads loaded
	bead1, err := store.Get(ctx, "bd-001")
	if err != nil {
		t.Fatalf("Get(bd-001) error = %v", err)
	}
	if bead1 == nil || bead1.Title != "Task 1" {
		t.Errorf("Get(bd-001) = %v, want Task 1", bead1)
	}

	bead2, err := store.Get(ctx, "bd-002")
	if err != nil {
		t.Fatalf("Get(bd-002) error = %v", err)
	}
	if bead2 == nil || bead2.Status != types.BeadStatusClosed {
		t.Errorf("Get(bd-002) status = %v, want closed", bead2)
	}
}

func TestFileBeadStore_Load_NonExistent(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	// Should not error for non-existent directory
	if err := store.Load(ctx); err != nil {
		t.Errorf("Load() error = %v, want nil", err)
	}
}

func TestFileBeadStore_GetNextReady(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write test data with dependencies
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-dep","type":"code","title":"Dependency","status":"closed","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-blocked","type":"task","title":"Blocked","status":"open","needs":["bd-notexist"],"created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-ready","type":"task","title":"Ready","status":"open","needs":["bd-dep"],"created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-inprogress","type":"task","title":"In Progress","status":"in_progress","created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	ready, err := store.GetNextReady(ctx)
	if err != nil {
		t.Fatalf("GetNextReady() error = %v", err)
	}

	if ready == nil || ready.ID != "bd-ready" {
		t.Errorf("GetNextReady() = %v, want bd-ready", ready)
	}
}

func TestFileBeadStore_GetNextReady_Priority(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write beads of different types - all ready
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-task","type":"task","title":"Task","status":"open","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-code","type":"code","title":"Code","status":"open","code_spec":{"code":"echo hi"},"created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-cond","type":"condition","title":"Condition","status":"open","condition_spec":{"condition":"true"},"created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// Condition should come first (priority 0)
	ready, err := store.GetNextReady(ctx)
	if err != nil {
		t.Fatalf("GetNextReady() error = %v", err)
	}

	if ready == nil || ready.ID != "bd-cond" {
		t.Errorf("GetNextReady() = %v, want bd-cond (condition has priority)", ready)
	}
}

func TestFileBeadStore_Update(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-001","type":"task","title":"Task 1","status":"open","created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// Update the bead
	bead, _ := store.Get(ctx, "bd-001")
	bead.Status = types.BeadStatusClosed
	now := time.Now()
	bead.ClosedAt = &now

	if err := store.Update(ctx, bead); err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	// Reload and verify
	store2 := NewFileBeadStore(beadsDir)
	if err := store2.Load(ctx); err != nil {
		t.Fatal(err)
	}

	updated, _ := store2.Get(ctx, "bd-001")
	if updated.Status != types.BeadStatusClosed {
		t.Errorf("Updated status = %s, want closed", updated.Status)
	}
}

func TestFileBeadStore_AllDone(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	// Empty store is done
	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	done, err := store.AllDone(ctx)
	if err != nil {
		t.Fatalf("AllDone() error = %v", err)
	}
	if !done {
		t.Error("AllDone() = false, want true for empty store")
	}

	// Add an open bead
	bead := &types.Bead{
		ID:        "bd-001",
		Type:      types.BeadTypeTask,
		Title:     "Task",
		Status:    types.BeadStatusOpen,
		CreatedAt: time.Now(),
	}
	if err := store.Create(ctx, bead); err != nil {
		t.Fatal(err)
	}

	done, err = store.AllDone(ctx)
	if err != nil {
		t.Fatalf("AllDone() error = %v", err)
	}
	if done {
		t.Error("AllDone() = true, want false with open bead")
	}

	// Close the bead
	bead.Status = types.BeadStatusClosed
	if err := store.Update(ctx, bead); err != nil {
		t.Fatal(err)
	}

	done, err = store.AllDone(ctx)
	if err != nil {
		t.Fatalf("AllDone() error = %v", err)
	}
	if !done {
		t.Error("AllDone() = false, want true with all closed")
	}
}

func TestFileBeadStore_Create(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	bead := &types.Bead{
		ID:        "bd-new",
		Type:      types.BeadTypeTask,
		Title:     "New Task",
		Status:    types.BeadStatusOpen,
		CreatedAt: time.Now(),
	}

	if err := store.Create(ctx, bead); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Verify file was created
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	if _, err := os.Stat(issuesPath); os.IsNotExist(err) {
		t.Error("issues.jsonl not created")
	}

	// Verify we can get it back
	got, err := store.Get(ctx, "bd-new")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.Title != "New Task" {
		t.Errorf("Get() = %v, want New Task", got)
	}

	// Try to create duplicate
	if err := store.Create(ctx, bead); err == nil {
		t.Error("Create() duplicate should error")
	}
}

func TestFileBeadStore_List(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-001","type":"task","title":"Open","status":"open","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-002","type":"task","title":"Closed","status":"closed","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-003","type":"task","title":"In Progress","status":"in_progress","created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// List all
	all, err := store.List(ctx, "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 3 {
		t.Errorf("List() count = %d, want 3", len(all))
	}

	// List open only
	open, err := store.List(ctx, types.BeadStatusOpen)
	if err != nil {
		t.Fatalf("List(open) error = %v", err)
	}
	if len(open) != 1 {
		t.Errorf("List(open) count = %d, want 1", len(open))
	}
}

func TestFileBeadStore_DependencyChain(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a dependency chain: A -> B -> C
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-a","type":"task","title":"A","status":"closed","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-b","type":"task","title":"B","status":"open","needs":["bd-a"],"created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-c","type":"task","title":"C","status":"open","needs":["bd-b"],"created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// Only B should be ready (A is closed, C depends on B which is open)
	ready, err := store.GetNextReady(ctx)
	if err != nil {
		t.Fatalf("GetNextReady() error = %v", err)
	}
	if ready == nil || ready.ID != "bd-b" {
		t.Errorf("GetNextReady() = %v, want bd-b", ready)
	}

	// Close B
	ready.Status = types.BeadStatusClosed
	if err := store.Update(ctx, ready); err != nil {
		t.Fatal(err)
	}

	// Now C should be ready
	ready, err = store.GetNextReady(ctx)
	if err != nil {
		t.Fatalf("GetNextReady() error = %v", err)
	}
	if ready == nil || ready.ID != "bd-c" {
		t.Errorf("GetNextReady() = %v, want bd-c", ready)
	}
}
