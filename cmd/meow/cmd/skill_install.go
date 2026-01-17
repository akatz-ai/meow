package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/akatz-ai/meow/internal/skill"
	"github.com/spf13/cobra"
)

var (
	skillInstallTarget string
	skillInstallForce  bool
	skillInstallDryRun bool
)

var skillInstallCmd = &cobra.Command{
	Use:   "install <skill-path>",
	Short: "Install a skill from a local path",
	Long: `Install a skill from a local directory to a target AI harness.

The skill directory must contain a valid skill.toml manifest file.

Examples:
  # Install to Claude Code
  meow skill install ./my-skill --target claude

  # Install to OpenCode
  meow skill install ./my-skill --target opencode

  # Install to all supported targets
  meow skill install ./my-skill --target all

  # Preview without installing
  meow skill install ./my-skill --target claude --dry-run

  # Force overwrite existing skill
  meow skill install ./my-skill --target claude --force`,
	Args: cobra.ExactArgs(1),
	RunE: runSkillInstall,
}

func init() {
	skillInstallCmd.Flags().StringVar(&skillInstallTarget, "target", "", "target harness (claude, opencode, or all)")
	skillInstallCmd.Flags().BoolVar(&skillInstallForce, "force", false, "overwrite existing skill")
	skillInstallCmd.Flags().BoolVar(&skillInstallDryRun, "dry-run", false, "show what would be installed without installing")
	_ = skillInstallCmd.MarkFlagRequired("target")
	skillCmd.AddCommand(skillInstallCmd)
}

func runSkillInstall(cmd *cobra.Command, args []string) error {
	sourcePath := args[0]

	// Resolve source path
	sourceDir, err := filepath.Abs(sourcePath)
	if err != nil {
		return fmt.Errorf("resolving path: %w", err)
	}

	// Check source exists
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("skill directory not found: %s", sourceDir)
	}

	// Load and validate skill manifest
	s, err := skill.LoadFromDir(sourceDir)
	if err != nil {
		return fmt.Errorf("loading skill manifest: %w", err)
	}

	// Validate skill
	result := s.Validate(sourceDir)
	if result.HasErrors() {
		return fmt.Errorf("invalid skill: %s", result.Error())
	}

	// Determine targets
	targets, err := resolveTargets(skillInstallTarget, s)
	if err != nil {
		return err
	}

	// Install to each target
	for _, target := range targets {
		if err := installToTarget(cmd, s, sourceDir, target); err != nil {
			return err
		}
	}

	return nil
}

// resolveTargets determines which targets to install to.
func resolveTargets(targetFlag string, s *skill.Skill) ([]string, error) {
	if targetFlag == "all" {
		// Install to all targets the skill supports
		var targets []string
		for t := range s.Targets {
			if _, known := skill.KnownTargets[t]; known {
				targets = append(targets, t)
			}
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("skill does not support any known targets")
		}
		return targets, nil
	}

	// Check if target is valid
	if _, ok := skill.KnownTargets[targetFlag]; !ok {
		return nil, fmt.Errorf("unknown target %q: valid targets are %v", targetFlag, skill.ListKnownTargets())
	}

	// Check if skill supports this target
	if _, ok := s.Targets[targetFlag]; !ok {
		return nil, fmt.Errorf("skill does not support target %q", targetFlag)
	}

	return []string{targetFlag}, nil
}

// installToTarget installs a skill to a specific target.
func installToTarget(cmd *cobra.Command, s *skill.Skill, sourceDir, target string) error {
	// Get destination path
	destPath, err := skill.ResolveTargetPath(target, s.Skill.Name, true)
	if err != nil {
		return fmt.Errorf("resolving target path: %w", err)
	}

	// Check if already exists
	if _, err := os.Stat(destPath); err == nil {
		if !skillInstallForce {
			return fmt.Errorf("skill %q already exists at %s. Use --force to overwrite", s.Skill.Name, destPath)
		}
		// Remove existing
		if !skillInstallDryRun {
			if err := os.RemoveAll(destPath); err != nil {
				return fmt.Errorf("removing existing skill: %w", err)
			}
		}
	}

	if skillInstallDryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Would install skill %q to %s\n", s.Skill.Name, destPath)
		printSkillFiles(cmd, sourceDir)
		return nil
	}

	// Create parent directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("creating parent directory: %w", err)
	}

	// Copy skill directory
	if err := copySkillDir(sourceDir, destPath); err != nil {
		return fmt.Errorf("copying skill: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Installed skill %q to %s\n", s.Skill.Name, destPath)
	return nil
}

// printSkillFiles prints the files that would be installed.
func printSkillFiles(cmd *cobra.Command, sourceDir string) {
	fmt.Fprintln(cmd.OutOrStdout(), "\nFiles:")
	_ = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Skip the root directory itself
		if path == sourceDir {
			return nil
		}
		relPath, _ := filepath.Rel(sourceDir, path)
		if info.IsDir() {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s/\n", relPath)
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", relPath)
		}
		return nil
	})
}

// copySkillDir recursively copies a skill directory.
func copySkillDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// Skip .git directory
		if entry.Name() == ".git" {
			continue
		}

		if entry.IsDir() {
			if err := copySkillDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copySkillFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copySkillFile copies a single file.
func copySkillFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
