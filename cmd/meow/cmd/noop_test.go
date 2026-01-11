package cmd

import (
	"os"
	"testing"
)

// TestEventNoOpWhenOrchSockNotSet verifies that meow event exits silently
// (returns nil) when MEOW_ORCH_SOCK is not set.
func TestEventNoOpWhenOrchSockNotSet(t *testing.T) {
	// Clear environment variable
	os.Unsetenv("MEOW_ORCH_SOCK")

	// Reset flags
	eventData = nil

	// Run event command
	err := runEvent(eventCmd, []string{"test-event"})
	if err != nil {
		t.Fatalf("expected nil (no-op), got error: %v", err)
	}
}

// TestDoneNoOpWhenOrchSockNotSet verifies that meow done exits silently
// (returns nil) when MEOW_ORCH_SOCK is not set.
func TestDoneNoOpWhenOrchSockNotSet(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("MEOW_ORCH_SOCK")
	os.Unsetenv("MEOW_AGENT")
	os.Unsetenv("MEOW_WORKFLOW")
	os.Unsetenv("MEOW_STEP")

	// Reset flags
	doneNotes = ""
	doneOutputs = nil
	doneOutputJSON = ""

	// Run done command
	err := runDone(doneCmd, nil)
	if err != nil {
		t.Fatalf("expected nil (no-op), got error: %v", err)
	}
}

// TestDoneNoOpEvenWithOutputs verifies that meow done exits silently
// even when output flags are provided, if MEOW_ORCH_SOCK is not set.
func TestDoneNoOpEvenWithOutputs(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("MEOW_ORCH_SOCK")
	os.Unsetenv("MEOW_AGENT")
	os.Unsetenv("MEOW_WORKFLOW")

	// Set output flags
	doneNotes = "some notes"
	doneOutputs = []string{"key=value"}
	doneOutputJSON = ""

	defer func() {
		doneNotes = ""
		doneOutputs = nil
	}()

	// Run done command
	err := runDone(doneCmd, nil)
	if err != nil {
		t.Fatalf("expected nil (no-op), got error: %v", err)
	}
}

// TestEventNoOpEvenWithData verifies that meow event exits silently
// even when data flags are provided, if MEOW_ORCH_SOCK is not set.
func TestEventNoOpEvenWithData(t *testing.T) {
	// Clear environment variable
	os.Unsetenv("MEOW_ORCH_SOCK")

	// Set data flags
	eventData = []string{"tool=Bash", "exit_code=0"}

	defer func() {
		eventData = nil
	}()

	// Run event command
	err := runEvent(eventCmd, []string{"tool-completed"})
	if err != nil {
		t.Fatalf("expected nil (no-op), got error: %v", err)
	}
}

// Note: TestAwaitEventTimeoutWhenOrchSockNotSet is not directly testable
// because runAwaitEvent calls os.Exit(1) directly. In practice, we test
// this behavior through E2E tests or manual testing.
// The behavior is: when MEOW_ORCH_SOCK is not set, await-event exits with
// code 1 (timeout semantics) without any output.
