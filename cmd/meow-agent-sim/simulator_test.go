package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mockIPCClient implements IPCClientInterface for testing.
type mockIPCClient struct {
	stepDoneCalls  []map[string]any
	eventCalls     []mockEvent
	promptResponse string
	promptError    error
	stepDoneError  error
}

type mockEvent struct {
	eventType string
	data      map[string]any
}

func newMockIPCClient() *mockIPCClient {
	return &mockIPCClient{
		stepDoneCalls: []map[string]any{},
		eventCalls:    []mockEvent{},
	}
}

func (m *mockIPCClient) StepDone(outputs map[string]any) error {
	m.stepDoneCalls = append(m.stepDoneCalls, outputs)
	return m.stepDoneError
}

func (m *mockIPCClient) GetPrompt() (string, error) {
	return m.promptResponse, m.promptError
}

func (m *mockIPCClient) Event(eventType string, data map[string]any) error {
	m.eventCalls = append(m.eventCalls, mockEvent{eventType: eventType, data: data})
	return nil
}

func (m *mockIPCClient) Close() error {
	return nil
}

// newTestSimulator creates a simulator with mock IPC for testing.
func newTestSimulator(config SimConfig) (*Simulator, *mockIPCClient) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	mock := newMockIPCClient()
	sim := &Simulator{
		config:        config,
		logger:        logger,
		state:         StateStarting,
		ipc:           mock,
		attemptCounts: make(map[string]int),
	}
	return sim, mock
}

// =============================================================================
// TestLoadConfig - Test YAML config loading
// =============================================================================

func TestLoadConfig_ValidFile(t *testing.T) {
	// Create a temp config file
	content := `
timing:
  startup_delay: 200ms
  default_work_delay: 50ms
  prompt_delay: 5ms
hooks:
  fire_stop_hook: false
  fire_tool_events: true
behaviors:
  - match: "test prompt"
    type: contains
    action:
      type: complete
      delay: 10ms
      outputs:
        result: "success"
default:
  behavior:
    action:
      type: complete
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify timing
	if config.Timing.StartupDelay != 200*time.Millisecond {
		t.Errorf("StartupDelay = %v, want 200ms", config.Timing.StartupDelay)
	}
	if config.Timing.DefaultWorkDelay != 50*time.Millisecond {
		t.Errorf("DefaultWorkDelay = %v, want 50ms", config.Timing.DefaultWorkDelay)
	}
	if config.Timing.PromptDelay != 5*time.Millisecond {
		t.Errorf("PromptDelay = %v, want 5ms", config.Timing.PromptDelay)
	}

	// Verify hooks
	if config.Hooks.FireStopHook != false {
		t.Errorf("FireStopHook = %v, want false", config.Hooks.FireStopHook)
	}
	if config.Hooks.FireToolEvents != true {
		t.Errorf("FireToolEvents = %v, want true", config.Hooks.FireToolEvents)
	}

	// Verify behaviors
	if len(config.Behaviors) != 1 {
		t.Fatalf("len(Behaviors) = %d, want 1", len(config.Behaviors))
	}
	if config.Behaviors[0].Match != "test prompt" {
		t.Errorf("Behaviors[0].Match = %q, want %q", config.Behaviors[0].Match, "test prompt")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("LoadConfig should fail for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.yaml")
	content := `
timing:
  startup_delay: not-a-duration
  this is: [invalid yaml
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	_, err := LoadConfig(configPath)
	if err == nil {
		t.Fatal("LoadConfig should fail for invalid YAML")
	}
}

func TestLoadConfig_DefaultValues(t *testing.T) {
	// Minimal config - should use defaults
	content := `
behaviors: []
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "minimal.yaml")
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	config, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Check that defaults were applied
	defaults := NewDefaultSimConfig()
	if config.Timing.StartupDelay != defaults.Timing.StartupDelay {
		t.Errorf("StartupDelay = %v, want default %v", config.Timing.StartupDelay, defaults.Timing.StartupDelay)
	}
	if config.Hooks.FireStopHook != defaults.Hooks.FireStopHook {
		t.Errorf("FireStopHook = %v, want default %v", config.Hooks.FireStopHook, defaults.Hooks.FireStopHook)
	}
}

// =============================================================================
// TestBehaviorMatching - Test behavior pattern matching
// =============================================================================

func TestBehaviorMatching_Contains(t *testing.T) {
	config := SimConfig{
		Behaviors: []Behavior{
			{
				Match: "hello",
				Type:  "contains",
				Action: Action{
					Type:    ActionComplete,
					Outputs: map[string]any{"matched": "hello"},
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, _ := newTestSimulator(config)

	tests := []struct {
		prompt  string
		matched bool
	}{
		{"hello world", true},
		{"say hello there", true},
		{"HELLO", false}, // case-sensitive
		{"goodbye", false},
	}

	for _, tt := range tests {
		b := sim.matchBehavior(tt.prompt)
		isMatch := b.Match == "hello"
		if isMatch != tt.matched {
			t.Errorf("matchBehavior(%q): matched=%v, want %v", tt.prompt, isMatch, tt.matched)
		}
	}
}

func TestBehaviorMatching_Regex(t *testing.T) {
	config := SimConfig{
		Behaviors: []Behavior{
			{
				Match: `^task-\d+$`,
				Type:  "regex",
				Action: Action{
					Type:    ActionComplete,
					Outputs: map[string]any{"matched": "task"},
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, _ := newTestSimulator(config)

	tests := []struct {
		prompt  string
		matched bool
	}{
		{"task-123", true},
		{"task-1", true},
		{"task-abc", false},
		{"prefix-task-123", false},
		{"task-123-suffix", false},
	}

	for _, tt := range tests {
		b := sim.matchBehavior(tt.prompt)
		isMatch := b.Match == `^task-\d+$`
		if isMatch != tt.matched {
			t.Errorf("matchBehavior(%q): matched=%v, want %v", tt.prompt, isMatch, tt.matched)
		}
	}
}

func TestBehaviorMatching_FirstMatchWins(t *testing.T) {
	config := SimConfig{
		Behaviors: []Behavior{
			{
				Match: "first",
				Type:  "contains",
				Action: Action{
					Type:    ActionComplete,
					Outputs: map[string]any{"winner": "first"},
				},
			},
			{
				Match: "first",
				Type:  "contains",
				Action: Action{
					Type:    ActionComplete,
					Outputs: map[string]any{"winner": "second"},
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, _ := newTestSimulator(config)
	b := sim.matchBehavior("first match test")

	// Should match the first behavior
	if b.Action.Outputs["winner"] != "first" {
		t.Errorf("Expected first behavior to win, got outputs: %v", b.Action.Outputs)
	}
}

func TestBehaviorMatching_DefaultFallback(t *testing.T) {
	config := SimConfig{
		Behaviors: []Behavior{
			{
				Match: "specific",
				Type:  "contains",
				Action: Action{
					Type:    ActionComplete,
					Outputs: map[string]any{"matched": "specific"},
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Match: "",
				Action: Action{
					Type:    ActionComplete,
					Outputs: map[string]any{"matched": "default"},
				},
			},
		},
	}

	sim, _ := newTestSimulator(config)

	// Should fall back to default
	b := sim.matchBehavior("no match here")
	if b.Action.Outputs["matched"] != "default" {
		t.Errorf("Expected default fallback, got outputs: %v", b.Action.Outputs)
	}
}

func TestBehaviorMatching_InvalidRegex(t *testing.T) {
	config := SimConfig{
		Behaviors: []Behavior{
			{
				Match: "[invalid(regex",
				Type:  "regex",
				Action: Action{
					Type: ActionComplete,
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{
					Type:    ActionComplete,
					Outputs: map[string]any{"matched": "default"},
				},
			},
		},
	}

	sim, _ := newTestSimulator(config)

	// Invalid regex should not match, fall back to default
	b := sim.matchBehavior("anything")
	if b.Action.Outputs["matched"] != "default" {
		t.Errorf("Invalid regex should fall back to default, got outputs: %v", b.Action.Outputs)
	}
}

func TestBehaviorMatching_EmptyMatchType(t *testing.T) {
	// Empty type should default to "contains"
	config := SimConfig{
		Behaviors: []Behavior{
			{
				Match: "hello",
				Type:  "", // Empty - should default to contains
				Action: Action{
					Type:    ActionComplete,
					Outputs: map[string]any{"matched": "hello"},
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, _ := newTestSimulator(config)
	b := sim.matchBehavior("hello world")

	if b.Match != "hello" {
		t.Errorf("Empty type should default to contains match, got match: %q", b.Match)
	}
}

// =============================================================================
// TestStateTransitions - Test state machine
// =============================================================================

func TestStateTransitions_StartingToIdle(t *testing.T) {
	config := NewDefaultSimConfig()
	sim, _ := newTestSimulator(config)

	if sim.state != StateStarting {
		t.Errorf("Initial state = %v, want %v", sim.state, StateStarting)
	}

	sim.transitionTo(StateIdle)
	if sim.state != StateIdle {
		t.Errorf("After transition, state = %v, want %v", sim.state, StateIdle)
	}
}

func TestStateTransitions_IdleToWorkingToIdle(t *testing.T) {
	config := SimConfig{
		Timing: TimingConfig{
			DefaultWorkDelay: 0, // No delay for tests
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{
					Type:    ActionComplete,
					Outputs: map[string]any{},
				},
			},
		},
	}

	sim, mock := newTestSimulator(config)
	sim.state = StateIdle

	// Handle input should transition to Working, then back to Idle
	err := sim.handleInput("test prompt")
	if err != nil {
		t.Fatalf("handleInput failed: %v", err)
	}

	// Should end in Idle state after completing
	if sim.state != StateIdle {
		t.Errorf("Final state = %v, want %v", sim.state, StateIdle)
	}

	// StepDone should have been called
	if len(mock.stepDoneCalls) != 1 {
		t.Errorf("StepDone called %d times, want 1", len(mock.stepDoneCalls))
	}
}

func TestStateTransitions_IdleToWorkingToAskingToWorkingToIdle(t *testing.T) {
	config := SimConfig{
		Timing: TimingConfig{
			DefaultWorkDelay: 0,
		},
		Behaviors: []Behavior{
			{
				Match: "ask me",
				Type:  "contains",
				Action: Action{
					Type:     ActionAsk,
					Question: "What is your answer?",
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{
					Type: ActionComplete,
				},
			},
		},
	}

	sim, mock := newTestSimulator(config)
	sim.state = StateIdle

	// First input should trigger ask behavior
	err := sim.handleInput("ask me something")
	if err != nil {
		t.Fatalf("handleInput (ask) failed: %v", err)
	}

	// Should be in Asking state
	if sim.state != StateAsking {
		t.Errorf("After ask, state = %v, want %v", sim.state, StateAsking)
	}

	// Provide answer
	err = sim.handleInput("my answer")
	if err != nil {
		t.Fatalf("handleInput (answer) failed: %v", err)
	}

	// Should end in Idle state
	if sim.state != StateIdle {
		t.Errorf("Final state = %v, want %v", sim.state, StateIdle)
	}

	// StepDone should have been called with the answer
	if len(mock.stepDoneCalls) != 1 {
		t.Fatalf("StepDone called %d times, want 1", len(mock.stepDoneCalls))
	}

	outputs := mock.stepDoneCalls[0]
	if outputs["answer"] != "my answer" {
		t.Errorf("Answer output = %v, want %q", outputs["answer"], "my answer")
	}
}

func TestStateTransitions_IgnoreInputWhileWorking(t *testing.T) {
	config := NewDefaultSimConfig()
	sim, _ := newTestSimulator(config)
	sim.state = StateWorking

	// Input while working should be ignored
	err := sim.handleInput("ignored prompt")
	if err != nil {
		t.Fatalf("handleInput failed: %v", err)
	}

	// State should remain Working
	if sim.state != StateWorking {
		t.Errorf("State = %v, want %v", sim.state, StateWorking)
	}
}

func TestStateTransitions_IgnoreInputWhileStarting(t *testing.T) {
	config := NewDefaultSimConfig()
	sim, _ := newTestSimulator(config)
	sim.state = StateStarting

	// Input while starting should be ignored
	err := sim.handleInput("ignored prompt")
	if err != nil {
		t.Fatalf("handleInput failed: %v", err)
	}

	// State should remain Starting
	if sim.state != StateStarting {
		t.Errorf("State = %v, want %v", sim.state, StateStarting)
	}
}

// =============================================================================
// TestActionTypes - Test action execution
// =============================================================================

func TestActionComplete(t *testing.T) {
	config := SimConfig{
		Timing: TimingConfig{
			DefaultWorkDelay: 0,
		},
		Behaviors: []Behavior{
			{
				Match: "complete",
				Type:  "contains",
				Action: Action{
					Type: ActionComplete,
					Outputs: map[string]any{
						"status": "done",
						"value":  42,
					},
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, mock := newTestSimulator(config)
	sim.state = StateIdle

	err := sim.handleInput("complete this task")
	if err != nil {
		t.Fatalf("handleInput failed: %v", err)
	}

	if len(mock.stepDoneCalls) != 1 {
		t.Fatalf("StepDone called %d times, want 1", len(mock.stepDoneCalls))
	}

	outputs := mock.stepDoneCalls[0]
	if outputs["status"] != "done" {
		t.Errorf("outputs[status] = %v, want %q", outputs["status"], "done")
	}
	if outputs["value"] != 42 {
		t.Errorf("outputs[value] = %v, want 42", outputs["value"])
	}

	if sim.state != StateIdle {
		t.Errorf("Final state = %v, want %v", sim.state, StateIdle)
	}
}

func TestActionAsk(t *testing.T) {
	config := SimConfig{
		Timing: TimingConfig{
			DefaultWorkDelay: 0,
		},
		Behaviors: []Behavior{
			{
				Match: "question",
				Type:  "contains",
				Action: Action{
					Type:     ActionAsk,
					Question: "What is your favorite color?",
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, mock := newTestSimulator(config)
	sim.state = StateIdle

	err := sim.handleInput("I have a question")
	if err != nil {
		t.Fatalf("handleInput failed: %v", err)
	}

	// Should be in Asking state
	if sim.state != StateAsking {
		t.Errorf("State = %v, want %v", sim.state, StateAsking)
	}

	// StepDone should NOT have been called yet
	if len(mock.stepDoneCalls) != 0 {
		t.Errorf("StepDone called %d times, want 0", len(mock.stepDoneCalls))
	}
}

func TestActionFail(t *testing.T) {
	config := SimConfig{
		Timing: TimingConfig{
			DefaultWorkDelay: 0,
		},
		Behaviors: []Behavior{
			{
				Match: "fail",
				Type:  "contains",
				Action: Action{
					Type:        ActionFail,
					FailMessage: "Something went wrong",
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, mock := newTestSimulator(config)
	sim.state = StateIdle

	err := sim.handleInput("fail this task")
	if err != nil {
		t.Fatalf("handleInput failed: %v", err)
	}

	// Should be back in Idle state
	if sim.state != StateIdle {
		t.Errorf("State = %v, want %v", sim.state, StateIdle)
	}

	// StepDone should NOT have been called (failure doesn't call done)
	if len(mock.stepDoneCalls) != 0 {
		t.Errorf("StepDone called %d times, want 0", len(mock.stepDoneCalls))
	}
}

func TestActionFailThenSucceed(t *testing.T) {
	config := SimConfig{
		Timing: TimingConfig{
			DefaultWorkDelay: 0,
		},
		Behaviors: []Behavior{
			{
				Match: "retry",
				Type:  "contains",
				Action: Action{
					Type:        ActionFailThenSucceed,
					FailCount:   2,
					FailMessage: "Simulated failure",
					Outputs:     map[string]any{"status": "succeeded"},
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, mock := newTestSimulator(config)

	// First attempt - should fail
	sim.state = StateIdle
	err := sim.handleInput("retry this")
	if err != nil {
		t.Fatalf("handleInput (attempt 1) failed: %v", err)
	}
	if len(mock.stepDoneCalls) != 0 {
		t.Errorf("After attempt 1: StepDone called %d times, want 0", len(mock.stepDoneCalls))
	}

	// Second attempt - should fail
	sim.state = StateIdle
	err = sim.handleInput("retry this")
	if err != nil {
		t.Fatalf("handleInput (attempt 2) failed: %v", err)
	}
	if len(mock.stepDoneCalls) != 0 {
		t.Errorf("After attempt 2: StepDone called %d times, want 0", len(mock.stepDoneCalls))
	}

	// Third attempt - should succeed
	sim.state = StateIdle
	err = sim.handleInput("retry this")
	if err != nil {
		t.Fatalf("handleInput (attempt 3) failed: %v", err)
	}
	if len(mock.stepDoneCalls) != 1 {
		t.Fatalf("After attempt 3: StepDone called %d times, want 1", len(mock.stepDoneCalls))
	}

	outputs := mock.stepDoneCalls[0]
	if outputs["status"] != "succeeded" {
		t.Errorf("outputs[status] = %v, want %q", outputs["status"], "succeeded")
	}
}

func TestActionFailThenSucceed_CounterReset(t *testing.T) {
	config := SimConfig{
		Timing: TimingConfig{
			DefaultWorkDelay: 0,
		},
		Behaviors: []Behavior{
			{
				Match: "retry",
				Type:  "contains",
				Action: Action{
					Type:      ActionFailThenSucceed,
					FailCount: 1,
					Outputs:   map[string]any{"status": "ok"},
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, mock := newTestSimulator(config)

	// First cycle: fail once, then succeed
	sim.state = StateIdle
	sim.handleInput("retry")
	if len(mock.stepDoneCalls) != 0 {
		t.Errorf("Cycle 1, attempt 1: StepDone called %d times, want 0", len(mock.stepDoneCalls))
	}

	sim.state = StateIdle
	sim.handleInput("retry")
	if len(mock.stepDoneCalls) != 1 {
		t.Errorf("Cycle 1, attempt 2: StepDone called %d times, want 1", len(mock.stepDoneCalls))
	}

	// Second cycle: counter should have reset, should fail first, then succeed
	sim.state = StateIdle
	sim.handleInput("retry")
	if len(mock.stepDoneCalls) != 1 {
		t.Errorf("Cycle 2, attempt 1: StepDone called %d times, want 1", len(mock.stepDoneCalls))
	}

	sim.state = StateIdle
	sim.handleInput("retry")
	if len(mock.stepDoneCalls) != 2 {
		t.Errorf("Cycle 2, attempt 2: StepDone called %d times, want 2", len(mock.stepDoneCalls))
	}
}

// =============================================================================
// TestTruncate - Test helper function
// =============================================================================

func TestTruncate(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello world", 8, "hello..."},
		{"hello", 5, "hello"},
		{"hello", 4, "h..."},
		{"hello", 3, "hel"},
		{"hello", 2, "he"},
		{"hello", 1, "h"},
		{"hello", 0, ""},
		{"", 10, ""},
	}

	for _, tt := range tests {
		got := truncate(tt.input, tt.maxLen)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
		}
	}
}

// =============================================================================
// TestMatches - Test matches helper function directly
// =============================================================================

func TestMatches(t *testing.T) {
	tests := []struct {
		behavior *Behavior
		prompt   string
		want     bool
	}{
		// Contains matches
		{&Behavior{Match: "hello", Type: "contains"}, "hello world", true},
		{&Behavior{Match: "hello", Type: "contains"}, "HELLO world", false},
		{&Behavior{Match: "hello", Type: "contains"}, "goodbye", false},

		// Regex matches
		{&Behavior{Match: `^\d+$`, Type: "regex"}, "123", true},
		{&Behavior{Match: `^\d+$`, Type: "regex"}, "abc", false},
		{&Behavior{Match: `task-\w+`, Type: "regex"}, "task-abc", true},

		// Empty type defaults to contains
		{&Behavior{Match: "hello", Type: ""}, "hello world", true},

		// Unknown type defaults to contains
		{&Behavior{Match: "hello", Type: "unknown"}, "hello world", true},
	}

	for _, tt := range tests {
		got := matches(tt.behavior, tt.prompt)
		if got != tt.want {
			t.Errorf("matches(%+v, %q) = %v, want %v", tt.behavior, tt.prompt, got, tt.want)
		}
	}
}

// =============================================================================
// TestToolEvents - Test tool event emission
// =============================================================================

func TestToolEvents_EmittedWhenEnabled(t *testing.T) {
	config := SimConfig{
		Timing: TimingConfig{
			DefaultWorkDelay: 0,
		},
		Hooks: HooksConfig{
			FireToolEvents: true,
		},
		Behaviors: []Behavior{
			{
				Match: "with events",
				Type:  "contains",
				Action: Action{
					Type: ActionComplete,
					Events: []EventDef{
						{Type: "file_written", Data: map[string]any{"path": "/tmp/test.txt"}},
						{Type: "command_run", Data: map[string]any{"cmd": "echo hello"}},
					},
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, mock := newTestSimulator(config)
	sim.state = StateIdle

	err := sim.handleInput("with events please")
	if err != nil {
		t.Fatalf("handleInput failed: %v", err)
	}

	if len(mock.eventCalls) != 2 {
		t.Fatalf("Event called %d times, want 2", len(mock.eventCalls))
	}

	if mock.eventCalls[0].eventType != "file_written" {
		t.Errorf("Event[0].type = %q, want %q", mock.eventCalls[0].eventType, "file_written")
	}
	if mock.eventCalls[1].eventType != "command_run" {
		t.Errorf("Event[1].type = %q, want %q", mock.eventCalls[1].eventType, "command_run")
	}
}

func TestToolEvents_NotEmittedWhenDisabled(t *testing.T) {
	config := SimConfig{
		Timing: TimingConfig{
			DefaultWorkDelay: 0,
		},
		Hooks: HooksConfig{
			FireToolEvents: false, // Disabled
		},
		Behaviors: []Behavior{
			{
				Match: "with events",
				Type:  "contains",
				Action: Action{
					Type: ActionComplete,
					Events: []EventDef{
						{Type: "file_written", Data: map[string]any{"path": "/tmp/test.txt"}},
					},
				},
			},
		},
		Default: DefaultConfig{
			Behavior: Behavior{
				Action: Action{Type: ActionComplete},
			},
		},
	}

	sim, mock := newTestSimulator(config)
	sim.state = StateIdle

	err := sim.handleInput("with events please")
	if err != nil {
		t.Fatalf("handleInput failed: %v", err)
	}

	if len(mock.eventCalls) != 0 {
		t.Errorf("Event called %d times, want 0 (events disabled)", len(mock.eventCalls))
	}
}

// =============================================================================
// TestNewDefaultSimConfig - Test default configuration
// =============================================================================

func TestNewDefaultSimConfig(t *testing.T) {
	config := NewDefaultSimConfig()

	// Verify timing defaults
	if config.Timing.StartupDelay != 100*time.Millisecond {
		t.Errorf("StartupDelay = %v, want 100ms", config.Timing.StartupDelay)
	}
	if config.Timing.DefaultWorkDelay != 100*time.Millisecond {
		t.Errorf("DefaultWorkDelay = %v, want 100ms", config.Timing.DefaultWorkDelay)
	}
	if config.Timing.PromptDelay != 10*time.Millisecond {
		t.Errorf("PromptDelay = %v, want 10ms", config.Timing.PromptDelay)
	}

	// Verify hooks defaults
	if config.Hooks.FireStopHook != true {
		t.Errorf("FireStopHook = %v, want true", config.Hooks.FireStopHook)
	}
	if config.Hooks.FireToolEvents != true {
		t.Errorf("FireToolEvents = %v, want true", config.Hooks.FireToolEvents)
	}

	// Verify default behavior
	if config.Default.Behavior.Action.Type != ActionComplete {
		t.Errorf("Default action type = %v, want %v", config.Default.Behavior.Action.Type, ActionComplete)
	}

	// Verify logging defaults
	if config.Logging.Level != "info" {
		t.Errorf("Logging.Level = %q, want %q", config.Logging.Level, "info")
	}
}
