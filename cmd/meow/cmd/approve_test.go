package cmd

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/akatz-ai/meow/internal/ipc"
)

// TestApproveWithEnvSock verifies that approve emits gate-approved event
func TestApproveWithEnvSock(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	// Create listener
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}
	defer listener.Close()

	received := false
	var receivedMsg *ipc.EventMessage

	// Handle connection
	done := make(chan bool)
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read message with buffering
		reader := make([]byte, 4096)
		n, err := conn.Read(reader)
		if err != nil {
			t.Logf("Error reading: %v", err)
			return
		}

		msg, err := ipc.ParseMessage(reader[:n])
		if err != nil {
			t.Logf("Error parsing: %v", err)
			return
		}

		if event, ok := msg.(*ipc.EventMessage); ok {
			received = true
			receivedMsg = event
		}

		// Send acknowledgement with newline delimiter
		ack := &ipc.AckMessage{Type: ipc.MsgAck, Success: true}
		ackData, _ := ipc.Marshal(ack)
		ackData = append(ackData, '\n')
		conn.Write(ackData)
	}()

	// Set environment
	os.Setenv("MEOW_ORCH_SOCK", sockPath)
	os.Setenv("MEOW_WORKFLOW", "test-workflow")
	defer func() {
		os.Unsetenv("MEOW_ORCH_SOCK")
		os.Unsetenv("MEOW_WORKFLOW")
		approveWorkflow = ""
		approveApprover = ""
	}()

	// Run approve command
	err = runApprove(approveCmd, []string{"gate-123"})
	if err != nil {
		t.Fatalf("runApprove failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if !received {
		t.Fatal("Event message was not received")
	}

	if receivedMsg.EventType != "gate-approved" {
		t.Errorf("EventType = %q, want %q", receivedMsg.EventType, "gate-approved")
	}

	if gate, ok := receivedMsg.Data["gate"].(string); !ok || gate != "gate-123" {
		t.Errorf("Data[gate] = %v, want %q", receivedMsg.Data["gate"], "gate-123")
	}

	if workflow, ok := receivedMsg.Data["workflow"].(string); !ok || workflow != "test-workflow" {
		t.Errorf("Data[workflow] = %v, want %q", receivedMsg.Data["workflow"], "test-workflow")
	}
}

// TestApproveWithApprover verifies that --approver flag is sent in event data
func TestApproveWithApprover(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}
	defer listener.Close()

	var receivedMsg *ipc.EventMessage

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		data := make([]byte, 4096)
		n, _ := conn.Read(data)
		msg, _ := ipc.ParseMessage(data[:n])
		if event, ok := msg.(*ipc.EventMessage); ok {
			receivedMsg = event
		}

		ack := &ipc.AckMessage{Type: ipc.MsgAck, Success: true}
		ackData, _ := ipc.Marshal(ack)
		ackData = append(ackData, '\n')
		conn.Write(ackData)
	}()

	os.Setenv("MEOW_ORCH_SOCK", sockPath)
	os.Setenv("MEOW_WORKFLOW", "test-workflow")
	approveApprover = "John Doe"
	defer func() {
		os.Unsetenv("MEOW_ORCH_SOCK")
		os.Unsetenv("MEOW_WORKFLOW")
		approveWorkflow = ""
		approveApprover = ""
	}()

	err = runApprove(approveCmd, []string{"gate-123"})
	if err != nil {
		t.Fatalf("runApprove failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if receivedMsg == nil {
		t.Fatal("No message received")
	}

	if approver, ok := receivedMsg.Data["approver"].(string); !ok || approver != "John Doe" {
		t.Errorf("Data[approver] = %v, want %q", receivedMsg.Data["approver"], "John Doe")
	}
}

// TestApproveRequiresWorkflow verifies error when no workflow specified
func TestApproveRequiresWorkflow(t *testing.T) {
	os.Unsetenv("MEOW_ORCH_SOCK")
	os.Unsetenv("MEOW_WORKFLOW")
	approveWorkflow = ""
	defer func() {
		approveWorkflow = ""
	}()

	err := runApprove(approveCmd, []string{"gate-123"})
	if err == nil {
		t.Fatal("Expected error when no workflow specified, got nil")
	}

	expected := "either MEOW_ORCH_SOCK or --workflow required"
	if err.Error() != expected {
		t.Errorf("Error = %q, want %q", err.Error(), expected)
	}
}

// TestRejectWithReason verifies that --reason flag is sent in event data
func TestRejectWithReason(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}
	defer listener.Close()

	var receivedMsg *ipc.EventMessage

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		data := make([]byte, 4096)
		n, _ := conn.Read(data)
		msg, _ := ipc.ParseMessage(data[:n])
		if event, ok := msg.(*ipc.EventMessage); ok {
			receivedMsg = event
		}

		ack := &ipc.AckMessage{Type: ipc.MsgAck, Success: true}
		ackData, _ := ipc.Marshal(ack)
		ackData = append(ackData, '\n')
		conn.Write(ackData)
	}()

	os.Setenv("MEOW_ORCH_SOCK", sockPath)
	os.Setenv("MEOW_WORKFLOW", "test-workflow")
	rejectReason = "Tests failing"
	defer func() {
		os.Unsetenv("MEOW_ORCH_SOCK")
		os.Unsetenv("MEOW_WORKFLOW")
		rejectWorkflow = ""
		rejectReason = ""
	}()

	err = runReject(rejectCmd, []string{"gate-123"})
	if err != nil {
		t.Fatalf("runReject failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if receivedMsg == nil {
		t.Fatal("No message received")
	}

	if receivedMsg.EventType != "gate-rejected" {
		t.Errorf("EventType = %q, want %q", receivedMsg.EventType, "gate-rejected")
	}

	if reason, ok := receivedMsg.Data["reason"].(string); !ok || reason != "Tests failing" {
		t.Errorf("Data[reason] = %v, want %q", receivedMsg.Data["reason"], "Tests failing")
	}
}
