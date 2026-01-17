package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for meow-i37g: Implement meow skill install command

const testSkillTOML = `
[skill]
name = "test-skill"
description = "A test skill for installation"
version = "1.0.0"

[targets.claude]

[targets.opencode]
`

func TestSkillInstallToClaude(t *testing.T) {
	// Set up test environment
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a source skill directory
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(testSkillTOML), 0644)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test Skill\nThis is a test."), 0644)

	// Reset flags
	oldTarget := skillInstallTarget
	oldForce := skillInstallForce
	oldDryRun := skillInstallDryRun
	defer func() {
		skillInstallTarget = oldTarget
		skillInstallForce = oldForce
		skillInstallDryRun = oldDryRun
	}()
	skillInstallTarget = "claude"
	skillInstallForce = false
	skillInstallDryRun = false

	// Run install
	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err != nil {
		t.Fatalf("runSkillInstall() error = %v", err)
	}

	// Verify skill was installed to Claude's skill directory
	installedPath := filepath.Join(home, ".claude", "skills", "test-skill")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		t.Errorf("skill should be installed at %s", installedPath)
	}
	if _, err := os.Stat(filepath.Join(installedPath, "skill.toml")); os.IsNotExist(err) {
		t.Error("skill.toml should be copied")
	}
	if _, err := os.Stat(filepath.Join(installedPath, "SKILL.md")); os.IsNotExist(err) {
		t.Error("SKILL.md should be copied")
	}
}

func TestSkillInstallToOpencode(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create source skill
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "opencode-skill")
	os.MkdirAll(skillDir, 0755)
	skillTOML := strings.Replace(testSkillTOML, "test-skill", "opencode-skill", 1)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillTOML), 0644)

	// Reset flags
	oldTarget := skillInstallTarget
	defer func() { skillInstallTarget = oldTarget }()
	skillInstallTarget = "opencode"

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err != nil {
		t.Fatalf("runSkillInstall() error = %v", err)
	}

	// Verify skill was installed to OpenCode's skill directory
	installedPath := filepath.Join(home, ".config", "opencode", "skill", "opencode-skill")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		t.Errorf("skill should be installed at %s", installedPath)
	}
}

func TestSkillInstallToAllTargets(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create source skill
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "multi-target")
	os.MkdirAll(skillDir, 0755)
	skillTOML := strings.Replace(testSkillTOML, "test-skill", "multi-target", 1)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillTOML), 0644)

	// Reset flags
	oldTarget := skillInstallTarget
	defer func() { skillInstallTarget = oldTarget }()
	skillInstallTarget = "all"

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err != nil {
		t.Fatalf("runSkillInstall() error = %v", err)
	}

	// Verify skill was installed to both targets
	claudePath := filepath.Join(home, ".claude", "skills", "multi-target")
	opencodePath := filepath.Join(home, ".config", "opencode", "skill", "multi-target")

	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		t.Errorf("skill should be installed to claude at %s", claudePath)
	}
	if _, err := os.Stat(opencodePath); os.IsNotExist(err) {
		t.Errorf("skill should be installed to opencode at %s", opencodePath)
	}
}

func TestSkillInstallForceOverwrite(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create existing skill installation
	existingPath := filepath.Join(home, ".claude", "skills", "existing-skill")
	os.MkdirAll(existingPath, 0755)
	os.WriteFile(filepath.Join(existingPath, "old-file.txt"), []byte("old content"), 0644)

	// Create source skill
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "existing-skill")
	os.MkdirAll(skillDir, 0755)
	skillTOML := strings.Replace(testSkillTOML, "test-skill", "existing-skill", 1)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillTOML), 0644)
	os.WriteFile(filepath.Join(skillDir, "new-file.txt"), []byte("new content"), 0644)

	// Reset flags
	oldTarget := skillInstallTarget
	oldForce := skillInstallForce
	defer func() {
		skillInstallTarget = oldTarget
		skillInstallForce = oldForce
	}()
	skillInstallTarget = "claude"
	skillInstallForce = true

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err != nil {
		t.Fatalf("runSkillInstall() with --force error = %v", err)
	}

	// Verify new files exist and old files are gone
	if _, err := os.Stat(filepath.Join(existingPath, "new-file.txt")); os.IsNotExist(err) {
		t.Error("new-file.txt should exist after --force install")
	}
	if _, err := os.Stat(filepath.Join(existingPath, "old-file.txt")); !os.IsNotExist(err) {
		t.Error("old-file.txt should be removed after --force install")
	}
}

func TestSkillInstallWithoutForceFailsOnExisting(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create existing skill installation
	existingPath := filepath.Join(home, ".claude", "skills", "conflict-skill")
	os.MkdirAll(existingPath, 0755)
	os.WriteFile(filepath.Join(existingPath, "skill.toml"), []byte("existing"), 0644)

	// Create source skill
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "conflict-skill")
	os.MkdirAll(skillDir, 0755)
	skillTOML := strings.Replace(testSkillTOML, "test-skill", "conflict-skill", 1)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillTOML), 0644)

	// Reset flags - no --force
	oldTarget := skillInstallTarget
	oldForce := skillInstallForce
	defer func() {
		skillInstallTarget = oldTarget
		skillInstallForce = oldForce
	}()
	skillInstallTarget = "claude"
	skillInstallForce = false

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err == nil {
		t.Fatal("expected error when skill already exists without --force")
	}
	if !strings.Contains(err.Error(), "already exists") && !strings.Contains(err.Error(), "force") {
		t.Errorf("error should mention 'already exists' or 'force', got: %v", err)
	}
}

func TestSkillInstallDryRun(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create source skill
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "dryrun-skill")
	os.MkdirAll(skillDir, 0755)
	skillTOML := strings.Replace(testSkillTOML, "test-skill", "dryrun-skill", 1)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillTOML), 0644)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# DryRun"), 0644)

	// Reset flags
	oldTarget := skillInstallTarget
	oldDryRun := skillInstallDryRun
	defer func() {
		skillInstallTarget = oldTarget
		skillInstallDryRun = oldDryRun
	}()
	skillInstallTarget = "claude"
	skillInstallDryRun = true

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err != nil {
		t.Fatalf("runSkillInstall() with --dry-run error = %v", err)
	}

	// Verify skill was NOT actually installed
	installedPath := filepath.Join(home, ".claude", "skills", "dryrun-skill")
	if _, err := os.Stat(installedPath); !os.IsNotExist(err) {
		t.Error("skill should NOT be installed in dry-run mode")
	}

	// Verify output shows what would happen
	output := buf.String()
	if !strings.Contains(output, "dryrun-skill") {
		t.Errorf("dry-run output should mention skill name, got: %q", output)
	}
}

func TestSkillInstallInvalidTarget(t *testing.T) {
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(testSkillTOML), 0644)

	// Reset flags
	oldTarget := skillInstallTarget
	defer func() { skillInstallTarget = oldTarget }()
	skillInstallTarget = "invalid-target"

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err == nil {
		t.Fatal("expected error for invalid target")
	}
	if !strings.Contains(err.Error(), "unknown target") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention invalid target, got: %v", err)
	}
}

func TestSkillInstallInvalidSkillPath(t *testing.T) {
	// Reset flags
	oldTarget := skillInstallTarget
	defer func() { skillInstallTarget = oldTarget }()
	skillInstallTarget = "claude"

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{"/nonexistent/path/to/skill"})
	if err == nil {
		t.Fatal("expected error for nonexistent skill path")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "not exist") && !strings.Contains(err.Error(), "no such") {
		t.Errorf("error should mention path not found, got: %v", err)
	}
}

func TestSkillInstallMissingManifest(t *testing.T) {
	// Create a directory without skill.toml
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "no-manifest")
	os.MkdirAll(skillDir, 0755)
	// No skill.toml!

	// Reset flags
	oldTarget := skillInstallTarget
	defer func() { skillInstallTarget = oldTarget }()
	skillInstallTarget = "claude"

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err == nil {
		t.Fatal("expected error for missing skill.toml")
	}
	if !strings.Contains(err.Error(), "skill.toml") && !strings.Contains(err.Error(), "manifest") {
		t.Errorf("error should mention missing skill.toml, got: %v", err)
	}
}

func TestSkillInstallInvalidManifest(t *testing.T) {
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "invalid-manifest")
	os.MkdirAll(skillDir, 0755)
	// Invalid TOML syntax
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte("invalid { toml"), 0644)

	// Reset flags
	oldTarget := skillInstallTarget
	defer func() { skillInstallTarget = oldTarget }()
	skillInstallTarget = "claude"

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err == nil {
		t.Fatal("expected error for invalid skill.toml syntax")
	}
}

func TestSkillInstallSkillNotSupportingTarget(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create skill that only supports claude
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "claude-only")
	os.MkdirAll(skillDir, 0755)
	claudeOnlyTOML := `
[skill]
name = "claude-only"
description = "Only supports Claude"
version = "1.0.0"

[targets.claude]
# No opencode target!
`
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(claudeOnlyTOML), 0644)

	// Try to install to opencode
	oldTarget := skillInstallTarget
	defer func() { skillInstallTarget = oldTarget }()
	skillInstallTarget = "opencode"

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err == nil {
		t.Fatal("expected error when skill doesn't support target")
	}
	if !strings.Contains(err.Error(), "not support") && !strings.Contains(err.Error(), "target") {
		t.Errorf("error should mention target not supported, got: %v", err)
	}
}

func TestSkillInstallCreatesParentDirectories(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Don't create any skill directories - they shouldn't exist yet
	// The install command should create them

	// Create source skill
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "new-skill")
	os.MkdirAll(skillDir, 0755)
	skillTOML := strings.Replace(testSkillTOML, "test-skill", "new-skill", 1)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillTOML), 0644)

	// Reset flags
	oldTarget := skillInstallTarget
	defer func() { skillInstallTarget = oldTarget }()
	skillInstallTarget = "claude"

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err != nil {
		t.Fatalf("runSkillInstall() error = %v", err)
	}

	// Verify the entire path was created
	installedPath := filepath.Join(home, ".claude", "skills", "new-skill")
	if _, err := os.Stat(installedPath); os.IsNotExist(err) {
		t.Errorf("skill should be installed at %s (parent dirs should be created)", installedPath)
	}
}

func TestSkillInstallCopiesAllFiles(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create source skill with multiple files and subdirectories
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "complex-skill")
	os.MkdirAll(skillDir, 0755)

	skillTOML := strings.Replace(testSkillTOML, "test-skill", "complex-skill", 1)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillTOML), 0644)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Complex Skill"), 0644)

	// Create subdirectory with files
	os.MkdirAll(filepath.Join(skillDir, "references"), 0755)
	os.WriteFile(filepath.Join(skillDir, "references", "api.md"), []byte("API docs"), 0644)
	os.WriteFile(filepath.Join(skillDir, "references", "examples.md"), []byte("Examples"), 0644)

	// Reset flags
	oldTarget := skillInstallTarget
	defer func() { skillInstallTarget = oldTarget }()
	skillInstallTarget = "claude"

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err != nil {
		t.Fatalf("runSkillInstall() error = %v", err)
	}

	// Verify all files were copied
	installedPath := filepath.Join(home, ".claude", "skills", "complex-skill")
	expectedFiles := []string{
		"skill.toml",
		"SKILL.md",
		"references/api.md",
		"references/examples.md",
	}

	for _, file := range expectedFiles {
		fullPath := filepath.Join(installedPath, file)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			t.Errorf("expected file %s to be copied", file)
		}
	}
}

func TestSkillInstallOutputMessage(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create source skill
	sourceDir := t.TempDir()
	skillDir := filepath.Join(sourceDir, "output-skill")
	os.MkdirAll(skillDir, 0755)
	skillTOML := strings.Replace(testSkillTOML, "test-skill", "output-skill", 1)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillTOML), 0644)

	// Reset flags
	oldTarget := skillInstallTarget
	defer func() { skillInstallTarget = oldTarget }()
	skillInstallTarget = "claude"

	var buf bytes.Buffer
	skillInstallCmd.SetOut(&buf)
	skillInstallCmd.SetErr(&buf)

	err := runSkillInstall(skillInstallCmd, []string{skillDir})
	if err != nil {
		t.Fatalf("runSkillInstall() error = %v", err)
	}

	// Verify output message
	output := buf.String()
	if !strings.Contains(output, "output-skill") {
		t.Errorf("output should mention skill name, got: %q", output)
	}
	if !strings.Contains(strings.ToLower(output), "install") {
		t.Errorf("output should indicate installation, got: %q", output)
	}
}
