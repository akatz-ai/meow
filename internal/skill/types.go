package skill

// Skill represents a skill manifest (skill.toml).
// Skills are bundles of files that can be installed into AI harnesses
// like Claude Code or OpenCode.
type Skill struct {
	// Skill contains metadata about the skill
	Skill SkillMeta `toml:"skill"`

	// Targets maps harness names to their installation configuration.
	// Keys are harness identifiers (e.g., "claude", "opencode").
	// An empty target (e.g., [targets.claude]) means use default paths.
	Targets map[string]Target `toml:"targets"`
}

// SkillMeta contains metadata for the skill.
type SkillMeta struct {
	// Name is the unique identifier for this skill (required)
	Name string `toml:"name"`

	// Description is a human-readable description of the skill (required)
	Description string `toml:"description"`

	// Version is the semantic version of the skill (optional)
	Version string `toml:"version,omitempty"`

	// Files lists specific files to include. If empty, all files in the
	// skill directory are included (optional)
	Files []string `toml:"files,omitempty"`
}

// Target describes harness-specific installation configuration.
type Target struct {
	// Path is a custom installation path. If empty, the default path
	// for the harness is used.
	Path string `toml:"path,omitempty"`
}

// TargetConfig describes a known AI harness's skill installation paths.
type TargetConfig struct {
	// Name is the human-readable harness name (e.g., "Claude Code")
	Name string

	// GlobalPath is the template for global skill installation.
	// Uses {{name}} placeholder for the skill name.
	// e.g., "~/.claude/skills/{{name}}"
	GlobalPath string

	// ProjectPath is the template for project-local skill installation.
	// Uses {{name}} placeholder for the skill name.
	// e.g., ".claude/skills/{{name}}"
	ProjectPath string
}
