package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/akatz-ai/meow/internal/registry"
	"github.com/spf13/cobra"
)

var (
	installLocal  bool
	installForce  bool
	installAs     string
	installDryRun bool
)

var installCmd = &cobra.Command{
	Use:   "install <collection>@<registry>",
	Short: "Install a workflow collection",
	Long: `Install a workflow collection from a registered registry.

Examples:
  meow install sprint@akatz-workflows              # Install to ~/.meow/workflows/
  meow install --local sprint@akatz-workflows      # Install to .meow/workflows/
  meow install sprint@akatz-workflows --as my-sprint  # Install with alias
  meow install github.com/user/meow-sprint         # Direct URL install
  meow install sprint@akatz-workflows --force      # Reinstall/update`,
	Args: cobra.ExactArgs(1),
	RunE: runInstall,
}

func init() {
	installCmd.Flags().BoolVar(&installLocal, "local", false, "install to project instead of user directory")
	installCmd.Flags().BoolVar(&installForce, "force", false, "overwrite existing collection")
	installCmd.Flags().StringVar(&installAs, "as", "", "install with a different name (for conflicts)")
	installCmd.Flags().BoolVar(&installDryRun, "dry-run", false, "show what would be installed without installing")
	rootCmd.AddCommand(installCmd)
}

func runInstall(cmd *cobra.Command, args []string) error {
	ref := args[0]

	// Parse reference: collection@registry or direct URL
	collectionName, registryName, err := parseInstallRef(ref)
	if err != nil {
		return err
	}

	// Use alias if provided
	installName := collectionName
	if installAs != "" {
		installName = installAs
	}

	// Determine destination
	destDir, scope, err := resolveInstallDestination(installName, installLocal)
	if err != nil {
		return err
	}

	// Check for existing
	installedStore, err := registry.NewInstalledStore()
	if err != nil {
		return err
	}

	existing, err := installedStore.Get(installName)
	if err != nil {
		return fmt.Errorf("checking existing installation: %w", err)
	}
	if existing != nil && !installForce {
		return fmt.Errorf("collection %q already exists (from %s)\n\nOptions:\n  meow install %s --force          # Reinstall\n  meow install %s --as <alias>     # Install with different name\n  meow collection remove %s        # Remove first",
			installName, existing.Registry, ref, ref, installName)
	}

	// Get source collection directory
	sourceDir, regVersion, err := resolveCollectionSource(collectionName, registryName)
	if err != nil {
		return err
	}

	// Load manifest
	manifest, err := registry.LoadManifest(sourceDir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	// Dry run output
	if installDryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Would install %s from %s\n\n", collectionName, registryName)
		fmt.Fprintf(cmd.OutOrStdout(), "Destination: %s\n", destDir)
		fmt.Fprintf(cmd.OutOrStdout(), "Entrypoint: %s\n", manifest.Entrypoint)
		return nil
	}

	// Copy collection
	fmt.Fprintf(cmd.OutOrStdout(), "Installing %s from %s...\n\n", collectionName, registryName)

	if err := copyDir(sourceDir, destDir); err != nil {
		return fmt.Errorf("copying collection: %w", err)
	}

	// Track installation
	if err := installedStore.Add(installName, registry.InstalledCollection{
		Registry:        registryName,
		RegistryVersion: regVersion,
		Path:            destDir,
		Scope:           scope,
	}); err != nil {
		return fmt.Errorf("tracking installation: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ Installed %s to %s\n\n", installName, destDir)
	fmt.Fprintf(cmd.OutOrStdout(), "Run: meow run %s\n", installName)

	return nil
}

func parseInstallRef(ref string) (collection, registryName string, err error) {
	// Handle collection@registry format
	if strings.Contains(ref, "@") {
		parts := strings.SplitN(ref, "@", 2)
		return parts[0], parts[1], nil
	}

	// Handle direct URL (github.com/user/repo)
	// TODO: implement direct URL handling
	return "", "", fmt.Errorf("direct URL install not yet supported, use collection@registry format")
}

func resolveInstallDestination(name string, local bool) (string, string, error) {
	if local {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", err
		}
		return filepath.Join(cwd, ".meow", "workflows", name), registry.ScopeProject, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	return filepath.Join(home, ".meow", "workflows", name), registry.ScopeUser, nil
}

func resolveCollectionSource(collection, registryName string) (string, string, error) {
	// Get registry from cache
	cache, err := registry.NewCache()
	if err != nil {
		return "", "", err
	}

	regDir := cache.Dir(registryName)
	if !cache.Exists(registryName) {
		return "", "", fmt.Errorf("registry %q not found. Add it with: meow registry add <source>", registryName)
	}

	// Load registry
	reg, err := registry.LoadRegistry(regDir)
	if err != nil {
		return "", "", err
	}

	// Find collection
	var entry *registry.CollectionEntry
	for i := range reg.Collections {
		if reg.Collections[i].Name == collection {
			entry = &reg.Collections[i]
			break
		}
	}

	if entry == nil {
		return "", "", fmt.Errorf("collection %q not found in registry %q", collection, registryName)
	}

	// Resolve source path
	sourcePath, err := registry.ResolveCollectionSource(*entry, regDir, reg.CollectionRoot)
	if err != nil {
		return "", "", err
	}

	return sourcePath, reg.Version, nil
}
