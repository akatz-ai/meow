package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/meow-stack/meow-machine/internal/agent"
	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/spf13/cobra"
)

var handoffCmd = &cobra.Command{
	Use:   "handoff",
	Short: "Request a context refresh",
	Long: `Request the orchestrator to cycle the agent's session.

This is used when the agent is approaching context limits. The orchestrator
will save the session ID, stop the agent, and restart with --resume.

The command:
1. Updates the current bead with handoff notes (if --notes provided)
2. Creates a handoff signal file for the orchestrator
3. The orchestrator will stop the agent and restart with --resume`,
	RunE: runHandoff,
}

var (
	handoffNotes string
	handoffAgent string
)

func init() {
	handoffCmd.Flags().StringVar(&handoffNotes, "notes", "", "handoff notes for the next session")
	handoffCmd.Flags().StringVar(&handoffAgent, "agent", "", "agent ID (default: from MEOW_AGENT env)")
	rootCmd.AddCommand(handoffCmd)
}

// HandoffSignal represents a handoff request from an agent.
type HandoffSignal struct {
	AgentID     string    `json:"agent_id"`
	Timestamp   time.Time `json:"timestamp"`
	Notes       string    `json:"notes,omitempty"`
	CurrentBead string    `json:"current_bead,omitempty"`
	Reason      string    `json:"reason"`
}

func runHandoff(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	ctx := context.Background()

	// Get working directory
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// Get agent ID
	agentID := handoffAgent
	if agentID == "" {
		agentID = os.Getenv("MEOW_AGENT")
	}
	if agentID == "" {
		return fmt.Errorf("agent ID required: use --agent flag or set MEOW_AGENT environment variable")
	}

	// Load agent store to find current bead
	agentStore := agent.NewStore(filepath.Join(dir, ".meow"))
	if err := agentStore.Load(ctx); err != nil {
		// Agent store may not exist in simple setups
		if verbose {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not load agent store: %v\n", err)
		}
	}

	// Get current bead from agent record
	var currentBeadID string
	agentInfo, err := agentStore.Get(ctx, agentID)
	if err == nil && agentInfo != nil {
		currentBeadID = agentInfo.CurrentBead
	}

	// If we have notes and a current bead, update the bead
	if handoffNotes != "" && currentBeadID != "" {
		beadsDir := filepath.Join(dir, ".beads")
		beadStore := orchestrator.NewFileBeadStore(beadsDir)
		if err := beadStore.Load(ctx); err != nil {
			return fmt.Errorf("loading beads: %w", err)
		}

		bead, err := beadStore.Get(ctx, currentBeadID)
		if err != nil {
			return fmt.Errorf("getting current bead: %w", err)
		}
		if bead != nil {
			// Append handoff notes to bead notes
			handoffHeader := fmt.Sprintf("\n\n=== HANDOFF NOTES (%s) ===\n", time.Now().Format(time.RFC3339))
			if bead.Notes != "" {
				bead.Notes = bead.Notes + handoffHeader + handoffNotes
			} else {
				bead.Notes = handoffHeader + handoffNotes
			}

			if err := beadStore.Update(ctx, bead); err != nil {
				return fmt.Errorf("updating bead with handoff notes: %w", err)
			}

			if verbose {
				fmt.Printf("Updated bead %s with handoff notes\n", currentBeadID)
			}
		}
	}

	// Create handoff signal for orchestrator
	signal := HandoffSignal{
		AgentID:     agentID,
		Timestamp:   time.Now(),
		Notes:       handoffNotes,
		CurrentBead: currentBeadID,
		Reason:      "context_refresh",
	}

	// Write handoff signal file
	signalsDir := filepath.Join(dir, ".meow", "signals")
	if err := os.MkdirAll(signalsDir, 0755); err != nil {
		return fmt.Errorf("creating signals directory: %w", err)
	}

	signalPath := filepath.Join(signalsDir, fmt.Sprintf("handoff-%s-%d.json", agentID, time.Now().UnixNano()))
	data, err := json.MarshalIndent(signal, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling handoff signal: %w", err)
	}

	if err := os.WriteFile(signalPath, data, 0644); err != nil {
		return fmt.Errorf("writing handoff signal: %w", err)
	}

	// Update agent status to indicate handoff requested
	if agentInfo != nil {
		if err := agentStore.Update(ctx, agentID, func(a *types.Agent) error {
			if a.Labels == nil {
				a.Labels = make(map[string]string)
			}
			a.Labels["handoff_requested"] = time.Now().Format(time.RFC3339)
			return nil
		}); err != nil {
			if verbose {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not update agent labels: %v\n", err)
			}
		}
	}

	// Output confirmation
	fmt.Printf("Handoff requested for agent: %s\n", agentID)
	if currentBeadID != "" {
		fmt.Printf("Current bead: %s\n", currentBeadID)
	}
	if handoffNotes != "" {
		fmt.Println("Notes saved for next session.")
	}
	fmt.Println("")
	fmt.Println("The orchestrator will cycle this agent's session.")
	fmt.Println("Your work will resume with the saved context.")

	return nil
}
