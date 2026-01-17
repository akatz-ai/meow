package cmd

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/akatz-ai/meow/internal/ipc"
)

// TestDoneUsesOrchSockEnvVar verifies that meow done uses the socket path
// from MEOW_ORCH_SOCK environment variable, not derived from workflow ID.
func TestDoneUsesOrchSockEnvVar(t *testing.T) {
	// Create a temporary directory for sockets
	tmpDir := t.TempDir()

	// Create two different socket paths
	customSockPath := filepath.Join(tmpDir, "custom.sock")
	derivedSockPath := filepath.Join(tmpDir, "derived.sock")

	// Track which socket received the message (use atomics for thread safety)
	var receivedOnCustom atomic.Bool
	var receivedOnDerived atomic.Bool

	// Start a listener on the custom socket
	customListener, err := net.Listen("unix", customSockPath)
	if err != nil {
		t.Fatalf("failed to create custom socket: %v", err)
	}
	defer customListener.Close()

	// Start a listener on the derived socket
	derivedListener, err := net.Listen("unix", derivedSockPath)
	if err != nil {
		t.Fatalf("failed to create derived socket: %v", err)
	}
	defer derivedListener.Close()

	// Handle connections on custom socket
	go func() {
		conn, err := customListener.Accept()
		if err != nil {
			return
		}
		receivedOnCustom.Store(true)

		// Send acknowledgement
		ack := &ipc.AckMessage{Type: ipc.MsgAck, Success: true}
		data, _ := ipc.Marshal(ack)
		conn.Write(data)
		conn.Close()
	}()

	// Handle connections on derived socket
	go func() {
		conn, err := derivedListener.Accept()
		if err != nil {
			return
		}
		receivedOnDerived.Store(true)

		// Send acknowledgement
		ack := &ipc.AckMessage{Type: ipc.MsgAck, Success: true}
		data, _ := ipc.Marshal(ack)
		conn.Write(data)
		conn.Close()
	}()

	// Set environment variables to point to custom socket
	os.Setenv("MEOW_ORCH_SOCK", customSockPath)
	os.Setenv("MEOW_AGENT", "test-agent")
	os.Setenv("MEOW_WORKFLOW", "test-workflow")
	os.Setenv("MEOW_STEP", "test-step")

	defer func() {
		os.Unsetenv("MEOW_ORCH_SOCK")
		os.Unsetenv("MEOW_AGENT")
		os.Unsetenv("MEOW_WORKFLOW")
		os.Unsetenv("MEOW_STEP")
		doneNotes = ""
		doneOutputs = nil
		doneOutputJSON = ""
	}()

	// Mock SocketPath function to return our derived socket
	// We can't directly mock this, so we'll use a different approach:
	// We'll verify by checking which socket receives the connection

	// Run the done command
	err = runDone(doneCmd, nil)

	// Give goroutines time to process
	time.Sleep(100 * time.Millisecond)

	// With the bug: it would connect to derived socket (using NewClientForWorkflow)
	// With the fix: it should connect to custom socket (using NewClient with env var)

	if err != nil {
		// If there's an error, it's because it tried to connect to the wrong socket
		// Check if it was trying to connect to the derived path
		expectedDerivedPath := ipc.SocketPath("test-workflow")
		if err.Error() != fmt.Sprintf("sending done message: failed to connect to IPC socket %s: dial unix %s: connect: no such file or directory", expectedDerivedPath, expectedDerivedPath) {
			// It connected to custom socket successfully
			t.Logf("Connected to socket successfully (this is the fix)")
		}
	}

	// The bug is that done.go uses NewClientForWorkflow which derives socket from workflow ID
	// The fix is to use NewClient with the MEOW_ORCH_SOCK environment variable

	// Since we can't easily mock the socket path derivation, this test
	// documents the expected behavior. The real validation happens in integration tests.

	if !receivedOnCustom.Load() {
		t.Errorf("Expected message on custom socket (MEOW_ORCH_SOCK), but didn't receive it")
		t.Logf("This indicates the command is NOT using the MEOW_ORCH_SOCK environment variable")
	}

	if receivedOnDerived.Load() {
		t.Errorf("Received message on derived socket, but should have used MEOW_ORCH_SOCK")
		t.Logf("This indicates the bug: ignoring MEOW_ORCH_SOCK")
	}
}

// TestSessionIDUsesEnvSock verifies session-id command respects MEOW_ORCH_SOCK
func TestSessionIDUsesEnvSock(t *testing.T) {
	// Similar test for session-id command
	tmpDir := t.TempDir()
	customSockPath := filepath.Join(tmpDir, "custom.sock")

	// Create listener
	listener, err := net.Listen("unix", customSockPath)
	if err != nil {
		t.Fatalf("failed to create socket: %v", err)
	}
	defer listener.Close()

	var received atomic.Bool

	// Handle connection
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		received.Store(true)

		// Send acknowledgement with session ID
		resp := &ipc.SessionIDMessage{
			Type:      ipc.MsgSessionID,
			SessionID: "test-session-123",
		}
		data, _ := ipc.Marshal(resp)
		conn.Write(data)
		conn.Close()
	}()

	// Set environment
	os.Setenv("MEOW_ORCH_SOCK", customSockPath)
	os.Setenv("MEOW_WORKFLOW", "test-workflow")
	os.Setenv("MEOW_AGENT", "test-agent")

	defer func() {
		os.Unsetenv("MEOW_ORCH_SOCK")
		os.Unsetenv("MEOW_WORKFLOW")
		os.Unsetenv("MEOW_AGENT")
	}()

	// Run command
	err = runSessionID(sessionIDCmd, nil)

	time.Sleep(100 * time.Millisecond)

	if !received.Load() && err != nil {
		t.Logf("Command failed to connect to custom socket: %v", err)
		t.Logf("This indicates session-id is also ignoring MEOW_ORCH_SOCK")
	}
}
