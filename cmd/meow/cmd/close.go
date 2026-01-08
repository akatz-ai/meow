package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/meow-stack/meow-machine/internal/validation"
	"github.com/spf13/cobra"
)

var closeCmd = &cobra.Command{
	Use:   "close <bead-id>",
	Short: "Close a task bead",
	Long: `Mark a task bead as closed, optionally with outputs.

This command is called by agents when they complete a task. If the task
has required outputs, they must be provided.

Examples:
  # Close a simple task
  meow close bd-task-001

  # Close with notes
  meow close bd-task-001 --notes "Completed implementation"

  # Close with outputs
  meow close bd-select-001 --output work_bead=bd-task-042 --output rationale="High priority"

  # Close with JSON output
  meow close bd-task-001 --output-json '{"key": "value"}'`,
	Args: cobra.ExactArgs(1),
	RunE: runClose,
}

var (
	closeNotes      string
	closeOutputs    []string
	closeOutputJSON string
)

func init() {
	closeCmd.Flags().StringVar(&closeNotes, "notes", "", "completion notes")
	closeCmd.Flags().StringArrayVar(&closeOutputs, "output", nil, "output values (format: name=value)")
	closeCmd.Flags().StringVar(&closeOutputJSON, "output-json", "", "outputs as JSON object")
	rootCmd.AddCommand(closeCmd)
}

// beadChecker implements validation.BeadChecker for validating bead_id outputs.
type beadChecker struct {
	store *orchestrator.FileBeadStore
}

func (c *beadChecker) BeadExists(id string) bool {
	ctx := context.Background()
	bead, err := c.store.Get(ctx, id)
	return err == nil && bead != nil
}

func (c *beadChecker) ListAllIDs() []string {
	ctx := context.Background()
	// List all beads (no status filter = empty string)
	beads, err := c.store.List(ctx, "")
	if err != nil {
		return nil
	}
	ids := make([]string, len(beads))
	for i, b := range beads {
		ids[i] = b.ID
	}
	return ids
}

func runClose(cmd *cobra.Command, args []string) error {
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

	// Check bead can be closed
	if bead.Status == types.BeadStatusClosed {
		return fmt.Errorf("bead %s is already closed", beadID)
	}

	// Parse outputs from flags
	providedOutputs := make(map[string]string)

	// First, parse --output flags
	for _, o := range closeOutputs {
		parts := strings.SplitN(o, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid output format: %s (expected name=value)", o)
		}
		providedOutputs[parts[0]] = parts[1]
	}

	// Then, parse --output-json if provided
	if closeOutputJSON != "" {
		var jsonOutputs map[string]any
		if err := json.Unmarshal([]byte(closeOutputJSON), &jsonOutputs); err != nil {
			return fmt.Errorf("invalid --output-json: %w", err)
		}
		for k, v := range jsonOutputs {
			// Convert to string for validation
			switch val := v.(type) {
			case string:
				providedOutputs[k] = val
			default:
				// Re-marshal non-strings as JSON
				data, _ := json.Marshal(v)
				providedOutputs[k] = string(data)
			}
		}
	}

	// Validate outputs if task has output spec
	checker := &beadChecker{store: store}
	validatedOutputs, err := validation.ValidateOutputs(beadID, bead.TaskOutputs, providedOutputs, checker)
	if err != nil {
		// Show helpful usage message on validation error
		if valErr, ok := err.(*validation.OutputValidationError); ok {
			fmt.Fprintln(cmd.ErrOrStderr(), valErr.Error())
			fmt.Fprintln(cmd.ErrOrStderr())
			fmt.Fprintln(cmd.ErrOrStderr(), validation.FormatUsage(beadID, bead.TaskOutputs))
			return fmt.Errorf("output validation failed")
		}
		return err
	}

	// Close the bead with validated outputs
	if err := bead.Close(validatedOutputs.Values); err != nil {
		return fmt.Errorf("closing bead: %w", err)
	}

	// Update notes if provided
	if closeNotes != "" {
		if bead.Notes != "" {
			bead.Notes = bead.Notes + "\n\n" + closeNotes
		} else {
			bead.Notes = closeNotes
		}
	}

	// Save the bead
	if err := store.Update(ctx, bead); err != nil {
		return fmt.Errorf("saving bead: %w", err)
	}

	// Output success
	if verbose {
		fmt.Printf("Closed bead: %s\n", beadID)
		if len(validatedOutputs.Values) > 0 {
			fmt.Printf("Outputs: %v\n", validatedOutputs.Values)
		}
	} else {
		fmt.Printf("Closed: %s\n", beadID)
	}

	return nil
}
