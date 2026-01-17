package skill

import (
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

func TestExportConfigParsing(t *testing.T) {
	t.Run("skill with export section", func(t *testing.T) {
		content := `
[skill]
name = "sprint-planner"
description = "Plan and execute sprints"
version = "1.0.0"

[targets.claude]

[export]
workflows = [
    "workflows/sprint.meow.toml",
    "workflows/lib/helpers.meow.toml",
]
requires = ["meow"]
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		if skill.Export == nil {
			t.Fatal("Export should not be nil")
		}

		if len(skill.Export.Workflows) != 2 {
			t.Errorf("Workflows len = %d, want 2", len(skill.Export.Workflows))
		}

		if skill.Export.Workflows[0] != "workflows/sprint.meow.toml" {
			t.Errorf("Workflows[0] = %q, want %q", skill.Export.Workflows[0], "workflows/sprint.meow.toml")
		}

		if len(skill.Export.Requires) != 1 || skill.Export.Requires[0] != "meow" {
			t.Errorf("Requires = %v, want [meow]", skill.Export.Requires)
		}
	})

	t.Run("skill without export section (backwards compatible)", func(t *testing.T) {
		content := `
[skill]
name = "simple-skill"
description = "A skill without export config"

[targets.claude]
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		if skill.Export != nil {
			t.Errorf("Export should be nil for skill without export section, got %+v", skill.Export)
		}
	})

	t.Run("export with marketplace config", func(t *testing.T) {
		content := `
[skill]
name = "marketplace-skill"
description = "A skill for the marketplace"
version = "1.0.0"

[targets.claude]

[export]
workflows = ["workflows/main.meow.toml"]
requires = ["meow"]

[export.marketplace]
plugin_name = "my-awesome-plugin"
version = "2.0.0"
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		if skill.Export == nil {
			t.Fatal("Export should not be nil")
		}

		if skill.Export.Marketplace == nil {
			t.Fatal("Marketplace config should not be nil")
		}

		if skill.Export.Marketplace.PluginName != "my-awesome-plugin" {
			t.Errorf("Marketplace.PluginName = %q, want %q", skill.Export.Marketplace.PluginName, "my-awesome-plugin")
		}

		if skill.Export.Marketplace.Version != "2.0.0" {
			t.Errorf("Marketplace.Version = %q, want %q", skill.Export.Marketplace.Version, "2.0.0")
		}
	})

	t.Run("empty export section is valid", func(t *testing.T) {
		content := `
[skill]
name = "empty-export"
description = "Skill with empty export"

[targets.claude]

[export]
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		// Export should exist (not nil) but have empty/nil fields
		if skill.Export == nil {
			t.Error("Export should not be nil when [export] section exists")
		}
	})

	t.Run("export with only workflows", func(t *testing.T) {
		content := `
[skill]
name = "workflows-only"
description = "Export with only workflows"

[targets.claude]

[export]
workflows = ["workflows/main.meow.toml"]
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		if skill.Export == nil {
			t.Fatal("Export should not be nil")
		}

		if len(skill.Export.Workflows) != 1 {
			t.Errorf("Workflows len = %d, want 1", len(skill.Export.Workflows))
		}

		if skill.Export.Requires != nil && len(skill.Export.Requires) != 0 {
			t.Errorf("Requires should be empty, got %v", skill.Export.Requires)
		}

		if skill.Export.Marketplace != nil {
			t.Errorf("Marketplace should be nil, got %+v", skill.Export.Marketplace)
		}
	})

	t.Run("export with only requires", func(t *testing.T) {
		content := `
[skill]
name = "requires-only"
description = "Export with only requires"

[targets.claude]

[export]
requires = ["meow", "beads"]
`
		var skill Skill
		_, err := toml.NewDecoder(strings.NewReader(content)).Decode(&skill)
		if err != nil {
			t.Fatalf("Decode error = %v", err)
		}

		if skill.Export == nil {
			t.Fatal("Export should not be nil")
		}

		if len(skill.Export.Requires) != 2 {
			t.Errorf("Requires len = %d, want 2", len(skill.Export.Requires))
		}

		if skill.Export.Requires[0] != "meow" || skill.Export.Requires[1] != "beads" {
			t.Errorf("Requires = %v, want [meow beads]", skill.Export.Requires)
		}
	})
}

func TestExportConfigStruct(t *testing.T) {
	t.Run("ExportConfig fields", func(t *testing.T) {
		export := ExportConfig{
			Workflows: []string{"a.meow.toml", "b.meow.toml"},
			Requires:  []string{"meow"},
			Marketplace: &MarketplaceConfig{
				PluginName: "test-plugin",
				Version:    "1.0.0",
			},
		}

		if len(export.Workflows) != 2 {
			t.Errorf("Workflows len = %d, want 2", len(export.Workflows))
		}

		if len(export.Requires) != 1 {
			t.Errorf("Requires len = %d, want 1", len(export.Requires))
		}

		if export.Marketplace == nil {
			t.Fatal("Marketplace should not be nil")
		}

		if export.Marketplace.PluginName != "test-plugin" {
			t.Errorf("Marketplace.PluginName = %q, want %q", export.Marketplace.PluginName, "test-plugin")
		}
	})

	t.Run("MarketplaceConfig fields", func(t *testing.T) {
		marketplace := MarketplaceConfig{
			PluginName: "my-plugin",
			Version:    "2.5.0",
		}

		if marketplace.PluginName != "my-plugin" {
			t.Errorf("PluginName = %q, want %q", marketplace.PluginName, "my-plugin")
		}

		if marketplace.Version != "2.5.0" {
			t.Errorf("Version = %q, want %q", marketplace.Version, "2.5.0")
		}
	})
}

func TestExportConfigParseFile(t *testing.T) {
	t.Run("ParseString with export config", func(t *testing.T) {
		content := `
[skill]
name = "parse-test"
description = "Test parsing with export"

[targets.claude]

[export]
workflows = ["main.meow.toml"]
requires = ["meow"]

[export.marketplace]
plugin_name = "parse-test-plugin"
`
		skill, err := ParseString(content)
		if err != nil {
			t.Fatalf("ParseString error = %v", err)
		}

		if skill.Export == nil {
			t.Fatal("Export should not be nil")
		}

		if skill.Export.Marketplace == nil {
			t.Fatal("Marketplace should not be nil")
		}

		if skill.Export.Marketplace.PluginName != "parse-test-plugin" {
			t.Errorf("Marketplace.PluginName = %q, want %q", skill.Export.Marketplace.PluginName, "parse-test-plugin")
		}
	})
}
