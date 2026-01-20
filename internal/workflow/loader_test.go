package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeModuleFile(t *testing.T, path string, description string, includeHelper bool) {
	t.Helper()

	content := fmt.Sprintf(`
[main]
name = "main"
description = %q

[[main.steps]]
id = "step1"
executor = "shell"
command = "echo hello"
`, description)

	if includeHelper {
		content += `
[helper]
name = "helper"
description = "helper workflow"
internal = true

[[helper.steps]]
id = "helper-step"
executor = "shell"
command = "echo helper"
`
	}

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}
}

func TestLoader_LoadWorkflow_ProjectOverridesUser(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	projectPath := filepath.Join(projectDir, ".meow", "workflows", "shared.meow.toml")
	userPath := filepath.Join(userHome, ".meow", "workflows", "shared.meow.toml")

	writeModuleFile(t, projectPath, "project version", false)
	writeModuleFile(t, userPath, "user version", false)

	loader := NewLoader(projectDir)
	result, err := loader.LoadWorkflow("shared")
	if err != nil {
		t.Fatalf("LoadWorkflow failed: %v", err)
	}

	if result.Source != "project" {
		t.Fatalf("Source = %s, want project", result.Source)
	}
	if result.Path != projectPath {
		t.Fatalf("Path = %s, want %s", result.Path, projectPath)
	}
	if result.Workflow.Description != "project version" {
		t.Fatalf("Description = %s, want project version", result.Workflow.Description)
	}
}

func TestLoader_LoadWorkflow_UserFallback(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	userPath := filepath.Join(userHome, ".meow", "workflows", "global.meow.toml")
	writeModuleFile(t, userPath, "user workflow", false)

	loader := NewLoader(projectDir)
	result, err := loader.LoadWorkflow("global")
	if err != nil {
		t.Fatalf("LoadWorkflow failed: %v", err)
	}

	if result.Source != "user" {
		t.Fatalf("Source = %s, want user", result.Source)
	}
	if result.Path != userPath {
		t.Fatalf("Path = %s, want %s", result.Path, userPath)
	}
}

func TestLoader_LoadWorkflow_Subdirectory(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	workflowPath := filepath.Join(projectDir, ".meow", "workflows", "lib", "tool.meow.toml")
	writeModuleFile(t, workflowPath, "lib workflow", false)

	loader := NewLoader(projectDir)
	result, err := loader.LoadWorkflow("lib/tool")
	if err != nil {
		t.Fatalf("LoadWorkflow failed: %v", err)
	}

	if result.Path != workflowPath {
		t.Fatalf("Path = %s, want %s", result.Path, workflowPath)
	}
}

func TestLoader_LoadWorkflow_WithSection(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	workflowPath := filepath.Join(projectDir, ".meow", "workflows", "multi.meow.toml")
	writeModuleFile(t, workflowPath, "main workflow", true)

	loader := NewLoader(projectDir)
	result, err := loader.LoadWorkflow("multi#helper")
	if err != nil {
		t.Fatalf("LoadWorkflow failed: %v", err)
	}

	if result.Name != "helper" {
		t.Fatalf("Name = %s, want helper", result.Name)
	}
	if result.Workflow.Name != "helper" {
		t.Fatalf("Workflow.Name = %s, want helper", result.Workflow.Name)
	}
}

func TestLoader_LoadWorkflow_NotFound(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	loader := NewLoader(projectDir)
	_, err := loader.LoadWorkflow("missing")
	if err == nil {
		t.Fatal("Expected error for missing workflow")
	}

	notFoundErr, ok := err.(*WorkflowNotFoundError)
	if !ok {
		t.Fatalf("Expected WorkflowNotFoundError, got %T", err)
	}

	if notFoundErr.Ref != "missing" {
		t.Fatalf("Ref = %s, want missing", notFoundErr.Ref)
	}
}

func TestNewLoader(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	loader := NewLoader(projectDir)
	if loader.ProjectDir != projectDir {
		t.Fatalf("ProjectDir = %s, want %s", loader.ProjectDir, projectDir)
	}

	expectedUserDir := filepath.Join(userHome, ".meow")
	if loader.UserDir != expectedUserDir {
		t.Fatalf("UserDir = %s, want %s", loader.UserDir, expectedUserDir)
	}

	if loader.EmbeddedDir != "workflows" {
		t.Fatalf("EmbeddedDir = %s, want workflows", loader.EmbeddedDir)
	}
}

func TestWorkflowNotFoundError_Error(t *testing.T) {
	err := &WorkflowNotFoundError{
		Ref:      "missing",
		Searched: []string{"/a/b/c.meow.toml", "/x/y/z.meow.toml"},
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}
}

func TestWorkflowNotFoundError_WithScope(t *testing.T) {
	err := &WorkflowNotFoundError{
		Ref:      "missing",
		Searched: []string{"/a/b/c.meow.toml"},
		Scope:    ScopeUser,
	}

	msg := err.Error()
	if !strings.Contains(msg, "scope: user") {
		t.Errorf("Error message should contain scope info: %s", msg)
	}
}

// Scope-aware loader tests

func TestNewLoaderWithScope(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	loader := NewLoaderWithScope(projectDir, ScopeUser)
	if loader.Scope != ScopeUser {
		t.Fatalf("Scope = %s, want user", loader.Scope)
	}
}

func TestLoader_ScopeUser_SkipsProject(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	// Create workflow in both locations
	projectPath := filepath.Join(projectDir, ".meow", "workflows", "shared.meow.toml")
	userPath := filepath.Join(userHome, ".meow", "workflows", "shared.meow.toml")

	writeModuleFile(t, projectPath, "project version", false)
	writeModuleFile(t, userPath, "user version", false)

	// User scope should skip project and find user version
	loader := NewLoaderWithScope(projectDir, ScopeUser)
	result, err := loader.LoadWorkflow("shared")
	if err != nil {
		t.Fatalf("LoadWorkflow failed: %v", err)
	}

	if result.Source != "user" {
		t.Fatalf("Source = %s, want user (should skip project)", result.Source)
	}
	if result.Workflow.Description != "user version" {
		t.Fatalf("Description = %s, want 'user version'", result.Workflow.Description)
	}
}

func TestLoader_ScopeUser_NotFoundWhenOnlyInProject(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	// Create workflow only in project
	projectPath := filepath.Join(projectDir, ".meow", "workflows", "project-only.meow.toml")
	writeModuleFile(t, projectPath, "project only", false)

	// User scope should not find it
	loader := NewLoaderWithScope(projectDir, ScopeUser)
	_, err := loader.LoadWorkflow("project-only")
	if err == nil {
		t.Fatal("Expected error: workflow exists only in project scope")
	}

	notFoundErr, ok := err.(*WorkflowNotFoundError)
	if !ok {
		t.Fatalf("Expected WorkflowNotFoundError, got %T", err)
	}
	if notFoundErr.Scope != ScopeUser {
		t.Fatalf("Scope in error = %s, want user", notFoundErr.Scope)
	}
}

func TestLoader_ScopeProject_FindsBoth(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()

	t.Setenv("HOME", userHome)

	// Create workflow only in user
	userPath := filepath.Join(userHome, ".meow", "workflows", "user-only.meow.toml")
	writeModuleFile(t, userPath, "user only", false)

	// Project scope should still find user workflows as fallback
	loader := NewLoaderWithScope(projectDir, ScopeProject)
	result, err := loader.LoadWorkflow("user-only")
	if err != nil {
		t.Fatalf("LoadWorkflow failed: %v", err)
	}

	if result.Source != "user" {
		t.Fatalf("Source = %s, want user (project scope falls back to user)", result.Source)
	}
}

func TestScope_SearchMethods(t *testing.T) {
	tests := []struct {
		scope           Scope
		searchesProject bool
		searchesUser    bool
		searchesEmbed   bool
	}{
		{"", true, true, true},
		{ScopeProject, true, true, true},
		{ScopeUser, false, true, true},
		{ScopeEmbedded, false, false, true},
	}

	for _, tc := range tests {
		t.Run(string(tc.scope), func(t *testing.T) {
			if tc.scope.SearchesProject() != tc.searchesProject {
				t.Errorf("SearchesProject() = %v, want %v", tc.scope.SearchesProject(), tc.searchesProject)
			}
			if tc.scope.SearchesUser() != tc.searchesUser {
				t.Errorf("SearchesUser() = %v, want %v", tc.scope.SearchesUser(), tc.searchesUser)
			}
			if tc.scope.SearchesEmbedded() != tc.searchesEmbed {
				t.Errorf("SearchesEmbedded() = %v, want %v", tc.scope.SearchesEmbedded(), tc.searchesEmbed)
			}
		})
	}
}

func TestScope_Valid(t *testing.T) {
	validScopes := []Scope{"", ScopeProject, ScopeUser, ScopeEmbedded}
	for _, s := range validScopes {
		if !s.Valid() {
			t.Errorf("Scope %q should be valid", s)
		}
	}

	invalidScopes := []Scope{"invalid", "local", "global"}
	for _, s := range invalidScopes {
		if s.Valid() {
			t.Errorf("Scope %q should be invalid", s)
		}
	}
}

// LoadContext tests

func TestNewLoadContext(t *testing.T) {
	ctx := NewLoadContext("/path/to/file.toml")

	if ctx.FilePath != "/path/to/file.toml" {
		t.Errorf("FilePath = %s, want /path/to/file.toml", ctx.FilePath)
	}
	if ctx.Module != nil {
		t.Error("Module should be nil initially")
	}
	if ctx.Depth() != 0 {
		t.Errorf("Depth() = %d, want 0", ctx.Depth())
	}
	if ctx.CurrentRef() != "" {
		t.Errorf("CurrentRef() = %s, want empty", ctx.CurrentRef())
	}
}

func TestLoadContext_Enter_Exit(t *testing.T) {
	ctx := NewLoadContext("/path/to/file.toml")

	// Enter first reference
	if err := ctx.Enter("file1.toml#main"); err != nil {
		t.Fatalf("Enter failed: %v", err)
	}
	if ctx.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", ctx.Depth())
	}
	if ctx.CurrentRef() != "file1.toml#main" {
		t.Errorf("CurrentRef() = %s, want file1.toml#main", ctx.CurrentRef())
	}

	// Enter second reference
	if err := ctx.Enter("file2.toml#helper"); err != nil {
		t.Fatalf("Enter failed: %v", err)
	}
	if ctx.Depth() != 2 {
		t.Errorf("Depth() = %d, want 2", ctx.Depth())
	}
	if ctx.CurrentRef() != "file2.toml#helper" {
		t.Errorf("CurrentRef() = %s, want file2.toml#helper", ctx.CurrentRef())
	}

	// Exit second reference
	ctx.Exit("file2.toml#helper")
	if ctx.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1", ctx.Depth())
	}
	if ctx.CurrentRef() != "file1.toml#main" {
		t.Errorf("CurrentRef() = %s, want file1.toml#main", ctx.CurrentRef())
	}

	// Exit first reference
	ctx.Exit("file1.toml#main")
	if ctx.Depth() != 0 {
		t.Errorf("Depth() = %d, want 0", ctx.Depth())
	}
	if ctx.CurrentRef() != "" {
		t.Errorf("CurrentRef() = %s, want empty", ctx.CurrentRef())
	}
}

func TestLoadContext_CycleDetection(t *testing.T) {
	ctx := NewLoadContext("/path/to/file.toml")

	// Enter chain: file1 -> file2 -> file3
	if err := ctx.Enter("file1.toml#main"); err != nil {
		t.Fatalf("Enter file1 failed: %v", err)
	}
	if err := ctx.Enter("file2.toml#helper"); err != nil {
		t.Fatalf("Enter file2 failed: %v", err)
	}
	if err := ctx.Enter("file3.toml#util"); err != nil {
		t.Fatalf("Enter file3 failed: %v", err)
	}

	// Try to enter file1 again - should detect cycle
	err := ctx.Enter("file1.toml#main")
	if err == nil {
		t.Fatal("Expected cycle detection error")
	}

	circErr, ok := err.(*CircularReferenceError)
	if !ok {
		t.Fatalf("Expected CircularReferenceError, got %T: %v", err, err)
	}

	if circErr.Reference != "file1.toml#main" {
		t.Errorf("Reference = %s, want file1.toml#main", circErr.Reference)
	}

	// Check that error message contains cycle path
	errMsg := err.Error()
	if !strings.Contains(errMsg, "file1.toml#main") {
		t.Errorf("Error should contain 'file1.toml#main': %s", errMsg)
	}
	if !strings.Contains(errMsg, "file2.toml#helper") {
		t.Errorf("Error should contain 'file2.toml#helper': %s", errMsg)
	}
}

func TestLoadContext_SelfReference(t *testing.T) {
	ctx := NewLoadContext("/path/to/file.toml")

	// Enter a reference
	if err := ctx.Enter("file.toml#workflow"); err != nil {
		t.Fatalf("Enter failed: %v", err)
	}

	// Try to enter the same reference - self-reference cycle
	err := ctx.Enter("file.toml#workflow")
	if err == nil {
		t.Fatal("Expected cycle detection for self-reference")
	}

	if _, ok := err.(*CircularReferenceError); !ok {
		t.Errorf("Expected CircularReferenceError, got %T", err)
	}
}

func TestLoadContext_Child(t *testing.T) {
	parent := NewLoadContext("/path/to/parent.toml")

	// Enter a reference in parent
	if err := parent.Enter("parent.toml#main"); err != nil {
		t.Fatalf("Enter in parent failed: %v", err)
	}

	// Create child context
	child := parent.Child("/path/to/child.toml")

	// Child should have different file path
	if child.FilePath != "/path/to/child.toml" {
		t.Errorf("child.FilePath = %s, want /path/to/child.toml", child.FilePath)
	}

	// Child should share visited set with parent
	if err := child.Enter("child.toml#helper"); err != nil {
		t.Fatalf("Enter in child failed: %v", err)
	}

	// Parent's visited set should see child's entry (shared)
	err := parent.Enter("child.toml#helper")
	if err == nil {
		t.Fatal("Expected cycle detection - child added to shared visited set")
	}

	// Child trying to enter parent's ref should also detect cycle
	err = child.Enter("parent.toml#main")
	if err == nil {
		t.Fatal("Expected cycle detection - parent added to shared visited set")
	}
}

func TestCircularReferenceError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *CircularReferenceError
		contains []string
	}{
		{
			name: "simple cycle",
			err: &CircularReferenceError{
				Reference: "file.toml#main",
				Path:      []string{"file.toml#main"},
			},
			contains: []string{"circular reference", "file.toml#main"},
		},
		{
			name: "longer cycle",
			err: &CircularReferenceError{
				Reference: "a.toml#x",
				Path:      []string{"a.toml#x", "b.toml#y", "c.toml#z", "a.toml#x"},
			},
			contains: []string{"a.toml#x", "b.toml#y", "c.toml#z", "→"},
		},
		{
			name: "empty path",
			err: &CircularReferenceError{
				Reference: "file.toml",
				Path:      nil,
			},
			contains: []string{"circular reference", "file.toml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			for _, s := range tt.contains {
				if !strings.Contains(msg, s) {
					t.Errorf("Error message should contain %q: %s", s, msg)
				}
			}
		})
	}
}

func TestLoadContext_ExitWrongRef(t *testing.T) {
	ctx := NewLoadContext("/path/to/file.toml")

	// Enter a reference
	if err := ctx.Enter("file.toml#main"); err != nil {
		t.Fatalf("Enter failed: %v", err)
	}

	// Exit with wrong ref - should not pop
	ctx.Exit("wrong.toml#other")
	if ctx.Depth() != 1 {
		t.Errorf("Depth() = %d, want 1 (exit with wrong ref should not pop)", ctx.Depth())
	}

	// Exit with correct ref - should pop
	ctx.Exit("file.toml#main")
	if ctx.Depth() != 0 {
		t.Errorf("Depth() = %d, want 0", ctx.Depth())
	}
}

func TestLoadContext_WithModule(t *testing.T) {
	ctx := NewLoadContext("/path/to/module.meow.toml")

	// Module is nil initially
	if ctx.Module != nil {
		t.Error("Module should be nil initially")
	}

	// Set module after parsing
	ctx.Module = &Module{
		Path: "/path/to/module.meow.toml",
		Workflows: map[string]*Workflow{
			"main": {Name: "main"},
		},
	}

	// Now module is accessible
	if ctx.Module == nil {
		t.Error("Module should not be nil after setting")
	}
	if ctx.Module.Workflows["main"] == nil {
		t.Error("Module should have main workflow")
	}
}

func TestLoadContext_DiamondDependency(t *testing.T) {
	// Test diamond dependency pattern: A→B, A→C→B
	// B should be loadable from both A directly and through C
	ctx := NewLoadContext("/path/to/a.toml")

	// A enters
	if err := ctx.Enter("a.toml#main"); err != nil {
		t.Fatalf("Enter A failed: %v", err)
	}

	// A loads B
	childB := ctx.Child("/path/to/b.toml")
	if err := childB.Enter("b.toml#helper"); err != nil {
		t.Fatalf("Enter B (first time) failed: %v", err)
	}
	// B completes
	childB.Exit("b.toml#helper")

	// A loads C
	childC := ctx.Child("/path/to/c.toml")
	if err := childC.Enter("c.toml#util"); err != nil {
		t.Fatalf("Enter C failed: %v", err)
	}

	// C loads B (should succeed - diamond dependency, not a cycle)
	grandchildB := childC.Child("/path/to/b.toml")
	if err := grandchildB.Enter("b.toml#helper"); err != nil {
		t.Errorf("Enter B (second time, from C) should succeed for diamond dependency: %v", err)
	}
}

func TestLoadContext_ExitAllowsReentry(t *testing.T) {
	// After Exit, the same ref should be enterable again
	ctx := NewLoadContext("/path/to/file.toml")

	// Enter and exit
	if err := ctx.Enter("file.toml#workflow"); err != nil {
		t.Fatalf("First Enter failed: %v", err)
	}
	ctx.Exit("file.toml#workflow")

	// Should be able to enter again
	if err := ctx.Enter("file.toml#workflow"); err != nil {
		t.Errorf("Second Enter after Exit should succeed: %v", err)
	}
}

func TestLoadContext_ChildStackIndependence(t *testing.T) {
	// Verify child has independent stack (copy, not shared)
	parent := NewLoadContext("/path/to/parent.toml")

	if err := parent.Enter("parent.toml#main"); err != nil {
		t.Fatalf("Parent Enter failed: %v", err)
	}

	child := parent.Child("/path/to/child.toml")

	// Child enters something
	if err := child.Enter("child.toml#helper"); err != nil {
		t.Fatalf("Child Enter failed: %v", err)
	}

	// Parent stack should still be depth 1
	if parent.Depth() != 1 {
		t.Errorf("Parent depth = %d after child Enter, want 1", parent.Depth())
	}

	// Child stack should be depth 2
	if child.Depth() != 2 {
		t.Errorf("Child depth = %d, want 2", child.Depth())
	}

	// Parent's current ref should still be its own
	if parent.CurrentRef() != "parent.toml#main" {
		t.Errorf("Parent CurrentRef = %s, want parent.toml#main", parent.CurrentRef())
	}

	// Child's current ref should be its own
	if child.CurrentRef() != "child.toml#helper" {
		t.Errorf("Child CurrentRef = %s, want child.toml#helper", child.CurrentRef())
	}
}

// Collection resolution tests

func writeManifest(t *testing.T, dir string, name string, entrypoint string) {
	t.Helper()

	manifest := fmt.Sprintf(`{
  "name": %q,
  "description": "Test collection",
  "entrypoint": %q
}`, name, entrypoint)

	metaDir := filepath.Join(dir, ".meow")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatalf("failed to create .meow dir: %v", err)
	}

	manifestPath := filepath.Join(metaDir, "manifest.json")
	if err := os.WriteFile(manifestPath, []byte(manifest), 0644); err != nil {
		t.Fatalf("failed to write manifest: %v", err)
	}
}

func TestLoader_ResolveWorkflow_CollectionWithEntrypoint(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a collection directory with manifest
	collectionDir := filepath.Join(projectDir, ".meow", "workflows", "sprint")
	writeManifest(t, collectionDir, "sprint", "main.meow.toml")

	// Create the entrypoint workflow
	entrypointPath := filepath.Join(collectionDir, "main.meow.toml")
	writeModuleFile(t, entrypointPath, "sprint main workflow", false)

	loader := NewLoader(projectDir)
	result, err := loader.ResolveWorkflow("sprint")
	if err != nil {
		t.Fatalf("ResolveWorkflow failed: %v", err)
	}

	if result.Source != "project-collection" {
		t.Errorf("Source = %s, want project-collection", result.Source)
	}
	if result.Path != entrypointPath {
		t.Errorf("Path = %s, want %s", result.Path, entrypointPath)
	}
	if result.Name != "main" {
		t.Errorf("Name = %s, want main", result.Name)
	}
	if result.CollectionDir != collectionDir {
		t.Errorf("CollectionDir = %s, want %s", result.CollectionDir, collectionDir)
	}
}

func TestLoader_ResolveWorkflow_CollectionWithPath(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a collection with lib/ subdirectory
	collectionDir := filepath.Join(projectDir, ".meow", "workflows", "sprint")
	writeManifest(t, collectionDir, "sprint", "main.meow.toml")

	// Create a workflow in lib/
	libWorkflowPath := filepath.Join(collectionDir, "lib", "agent-track.meow.toml")
	writeModuleFile(t, libWorkflowPath, "agent track workflow", false)

	loader := NewLoader(projectDir)
	result, err := loader.ResolveWorkflow("sprint:lib/agent-track")
	if err != nil {
		t.Fatalf("ResolveWorkflow failed: %v", err)
	}

	if result.Source != "project-collection" {
		t.Errorf("Source = %s, want project-collection", result.Source)
	}
	if result.Path != libWorkflowPath {
		t.Errorf("Path = %s, want %s", result.Path, libWorkflowPath)
	}
	if result.CollectionDir != collectionDir {
		t.Errorf("CollectionDir = %s, want %s", result.CollectionDir, collectionDir)
	}
}

func TestLoader_ResolveWorkflow_CollectionWithSection(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a collection
	collectionDir := filepath.Join(projectDir, ".meow", "workflows", "sprint")
	writeManifest(t, collectionDir, "sprint", "main.meow.toml")

	// Create entrypoint with helper workflow
	entrypointPath := filepath.Join(collectionDir, "main.meow.toml")
	writeModuleFile(t, entrypointPath, "sprint main", true)

	loader := NewLoader(projectDir)
	result, err := loader.ResolveWorkflow("sprint#helper")
	if err != nil {
		t.Fatalf("ResolveWorkflow failed: %v", err)
	}

	if result.Name != "helper" {
		t.Errorf("Name = %s, want helper", result.Name)
	}
	if result.Path != entrypointPath {
		t.Errorf("Path = %s, want %s", result.Path, entrypointPath)
	}
}

func TestLoader_ResolveWorkflow_CollectionWithPathAndSection(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a collection
	collectionDir := filepath.Join(projectDir, ".meow", "workflows", "sprint")
	writeManifest(t, collectionDir, "sprint", "main.meow.toml")

	// Create lib workflow with multiple sections
	libWorkflowPath := filepath.Join(collectionDir, "lib", "tools.meow.toml")
	writeModuleFile(t, libWorkflowPath, "tools main", true)

	loader := NewLoader(projectDir)
	result, err := loader.ResolveWorkflow("sprint:lib/tools#helper")
	if err != nil {
		t.Fatalf("ResolveWorkflow failed: %v", err)
	}

	if result.Name != "helper" {
		t.Errorf("Name = %s, want helper", result.Name)
	}
	if result.Path != libWorkflowPath {
		t.Errorf("Path = %s, want %s", result.Path, libWorkflowPath)
	}
	if result.CollectionDir != collectionDir {
		t.Errorf("CollectionDir = %s, want %s", result.CollectionDir, collectionDir)
	}
}

func TestLoader_ResolveWorkflow_CollectionPrecedenceOverStandalone(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create both a collection and a standalone file with the same name
	collectionDir := filepath.Join(projectDir, ".meow", "workflows", "shared")
	writeManifest(t, collectionDir, "shared", "main.meow.toml")
	collectionEntrypoint := filepath.Join(collectionDir, "main.meow.toml")
	writeModuleFile(t, collectionEntrypoint, "collection version", false)

	standalonePath := filepath.Join(projectDir, ".meow", "workflows", "shared.meow.toml")
	writeModuleFile(t, standalonePath, "standalone version", false)

	loader := NewLoader(projectDir)
	result, err := loader.ResolveWorkflow("shared")
	if err != nil {
		t.Fatalf("ResolveWorkflow failed: %v", err)
	}

	// Collection should take precedence
	if result.Source != "project-collection" {
		t.Errorf("Source = %s, want project-collection (collections have precedence)", result.Source)
	}
	if result.Path != collectionEntrypoint {
		t.Errorf("Path = %s, want %s", result.Path, collectionEntrypoint)
	}
}

func TestLoader_ResolveWorkflow_ProjectCollectionOverUserCollection(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create collection in both project and user
	projectCollectionDir := filepath.Join(projectDir, ".meow", "workflows", "shared")
	writeManifest(t, projectCollectionDir, "shared", "main.meow.toml")
	projectEntrypoint := filepath.Join(projectCollectionDir, "main.meow.toml")
	writeModuleFile(t, projectEntrypoint, "project collection", false)

	userCollectionDir := filepath.Join(userHome, ".meow", "workflows", "shared")
	writeManifest(t, userCollectionDir, "shared", "main.meow.toml")
	userEntrypoint := filepath.Join(userCollectionDir, "main.meow.toml")
	writeModuleFile(t, userEntrypoint, "user collection", false)

	loader := NewLoader(projectDir)
	result, err := loader.ResolveWorkflow("shared")
	if err != nil {
		t.Fatalf("ResolveWorkflow failed: %v", err)
	}

	// Project collection should win
	if result.Source != "project-collection" {
		t.Errorf("Source = %s, want project-collection", result.Source)
	}
	if result.Path != projectEntrypoint {
		t.Errorf("Path = %s, want %s", result.Path, projectEntrypoint)
	}
}

func TestLoader_ResolveWorkflow_UserCollectionFallback(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create collection only in user
	userCollectionDir := filepath.Join(userHome, ".meow", "workflows", "global")
	writeManifest(t, userCollectionDir, "global", "main.meow.toml")
	userEntrypoint := filepath.Join(userCollectionDir, "main.meow.toml")
	writeModuleFile(t, userEntrypoint, "user collection", false)

	loader := NewLoader(projectDir)
	result, err := loader.ResolveWorkflow("global")
	if err != nil {
		t.Fatalf("ResolveWorkflow failed: %v", err)
	}

	if result.Source != "user-collection" {
		t.Errorf("Source = %s, want user-collection", result.Source)
	}
	if result.Path != userEntrypoint {
		t.Errorf("Path = %s, want %s", result.Path, userEntrypoint)
	}
}

func TestLoader_ResolveWorkflow_DirectoryWithoutManifestNotCollection(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a directory without manifest (just a regular directory)
	dirWithoutManifest := filepath.Join(projectDir, ".meow", "workflows", "notacollection")
	workflowInDir := filepath.Join(dirWithoutManifest, "main.meow.toml")
	writeModuleFile(t, workflowInDir, "not a collection", false)

	// Also create a standalone file as fallback
	standalonePath := filepath.Join(projectDir, ".meow", "workflows", "notacollection.meow.toml")
	writeModuleFile(t, standalonePath, "standalone fallback", false)

	loader := NewLoader(projectDir)
	result, err := loader.ResolveWorkflow("notacollection")
	if err != nil {
		t.Fatalf("ResolveWorkflow failed: %v", err)
	}

	// Should fall back to standalone file (no manifest, so not a collection)
	if result.Source != "project" {
		t.Errorf("Source = %s, want project (directory without manifest is not a collection)", result.Source)
	}
	if result.Path != standalonePath {
		t.Errorf("Path = %s, want %s", result.Path, standalonePath)
	}
}

func TestLoader_ResolveWorkflow_CollectionMissingEntrypointFile(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create collection with manifest but missing entrypoint file
	collectionDir := filepath.Join(projectDir, ".meow", "workflows", "broken")
	writeManifest(t, collectionDir, "broken", "missing.meow.toml")
	// Don't create the actual workflow file

	// Create a standalone fallback
	standalonePath := filepath.Join(projectDir, ".meow", "workflows", "broken.meow.toml")
	writeModuleFile(t, standalonePath, "standalone fallback", false)

	loader := NewLoader(projectDir)
	result, err := loader.ResolveWorkflow("broken")
	if err != nil {
		t.Fatalf("ResolveWorkflow failed: %v", err)
	}

	// Should fall back to standalone (collection entrypoint doesn't exist)
	if result.Source != "project" {
		t.Errorf("Source = %s, want project (collection entrypoint missing)", result.Source)
	}
	if result.Path != standalonePath {
		t.Errorf("Path = %s, want %s", result.Path, standalonePath)
	}
}

func TestLoader_ResolveWorkflow_CollectionWithPathNotFound(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create collection but without the requested path
	collectionDir := filepath.Join(projectDir, ".meow", "workflows", "sprint")
	writeManifest(t, collectionDir, "sprint", "main.meow.toml")
	entrypointPath := filepath.Join(collectionDir, "main.meow.toml")
	writeModuleFile(t, entrypointPath, "sprint main", false)

	loader := NewLoader(projectDir)
	_, err := loader.ResolveWorkflow("sprint:lib/missing")
	if err == nil {
		t.Fatal("Expected error for missing collection path")
	}

	// Should get a WorkflowNotFoundError
	if _, ok := err.(*WorkflowNotFoundError); !ok {
		t.Errorf("Expected WorkflowNotFoundError, got %T: %v", err, err)
	}
}

func TestLoader_ResolveWorkflow_StandaloneStillWorks(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create only standalone files (no collections)
	standalonePath := filepath.Join(projectDir, ".meow", "workflows", "simple.meow.toml")
	writeModuleFile(t, standalonePath, "simple standalone", false)

	loader := NewLoader(projectDir)
	result, err := loader.ResolveWorkflow("simple")
	if err != nil {
		t.Fatalf("ResolveWorkflow failed: %v", err)
	}

	// Should resolve as standalone
	if result.Source != "project" {
		t.Errorf("Source = %s, want project", result.Source)
	}
	if result.Path != standalonePath {
		t.Errorf("Path = %s, want %s", result.Path, standalonePath)
	}
	if result.CollectionDir != "" {
		t.Errorf("CollectionDir should be empty for standalone files, got %s", result.CollectionDir)
	}
}

func TestLoader_LoadWorkflow_Collection(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	// Create a collection
	collectionDir := filepath.Join(projectDir, ".meow", "workflows", "sprint")
	writeManifest(t, collectionDir, "sprint", "main.meow.toml")
	entrypointPath := filepath.Join(collectionDir, "main.meow.toml")
	writeModuleFile(t, entrypointPath, "sprint workflow", false)

	loader := NewLoader(projectDir)
	result, err := loader.LoadWorkflow("sprint")
	if err != nil {
		t.Fatalf("LoadWorkflow failed: %v", err)
	}

	if result.Source != "project-collection" {
		t.Errorf("Source = %s, want project-collection", result.Source)
	}
	if result.Workflow.Description != "sprint workflow" {
		t.Errorf("Description = %s, want 'sprint workflow'", result.Workflow.Description)
	}
}

func TestLoader_ResolveWorkflow_EmptyReference(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	loader := NewLoader(projectDir)

	testCases := []struct {
		name string
		ref  string
	}{
		{"empty string", ""},
		{"whitespace only", "   "},
		{"tab only", "\t"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loader.ResolveWorkflow(tc.ref)
			if err == nil {
				t.Fatal("Expected error for empty reference")
			}
			if !strings.Contains(err.Error(), "empty") {
				t.Errorf("Error should mention 'empty': %s", err.Error())
			}
		})
	}
}

func TestLoader_ResolveWorkflow_InvalidReferences(t *testing.T) {
	projectDir := t.TempDir()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)

	loader := NewLoader(projectDir)

	testCases := []struct {
		name        string
		ref         string
		errContains string
	}{
		{"section only", "#section", "missing file path"},
		{"colon only", ":", "missing file path"},
		{"empty section", "sprint#", "missing workflow name"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := loader.ResolveWorkflow(tc.ref)
			if err == nil {
				t.Fatalf("Expected error for invalid reference %q", tc.ref)
			}
			if !strings.Contains(err.Error(), tc.errContains) {
				t.Errorf("Error should contain %q: %s", tc.errContains, err.Error())
			}
		})
	}
}
