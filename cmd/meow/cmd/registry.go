package cmd

import "github.com/spf13/cobra"

var registryCmd = &cobra.Command{
	Use:   "registry",
	Short: "Manage workflow registries",
	Long: `Manage MEOW workflow registries.

A registry is a Git repository containing one or more workflow collections.
Use registries to discover and install workflow collections.`,
}

func init() {
	rootCmd.AddCommand(registryCmd)
}
