package validation

import (
	"os"
	"path/filepath"
	"strings"
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

func (m *mockBeadChecker) ListAllIDs() []string {
	ids := make([]string, 0, len(m.beads))
	for id := range m.beads {
		ids = append(ids, id)
	}
	return ids
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
	return strings.Contains(s, substr)
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

func TestBeadNotFoundError_Error(t *testing.T) {
	tests := []struct {
		name        string
		err         *BeadNotFoundError
		wantID      bool
		wantSuggest bool
		wantHint    bool
	}{
		{
			name:        "with suggestions",
			err:         &BeadNotFoundError{ID: "bd-typo", Suggestions: []string{"bd-task-001", "bd-task-002"}},
			wantID:      true,
			wantSuggest: true,
			wantHint:    true,
		},
		{
			name:        "without suggestions",
			err:         &BeadNotFoundError{ID: "bd-unknown", Suggestions: nil},
			wantID:      true,
			wantSuggest: false,
			wantHint:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()

			if tt.wantID && !containsSubstring(msg, tt.err.ID) {
				t.Errorf("Error message should contain bead ID %s", tt.err.ID)
			}
			if tt.wantSuggest && !containsSubstring(msg, "Did you mean") {
				t.Error("Error message should contain 'Did you mean'")
			}
			if tt.wantSuggest {
				for _, s := range tt.err.Suggestions {
					if !containsSubstring(msg, s) {
						t.Errorf("Error message should contain suggestion %s", s)
					}
				}
			}
			if tt.wantHint && !containsSubstring(msg, "bd list") {
				t.Error("Error message should contain hint about 'bd list'")
			}
		})
	}
}

func TestValidateBeadID_Format(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "selected", Type: types.TaskOutputTypeBeadID},
		},
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
		errMsg  string
	}{
		{"valid bd-xxx", "bd-task-001", false, ""},
		{"valid meow-xxx", "meow-e7.4", false, ""},
		{"valid task-xxx", "task-123", false, ""},
		{"invalid no hyphen", "bdtask001", true, "invalid bead ID format"},
		{"invalid starts with hyphen", "-bd-001", true, "invalid bead ID format"},
		{"invalid ends with hyphen", "bd-", true, "invalid bead ID format"},
		{"invalid special chars", "bd$task-001", true, "invalid bead ID format"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provided := map[string]string{"selected": tt.value}
			_, err := ValidateOutputs("bd-test", spec, provided, nil)

			if tt.wantErr {
				if err == nil {
					t.Error("Expected error but got nil")
				} else if tt.errMsg != "" && !containsSubstring(err.Error(), tt.errMsg) {
					t.Errorf("Error should contain '%s', got: %s", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestValidateBeadID_Suggestions(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "selected", Type: types.TaskOutputTypeBeadID},
		},
	}

	checker := &mockBeadChecker{
		beads: map[string]bool{
			"bd-task-001": true,
			"bd-task-002": true,
			"bd-task-003": true,
			"bd-feature-x": true,
		},
	}

	// Typo in bead ID
	provided := map[string]string{"selected": "bd-task-001x"}
	_, err := ValidateOutputs("bd-test", spec, provided, checker)

	if err == nil {
		t.Fatal("Expected error for non-existent bead")
	}

	beadErr, ok := err.(*OutputValidationError)
	if !ok {
		t.Fatalf("Expected OutputValidationError, got %T: %v", err, err)
	}

	// The invalid map should contain the error
	errMsg, exists := beadErr.Invalid["selected"]
	if !exists {
		t.Fatal("Expected 'selected' in Invalid map")
	}

	// Error should mention the typo
	if !containsSubstring(errMsg, "bd-task-001x") {
		t.Errorf("Error should mention the invalid ID, got: %s", errMsg)
	}

	// Error should suggest similar beads
	if !containsSubstring(errMsg, "bd-task-001") {
		t.Errorf("Error should suggest bd-task-001, got: %s", errMsg)
	}
}

func TestValidateBeadIDArr_Validation(t *testing.T) {
	spec := &types.TaskOutputSpec{
		Required: []types.TaskOutputDef{
			{Name: "selected", Type: types.TaskOutputTypeBeadIDArr},
		},
	}

	checker := &mockBeadChecker{
		beads: map[string]bool{
			"bd-001": true,
			"bd-002": true,
		},
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"valid json array", `["bd-001","bd-002"]`, false},
		{"valid comma separated", "bd-001,bd-002", false},
		{"one invalid in array", `["bd-001","bd-999"]`, true},
		{"all valid", "bd-001, bd-002", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provided := map[string]string{"selected": tt.value}
			_, err := ValidateOutputs("bd-test", spec, provided, checker)

			if tt.wantErr && err == nil {
				t.Error("Expected error but got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestLevenshteinDistance(t *testing.T) {
	tests := []struct {
		s1, s2 string
		want   int
	}{
		{"", "", 0},
		{"abc", "", 3},
		{"", "abc", 3},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "abcd", 1},
		{"abc", "ab", 1},
		{"bd-task-001", "bd-task-002", 1},
		{"bd-task-001", "bd-task-001x", 1},
		{"kitten", "sitting", 3},
	}

	for _, tt := range tests {
		t.Run(tt.s1+"_"+tt.s2, func(t *testing.T) {
			got := levenshteinDistance(tt.s1, tt.s2)
			if got != tt.want {
				t.Errorf("levenshteinDistance(%q, %q) = %d, want %d", tt.s1, tt.s2, got, tt.want)
			}
		})
	}
}

func TestIsValidBeadIDFormat(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"bd-001", true},
		{"meow-e7.4", true},
		{"task-abc", true},
		{"CODE-123", true},
		{"bd2-test", true},
		{"", false},
		{"bd", false},
		{"bd-", false},
		{"-bd", false},
		{"bd--001", true}, // This is valid: prefix "bd" and suffix "-001"
		{"123-abc", true},
		{"bd$-001", false},
		{"bd -001", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := isValidBeadIDFormat(tt.id)
			if got != tt.want {
				t.Errorf("isValidBeadIDFormat(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}
