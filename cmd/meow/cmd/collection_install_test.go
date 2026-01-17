package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for meow-fras: Integrate skill installation into meow collection install

const testCollectionWithSkills = `
[collection]
name = "test-collection"
description = "A test collection with skills"
version = "1.0.0"

[collection.owner]
name = "Test User"

[[packs]]
name = "utils"
description = "Utility workflows"
workflows = ["workflows/test.meow.toml"]

[skills]
test-skill = "skills/test-skill/skill.toml"
`

const testCollectionWithoutSkills = `
[collection]
name = "test-collection"
description = "A test collection without skills"
version = "1.0.0"

[collection.owner]
name = "Test User"

[[packs]]
name = "utils"
description = "Utility workflows"
workflows = ["workflows/test.meow.toml"]
`

const testWorkflow = `
[meta]
name = "test-workflow"
version = "1.0.0"

[[steps]]
id = "start"
executor = "shell"
command = "echo test"
`

const testSkillManifest = `
[skill]
name = "test-skill"
description = "A test skill"
version = "1.0.0"

[targets.claude]

[targets.opencode]
`

func TestCollectionInstallWithSkillsNonInteractiveClaude(t *testing.T) {
	// Set up test environment
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create a test collection directory with skills
	collectionDir := t.TempDir()

	// Create workflow
	workflowDir := filepath.Join(collectionDir, "workflows")
	os.MkdirAll(workflowDir, 0755)
	os.WriteFile(filepath.Join(workflowDir, "test.meow.toml"), []byte(testWorkflow), 0644)

	// Create skill
	skillDir := filepath.Join(collectionDir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(testSkillManifest), 0644)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test Skill"), 0644)

	// Create collection manifest
	os.WriteFile(filepath.Join(collectionDir, "meow-collection.toml"), []byte(testCollectionWithSkills), 0644)

	// Reset flags
	oldSkill := collectionInstallSkill
	oldNoSkills := collectionInstallNoSkills
	defer func() {
		collectionInstallSkill = oldSkill
		collectionInstallNoSkills = oldNoSkills
	}()
	collectionInstallSkill = "claude"
	collectionInstallNoSkills = false

	// Run collection install
	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	err := runCollectionInstall(collectionInstallCmd, []string{collectionDir})
	if err != nil {
		t.Fatalf("runCollectionInstall() error = %v", err)
	}

	// Verify skill was installed to Claude's skill directory
	installedSkillPath := filepath.Join(home, ".claude", "skills", "test-skill")
	if _, err := os.Stat(installedSkillPath); os.IsNotExist(err) {
		t.Errorf("skill should be installed at %s", installedSkillPath)
	}
	if _, err := os.Stat(filepath.Join(installedSkillPath, "skill.toml")); os.IsNotExist(err) {
		t.Error("skill.toml should be copied")
	}
	if _, err := os.Stat(filepath.Join(installedSkillPath, "SKILL.md")); os.IsNotExist(err) {
		t.Error("SKILL.md should be copied")
	}
}

func TestCollectionInstallWithSkillsNonInteractiveAll(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create collection with skills
	collectionDir := t.TempDir()

	workflowDir := filepath.Join(collectionDir, "workflows")
	os.MkdirAll(workflowDir, 0755)
	os.WriteFile(filepath.Join(workflowDir, "test.meow.toml"), []byte(testWorkflow), 0644)

	skillDir := filepath.Join(collectionDir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(testSkillManifest), 0644)

	os.WriteFile(filepath.Join(collectionDir, "meow-collection.toml"), []byte(testCollectionWithSkills), 0644)

	// Reset flags
	oldSkill := collectionInstallSkill
	defer func() { collectionInstallSkill = oldSkill }()
	collectionInstallSkill = "all"

	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	err := runCollectionInstall(collectionInstallCmd, []string{collectionDir})
	if err != nil {
		t.Fatalf("runCollectionInstall() error = %v", err)
	}

	// Verify skill was installed to all targets
	claudePath := filepath.Join(home, ".claude", "skills", "test-skill")
	opencodePath := filepath.Join(home, ".config", "opencode", "skill", "test-skill")

	if _, err := os.Stat(claudePath); os.IsNotExist(err) {
		t.Errorf("skill should be installed to claude at %s", claudePath)
	}
	if _, err := os.Stat(opencodePath); os.IsNotExist(err) {
		t.Errorf("skill should be installed to opencode at %s", opencodePath)
	}
}

func TestCollectionInstallWithSkillsNoSkillsFlag(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create collection with skills
	collectionDir := t.TempDir()

	workflowDir := filepath.Join(collectionDir, "workflows")
	os.MkdirAll(workflowDir, 0755)
	os.WriteFile(filepath.Join(workflowDir, "test.meow.toml"), []byte(testWorkflow), 0644)

	skillDir := filepath.Join(collectionDir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(testSkillManifest), 0644)

	os.WriteFile(filepath.Join(collectionDir, "meow-collection.toml"), []byte(testCollectionWithSkills), 0644)

	// Reset flags
	oldNoSkills := collectionInstallNoSkills
	defer func() { collectionInstallNoSkills = oldNoSkills }()
	collectionInstallNoSkills = true

	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	err := runCollectionInstall(collectionInstallCmd, []string{collectionDir})
	if err != nil {
		t.Fatalf("runCollectionInstall() error = %v", err)
	}

	// Verify skill was NOT installed
	claudePath := filepath.Join(home, ".claude", "skills", "test-skill")
	if _, err := os.Stat(claudePath); !os.IsNotExist(err) {
		t.Error("skill should NOT be installed when --no-skills is used")
	}

	// Output should mention skipping skills
	output := buf.String()
	if !strings.Contains(strings.ToLower(output), "skip") {
		t.Errorf("output should mention skipping skills, got: %q", output)
	}
}

func TestCollectionInstallWithoutSkillsWorks(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create collection WITHOUT skills
	collectionDir := t.TempDir()

	workflowDir := filepath.Join(collectionDir, "workflows")
	os.MkdirAll(workflowDir, 0755)
	os.WriteFile(filepath.Join(workflowDir, "test.meow.toml"), []byte(testWorkflow), 0644)

	os.WriteFile(filepath.Join(collectionDir, "meow-collection.toml"), []byte(testCollectionWithoutSkills), 0644)

	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	err := runCollectionInstall(collectionInstallCmd, []string{collectionDir})
	if err != nil {
		t.Fatalf("runCollectionInstall() error = %v", err)
	}

	// Should complete successfully without any skill installation
	output := buf.String()
	if strings.Contains(strings.ToLower(output), "skill") {
		t.Errorf("output should not mention skills when collection has none, got: %q", output)
	}
}

func TestCollectionInstallInvalidCollectionPath(t *testing.T) {
	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	err := runCollectionInstall(collectionInstallCmd, []string{"/nonexistent/collection"})
	if err == nil {
		t.Fatal("expected error for nonexistent collection path")
	}
	if !strings.Contains(err.Error(), "not found") && !strings.Contains(err.Error(), "not exist") {
		t.Errorf("error should mention path not found, got: %v", err)
	}
}

func TestCollectionInstallMissingManifest(t *testing.T) {
	// Create a directory without meow-collection.toml
	collectionDir := t.TempDir()
	os.MkdirAll(collectionDir, 0755)

	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	err := runCollectionInstall(collectionInstallCmd, []string{collectionDir})
	if err == nil {
		t.Fatal("expected error for missing collection manifest")
	}
	if !strings.Contains(err.Error(), "meow-collection.toml") && !strings.Contains(err.Error(), "manifest") {
		t.Errorf("error should mention missing manifest, got: %v", err)
	}
}

func TestCollectionInstallInvalidManifest(t *testing.T) {
	collectionDir := t.TempDir()

	// Invalid TOML syntax
	os.WriteFile(filepath.Join(collectionDir, "meow-collection.toml"), []byte("invalid { toml"), 0644)

	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	err := runCollectionInstall(collectionInstallCmd, []string{collectionDir})
	if err == nil {
		t.Fatal("expected error for invalid manifest syntax")
	}
}

func TestCollectionInstallInvalidSkillTarget(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create collection with skills
	collectionDir := t.TempDir()

	workflowDir := filepath.Join(collectionDir, "workflows")
	os.MkdirAll(workflowDir, 0755)
	os.WriteFile(filepath.Join(workflowDir, "test.meow.toml"), []byte(testWorkflow), 0644)

	skillDir := filepath.Join(collectionDir, "skills", "test-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(testSkillManifest), 0644)

	os.WriteFile(filepath.Join(collectionDir, "meow-collection.toml"), []byte(testCollectionWithSkills), 0644)

	// Reset flags
	oldSkill := collectionInstallSkill
	defer func() { collectionInstallSkill = oldSkill }()
	collectionInstallSkill = "invalid-target"

	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	err := runCollectionInstall(collectionInstallCmd, []string{collectionDir})
	if err == nil {
		t.Fatal("expected error for invalid skill target")
	}
	if !strings.Contains(err.Error(), "unknown target") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error should mention invalid target, got: %v", err)
	}
}

func TestCollectionInstallMultipleSkills(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create collection with multiple skills
	collectionDir := t.TempDir()

	workflowDir := filepath.Join(collectionDir, "workflows")
	os.MkdirAll(workflowDir, 0755)
	os.WriteFile(filepath.Join(workflowDir, "test.meow.toml"), []byte(testWorkflow), 0644)

	// Create multiple skills
	for _, skillName := range []string{"skill-one", "skill-two"} {
		skillDir := filepath.Join(collectionDir, "skills", skillName)
		os.MkdirAll(skillDir, 0755)
		skillManifest := strings.Replace(testSkillManifest, "test-skill", skillName, 1)
		os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillManifest), 0644)
	}

	multiSkillManifest := `
[collection]
name = "multi-skill-collection"
description = "Collection with multiple skills"
version = "1.0.0"

[collection.owner]
name = "Test User"

[[packs]]
name = "utils"
description = "Utility workflows"
workflows = ["workflows/test.meow.toml"]

[skills]
skill-one = "skills/skill-one/skill.toml"
skill-two = "skills/skill-two/skill.toml"
`
	os.WriteFile(filepath.Join(collectionDir, "meow-collection.toml"), []byte(multiSkillManifest), 0644)

	// Reset flags
	oldSkill := collectionInstallSkill
	defer func() { collectionInstallSkill = oldSkill }()
	collectionInstallSkill = "claude"

	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	err := runCollectionInstall(collectionInstallCmd, []string{collectionDir})
	if err != nil {
		t.Fatalf("runCollectionInstall() error = %v", err)
	}

	// Verify both skills were installed
	for _, skillName := range []string{"skill-one", "skill-two"} {
		skillPath := filepath.Join(home, ".claude", "skills", skillName)
		if _, err := os.Stat(skillPath); os.IsNotExist(err) {
			t.Errorf("skill %q should be installed at %s", skillName, skillPath)
		}
	}
}

func TestCollectionInstallSkillInstallationFailsGracefully(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create collection with a skill that has missing required files
	collectionDir := t.TempDir()

	workflowDir := filepath.Join(collectionDir, "workflows")
	os.MkdirAll(workflowDir, 0755)
	os.WriteFile(filepath.Join(workflowDir, "test.meow.toml"), []byte(testWorkflow), 0644)

	// Create skill with invalid manifest (missing required fields)
	skillDir := filepath.Join(collectionDir, "skills", "bad-skill")
	os.MkdirAll(skillDir, 0755)
	invalidSkill := `
[skill]
# Missing name field
description = "Bad skill"
version = "1.0.0"

[targets.claude]
`
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(invalidSkill), 0644)

	badSkillManifest := `
[collection]
name = "bad-skill-collection"
description = "Collection with invalid skill"
version = "1.0.0"

[collection.owner]
name = "Test User"

[[packs]]
name = "utils"
description = "Utility workflows"
workflows = ["workflows/test.meow.toml"]

[skills]
bad-skill = "skills/bad-skill/skill.toml"
`
	os.WriteFile(filepath.Join(collectionDir, "meow-collection.toml"), []byte(badSkillManifest), 0644)

	// Reset flags
	oldSkill := collectionInstallSkill
	defer func() { collectionInstallSkill = oldSkill }()
	collectionInstallSkill = "claude"

	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	// Should fail during validation
	err := runCollectionInstall(collectionInstallCmd, []string{collectionDir})
	if err == nil {
		t.Fatal("expected error when collection has invalid skill")
	}
}

func TestCollectionInstallOutputShowsInstalledSkills(t *testing.T) {
	home := t.TempDir()
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", home)
	defer os.Setenv("HOME", oldHome)

	// Create collection with skills
	collectionDir := t.TempDir()

	workflowDir := filepath.Join(collectionDir, "workflows")
	os.MkdirAll(workflowDir, 0755)
	os.WriteFile(filepath.Join(workflowDir, "test.meow.toml"), []byte(testWorkflow), 0644)

	skillDir := filepath.Join(collectionDir, "skills", "output-skill")
	os.MkdirAll(skillDir, 0755)
	skillManifest := strings.Replace(testSkillManifest, "test-skill", "output-skill", 1)
	os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillManifest), 0644)

	outputSkillManifest := strings.Replace(testCollectionWithSkills, "test-skill", "output-skill", -1)
	os.WriteFile(filepath.Join(collectionDir, "meow-collection.toml"), []byte(outputSkillManifest), 0644)

	// Reset flags
	oldSkill := collectionInstallSkill
	defer func() { collectionInstallSkill = oldSkill }()
	collectionInstallSkill = "claude"

	var buf bytes.Buffer
	collectionInstallCmd.SetOut(&buf)
	collectionInstallCmd.SetErr(&buf)

	err := runCollectionInstall(collectionInstallCmd, []string{collectionDir})
	if err != nil {
		t.Fatalf("runCollectionInstall() error = %v", err)
	}

	// Verify output mentions the installed skill
	output := buf.String()
	if !strings.Contains(output, "output-skill") {
		t.Errorf("output should mention installed skill name, got: %q", output)
	}
}
