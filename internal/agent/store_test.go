package agent

import (
	"context"
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
