package template

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// EmbeddedFS holds embedded templates. This should be set by the main package.
var EmbeddedFS embed.FS

// SetEmbeddedFS sets the embedded filesystem for templates.
func SetEmbeddedFS(efs embed.FS) {
	EmbeddedFS = efs
}

// Loader loads templates from multiple sources with precedence.
type Loader struct {
	// ProjectDir is the project root (containing .meow/templates/)
	ProjectDir string

	// UserDir is the user config dir (containing templates/)
	// Default: ~/.config/meow
	UserDir string

	// EmbeddedDir is the subdirectory in EmbeddedFS
	// Default: "templates"
	EmbeddedDir string
}

// NewLoader creates a new template loader.
func NewLoader(projectDir string) *Loader {
	userDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		userDir = filepath.Join(home, ".config", "meow")
	}

	return &Loader{
		ProjectDir:  projectDir,
		UserDir:     userDir,
		EmbeddedDir: "templates",
	}
}

// Load loads a template by name, checking sources in priority order:
// 1. Project: {projectDir}/.meow/templates/{name}.toml
// 2. User: {userDir}/templates/{name}.toml
// 3. Embedded: templates/{name}.toml (from EmbeddedFS)
func (l *Loader) Load(name string) (*Template, error) {
	filename := name + ".toml"

	// 1. Try project-local templates
	if l.ProjectDir != "" {
		path := filepath.Join(l.ProjectDir, ".meow", "templates", filename)
		if tmpl, err := l.loadFromPath(path); err == nil {
			return tmpl, nil
		}
	}

	// 2. Try user templates
	if l.UserDir != "" {
		path := filepath.Join(l.UserDir, "templates", filename)
		if tmpl, err := l.loadFromPath(path); err == nil {
			return tmpl, nil
		}
	}

	// 3. Try embedded templates
	if tmpl, err := l.loadFromEmbedded(filename); err == nil {
		return tmpl, nil
	}

	// Not found anywhere
	return nil, &TemplateNotFoundError{Name: name, SearchPaths: l.searchPaths(name)}
}

// LoadFromPath loads a template from a specific path (no search).
func (l *Loader) LoadFromPath(path string) (*Template, error) {
	return ParseFile(path)
}

// loadFromPath attempts to load from a filesystem path.
func (l *Loader) loadFromPath(path string) (*Template, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, err
	}
	return ParseFile(path)
}

// loadFromEmbedded attempts to load from the embedded filesystem.
func (l *Loader) loadFromEmbedded(filename string) (*Template, error) {
	if EmbeddedFS == (embed.FS{}) {
		return nil, fmt.Errorf("no embedded templates")
	}

	// embed.FS always uses forward slashes, regardless of OS
	path := l.EmbeddedDir + "/" + filename
	data, err := EmbeddedFS.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return ParseString(string(data))
}

// searchPaths returns all paths that would be searched for a template.
func (l *Loader) searchPaths(name string) []string {
	filename := name + ".toml"
	var paths []string

	if l.ProjectDir != "" {
		paths = append(paths, filepath.Join(l.ProjectDir, ".meow", "templates", filename))
	}
	if l.UserDir != "" {
		paths = append(paths, filepath.Join(l.UserDir, "templates", filename))
	}
	paths = append(paths, "<embedded>/"+l.EmbeddedDir+"/"+filename)

	return paths
}

// List returns all available template names from all sources.
func (l *Loader) List() ([]TemplateInfo, error) {
	seen := make(map[string]bool)
	var templates []TemplateInfo

	// Project templates
	if l.ProjectDir != "" {
		dir := filepath.Join(l.ProjectDir, ".meow", "templates")
		if infos, err := l.listDir(dir, "project"); err == nil {
			for _, info := range infos {
				if !seen[info.Name] {
					seen[info.Name] = true
					templates = append(templates, info)
				}
			}
		}
	}

	// User templates
	if l.UserDir != "" {
		dir := filepath.Join(l.UserDir, "templates")
		if infos, err := l.listDir(dir, "user"); err == nil {
			for _, info := range infos {
				if !seen[info.Name] {
					seen[info.Name] = true
					templates = append(templates, info)
				}
			}
		}
	}

	// Embedded templates
	if EmbeddedFS != (embed.FS{}) {
		if infos, err := l.listEmbedded(); err == nil {
			for _, info := range infos {
				if !seen[info.Name] {
					seen[info.Name] = true
					templates = append(templates, info)
				}
			}
		}
	}

	return templates, nil
}

// listDir lists templates in a directory.
func (l *Loader) listDir(dir, source string) ([]TemplateInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var templates []TemplateInfo
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		name := entry.Name()[:len(entry.Name())-5] // Remove .toml
		templates = append(templates, TemplateInfo{
			Name:   name,
			Source: source,
			Path:   filepath.Join(dir, entry.Name()),
		})
	}
	return templates, nil
}

// listEmbedded lists templates from the embedded filesystem.
func (l *Loader) listEmbedded() ([]TemplateInfo, error) {
	entries, err := fs.ReadDir(EmbeddedFS, l.EmbeddedDir)
	if err != nil {
		return nil, err
	}

	var templates []TemplateInfo
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		name := entry.Name()[:len(entry.Name())-5] // Remove .toml
		templates = append(templates, TemplateInfo{
			Name:   name,
			Source: "embedded",
			Path:   "<embedded>/" + l.EmbeddedDir + "/" + entry.Name(),
		})
	}
	return templates, nil
}

// TemplateInfo contains metadata about an available template.
type TemplateInfo struct {
	Name   string // Template name (without .toml extension)
	Source string // "project", "user", or "embedded"
	Path   string // Full path or "<embedded>/..."
}

// TemplateNotFoundError is returned when a template cannot be found.
type TemplateNotFoundError struct {
	Name        string
	SearchPaths []string
}

func (e *TemplateNotFoundError) Error() string {
	return fmt.Sprintf("template %q not found in: %v", e.Name, e.SearchPaths)
}

// LoadContext tracks state during template/module loading for cycle detection
// and local reference resolution.
//
// When loading templates that reference other templates (via template fields
// or expansion targets), LoadContext ensures:
//   - Circular references are detected and reported
//   - Local references (.workflow) can resolve against the current module
//   - File paths are tracked for meaningful error messages
type LoadContext struct {
	// FilePath is the absolute path of the file currently being loaded.
	// Used for resolving relative references and error messages.
	FilePath string

	// Module is the parsed module from FilePath, if loading a module-format file.
	// Used for resolving local references (.workflow syntax).
	// Nil when loading legacy-format templates.
	Module *Module

	// visited tracks all file#workflow references we've seen during loading.
	// Used to detect circular references. Keys are normalized reference strings
	// like "path/to/file.toml#workflow" or "path/to/file.toml" for legacy.
	visited map[string]bool

	// stack holds the loading path for error messages when a cycle is detected.
	// Each entry is a reference string showing the chain of loads.
	stack []string
}

// NewLoadContext creates a new LoadContext for loading from the given file path.
func NewLoadContext(filePath string) *LoadContext {
	return &LoadContext{
		FilePath: filePath,
		visited:  make(map[string]bool),
		stack:    []string{},
	}
}

// Enter marks that we are entering a reference during loading.
// Returns an error if entering this reference would create a cycle.
// The reference should be a normalized string like "file.toml#workflow".
func (c *LoadContext) Enter(ref string) error {
	if c.visited[ref] {
		// Build cycle path for error message
		cyclePath := append(c.stack, ref)
		return &CircularReferenceError{
			Reference: ref,
			Path:      cyclePath,
		}
	}

	c.visited[ref] = true
	c.stack = append(c.stack, ref)
	return nil
}

// Exit marks that we are done loading a reference.
// Should be called after Enter when loading is complete.
func (c *LoadContext) Exit(ref string) {
	if len(c.stack) > 0 && c.stack[len(c.stack)-1] == ref {
		c.stack = c.stack[:len(c.stack)-1]
	}
}

// Child creates a child LoadContext for loading a referenced file.
// The child inherits the visited set and stack but has its own FilePath.
// The child's Module is initially nil; set it after parsing the file.
func (c *LoadContext) Child(filePath string) *LoadContext {
	return &LoadContext{
		FilePath: filePath,
		Module:   nil, // Set after parsing
		visited:  c.visited,
		stack:    c.stack,
	}
}

// CurrentRef returns the current reference being loaded (top of stack),
// or empty string if the stack is empty.
func (c *LoadContext) CurrentRef() string {
	if len(c.stack) == 0 {
		return ""
	}
	return c.stack[len(c.stack)-1]
}

// Depth returns the current nesting depth of reference resolution.
func (c *LoadContext) Depth() int {
	return len(c.stack)
}

// CircularReferenceError is returned when a circular reference is detected.
type CircularReferenceError struct {
	Reference string   // The reference that caused the cycle
	Path      []string // The full path showing the cycle
}

func (e *CircularReferenceError) Error() string {
	if len(e.Path) == 0 {
		return fmt.Sprintf("circular reference detected: %s", e.Reference)
	}
	return fmt.Sprintf("circular reference detected: %s (path: %s)",
		e.Reference, formatCyclePath(e.Path))
}

// formatCyclePath formats the cycle path for display.
func formatCyclePath(path []string) string {
	if len(path) == 0 {
		return ""
	}
	result := path[0]
	for i := 1; i < len(path); i++ {
		result += " → " + path[i]
	}
	return result
}
