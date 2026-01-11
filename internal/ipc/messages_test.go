package ipc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageType_Valid(t *testing.T) {
	tests := []struct {
		mt   MessageType
		want bool
	}{
		{MsgStepDone, true},
		{MsgGetSessionID, true},
		{MsgApproval, true},
		{MsgEvent, true},
		{MsgAwaitEvent, true},
		{MsgGetStepStatus, true},
		{MsgAck, true},
		{MsgError, true},
		{MsgSessionID, true},
		{MsgEventMatch, true},
		{MsgStepStatus, true},
		{"unknown", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := tt.mt.Valid(); got != tt.want {
			t.Errorf("MessageType(%q).Valid() = %v, want %v", tt.mt, got, tt.want)
		}
	}
}

func TestMessageType_IsRequest(t *testing.T) {
	requests := []MessageType{MsgStepDone, MsgGetSessionID, MsgApproval, MsgEvent, MsgAwaitEvent, MsgGetStepStatus}
	responses := []MessageType{MsgAck, MsgError, MsgSessionID, MsgEventMatch, MsgStepStatus}

	for _, mt := range requests {
		if !mt.IsRequest() {
			t.Errorf("MessageType(%q).IsRequest() = false, want true", mt)
		}
		if mt.IsResponse() {
			t.Errorf("MessageType(%q).IsResponse() = true, want false", mt)
		}
	}

	for _, mt := range responses {
		if mt.IsRequest() {
			t.Errorf("MessageType(%q).IsRequest() = true, want false", mt)
		}
		if !mt.IsResponse() {
			t.Errorf("MessageType(%q).IsResponse() = false, want true", mt)
		}
	}
}

func TestStepDoneMessage_Marshal(t *testing.T) {
	msg := StepDoneMessage{
		Type:     MsgStepDone,
		Workflow: "wf-abc123",
		Agent:    "worker-1",
		Step:     "impl.write-tests",
		Outputs:  map[string]any{"test_file": "src/test.ts"},
		Notes:    "Tests written successfully",
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Verify single-line output (no embedded newlines except escaped)
	if strings.Count(string(data), "\n") > 0 {
		t.Errorf("Marshal() produced multi-line output: %s", data)
	}

	// Verify round-trip
	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*StepDoneMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *StepDoneMessage", parsed)
	}

	if got.Workflow != msg.Workflow {
		t.Errorf("Workflow = %q, want %q", got.Workflow, msg.Workflow)
	}
	if got.Agent != msg.Agent {
		t.Errorf("Agent = %q, want %q", got.Agent, msg.Agent)
	}
	if got.Step != msg.Step {
		t.Errorf("Step = %q, want %q", got.Step, msg.Step)
	}
	if got.Notes != msg.Notes {
		t.Errorf("Notes = %q, want %q", got.Notes, msg.Notes)
	}
}

func TestGetSessionIDMessage_Marshal(t *testing.T) {
	msg := GetSessionIDMessage{
		Type:  MsgGetSessionID,
		Agent: "worker-1",
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*GetSessionIDMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *GetSessionIDMessage", parsed)
	}

	if got.Agent != msg.Agent {
		t.Errorf("Agent = %q, want %q", got.Agent, msg.Agent)
	}
}

func TestApprovalMessage_Marshal(t *testing.T) {
	tests := []struct {
		name string
		msg  ApprovalMessage
	}{
		{
			name: "approved",
			msg: ApprovalMessage{
				Type:     MsgApproval,
				Workflow: "wf-abc123",
				GateID:   "review-gate",
				Approved: true,
				Notes:    "LGTM",
			},
		},
		{
			name: "rejected",
			msg: ApprovalMessage{
				Type:     MsgApproval,
				Workflow: "wf-abc123",
				GateID:   "review-gate",
				Approved: false,
				Reason:   "Missing error handling",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := Marshal(tt.msg)
			if err != nil {
				t.Fatalf("Marshal() error = %v", err)
			}

			parsed, err := ParseMessage(data)
			if err != nil {
				t.Fatalf("ParseMessage() error = %v", err)
			}

			got, ok := parsed.(*ApprovalMessage)
			if !ok {
				t.Fatalf("ParseMessage() returned %T, want *ApprovalMessage", parsed)
			}

			if got.Workflow != tt.msg.Workflow {
				t.Errorf("Workflow = %q, want %q", got.Workflow, tt.msg.Workflow)
			}
			if got.GateID != tt.msg.GateID {
				t.Errorf("GateID = %q, want %q", got.GateID, tt.msg.GateID)
			}
			if got.Approved != tt.msg.Approved {
				t.Errorf("Approved = %v, want %v", got.Approved, tt.msg.Approved)
			}
		})
	}
}

func TestAckMessage_Marshal(t *testing.T) {
	msg := AckMessage{
		Type:    MsgAck,
		Success: true,
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*AckMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *AckMessage", parsed)
	}

	if got.Success != msg.Success {
		t.Errorf("Success = %v, want %v", got.Success, msg.Success)
	}
}

func TestErrorMessage_Marshal(t *testing.T) {
	msg := ErrorMessage{
		Type:    MsgError,
		Message: "Missing required output: task_id",
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*ErrorMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *ErrorMessage", parsed)
	}

	if got.Message != msg.Message {
		t.Errorf("Message = %q, want %q", got.Message, msg.Message)
	}
}

func TestSessionIDMessage_Marshal(t *testing.T) {
	msg := SessionIDMessage{
		Type:      MsgSessionID,
		SessionID: "session-abc123xyz",
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*SessionIDMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *SessionIDMessage", parsed)
	}

	if got.SessionID != msg.SessionID {
		t.Errorf("SessionID = %q, want %q", got.SessionID, msg.SessionID)
	}
}

func TestParseMessage_UnknownType(t *testing.T) {
	data := []byte(`{"type":"unknown_type"}`)

	_, err := ParseMessage(data)
	if err == nil {
		t.Fatal("ParseMessage() expected error for unknown type")
	}

	if !strings.Contains(err.Error(), "unknown message type") {
		t.Errorf("error = %q, want to contain 'unknown message type'", err.Error())
	}
}

func TestParseMessage_MalformedJSON(t *testing.T) {
	data := []byte(`{not valid json}`)

	_, err := ParseMessage(data)
	if err == nil {
		t.Fatal("ParseMessage() expected error for malformed JSON")
	}

	if !strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("error = %q, want to contain 'invalid JSON'", err.Error())
	}
}

func TestParseMessage_MissingType(t *testing.T) {
	data := []byte(`{"workflow":"wf-123"}`)

	_, err := ParseMessage(data)
	if err == nil {
		t.Fatal("ParseMessage() expected error for missing type")
	}
}

func TestMarshal_SingleLine(t *testing.T) {
	// Test that complex content with newlines is properly escaped
	msg := StepDoneMessage{
		Type:     MsgStepDone,
		Workflow: "wf-test",
		Agent:    "worker",
		Step:     "step1",
		Notes:    "Line 1\nLine 2\nLine 3",
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Count actual newline bytes in output
	newlineCount := 0
	for _, b := range data {
		if b == '\n' {
			newlineCount++
		}
	}

	if newlineCount > 0 {
		t.Errorf("Marshal() output contains %d literal newlines, want 0", newlineCount)
	}

	// Verify the escaped newlines are present
	if !strings.Contains(string(data), `\n`) {
		t.Error("Marshal() should contain escaped newline sequences")
	}

	// Verify round-trip preserves newlines in content
	var decoded StepDoneMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.Notes != msg.Notes {
		t.Errorf("Round-trip Notes = %q, want %q", decoded.Notes, msg.Notes)
	}
}

func TestStepDoneMessage_OutputTypes(t *testing.T) {
	// Test various output value types
	msg := StepDoneMessage{
		Type:     MsgStepDone,
		Workflow: "wf-test",
		Agent:    "worker",
		Step:     "step1",
		Outputs: map[string]any{
			"string_val":  "hello",
			"number_val":  42,
			"float_val":   3.14,
			"bool_val":    true,
			"json_val":    map[string]any{"nested": "value"},
			"array_val":   []any{"a", "b", "c"},
			"file_path":   "src/main.go",
		},
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*StepDoneMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *StepDoneMessage", parsed)
	}

	// Check string value
	if v, ok := got.Outputs["string_val"].(string); !ok || v != "hello" {
		t.Errorf("Outputs[string_val] = %v, want 'hello'", got.Outputs["string_val"])
	}

	// Check number value (JSON numbers become float64)
	if v, ok := got.Outputs["number_val"].(float64); !ok || v != 42 {
		t.Errorf("Outputs[number_val] = %v, want 42", got.Outputs["number_val"])
	}

	// Check bool value
	if v, ok := got.Outputs["bool_val"].(bool); !ok || v != true {
		t.Errorf("Outputs[bool_val] = %v, want true", got.Outputs["bool_val"])
	}
}

func TestEventMessage_Marshal(t *testing.T) {
	msg := EventMessage{
		Type:      MsgEvent,
		EventType: "tool-completed",
		Data: map[string]any{
			"tool":      "Bash",
			"exit_code": 0,
		},
		Agent:     "worker-1",
		Workflow:  "wf-abc123",
		Timestamp: 1704825600,
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	// Verify single-line output
	if strings.Count(string(data), "\n") > 0 {
		t.Errorf("Marshal() produced multi-line output")
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*EventMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *EventMessage", parsed)
	}

	if got.EventType != msg.EventType {
		t.Errorf("EventType = %q, want %q", got.EventType, msg.EventType)
	}
	if got.Agent != msg.Agent {
		t.Errorf("Agent = %q, want %q", got.Agent, msg.Agent)
	}
	if got.Workflow != msg.Workflow {
		t.Errorf("Workflow = %q, want %q", got.Workflow, msg.Workflow)
	}
	if got.Timestamp != msg.Timestamp {
		t.Errorf("Timestamp = %d, want %d", got.Timestamp, msg.Timestamp)
	}
	if got.Data["tool"] != "Bash" {
		t.Errorf("Data[tool] = %v, want 'Bash'", got.Data["tool"])
	}
}

func TestAwaitEventMessage_Marshal(t *testing.T) {
	msg := AwaitEventMessage{
		Type:      MsgAwaitEvent,
		EventType: "tool-completed",
		Filter: map[string]string{
			"tool":   "Bash",
			"status": "success",
		},
		Timeout: "5m",
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*AwaitEventMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *AwaitEventMessage", parsed)
	}

	if got.EventType != msg.EventType {
		t.Errorf("EventType = %q, want %q", got.EventType, msg.EventType)
	}
	if got.Timeout != msg.Timeout {
		t.Errorf("Timeout = %q, want %q", got.Timeout, msg.Timeout)
	}
	if got.Filter["tool"] != "Bash" {
		t.Errorf("Filter[tool] = %v, want 'Bash'", got.Filter["tool"])
	}
	if got.Filter["status"] != "success" {
		t.Errorf("Filter[status] = %v, want 'success'", got.Filter["status"])
	}
}

func TestGetStepStatusMessage_Marshal(t *testing.T) {
	msg := GetStepStatusMessage{
		Type:     MsgGetStepStatus,
		Workflow: "wf-abc123",
		StepID:   "impl.write-tests",
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*GetStepStatusMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *GetStepStatusMessage", parsed)
	}

	if got.Workflow != msg.Workflow {
		t.Errorf("Workflow = %q, want %q", got.Workflow, msg.Workflow)
	}
	if got.StepID != msg.StepID {
		t.Errorf("StepID = %q, want %q", got.StepID, msg.StepID)
	}
}

func TestEventMatchMessage_Marshal(t *testing.T) {
	msg := EventMatchMessage{
		Type:      MsgEventMatch,
		EventType: "tool-completed",
		Data: map[string]any{
			"tool":      "Bash",
			"exit_code": 0,
		},
		Timestamp: 1704825600,
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*EventMatchMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *EventMatchMessage", parsed)
	}

	if got.EventType != msg.EventType {
		t.Errorf("EventType = %q, want %q", got.EventType, msg.EventType)
	}
	if got.Timestamp != msg.Timestamp {
		t.Errorf("Timestamp = %d, want %d", got.Timestamp, msg.Timestamp)
	}
}

func TestStepStatusMessage_Marshal(t *testing.T) {
	msg := StepStatusMessage{
		Type:   MsgStepStatus,
		StepID: "impl.write-tests",
		Status: "done",
	}

	data, err := Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	parsed, err := ParseMessage(data)
	if err != nil {
		t.Fatalf("ParseMessage() error = %v", err)
	}

	got, ok := parsed.(*StepStatusMessage)
	if !ok {
		t.Fatalf("ParseMessage() returned %T, want *StepStatusMessage", parsed)
	}

	if got.StepID != msg.StepID {
		t.Errorf("StepID = %q, want %q", got.StepID, msg.StepID)
	}
	if got.Status != msg.Status {
		t.Errorf("Status = %q, want %q", got.Status, msg.Status)
	}
}
