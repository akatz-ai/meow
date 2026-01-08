// Package validation provides output validation for task beads.
package validation

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/meow-stack/meow-machine/internal/types"
)

// OutputValidationError contains details about validation failures.
type OutputValidationError struct {
	BeadID  string
	Missing []string          // Names of missing required outputs
	Invalid map[string]string // Name -> error message for invalid outputs
}

func (e *OutputValidationError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("cannot close bead %s - output validation failed\n", e.BeadID))

	if len(e.Missing) > 0 {
		sb.WriteString("\nMissing required outputs:\n")
		for _, name := range e.Missing {
			sb.WriteString(fmt.Sprintf("  - %s\n", name))
		}
	}

	if len(e.Invalid) > 0 {
		sb.WriteString("\nInvalid outputs:\n")
		for name, msg := range e.Invalid {
			sb.WriteString(fmt.Sprintf("  - %s: %s\n", name, msg))
		}
	}

	return sb.String()
}

// ValidatedOutputs contains the typed outputs after validation.
type ValidatedOutputs struct {
	Values map[string]any
}

// BeadChecker is used to validate bead_id outputs.
type BeadChecker interface {
	BeadExists(id string) bool
	// ListAllIDs returns all known bead IDs for suggestion generation.
	// Returns nil if not available.
	ListAllIDs() []string
}

// BeadNotFoundError is returned when a bead_id references a non-existent bead.
type BeadNotFoundError struct {
	ID          string   // The ID that was not found
	Suggestions []string // Similar bead IDs (may be empty)
}

func (e *BeadNotFoundError) Error() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("bead not found: %s", e.ID))
	if len(e.Suggestions) > 0 {
		sb.WriteString("\n\nDid you mean one of these?\n")
		for _, s := range e.Suggestions {
			sb.WriteString(fmt.Sprintf("  • %s\n", s))
		}
	}
	sb.WriteString("\nHint: Run 'bd list --status=open' to see available beads")
	return sb.String()
}

// findSimilarBeads finds bead IDs similar to the given ID using Levenshtein distance.
// Returns up to maxSuggestions sorted by similarity.
func findSimilarBeads(id string, checker BeadChecker, maxSuggestions int) []string {
	allIDs := checker.ListAllIDs()
	if len(allIDs) == 0 {
		return nil
	}

	type scored struct {
		id    string
		score int
	}

	var candidates []scored
	for _, candidate := range allIDs {
		dist := levenshteinDistance(id, candidate)
		// Only consider candidates with distance <= 5 (reasonable typo range)
		if dist <= 5 {
			candidates = append(candidates, scored{candidate, dist})
		}
	}

	// Sort by distance (lower is better)
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score < candidates[j].score
	})

	// Return top suggestions
	result := make([]string, 0, maxSuggestions)
	for i := 0; i < len(candidates) && i < maxSuggestions; i++ {
		result = append(result, candidates[i].id)
	}
	return result
}

// levenshteinDistance computes the edit distance between two strings.
func levenshteinDistance(s1, s2 string) int {
	if len(s1) == 0 {
		return len(s2)
	}
	if len(s2) == 0 {
		return len(s1)
	}

	// Create matrix
	d := make([][]int, len(s1)+1)
	for i := range d {
		d[i] = make([]int, len(s2)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}

	for i := 1; i <= len(s1); i++ {
		for j := 1; j <= len(s2); j++ {
			cost := 1
			if s1[i-1] == s2[j-1] {
				cost = 0
			}
			d[i][j] = min(
				d[i-1][j]+1,      // deletion
				d[i][j-1]+1,      // insertion
				d[i-1][j-1]+cost, // substitution
			)
		}
	}

	return d[len(s1)][len(s2)]
}

// validateBeadID validates a single bead ID value.
// Checks format and existence, returning helpful errors with suggestions.
func validateBeadID(value string, beadChecker BeadChecker) error {
	// Check format - must have a recognizable prefix
	if !isValidBeadIDFormat(value) {
		return fmt.Errorf("invalid bead ID format: '%s' (expected format like 'bd-xxx', 'meow-xxx', 'task-xxx')", value)
	}

	if beadChecker == nil {
		// No checker provided - skip existence validation
		return nil
	}

	if !beadChecker.BeadExists(value) {
		suggestions := findSimilarBeads(value, beadChecker, 3)
		return &BeadNotFoundError{
			ID:          value,
			Suggestions: suggestions,
		}
	}

	return nil
}

// isValidBeadIDFormat checks if a string looks like a valid bead ID.
// Valid formats include: bd-xxx, meow-xxx, task-xxx, code-xxx, etc.
func isValidBeadIDFormat(id string) bool {
	// Must contain a hyphen and have content before and after
	idx := strings.Index(id, "-")
	if idx <= 0 || idx >= len(id)-1 {
		return false
	}
	// Prefix should be alphanumeric
	prefix := id[:idx]
	for _, c := range prefix {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

// ValidateOutputs validates the provided outputs against the task's output spec.
// Returns validated and typed outputs on success.
func ValidateOutputs(beadID string, spec *types.TaskOutputSpec, provided map[string]string, beadChecker BeadChecker) (*ValidatedOutputs, error) {
	if spec == nil {
		// No spec means no validation needed
		result := &ValidatedOutputs{Values: make(map[string]any)}
		for k, v := range provided {
			result.Values[k] = v
		}
		return result, nil
	}

	valErr := &OutputValidationError{
		BeadID:  beadID,
		Invalid: make(map[string]string),
	}

	result := &ValidatedOutputs{Values: make(map[string]any)}

	// Check required outputs
	for _, req := range spec.Required {
		value, ok := provided[req.Name]
		if !ok || value == "" {
			valErr.Missing = append(valErr.Missing, req.Name)
			continue
		}

		// Validate and convert the value
		typed, err := validateAndConvert(req.Name, value, req.Type, beadChecker)
		if err != nil {
			valErr.Invalid[req.Name] = err.Error()
		} else {
			result.Values[req.Name] = typed
		}
	}

	// Validate optional outputs that were provided
	for _, opt := range spec.Optional {
		value, ok := provided[opt.Name]
		if !ok || value == "" {
			continue
		}

		typed, err := validateAndConvert(opt.Name, value, opt.Type, beadChecker)
		if err != nil {
			valErr.Invalid[opt.Name] = err.Error()
		} else {
			result.Values[opt.Name] = typed
		}
	}

	// Add any extra outputs that weren't in the spec
	specNames := make(map[string]bool)
	for _, req := range spec.Required {
		specNames[req.Name] = true
	}
	for _, opt := range spec.Optional {
		specNames[opt.Name] = true
	}
	for k, v := range provided {
		if !specNames[k] {
			result.Values[k] = v
		}
	}

	if len(valErr.Missing) > 0 || len(valErr.Invalid) > 0 {
		return nil, valErr
	}

	return result, nil
}

// validateAndConvert validates a value against its expected type and converts it.
func validateAndConvert(name, value string, outputType types.TaskOutputType, beadChecker BeadChecker) (any, error) {
	switch outputType {
	case types.TaskOutputTypeString:
		return value, nil

	case types.TaskOutputTypeStringArr:
		// Parse as JSON array or comma-separated
		if strings.HasPrefix(value, "[") {
			var arr []string
			if err := json.Unmarshal([]byte(value), &arr); err != nil {
				return nil, fmt.Errorf("invalid string array format: %w", err)
			}
			return arr, nil
		}
		// Comma-separated fallback
		return strings.Split(value, ","), nil

	case types.TaskOutputTypeNumber:
		// Try int first, then float
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i, nil
		}
		f, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid number: %s", value)
		}
		return f, nil

	case types.TaskOutputTypeBool:
		switch strings.ToLower(value) {
		case "true", "yes", "1":
			return true, nil
		case "false", "no", "0":
			return false, nil
		default:
			return nil, fmt.Errorf("invalid boolean: %s (expected true/false)", value)
		}

	case types.TaskOutputTypeJSON:
		var obj any
		if err := json.Unmarshal([]byte(value), &obj); err != nil {
			return nil, fmt.Errorf("invalid JSON: %w", err)
		}
		return obj, nil

	case types.TaskOutputTypeBeadID:
		if err := validateBeadID(value, beadChecker); err != nil {
			return nil, err
		}
		return value, nil

	case types.TaskOutputTypeBeadIDArr:
		var ids []string
		if strings.HasPrefix(value, "[") {
			if err := json.Unmarshal([]byte(value), &ids); err != nil {
				return nil, fmt.Errorf("invalid bead ID array format: %w", err)
			}
		} else {
			ids = strings.Split(value, ",")
		}
		for i := range ids {
			ids[i] = strings.TrimSpace(ids[i])
		}
		// Validate each bead ID if checker is available
		for _, id := range ids {
			if err := validateBeadID(id, beadChecker); err != nil {
				return nil, err
			}
		}
		return ids, nil

	case types.TaskOutputTypeFilePath:
		if _, err := os.Stat(value); err != nil {
			return nil, fmt.Errorf("file not found: %s", value)
		}
		return value, nil

	default:
		// Unknown type - pass through as string
		return value, nil
	}
}

// FormatUsage returns a help message showing how to close the bead with its required outputs.
func FormatUsage(beadID string, spec *types.TaskOutputSpec) string {
	if spec == nil || len(spec.Required) == 0 {
		return fmt.Sprintf("Usage: meow close %s", beadID)
	}

	var sb strings.Builder
	sb.WriteString("Usage:\n  meow close ")
	sb.WriteString(beadID)

	for _, req := range spec.Required {
		sb.WriteString(fmt.Sprintf(" --output %s=<%s>", req.Name, req.Type))
	}

	sb.WriteString("\n\nRequired outputs:\n")
	for _, req := range spec.Required {
		desc := req.Description
		if desc == "" {
			desc = string(req.Type)
		}
		sb.WriteString(fmt.Sprintf("  - %s (%s): %s\n", req.Name, req.Type, desc))
	}

	if len(spec.Optional) > 0 {
		sb.WriteString("\nOptional outputs:\n")
		for _, opt := range spec.Optional {
			desc := opt.Description
			if desc == "" {
				desc = string(opt.Type)
			}
			sb.WriteString(fmt.Sprintf("  - %s (%s): %s\n", opt.Name, opt.Type, desc))
		}
	}

	return sb.String()
}
