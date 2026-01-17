package workflow_test

import (
	"os"
	"path/filepath"
	"testing"
	
	"github.com/meow-stack/meow-machine/internal/workflow"
)

func TestBaker_PreservesOutputType(t *testing.T) {
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

	// Bake the workflow
	baker := workflow.NewBaker("test-workflow")
	result, err := baker.BakeWorkflow(wf, nil)
	if err != nil {
		t.Fatalf("BakeWorkflow failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Steps))
	}

	step := result.Steps[0]
	if step.Shell == nil {
		t.Fatalf("step.Shell is nil")
	}

	if len(step.Shell.Outputs) == 0 {
		t.Fatalf("step.Shell.Outputs is empty")
	}

	config, ok := step.Shell.Outputs["config"]
	if !ok {
		t.Fatalf("config not found in Shell.Outputs: %v", step.Shell.Outputs)
	}

	if config.Source != "stdout" {
		t.Errorf("expected source='stdout', got %q", config.Source)
	}

	if config.Type != "json" {
		t.Errorf("expected type='json', got %q", config.Type)
	}
}
