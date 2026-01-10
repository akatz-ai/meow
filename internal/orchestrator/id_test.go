package orchestrator

import (
	"regexp"
	"strings"
	"testing"
)

func TestGenerateWorkflowID(t *testing.T) {
	t.Run("format matches wf-{hex}-{hex}", func(t *testing.T) {
		id := GenerateWorkflowID()

		// Should start with "wf-"
		if !strings.HasPrefix(id, "wf-") {
			t.Errorf("workflow ID should start with 'wf-', got %s", id)
		}

		// Should match expected pattern
		pattern := regexp.MustCompile(`^wf-[0-9a-f]+-[0-9a-f]{8}$`)
		if !pattern.MatchString(id) {
			t.Errorf("workflow ID should match pattern 'wf-{hex}-{8hex}', got %s", id)
		}
	})

	t.Run("generates unique IDs", func(t *testing.T) {
		seen := make(map[string]bool)
		for i := 0; i < 1000; i++ {
			id := GenerateWorkflowID()
			if seen[id] {
				t.Errorf("duplicate workflow ID generated: %s", id)
			}
			seen[id] = true
		}
	})

	t.Run("is filesystem safe", func(t *testing.T) {
		id := GenerateWorkflowID()

		// Check for unsafe characters
		unsafeChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", " "}
		for _, char := range unsafeChars {
			if strings.Contains(id, char) {
				t.Errorf("workflow ID contains unsafe character %q: %s", char, id)
			}
		}
	})
}

func TestGenerateExpandedStepID(t *testing.T) {
	tests := []struct {
		name     string
		parentID string
		stepID   string
		expected string
	}{
		{
			name:     "with parent",
			parentID: "implement",
			stepID:   "load-context",
			expected: "implement.load-context",
		},
		{
			name:     "empty parent",
			parentID: "",
			stepID:   "do-work",
			expected: "do-work",
		},
		{
			name:     "nested parent",
			parentID: "outer.inner",
			stepID:   "leaf",
			expected: "outer.inner.leaf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateExpandedStepID(tt.parentID, tt.stepID)
			if result != tt.expected {
				t.Errorf("GenerateExpandedStepID(%q, %q) = %q, want %q",
					tt.parentID, tt.stepID, result, tt.expected)
			}
		})
	}
}
