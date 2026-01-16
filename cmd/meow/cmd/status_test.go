package cmd

import (
	"testing"
)

func TestStatusExitError(t *testing.T) {
	t.Run("Error method returns message", func(t *testing.T) {
		err := &StatusExitError{Code: ExitNoWorkflows, Message: "no active workflows"}
		if err.Error() != "no active workflows" {
			t.Errorf("expected 'no active workflows', got %q", err.Error())
		}
	})

	t.Run("exit codes are distinct", func(t *testing.T) {
		// Ensure exit codes don't overlap
		if ExitSuccess == ExitNoWorkflows {
			t.Error("ExitSuccess and ExitNoWorkflows should be different")
		}
		if ExitNoWorkflows == ExitWorkflowNotFound {
			t.Error("ExitNoWorkflows and ExitWorkflowNotFound should be different")
		}
		if ExitSuccess == ExitWorkflowNotFound {
			t.Error("ExitSuccess and ExitWorkflowNotFound should be different")
		}
	})
}

func TestStatusStrictFlag(t *testing.T) {
	// The --strict flag should be registered on statusCmd
	flag := statusCmd.Flags().Lookup("strict")
	if flag == nil {
		t.Fatal("--strict flag not found on status command")
	}

	if flag.DefValue != "false" {
		t.Errorf("--strict default should be false, got %s", flag.DefValue)
	}

	if flag.Usage == "" {
		t.Error("--strict flag should have usage text")
	}
}
