package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillListEmpty(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	defer func() {
		workDir = oldWorkDir
	}()
	workDir = tmpDir

	// Create empty skill directories for known targets
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create skill directories but leave them empty
	os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0755)
	os.MkdirAll(filepath.Join(home, ".config", "opencode", "skill"), 0755)

	// Capture output
	var buf bytes.Buffer
	skillListCmd.SetOut(&buf)
	skillListCmd.SetErr(&buf)

	err := runSkillList(skillListCmd, []string{})
	if err != nil {
		t.Fatalf("runSkillList failed: %v", err)
	}

	output := buf.String()
	// Should indicate no skills installed or show empty list
	if !strings.Contains(output, "No skills") && !strings.Contains(output, "SKILL") {
		t.Errorf("expected empty list indication, got: %q", output)
	}
}

func TestSkillListWithInstalledSkills(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	defer func() {
		workDir = oldWorkDir
	}()
	workDir = tmpDir

	// Set up test home directory
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a skill in Claude's skill directory
	skillDir := filepath.Join(home, ".claude", "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)

	skillToml := `[skill]
name = "test-skill"
description = "A test skill"

[targets.claude]
`
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillToml), 0644)

	// Capture output
	var buf bytes.Buffer
	skillListCmd.SetOut(&buf)
	skillListCmd.SetErr(&buf)

	err := runSkillList(skillListCmd, []string{})
	if err != nil {
		t.Fatalf("runSkillList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "test-skill") {
		t.Errorf("expected output to contain 'test-skill', got: %q", output)
	}
	if !strings.Contains(output, "claude") {
		t.Errorf("expected output to contain 'claude' target, got: %q", output)
	}
}

func TestSkillListTargetFilter(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	oldTarget := skillListTarget
	defer func() {
		workDir = oldWorkDir
		skillListTarget = oldTarget
	}()
	workDir = tmpDir

	// Set up test home directory
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create skills in both Claude and OpenCode directories
	claudeSkillDir := filepath.Join(home, ".claude", "skills", "claude-skill")
	os.MkdirAll(claudeSkillDir, 0755)
	os.WriteFile(filepath.Join(claudeSkillDir, "skill.toml"), []byte(`[skill]
name = "claude-skill"
description = "Claude only skill"

[targets.claude]
`), 0644)

	opencodeSkillDir := filepath.Join(home, ".config", "opencode", "skill", "opencode-skill")
	os.MkdirAll(opencodeSkillDir, 0755)
	os.WriteFile(filepath.Join(opencodeSkillDir, "skill.toml"), []byte(`[skill]
name = "opencode-skill"
description = "OpenCode only skill"

[targets.opencode]
`), 0644)

	// Filter by claude target
	skillListTarget = "claude"

	var buf bytes.Buffer
	skillListCmd.SetOut(&buf)
	skillListCmd.SetErr(&buf)

	err := runSkillList(skillListCmd, []string{})
	if err != nil {
		t.Fatalf("runSkillList failed: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "claude-skill") {
		t.Errorf("expected 'claude-skill' in filtered output, got: %q", output)
	}
	if strings.Contains(output, "opencode-skill") {
		t.Errorf("'opencode-skill' should not appear when filtering by claude, got: %q", output)
	}
}

func TestSkillListJSONOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	oldJSON := skillListJSON
	defer func() {
		workDir = oldWorkDir
		skillListJSON = oldJSON
	}()
	workDir = tmpDir
	skillListJSON = true

	// Set up test home directory
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a skill
	skillDir := filepath.Join(home, ".claude", "skills", "json-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(`[skill]
name = "json-skill"
description = "Skill for JSON output test"

[targets.claude]
`), 0644)

	var buf bytes.Buffer
	skillListCmd.SetOut(&buf)
	skillListCmd.SetErr(&buf)

	err := runSkillList(skillListCmd, []string{})
	if err != nil {
		t.Fatalf("runSkillList failed: %v", err)
	}

	output := buf.String()

	// Should be valid JSON
	var result []struct {
		Name   string `json:"name"`
		Target string `json:"target"`
		Path   string `json:"path"`
	}

	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("output should be valid JSON, got: %q, error: %v", output, err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 skill in JSON output, got %d", len(result))
	}

	if result[0].Name != "json-skill" {
		t.Errorf("JSON name = %q, want %q", result[0].Name, "json-skill")
	}

	if result[0].Target != "claude" {
		t.Errorf("JSON target = %q, want %q", result[0].Target, "claude")
	}
}

func TestSkillListMultipleTargets(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	defer func() {
		workDir = oldWorkDir
	}()
	workDir = tmpDir

	// Set up test home directory
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create the same skill in both targets
	claudeSkillDir := filepath.Join(home, ".claude", "skills", "multi-target-skill")
	os.MkdirAll(claudeSkillDir, 0755)
	os.WriteFile(filepath.Join(claudeSkillDir, "skill.toml"), []byte(`[skill]
name = "multi-target-skill"
description = "Multi-target skill"

[targets.claude]
`), 0644)

	opencodeSkillDir := filepath.Join(home, ".config", "opencode", "skill", "multi-target-skill")
	os.MkdirAll(opencodeSkillDir, 0755)
	os.WriteFile(filepath.Join(opencodeSkillDir, "skill.toml"), []byte(`[skill]
name = "multi-target-skill"
description = "Multi-target skill"

[targets.opencode]
`), 0644)

	var buf bytes.Buffer
	skillListCmd.SetOut(&buf)
	skillListCmd.SetErr(&buf)

	err := runSkillList(skillListCmd, []string{})
	if err != nil {
		t.Fatalf("runSkillList failed: %v", err)
	}

	output := buf.String()

	// Should show the skill twice (once for each target)
	claudeCount := strings.Count(output, "multi-target-skill")
	if claudeCount < 2 {
		t.Errorf("expected skill to appear for both targets, appears %d times in: %q", claudeCount, output)
	}
}
