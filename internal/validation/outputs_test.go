package validation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meow-stack/meow-machine/internal/types"
)

// mockBeadChecker for testing bead_id validation.
type mockBeadChecker struct {
	beads map[string]bool
}

func (m *mockBeadChecker) BeadExists(id string) bool {
	return m.beads[id]
}

func TestValidateOutputs_NoSpec(t *testing.T) {
	provided := map[string]string{"foo": "bar", "baz": "qux"}
	result, err := ValidateOutputs("bd-test", nil, provided, nil)

	if err != nil {
		t.Errorf("ValidateOutputs() error = %v, want nil", err)
	}
	if result.Values["foo"] != "bar" {
		t.Errorf("Values[foo] = %v, want bar", result.Values["foo"])
	}
	if result.Values["baz"] != "qux" {
		t.Errorf("Values[baz] = %v, want qux", result.Values["baz"])
	}
}

func TestValidateOutputs_RequiredMissing(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "work_bead", Type: types.TaskOutputTypeString},
			{Name: "notes", Type: types.TaskOutputTypeString},
		},
	}

	provided := map[string]string{"notes": "some notes"}
	_, err := ValidateOutputs("bd-test", spec, provided, nil)

	if err == nil {
		t.Error("ValidateOutputs() should fail with missing required output")
	}

	valErr, ok := err.(*OutputValidationError)
	if !ok {
		t.Fatalf("Expected OutputValidationError, got %T", err)
	}

	if len(valErr.Missing) != 1 || valErr.Missing[0] != "work_bead" {
		t.Errorf("Missing = %v, want [work_bead]", valErr.Missing)
	}
}

func TestValidateOutputs_TypeValidation_String(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "message", Type: types.TaskOutputTypeString},
		},
	}

	provided := map[string]string{"message": "hello world"}
	result, err := ValidateOutputs("bd-test", spec, provided, nil)

	if err != nil {
		t.Errorf("ValidateOutputs() error = %v", err)
	}
	if result.Values["message"] != "hello world" {
		t.Errorf("Values[message] = %v, want 'hello world'", result.Values["message"])
	}
}

func TestValidateOutputs_TypeValidation_Number(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "count", Type: types.TaskOutputTypeNumber},
		},
	}

	tests := []struct {
		name     string
		value    string
		wantInt  bool
		expected any
	}{
		{"integer", "42", true, int64(42)},
		{"float", "3.14", false, 3.14},
		{"negative", "-10", true, int64(-10)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provided := map[string]string{"count": tt.value}
			result, err := ValidateOutputs("bd-test", spec, provided, nil)

			if err != nil {
				t.Errorf("ValidateOutputs() error = %v", err)
			}
			if result.Values["count"] != tt.expected {
				t.Errorf("Values[count] = %v (%T), want %v (%T)", result.Values["count"], result.Values["count"], tt.expected, tt.expected)
			}
		})
	}
}

func TestValidateOutputs_TypeValidation_InvalidNumber(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "count", Type: types.TaskOutputTypeNumber},
		},
	}

	provided := map[string]string{"count": "not-a-number"}
	_, err := ValidateOutputs("bd-test", spec, provided, nil)

	if err == nil {
		t.Error("ValidateOutputs() should fail with invalid number")
	}

	valErr, ok := err.(*OutputValidationError)
	if !ok {
		t.Fatalf("Expected OutputValidationError, got %T", err)
	}

	if _, exists := valErr.Invalid["count"]; !exists {
		t.Error("Expected 'count' in Invalid map")
	}
}

func TestValidateOutputs_TypeValidation_Boolean(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "flag", Type: types.TaskOutputTypeBool},
		},
	}

	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"false", false},
		{"yes", true},
		{"no", false},
		{"1", true},
		{"0", false},
		{"TRUE", true},
		{"FALSE", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			provided := map[string]string{"flag": tt.value}
			result, err := ValidateOutputs("bd-test", spec, provided, nil)

			if err != nil {
				t.Errorf("ValidateOutputs() error = %v", err)
			}
			if result.Values["flag"] != tt.expected {
				t.Errorf("Values[flag] = %v, want %v", result.Values["flag"], tt.expected)
			}
		})
	}
}

func TestValidateOutputs_TypeValidation_StringArray(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "tags", Type: types.TaskOutputTypeStringArr},
		},
	}

	tests := []struct {
		name     string
		value    string
		expected []string
	}{
		{"json array", `["foo","bar","baz"]`, []string{"foo", "bar", "baz"}},
		{"comma separated", "foo,bar,baz", []string{"foo", "bar", "baz"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provided := map[string]string{"tags": tt.value}
			result, err := ValidateOutputs("bd-test", spec, provided, nil)

			if err != nil {
				t.Errorf("ValidateOutputs() error = %v", err)
			}
			arr, ok := result.Values["tags"].([]string)
			if !ok {
				t.Fatalf("Values[tags] = %T, want []string", result.Values["tags"])
			}
			if len(arr) != len(tt.expected) {
				t.Errorf("len(tags) = %d, want %d", len(arr), len(tt.expected))
			}
			for i, v := range arr {
				if v != tt.expected[i] {
					t.Errorf("tags[%d] = %s, want %s", i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestValidateOutputs_TypeValidation_JSON(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "data", Type: types.TaskOutputTypeJSON},
		},
	}

	provided := map[string]string{"data": `{"key": "value", "count": 42}`}
	result, err := ValidateOutputs("bd-test", spec, provided, nil)

	if err != nil {
		t.Errorf("ValidateOutputs() error = %v", err)
	}

	obj, ok := result.Values["data"].(map[string]any)
	if !ok {
		t.Fatalf("Values[data] = %T, want map[string]any", result.Values["data"])
	}
	if obj["key"] != "value" {
		t.Errorf("data.key = %v, want 'value'", obj["key"])
	}
}

func TestValidateOutputs_TypeValidation_BeadID(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "selected", Type: types.TaskOutputTypeBeadID},
		},
	}

	checker := &mockBeadChecker{
		beads: map[string]bool{"bd-exists": true},
	}

	// Valid bead ID
	provided := map[string]string{"selected": "bd-exists"}
	result, err := ValidateOutputs("bd-test", spec, provided, checker)

	if err != nil {
		t.Errorf("ValidateOutputs() error = %v", err)
	}
	if result.Values["selected"] != "bd-exists" {
		t.Errorf("Values[selected] = %v, want 'bd-exists'", result.Values["selected"])
	}

	// Invalid bead ID
	provided = map[string]string{"selected": "bd-nonexistent"}
	_, err = ValidateOutputs("bd-test", spec, provided, checker)

	if err == nil {
		t.Error("ValidateOutputs() should fail with non-existent bead")
	}
}

func TestValidateOutputs_TypeValidation_FilePath(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "file", Type: types.TaskOutputTypeFilePath},
		},
	}

	// Create a temp file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(tmpFile, []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Valid file path
	provided := map[string]string{"file": tmpFile}
	result, err := ValidateOutputs("bd-test", spec, provided, nil)

	if err != nil {
		t.Errorf("ValidateOutputs() error = %v", err)
	}
	if result.Values["file"] != tmpFile {
		t.Errorf("Values[file] = %v, want %s", result.Values["file"], tmpFile)
	}

	// Non-existent file
	provided = map[string]string{"file": "/nonexistent/file.txt"}
	_, err = ValidateOutputs("bd-test", spec, provided, nil)

	if err == nil {
		t.Error("ValidateOutputs() should fail with non-existent file")
	}
}

func TestValidateOutputs_OptionalOutputs(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "required_field", Type: types.TaskOutputTypeString},
		},
		Optional: []types.TaskOutputDef{
			{Name: "optional_field", Type: types.TaskOutputTypeNumber},
		},
	}

	// Only required provided
	provided := map[string]string{"required_field": "value"}
	result, err := ValidateOutputs("bd-test", spec, provided, nil)

	if err != nil {
		t.Errorf("ValidateOutputs() error = %v", err)
	}
	if result.Values["required_field"] != "value" {
		t.Errorf("Values[required_field] = %v, want 'value'", result.Values["required_field"])
	}
	if _, exists := result.Values["optional_field"]; exists {
		t.Error("optional_field should not be in result when not provided")
	}

	// Both provided
	provided = map[string]string{
		"required_field": "value",
		"optional_field": "42",
	}
	result, err = ValidateOutputs("bd-test", spec, provided, nil)

	if err != nil {
		t.Errorf("ValidateOutputs() error = %v", err)
	}
	if result.Values["optional_field"] != int64(42) {
		t.Errorf("Values[optional_field] = %v, want 42", result.Values["optional_field"])
	}
}

func TestValidateOutputs_ExtraOutputs(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "known", Type: types.TaskOutputTypeString},
		},
	}

	// Provide extra outputs not in spec
	provided := map[string]string{
		"known":   "value",
		"unknown": "extra",
	}
	result, err := ValidateOutputs("bd-test", spec, provided, nil)

	if err != nil {
		t.Errorf("ValidateOutputs() error = %v", err)
	}
	if result.Values["unknown"] != "extra" {
		t.Errorf("Values[unknown] = %v, want 'extra'", result.Values["unknown"])
	}
}

func TestFormatUsage(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "work_bead", Type: types.TaskOutputTypeBeadID, Description: "The selected work item"},
			{Name: "rationale", Type: types.TaskOutputTypeString},
		},
		Optional: []types.TaskOutputDef{
			{Name: "notes", Type: types.TaskOutputTypeString, Description: "Additional notes"},
		},
	}

	usage := FormatUsage("bd-select-001", spec)

	// Check key parts are present
	if !containsSubstring(usage, "meow close bd-select-001") {
		t.Error("Usage should contain the close command")
	}
	if !containsSubstring(usage, "--output work_bead=<bead_id>") {
		t.Error("Usage should show work_bead output")
	}
	if !containsSubstring(usage, "Required outputs:") {
		t.Error("Usage should have Required outputs section")
	}
	if !containsSubstring(usage, "Optional outputs:") {
		t.Error("Usage should have Optional outputs section")
	}
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && (s[0:len(substr)] == substr || containsSubstring(s[1:], substr)))
}

func TestOutputValidationError_Error(t *testing.T) {
	err := &OutputValidationError{
		BeadID:  "bd-test",
		Missing: []string{"field1", "field2"},
		Invalid: map[string]string{"field3": "invalid value"},
	}

	msg := err.Error()

	if !containsSubstring(msg, "bd-test") {
		t.Error("Error message should contain bead ID")
	}
	if !containsSubstring(msg, "field1") {
		t.Error("Error message should list missing field1")
	}
	if !containsSubstring(msg, "field3") {
		t.Error("Error message should list invalid field3")
	}
}
