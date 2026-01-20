package cmd

import (
	"fmt"
	"text/tabwriter"

	"github.com/akatz-ai/meow/internal/registry"
	"github.com/spf13/cobra"
)

var collectionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed collections",
	RunE: func(cmd *cobra.Command, args []string) error {
		store, err := registry.NewInstalledStore()
		if err != nil {
			return err
		}

		collections, err := store.List()
		if err != nil {
			return err
		}

		if len(collections) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No collections installed")
			fmt.Fprintln(cmd.OutOrStdout(), "\nTo install: meow install <collection>@<registry>")
			return nil
		}

		w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "COLLECTION\tREGISTRY\tSCOPE\tPATH")
		for name, c := range collections {
			source := c.Registry
			if source == "" {
				source = c.Source
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, source, c.Scope, c.Path)
		}
		return w.Flush()
	},
}

func init() {
	collectionCmd.AddCommand(collectionListCmd)
}
