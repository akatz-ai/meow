package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akatz-ai/meow/internal/skill"
	"github.com/spf13/cobra"
)

var skillRemoveCmd = &cobra.Command{
	Use:   "remove <skill-name>",
	Short: "Remove an installed skill",
	Long: `Remove an installed skill from a harness target.

Requires --target to specify which harness to remove from.
Use --target all to remove from all installed locations.

Examples:
  meow skill remove sprint-planner --target claude
  meow skill remove sprint-planner --target all
  meow skill remove sprint-planner --target claude --yes`,
	Args: cobra.ExactArgs(1),
	RunE: runSkillRemove,
}

var (
	skillRemoveTarget string
	skillRemoveYes    bool
)

func init() {
	skillRemoveCmd.Flags().StringVar(&skillRemoveTarget, "target", "", "target harness (required: claude, opencode, or all)")
	skillRemoveCmd.Flags().BoolVarP(&skillRemoveYes, "yes", "y", false, "skip confirmation prompt")
	skillCmd.AddCommand(skillRemoveCmd)
}

func runSkillRemove(cmd *cobra.Command, args []string) error {
	skillName := args[0]

	// Require --target
	if skillRemoveTarget == "" {
		return fmt.Errorf("--target is required (use claude, opencode, or all)")
	}

	// Build list of targets to remove from
	var targets []string
	if skillRemoveTarget == "all" {
		targets = skill.ListKnownTargets()
	} else {
		if _, ok := skill.KnownTargets[skillRemoveTarget]; !ok {
			return fmt.Errorf("unknown target %q: valid targets are %v or 'all'", skillRemoveTarget, skill.ListKnownTargets())
		}
		targets = []string{skillRemoveTarget}
	}

	// Find where the skill is installed
	type installLocation struct {
		target string
		path   string
	}
	var locations []installLocation

	for _, target := range targets {
		targetConfig := skill.KnownTargets[target]
		skillsDir := getSkillsBaseDir(targetConfig.GlobalPath)
		skillDir := filepath.Join(skillsDir, skillName)

		if skillExists(skillDir) {
			locations = append(locations, installLocation{
				target: target,
				path:   skillDir,
			})
		}
	}

	// Check if skill was found
	if len(locations) == 0 {
		if skillRemoveTarget == "all" {
			return fmt.Errorf("skill %q not found in any target", skillName)
		}
		return fmt.Errorf("skill %q not found for target %q", skillName, skillRemoveTarget)
	}

	// Confirmation prompt unless --yes
	if !skillRemoveYes {
		if len(locations) == 1 {
			fmt.Printf("Remove skill %q from %s? [y/N] ", skillName, locations[0].target)
		} else {
			fmt.Printf("Remove skill %q from %d targets?\n", skillName, len(locations))
			for _, loc := range locations {
				fmt.Printf("  - %s: %s\n", loc.target, loc.path)
			}
			fmt.Print("[y/N] ")
		}

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Remove from each location
	for _, loc := range locations {
		if err := os.RemoveAll(loc.path); err != nil {
			return fmt.Errorf("removing skill from %s: %w", loc.target, err)
		}
		fmt.Printf("Removed skill %q from %s\n", skillName, loc.target)
	}

	if len(locations) > 1 {
		fmt.Printf("Removed from %d targets\n", len(locations))
	}

	return nil
}

// skillExists checks if a skill directory exists with a skill.toml file.
func skillExists(path string) bool {
	skillToml := filepath.Join(path, "skill.toml")
	_, err := os.Stat(skillToml)
	return err == nil
}
