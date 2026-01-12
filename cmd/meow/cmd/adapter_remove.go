package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/meow-stack/meow-machine/internal/adapter"
	"github.com/spf13/cobra"
)

var adapterRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove an installed adapter",
	Long: `Remove an installed adapter from the filesystem.

By default, removes the project-local adapter if it exists, otherwise removes
the global adapter. Use --project or --global to specify which to remove.

Examples:
  meow adapter remove aider           # Remove project-local first, then global
  meow adapter remove aider --project # Remove only project-local
  meow adapter remove aider --global  # Remove only global
  meow adapter remove aider --force   # Skip confirmation prompt`,
	Args: cobra.ExactArgs(1),
	RunE: runAdapterRemove,
}

var (
	adapterRemoveForce   bool
	adapterRemoveProject bool
	adapterRemoveGlobal  bool
)

func init() {
	adapterRemoveCmd.Flags().BoolVarP(&adapterRemoveForce, "force", "f", false, "skip confirmation prompt")
	adapterRemoveCmd.Flags().BoolVar(&adapterRemoveProject, "project", false, "remove project-local adapter only")
	adapterRemoveCmd.Flags().BoolVar(&adapterRemoveGlobal, "global", false, "remove global adapter only")
	adapterCmd.AddCommand(adapterRemoveCmd)
}

func runAdapterRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Validate mutually exclusive flags
	if adapterRemoveProject && adapterRemoveGlobal {
		return fmt.Errorf("--project and --global are mutually exclusive")
	}

	// Determine directories to check
	globalDir := adapter.DefaultGlobalDir()
	projectDir := ""
	if dir, err := getWorkDir(); err == nil {
		projectDir = filepath.Join(dir, ".meow", "adapters")
	}

	// Find where the adapter exists
	var adapterPath string
	var location string // "project" or "global"

	projectPath := filepath.Join(projectDir, name)
	globalPath := filepath.Join(globalDir, name)

	projectExists := adapterExists(projectPath)
	globalExists := adapterExists(globalPath)

	// Determine which adapter to remove based on flags
	if adapterRemoveProject {
		if !projectExists {
			return fmt.Errorf("adapter %q not found in project directory (%s)", name, projectDir)
		}
		adapterPath = projectPath
		location = "project"
	} else if adapterRemoveGlobal {
		if !globalExists {
			return fmt.Errorf("adapter %q not found in global directory (%s)", name, globalDir)
		}
		adapterPath = globalPath
		location = "global"
	} else {
		// Default behavior: prefer project-local, then global
		if projectExists {
			adapterPath = projectPath
			location = "project"
		} else if globalExists {
			adapterPath = globalPath
			location = "global"
		} else {
			return fmt.Errorf("adapter %q not found", name)
		}
	}

	// Confirmation prompt unless --force
	if !adapterRemoveForce {
		fmt.Printf("Remove %s adapter %q from %s? [y/N] ", location, name, adapterPath)
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

	// Remove the adapter directory
	if err := os.RemoveAll(adapterPath); err != nil {
		return fmt.Errorf("removing adapter: %w", err)
	}

	fmt.Printf("Removed %s adapter %q\n", location, name)

	// Inform user if adapter exists in another location
	if location == "project" && globalExists {
		fmt.Printf("Note: Global adapter %q still exists at %s\n", name, globalPath)
	} else if location == "global" && projectExists {
		fmt.Printf("Note: Project adapter %q still exists at %s\n", name, projectPath)
	}

	return nil
}

// adapterExists checks if an adapter directory exists with an adapter.toml file.
func adapterExists(path string) bool {
	configPath := filepath.Join(path, "adapter.toml")
	_, err := os.Stat(configPath)
	return err == nil
}
