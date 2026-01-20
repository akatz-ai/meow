package cmd

import (
	"fmt"

	"github.com/akatz-ai/meow/internal/registry"
	"github.com/spf13/cobra"
)

var registryRemoveCmd = &cobra.Command{
	Use:   "remove <registry>",
	Short: "Unregister a registry",
	Long: `Unregister a registry and remove its cache.

Removes the registry from your subscriptions and cleans up cached data.
If you have collections installed from this registry, they will remain
installed but will no longer receive updates.

To reinstall later, use 'meow registry add <source>'.`,
	Args: cobra.ExactArgs(1),
	RunE: runRegistryRemove,
}

func init() {
	registryCmd.AddCommand(registryRemoveCmd)
}

func runRegistryRemove(cmd *cobra.Command, args []string) error {
	name := args[0]

	store, err := registry.NewRegistriesStore()
	if err != nil {
		return err
	}

	// Verify registry exists
	if _, err := store.Get(name); err != nil {
		return err
	}

	// Check for installed collections from this registry
	installedStore, err := registry.NewInstalledStore()
	if err != nil {
		return err
	}

	installed, err := installedStore.List()
	if err != nil {
		return fmt.Errorf("checking installed collections: %w", err)
	}

	var installedFromRegistry []string
	for collName, coll := range installed {
		if coll.Registry == name {
			installedFromRegistry = append(installedFromRegistry, collName)
		}
	}

	if len(installedFromRegistry) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "Warning: The following collections from %s will remain installed but won't receive updates:\n", name)
		for _, collName := range installedFromRegistry {
			fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", collName)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	// Remove from registries store
	if err := store.Remove(name); err != nil {
		return fmt.Errorf("removing registry: %w", err)
	}

	// Remove cache
	cache, err := registry.NewCache()
	if err != nil {
		return err
	}

	if err := cache.Remove(name); err != nil {
		// Non-fatal: registry is already unregistered
		fmt.Fprintf(cmd.OutOrStdout(), "Warning: failed to remove cache for %s: %v\n", name, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed registry: %s\n", name)

	return nil
}
