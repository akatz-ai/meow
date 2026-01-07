package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var traceCmd = &cobra.Command{
	Use:   "trace",
	Short: "Show execution trace",
	Long: `Display the execution trace of the current workflow.

Shows the sequence of bead executions with timestamps, durations,
and outcomes. Useful for debugging workflow issues.`,
	RunE: runTrace,
}

var (
	traceLimit  int
	traceFormat string
)

func init() {
	traceCmd.Flags().IntVar(&traceLimit, "limit", 50, "maximum entries to show")
	traceCmd.Flags().StringVar(&traceFormat, "format", "text", "output format: text, json")
	rootCmd.AddCommand(traceCmd)
}

func runTrace(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	// TODO: Load trace from .meow/state/trace.jsonl
	fmt.Println("Execution Trace")
	fmt.Println("---------------")
	fmt.Println("(not yet implemented)")

	return nil
}
