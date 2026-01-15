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

	stepDoneCalls      []*StepDoneMessage
	getSessionIDCalls  []*GetSessionIDMessage
	eventCalls         []*EventMessage
	awaitEventCalls    []*AwaitEventMessage
	getStepStatusCalls []*GetStepStatusMessage

	// Configurable responses
	stepDoneResponse      any
	getSessionIDResponse  any
	eventResponse         any
	awaitEventResponse    any
	getStepStatusResponse any
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		stepDoneResponse:      &AckMessage{Type: MsgAck, Success: true},
		getSessionIDResponse:  &SessionIDMessage{Type: MsgSessionID, SessionID: "test-session-123"},
		eventResponse:         &AckMessage{Type: MsgAck, Success: true},
		awaitEventResponse:    &EventMatchMessage{Type: MsgEventMatch, EventType: "test", Data: nil, Timestamp: 0},
		getStepStatusResponse: &StepStatusMessage{Type: MsgStepStatus, StepID: "step-1", Status: "done"},
	}
}

func (h *mockHandler) HandleStepDone(ctx context.Context, msg *StepDoneMessage) any {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stepDoneCalls = append(h.stepDoneCalls, msg)
	return h.stepDoneResponse
}

func (h *mockHandler) HandleGetSessionID(ctx context.Context, msg *GetSessionIDMessage) any {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.getSessionIDCalls = append(h.getSessionIDCalls, msg)
	return h.getSessionIDResponse
}

func (h *mockHandler) HandleEvent(ctx context.Context, msg *EventMessage) any {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.eventCalls = append(h.eventCalls, msg)
	return h.eventResponse
}

func (h *mockHandler) HandleAwaitEvent(ctx context.Context, msg *AwaitEventMessage) any {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.awaitEventCalls = append(h.awaitEventCalls, msg)
	return h.awaitEventResponse
}

func (h *mockHandler) HandleGetStepStatus(ctx context.Context, msg *GetStepStatusMessage) any {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.getStepStatusCalls = append(h.getStepStatusCalls, msg)
	return h.getStepStatusResponse
}

func TestSocketPath(t *testing.T) {
	path := SocketPath("run-abc123")
	expected := filepath.Join(os.TempDir(), "meow-run-abc123.sock")
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

	response, err := client.SendStepDone("run-test", "agent-1", "step-1", map[string]any{"key": "value"}, "test notes")
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
	if call.Workflow != "run-test" {
		t.Errorf("Workflow = %q, want %q", call.Workflow, "run-test")
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

func TestServer_ErrorResponse(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "test.sock")
	handler := newMockHandler()
	handler.getSessionIDResponse = &ErrorMessage{Type: MsgError, Message: "agent not found"}
	server := NewServerWithPath(socketPath, handler, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.StartAsync(ctx); err != nil {
		t.Fatalf("StartAsync() error: %v", err)
	}
	defer server.Shutdown()

	time.Sleep(50 * time.Millisecond)

	client := NewClient(socketPath)
	_, err := client.GetSessionID("unknown-agent")
	if err == nil {
		t.Fatal("GetSessionID() should return error when server returns ErrorMessage")
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
			_, err := client.GetSessionID("agent-1")
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

	if len(handler.getSessionIDCalls) != 10 {
		t.Errorf("getSessionIDCalls = %d, want 10", len(handler.getSessionIDCalls))
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

	_, err := client.GetSessionID("agent-1")
	if err == nil {
		t.Fatal("GetSessionID() should return error for non-existent socket")
	}
}
