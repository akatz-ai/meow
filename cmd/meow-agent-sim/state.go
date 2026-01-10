package main

import (
	"bufio"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// Simulator is the main state machine for the Claude Code simulator.
type Simulator struct {
	config SimConfig
	logger *slog.Logger
	state  State
	ipc    IPCClientInterface

	// Environment (from orchestrator)
	agentID    string
	workflowID string
	stepID     string

	// State tracking for fail_then_succeed
	attemptCounts map[string]int
}

// NewSimulator creates a new simulator instance.
func NewSimulator(config SimConfig, logger *slog.Logger) *Simulator {
	return &Simulator{
		config:        config,
		logger:        logger,
		state:         StateStarting,
		ipc:           NewStubIPCClient(os.Getenv("MEOW_ORCH_SOCK")),
		agentID:       os.Getenv("MEOW_AGENT"),
		workflowID:    os.Getenv("MEOW_WORKFLOW"),
		stepID:        os.Getenv("MEOW_STEP"),
		attemptCounts: make(map[string]int),
	}
}

// Run executes the simulator main loop.
func (s *Simulator) Run() error {
	// Startup phase
	s.logger.Debug("startup phase",
		"state", s.state,
		"delay", s.config.Timing.StartupDelay,
	)

	// Print startup messages (like Claude Code does)
	fmt.Println("Claude Code Simulator v1.0")
	fmt.Println("Ready for prompts...")

	time.Sleep(s.config.Timing.StartupDelay)

	s.transitionTo(StateIdle)

	// Main loop
	reader := bufio.NewReader(os.Stdin)
	for {
		if s.state == StateIdle || s.state == StateAsking {
			// Small delay before showing prompt
			time.Sleep(s.config.Timing.PromptDelay)
			s.showPrompt()

			// Fire stop hook if configured and in IDLE state (not ASKING)
			if s.config.Hooks.FireStopHook && s.state == StateIdle {
				s.fireStopHook()
			}
		}

		// Read input
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				s.logger.Info("stdin closed, exiting")
				return nil
			}
			return fmt.Errorf("reading input: %w", err)
		}

		prompt := strings.TrimSpace(line)
		if prompt == "" {
			continue
		}

		s.logger.Debug("received input",
			"prompt", truncate(prompt, 100),
			"state", s.state,
		)

		// Handle input based on current state
		if err := s.handleInput(prompt); err != nil {
			s.logger.Error("handling input", "error", err)
		}
	}
}

// transitionTo changes the simulator state with logging.
func (s *Simulator) transitionTo(newState State) {
	s.logger.Debug("state transition",
		"from", s.state,
		"to", newState,
	)
	s.state = newState
}

// showPrompt displays the prompt indicator.
func (s *Simulator) showPrompt() {
	fmt.Print("> ")
	os.Stdout.Sync() // Flush to ensure prompt is visible
}

// handleInput processes input based on current state.
func (s *Simulator) handleInput(prompt string) error {
	switch s.state {
	case StateIdle:
		// Normal prompt from orchestrator
		s.transitionTo(StateWorking)
		behavior := s.matchBehavior(prompt)
		return s.executeBehavior(behavior, prompt)

	case StateAsking:
		// Response to a question we asked
		s.logger.Debug("received answer to question", "answer", truncate(prompt, 50))
		s.transitionTo(StateWorking)
		// After getting an answer, we complete successfully
		return s.actionComplete(Action{
			Type:    ActionComplete,
			Delay:   s.config.Timing.DefaultWorkDelay,
			Outputs: map[string]any{"answer": prompt},
		})

	case StateWorking:
		// Unexpected input while working - log and ignore
		s.logger.Warn("received input while working, ignoring",
			"prompt", truncate(prompt, 50),
		)
		return nil

	case StateStarting:
		// Shouldn't receive input during startup
		s.logger.Warn("received input during startup, ignoring",
			"prompt", truncate(prompt, 50),
		)
		return nil

	default:
		return fmt.Errorf("unknown state: %s", s.state)
	}
}

// fireStopHook emulates the Claude Code stop hook behavior.
// When transitioning to IDLE, this fires:
// 1. meow event agent-stopped
// 2. meow prime --format prompt (to check for next prompt)
func (s *Simulator) fireStopHook() {
	s.logger.Debug("firing stop hook")

	// Fire agent-stopped event
	if err := s.ipc.Event("agent-stopped", nil); err != nil {
		s.logger.Debug("agent-stopped event failed", "error", err)
	}

	// Check for next prompt from orchestrator
	prompt, err := s.ipc.GetPrompt()
	if err != nil {
		s.logger.Debug("get prompt failed", "error", err)
		return
	}

	// If we got a prompt, self-inject it
	if prompt != "" {
		s.logger.Debug("got prompt from orchestrator, self-injecting",
			"prompt", truncate(prompt, 50),
		)
		// Process the prompt as if it came from stdin
		if err := s.handleInput(prompt); err != nil {
			s.logger.Error("handling injected prompt", "error", err)
		}
	}
}

// truncate shortens a string to max length, adding "..." if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// ============================================================================
// Stub IPC Client Implementation
// Will be replaced by Track 3 (meow-vvr9)
// ============================================================================

// StubIPCClient is a stub implementation of IPCClientInterface.
// Track 3 will implement the real IPC client.
type StubIPCClient struct {
	socketPath string
}

// NewStubIPCClient creates a stub IPC client.
func NewStubIPCClient(socketPath string) *StubIPCClient {
	return &StubIPCClient{socketPath: socketPath}
}

// StepDone signals step completion to the orchestrator.
func (c *StubIPCClient) StepDone(outputs map[string]any) error {
	// STUB: Log the call for debugging
	slog.Debug("IPC: meow done", "outputs", outputs)
	return nil
}

// GetPrompt retrieves the current prompt from the orchestrator.
func (c *StubIPCClient) GetPrompt() (string, error) {
	// STUB: Return empty (no prompt available)
	slog.Debug("IPC: meow prime")
	return "", nil
}

// Event sends an event to the orchestrator.
func (c *StubIPCClient) Event(eventType string, data map[string]any) error {
	// STUB: Log the call for debugging
	slog.Debug("IPC: meow event", "type", eventType, "data", data)
	return nil
}

// Close closes the IPC connection.
func (c *StubIPCClient) Close() error {
	return nil
}
