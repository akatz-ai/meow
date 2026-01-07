package testutil

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// MockClaude simulates Claude Code behavior for testing.
// It can respond to commands and simulate workflow execution.
type MockClaude struct {
	mu sync.RWMutex
	t  *testing.T

	// Responses maps command patterns to canned responses
	Responses map[string]MockResponse

	// Interactions records all interactions with the mock
	Interactions []MockInteraction

	// Behavior configuration
	ResponseDelay time.Duration
	FailOnCommand string   // If set, fail when this command is received
	FailError     string   // Error message to return on failure

	// State
	SessionID     string
	CurrentBead   string
	CompletedBeads []string
}

// MockResponse defines a canned response for a command.
type MockResponse struct {
	Output    string
	ExitCode  int
	Delay     time.Duration
	Callback  func(cmd string) (string, error)  // Dynamic response
}

// MockInteraction records an interaction with the mock.
type MockInteraction struct {
	Command   string
	Response  string
	ExitCode  int
	Timestamp time.Time
	Error     error
}

// NewMockClaude creates a new mock Claude for testing.
func NewMockClaude(t *testing.T) *MockClaude {
	t.Helper()
	return &MockClaude{
		t:              t,
		Responses:      make(map[string]MockResponse),
		Interactions:   make([]MockInteraction, 0),
		SessionID:      fmt.Sprintf("session-%d", time.Now().UnixNano()%100000),
		CompletedBeads: make([]string, 0),
	}
}

// SetResponse sets a canned response for a command pattern.
// The pattern is matched as a prefix of the command.
func (m *MockClaude) SetResponse(pattern string, output string, exitCode int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses[pattern] = MockResponse{
		Output:   output,
		ExitCode: exitCode,
	}
}

// SetDynamicResponse sets a callback-based response for a command pattern.
func (m *MockClaude) SetDynamicResponse(pattern string, callback func(cmd string) (string, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Responses[pattern] = MockResponse{
		Callback: callback,
	}
}

// Execute simulates executing a command in Claude.
func (m *MockClaude) Execute(command string) (string, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ResponseDelay > 0 {
		time.Sleep(m.ResponseDelay)
	}

	// Check for forced failure
	if m.FailOnCommand != "" && strings.Contains(command, m.FailOnCommand) {
		err := fmt.Errorf("mock failure: %s", m.FailError)
		m.recordInteraction(command, "", 1, err)
		return "", 1, err
	}

	// Look for matching response
	for pattern, resp := range m.Responses {
		if strings.HasPrefix(command, pattern) {
			if resp.Callback != nil {
				output, err := resp.Callback(command)
				exitCode := 0
				if err != nil {
					exitCode = 1
				}
				m.recordInteraction(command, output, exitCode, err)
				return output, exitCode, err
			}

			if resp.Delay > 0 {
				time.Sleep(resp.Delay)
			}
			m.recordInteraction(command, resp.Output, resp.ExitCode, nil)
			return resp.Output, resp.ExitCode, nil
		}
	}

	// Default response for known commands
	output, exitCode := m.defaultResponse(command)
	m.recordInteraction(command, output, exitCode, nil)
	return output, exitCode, nil
}

// defaultResponse provides default responses for common commands.
func (m *MockClaude) defaultResponse(command string) (string, int) {
	switch {
	case strings.HasPrefix(command, "meow prime"):
		return m.handleMeowPrime(command)
	case strings.HasPrefix(command, "meow close"):
		return m.handleMeowClose(command)
	case strings.HasPrefix(command, "meow status"):
		return m.handleMeowStatus(command)
	case strings.HasPrefix(command, "meow session-id"):
		return m.SessionID, 0
	default:
		return fmt.Sprintf("Mock Claude executed: %s", command), 0
	}
}

// handleMeowPrime simulates the meow prime command.
func (m *MockClaude) handleMeowPrime(command string) (string, int) {
	// Check for --format json flag
	if strings.Contains(command, "--format json") {
		response := map[string]string{
			"bead_id":      m.CurrentBead,
			"session_id":   m.SessionID,
			"instructions": "Test instructions",
		}
		data, _ := json.Marshal(response)
		return string(data), 0
	}

	return fmt.Sprintf(`Bead: %s
Session: %s
Instructions: Test task instructions

Ready to work.`, m.CurrentBead, m.SessionID), 0
}

// handleMeowClose simulates the meow close command.
func (m *MockClaude) handleMeowClose(command string) (string, int) {
	// Extract bead ID from command
	parts := strings.Fields(command)
	beadID := ""
	for i, part := range parts {
		if part == "meow" || part == "close" || strings.HasPrefix(part, "--") {
			continue
		}
		beadID = parts[i]
		break
	}

	if beadID == "" && m.CurrentBead != "" {
		beadID = m.CurrentBead
	}

	if beadID != "" {
		m.CompletedBeads = append(m.CompletedBeads, beadID)
		m.CurrentBead = ""
	}

	return fmt.Sprintf("Closed bead: %s", beadID), 0
}

// handleMeowStatus simulates the meow status command.
func (m *MockClaude) handleMeowStatus(command string) (string, int) {
	return fmt.Sprintf(`Session: %s
Current Bead: %s
Completed: %d beads`, m.SessionID, m.CurrentBead, len(m.CompletedBeads)), 0
}

// SetCurrentBead sets the current bead being worked on.
func (m *MockClaude) SetCurrentBead(beadID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CurrentBead = beadID
}

// GetCompletedBeads returns all beads that were closed.
func (m *MockClaude) GetCompletedBeads() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]string, len(m.CompletedBeads))
	copy(result, m.CompletedBeads)
	return result
}

// recordInteraction records an interaction (must be called with lock held).
func (m *MockClaude) recordInteraction(cmd, output string, exitCode int, err error) {
	m.Interactions = append(m.Interactions, MockInteraction{
		Command:   cmd,
		Response:  output,
		ExitCode:  exitCode,
		Timestamp: time.Now(),
		Error:     err,
	})
}

// GetInteractions returns all recorded interactions.
func (m *MockClaude) GetInteractions() []MockInteraction {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make([]MockInteraction, len(m.Interactions))
	copy(result, m.Interactions)
	return result
}

// Reset clears all state.
func (m *MockClaude) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Responses = make(map[string]MockResponse)
	m.Interactions = make([]MockInteraction, 0)
	m.CompletedBeads = make([]string, 0)
	m.CurrentBead = ""
	m.ResponseDelay = 0
	m.FailOnCommand = ""
	m.FailError = ""
}

// AssertCommandExecuted asserts that a command was executed.
func (m *MockClaude) AssertCommandExecuted(t *testing.T, expectedCmd string) {
	t.Helper()

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, interaction := range m.Interactions {
		if strings.Contains(interaction.Command, expectedCmd) {
			return
		}
	}
	t.Errorf("Expected command %q to be executed, but it wasn't", expectedCmd)
}

// AssertBeadCompleted asserts that a bead was closed.
func (m *MockClaude) AssertBeadCompleted(t *testing.T, beadID string) {
	t.Helper()

	completed := m.GetCompletedBeads()
	for _, id := range completed {
		if id == beadID {
			return
		}
	}
	t.Errorf("Expected bead %q to be completed, but it wasn't. Completed: %v", beadID, completed)
}

// MockClaudeScript creates a shell script that simulates Claude Code behavior.
// This can be used for integration tests that spawn actual processes.
func MockClaudeScript(t *testing.T, dir string) string {
	t.Helper()

	scriptPath := filepath.Join(dir, "mock_claude.sh")
	script := `#!/bin/bash
# Mock Claude Code script for testing
# Simulates Claude Code behavior without actual AI

set -e

# Parse arguments
BEAD_ID=""
NOTES="Mock completed"

while [[ $# -gt 0 ]]; do
    case $1 in
        --dangerously-skip-permissions)
            shift
            ;;
        --resume)
            shift
            shift
            ;;
        *)
            shift
            ;;
    esac
done

# Enter mock REPL
while IFS= read -r line; do
    case "$line" in
        "meow prime"*)
            if [[ "$line" == *"--format json"* ]]; then
                echo '{"bead_id":"bd-test-001","session_id":"mock-session","instructions":"Test task"}'
            else
                echo "Ready to work on test task"
            fi
            ;;
        "meow close"*)
            # Extract bead ID
            BEAD_ID=$(echo "$line" | grep -oE 'bd-[a-zA-Z0-9-]+' || echo "bd-test-001")
            echo "Closed bead: $BEAD_ID"
            ;;
        "meow status")
            echo "Session: mock-session"
            echo "Current Bead: bd-test-001"
            ;;
        "meow session-id")
            echo "mock-session-$(date +%s)"
            ;;
        "exit"|"quit")
            exit 0
            ;;
        *)
            echo "Mock Claude: $line"
            ;;
    esac
done
`

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock Claude script: %v", err)
	}

	return scriptPath
}

// MockClaudeWithBehavior creates a mock Claude script with configurable behavior.
func MockClaudeWithBehavior(t *testing.T, dir string, behavior MockClaudeBehavior) string {
	t.Helper()

	scriptPath := filepath.Join(dir, "mock_claude.sh")

	// Build response handlers
	primeResponse := behavior.PrimeResponse
	if primeResponse == "" {
		primeResponse = `{"bead_id":"bd-test-001","session_id":"mock-session","instructions":"Test task"}`
	}

	closeResponse := behavior.CloseResponse
	if closeResponse == "" {
		closeResponse = "Closed bead"
	}

	script := fmt.Sprintf(`#!/bin/bash
# Configurable Mock Claude Code script

set -e

FAIL_ON_PRIME=%t
FAIL_ON_CLOSE=%t
DELAY_MS=%d

while IFS= read -r line; do
    if [[ $DELAY_MS -gt 0 ]]; then
        sleep $(echo "scale=3; $DELAY_MS/1000" | bc)
    fi

    case "$line" in
        "meow prime"*)
            if $FAIL_ON_PRIME; then
                echo "Error: Mock prime failure" >&2
                exit 1
            fi
            echo '%s'
            ;;
        "meow close"*)
            if $FAIL_ON_CLOSE; then
                echo "Error: Mock close failure" >&2
                exit 1
            fi
            echo '%s'
            ;;
        "exit"|"quit")
            exit 0
            ;;
        *)
            echo "Mock Claude: $line"
            ;;
    esac
done
`, behavior.FailOnPrime, behavior.FailOnClose, behavior.DelayMs, primeResponse, closeResponse)

	if err := os.WriteFile(scriptPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock Claude script: %v", err)
	}

	return scriptPath
}

// MockClaudeBehavior configures mock Claude script behavior.
type MockClaudeBehavior struct {
	FailOnPrime   bool
	FailOnClose   bool
	DelayMs       int
	PrimeResponse string
	CloseResponse string
}
