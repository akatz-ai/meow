package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/meow-stack/meow-machine/internal/config"
	"github.com/meow-stack/meow-machine/internal/types"
)

// BeadStore provides access to bead state.
type BeadStore interface {
	// GetNextReady returns the next bead that is ready to execute.
	// A bead is ready if all its dependencies are closed.
	// Returns nil if no beads are ready.
	GetNextReady(ctx context.Context) (*types.Bead, error)

	// Get retrieves a bead by ID.
	Get(ctx context.Context, id string) (*types.Bead, error)

	// Update saves changes to a bead.
	Update(ctx context.Context, bead *types.Bead) error

	// AllDone returns true if there are no open or in-progress beads.
	AllDone(ctx context.Context) (bool, error)
}

// AgentManager manages agent lifecycle.
type AgentManager interface {
	// Start spawns an agent in a tmux session.
	Start(ctx context.Context, spec *types.StartSpec) error

	// Stop kills an agent's tmux session.
	Stop(ctx context.Context, spec *types.StopSpec) error

	// IsRunning checks if an agent is currently running.
	IsRunning(ctx context.Context, agentID string) (bool, error)
}

// TemplateExpander expands templates into beads.
type TemplateExpander interface {
	// Expand loads a template and creates beads from it.
	Expand(ctx context.Context, spec *types.ExpandSpec, parentBead *types.Bead) error
}

// CodeExecutor runs shell code.
type CodeExecutor interface {
	// Execute runs shell code and captures outputs.
	Execute(ctx context.Context, spec *types.CodeSpec) (map[string]any, error)
}

// Orchestrator is the main workflow engine.
type Orchestrator struct {
	cfg      *config.Config
	store    BeadStore
	agents   AgentManager
	expander TemplateExpander
	executor CodeExecutor
	logger   *slog.Logger

	// Shutdown coordination
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Condition goroutines
	condMu       sync.Mutex
	condRoutines map[string]context.CancelFunc
}

// New creates a new Orchestrator.
func New(cfg *config.Config, store BeadStore, agents AgentManager, expander TemplateExpander, executor CodeExecutor, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		cfg:          cfg,
		store:        store,
		agents:       agents,
		expander:     expander,
		executor:     executor,
		logger:       logger,
		condRoutines: make(map[string]context.CancelFunc),
	}
}

// Run starts the orchestrator main loop.
// It blocks until the context is cancelled or all work is done.
func (o *Orchestrator) Run(ctx context.Context) error {
	ctx, o.cancel = context.WithCancel(ctx)
	defer o.cancel()

	o.logger.Info("orchestrator starting")

	ticker := time.NewTicker(o.cfg.Orchestrator.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			o.logger.Info("orchestrator shutting down", "reason", ctx.Err())
			o.waitForConditions()
			return ctx.Err()

		case <-ticker.C:
			if err := o.tick(ctx); err != nil {
				if err == errAllDone {
					o.logger.Info("all work complete")
					o.waitForConditions()
					return nil
				}
				o.logger.Error("tick error", "error", err)
				// Continue running on non-fatal errors
			}
		}
	}
}

// Shutdown gracefully stops the orchestrator.
func (o *Orchestrator) Shutdown() {
	if o.cancel != nil {
		o.cancel()
	}
	o.waitForConditions()
}

var errAllDone = fmt.Errorf("all beads complete")

// tick performs one iteration of the main loop.
func (o *Orchestrator) tick(ctx context.Context) error {
	// Check if all done
	done, err := o.store.AllDone(ctx)
	if err != nil {
		return fmt.Errorf("checking completion: %w", err)
	}
	if done {
		return errAllDone
	}

	// Get next ready bead
	bead, err := o.store.GetNextReady(ctx)
	if err != nil {
		return fmt.Errorf("getting next ready bead: %w", err)
	}
	if bead == nil {
		// No ready beads, but not done - waiting on dependencies or conditions
		return nil
	}

	// Dispatch based on type
	return o.dispatch(ctx, bead)
}

// dispatch routes a bead to the appropriate handler.
func (o *Orchestrator) dispatch(ctx context.Context, bead *types.Bead) error {
	o.logger.Info("dispatching bead",
		"id", bead.ID,
		"type", bead.Type,
		"title", bead.Title,
	)

	switch bead.Type {
	case types.BeadTypeTask:
		return o.handleTask(ctx, bead)
	case types.BeadTypeCondition:
		return o.handleCondition(ctx, bead)
	case types.BeadTypeStop:
		return o.handleStop(ctx, bead)
	case types.BeadTypeStart:
		return o.handleStart(ctx, bead)
	case types.BeadTypeCode:
		return o.handleCode(ctx, bead)
	case types.BeadTypeExpand:
		return o.handleExpand(ctx, bead)
	default:
		return fmt.Errorf("unknown bead type: %s", bead.Type)
	}
}

// handleTask waits for an agent to close the task bead.
func (o *Orchestrator) handleTask(ctx context.Context, bead *types.Bead) error {
	// Ensure the assigned agent is running
	if bead.Assignee != "" {
		running, err := o.agents.IsRunning(ctx, bead.Assignee)
		if err != nil {
			return fmt.Errorf("checking agent %s: %w", bead.Assignee, err)
		}
		if !running {
			o.logger.Warn("task assigned to non-running agent",
				"bead", bead.ID,
				"agent", bead.Assignee,
			)
			// Don't spawn automatically - let the workflow handle it
		}
	}

	// Mark as in_progress if not already
	if bead.Status == types.BeadStatusOpen {
		bead.Status = types.BeadStatusInProgress
		if err := o.store.Update(ctx, bead); err != nil {
			return fmt.Errorf("updating bead status: %w", err)
		}
	}

	// Task beads are closed by agents via `meow close`
	// The orchestrator just waits - it will be picked up on next tick
	return nil
}

// handleCondition evaluates a condition and expands the appropriate branch.
// Conditions run in goroutines to avoid blocking the main loop.
func (o *Orchestrator) handleCondition(ctx context.Context, bead *types.Bead) error {
	if bead.ConditionSpec == nil {
		return fmt.Errorf("condition bead %s missing spec", bead.ID)
	}

	// Mark as in_progress
	bead.Status = types.BeadStatusInProgress
	if err := o.store.Update(ctx, bead); err != nil {
		return fmt.Errorf("updating bead status: %w", err)
	}

	// Run condition in goroutine (may block!)
	condCtx, cancel := context.WithCancel(ctx)
	o.condMu.Lock()
	o.condRoutines[bead.ID] = cancel
	o.condMu.Unlock()

	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		defer func() {
			o.condMu.Lock()
			delete(o.condRoutines, bead.ID)
			o.condMu.Unlock()
		}()

		o.evalCondition(condCtx, bead)
	}()

	return nil
}

// evalCondition runs the condition command and handles the result.
func (o *Orchestrator) evalCondition(ctx context.Context, bead *types.Bead) {
	spec := bead.ConditionSpec

	// Execute condition as shell command
	codeSpec := &types.CodeSpec{
		Code: spec.Condition,
	}

	outputs, err := o.executor.Execute(ctx, codeSpec)

	// Determine which branch to take
	var target *types.ExpansionTarget
	if err != nil {
		o.logger.Info("condition evaluated false",
			"bead", bead.ID,
			"error", err,
		)
		target = spec.OnFalse
	} else {
		exitCode, _ := outputs["exit_code"].(int)
		if exitCode == 0 {
			o.logger.Info("condition evaluated true", "bead", bead.ID)
			target = spec.OnTrue
		} else {
			o.logger.Info("condition evaluated false",
				"bead", bead.ID,
				"exit_code", exitCode,
			)
			target = spec.OnFalse
		}
	}

	// Expand the target template if specified
	if target != nil && target.Template != "" {
		expandSpec := &types.ExpandSpec{
			Template:  target.Template,
			Variables: target.Variables,
		}
		if err := o.expander.Expand(ctx, expandSpec, bead); err != nil {
			o.logger.Error("failed to expand condition branch",
				"bead", bead.ID,
				"template", target.Template,
				"error", err,
			)
			return
		}
	}

	// Close the condition bead
	if err := bead.Close(nil); err != nil {
		o.logger.Error("failed to close condition bead",
			"bead", bead.ID,
			"error", err,
		)
		return
	}
	if err := o.store.Update(ctx, bead); err != nil {
		o.logger.Error("failed to save closed condition bead",
			"bead", bead.ID,
			"error", err,
		)
	}
}

// handleStop kills an agent's tmux session.
func (o *Orchestrator) handleStop(ctx context.Context, bead *types.Bead) error {
	if bead.StopSpec == nil {
		return fmt.Errorf("stop bead %s missing spec", bead.ID)
	}

	if err := o.agents.Stop(ctx, bead.StopSpec); err != nil {
		return fmt.Errorf("stopping agent %s: %w", bead.StopSpec.Agent, err)
	}

	// Auto-close the bead
	if err := bead.Close(nil); err != nil {
		return fmt.Errorf("closing stop bead: %w", err)
	}
	return o.store.Update(ctx, bead)
}

// handleStart spawns an agent in a tmux session.
func (o *Orchestrator) handleStart(ctx context.Context, bead *types.Bead) error {
	if bead.StartSpec == nil {
		return fmt.Errorf("start bead %s missing spec", bead.ID)
	}

	if err := o.agents.Start(ctx, bead.StartSpec); err != nil {
		return fmt.Errorf("starting agent %s: %w", bead.StartSpec.Agent, err)
	}

	// Auto-close the bead
	if err := bead.Close(nil); err != nil {
		return fmt.Errorf("closing start bead: %w", err)
	}
	return o.store.Update(ctx, bead)
}

// handleCode executes shell code and captures outputs.
func (o *Orchestrator) handleCode(ctx context.Context, bead *types.Bead) error {
	if bead.CodeSpec == nil {
		return fmt.Errorf("code bead %s missing spec", bead.ID)
	}

	outputs, err := o.executor.Execute(ctx, bead.CodeSpec)
	if err != nil {
		// Check error handling strategy
		switch bead.CodeSpec.OnError {
		case types.OnErrorAbort:
			return fmt.Errorf("code execution failed (abort): %w", err)
		case types.OnErrorRetry:
			// TODO: Implement retry logic
			o.logger.Warn("code execution failed, retry not implemented",
				"bead", bead.ID,
				"error", err,
			)
		default: // OnErrorContinue
			o.logger.Warn("code execution failed, continuing",
				"bead", bead.ID,
				"error", err,
			)
		}
	}

	// Auto-close the bead with captured outputs
	if err := bead.Close(outputs); err != nil {
		return fmt.Errorf("closing code bead: %w", err)
	}
	return o.store.Update(ctx, bead)
}

// handleExpand expands a template into beads.
func (o *Orchestrator) handleExpand(ctx context.Context, bead *types.Bead) error {
	if bead.ExpandSpec == nil {
		return fmt.Errorf("expand bead %s missing spec", bead.ID)
	}

	if err := o.expander.Expand(ctx, bead.ExpandSpec, bead); err != nil {
		return fmt.Errorf("expanding template %s: %w", bead.ExpandSpec.Template, err)
	}

	// Auto-close the bead
	if err := bead.Close(nil); err != nil {
		return fmt.Errorf("closing expand bead: %w", err)
	}
	return o.store.Update(ctx, bead)
}

// waitForConditions waits for all condition goroutines to complete.
func (o *Orchestrator) waitForConditions() {
	o.condMu.Lock()
	for _, cancel := range o.condRoutines {
		cancel()
	}
	o.condMu.Unlock()

	o.wg.Wait()
}
