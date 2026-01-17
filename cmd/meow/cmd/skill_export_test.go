package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// resetExportFlags resets global flags between tests.
func resetExportFlags() {
	skillExportForMarketplace = false
	skillExportOutput = ""
	skillExportRepo = ""
	skillExportDryRun = false
}

func TestSkillExport(t *testing.T) {
	t.Run("exports skill with correct directory structure", func(t *testing.T) {
		resetExportFlags()
		// Setup test skill directory
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills", "test-skill")
		workflowDir := filepath.Join(tmpDir, "workflows")
		outputDir := filepath.Join(tmpDir, "dist")

		// Create skill directory structure
		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("Failed to create skill dir: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(workflowDir, "lib"), 0755); err != nil {
			t.Fatalf("Failed to create workflow dir: %v", err)
		}

		// Create skill.toml
		skillToml := `[skill]
name = "test-skill"
description = "A test skill"
version = "1.0.0"

[targets.claude]

[export]
workflows = [
    "workflows/main.meow.toml",
    "workflows/lib/helpers.meow.toml",
]
requires = ["meow"]

[export.marketplace]
plugin_name = "test-skill"
version = "1.0.0"
`
		if err := os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillToml), 0644); err != nil {
			t.Fatalf("Failed to write skill.toml: %v", err)
		}

		// Create SKILL.md
		skillMd := `# Test Skill

## Overview

A test skill for testing.

## Usage

Use it.
`
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMd), 0644); err != nil {
			t.Fatalf("Failed to write SKILL.md: %v", err)
		}

		// Create workflow files
		mainWorkflow := `[workflow]
name = "main"

[[steps]]
id = "hello"
executor = "shell"
command = "echo hello"
`
		helperWorkflow := `[workflow]
name = "helpers"
`
		if err := os.WriteFile(filepath.Join(workflowDir, "main.meow.toml"), []byte(mainWorkflow), 0644); err != nil {
			t.Fatalf("Failed to write main workflow: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, "lib", "helpers.meow.toml"), []byte(helperWorkflow), 0644); err != nil {
			t.Fatalf("Failed to write helper workflow: %v", err)
		}

		// Run export command
		cmd := rootCmd
		cmd.SetArgs([]string{
			"skill", "export", "test-skill",
			"--for-marketplace",
			"--repo", tmpDir,
			"--output", outputDir,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Export command failed: %v", err)
		}

		// Verify directory structure
		expectedPaths := []string{
			".claude-plugin/marketplace.json",
			"plugins/test-skill/plugin.json",
			"plugins/test-skill/skills/test-skill/SKILL.md",
			"plugins/test-skill/skills/test-skill/workflows/main.meow.toml",
			"plugins/test-skill/skills/test-skill/workflows/lib/helpers.meow.toml",
		}

		for _, path := range expectedPaths {
			fullPath := filepath.Join(outputDir, path)
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				t.Errorf("Expected file not found: %s", path)
			}
		}
	})

	t.Run("generates valid marketplace.json", func(t *testing.T) {
		resetExportFlags()
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills", "market-test")
		workflowDir := filepath.Join(tmpDir, "workflows")
		outputDir := filepath.Join(tmpDir, "dist")

		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("Failed to create skill dir: %v", err)
		}
		if err := os.MkdirAll(workflowDir, 0755); err != nil {
			t.Fatalf("Failed to create workflow dir: %v", err)
		}

		skillToml := `[skill]
name = "market-test"
description = "Test marketplace export"
version = "2.0.0"

[targets.claude]

[export]
workflows = ["workflows/test.meow.toml"]

[export.marketplace]
plugin_name = "my-marketplace-plugin"
version = "2.0.0"
`
		if err := os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillToml), 0644); err != nil {
			t.Fatalf("Failed to write skill.toml: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Test"), 0644); err != nil {
			t.Fatalf("Failed to write SKILL.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, "test.meow.toml"), []byte("[workflow]\nname = \"test\""), 0644); err != nil {
			t.Fatalf("Failed to write workflow: %v", err)
		}

		cmd := rootCmd
		cmd.SetArgs([]string{
			"skill", "export", "market-test",
			"--for-marketplace",
			"--repo", tmpDir,
			"--output", outputDir,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Export command failed: %v", err)
		}

		// Read and verify marketplace.json
		marketplaceFile := filepath.Join(outputDir, ".claude-plugin", "marketplace.json")
		data, err := os.ReadFile(marketplaceFile)
		if err != nil {
			t.Fatalf("Failed to read marketplace.json: %v", err)
		}

		var marketplace map[string]interface{}
		if err := json.Unmarshal(data, &marketplace); err != nil {
			t.Fatalf("Invalid JSON in marketplace.json: %v", err)
		}

		if marketplace["name"] != "my-marketplace-plugin" {
			t.Errorf("marketplace.json name = %q, want %q", marketplace["name"], "my-marketplace-plugin")
		}

		if marketplace["description"] != "Test marketplace export" {
			t.Errorf("marketplace.json description mismatch")
		}

		plugins, ok := marketplace["plugins"].([]interface{})
		if !ok || len(plugins) != 1 {
			t.Errorf("marketplace.json should have exactly 1 plugin")
		}
	})

	t.Run("generates valid plugin.json", func(t *testing.T) {
		resetExportFlags()
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills", "plugin-test")
		workflowDir := filepath.Join(tmpDir, "workflows")
		outputDir := filepath.Join(tmpDir, "dist")

		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("Failed to create skill dir: %v", err)
		}
		if err := os.MkdirAll(workflowDir, 0755); err != nil {
			t.Fatalf("Failed to create workflow dir: %v", err)
		}

		skillToml := `[skill]
name = "plugin-test"
description = "Test plugin.json generation"
version = "1.2.3"

[targets.claude]

[export]
workflows = ["workflows/main.meow.toml"]
`
		if err := os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillToml), 0644); err != nil {
			t.Fatalf("Failed to write skill.toml: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Plugin Test"), 0644); err != nil {
			t.Fatalf("Failed to write SKILL.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, "main.meow.toml"), []byte("[workflow]\nname = \"main\""), 0644); err != nil {
			t.Fatalf("Failed to write workflow: %v", err)
		}

		cmd := rootCmd
		cmd.SetArgs([]string{
			"skill", "export", "plugin-test",
			"--for-marketplace",
			"--repo", tmpDir,
			"--output", outputDir,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Export command failed: %v", err)
		}

		// Read and verify plugin.json
		pluginFile := filepath.Join(outputDir, "plugins", "plugin-test", "plugin.json")
		data, err := os.ReadFile(pluginFile)
		if err != nil {
			t.Fatalf("Failed to read plugin.json: %v", err)
		}

		var plugin map[string]interface{}
		if err := json.Unmarshal(data, &plugin); err != nil {
			t.Fatalf("Invalid JSON in plugin.json: %v", err)
		}

		if plugin["name"] != "plugin-test" {
			t.Errorf("plugin.json name = %q, want %q", plugin["name"], "plugin-test")
		}

		if plugin["version"] != "1.2.3" {
			t.Errorf("plugin.json version = %q, want %q", plugin["version"], "1.2.3")
		}

		if plugin["description"] != "Test plugin.json generation" {
			t.Errorf("plugin.json description mismatch")
		}

		if plugin["skills"] != "./skills/" {
			t.Errorf("plugin.json skills path = %q, want %q", plugin["skills"], "./skills/")
		}
	})

	t.Run("dry-run does not write files", func(t *testing.T) {
		resetExportFlags()
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills", "dry-test")
		workflowDir := filepath.Join(tmpDir, "workflows")
		outputDir := filepath.Join(tmpDir, "dist")

		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("Failed to create skill dir: %v", err)
		}
		if err := os.MkdirAll(workflowDir, 0755); err != nil {
			t.Fatalf("Failed to create workflow dir: %v", err)
		}

		skillToml := `[skill]
name = "dry-test"
description = "Test dry run"

[targets.claude]

[export]
workflows = ["workflows/main.meow.toml"]
`
		if err := os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillToml), 0644); err != nil {
			t.Fatalf("Failed to write skill.toml: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Dry Test"), 0644); err != nil {
			t.Fatalf("Failed to write SKILL.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, "main.meow.toml"), []byte("[workflow]\nname = \"main\""), 0644); err != nil {
			t.Fatalf("Failed to write workflow: %v", err)
		}

		cmd := rootCmd
		cmd.SetArgs([]string{
			"skill", "export", "dry-test",
			"--for-marketplace",
			"--repo", tmpDir,
			"--output", outputDir,
			"--dry-run",
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Export command failed: %v", err)
		}

		// Output directory should not exist (or be empty)
		if _, err := os.Stat(outputDir); !os.IsNotExist(err) {
			// If exists, check it's empty
			entries, _ := os.ReadDir(outputDir)
			if len(entries) > 0 {
				t.Errorf("Dry-run should not create output files")
			}
		}
	})

	t.Run("fails if workflow paths do not exist", func(t *testing.T) {
		resetExportFlags()
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills", "missing-workflow")
		outputDir := filepath.Join(tmpDir, "dist")

		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("Failed to create skill dir: %v", err)
		}

		skillToml := `[skill]
name = "missing-workflow"
description = "Test missing workflow"

[targets.claude]

[export]
workflows = ["workflows/nonexistent.meow.toml"]
`
		if err := os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillToml), 0644); err != nil {
			t.Fatalf("Failed to write skill.toml: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Missing"), 0644); err != nil {
			t.Fatalf("Failed to write SKILL.md: %v", err)
		}

		cmd := rootCmd
		cmd.SetArgs([]string{
			"skill", "export", "missing-workflow",
			"--for-marketplace",
			"--repo", tmpDir,
			"--output", outputDir,
		})

		err := cmd.Execute()
		if err == nil {
			t.Error("Export should fail when workflow paths don't exist")
		}
	})

	t.Run("fails if export.workflows is empty", func(t *testing.T) {
		resetExportFlags()
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills", "empty-workflows")
		outputDir := filepath.Join(tmpDir, "dist")

		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("Failed to create skill dir: %v", err)
		}

		skillToml := `[skill]
name = "empty-workflows"
description = "Test empty workflows"

[targets.claude]

[export]
workflows = []
`
		if err := os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillToml), 0644); err != nil {
			t.Fatalf("Failed to write skill.toml: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Empty"), 0644); err != nil {
			t.Fatalf("Failed to write SKILL.md: %v", err)
		}

		cmd := rootCmd
		cmd.SetArgs([]string{
			"skill", "export", "empty-workflows",
			"--for-marketplace",
			"--repo", tmpDir,
			"--output", outputDir,
		})

		err := cmd.Execute()
		if err == nil {
			t.Error("Export should fail when workflows list is empty")
		}
	})

	t.Run("copies references directory", func(t *testing.T) {
		resetExportFlags()
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills", "refs-test")
		refDir := filepath.Join(skillDir, "references")
		workflowDir := filepath.Join(tmpDir, "workflows")
		outputDir := filepath.Join(tmpDir, "dist")

		if err := os.MkdirAll(refDir, 0755); err != nil {
			t.Fatalf("Failed to create ref dir: %v", err)
		}
		if err := os.MkdirAll(workflowDir, 0755); err != nil {
			t.Fatalf("Failed to create workflow dir: %v", err)
		}

		skillToml := `[skill]
name = "refs-test"
description = "Test references copy"

[targets.claude]

[export]
workflows = ["workflows/main.meow.toml"]
`
		if err := os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillToml), 0644); err != nil {
			t.Fatalf("Failed to write skill.toml: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Refs Test"), 0644); err != nil {
			t.Fatalf("Failed to write SKILL.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(refDir, "usage.md"), []byte("# Usage Guide"), 0644); err != nil {
			t.Fatalf("Failed to write usage.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, "main.meow.toml"), []byte("[workflow]\nname = \"main\""), 0644); err != nil {
			t.Fatalf("Failed to write workflow: %v", err)
		}

		cmd := rootCmd
		cmd.SetArgs([]string{
			"skill", "export", "refs-test",
			"--for-marketplace",
			"--repo", tmpDir,
			"--output", outputDir,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Export command failed: %v", err)
		}

		// Verify references were copied
		refFile := filepath.Join(outputDir, "plugins", "refs-test", "skills", "refs-test", "references", "usage.md")
		if _, err := os.Stat(refFile); os.IsNotExist(err) {
			t.Error("References directory should be copied")
		}
	})

	t.Run("preserves workflow directory structure", func(t *testing.T) {
		resetExportFlags()
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills", "nested-test")
		workflowDir := filepath.Join(tmpDir, "workflows")
		outputDir := filepath.Join(tmpDir, "dist")

		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("Failed to create skill dir: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(workflowDir, "lib", "nested"), 0755); err != nil {
			t.Fatalf("Failed to create workflow dir: %v", err)
		}

		skillToml := `[skill]
name = "nested-test"
description = "Test nested workflow structure"

[targets.claude]

[export]
workflows = [
    "workflows/main.meow.toml",
    "workflows/lib/nested/deep.meow.toml",
]
`
		if err := os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillToml), 0644); err != nil {
			t.Fatalf("Failed to write skill.toml: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Nested"), 0644); err != nil {
			t.Fatalf("Failed to write SKILL.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, "main.meow.toml"), []byte("[workflow]\nname = \"main\""), 0644); err != nil {
			t.Fatalf("Failed to write main workflow: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, "lib", "nested", "deep.meow.toml"), []byte("[workflow]\nname = \"deep\""), 0644); err != nil {
			t.Fatalf("Failed to write deep workflow: %v", err)
		}

		cmd := rootCmd
		cmd.SetArgs([]string{
			"skill", "export", "nested-test",
			"--for-marketplace",
			"--repo", tmpDir,
			"--output", outputDir,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Export command failed: %v", err)
		}

		// Verify nested structure is preserved
		deepFile := filepath.Join(outputDir, "plugins", "nested-test", "skills", "nested-test", "workflows", "lib", "nested", "deep.meow.toml")
		if _, err := os.Stat(deepFile); os.IsNotExist(err) {
			t.Error("Nested workflow directory structure should be preserved")
		}
	})

	t.Run("uses skill name for plugin when marketplace config not set", func(t *testing.T) {
		resetExportFlags()
		tmpDir := t.TempDir()
		skillDir := filepath.Join(tmpDir, "skills", "no-marketplace")
		workflowDir := filepath.Join(tmpDir, "workflows")
		outputDir := filepath.Join(tmpDir, "dist")

		if err := os.MkdirAll(skillDir, 0755); err != nil {
			t.Fatalf("Failed to create skill dir: %v", err)
		}
		if err := os.MkdirAll(workflowDir, 0755); err != nil {
			t.Fatalf("Failed to create workflow dir: %v", err)
		}

		// No [export.marketplace] section
		skillToml := `[skill]
name = "no-marketplace"
description = "Test default plugin name"
version = "1.0.0"

[targets.claude]

[export]
workflows = ["workflows/main.meow.toml"]
`
		if err := os.WriteFile(filepath.Join(skillDir, "skill.toml"), []byte(skillToml), 0644); err != nil {
			t.Fatalf("Failed to write skill.toml: %v", err)
		}
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# No Marketplace"), 0644); err != nil {
			t.Fatalf("Failed to write SKILL.md: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, "main.meow.toml"), []byte("[workflow]\nname = \"main\""), 0644); err != nil {
			t.Fatalf("Failed to write workflow: %v", err)
		}

		cmd := rootCmd
		cmd.SetArgs([]string{
			"skill", "export", "no-marketplace",
			"--for-marketplace",
			"--repo", tmpDir,
			"--output", outputDir,
		})

		if err := cmd.Execute(); err != nil {
			t.Fatalf("Export command failed: %v", err)
		}

		// Verify marketplace.json uses skill name
		data, err := os.ReadFile(filepath.Join(outputDir, ".claude-plugin", "marketplace.json"))
		if err != nil {
			t.Fatalf("Failed to read marketplace.json: %v", err)
		}

		var marketplace map[string]interface{}
		if err := json.Unmarshal(data, &marketplace); err != nil {
			t.Fatalf("Invalid JSON: %v", err)
		}

		if marketplace["name"] != "no-marketplace" {
			t.Errorf("marketplace.json name should default to skill name, got %q", marketplace["name"])
		}
	})
}
