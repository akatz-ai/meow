package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// meowHooksJSON is the Claude Code settings.json content that configures
// the Stop hook to emit meow events for the Ralph Wiggum persistence loop.
const meowHooksJSON = `{
  "hooks": {
    "Stop": [
      {
        "type": "command",
        "command": "meow event agent-stopped"
      }
    ]
  }
}`

//go:embed templates/*.toml
var embeddedTemplates embed.FS

//go:embed adapters/*/adapter.toml
var embeddedAdapters embed.FS

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a MEOW project",
	Long: `Initialize a new MEOW project in the current directory.

Creates the following structure:

  .meow/
  ├── config.toml      # Project configuration
  ├── templates/       # Workflow templates (starter templates included)
  │   ├── simple.meow.toml
  │   └── tdd.meow.toml
  ├── adapters/        # Agent adapter configs (claude, codex, opencode)
  ├── runs/            # Runtime state for active runs (gitignored)
  └── logs/            # Per-run log files (gitignored)

The runs/ and logs/ directories should be added to .gitignore as they
contain ephemeral runtime state.

Use --hooks to also create .claude/settings.json with MEOW hooks for
agent automation (typically only needed in agent worktrees).`,
	RunE: runInit,
}

var (
	initWithHooks     bool
	initSkipTemplates bool
)

func init() {
	initCmd.Flags().BoolVar(&initWithHooks, "hooks", false, "setup Claude Code hooks for automation (use only in agent worktrees)")
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
		filepath.Join(meowDir, "runs"),
		filepath.Join(meowDir, "logs"),
		filepath.Join(meowDir, "adapters"),
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
runs_dir = ".meow/runs"
logs_dir = ".meow/logs"

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

[agent]
# setup_hooks controls whether spawned agents get .claude/settings.json with MEOW hooks.
# When true (default), agents run the Ralph Wiggum loop for autonomous operation.
# Set to false to disable automatic hook injection for all agents.
setup_hooks = true
# default_adapter controls which adapter spawn steps use when none is specified.
# Change this if you prefer a different default adapter.
default_adapter = "claude"
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

	// Copy default adapters
	if err := copyEmbeddedAdapters(filepath.Join(meowDir, "adapters")); err != nil {
		return fmt.Errorf("copying adapters: %w", err)
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
	fmt.Println("  .meow/adapters/      - adapter configs")
	fmt.Println("  .meow/runs/          - run state files")
	fmt.Println("  .meow/logs/          - per-run log files")
	if hooksCreated {
		fmt.Println("  .claude/settings.json - Claude Code hooks")
	}
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Run a workflow:    meow run simple")
	fmt.Println("  2. Check your task:   meow prime")
	fmt.Println("  3. Complete it:       meow done")

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

// copyEmbeddedAdapters copies adapter configs from the embedded filesystem.
func copyEmbeddedAdapters(destDir string) error {
	return fs.WalkDir(embeddedAdapters, "adapters", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		// Read embedded file
		content, err := embeddedAdapters.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		relPath, err := filepath.Rel("adapters", path)
		if err != nil {
			return fmt.Errorf("resolving adapter path %s: %w", path, err)
		}
		destPath := filepath.Join(destDir, relPath)

		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("creating adapter dir %s: %w", filepath.Dir(destPath), err)
		}

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
	if err := os.WriteFile(settingsPath, []byte(meowHooksJSON), 0644); err != nil {
		return false, fmt.Errorf("writing settings: %w", err)
	}

	return true, nil
}
