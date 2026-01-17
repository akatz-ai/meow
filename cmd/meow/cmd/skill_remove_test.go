package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillRemoveNotFound(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	oldTarget := skillRemoveTarget
	oldYes := skillRemoveYes
	defer func() {
		workDir = oldWorkDir
		skillRemoveTarget = oldTarget
		skillRemoveYes = oldYes
	}()
	workDir = tmpDir
	skillRemoveTarget = "claude"
	skillRemoveYes = true // Skip confirmation

	// Set up test home directory
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create empty skill directory
	os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0755)

	err := runSkillRemove(skillRemoveCmd, []string{"nonexistent-skill"})
	if err == nil {
		t.Fatal("expected error when skill not found")
	}

	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' error, got: %v", err)
	}
}

func TestSkillRemoveExistingSkill(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	oldTarget := skillRemoveTarget
	oldYes := skillRemoveYes
	defer func() {
		workDir = oldWorkDir
		skillRemoveTarget = oldTarget
		skillRemoveYes = oldYes
	}()
	workDir = tmpDir
	skillRemoveTarget = "claude"
	skillRemoveYes = true // Skip confirmation

	// Set up test home directory
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a skill
	skillDir := filepath.Join(home, ".claude", "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(`[skill]
name = "test-skill"
description = "Test skill"

[targets.claude]
`), 0644)
	os.WriteFile(filepath.Join(skillDir, "skill.md"), []byte("# Test Skill"), 0644)

	// Verify skill exists before removal
	if !skillExists(skillDir) {
		t.Fatal("skill should exist before removal")
	}

	err := runSkillRemove(skillRemoveCmd, []string{"test-skill"})
	if err != nil {
		t.Fatalf("runSkillRemove failed: %v", err)
	}

	// Verify skill was removed
	if skillExists(skillDir) {
		t.Error("skill should not exist after removal")
	}
}

func TestSkillRemoveFromAllTargets(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	oldTarget := skillRemoveTarget
	oldYes := skillRemoveYes
	defer func() {
		workDir = oldWorkDir
		skillRemoveTarget = oldTarget
		skillRemoveYes = oldYes
	}()
	workDir = tmpDir
	skillRemoveTarget = "all" // Remove from all targets
	skillRemoveYes = true

	// Set up test home directory
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create the skill in both targets
	claudeSkillDir := filepath.Join(home, ".claude", "skills", "multi-skill")
	os.MkdirAll(claudeSkillDir, 0755)
	os.WriteFile(filepath.Join(claudeSkillDir, "skill.toml"), []byte(`[skill]
name = "multi-skill"
description = "Multi-target skill"

[targets.claude]
`), 0644)

	opencodeSkillDir := filepath.Join(home, ".config", "opencode", "skill", "multi-skill")
	os.MkdirAll(opencodeSkillDir, 0755)
	os.WriteFile(filepath.Join(opencodeSkillDir, "skill.toml"), []byte(`[skill]
name = "multi-skill"
description = "Multi-target skill"

[targets.opencode]
`), 0644)

	// Verify both exist
	if !skillExists(claudeSkillDir) || !skillExists(opencodeSkillDir) {
		t.Fatal("skills should exist in both targets before removal")
	}

	err := runSkillRemove(skillRemoveCmd, []string{"multi-skill"})
	if err != nil {
		t.Fatalf("runSkillRemove failed: %v", err)
	}

	// Verify both were removed
	if skillExists(claudeSkillDir) {
		t.Error("skill should not exist in claude after removal with --target all")
	}
	if skillExists(opencodeSkillDir) {
		t.Error("skill should not exist in opencode after removal with --target all")
	}
}

func TestSkillRemoveRequiresTarget(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	oldTarget := skillRemoveTarget
	oldYes := skillRemoveYes
	defer func() {
		workDir = oldWorkDir
		skillRemoveTarget = oldTarget
		skillRemoveYes = oldYes
	}()
	workDir = tmpDir
	skillRemoveTarget = "" // No target specified
	skillRemoveYes = true

	err := runSkillRemove(skillRemoveCmd, []string{"some-skill"})
	if err == nil {
		t.Fatal("expected error when no target specified")
	}

	if !strings.Contains(err.Error(), "target") {
		t.Errorf("expected error about target, got: %v", err)
	}
}

func TestSkillRemoveYesFlag(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	oldTarget := skillRemoveTarget
	oldYes := skillRemoveYes
	defer func() {
		workDir = oldWorkDir
		skillRemoveTarget = oldTarget
		skillRemoveYes = oldYes
	}()
	workDir = tmpDir
	skillRemoveTarget = "claude"
	skillRemoveYes = true // --yes flag should skip prompt

	// Set up test home directory
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a skill
	skillDir := filepath.Join(home, ".claude", "skills", "yes-flag-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(`[skill]
name = "yes-flag-skill"
description = "Test yes flag"

[targets.claude]
`), 0644)

	// Should not prompt when --yes is set
	err := runSkillRemove(skillRemoveCmd, []string{"yes-flag-skill"})
	if err != nil {
		t.Fatalf("runSkillRemove should succeed with --yes flag: %v", err)
	}

	// Skill should be removed
	if skillExists(skillDir) {
		t.Error("skill should be removed when --yes flag is set")
	}
}

func TestSkillExistsFunction(t *testing.T) {
	tmpDir := t.TempDir()

	// Test non-existent path
	if skillExists(filepath.Join(tmpDir, "nonexistent")) {
		t.Error("skillExists should return false for non-existent path")
	}

	// Test directory without skill.toml
	noTomlDir := filepath.Join(tmpDir, "no-toml")
	os.MkdirAll(noTomlDir, 0755)
	if skillExists(noTomlDir) {
		t.Error("skillExists should return false for directory without skill.toml")
	}

	// Test valid skill directory
	validDir := filepath.Join(tmpDir, "valid")
	os.MkdirAll(validDir, 0755)
	os.WriteFile(filepath.Join(validDir, "skill.toml"), []byte(`[skill]
name = "valid"
description = "Valid skill"

[targets.claude]
`), 0644)
	if !skillExists(validDir) {
		t.Error("skillExists should return true for valid skill directory")
	}
}

func TestSkillRemoveSpecificTarget(t *testing.T) {
	tmpDir := t.TempDir()

	// Save and restore
	oldWorkDir := workDir
	oldTarget := skillRemoveTarget
	oldYes := skillRemoveYes
	defer func() {
		workDir = oldWorkDir
		skillRemoveTarget = oldTarget
		skillRemoveYes = oldYes
	}()
	workDir = tmpDir
	skillRemoveTarget = "opencode" // Only remove from opencode
	skillRemoveYes = true

	// Set up test home directory
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create the skill in both targets
	claudeSkillDir := filepath.Join(home, ".claude", "skills", "target-skill")
	os.MkdirAll(claudeSkillDir, 0755)
	os.WriteFile(filepath.Join(claudeSkillDir, "skill.toml"), []byte(`[skill]
name = "target-skill"
description = "Target skill"

[targets.claude]
`), 0644)

	opencodeSkillDir := filepath.Join(home, ".config", "opencode", "skill", "target-skill")
	os.MkdirAll(opencodeSkillDir, 0755)
	os.WriteFile(filepath.Join(opencodeSkillDir, "skill.toml"), []byte(`[skill]
name = "target-skill"
description = "Target skill"

[targets.opencode]
`), 0644)

	err := runSkillRemove(skillRemoveCmd, []string{"target-skill"})
	if err != nil {
		t.Fatalf("runSkillRemove failed: %v", err)
	}

	// Claude skill should still exist
	if !skillExists(claudeSkillDir) {
		t.Error("claude skill should still exist when removing only from opencode")
	}

	// OpenCode skill should be removed
	if skillExists(opencodeSkillDir) {
		t.Error("opencode skill should be removed")
	}
}
