package types

import (
	"testing"
	"time"

	"github.com/BurntSushi/toml"
)

func TestAdapterConfigTOMLParsing(t *testing.T) {
	const sampleConfig = `
[adapter]
name = "claude"
description = "Claude Code CLI agent"

[spawn]
command = "claude --dangerously-skip-permissions"
resume_command = "claude --dangerously-skip-permissions --resume {{session_id}}"
startup_delay = "3s"

[environment]
TMUX = ""
MY_VAR = "value"

[prompt_injection]
pre_keys = ["Escape"]
pre_delay = "100ms"
method = "literal"
post_keys = ["Enter"]

[graceful_stop]
keys = ["C-c"]
wait = "2s"

[events]
translator = "./event-translator.sh"

[events.agent_config]
Stop = "{{adapter_dir}}/event-translator.sh Stop"
PreToolUse = "{{adapter_dir}}/event-translator.sh PreToolUse $TOOL_NAME"
`

	var cfg AdapterConfig
	_, err := toml.Decode(sampleConfig, &cfg)
	if err != nil {
		t.Fatalf("failed to parse TOML: %v", err)
	}

	// Verify adapter meta
	if cfg.Adapter.Name != "claude" {
		t.Errorf("expected adapter name 'claude', got %q", cfg.Adapter.Name)
	}
	if cfg.Adapter.Description != "Claude Code CLI agent" {
		t.Errorf("expected description 'Claude Code CLI agent', got %q", cfg.Adapter.Description)
	}

	// Verify spawn config
	if cfg.Spawn.Command != "claude --dangerously-skip-permissions" {
		t.Errorf("unexpected spawn command: %q", cfg.Spawn.Command)
	}
	if cfg.Spawn.ResumeCommand != "claude --dangerously-skip-permissions --resume {{session_id}}" {
		t.Errorf("unexpected resume command: %q", cfg.Spawn.ResumeCommand)
	}
	if cfg.Spawn.StartupDelay.Duration != 3*time.Second {
		t.Errorf("expected startup_delay 3s, got %v", cfg.Spawn.StartupDelay.Duration)
	}

	// Verify environment
	if len(cfg.Environment) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(cfg.Environment))
	}
	if cfg.Environment["TMUX"] != "" {
		t.Errorf("expected TMUX='', got %q", cfg.Environment["TMUX"])
	}
	if cfg.Environment["MY_VAR"] != "value" {
		t.Errorf("expected MY_VAR='value', got %q", cfg.Environment["MY_VAR"])
	}

	// Verify prompt injection
	if len(cfg.PromptInject.PreKeys) != 1 || cfg.PromptInject.PreKeys[0] != "Escape" {
		t.Errorf("unexpected pre_keys: %v", cfg.PromptInject.PreKeys)
	}
	if cfg.PromptInject.PreDelay.Duration != 100*time.Millisecond {
		t.Errorf("expected pre_delay 100ms, got %v", cfg.PromptInject.PreDelay.Duration)
	}
	if cfg.PromptInject.Method != "literal" {
		t.Errorf("expected method 'literal', got %q", cfg.PromptInject.Method)
	}
	if len(cfg.PromptInject.PostKeys) != 1 || cfg.PromptInject.PostKeys[0] != "Enter" {
		t.Errorf("unexpected post_keys: %v", cfg.PromptInject.PostKeys)
	}

	// Verify graceful stop
	if len(cfg.GracefulStop.Keys) != 1 || cfg.GracefulStop.Keys[0] != "C-c" {
		t.Errorf("unexpected graceful_stop keys: %v", cfg.GracefulStop.Keys)
	}
	if cfg.GracefulStop.Wait.Duration != 2*time.Second {
		t.Errorf("expected wait 2s, got %v", cfg.GracefulStop.Wait.Duration)
	}

	// Verify events
	if cfg.Events.Translator != "./event-translator.sh" {
		t.Errorf("unexpected translator: %q", cfg.Events.Translator)
	}
	if len(cfg.Events.AgentConfig) != 2 {
		t.Errorf("expected 2 agent_config entries, got %d", len(cfg.Events.AgentConfig))
	}
}

func TestAdapterConfigValidation(t *testing.T) {
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
			errMsg:  "adapter name is required",
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
			name: "invalid method",
			config: AdapterConfig{
				Adapter:      AdapterMeta{Name: "test"},
				Spawn:        AdapterSpawnConfig{Command: "test-agent"},
				PromptInject: PromptInjection{Method: "invalid"},
			},
			wantErr: true,
			errMsg:  "prompt_injection.method must be 'literal' or 'keys'",
		},
		{
			name: "valid literal method",
			config: AdapterConfig{
				Adapter:      AdapterMeta{Name: "test"},
				Spawn:        AdapterSpawnConfig{Command: "test-agent"},
				PromptInject: PromptInjection{Method: "literal"},
			},
			wantErr: false,
		},
		{
			name: "valid keys method",
			config: AdapterConfig{
				Adapter:      AdapterMeta{Name: "test"},
				Spawn:        AdapterSpawnConfig{Command: "test-agent"},
				PromptInject: PromptInjection{Method: "keys"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && err.Error() != tt.errMsg {
					// Check if error contains the expected message
					if err.Error()[:len(tt.errMsg)] != tt.errMsg[:min(len(err.Error()), len(tt.errMsg))] {
						t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
					}
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestDurationParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"100ms", 100 * time.Millisecond, false},
		{"3s", 3 * time.Second, false},
		{"1m30s", 90 * time.Second, false},
		{"1h", time.Hour, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			var d Duration
			err := d.UnmarshalText([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error for input %q", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if d.Duration != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, d.Duration)
				}
			}
		})
	}
}

func TestDurationMarshal(t *testing.T) {
	d := Duration{3 * time.Second}
	text, err := d.MarshalText()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(text) != "3s" {
		t.Errorf("expected '3s', got %q", string(text))
	}
}

func TestDefaultPromptInjection(t *testing.T) {
	defaults := DefaultPromptInjection()

	if len(defaults.PreKeys) != 1 || defaults.PreKeys[0] != "Escape" {
		t.Errorf("unexpected default pre_keys: %v", defaults.PreKeys)
	}
	if defaults.PreDelay.Duration != 100*time.Millisecond {
		t.Errorf("expected default pre_delay 100ms, got %v", defaults.PreDelay.Duration)
	}
	if defaults.Method != "literal" {
		t.Errorf("expected default method 'literal', got %q", defaults.Method)
	}
	if len(defaults.PostKeys) != 1 || defaults.PostKeys[0] != "Enter" {
		t.Errorf("unexpected default post_keys: %v", defaults.PostKeys)
	}
}

func TestDefaultGracefulStop(t *testing.T) {
	defaults := DefaultGracefulStop()

	if len(defaults.Keys) != 1 || defaults.Keys[0] != "C-c" {
		t.Errorf("unexpected default keys: %v", defaults.Keys)
	}
	if defaults.Wait.Duration != 2*time.Second {
		t.Errorf("expected default wait 2s, got %v", defaults.Wait.Duration)
	}
}

func TestGetMethodDefault(t *testing.T) {
	// Empty method should default to "literal"
	p := PromptInjection{}
	if p.GetMethod() != "literal" {
		t.Errorf("expected default method 'literal', got %q", p.GetMethod())
	}

	// Explicit method should be returned
	p.Method = "keys"
	if p.GetMethod() != "keys" {
		t.Errorf("expected method 'keys', got %q", p.GetMethod())
	}
}

func TestGetWaitDefault(t *testing.T) {
	// Zero wait should default to 2s
	g := GracefulStopConfig{}
	if g.GetWait() != 2*time.Second {
		t.Errorf("expected default wait 2s, got %v", g.GetWait())
	}

	// Explicit wait should be returned
	g.Wait = Duration{5 * time.Second}
	if g.GetWait() != 5*time.Second {
		t.Errorf("expected wait 5s, got %v", g.GetWait())
	}
}

func TestMinimalTOMLConfig(t *testing.T) {
	const minimal = `
[adapter]
name = "minimal"

[spawn]
command = "my-agent"
`
	var cfg AdapterConfig
	_, err := toml.Decode(minimal, &cfg)
	if err != nil {
		t.Fatalf("failed to parse minimal TOML: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		t.Errorf("minimal config should be valid: %v", err)
	}

	// Check defaults are used
	if cfg.PromptInject.GetMethod() != "literal" {
		t.Errorf("expected default method 'literal', got %q", cfg.PromptInject.GetMethod())
	}
}
