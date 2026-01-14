package collection

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/meow-stack/meow-machine/internal/workflow"
)

var (
	namePattern   = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
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

// Validate checks the collection manifest for errors.
func (c *Collection) Validate(baseDir string) *ValidationResult {
	result := &ValidationResult{}

	validateCollectionMeta(c.Collection, result)
	validatePacks(c.Packs, baseDir, result)

	return result
}

func validateCollectionMeta(meta CollectionMeta, result *ValidationResult) {
	if meta.Name == "" {
		result.Add("collection.name", "is required")
	} else if !namePattern.MatchString(meta.Name) {
		result.Add("collection.name", "must be lowercase alphanumeric with hyphens")
	}

	if meta.Description == "" {
		result.Add("collection.description", "is required")
	}

	if meta.Version == "" {
		result.Add("collection.version", "is required")
	} else if !semverPattern.MatchString(meta.Version) {
		result.Add("collection.version", "must be semver format")
	}

	if meta.Owner.Name == "" {
		result.Add("collection.owner.name", "is required")
	}

	if meta.MeowVersion != "" {
		if err := validateConstraint(meta.MeowVersion); err != nil {
			result.Add("collection.meow_version", fmt.Sprintf("meow_version constraint is invalid: %v", err))
		}
	}
}

func validatePacks(packs []Pack, baseDir string, result *ValidationResult) {
	if len(packs) == 0 {
		result.Add("packs", "are required")
		return
	}

	seen := make(map[string]int)
	for i, pack := range packs {
		fieldPrefix := fmt.Sprintf("packs[%d]", i)
		if pack.Name == "" {
			result.Add(fieldPrefix+".name", "is required")
		} else {
			if !namePattern.MatchString(pack.Name) {
				result.Add(fieldPrefix+".name", "must be lowercase alphanumeric with hyphens")
			}
			if prev, ok := seen[pack.Name]; ok {
				result.Add(fieldPrefix+".name", fmt.Sprintf("duplicate pack name %q (first at index %d)", pack.Name, prev))
			} else {
				seen[pack.Name] = i
			}
		}

		if pack.Description == "" {
			result.Add(fieldPrefix+".description", "is required")
		}

		if len(pack.Workflows) == 0 {
			result.Add(fieldPrefix+".workflows", "is required")
			continue
		}

		for j, workflowPath := range pack.Workflows {
			workflowField := fmt.Sprintf("%s.workflows[%d]", fieldPrefix, j)
			trimmed := strings.TrimSpace(workflowPath)
			if trimmed == "" {
				result.Add(workflowField, "workflow path is required")
				continue
			}
			if filepath.IsAbs(trimmed) {
				result.Add(workflowField, "workflow path must be relative to repo root")
				continue
			}
			if !strings.HasSuffix(trimmed, ".meow.toml") {
				result.Add(workflowField, "workflow path must end with .meow.toml")
				continue
			}

			fullPath := filepath.Join(baseDir, filepath.FromSlash(trimmed))
			info, err := os.Stat(fullPath)
			if err != nil {
				result.Add(workflowField, "workflow path does not exist")
				continue
			}
			if info.IsDir() {
				result.Add(workflowField, "workflow path must be a file")
				continue
			}

			if _, err := workflow.ParseFile(fullPath); err != nil {
				result.Add(workflowField, fmt.Sprintf("workflow file is invalid: %v", err))
			}
		}
	}
}

type semver struct {
	major int
	minor int
	patch int
}

func parseSemver(value string) (semver, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("expected version in X.Y.Z format")
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 0 {
		return semver{}, fmt.Errorf("invalid major version")
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil || minor < 0 {
		return semver{}, fmt.Errorf("invalid minor version")
	}
	patch, err := strconv.Atoi(parts[2])
	if err != nil || patch < 0 {
		return semver{}, fmt.Errorf("invalid patch version")
	}

	return semver{major: major, minor: minor, patch: patch}, nil
}

func compareSemver(a, b semver) int {
	if a.major != b.major {
		if a.major < b.major {
			return -1
		}
		return 1
	}
	if a.minor != b.minor {
		if a.minor < b.minor {
			return -1
		}
		return 1
	}
	if a.patch != b.patch {
		if a.patch < b.patch {
			return -1
		}
		return 1
	}
	return 0
}

type versionBound struct {
	version   semver
	inclusive bool
	set       bool
}

func validateConstraint(expr string) error {
	if strings.TrimSpace(expr) == "" {
		return fmt.Errorf("constraint cannot be empty")
	}

	parts := strings.Split(expr, ",")
	var lower versionBound
	var upper versionBound

	for _, part := range parts {
		constraint := strings.TrimSpace(part)
		if constraint == "" {
			return fmt.Errorf("empty constraint")
		}

		op, version, err := parseConstraintPart(constraint)
		if err != nil {
			return err
		}

		switch op {
		case ">=":
			lower = tightenLower(lower, version, true)
		case ">":
			lower = tightenLower(lower, version, false)
		case "<=":
			upper = tightenUpper(upper, version, true)
		case "<":
			upper = tightenUpper(upper, version, false)
		case "=":
			lower = tightenLower(lower, version, true)
			upper = tightenUpper(upper, version, true)
		default:
			return fmt.Errorf("unsupported operator %q", op)
		}
	}

	if lower.set && upper.set {
		cmp := compareSemver(lower.version, upper.version)
		if cmp > 0 {
			return fmt.Errorf("constraint is unsatisfiable")
		}
		if cmp == 0 && (!lower.inclusive || !upper.inclusive) {
			return fmt.Errorf("constraint is unsatisfiable")
		}
	}

	return nil
}

func parseConstraintPart(part string) (string, semver, error) {
	operators := []string{"<=", ">=", "<", ">", "="}
	for _, op := range operators {
		if strings.HasPrefix(part, op) {
			version := strings.TrimSpace(strings.TrimPrefix(part, op))
			return parseConstraintVersion(op, version)
		}
	}

	return parseConstraintVersion("=", part)
}

func parseConstraintVersion(op string, version string) (string, semver, error) {
	if !semverPattern.MatchString(version) {
		return "", semver{}, fmt.Errorf("invalid version %q", version)
	}

	parsed, err := parseSemver(version)
	if err != nil {
		return "", semver{}, err
	}

	return op, parsed, nil
}

func tightenLower(current versionBound, candidate semver, inclusive bool) versionBound {
	if !current.set {
		return versionBound{version: candidate, inclusive: inclusive, set: true}
	}
	cmp := compareSemver(candidate, current.version)
	if cmp > 0 {
		return versionBound{version: candidate, inclusive: inclusive, set: true}
	}
	if cmp == 0 && current.inclusive && !inclusive {
		current.inclusive = false
	}
	return current
}

func tightenUpper(current versionBound, candidate semver, inclusive bool) versionBound {
	if !current.set {
		return versionBound{version: candidate, inclusive: inclusive, set: true}
	}
	cmp := compareSemver(candidate, current.version)
	if cmp < 0 {
		return versionBound{version: candidate, inclusive: inclusive, set: true}
	}
	if cmp == 0 && current.inclusive && !inclusive {
		current.inclusive = false
	}
	return current
}
