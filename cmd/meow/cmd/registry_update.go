package cmd

import (
	"fmt"
	"sort"

	"github.com/akatz-ai/meow/internal/registry"
	"github.com/spf13/cobra"
)

var registryUpdateAll bool

var registryUpdateCmd = &cobra.Command{
	Use:   "update [registry]",
	Short: "Update registry from remote",
	Long: `Update a registry by fetching the latest version from its source.

Updates the local cache and metadata for the specified registry.
If the registry has a new version, installed collections may have updates available.

Use --all to update all registered registries.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryUpdate,
}

func init() {
	registryUpdateCmd.Flags().BoolVar(&registryUpdateAll, "all", false, "update all registries")
	registryCmd.AddCommand(registryUpdateCmd)
}

func runRegistryUpdate(cmd *cobra.Command, args []string) error {
	store, err := registry.NewRegistriesStore()
	if err != nil {
		return err
	}

	cache, err := registry.NewCache()
	if err != nil {
		return err
	}

	// Determine which registries to update
	var registriesToUpdate []string

	if registryUpdateAll {
		// Update all registries
		regs, err := store.List()
		if err != nil {
			return fmt.Errorf("listing registries: %w", err)
		}

		for name := range regs {
			registriesToUpdate = append(registriesToUpdate, name)
		}
		sort.Strings(registriesToUpdate)

		if len(registriesToUpdate) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No registries to update")
			return nil
		}
	} else {
		// Update single registry
		if len(args) == 0 {
			return fmt.Errorf("registry name required (or use --all)")
		}

		name := args[0]

		// Verify registry exists
		if _, err := store.Get(name); err != nil {
			return err
		}

		registriesToUpdate = []string{name}
	}

	// Update each registry
	for _, name := range registriesToUpdate {
		fmt.Fprintf(cmd.OutOrStdout(), "Updating %s...\n", name)

		// Fetch from remote
		if err := cache.Fetch(name); err != nil {
			return fmt.Errorf("fetching %s: %w", name, err)
		}

		// Load registry to get new version
		reg, err := registry.LoadRegistry(cache.Dir(name))
		if err != nil {
			return fmt.Errorf("loading %s: %w", name, err)
		}

		// Update version in registries store
		if err := store.Update(name, reg.Version); err != nil {
			return fmt.Errorf("updating %s metadata: %w", name, err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "  Updated to v%s\n", reg.Version)
	}

	if registryUpdateAll {
		fmt.Fprintf(cmd.OutOrStdout(), "\nUpdated %d registries\n", len(registriesToUpdate))
	}

	return nil
}
