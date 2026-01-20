package cmd

import (
	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage MEOW workflow registries",
	Long: `Manage MEOW workflow registries that index collections.

Registries are Git repositories containing:
  - Registry manifest (.meow/registry.json)
  - One or more collections
  - Collection metadata

You can add registries, list available collections, and install from them.`,
}

func init() {
	rootCmd.AddCommand(registryCmd)
}
