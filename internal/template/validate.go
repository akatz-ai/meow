package template

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ValidationError represents a single validation error.
type ValidationError struct {
	Template string // Template name
	StepID   string // Step ID if applicable
	Field    string // Field name
	Message  string // Error message
	Suggest  string // Suggestion for fixing
}

func (e ValidationError) Error() string {
	var parts []string
	if e.Template != "" {
		parts = append(parts, fmt.Sprintf("template %q", e.Template))
	}
	if e.StepID != "" {
		parts = append(parts, fmt.Sprintf("step %q", e.StepID))
	}
	if e.Field != "" {
		parts = append(parts, fmt.Sprintf("field %q", e.Field))
	}

	location := strings.Join(parts, ", ")
	msg := e.Message
	if e.Suggest != "" {
		msg += fmt.Sprintf(" (suggestion: %s)", e.Suggest)
	}

	if location != "" {
		return fmt.Sprintf("%s: %s", location, msg)
	}
	return msg
}

// ValidationResult holds all validation errors.
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
	var msgs []string
	for _, e := range r.Errors {
		msgs = append(msgs, e.Error())
	}
	return fmt.Sprintf("validation failed with %d error(s):\n  - %s",
		len(r.Errors), strings.Join(msgs, "\n  - "))
}

// Add adds a validation error.
func (r *ValidationResult) Add(template, stepID, field, message, suggest string) {
	r.Errors = append(r.Errors, ValidationError{
		Template: template,
		StepID:   stepID,
		Field:    field,
		Message:  message,
		Suggest:  suggest,
	})
}

// ValidateFull performs comprehensive validation on a template.
// Returns all errors found, not just the first one.
func ValidateFull(t *Template) *ValidationResult {
	result := &ValidationResult{}
	name := t.Meta.Name

	// Basic structure validation
	validateMeta(t, name, result)
	validateSteps(t, name, result)
	validateDependencies(t, name, result)
	validateVariableReferences(t, name, result)
	validateTypeSpecific(t, name, result)

	return result
}

func validateMeta(t *Template, name string, result *ValidationResult) {
	if t.Meta.Name == "" {
		result.Add(name, "", "meta.name", "name is required", "add name = \"my-template\"")
	}

	if t.Meta.Version != "" {
		// Simple semver check
		if !regexp.MustCompile(`^\d+\.\d+\.\d+`).MatchString(t.Meta.Version) {
			result.Add(name, "", "meta.version", "version should be semver format", "use format X.Y.Z")
		}
	}

	// Validate on_error if set
	if t.Meta.OnError != "" {
		valid := map[string]bool{
			"continue": true, "abort": true, "retry": true, "inject-gate": true,
		}
		if !valid[t.Meta.OnError] {
			result.Add(name, "", "meta.on_error",
				fmt.Sprintf("invalid on_error: %q", t.Meta.OnError),
				"use continue, abort, retry, or inject-gate")
		}
	}

	// Validate type if set
	if t.Meta.Type != "" {
		valid := map[string]bool{"loop": true, "linear": true}
		if !valid[t.Meta.Type] {
			result.Add(name, "", "meta.type",
				fmt.Sprintf("invalid type: %q", t.Meta.Type),
				"use loop or linear")
		}
	}
}

func validateSteps(t *Template, name string, result *ValidationResult) {
	if len(t.Steps) == 0 {
		result.Add(name, "", "steps", "template must have at least one step", "add [[steps]] section")
		return
	}

	stepIDs := make(map[string]int) // step ID -> index
	for i, step := range t.Steps {
		if step.ID == "" {
			result.Add(name, fmt.Sprintf("steps[%d]", i), "id", "step id is required", "")
			continue
		}

		if prevIdx, exists := stepIDs[step.ID]; exists {
			result.Add(name, step.ID, "id",
				fmt.Sprintf("duplicate step id (first at index %d)", prevIdx),
				"use unique step ids")
		}
		stepIDs[step.ID] = i

		// Validate step type if set
		if step.Type != "" {
			valid := map[string]bool{
				"blocking-gate": true, "restart": true,
			}
			if !valid[step.Type] {
				result.Add(name, step.ID, "type",
					fmt.Sprintf("invalid step type: %q", step.Type),
					"use blocking-gate or restart")
			}
		}
	}
}

func validateDependencies(t *Template, name string, result *ValidationResult) {
	stepIDs := make(map[string]bool)
	for _, step := range t.Steps {
		stepIDs[step.ID] = true
	}

	// Check that all needs references exist
	for _, step := range t.Steps {
		for _, need := range step.Needs {
			if !stepIDs[need] {
				suggest := findSimilar(need, stepIDs)
				result.Add(name, step.ID, "needs",
					fmt.Sprintf("references unknown step %q", need),
					suggest)
			}
		}
	}

	// Check for cycles using DFS
	if cycle := findCycle(t); len(cycle) > 0 {
		result.Add(name, "", "needs",
			fmt.Sprintf("circular dependency detected: %s", strings.Join(cycle, " → ")),
			"remove one of the dependencies to break the cycle")
	}
}

// findCycle returns the cycle path if one exists, empty slice otherwise.
func findCycle(t *Template) []string {
	// Build adjacency list (step -> its dependencies)
	deps := make(map[string][]string)
	for _, step := range t.Steps {
		deps[step.ID] = step.Needs
	}

	// States: 0 = unvisited, 1 = visiting, 2 = visited
	state := make(map[string]int)
	parent := make(map[string]string)

	var cycle []string

	var dfs func(id string) bool
	dfs = func(id string) bool {
		state[id] = 1 // visiting

		for _, dep := range deps[id] {
			if state[dep] == 1 {
				// Found cycle - reconstruct path
				cycle = []string{dep}
				for cur := id; cur != dep; {
					cycle = append([]string{cur}, cycle...)
					cur = parent[cur]
				}
				cycle = append([]string{dep}, cycle...)
				return true
			}
			if state[dep] == 0 {
				parent[dep] = id
				if dfs(dep) {
					return true
				}
			}
		}

		state[id] = 2 // visited
		return false
	}

	for _, step := range t.Steps {
		if state[step.ID] == 0 {
			if dfs(step.ID) {
				return cycle
			}
		}
	}

	return nil
}

// varRefPattern matches {{variable}} patterns
var varRefPattern = regexp.MustCompile(`\{\{([^{}]+)\}\}`)

func validateVariableReferences(t *Template, name string, result *ValidationResult) {
	// Collect all defined variables
	defined := make(map[string]bool)
	for varName := range t.Variables {
		defined[varName] = true
	}

	// Add builtins
	builtins := []string{
		"timestamp", "date", "time", "agent", "bead_id", "molecule_id",
		"workflow_id", "step_id",
	}
	for _, b := range builtins {
		defined[b] = true
	}

	// Check all string fields in steps for variable references
	for _, step := range t.Steps {
		checkVarRefs(step.Description, name, step.ID, "description", defined, t, result)
		checkVarRefs(step.Instructions, name, step.ID, "instructions", defined, t, result)
		checkVarRefs(step.Condition, name, step.ID, "condition", defined, t, result)
		checkVarRefs(step.Template, name, step.ID, "template", defined, t, result)
		checkVarRefs(step.Validation, name, step.ID, "validation", defined, t, result)

		for k, v := range step.Variables {
			checkVarRefs(v, name, step.ID, fmt.Sprintf("variables.%s", k), defined, t, result)
		}

		if step.OnTrue != nil {
			checkExpansionTargetVarRefs(step.OnTrue, name, step.ID, "on_true", defined, t, result)
		}
		if step.OnFalse != nil {
			checkExpansionTargetVarRefs(step.OnFalse, name, step.ID, "on_false", defined, t, result)
		}
		if step.OnTimeout != nil {
			checkExpansionTargetVarRefs(step.OnTimeout, name, step.ID, "on_timeout", defined, t, result)
		}
	}
}

func checkExpansionTargetVarRefs(target *ExpansionTarget, name, stepID, field string, defined map[string]bool, t *Template, result *ValidationResult) {
	checkVarRefs(target.Template, name, stepID, field+".template", defined, t, result)
	for k, v := range target.Variables {
		checkVarRefs(v, name, stepID, fmt.Sprintf("%s.variables.%s", field, k), defined, t, result)
	}
}

func checkVarRefs(text, name, stepID, field string, defined map[string]bool, t *Template, result *ValidationResult) {
	if text == "" {
		return
	}

	matches := varRefPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		path := strings.TrimSpace(match[1])
		parts := strings.Split(path, ".")
		root := parts[0]

		// Skip output references - they're validated at runtime
		if root == "output" || (len(parts) >= 2 && parts[1] == "outputs") {
			continue
		}

		if !defined[root] {
			suggest := findSimilarVar(root, defined)
			result.Add(name, stepID, field,
				fmt.Sprintf("undefined variable %q", root),
				suggest)
		}
	}
}

func validateTypeSpecific(t *Template, name string, result *ValidationResult) {
	for _, step := range t.Steps {
		// Condition steps should have branches
		if step.Condition != "" && step.OnTrue == nil && step.OnFalse == nil && step.Type != "restart" {
			result.Add(name, step.ID, "condition",
				"condition without on_true or on_false branch",
				"add on_true and/or on_false to specify branch actions")
		}

		// Template reference should be non-empty if specified
		if step.Template != "" && !strings.Contains(step.Template, "{{") {
			// Static template reference - could validate existence but that's runtime
		}

		// Restart steps should have condition
		if step.Type == "restart" && step.Condition == "" {
			result.Add(name, step.ID, "type",
				"restart step without condition",
				"add condition to control when loop restarts")
		}

		// blocking-gate type should be explicit
		if step.Type == "blocking-gate" && step.Instructions == "" {
			result.Add(name, step.ID, "type",
				"blocking-gate without instructions",
				"add instructions explaining what the human should do")
		}
	}
}

// findSimilar finds a similar step ID for "did you mean" suggestions.
func findSimilar(target string, candidates map[string]bool) string {
	var best string
	bestScore := 0

	for candidate := range candidates {
		score := similarity(target, candidate)
		if score > bestScore {
			bestScore = score
			best = candidate
		}
	}

	if bestScore > len(target)/2 {
		return fmt.Sprintf("did you mean %q?", best)
	}
	return ""
}

func findSimilarVar(target string, candidates map[string]bool) string {
	var all []string
	for c := range candidates {
		all = append(all, c)
	}
	sort.Strings(all)

	for _, candidate := range all {
		score := similarity(target, candidate)
		if score > len(target)/2 {
			return fmt.Sprintf("did you mean %q?", candidate)
		}
	}
	return ""
}

// similarity returns a simple score based on common prefix and suffix.
func similarity(a, b string) int {
	score := 0

	// Common prefix
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] == b[i] {
			score++
		} else {
			break
		}
	}

	// Common suffix
	for i := 0; i < minLen-score; i++ {
		if a[len(a)-1-i] == b[len(b)-1-i] {
			score++
		} else {
			break
		}
	}

	return score
}
