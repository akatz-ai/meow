package template

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Minimal test template content
const testTemplateContent = `
[meta]
name = "test-template"
version = "1.0.0"
description = "Test template"

[[steps]]
id = "step1"
description = "First step"
`

func TestLoader_Load_ProjectTemplate(t *testing.T) {
	dir := t.TempDir()

	// Create project template
	templatesDir := filepath.Join(dir, ".meow", "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("Failed to create templates dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(templatesDir, "test.toml"), []byte(testTemplateContent), 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	loader := NewLoader(dir)
	tmpl, err := loader.Load("test")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if tmpl.Meta.Name != "test-template" {
		t.Errorf("Meta.Name = %s, want test-template", tmpl.Meta.Name)
	}
}

func TestLoader_Load_NotFound(t *testing.T) {
	dir := t.TempDir()
	loader := NewLoader(dir)

	_, err := loader.Load("nonexistent")
	if err == nil {
		t.Fatal("Expected error for nonexistent template")
	}

	notFoundErr, ok := err.(*TemplateNotFoundError)
	if !ok {
		t.Fatalf("Expected TemplateNotFoundError, got %T", err)
	}

	if notFoundErr.Name != "nonexistent" {
		t.Errorf("Name = %s, want nonexistent", notFoundErr.Name)
	}
}

func TestLoader_Load_Priority(t *testing.T) {
	dir := t.TempDir()

	// Create project template with specific content
	templatesDir := filepath.Join(dir, ".meow", "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("Failed to create templates dir: %v", err)
	}

	projectContent := `
[meta]
name = "project-version"
version = "1.0.0"

[[steps]]
id = "step1"
`
	if err := os.WriteFile(filepath.Join(templatesDir, "priority.toml"), []byte(projectContent), 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	loader := NewLoader(dir)
	tmpl, err := loader.Load("priority")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should load project version (highest priority)
	if tmpl.Meta.Name != "project-version" {
		t.Errorf("Meta.Name = %s, want project-version (project should take priority)", tmpl.Meta.Name)
	}
}

func TestLoader_LoadFromPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom.toml")

	if err := os.WriteFile(path, []byte(testTemplateContent), 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	loader := NewLoader(dir)
	tmpl, err := loader.LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	if tmpl.Meta.Name != "test-template" {
		t.Errorf("Meta.Name = %s, want test-template", tmpl.Meta.Name)
	}
}

func TestLoader_List(t *testing.T) {
	dir := t.TempDir()

	// Create project templates
	templatesDir := filepath.Join(dir, ".meow", "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("Failed to create templates dir: %v", err)
	}

	for _, name := range []string{"alpha", "beta"} {
		content := `[meta]
name = "` + name + `"
version = "1.0.0"
[[steps]]
id = "step1"
`
		if err := os.WriteFile(filepath.Join(templatesDir, name+".toml"), []byte(content), 0644); err != nil {
			t.Fatalf("Failed to write template: %v", err)
		}
	}

	loader := NewLoader(dir)
	templates, err := loader.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(templates) != 2 {
		t.Fatalf("Expected 2 templates, got %d", len(templates))
	}

	names := make(map[string]bool)
	for _, tmpl := range templates {
		names[tmpl.Name] = true
		if tmpl.Source != "project" {
			t.Errorf("Template %s source = %s, want project", tmpl.Name, tmpl.Source)
		}
	}

	if !names["alpha"] || !names["beta"] {
		t.Errorf("Missing expected templates: %v", names)
	}
}

func TestLoader_List_NoDuplicates(t *testing.T) {
	dir := t.TempDir()

	// Create same template in project
	templatesDir := filepath.Join(dir, ".meow", "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("Failed to create templates dir: %v", err)
	}

	content := `[meta]
name = "same"
version = "1.0.0"
[[steps]]
id = "step1"
`
	if err := os.WriteFile(filepath.Join(templatesDir, "same.toml"), []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	loader := NewLoader(dir)
	templates, err := loader.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	// Count how many times "same" appears
	count := 0
	for _, tmpl := range templates {
		if tmpl.Name == "same" {
			count++
		}
	}

	if count != 1 {
		t.Errorf("Expected 'same' template once, got %d times", count)
	}
}

func TestTemplateNotFoundError_Error(t *testing.T) {
	err := &TemplateNotFoundError{
		Name:        "missing",
		SearchPaths: []string{"/a/b/c.toml", "/x/y/z.toml"},
	}

	msg := err.Error()
	if msg == "" {
		t.Error("Error message should not be empty")
	}
}

func TestNewLoader(t *testing.T) {
	loader := NewLoader("/project")

	if loader.ProjectDir != "/project" {
		t.Errorf("ProjectDir = %s, want /project", loader.ProjectDir)
	}
	if loader.EmbeddedDir != "templates" {
		t.Errorf("EmbeddedDir = %s, want templates", loader.EmbeddedDir)
	}
}

// Test embedded loading (requires setting up a mock embed.FS)
func TestLoader_LoadFromEmbedded_NoEmbedded(t *testing.T) {
	// Reset embedded FS
	EmbeddedFS = embed.FS{}
	defer func() { EmbeddedFS = embed.FS{} }()

	loader := NewLoader("/nonexistent")
	_, err := loader.Load("test")
	if err == nil {
		t.Fatal("Expected error when no embedded templates")
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
