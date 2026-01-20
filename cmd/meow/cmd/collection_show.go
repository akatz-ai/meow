package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/akatz-ai/meow/internal/registry"
	"github.com/spf13/cobra"
)

var collectionShowCmd = &cobra.Command{
	Use:   "show <collection>",
	Short: "Show collection details",
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

		// Load manifest
		manifest, err := registry.LoadManifest(c.Path)
		if err != nil {
			return fmt.Errorf("loading manifest: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Collection: %s\n", manifest.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "Description: %s\n", manifest.Description)
		fmt.Fprintf(cmd.OutOrStdout(), "Entrypoint: %s\n", manifest.Entrypoint)
		fmt.Fprintf(cmd.OutOrStdout(), "Path: %s\n", c.Path)
		fmt.Fprintf(cmd.OutOrStdout(), "Scope: %s\n", c.Scope)
		if c.Registry != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Registry: %s (v%s)\n", c.Registry, c.RegistryVersion)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Installed: %s\n", c.InstalledAt.Format(time.RFC3339))

		// List workflows in collection
		fmt.Fprintf(cmd.OutOrStdout(), "\nWorkflows:\n")
		filepath.Walk(c.Path, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if strings.HasSuffix(path, ".meow.toml") {
				rel, _ := filepath.Rel(c.Path, path)
				fmt.Fprintf(cmd.OutOrStdout(), "  %s:%s\n", name, strings.TrimSuffix(rel, ".meow.toml"))
			}
			return nil
		})

		return nil
	},
}

func init() {
	collectionCmd.AddCommand(collectionShowCmd)
}
