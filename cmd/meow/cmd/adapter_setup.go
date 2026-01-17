package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/akatz-ai/meow/internal/adapter"
	"github.com/spf13/cobra"
)

var adapterSetupCmd = &cobra.Command{
	Use:   "setup <adapter> [worktree]",
	Short: "Run an adapter's setup script",
	Long: `Run an adapter's setup.sh script for a worktree.

Some adapters need one-time setup per worktree/project (e.g., Claude needs
hooks configured in .claude/settings.json). This command runs the adapter's
setup.sh script if it exists.

The setup script receives:
  - Worktree path as the first argument
  - ADAPTER_DIR environment variable pointing to the adapter directory

Examples:
  meow adapter setup claude /path/to/worktree
  meow adapter setup claude .                   # Current directory
  meow adapter setup claude                     # Defaults to current directory

If the adapter has no setup.sh script, a message is printed and the command
succeeds.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runAdapterSetup,
}

var adapterSetupDryRun bool

func init() {
	adapterSetupCmd.Flags().BoolVar(&adapterSetupDryRun, "dry-run", false, "show what would be done without executing")
	adapterCmd.AddCommand(adapterSetupCmd)
}

func runAdapterSetup(cmd *cobra.Command, args []string) error {
	adapterName := args[0]

	// Determine worktree path
	worktreePath := "."
	if len(args) > 1 {
		worktreePath = args[1]
	}

	// Resolve worktree to absolute path
	absWorktree, err := filepath.Abs(worktreePath)
	if err != nil {
		return fmt.Errorf("resolving worktree path: %w", err)
	}

	// Verify worktree exists
	if _, err := os.Stat(absWorktree); os.IsNotExist(err) {
		return fmt.Errorf("worktree path does not exist: %s", absWorktree)
	}

	// Create registry to find the adapter
	registry, err := adapter.NewDefaultRegistry(absWorktree)
	if err != nil {
		return fmt.Errorf("creating adapter registry: %w", err)
	}

	// First, try to load the adapter to verify it exists
	_, err = registry.Load(adapterName)
	if err != nil {
		return fmt.Errorf("loading adapter: %w", err)
	}

	// Get the adapter directory path
	adapterDir, err := getAdapterDir(registry, adapterName)
	if err != nil {
		return fmt.Errorf("getting adapter directory: %w", err)
	}

	// Check for setup.sh
	setupScript := filepath.Join(adapterDir, "setup.sh")
	if _, err := os.Stat(setupScript); os.IsNotExist(err) {
		fmt.Printf("Adapter %q has no setup.sh script - nothing to do\n", adapterName)
		return nil
	}

	// Dry run mode
	if adapterSetupDryRun {
		fmt.Printf("Would run: %s %s\n", setupScript, absWorktree)
		fmt.Printf("With ADAPTER_DIR=%s\n", adapterDir)
		return nil
	}

	// Run setup.sh
	if verbose {
		fmt.Printf("Running: %s %s\n", setupScript, absWorktree)
	}

	execCmd := exec.Command(setupScript, absWorktree)
	execCmd.Dir = adapterDir
	execCmd.Env = append(os.Environ(), "ADAPTER_DIR="+adapterDir)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if err := execCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("setup script failed with exit code %d", exitErr.ExitCode())
		}
		return fmt.Errorf("running setup script: %w", err)
	}

	fmt.Printf("Adapter %q setup completed for %s\n", adapterName, absWorktree)
	return nil
}

// getAdapterDir returns the filesystem path to an adapter's directory.
func getAdapterDir(registry *adapter.Registry, name string) (string, error) {
	path, err := registry.GetPath(name)
	if err == nil {
		return path, nil
	}

	return "", err
}
