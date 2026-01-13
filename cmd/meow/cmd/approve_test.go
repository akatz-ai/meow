package cmd

import (
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/ipc"
)

// TestApproveWithEnvSock verifies that approve uses MEOW_ORCH_SOCK
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
	var receivedMsg *ipc.ApprovalMessage

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

		if approval, ok := msg.(*ipc.ApprovalMessage); ok {
			received = true
			receivedMsg = approval
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
		t.Fatal("Approval message was not received")
	}

	if receivedMsg.Workflow != "test-workflow" {
		t.Errorf("Workflow = %q, want %q", receivedMsg.Workflow, "test-workflow")
	}

	if receivedMsg.GateID != "gate-123" {
		t.Errorf("GateID = %q, want %q", receivedMsg.GateID, "gate-123")
	}

	if !receivedMsg.Approved {
		t.Error("Approved = false, want true")
	}
}

// TestApproveWithApprover verifies that --approver flag is sent in notes
func TestApproveWithApprover(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}
	defer listener.Close()

	var receivedMsg *ipc.ApprovalMessage

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		data := make([]byte, 4096)
		n, _ := conn.Read(data)
		msg, _ := ipc.ParseMessage(data[:n])
		if approval, ok := msg.(*ipc.ApprovalMessage); ok {
			receivedMsg = approval
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

	if receivedMsg.Notes != "John Doe" {
		t.Errorf("Notes = %q, want %q", receivedMsg.Notes, "John Doe")
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

// TestRejectWithReason verifies that --reason flag is sent
func TestRejectWithReason(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}
	defer listener.Close()

	var receivedMsg *ipc.ApprovalMessage

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		data := make([]byte, 4096)
		n, _ := conn.Read(data)
		msg, _ := ipc.ParseMessage(data[:n])
		if approval, ok := msg.(*ipc.ApprovalMessage); ok {
			receivedMsg = approval
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

	if receivedMsg.Approved {
		t.Error("Approved = true, want false for rejection")
	}

	if receivedMsg.Reason != "Tests failing" {
		t.Errorf("Reason = %q, want %q", receivedMsg.Reason, "Tests failing")
	}
}
