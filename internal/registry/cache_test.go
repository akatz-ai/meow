package registry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewCache(t *testing.T) {
	// Test default behavior (uses ~/.cache/meow/registries)
	cache, err := NewCache()
	if err != nil {
		t.Fatalf("NewCache() error: %v", err)
	}
	if cache == nil {
		t.Fatal("NewCache() returned nil")
	}

	// Verify baseDir ends with expected path
	if !contains(cache.baseDir, "meow") || !contains(cache.baseDir, CacheSubdir) {
		t.Errorf("baseDir = %q, want to contain 'meow' and %q", cache.baseDir, CacheSubdir)
	}

	// Verify default TTL
	if cache.ttl != DefaultCacheTTL {
		t.Errorf("ttl = %v, want %v", cache.ttl, DefaultCacheTTL)
	}
}

func TestNewCache_XDGCacheHome(t *testing.T) {
	// Set XDG_CACHE_HOME and verify it's used
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	cache, err := NewCache()
	if err != nil {
		t.Fatalf("NewCache() error: %v", err)
	}

	expected := filepath.Join(tmpDir, "meow", CacheSubdir)
	if cache.baseDir != expected {
		t.Errorf("baseDir = %q, want %q", cache.baseDir, expected)
	}
}

func TestCache_Dir(t *testing.T) {
	cache := &Cache{
		baseDir: "/test/cache/meow/registries",
		ttl:     DefaultCacheTTL,
	}

	dir := cache.Dir("my-registry")
	expected := "/test/cache/meow/registries/my-registry"
	if dir != expected {
		t.Errorf("Dir() = %q, want %q", dir, expected)
	}
}

func TestCache_resolveGitURL(t *testing.T) {
	cache := &Cache{}

	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{
			name:     "github shorthand owner/repo",
			source:   "anthropics/meow-registry",
			expected: "https://github.com/anthropics/meow-registry.git",
		},
		{
			name:     "github.com prefix",
			source:   "github.com/anthropics/meow-registry",
			expected: "https://github.com/anthropics/meow-registry.git",
		},
		{
			name:     "full https URL",
			source:   "https://github.com/anthropics/meow-registry.git",
			expected: "https://github.com/anthropics/meow-registry.git",
		},
		{
			name:     "git@ URL",
			source:   "git@github.com:anthropics/meow-registry.git",
			expected: "git@github.com:anthropics/meow-registry.git",
		},
		{
			name:     "gitlab URL",
			source:   "https://gitlab.com/org/repo.git",
			expected: "https://gitlab.com/org/repo.git",
		},
		{
			name:     "nested path not converted",
			source:   "some/nested/path/repo",
			expected: "some/nested/path/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cache.resolveGitURL(tt.source)
			if got != tt.expected {
				t.Errorf("resolveGitURL(%q) = %q, want %q", tt.source, got, tt.expected)
			}
		})
	}
}

func TestCache_Exists(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &Cache{
		baseDir: tmpDir,
		ttl:     DefaultCacheTTL,
	}

	// Non-existent registry
	if cache.Exists("nonexistent") {
		t.Error("Exists() = true for nonexistent registry, want false")
	}

	// Create a registry directory
	regDir := filepath.Join(tmpDir, "my-registry")
	if err := os.MkdirAll(regDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	// Now it should exist
	if !cache.Exists("my-registry") {
		t.Error("Exists() = false for existing registry, want true")
	}
}

func TestCache_Remove(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &Cache{
		baseDir: tmpDir,
		ttl:     DefaultCacheTTL,
	}

	// Create a registry directory with some content
	regDir := filepath.Join(tmpDir, "my-registry")
	subDir := filepath.Join(regDir, ".git")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(regDir, "file.txt"), []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Verify it exists
	if !cache.Exists("my-registry") {
		t.Fatal("Registry should exist before removal")
	}

	// Remove it
	if err := cache.Remove("my-registry"); err != nil {
		t.Fatalf("Remove() error: %v", err)
	}

	// Verify it's gone
	if cache.Exists("my-registry") {
		t.Error("Registry still exists after Remove()")
	}
}

func TestCache_Remove_Nonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &Cache{
		baseDir: tmpDir,
		ttl:     DefaultCacheTTL,
	}

	// Removing nonexistent should not error (os.RemoveAll behavior)
	if err := cache.Remove("nonexistent"); err != nil {
		t.Errorf("Remove() error for nonexistent: %v", err)
	}
}

func TestCache_IsFresh(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &Cache{
		baseDir: tmpDir,
		ttl:     1 * time.Hour,
	}

	// Non-existent registry
	fresh, err := cache.IsFresh("nonexistent")
	if err != nil {
		t.Fatalf("IsFresh() error: %v", err)
	}
	if fresh {
		t.Error("IsFresh() = true for nonexistent registry, want false")
	}

	// Create a registry with .git directory
	regDir := filepath.Join(tmpDir, "my-registry")
	gitDir := filepath.Join(regDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	// Fresh registry (just created)
	fresh, err = cache.IsFresh("my-registry")
	if err != nil {
		t.Fatalf("IsFresh() error: %v", err)
	}
	if !fresh {
		t.Error("IsFresh() = false for fresh registry, want true")
	}

	// Test with very short TTL
	cache.ttl = 1 * time.Millisecond
	time.Sleep(5 * time.Millisecond)

	fresh, err = cache.IsFresh("my-registry")
	if err != nil {
		t.Fatalf("IsFresh() error: %v", err)
	}
	if fresh {
		t.Error("IsFresh() = true for stale registry, want false")
	}
}

func TestCache_IsFresh_NoGitDir(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &Cache{
		baseDir: tmpDir,
		ttl:     1 * time.Hour,
	}

	// Create a registry without .git directory
	regDir := filepath.Join(tmpDir, "my-registry")
	if err := os.MkdirAll(regDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}

	// Should not be fresh (no .git)
	fresh, err := cache.IsFresh("my-registry")
	if err != nil {
		t.Fatalf("IsFresh() error: %v", err)
	}
	if fresh {
		t.Error("IsFresh() = true for dir without .git, want false")
	}
}

// contains is a helper to check if s contains substr
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

// Integration tests that require git - skipped in short mode
func TestCache_Clone_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	cache := &Cache{
		baseDir: tmpDir,
		ttl:     DefaultCacheTTL,
	}

	// Clone a small public repo (this is slow, skip in short mode)
	err := cache.Clone("test-registry", "github/gitignore")
	if err != nil {
		t.Fatalf("Clone() error: %v", err)
	}

	// Verify it was cloned
	if !cache.Exists("test-registry") {
		t.Error("Registry does not exist after Clone()")
	}

	// Verify .git exists
	gitDir := filepath.Join(cache.Dir("test-registry"), ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Error(".git directory does not exist after Clone()")
	}
}

func TestCache_Fetch_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	cache := &Cache{
		baseDir: tmpDir,
		ttl:     DefaultCacheTTL,
	}

	// First clone
	if err := cache.Clone("test-registry", "github/gitignore"); err != nil {
		t.Fatalf("Clone() error: %v", err)
	}

	// Then fetch
	if err := cache.Fetch("test-registry"); err != nil {
		t.Fatalf("Fetch() error: %v", err)
	}
}

func TestCache_Fetch_NotCached(t *testing.T) {
	tmpDir := t.TempDir()
	cache := &Cache{
		baseDir: tmpDir,
		ttl:     DefaultCacheTTL,
	}

	// Fetch without clone should error
	err := cache.Fetch("nonexistent")
	if err == nil {
		t.Error("Fetch() should error for uncached registry")
	}
}

func TestCache_Clone_OverwritesExisting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	tmpDir := t.TempDir()
	cache := &Cache{
		baseDir: tmpDir,
		ttl:     DefaultCacheTTL,
	}

	// Create a pre-existing directory with content
	regDir := filepath.Join(tmpDir, "test-registry")
	if err := os.MkdirAll(regDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}
	markerFile := filepath.Join(regDir, "should-be-deleted.txt")
	if err := os.WriteFile(markerFile, []byte("marker"), 0644); err != nil {
		t.Fatalf("Failed to create marker file: %v", err)
	}

	// Clone should remove existing and clone fresh
	if err := cache.Clone("test-registry", "github/gitignore"); err != nil {
		t.Fatalf("Clone() error: %v", err)
	}

	// Marker file should be gone
	if _, err := os.Stat(markerFile); !os.IsNotExist(err) {
		t.Error("Clone() did not remove existing directory")
	}
}
