package cmd

import (
	"testing"
)

func TestRootCmdHasRun(t *testing.T) {
	// Verify root command has a Run function (for no-args workflow listing)
	if rootCmd.Run == nil {
		t.Error("rootCmd.Run should be set to list workflows when no args provided")
	}
}

func TestRootCmdFlags(t *testing.T) {
	// Test that global flags are registered
	verboseFlag := rootCmd.PersistentFlags().Lookup("verbose")
	if verboseFlag == nil {
		t.Error("--verbose flag not found")
	}

	workdirFlag := rootCmd.PersistentFlags().Lookup("workdir")
	if workdirFlag == nil {
		t.Error("--workdir flag not found")
	}
}
