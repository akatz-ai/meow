package cmd

import (
	"github.com/spf13/cobra"
)

var registryAddCmd = &cobra.Command{
	Use:   "add <source>",
	Short: "Add a workflow registry",
	Long: `Add a workflow registry from a Git repository.

Examples:
  meow registry add akatz-ai/meow-workflows          # GitHub shorthand
  meow registry add github.com/akatz-ai/meow-workflows
  meow registry add https://gitlab.com/team/workflows.git
  meow registry add ./local-registry                  # Local path (testing)`,
	Args: cobra.ExactArgs(1),
	RunE: runRegistryAdd,
}

func init() {
	registryCmd.AddCommand(registryAddCmd)
}

func runRegistryAdd(cmd *cobra.Command, args []string) error {
	// TODO: Implement
	return nil
}
