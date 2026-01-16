package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// KnownTargets maps harness names to their configuration.
// These are the built-in AI harnesses that MEOW knows how to install skills for.
var KnownTargets = map[string]TargetConfig{
	"claude": {
		Name:        "Claude Code",
		GlobalPath:  "~/.claude/skills/{{name}}",
		ProjectPath: ".claude/skills/{{name}}",
	},
	"opencode": {
		Name:        "OpenCode",
		GlobalPath:  "~/.config/opencode/skill/{{name}}",
		ProjectPath: ".opencode/skill/{{name}}",
	},
}

// ListKnownTargets returns the names of all known harnesses in sorted order.
func ListKnownTargets() []string {
	targets := make([]string, 0, len(KnownTargets))
	for name := range KnownTargets {
		targets = append(targets, name)
	}
	sort.Strings(targets)
	return targets
}

// ResolveTargetPath returns the installation path for a target.
// It expands {{name}} with the skill name and ~ with the home directory.
// If global is true, uses GlobalPath; otherwise uses ProjectPath.
// Returns an error if the target is not in KnownTargets.
func ResolveTargetPath(target, skillName string, global bool) (string, error) {
	config, ok := KnownTargets[target]
	if !ok {
		return "", fmt.Errorf("unknown target %q: use a custom path or one of: %v", target, ListKnownTargets())
	}

	var template string
	if global {
		template = config.GlobalPath
	} else {
		template = config.ProjectPath
	}

	// Replace {{name}} placeholder with skill name
	path := strings.ReplaceAll(template, "{{name}}", skillName)

	// Expand ~ to home directory
	path = ExpandPath(path)

	return path, nil
}

// ExpandPath expands ~ at the start of a path to the user's home directory.
// If ~ is not at the start or home directory cannot be determined, returns path unchanged.
func ExpandPath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}

	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}

	return path
}
