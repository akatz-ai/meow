// Package errors provides structured error types for MEOW.
package errors

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Error codes for MEOW operations.
const (
	// Config errors
	CodeConfigMissingField = "CONFIG_001" // Missing required field
	CodeConfigInvalidValue = "CONFIG_002" // Invalid value type

	// Template errors
	CodeTemplateParseError     = "TMPL_001" // Parse error
	CodeTemplateMissingField   = "TMPL_002" // Validation error - missing field
	CodeTemplateCycleDetected  = "TMPL_003" // Validation error - cycle detected
	CodeTemplateNotFound       = "TMPL_004" // Template not found
	CodeTemplateInvalidType    = "TMPL_005" // Invalid bead type in template

	// Bead errors
	CodeBeadInvalidTransition = "BEAD_001" // Invalid status transition
	CodeBeadDepNotSatisfied   = "BEAD_002" // Dependency not satisfied
	CodeBeadNotFound          = "BEAD_003" // Bead not found
	CodeBeadCycleDetected     = "BEAD_004" // Cycle detected in dependencies

	// Agent errors
	CodeAgentSpawnFailed   = "AGENT_001" // Session spawn failed
	CodeAgentNotFound      = "AGENT_002" // Session not found
	CodeAgentTimeout       = "AGENT_003" // Agent operation timeout
	CodeAgentAlreadyExists = "AGENT_004" // Agent session already exists

	// Output errors
	CodeOutputMissing       = "OUTPUT_001" // Missing required output
	CodeOutputTypeMismatch  = "OUTPUT_002" // Type validation failed
	CodeOutputBeadNotFound  = "OUTPUT_003" // Referenced bead not found

	// IO errors
	CodeIOFileNotFound   = "IO_001" // File not found
	CodeIOPermission     = "IO_002" // Permission denied
	CodeIODiskFull       = "IO_003" // Disk full
	CodeIOReadError      = "IO_004" // Read error
	CodeIOWriteError     = "IO_005" // Write error
)

// MeowError is the structured error type for MEOW operations.
type MeowError struct {
	Code    string         `json:"code"`              // Error code (e.g., "TMPL_001")
	Message string         `json:"message"`           // Human-readable message
	Details map[string]any `json:"details,omitempty"` // Context (bead_id, template, etc.)
	Cause   error          `json:"-"`                 // Wrapped error (not serialized)
}

// Error implements the error interface.
func (e *MeowError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *MeowError) Unwrap() error {
	return e.Cause
}

// WithDetail adds a detail to the error.
func (e *MeowError) WithDetail(key string, value any) *MeowError {
	if e.Details == nil {
		e.Details = make(map[string]any)
	}
	e.Details[key] = value
	return e
}

// WithCause wraps an underlying error.
func (e *MeowError) WithCause(err error) *MeowError {
	e.Cause = err
	return e
}

// MarshalJSON implements json.Marshaler with cause error message.
func (e *MeowError) MarshalJSON() ([]byte, error) {
	type alias MeowError
	aux := struct {
		*alias
		CauseMsg string `json:"cause,omitempty"`
	}{
		alias: (*alias)(e),
	}
	if e.Cause != nil {
		aux.CauseMsg = e.Cause.Error()
	}
	return json.Marshal(aux)
}

// New creates a new MeowError.
func New(code, message string) *MeowError {
	return &MeowError{
		Code:    code,
		Message: message,
	}
}

// Newf creates a new MeowError with formatted message.
func Newf(code, format string, args ...any) *MeowError {
	return &MeowError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}

// Wrap wraps an error with a MeowError.
func Wrap(code, message string, err error) *MeowError {
	return &MeowError{
		Code:    code,
		Message: message,
		Cause:   err,
	}
}

// Wrapf wraps an error with a formatted MeowError.
func Wrapf(code string, err error, format string, args ...any) *MeowError {
	return &MeowError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
		Cause:   err,
	}
}

// --- Config Errors ---

// ConfigMissingField creates an error for missing config field.
func ConfigMissingField(field string) *MeowError {
	return Newf(CodeConfigMissingField, "missing required config field: %s", field).
		WithDetail("field", field)
}

// ConfigInvalidValue creates an error for invalid config value.
func ConfigInvalidValue(field string, value any, reason string) *MeowError {
	return Newf(CodeConfigInvalidValue, "invalid config value for %s: %s", field, reason).
		WithDetail("field", field).
		WithDetail("value", value).
		WithDetail("reason", reason)
}

// --- Template Errors ---

// TemplateParseError creates an error for template parsing failure.
func TemplateParseError(template string, err error) *MeowError {
	return Wrap(CodeTemplateParseError, "failed to parse template", err).
		WithDetail("template", template)
}

// TemplateMissingField creates an error for missing template field.
func TemplateMissingField(template, field string) *MeowError {
	return Newf(CodeTemplateMissingField, "template %s missing required field: %s", template, field).
		WithDetail("template", template).
		WithDetail("field", field)
}

// TemplateCycleDetected creates an error for cycle in template.
func TemplateCycleDetected(template string, cycle []string) *MeowError {
	return Newf(CodeTemplateCycleDetected, "cycle detected in template %s", template).
		WithDetail("template", template).
		WithDetail("cycle", cycle)
}

// TemplateNotFound creates an error for missing template.
func TemplateNotFound(template string) *MeowError {
	return Newf(CodeTemplateNotFound, "template not found: %s", template).
		WithDetail("template", template)
}

// TemplateInvalidType creates an error for invalid bead type.
func TemplateInvalidType(template, beadType string) *MeowError {
	return Newf(CodeTemplateInvalidType, "invalid bead type %q in template %s", beadType, template).
		WithDetail("template", template).
		WithDetail("type", beadType)
}

// --- Bead Errors ---

// BeadInvalidTransition creates an error for invalid status transition.
func BeadInvalidTransition(beadID, from, to string) *MeowError {
	return Newf(CodeBeadInvalidTransition, "invalid status transition for bead %s: %s -> %s", beadID, from, to).
		WithDetail("bead_id", beadID).
		WithDetail("from", from).
		WithDetail("to", to)
}

// BeadDependencyNotSatisfied creates an error for unmet dependency.
func BeadDependencyNotSatisfied(beadID, depID string) *MeowError {
	return Newf(CodeBeadDepNotSatisfied, "bead %s has unsatisfied dependency: %s", beadID, depID).
		WithDetail("bead_id", beadID).
		WithDetail("dependency_id", depID)
}

// BeadNotFound creates an error for missing bead.
func BeadNotFound(beadID string) *MeowError {
	return Newf(CodeBeadNotFound, "bead not found: %s", beadID).
		WithDetail("bead_id", beadID)
}

// BeadCycleDetected creates an error for cycle in bead dependencies.
func BeadCycleDetected(cycle []string) *MeowError {
	return New(CodeBeadCycleDetected, "cycle detected in bead dependencies").
		WithDetail("cycle", cycle)
}

// --- Agent Errors ---

// AgentSpawnFailed creates an error for failed agent spawn.
func AgentSpawnFailed(agentName string, err error) *MeowError {
	return Wrap(CodeAgentSpawnFailed, "failed to spawn agent", err).
		WithDetail("agent", agentName)
}

// AgentNotFound creates an error for missing agent.
func AgentNotFound(agentName string) *MeowError {
	return Newf(CodeAgentNotFound, "agent not found: %s", agentName).
		WithDetail("agent", agentName)
}

// AgentTimeout creates an error for agent timeout.
func AgentTimeout(agentName string, operation string) *MeowError {
	return Newf(CodeAgentTimeout, "agent %s timed out during %s", agentName, operation).
		WithDetail("agent", agentName).
		WithDetail("operation", operation)
}

// AgentAlreadyExists creates an error for duplicate agent.
func AgentAlreadyExists(agentName string) *MeowError {
	return Newf(CodeAgentAlreadyExists, "agent already exists: %s", agentName).
		WithDetail("agent", agentName)
}

// --- Output Errors ---

// OutputMissing creates an error for missing required output.
func OutputMissing(beadID, outputName string) *MeowError {
	return Newf(CodeOutputMissing, "bead %s missing required output: %s", beadID, outputName).
		WithDetail("bead_id", beadID).
		WithDetail("output", outputName)
}

// OutputTypeMismatch creates an error for output type mismatch.
func OutputTypeMismatch(beadID, outputName, expected, actual string) *MeowError {
	return Newf(CodeOutputTypeMismatch, "bead %s output %s type mismatch: expected %s, got %s", beadID, outputName, expected, actual).
		WithDetail("bead_id", beadID).
		WithDetail("output", outputName).
		WithDetail("expected", expected).
		WithDetail("actual", actual)
}

// OutputBeadNotFound creates an error for missing bead in output reference.
func OutputBeadNotFound(refBeadID, outputName string) *MeowError {
	return Newf(CodeOutputBeadNotFound, "output reference to non-existent bead %s.outputs.%s", refBeadID, outputName).
		WithDetail("ref_bead_id", refBeadID).
		WithDetail("output", outputName)
}

// --- IO Errors ---

// IOFileNotFound creates an error for missing file.
func IOFileNotFound(path string) *MeowError {
	return Newf(CodeIOFileNotFound, "file not found: %s", path).
		WithDetail("path", path)
}

// IOPermissionDenied creates an error for permission issues.
func IOPermissionDenied(path string, err error) *MeowError {
	return Wrap(CodeIOPermission, "permission denied", err).
		WithDetail("path", path)
}

// IODiskFull creates an error for disk space issues.
func IODiskFull(path string, err error) *MeowError {
	return Wrap(CodeIODiskFull, "disk full", err).
		WithDetail("path", path)
}

// IOReadError creates an error for read failures.
func IOReadError(path string, err error) *MeowError {
	return Wrap(CodeIOReadError, "failed to read file", err).
		WithDetail("path", path)
}

// IOWriteError creates an error for write failures.
func IOWriteError(path string, err error) *MeowError {
	return Wrap(CodeIOWriteError, "failed to write file", err).
		WithDetail("path", path)
}

// HasCode checks if an error is a MeowError with the given code.
// It handles wrapped errors by unwrapping to find a MeowError.
func HasCode(err error, code string) bool {
	var merr *MeowError
	if errors.As(err, &merr) {
		return merr.Code == code
	}
	return false
}

// Code returns the error code if err is a MeowError, empty string otherwise.
// It handles wrapped errors by unwrapping to find a MeowError.
func Code(err error) string {
	var merr *MeowError
	if errors.As(err, &merr) {
		return merr.Code
	}
	return ""
}
