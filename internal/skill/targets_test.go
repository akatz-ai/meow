package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKnownTargets(t *testing.T) {
	t.Run("contains claude", func(t *testing.T) {
		config, ok := KnownTargets["claude"]
		if !ok {
			t.Fatal("KnownTargets should contain 'claude'")
		}
		if config.Name != "Claude Code" {
			t.Errorf("Name = %q, want %q", config.Name, "Claude Code")
		}
		if !strings.Contains(config.GlobalPath, "{{name}}") {
			t.Errorf("GlobalPath = %q, should contain {{name}}", config.GlobalPath)
		}
		if !strings.Contains(config.ProjectPath, "{{name}}") {
			t.Errorf("ProjectPath = %q, should contain {{name}}", config.ProjectPath)
		}
	})

	t.Run("contains opencode", func(t *testing.T) {
		config, ok := KnownTargets["opencode"]
		if !ok {
			t.Fatal("KnownTargets should contain 'opencode'")
		}
		if config.Name != "OpenCode" {
			t.Errorf("Name = %q, want %q", config.Name, "OpenCode")
		}
	})
}

func TestListKnownTargets(t *testing.T) {
	targets := ListKnownTargets()
	if len(targets) < 2 {
		t.Fatalf("ListKnownTargets() returned %d targets, want at least 2", len(targets))
	}

	hasTarget := func(name string) bool {
		for _, t := range targets {
			if t == name {
				return true
			}
		}
		return false
	}

	if !hasTarget("claude") {
		t.Error("ListKnownTargets() should include 'claude'")
	}
	if !hasTarget("opencode") {
		t.Error("ListKnownTargets() should include 'opencode'")
	}
}

func TestResolveTargetPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir error = %v", err)
	}

	t.Run("claude global path", func(t *testing.T) {
		path, err := ResolveTargetPath("claude", "my-skill", true)
		if err != nil {
			t.Fatalf("ResolveTargetPath() error = %v", err)
		}

		expected := filepath.Join(home, ".claude", "skills", "my-skill")
		if path != expected {
			t.Errorf("path = %q, want %q", path, expected)
		}
	})

	t.Run("claude project path", func(t *testing.T) {
		path, err := ResolveTargetPath("claude", "my-skill", false)
		if err != nil {
			t.Fatalf("ResolveTargetPath() error = %v", err)
		}

		expected := filepath.Join(".claude", "skills", "my-skill")
		if path != expected {
			t.Errorf("path = %q, want %q", path, expected)
		}
	})

	t.Run("opencode global path", func(t *testing.T) {
		path, err := ResolveTargetPath("opencode", "sprint-planner", true)
		if err != nil {
			t.Fatalf("ResolveTargetPath() error = %v", err)
		}

		expected := filepath.Join(home, ".config", "opencode", "skill", "sprint-planner")
		if path != expected {
			t.Errorf("path = %q, want %q", path, expected)
		}
	})

	t.Run("opencode project path", func(t *testing.T) {
		path, err := ResolveTargetPath("opencode", "sprint-planner", false)
		if err != nil {
			t.Fatalf("ResolveTargetPath() error = %v", err)
		}

		expected := filepath.Join(".opencode", "skill", "sprint-planner")
		if path != expected {
			t.Errorf("path = %q, want %q", path, expected)
		}
	})

	t.Run("unknown target returns error", func(t *testing.T) {
		_, err := ResolveTargetPath("unknown-harness", "my-skill", true)
		if err == nil {
			t.Fatal("ResolveTargetPath() expected error for unknown target")
		}
	})

	t.Run("name substitution works", func(t *testing.T) {
		path, err := ResolveTargetPath("claude", "complex-skill-name", true)
		if err != nil {
			t.Fatalf("ResolveTargetPath() error = %v", err)
		}

		if !strings.Contains(path, "complex-skill-name") {
			t.Errorf("path = %q, should contain skill name", path)
		}
		if strings.Contains(path, "{{name}}") {
			t.Errorf("path = %q, should not contain {{name}} placeholder", path)
		}
	})
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("UserHomeDir error = %v", err)
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"tilde at start", "~/.claude/skills", filepath.Join(home, ".claude", "skills")},
		{"no tilde", ".claude/skills", ".claude/skills"},
		{"tilde in middle ignored", "foo/~/bar", "foo/~/bar"},
		{"just tilde", "~", home},
		{"absolute path", "/absolute/path", "/absolute/path"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExpandPath(tt.input)
			if result != tt.expected {
				t.Errorf("ExpandPath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
