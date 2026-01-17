package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/akatz-ai/meow/internal/types"
)

func writeWorkflowModule(t *testing.T, path string, workflowName string) {
	t.Helper()

	content := fmt.Sprintf(`
[%s]
name = %q

[[%s.steps]]
id = "step"
executor = "shell"
command = "echo hello"
`, workflowName, workflowName, workflowName)

	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create module dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write module: %v", err)
	}
}

func TestFileTemplateExpander_SubdirectoryWorkflow(t *testing.T) {
	baseDir := t.TempDir()
	modulePath := filepath.Join(baseDir, ".meow", "workflows", "lib", "util.meow.toml")
	writeWorkflowModule(t, modulePath, "monitor")

	expander := NewFileTemplateExpander(baseDir)
	config := &types.ExpandConfig{Template: "lib/util#monitor"}

	result, err := expander.Expand(context.Background(), config, "expand", "", "")
	if err != nil {
		t.Fatalf("Expand failed: %v", err)
	}

	if len(result.Steps) != 1 {
		t.Fatalf("Expected 1 step, got %d", len(result.Steps))
	}

	if result.Steps[0].ID != "expand.step" {
		t.Fatalf("Step ID = %s, want expand.step", result.Steps[0].ID)
	}

	if result.Steps[0].SourceModule != modulePath {
		t.Fatalf("SourceModule = %s, want %s", result.Steps[0].SourceModule, modulePath)
	}
}
