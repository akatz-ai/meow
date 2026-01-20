package cmd

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/akatz-ai/meow/internal/registry"
	"github.com/spf13/cobra"
)

var registryListCmd = &cobra.Command{
	Use:   "list",
	Short: "List registered registries",
	Long: `List all registered registries.

Shows all registries you've added with 'meow registry add'.
For each registry, displays:
  - NAME:    Registry identifier
  - VERSION: Current cached version
  - SOURCE:  Registry source (URL or GitHub shorthand)

Use 'meow registry show <name>' to see collections in a registry.`,
	RunE: runRegistryList,
}

func init() {
	registryCmd.AddCommand(registryListCmd)
}

func runRegistryList(cmd *cobra.Command, args []string) error {
	store, err := registry.NewRegistriesStore()
	if err != nil {
		return err
	}

	regs, err := store.List()
	if err != nil {
		return fmt.Errorf("listing registries: %w", err)
	}

	if len(regs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No registries registered")
		fmt.Fprintln(cmd.OutOrStdout(), "")
		fmt.Fprintln(cmd.OutOrStdout(), "To add a registry: meow registry add <source>")
		return nil
	}

	// Sort by name for consistent output
	names := make([]string, 0, len(regs))
	for name := range regs {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "REGISTRY\tVERSION\tSOURCE")

	for _, name := range names {
		reg := regs[name]
		fmt.Fprintf(w, "%s\t%s\t%s\n", name, reg.Version, reg.Source)
	}

	return w.Flush()
}
