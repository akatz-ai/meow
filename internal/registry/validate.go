package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// namePattern matches valid kebab-case names: lowercase, start with letter, hyphens allowed.
var namePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// ValidationError represents a single validation error.
type ValidationError struct {
	Field   string
	Message string
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s %s", e.Field, e.Message)
}

// ValidationResult holds validation errors and warnings.
type ValidationResult struct {
	Errors   []ValidationError
	Warnings []string
}

// HasErrors returns true if there are any validation errors.
func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// AddError appends a validation error.
func (r *ValidationResult) AddError(field, message string) {
	r.Errors = append(r.Errors, ValidationError{Field: field, Message: message})
}

// AddWarning appends a validation warning.
func (r *ValidationResult) AddWarning(message string) {
	r.Warnings = append(r.Warnings, message)
}

// Error implements the error interface.
func (r *ValidationResult) Error() string {
	if len(r.Errors) == 0 {
		return ""
	}

	var messages []string
	for _, err := range r.Errors {
		messages = append(messages, err.Error())
	}

	return fmt.Sprintf("validation failed with %d error(s):\n  - %s",
		len(r.Errors), strings.Join(messages, "\n  - "))
}

// ValidateRegistry validates a registry.json structure.
func ValidateRegistry(r *Registry) *ValidationResult {
	result := &ValidationResult{}

	// Validate required fields
	if r.Name == "" {
		result.AddError("name", "is required")
	} else if !namePattern.MatchString(r.Name) {
		result.AddError("name", "must be kebab-case (lowercase, hyphens, start with letter)")
	}

	if r.Version == "" {
		result.AddError("version", "is required")
	}

	if r.Owner.Name == "" {
		result.AddError("owner.name", "is required")
	}

	// Validate collections
	if len(r.Collections) == 0 {
		result.AddWarning("registry has no collections")
	}

	// Check for duplicate collection names and validate each collection
	names := make(map[string]int)
	for i, c := range r.Collections {
		fieldPrefix := fmt.Sprintf("collections[%d]", i)

		if c.Name == "" {
			result.AddError(fieldPrefix, "name is required")
		} else {
			if !namePattern.MatchString(c.Name) {
				result.AddError(fieldPrefix, fmt.Sprintf("name %q must be kebab-case", c.Name))
			}
			if prev, ok := names[c.Name]; ok {
				result.AddError(fieldPrefix, fmt.Sprintf("duplicate collection name %q (first at index %d)", c.Name, prev))
			} else {
				names[c.Name] = i
			}
		}

		// Source is required - check if it's empty
		if c.Source.IsPath() && c.Source.Path == "" && c.Source.Object == nil {
			result.AddError(fieldPrefix, "source is required")
		}

		// Description is optional but warn if missing
		if c.Description == "" {
			result.AddWarning(fmt.Sprintf("%s (%s): missing description", fieldPrefix, c.Name))
		}
	}

	return result
}

// ValidateManifest validates a manifest.json structure.
func ValidateManifest(m *Manifest) *ValidationResult {
	result := &ValidationResult{}

	if m.Name == "" {
		result.AddError("name", "is required")
	} else if !namePattern.MatchString(m.Name) {
		result.AddError("name", "must be kebab-case (lowercase, hyphens, start with letter)")
	}

	if m.Description == "" {
		result.AddError("description", "is required")
	}

	if m.Entrypoint == "" {
		result.AddError("entrypoint", "is required")
	} else if !strings.HasSuffix(m.Entrypoint, ".meow.toml") {
		result.AddError("entrypoint", "must be a .meow.toml file")
	}

	return result
}

// ValidateCollection validates a collection directory structure.
// It validates the manifest and checks that the entrypoint file exists.
func ValidateCollection(dir string, manifest *Manifest) *ValidationResult {
	result := ValidateManifest(manifest)

	// Check entrypoint exists
	if manifest.Entrypoint != "" {
		entryPath := filepath.Join(dir, manifest.Entrypoint)
		if _, err := os.Stat(entryPath); err != nil {
			result.AddError("entrypoint", fmt.Sprintf("not found: %s", manifest.Entrypoint))
		}
	}

	return result
}
