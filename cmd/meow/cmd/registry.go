package cmd

import (
	"github.com/spf13/cobra"
)

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage MEOW registries",
	Long: `Manage MEOW registries that index workflow collections.

Registries are Git repositories containing a .meow/registry.json file that
indexes one or more collections. You can subscribe to registries to discover
and install collections.

Registries can be added from:
  - GitHub shorthand:  owner/repo
  - GitHub URL:        github.com/owner/repo
  - Git URL:           https://example.com/repo.git

After adding a registry, use 'meow collection install' to install collections
from it.`,
}

func init() {
	rootCmd.AddCommand(registryCmd)
}
