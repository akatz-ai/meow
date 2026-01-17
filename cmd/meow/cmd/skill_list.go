package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/akatz-ai/meow/internal/skill"
	"github.com/spf13/cobra"
)

var skillListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed skills",
	Long: `List all installed skills across known AI harness targets.

The output shows:
  - SKILL:  Skill name
  - TARGET: The harness where it's installed
  - PATH:   Installation path

Use --target to filter by a specific harness.
Use --json for machine-readable output.`,
	RunE: runSkillList,
}

var (
	skillListTarget string
	skillListJSON   bool
)

func init() {
	skillListCmd.Flags().StringVar(&skillListTarget, "target", "", "filter by target harness (e.g., claude, opencode)")
	skillListCmd.Flags().BoolVar(&skillListJSON, "json", false, "output as JSON")
	skillCmd.AddCommand(skillListCmd)
}

// skillListEntry represents an installed skill for listing.
type skillListEntry struct {
	Name   string `json:"name"`
	Target string `json:"target"`
	Path   string `json:"path"`
}

func runSkillList(cmd *cobra.Command, args []string) error {
	var entries []skillListEntry

	// Determine which targets to scan
	targets := skill.ListKnownTargets()
	if skillListTarget != "" && skillListTarget != "all" {
		// Validate target
		if _, ok := skill.KnownTargets[skillListTarget]; !ok {
			return fmt.Errorf("unknown target %q: valid targets are %v", skillListTarget, targets)
		}
		targets = []string{skillListTarget}
	}

	// Scan each target for installed skills
	for _, target := range targets {
		targetConfig := skill.KnownTargets[target]

		// Get the base skills directory for this target
		skillsDir := getSkillsBaseDir(targetConfig.GlobalPath)
		if skillsDir == "" {
			continue
		}

		// List skill directories
		files, err := os.ReadDir(skillsDir)
		if err != nil {
			// Directory doesn't exist - no skills installed
			continue
		}

		for _, f := range files {
			if !f.IsDir() {
				continue
			}

			skillDir := filepath.Join(skillsDir, f.Name())
			skillToml := filepath.Join(skillDir, "skill.toml")

			// Check if skill.toml exists
			if _, err := os.Stat(skillToml); err != nil {
				continue
			}

			entries = append(entries, skillListEntry{
				Name:   f.Name(),
				Target: target,
				Path:   skillDir,
			})
		}
	}

	// Sort entries by name, then target
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Target < entries[j].Target
	})

	if skillListJSON {
		return outputSkillListJSON(cmd, entries)
	}

	return outputSkillListText(cmd, entries)
}

// getSkillsBaseDir extracts the base skills directory from a path template.
// e.g., "~/.claude/skills/{{name}}" -> expanded "~/.claude/skills"
func getSkillsBaseDir(pathTemplate string) string {
	// Remove {{name}} placeholder and clean up
	path := pathTemplate
	if idx := len(path) - len("/{{name}}"); idx > 0 && path[idx:] == "/{{name}}" {
		path = path[:idx]
	}

	// Expand ~ to home directory
	return skill.ExpandPath(path)
}

func outputSkillListJSON(cmd *cobra.Command, entries []skillListEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(data))
	return nil
}

func outputSkillListText(cmd *cobra.Command, entries []skillListEntry) error {
	out := cmd.OutOrStdout()

	if len(entries) == 0 {
		fmt.Fprintln(out, "No skills installed.")
		return nil
	}

	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SKILL\tTARGET\tPATH")

	for _, e := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\n", e.Name, e.Target, formatSkillPath(e.Path))
	}

	return w.Flush()
}

// formatSkillPath formats a path for display, using ~ for home directory.
func formatSkillPath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}

	if len(path) > len(home) && path[:len(home)] == home {
		return "~" + path[len(home):]
	}
	return path
}
