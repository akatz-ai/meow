package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/meow-stack/meow-machine/internal/types"
)

func TestPrimeRequiresAgent(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("MEOW_AGENT")
	os.Unsetenv("MEOW_WORKFLOW")

	// Reset flags
	primeAgent = ""
	primeFormat = "text"

	// Run prime command
	err := runPrime(primeCmd, nil)
	if err == nil {
		t.Fatal("expected error when MEOW_AGENT not set")
	}

	if err.Error() != "agent not specified and MEOW_AGENT not set" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrimePromptFormatReturnsEmptyWithoutAgent(t *testing.T) {
	// Clear environment variables
	os.Unsetenv("MEOW_AGENT")
	os.Unsetenv("MEOW_WORKFLOW")

	// Reset flags
	primeAgent = ""
	primeFormat = "prompt"

	// Run prime command - should not error in prompt format
	err := runPrime(primeCmd, nil)
	if err != nil {
		t.Fatalf("expected no error in prompt format, got: %v", err)
	}
}

func TestPrimePromptFormatReturnsEmptyWithoutWorkflow(t *testing.T) {
	// Set agent but no workflow
	os.Setenv("MEOW_AGENT", "test-agent")
	os.Unsetenv("MEOW_WORKFLOW")
	defer os.Unsetenv("MEOW_AGENT")

	// Reset flags
	primeAgent = ""
	primeFormat = "prompt"

	// Run prime command - should not error
	err := runPrime(primeCmd, nil)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestGetPrimeOutputFromFiles_NoWorkflowsDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Non-existent workflows dir
	workflowsDir := filepath.Join(tmpDir, "workflows")

	output, err := getPrimeOutputFromFiles(workflowsDir, "test-agent", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !output.NoWork {
		t.Error("expected NoWork=true for non-existent workflows dir")
	}
}

func TestGetPrimeOutputFromFiles_EmptyWorkflowsDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workflowsDir := filepath.Join(tmpDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}

	output, err := getPrimeOutputFromFiles(workflowsDir, "test-agent", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !output.NoWork {
		t.Error("expected NoWork=true for empty workflows dir")
	}
}

func TestGetPrimeOutputFromFiles_RunningStep(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workflowsDir := filepath.Join(tmpDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a workflow with a running agent step
	wf := &types.Workflow{
		ID:       "wf-test-1",
		Template: "test.meow.toml",
		Status:   types.WorkflowStatusRunning,
		Steps: map[string]*types.Step{
			"do-work": {
				ID:       "do-work",
				Executor: types.ExecutorAgent,
				Status:   types.StepStatusRunning,
				Agent: &types.AgentConfig{
					Agent:  "test-agent",
					Prompt: "Do the work",
					Mode:   "autonomous",
				},
			},
		},
	}

	data, err := yaml.Marshal(wf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowsDir, "wf-test-1.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	output, err := getPrimeOutputFromFiles(workflowsDir, "test-agent", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.NoWork {
		t.Error("expected work to be found")
	}
	if output.CurrentStep == nil {
		t.Fatal("expected current step")
	}
	if output.CurrentStep.ID != "do-work" {
		t.Errorf("expected step ID do-work, got %s", output.CurrentStep.ID)
	}
	if output.CurrentStep.Prompt != "Do the work" {
		t.Errorf("expected prompt 'Do the work', got %q", output.CurrentStep.Prompt)
	}
	if output.CurrentStep.Mode != "autonomous" {
		t.Errorf("expected mode autonomous, got %s", output.CurrentStep.Mode)
	}
}

func TestGetPrimeOutputFromFiles_InteractiveMode(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workflowsDir := filepath.Join(tmpDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a workflow with an interactive step
	wf := &types.Workflow{
		ID:       "wf-test-1",
		Template: "test.meow.toml",
		Status:   types.WorkflowStatusRunning,
		Steps: map[string]*types.Step{
			"interactive-step": {
				ID:       "interactive-step",
				Executor: types.ExecutorAgent,
				Status:   types.StepStatusRunning,
				Agent: &types.AgentConfig{
					Agent:  "test-agent",
					Prompt: "Interactive discussion",
					Mode:   "interactive",
				},
			},
		},
	}

	data, err := yaml.Marshal(wf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowsDir, "wf-test-1.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	output, err := getPrimeOutputFromFiles(workflowsDir, "test-agent", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !output.Interactive {
		t.Error("expected Interactive=true for interactive mode step")
	}
	if output.CurrentStep != nil {
		t.Error("expected no current step for interactive mode (agent should idle)")
	}
}

func TestGetPrimeOutputFromFiles_CompletingStatus(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workflowsDir := filepath.Join(tmpDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a workflow with a completing step
	wf := &types.Workflow{
		ID:       "wf-test-1",
		Template: "test.meow.toml",
		Status:   types.WorkflowStatusRunning,
		Steps: map[string]*types.Step{
			"completing-step": {
				ID:       "completing-step",
				Executor: types.ExecutorAgent,
				Status:   types.StepStatusCompleting,
				Agent: &types.AgentConfig{
					Agent:  "test-agent",
					Prompt: "Do the work",
					Mode:   "autonomous",
				},
			},
		},
	}

	data, err := yaml.Marshal(wf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowsDir, "wf-test-1.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	output, err := getPrimeOutputFromFiles(workflowsDir, "test-agent", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !output.NoWork {
		t.Error("expected NoWork=true for completing step")
	}
}

func TestGetPrimeOutputFromFiles_WithOutputs(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workflowsDir := filepath.Join(tmpDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a workflow with required outputs
	wf := &types.Workflow{
		ID:       "wf-test-1",
		Template: "test.meow.toml",
		Status:   types.WorkflowStatusRunning,
		Steps: map[string]*types.Step{
			"do-work": {
				ID:       "do-work",
				Executor: types.ExecutorAgent,
				Status:   types.StepStatusRunning,
				Agent: &types.AgentConfig{
					Agent:  "test-agent",
					Prompt: "Write the code",
					Mode:   "autonomous",
					Outputs: map[string]types.AgentOutputDef{
						"file_path": {
							Required:    true,
							Type:        "file_path",
							Description: "Path to the created file",
						},
						"commit_sha": {
							Required:    false,
							Type:        "string",
							Description: "Git commit SHA if committed",
						},
					},
				},
			},
		},
	}

	data, err := yaml.Marshal(wf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowsDir, "wf-test-1.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	output, err := getPrimeOutputFromFiles(workflowsDir, "test-agent", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.CurrentStep == nil {
		t.Fatal("expected current step")
	}
	if len(output.CurrentStep.Outputs) != 2 {
		t.Errorf("expected 2 outputs, got %d", len(output.CurrentStep.Outputs))
	}

	filePath, ok := output.CurrentStep.Outputs["file_path"]
	if !ok {
		t.Error("expected file_path output")
	} else {
		if !filePath.Required {
			t.Error("expected file_path to be required")
		}
		if filePath.Type != "file_path" {
			t.Errorf("expected file_path type, got %s", filePath.Type)
		}
	}
}

func TestGetPrimeOutputFromFiles_DefaultMode(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workflowsDir := filepath.Join(tmpDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a workflow with no mode set (should default to autonomous)
	wf := &types.Workflow{
		ID:       "wf-test-1",
		Template: "test.meow.toml",
		Status:   types.WorkflowStatusRunning,
		Steps: map[string]*types.Step{
			"do-work": {
				ID:       "do-work",
				Executor: types.ExecutorAgent,
				Status:   types.StepStatusRunning,
				Agent: &types.AgentConfig{
					Agent:  "test-agent",
					Prompt: "Do the work",
					Mode:   "", // Empty - should default to autonomous
				},
			},
		},
	}

	data, err := yaml.Marshal(wf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowsDir, "wf-test-1.yaml"), data, 0644); err != nil {
		t.Fatal(err)
	}

	output, err := getPrimeOutputFromFiles(workflowsDir, "test-agent", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.CurrentStep == nil {
		t.Fatal("expected current step")
	}
	if output.CurrentStep.Mode != "autonomous" {
		t.Errorf("expected default mode to be autonomous, got %s", output.CurrentStep.Mode)
	}
}

func TestGetPrimeOutputFromFiles_SpecificWorkflowID(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "meow-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	workflowsDir := filepath.Join(tmpDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create two workflows
	wf1 := &types.Workflow{
		ID:       "wf-1",
		Template: "test1.meow.toml",
		Status:   types.WorkflowStatusRunning,
		Steps: map[string]*types.Step{
			"step-1": {
				ID:       "step-1",
				Executor: types.ExecutorAgent,
				Status:   types.StepStatusRunning,
				Agent: &types.AgentConfig{
					Agent:  "test-agent",
					Prompt: "Prompt from workflow 1",
				},
			},
		},
	}

	wf2 := &types.Workflow{
		ID:       "wf-2",
		Template: "test2.meow.toml",
		Status:   types.WorkflowStatusRunning,
		Steps: map[string]*types.Step{
			"step-2": {
				ID:       "step-2",
				Executor: types.ExecutorAgent,
				Status:   types.StepStatusRunning,
				Agent: &types.AgentConfig{
					Agent:  "test-agent",
					Prompt: "Prompt from workflow 2",
				},
			},
		},
	}

	for _, wf := range []*types.Workflow{wf1, wf2} {
		data, err := yaml.Marshal(wf)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(workflowsDir, wf.ID+".yaml"), data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Request specific workflow
	output, err := getPrimeOutputFromFiles(workflowsDir, "test-agent", "wf-2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.CurrentStep == nil {
		t.Fatal("expected current step")
	}
	if output.CurrentStep.Prompt != "Prompt from workflow 2" {
		t.Errorf("expected prompt from workflow 2, got %q", output.CurrentStep.Prompt)
	}
}

func TestFormatPrimeText_NoWork(t *testing.T) {
	output := &PrimeOutput{NoWork: true}
	text := formatPrimeText(output)
	if text != "No work assigned.\n" {
		t.Errorf("unexpected text: %q", text)
	}
}

func TestFormatPrimeText_Interactive(t *testing.T) {
	output := &PrimeOutput{
		Interactive: true,
		Workflow: &WorkflowInfo{
			ID:       "wf-test",
			Template: "test.meow.toml",
			Status:   "running",
		},
	}
	text := formatPrimeText(output)
	if text == "" {
		t.Error("expected non-empty text")
	}
	if !contains(text, "Interactive mode") {
		t.Error("expected 'Interactive mode' in output")
	}
	if !contains(text, "wf-test") {
		t.Error("expected workflow ID in output")
	}
}

func TestFormatPrimeText_WithStep(t *testing.T) {
	output := &PrimeOutput{
		Workflow: &WorkflowInfo{
			ID:       "wf-test",
			Template: "test.meow.toml",
			Status:   "running",
		},
		CurrentStep: &StepInfo{
			ID:     "do-work",
			Status: "running",
			Prompt: "Write the code",
			Mode:   "autonomous",
			Outputs: map[string]types.AgentOutputDef{
				"file_path": {
					Required:    true,
					Type:        "file_path",
					Description: "Path to file",
				},
			},
		},
	}
	text := formatPrimeText(output)

	if !contains(text, "do-work") {
		t.Error("expected step ID in output")
	}
	if !contains(text, "autonomous mode") {
		t.Error("expected mode in output")
	}
	if !contains(text, "Write the code") {
		t.Error("expected prompt in output")
	}
	if !contains(text, "file_path") {
		t.Error("expected output name in output")
	}
	if !contains(text, "meow done") {
		t.Error("expected completion instructions in output")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 1; i < len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
