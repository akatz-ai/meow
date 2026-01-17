package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/akatz-ai/meow/internal/collection"
	"github.com/akatz-ai/meow/internal/skill"
	"github.com/spf13/cobra"
)

var (
	collectionInstallSkill    string
	collectionInstallNoSkills bool
)

var collectionInstallCmd = &cobra.Command{
	Use:   "install <collection-path>",
	Short: "Install a collection with optional skills",
	Long: `Install a MEOW workflow collection from a local directory.

If the collection includes skills for AI harnesses (Claude Code, OpenCode, etc.),
you can choose to install them as well.

Examples:
  # Install collection and skills for Claude Code
  meow collection install ./my-collection --skill claude

  # Install collection and skills for all supported targets
  meow collection install ./my-collection --skill all

  # Install collection without skills
  meow collection install ./my-collection --no-skills`,
	Args: cobra.ExactArgs(1),
	RunE: runCollectionInstall,
}

func init() {
	collectionInstallCmd.Flags().StringVar(&collectionInstallSkill, "skill", "", "install skills to target harness (claude, opencode, or all)")
	collectionInstallCmd.Flags().BoolVar(&collectionInstallNoSkills, "no-skills", false, "skip skill installation even if collection has skills")
	collectionCmd.AddCommand(collectionInstallCmd)
}

func runCollectionInstall(cmd *cobra.Command, args []string) error {
	collectionPath := args[0]

	// Resolve collection path
	collectionDir, err := filepath.Abs(collectionPath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Check collection directory exists
	if _, err := os.Stat(collectionDir); os.IsNotExist(err) {
		return fmt.Errorf("collection directory not found: %s", collectionDir)
	}

	// Load collection manifest
	col, err := collection.LoadFromDir(collectionDir)
	if err != nil {
		return fmt.Errorf("loading collection manifest: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Collection: %s\n", col.Collection.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Version: %s\n", col.Collection.Version)

	// Handle skills if present
	if len(col.Skills) > 0 {
		if collectionInstallNoSkills {
			fmt.Fprintf(cmd.OutOrStdout(), "\nSkipping %d skill(s) (--no-skills)\n", len(col.Skills))
		} else if collectionInstallSkill != "" {
			if err := installCollectionSkills(cmd, col, collectionDir, collectionInstallSkill); err != nil {
				return err
			}
		} else {
			// No flag specified but collection has skills - just note it for now
			// In the future, this could show an interactive prompt
			fmt.Fprintf(cmd.OutOrStdout(), "\nNote: This collection includes %d skill(s). Use --skill <target> to install them.\n", len(col.Skills))
		}
	}

	return nil
}

// installCollectionSkills installs all skills from a collection to the specified target(s).
func installCollectionSkills(cmd *cobra.Command, col *collection.Collection, collectionDir, target string) error {
	if len(col.Skills) == 0 {
		return nil
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nInstalling %d skill(s)...\n", len(col.Skills))

	for skillName, skillPath := range col.Skills {
		// Resolve skill directory (parent of skill.toml)
		fullSkillPath := filepath.Join(collectionDir, skillPath)
		skillDir := filepath.Dir(fullSkillPath)

		// Load skill manifest
		s, err := skill.LoadFromDir(skillDir)
		if err != nil {
			return fmt.Errorf("loading skill %q: %w", skillName, err)
		}

		// Validate skill
		result := s.Validate(skillDir)
		if result.HasErrors() {
			return fmt.Errorf("invalid skill %q: %s", skillName, result.Error())
		}

		// Determine targets
		var targets []string
		if target == "all" {
			// Install to all targets the skill supports
			for t := range s.Targets {
				if _, known := skill.KnownTargets[t]; known {
					targets = append(targets, t)
				}
			}
			if len(targets) == 0 {
				return fmt.Errorf("skill %q does not support any known targets", skillName)
			}
		} else {
			// Check if target is valid
			if _, ok := skill.KnownTargets[target]; !ok {
				return fmt.Errorf("unknown target %q: valid targets are %v", target, skill.ListKnownTargets())
			}

			// Check if skill supports this target
			if _, ok := s.Targets[target]; !ok {
				return fmt.Errorf("skill %q does not support target %q", skillName, target)
			}

			targets = []string{target}
		}

		// Install to each target
		for _, t := range targets {
			if err := installSkillToTarget(cmd, s, skillDir, t); err != nil {
				return fmt.Errorf("installing skill %q to %s: %w", skillName, t, err)
			}
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "\nSuccessfully installed %d skill(s)\n", len(col.Skills))
	return nil
}

// installSkillToTarget installs a skill to a specific target (reused from skill install logic).
func installSkillToTarget(cmd *cobra.Command, s *skill.Skill, sourceDir, target string) error {
	// Get destination path
	destPath, err := skill.ResolveTargetPath(target, s.Skill.Name, true)
	if err != nil {
		return fmt.Errorf("resolving target path: %w", err)
	}

	// Check if already exists
	if _, err := os.Stat(destPath); err == nil {
		// Remove existing (collection install always overwrites)
		if err := os.RemoveAll(destPath); err != nil {
			return fmt.Errorf("removing existing skill: %w", err)
		}
	}

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// Copy skill directory
	if err := copySkillDir(sourceDir, destPath); err != nil {
		return fmt.Errorf("copying skill: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "  ✓ Installed %q to %s\n", s.Skill.Name, destPath)
	return nil
}
