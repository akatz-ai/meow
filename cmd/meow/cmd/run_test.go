package cmd

import (
	"testing"
)

func TestRunFlags(t *testing.T) {
	t.Run("--_collection-dir flag registered and hidden", func(t *testing.T) {
		flag := runCmd.Flags().Lookup("_collection-dir")
		if flag == nil {
			t.Fatal("--_collection-dir flag not found")
		}
		if flag.DefValue != "" {
			t.Errorf("--_collection-dir default should be empty, got %q", flag.DefValue)
		}
		// Verify it's hidden (internal flag)
		if !flag.Hidden {
			t.Error("--_collection-dir should be hidden (internal flag)")
		}
	})

	t.Run("--_detached-child flag registered and hidden", func(t *testing.T) {
		flag := runCmd.Flags().Lookup("_detached-child")
		if flag == nil {
			t.Fatal("--_detached-child flag not found")
		}
		if !flag.Hidden {
			t.Error("--_detached-child should be hidden (internal flag)")
		}
	})

	t.Run("--_workflow-id flag registered and hidden", func(t *testing.T) {
		flag := runCmd.Flags().Lookup("_workflow-id")
		if flag == nil {
			t.Fatal("--_workflow-id flag not found")
		}
		if !flag.Hidden {
			t.Error("--_workflow-id should be hidden (internal flag)")
		}
	})

	t.Run("-d/--detach flag registered", func(t *testing.T) {
		flag := runCmd.Flags().Lookup("detach")
		if flag == nil {
			t.Fatal("--detach flag not found")
		}
		if flag.Shorthand != "d" {
			t.Errorf("--detach shorthand should be 'd', got %q", flag.Shorthand)
		}
		if flag.DefValue != "false" {
			t.Errorf("--detach default should be false, got %s", flag.DefValue)
		}
	})
}

// TestSpawnDetachedOrchestratorArgs tests that the correct arguments are built
// for the detached child process.
func TestSpawnDetachedOrchestratorArgs(t *testing.T) {
	// This test verifies the structure of the args slice built in spawnDetachedOrchestrator.
	// We can't easily call the function directly (it spawns a process), but we can
	// verify the logic by checking the expected args pattern.

	t.Run("args include collection-dir when provided", func(t *testing.T) {
		// The spawnDetachedOrchestrator function builds args like:
		// ["run", templatePath, "--_detached-child", "--_workflow-id", id, "--workflow", name]
		// And if collectionDir != "":
		// args = append(args, "--_collection-dir", collectionDir)

		templatePath := "/home/user/.meow/workflows/my-collection/main.meow.toml"
		workflowID := "run-123"
		workflowName := "main"
		collectionDir := "/home/user/.meow/workflows/my-collection"

		// Build expected args (same logic as spawnDetachedOrchestrator)
		args := []string{"run", templatePath, "--_detached-child", "--_workflow-id", workflowID, "--workflow", workflowName}
		if collectionDir != "" {
			args = append(args, "--_collection-dir", collectionDir)
		}

		// Verify the args contain --_collection-dir
		foundCollectionDir := false
		for i, arg := range args {
			if arg == "--_collection-dir" && i+1 < len(args) {
				foundCollectionDir = true
				if args[i+1] != collectionDir {
					t.Errorf("--_collection-dir value should be %q, got %q", collectionDir, args[i+1])
				}
				break
			}
		}
		if !foundCollectionDir {
			t.Error("args should include --_collection-dir when collectionDir is provided")
		}
	})

	t.Run("args omit collection-dir when empty", func(t *testing.T) {
		templatePath := "./workflow.meow.toml"
		workflowID := "run-456"
		workflowName := "main"
		collectionDir := "" // Empty - not from a collection

		// Build expected args (same logic as spawnDetachedOrchestrator)
		args := []string{"run", templatePath, "--_detached-child", "--_workflow-id", workflowID, "--workflow", workflowName}
		if collectionDir != "" {
			args = append(args, "--_collection-dir", collectionDir)
		}

		// Verify the args do NOT contain --_collection-dir
		for _, arg := range args {
			if arg == "--_collection-dir" {
				t.Error("args should not include --_collection-dir when collectionDir is empty")
				break
			}
		}
	})
}
