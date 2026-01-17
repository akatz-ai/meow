package workflow_test

import (
	"os"
	"path/filepath"
	"testing"
	
	"github.com/akatz-ai/meow/internal/workflow"
)

func TestParseShellOutputsWithType(t *testing.T) {
	// Create a temporary template file
	template := `
[main]
name = "test-json-output"

[[main.steps]]
id = "init"
executor = "shell"
command = "echo '{\"name\":\"test\"}'"

[main.steps.shell_outputs]
config = { source = "stdout", type = "json" }
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.toml")
	if err := os.WriteFile(tmpFile, []byte(template), 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	module, err := workflow.ParseModuleFile(tmpFile)
	if err != nil {
		t.Fatalf("ParseModuleFile failed: %v", err)
	}

	wf := module.GetWorkflow("main")
	if wf == nil {
		t.Fatalf("main workflow not found")
	}

	if len(wf.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(wf.Steps))
	}

	step := wf.Steps[0]
	if step.ID != "init" {
		t.Errorf("expected step id 'init', got %q", step.ID)
	}

	if len(step.ShellOutputs) == 0 {
		t.Fatalf("ShellOutputs is empty")
	}

	config, ok := step.ShellOutputs["config"]
	if !ok {
		t.Fatalf("config not found in ShellOutputs: %v", step.ShellOutputs)
	}

	if config.Source != "stdout" {
		t.Errorf("expected source='stdout', got %q", config.Source)
	}

	if config.Type != "json" {
		t.Errorf("expected type='json', got %q", config.Type)
	}
}
