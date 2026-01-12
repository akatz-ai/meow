package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/meow-stack/meow-machine/internal/agent"
	"github.com/spf13/cobra"
)

var agentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List agents",
	Long: `Display all agents and their current status.

Shows:
- Agent ID and name
- Status (active/stopped)
- Current bead assignment
- Last heartbeat`,
	RunE: runAgents,
}

var agentsFormat string

func init() {
	agentsCmd.Flags().StringVar(&agentsFormat, "format", "text", "output format: text, json")
	rootCmd.AddCommand(agentsCmd)
}

func runAgents(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// Load agents from store
	store := agent.NewStore(dir + "/.meow")
	ctx := context.Background()

	if err := store.Load(ctx); err != nil {
		return fmt.Errorf("loading agents: %w", err)
	}

	agents, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("listing agents: %w", err)
	}

	// Sort agents by ID for consistent output
	sort.Slice(agents, func(i, j int) bool {
		return agents[i].ID < agents[j].ID
	})

	// Handle JSON output
	if agentsFormat == "json" {
		data, err := json.MarshalIndent(agents, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling agents: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Text output
	if len(agents) == 0 {
		fmt.Println("No agents registered.")
		fmt.Println("")
		fmt.Println("Use 'meow run' to start the orchestrator which will spawn agents.")
		return nil
	}

	fmt.Println("Agents")
	fmt.Println("------")
	fmt.Println()

	for _, a := range agents {
		// Status indicator
		statusIcon := "x"
		if a.Status == "active" {
			statusIcon = "*"
		}

		fmt.Printf("[%s] %s", statusIcon, a.ID)
		if a.Name != "" && a.Name != a.ID {
			fmt.Printf(" (%s)", a.Name)
		}
		fmt.Println()

		fmt.Printf("    Status: %s\n", a.Status)

		if a.TmuxSession != "" {
			fmt.Printf("    Tmux:   %s\n", a.TmuxSession)
		}

		if a.Worktree != "" {
			fmt.Printf("    Tree:   %s\n", a.Worktree)
		}

		if a.LastHeartbeat != nil {
			ago := time.Since(*a.LastHeartbeat).Round(time.Second)
			fmt.Printf("    Last:   %s ago\n", ago)
		}

		fmt.Println()
	}

	// Summary
	activeCount := 0
	for _, a := range agents {
		if a.Status == "active" {
			activeCount++
		}
	}
	fmt.Printf("Total: %d agents (%d active, %d stopped)\n", len(agents), activeCount, len(agents)-activeCount)

	return nil
}
