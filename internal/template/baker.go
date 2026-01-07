package template

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/meow-stack/meow-machine/internal/types"
)

// Baker transforms templates into executable beads.
type Baker struct {
	// WorkflowID is the prefix for generated bead IDs
	WorkflowID string

	// VarContext provides variable values for substitution
	VarContext *VarContext

	// ParentBead is set when baking a nested expansion
	ParentBead string

	// Assignee is the default agent for task beads
	Assignee string

	// Now allows injecting time for testing
	Now func() time.Time
}

// NewBaker creates a new Baker with default settings.
func NewBaker(workflowID string) *Baker {
	return &Baker{
		WorkflowID: workflowID,
		VarContext: NewVarContext(),
		Now:        time.Now,
	}
}

// BakeResult contains the beads generated from baking a template.
type BakeResult struct {
	Beads    []*types.Bead
	StepToID map[string]string // step ID -> bead ID mapping
}

// Bake transforms a template into beads.
func (b *Baker) Bake(tmpl *Template) (*BakeResult, error) {
	// Validate first
	if result := ValidateFull(tmpl); result.HasErrors() {
		return nil, result
	}

	// Apply defaults
	b.VarContext.ApplyDefaults(tmpl.Variables)

	// Validate required variables
	if err := b.VarContext.ValidateRequired(tmpl.Variables); err != nil {
		return nil, err
	}

	// Set builtin variables
	b.VarContext.SetBuiltin("workflow_id", b.WorkflowID)
	b.VarContext.SetBuiltin("molecule_id", b.WorkflowID)

	// Generate step ID to bead ID mapping
	stepToID := make(map[string]string)
	for _, step := range tmpl.Steps {
		beadID := b.generateBeadID(step.ID)
		stepToID[step.ID] = beadID
	}

	// Process steps in topological order
	order, err := tmpl.StepOrder()
	if err != nil {
		return nil, fmt.Errorf("determine step order: %w", err)
	}

	var beads []*types.Bead
	for _, stepID := range order {
		step := tmpl.GetStep(stepID)
		if step == nil {
			return nil, fmt.Errorf("step %q not found", stepID)
		}

		bead, err := b.stepToBead(step, stepToID)
		if err != nil {
			return nil, fmt.Errorf("bake step %q: %w", stepID, err)
		}
		beads = append(beads, bead)
	}

	return &BakeResult{
		Beads:    beads,
		StepToID: stepToID,
	}, nil
}

// generateBeadID creates a unique bead ID from a step ID.
// Format: {workflow_id}.{step_id}-{hash}
func (b *Baker) generateBeadID(stepID string) string {
	// Create a short hash for uniqueness
	h := sha256.New()
	h.Write([]byte(b.WorkflowID))
	h.Write([]byte(stepID))
	h.Write([]byte(fmt.Sprintf("%d", b.Now().UnixNano())))
	hash := hex.EncodeToString(h.Sum(nil))[:8]

	return fmt.Sprintf("%s.%s-%s", b.WorkflowID, stepID, hash)
}

// stepToBead converts a template step to a bead.
func (b *Baker) stepToBead(step *Step, stepToID map[string]string) (*types.Bead, error) {
	beadID := stepToID[step.ID]

	// Set step-specific builtins BEFORE substitution
	b.VarContext.SetBuiltin("step_id", step.ID)
	b.VarContext.SetBuiltin("bead_id", beadID)

	// Substitute variables in the step
	subbed, err := b.VarContext.SubstituteStep(step)
	if err != nil {
		return nil, err
	}

	// Translate dependencies from step IDs to bead IDs
	var needs []string
	for _, need := range step.Needs {
		if beadNeed, ok := stepToID[need]; ok {
			needs = append(needs, beadNeed)
		} else {
			return nil, fmt.Errorf("unknown dependency: %s", need)
		}
	}

	// Determine bead type
	beadType := b.determineBeadType(subbed)

	// Create base bead
	bead := &types.Bead{
		ID:          beadID,
		Type:        beadType,
		Title:       subbed.Description,
		Description: subbed.Instructions,
		Status:      types.BeadStatusOpen,
		Assignee:    b.Assignee,
		Needs:       needs,
		Parent:      b.ParentBead,
		CreatedAt:   b.Now(),
	}

	// Add ephemeral label if step is ephemeral
	if step.Ephemeral {
		bead.Labels = append(bead.Labels, "meow:ephemeral")
	}

	// Set instructions for task beads
	if beadType == types.BeadTypeTask {
		bead.Instructions = subbed.Instructions
	}

	// Set type-specific specs
	if err := b.setTypeSpec(bead, subbed, stepToID); err != nil {
		return nil, err
	}

	return bead, nil
}

// determineBeadType infers the bead type from step fields.
func (b *Baker) determineBeadType(step *Step) types.BeadType {
	// Explicit type via step.Type
	switch step.Type {
	case "blocking-gate":
		// Blocking gates are condition beads that wait
		return types.BeadTypeCondition
	case "restart":
		// Restart is a condition that controls loop continuation
		return types.BeadTypeCondition
	}

	// Infer from other fields
	if step.Template != "" {
		return types.BeadTypeExpand
	}
	if step.Condition != "" {
		return types.BeadTypeCondition
	}

	// Default to task
	return types.BeadTypeTask
}

// setTypeSpec sets the type-specific spec on the bead.
func (b *Baker) setTypeSpec(bead *types.Bead, step *Step, stepToID map[string]string) error {
	switch bead.Type {
	case types.BeadTypeTask:
		// Task beads don't need a spec, they use Instructions
		return nil

	case types.BeadTypeCondition:
		spec := &types.ConditionSpec{
			Condition: step.Condition,
			Timeout:   step.Timeout,
		}

		if step.OnTrue != nil {
			spec.OnTrue = b.expansionTargetToTypes(step.OnTrue)
		}
		if step.OnFalse != nil {
			spec.OnFalse = b.expansionTargetToTypes(step.OnFalse)
		}
		if step.OnTimeout != nil {
			spec.OnTimeout = b.expansionTargetToTypes(step.OnTimeout)
		}

		// For blocking-gate type, set a blocking condition
		if step.Type == "blocking-gate" {
			// The orchestrator will handle this specially
			spec.Condition = "meow wait-approve --bead " + bead.ID
		}

		bead.ConditionSpec = spec
		return nil

	case types.BeadTypeExpand:
		spec := &types.ExpandSpec{
			Template:  step.Template,
			Assignee:  b.Assignee,
			Ephemeral: step.Ephemeral,
		}
		if len(step.Variables) > 0 {
			spec.Variables = step.Variables
		}
		bead.ExpandSpec = spec
		return nil

	default:
		return fmt.Errorf("unsupported bead type: %s", bead.Type)
	}
}

// expansionTargetToTypes converts template ExpansionTarget to types.ExpansionTarget.
func (b *Baker) expansionTargetToTypes(et *ExpansionTarget) *types.ExpansionTarget {
	if et == nil {
		return nil
	}

	return &types.ExpansionTarget{
		Template:  et.Template,
		Variables: et.Variables,
		// Note: Inline steps are handled separately during expansion
	}
}

// BakeInline converts inline steps into beads.
// Used when a condition branch has inline steps instead of a template reference.
func (b *Baker) BakeInline(steps []InlineStep, parentBeadID string) ([]*types.Bead, error) {
	prevParent := b.ParentBead
	b.ParentBead = parentBeadID
	defer func() { b.ParentBead = prevParent }()

	var beads []*types.Bead
	stepToID := make(map[string]string)

	// First pass: generate IDs
	for _, step := range steps {
		stepToID[step.ID] = b.generateBeadID(step.ID)
	}

	// Second pass: create beads
	for i, step := range steps {
		// Translate dependencies
		var needs []string
		for _, need := range step.Needs {
			if beadNeed, ok := stepToID[need]; ok {
				needs = append(needs, beadNeed)
			} else {
				// Could be a dependency on the parent or a prior step
				needs = append(needs, need)
			}
		}

		beadType := types.BeadType(step.Type)
		if !beadType.Valid() {
			beadType = types.BeadTypeTask
		}

		bead := &types.Bead{
			ID:          stepToID[step.ID],
			Type:        beadType,
			Title:       step.Description,
			Description: step.Instructions,
			Status:      types.BeadStatusOpen,
			Assignee:    b.Assignee,
			Needs:       needs,
			Parent:      parentBeadID,
			CreatedAt:   b.Now(),
		}

		if i == 0 && parentBeadID != "" {
			// First inline step depends on parent completing
			bead.Needs = append([]string{parentBeadID}, bead.Needs...)
		}

		beads = append(beads, bead)
	}

	return beads, nil
}
