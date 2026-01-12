package testutil

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// AssertEqual asserts that two values are equal.
func AssertEqual(t *testing.T, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		msg := formatMessage("Expected values to be equal", msgAndArgs...)
		t.Errorf("%s\nExpected: %v\nActual: %v", msg, expected, actual)
	}
}

// AssertNotEqual asserts that two values are not equal.
func AssertNotEqual(t *testing.T, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	if reflect.DeepEqual(expected, actual) {
		msg := formatMessage("Expected values to be different", msgAndArgs...)
		t.Errorf("%s\nBoth values: %v", msg, actual)
	}
}

// AssertNil asserts that a value is nil.
func AssertNil(t *testing.T, value any, msgAndArgs ...any) {
	t.Helper()
	if !isNil(value) {
		msg := formatMessage("Expected nil", msgAndArgs...)
		t.Errorf("%s\nActual: %v", msg, value)
	}
}

// AssertNotNil asserts that a value is not nil.
func AssertNotNil(t *testing.T, value any, msgAndArgs ...any) {
	t.Helper()
	if isNil(value) {
		msg := formatMessage("Expected non-nil value", msgAndArgs...)
		t.Errorf("%s", msg)
	}
}

// AssertError asserts that an error is not nil.
func AssertError(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err == nil {
		msg := formatMessage("Expected an error", msgAndArgs...)
		t.Errorf("%s", msg)
	}
}

// AssertNoError asserts that an error is nil.
func AssertNoError(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err != nil {
		msg := formatMessage("Expected no error", msgAndArgs...)
		t.Errorf("%s\nError: %v", msg, err)
	}
}

// AssertErrorContains asserts that an error contains a substring.
func AssertErrorContains(t *testing.T, err error, substring string, msgAndArgs ...any) {
	t.Helper()
	if err == nil {
		msg := formatMessage("Expected an error containing "+substring, msgAndArgs...)
		t.Errorf("%s\nGot: nil", msg)
		return
	}
	if !strings.Contains(err.Error(), substring) {
		msg := formatMessage("Expected error to contain substring", msgAndArgs...)
		t.Errorf("%s\nSubstring: %q\nError: %v", msg, substring, err)
	}
}

// AssertTrue asserts that a value is true.
func AssertTrue(t *testing.T, value bool, msgAndArgs ...any) {
	t.Helper()
	if !value {
		msg := formatMessage("Expected true", msgAndArgs...)
		t.Errorf("%s", msg)
	}
}

// AssertFalse asserts that a value is false.
func AssertFalse(t *testing.T, value bool, msgAndArgs ...any) {
	t.Helper()
	if value {
		msg := formatMessage("Expected false", msgAndArgs...)
		t.Errorf("%s", msg)
	}
}

// AssertContains asserts that a string contains a substring.
func AssertContains(t *testing.T, s, substring string, msgAndArgs ...any) {
	t.Helper()
	if !strings.Contains(s, substring) {
		msg := formatMessage("Expected string to contain substring", msgAndArgs...)
		t.Errorf("%s\nString: %q\nSubstring: %q", msg, s, substring)
	}
}

// AssertNotContains asserts that a string does not contain a substring.
func AssertNotContains(t *testing.T, s, substring string, msgAndArgs ...any) {
	t.Helper()
	if strings.Contains(s, substring) {
		msg := formatMessage("Expected string to not contain substring", msgAndArgs...)
		t.Errorf("%s\nString: %q\nSubstring: %q", msg, s, substring)
	}
}

// AssertLen asserts that a collection has the expected length.
func AssertLen(t *testing.T, collection any, expectedLen int, msgAndArgs ...any) {
	t.Helper()
	actualLen := reflect.ValueOf(collection).Len()
	if actualLen != expectedLen {
		msg := formatMessage("Expected length mismatch", msgAndArgs...)
		t.Errorf("%s\nExpected: %d\nActual: %d", msg, expectedLen, actualLen)
	}
}

// AssertEmpty asserts that a collection is empty.
func AssertEmpty(t *testing.T, collection any, msgAndArgs ...any) {
	t.Helper()
	v := reflect.ValueOf(collection)
	if v.Len() != 0 {
		msg := formatMessage("Expected empty collection", msgAndArgs...)
		t.Errorf("%s\nLength: %d", msg, v.Len())
	}
}

// AssertNotEmpty asserts that a collection is not empty.
func AssertNotEmpty(t *testing.T, collection any, msgAndArgs ...any) {
	t.Helper()
	v := reflect.ValueOf(collection)
	if v.Len() == 0 {
		msg := formatMessage("Expected non-empty collection", msgAndArgs...)
		t.Errorf("%s", msg)
	}
}

// Agent-specific assertions

// AssertAgentActive asserts that an agent is active.
func AssertAgentActive(t *testing.T, agent *types.Agent) {
	t.Helper()
	if agent.Status != types.AgentStatusActive {
		t.Errorf("Expected agent %s to be active, got %s", agent.ID, agent.Status)
	}
}

// AssertAgentStopped asserts that an agent is stopped.
func AssertAgentStopped(t *testing.T, agent *types.Agent) {
	t.Helper()
	if agent.Status != types.AgentStatusStopped {
		t.Errorf("Expected agent %s to be stopped, got %s", agent.ID, agent.Status)
	}
	if agent.StoppedAt == nil {
		t.Errorf("Expected agent %s to have StoppedAt set", agent.ID)
	}
}

// AssertAgentValid asserts that an agent passes validation.
func AssertAgentValid(t *testing.T, agent *types.Agent) {
	t.Helper()
	if err := agent.Validate(); err != nil {
		t.Errorf("Expected agent %s to be valid, got error: %v", agent.ID, err)
	}
}

// File-related assertions

// AssertFileExists asserts that a file exists.
func AssertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("Expected file %s to exist", path)
	}
}

// AssertFileNotExists asserts that a file does not exist.
func AssertFileNotExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("Expected file %s to not exist", path)
	}
}

// AssertFileContains asserts that a file contains a substring.
func AssertFileContains(t *testing.T, path, substring string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("Failed to read file %s: %v", path, err)
		return
	}
	if !strings.Contains(string(content), substring) {
		t.Errorf("Expected file %s to contain %q", path, substring)
	}
}

// AssertDirExists asserts that a directory exists.
func AssertDirExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		t.Errorf("Expected directory %s to exist", path)
		return
	}
	if !info.IsDir() {
		t.Errorf("Expected %s to be a directory", path)
	}
}

// JSON assertions

// AssertJSONEqual asserts that two JSON strings are equal (ignoring formatting).
func AssertJSONEqual(t *testing.T, expected, actual string) {
	t.Helper()
	var expectedMap, actualMap any
	if err := json.Unmarshal([]byte(expected), &expectedMap); err != nil {
		t.Errorf("Failed to parse expected JSON: %v", err)
		return
	}
	if err := json.Unmarshal([]byte(actual), &actualMap); err != nil {
		t.Errorf("Failed to parse actual JSON: %v", err)
		return
	}
	if !reflect.DeepEqual(expectedMap, actualMap) {
		t.Errorf("JSON not equal\nExpected: %s\nActual: %s", expected, actual)
	}
}

// AssertJSONContainsKey asserts that a JSON object contains a key.
func AssertJSONContainsKey(t *testing.T, jsonStr, key string) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &m); err != nil {
		t.Errorf("Failed to parse JSON: %v", err)
		return
	}
	if _, exists := m[key]; !exists {
		t.Errorf("Expected JSON to contain key %q", key)
	}
}

// Time assertions

// AssertTimeWithin asserts that a time is within a duration of another time.
func AssertTimeWithin(t *testing.T, actual, expected time.Time, tolerance time.Duration) {
	t.Helper()
	diff := actual.Sub(expected)
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Errorf("Expected time to be within %v of %v, got %v (diff: %v)",
			tolerance, expected, actual, diff)
	}
}

// AssertTimeAfter asserts that actual time is after expected time.
func AssertTimeAfter(t *testing.T, actual, expected time.Time) {
	t.Helper()
	if !actual.After(expected) {
		t.Errorf("Expected time %v to be after %v", actual, expected)
	}
}

// AssertTimeBefore asserts that actual time is before expected time.
func AssertTimeBefore(t *testing.T, actual, expected time.Time) {
	t.Helper()
	if !actual.Before(expected) {
		t.Errorf("Expected time %v to be before %v", actual, expected)
	}
}

// Helper functions

// formatMessage formats an error message with optional additional context.
func formatMessage(defaultMsg string, msgAndArgs ...any) string {
	if len(msgAndArgs) == 0 {
		return defaultMsg
	}
	if len(msgAndArgs) == 1 {
		return msgAndArgs[0].(string)
	}
	return msgAndArgs[0].(string) + ": " + strings.TrimSpace(
		strings.Repeat("%v ", len(msgAndArgs)-1))
}

// isNil checks if a value is nil, handling interface nil properly.
func isNil(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	}
	return false
}

// RequireEqual is like AssertEqual but fails the test immediately.
func RequireEqual(t *testing.T, expected, actual any, msgAndArgs ...any) {
	t.Helper()
	if !reflect.DeepEqual(expected, actual) {
		msg := formatMessage("Expected values to be equal", msgAndArgs...)
		t.Fatalf("%s\nExpected: %v\nActual: %v", msg, expected, actual)
	}
}

// RequireNoError is like AssertNoError but fails the test immediately.
func RequireNoError(t *testing.T, err error, msgAndArgs ...any) {
	t.Helper()
	if err != nil {
		msg := formatMessage("Expected no error", msgAndArgs...)
		t.Fatalf("%s\nError: %v", msg, err)
	}
}

// RequireNotNil is like AssertNotNil but fails the test immediately.
func RequireNotNil(t *testing.T, value any, msgAndArgs ...any) {
	t.Helper()
	if isNil(value) {
		msg := formatMessage("Expected non-nil value", msgAndArgs...)
		t.Fatalf("%s", msg)
	}
}

// RequireFileExists is like AssertFileExists but fails the test immediately.
func RequireFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatalf("Required file %s does not exist", path)
	}
}

// RequireWorkspace creates a test workspace and fails immediately if it can't.
func RequireWorkspace(t *testing.T) string {
	t.Helper()
	workspace, _ := NewTestWorkspace(t)
	return workspace
}

// RequireFile creates a file with content and fails immediately if it can't.
func RequireFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("Failed to create directory for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write file %s: %v", path, err)
	}
}
