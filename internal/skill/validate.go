package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// namePattern matches lowercase alphanumeric with single hyphens between words
	namePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	// semverPattern matches strict semver (X.Y.Z)
	semverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+$`)
)

// ValidationError represents a single validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s %s", e.Field, e.Message)
}

// ValidationResult holds validation errors.
type ValidationResult struct {
	Errors []ValidationError
}

// HasErrors returns true if there are any validation errors.
func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
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

// Add appends a validation error.
func (r *ValidationResult) Add(field, message string) {
	r.Errors = append(r.Errors, ValidationError{Field: field, Message: message})
}

// Validate checks the skill manifest for errors.
// baseDir should be the skill directory path (used for file existence checks
// and validating that skill name matches directory name).
func (s *Skill) Validate(baseDir string) *ValidationResult {
	result := &ValidationResult{}

	validateSkillMeta(s.Skill, baseDir, result)
	validateTargets(s.Targets, result)
	validateFiles(s.Skill.Files, baseDir, result)

	return result
}

func validateSkillMeta(meta SkillMeta, baseDir string, result *ValidationResult) {
	// Validate name
	if meta.Name == "" {
		result.Add("skill.name", "is required")
	} else {
		if !namePattern.MatchString(meta.Name) {
			result.Add("skill.name", "must be lowercase alphanumeric with hyphens")
		}

		// Check name matches directory name
		dirName := filepath.Base(baseDir)
		if meta.Name != dirName {
			result.Add("skill.name", fmt.Sprintf("must match directory name (got %q, directory is %q)", meta.Name, dirName))
		}
	}

	// Validate description
	if meta.Description == "" {
		result.Add("skill.description", "is required")
	} else if len(meta.Description) > 1024 {
		result.Add("skill.description", "must be 1024 characters or less")
	}

	// Validate version (optional, but must be semver if provided)
	if meta.Version != "" && !semverPattern.MatchString(meta.Version) {
		result.Add("skill.version", "must be semver format (X.Y.Z)")
	}
}

func validateTargets(targets map[string]Target, result *ValidationResult) {
	if len(targets) == 0 {
		result.Add("targets", "at least one target is required")
		return
	}

	for name, target := range targets {
		// If custom path is specified, it must contain {{name}} placeholder
		if target.Path != "" {
			// Check if this is a known target - if so, custom path is an override
			_, isKnown := KnownTargets[name]
			if !isKnown && !strings.Contains(target.Path, "{{name}}") {
				result.Add(fmt.Sprintf("targets.%s.path", name), "custom path must contain {{name}} placeholder")
			}
		}
	}
}

func validateFiles(files []string, baseDir string, result *ValidationResult) {
	for _, file := range files {
		filePath := filepath.Join(baseDir, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			result.Add("skill.files", fmt.Sprintf("file %q does not exist", file))
		}
	}
}
