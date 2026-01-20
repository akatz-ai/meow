package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/akatz-ai/meow/internal/registry"
	"github.com/spf13/cobra"
)

var registryAddCmd = &cobra.Command{
	Use:   "add <source>",
	Short: "Add a workflow registry",
	Long: `Add a workflow registry from a Git repository.

Examples:
  meow registry add akatz-ai/meow-workflows          # GitHub shorthand
  meow registry add github.com/akatz-ai/meow-workflows
  meow registry add https://gitlab.com/team/workflows.git
  meow registry add ./local-registry                  # Local path (testing)`,
	Args: cobra.ExactArgs(1),
	RunE: runRegistryAdd,
}

func init() {
	registryCmd.AddCommand(registryAddCmd)
}

func runRegistryAdd(cmd *cobra.Command, args []string) error {
	source := args[0]

	// Initialize stores
	store, err := registry.NewRegistriesStore()
	if err != nil {
		return err
	}

	cache, err := registry.NewCache()
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Fetching registry from %s...\n\n", source)

	// Clone to cache with temp name
	tempName := "_temp_" + filepath.Base(source)
	if err := cache.Clone(tempName, source); err != nil {
		return fmt.Errorf("fetching registry: %w", err)
	}

	// Load registry.json
	reg, err := registry.LoadRegistry(cache.Dir(tempName))
	if err != nil {
		cache.Remove(tempName)
		return fmt.Errorf("loading registry: %w", err)
	}

	// Validate
	result := registry.ValidateRegistry(reg)
	if result.HasErrors() {
		cache.Remove(tempName)
		return fmt.Errorf("invalid registry: %s", result.Error())
	}

	// Check if already registered
	existing, _ := store.Get(reg.Name)
	if existing != nil {
		cache.Remove(tempName)
		return fmt.Errorf("registry %q already registered\n\nTo update: meow registry update %s", reg.Name, reg.Name)
	}

	// Rename cache dir to actual name
	if err := os.Rename(cache.Dir(tempName), cache.Dir(reg.Name)); err != nil {
		cache.Remove(tempName)
		return fmt.Errorf("renaming cache directory: %w", err)
	}

	// Register
	if err := store.Add(reg.Name, source, reg.Version); err != nil {
		cache.Remove(reg.Name)
		return err
	}

	// Print success
	fmt.Fprintf(cmd.OutOrStdout(), "Added registry: %s (v%s)\n", reg.Name, reg.Version)
	fmt.Fprintf(cmd.OutOrStdout(), "  Owner: %s\n\n", reg.Owner.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "  %d collections available:\n", len(reg.Collections))
	for _, c := range reg.Collections {
		fmt.Fprintf(cmd.OutOrStdout(), "    %-20s %s\n", c.Name, c.Description)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "\nBrowse:  meow registry show %s\n", reg.Name)
	fmt.Fprintf(cmd.OutOrStdout(), "Install: meow install <collection>@%s\n", reg.Name)

	return nil
}
