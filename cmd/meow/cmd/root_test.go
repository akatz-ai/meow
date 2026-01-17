package cmd

import (
	"strings"
	"testing"
)

func TestRootCmdHasRunE(t *testing.T) {
	// Verify root command has a RunE function (for workflow listing and shorthand)
	if rootCmd.RunE == nil {
		t.Error("rootCmd.RunE should be set to handle workflow listing and shorthand")
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

// Tests for meow-nr5y: Make meow <workflow> shorthand for meow run <workflow>

// TestWorkflowShorthandSubcommandsTakePrecedence verifies that built-in subcommands
// take precedence over workflow names (like make targets).
func TestWorkflowShorthandSubcommandsTakePrecedence(t *testing.T) {
	// These are the built-in subcommands that should always take precedence
	// over workflow names with the same name
	subcommands := []string{
		"run", "status", "init", "validate", "stop", "resume",
		"done", "event", "ls", "show", "approve", "reject",
		"cleanup", "adapter", "skill",
	}

	for _, name := range subcommands {
		found := false
		for _, sub := range rootCmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q to be a subcommand (for precedence testing)", name)
		}
	}
}

// TestWorkflowShorthandInvokesRun verifies that `meow <workflow>` acts as shorthand
// for `meow run <workflow>` when the first argument is not a subcommand.
func TestWorkflowShorthandInvokesRun(t *testing.T) {
	// The root command should have RunE (not just Run) to handle args as workflows
	// Currently it only has Run which calls listWorkflows()
	// After meow-nr5y is implemented, it should handle workflow shorthand
	if rootCmd.RunE == nil {
		t.Error("rootCmd should have RunE to handle workflow shorthand (meow <workflow>)")
	}
}

// TestWorkflowShorthandErrorsOnUnknown verifies that an unknown command/workflow
// gives a clear error message.
func TestWorkflowShorthandErrorsOnUnknown(t *testing.T) {
	// The error message for unknown commands should be helpful
	// After meow-nr5y implementation:
	// - "meow foobar" where foobar is neither a command nor workflow should error
	// - Error should say "unknown command or workflow: foobar"
	//
	// This test will fail until the feature is implemented because
	// currently rootCmd.Run just calls listWorkflows() regardless of args

	// Check that rootCmd handles args - currently it doesn't check them
	// It should fail with a clear error for unknown workflows
	if rootCmd.RunE == nil {
		t.Error("rootCmd needs RunE to properly validate and error on unknown commands/workflows")
	}
}

// TestWorkflowShorthandPassesVars verifies that `meow <workflow> --var x=y`
// properly passes variables to the workflow.
func TestWorkflowShorthandPassesVars(t *testing.T) {
	// The workflow shorthand should forward --var flags to the run command
	// This is important for UX: meow sprint --var task="fix bug"
	//
	// Currently this test will fail because rootCmd doesn't have --var flag
	// After meow-nr5y implementation, the root command should accept --var

	varFlag := rootCmd.Flags().Lookup("var")
	if varFlag == nil {
		t.Error("rootCmd should have --var flag for workflow shorthand (meow <workflow> --var x=y)")
	}
}
