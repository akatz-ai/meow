package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInstalledStore_LoadMissingFile(t *testing.T) {
	// Create a temp directory for testing
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	// Load should return empty struct when file doesn't exist
	f, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}
	if f == nil {
		t.Fatal("Load() returned nil, want non-nil InstalledFile")
	}
	if f.Collections == nil {
		t.Error("Load() returned nil Collections map, want initialized map")
	}
	if len(f.Collections) != 0 {
		t.Errorf("Load() returned %d collections, want 0", len(f.Collections))
	}
}

func TestInstalledStore_SaveCreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	// Use a nested path to test directory creation
	store := &InstalledStore{path: filepath.Join(tmpDir, "nested", "dir", "installed.json")}

	f := &InstalledFile{Collections: make(map[string]InstalledCollection)}
	if err := store.Save(f); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	// Verify the file was created
	if _, err := os.Stat(store.path); os.IsNotExist(err) {
		t.Error("Save() did not create the file")
	}
}

func TestInstalledStore_AddNewCollection(t *testing.T) {
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	info := InstalledCollection{
		Registry:        "meow-official",
		RegistryVersion: "1.0.0",
		Path:            "/home/user/.meow/workflows/sprint",
		Scope:           ScopeUser,
	}

	if err := store.Add("sprint", info); err != nil {
		t.Fatalf("Add() error = %v, want nil", err)
	}

	// Verify it was saved
	f, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(f.Collections) != 1 {
		t.Errorf("Load() returned %d collections, want 1", len(f.Collections))
	}

	c, exists := f.Collections["sprint"]
	if !exists {
		t.Fatal("Collection 'sprint' not found after Add()")
	}
	if c.Registry != "meow-official" {
		t.Errorf("Registry = %q, want %q", c.Registry, "meow-official")
	}
	if c.InstalledAt.IsZero() {
		t.Error("InstalledAt was not set")
	}
}

func TestInstalledStore_AddOverwritesExisting(t *testing.T) {
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	// Add first version
	info1 := InstalledCollection{
		Registry:        "meow-official",
		RegistryVersion: "1.0.0",
		Path:            "/home/user/.meow/workflows/sprint",
		Scope:           ScopeUser,
	}
	if err := store.Add("sprint", info1); err != nil {
		t.Fatalf("First Add() error = %v", err)
	}

	// Wait a tiny bit so timestamps differ
	time.Sleep(10 * time.Millisecond)

	// Add second version (should overwrite)
	info2 := InstalledCollection{
		Registry:        "meow-official",
		RegistryVersion: "2.0.0",
		Path:            "/home/user/.meow/workflows/sprint",
		Scope:           ScopeUser,
	}
	if err := store.Add("sprint", info2); err != nil {
		t.Fatalf("Second Add() error = %v", err)
	}

	// Verify it was overwritten
	f, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(f.Collections) != 1 {
		t.Errorf("Load() returned %d collections, want 1", len(f.Collections))
	}

	c := f.Collections["sprint"]
	if c.RegistryVersion != "2.0.0" {
		t.Errorf("RegistryVersion = %q, want %q", c.RegistryVersion, "2.0.0")
	}
}

func TestInstalledStore_RemoveExisting(t *testing.T) {
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	// Add a collection first
	info := InstalledCollection{
		Registry: "meow-official",
		Path:     "/home/user/.meow/workflows/sprint",
		Scope:    ScopeUser,
	}
	if err := store.Add("sprint", info); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Remove it
	if err := store.Remove("sprint"); err != nil {
		t.Fatalf("Remove() error = %v, want nil", err)
	}

	// Verify it's gone
	f, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(f.Collections) != 0 {
		t.Errorf("Load() returned %d collections, want 0", len(f.Collections))
	}
}

func TestInstalledStore_RemoveNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	// Try to remove a collection that doesn't exist
	err := store.Remove("nonexistent")
	if err == nil {
		t.Fatal("Remove() error = nil, want error for nonexistent collection")
	}

	// Error message should mention the collection name
	if !contains(err.Error(), "nonexistent") || !contains(err.Error(), "not installed") {
		t.Errorf("Remove() error = %q, want error mentioning collection not installed", err)
	}
}

func TestInstalledStore_GetExisting(t *testing.T) {
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	// Add a collection
	info := InstalledCollection{
		Registry:        "meow-official",
		RegistryVersion: "1.0.0",
		Path:            "/home/user/.meow/workflows/sprint",
		Scope:           ScopeUser,
	}
	if err := store.Add("sprint", info); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Get it
	c, err := store.Get("sprint")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if c == nil {
		t.Fatal("Get() returned nil, want collection")
	}
	if c.Registry != "meow-official" {
		t.Errorf("Registry = %q, want %q", c.Registry, "meow-official")
	}
}

func TestInstalledStore_GetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	// Get should return nil (not error) when collection doesn't exist
	c, err := store.Get("nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if c != nil {
		t.Errorf("Get() = %v, want nil for nonexistent collection", c)
	}
}

func TestInstalledStore_ExistsTrue(t *testing.T) {
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	// Add a collection
	info := InstalledCollection{
		Registry: "meow-official",
		Path:     "/home/user/.meow/workflows/sprint",
		Scope:    ScopeUser,
	}
	if err := store.Add("sprint", info); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Exists should return true
	exists, err := store.Exists("sprint")
	if err != nil {
		t.Fatalf("Exists() error = %v, want nil", err)
	}
	if !exists {
		t.Error("Exists() = false, want true")
	}
}

func TestInstalledStore_ExistsFalse(t *testing.T) {
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	// Exists should return false for nonexistent collection
	exists, err := store.Exists("nonexistent")
	if err != nil {
		t.Fatalf("Exists() error = %v, want nil", err)
	}
	if exists {
		t.Error("Exists() = true, want false")
	}
}

func TestInstalledStore_List(t *testing.T) {
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	// Add multiple collections
	collections := map[string]InstalledCollection{
		"sprint": {
			Registry: "meow-official",
			Path:     "/home/user/.meow/workflows/sprint",
			Scope:    ScopeUser,
		},
		"deploy": {
			Registry: "meow-official",
			Path:     "/home/user/.meow/workflows/deploy",
			Scope:    ScopeUser,
		},
	}

	for name, info := range collections {
		if err := store.Add(name, info); err != nil {
			t.Fatalf("Add(%q) error = %v", name, err)
		}
	}

	// List should return all
	list, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if len(list) != 2 {
		t.Errorf("List() returned %d collections, want 2", len(list))
	}
	if _, ok := list["sprint"]; !ok {
		t.Error("List() missing 'sprint'")
	}
	if _, ok := list["deploy"]; !ok {
		t.Error("List() missing 'deploy'")
	}
}

func TestInstalledStore_ListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	store := &InstalledStore{path: filepath.Join(tmpDir, "installed.json")}

	// List should return empty map (not nil) when no collections
	list, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}
	if list == nil {
		t.Error("List() returned nil, want empty map")
	}
	if len(list) != 0 {
		t.Errorf("List() returned %d collections, want 0", len(list))
	}
}

func TestNewInstalledStore(t *testing.T) {
	// NewInstalledStore should succeed and point to ~/.meow/installed.json
	store, err := NewInstalledStore()
	if err != nil {
		t.Fatalf("NewInstalledStore() error = %v, want nil", err)
	}
	if store == nil {
		t.Fatal("NewInstalledStore() returned nil store")
	}

	// Verify the path looks correct
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, ".meow", InstalledFileName)
	if store.path != expected {
		t.Errorf("store.path = %q, want %q", store.path, expected)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
