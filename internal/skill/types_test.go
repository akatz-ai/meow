package skill

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestSkillTypesUnmarshal(t *testing.T) {
	t.Run("basic skill manifest", func(t *testing.T) {
		content := `
[skill]
name = "sprint-planner"
description = "Plan and execute sprints"
version = "1.0.0"

[targets.claude]
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		if skill.Skill.Name != "sprint-planner" {
			t.Errorf("Name = %q, want %q", skill.Skill.Name, "sprint-planner")
		}
		if skill.Skill.Description != "Plan and execute sprints" {
			t.Errorf("Description = %q, want %q", skill.Skill.Description, "Plan and execute sprints")
		}
		if skill.Skill.Version != "1.0.0" {
			t.Errorf("Version = %q, want %q", skill.Skill.Version, "1.0.0")
		}
		if _, ok := skill.Targets["claude"]; !ok {
			t.Error("Targets should contain 'claude'")
		}
	})

	t.Run("skill with files list", func(t *testing.T) {
		content := `
[skill]
name = "my-skill"
description = "A custom skill"
files = ["skill.md", "lib/helpers.py"]

[targets.claude]
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		if len(skill.Skill.Files) != 2 {
			t.Fatalf("Files len = %d, want 2", len(skill.Skill.Files))
		}
		if skill.Skill.Files[0] != "skill.md" {
			t.Errorf("Files[0] = %q, want %q", skill.Skill.Files[0], "skill.md")
		}
		if skill.Skill.Files[1] != "lib/helpers.py" {
			t.Errorf("Files[1] = %q, want %q", skill.Skill.Files[1], "lib/helpers.py")
		}
	})

	t.Run("target with custom path", func(t *testing.T) {
		content := `
[skill]
name = "custom-skill"
description = "Skill with custom path"

[targets.claude]
path = "/custom/path/to/skill"
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		if skill.Targets["claude"].Path != "/custom/path/to/skill" {
			t.Errorf("Targets[claude].Path = %q, want %q", skill.Targets["claude"].Path, "/custom/path/to/skill")
		}
	})

	t.Run("multiple targets", func(t *testing.T) {
		content := `
[skill]
name = "multi-target"
description = "Supports multiple harnesses"

[targets.claude]

[targets.opencode]
path = "~/.config/opencode/skill/my-skill"
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		if _, ok := skill.Targets["claude"]; !ok {
			t.Error("Targets should contain 'claude'")
		}
		if _, ok := skill.Targets["opencode"]; !ok {
			t.Error("Targets should contain 'opencode'")
		}
		if skill.Targets["claude"].Path != "" {
			t.Errorf("Targets[claude].Path = %q, want empty", skill.Targets["claude"].Path)
		}
		if skill.Targets["opencode"].Path != "~/.config/opencode/skill/my-skill" {
			t.Errorf("Targets[opencode].Path = %q, want %q", skill.Targets["opencode"].Path, "~/.config/opencode/skill/my-skill")
		}
	})

	t.Run("empty targets", func(t *testing.T) {
		content := `
[skill]
name = "empty-targets"
description = "No explicit targets"
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		if len(skill.Targets) != 0 {
			t.Errorf("Targets len = %d, want 0", len(skill.Targets))
		}
	})
}

func TestSkillMetaFields(t *testing.T) {
	t.Run("required fields", func(t *testing.T) {
		meta := SkillMeta{
			Name:        "test-skill",
			Description: "A test skill",
		}
		if meta.Name != "test-skill" {
			t.Errorf("Name = %q, want %q", meta.Name, "test-skill")
		}
		if meta.Description != "A test skill" {
			t.Errorf("Description = %q, want %q", meta.Description, "A test skill")
		}
	})

	t.Run("optional fields", func(t *testing.T) {
		meta := SkillMeta{
			Name:        "test-skill",
			Description: "A test skill",
			Version:     "2.0.0",
			Files:       []string{"main.md", "utils.py"},
		}
		if meta.Version != "2.0.0" {
			t.Errorf("Version = %q, want %q", meta.Version, "2.0.0")
		}
		if len(meta.Files) != 2 {
			t.Errorf("Files len = %d, want 2", len(meta.Files))
		}
	})
}

func TestTargetStruct(t *testing.T) {
	t.Run("empty target means use defaults", func(t *testing.T) {
		target := Target{}
		if target.Path != "" {
			t.Errorf("Path = %q, want empty", target.Path)
		}
	})

	t.Run("custom path overrides defaults", func(t *testing.T) {
		target := Target{Path: "/custom/path"}
		if target.Path != "/custom/path" {
			t.Errorf("Path = %q, want %q", target.Path, "/custom/path")
		}
	})
}

func TestTargetConfigStruct(t *testing.T) {
	t.Run("target config fields", func(t *testing.T) {
		config := TargetConfig{
			Name:        "Claude Code",
			GlobalPath:  "~/.claude/skills/{{name}}",
			ProjectPath: ".claude/skills/{{name}}",
		}
		if config.Name != "Claude Code" {
			t.Errorf("Name = %q, want %q", config.Name, "Claude Code")
		}
		if !strings.Contains(config.GlobalPath, "{{name}}") {
			t.Errorf("GlobalPath = %q, want to contain {{name}}", config.GlobalPath)
		}
		if !strings.Contains(config.ProjectPath, "{{name}}") {
			t.Errorf("ProjectPath = %q, want to contain {{name}}", config.ProjectPath)
		}
	})
}
