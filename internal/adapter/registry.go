// Package adapter provides the adapter system for MEOW agent orchestration.
// Adapters encapsulate agent-specific behavior (how to start, stop, inject prompts)
// while keeping the orchestrator core agent-agnostic.
package adapter

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/BurntSushi/toml"
	"github.com/meow-stack/meow-machine/internal/types"
)

// Registry manages adapter loading and caching.
// It checks multiple directories in priority order and caches loaded adapters.
type Registry struct {
	mu sync.RWMutex

	// globalDir is ~/.meow/adapters/
	globalDir string
	// projectDir is .meow/adapters/ (optional, project-local overrides)
	projectDir string

	// cache stores loaded adapters by name
	cache map[string]*types.AdapterConfig
}

// NewRegistry creates a new adapter registry.
// globalDir is typically ~/.meow/adapters/
// projectDir is typically .meow/adapters/ (can be empty string to disable)
func NewRegistry(globalDir, projectDir string) *Registry {
	return &Registry{
		globalDir:  globalDir,
		projectDir: projectDir,
		cache:      make(map[string]*types.AdapterConfig),
	}
}

// Load returns an adapter by name.
// Resolution order:
// 1. Cache (if already loaded)
// 2. Project directory (.meow/adapters/<name>/adapter.toml)
// 3. Global directory (~/.meow/adapters/<name>/adapter.toml)
//
// Project adapters override global ones with the same name.
func (r *Registry) Load(name string) (*types.AdapterConfig, error) {
	// Check cache first
	r.mu.RLock()
	if config, ok := r.cache[name]; ok {
		r.mu.RUnlock()
		return config, nil
	}
	r.mu.RUnlock()

	// Try to load from disk
	config, dir, err := r.loadFromDisk(name)
	if err != nil {
		return nil, err
	}

	if config != nil {
		// Resolve relative paths in config
		r.resolveRelativePaths(config, dir)

		// Validate and cache
		if err := config.Validate(); err != nil {
			return nil, fmt.Errorf("adapter %q is invalid: %w", name, err)
		}

		r.mu.Lock()
		r.cache[name] = config
		r.mu.Unlock()
		return config, nil
	}

	return nil, &NotFoundError{Name: name}
}

// loadFromDisk tries to load an adapter from project or global directory.
// Returns the config and the directory it was loaded from.
func (r *Registry) loadFromDisk(name string) (*types.AdapterConfig, string, error) {
	// Try project directory first (higher priority)
	if r.projectDir != "" {
		configPath := filepath.Join(r.projectDir, name, "adapter.toml")
		if _, err := os.Stat(configPath); err == nil {
			config, err := r.loadFile(configPath)
			if err != nil {
				return nil, "", fmt.Errorf("loading project adapter %q: %w", name, err)
			}
			return config, filepath.Dir(configPath), nil
		}
	}

	// Try global directory
	if r.globalDir != "" {
		configPath := filepath.Join(r.globalDir, name, "adapter.toml")
		if _, err := os.Stat(configPath); err == nil {
			config, err := r.loadFile(configPath)
			if err != nil {
				return nil, "", fmt.Errorf("loading global adapter %q: %w", name, err)
			}
			return config, filepath.Dir(configPath), nil
		}
	}

	// Not found on disk
	return nil, "", nil
}

// loadFile parses an adapter.toml file.
func (r *Registry) loadFile(path string) (*types.AdapterConfig, error) {
	var config types.AdapterConfig
	if _, err := toml.DecodeFile(path, &config); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &config, nil
}

// resolveRelativePaths converts relative paths in config to absolute paths.
// Note: Event hook configuration is handled by library templates (lib/claude-events.meow.toml),
// not adapters. This function is retained for future use if adapters need path resolution.
func (r *Registry) resolveRelativePaths(config *types.AdapterConfig, adapterDir string) {
	// Currently no relative paths need resolution in adapters.
	// Event hooks are configured via library templates, not adapter config.
	_ = adapterDir // Silence unused warning
}

// Resolve determines the adapter name to use based on the resolution hierarchy.
// Resolution order:
// 1. Step-level adapter (stepAdapter)
// 2. Workflow-level default (workflowDefault)
// Returns empty string if neither is set.
func (r *Registry) Resolve(stepAdapter, workflowDefault string) string {
	if stepAdapter != "" {
		return stepAdapter
	}
	if workflowDefault != "" {
		return workflowDefault
	}
	return ""
}

// List returns the names of all available adapters.
// This includes adapters from project directory and global directory.
func (r *Registry) List() ([]string, error) {
	seen := make(map[string]bool)
	var names []string

	// List project adapters
	if r.projectDir != "" {
		projectNames, err := r.listDir(r.projectDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("listing project adapters: %w", err)
		}
		for _, name := range projectNames {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	// List global adapters
	if r.globalDir != "" {
		globalNames, err := r.listDir(r.globalDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("listing global adapters: %w", err)
		}
		for _, name := range globalNames {
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	}

	return names, nil
}

// listDir returns adapter names in a directory.
// Each subdirectory with an adapter.toml is considered an adapter.
func (r *Registry) listDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// Check if adapter.toml exists
		configPath := filepath.Join(dir, entry.Name(), "adapter.toml")
		if _, err := os.Stat(configPath); err == nil {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}

// GetPath returns the directory path for an adapter.
// This is useful for resolving paths to scripts like event-translator.sh.
func (r *Registry) GetPath(name string) (string, error) {
	// Check project directory first
	if r.projectDir != "" {
		path := filepath.Join(r.projectDir, name)
		configPath := filepath.Join(path, "adapter.toml")
		if _, err := os.Stat(configPath); err == nil {
			return path, nil
		}
	}

	// Check global directory
	if r.globalDir != "" {
		path := filepath.Join(r.globalDir, name)
		configPath := filepath.Join(path, "adapter.toml")
		if _, err := os.Stat(configPath); err == nil {
			return path, nil
		}
	}

	return "", &NotFoundError{Name: name}
}

// ClearCache clears the adapter cache.
// This forces re-loading from disk on next Load call.
func (r *Registry) ClearCache() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = make(map[string]*types.AdapterConfig)
}

// NotFoundError is returned when an adapter cannot be found.
type NotFoundError struct {
	Name string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("adapter %q not found", e.Name)
}

// IsNotFound returns true if the error is a NotFoundError.
func IsNotFound(err error) bool {
	_, ok := err.(*NotFoundError)
	return ok
}

// AdapterSource represents where an adapter was loaded from.
type AdapterSource string

const (
	// SourceGlobal indicates an adapter from ~/.meow/adapters/.
	SourceGlobal AdapterSource = "global"
	// SourceProject indicates an adapter from .meow/adapters/.
	SourceProject AdapterSource = "project"
)

// AdapterInfo contains information about an adapter including its source.
type AdapterInfo struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Source      AdapterSource         `json:"source"`
	Path        string                `json:"path,omitempty"`
	Config      *types.AdapterConfig  `json:"config"`
	Overrides   *AdapterOverrideInfo  `json:"overrides,omitempty"` // What this adapter is overriding
}

// AdapterOverrideInfo describes what an adapter is overriding.
type AdapterOverrideInfo struct {
	Source AdapterSource `json:"source"`
	Path   string        `json:"path,omitempty"`
}

// ListWithInfo returns detailed information about all available adapters.
// Unlike List(), this provides the source location and override information.
func (r *Registry) ListWithInfo() ([]AdapterInfo, error) {
	type adapterLocation struct {
		name   string
		source AdapterSource
		path   string
	}

	// Collect all adapter locations (project, global)
	var locations []adapterLocation

	// Project adapters
	if r.projectDir != "" {
		projectNames, err := r.listDir(r.projectDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("listing project adapters: %w", err)
		}
		for _, name := range projectNames {
			locations = append(locations, adapterLocation{
				name:   name,
				source: SourceProject,
				path:   filepath.Join(r.projectDir, name),
			})
		}
	}

	// Global adapters
	if r.globalDir != "" {
		globalNames, err := r.listDir(r.globalDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("listing global adapters: %w", err)
		}
		for _, name := range globalNames {
			locations = append(locations, adapterLocation{
				name:   name,
				source: SourceGlobal,
				path:   filepath.Join(r.globalDir, name),
			})
		}
	}

	// Build result map - first occurrence wins (project > global)
	seen := make(map[string]bool)
	overrides := make(map[string][]adapterLocation) // Track what each name overrides

	for _, loc := range locations {
		if !seen[loc.name] {
			seen[loc.name] = true
		} else {
			// This is an override - track the overridden location
			overrides[loc.name] = append(overrides[loc.name], loc)
		}
	}

	// Build the result
	var result []AdapterInfo
	seen = make(map[string]bool) // Reset for iteration

	for _, loc := range locations {
		if seen[loc.name] {
			continue // Skip duplicates - only keep the first (highest priority)
		}
		seen[loc.name] = true

		// Load the config
		config, err := r.Load(loc.name)
		if err != nil {
			return nil, fmt.Errorf("loading adapter %q: %w", loc.name, err)
		}

		info := AdapterInfo{
			Name:        loc.name,
			Description: config.Adapter.Description,
			Source:      loc.source,
			Path:        loc.path,
			Config:      config,
		}

		// Check for overrides
		if ov, ok := overrides[loc.name]; ok && len(ov) > 0 {
			// The first override is what the current adapter is overriding
			info.Overrides = &AdapterOverrideInfo{
				Source: ov[0].source,
				Path:   ov[0].path,
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// GetInfo returns detailed information about a specific adapter.
func (r *Registry) GetInfo(name string) (*AdapterInfo, error) {
	config, err := r.Load(name)
	if err != nil {
		return nil, err
	}

	// Determine the source
	var source AdapterSource
	var path string
	var overrides *AdapterOverrideInfo

	// Check project first
	if r.projectDir != "" {
		configPath := filepath.Join(r.projectDir, name, "adapter.toml")
		if _, err := os.Stat(configPath); err == nil {
			source = SourceProject
			path = filepath.Join(r.projectDir, name)

			// Check if it overrides global
			if r.globalDir != "" {
				globalPath := filepath.Join(r.globalDir, name, "adapter.toml")
				if _, err := os.Stat(globalPath); err == nil {
					overrides = &AdapterOverrideInfo{
						Source: SourceGlobal,
						Path:   filepath.Join(r.globalDir, name),
					}
				}
			}
		}
	}

	// Check global
	if source == "" && r.globalDir != "" {
		configPath := filepath.Join(r.globalDir, name, "adapter.toml")
		if _, err := os.Stat(configPath); err == nil {
			source = SourceGlobal
			path = filepath.Join(r.globalDir, name)

		}
	}
	if source == "" {
		return nil, &NotFoundError{Name: name}
	}

	return &AdapterInfo{
		Name:        name,
		Description: config.Adapter.Description,
		Source:      source,
		Path:        path,
		Config:      config,
		Overrides:   overrides,
	}, nil
}
