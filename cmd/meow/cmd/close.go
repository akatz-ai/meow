package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var closeCmd = &cobra.Command{
	Use:   "close <bead-id>",
	Short: "Close a task bead",
	Long: `Mark a task bead as closed, optionally with outputs.

This command is called by agents when they complete a task. If the task
has required outputs, they must be provided.`,
	Args: cobra.ExactArgs(1),
	RunE: runClose,
}

var (
	closeNotes   string
	closeOutputs []string
)

func init() {
	closeCmd.Flags().StringVar(&closeNotes, "notes", "", "completion notes")
	closeCmd.Flags().StringArrayVar(&closeOutputs, "output", nil, "output values (format: name=value)")
	rootCmd.AddCommand(closeCmd)
}

func runClose(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	beadID := args[0]

	// Parse outputs
	outputs := make(map[string]string)
	for _, o := range closeOutputs {
		parts := strings.SplitN(o, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid output format: %s (expected name=value)", o)
		}
		outputs[parts[0]] = parts[1]
	}

	// TODO: Load bead, validate outputs, close it
	fmt.Printf("Closing bead: %s\n", beadID)
	if closeNotes != "" {
		fmt.Printf("Notes: %s\n", closeNotes)
	}
	if len(outputs) > 0 {
		fmt.Printf("Outputs: %v\n", outputs)
	}
	fmt.Println("(not yet implemented)")

	return nil
}
