package types

import (
	"fmt"
	"time"
)

// AdapterConfig represents the complete configuration for an agent adapter.
// This is loaded from adapter.toml files in ~/.meow/adapters/<name>/.
type AdapterConfig struct {
	Adapter      AdapterMeta       `toml:"adapter"`
	Spawn        AdapterSpawnConfig `toml:"spawn"`
	Environment  map[string]string `toml:"environment"`
	PromptInject PromptInjection   `toml:"prompt_injection"`
	GracefulStop GracefulStopConfig `toml:"graceful_stop"`
	Events       EventConfig       `toml:"events"`
}

// AdapterMeta contains adapter identification information.
type AdapterMeta struct {
	Name        string `toml:"name"`
	Description string `toml:"description"`
}

// AdapterSpawnConfig defines how to start the agent process.
// Note: This is different from types.SpawnConfig which is for workflow steps.
type AdapterSpawnConfig struct {
	Command       string   `toml:"command"`
	ResumeCommand string   `toml:"resume_command"`
	StartupDelay  Duration `toml:"startup_delay"`
}

// PromptInjection defines how to inject prompts into the agent.
type PromptInjection struct {
	PreKeys  []string `toml:"pre_keys"`  // Keys to send before prompt (e.g., ["Escape"])
	PreDelay Duration `toml:"pre_delay"` // Delay after pre_keys
	Method   string   `toml:"method"`    // "literal" or "keys"
	PostKeys []string `toml:"post_keys"` // Keys to send after prompt (e.g., ["Enter"])
}

// GracefulStopConfig defines how to gracefully stop the agent.
type GracefulStopConfig struct {
	Keys []string `toml:"keys"` // Keys to send (e.g., ["C-c"])
	Wait Duration `toml:"wait"` // How long to wait after sending keys
}

// EventConfig defines event translation configuration.
type EventConfig struct {
	Translator  string            `toml:"translator"`    // Path to event translator script
	AgentConfig map[string]string `toml:"agent_config"`  // Agent-specific config (e.g., hooks)
}

// Duration is a wrapper around time.Duration that supports TOML string parsing.
// Accepts formats like "3s", "100ms", "1h30m".
type Duration struct {
	time.Duration
}

// UnmarshalText implements encoding.TextUnmarshaler for Duration.
func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	return nil
}

// MarshalText implements encoding.TextMarshaler for Duration.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

// Validate checks that the adapter configuration is valid.
func (a *AdapterConfig) Validate() error {
	if a.Adapter.Name == "" {
		return fmt.Errorf("adapter name is required")
	}
	if a.Spawn.Command == "" {
		return fmt.Errorf("spawn.command is required")
	}
	// Method defaults to "literal" if empty, but must be valid if specified
	if a.PromptInject.Method != "" && a.PromptInject.Method != "literal" && a.PromptInject.Method != "keys" {
		return fmt.Errorf("prompt_injection.method must be 'literal' or 'keys', got %q", a.PromptInject.Method)
	}
	return nil
}

// DefaultPromptInjection returns sensible defaults for prompt injection.
func DefaultPromptInjection() PromptInjection {
	return PromptInjection{
		PreKeys:  []string{"Escape"},
		PreDelay: Duration{100 * time.Millisecond},
		Method:   "literal",
		PostKeys: []string{"Enter"},
	}
}

// DefaultGracefulStop returns sensible defaults for graceful stop.
func DefaultGracefulStop() GracefulStopConfig {
	return GracefulStopConfig{
		Keys: []string{"C-c"},
		Wait: Duration{2 * time.Second},
	}
}

// GetMethod returns the prompt injection method, defaulting to "literal".
func (p *PromptInjection) GetMethod() string {
	if p.Method == "" {
		return "literal"
	}
	return p.Method
}

// GetStartupDelay returns the startup delay, defaulting to 0.
func (s *AdapterSpawnConfig) GetStartupDelay() time.Duration {
	return s.StartupDelay.Duration
}

// GetWait returns the graceful stop wait duration, defaulting to 2s.
func (g *GracefulStopConfig) GetWait() time.Duration {
	if g.Wait.Duration == 0 {
		return 2 * time.Second
	}
	return g.Wait.Duration
}
