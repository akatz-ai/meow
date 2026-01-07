// Package validation provides output validation for task beads.
package validation

import (
	"encoding/json"
	"fmt"
	"os"
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
		if beadChecker == nil {
			// No checker provided - skip validation but accept the value
			return value, nil
		}
		if !beadChecker.BeadExists(value) {
			return nil, fmt.Errorf("bead not found: %s", value)
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
		if beadChecker != nil {
			for _, id := range ids {
				if !beadChecker.BeadExists(id) {
					return nil, fmt.Errorf("bead not found: %s", id)
				}
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
