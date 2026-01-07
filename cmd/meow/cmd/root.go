package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	// Version is set at build time via ldflags
	Version = "dev"

	// Global flags
	verbose bool
	workDir string
)

var rootCmd = &cobra.Command{
	Use:   "meow",
	Short: "MEOW Stack - Molecular Expression Of Work",
	Long: `MEOW is a durable, recursive, composable workflow system for AI agent orchestration.

Built on 6 primitive bead types (task, condition, stop, start, code, expand),
MEOW enables complex multi-agent workflows with crash recovery and context management.

For more information, see: https://github.com/meow-stack/meow-machine`,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "verbose output")
	rootCmd.PersistentFlags().StringVarP(&workDir, "workdir", "C", "", "working directory (default: current)")

	// Version flag
	rootCmd.Version = Version
	rootCmd.SetVersionTemplate("meow {{.Version}}\n")
}

// checkWorkDir ensures we're in a MEOW project directory.
func checkWorkDir() error {
	dir := workDir
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("getting working directory: %w", err)
		}
	}

	// Check for .meow directory
	meowDir := dir + "/.meow"
	if _, err := os.Stat(meowDir); os.IsNotExist(err) {
		return fmt.Errorf("not a MEOW project (no .meow directory). Run 'meow init' first")
	}

	return nil
}

// getWorkDir returns the effective working directory.
func getWorkDir() (string, error) {
	if workDir != "" {
		return workDir, nil
	}
	return os.Getwd()
}
