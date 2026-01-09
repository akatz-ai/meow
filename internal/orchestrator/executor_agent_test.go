package orchestrator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

func TestStartAgentStep_Basic(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Do the work",
		},
	}

	result, stepErr := StartAgentStep(step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if !strings.Contains(result.Prompt, "Do the work") {
		t.Errorf("expected prompt to contain 'Do the work', got %q", result.Prompt)
	}
	if !strings.Contains(result.Prompt, "meow done") {
		t.Errorf("expected prompt to mention 'meow done', got %q", result.Prompt)
	}
}

func TestStartAgentStep_WithOutputs(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Implement the feature",
			Outputs: map[string]types.AgentOutputDef{
				"main_file": {
					Required:    true,
					Type:        "file_path",
					Description: "Path to main implementation file",
				},
				"test_count": {
					Required: false,
					Type:     "number",
				},
			},
		},
	}

	result, stepErr := StartAgentStep(step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Check prompt includes output expectations
	if !strings.Contains(result.Prompt, "Expected Outputs") {
		t.Error("expected prompt to include 'Expected Outputs' section")
	}
	if !strings.Contains(result.Prompt, "main_file") {
		t.Error("expected prompt to mention 'main_file' output")
	}
	if !strings.Contains(result.Prompt, "(required)") {
		t.Error("expected prompt to mark required outputs")
	}
	if !strings.Contains(result.Prompt, "file_path") {
		t.Error("expected prompt to show output type")
	}
	if !strings.Contains(result.Prompt, "meow done --output") {
		t.Error("expected prompt to show meow done syntax with outputs")
	}
}

func TestStartAgentStep_MissingConfig(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent:    nil,
	}

	_, stepErr := StartAgentStep(step)
	if stepErr == nil {
		t.Fatal("expected error for missing config")
	}
	if stepErr.Message != "agent step missing config" {
		t.Errorf("unexpected error: %s", stepErr.Message)
	}
}

func TestStartAgentStep_MissingAgent(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "",
			Prompt: "Do something",
		},
	}

	_, stepErr := StartAgentStep(step)
	if stepErr == nil {
		t.Fatal("expected error for missing agent")
	}
	if stepErr.Message != "agent step missing agent field" {
		t.Errorf("unexpected error: %s", stepErr.Message)
	}
}

func TestStartAgentStep_MissingPrompt(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "",
		},
	}

	_, stepErr := StartAgentStep(step)
	if stepErr == nil {
		t.Fatal("expected error for missing prompt")
	}
	if stepErr.Message != "agent step missing prompt field" {
		t.Errorf("unexpected error: %s", stepErr.Message)
	}
}

func TestCompleteAgentStep_NoOutputDefs(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Do something",
			// No output definitions
		},
	}

	outputs := map[string]any{
		"anything": "is allowed",
	}

	result, stepErr := CompleteAgentStep(step, outputs, "")
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if len(result.ValidationErrors) != 0 {
		t.Errorf("expected no validation errors, got %v", result.ValidationErrors)
	}
}

func TestCompleteAgentStep_ValidOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "main.go")
	if err := os.WriteFile(testFile, []byte("package main"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Implement feature",
			Outputs: map[string]types.AgentOutputDef{
				"main_file": {Required: true, Type: "file_path"},
				"count":     {Required: true, Type: "number"},
				"name":      {Required: false, Type: "string"},
			},
		},
	}

	outputs := map[string]any{
		"main_file": testFile,
		"count":     42,
		"name":      "feature-x",
	}

	result, stepErr := CompleteAgentStep(step, outputs, tmpDir)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	if len(result.ValidationErrors) != 0 {
		t.Errorf("expected no validation errors, got %v", result.ValidationErrors)
	}
}

func TestCompleteAgentStep_MissingRequiredOutput(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Implement feature",
			Outputs: map[string]types.AgentOutputDef{
				"required_field": {Required: true, Type: "string"},
			},
		},
	}

	outputs := map[string]any{
		// Missing required_field
	}

	result, stepErr := CompleteAgentStep(step, outputs, "")
	if stepErr == nil {
		t.Fatal("expected validation error for missing required output")
	}

	if len(result.ValidationErrors) == 0 {
		t.Error("expected validation errors")
	}
	if !strings.Contains(result.ValidationErrors[0], "missing required output") {
		t.Errorf("expected 'missing required output' error, got %q", result.ValidationErrors[0])
	}
}

func TestCompleteAgentStep_WrongType(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Do something",
			Outputs: map[string]types.AgentOutputDef{
				"count": {Required: true, Type: "number"},
			},
		},
	}

	outputs := map[string]any{
		"count": "not a number", // Wrong type
	}

	result, stepErr := CompleteAgentStep(step, outputs, "")
	if stepErr == nil {
		t.Fatal("expected validation error for wrong type")
	}

	if !strings.Contains(result.ValidationErrors[0], "expected number") {
		t.Errorf("expected type error, got %q", result.ValidationErrors[0])
	}
}

func TestCompleteAgentStep_FilePathNotExists(t *testing.T) {
	tmpDir := t.TempDir()

	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Create file",
			Outputs: map[string]types.AgentOutputDef{
				"file": {Required: true, Type: "file_path"},
			},
		},
	}

	outputs := map[string]any{
		"file": filepath.Join(tmpDir, "nonexistent.txt"),
	}

	result, stepErr := CompleteAgentStep(step, outputs, tmpDir)
	if stepErr == nil {
		t.Fatal("expected validation error for nonexistent file")
	}

	if !strings.Contains(result.ValidationErrors[0], "does not exist") {
		t.Errorf("expected 'does not exist' error, got %q", result.ValidationErrors[0])
	}
}

func TestCompleteAgentStep_FilePathOutsideWorkdir(t *testing.T) {
	tmpDir := t.TempDir()
	outsideFile := "/etc/passwd"

	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Create file",
			Outputs: map[string]types.AgentOutputDef{
				"file": {Required: true, Type: "file_path"},
			},
		},
	}

	outputs := map[string]any{
		"file": outsideFile,
	}

	result, stepErr := CompleteAgentStep(step, outputs, tmpDir)
	if stepErr == nil {
		t.Fatal("expected validation error for file outside workdir")
	}

	if !strings.Contains(result.ValidationErrors[0], "within agent workdir") {
		t.Errorf("expected workdir error, got %q", result.ValidationErrors[0])
	}
}

func TestCompleteAgentStep_FilePathPrefixAttack(t *testing.T) {
	// Test that path prefix attacks are blocked
	// e.g., workdir="/tmp/work" shouldn't allow "/tmp/workspace/evil.txt"
	tmpBase := t.TempDir()
	workDir := filepath.Join(tmpBase, "work")
	attackDir := filepath.Join(tmpBase, "workspace") // Similar prefix!
	os.MkdirAll(workDir, 0755)
	os.MkdirAll(attackDir, 0755)

	// Create a file in the attack directory
	evilFile := filepath.Join(attackDir, "evil.txt")
	os.WriteFile(evilFile, []byte("malicious"), 0644)

	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Create file",
			Outputs: map[string]types.AgentOutputDef{
				"file": {Required: true, Type: "file_path"},
			},
		},
	}

	outputs := map[string]any{
		"file": evilFile,
	}

	result, stepErr := CompleteAgentStep(step, outputs, workDir)
	if stepErr == nil {
		t.Fatal("expected validation error for path prefix attack")
	}

	if !strings.Contains(result.ValidationErrors[0], "within agent workdir") {
		t.Errorf("expected workdir error for prefix attack, got %q", result.ValidationErrors[0])
	}
}

func TestCompleteAgentStep_OptionalOutputMissing(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Do something",
			Outputs: map[string]types.AgentOutputDef{
				"optional_field": {Required: false, Type: "string"},
			},
		},
	}

	outputs := map[string]any{
		// Missing optional field - should be OK
	}

	result, stepErr := CompleteAgentStep(step, outputs, "")
	if stepErr != nil {
		t.Fatalf("optional missing output should not error: %v", stepErr)
	}

	if len(result.ValidationErrors) != 0 {
		t.Errorf("expected no validation errors for optional field, got %v", result.ValidationErrors)
	}
}

func TestGetPromptForStopHook_Autonomous(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusRunning,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Do the work",
			Mode:   "autonomous",
		},
	}

	prompt := GetPromptForStopHook(step)
	if prompt == "" {
		t.Error("expected prompt for autonomous mode")
	}
	if !strings.Contains(prompt, "Do the work") {
		t.Errorf("expected prompt to contain original instructions, got %q", prompt)
	}
}

func TestGetPromptForStopHook_Interactive(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusRunning,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Do the work",
			Mode:   "interactive",
		},
	}

	prompt := GetPromptForStopHook(step)
	if prompt != "" {
		t.Errorf("expected empty prompt for interactive mode, got %q", prompt)
	}
}

func TestGetPromptForStopHook_Completing(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusCompleting,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Do the work",
			Mode:   "autonomous",
		},
	}

	prompt := GetPromptForStopHook(step)
	if prompt != "" {
		t.Errorf("expected empty prompt during completing, got %q", prompt)
	}
}

func TestGetPromptForStopHook_Done(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusDone,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Do the work",
		},
	}

	prompt := GetPromptForStopHook(step)
	if prompt != "" {
		t.Errorf("expected empty prompt when done, got %q", prompt)
	}
}

func TestGetPromptForStopHook_DefaultMode(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusRunning,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Do the work",
			Mode:   "", // Empty - should default to autonomous
		},
	}

	prompt := GetPromptForStopHook(step)
	if prompt == "" {
		t.Error("expected prompt for default (autonomous) mode")
	}
}

func TestGetPromptForStopHook_NilStep(t *testing.T) {
	prompt := GetPromptForStopHook(nil)
	if prompt != "" {
		t.Errorf("expected empty prompt for nil step, got %q", prompt)
	}
}

func TestGetPromptForStopHook_NilAgentConfig(t *testing.T) {
	step := &types.Step{
		ID:       "agent-step",
		Executor: types.ExecutorAgent,
		Status:   types.StepStatusRunning,
		Agent:    nil,
	}

	prompt := GetPromptForStopHook(step)
	if prompt != "" {
		t.Errorf("expected empty prompt for nil agent config, got %q", prompt)
	}
}

func TestValidateAgentOutputs_AllTypes(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)

	defs := map[string]types.AgentOutputDef{
		"str":  {Required: true, Type: "string"},
		"num":  {Required: true, Type: "number"},
		"bool": {Required: true, Type: "boolean"},
		"json": {Required: true, Type: "json"},
		"file": {Required: true, Type: "file_path"},
	}

	outputs := map[string]any{
		"str":  "hello",
		"num":  42,
		"bool": true,
		"json": map[string]any{"key": "value"},
		"file": testFile,
	}

	errs := ValidateAgentOutputs(outputs, defs, tmpDir)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateAgentOutputs_NumberTypes(t *testing.T) {
	defs := map[string]types.AgentOutputDef{
		"int_val":   {Required: true, Type: "number"},
		"float_val": {Required: true, Type: "number"},
	}

	outputs := map[string]any{
		"int_val":   int64(42),
		"float_val": 3.14,
	}

	errs := ValidateAgentOutputs(outputs, defs, "")
	if len(errs) != 0 {
		t.Errorf("expected no errors for numeric types, got %v", errs)
	}
}

func TestValidateAgentOutputs_JsonArray(t *testing.T) {
	defs := map[string]types.AgentOutputDef{
		"arr": {Required: true, Type: "json"},
	}

	outputs := map[string]any{
		"arr": []any{"a", "b", "c"},
	}

	errs := ValidateAgentOutputs(outputs, defs, "")
	if len(errs) != 0 {
		t.Errorf("expected no errors for json array, got %v", errs)
	}
}

func TestParseAgentMode(t *testing.T) {
	tests := []struct {
		input    string
		expected AgentMode
	}{
		{"autonomous", AgentModeAutonomous},
		{"AUTONOMOUS", AgentModeAutonomous},
		{"interactive", AgentModeInteractive},
		{"INTERACTIVE", AgentModeInteractive},
		{"Interactive", AgentModeInteractive},
		{"", AgentModeAutonomous},
		{"unknown", AgentModeAutonomous},
	}

	for _, tc := range tests {
		result := ParseAgentMode(tc.input)
		if result != tc.expected {
			t.Errorf("ParseAgentMode(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}
