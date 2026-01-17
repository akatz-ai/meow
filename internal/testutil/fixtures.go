// Package testutil provides test infrastructure, fixtures, and helpers for MEOW.
package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/akatz-ai/meow/internal/config"
	"github.com/akatz-ai/meow/internal/workflow"
	"github.com/akatz-ai/meow/internal/types"
)

// NewTestAgent creates a test agent with sensible defaults.
func NewTestAgent(t *testing.T) *types.Agent {
	t.Helper()
	now := time.Now()
	return &types.Agent{
		ID:            fmt.Sprintf("test-agent-%d", now.UnixNano()%100000),
		Name:          "Test Agent",
		Status:        types.AgentStatusActive,
		TmuxSession:   "meow-test-agent",
		Workdir:       t.TempDir(),
		CreatedAt:     &now,
		LastHeartbeat: &now,
	}
}

// NewTestAgentWithID creates a test agent with a specific ID.
func NewTestAgentWithID(t *testing.T, id string) *types.Agent {
	t.Helper()
	agent := NewTestAgent(t)
	agent.ID = id
	agent.Name = id
	agent.TmuxSession = "meow-" + id
	return agent
}

// NewTestConfig creates a test configuration with sensible defaults.
// The paths are set to temporary directories that will be cleaned up.
func NewTestConfig(t *testing.T) *config.Config {
	t.Helper()
	tmpDir := t.TempDir()

	cfg := config.Default()
	cfg.Paths.WorkflowDir = filepath.Join(tmpDir, "workflows")
	cfg.Paths.RunsDir = filepath.Join(tmpDir, "runs")
	cfg.Paths.LogsDir = filepath.Join(tmpDir, "logs")
	cfg.Logging.Level = config.LogLevelDebug

	// Create the directories
	for _, dir := range []string{cfg.Paths.WorkflowDir, cfg.Paths.RunsDir, cfg.Paths.LogsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	return cfg
}

// NewTestTemplate creates a test template with the given name and steps.
func NewTestTemplate(t *testing.T, name string) *workflow.Template {
	t.Helper()
	return &workflow.Template{
		Meta: workflow.Meta{
			Name:        name,
			Version:     "1.0.0",
			Description: fmt.Sprintf("Test template: %s", name),
		},
		Variables: map[string]workflow.Var{
			"test_var": {
				Required:    false,
				Default:     "default_value",
				Type:        workflow.VarTypeString,
				Description: "A test variable",
			},
		},
		Steps: []workflow.Step{
			{
				ID:      "step-1",
				Executor: workflow.ExecutorShell,
				Command: "echo test",
			},
		},
	}
}

// NewTestTemplateWithSteps creates a test template with custom steps.
func NewTestTemplateWithSteps(t *testing.T, name string, steps []workflow.Step) *workflow.Template {
	t.Helper()
	tmpl := NewTestTemplate(t, name)
	tmpl.Steps = steps
	return tmpl
}

// NewTestWorkspace creates a temporary workspace directory with a basic structure.
// Returns the workspace path and a cleanup function.
func NewTestWorkspace(t *testing.T) (string, func()) {
	t.Helper()

	// Create a temporary directory for the workspace
	tmpDir := t.TempDir()

	// Create standard subdirectories
	dirs := []string{
		".meow",
		".meow/workflows",
		".meow/runs",
		".meow/logs",
	}

	for _, dir := range dirs {
		path := filepath.Join(tmpDir, dir)
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", path, err)
		}
	}

	// Create a minimal config file
	configPath := filepath.Join(tmpDir, ".meow", "config.toml")
	configContent := `version = "1"

[paths]
workflow_dir = ".meow/workflows"
runs_dir = ".meow/runs"
logs_dir = ".meow/logs"

[logging]
level = "debug"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cleanup := func() {
		// t.TempDir() handles cleanup automatically
	}

	return tmpDir, cleanup
}

// NewTestWorkspaceWithTemplate creates a workspace with a template file.
func NewTestWorkspaceWithTemplate(t *testing.T, templateName, templateContent string) (string, func()) {
	t.Helper()

	workspace, cleanup := NewTestWorkspace(t)

	templatePath := filepath.Join(workspace, ".meow", "workflows", templateName+".toml")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("Failed to write template file: %v", err)
	}

	return workspace, cleanup
}

// TestTemplateContent returns a valid TOML template string for testing.
func TestTemplateContent() string {
	return `[meta]
name = "test-template"
version = "1.0.0"
description = "A test template"

[variables.test_var]
required = false
default = "test_value"
description = "Test variable"

[[steps]]
id = "step-1"
description = "First step"
instructions = "Do the first thing"

[[steps]]
id = "step-2"
description = "Second step"
needs = ["step-1"]
instructions = "Do the second thing"
`
}

// ComplexTemplateContent returns a more complex template for integration testing.
func ComplexTemplateContent() string {
	return `[meta]
name = "complex-test-template"
version = "1.0.0"
description = "A complex test template with conditions and multiple steps"
type = "loop"
max_iterations = 3
on_error = "inject-gate"

[variables.work_bead]
required = true
type = "string"
description = "The bead ID to work on"

[variables.max_retries]
required = false
default = 3
type = "int"
description = "Maximum retry count"

[[steps]]
id = "setup"
description = "Setup step"
instructions = "Initialize the workspace"

[[steps]]
id = "work"
description = "Main work step"
needs = ["setup"]
instructions = "Do the main work on ${work_bead}"

[[steps]]
id = "verify"
description = "Verification step"
needs = ["work"]
condition = "test -f result.txt"
[steps.on_true]
template = "complete"
[steps.on_false]
template = "retry"

[[steps]]
id = "cleanup"
description = "Cleanup step"
needs = ["verify"]
instructions = "Clean up resources"
ephemeral = true
`
}
