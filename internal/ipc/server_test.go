package ipc

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// mockHandler implements Handler for testing.
type mockHandler struct {
	mu sync.Mutex

	stepDoneCalls     []*StepDoneMessage
	getPromptCalls    []*GetPromptMessage
	getSessionIDCalls []*GetSessionIDMessage
	approvalCalls     []*ApprovalMessage

	// Configurable responses
	stepDoneResponse     any
	getPromptResponse    any
	getSessionIDResponse any
	approvalResponse     any
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		stepDoneResponse: &AckMessage{Type: MsgAck, Success: true},
		getPromptResponse: &PromptMessage{Type: MsgPrompt, Content: "Test prompt"},
		getSessionIDResponse: &SessionIDMessage{Type: MsgSessionID, SessionID: "test-session-123"},
		approvalResponse: &AckMessage{Type: MsgAck, Success: true},
	}
}

func (h *mockHandler) HandleStepDone(ctx context.Context, msg *StepDoneMessage) any {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stepDoneCalls = append(h.stepDoneCalls, msg)
	return h.stepDoneResponse
}

func (h *mockHandler) HandleGetPrompt(ctx context.Context, msg *GetPromptMessage) any {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.getPromptCalls = append(h.getPromptCalls, msg)
	return h.getPromptResponse
}

func (h *mockHandler) HandleGetSessionID(ctx context.Context, msg *GetSessionIDMessage) any {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.getSessionIDCalls = append(h.getSessionIDCalls, msg)
	return h.getSessionIDResponse
}

func (h *mockHandler) HandleApproval(ctx context.Context, msg *ApprovalMessage) any {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.approvalCalls = append(h.approvalCalls, msg)
	return h.approvalResponse
}

func TestSocketPath(t *testing.T) {
	path := SocketPath("wf-abc123")
	expected := filepath.Join(os.TempDir(), "meow-wf-abc123.sock")
	if path != expected {
		t.Errorf("SocketPath() = %q, want %q", path, expected)
	}
}

func TestServer_StartShutdown(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	handler := newMockHandler()
	server := NewServerWithPath(socketPath, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())

	// Start server in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Verify socket file exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		t.Error("socket file should exist after start")
	}

	// Shutdown
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Start() returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("server did not shut down in time")
	}

	// Verify socket file is removed
	if _, err := os.Stat(socketPath); !os.IsNotExist(err) {
		t.Error("socket file should be removed after shutdown")
	}
}

func TestServer_HandleStepDone(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	handler := newMockHandler()
	server := NewServerWithPath(socketPath, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server
	if err := server.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync() error: %v", err)
	}
	defer server.Shutdown()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Create client and send message
	client := NewClient(socketPath)
	client.SetTimeout(5 * time.Second)

	response, err := client.SendStepDone("wf-test", "agent-1", "step-1", map[string]any{"key": "value"}, "test notes")
	if err != nil {
		t.Fatalf("SendStepDone() error: %v", err)
	}

	// Verify response
	ack, ok := response.(*AckMessage)
	if !ok {
		t.Fatalf("response is %T, want *AckMessage", response)
	}
	if !ack.Success {
		t.Error("AckMessage.Success = false, want true")
	}

	// Verify handler was called
	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.stepDoneCalls) != 1 {
		t.Fatalf("stepDoneCalls = %d, want 1", len(handler.stepDoneCalls))
	}

	call := handler.stepDoneCalls[0]
	if call.Workflow != "wf-test" {
		t.Errorf("Workflow = %q, want %q", call.Workflow, "wf-test")
	}
	if call.Agent != "agent-1" {
		t.Errorf("Agent = %q, want %q", call.Agent, "agent-1")
	}
	if call.Step != "step-1" {
		t.Errorf("Step = %q, want %q", call.Step, "step-1")
	}
	if call.Notes != "test notes" {
		t.Errorf("Notes = %q, want %q", call.Notes, "test notes")
	}
}

func TestServer_HandleGetPrompt(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	handler := newMockHandler()
	handler.getPromptResponse = &PromptMessage{Type: MsgPrompt, Content: "Do this task"}
	server := NewServerWithPath(socketPath, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync() error: %v", err)
	}
	defer server.Shutdown()

	time.Sleep(50 * time.Millisecond)

	client := NewClient(socketPath)
	prompt, err := client.GetPrompt("agent-1")
	if err != nil {
		t.Fatalf("GetPrompt() error: %v", err)
	}

	if prompt != "Do this task" {
		t.Errorf("prompt = %q, want %q", prompt, "Do this task")
	}

	// Verify handler was called
	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.getPromptCalls) != 1 {
		t.Fatalf("getPromptCalls = %d, want 1", len(handler.getPromptCalls))
	}
	if handler.getPromptCalls[0].Agent != "agent-1" {
		t.Errorf("Agent = %q, want %q", handler.getPromptCalls[0].Agent, "agent-1")
	}
}

func TestServer_HandleGetPrompt_Empty(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	handler := newMockHandler()
	handler.getPromptResponse = &PromptMessage{Type: MsgPrompt, Content: ""} // Empty = stay idle
	server := NewServerWithPath(socketPath, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync() error: %v", err)
	}
	defer server.Shutdown()

	time.Sleep(50 * time.Millisecond)

	client := NewClient(socketPath)
	prompt, err := client.GetPrompt("agent-1")
	if err != nil {
		t.Fatalf("GetPrompt() error: %v", err)
	}

	if prompt != "" {
		t.Errorf("prompt = %q, want empty string", prompt)
	}
}

func TestServer_HandleGetSessionID(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	handler := newMockHandler()
	handler.getSessionIDResponse = &SessionIDMessage{Type: MsgSessionID, SessionID: "sess-xyz789"}
	server := NewServerWithPath(socketPath, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync() error: %v", err)
	}
	defer server.Shutdown()

	time.Sleep(50 * time.Millisecond)

	client := NewClient(socketPath)
	sessionID, err := client.GetSessionID("agent-1")
	if err != nil {
		t.Fatalf("GetSessionID() error: %v", err)
	}

	if sessionID != "sess-xyz789" {
		t.Errorf("sessionID = %q, want %q", sessionID, "sess-xyz789")
	}
}

func TestServer_HandleApproval(t *testing.T) {
	tests := []struct {
		name     string
		approved bool
		notes    string
		reason   string
	}{
		{"approved", true, "LGTM", ""},
		{"rejected", false, "", "Missing tests"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			socketPath := filepath.Join(t.TempDir(), "test.sock")
			handler := newMockHandler()
			server := NewServerWithPath(socketPath, handler, nil)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if err := server.StartAsync(ctx); err != nil {
				t.Fatalf("StartAsync() error: %v", err)
			}
			defer server.Shutdown()

			time.Sleep(50 * time.Millisecond)

			client := NewClient(socketPath)
			err := client.SendApproval("wf-test", "gate-1", tt.approved, tt.notes, tt.reason)
			if err != nil {
				t.Fatalf("SendApproval() error: %v", err)
			}

			handler.mu.Lock()
			defer handler.mu.Unlock()

			if len(handler.approvalCalls) != 1 {
				t.Fatalf("approvalCalls = %d, want 1", len(handler.approvalCalls))
			}

			call := handler.approvalCalls[0]
			if call.Approved != tt.approved {
				t.Errorf("Approved = %v, want %v", call.Approved, tt.approved)
			}
			if call.Notes != tt.notes {
				t.Errorf("Notes = %q, want %q", call.Notes, tt.notes)
			}
			if call.Reason != tt.reason {
				t.Errorf("Reason = %q, want %q", call.Reason, tt.reason)
			}
		})
	}
}

func TestServer_ErrorResponse(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	handler := newMockHandler()
	handler.getPromptResponse = &ErrorMessage{Type: MsgError, Message: "agent not found"}
	server := NewServerWithPath(socketPath, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync() error: %v", err)
	}
	defer server.Shutdown()

	time.Sleep(50 * time.Millisecond)

	client := NewClient(socketPath)
	_, err := client.GetPrompt("unknown-agent")
	if err == nil {
		t.Fatal("GetPrompt() should return error when server returns ErrorMessage")
	}

	if err.Error() != "server error: agent not found" {
		t.Errorf("error = %q, want 'server error: agent not found'", err.Error())
	}
}

func TestServer_MultipleConnections(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	handler := newMockHandler()
	server := NewServerWithPath(socketPath, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync() error: %v", err)
	}
	defer server.Shutdown()

	time.Sleep(50 * time.Millisecond)

	// Send multiple requests concurrently
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client := NewClient(socketPath)
			_, err := client.GetPrompt("agent-1")
			if err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent request error: %v", err)
	}

	// Verify all requests were handled
	handler.mu.Lock()
	defer handler.mu.Unlock()

	if len(handler.getPromptCalls) != 10 {
		t.Errorf("getPromptCalls = %d, want 10", len(handler.getPromptCalls))
	}
}

func TestServer_InvalidMessage(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	handler := newMockHandler()
	server := NewServerWithPath(socketPath, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync() error: %v", err)
	}
	defer server.Shutdown()

	time.Sleep(50 * time.Millisecond)

	// Send an invalid message type directly
	client := NewClient(socketPath)
	response, err := client.Send(&struct {
		Type string `json:"type"`
	}{Type: "invalid_type"})

	if err != nil {
		t.Fatalf("Send() error: %v", err)
	}

	errMsg, ok := response.(*ErrorMessage)
	if !ok {
		t.Fatalf("response is %T, want *ErrorMessage", response)
	}

	if errMsg.Message == "" {
		t.Error("ErrorMessage.Message should not be empty")
	}
}

func TestClient_ConnectionError(t *testing.T) {
	// Try to connect to a non-existent socket
	client := NewClient("/tmp/nonexistent-socket-12345.sock")
	client.SetTimeout(100 * time.Millisecond)

	_, err := client.GetPrompt("agent-1")
	if err == nil {
		t.Fatal("GetPrompt() should return error for non-existent socket")
	}
}
