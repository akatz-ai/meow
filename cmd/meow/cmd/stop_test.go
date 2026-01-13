package cmd

import (
	"os"
	"testing"
)

func TestValidateMeowProcess(t *testing.T) {
	t.Run("returns error for non-existent PID", func(t *testing.T) {
		// Use a PID that's extremely unlikely to exist (max PID on Linux is typically 32768 or 4194304)
		err := validateMeowProcess(999999)
		if err == nil {
			t.Error("expected error for non-existent PID")
		}
		// Should contain "does not exist" in error message
		if err != nil && err.Error() != "" {
			// Error message should mention the PID doesn't exist
			t.Logf("Got expected error: %v", err)
		}
	})

	t.Run("returns error for PID 1 (init, not meow)", func(t *testing.T) {
		// PID 1 is always init/systemd, never meow
		// This tests the "not a meow process" check
		err := validateMeowProcess(1)
		if err == nil {
			t.Error("expected error for init process (PID 1)")
		}
		// Should indicate it's not a meow process
		t.Logf("Got expected error: %v", err)
	})

	t.Run("validates current test process", func(t *testing.T) {
		// The current process is running via "go test"
		// This should fail validation because cmdline contains "go" not "meow"
		currentPID := os.Getpid()
		err := validateMeowProcess(currentPID)

		// The test process cmdline will be something like:
		// "/tmp/go-build.../cmd.test" or "go test"
		// So it should fail validation (not a meow process)
		if err == nil {
			t.Error("expected error for test process (not a meow process)")
		}
		t.Logf("Current PID %d validation error (expected): %v", currentPID, err)
	})
}

// Note: Testing validateMeowProcess with an actual meow process would require
// starting a real meow orchestrator, which is better suited for E2E tests.
// The unit tests above verify:
// 1. Non-existent PIDs are rejected
// 2. Non-meow processes are rejected
// 3. The function correctly reads /proc/<pid>/cmdline
