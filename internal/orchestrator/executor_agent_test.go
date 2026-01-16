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

	// With string coercion, the error message now says "cannot parse" instead of "expected number"
	if !strings.Contains(result.ValidationErrors[0], "cannot parse") || !strings.Contains(result.ValidationErrors[0], "as number") {
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
		{"fire_forget", AgentModeFireForget},
		{"FIRE_FORGET", AgentModeFireForget},
		{"Fire_Forget", AgentModeFireForget},
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

// --- Fire-and-Forget Mode Tests ---

func TestStartAgentStep_FireForgetBasic(t *testing.T) {
	step := &types.Step{
		ID:       "compact-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "/compact",
			Mode:   "fire_forget",
		},
	}

	result, stepErr := StartAgentStep(step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Fire-forget prompts should NOT contain meow done instructions
	if strings.Contains(result.Prompt, "meow done") {
		t.Errorf("fire_forget prompt should not mention 'meow done', got %q", result.Prompt)
	}

	// Should contain the original prompt
	if result.Prompt != "/compact" {
		t.Errorf("expected prompt to be just '/compact', got %q", result.Prompt)
	}
}

func TestStartAgentStep_FireForgetRejectsOutputs(t *testing.T) {
	step := &types.Step{
		ID:       "bad-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "/compact",
			Mode:   "fire_forget",
			Outputs: map[string]types.AgentOutputDef{
				"result": {Required: true, Type: "string"},
			},
		},
	}

	_, stepErr := StartAgentStep(step)
	if stepErr == nil {
		t.Fatal("expected error for fire_forget with outputs")
	}
	if stepErr.Message != "fire_forget mode cannot have outputs" {
		t.Errorf("unexpected error: %s", stepErr.Message)
	}
}

func TestStartAgentStep_FireForgetEscapeKey(t *testing.T) {
	step := &types.Step{
		ID:       "escape-step",
		Executor: types.ExecutorAgent,
		Agent: &types.AgentConfig{
			Agent:  "worker-1",
			Prompt: "Escape",
			Mode:   "fire_forget",
		},
	}

	result, stepErr := StartAgentStep(step)
	if stepErr != nil {
		t.Fatalf("unexpected error: %v", stepErr)
	}

	// Prompt should be just "Escape" without any meow done additions
	if result.Prompt != "Escape" {
		t.Errorf("expected prompt to be just 'Escape', got %q", result.Prompt)
	}
}

// --- String Type Coercion Tests ---
// These test that string values from `meow done --output key=value` can be coerced
// to the expected types (boolean, number, json) since CLI args are always strings.

func TestValidateAgentOutputs_StringToBooleanCoercion(t *testing.T) {
	defs := map[string]types.AgentOutputDef{
		"success": {Required: true, Type: "boolean"},
	}

	testCases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"true", "true", false},
		{"false", "false", false},
		{"TRUE", "TRUE", false},
		{"FALSE", "FALSE", false},
		{"True", "True", false},
		{"1", "1", false},
		{"0", "0", false},
		{"yes", "yes", false},
		{"no", "no", false},
		{"invalid", "maybe", true},
		{"empty", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			outputs := map[string]any{
				"success": tc.value,
			}
			errs := ValidateAgentOutputs(outputs, defs, "")
			if tc.wantErr && len(errs) == 0 {
				t.Errorf("expected error for value %q, got none", tc.value)
			}
			if !tc.wantErr && len(errs) > 0 {
				t.Errorf("unexpected error for value %q: %v", tc.value, errs)
			}
		})
	}
}

func TestValidateAgentOutputs_StringToNumberCoercion(t *testing.T) {
	defs := map[string]types.AgentOutputDef{
		"count": {Required: true, Type: "number"},
	}

	testCases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"integer", "42", false},
		{"negative", "-10", false},
		{"float", "3.14", false},
		{"negative_float", "-2.5", false},
		{"zero", "0", false},
		{"not_a_number", "abc", true},
		{"empty", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			outputs := map[string]any{
				"count": tc.value,
			}
			errs := ValidateAgentOutputs(outputs, defs, "")
			if tc.wantErr && len(errs) == 0 {
				t.Errorf("expected error for value %q, got none", tc.value)
			}
			if !tc.wantErr && len(errs) > 0 {
				t.Errorf("unexpected error for value %q: %v", tc.value, errs)
			}
		})
	}
}

func TestValidateAgentOutputs_StringToJSONCoercion(t *testing.T) {
	defs := map[string]types.AgentOutputDef{
		"data": {Required: true, Type: "json"},
	}

	testCases := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"object", `{"foo":"bar"}`, false},
		{"array", `[1,2,3]`, false},
		{"nested", `{"items":[1,2,3]}`, false},
		{"invalid_json", `{not valid}`, true},
		{"plain_string", `hello`, true},
		{"empty", "", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			outputs := map[string]any{
				"data": tc.value,
			}
			errs := ValidateAgentOutputs(outputs, defs, "")
			if tc.wantErr && len(errs) == 0 {
				t.Errorf("expected error for value %q, got none", tc.value)
			}
			if !tc.wantErr && len(errs) > 0 {
				t.Errorf("unexpected error for value %q: %v", tc.value, errs)
			}
		})
	}
}

func TestValidateAgentOutputs_NativeTypesStillWork(t *testing.T) {
	// Ensure that native Go types (from --output-json) still work
	defs := map[string]types.AgentOutputDef{
		"bool_val":   {Required: true, Type: "boolean"},
		"num_val":    {Required: true, Type: "number"},
		"json_val":   {Required: true, Type: "json"},
		"string_val": {Required: true, Type: "string"},
	}

	outputs := map[string]any{
		"bool_val":   true,                      // native bool
		"num_val":    42,                        // native int
		"json_val":   map[string]any{"a": "b"}, // native map
		"string_val": "hello",                   // native string
	}

	errs := ValidateAgentOutputs(outputs, defs, "")
	if len(errs) != 0 {
		t.Errorf("expected no errors for native types, got %v", errs)
	}
}

func TestIsFireForget(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *types.AgentConfig
		expected bool
	}{
		{
			name:     "nil config",
			cfg:      nil,
			expected: false,
		},
		{
			name: "fire_forget mode",
			cfg: &types.AgentConfig{
				Agent:  "worker",
				Prompt: "/compact",
				Mode:   "fire_forget",
			},
			expected: true,
		},
		{
			name: "autonomous mode",
			cfg: &types.AgentConfig{
				Agent:  "worker",
				Prompt: "Do work",
				Mode:   "autonomous",
			},
			expected: false,
		},
		{
			name: "interactive mode",
			cfg: &types.AgentConfig{
				Agent:  "worker",
				Prompt: "Do work",
				Mode:   "interactive",
			},
			expected: false,
		},
		{
			name: "empty mode (defaults to autonomous)",
			cfg: &types.AgentConfig{
				Agent:  "worker",
				Prompt: "Do work",
				Mode:   "",
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := IsFireForget(tc.cfg)
			if result != tc.expected {
				t.Errorf("IsFireForget() = %v, expected %v", result, tc.expected)
			}
		})
	}
}
