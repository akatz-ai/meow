package cmd

import (
	"strings"
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

// TestRootCmdHelpUsesCorrectVocabulary verifies the help text uses correct terminology.
// There are 7 executors (shell, spawn, kill, expand, branch, foreach, agent),
// not "6 bead types".
func TestRootCmdHelpUsesCorrectVocabulary(t *testing.T) {
	longHelp := rootCmd.Long

	// Should mention "7" executors, not "6"
	if strings.Contains(longHelp, "6 primitive") {
		t.Error("help text should not say '6 primitive' - MEOW has 7 executors")
	}

	// Should use "executors" terminology, not "bead types"
	if strings.Contains(longHelp, "bead types") {
		t.Error("help text should not mention 'bead types' - use 'executors' instead")
	}

	// Should mention "7 primitive executors"
	if !strings.Contains(longHelp, "7 primitive executors") {
		t.Error("help text should mention '7 primitive executors'")
	}

	// Should list all 7 executors
	executors := []string{"shell", "spawn", "kill", "expand", "branch", "foreach", "agent"}
	for _, exec := range executors {
		if !strings.Contains(longHelp, exec) {
			t.Errorf("help text should mention executor %q", exec)
		}
	}
}
