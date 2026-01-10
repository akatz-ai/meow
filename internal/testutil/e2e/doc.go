// Package e2e provides End-to-End test infrastructure for MEOW workflows.
//
// The package provides three main components:
//
// # SimConfigBuilder
//
// A fluent API for building simulator configurations. The simulator
// (meow-agent-sim) uses these configs to define how it responds to
// different prompts during tests.
//
//	cfg := e2e.NewSimConfigBuilder().
//	    WithBehavior("implement feature", e2e.ActionComplete).
//	    WithBehaviorOutputs("count files", map[string]any{"count": 42}).
//	    WithDelay(10 * time.Millisecond).
//	    Build()
//
// # Harness
//
// Provides test isolation with:
//   - Isolated temporary directories for each test
//   - Separate tmux socket to prevent interference
//   - Easy workflow state management
//   - Automatic cleanup via t.Cleanup
//
//	h := e2e.NewHarness(t)
//	h.WriteSimConfig(cfg)
//	h.WriteTemplate("my-workflow", templateContent)
//
// # WorkflowRun
//
// Helpers for observing and asserting on running workflows:
//
//	run, _ := h.RunWorkflow("my-workflow")
//	err := run.WaitForStep("step-1", "done", 5*time.Second)
//	output, _ := run.StepOutput("step-1", "result")
//
// # Usage Example
//
//	func TestSimpleWorkflow(t *testing.T) {
//	    h := e2e.NewHarness(t)
//
//	    // Configure simulator
//	    cfg := e2e.NewSimConfigBuilder().
//	        WithBehavior("do work", e2e.ActionComplete).
//	        Build()
//	    h.WriteSimConfig(cfg)
//
//	    // Run workflow
//	    run, err := h.RunWorkflow("simple-workflow")
//	    require.NoError(t, err)
//
//	    // Wait and assert
//	    err = run.WaitForDone(10 * time.Second)
//	    require.NoError(t, err)
//	    require.NoError(t, run.AssertWorkflowDone())
//	}
package e2e
