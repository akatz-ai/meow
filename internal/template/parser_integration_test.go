package template

import (
	"testing"
)

func TestParseFile_RealTemplates(t *testing.T) {
	templates := []string{
		"../../examples/templates/implement.toml",
		"../../examples/templates/human-gate.toml",
		"../../examples/templates/outer-loop.toml",
		"../../examples/templates/analyze-pick.toml",
		"../../examples/templates/test-suite.toml",
		"../../examples/templates/bake-meta.toml",
	}

	for _, path := range templates {
		t.Run(path, func(t *testing.T) {
			tmpl, err := ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile failed: %v", err)
			}

			// Basic sanity checks
			if tmpl.Meta.Name == "" {
				t.Error("expected non-empty name")
			}
			if len(tmpl.Steps) == 0 {
				t.Error("expected at least one step")
			}

			// Validate can generate step order
			_, err = tmpl.StepOrder()
			if err != nil {
				t.Errorf("StepOrder failed: %v", err)
			}
		})
	}
}
