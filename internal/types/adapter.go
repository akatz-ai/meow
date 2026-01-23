package types

import (
	"fmt"
	"time"
)

// AdapterConfig represents the complete configuration for an agent adapter.
// This is loaded from adapter.toml files.
//
// Adapters define runtime behavior only: how to spawn, inject prompts into, and stop
// agent processes. Event hook configuration (like Claude's Stop/PreToolUse/PostToolUse)
// is handled by library templates, not adapters. See lib/claude-events.meow.toml.
type AdapterConfig struct {
	// Adapter contains metadata about the adapter
	Adapter AdapterMeta `toml:"adapter"`

	// Spawn defines how to start the agent process
	Spawn AdapterSpawnConfig `toml:"spawn"`

	// Environment contains additional environment variables for the agent
	Environment map[string]string `toml:"environment"`

	// PromptInjection defines how to inject prompts into the agent
	PromptInjection PromptInjectionConfig `toml:"prompt_injection"`

	// GracefulStop defines how to gracefully stop the agent
	GracefulStop GracefulStopConfig `toml:"graceful_stop"`
}

// AdapterMeta contains metadata about the adapter.
type AdapterMeta struct {
	// Name is the unique identifier for this adapter
	Name string `toml:"name"`

	// Description is a human-readable description of the adapter
	Description string `toml:"description"`
}

// AdapterSpawnConfig defines how to start an agent process.
type AdapterSpawnConfig struct {
	// Command is the command to start the agent (e.g., "claude --dangerously-skip-permissions")
	Command string `toml:"command"`

	// ResumeCommand is the command to resume an existing session
	// Uses {{session_id}} placeholder for the session ID
	ResumeCommand string `toml:"resume_command"`

	// StartupDelay is how long to wait after starting the agent before it's ready
	StartupDelay Duration `toml:"startup_delay"`
}

// StabilizeStep defines a single key + delay in the stabilization sequence.
// Used to ensure the agent is in a clean, idle state before prompt injection.
type StabilizeStep struct {
	Key   string   `toml:"key"`
	Delay Duration `toml:"delay"`
}

// PromptInjectionConfig defines how to inject prompts into an agent's tmux session.
type PromptInjectionConfig struct {
	// Stabilization sequence (for subsequent prompts, not first prompt after spawn)
	// This helps ensure the agent is fully idle before injecting a new prompt.

	// PreStabilizeDelay is an initial wait before running the stabilization sequence
	PreStabilizeDelay Duration `toml:"pre_stabilize_delay"`

	// StabilizeSequence is a sequence of key presses with delays to stabilize the agent
	// Example: [{ key = "Escape", delay = "5s" }, { key = "Escape", delay = "3s" }]
	StabilizeSequence []StabilizeStep `toml:"stabilize_sequence"`

	// Standard injection (runs after stabilization)

	// PreKeys are keys to send before the prompt (e.g., ["Escape"] to exit copy mode)
	PreKeys []string `toml:"pre_keys"`

	// PreDelay is how long to wait after pre_keys before sending the prompt
	PreDelay Duration `toml:"pre_delay"`

	// Method is how to send the prompt text: "literal" (tmux send-keys -l) or "keys"
	Method string `toml:"method"`

	// PostKeys are keys to send after the prompt (e.g., ["Enter"] to submit)
	PostKeys []string `toml:"post_keys"`

	// PostDelay is how long to wait after sending prompt before sending post_keys
	PostDelay Duration `toml:"post_delay"`
}

// GracefulStopConfig defines how to gracefully stop an agent.
type GracefulStopConfig struct {
	// Keys are the keys to send to initiate graceful shutdown (e.g., ["C-c"])
	Keys []string `toml:"keys"`

	// Wait is how long to wait for graceful shutdown before killing
	Wait Duration `toml:"wait"`
}

// Duration is a time.Duration that can be unmarshaled from TOML strings like "3s", "100ms".
type Duration time.Duration

// UnmarshalText implements encoding.TextUnmarshaler for Duration.
func (d *Duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", text, err)
	}
	*d = Duration(parsed)
	return nil
}

// MarshalText implements encoding.TextMarshaler for Duration.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(time.Duration(d).String()), nil
}

// Duration returns the underlying time.Duration value.
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// String implements fmt.Stringer.
func (d Duration) String() string {
	return time.Duration(d).String()
}

// Validate checks that the adapter configuration has all required fields.
func (c *AdapterConfig) Validate() error {
	if c.Adapter.Name == "" {
		return fmt.Errorf("adapter.name is required")
	}
	if c.Spawn.Command == "" {
		return fmt.Errorf("spawn.command is required")
	}
	// Validate prompt injection method if specified
	if c.PromptInjection.Method != "" && c.PromptInjection.Method != "literal" && c.PromptInjection.Method != "keys" {
		return fmt.Errorf("prompt_injection.method must be 'literal' or 'keys', got %q", c.PromptInjection.Method)
	}
	return nil
}

// GetPromptInjectionMethod returns the prompt injection method, defaulting to "literal".
func (c *AdapterConfig) GetPromptInjectionMethod() string {
	if c.PromptInjection.Method == "" {
		return "literal"
	}
	return c.PromptInjection.Method
}

// GetStartupDelay returns the startup delay, defaulting to 3 seconds.
func (c *AdapterConfig) GetStartupDelay() time.Duration {
	if c.Spawn.StartupDelay == 0 {
		return 3 * time.Second
	}
	return c.Spawn.StartupDelay.Duration()
}

// GetGracefulStopWait returns the graceful stop wait duration, defaulting to 2 seconds.
func (c *AdapterConfig) GetGracefulStopWait() time.Duration {
	if c.GracefulStop.Wait == 0 {
		return 2 * time.Second
	}
	return c.GracefulStop.Wait.Duration()
}
