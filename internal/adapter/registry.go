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

	// builtins stores embedded adapters
	builtins map[string]*types.AdapterConfig
}

// NewRegistry creates a new adapter registry.
// globalDir is typically ~/.meow/adapters/
// projectDir is typically .meow/adapters/ (can be empty string to disable)
func NewRegistry(globalDir, projectDir string) *Registry {
	return &Registry{
		globalDir:  globalDir,
		projectDir: projectDir,
		cache:      make(map[string]*types.AdapterConfig),
		builtins:   make(map[string]*types.AdapterConfig),
	}
}

// RegisterBuiltin registers a built-in adapter.
// Built-in adapters are used when no file-based adapter is found.
func (r *Registry) RegisterBuiltin(name string, config *types.AdapterConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.builtins[name] = config
}

// Load returns an adapter by name.
// Resolution order:
// 1. Cache (if already loaded)
// 2. Project directory (.meow/adapters/<name>/adapter.toml)
// 3. Global directory (~/.meow/adapters/<name>/adapter.toml)
// 4. Built-in adapters
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

	// Check built-in adapters
	r.mu.RLock()
	if builtin, ok := r.builtins[name]; ok {
		r.mu.RUnlock()
		return builtin, nil
	}
	r.mu.RUnlock()

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
func (r *Registry) resolveRelativePaths(config *types.AdapterConfig, adapterDir string) {
	// Resolve event translator path
	if config.Events.Translator != "" && !filepath.IsAbs(config.Events.Translator) {
		config.Events.Translator = filepath.Join(adapterDir, config.Events.Translator)
	}
}

// Resolve determines the adapter name to use based on the resolution hierarchy.
// Resolution order:
// 1. Step-level adapter (stepAdapter)
// 2. Workflow-level default (workflowDefault)
// 3. Project config (projectDefault)
// 4. Global config (globalDefault)
// 5. Built-in default: "claude"
func (r *Registry) Resolve(stepAdapter, workflowDefault, projectDefault, globalDefault string) string {
	if stepAdapter != "" {
		return stepAdapter
	}
	if workflowDefault != "" {
		return workflowDefault
	}
	if projectDefault != "" {
		return projectDefault
	}
	if globalDefault != "" {
		return globalDefault
	}
	return "claude" // Built-in default
}

// List returns the names of all available adapters.
// This includes adapters from project directory, global directory, and built-ins.
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

	// Add built-ins
	r.mu.RLock()
	for name := range r.builtins {
		if !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	r.mu.RUnlock()

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

	// For built-ins, we don't have a path (they're embedded)
	r.mu.RLock()
	_, isBuiltin := r.builtins[name]
	r.mu.RUnlock()
	if isBuiltin {
		return "", &BuiltinPathError{Name: name}
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

// BuiltinPathError is returned when trying to get the path of a built-in adapter.
type BuiltinPathError struct {
	Name string
}

func (e *BuiltinPathError) Error() string {
	return fmt.Sprintf("adapter %q is built-in and has no filesystem path", e.Name)
}

// IsBuiltinPath returns true if the error is a BuiltinPathError.
func IsBuiltinPath(err error) bool {
	_, ok := err.(*BuiltinPathError)
	return ok
}
