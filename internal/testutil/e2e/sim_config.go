// Package e2e provides E2E test infrastructure for MEOW workflows.
package e2e

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// SimTestConfig holds the complete simulator configuration.
// This mirrors the SimConfig from meow-agent-sim but is defined here
// to avoid import cycles and provide a test-focused API.
type SimTestConfig struct {
	Timing    TimingConfig   `yaml:"timing"`
	Hooks     HooksConfig    `yaml:"hooks"`
	Behaviors []Behavior     `yaml:"behaviors"`
	Default   DefaultConfig  `yaml:"default"`
	Logging   LoggingConfig  `yaml:"logging"`
}

// TimingConfig controls simulator timing.
type TimingConfig struct {
	StartupDelay     time.Duration `yaml:"startup_delay"`
	DefaultWorkDelay time.Duration `yaml:"default_work_delay"`
	PromptDelay      time.Duration `yaml:"prompt_delay"`
}

// HooksConfig controls simulator hook behavior.
type HooksConfig struct {
	FireStopHook   bool `yaml:"fire_stop_hook"`
	FireToolEvents bool `yaml:"fire_tool_events"`
}

// DefaultConfig provides default behavior settings.
type DefaultConfig struct {
	Behavior Behavior `yaml:"behavior"`
}

// LoggingConfig controls simulator logging.
type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Behavior defines how the simulator responds to a prompt pattern.
type Behavior struct {
	Match  string `yaml:"match"`
	Type   string `yaml:"type"` // "contains" or "regex"
	Action Action `yaml:"action"`
}

// ActionType defines what the simulator does when a prompt matches.
type ActionType string

const (
	ActionComplete        ActionType = "complete"
	ActionAsk             ActionType = "ask"
	ActionFail            ActionType = "fail"
	ActionFailThenSucceed ActionType = "fail_then_succeed"
	ActionHang            ActionType = "hang"
	ActionCrash           ActionType = "crash"
)

// Action defines the simulator's response action.
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

// EventDef defines a tool event to emit.
type EventDef struct {
	Type string         `yaml:"type"`
	Data map[string]any `yaml:"data"`
	When time.Duration  `yaml:"when"`
}

// SimConfigBuilder provides a fluent API for building simulator configs.
type SimConfigBuilder struct {
	config SimTestConfig
}

// NewSimConfigBuilder creates a new builder with sensible defaults.
func NewSimConfigBuilder() *SimConfigBuilder {
	return &SimConfigBuilder{
		config: SimTestConfig{
			Timing: TimingConfig{
				StartupDelay:     100 * time.Millisecond,
				DefaultWorkDelay: 10 * time.Millisecond,
				PromptDelay:      10 * time.Millisecond,
			},
			Hooks: HooksConfig{
				FireStopHook:   true,
				FireToolEvents: false,
			},
			Default: DefaultConfig{
				Behavior: Behavior{
					Action: Action{
						Type:  ActionComplete,
						Delay: 10 * time.Millisecond,
					},
				},
			},
			Logging: LoggingConfig{
				Level:  "info",
				Format: "json",
			},
		},
	}
}

// WithBehavior adds a behavior that matches prompts containing the pattern.
func (b *SimConfigBuilder) WithBehavior(pattern string, action ActionType) *SimConfigBuilder {
	behavior := Behavior{
		Match: pattern,
		Type:  "contains",
		Action: Action{
			Type:  action,
			Delay: 10 * time.Millisecond,
		},
	}
	b.config.Behaviors = append(b.config.Behaviors, behavior)
	return b
}

// WithRegexBehavior adds a behavior that matches prompts using a regex pattern.
func (b *SimConfigBuilder) WithRegexBehavior(pattern string, action ActionType) *SimConfigBuilder {
	behavior := Behavior{
		Match: pattern,
		Type:  "regex",
		Action: Action{
			Type:  action,
			Delay: 10 * time.Millisecond,
		},
	}
	b.config.Behaviors = append(b.config.Behaviors, behavior)
	return b
}

// WithBehaviorOutputs adds a behavior that produces outputs when matched.
func (b *SimConfigBuilder) WithBehaviorOutputs(pattern string, outputs map[string]any) *SimConfigBuilder {
	behavior := Behavior{
		Match: pattern,
		Type:  "contains",
		Action: Action{
			Type:    ActionComplete,
			Delay:   10 * time.Millisecond,
			Outputs: outputs,
		},
	}
	b.config.Behaviors = append(b.config.Behaviors, behavior)
	return b
}

// WithDelay sets the default work delay.
func (b *SimConfigBuilder) WithDelay(d time.Duration) *SimConfigBuilder {
	b.config.Timing.DefaultWorkDelay = d
	return b
}

// WithStartupDelay sets the startup delay.
func (b *SimConfigBuilder) WithStartupDelay(d time.Duration) *SimConfigBuilder {
	b.config.Timing.StartupDelay = d
	return b
}

// WithPromptDelay sets the prompt processing delay.
func (b *SimConfigBuilder) WithPromptDelay(d time.Duration) *SimConfigBuilder {
	b.config.Timing.PromptDelay = d
	return b
}

// WithDefaultAction sets the default action when no behavior matches.
func (b *SimConfigBuilder) WithDefaultAction(action ActionType) *SimConfigBuilder {
	b.config.Default.Behavior.Action.Type = action
	return b
}

// WithDefaultOutputs sets outputs for the default behavior.
func (b *SimConfigBuilder) WithDefaultOutputs(outputs map[string]any) *SimConfigBuilder {
	b.config.Default.Behavior.Action.Outputs = outputs
	return b
}

// WithStopHook enables or disables the stop hook.
func (b *SimConfigBuilder) WithStopHook(enabled bool) *SimConfigBuilder {
	b.config.Hooks.FireStopHook = enabled
	return b
}

// WithToolEvents enables or disables tool events.
func (b *SimConfigBuilder) WithToolEvents(enabled bool) *SimConfigBuilder {
	b.config.Hooks.FireToolEvents = enabled
	return b
}

// WithLogLevel sets the logging level.
func (b *SimConfigBuilder) WithLogLevel(level string) *SimConfigBuilder {
	b.config.Logging.Level = level
	return b
}

// WithHangBehavior adds a behavior that hangs forever (for timeout testing).
func (b *SimConfigBuilder) WithHangBehavior(pattern string) *SimConfigBuilder {
	behavior := Behavior{
		Match: pattern,
		Type:  "contains",
		Action: Action{
			Type: ActionHang,
		},
	}
	b.config.Behaviors = append(b.config.Behaviors, behavior)
	return b
}

// WithBehaviorSequence adds a behavior that produces different outputs on successive calls.
// The first call returns outputs[0], second returns outputs[1], etc.
// After the sequence is exhausted, the last output is repeated.
func (b *SimConfigBuilder) WithBehaviorSequence(pattern string, outputs []map[string]any) *SimConfigBuilder {
	behavior := Behavior{
		Match: pattern,
		Type:  "contains",
		Action: Action{
			Type:            ActionComplete,
			Delay:           10 * time.Millisecond,
			OutputsSequence: outputs,
		},
	}
	b.config.Behaviors = append(b.config.Behaviors, behavior)
	return b
}

// Build returns the constructed configuration.
func (b *SimConfigBuilder) Build() SimTestConfig {
	return b.config
}

// WriteToFile writes the configuration to a YAML file.
func (b *SimConfigBuilder) WriteToFile(path string) error {
	data, err := yaml.Marshal(b.config)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
