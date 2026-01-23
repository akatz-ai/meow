package types

import (
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

func TestAdapterConfig_ParseTOML(t *testing.T) {
	// Note: Event hook configuration is handled by library templates (lib/claude-events.meow.toml),
	// not adapters. Adapters only define runtime behavior.
	configTOML := `
[adapter]
name = "claude"
description = "Claude Code CLI agent"

[spawn]
command = "claude --dangerously-skip-permissions"
resume_command = "claude --dangerously-skip-permissions --resume {{session_id}}"
startup_delay = "3s"

[environment]
TMUX = ""
CUSTOM_VAR = "value"

[prompt_injection]
pre_keys = ["Escape"]
pre_delay = "100ms"
method = "literal"
post_keys = ["Enter"]
post_delay = "500ms"

[graceful_stop]
keys = ["C-c"]
wait = "2s"
`

	var config AdapterConfig
	_, err := toml.Decode(configTOML, &config)
	if err != nil {
		t.Fatalf("failed to parse TOML: %v", err)
	}

	// Check adapter metadata
	if config.Adapter.Name != "claude" {
		t.Errorf("expected adapter.name = 'claude', got %q", config.Adapter.Name)
	}
	if config.Adapter.Description != "Claude Code CLI agent" {
		t.Errorf("expected description, got %q", config.Adapter.Description)
	}

	// Check spawn config
	if config.Spawn.Command != "claude --dangerously-skip-permissions" {
		t.Errorf("expected spawn.command, got %q", config.Spawn.Command)
	}
	if config.Spawn.StartupDelay.Duration() != 3*time.Second {
		t.Errorf("expected startup_delay = 3s, got %v", config.Spawn.StartupDelay)
	}

	// Check environment
	if config.Environment["TMUX"] != "" {
		t.Errorf("expected TMUX='', got %q", config.Environment["TMUX"])
	}
	if config.Environment["CUSTOM_VAR"] != "value" {
		t.Errorf("expected CUSTOM_VAR='value', got %q", config.Environment["CUSTOM_VAR"])
	}

	// Check prompt injection
	if len(config.PromptInjection.PreKeys) != 1 || config.PromptInjection.PreKeys[0] != "Escape" {
		t.Errorf("expected pre_keys = [Escape], got %v", config.PromptInjection.PreKeys)
	}
	if config.PromptInjection.PreDelay.Duration() != 100*time.Millisecond {
		t.Errorf("expected pre_delay = 100ms, got %v", config.PromptInjection.PreDelay)
	}
	if config.PromptInjection.Method != "literal" {
		t.Errorf("expected method = literal, got %q", config.PromptInjection.Method)
	}
	if len(config.PromptInjection.PostKeys) != 1 || config.PromptInjection.PostKeys[0] != "Enter" {
		t.Errorf("expected post_keys = [Enter], got %v", config.PromptInjection.PostKeys)
	}

	// Check graceful stop
	if len(config.GracefulStop.Keys) != 1 || config.GracefulStop.Keys[0] != "C-c" {
		t.Errorf("expected graceful_stop.keys = [C-c], got %v", config.GracefulStop.Keys)
	}
	if config.GracefulStop.Wait.Duration() != 2*time.Second {
		t.Errorf("expected graceful_stop.wait = 2s, got %v", config.GracefulStop.Wait)
	}
}

func TestAdapterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  AdapterConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid minimal config",
			config: AdapterConfig{
				Adapter: AdapterMeta{Name: "test"},
				Spawn:   AdapterSpawnConfig{Command: "test-agent"},
			},
			wantErr: false,
		},
		{
			name: "missing adapter name",
			config: AdapterConfig{
				Spawn: AdapterSpawnConfig{Command: "test-agent"},
			},
			wantErr: true,
			errMsg:  "adapter.name is required",
		},
		{
			name: "missing spawn command",
			config: AdapterConfig{
				Adapter: AdapterMeta{Name: "test"},
			},
			wantErr: true,
			errMsg:  "spawn.command is required",
		},
		{
			name: "invalid prompt injection method",
			config: AdapterConfig{
				Adapter:         AdapterMeta{Name: "test"},
				Spawn:           AdapterSpawnConfig{Command: "test-agent"},
				PromptInjection: PromptInjectionConfig{Method: "invalid"},
			},
			wantErr: true,
			errMsg:  "prompt_injection.method must be 'literal' or 'keys', got \"invalid\"",
		},
		{
			name: "valid with literal method",
			config: AdapterConfig{
				Adapter:         AdapterMeta{Name: "test"},
				Spawn:           AdapterSpawnConfig{Command: "test-agent"},
				PromptInjection: PromptInjectionConfig{Method: "literal"},
			},
			wantErr: false,
		},
		{
			name: "valid with keys method",
			config: AdapterConfig{
				Adapter:         AdapterMeta{Name: "test"},
				Spawn:           AdapterSpawnConfig{Command: "test-agent"},
				PromptInjection: PromptInjectionConfig{Method: "keys"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					t.Errorf("expected error %q, got %q", tt.errMsg, err.Error())
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestAdapterConfig_Defaults(t *testing.T) {
	config := AdapterConfig{
		Adapter: AdapterMeta{Name: "test"},
		Spawn:   AdapterSpawnConfig{Command: "test-agent"},
	}

	// Test default prompt injection method
	if config.GetPromptInjectionMethod() != "literal" {
		t.Errorf("expected default method = literal, got %q", config.GetPromptInjectionMethod())
	}

	// Test default startup delay
	if config.GetStartupDelay() != 3*time.Second {
		t.Errorf("expected default startup delay = 3s, got %v", config.GetStartupDelay())
	}

	// Test default graceful stop wait
	if config.GetGracefulStopWait() != 2*time.Second {
		t.Errorf("expected default graceful stop wait = 2s, got %v", config.GetGracefulStopWait())
	}
}

func TestDuration_UnmarshalText(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"3s", 3 * time.Second, false},
		{"100ms", 100 * time.Millisecond, false},
		{"1h", 1 * time.Hour, false},
		{"500µs", 500 * time.Microsecond, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var d Duration
			err := d.UnmarshalText([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if d.Duration() != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, d.Duration())
				}
			}
		})
	}
}

func TestDuration_MarshalText(t *testing.T) {
	d := Duration(3 * time.Second)
	text, err := d.MarshalText()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(text) != "3s" {
		t.Errorf("expected '3s', got %q", string(text))
	}
}

func TestDuration_String(t *testing.T) {
	d := Duration(100 * time.Millisecond)
	if d.String() != "100ms" {
		t.Errorf("expected '100ms', got %q", d.String())
	}
}

func TestAdapterConfig_StabilizeSequence(t *testing.T) {
	configTOML := `
[adapter]
name = "claude-stabilize"
description = "Claude with stabilization"

[spawn]
command = "claude --dangerously-skip-permissions"

[prompt_injection]
# Stabilization sequence for subsequent prompts
pre_stabilize_delay = "1s"
stabilize_sequence = [
    { key = "Escape", delay = "5s" },
    { key = "Escape", delay = "3s" }
]

# Standard injection (after stabilization)
pre_keys = []
pre_delay = "0s"
method = "literal"
post_keys = ["Enter"]
post_delay = "500ms"
`

	var config AdapterConfig
	_, err := toml.Decode(configTOML, &config)
	if err != nil {
		t.Fatalf("failed to parse TOML: %v", err)
	}

	// Check stabilization config
	if config.PromptInjection.PreStabilizeDelay.Duration() != 1*time.Second {
		t.Errorf("expected pre_stabilize_delay = 1s, got %v", config.PromptInjection.PreStabilizeDelay)
	}

	if len(config.PromptInjection.StabilizeSequence) != 2 {
		t.Fatalf("expected 2 stabilize steps, got %d", len(config.PromptInjection.StabilizeSequence))
	}

	// Check first step
	step0 := config.PromptInjection.StabilizeSequence[0]
	if step0.Key != "Escape" {
		t.Errorf("expected step[0].key = 'Escape', got %q", step0.Key)
	}
	if step0.Delay.Duration() != 5*time.Second {
		t.Errorf("expected step[0].delay = 5s, got %v", step0.Delay)
	}

	// Check second step
	step1 := config.PromptInjection.StabilizeSequence[1]
	if step1.Key != "Escape" {
		t.Errorf("expected step[1].key = 'Escape', got %q", step1.Key)
	}
	if step1.Delay.Duration() != 3*time.Second {
		t.Errorf("expected step[1].delay = 3s, got %v", step1.Delay)
	}

	// Check that standard injection still works
	if len(config.PromptInjection.PreKeys) != 0 {
		t.Errorf("expected pre_keys = [], got %v", config.PromptInjection.PreKeys)
	}
	if len(config.PromptInjection.PostKeys) != 1 || config.PromptInjection.PostKeys[0] != "Enter" {
		t.Errorf("expected post_keys = [Enter], got %v", config.PromptInjection.PostKeys)
	}
}

func TestAdapterConfig_StabilizeSequenceEmpty(t *testing.T) {
	// Config without stabilization sequence should parse fine
	configTOML := `
[adapter]
name = "claude-no-stabilize"

[spawn]
command = "claude"

[prompt_injection]
pre_keys = ["Escape"]
method = "literal"
post_keys = ["Enter"]
`

	var config AdapterConfig
	_, err := toml.Decode(configTOML, &config)
	if err != nil {
		t.Fatalf("failed to parse TOML: %v", err)
	}

	if config.PromptInjection.PreStabilizeDelay != 0 {
		t.Errorf("expected pre_stabilize_delay = 0, got %v", config.PromptInjection.PreStabilizeDelay)
	}
	if len(config.PromptInjection.StabilizeSequence) != 0 {
		t.Errorf("expected empty stabilize_sequence, got %d steps", len(config.PromptInjection.StabilizeSequence))
	}
}
