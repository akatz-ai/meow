package cmd

import (
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/akatz-ai/meow/internal/registry"
	"github.com/spf13/cobra"
)

var registryShowCmd = &cobra.Command{
	Use:   "show <registry>",
	Short: "Show registry details and collections",
	Long: `Show detailed information about a registry.

Displays:
  - Registry metadata (name, version, owner)
  - Source URL
  - Available collections with descriptions and tags

If the cached registry is stale, it will be automatically refreshed.`,
	Args: cobra.ExactArgs(1),
	RunE: runRegistryShow,
}

func init() {
	registryCmd.AddCommand(registryShowCmd)
}

func runRegistryShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	// Get registry info from registries store
	store, err := registry.NewRegistriesStore()
	if err != nil {
		return err
	}

	regInfo, err := store.Get(name)
	if err != nil {
		return err
	}

	// Get cache manager
	cache, err := registry.NewCache()
	if err != nil {
		return err
	}

	// Check if cache needs refresh
	fresh, err := cache.IsFresh(name)
	if err != nil {
		return fmt.Errorf("checking cache freshness: %w", err)
	}

	if !fresh {
		fmt.Fprintf(cmd.OutOrStdout(), "Refreshing cache...\n")
		if err := cache.Fetch(name); err != nil {
			return fmt.Errorf("fetching registry: %w", err)
		}
	}

	// Load registry from cache
	reg, err := registry.LoadRegistry(cache.Dir(name))
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	// Print registry details
	fmt.Fprintf(cmd.OutOrStdout(), "Registry: %s (v%s)\n", reg.Name, reg.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "Source: %s\n", regInfo.Source)
	fmt.Fprintf(cmd.OutOrStdout(), "Owner: %s", reg.Owner.Name)
	if reg.Owner.Email != "" {
		fmt.Fprintf(cmd.OutOrStdout(), " <%s>", reg.Owner.Email)
	}
	fmt.Fprintln(cmd.OutOrStdout())

	if reg.Description != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", reg.Description)
	}

	fmt.Fprintln(cmd.OutOrStdout())

	// Print collections table
	if len(reg.Collections) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No collections in this registry")
		return nil
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "COLLECTION\tTAGS\tDESCRIPTION")

	for _, c := range reg.Collections {
		tags := strings.Join(c.Tags, ", ")
		if tags == "" {
			tags = "-"
		}
		desc := c.Description
		if desc == "" {
			desc = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Name, tags, desc)
	}

	return w.Flush()
}
