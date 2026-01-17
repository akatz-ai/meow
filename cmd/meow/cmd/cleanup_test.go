package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
)

func TestContains(t *testing.T) {
	t.Run("finds existing element", func(t *testing.T) {
		slice := []string{"a", "b", "c"}
		if !contains(slice, "b") {
			t.Error("expected to find 'b' in slice")
		}
	})

	t.Run("returns false for missing element", func(t *testing.T) {
		slice := []string{"a", "b", "c"}
		if contains(slice, "d") {
			t.Error("expected not to find 'd' in slice")
		}
	})

	t.Run("handles empty slice", func(t *testing.T) {
		var slice []string
		if contains(slice, "a") {
			t.Error("expected not to find 'a' in empty slice")
		}
	})
}

func TestCleanupFlags(t *testing.T) {
	t.Run("--dry-run flag registered", func(t *testing.T) {
		flag := cleanupCmd.Flags().Lookup("dry-run")
		if flag == nil {
			t.Fatal("--dry-run flag not found")
		}
		if flag.DefValue != "false" {
			t.Errorf("--dry-run default should be false, got %s", flag.DefValue)
		}
	})

	t.Run("--yes/-y flag registered", func(t *testing.T) {
		flag := cleanupCmd.Flags().Lookup("yes")
		if flag == nil {
			t.Fatal("--yes flag not found")
		}
		if flag.Shorthand != "y" {
			t.Errorf("--yes shorthand should be 'y', got %q", flag.Shorthand)
		}
		if flag.DefValue != "false" {
			t.Errorf("--yes default should be false, got %s", flag.DefValue)
		}
	})

	t.Run("--list/-l flag registered", func(t *testing.T) {
		flag := cleanupCmd.Flags().Lookup("list")
		if flag == nil {
			t.Fatal("--list flag not found")
		}
		if flag.Shorthand != "l" {
			t.Errorf("--list shorthand should be 'l', got %q", flag.Shorthand)
		}
		if flag.DefValue != "false" {
			t.Errorf("--list default should be false, got %s", flag.DefValue)
		}
	})
}

func TestDiscoverResources(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	t.Run("discovers sessions from run state", func(t *testing.T) {
		wf := &types.Run{
			ID:       "run-test-123",
			Template: "test-template",
			Status:   types.RunStatusStopped,
			Agents: map[string]*types.AgentInfo{
				"agent-0": {TmuxSession: "meow-run-test-123-agent-0"},
				"agent-1": {TmuxSession: "meow-run-test-123-agent-1"},
			},
		}

		resources, err := discoverResources(ctx, wf, tmpDir)
		if err != nil {
			t.Fatalf("discoverResources failed: %v", err)
		}

		if len(resources.SessionsFromState) != 2 {
			t.Errorf("expected 2 sessions from state, got %d", len(resources.SessionsFromState))
		}

		// Sessions should be discovered
		if len(resources.Sessions) != 2 {
			t.Errorf("expected 2 total sessions, got %d", len(resources.Sessions))
		}
	})

	t.Run("selects correct cleanup script based on status", func(t *testing.T) {
		tests := []struct {
			status   types.RunStatus
			onSuccess string
			onFailure string
			onStop    string
			wantTrigger string
		}{
			{
				status:      types.RunStatusDone,
				onSuccess:   "echo success",
				wantTrigger: "cleanup_on_success",
			},
			{
				status:      types.RunStatusFailed,
				onFailure:   "echo failed",
				wantTrigger: "cleanup_on_failure",
			},
			{
				status:      types.RunStatusStopped,
				onStop:      "echo stopped",
				wantTrigger: "cleanup_on_stop",
			},
			{
				status:      types.RunStatusDone,
				// No scripts defined
				wantTrigger: "",
			},
		}

		for _, tt := range tests {
			t.Run(string(tt.status), func(t *testing.T) {
				wf := &types.Run{
					ID:               "run-test",
					Status:           tt.status,
					CleanupOnSuccess: tt.onSuccess,
					CleanupOnFailure: tt.onFailure,
					CleanupOnStop:    tt.onStop,
					Agents:           make(map[string]*types.AgentInfo),
				}

				resources, err := discoverResources(ctx, wf, tmpDir)
				if err != nil {
					t.Fatalf("discoverResources failed: %v", err)
				}

				if resources.CleanupTrigger != tt.wantTrigger {
					t.Errorf("CleanupTrigger = %q, want %q", resources.CleanupTrigger, tt.wantTrigger)
				}

				if tt.wantTrigger != "" && resources.CleanupScript == "" {
					t.Error("expected cleanup script to be set")
				}
			})
		}
	})
}

func TestCleanupValidation(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()
	runsDir := filepath.Join(tmpDir, ".meow", "runs")

	// Create runs directory
	if err := os.MkdirAll(runsDir, 0755); err != nil {
		t.Fatalf("failed to create runs dir: %v", err)
	}

	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	t.Run("rejects non-terminal workflow", func(t *testing.T) {
		// Create a running workflow
		now := time.Now()
		wf := &types.Run{
			ID:        "run-running",
			Template:  "test",
			Status:    types.RunStatusRunning,
			StartedAt: now,
			Steps:     make(map[string]*types.Step),
		}

		if err := store.Save(ctx, wf); err != nil {
			t.Fatalf("failed to save workflow: %v", err)
		}

		// Verify the validation logic
		loaded, err := store.Get(ctx, "run-running")
		if err != nil {
			t.Fatalf("failed to load workflow: %v", err)
		}

		if loaded.Status.IsTerminal() {
			t.Error("running workflow should not be terminal")
		}
	})

	t.Run("accepts terminal workflows", func(t *testing.T) {
		terminalStatuses := []types.RunStatus{
			types.RunStatusDone,
			types.RunStatusFailed,
			types.RunStatusStopped,
		}

		for _, status := range terminalStatuses {
			t.Run(string(status), func(t *testing.T) {
				now := time.Now()
				wf := &types.Run{
					ID:        "run-" + string(status),
					Template:  "test",
					Status:    status,
					StartedAt: now,
					DoneAt:    &now,
					Steps:     make(map[string]*types.Step),
				}

				if err := store.Save(ctx, wf); err != nil {
					t.Fatalf("failed to save workflow: %v", err)
				}

				loaded, err := store.Get(ctx, wf.ID)
				if err != nil {
					t.Fatalf("failed to load workflow: %v", err)
				}

				if !loaded.Status.IsTerminal() {
					t.Errorf("status %s should be terminal", status)
				}
			})
		}
	})
}

func TestCleanupTimeout(t *testing.T) {
	// Verify the cleanup timeout constant is reasonable
	if CleanupTimeout < 30*time.Second {
		t.Errorf("CleanupTimeout too short: %v", CleanupTimeout)
	}
	if CleanupTimeout > 5*time.Minute {
		t.Errorf("CleanupTimeout too long: %v", CleanupTimeout)
	}
}
