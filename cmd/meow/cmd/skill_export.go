package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/meow-stack/meow-machine/internal/skill"
	"github.com/spf13/cobra"
)

var (
	skillExportForMarketplace bool
	skillExportOutput         string
	skillExportRepo           string
	skillExportDryRun         bool
)

var skillExportCmd = &cobra.Command{
	Use:   "export <skill-name>",
	Short: "Export a skill for marketplace distribution",
	Long: `Export a skill with its dependent workflows into a self-contained Claude marketplace plugin.

The skill must have an [export] section in its skill.toml with workflows to bundle.

Examples:
  # Export for Claude marketplace
  meow skill export sprint-planner --for-marketplace --output dist/

  # Export from a specific repo
  meow skill export sprint-planner --repo /path/to/my-pack --output dist/

  # Preview what would be exported
  meow skill export sprint-planner --for-marketplace --dry-run`,
	Args: cobra.ExactArgs(1),
	RunE: runSkillExport,
}

func init() {
	skillExportCmd.Flags().BoolVar(&skillExportForMarketplace, "for-marketplace", false, "export for Claude marketplace distribution")
	skillExportCmd.Flags().StringVar(&skillExportOutput, "output", "", "output directory for exported skill")
	skillExportCmd.Flags().StringVar(&skillExportRepo, "repo", "", "path to skill collection repository (defaults to current directory)")
	skillExportCmd.Flags().BoolVar(&skillExportDryRun, "dry-run", false, "show what would be exported without writing files")
	skillCmd.AddCommand(skillExportCmd)
}

func runSkillExport(cmd *cobra.Command, args []string) error {
	skillName := args[0]

	// Currently only marketplace export is supported
	if !skillExportForMarketplace {
		return fmt.Errorf("--for-marketplace flag is required (only marketplace export is currently supported)")
	}

	// Resolve repo path
	repoPath := skillExportRepo
	if repoPath == "" {
		var err error
		repoPath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
	}
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return fmt.Errorf("resolving repo path: %w", err)
	}

	// Load skill manifest
	skillDir := filepath.Join(repoPath, "skills", skillName)
	s, err := skill.LoadFromDir(skillDir)
	if err != nil {
		return fmt.Errorf("loading skill: %w", err)
	}

	// Validate export config
	if s.Export == nil {
		return fmt.Errorf("skill %q has no [export] section in skill.toml", skillName)
	}
	if len(s.Export.Workflows) == 0 {
		return fmt.Errorf("skill %q has empty export.workflows list", skillName)
	}

	// Validate workflow paths exist
	for _, wfPath := range s.Export.Workflows {
		fullPath := filepath.Join(repoPath, wfPath)
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return fmt.Errorf("workflow not found: %s", wfPath)
		}
	}

	// Output directory
	outputDir := skillExportOutput
	if outputDir == "" {
		outputDir = filepath.Join(repoPath, "dist")
	}
	outputDir, err = filepath.Abs(outputDir)
	if err != nil {
		return fmt.Errorf("resolving output path: %w", err)
	}

	if skillExportDryRun {
		return printExportDryRun(cmd, s, skillName, repoPath, outputDir)
	}

	return doExport(cmd, s, skillName, skillDir, repoPath, outputDir)
}

func printExportDryRun(cmd *cobra.Command, s *skill.Skill, skillName, repoPath, outputDir string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Would export skill %q for Claude marketplace...\n\n", skillName)

	fmt.Fprintln(cmd.OutOrStdout(), "Source files:")
	fmt.Fprintf(cmd.OutOrStdout(), "  skills/%s/skill.toml\n", skillName)
	fmt.Fprintf(cmd.OutOrStdout(), "  skills/%s/SKILL.md\n", skillName)

	// Check for references
	refDir := filepath.Join(repoPath, "skills", skillName, "references")
	if _, err := os.Stat(refDir); err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "  skills/%s/references/\n", skillName)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "\nBundled workflows (from export.workflows):")
	for _, wf := range s.Export.Workflows {
		fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", wf)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "\nWould generate:")
	fmt.Fprintf(cmd.OutOrStdout(), "  %s/.claude-plugin/marketplace.json\n", outputDir)
	fmt.Fprintf(cmd.OutOrStdout(), "  %s/plugins/%s/plugin.json\n", outputDir, skillName)
	fmt.Fprintf(cmd.OutOrStdout(), "  %s/plugins/%s/skills/%s/\n", outputDir, skillName, skillName)

	return nil
}

func doExport(cmd *cobra.Command, s *skill.Skill, skillName, skillDir, repoPath, outputDir string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Exporting skill %q for Claude marketplace...\n\n", skillName)

	// Create output structure
	pluginDir := filepath.Join(outputDir, "plugins", skillName)
	skillOutDir := filepath.Join(pluginDir, "skills", skillName)
	workflowOutDir := filepath.Join(skillOutDir, "workflows")
	claudePluginDir := filepath.Join(outputDir, ".claude-plugin")

	// Create directories
	if err := os.MkdirAll(claudePluginDir, 0755); err != nil {
		return fmt.Errorf("creating .claude-plugin directory: %w", err)
	}
	if err := os.MkdirAll(skillOutDir, 0755); err != nil {
		return fmt.Errorf("creating skill output directory: %w", err)
	}

	// Copy skill files (except skill.toml)
	fmt.Fprintln(cmd.OutOrStdout(), "Source files:")
	entries, err := os.ReadDir(skillDir)
	if err != nil {
		return fmt.Errorf("reading skill directory: %w", err)
	}

	for _, entry := range entries {
		// Skip skill.toml - it's not needed in the exported package
		if entry.Name() == "skill.toml" {
			continue
		}

		srcPath := filepath.Join(skillDir, entry.Name())
		dstPath := filepath.Join(skillOutDir, entry.Name())

		if entry.IsDir() {
			if err := copyExportDir(srcPath, dstPath); err != nil {
				return fmt.Errorf("copying directory %s: %w", entry.Name(), err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  skills/%s/%s/\n", skillName, entry.Name())
		} else {
			if err := copyExportFile(srcPath, dstPath); err != nil {
				return fmt.Errorf("copying file %s: %w", entry.Name(), err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  skills/%s/%s\n", skillName, entry.Name())
		}
	}

	// Copy workflows
	fmt.Fprintln(cmd.OutOrStdout(), "\nBundled workflows (from export.workflows):")
	for _, wfPath := range s.Export.Workflows {
		srcPath := filepath.Join(repoPath, wfPath)
		// Preserve directory structure relative to "workflows/"
		relPath := wfPath
		if trimmed, ok := strings.CutPrefix(relPath, "workflows/"); ok {
			relPath = trimmed
		}
		dstPath := filepath.Join(workflowOutDir, relPath)

		// Create parent directory
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("creating workflow directory: %w", err)
		}

		if err := copyExportFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("copying workflow %s: %w", wfPath, err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "  %s → %s\n", wfPath, filepath.Join("plugins", skillName, "skills", skillName, "workflows", relPath))
	}

	// Generate marketplace.json
	marketplaceJSON, err := generateMarketplaceJSON(s, skillName)
	if err != nil {
		return fmt.Errorf("generating marketplace.json: %w", err)
	}
	marketplacePath := filepath.Join(claudePluginDir, "marketplace.json")
	if err := os.WriteFile(marketplacePath, marketplaceJSON, 0644); err != nil {
		return fmt.Errorf("writing marketplace.json: %w", err)
	}

	// Generate plugin.json
	pluginJSON, err := generatePluginJSON(s, skillName)
	if err != nil {
		return fmt.Errorf("generating plugin.json: %w", err)
	}
	pluginPath := filepath.Join(pluginDir, "plugin.json")
	if err := os.WriteFile(pluginPath, pluginJSON, 0644); err != nil {
		return fmt.Errorf("writing plugin.json: %w", err)
	}

	fmt.Fprintln(cmd.OutOrStdout(), "\nGenerated:")
	fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", marketplacePath)
	fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", pluginPath)

	fmt.Fprintf(cmd.OutOrStdout(), "\n✓ Exported to %s/\n\n", outputDir)
	fmt.Fprintln(cmd.OutOrStdout(), "To publish:")
	fmt.Fprintf(cmd.OutOrStdout(), "  cd %s && git init && git add . && git commit -m \"Initial\"\n", outputDir)
	fmt.Fprintln(cmd.OutOrStdout(), "  # Push to GitHub, then users can: /plugin marketplace add <user>/<repo>")

	return nil
}

// MarketplaceJSON represents the .claude-plugin/marketplace.json structure.
type MarketplaceJSON struct {
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Owner       MarketplaceOwner `json:"owner"`
	Plugins     []PluginEntry    `json:"plugins"`
}

// MarketplaceOwner represents the owner field in marketplace.json.
type MarketplaceOwner struct {
	Name string `json:"name"`
}

// PluginEntry represents a plugin entry in marketplace.json.
type PluginEntry struct {
	Name        string `json:"name"`
	Source      string `json:"source"`
	Description string `json:"description"`
}

func generateMarketplaceJSON(s *skill.Skill, skillName string) ([]byte, error) {
	// Use marketplace config if available, otherwise defaults
	name := skillName
	if s.Export.Marketplace != nil && s.Export.Marketplace.PluginName != "" {
		name = s.Export.Marketplace.PluginName
	}

	marketplace := MarketplaceJSON{
		Name:        name,
		Description: s.Skill.Description,
		Owner:       MarketplaceOwner{Name: ""},
		Plugins: []PluginEntry{
			{
				Name:        skillName,
				Source:      fmt.Sprintf("./plugins/%s", skillName),
				Description: s.Skill.Description,
			},
		},
	}

	return json.MarshalIndent(marketplace, "", "  ")
}

// PluginJSON represents the plugin.json structure.
type PluginJSON struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Skills      string `json:"skills"`
}

func generatePluginJSON(s *skill.Skill, skillName string) ([]byte, error) {
	version := s.Skill.Version
	if version == "" {
		version = "1.0.0"
	}
	if s.Export.Marketplace != nil && s.Export.Marketplace.Version != "" {
		version = s.Export.Marketplace.Version
	}

	plugin := PluginJSON{
		Name:        skillName,
		Version:     version,
		Description: s.Skill.Description,
		Skills:      "./skills/",
	}

	return json.MarshalIndent(plugin, "", "  ")
}

// copyExportDir recursively copies a directory.
func copyExportDir(src, dst string) error {
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

		if entry.IsDir() {
			if err := copyExportDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyExportFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyExportFile copies a single file.
func copyExportFile(src, dst string) error {
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
