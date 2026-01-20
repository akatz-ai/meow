package cmd

import (
	"fmt"
	"os"

	"github.com/akatz-ai/meow/internal/registry"
	"github.com/spf13/cobra"
)

var collectionRemoveCmd = &cobra.Command{
	Use:   "remove <collection>",
	Short: "Remove an installed collection",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		store, err := registry.NewInstalledStore()
		if err != nil {
			return err
		}

		c, err := store.Get(name)
		if err != nil {
			return err
		}
		if c == nil {
			return fmt.Errorf("collection %q not installed", name)
		}

		// Remove directory
		if err := os.RemoveAll(c.Path); err != nil {
			return fmt.Errorf("removing collection: %w", err)
		}

		// Remove from tracking
		if err := store.Remove(name); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "✓ Removed collection %q\n", name)
		return nil
	},
}

func init() {
	collectionCmd.AddCommand(collectionRemoveCmd)
}
