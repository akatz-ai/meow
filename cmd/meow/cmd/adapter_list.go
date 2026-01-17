package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/akatz-ai/meow/internal/adapter"
	"github.com/spf13/cobra"
)

var adapterListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available adapters",
	Long: `List all available agent adapters from project and global sources.

The output shows:
  - NAME:        Adapter identifier
  - DESCRIPTION: Human-readable description
  - LOCATION:    Where the adapter is defined

Location values:
  - ~/.meow/...: Global adapter from home directory
  - .meow/...:   Project-local adapter (relative path shown)

When an adapter exists in multiple locations, only the highest-priority
version is shown. Use 'meow adapter show <name>' to see override details.`,
	RunE: runAdapterList,
}

var adapterListJSON bool

func init() {
	adapterListCmd.Flags().BoolVar(&adapterListJSON, "json", false, "output as JSON")
	adapterCmd.AddCommand(adapterListCmd)
}

func runAdapterList(cmd *cobra.Command, args []string) error {
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	registry, err := adapter.NewDefaultRegistry(dir)
	if err != nil {
		return fmt.Errorf("creating adapter registry: %w", err)
	}

	adapters, err := registry.ListWithInfo()
	if err != nil {
		return fmt.Errorf("listing adapters: %w", err)
	}

	// Sort by name for consistent output
	sort.Slice(adapters, func(i, j int) bool {
		return adapters[i].Name < adapters[j].Name
	})

	if adapterListJSON {
		return outputAdapterListJSON(adapters)
	}

	return outputAdapterListText(adapters)
}

// adapterListEntry is a simplified struct for JSON output
type adapterListEntry struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Source      adapter.AdapterSource `json:"source"`
	Path        string                `json:"path,omitempty"`
}

func outputAdapterListJSON(adapters []adapter.AdapterInfo) error {
	entries := make([]adapterListEntry, len(adapters))
	for i, a := range adapters {
		entries[i] = adapterListEntry{
			Name:        a.Name,
			Description: a.Description,
			Source:      a.Source,
			Path:        a.Path,
		}
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func outputAdapterListText(adapters []adapter.AdapterInfo) error {
	if len(adapters) == 0 {
		fmt.Println("No adapters found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tDESCRIPTION\tLOCATION")

	for _, a := range adapters {
		location := formatLocation(a)
		description := a.Description
		if description == "" {
			description = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", a.Name, description, location)
	}

	return w.Flush()
}

// formatLocation returns a human-readable location string for an adapter.
func formatLocation(a adapter.AdapterInfo) string {
	switch a.Source {
	case adapter.SourceProject:
		// Show relative path for project adapters
		return ".meow/adapters/" + a.Name
	case adapter.SourceGlobal:
		// Show home-relative path for global adapters
		home, _ := os.UserHomeDir()
		if home != "" && len(a.Path) > len(home) && a.Path[:len(home)] == home {
			return "~" + a.Path[len(home):]
		}
		return a.Path
	default:
		return a.Path
	}
}
