package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

//go:embed templates/*.toml
var embeddedTemplates embed.FS

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a MEOW project",
	Long: `Initialize a new MEOW project in the current directory.

Creates the .meow directory structure with default configuration,
templates, and optionally Claude Code hooks for automation.`,
	RunE: runInit,
}

var (
	initWithHooks     bool
	initSkipTemplates bool
)

func init() {
	initCmd.Flags().BoolVar(&initWithHooks, "hooks", true, "setup Claude Code hooks for automation")
	initCmd.Flags().BoolVar(&initSkipTemplates, "skip-templates", false, "skip copying default templates")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	meowDir := filepath.Join(dir, ".meow")
	if _, err := os.Stat(meowDir); err == nil {
		return fmt.Errorf("MEOW project already initialized (found .meow directory)")
	}

	// Create directory structure
	dirs := []string{
		filepath.Join(meowDir, "templates"),
		filepath.Join(meowDir, "state"),
	}

	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", d, err)
		}
	}

	// Create default config
	configPath := filepath.Join(meowDir, "config.toml")
	configContent := `# MEOW Configuration
version = "1"

[paths]
template_dir = ".meow/templates"
beads_dir = ".beads"
state_dir = ".meow/state"

[defaults]
agent = "claude-1"
stop_grace_period = 10

[orchestrator]
poll_interval = "100ms"
heartbeat_interval = "30s"

[cleanup]
ephemeral = "on_complete"

[logging]
level = "info"
format = "json"
file = ".meow/state/meow.log"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Copy default templates
	if !initSkipTemplates {
		if err := copyEmbeddedTemplates(filepath.Join(meowDir, "templates")); err != nil {
			return fmt.Errorf("copying templates: %w", err)
		}
	}

	// Ensure .beads directory exists
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		return fmt.Errorf("creating .beads directory: %w", err)
	}

	// Setup Claude Code hooks
	var hooksCreated bool
	if initWithHooks {
		hooksCreated, err = setupClaudeHooks(dir)
		if err != nil {
			// Non-fatal - just warn
			fmt.Printf("Warning: could not setup Claude Code hooks: %v\n", err)
		}
	}

	fmt.Println("Initialized MEOW project in", dir)
	fmt.Println("\nCreated:")
	fmt.Println("  .meow/config.toml    - configuration")
	fmt.Println("  .meow/templates/     - workflow templates")
	fmt.Println("  .meow/state/         - runtime state")
	fmt.Println("  .beads/              - bead storage")
	if hooksCreated {
		fmt.Println("  .claude/settings.json - Claude Code hooks")
	}
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Create a workflow: meow run .meow/templates/simple.meow.toml")
	fmt.Println("  2. Check your task:   meow prime")
	fmt.Println("  3. Complete it:       meow close <bead-id>")

	return nil
}

// copyEmbeddedTemplates copies templates from the embedded filesystem.
func copyEmbeddedTemplates(destDir string) error {
	return fs.WalkDir(embeddedTemplates, "templates", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Read embedded file
		content, err := embeddedTemplates.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		// Write to destination
		destPath := filepath.Join(destDir, filepath.Base(path))
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", destPath, err)
		}

		return nil
	})
}

// setupClaudeHooks creates .claude/settings.json with MEOW hooks.
func setupClaudeHooks(dir string) (bool, error) {
	claudeDir := filepath.Join(dir, ".claude")
	settingsPath := filepath.Join(claudeDir, "settings.json")

	// Check if settings already exist
	if _, err := os.Stat(settingsPath); err == nil {
		// File exists - don't overwrite
		return false, nil
	}

	// Create .claude directory
	if err := os.MkdirAll(claudeDir, 0755); err != nil {
		return false, fmt.Errorf("creating .claude directory: %w", err)
	}

	// Create settings with hooks
	settingsContent := `{
  "hooks": {
    "SessionStart": [
      {
        "type": "command",
        "command": "meow prime --hook 2>/dev/null || true"
      }
    ],
    "Stop": [
      {
        "type": "command",
        "command": "meow prime --format prompt 2>/dev/null || true"
      }
    ]
  }
}
`
	if err := os.WriteFile(settingsPath, []byte(settingsContent), 0644); err != nil {
		return false, fmt.Errorf("writing settings: %w", err)
	}

	return true, nil
}
