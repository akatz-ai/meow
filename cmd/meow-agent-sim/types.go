package main

import "time"

// State represents the simulator's current state
type State string

const (
    StateStarting State = "starting"
    StateIdle     State = "idle"
    StateWorking  State = "working"
    StateAsking   State = "asking"
)

// SimConfig holds the complete simulator configuration
type SimConfig struct {
    Timing    TimingConfig   `yaml:"timing"`
    Hooks     HooksConfig    `yaml:"hooks"`
    Behaviors []Behavior     `yaml:"behaviors"`
    Default   DefaultConfig  `yaml:"default"`
    Logging   LoggingConfig  `yaml:"logging"`
}

type TimingConfig struct {
    StartupDelay     time.Duration `yaml:"startup_delay"`
    DefaultWorkDelay time.Duration `yaml:"default_work_delay"`
    PromptDelay      time.Duration `yaml:"prompt_delay"`
}

type HooksConfig struct {
    FireStopHook   bool `yaml:"fire_stop_hook"`
    FireToolEvents bool `yaml:"fire_tool_events"`
}

type DefaultConfig struct {
    Behavior Behavior `yaml:"behavior"`
}

type LoggingConfig struct {
    Level  string `yaml:"level"`
    Format string `yaml:"format"`
}

// ActionType defines what the simulator does when a prompt matches
type ActionType string

const (
    ActionComplete        ActionType = "complete"
    ActionAsk             ActionType = "ask"
    ActionFail            ActionType = "fail"
    ActionFailThenSucceed ActionType = "fail_then_succeed"
    ActionHang            ActionType = "hang"
    ActionCrash           ActionType = "crash"
)

// Behavior defines how the simulator responds to a prompt pattern
type Behavior struct {
    Match  string `yaml:"match"`
    Type   string `yaml:"type"` // "contains" or "regex"
    Action Action `yaml:"action"`
}

// Action defines the simulator's response action
type Action struct {
    Type            ActionType       `yaml:"type"`
    Delay           time.Duration    `yaml:"delay"`
    Outputs         map[string]any   `yaml:"outputs"`
    OutputsSequence []map[string]any `yaml:"outputs_sequence"` // For sequence mode: different outputs per call
    Events          []EventDef       `yaml:"events"`
    Question        string           `yaml:"question"`
    FailCount       int              `yaml:"fail_count"`
    FailMessage     string           `yaml:"fail_message"`
    ExitCode        int              `yaml:"exit_code"`
}

// EventDef defines a tool event to emit
type EventDef struct {
    Type string         `yaml:"type"`
    Data map[string]any `yaml:"data"`
    When time.Duration  `yaml:"when"`
}

// IPCClientInterface defines the interface for orchestrator communication
type IPCClientInterface interface {
    StepDone(outputs map[string]any) error
    Event(eventType string, data map[string]any) error
    Close() error
}
