package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
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
	// parentBead may be nil for root-level expansions (e.g., initial workflow template).
	// When parentBead is nil, the created beads have no parent reference.
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

	// State persistence
	persister *StatePersister
	state     *OrchestratorState

	// Shutdown coordination
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Condition goroutines
	condMu       sync.Mutex
	condRoutines map[string]context.CancelFunc
}

// StartupConfig holds configuration for orchestrator startup.
type StartupConfig struct {
	// Template to execute (for fresh start)
	Template string

	// WorkflowID for identification
	WorkflowID string

	// StateDir is the directory for state files
	StateDir string
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

// StartOrResume initializes the orchestrator, either resuming from a crash or starting fresh.
// It acquires the exclusive lock, recovers from any previous crash, and prepares to run.
func (o *Orchestrator) StartOrResume(ctx context.Context, startCfg *StartupConfig) error {
	// Initialize persister
	o.persister = NewStatePersister(startCfg.StateDir)

	// Acquire exclusive lock
	if err := o.persister.AcquireLock(); err != nil {
		return fmt.Errorf("acquiring lock: %w", err)
	}

	// Load bead store
	if loader, ok := o.store.(interface{ Load(context.Context) error }); ok {
		if err := loader.Load(ctx); err != nil {
			o.persister.ReleaseLock()
			return fmt.Errorf("loading bead store: %w", err)
		}
	}

	// Check for existing state (resume vs fresh start)
	existingState, err := o.persister.LoadState()
	if err != nil {
		o.persister.ReleaseLock()
		return fmt.Errorf("loading state: %w", err)
	}

	if existingState != nil {
		// Resume from existing state
		o.logger.Info("resuming from existing state",
			"workflow_id", existingState.WorkflowID,
			"tick_count", existingState.TickCount,
			"previous_pid", existingState.PID,
		)
		o.state = existingState

		// Perform crash recovery
		if err := o.recoverFromCrash(ctx, existingState); err != nil {
			o.persister.ReleaseLock()
			return fmt.Errorf("crash recovery: %w", err)
		}
	} else {
		// Fresh start
		o.logger.Info("starting fresh workflow",
			"workflow_id", startCfg.WorkflowID,
			"template", startCfg.Template,
		)

		o.state = &OrchestratorState{
			Version:      "1",
			WorkflowID:   startCfg.WorkflowID,
			TemplateName: startCfg.Template,
			StartedAt:    time.Now(),
			PID:          os.Getpid(),
		}

		// If a template is specified, expand it to create initial beads
		if startCfg.Template != "" {
			if err := o.expandInitialTemplate(ctx, startCfg); err != nil {
				o.persister.ReleaseLock()
				return fmt.Errorf("expanding initial template: %w", err)
			}
		}
	}

	// Update state with current PID
	o.state.PID = os.Getpid()

	// Save initial state
	if err := o.persister.SaveState(o.state); err != nil {
		o.persister.ReleaseLock()
		return fmt.Errorf("saving state: %w", err)
	}

	return nil
}

// recoverFromCrash handles recovery after a crash by:
// 1. Checking which agents are still running via tmux
// 2. Resetting in-progress beads from dead agents back to open
func (o *Orchestrator) recoverFromCrash(ctx context.Context, prevState *OrchestratorState) error {
	o.logger.Info("performing crash recovery")

	// Get all in-progress beads
	inProgressBeads, err := o.getInProgressBeads(ctx)
	if err != nil {
		return fmt.Errorf("getting in-progress beads: %w", err)
	}

	if len(inProgressBeads) == 0 {
		o.logger.Info("no in-progress beads to recover")
		return nil
	}

	o.logger.Info("found in-progress beads", "count", len(inProgressBeads))

	// Check each bead's assignee
	for _, bead := range inProgressBeads {
		if bead.Assignee == "" {
			// No assignee - this is either an orchestrator bead (condition, code, expand)
			// or an unassigned task. Either way, reset it since:
			// - Orchestrator beads: the orchestrator crashed mid-execution
			// - Unassigned tasks: no agent is working on it
			o.logger.Info("resetting unassigned in-progress bead",
				"id", bead.ID,
				"type", bead.Type,
			)
			bead.Status = types.BeadStatusOpen
			if err := o.store.Update(ctx, bead); err != nil {
				return fmt.Errorf("resetting bead %s: %w", bead.ID, err)
			}
			continue
		}

		// Bead has an assignee - check if the assigned agent is still running
		running, err := o.agents.IsRunning(ctx, bead.Assignee)
		if err != nil {
			o.logger.Warn("error checking agent status",
				"agent", bead.Assignee,
				"bead", bead.ID,
				"error", err,
			)
			continue
		}

		if !running {
			// Agent is dead - reset the bead so it can be picked up again
			o.logger.Info("resetting bead from dead agent",
				"id", bead.ID,
				"agent", bead.Assignee,
			)
			bead.Status = types.BeadStatusOpen
			if err := o.store.Update(ctx, bead); err != nil {
				return fmt.Errorf("resetting bead %s: %w", bead.ID, err)
			}
		} else {
			o.logger.Info("agent still running, keeping bead in progress",
				"id", bead.ID,
				"agent", bead.Assignee,
			)
		}
	}

	return nil
}

// getInProgressBeads returns all beads with status in_progress.
func (o *Orchestrator) getInProgressBeads(ctx context.Context) ([]*types.Bead, error) {
	// Check if store supports listing
	if lister, ok := o.store.(interface {
		List(context.Context, types.BeadStatus) ([]*types.Bead, error)
	}); ok {
		return lister.List(ctx, types.BeadStatusInProgress)
	}

	// Fallback: we can't list beads, return empty
	o.logger.Warn("bead store does not support listing, skipping recovery scan")
	return nil, nil
}

// expandInitialTemplate expands the initial workflow template.
func (o *Orchestrator) expandInitialTemplate(ctx context.Context, startCfg *StartupConfig) error {
	expandSpec := &types.ExpandSpec{
		Template: startCfg.Template,
	}

	// Pass nil as parent - this is the root of the workflow, there's no parent bead.
	// The expander should handle nil parent gracefully (top-level beads have no parent).
	if err := o.expander.Expand(ctx, expandSpec, nil); err != nil {
		return err
	}

	return nil
}

// ReleaseLock releases the orchestrator lock.
// Call this when done with the orchestrator (typically deferred after StartOrResume).
func (o *Orchestrator) ReleaseLock() error {
	if o.persister != nil {
		return o.persister.ReleaseLock()
	}
	return nil
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
					// Clean up ephemeral beads
					if cleanupErr := o.cleanupEphemeralBeads(ctx); cleanupErr != nil {
						o.logger.Warn("failed to cleanup ephemeral beads", "error", cleanupErr)
					}
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

	// Parse timeout if specified
	var timeout time.Duration
	if spec.Timeout != "" {
		var err error
		timeout, err = time.ParseDuration(spec.Timeout)
		if err != nil {
			o.logger.Warn("invalid condition timeout, ignoring",
				"bead", bead.ID,
				"timeout", spec.Timeout,
				"error", err,
			)
		}
	}

	// Create context with timeout if specified
	execCtx := ctx
	var cancel context.CancelFunc
	if timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Execute condition as shell command
	codeSpec := &types.CodeSpec{
		Code: spec.Condition,
	}

	outputs, err := o.executor.Execute(execCtx, codeSpec)

	// Determine which branch to take
	var target *types.ExpansionTarget
	isTimeout := execCtx.Err() == context.DeadlineExceeded

	if isTimeout {
		o.logger.Info("condition timed out",
			"bead", bead.ID,
			"timeout", timeout,
		)
		if spec.OnTimeout != nil {
			target = spec.OnTimeout
		} else {
			// Default to on_false if no on_timeout specified
			target = spec.OnFalse
		}
	} else if err != nil {
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

	// Mark as in_progress
	bead.Status = types.BeadStatusInProgress
	if err := o.store.Update(ctx, bead); err != nil {
		return fmt.Errorf("updating bead status: %w", err)
	}

	maxRetries := bead.CodeSpec.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3 // Default retries
	}

	var outputs map[string]any
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		outputs, lastErr = o.executor.Execute(ctx, bead.CodeSpec)
		if lastErr == nil {
			// Success
			break
		}

		// Check error handling strategy
		switch bead.CodeSpec.OnError {
		case types.OnErrorAbort:
			return fmt.Errorf("code execution failed (abort): %w", lastErr)

		case types.OnErrorRetry:
			if attempt < maxRetries {
				o.logger.Warn("code execution failed, retrying",
					"bead", bead.ID,
					"attempt", attempt,
					"max_retries", maxRetries,
					"error", lastErr,
				)
				// Exponential backoff: 100ms, 200ms, 400ms, ...
				backoff := time.Duration(100<<(attempt-1)) * time.Millisecond
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(backoff):
				}
				continue
			}
			o.logger.Error("code execution failed after retries",
				"bead", bead.ID,
				"attempts", maxRetries,
				"error", lastErr,
			)

		default: // OnErrorContinue
			o.logger.Warn("code execution failed, continuing",
				"bead", bead.ID,
				"error", lastErr,
			)
			break // Don't retry on continue
		}
		break
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

// cleanupEphemeralBeads removes all beads labeled as ephemeral.
// This is called after workflow completion to clean up machinery beads
// that were created during execution.
func (o *Orchestrator) cleanupEphemeralBeads(ctx context.Context) error {
	// Check if store supports deletion
	deleter, ok := o.store.(interface {
		Delete(context.Context, string) error
	})
	if !ok {
		o.logger.Debug("bead store does not support deletion, skipping ephemeral cleanup")
		return nil
	}

	// Check if store supports listing all beads
	lister, ok := o.store.(interface {
		List(context.Context, types.BeadStatus) ([]*types.Bead, error)
	})
	if !ok {
		o.logger.Debug("bead store does not support listing, skipping ephemeral cleanup")
		return nil
	}

	// Get all beads (empty status = all)
	beads, err := lister.List(ctx, "")
	if err != nil {
		return fmt.Errorf("listing beads: %w", err)
	}

	var deleted int
	for _, bead := range beads {
		if bead.IsEphemeral() {
			if err := deleter.Delete(ctx, bead.ID); err != nil {
				o.logger.Warn("failed to delete ephemeral bead",
					"bead", bead.ID,
					"error", err,
				)
				continue
			}
			deleted++
		}
	}

	if deleted > 0 {
		o.logger.Info("cleaned up ephemeral beads", "count", deleted)
	}

	return nil
}
