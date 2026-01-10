package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"sync"
	"time"

	"github.com/meow-stack/meow-machine/internal/config"
	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/meow-stack/meow-machine/internal/types"
)

var (
	// ErrAllDone signals that all workflows have completed.
	ErrAllDone = errors.New("all workflows complete")

	// ErrNotImplemented signals that an executor is not yet implemented.
	ErrNotImplemented = errors.New("executor not implemented")
)

// AgentManager manages agent lifecycle (tmux sessions).
type AgentManager interface {
	// Start spawns an agent in a tmux session.
	Start(ctx context.Context, wf *types.Workflow, step *types.Step) error

	// Stop kills an agent's tmux session.
	Stop(ctx context.Context, wf *types.Workflow, step *types.Step) error

	// IsRunning checks if an agent is currently running.
	IsRunning(ctx context.Context, agentID string) (bool, error)

	// InjectPrompt sends ESC + prompt to agent's tmux session.
	InjectPrompt(ctx context.Context, agentID string, prompt string) error
}

// ShellRunner executes shell commands.
type ShellRunner interface {
	// Run executes a shell command and captures outputs.
	Run(ctx context.Context, cfg *types.ShellConfig) (map[string]any, error)
}

// TemplateExpander expands templates into steps.
type TemplateExpander interface {
	// Expand loads a template and inserts steps into the workflow.
	Expand(ctx context.Context, wf *types.Workflow, step *types.Step) error
}

// Orchestrator is the main workflow engine.
// It processes workflows by dispatching ready steps to appropriate executors.
// The Orchestrator is the single owner of workflow state mutations - all state
// changes go through its mutex-protected methods to prevent race conditions.
type Orchestrator struct {
	cfg      *config.Config
	store    WorkflowStore
	agents   AgentManager
	shell    ShellRunner
	expander TemplateExpander
	logger   *slog.Logger

	// Active workflow ID (for single-workflow mode)
	workflowID string

	// Mutex to protect workflow state during concurrent operations.
	// All state-mutating operations (HandleStepDone, HandleApproval, processWorkflow)
	// must acquire this mutex before reading/writing workflow state.
	wfMu sync.Mutex

	// Shutdown coordination
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new Orchestrator.
func New(cfg *config.Config, store WorkflowStore, agents AgentManager, shell ShellRunner, expander TemplateExpander, logger *slog.Logger) *Orchestrator {
	return &Orchestrator{
		cfg:      cfg,
		store:    store,
		agents:   agents,
		shell:    shell,
		expander: expander,
		logger:   logger,
	}
}

// SetWorkflowID sets the active workflow ID for single-workflow mode.
func (o *Orchestrator) SetWorkflowID(id string) {
	o.workflowID = id
}

// Run starts the orchestrator main loop.
// It blocks until the context is cancelled or all work is done.
// IPC messages are handled by IPCHandler which delegates to Orchestrator methods
// (HandleStepDone, HandleApproval) for thread-safe state mutations.
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
			o.wg.Wait()
			return ctx.Err()

		case <-ticker.C:
			if err := o.tick(ctx); err != nil {
				if errors.Is(err, ErrAllDone) {
					o.logger.Info("all work complete")
					o.wg.Wait()
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
	o.wg.Wait()
}

// tick performs one iteration of the main loop.
func (o *Orchestrator) tick(ctx context.Context) error {
	// Get running workflows
	workflows, err := o.store.List(ctx, WorkflowFilter{Status: types.WorkflowStatusRunning})
	if err != nil {
		return fmt.Errorf("listing workflows: %w", err)
	}

	// If single-workflow mode, filter to just that workflow
	if o.workflowID != "" {
		filtered := make([]*types.Workflow, 0, 1)
		for _, wf := range workflows {
			if wf.ID == o.workflowID {
				filtered = append(filtered, wf)
				break
			}
		}
		workflows = filtered

		// If workflow not in running list, check its actual status
		if len(workflows) == 0 {
			wf, err := o.store.Get(ctx, o.workflowID)
			if err != nil {
				return fmt.Errorf("getting workflow %s: %w", o.workflowID, err)
			}
			// Workflow completed (done or failed)
			if wf.Status.IsTerminal() {
				return ErrAllDone
			}
			// Workflow is pending - wait for it to start
			return nil
		}
	}

	if len(workflows) == 0 {
		return ErrAllDone
	}

	allComplete := true
	for _, wf := range workflows {
		if err := o.processWorkflow(ctx, wf); err != nil {
			return err
		}
		if wf.Status == types.WorkflowStatusRunning {
			allComplete = false
		}
	}

	if allComplete {
		return ErrAllDone
	}
	return nil
}

// processWorkflow processes a single workflow, dispatching all ready steps.
func (o *Orchestrator) processWorkflow(ctx context.Context, wf *types.Workflow) error {
	// Lock to coordinate with async handlers (handleStepDone, handleKill goroutines)
	o.wfMu.Lock()
	defer o.wfMu.Unlock()

	// Re-read workflow to get latest state (async operations may have updated it)
	freshWf, err := o.store.Get(ctx, wf.ID)
	if err != nil {
		return fmt.Errorf("re-reading workflow: %w", err)
	}
	wf = freshWf

	// Check timeouts for running agent steps
	o.checkStepTimeouts(ctx, wf)

	readySteps := wf.GetReadySteps()
	if len(readySteps) == 0 {
		if wf.AllDone() {
			// Check for failures first - HasFailed() implies AllDone() but with failures
			if wf.HasFailed() {
				wf.Fail()
				o.logger.Info("workflow failed", "id", wf.ID)
			} else {
				wf.Complete()
				o.logger.Info("workflow completed", "id", wf.ID)
			}
			return o.store.Save(ctx, wf)
		}
		return nil // Waiting for external completion
	}

	// Sort by priority: orchestrator executors first, then by step ID
	sort.Slice(readySteps, func(i, j int) bool {
		if readySteps[i].Executor.IsOrchestrator() != readySteps[j].Executor.IsOrchestrator() {
			return readySteps[i].Executor.IsOrchestrator()
		}
		return readySteps[i].ID < readySteps[j].ID
	})

	// Track which steps we dispatched for merging later
	dispatchedSteps := make(map[string]*types.Step)

	// Process ALL ready steps (enables parallel agent execution)
	for _, step := range readySteps {
		// For agent steps, only dispatch if agent is idle
		if step.Executor == types.ExecutorAgent {
			if step.Agent == nil {
				o.logger.Error("agent step missing config", "step", step.ID)
				continue
			}
			if !wf.AgentIsIdle(step.Agent.Agent) {
				continue
			}
		}

		if err := o.dispatch(ctx, wf, step); err != nil {
			o.logger.Error("dispatch error", "step", step.ID, "error", err)
			// For orchestrator executors, mark step as failed
			if step.Executor.IsOrchestrator() {
				step.Fail(&types.StepError{Message: err.Error()})
			}
		}
		dispatchedSteps[step.ID] = step
	}

	// If we dispatched any steps, we need to save. But first re-read the workflow
	// to avoid overwriting concurrent IPC handler updates (race condition fix).
	if len(dispatchedSteps) > 0 {
		freshWf, err := o.store.Get(ctx, wf.ID)
		if err != nil {
			return fmt.Errorf("re-reading workflow before save: %w", err)
		}

		// Merge step states between our copy and fresh copy.
		// Strategy: use the more "advanced" state for each step.
		// - Our copy has synchronous step completions (shell/spawn/kill/expand)
		// - Fresh copy may have IPC handler completions (agent steps)
		for stepID, ourStep := range wf.Steps {
			freshStep, ok := freshWf.GetStep(stepID)
			if !ok {
				// New step (from expand) - add to fresh
				freshWf.Steps[stepID] = ourStep
				continue
			}

			// Use the more advanced state
			ourRank := stepStatusRank(ourStep.Status)
			freshRank := stepStatusRank(freshStep.Status)

			if ourRank > freshRank {
				// Our step is more advanced - copy our state to fresh
				freshStep.Status = ourStep.Status
				freshStep.StartedAt = ourStep.StartedAt
				freshStep.DoneAt = ourStep.DoneAt
				freshStep.Outputs = ourStep.Outputs
				freshStep.Error = ourStep.Error
				freshStep.ExpandedInto = ourStep.ExpandedInto
			}
			// If fresh is more advanced (IPC handler completed it), keep fresh state
		}

		return o.store.Save(ctx, freshWf)
	}

	return nil
}

// stepStatusRank returns a numeric rank for step status (higher = more advanced).
func stepStatusRank(status types.StepStatus) int {
	switch status {
	case types.StepStatusPending:
		return 0
	case types.StepStatusRunning:
		return 1
	case types.StepStatusCompleting:
		return 2
	case types.StepStatusDone:
		return 3
	case types.StepStatusFailed:
		return 3 // Same as done - both terminal
	default:
		return 0
	}
}

// dispatch routes a step to the appropriate executor handler.
// IMPORTANT: Exactly 6 executors. Gate is NOT an executor.
func (o *Orchestrator) dispatch(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	o.logger.Info("dispatching step", "id", step.ID, "executor", step.Executor)

	// Resolve any deferred step output references before executing
	o.resolveStepOutputRefs(wf, step)

	switch step.Executor {
	case types.ExecutorShell:
		return o.handleShell(ctx, wf, step)
	case types.ExecutorSpawn:
		return o.handleSpawn(ctx, wf, step)
	case types.ExecutorKill:
		return o.handleKill(ctx, wf, step)
	case types.ExecutorExpand:
		return o.handleExpand(ctx, wf, step)
	case types.ExecutorBranch:
		return o.handleBranch(ctx, wf, step)
	case types.ExecutorAgent:
		return o.handleAgent(ctx, wf, step)
	default:
		return fmt.Errorf("unknown executor: %s", step.Executor)
	}
}

// stepOutputRefPattern matches {{step-id.outputs.field}} references
var stepOutputRefPattern = regexp.MustCompile(`\{\{([a-zA-Z0-9_-]+)\.outputs\.([a-zA-Z0-9_]+)\}\}`)

// resolveStepOutputRefs substitutes {{step.outputs.field}} references with actual values
// from completed steps in the workflow.
func (o *Orchestrator) resolveStepOutputRefs(wf *types.Workflow, step *types.Step) {
	// Build a resolver function
	resolve := func(s string) string {
		return stepOutputRefPattern.ReplaceAllStringFunc(s, func(match string) string {
			// Extract step ID and field name
			parts := stepOutputRefPattern.FindStringSubmatch(match)
			if len(parts) != 3 {
				return match // Keep original if parse fails
			}
			stepID := parts[1]
			fieldName := parts[2]

			// Look up the step
			depStep, ok := wf.Steps[stepID]
			if !ok {
				o.logger.Warn("step output ref: step not found", "ref", match, "stepID", stepID)
				return match
			}

			// Get the output value
			if depStep.Outputs == nil {
				o.logger.Warn("step output ref: step has no outputs", "ref", match, "stepID", stepID)
				return match
			}

			val, ok := depStep.Outputs[fieldName]
			if !ok {
				o.logger.Warn("step output ref: field not found", "ref", match, "stepID", stepID, "field", fieldName)
				return match
			}

			// Convert to string
			switch v := val.(type) {
			case string:
				return v
			case fmt.Stringer:
				return v.String()
			default:
				return fmt.Sprintf("%v", v)
			}
		})
	}

	// Resolve references in executor-specific configs
	switch step.Executor {
	case types.ExecutorShell:
		if step.Shell != nil {
			step.Shell.Command = resolve(step.Shell.Command)
			step.Shell.Workdir = resolve(step.Shell.Workdir)
			for k, v := range step.Shell.Env {
				step.Shell.Env[k] = resolve(v)
			}
		}
	case types.ExecutorSpawn:
		if step.Spawn != nil {
			step.Spawn.Agent = resolve(step.Spawn.Agent)
			step.Spawn.Workdir = resolve(step.Spawn.Workdir)
			step.Spawn.ResumeSession = resolve(step.Spawn.ResumeSession)
			for k, v := range step.Spawn.Env {
				step.Spawn.Env[k] = resolve(v)
			}
		}
	case types.ExecutorKill:
		if step.Kill != nil {
			step.Kill.Agent = resolve(step.Kill.Agent)
		}
	case types.ExecutorAgent:
		if step.Agent != nil {
			step.Agent.Agent = resolve(step.Agent.Agent)
			step.Agent.Prompt = resolve(step.Agent.Prompt)
		}
	}
}

// handleIPC processes an IPC message from an agent.
// Note: This method is currently unused since the IPC server routes messages
// directly through IPCHandler. It's kept for potential future use with channel-based IPC.
func (o *Orchestrator) handleIPC(ctx context.Context, msg ipc.Message) error {
	switch m := msg.(type) {
	case *ipc.StepDoneMessage:
		return o.HandleStepDone(ctx, m)
	case *ipc.GetPromptMessage:
		return o.handleGetPrompt(ctx, m)
	case *ipc.ApprovalMessage:
		return o.HandleApproval(ctx, m)
	default:
		o.logger.Warn("unknown IPC message type", "type", fmt.Sprintf("%T", msg))
		return nil
	}
}

// HandleStepDone processes a meow done message from an agent.
// Thread-safe: acquires wfMu before any state changes.
// Called by IPCHandler - this is the ONLY code path for step completion.
func (o *Orchestrator) HandleStepDone(ctx context.Context, msg *ipc.StepDoneMessage) error {
	o.wfMu.Lock()
	defer o.wfMu.Unlock()

	wf, err := o.store.Get(ctx, msg.Workflow)
	if err != nil {
		return fmt.Errorf("getting workflow %s: %w", msg.Workflow, err)
	}

	var step *types.Step
	var ok bool

	// If step ID is provided, use it; otherwise find the running step for this agent
	if msg.Step != "" {
		step, ok = wf.GetStep(msg.Step)
		if !ok {
			return fmt.Errorf("step %s not found in workflow %s", msg.Step, msg.Workflow)
		}
	} else {
		// Find the running agent step for this agent
		step = wf.GetRunningStepForAgent(msg.Agent)
		if step == nil {
			return fmt.Errorf("no running step found for agent: %s", msg.Agent)
		}
	}

	// Validate step is in running state
	if step.Status != types.StepStatusRunning {
		return fmt.Errorf("step %s is not running (status: %s)", step.ID, step.Status)
	}

	// Validate agent matches
	if step.Agent != nil && step.Agent.Agent != msg.Agent {
		return fmt.Errorf("step %s is not assigned to agent %s", step.ID, msg.Agent)
	}

	// Transition to completing to prevent race with stop hook
	if err := step.SetCompleting(); err != nil {
		return fmt.Errorf("setting step completing: %w", err)
	}

	// Validate outputs if defined
	if step.Agent != nil && len(step.Agent.Outputs) > 0 {
		agentWorkdir := ""
		if o.agents != nil {
			if mgr, ok := o.agents.(*TmuxAgentManager); ok {
				agentWorkdir = mgr.GetWorkdir(msg.Agent)
			}
		}
		errs := ValidateAgentOutputs(msg.Outputs, step.Agent.Outputs, agentWorkdir)
		if len(errs) > 0 {
			// Validation failed - keep step running so agent can retry
			step.Status = types.StepStatusRunning
			o.logger.Warn("output validation failed", "step", step.ID, "errors", errs)
			if saveErr := o.store.Save(ctx, wf); saveErr != nil {
				o.logger.Error("failed to save workflow after validation failure", "error", saveErr)
			}
			return fmt.Errorf("output validation failed: %v", errs)
		}
	}

	// Mark step complete
	if err := step.Complete(msg.Outputs); err != nil {
		return fmt.Errorf("completing step: %w", err)
	}

	o.logger.Info("step completed", "step", step.ID, "workflow", wf.ID)
	return o.store.Save(ctx, wf)
}

// handleGetPrompt processes a meow prime request (stop hook).
func (o *Orchestrator) handleGetPrompt(ctx context.Context, msg *ipc.GetPromptMessage) error {
	// Find workflows with work for this agent
	workflows, err := o.store.GetByAgent(ctx, msg.Agent)
	if err != nil {
		return fmt.Errorf("getting workflows for agent %s: %w", msg.Agent, err)
	}

	for _, wf := range workflows {
		// Check for running step (completing state)
		step := wf.GetRunningStepForAgent(msg.Agent)
		if step != nil && step.Status == types.StepStatusCompleting {
			// Step is transitioning - return empty (stay idle)
			return nil
		}

		// Check for next ready step
		nextStep := wf.GetNextReadyStepForAgent(msg.Agent)
		if nextStep != nil {
			// There's work - orchestrator will inject prompt on next tick
			return nil
		}
	}

	return nil
}

// HandleApproval processes a gate approval/rejection.
// Thread-safe: acquires wfMu before any state changes.
// Called by IPCHandler - this is the ONLY code path for approval handling.
func (o *Orchestrator) HandleApproval(ctx context.Context, msg *ipc.ApprovalMessage) error {
	o.wfMu.Lock()
	defer o.wfMu.Unlock()

	wf, err := o.store.Get(ctx, msg.Workflow)
	if err != nil {
		return fmt.Errorf("getting workflow %s: %w", msg.Workflow, err)
	}

	step, ok := wf.GetStep(msg.GateID)
	if !ok {
		return fmt.Errorf("gate step %s not found", msg.GateID)
	}

	if step.Status != types.StepStatusRunning {
		return fmt.Errorf("gate step %s is not running", msg.GateID)
	}

	// Gate approval is handled by branch executor via await-approval
	// This message sets an output that the condition can check
	if step.Outputs == nil {
		step.Outputs = make(map[string]any)
	}
	step.Outputs["approved"] = msg.Approved
	step.Outputs["notes"] = msg.Notes

	return o.store.Save(ctx, wf)
}

// validateOutputs checks that outputs match the expected schema.
func (o *Orchestrator) validateOutputs(step *types.Step, outputs map[string]any) error {
	if step.Agent == nil {
		return nil
	}

	for name, def := range step.Agent.Outputs {
		val, ok := outputs[name]
		if !ok {
			if def.Required {
				return fmt.Errorf("missing required output: %s", name)
			}
			continue
		}

		// Type validation
		switch def.Type {
		case "string":
			if _, ok := val.(string); !ok {
				return fmt.Errorf("output %s: expected string, got %T", name, val)
			}
		case "number":
			switch val.(type) {
			case int, int64, float64:
				// OK
			default:
				return fmt.Errorf("output %s: expected number, got %T", name, val)
			}
		case "boolean":
			if _, ok := val.(bool); !ok {
				return fmt.Errorf("output %s: expected boolean, got %T", name, val)
			}
		case "file_path":
			// File path validation would check against agent's workdir
			// Stub for now - executor track will implement
		}
	}

	return nil
}

// checkStepTimeouts checks for timed-out agent steps.
func (o *Orchestrator) checkStepTimeouts(ctx context.Context, wf *types.Workflow) {
	for _, step := range wf.Steps {
		if step.Status != types.StepStatusRunning {
			continue
		}
		if step.Executor != types.ExecutorAgent {
			continue
		}
		if step.Agent == nil || step.Agent.Timeout == "" {
			continue
		}

		timeout, err := time.ParseDuration(step.Agent.Timeout)
		if err != nil {
			continue
		}

		if step.StartedAt == nil {
			continue
		}

		if time.Since(*step.StartedAt) > timeout {
			o.logger.Warn("step timed out", "step", step.ID)
			// Send C-c to agent, mark as failed after grace period
			// Stub - executor track will implement timeout handling
		}
	}
}

// --- Executor Handlers (Stubs) ---
// These will be implemented by the executor track (pivot-402 through pivot-407)

// handleShell executes a shell command.
func (o *Orchestrator) handleShell(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Shell == nil {
		return fmt.Errorf("shell step %s missing config", step.ID)
	}

	if err := step.Start(); err != nil {
		return fmt.Errorf("starting step: %w", err)
	}

	if o.shell == nil {
		return fmt.Errorf("shell executor not implemented: %w", ErrNotImplemented)
	}

	outputs, err := o.shell.Run(ctx, step.Shell)
	if err != nil {
		if step.Shell.OnError == "continue" {
			o.logger.Warn("shell command failed, continuing", "step", step.ID, "error", err)
			if completeErr := step.Complete(outputs); completeErr != nil {
				return fmt.Errorf("completing step after error: %w", completeErr)
			}
			return nil
		}
		return fmt.Errorf("shell command failed: %w", err)
	}

	if err := step.Complete(outputs); err != nil {
		return fmt.Errorf("completing step: %w", err)
	}
	return nil
}

// handleSpawn starts an agent in a tmux session.
func (o *Orchestrator) handleSpawn(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Spawn == nil {
		return fmt.Errorf("spawn step %s missing config", step.ID)
	}

	if err := step.Start(); err != nil {
		return fmt.Errorf("starting step: %w", err)
	}

	if o.agents == nil {
		return fmt.Errorf("spawn executor not implemented: %w", ErrNotImplemented)
	}

	if err := o.agents.Start(ctx, wf, step); err != nil {
		return fmt.Errorf("starting agent: %w", err)
	}

	// Spawn completes when agent is running
	if err := step.Complete(nil); err != nil {
		return fmt.Errorf("completing step: %w", err)
	}
	return nil
}

// handleKill stops an agent's tmux session.
// Runs asynchronously to avoid blocking parallel step dispatch.
func (o *Orchestrator) handleKill(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Kill == nil {
		return fmt.Errorf("kill step %s missing config", step.ID)
	}

	if err := step.Start(); err != nil {
		return fmt.Errorf("starting step: %w", err)
	}

	if o.agents == nil {
		return fmt.Errorf("kill executor not implemented: %w", ErrNotImplemented)
	}

	// Capture IDs for the goroutine (don't capture pointers that may be stale)
	workflowID := wf.ID
	stepID := step.ID

	// Run kill asynchronously to allow parallel kills
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()

		// The actual stop operation doesn't need the lock
		stopErr := o.agents.Stop(ctx, wf, step)

		// Lock when modifying workflow state
		o.wfMu.Lock()
		defer o.wfMu.Unlock()

		// Re-fetch workflow to get latest state (avoid overwriting other changes)
		freshWf, err := o.store.Get(ctx, workflowID)
		if err != nil {
			o.logger.Error("re-fetching workflow after kill", "error", err)
			return
		}
		if freshWf == nil {
			o.logger.Error("workflow not found after kill", "workflow", workflowID)
			return
		}

		// Find the step in the fresh workflow
		freshStep, ok := freshWf.GetStep(stepID)
		if !ok {
			o.logger.Error("step not found after kill", "step", stepID)
			return
		}

		if stopErr != nil {
			o.logger.Error("kill step failed", "step", stepID, "error", stopErr)
			freshStep.Fail(&types.StepError{Message: stopErr.Error()})
		} else {
			if err := freshStep.Complete(nil); err != nil {
				o.logger.Error("completing kill step", "step", stepID, "error", err)
			}
		}

		// Save workflow state after step completes
		if err := o.store.Save(ctx, freshWf); err != nil {
			o.logger.Error("saving workflow after kill", "error", err)
		}
	}()

	// Step is now running, returns immediately
	return nil
}

// handleExpand inlines another workflow template.
func (o *Orchestrator) handleExpand(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Expand == nil {
		return fmt.Errorf("expand step %s missing config", step.ID)
	}

	if err := step.Start(); err != nil {
		return fmt.Errorf("starting step: %w", err)
	}

	if o.expander == nil {
		return fmt.Errorf("expand executor not implemented: %w", ErrNotImplemented)
	}

	if err := o.expander.Expand(ctx, wf, step); err != nil {
		return fmt.Errorf("expanding template: %w", err)
	}

	if err := step.Complete(nil); err != nil {
		return fmt.Errorf("completing step: %w", err)
	}
	return nil
}

// handleBranch evaluates a condition and expands the appropriate branch.
func (o *Orchestrator) handleBranch(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Branch == nil {
		return fmt.Errorf("branch step %s missing config", step.ID)
	}

	if err := step.Start(); err != nil {
		return fmt.Errorf("starting step: %w", err)
	}

	// Branch executor evaluates condition and expands appropriate target
	// This is a stub - will be implemented in pivot-406
	return fmt.Errorf("branch executor not implemented: %w", ErrNotImplemented)
}

// handleAgent injects a prompt into an agent.
func (o *Orchestrator) handleAgent(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Agent == nil {
		return fmt.Errorf("agent step %s missing config", step.ID)
	}

	if err := step.Start(); err != nil {
		return fmt.Errorf("starting step: %w", err)
	}

	if o.agents == nil {
		return fmt.Errorf("agent executor not implemented: %w", ErrNotImplemented)
	}

	// Build prompt for agent using the helper from executor_agent.go
	result, stepErr := StartAgentStep(step)
	if stepErr != nil {
		return fmt.Errorf("building agent prompt: %s", stepErr.Message)
	}

	// Inject prompt to agent's tmux session
	if err := o.agents.InjectPrompt(ctx, step.Agent.Agent, result.Prompt); err != nil {
		return fmt.Errorf("injecting prompt: %w", err)
	}

	// Fire-and-forget mode: complete immediately after injection
	if IsFireForget(step.Agent) {
		if err := step.Complete(nil); err != nil {
			return fmt.Errorf("completing fire-forget step: %w", err)
		}
		o.logger.Info("fire-forget step completed", "step", step.ID)
		return nil
	}

	// Agent step stays running until agent calls meow done
	return nil
}
