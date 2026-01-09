package orchestrator

import (
	"context"

	"github.com/meow-stack/meow-machine/internal/types"
)

// WorkflowStore provides persistence for workflow state.
type WorkflowStore interface {
	// Create persists a new workflow.
	Create(ctx context.Context, wf *types.Workflow) error

	// Get retrieves a workflow by ID.
	Get(ctx context.Context, id string) (*types.Workflow, error)

	// Save persists workflow state (atomic write).
	Save(ctx context.Context, wf *types.Workflow) error

	// Delete removes a workflow.
	Delete(ctx context.Context, id string) error

	// List returns all workflows matching filter.
	List(ctx context.Context, filter WorkflowFilter) ([]*types.Workflow, error)

	// GetByAgent returns workflows with steps assigned to agent.
	GetByAgent(ctx context.Context, agentID string) ([]*types.Workflow, error)
}

// WorkflowFilter for listing workflows.
type WorkflowFilter struct {
	Status types.WorkflowStatus // Filter by status (empty = all)
}
