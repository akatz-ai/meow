package workflow

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// EmbeddedFS holds embedded workflows. This should be set by the main package.
var EmbeddedFS embed.FS

// SetEmbeddedFS sets the embedded filesystem for workflows.
func SetEmbeddedFS(efs embed.FS) {
	EmbeddedFS = efs
}

// Loader loads workflows from multiple sources with precedence.
type Loader struct {
	// ProjectDir is the project root (containing .meow/workflows/)
	ProjectDir string

	// UserDir is the user config dir (containing workflows/)
	// Default: ~/.meow
	UserDir string

	// EmbeddedDir is the subdirectory in EmbeddedFS
	// Default: "workflows"
	EmbeddedDir string
}

// WorkflowLocation describes where a workflow reference resolved.
type WorkflowLocation struct {
	Path   string
	Source string
	Name   string
}

// LoadedWorkflow contains a resolved workflow and its module.
type LoadedWorkflow struct {
	Module   *Module
	Workflow *Workflow
	Path     string
	Source   string
	Name     string
}

// NewLoader creates a new workflow loader.
func NewLoader(projectDir string) *Loader {
	userDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		userDir = filepath.Join(home, ".meow")
	}

	return &Loader{
		ProjectDir:  projectDir,
		UserDir:     userDir,
		EmbeddedDir: "workflows",
	}
}

// LoadWorkflow loads a workflow by reference, returning its module and metadata.
func (l *Loader) LoadWorkflow(ref string) (*LoadedWorkflow, error) {
	location, err := l.ResolveWorkflow(ref)
	if err != nil {
		return nil, err
	}

	module, err := l.loadModule(location)
	if err != nil {
		return nil, err
	}

	wf := module.GetWorkflow(location.Name)
	if wf == nil {
		var available []string
		for name := range module.Workflows {
			available = append(available, name)
		}
		return nil, fmt.Errorf("workflow %q not found in %s (available: %v)", location.Name, location.Path, available)
	}

	return &LoadedWorkflow{
		Module:   module,
		Workflow: wf,
		Path:     location.Path,
		Source:   location.Source,
		Name:     location.Name,
	}, nil
}

// ResolveWorkflow returns the path and workflow name for a reference.
// Search order:
// 1. Project: {projectDir}/.meow/workflows/{path}.meow.toml
// 2. User: {userDir}/workflows/{path}.meow.toml
// 3. Embedded: workflows/{path}.meow.toml (from EmbeddedFS)
func (l *Loader) ResolveWorkflow(ref string) (*WorkflowLocation, error) {
	fileRef, workflowName, err := parseWorkflowRef(ref)
	if err != nil {
		return nil, err
	}

	filename := fileRef + ".meow.toml"

	if l.ProjectDir != "" {
		path := filepath.Join(l.ProjectDir, ".meow", "workflows", filename)
		if fileExists(path) {
			return &WorkflowLocation{Path: path, Source: "project", Name: workflowName}, nil
		}
	}

	if l.UserDir != "" {
		path := filepath.Join(l.UserDir, "workflows", filename)
		if fileExists(path) {
			return &WorkflowLocation{Path: path, Source: "user", Name: workflowName}, nil
		}
	}

	if EmbeddedFS != (embed.FS{}) {
		embeddedPath := path.Join(l.EmbeddedDir, filepath.ToSlash(filename))
		if _, err := fs.Stat(EmbeddedFS, embeddedPath); err == nil {
			return &WorkflowLocation{Path: embeddedPath, Source: "embedded", Name: workflowName}, nil
		}
	}

	return nil, &WorkflowNotFoundError{Ref: ref, Searched: l.searchPaths(fileRef)}
}

func (l *Loader) loadModule(location *WorkflowLocation) (*Module, error) {
	if location.Source == "embedded" {
		data, err := EmbeddedFS.ReadFile(location.Path)
		if err != nil {
			return nil, fmt.Errorf("reading embedded workflow %s: %w", location.Path, err)
		}
		return ParseModuleString(string(data), location.Path)
	}

	return ParseModuleFile(location.Path)
}

func parseWorkflowRef(ref string) (string, string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", "", fmt.Errorf("workflow reference is empty")
	}

	fileRef := ref
	workflowName := "main"

	if strings.Contains(ref, "#") {
		parts := strings.SplitN(ref, "#", 2)
		fileRef = strings.TrimSpace(parts[0])
		workflowName = strings.TrimSpace(parts[1])
		if fileRef == "" {
			return "", "", fmt.Errorf("workflow reference missing file path: %q", ref)
		}
		if workflowName == "" {
			return "", "", fmt.Errorf("workflow reference missing workflow name: %q", ref)
		}
	}

	fileRef = strings.TrimSuffix(fileRef, ".meow.toml")
	fileRef = filepath.ToSlash(filepath.Clean(fileRef))
	fileRef = strings.TrimPrefix(fileRef, "./")
	if fileRef == "." || fileRef == "" {
		return "", "", fmt.Errorf("workflow reference missing file path: %q", ref)
	}

	return fileRef, workflowName, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (l *Loader) searchPaths(fileRef string) []string {
	filename := fileRef + ".meow.toml"
	var paths []string

	if l.ProjectDir != "" {
		paths = append(paths, filepath.Join(l.ProjectDir, ".meow", "workflows", filename))
	}
	if l.UserDir != "" {
		paths = append(paths, filepath.Join(l.UserDir, "workflows", filename))
	}
	paths = append(paths, "<embedded>/"+path.Join(l.EmbeddedDir, filepath.ToSlash(filename)))

	return paths
}

// WorkflowNotFoundError is returned when a workflow cannot be found.
type WorkflowNotFoundError struct {
	Ref      string
	Searched []string
}

func (e *WorkflowNotFoundError) Error() string {
	return fmt.Sprintf("workflow %q not found in: %v", e.Ref, e.Searched)
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
		// Build cycle path for error message - make a copy to avoid sharing backing array
		cyclePath := make([]string, len(c.stack)+1)
		copy(cyclePath, c.stack)
		cyclePath[len(c.stack)] = ref
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
// This removes the reference from both the stack and visited set,
// allowing the same reference to be entered again from a different path
// (supporting diamond dependencies like A→B, A→C→B).
func (c *LoadContext) Exit(ref string) {
	if len(c.stack) > 0 && c.stack[len(c.stack)-1] == ref {
		c.stack = c.stack[:len(c.stack)-1]
		delete(c.visited, ref)
	}
}

// Child creates a child LoadContext for loading a referenced file.
// The child inherits the visited set (shared for cycle detection) and
// a copy of the current stack (for accurate error paths).
// The child's Module is initially nil; set it after parsing the file.
func (c *LoadContext) Child(filePath string) *LoadContext {
	// Copy the stack to avoid interference between parent and child appends
	stackCopy := make([]string, len(c.stack))
	copy(stackCopy, c.stack)

	return &LoadContext{
		FilePath: filePath,
		Module:   nil, // Set after parsing
		visited:  c.visited,
		stack:    stackCopy,
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

// AvailableWorkflow describes a workflow available for execution.
type AvailableWorkflow struct {
	Name        string // Workflow file name (without .meow.toml)
	Description string // From workflow metadata
	Source      string // "project", "library", "user", or "embedded"
	Path        string // Full path to the file
	Internal    bool   // Whether the workflow is marked internal
}

// ListAvailable returns all workflows available from all sources.
// Workflows are returned grouped by source, with internal workflows excluded.
func (l *Loader) ListAvailable() (map[string][]AvailableWorkflow, error) {
	result := make(map[string][]AvailableWorkflow)

	// Project workflows (.meow/workflows/*.meow.toml, excluding lib/)
	if l.ProjectDir != "" {
		projectDir := filepath.Join(l.ProjectDir, ".meow", "workflows")
		projectWorkflows, err := l.listFromDir(projectDir, "project")
		if err == nil && len(projectWorkflows) > 0 {
			result["project"] = projectWorkflows
		}

		// Library workflows (.meow/workflows/lib/*.meow.toml)
		libDir := filepath.Join(projectDir, "lib")
		libWorkflows, err := l.listFromDir(libDir, "library")
		if err == nil && len(libWorkflows) > 0 {
			result["library"] = libWorkflows
		}
	}

	// User workflows (~/.meow/workflows/*.meow.toml)
	if l.UserDir != "" {
		userDir := filepath.Join(l.UserDir, "workflows")
		userWorkflows, err := l.listFromDir(userDir, "user")
		if err == nil && len(userWorkflows) > 0 {
			result["user"] = userWorkflows
		}
	}

	// Embedded workflows
	if EmbeddedFS != (embed.FS{}) {
		embeddedWorkflows, err := l.listFromEmbedded()
		if err == nil && len(embeddedWorkflows) > 0 {
			result["embedded"] = embeddedWorkflows
		}
	}

	return result, nil
}

// listFromDir lists workflows from a filesystem directory.
func (l *Loader) listFromDir(dir string, source string) ([]AvailableWorkflow, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var workflows []AvailableWorkflow
	for _, entry := range entries {
		if entry.IsDir() {
			continue // Skip subdirectories (lib/ handled separately)
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".meow.toml") {
			continue
		}

		baseName := strings.TrimSuffix(name, ".meow.toml")
		fullPath := filepath.Join(dir, name)

		// Parse module to get description and internal flag
		module, err := ParseModuleFile(fullPath)
		if err != nil {
			// Skip files that fail to parse
			continue
		}

		// Get the main workflow's description
		mainWf := module.DefaultWorkflow()
		if mainWf == nil {
			continue
		}

		// Skip internal workflows
		if mainWf.Internal {
			continue
		}

		workflows = append(workflows, AvailableWorkflow{
			Name:        baseName,
			Description: mainWf.Description,
			Source:      source,
			Path:        fullPath,
			Internal:    mainWf.Internal,
		})
	}

	return workflows, nil
}

// listFromEmbedded lists workflows from the embedded filesystem.
func (l *Loader) listFromEmbedded() ([]AvailableWorkflow, error) {
	entries, err := fs.ReadDir(EmbeddedFS, l.EmbeddedDir)
	if err != nil {
		return nil, err
	}

	var workflows []AvailableWorkflow
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".meow.toml") {
			continue
		}

		baseName := strings.TrimSuffix(name, ".meow.toml")
		embeddedPath := path.Join(l.EmbeddedDir, name)

		// Parse module to get description
		data, err := EmbeddedFS.ReadFile(embeddedPath)
		if err != nil {
			continue
		}

		module, err := ParseModuleString(string(data), embeddedPath)
		if err != nil {
			continue
		}

		mainWf := module.DefaultWorkflow()
		if mainWf == nil {
			continue
		}

		// Skip internal workflows
		if mainWf.Internal {
			continue
		}

		workflows = append(workflows, AvailableWorkflow{
			Name:        baseName,
			Description: mainWf.Description,
			Source:      "embedded",
			Path:        embeddedPath,
			Internal:    mainWf.Internal,
		})
	}

	return workflows, nil
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
