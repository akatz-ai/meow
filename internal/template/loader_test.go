package template

import (
	"embed"
	"os"
	"path/filepath"
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
