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

func TestFileBeadStore_TierPriority(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create beads of different tiers - all ready (same type to isolate tier priority)
	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-work","type":"task","title":"Work Task","status":"open","tier":"work","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-wisp","type":"task","title":"Wisp Task","status":"open","tier":"wisp","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-orch","type":"task","title":"Orch Task","status":"open","tier":"orchestrator","created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// Orchestrator tier should come first
	ready, err := store.GetNextReady(ctx)
	if err != nil {
		t.Fatalf("GetNextReady() error = %v", err)
	}
	if ready == nil || ready.ID != "bd-orch" {
		t.Errorf("GetNextReady() = %v, want bd-orch (orchestrator tier has priority)", ready)
	}

	// Close orchestrator bead
	ready.Status = types.BeadStatusClosed
	if err := store.Update(ctx, ready); err != nil {
		t.Fatal(err)
	}

	// Now wisp tier should be next
	ready, err = store.GetNextReady(ctx)
	if err != nil {
		t.Fatalf("GetNextReady() error = %v", err)
	}
	if ready == nil || ready.ID != "bd-wisp" {
		t.Errorf("GetNextReady() = %v, want bd-wisp (wisp tier has priority over work)", ready)
	}

	// Close wisp bead
	ready.Status = types.BeadStatusClosed
	if err := store.Update(ctx, ready); err != nil {
		t.Fatal(err)
	}

	// Finally work tier
	ready, err = store.GetNextReady(ctx)
	if err != nil {
		t.Fatalf("GetNextReady() error = %v", err)
	}
	if ready == nil || ready.ID != "bd-work" {
		t.Errorf("GetNextReady() = %v, want bd-work", ready)
	}
}

func TestFileBeadStore_ListByTier(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-work1","type":"task","title":"Work 1","status":"open","tier":"work","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-work2","type":"task","title":"Work 2","status":"open","tier":"work","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-wisp1","type":"task","title":"Wisp 1","status":"open","tier":"wisp","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-orch1","type":"task","title":"Orch 1","status":"open","tier":"orchestrator","created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// List work tier
	workBeads, err := store.ListByTier(ctx, types.TierWork)
	if err != nil {
		t.Fatalf("ListByTier(work) error = %v", err)
	}
	if len(workBeads) != 2 {
		t.Errorf("ListByTier(work) count = %d, want 2", len(workBeads))
	}

	// List wisp tier
	wispBeads, err := store.ListByTier(ctx, types.TierWisp)
	if err != nil {
		t.Fatalf("ListByTier(wisp) error = %v", err)
	}
	if len(wispBeads) != 1 {
		t.Errorf("ListByTier(wisp) count = %d, want 1", len(wispBeads))
	}

	// List orchestrator tier
	orchBeads, err := store.ListByTier(ctx, types.TierOrchestrator)
	if err != nil {
		t.Fatalf("ListByTier(orchestrator) error = %v", err)
	}
	if len(orchBeads) != 1 {
		t.Errorf("ListByTier(orchestrator) count = %d, want 1", len(orchBeads))
	}
}

func TestFileBeadStore_ListWispsForAgent(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-wisp1","type":"task","title":"Wisp 1","status":"open","tier":"wisp","assignee":"agent-a","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-wisp2","type":"task","title":"Wisp 2","status":"open","tier":"wisp","assignee":"agent-a","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-wisp3","type":"task","title":"Wisp 3","status":"open","tier":"wisp","assignee":"agent-b","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-work1","type":"task","title":"Work 1","status":"open","tier":"work","assignee":"agent-a","created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// List wisps for agent-a
	wispsA, err := store.ListWispsForAgent(ctx, "agent-a")
	if err != nil {
		t.Fatalf("ListWispsForAgent(agent-a) error = %v", err)
	}
	if len(wispsA) != 2 {
		t.Errorf("ListWispsForAgent(agent-a) count = %d, want 2", len(wispsA))
	}

	// List wisps for agent-b
	wispsB, err := store.ListWispsForAgent(ctx, "agent-b")
	if err != nil {
		t.Fatalf("ListWispsForAgent(agent-b) error = %v", err)
	}
	if len(wispsB) != 1 {
		t.Errorf("ListWispsForAgent(agent-b) count = %d, want 1", len(wispsB))
	}

	// List wisps for unknown agent
	wispsC, err := store.ListWispsForAgent(ctx, "agent-c")
	if err != nil {
		t.Fatalf("ListWispsForAgent(agent-c) error = %v", err)
	}
	if len(wispsC) != 0 {
		t.Errorf("ListWispsForAgent(agent-c) count = %d, want 0", len(wispsC))
	}
}

func TestFileBeadStore_ListOrchestrator(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-orch1","type":"condition","title":"Orch 1","status":"open","tier":"orchestrator","workflow_id":"wf-001","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-orch2","type":"code","title":"Orch 2","status":"open","tier":"orchestrator","workflow_id":"wf-001","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-orch3","type":"expand","title":"Orch 3","status":"open","tier":"orchestrator","workflow_id":"wf-002","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-wisp1","type":"task","title":"Wisp 1","status":"open","tier":"wisp","workflow_id":"wf-001","created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// List orchestrator beads for wf-001
	orch1, err := store.ListOrchestrator(ctx, "wf-001")
	if err != nil {
		t.Fatalf("ListOrchestrator(wf-001) error = %v", err)
	}
	if len(orch1) != 2 {
		t.Errorf("ListOrchestrator(wf-001) count = %d, want 2", len(orch1))
	}

	// List orchestrator beads for wf-002
	orch2, err := store.ListOrchestrator(ctx, "wf-002")
	if err != nil {
		t.Fatalf("ListOrchestrator(wf-002) error = %v", err)
	}
	if len(orch2) != 1 {
		t.Errorf("ListOrchestrator(wf-002) count = %d, want 1", len(orch2))
	}
}

func TestFileBeadStore_Get_ReturnsCopy(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-001","type":"task","title":"Task 1","status":"open","needs":["bd-dep"],"labels":["label1"],"created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// Get the bead twice
	bead1, err := store.Get(ctx, "bd-001")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	bead2, err := store.Get(ctx, "bd-001")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Modify bead1
	bead1.Title = "Modified"
	bead1.Needs = append(bead1.Needs, "bd-new")
	bead1.Labels = append(bead1.Labels, "label2")

	// bead2 should be unchanged (separate copy)
	if bead2.Title == "Modified" {
		t.Error("Get() returned pointer to internal state - title was mutated")
	}
	if len(bead2.Needs) != 1 {
		t.Error("Get() returned pointer to internal state - needs slice was mutated")
	}
	if len(bead2.Labels) != 1 {
		t.Error("Get() returned pointer to internal state - labels slice was mutated")
	}

	// Verify internal state is also unchanged
	bead3, err := store.Get(ctx, "bd-001")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if bead3.Title == "Modified" {
		t.Error("Internal state was mutated via returned pointer - title changed")
	}
	if len(bead3.Needs) != 1 {
		t.Error("Internal state was mutated via returned pointer - needs slice changed")
	}
}

func TestFileBeadStore_FilterByHookBead(t *testing.T) {
	dir := t.TempDir()
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		t.Fatal(err)
	}

	issuesPath := filepath.Join(beadsDir, "issues.jsonl")
	content := `{"id":"bd-work","type":"task","title":"Work Bead","status":"open","tier":"work","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-wisp1","type":"task","title":"Wisp 1","status":"open","tier":"wisp","hook_bead":"bd-work","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-wisp2","type":"task","title":"Wisp 2","status":"open","tier":"wisp","hook_bead":"bd-work","created_at":"2026-01-01T00:00:00Z"}
{"id":"bd-wisp3","type":"task","title":"Wisp 3","status":"open","tier":"wisp","hook_bead":"bd-other","created_at":"2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(issuesPath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	store := NewFileBeadStore(beadsDir)
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		t.Fatal(err)
	}

	// List wisps hooked to bd-work
	wisps, err := store.ListFiltered(ctx, BeadFilter{HookBead: "bd-work"})
	if err != nil {
		t.Fatalf("ListFiltered(hook_bead=bd-work) error = %v", err)
	}
	if len(wisps) != 2 {
		t.Errorf("ListFiltered(hook_bead=bd-work) count = %d, want 2", len(wisps))
	}

	// List wisps hooked to bd-other
	wisps2, err := store.ListFiltered(ctx, BeadFilter{HookBead: "bd-other"})
	if err != nil {
		t.Fatalf("ListFiltered(hook_bead=bd-other) error = %v", err)
	}
	if len(wisps2) != 1 {
		t.Errorf("ListFiltered(hook_bead=bd-other) count = %d, want 1", len(wisps2))
	}
}
