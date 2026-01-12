package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/meow-stack/meow-machine/internal/adapter"
	"github.com/spf13/cobra"
)

var adapterShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show adapter configuration",
	Long: `Display the full configuration for an adapter.

Shows all adapter settings including:
  - Metadata (name, description, location)
  - Spawn configuration (command, resume command, startup delay)
  - Environment variables
  - Prompt injection settings
  - Graceful stop configuration

If the adapter overrides another (e.g., a project adapter with the same name
as a global adapter), the override information is also displayed.`,
	Args: cobra.ExactArgs(1),
	RunE: runAdapterShow,
}

var adapterShowJSON bool

func init() {
	adapterShowCmd.Flags().BoolVar(&adapterShowJSON, "json", false, "output as JSON")
	adapterCmd.AddCommand(adapterShowCmd)
}

func runAdapterShow(cmd *cobra.Command, args []string) error {
	name := args[0]

	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	registry, err := adapter.NewDefaultRegistry(dir)
	if err != nil {
		return fmt.Errorf("creating adapter registry: %w", err)
	}

	info, err := registry.GetInfo(name)
	if err != nil {
		if adapter.IsNotFound(err) {
			return fmt.Errorf("adapter %q not found\n\nUse 'meow adapter list' to see available adapters", name)
		}
		return fmt.Errorf("loading adapter: %w", err)
	}

	if adapterShowJSON {
		return outputAdapterShowJSON(info)
	}

	return outputAdapterShowText(info)
}

func outputAdapterShowJSON(info *adapter.AdapterInfo) error {
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func outputAdapterShowText(info *adapter.AdapterInfo) error {
	cfg := info.Config

	// Header
	fmt.Printf("Adapter: %s\n", info.Name)
	if info.Description != "" {
		fmt.Printf("Description: %s\n", info.Description)
	}
	fmt.Printf("Location: %s\n", formatLocation(*info))

	// Override info
	if info.Overrides != nil {
		overrideLocation := formatOverrideLocation(info.Overrides)
		fmt.Printf("Overrides: %s\n", overrideLocation)
	}

	fmt.Println()

	// Spawn configuration
	fmt.Println("Spawn:")
	fmt.Printf("  Command: %s\n", cfg.Spawn.Command)
	if cfg.Spawn.ResumeCommand != "" {
		fmt.Printf("  Resume: %s\n", cfg.Spawn.ResumeCommand)
	}
	fmt.Printf("  Startup Delay: %s\n", cfg.GetStartupDelay())

	// Environment
	if len(cfg.Environment) > 0 {
		fmt.Println()
		fmt.Println("Environment:")
		for k, v := range cfg.Environment {
			if v == "" {
				fmt.Printf("  %s: (unset)\n", k)
			} else {
				fmt.Printf("  %s: %q\n", k, v)
			}
		}
	}

	// Prompt injection
	fmt.Println()
	fmt.Println("Prompt Injection:")
	if len(cfg.PromptInjection.PreKeys) > 0 {
		fmt.Printf("  Pre-keys: %s\n", formatKeys(cfg.PromptInjection.PreKeys))
	}
	if cfg.PromptInjection.PreDelay.Duration() > 0 {
		fmt.Printf("  Pre-delay: %s\n", cfg.PromptInjection.PreDelay)
	}
	fmt.Printf("  Method: %s\n", cfg.GetPromptInjectionMethod())
	if len(cfg.PromptInjection.PostKeys) > 0 {
		fmt.Printf("  Post-keys: %s\n", formatKeys(cfg.PromptInjection.PostKeys))
	}
	if cfg.PromptInjection.PostDelay.Duration() > 0 {
		fmt.Printf("  Post-delay: %s\n", cfg.PromptInjection.PostDelay)
	}

	// Graceful stop
	if len(cfg.GracefulStop.Keys) > 0 || cfg.GracefulStop.Wait.Duration() > 0 {
		fmt.Println()
		fmt.Println("Graceful Stop:")
		if len(cfg.GracefulStop.Keys) > 0 {
			fmt.Printf("  Keys: %s\n", formatKeys(cfg.GracefulStop.Keys))
		}
		fmt.Printf("  Wait: %s\n", cfg.GetGracefulStopWait())
	}

	return nil
}

// formatKeys formats a slice of key names for display.
func formatKeys(keys []string) string {
	return strings.Join(keys, ", ")
}

// formatOverrideLocation formats the override source for display.
func formatOverrideLocation(override *adapter.AdapterOverrideInfo) string {
	switch override.Source {
	case adapter.SourceGlobal:
		home, _ := os.UserHomeDir()
		if home != "" && len(override.Path) > len(home) && override.Path[:len(home)] == home {
			return "~" + override.Path[len(home):]
		}
		return override.Path
	case adapter.SourceProject:
		return ".meow/adapters/" + filepath.Base(override.Path)
	default:
		return override.Path
	}
}
