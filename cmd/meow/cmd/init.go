package cmd

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

//go:embed adapters/*/adapter.toml
var embeddedAdapters embed.FS

//go:embed agents_template.md
var embeddedAgentsMD string

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a MEOW project",
	Long: `Initialize a new MEOW project in the current directory.

Creates the following structure:

  .meow/
  ├── config.toml      # Project configuration
  ├── AGENTS.md        # Guidelines for agents working in workflows
  ├── workflows/       # Your workflow definitions (empty by default)
  ├── adapters/        # Agent adapter configs (claude, codex, opencode)
  ├── runs/            # Runtime state for active runs (gitignored)
  └── logs/            # Per-run log files (gitignored)

The runs/ and logs/ directories should be added to .gitignore as they
contain ephemeral runtime state.

AGENTS.md contains guidelines for AI agents working within MEOW workflows.
Include it in your agent worktrees or reference from your project's CLAUDE.md.

To install workflow collections, use 'meow install' (coming soon).`,
	RunE: runInit,
}

func init() {
	initCmd.Flags().BoolP("global", "g", false, "initialize ~/.meow/ instead of .meow/")
	initCmd.Flags().Bool("force", false, "reinitialize even if directory exists")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	global, _ := cmd.Flags().GetBool("global")
	force, _ := cmd.Flags().GetBool("force")

	if global {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("cannot determine home directory: %w", err)
		}
		meowDir := filepath.Join(home, ".meow")
		return initGlobalDirectory(meowDir, force)
	}

	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	meowDir := filepath.Join(dir, ".meow")
	return initProjectDirectory(dir, meowDir, force)
}

func initProjectDirectory(dir, meowDir string, force bool) error {
	if _, err := os.Stat(meowDir); err == nil {
		if !force {
			return fmt.Errorf("MEOW project already initialized (found .meow directory)")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking .meow directory: %w", err)
	}

	// Create directory structure
	dirs := []string{
		filepath.Join(meowDir, "workflows"),
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
	configContent := `# MEOW Project Configuration
version = "1"

[paths]
workflow_dir = ".meow/workflows"
runs_dir = ".meow/runs"
logs_dir = ".meow/logs"

[orchestrator]
poll_interval = "100ms"

[logging]
level = "info"

[agent]
# default_adapter controls which adapter spawn steps use when none is specified.
default_adapter = "claude"
`
	if _, err := writeFileIfMissing(configPath, []byte(configContent)); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Copy default adapters
	if err := copyEmbeddedAdapters(filepath.Join(meowDir, "adapters"), force); err != nil {
		return fmt.Errorf("copying adapters: %w", err)
	}

	// Write AGENTS.md (agent guidelines for workflow participants)
	agentsPath := filepath.Join(meowDir, "AGENTS.md")
	if _, err := writeFileIfMissing(agentsPath, []byte(embeddedAgentsMD)); err != nil {
		return fmt.Errorf("writing AGENTS.md: %w", err)
	}

	// Ensure .beads directory exists
	beadsDir := filepath.Join(dir, ".beads")
	if err := os.MkdirAll(beadsDir, 0755); err != nil {
		return fmt.Errorf("creating .beads directory: %w", err)
	}

	fmt.Println("Initialized MEOW project in", dir)
	fmt.Println("\nCreated:")
	fmt.Println("  .meow/config.toml  - configuration")
	fmt.Println("  .meow/AGENTS.md    - agent workflow guidelines")
	fmt.Println("  .meow/workflows/   - your workflow definitions (empty)")
	fmt.Println("  .meow/adapters/    - adapter configs")
	fmt.Println("  .meow/runs/        - run state files")
	fmt.Println("  .meow/logs/        - per-run log files")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Add workflows to .meow/workflows/")
	fmt.Println("  2. Run a workflow:  meow run <workflow>")
	fmt.Println("  3. Check status:    meow status")

	return nil
}

func initGlobalDirectory(meowDir string, force bool) error {
	if _, err := os.Stat(meowDir); err == nil {
		if !force {
			fmt.Println("~/.meow/ already exists.")
			fmt.Println()
			fmt.Println("Use --force to reinitialize (existing files preserved, missing files created).")
			return nil
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("checking ~/.meow directory: %w", err)
	}

	if force {
		fmt.Println("Reinitializing ~/.meow/...")
	} else {
		fmt.Println("Initializing user-global MEOW directory...")
	}

	if _, err := ensureDir(meowDir); err != nil {
		return fmt.Errorf("creating directory %s: %w", meowDir, err)
	}

	workflowsDir := filepath.Join(meowDir, "workflows")
	workflowsLibDir := filepath.Join(workflowsDir, "lib")
	adaptersDir := filepath.Join(meowDir, "adapters")

	workflowsCreated, err := ensureDir(workflowsDir)
	if err != nil {
		return fmt.Errorf("creating workflows directory: %w", err)
	}

	workflowsLibCreated, err := ensureDir(workflowsLibDir)
	if err != nil {
		return fmt.Errorf("creating workflows/lib directory: %w", err)
	}

	adaptersCreated, err := ensureDir(adaptersDir)
	if err != nil {
		return fmt.Errorf("creating adapters directory: %w", err)
	}

	// Copy default adapters (skip existing ones)
	if err := copyEmbeddedAdapters(adaptersDir, true); err != nil {
		return fmt.Errorf("copying adapters: %w", err)
	}

	globalConfigContent := `# MEOW Global Configuration
# These settings apply to all projects unless overridden.

version = "1"

[agent]
# default_adapter controls which adapter spawn steps use when none is specified.
# Uncomment to set a global default:
# default_adapter = "claude"
`

	configPath := filepath.Join(meowDir, "config.toml")
	configCreated, err := writeFileIfMissing(configPath, []byte(globalConfigContent))
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	if force {
		if configCreated {
			fmt.Println("  config.toml created")
		} else {
			fmt.Println("  config.toml already exists (preserved)")
		}
		if workflowsCreated {
			fmt.Println("  workflows/ created")
		} else {
			fmt.Println("  workflows/ already exists (preserved)")
		}
		if workflowsLibCreated {
			fmt.Println("  workflows/lib/ created")
		} else {
			fmt.Println("  workflows/lib/ already exists (preserved)")
		}
		if adaptersCreated {
			fmt.Println("  adapters/ created")
		} else {
			fmt.Println("  adapters/ already exists (preserved)")
		}
		fmt.Println()
		fmt.Println("Done.")
		return nil
	}

	fmt.Println()
	fmt.Println("Created ~/.meow/")
	fmt.Println("├── config.toml      # Global configuration")
	fmt.Println("├── workflows/       # Your reusable workflows")
	fmt.Println("│   └── lib/         # Utilities (filtered from meow ls by default)")
	fmt.Println("└── adapters/        # Custom agent adapters")
	fmt.Println()
	fmt.Println("To add a global workflow:")
	fmt.Println("  cp my-workflow.meow.toml ~/.meow/workflows/")
	fmt.Println()
	fmt.Println("To install from a collection:")
	fmt.Println("  meow collection add github.com/user/collection")
	fmt.Println("  meow install collection/pack")
	fmt.Println()
	fmt.Println("Global workflows are available in all your projects.")

	return nil
}

func ensureDir(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if err := os.MkdirAll(path, 0755); err != nil {
		return false, err
	}
	return true, nil
}

func writeFileIfMissing(path string, content []byte) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, err
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return false, err
	}
	return true, nil
}

// copyEmbeddedAdapters copies adapter configs from the embedded filesystem.
func copyEmbeddedAdapters(destDir string, skipExisting bool) error {
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

		if skipExisting {
			if _, err := os.Stat(destPath); err == nil {
				return nil
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("checking adapter %s: %w", destPath, err)
			}
		}

		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("writing %s: %w", destPath, err)
		}

		return nil
	})
}
