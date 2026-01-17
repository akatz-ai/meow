package skill

import (
	"fmt"
	"strings"
)

// RenderTemplate generates a complete SKILL.md from a skill manifest and user content.
// It includes Prerequisites and Workflow Setup sections for self-installing skills.
func RenderTemplate(s *Skill, userContent string) string {
	var sb strings.Builder

	// Title
	sb.WriteString(fmt.Sprintf("# %s\n\n", s.Skill.Name))

	// Description
	if s.Skill.Description != "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", s.Skill.Description))
	}

	// Get workflows list
	var workflows []string
	if s.Export != nil && len(s.Export.Workflows) > 0 {
		workflows = s.Export.Workflows
	}

	// Add setup section if we have workflows
	if len(workflows) > 0 {
		sb.WriteString(GenerateSetupSection(workflows))
		sb.WriteString("\n")
	}

	// Add user content
	if userContent != "" {
		sb.WriteString(userContent)
		if !strings.HasSuffix(userContent, "\n") {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// InjectSetupSection injects Prerequisites and Workflow Setup sections into existing SKILL.md.
// If the content already has these sections, it returns the content unchanged.
func InjectSetupSection(existing string, s *Skill) string {
	// If content already has setup sections, don't inject
	if HasSetupSection(existing) {
		return existing
	}

	// If empty, generate from template
	if strings.TrimSpace(existing) == "" {
		return RenderTemplate(s, "")
	}

	// Get workflows
	var workflows []string
	if s.Export != nil && len(s.Export.Workflows) > 0 {
		workflows = s.Export.Workflows
	}

	// If no workflows, nothing to inject
	if len(workflows) == 0 {
		return existing
	}

	setupSection := GenerateSetupSection(workflows)

	// Find where to inject: after ## Overview, or before ## Usage, or at end
	lines := strings.Split(existing, "\n")
	var result []string
	injected := false

	for i, line := range lines {
		result = append(result, line)

		// Inject after Overview section header and its content
		if !injected && strings.HasPrefix(line, "## Overview") {
			// Find the next section header or end
			for j := i + 1; j < len(lines); j++ {
				if strings.HasPrefix(lines[j], "## ") {
					// Insert setup section before next section
					result = append(result, lines[i+1:j]...)
					result = append(result, "")
					result = append(result, strings.Split(strings.TrimSuffix(setupSection, "\n"), "\n")...)
					result = append(result, "")

					// Continue from section j
					for k := j; k < len(lines); k++ {
						result = append(result, lines[k])
					}
					injected = true
					return strings.Join(result, "\n")
				}
			}
		}

		// If no Overview, inject before Usage
		if !injected && strings.HasPrefix(line, "## Usage") {
			// Insert before this line
			result = result[:len(result)-1] // Remove the Usage line we just added
			result = append(result, strings.Split(strings.TrimSuffix(setupSection, "\n"), "\n")...)
			result = append(result, "")
			result = append(result, line) // Re-add Usage
			injected = true
		}
	}

	// If still not injected, append at end
	if !injected {
		result = append(result, "")
		result = append(result, strings.Split(strings.TrimSuffix(setupSection, "\n"), "\n")...)
	}

	return strings.Join(result, "\n")
}

// HasSetupSection checks if content already has Prerequisites or Workflow Setup sections.
func HasSetupSection(content string) bool {
	return strings.Contains(content, "## Prerequisites") ||
		strings.Contains(content, "## Workflow Setup")
}

// GenerateSetupSection generates the Prerequisites and Workflow Setup markdown sections.
func GenerateSetupSection(workflows []string) string {
	var sb strings.Builder

	// Prerequisites section
	sb.WriteString("## Prerequisites\n\n")
	sb.WriteString("This skill uses MEOW for workflow orchestration.\n\n")
	sb.WriteString("### Check Installation\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("which meow && meow --version\n")
	sb.WriteString("```\n\n")
	sb.WriteString("### Install MEOW (if needed)\n\n")
	sb.WriteString("If MEOW is not installed, run:\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("curl -fsSL https://raw.githubusercontent.com/meow-stack/meow-machine/main/install.sh | sh\n")
	sb.WriteString("```\n\n")
	sb.WriteString("Or with Go:\n\n")
	sb.WriteString("```bash\n")
	sb.WriteString("go install github.com/meow-stack/meow-machine/cmd/meow@latest\n")
	sb.WriteString("```\n\n")

	// Workflow Setup section (only if workflows provided)
	if len(workflows) > 0 {
		sb.WriteString("## Workflow Setup (First Time)\n\n")
		sb.WriteString("This skill includes bundled workflows. Install them to your MEOW directory:\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("# Create MEOW directory if needed\n")
		sb.WriteString("mkdir -p ~/.meow/workflows\n\n")
		sb.WriteString("# Find where this skill is installed\n")
		sb.WriteString("# (Claude: use the path where you found this SKILL.md)\n")
		sb.WriteString("SKILL_DIR=\"$(dirname \"$(realpath \"$0\")\")\"\n\n")
		sb.WriteString("# Copy bundled workflows\n")
		sb.WriteString("cp -r \"$SKILL_DIR/workflows/\"* ~/.meow/workflows/\n\n")
		sb.WriteString("# Verify installation\n")
		sb.WriteString("meow ls\n")
		sb.WriteString("```\n\n")
		sb.WriteString("**Bundled workflows:**\n")
		for _, wf := range workflows {
			sb.WriteString(fmt.Sprintf("- `%s`\n", wf))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
