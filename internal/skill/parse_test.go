package skill

import (
	"os"
	"path/filepath"
	"testing"
)

const validSkillTOML = `
[skill]
name = "sprint-planner"
description = "Plan and execute sprints"
version = "1.0.0"

[targets.claude]

[targets.opencode]
path = "~/.config/opencode/skill/sprint-planner"
`

func TestParseFile(t *testing.T) {
	t.Run("valid skill.toml", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "skill.toml")
		writeTestFile(t, path, validSkillTOML)

		skill, err := ParseFile(path)
		if err != nil {
			t.Fatalf("ParseFile() error = %v", err)
		}

		if skill.Skill.Name != "sprint-planner" {
			t.Errorf("Name = %q, want %q", skill.Skill.Name, "sprint-planner")
		}
		if skill.Skill.Description != "Plan and execute sprints" {
			t.Errorf("Description = %q, want %q", skill.Skill.Description, "Plan and execute sprints")
		}
		if _, ok := skill.Targets["claude"]; !ok {
			t.Error("Targets should contain 'claude'")
		}
		if _, ok := skill.Targets["opencode"]; !ok {
			t.Error("Targets should contain 'opencode'")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		_, err := ParseFile("/nonexistent/skill.toml")
		if err == nil {
			t.Fatal("ParseFile() expected error for nonexistent file")
		}
	})

	t.Run("invalid toml syntax", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "skill.toml")
		writeTestFile(t, path, "invalid { toml syntax")

		_, err := ParseFile(path)
		if err == nil {
			t.Fatal("ParseFile() expected error for invalid TOML")
		}
	})
}

func TestParseString(t *testing.T) {
	t.Run("valid content", func(t *testing.T) {
		skill, err := ParseString(validSkillTOML)
		if err != nil {
			t.Fatalf("ParseString() error = %v", err)
		}

		if skill.Skill.Name != "sprint-planner" {
			t.Errorf("Name = %q, want %q", skill.Skill.Name, "sprint-planner")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		skill, err := ParseString("")
		if err != nil {
			t.Fatalf("ParseString() error = %v", err)
		}

		// Empty content is valid TOML - produces empty struct
		if skill.Skill.Name != "" {
			t.Errorf("Name = %q, want empty", skill.Skill.Name)
		}
	})
}

func TestLoadFromDir(t *testing.T) {
	t.Run("loads skill.toml from directory", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, ManifestName), validSkillTOML)

		skill, err := LoadFromDir(dir)
		if err != nil {
			t.Fatalf("LoadFromDir() error = %v", err)
		}

		if skill.Skill.Name != "sprint-planner" {
			t.Errorf("Name = %q, want %q", skill.Skill.Name, "sprint-planner")
		}
	})

	t.Run("error when manifest missing", func(t *testing.T) {
		dir := t.TempDir()
		_, err := LoadFromDir(dir)
		if err == nil {
			t.Fatal("LoadFromDir() expected error when manifest missing")
		}
	})
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
}

func TestExampleSkillFixture(t *testing.T) {
	// This test validates the example skill fixture in testdata/example-collection/skills/example-helper
	skillPath := filepath.Join("..", "..", "testdata", "example-collection", "skills", "example-helper", "skill.toml")

	// Check if testdata exists (skip test if it doesn't - fixtures may not be committed yet)
	if _, err := os.Stat(skillPath); os.IsNotExist(err) {
		t.Skip("testdata/example-collection/skills/example-helper not found")
	}

	skill, err := ParseFile(skillPath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Validate skill metadata
	if skill.Skill.Name != "example-helper" {
		t.Errorf("Name = %q, want %q", skill.Skill.Name, "example-helper")
	}
	if skill.Skill.Description == "" {
		t.Error("Description should not be empty")
	}
	if skill.Skill.Version != "1.0.0" {
		t.Errorf("Version = %q, want %q", skill.Skill.Version, "1.0.0")
	}

	// Validate targets
	if len(skill.Targets) == 0 {
		t.Fatal("Targets should not be empty")
	}
	if _, ok := skill.Targets["claude"]; !ok {
		t.Error("Targets should contain 'claude'")
	}
	if _, ok := skill.Targets["opencode"]; !ok {
		t.Error("Targets should contain 'opencode'")
	}

	// Validate files are specified
	if len(skill.Skill.Files) == 0 {
		t.Error("Files should be specified in example skill")
	}
}
