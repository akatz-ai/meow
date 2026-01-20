package workflow

import (
	"embed"
	"encoding/json"
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

	// Scope restricts resolution to a specific search hierarchy.
	// Empty means no restriction (search all: project -> user -> embedded).
	// ScopeProject searches: project -> user -> embedded
	// ScopeUser searches: user -> embedded (never project)
	// ScopeEmbedded searches: embedded only
	Scope Scope
}

// WorkflowLocation describes where a workflow reference resolved.
type WorkflowLocation struct {
	Path          string
	Source        string
	Name          string
	CollectionDir string // Root of collection for collection-relative resolution (empty for standalone files)
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

// NewLoaderWithScope creates a loader restricted to a specific scope hierarchy.
// See Scope type for search behavior documentation.
func NewLoaderWithScope(projectDir string, scope Scope) *Loader {
	loader := NewLoader(projectDir)
	loader.Scope = scope
	return loader
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
// Search order depends on the Scope setting:
//   - ScopeProject or empty: project collection -> user collection -> project standalone -> user standalone -> embedded
//   - ScopeUser: user collection -> user standalone -> embedded (never project)
//   - ScopeEmbedded: embedded only
//
// Collection syntax:
//   - "sprint" - resolves to collection's entrypoint
//   - "sprint:lib/foo" - resolves to lib/foo.meow.toml within collection
//   - "sprint#section" - resolves to collection entrypoint, specific section
//   - "sprint:lib/foo#section" - combination of both
func (l *Loader) ResolveWorkflow(ref string) (*WorkflowLocation, error) {
	// Parse as collection reference first
	collectionRef, workflowPath, workflowName := parseCollectionRef(ref)

	// Try to resolve as collection (collections have precedence over standalone files)
	if location := l.resolveAsCollection(collectionRef, workflowPath, workflowName); location != nil {
		return location, nil
	}

	// Fall back to standalone file resolution
	// If workflowPath is set, the user used collection:path syntax but it wasn't a collection
	fileRef := collectionRef
	if workflowPath != "" {
		fileRef = workflowPath
	}

	filename := fileRef + ".meow.toml"

	// Project: only if scope allows it
	if l.Scope.SearchesProject() && l.ProjectDir != "" {
		path := filepath.Join(l.ProjectDir, ".meow", "workflows", filename)
		if fileExists(path) {
			return &WorkflowLocation{Path: path, Source: "project", Name: workflowName}, nil
		}
	}

	// User: only if scope allows it
	if l.Scope.SearchesUser() && l.UserDir != "" {
		path := filepath.Join(l.UserDir, "workflows", filename)
		if fileExists(path) {
			return &WorkflowLocation{Path: path, Source: "user", Name: workflowName}, nil
		}
	}

	// Embedded: always searched as fallback
	if l.Scope.SearchesEmbedded() && EmbeddedFS != (embed.FS{}) {
		embeddedPath := path.Join(l.EmbeddedDir, filepath.ToSlash(filename))
		if _, err := fs.Stat(EmbeddedFS, embeddedPath); err == nil {
			return &WorkflowLocation{Path: embeddedPath, Source: "embedded", Name: workflowName}, nil
		}
	}

	return nil, &WorkflowNotFoundError{Ref: ref, Searched: l.searchPaths(fileRef), Scope: l.Scope}
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

// resolveAsCollection checks if the reference resolves to a collection.
// Returns nil if not a collection or collection not found.
func (l *Loader) resolveAsCollection(name, subPath, workflowName string) *WorkflowLocation {
	// Check project collections
	if l.Scope.SearchesProject() && l.ProjectDir != "" {
		if loc := l.checkCollection(
			filepath.Join(l.ProjectDir, ".meow", "workflows", name),
			subPath, workflowName, "project-collection",
		); loc != nil {
			return loc
		}
	}

	// Check user collections
	if l.Scope.SearchesUser() && l.UserDir != "" {
		if loc := l.checkCollection(
			filepath.Join(l.UserDir, "workflows", name),
			subPath, workflowName, "user-collection",
		); loc != nil {
			return loc
		}
	}

	return nil
}

// checkCollection verifies a directory is a collection and resolves the workflow.
func (l *Loader) checkCollection(collectionDir, subPath, workflowName, source string) *WorkflowLocation {
	manifestPath := filepath.Join(collectionDir, ".meow", "manifest.json")
	if !fileExists(manifestPath) {
		return nil
	}

	// Load manifest to get entrypoint
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil
	}

	var manifest struct {
		Entrypoint string `json:"entrypoint"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil
	}

	// Determine workflow file
	var workflowFile string
	if subPath != "" {
		// User specified path: sprint:lib/agent-track
		workflowFile = subPath + ".meow.toml"
	} else {
		// Use entrypoint
		workflowFile = manifest.Entrypoint
	}

	workflowPath := filepath.Join(collectionDir, workflowFile)
	if !fileExists(workflowPath) {
		return nil
	}

	return &WorkflowLocation{
		Path:          workflowPath,
		Source:        source,
		Name:          workflowName,
		CollectionDir: collectionDir,
	}
}

// parseCollectionRef parses a workflow reference that may be a collection.
// Handles: sprint, sprint:lib/foo, sprint#section, sprint:lib/foo#section
// Returns: collection name, path within collection (or empty), workflow name
func parseCollectionRef(ref string) (collection, path, workflowName string) {
	workflowName = "main"

	// Handle # for workflow section
	if idx := strings.Index(ref, "#"); idx != -1 {
		workflowName = ref[idx+1:]
		ref = ref[:idx]
	}

	// Handle : for collection:path
	if idx := strings.Index(ref, ":"); idx != -1 {
		collection = ref[:idx]
		path = ref[idx+1:]
	} else {
		collection = ref
	}

	return collection, path, workflowName
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

	if l.Scope.SearchesProject() && l.ProjectDir != "" {
		paths = append(paths, filepath.Join(l.ProjectDir, ".meow", "workflows", filename))
	}
	if l.Scope.SearchesUser() && l.UserDir != "" {
		paths = append(paths, filepath.Join(l.UserDir, "workflows", filename))
	}
	if l.Scope.SearchesEmbedded() {
		paths = append(paths, "<embedded>/"+path.Join(l.EmbeddedDir, filepath.ToSlash(filename)))
	}

	return paths
}

// WorkflowNotFoundError is returned when a workflow cannot be found.
type WorkflowNotFoundError struct {
	Ref      string
	Searched []string
	Scope    Scope // Scope restriction that was applied (empty if none)
}

func (e *WorkflowNotFoundError) Error() string {
	msg := fmt.Sprintf("workflow %q not found in: %v", e.Ref, e.Searched)
	if e.Scope != "" {
		msg += fmt.Sprintf(" (scope: %s)", e.Scope)
	}
	return msg
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
