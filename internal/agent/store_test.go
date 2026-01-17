package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

func TestNewStore(t *testing.T) {
	s := NewStore("/tmp/test-meow")
	if s == nil {
		t.Fatal("NewStore() returned nil")
	}
	if s.stateDir != "/tmp/test-meow" {
		t.Errorf("stateDir = %s, want /tmp/test-meow", s.stateDir)
	}
}

func TestStore_LoadEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)

	err := s.Load(context.Background())
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	agents, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("List() returned %d agents, want 0", len(agents))
	}
}

func TestStore_SetAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	now := time.Now()
	agent := &types.Agent{
		ID:        "test-agent-1",
		Name:      "Test Agent 1",
		Status:    types.AgentStatusActive,
		SessionID: "session-123",
		CreatedAt: &now,
	}

	err := s.Set(ctx, agent)
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	got, err := s.Get(ctx, "test-agent-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil")
	}
	if got.ID != "test-agent-1" {
		t.Errorf("ID = %s, want test-agent-1", got.ID)
	}
	if got.SessionID != "session-123" {
		t.Errorf("SessionID = %s, want session-123", got.SessionID)
	}
}

func TestStore_SetInvalidAgent(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Empty ID should fail validation
	agent := &types.Agent{
		Status: types.AgentStatusActive,
	}

	err := s.Set(ctx, agent)
	if err == nil {
		t.Error("Set() should fail for invalid agent")
	}
}

func TestStore_GetNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	got, err := s.Get(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Error("Get() should return nil for nonexistent agent")
	}
}

func TestStore_Update(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	agent := &types.Agent{
		ID:     "test-agent",
		Name:   "Test Agent",
		Status: types.AgentStatusActive,
	}
	if err := s.Set(ctx, agent); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	err := s.Update(ctx, "test-agent", func(a *types.Agent) error {
		a.SessionID = "updated-session"
		return nil
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	got, _ := s.Get(ctx, "test-agent")
	if got.SessionID != "updated-session" {
		t.Errorf("SessionID = %s, want updated-session", got.SessionID)
	}
}

func TestStore_UpdateNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	err := s.Update(ctx, "nonexistent", func(a *types.Agent) error {
		return nil
	})
	if err == nil {
		t.Error("Update() should fail for nonexistent agent")
	}
}

func TestStore_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	agent := &types.Agent{
		ID:     "test-agent",
		Name:   "Test Agent",
		Status: types.AgentStatusActive,
	}
	if err := s.Set(ctx, agent); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	err := s.Delete(ctx, "test-agent")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	got, _ := s.Get(ctx, "test-agent")
	if got != nil {
		t.Error("Agent should be deleted")
	}
}

func TestStore_DeleteNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	err := s.Delete(ctx, "nonexistent")
	if err == nil {
		t.Error("Delete() should fail for nonexistent agent")
	}
}

func TestStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Add some agents
	agents := []*types.Agent{
		{ID: "agent-1", Name: "Agent 1", Status: types.AgentStatusActive},
		{ID: "agent-2", Name: "Agent 2", Status: types.AgentStatusStopped},
		{ID: "agent-3", Name: "Agent 3", Status: types.AgentStatusActive},
	}
	for _, a := range agents {
		if err := s.Set(ctx, a); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(list) != 3 {
		t.Errorf("List() returned %d agents, want 3", len(list))
	}
}

func TestStore_ListByStatus(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Add some agents
	agents := []*types.Agent{
		{ID: "agent-1", Name: "Agent 1", Status: types.AgentStatusActive},
		{ID: "agent-2", Name: "Agent 2", Status: types.AgentStatusStopped},
		{ID: "agent-3", Name: "Agent 3", Status: types.AgentStatusActive},
	}
	for _, a := range agents {
		if err := s.Set(ctx, a); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	active, err := s.ListByStatus(ctx, types.AgentStatusActive)
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(active) != 2 {
		t.Errorf("ListByStatus(active) returned %d agents, want 2", len(active))
	}

	stopped, err := s.ListByStatus(ctx, types.AgentStatusStopped)
	if err != nil {
		t.Fatalf("ListByStatus() error = %v", err)
	}
	if len(stopped) != 1 {
		t.Errorf("ListByStatus(stopped) returned %d agents, want 1", len(stopped))
	}
}

func TestStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()

	// Create store and add agent
	s1 := NewStore(tmpDir)
	if err := s1.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	now := time.Now()
	agent := &types.Agent{
		ID:          "persistent-agent",
		Name:        "Persistent Agent",
		Status:      types.AgentStatusActive,
		SessionID:   "session-456",
		TmuxSession: "meow-test",
		Workdir:     "/tmp/test",
		CreatedAt:   &now,
	}
	if err := s1.Set(ctx, agent); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Create new store instance and load
	s2 := NewStore(tmpDir)
	if err := s2.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	got, err := s2.Get(ctx, "persistent-agent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil after reload")
	}
	if got.SessionID != "session-456" {
		t.Errorf("SessionID = %s, want session-456", got.SessionID)
	}
	if got.TmuxSession != "meow-test" {
		t.Errorf("TmuxSession = %s, want meow-test", got.TmuxSession)
	}
	if got.Workdir != "/tmp/test" {
		t.Errorf("Workdir = %s, want /tmp/test", got.Workdir)
	}
}

func TestStore_AtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	agent := &types.Agent{
		ID:     "test-agent",
		Name:   "Test Agent",
		Status: types.AgentStatusActive,
	}
	if err := s.Set(ctx, agent); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Check that the file exists and no temp file remains
	agentsPath := filepath.Join(tmpDir, "agents.json")
	tmpPath := agentsPath + ".tmp"

	if _, err := os.Stat(agentsPath); os.IsNotExist(err) {
		t.Error("agents.json should exist")
	}
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful write")
	}
}

func TestStore_NotLoaded(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	// Don't call Load()

	_, err := s.Get(ctx, "test")
	if err == nil {
		t.Error("Get() should fail when not loaded")
	}

	err = s.Set(ctx, &types.Agent{ID: "test", Status: types.AgentStatusActive})
	if err == nil {
		t.Error("Set() should fail when not loaded")
	}

	_, err = s.List(ctx)
	if err == nil {
		t.Error("List() should fail when not loaded")
	}

	_, err = s.ListByStatus(ctx, types.AgentStatusActive)
	if err == nil {
		t.Error("ListByStatus() should fail when not loaded")
	}

	err = s.Update(ctx, "test", func(a *types.Agent) error { return nil })
	if err == nil {
		t.Error("Update() should fail when not loaded")
	}

	err = s.Delete(ctx, "test")
	if err == nil {
		t.Error("Delete() should fail when not loaded")
	}
}

func TestStore_SaveExplicit(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Manually modify in-memory and call Save
	s.mu.Lock()
	s.agents["manual-agent"] = &types.Agent{
		ID:     "manual-agent",
		Name:   "Manual Agent",
		Status: types.AgentStatusActive,
	}
	s.mu.Unlock()

	if err := s.Save(ctx); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Reload and verify
	s2 := NewStore(tmpDir)
	if err := s2.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	got, _ := s2.Get(ctx, "manual-agent")
	if got == nil {
		t.Error("Agent should be persisted after explicit Save()")
	}
}

func TestStore_GetReturnsCopy(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	agent := &types.Agent{
		ID:     "test-agent",
		Name:   "Original Name",
		Status: types.AgentStatusActive,
	}
	if err := s.Set(ctx, agent); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get the agent and modify it
	got, _ := s.Get(ctx, "test-agent")
	got.Name = "Modified Name"
	got.SessionID = "modified-session"

	// Get again - should have original values (not modified)
	got2, _ := s.Get(ctx, "test-agent")
	if got2.Name != "Original Name" {
		t.Errorf("Internal state was modified: Name = %s, want 'Original Name'", got2.Name)
	}
	if got2.SessionID != "" {
		t.Errorf("Internal state was modified: SessionID = %s, want empty", got2.SessionID)
	}
}

func TestStore_SetStoresCopy(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	agent := &types.Agent{
		ID:     "test-agent",
		Name:   "Original Name",
		Status: types.AgentStatusActive,
	}
	if err := s.Set(ctx, agent); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Modify the original agent after Set
	agent.Name = "Modified After Set"

	// Get should return the original value
	got, _ := s.Get(ctx, "test-agent")
	if got.Name != "Original Name" {
		t.Errorf("Set did not store a copy: Name = %s, want 'Original Name'", got.Name)
	}
}

func TestStore_ListReturnsCopies(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	agent := &types.Agent{
		ID:     "test-agent",
		Name:   "Original Name",
		Status: types.AgentStatusActive,
	}
	if err := s.Set(ctx, agent); err != nil {
		t.Fatalf("Set() error = %v", err)
	}

	// Get list and modify
	list, _ := s.List(ctx)
	list[0].Name = "Modified Via List"

	// Get should return original
	got, _ := s.Get(ctx, "test-agent")
	if got.Name != "Original Name" {
		t.Errorf("List returned reference to internal state: Name = %s, want 'Original Name'", got.Name)
	}
}

// TestStore_SaveUsesExclusiveLock verifies that Save() uses an exclusive lock (Lock),
// not a read lock (RLock). This test exercises concurrent Save() calls which
// should NOT race since Save() writes to disk and requires exclusive access.
//
// Run with: go test -race ./internal/agent/...
//
// If Save() incorrectly uses RLock(), the race detector will catch it because
// multiple goroutines will be writing to the same file concurrently.
func TestStore_SaveUsesExclusiveLock(t *testing.T) {
	tmpDir := t.TempDir()
	s := NewStore(tmpDir)
	ctx := context.Background()

	if err := s.Load(ctx); err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Add some agents to have data to save
	for i := 0; i < 5; i++ {
		agent := &types.Agent{
			ID:     fmt.Sprintf("agent-%d", i),
			Name:   fmt.Sprintf("Agent %d", i),
			Status: types.AgentStatusActive,
		}
		if err := s.Set(ctx, agent); err != nil {
			t.Fatalf("Set() error = %v", err)
		}
	}

	// Run concurrent Save() calls
	// If Save() uses RLock incorrectly, the race detector will catch concurrent writes
	const numGoroutines = 10
	errCh := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			errCh <- s.Save(ctx)
		}()
	}

	// Collect results
	for i := 0; i < numGoroutines; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("Save() error = %v", err)
		}
	}

	// Verify data is still intact
	agents, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(agents) != 5 {
		t.Errorf("expected 5 agents, got %d", len(agents))
	}
}

func TestCopyAgent(t *testing.T) {
	now := time.Now()
	original := &types.Agent{
		ID:            "test",
		Name:          "Test",
		Status:        types.AgentStatusActive,
		SessionID:     "session-1",
		LastHeartbeat: &now,
		CreatedAt:     &now,
		Env:           map[string]string{"KEY": "value"},
		Labels:        map[string]string{"label": "value"},
	}

	cp := copyAgent(original)

	// Verify values match
	if cp.ID != original.ID || cp.Name != original.Name {
		t.Error("Basic fields not copied correctly")
	}

	// Modify copy - original should be unchanged
	cp.Name = "Modified"
	if original.Name != "Test" {
		t.Error("Modifying copy affected original")
	}

	// Modify copy's time pointer
	newTime := now.Add(time.Hour)
	cp.LastHeartbeat = &newTime
	if !original.LastHeartbeat.Equal(now) {
		t.Error("Modifying copy's time pointer affected original")
	}

	// Modify copy's map
	cp.Env["KEY"] = "modified"
	if original.Env["KEY"] != "value" {
		t.Error("Modifying copy's Env map affected original")
	}

	// Test nil input
	if copyAgent(nil) != nil {
		t.Error("copyAgent(nil) should return nil")
	}
}
