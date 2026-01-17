package skill

import (
	"strings"
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	t.Run("renders template with all fields", func(t *testing.T) {
		skill := &Skill{
			Skill: SkillMeta{
				Name:        "sprint-planner",
				Description: "Plan and execute sprint workflows",
				Version:     "1.0.0",
			},
			Export: &ExportConfig{
				Workflows: []string{
					"workflows/sprint.meow.toml",
					"workflows/lib/helpers.meow.toml",
				},
			},
		}

		userContent := `## Usage

Run the sprint planner with your tasks.

### Examples

` + "```bash\nmeow run sprint --var task=\"Plan the sprint\"\n```"

		result := RenderTemplate(skill, userContent)

		// Check name substitution
		if !strings.Contains(result, "# sprint-planner") {
			t.Error("Template should contain skill name as title")
		}

		// Check Prerequisites section
		if !strings.Contains(result, "## Prerequisites") {
			t.Error("Template should contain Prerequisites section")
		}

		// Check MEOW installation instructions
		if !strings.Contains(result, "which meow && meow --version") {
			t.Error("Template should contain MEOW check command")
		}

		// Check curl installation
		if !strings.Contains(result, "curl -fsSL") {
			t.Error("Template should contain curl install command")
		}

		// Check go install option
		if !strings.Contains(result, "go install") {
			t.Error("Template should contain go install command")
		}

		// Check Workflow Setup section
		if !strings.Contains(result, "## Workflow Setup") {
			t.Error("Template should contain Workflow Setup section")
		}

		// Check workflow copy instructions
		if !strings.Contains(result, "cp -r") {
			t.Error("Template should contain workflow copy command")
		}

		// Check bundled workflows are listed
		if !strings.Contains(result, "workflows/sprint.meow.toml") {
			t.Error("Template should list bundled workflows")
		}
		if !strings.Contains(result, "workflows/lib/helpers.meow.toml") {
			t.Error("Template should list all bundled workflows")
		}

		// Check user content is preserved
		if !strings.Contains(result, "## Usage") {
			t.Error("Template should preserve user content")
		}
		if !strings.Contains(result, "Run the sprint planner") {
			t.Error("Template should preserve user content details")
		}
	})

	t.Run("renders template with minimal fields", func(t *testing.T) {
		skill := &Skill{
			Skill: SkillMeta{
				Name:        "simple-skill",
				Description: "A simple skill",
			},
			Export: &ExportConfig{
				Workflows: []string{"workflows/main.meow.toml"},
			},
		}

		result := RenderTemplate(skill, "")

		if !strings.Contains(result, "# simple-skill") {
			t.Error("Template should contain skill name")
		}

		if !strings.Contains(result, "## Prerequisites") {
			t.Error("Template should contain Prerequisites section")
		}
	})

	t.Run("handles nil export config", func(t *testing.T) {
		skill := &Skill{
			Skill: SkillMeta{
				Name:        "no-export",
				Description: "Skill without export config",
			},
			Export: nil,
		}

		result := RenderTemplate(skill, "")

		// Should still render basic template
		if !strings.Contains(result, "# no-export") {
			t.Error("Template should contain skill name")
		}

		// Should not list workflows when none specified
		if strings.Contains(result, "Bundled workflows:") {
			t.Error("Template should not list workflows section when none exist")
		}
	})

	t.Run("handles empty workflows list", func(t *testing.T) {
		skill := &Skill{
			Skill: SkillMeta{
				Name:        "empty-workflows",
				Description: "Skill with empty workflows",
			},
			Export: &ExportConfig{
				Workflows: []string{},
			},
		}

		result := RenderTemplate(skill, "")

		if !strings.Contains(result, "# empty-workflows") {
			t.Error("Template should contain skill name")
		}

		// Should not list workflows when empty
		if strings.Contains(result, "Bundled workflows:") {
			t.Error("Template should not list empty workflows section")
		}
	})
}

func TestInjectSetupSection(t *testing.T) {
	t.Run("injects setup into existing SKILL.md", func(t *testing.T) {
		existing := `---
name: my-skill
---

# My Skill

## Overview

This is a great skill.

## Usage

Use it like this.
`
		skill := &Skill{
			Skill: SkillMeta{
				Name:        "my-skill",
				Description: "A great skill",
			},
			Export: &ExportConfig{
				Workflows: []string{"workflows/main.meow.toml"},
			},
		}

		result := InjectSetupSection(existing, skill)

		// Should preserve Overview
		if !strings.Contains(result, "## Overview") {
			t.Error("Should preserve Overview section")
		}

		// Should inject Prerequisites after Overview
		overviewIdx := strings.Index(result, "## Overview")
		prereqIdx := strings.Index(result, "## Prerequisites")
		usageIdx := strings.Index(result, "## Usage")

		if prereqIdx < 0 {
			t.Fatal("Should inject Prerequisites section")
		}

		if prereqIdx < overviewIdx {
			t.Error("Prerequisites should come after Overview")
		}

		if usageIdx < prereqIdx {
			t.Error("Prerequisites should come before Usage")
		}

		// Should preserve Usage
		if !strings.Contains(result, "Use it like this") {
			t.Error("Should preserve user content")
		}
	})

	t.Run("does not double inject if Prerequisites exists", func(t *testing.T) {
		existing := `# My Skill

## Prerequisites

My custom prerequisites.

## Usage

Use it.
`
		skill := &Skill{
			Skill: SkillMeta{
				Name:        "my-skill",
				Description: "A skill",
			},
			Export: &ExportConfig{
				Workflows: []string{"workflows/main.meow.toml"},
			},
		}

		result := InjectSetupSection(existing, skill)

		// Should preserve existing Prerequisites content
		if !strings.Contains(result, "My custom prerequisites") {
			t.Error("Should preserve existing Prerequisites content")
		}

		// Should not have duplicate Prerequisites
		count := strings.Count(result, "## Prerequisites")
		if count != 1 {
			t.Errorf("Should have exactly 1 Prerequisites section, got %d", count)
		}
	})

	t.Run("does not double inject if Workflow Setup exists", func(t *testing.T) {
		existing := `# My Skill

## Overview

Overview text.

## Workflow Setup

Custom workflow setup instructions.

## Usage

Use it.
`
		skill := &Skill{
			Skill: SkillMeta{
				Name:        "my-skill",
				Description: "A skill",
			},
			Export: &ExportConfig{
				Workflows: []string{"workflows/main.meow.toml"},
			},
		}

		result := InjectSetupSection(existing, skill)

		// Should preserve existing Workflow Setup
		if !strings.Contains(result, "Custom workflow setup instructions") {
			t.Error("Should preserve existing Workflow Setup content")
		}

		// Should not have duplicate
		count := strings.Count(result, "## Workflow Setup")
		if count != 1 {
			t.Errorf("Should have exactly 1 Workflow Setup section, got %d", count)
		}
	})

	t.Run("handles SKILL.md without Overview", func(t *testing.T) {
		existing := `# My Skill

This skill does things.

## Usage

Use it.
`
		skill := &Skill{
			Skill: SkillMeta{
				Name:        "my-skill",
				Description: "A skill",
			},
			Export: &ExportConfig{
				Workflows: []string{"workflows/main.meow.toml"},
			},
		}

		result := InjectSetupSection(existing, skill)

		// Should inject Prerequisites before Usage
		prereqIdx := strings.Index(result, "## Prerequisites")
		usageIdx := strings.Index(result, "## Usage")

		if prereqIdx < 0 {
			t.Fatal("Should inject Prerequisites section")
		}

		if prereqIdx > usageIdx {
			t.Error("Prerequisites should come before Usage")
		}
	})

	t.Run("handles empty content", func(t *testing.T) {
		skill := &Skill{
			Skill: SkillMeta{
				Name:        "my-skill",
				Description: "A skill",
			},
			Export: &ExportConfig{
				Workflows: []string{"workflows/main.meow.toml"},
			},
		}

		result := InjectSetupSection("", skill)

		// Should return basic template when input is empty
		if !strings.Contains(result, "## Prerequisites") {
			t.Error("Should inject Prerequisites into empty content")
		}
	})
}

func TestHasSetupSection(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected bool
	}{
		{
			name:     "has Prerequisites",
			content:  "# Skill\n\n## Prerequisites\n\nSome content",
			expected: true,
		},
		{
			name:     "has Workflow Setup",
			content:  "# Skill\n\n## Workflow Setup\n\nSome content",
			expected: true,
		},
		{
			name:     "has both",
			content:  "# Skill\n\n## Prerequisites\n\n## Workflow Setup\n\nSome content",
			expected: true,
		},
		{
			name:     "has neither",
			content:  "# Skill\n\n## Usage\n\nSome content",
			expected: false,
		},
		{
			name:     "empty content",
			content:  "",
			expected: false,
		},
		{
			name:     "partial match (case sensitive)",
			content:  "## prerequisites\n\nLowercase",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasSetupSection(tt.content)
			if result != tt.expected {
				t.Errorf("HasSetupSection() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGenerateSetupSection(t *testing.T) {
	t.Run("generates complete setup section", func(t *testing.T) {
		workflows := []string{
			"workflows/main.meow.toml",
			"workflows/lib/helper.meow.toml",
		}

		result := GenerateSetupSection(workflows)

		// Prerequisites section
		if !strings.Contains(result, "## Prerequisites") {
			t.Error("Should contain Prerequisites header")
		}

		// Check installation command
		if !strings.Contains(result, "which meow && meow --version") {
			t.Error("Should contain MEOW check command")
		}

		// Install instructions
		if !strings.Contains(result, "### Install MEOW") {
			t.Error("Should contain Install MEOW header")
		}

		// Workflow Setup section
		if !strings.Contains(result, "## Workflow Setup") {
			t.Error("Should contain Workflow Setup header")
		}

		// Lists workflows
		if !strings.Contains(result, "- `workflows/main.meow.toml`") {
			t.Error("Should list main workflow")
		}
		if !strings.Contains(result, "- `workflows/lib/helper.meow.toml`") {
			t.Error("Should list helper workflow")
		}
	})

	t.Run("generates minimal section without workflows", func(t *testing.T) {
		result := GenerateSetupSection(nil)

		// Should still have Prerequisites
		if !strings.Contains(result, "## Prerequisites") {
			t.Error("Should contain Prerequisites header")
		}

		// Should not have Bundled workflows list
		if strings.Contains(result, "**Bundled workflows:**") {
			t.Error("Should not have Bundled workflows section when empty")
		}
	})
}
