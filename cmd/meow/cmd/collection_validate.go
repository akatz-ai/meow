package cmd

import (
	"github.com/spf13/cobra"
)

var collectionValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate a collection structure",
	Long: `Validate the structure of a collection (.meow/manifest.json).

If path is omitted, validates the current directory.

Examples:
  meow collection validate                    # Validate current dir
  meow collection validate ./my-collection    # Validate specific collection`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryValidate, // Reuse the same logic
}

func init() {
	collectionCmd.AddCommand(collectionValidateCmd)
}
