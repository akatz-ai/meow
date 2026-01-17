package cmd

import (
	"github.com/spf13/cobra"
)

var collectionCmd = &cobra.Command{
	Use:   "collection",
	Short: "Manage MEOW workflow collections",
	Long: `Manage MEOW workflow collections that bundle workflows and skills.

Collections are repositories containing:
  - Workflows (.meow.toml files)
  - Skills for AI harnesses (optional)
  - Collection manifest (meow-collection.toml)

You can install collections from local directories or remote URLs.`,
}

func init() {
	rootCmd.AddCommand(collectionCmd)
}
