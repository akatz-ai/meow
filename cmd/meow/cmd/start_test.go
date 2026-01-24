package cmd

import (
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akatz-ai/meow/internal/ipc"
)

// TestStartSilentNoOpOutsideMeow verifies that meow start exits silently
// when MEOW_ORCH_SOCK is not set (not running in a MEOW workflow).
func TestStartSilentNoOpOutsideMeow(t *testing.T) {
	// Ensure env vars are not set
	os.Unsetenv("MEOW_ORCH_SOCK")
	os.Unsetenv("MEOW_AGENT")
	os.Unsetenv("MEOW_WORKFLOW")

	// Run the start command - should return nil (silent no-op)
	err := runStart(startCmd, nil)
	if err != nil {
		t.Errorf("expected nil error for silent no-op, got: %v", err)
	}
}

// TestStartRequiresAgent verifies that meow start returns an error
// when MEOW_AGENT is not set.
func TestStartRequiresAgent(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	os.Setenv("MEOW_ORCH_SOCK", sockPath)
	os.Unsetenv("MEOW_AGENT")
	os.Setenv("MEOW_WORKFLOW", "test-workflow")

	defer func() {
		os.Unsetenv("MEOW_ORCH_SOCK")
		os.Unsetenv("MEOW_WORKFLOW")
	}()

	err := runStart(startCmd, nil)
	if err == nil {
		t.Error("expected error when MEOW_AGENT not set")
	}
}

// TestStartRequiresWorkflow verifies that meow start returns an error
// when MEOW_WORKFLOW is not set.
func TestStartRequiresWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	os.Setenv("MEOW_ORCH_SOCK", sockPath)
	os.Setenv("MEOW_AGENT", "test-agent")
	os.Unsetenv("MEOW_WORKFLOW")

	defer func() {
		os.Unsetenv("MEOW_ORCH_SOCK")
		os.Unsetenv("MEOW_AGENT")
	}()

	err := runStart(startCmd, nil)
	if err == nil {
		t.Error("expected error when MEOW_WORKFLOW not set")
	}
}

// TestStartUsesOrchSockEnvVar verifies that meow start uses the socket path
// from MEOW_ORCH_SOCK environment variable.
func TestStartUsesOrchSockEnvVar(t *testing.T) {
	tmpDir := t.TempDir()
	customSockPath := filepath.Join(tmpDir, "custom.sock")

	// Track whether we received a message
	var received atomic.Bool

	// Start a listener on the custom socket
	listener, err := net.Listen("unix", customSockPath)
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}
	defer listener.Close()

	// Handle connections
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		received.Store(true)

		// Send acknowledgement
		ack := &ipc.AckMessage{Type: ipc.MsgAck, Success: true}
		data, _ := ipc.Marshal(ack)
		data = append(data, '\n')
		conn.Write(data)
	}()

	// Set environment variables
	os.Setenv("MEOW_ORCH_SOCK", customSockPath)
	os.Setenv("MEOW_AGENT", "test-agent")
	os.Setenv("MEOW_WORKFLOW", "test-workflow")
	os.Setenv("MEOW_STEP", "test-step")

	defer func() {
		os.Unsetenv("MEOW_ORCH_SOCK")
		os.Unsetenv("MEOW_AGENT")
		os.Unsetenv("MEOW_WORKFLOW")
		os.Unsetenv("MEOW_STEP")
	}()

	// Run the start command
	err = runStart(startCmd, nil)

	// Give goroutine time to process
	time.Sleep(100 * time.Millisecond)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !received.Load() {
		t.Error("expected message on socket, but didn't receive it")
	}
}

// TestStartUsesStepFromArg verifies that meow start can take step ID from arguments.
func TestStartUsesStepFromArg(t *testing.T) {
	tmpDir := t.TempDir()
	sockPath := filepath.Join(tmpDir, "test.sock")

	var receivedStep string

	// Start a listener
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}
	defer listener.Close()

	// Handle connections
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read the message
		buf := make([]byte, 1024)
		n, _ := conn.Read(buf)
		msg, _ := ipc.ParseMessage(buf[:n])
		if startMsg, ok := msg.(*ipc.StepStartMessage); ok {
			receivedStep = startMsg.Step
		}

		// Send acknowledgement
		ack := &ipc.AckMessage{Type: ipc.MsgAck, Success: true}
		data, _ := ipc.Marshal(ack)
		data = append(data, '\n')
		conn.Write(data)
	}()

	// Set environment variables (no MEOW_STEP)
	os.Setenv("MEOW_ORCH_SOCK", sockPath)
	os.Setenv("MEOW_AGENT", "test-agent")
	os.Setenv("MEOW_WORKFLOW", "test-workflow")
	os.Unsetenv("MEOW_STEP")

	defer func() {
		os.Unsetenv("MEOW_ORCH_SOCK")
		os.Unsetenv("MEOW_AGENT")
		os.Unsetenv("MEOW_WORKFLOW")
	}()

	// Run with step ID as argument
	err = runStart(startCmd, []string{"arg-step-id"})

	time.Sleep(100 * time.Millisecond)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if receivedStep != "arg-step-id" {
		t.Errorf("expected step 'arg-step-id', got '%s'", receivedStep)
	}
}
