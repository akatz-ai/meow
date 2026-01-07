package errors

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func TestMeowError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *MeowError
		wantStr  string
	}{
		{
			name: "simple error",
			err: &MeowError{
				Code:    "TEST_001",
				Message: "test error",
			},
			wantStr: "[TEST_001] test error",
		},
		{
			name: "error with cause",
			err: &MeowError{
				Code:    "TEST_002",
				Message: "wrapped error",
				Cause:   errors.New("underlying"),
			},
			wantStr: "[TEST_002] wrapped error: underlying",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.wantStr {
				t.Errorf("Error() = %q, want %q", got, tt.wantStr)
			}
		})
	}
}

func TestMeowError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	err := &MeowError{
		Code:    "TEST_001",
		Message: "test",
		Cause:   underlying,
	}

	if got := err.Unwrap(); got != underlying {
		t.Errorf("Unwrap() = %v, want %v", got, underlying)
	}
}

func TestMeowError_WithDetail(t *testing.T) {
	err := New("TEST_001", "test").
		WithDetail("key1", "value1").
		WithDetail("key2", 42)

	if err.Details["key1"] != "value1" {
		t.Errorf("Details[key1] = %v, want value1", err.Details["key1"])
	}
	if err.Details["key2"] != 42 {
		t.Errorf("Details[key2] = %v, want 42", err.Details["key2"])
	}
}

func TestMeowError_WithCause(t *testing.T) {
	cause := errors.New("cause")
	err := New("TEST_001", "test").WithCause(cause)

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
}

func TestMeowError_MarshalJSON(t *testing.T) {
	err := &MeowError{
		Code:    "TEST_001",
		Message: "test error",
		Details: map[string]any{"bead_id": "bd-123"},
		Cause:   errors.New("underlying"),
	}

	data, jsonErr := json.Marshal(err)
	if jsonErr != nil {
		t.Fatalf("Marshal failed: %v", jsonErr)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal(data, &result); jsonErr != nil {
		t.Fatalf("Unmarshal failed: %v", jsonErr)
	}

	if result["code"] != "TEST_001" {
		t.Errorf("code = %v, want TEST_001", result["code"])
	}
	if result["message"] != "test error" {
		t.Errorf("message = %v, want test error", result["message"])
	}
	if result["cause"] != "underlying" {
		t.Errorf("cause = %v, want underlying", result["cause"])
	}
	details, ok := result["details"].(map[string]any)
	if !ok {
		t.Fatalf("details not a map")
	}
	if details["bead_id"] != "bd-123" {
		t.Errorf("details.bead_id = %v, want bd-123", details["bead_id"])
	}
}

func TestNew(t *testing.T) {
	err := New("CODE_001", "message")
	if err.Code != "CODE_001" {
		t.Errorf("Code = %s, want CODE_001", err.Code)
	}
	if err.Message != "message" {
		t.Errorf("Message = %s, want message", err.Message)
	}
}

func TestNewf(t *testing.T) {
	err := Newf("CODE_001", "value is %d", 42)
	if err.Message != "value is 42" {
		t.Errorf("Message = %s, want 'value is 42'", err.Message)
	}
}

func TestWrap(t *testing.T) {
	cause := errors.New("original")
	err := Wrap("CODE_001", "wrapped", cause)

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
	if err.Message != "wrapped" {
		t.Errorf("Message = %s, want wrapped", err.Message)
	}
}

func TestWrapf(t *testing.T) {
	cause := errors.New("original")
	err := Wrapf("CODE_001", cause, "wrapped %s", "value")

	if err.Cause != cause {
		t.Errorf("Cause = %v, want %v", err.Cause, cause)
	}
	if err.Message != "wrapped value" {
		t.Errorf("Message = %s, want 'wrapped value'", err.Message)
	}
}

func TestHasCode(t *testing.T) {
	err := New("TEST_001", "test")
	if !HasCode(err, "TEST_001") {
		t.Error("HasCode(err, TEST_001) = false, want true")
	}
	if HasCode(err, "TEST_002") {
		t.Error("HasCode(err, TEST_002) = true, want false")
	}
	if HasCode(errors.New("not meow"), "TEST_001") {
		t.Error("HasCode(regular error) = true, want false")
	}

	// Test wrapped error
	wrapped := fmt.Errorf("outer: %w", err)
	if !HasCode(wrapped, "TEST_001") {
		t.Error("HasCode should find code in wrapped error")
	}
}

func TestCode(t *testing.T) {
	err := New("TEST_001", "test")
	if got := Code(err); got != "TEST_001" {
		t.Errorf("Code() = %s, want TEST_001", got)
	}
	if got := Code(errors.New("regular")); got != "" {
		t.Errorf("Code(regular) = %s, want empty", got)
	}

	// Test wrapped error
	wrapped := fmt.Errorf("outer: %w", err)
	if got := Code(wrapped); got != "TEST_001" {
		t.Errorf("Code(wrapped) = %s, want TEST_001", got)
	}
}

// Test factory functions produce correct codes
func TestFactoryFunctions(t *testing.T) {
	tests := []struct {
		name     string
		err      *MeowError
		wantCode string
	}{
		{"ConfigMissingField", ConfigMissingField("field"), CodeConfigMissingField},
		{"ConfigInvalidValue", ConfigInvalidValue("field", "val", "reason"), CodeConfigInvalidValue},
		{"TemplateParseError", TemplateParseError("tmpl", errors.New("err")), CodeTemplateParseError},
		{"TemplateMissingField", TemplateMissingField("tmpl", "field"), CodeTemplateMissingField},
		{"TemplateCycleDetected", TemplateCycleDetected("tmpl", []string{"a", "b"}), CodeTemplateCycleDetected},
		{"TemplateNotFound", TemplateNotFound("tmpl"), CodeTemplateNotFound},
		{"TemplateInvalidType", TemplateInvalidType("tmpl", "bad"), CodeTemplateInvalidType},
		{"BeadInvalidTransition", BeadInvalidTransition("bd-1", "open", "closed"), CodeBeadInvalidTransition},
		{"BeadDependencyNotSatisfied", BeadDependencyNotSatisfied("bd-1", "bd-2"), CodeBeadDepNotSatisfied},
		{"BeadNotFound", BeadNotFound("bd-1"), CodeBeadNotFound},
		{"BeadCycleDetected", BeadCycleDetected([]string{"a", "b"}), CodeBeadCycleDetected},
		{"AgentSpawnFailed", AgentSpawnFailed("agent", errors.New("err")), CodeAgentSpawnFailed},
		{"AgentNotFound", AgentNotFound("agent"), CodeAgentNotFound},
		{"AgentTimeout", AgentTimeout("agent", "spawn"), CodeAgentTimeout},
		{"AgentAlreadyExists", AgentAlreadyExists("agent"), CodeAgentAlreadyExists},
		{"OutputMissing", OutputMissing("bd-1", "out"), CodeOutputMissing},
		{"OutputTypeMismatch", OutputTypeMismatch("bd-1", "out", "string", "int"), CodeOutputTypeMismatch},
		{"OutputBeadNotFound", OutputBeadNotFound("bd-1", "out"), CodeOutputBeadNotFound},
		{"IOFileNotFound", IOFileNotFound("/path"), CodeIOFileNotFound},
		{"IOPermissionDenied", IOPermissionDenied("/path", errors.New("err")), CodeIOPermission},
		{"IODiskFull", IODiskFull("/path", errors.New("err")), CodeIODiskFull},
		{"IOReadError", IOReadError("/path", errors.New("err")), CodeIOReadError},
		{"IOWriteError", IOWriteError("/path", errors.New("err")), CodeIOWriteError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Code != tt.wantCode {
				t.Errorf("%s Code = %s, want %s", tt.name, tt.err.Code, tt.wantCode)
			}
			// Verify error string is non-empty
			if tt.err.Error() == "" {
				t.Errorf("%s Error() is empty", tt.name)
			}
		})
	}
}

func TestErrorsUnwrapChain(t *testing.T) {
	root := errors.New("root cause")
	wrapped := Wrap("WRAP_001", "wrapped", root)

	// Test errors.Is works through the chain
	if !errors.Is(wrapped, root) {
		t.Error("errors.Is should find root cause")
	}
}
