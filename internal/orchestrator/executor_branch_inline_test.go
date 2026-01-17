package orchestrator

import (
	"testing"

	"github.com/akatz-ai/meow/internal/types"
)

// TestExpandInlineSteps_ShellWithVariables tests that inline shell steps get variable substitution
func TestExpandInlineSteps_ShellWithVariables(t *testing.T) {
	inline := []types.InlineStep{
		{
			ID:       "check-file",
			Executor: types.ExecutorShell,
			Command:  "test -f {{filename}}",
		},
	}

	vars := map[string]any{
		"filename": "/tmp/test.txt",
	}

	result, err := expandInlineSteps("parent", inline, vars)
	if err != nil {
		t.Fatalf("expandInlineSteps failed: %v", err)
	}

	if len(result.ExpandedSteps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.ExpandedSteps))
	}

	step := result.ExpandedSteps[0]
	if step.Shell == nil {
		t.Fatal("expected Shell config")
	}

	if step.Shell.Command != "test -f /tmp/test.txt" {
		t.Errorf("expected variable substitution, got %q", step.Shell.Command)
	}
}

// TestExpandInlineSteps_AgentWithVariables tests that inline agent steps get variable substitution
func TestExpandInlineSteps_AgentWithVariables(t *testing.T) {
	inline := []types.InlineStep{
		{
			ID:       "task",
			Executor: types.ExecutorAgent,
			Agent:    "{{agent_name}}",
			Prompt:   "Process {{item}}",
		},
	}

	vars := map[string]any{
		"agent_name": "claude",
		"item":       "file.txt",
	}

	result, err := expandInlineSteps("parent", inline, vars)
	if err != nil {
		t.Fatalf("expandInlineSteps failed: %v", err)
	}

	if len(result.ExpandedSteps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.ExpandedSteps))
	}

	step := result.ExpandedSteps[0]
	if step.Agent == nil {
		t.Fatal("expected Agent config")
	}

	if step.Agent.Agent != "claude" {
		t.Errorf("expected variable substitution in agent, got %q", step.Agent.Agent)
	}

	if step.Agent.Prompt != "Process file.txt" {
		t.Errorf("expected variable substitution in prompt, got %q", step.Agent.Prompt)
	}
}
