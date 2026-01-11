package adapter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

func TestRegistry_LoadFromProject(t *testing.T) {
	// Create temp directories
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "project")
	globalDir := filepath.Join(tempDir, "global")

	// Create project adapter
	projectAdapterDir := filepath.Join(projectDir, "test-agent")
	if err := os.MkdirAll(projectAdapterDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `
[adapter]
name = "test-agent"
description = "Project test agent"

[spawn]
command = "project-agent"
startup_delay = "1s"
`
	if err := os.WriteFile(filepath.Join(projectAdapterDir, "adapter.toml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(globalDir, projectDir)
	config, err := registry.Load("test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Adapter.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", config.Adapter.Name)
	}
	if config.Adapter.Description != "Project test agent" {
		t.Errorf("expected project description, got %q", config.Adapter.Description)
	}
	if config.Spawn.Command != "project-agent" {
		t.Errorf("expected command 'project-agent', got %q", config.Spawn.Command)
	}
}

func TestRegistry_LoadFromGlobal(t *testing.T) {
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "project")
	globalDir := filepath.Join(tempDir, "global")

	// Create global adapter only (no project adapter)
	globalAdapterDir := filepath.Join(globalDir, "test-agent")
	if err := os.MkdirAll(globalAdapterDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `
[adapter]
name = "test-agent"
description = "Global test agent"

[spawn]
command = "global-agent"
`
	if err := os.WriteFile(filepath.Join(globalAdapterDir, "adapter.toml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(globalDir, projectDir)
	config, err := registry.Load("test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Adapter.Description != "Global test agent" {
		t.Errorf("expected global description, got %q", config.Adapter.Description)
	}
}

func TestRegistry_ProjectOverridesGlobal(t *testing.T) {
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "project")
	globalDir := filepath.Join(tempDir, "global")

	// Create both project and global adapters with same name
	projectAdapterDir := filepath.Join(projectDir, "test-agent")
	globalAdapterDir := filepath.Join(globalDir, "test-agent")
	if err := os.MkdirAll(projectAdapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(globalAdapterDir, 0755); err != nil {
		t.Fatal(err)
	}

	projectConfig := `
[adapter]
name = "test-agent"
description = "PROJECT VERSION"

[spawn]
command = "project-agent"
`
	globalConfig := `
[adapter]
name = "test-agent"
description = "GLOBAL VERSION"

[spawn]
command = "global-agent"
`
	if err := os.WriteFile(filepath.Join(projectAdapterDir, "adapter.toml"), []byte(projectConfig), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalAdapterDir, "adapter.toml"), []byte(globalConfig), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(globalDir, projectDir)
	config, err := registry.Load("test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Project should win
	if config.Adapter.Description != "PROJECT VERSION" {
		t.Errorf("expected project to override global, got description %q", config.Adapter.Description)
	}
}

func TestRegistry_LoadBuiltin(t *testing.T) {
	registry := NewRegistry("", "")

	// Register a built-in adapter
	builtinConfig := &types.AdapterConfig{
		Adapter: types.AdapterMeta{
			Name:        "builtin-agent",
			Description: "A built-in adapter",
		},
		Spawn: types.AdapterSpawnConfig{
			Command: "builtin-command",
		},
	}
	registry.RegisterBuiltin("builtin-agent", builtinConfig)

	config, err := registry.Load("builtin-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config.Adapter.Name != "builtin-agent" {
		t.Errorf("expected name 'builtin-agent', got %q", config.Adapter.Name)
	}
}

func TestRegistry_NotFound(t *testing.T) {
	registry := NewRegistry("", "")

	_, err := registry.Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent adapter")
	}

	if !IsNotFound(err) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestRegistry_Cache(t *testing.T) {
	tempDir := t.TempDir()
	adapterDir := filepath.Join(tempDir, "test-agent")
	if err := os.MkdirAll(adapterDir, 0755); err != nil {
		t.Fatal(err)
	}

	configContent := `
[adapter]
name = "test-agent"
description = "Original description"

[spawn]
command = "original-command"
`
	configPath := filepath.Join(adapterDir, "adapter.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(tempDir, "")

	// First load
	config1, err := registry.Load("test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Modify file on disk
	newConfig := `
[adapter]
name = "test-agent"
description = "Modified description"

[spawn]
command = "modified-command"
`
	if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Second load should return cached version
	config2, err := registry.Load("test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config2.Adapter.Description != "Original description" {
		t.Errorf("expected cached config, got %q", config2.Adapter.Description)
	}

	// Same pointer (from cache)
	if config1 != config2 {
		t.Error("expected same cached config pointer")
	}

	// Clear cache and reload
	registry.ClearCache()
	config3, err := registry.Load("test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if config3.Adapter.Description != "Modified description" {
		t.Errorf("expected reloaded config, got %q", config3.Adapter.Description)
	}
}

func TestRegistry_Resolve(t *testing.T) {
	registry := NewRegistry("", "")

	tests := []struct {
		name            string
		stepAdapter     string
		workflowDefault string
		projectDefault  string
		globalDefault   string
		expected        string
	}{
		{
			name:            "step adapter wins",
			stepAdapter:     "step-adapter",
			workflowDefault: "workflow-adapter",
			projectDefault:  "project-adapter",
			globalDefault:   "global-adapter",
			expected:        "step-adapter",
		},
		{
			name:            "workflow default when no step",
			stepAdapter:     "",
			workflowDefault: "workflow-adapter",
			projectDefault:  "project-adapter",
			globalDefault:   "global-adapter",
			expected:        "workflow-adapter",
		},
		{
			name:            "project default when no workflow",
			stepAdapter:     "",
			workflowDefault: "",
			projectDefault:  "project-adapter",
			globalDefault:   "global-adapter",
			expected:        "project-adapter",
		},
		{
			name:            "global default when no project",
			stepAdapter:     "",
			workflowDefault: "",
			projectDefault:  "",
			globalDefault:   "global-adapter",
			expected:        "global-adapter",
		},
		{
			name:            "builtin default when nothing set",
			stepAdapter:     "",
			workflowDefault: "",
			projectDefault:  "",
			globalDefault:   "",
			expected:        "claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := registry.Resolve(tt.stepAdapter, tt.workflowDefault, tt.projectDefault, tt.globalDefault)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRegistry_List(t *testing.T) {
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "project")
	globalDir := filepath.Join(tempDir, "global")

	// Create project adapter
	projectAdapterDir := filepath.Join(projectDir, "project-only")
	if err := os.MkdirAll(projectAdapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectAdapterDir, "adapter.toml"), []byte(`
[adapter]
name = "project-only"
[spawn]
command = "cmd"
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create global adapter
	globalAdapterDir := filepath.Join(globalDir, "global-only")
	if err := os.MkdirAll(globalAdapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalAdapterDir, "adapter.toml"), []byte(`
[adapter]
name = "global-only"
[spawn]
command = "cmd"
`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(globalDir, projectDir)

	// Register a built-in
	registry.RegisterBuiltin("builtin", &types.AdapterConfig{
		Adapter: types.AdapterMeta{Name: "builtin"},
		Spawn:   types.AdapterSpawnConfig{Command: "cmd"},
	})

	names, err := registry.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := map[string]bool{
		"project-only": true,
		"global-only":  true,
		"builtin":      true,
	}

	if len(names) != len(expected) {
		t.Errorf("expected %d adapters, got %d: %v", len(expected), len(names), names)
	}

	for _, name := range names {
		if !expected[name] {
			t.Errorf("unexpected adapter: %s", name)
		}
	}
}

func TestRegistry_GetPath(t *testing.T) {
	tempDir := t.TempDir()
	globalDir := filepath.Join(tempDir, "global")

	// Create adapter
	adapterDir := filepath.Join(globalDir, "test-agent")
	if err := os.MkdirAll(adapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adapterDir, "adapter.toml"), []byte(`
[adapter]
name = "test-agent"
[spawn]
command = "cmd"
`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(globalDir, "")

	// File-based adapter
	path, err := registry.GetPath("test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != adapterDir {
		t.Errorf("expected %s, got %s", adapterDir, path)
	}

	// Built-in adapter
	registry.RegisterBuiltin("builtin", &types.AdapterConfig{
		Adapter: types.AdapterMeta{Name: "builtin"},
		Spawn:   types.AdapterSpawnConfig{Command: "cmd"},
	})
	_, err = registry.GetPath("builtin")
	if !IsBuiltinPath(err) {
		t.Errorf("expected BuiltinPathError, got %T: %v", err, err)
	}

	// Non-existent
	_, err = registry.GetPath("nonexistent")
	if !IsNotFound(err) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

// Note: TestRegistry_ResolvesRelativePaths was removed - event hook configuration
// is now handled by library templates (lib/claude-events.meow.toml), not adapters.

func TestRegistry_InvalidConfig(t *testing.T) {
	tempDir := t.TempDir()
	adapterDir := filepath.Join(tempDir, "invalid-agent")
	if err := os.MkdirAll(adapterDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Missing required field
	configContent := `
[adapter]
name = ""  # Empty name is invalid

[spawn]
command = "cmd"
`
	if err := os.WriteFile(filepath.Join(adapterDir, "adapter.toml"), []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(tempDir, "")
	_, err := registry.Load("invalid-agent")
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestRegistry_ListWithInfo(t *testing.T) {
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "project")
	globalDir := filepath.Join(tempDir, "global")

	// Create project adapter
	projectAdapterDir := filepath.Join(projectDir, "project-only")
	if err := os.MkdirAll(projectAdapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectAdapterDir, "adapter.toml"), []byte(`
[adapter]
name = "project-only"
description = "Project-only adapter"
[spawn]
command = "cmd"
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create global adapter
	globalAdapterDir := filepath.Join(globalDir, "global-only")
	if err := os.MkdirAll(globalAdapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalAdapterDir, "adapter.toml"), []byte(`
[adapter]
name = "global-only"
description = "Global-only adapter"
[spawn]
command = "cmd"
`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(globalDir, projectDir)

	// Register a built-in
	registry.RegisterBuiltin("builtin", &types.AdapterConfig{
		Adapter: types.AdapterMeta{Name: "builtin", Description: "Built-in adapter"},
		Spawn:   types.AdapterSpawnConfig{Command: "cmd"},
	})

	infos, err := registry.ListWithInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(infos) != 3 {
		t.Errorf("expected 3 adapters, got %d", len(infos))
	}

	// Check that we have all sources represented
	sources := make(map[AdapterSource]bool)
	for _, info := range infos {
		sources[info.Source] = true
		if info.Config == nil {
			t.Errorf("adapter %q has nil config", info.Name)
		}
	}

	if !sources[SourceProject] {
		t.Error("missing project adapter")
	}
	if !sources[SourceGlobal] {
		t.Error("missing global adapter")
	}
	if !sources[SourceBuiltin] {
		t.Error("missing built-in adapter")
	}
}

func TestRegistry_ListWithInfo_Override(t *testing.T) {
	tempDir := t.TempDir()
	projectDir := filepath.Join(tempDir, "project")
	globalDir := filepath.Join(tempDir, "global")

	// Create project adapter that overrides global
	projectAdapterDir := filepath.Join(projectDir, "shared-name")
	if err := os.MkdirAll(projectAdapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectAdapterDir, "adapter.toml"), []byte(`
[adapter]
name = "shared-name"
description = "Project version"
[spawn]
command = "project-cmd"
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create global adapter with same name
	globalAdapterDir := filepath.Join(globalDir, "shared-name")
	if err := os.MkdirAll(globalAdapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(globalAdapterDir, "adapter.toml"), []byte(`
[adapter]
name = "shared-name"
description = "Global version"
[spawn]
command = "global-cmd"
`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(globalDir, projectDir)

	infos, err := registry.ListWithInfo()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only have one adapter (project overrides global)
	if len(infos) != 1 {
		t.Errorf("expected 1 adapter, got %d", len(infos))
	}

	info := infos[0]
	if info.Source != SourceProject {
		t.Errorf("expected source %q, got %q", SourceProject, info.Source)
	}
	if info.Description != "Project version" {
		t.Errorf("expected project description, got %q", info.Description)
	}
	if info.Overrides == nil {
		t.Error("expected override info")
	} else if info.Overrides.Source != SourceGlobal {
		t.Errorf("expected override source %q, got %q", SourceGlobal, info.Overrides.Source)
	}
}

func TestRegistry_GetInfo(t *testing.T) {
	tempDir := t.TempDir()
	globalDir := filepath.Join(tempDir, "global")

	// Create adapter
	adapterDir := filepath.Join(globalDir, "test-agent")
	if err := os.MkdirAll(adapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adapterDir, "adapter.toml"), []byte(`
[adapter]
name = "test-agent"
description = "Test agent"
[spawn]
command = "cmd"
`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(globalDir, "")

	// Global adapter
	info, err := registry.GetInfo("test-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Source != SourceGlobal {
		t.Errorf("expected source %q, got %q", SourceGlobal, info.Source)
	}
	if info.Path != adapterDir {
		t.Errorf("expected path %q, got %q", adapterDir, info.Path)
	}

	// Built-in adapter
	registry.RegisterBuiltin("builtin", &types.AdapterConfig{
		Adapter: types.AdapterMeta{Name: "builtin", Description: "Built-in"},
		Spawn:   types.AdapterSpawnConfig{Command: "cmd"},
	})
	info, err = registry.GetInfo("builtin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.Source != SourceBuiltin {
		t.Errorf("expected source %q, got %q", SourceBuiltin, info.Source)
	}
	if info.Path != "" {
		t.Errorf("expected empty path for built-in, got %q", info.Path)
	}

	// Non-existent
	_, err = registry.GetInfo("nonexistent")
	if !IsNotFound(err) {
		t.Errorf("expected NotFoundError, got %T: %v", err, err)
	}
}

func TestRegistry_GetInfo_OverrideBuiltin(t *testing.T) {
	tempDir := t.TempDir()
	globalDir := filepath.Join(tempDir, "global")

	// Create global adapter that overrides built-in
	adapterDir := filepath.Join(globalDir, "builtin")
	if err := os.MkdirAll(adapterDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(adapterDir, "adapter.toml"), []byte(`
[adapter]
name = "builtin"
description = "Custom version"
[spawn]
command = "custom-cmd"
`), 0644); err != nil {
		t.Fatal(err)
	}

	registry := NewRegistry(globalDir, "")
	registry.RegisterBuiltin("builtin", &types.AdapterConfig{
		Adapter: types.AdapterMeta{Name: "builtin", Description: "Original built-in"},
		Spawn:   types.AdapterSpawnConfig{Command: "builtin-cmd"},
	})

	info, err := registry.GetInfo("builtin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if info.Source != SourceGlobal {
		t.Errorf("expected source %q, got %q", SourceGlobal, info.Source)
	}
	if info.Description != "Custom version" {
		t.Errorf("expected custom description, got %q", info.Description)
	}
	if info.Overrides == nil {
		t.Error("expected override info")
	} else if info.Overrides.Source != SourceBuiltin {
		t.Errorf("expected override source %q, got %q", SourceBuiltin, info.Overrides.Source)
	}
}
