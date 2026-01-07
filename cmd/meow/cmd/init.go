package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a MEOW project",
	Long: `Initialize a new MEOW project in the current directory.

Creates the .meow directory structure with default configuration.`,
	RunE: runInit,
}

func init() {
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

	fmt.Println("Initialized MEOW project in", dir)
	fmt.Println("\nCreated:")
	fmt.Println("  .meow/config.toml    - configuration")
	fmt.Println("  .meow/templates/     - workflow templates")
	fmt.Println("  .meow/state/         - runtime state")
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Add templates to .meow/templates/")
	fmt.Println("  2. Run 'meow run <template>' to start a workflow")

	return nil
}
