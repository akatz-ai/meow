package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegistriesStore_Load_MissingFile(t *testing.T) {
	// Load should return empty struct if file doesn't exist
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	f, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if f == nil {
		t.Fatal("Load() returned nil, want non-nil RegistriesFile")
	}

	if f.Registries == nil {
		t.Fatal("Load() returned nil Registries map, want empty map")
	}

	if len(f.Registries) != 0 {
		t.Errorf("Load() Registries length = %d, want 0", len(f.Registries))
	}
}

func TestRegistriesStore_Save_CreatesDirectory(t *testing.T) {
	// Save should create parent directories if they don't exist
	tmpDir := t.TempDir()
	nestedPath := filepath.Join(tmpDir, "nested", "dir", "registries.json")
	store := NewRegistriesStoreWithPath(nestedPath)

	f := &RegistriesFile{
		Registries: make(map[string]RegisteredRegistry),
	}

	if err := store.Save(f); err != nil {
		t.Fatalf("Save() error = %v, want nil", err)
	}

	// Verify file exists
	if _, err := os.Stat(nestedPath); os.IsNotExist(err) {
		t.Errorf("Save() did not create file at %s", nestedPath)
	}
}

func TestRegistriesStore_Add_NewRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	err := store.Add("test-registry", "github.com/user/repo", "1.0.0")
	if err != nil {
		t.Fatalf("Add() error = %v, want nil", err)
	}

	// Verify it was saved
	f, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	reg, exists := f.Registries["test-registry"]
	if !exists {
		t.Fatal("Add() did not add registry to file")
	}

	if reg.Source != "github.com/user/repo" {
		t.Errorf("Add() source = %q, want %q", reg.Source, "github.com/user/repo")
	}

	if reg.Version != "1.0.0" {
		t.Errorf("Add() version = %q, want %q", reg.Version, "1.0.0")
	}

	if reg.AddedAt.IsZero() {
		t.Error("Add() AddedAt is zero, want non-zero time")
	}

	if reg.UpdatedAt.IsZero() {
		t.Error("Add() UpdatedAt is zero, want non-zero time")
	}
}

func TestRegistriesStore_Add_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	// Add first time
	if err := store.Add("test-registry", "github.com/user/repo", "1.0.0"); err != nil {
		t.Fatalf("Add() first call error = %v, want nil", err)
	}

	// Add second time - should fail
	err := store.Add("test-registry", "github.com/other/repo", "2.0.0")
	if err == nil {
		t.Fatal("Add() duplicate error = nil, want error")
	}
}

func TestRegistriesStore_Remove_Existing(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	// Add a registry first
	if err := store.Add("test-registry", "github.com/user/repo", "1.0.0"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Remove it
	if err := store.Remove("test-registry"); err != nil {
		t.Fatalf("Remove() error = %v, want nil", err)
	}

	// Verify it's gone
	f, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if _, exists := f.Registries["test-registry"]; exists {
		t.Error("Remove() did not remove registry from file")
	}
}

func TestRegistriesStore_Remove_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	err := store.Remove("nonexistent")
	if err == nil {
		t.Fatal("Remove() nonexistent error = nil, want error")
	}
}

func TestRegistriesStore_Get_Existing(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	// Add a registry first
	if err := store.Add("test-registry", "github.com/user/repo", "1.0.0"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	reg, err := store.Get("test-registry")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}

	if reg == nil {
		t.Fatal("Get() returned nil, want non-nil")
	}

	if reg.Source != "github.com/user/repo" {
		t.Errorf("Get() source = %q, want %q", reg.Source, "github.com/user/repo")
	}
}

func TestRegistriesStore_Get_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	_, err := store.Get("nonexistent")
	if err == nil {
		t.Fatal("Get() nonexistent error = nil, want error")
	}
}

func TestRegistriesStore_Update_ExistingRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	// Add a registry first
	if err := store.Add("test-registry", "github.com/user/repo", "1.0.0"); err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	// Get original timestamps
	reg1, _ := store.Get("test-registry")
	originalAddedAt := reg1.AddedAt
	originalUpdatedAt := reg1.UpdatedAt

	// Wait a bit to ensure time difference
	time.Sleep(10 * time.Millisecond)

	// Update version
	if err := store.Update("test-registry", "2.0.0"); err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}

	// Verify changes
	reg2, err := store.Get("test-registry")
	if err != nil {
		t.Fatalf("Get() after update error = %v", err)
	}

	if reg2.Version != "2.0.0" {
		t.Errorf("Update() version = %q, want %q", reg2.Version, "2.0.0")
	}

	if !reg2.AddedAt.Equal(originalAddedAt) {
		t.Error("Update() changed AddedAt, should remain unchanged")
	}

	if !reg2.UpdatedAt.After(originalUpdatedAt) {
		t.Error("Update() did not update UpdatedAt timestamp")
	}
}

func TestRegistriesStore_Update_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	err := store.Update("nonexistent", "1.0.0")
	if err == nil {
		t.Fatal("Update() nonexistent error = nil, want error")
	}
}

func TestRegistriesStore_List_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	regs, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}

	if len(regs) != 0 {
		t.Errorf("List() empty store returned %d registries, want 0", len(regs))
	}
}

func TestRegistriesStore_List_Multiple(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewRegistriesStoreWithPath(filepath.Join(tmpDir, "registries.json"))

	// Add multiple registries
	store.Add("registry-1", "github.com/user/repo1", "1.0.0")
	store.Add("registry-2", "github.com/user/repo2", "2.0.0")
	store.Add("registry-3", "github.com/user/repo3", "3.0.0")

	regs, err := store.List()
	if err != nil {
		t.Fatalf("List() error = %v, want nil", err)
	}

	if len(regs) != 3 {
		t.Errorf("List() returned %d registries, want 3", len(regs))
	}

	// Verify all registries are present
	for _, name := range []string{"registry-1", "registry-2", "registry-3"} {
		if _, exists := regs[name]; !exists {
			t.Errorf("List() missing registry %q", name)
		}
	}
}

func TestNewRegistriesStore_DefaultPath(t *testing.T) {
	store, err := NewRegistriesStore()
	if err != nil {
		t.Fatalf("NewRegistriesStore() error = %v, want nil", err)
	}

	if store == nil {
		t.Fatal("NewRegistriesStore() returned nil, want non-nil")
	}

	// The path should end with ~/.meow/registries.json
	home, _ := os.UserHomeDir()
	expectedPath := filepath.Join(home, ".meow", RegistriesFileName)
	if store.Path() != expectedPath {
		t.Errorf("NewRegistriesStore() path = %q, want %q", store.Path(), expectedPath)
	}
}
