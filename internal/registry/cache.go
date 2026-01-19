package registry

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// CacheSubdir is the subdirectory under the cache dir for registries.
	CacheSubdir = "registries"

	// DefaultCacheTTL is the default freshness TTL for cached registries.
	DefaultCacheTTL = 1 * time.Hour
)

// Cache manages the registry cache at ~/.cache/meow/registries/.
// Registries are cloned as shallow git repos for fast lookups.
type Cache struct {
	baseDir string
	ttl     time.Duration
}

// NewCache creates a cache manager using the standard cache directory.
// Respects XDG_CACHE_HOME if set, otherwise uses ~/.cache.
func NewCache() (*Cache, error) {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("getting home dir: %w", err)
		}
		cacheDir = filepath.Join(home, ".cache")
	}

	return &Cache{
		baseDir: filepath.Join(cacheDir, "meow", CacheSubdir),
		ttl:     DefaultCacheTTL,
	}, nil
}

// Dir returns the cache directory path for a registry.
func (c *Cache) Dir(name string) string {
	return filepath.Join(c.baseDir, name)
}

// Clone clones a registry to the cache.
// If the registry already exists in the cache, it is removed first.
func (c *Cache) Clone(name, source string) error {
	dir := c.Dir(name)

	// Remove existing if present
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("removing existing cache: %w", err)
	}

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(dir), 0755); err != nil {
		return fmt.Errorf("creating cache dir: %w", err)
	}

	// Clone with --depth 1 for speed
	url := c.resolveGitURL(source)
	cmd := exec.Command("git", "clone", "--depth", "1", url, dir)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clone failed: %w\n%s", err, output)
	}

	return nil
}

// Fetch updates the cache for a registry by fetching and resetting to origin.
func (c *Cache) Fetch(name string) error {
	dir := c.Dir(name)

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("registry %q not cached", name)
	}

	// Fetch from origin
	cmd := exec.Command("git", "-C", dir, "fetch", "origin")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git fetch failed: %w\n%s", err, output)
	}

	// Reset to origin/HEAD
	cmd = exec.Command("git", "-C", dir, "reset", "--hard", "origin/HEAD")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset failed: %w\n%s", err, output)
	}

	return nil
}

// IsFresh checks if the cache is fresh (within TTL).
// Returns false if the registry is not cached or has no .git directory.
func (c *Cache) IsFresh(name string) (bool, error) {
	dir := c.Dir(name)

	info, err := os.Stat(filepath.Join(dir, ".git"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return time.Since(info.ModTime()) < c.ttl, nil
}

// Exists checks if a registry is cached.
func (c *Cache) Exists(name string) bool {
	_, err := os.Stat(c.Dir(name))
	return err == nil
}

// Remove deletes a cached registry.
func (c *Cache) Remove(name string) error {
	return os.RemoveAll(c.Dir(name))
}

// resolveGitURL converts source to a git URL.
// Handles GitHub shorthand (owner/repo) and github.com prefix.
func (c *Cache) resolveGitURL(source string) string {
	// Already a full URL - return as-is
	if strings.Contains(source, "://") || strings.HasPrefix(source, "git@") {
		return source
	}

	// GitHub shorthand: owner/repo (exactly one slash)
	if strings.Count(source, "/") == 1 {
		return "https://github.com/" + source + ".git"
	}

	// Handle github.com/owner/repo
	if strings.HasPrefix(source, "github.com/") {
		return "https://" + source + ".git"
	}

	// Unknown format, return as-is
	return source
}
