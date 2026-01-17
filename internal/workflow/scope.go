package workflow

// Scope represents the source hierarchy for workflow resolution.
// When a workflow is resolved from a specific scope, all nested template
// references resolve within that scope's search hierarchy.
type Scope string

const (
	// ScopeProject searches project -> user -> embedded.
	// This is the default behavior when running a project-level workflow.
	ScopeProject Scope = "project"

	// ScopeUser searches user -> embedded (never project).
	// Used when running a user-level workflow to prevent shadowing.
	ScopeUser Scope = "user"

	// ScopeEmbedded searches embedded only.
	// Used for workflows compiled into the binary.
	ScopeEmbedded Scope = "embedded"
)

// Valid returns true if this is a recognized scope or empty (unspecified).
func (s Scope) Valid() bool {
	switch s {
	case ScopeProject, ScopeUser, ScopeEmbedded, "":
		return true
	}
	return false
}

// SearchesProject returns true if this scope includes project workflows.
func (s Scope) SearchesProject() bool {
	return s == "" || s == ScopeProject
}

// SearchesUser returns true if this scope includes user workflows.
func (s Scope) SearchesUser() bool {
	return s == "" || s == ScopeProject || s == ScopeUser
}

// SearchesEmbedded returns true if this scope includes embedded workflows.
// All scopes search embedded as a fallback.
func (s Scope) SearchesEmbedded() bool {
	return true
}
