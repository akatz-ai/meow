package template

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	Beads      []*types.Bead
	StepToID   map[string]string // step ID -> bead ID mapping
	WorkflowID string            // Unique workflow instance ID
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
		Beads:      beads,
		StepToID:   stepToID,
		WorkflowID: b.WorkflowID,
	}, nil
}

// BakeWorkflow transforms a module-format workflow into beads with tier detection.
func (b *Baker) BakeWorkflow(workflow *Workflow, vars map[string]string) (*BakeResult, error) {
	if workflow == nil {
		return nil, fmt.Errorf("workflow is nil")
	}

	// Apply provided variables to context
	for k, v := range vars {
		b.VarContext.Set(k, v)
	}

	// Apply variable defaults from workflow
	for name, v := range workflow.Variables {
		if v.Default != nil && b.VarContext.Get(name) == "" {
			switch d := v.Default.(type) {
			case string:
				b.VarContext.Set(name, d)
			default:
				b.VarContext.Set(name, fmt.Sprintf("%v", d))
			}
		}
	}

	// Validate required variables
	for name, v := range workflow.Variables {
		if v.Required && b.VarContext.Get(name) == "" {
			return nil, fmt.Errorf("required variable %q not provided", name)
		}
	}

	// Set builtin variables
	b.VarContext.SetBuiltin("workflow_id", b.WorkflowID)
	b.VarContext.SetBuiltin("molecule_id", b.WorkflowID)

	// Generate step ID to bead ID mapping
	stepToID := make(map[string]string)
	for _, step := range workflow.Steps {
		beadID := b.generateBeadID(step.ID)
		stepToID[step.ID] = beadID
	}

	// Determine HookBead from hooks_to variable
	var hookBead string
	if workflow.HooksTo != "" {
		hookBead = b.VarContext.Get(workflow.HooksTo)
	}

	// Process steps in order (topological sort for complex deps)
	var beads []*types.Bead
	for _, step := range workflow.Steps {
		bead, err := b.workflowStepToBead(step, stepToID, workflow, hookBead)
		if err != nil {
			return nil, fmt.Errorf("bake step %q: %w", step.ID, err)
		}
		beads = append(beads, bead)
	}

	return &BakeResult{
		Beads:      beads,
		StepToID:   stepToID,
		WorkflowID: b.WorkflowID,
	}, nil
}

// workflowStepToBead converts a module-format workflow step to a bead with tier detection.
func (b *Baker) workflowStepToBead(step *Step, stepToID map[string]string, workflow *Workflow, hookBead string) (*types.Bead, error) {
	beadID := stepToID[step.ID]

	// Set step-specific builtins BEFORE substitution
	b.VarContext.SetBuiltin("step_id", step.ID)
	b.VarContext.SetBuiltin("bead_id", beadID)

	// Substitute variables in string fields
	title := step.Title
	if title == "" {
		title = step.Description
	}
	if title != "" {
		var err error
		title, err = b.VarContext.Substitute(title)
		if err != nil {
			return nil, fmt.Errorf("substitute title: %w", err)
		}
	}

	instructions := step.Instructions
	if instructions != "" {
		var err error
		instructions, err = b.VarContext.Substitute(instructions)
		if err != nil {
			return nil, fmt.Errorf("substitute instructions: %w", err)
		}
	}

	assignee := step.Assignee
	if assignee != "" {
		var err error
		assignee, err = b.VarContext.Substitute(assignee)
		if err != nil {
			return nil, fmt.Errorf("substitute assignee: %w", err)
		}
	}
	// Use default assignee if none specified
	if assignee == "" {
		assignee = b.Assignee
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

	// Determine bead type from step.Type
	beadType := b.determineBeadTypeFromString(step.Type)

	// Determine tier based on workflow and step type
	tier := b.determineTier(step, workflow)

	// Create bead
	bead := &types.Bead{
		ID:             beadID,
		Type:           beadType,
		Title:          title,
		Description:    step.Description,
		Status:         types.BeadStatusOpen,
		Assignee:       assignee,
		Needs:          needs,
		Parent:         b.ParentBead,
		Tier:           tier,
		HookBead:       hookBead,
		SourceWorkflow: workflow.Name,
		WorkflowID:     b.WorkflowID,
		CreatedAt:      b.Now(),
		Instructions:   instructions,
	}

	// Validate gate has no assignee
	if beadType == types.BeadTypeGate {
		bead.Assignee = ""
	}

	// Add ephemeral label if step or workflow is ephemeral
	if step.Ephemeral || workflow.Ephemeral {
		bead.Labels = append(bead.Labels, "meow:ephemeral")
	}

	// Set type-specific specs
	if err := b.setTypeSpec(bead, step, stepToID); err != nil {
		return nil, err
	}

	return bead, nil
}

// determineTier determines the bead tier based on workflow and step type.
func (b *Baker) determineTier(step *Step, workflow *Workflow) types.BeadTier {
	stepType := step.Type
	if stepType == "" {
		stepType = "task" // Default
	}

	switch stepType {
	case "task", "collaborative":
		if workflow.Ephemeral {
			return types.TierWisp
		}
		return types.TierWork
	case "gate":
		return types.TierOrchestrator
	default:
		// condition, code, start, stop, expand are all orchestrator
		return types.TierOrchestrator
	}
}

// determineBeadTypeFromString converts a string type to BeadType.
func (b *Baker) determineBeadTypeFromString(stepType string) types.BeadType {
	switch stepType {
	case "task", "":
		return types.BeadTypeTask
	case "collaborative":
		return types.BeadTypeCollaborative
	case "gate":
		return types.BeadTypeGate
	case "condition":
		return types.BeadTypeCondition
	case "start":
		return types.BeadTypeStart
	case "stop":
		return types.BeadTypeStop
	case "code":
		return types.BeadTypeCode
	case "expand":
		return types.BeadTypeExpand
	default:
		return types.BeadTypeTask
	}
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
	case "task":
		return types.BeadTypeTask
	case "collaborative":
		return types.BeadTypeCollaborative
	case "gate":
		return types.BeadTypeGate
	case "condition":
		return types.BeadTypeCondition
	case "code":
		return types.BeadTypeCode
	case "start":
		return types.BeadTypeStart
	case "stop":
		return types.BeadTypeStop
	case "expand":
		return types.BeadTypeExpand
	}

	// Infer from other fields if type not specified
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

	case types.BeadTypeCollaborative:
		// Collaborative beads also use Instructions, no special spec needed
		return nil

	case types.BeadTypeGate:
		// Gates don't need a spec - they're approved by humans via meow approve
		return nil

	case types.BeadTypeCondition:
		spec := &types.ConditionSpec{
			Condition: step.Condition,
			Timeout:   step.Timeout,
		}

		if step.OnTrue != nil {
			target, err := b.expansionTargetToTypes(step.OnTrue)
			if err != nil {
				return fmt.Errorf("converting on_true: %w", err)
			}
			spec.OnTrue = target
		}
		if step.OnFalse != nil {
			target, err := b.expansionTargetToTypes(step.OnFalse)
			if err != nil {
				return fmt.Errorf("converting on_false: %w", err)
			}
			spec.OnFalse = target
		}
		if step.OnTimeout != nil {
			target, err := b.expansionTargetToTypes(step.OnTimeout)
			if err != nil {
				return fmt.Errorf("converting on_timeout: %w", err)
			}
			spec.OnTimeout = target
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

	case types.BeadTypeCode:
		// Code beads need a CodeSpec
		spec := &types.CodeSpec{
			Code: step.Code,
		}
		bead.CodeSpec = spec
		return nil

	case types.BeadTypeStart:
		// Start beads need a StartSpec - agent comes from assignee
		spec := &types.StartSpec{
			Agent: step.Assignee,
		}
		bead.StartSpec = spec
		return nil

	case types.BeadTypeStop:
		// Stop beads need a StopSpec - agent comes from assignee
		spec := &types.StopSpec{
			Agent: step.Assignee,
		}
		bead.StopSpec = spec
		return nil

	default:
		return fmt.Errorf("unsupported bead type: %s", bead.Type)
	}
}

// expansionTargetToTypes converts template ExpansionTarget to types.ExpansionTarget.
func (b *Baker) expansionTargetToTypes(et *ExpansionTarget) (*types.ExpansionTarget, error) {
	if et == nil {
		return nil, nil
	}

	result := &types.ExpansionTarget{
		Template:  et.Template,
		Variables: et.Variables,
	}

	// Serialize inline steps to json.RawMessage for storage
	if len(et.Inline) > 0 {
		result.Inline = make([]json.RawMessage, len(et.Inline))
		for i, step := range et.Inline {
			data, err := json.Marshal(step)
			if err != nil {
				return nil, fmt.Errorf("marshaling inline step %q: %w", step.ID, err)
			}
			result.Inline[i] = data
		}
	}

	return result, nil
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
	for _, step := range steps {
		beadID := stepToID[step.ID]

		// Set step-specific builtins for substitution
		b.VarContext.SetBuiltin("step_id", step.ID)
		b.VarContext.SetBuiltin("bead_id", beadID)

		// Substitute variables in description and instructions
		description := step.Description
		instructions := step.Instructions
		if description != "" {
			var err error
			description, err = b.VarContext.Substitute(description)
			if err != nil {
				return nil, fmt.Errorf("substitute description in inline step %q: %w", step.ID, err)
			}
		}
		if instructions != "" {
			var err error
			instructions, err = b.VarContext.Substitute(instructions)
			if err != nil {
				return nil, fmt.Errorf("substitute instructions in inline step %q: %w", step.ID, err)
			}
		}

		// Translate dependencies
		var needs []string
		hasInternalDep := false
		for _, need := range step.Needs {
			if beadNeed, ok := stepToID[need]; ok {
				needs = append(needs, beadNeed)
				hasInternalDep = true
			} else {
				// Could be a dependency on the parent or a prior step
				needs = append(needs, need)
			}
		}

		// Steps without internal dependencies must depend on the parent.
		// This ensures they don't execute until the parent is complete.
		if !hasInternalDep && parentBeadID != "" {
			needs = append([]string{parentBeadID}, needs...)
		}

		beadType := types.BeadType(step.Type)
		if !beadType.Valid() {
			beadType = types.BeadTypeTask
		}

		bead := &types.Bead{
			ID:          beadID,
			Type:        beadType,
			Title:       description,
			Description: instructions,
			Status:      types.BeadStatusOpen,
			Assignee:    b.Assignee,
			Needs:       needs,
			Parent:      parentBeadID,
			CreatedAt:   b.Now(),
		}

		beads = append(beads, bead)
	}

	return beads, nil
}
