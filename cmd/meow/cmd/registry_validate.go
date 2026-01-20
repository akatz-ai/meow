package cmd

import (
	"fmt"

	"github.com/akatz-ai/meow/internal/registry"
	"github.com/spf13/cobra"
)

var registryValidateCmd = &cobra.Command{
	Use:   "validate [path]",
	Short: "Validate a registry or collection structure",
	Long: `Validate the structure of a registry (.meow/registry.json) or
collection (.meow/manifest.json).

If path is omitted, validates the current directory.

Examples:
  meow registry validate                     # Validate current dir as registry
  meow registry validate ./my-registry       # Validate specific registry
  meow collection validate ./collections/sprint  # Validate a collection`,
	Args: cobra.MaximumNArgs(1),
	RunE: runRegistryValidate,
}

func init() {
	registryCmd.AddCommand(registryValidateCmd)
}

func runRegistryValidate(cmd *cobra.Command, args []string) error {
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}

	// Check what we're validating
	if registry.HasRegistry(dir) {
		return validateRegistry(cmd, dir)
	}

	if registry.HasManifest(dir) {
		return validateCollection(cmd, dir)
	}

	return fmt.Errorf("no registry.json or manifest.json found in %s/.meow/", dir)
}

func validateRegistry(cmd *cobra.Command, dir string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Validating registry at %s...\n\n", dir)

	reg, err := registry.LoadRegistry(dir)
	if err != nil {
		return fmt.Errorf("loading registry: %w", err)
	}

	result := registry.ValidateRegistry(reg)

	// Validate each collection
	for _, c := range reg.Collections {
		sourcePath, err := registry.ResolveCollectionSource(c, dir, reg.CollectionRoot)
		if err != nil {
			result.AddError(fmt.Sprintf("collection %s", c.Name), fmt.Sprintf("invalid source: %v", err))
			continue
		}

		if !registry.HasManifest(sourcePath) && (c.Strict == nil || *c.Strict) {
			result.AddError(fmt.Sprintf("collection %s", c.Name), "missing .meow/manifest.json (use strict: false to skip)")
			continue
		}

		if registry.HasManifest(sourcePath) {
			manifest, err := registry.LoadManifest(sourcePath)
			if err != nil {
				result.AddError(fmt.Sprintf("collection %s", c.Name), err.Error())
				continue
			}

			collResult := registry.ValidateCollection(sourcePath, manifest)
			for _, e := range collResult.Errors {
				result.AddError(fmt.Sprintf("collection %s", c.Name), e.Error())
			}
			for _, w := range collResult.Warnings {
				result.AddWarning(fmt.Sprintf("collection %s: %s", c.Name, w))
			}
		}
	}

	// Print results
	if len(result.Warnings) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Warnings:")
		for _, w := range result.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ %s\n", w)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	if result.HasErrors() {
		fmt.Fprintln(cmd.OutOrStdout(), "Errors:")
		for _, e := range result.Errors {
			fmt.Fprintf(cmd.OutOrStdout(), "  ✗ %s\n", e)
		}
		return fmt.Errorf("validation failed with %d errors", len(result.Errors))
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ Registry is valid (%d collections)\n", len(reg.Collections))
	return nil
}

func validateCollection(cmd *cobra.Command, dir string) error {
	fmt.Fprintf(cmd.OutOrStdout(), "Validating collection at %s...\n\n", dir)

	manifest, err := registry.LoadManifest(dir)
	if err != nil {
		return fmt.Errorf("loading manifest: %w", err)
	}

	result := registry.ValidateCollection(dir, manifest)

	// Print results
	if len(result.Warnings) > 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "Warnings:")
		for _, w := range result.Warnings {
			fmt.Fprintf(cmd.OutOrStdout(), "  ⚠ %s\n", w)
		}
		fmt.Fprintln(cmd.OutOrStdout())
	}

	if result.HasErrors() {
		fmt.Fprintln(cmd.OutOrStdout(), "Errors:")
		for _, e := range result.Errors {
			fmt.Fprintf(cmd.OutOrStdout(), "  ✗ %s\n", e)
		}
		return fmt.Errorf("validation failed")
	}

	fmt.Fprintf(cmd.OutOrStdout(), "✓ Collection %q is valid\n", manifest.Name)
	return nil
}
