package orchestrator

import (
	"context"

	"github.com/akatz-ai/meow/internal/types"
)

// RunStore provides persistence for run state.
type RunStore interface {
	// Create persists a new run.
	Create(ctx context.Context, run *types.Run) error

	// Get retrieves a run by ID.
	Get(ctx context.Context, id string) (*types.Run, error)

	// Save persists run state (atomic write).
	Save(ctx context.Context, run *types.Run) error

	// Delete removes a run.
	Delete(ctx context.Context, id string) error

	// List returns all runs matching filter.
	List(ctx context.Context, filter RunFilter) ([]*types.Run, error)

	// GetByAgent returns runs with steps assigned to agent.
	GetByAgent(ctx context.Context, agentID string) ([]*types.Run, error)
}

// RunFilter for listing runs.
type RunFilter struct {
	Status types.RunStatus // Filter by status (empty = all)
}
