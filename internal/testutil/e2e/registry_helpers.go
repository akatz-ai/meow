package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// RegistryTestSetup holds paths for registry E2E testing.
// It provides isolated HOME and cache directories for testing
// the full registry lifecycle without affecting user state.
type RegistryTestSetup struct {
	// HomeDir is the isolated HOME directory for this test.
	// Overrides ~ for ~/.meow/registries.json, ~/.meow/installed.json, etc.
	HomeDir string

	// CacheDir is the isolated cache directory.
	// Overrides ~/.cache/meow/registries/
	CacheDir string

	// harness is the parent E2E harness
	harness *Harness
}

// NewRegistryTestSetup creates an isolated environment for registry testing.
// It sets up:
// - Isolated HOME directory (for ~/.meow/)
// - Isolated cache directory (for ~/.cache/meow/)
// - Required directory structure
func (h *Harness) NewRegistryTestSetup() *RegistryTestSetup {
	h.t.Helper()

	homeDir := filepath.Join(h.TempDir, "home")
	cacheDir := filepath.Join(h.TempDir, "cache")

	// Create directory structure
	dirs := []string{
		filepath.Join(homeDir, ".meow", "workflows"),
		filepath.Join(cacheDir, "meow", "registries"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			h.t.Fatalf("failed to create directory %s: %v", dir, err)
		}
	}

	return &RegistryTestSetup{
		HomeDir:  homeDir,
		CacheDir: cacheDir,
		harness:  h,
	}
}

// Env returns environment variables with HOME and XDG_CACHE_HOME overridden.
// Use this when running meow commands that interact with registries.
func (r *RegistryTestSetup) Env() []string {
	env := r.harness.Env()
	env = append(env,
		fmt.Sprintf("HOME=%s", r.HomeDir),
		fmt.Sprintf("XDG_CACHE_HOME=%s", r.CacheDir),
	)
	return env
}

// TestRegistry represents a test registry created in temp directory.
type TestRegistry struct {
	// Name is the registry name (from registry.json)
	Name string

	// Path is the filesystem path to the registry root
	Path string

	// Version is the registry version
	Version string

	// Collections is a list of collection names in this registry
	Collections []string
}

// CreateTestRegistry creates a complete test registry as a git repo.
// The registry contains a single collection by default.
func (r *RegistryTestSetup) CreateTestRegistry(name, version string) *TestRegistry {
	r.harness.t.Helper()

	registryPath := filepath.Join(r.harness.TempDir, "registries", name)
	collectionsPath := filepath.Join(registryPath, "collections")

	// Create directory structure
	if err := os.MkdirAll(filepath.Join(registryPath, ".meow"), 0755); err != nil {
		r.harness.t.Fatalf("failed to create registry .meow dir: %v", err)
	}
	if err := os.MkdirAll(collectionsPath, 0755); err != nil {
		r.harness.t.Fatalf("failed to create collections dir: %v", err)
	}

	// Create registry.json
	registryJSON := map[string]any{
		"name":           name,
		"description":    fmt.Sprintf("Test registry: %s", name),
		"version":        version,
		"owner":          map[string]string{"name": "Test Author"},
		"collectionRoot": "./collections",
		"collections":    []any{},
	}

	data, err := json.MarshalIndent(registryJSON, "", "  ")
	if err != nil {
		r.harness.t.Fatalf("failed to marshal registry.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(registryPath, ".meow", "registry.json"), data, 0644); err != nil {
		r.harness.t.Fatalf("failed to write registry.json: %v", err)
	}

	// Initialize as git repo
	r.gitInit(registryPath)

	return &TestRegistry{
		Name:        name,
		Path:        registryPath,
		Version:     version,
		Collections: []string{},
	}
}

// AddCollection adds a collection to the test registry.
func (r *RegistryTestSetup) AddCollection(registry *TestRegistry, collName string) {
	r.harness.t.Helper()

	collPath := filepath.Join(registry.Path, "collections", collName)

	// Create collection directory structure
	if err := os.MkdirAll(filepath.Join(collPath, ".meow"), 0755); err != nil {
		r.harness.t.Fatalf("failed to create collection .meow dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(collPath, "lib"), 0755); err != nil {
		r.harness.t.Fatalf("failed to create collection lib dir: %v", err)
	}

	// Create manifest.json
	manifest := map[string]any{
		"name":        collName,
		"description": fmt.Sprintf("Test collection: %s", collName),
		"entrypoint":  "main.meow.toml",
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		r.harness.t.Fatalf("failed to marshal manifest.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(collPath, ".meow", "manifest.json"), data, 0644); err != nil {
		r.harness.t.Fatalf("failed to write manifest.json: %v", err)
	}

	// Create main.meow.toml (entrypoint)
	mainWorkflow := fmt.Sprintf(`[main]
name = "%s-main"
description = "Main workflow for %s"

[[main.steps]]
id = "hello"
executor = "shell"
command = "echo 'Hello from %s'"
`, collName, collName, collName)
	if err := os.WriteFile(filepath.Join(collPath, "main.meow.toml"), []byte(mainWorkflow), 0644); err != nil {
		r.harness.t.Fatalf("failed to write main.meow.toml: %v", err)
	}

	// Create lib/helper.meow.toml
	helperWorkflow := fmt.Sprintf(`[main]
name = "%s-helper"
description = "Helper workflow for %s"

[[main.steps]]
id = "helper-step"
executor = "shell"
command = "echo 'Helper from %s'"
`, collName, collName, collName)
	if err := os.WriteFile(filepath.Join(collPath, "lib", "helper.meow.toml"), []byte(helperWorkflow), 0644); err != nil {
		r.harness.t.Fatalf("failed to write lib/helper.meow.toml: %v", err)
	}

	// Update registry.json with new collection
	registryJSONPath := filepath.Join(registry.Path, ".meow", "registry.json")
	registryData, err := os.ReadFile(registryJSONPath)
	if err != nil {
		r.harness.t.Fatalf("failed to read registry.json: %v", err)
	}

	var registryJSON map[string]any
	if err := json.Unmarshal(registryData, &registryJSON); err != nil {
		r.harness.t.Fatalf("failed to parse registry.json: %v", err)
	}

	collections := registryJSON["collections"].([]any)
	collections = append(collections, map[string]any{
		"name":        collName,
		"source":      collName,
		"description": fmt.Sprintf("Test collection: %s", collName),
		"tags":        []string{"test"},
	})
	registryJSON["collections"] = collections

	data, err = json.MarshalIndent(registryJSON, "", "  ")
	if err != nil {
		r.harness.t.Fatalf("failed to marshal updated registry.json: %v", err)
	}
	if err := os.WriteFile(registryJSONPath, data, 0644); err != nil {
		r.harness.t.Fatalf("failed to write updated registry.json: %v", err)
	}

	registry.Collections = append(registry.Collections, collName)
}

// AddCollectionWithExpand adds a collection that uses expand to include lib templates.
func (r *RegistryTestSetup) AddCollectionWithExpand(registry *TestRegistry, collName string) {
	r.harness.t.Helper()

	collPath := filepath.Join(registry.Path, "collections", collName)

	// Create collection directory structure
	if err := os.MkdirAll(filepath.Join(collPath, ".meow"), 0755); err != nil {
		r.harness.t.Fatalf("failed to create collection .meow dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(collPath, "lib"), 0755); err != nil {
		r.harness.t.Fatalf("failed to create collection lib dir: %v", err)
	}

	// Create manifest.json
	manifest := map[string]any{
		"name":        collName,
		"description": fmt.Sprintf("Collection with expand: %s", collName),
		"entrypoint":  "main.meow.toml",
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		r.harness.t.Fatalf("failed to marshal manifest.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(collPath, ".meow", "manifest.json"), data, 0644); err != nil {
		r.harness.t.Fatalf("failed to write manifest.json: %v", err)
	}

	// Create main.meow.toml that expands lib/helper
	mainWorkflow := `[main]
name = "expand-main"
description = "Main workflow that expands collection-local template"

[[main.steps]]
id = "greet"
executor = "shell"
command = "echo 'Starting expand test'"

[[main.steps]]
id = "expand-helper"
executor = "expand"
needs = ["greet"]
template = "lib/helper"

[[main.steps]]
id = "done"
executor = "shell"
needs = ["expand-helper"]
command = "echo 'Expand test complete'"
`
	if err := os.WriteFile(filepath.Join(collPath, "main.meow.toml"), []byte(mainWorkflow), 0644); err != nil {
		r.harness.t.Fatalf("failed to write main.meow.toml: %v", err)
	}

	// Create lib/helper.meow.toml (the template being expanded)
	helperWorkflow := `[main]
name = "expanded-helper"
description = "Helper workflow expanded from collection"

[[main.steps]]
id = "expanded-step"
executor = "shell"
command = "echo 'Expanded helper executed - collection-relative expand works!'"
`
	if err := os.WriteFile(filepath.Join(collPath, "lib", "helper.meow.toml"), []byte(helperWorkflow), 0644); err != nil {
		r.harness.t.Fatalf("failed to write lib/helper.meow.toml: %v", err)
	}

	// Update registry.json
	registryJSONPath := filepath.Join(registry.Path, ".meow", "registry.json")
	registryData, err := os.ReadFile(registryJSONPath)
	if err != nil {
		r.harness.t.Fatalf("failed to read registry.json: %v", err)
	}

	var registryJSON map[string]any
	if err := json.Unmarshal(registryData, &registryJSON); err != nil {
		r.harness.t.Fatalf("failed to parse registry.json: %v", err)
	}

	collections := registryJSON["collections"].([]any)
	collections = append(collections, map[string]any{
		"name":        collName,
		"source":      collName,
		"description": fmt.Sprintf("Collection with expand: %s", collName),
		"tags":        []string{"test", "expand"},
	})
	registryJSON["collections"] = collections

	data, err = json.MarshalIndent(registryJSON, "", "  ")
	if err != nil {
		r.harness.t.Fatalf("failed to marshal updated registry.json: %v", err)
	}
	if err := os.WriteFile(registryJSONPath, data, 0644); err != nil {
		r.harness.t.Fatalf("failed to write updated registry.json: %v", err)
	}

	registry.Collections = append(registry.Collections, collName)
}

// CommitRegistry commits all changes in the registry git repo.
func (r *RegistryTestSetup) CommitRegistry(registry *TestRegistry, message string) {
	r.harness.t.Helper()
	r.gitCommit(registry.Path, message)
}

// UpdateRegistryVersion updates the registry version and commits.
func (r *RegistryTestSetup) UpdateRegistryVersion(registry *TestRegistry, newVersion string) {
	r.harness.t.Helper()

	registryJSONPath := filepath.Join(registry.Path, ".meow", "registry.json")
	data, err := os.ReadFile(registryJSONPath)
	if err != nil {
		r.harness.t.Fatalf("failed to read registry.json: %v", err)
	}

	var registryJSON map[string]any
	if err := json.Unmarshal(data, &registryJSON); err != nil {
		r.harness.t.Fatalf("failed to parse registry.json: %v", err)
	}

	registryJSON["version"] = newVersion

	data, err = json.MarshalIndent(registryJSON, "", "  ")
	if err != nil {
		r.harness.t.Fatalf("failed to marshal registry.json: %v", err)
	}
	if err := os.WriteFile(registryJSONPath, data, 0644); err != nil {
		r.harness.t.Fatalf("failed to write registry.json: %v", err)
	}

	registry.Version = newVersion
	r.gitCommit(registry.Path, fmt.Sprintf("Bump version to %s", newVersion))
}

// gitInit initializes a git repository at the given path.
func (r *RegistryTestSetup) gitInit(path string) {
	r.harness.t.Helper()

	cmd := exec.Command("git", "init")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		r.harness.t.Fatalf("git init failed: %v\n%s", err, out)
	}

	// Configure git user for commits
	cmd = exec.Command("git", "config", "user.email", "test@example.com")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		r.harness.t.Fatalf("git config email failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "config", "user.name", "Test User")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		r.harness.t.Fatalf("git config name failed: %v\n%s", err, out)
	}
}

// gitCommit stages all changes and commits.
func (r *RegistryTestSetup) gitCommit(path, message string) {
	r.harness.t.Helper()

	cmd := exec.Command("git", "add", "-A")
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		r.harness.t.Fatalf("git add failed: %v\n%s", err, out)
	}

	cmd = exec.Command("git", "commit", "-m", message)
	cmd.Dir = path
	if out, err := cmd.CombinedOutput(); err != nil {
		r.harness.t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

// RegistriesJSONPath returns the path to the registries.json file.
func (r *RegistryTestSetup) RegistriesJSONPath() string {
	return filepath.Join(r.HomeDir, ".meow", "registries.json")
}

// InstalledJSONPath returns the path to the installed.json file.
func (r *RegistryTestSetup) InstalledJSONPath() string {
	return filepath.Join(r.HomeDir, ".meow", "installed.json")
}

// InstalledCollectionPath returns the path where a collection would be installed.
func (r *RegistryTestSetup) InstalledCollectionPath(collName string) string {
	return filepath.Join(r.HomeDir, ".meow", "workflows", collName)
}

// FileExists checks if a file exists.
func FileExists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

// DirExists checks if a directory exists.
func DirExists(t *testing.T, path string) bool {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}
