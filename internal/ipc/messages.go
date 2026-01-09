// Package ipc provides message types and utilities for orchestrator-agent communication.
//
// The IPC protocol uses newline-delimited JSON over Unix domain sockets.
// Each message is a single JSON object on one line. Socket path: /tmp/meow-{workflow_id}.sock
package ipc

import (
	"encoding/json"
	"fmt"
)

// MessageType identifies the IPC message kind.
type MessageType string

const (
	// Request types (agent → orchestrator)
	MsgStepDone      MessageType = "step_done"
	MsgGetPrompt     MessageType = "get_prompt"
	MsgGetSessionID  MessageType = "get_session_id"
	MsgApproval      MessageType = "approval"
	MsgEvent         MessageType = "event"
	MsgAwaitEvent    MessageType = "await_event"
	MsgGetStepStatus MessageType = "get_step_status"

	// Response types (orchestrator → agent)
	MsgAck        MessageType = "ack"
	MsgError      MessageType = "error"
	MsgPrompt     MessageType = "prompt"
	MsgSessionID  MessageType = "session_id"
	MsgEventMatch MessageType = "event_match"
	MsgStepStatus MessageType = "step_status"
)

// Valid returns true if this is a recognized message type.
func (t MessageType) Valid() bool {
	switch t {
	case MsgStepDone, MsgGetPrompt, MsgGetSessionID, MsgApproval,
		MsgEvent, MsgAwaitEvent, MsgGetStepStatus,
		MsgAck, MsgError, MsgPrompt, MsgSessionID,
		MsgEventMatch, MsgStepStatus:
		return true
	}
	return false
}

// IsRequest returns true if this message type is sent from agent to orchestrator.
func (t MessageType) IsRequest() bool {
	switch t {
	case MsgStepDone, MsgGetPrompt, MsgGetSessionID, MsgApproval,
		MsgEvent, MsgAwaitEvent, MsgGetStepStatus:
		return true
	}
	return false
}

// IsResponse returns true if this message type is sent from orchestrator to agent.
func (t MessageType) IsResponse() bool {
	switch t {
	case MsgAck, MsgError, MsgPrompt, MsgSessionID,
		MsgEventMatch, MsgStepStatus:
		return true
	}
	return false
}

// --- Request Messages (agent → orchestrator) ---

// StepDoneMessage signals step completion from an agent.
// Sent by: meow done
type StepDoneMessage struct {
	Type     MessageType    `json:"type"` // Always "step_done"
	Workflow string         `json:"workflow"`
	Agent    string         `json:"agent"`
	Step     string         `json:"step"`
	Outputs  map[string]any `json:"outputs,omitempty"`
	Notes    string         `json:"notes,omitempty"`
}

// GetPromptMessage requests the current prompt for an agent.
// Sent by: meow prime (stop hook)
type GetPromptMessage struct {
	Type  MessageType `json:"type"` // Always "get_prompt"
	Agent string      `json:"agent"`
}

// GetSessionIDMessage requests the Claude session ID for an agent.
// Sent by: meow session-id
type GetSessionIDMessage struct {
	Type  MessageType `json:"type"` // Always "get_session_id"
	Agent string      `json:"agent"`
}

// ApprovalMessage signals human approval or rejection of a gate.
// Sent by: meow approve / meow reject
//
// Note: GateID is the step ID of the branch step implementing the gate pattern.
// Gates are not a separate executor - they're implemented as branch + await-approval.
type ApprovalMessage struct {
	Type     MessageType `json:"type"` // Always "approval"
	Workflow string      `json:"workflow"`
	GateID   string      `json:"gate_id"` // Step ID of the branch implementing the gate
	Approved bool        `json:"approved"`
	Notes    string      `json:"notes,omitempty"`
	Reason   string      `json:"reason,omitempty"` // For rejections
}

// EventMessage emits an event from an agent.
// Sent by: meow event
//
// Events are fire-and-forget notifications that can be matched by await-event waiters.
// The orchestrator adds Agent, Workflow, and Timestamp when processing.
type EventMessage struct {
	Type      MessageType    `json:"type"`       // Always "event"
	EventType string         `json:"event_type"` // e.g., "agent-stopped", "tool-completed"
	Data      map[string]any `json:"data"`       // Event-specific data
	Agent     string         `json:"agent"`      // Which agent emitted (set by orchestrator)
	Workflow  string         `json:"workflow"`   // Which workflow (set by orchestrator)
	Timestamp int64          `json:"timestamp"`  // Unix timestamp (set by orchestrator)
}

// AwaitEventMessage waits for an event matching filters.
// Sent by: meow await-event
//
// Blocks until a matching event arrives or timeout occurs.
type AwaitEventMessage struct {
	Type      MessageType       `json:"type"`       // Always "await_event"
	EventType string            `json:"event_type"` // Event type to wait for
	Filter    map[string]string `json:"filter"`     // Key-value filters (all must match)
	Timeout   string            `json:"timeout"`    // Duration string, e.g., "5m"
}

// GetStepStatusMessage requests the status of a step.
// Sent by: meow step-status
type GetStepStatusMessage struct {
	Type     MessageType `json:"type"`     // Always "get_step_status"
	Workflow string      `json:"workflow"` // Workflow ID
	StepID   string      `json:"step_id"`  // Step ID to query
}

// --- Response Messages (orchestrator → agent) ---

// AckMessage confirms successful operation.
type AckMessage struct {
	Type    MessageType `json:"type"` // Always "ack"
	Success bool        `json:"success"`
}

// ErrorMessage reports an error to the agent.
type ErrorMessage struct {
	Type    MessageType `json:"type"` // Always "error"
	Message string      `json:"message"`
}

// PromptMessage returns the current prompt for an agent.
// Empty Content means "no prompt, stay idle".
type PromptMessage struct {
	Type    MessageType `json:"type"` // Always "prompt"
	Content string      `json:"content"`
}

// SessionIDMessage returns the Claude session ID for an agent.
type SessionIDMessage struct {
	Type      MessageType `json:"type"` // Always "session_id"
	SessionID string      `json:"session_id"`
}

// EventMatchMessage is returned when an awaited event matches.
type EventMatchMessage struct {
	Type      MessageType    `json:"type"`       // Always "event_match"
	EventType string         `json:"event_type"` // The matched event type
	Data      map[string]any `json:"data"`       // Event data
	Timestamp int64          `json:"timestamp"`  // When the event was emitted
}

// StepStatusMessage returns the status of a step.
type StepStatusMessage struct {
	Type   MessageType `json:"type"`    // Always "step_status"
	StepID string      `json:"step_id"` // Step ID
	Status string      `json:"status"`  // "pending", "running", "completing", "done", "failed"
}

// --- Message Interface ---

// Message is the interface implemented by all IPC messages.
type Message interface {
	// MessageType returns the type identifier for this message.
	MessageType() MessageType
}

// Implement Message interface for all message types

func (m *StepDoneMessage) MessageType() MessageType      { return MsgStepDone }
func (m *GetPromptMessage) MessageType() MessageType     { return MsgGetPrompt }
func (m *GetSessionIDMessage) MessageType() MessageType  { return MsgGetSessionID }
func (m *ApprovalMessage) MessageType() MessageType      { return MsgApproval }
func (m *EventMessage) MessageType() MessageType         { return MsgEvent }
func (m *AwaitEventMessage) MessageType() MessageType    { return MsgAwaitEvent }
func (m *GetStepStatusMessage) MessageType() MessageType { return MsgGetStepStatus }
func (m *AckMessage) MessageType() MessageType           { return MsgAck }
func (m *ErrorMessage) MessageType() MessageType         { return MsgError }
func (m *PromptMessage) MessageType() MessageType        { return MsgPrompt }
func (m *SessionIDMessage) MessageType() MessageType     { return MsgSessionID }
func (m *EventMatchMessage) MessageType() MessageType    { return MsgEventMatch }
func (m *StepStatusMessage) MessageType() MessageType    { return MsgStepStatus }

// --- Parsing Helpers ---

// RawMessage is used for initial parsing to determine message type.
type RawMessage struct {
	Type MessageType `json:"type"`
}

// ParseMessage parses a JSON message and returns the appropriate typed message.
// Returns an error if the message type is unknown or JSON is malformed.
func ParseMessage(data []byte) (Message, error) {
	var raw RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	var msg Message
	switch raw.Type {
	case MsgStepDone:
		msg = &StepDoneMessage{}
	case MsgGetPrompt:
		msg = &GetPromptMessage{}
	case MsgGetSessionID:
		msg = &GetSessionIDMessage{}
	case MsgApproval:
		msg = &ApprovalMessage{}
	case MsgEvent:
		msg = &EventMessage{}
	case MsgAwaitEvent:
		msg = &AwaitEventMessage{}
	case MsgGetStepStatus:
		msg = &GetStepStatusMessage{}
	case MsgAck:
		msg = &AckMessage{}
	case MsgError:
		msg = &ErrorMessage{}
	case MsgPrompt:
		msg = &PromptMessage{}
	case MsgSessionID:
		msg = &SessionIDMessage{}
	case MsgEventMatch:
		msg = &EventMatchMessage{}
	case MsgStepStatus:
		msg = &StepStatusMessage{}
	default:
		return nil, fmt.Errorf("unknown message type: %q", raw.Type)
	}

	if err := json.Unmarshal(data, msg); err != nil {
		return nil, fmt.Errorf("failed to parse %s message: %w", raw.Type, err)
	}

	return msg, nil
}

// Marshal serializes a message to JSON as a single line (no pretty printing).
func Marshal(msg any) ([]byte, error) {
	return json.Marshal(msg)
}
