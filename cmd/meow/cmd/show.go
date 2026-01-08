package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <bead-id>",
	Short: "Show bead details",
	Long: `Display detailed information about a bead, including outputs.

This command shows all MEOW-specific fields that bd show doesn't display,
such as outputs, type-specific specs, tier, and workflow information.

Examples:
  # Show a bead
  meow show bd-task-001

  # Show as JSON
  meow show bd-task-001 --json`,
	Args: cobra.ExactArgs(1),
	RunE: runShow,
}

var showJSON bool

func init() {
	showCmd.Flags().BoolVar(&showJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) error {
	if err := checkBeadsDir(); err != nil {
		return err
	}

	beadID := args[0]
	ctx := context.Background()

	// Get working directory
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// Load bead store
	beadsDir := filepath.Join(dir, ".beads")
	store := orchestrator.NewFileBeadStore(beadsDir)
	if err := store.Load(ctx); err != nil {
		return fmt.Errorf("loading beads: %w", err)
	}

	// Get the bead
	bead, err := store.Get(ctx, beadID)
	if err != nil {
		return fmt.Errorf("getting bead: %w", err)
	}
	if bead == nil {
		return fmt.Errorf("bead not found: %s", beadID)
	}

	// Output as JSON if requested
	if showJSON {
		data, err := json.MarshalIndent(bead, "", "  ")
		if err != nil {
			return fmt.Errorf("marshaling bead: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	// Format for terminal
	printBead(bead)
	return nil
}

// printBead displays a bead in a human-readable format.
func printBead(bead *types.Bead) {
	// Header
	fmt.Printf("%s: %s\n", bead.ID, bead.Title)
	fmt.Printf("Status: %s\n", bead.Status)
	fmt.Printf("Type: %s\n", bead.Type)

	// Tier (MEOW-specific)
	if bead.Tier != "" {
		fmt.Printf("Tier: %s\n", bead.Tier)
	}

	// Assignment
	if bead.Assignee != "" {
		fmt.Printf("Assignee: %s\n", bead.Assignee)
	}

	// Timestamps
	fmt.Printf("Created: %s\n", bead.CreatedAt.Format("2006-01-02 15:04"))
	if bead.ClosedAt != nil {
		fmt.Printf("Closed: %s\n", bead.ClosedAt.Format("2006-01-02 15:04"))
	}

	// Workflow tracking (MEOW-specific)
	if bead.WorkflowID != "" {
		fmt.Printf("Workflow: %s\n", bead.WorkflowID)
	}
	if bead.SourceWorkflow != "" {
		fmt.Printf("Source Workflow: %s\n", bead.SourceWorkflow)
	}
	if bead.HookBead != "" {
		fmt.Printf("Hook Bead: %s\n", bead.HookBead)
	}
	if bead.Parent != "" {
		fmt.Printf("Parent: %s\n", bead.Parent)
	}

	// Description
	if bead.Description != "" {
		fmt.Printf("\nDescription:\n%s\n", bead.Description)
	}

	// Instructions (for tasks)
	if bead.Instructions != "" {
		fmt.Printf("\nInstructions:\n%s\n", bead.Instructions)
	}

	// Notes
	if bead.Notes != "" {
		fmt.Printf("\nNotes:\n%s\n", bead.Notes)
	}

	// Dependencies
	if len(bead.Needs) > 0 {
		fmt.Printf("\nDepends on (%d):\n", len(bead.Needs))
		for _, dep := range bead.Needs {
			fmt.Printf("  -> %s\n", dep)
		}
	}

	// Labels
	if len(bead.Labels) > 0 {
		fmt.Printf("\nLabels: %v\n", bead.Labels)
	}

	// Outputs (the key MEOW-specific feature!)
	if len(bead.Outputs) > 0 {
		fmt.Printf("\nOutputs:\n")
		for key, value := range bead.Outputs {
			// Format the value nicely
			switch v := value.(type) {
			case string:
				// Truncate long strings
				if len(v) > 100 {
					fmt.Printf("  %s: %s...\n", key, v[:100])
				} else {
					fmt.Printf("  %s: %s\n", key, v)
				}
			case map[string]any:
				// Pretty print JSON objects
				data, _ := json.MarshalIndent(v, "  ", "  ")
				fmt.Printf("  %s:\n  %s\n", key, string(data))
			default:
				fmt.Printf("  %s: %v\n", key, value)
			}
		}
	}

	// Task output spec (expected outputs)
	if bead.TaskOutputs != nil {
		if len(bead.TaskOutputs.Required) > 0 || len(bead.TaskOutputs.Optional) > 0 {
			fmt.Printf("\nExpected Outputs:\n")
			for _, out := range bead.TaskOutputs.Required {
				desc := ""
				if out.Description != "" {
					desc = " - " + out.Description
				}
				fmt.Printf("  %s (%s, required)%s\n", out.Name, out.Type, desc)
			}
			for _, out := range bead.TaskOutputs.Optional {
				desc := ""
				if out.Description != "" {
					desc = " - " + out.Description
				}
				fmt.Printf("  %s (%s, optional)%s\n", out.Name, out.Type, desc)
			}
		}
	}

	// Type-specific specs
	printTypeSpec(bead)

	fmt.Println()
}

// printTypeSpec displays type-specific configuration.
func printTypeSpec(bead *types.Bead) {
	switch bead.Type {
	case types.BeadTypeCondition:
		if bead.ConditionSpec != nil {
			fmt.Printf("\nCondition Spec:\n")
			fmt.Printf("  Condition: %s\n", truncateCode(bead.ConditionSpec.Condition))
			if bead.ConditionSpec.Timeout != "" {
				fmt.Printf("  Timeout: %s\n", bead.ConditionSpec.Timeout)
			}
			if bead.ConditionSpec.OnTrue != nil {
				fmt.Printf("  On True: %s\n", formatExpansionTarget(bead.ConditionSpec.OnTrue))
			}
			if bead.ConditionSpec.OnFalse != nil {
				fmt.Printf("  On False: %s\n", formatExpansionTarget(bead.ConditionSpec.OnFalse))
			}
		}

	case types.BeadTypeCode:
		if bead.CodeSpec != nil {
			fmt.Printf("\nCode Spec:\n")
			fmt.Printf("  Code: %s\n", truncateCode(bead.CodeSpec.Code))
			if bead.CodeSpec.Workdir != "" {
				fmt.Printf("  Workdir: %s\n", bead.CodeSpec.Workdir)
			}
			if bead.CodeSpec.OnError != "" {
				fmt.Printf("  On Error: %s\n", bead.CodeSpec.OnError)
			}
			if len(bead.CodeSpec.Outputs) > 0 {
				fmt.Printf("  Captures: ")
				names := make([]string, len(bead.CodeSpec.Outputs))
				for i, o := range bead.CodeSpec.Outputs {
					names[i] = o.Name
				}
				fmt.Printf("%s\n", strings.Join(names, ", "))
			}
		}

	case types.BeadTypeStart:
		if bead.StartSpec != nil {
			fmt.Printf("\nStart Spec:\n")
			fmt.Printf("  Agent: %s\n", bead.StartSpec.Agent)
			if bead.StartSpec.Workdir != "" {
				fmt.Printf("  Workdir: %s\n", bead.StartSpec.Workdir)
			}
			if bead.StartSpec.Prompt != "" {
				fmt.Printf("  Prompt: %s\n", truncateCode(bead.StartSpec.Prompt))
			}
		}

	case types.BeadTypeStop:
		if bead.StopSpec != nil {
			fmt.Printf("\nStop Spec:\n")
			fmt.Printf("  Agent: %s\n", bead.StopSpec.Agent)
			if bead.StopSpec.Graceful {
				fmt.Printf("  Graceful: true\n")
			}
			if bead.StopSpec.Timeout > 0 {
				fmt.Printf("  Timeout: %ds\n", bead.StopSpec.Timeout)
			}
		}

	case types.BeadTypeExpand:
		if bead.ExpandSpec != nil {
			fmt.Printf("\nExpand Spec:\n")
			fmt.Printf("  Template: %s\n", bead.ExpandSpec.Template)
			if bead.ExpandSpec.Assignee != "" {
				fmt.Printf("  Assignee: %s\n", bead.ExpandSpec.Assignee)
			}
			if bead.ExpandSpec.Ephemeral {
				fmt.Printf("  Ephemeral: true\n")
			}
		}
	}
}

// truncateCode truncates long code strings for display.
func truncateCode(code string) string {
	// Replace newlines with spaces for single-line display
	code = strings.ReplaceAll(code, "\n", " ")
	code = strings.Join(strings.Fields(code), " ") // Normalize whitespace
	if len(code) > 60 {
		return code[:60] + "..."
	}
	return code
}

// formatExpansionTarget formats an expansion target for display.
func formatExpansionTarget(t *types.ExpansionTarget) string {
	if t.Template != "" {
		return "template:" + t.Template
	}
	if len(t.Inline) > 0 {
		return fmt.Sprintf("inline (%d steps)", len(t.Inline))
	}
	return "(empty)"
}
