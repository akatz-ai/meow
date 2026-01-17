package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/meow-stack/meow-machine/internal/config"
	"github.com/meow-stack/meow-machine/internal/ipc"
	"github.com/meow-stack/meow-machine/internal/workflow"
	"github.com/meow-stack/meow-machine/internal/types"
)

var (
	// ErrAllDone signals that all workflows have completed.
	ErrAllDone = errors.New("all workflows complete")

	// ErrNotImplemented signals that an executor is not yet implemented.
	ErrNotImplemented = errors.New("executor not implemented")
)

// stringifyValue converts any value to a string representation.
// For maps and slices, it JSON-marshals them instead of using Go's %v format.
// This prevents outputs like "map[foo:bar]" and produces valid JSON like {"foo":"bar"}.
func stringifyValue(val any) string {
	if val == nil {
		return ""
	}

	switch v := val.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	}

	// Use reflection to detect maps and slices of any type
	rv := reflect.ValueOf(val)
	kind := rv.Kind()

	if kind == reflect.Map || kind == reflect.Slice || kind == reflect.Array {
		// JSON-marshal structured types
		if b, err := json.Marshal(val); err == nil {
			return string(b)
		}
		// Fallback to %v if JSON marshaling fails
		return fmt.Sprintf("%v", val)
	}

	// Use %v for scalars (int, bool, float, etc.)
	return fmt.Sprintf("%v", val)
}

// AgentManager manages agent lifecycle (tmux sessions).
type AgentManager interface {
	// Start spawns an agent in a tmux session.
	Start(ctx context.Context, wf *types.Run, step *types.Step) error

	// Stop kills an agent's tmux session.
	Stop(ctx context.Context, wf *types.Run, step *types.Step) error

	// IsRunning checks if an agent is currently running.
	IsRunning(ctx context.Context, agentID string) (bool, error)

	// InjectPrompt sends ESC + prompt to agent's tmux session.
	InjectPrompt(ctx context.Context, agentID string, prompt string) error

	// Interrupt sends C-c to an agent's tmux session for graceful cancellation.
	Interrupt(ctx context.Context, agentID string) error

	// KillAll kills all agent sessions for a workflow.
	KillAll(ctx context.Context, wf *types.Run) error
}

// ShellRunner executes shell commands.
type ShellRunner interface {
	// Run executes a shell command and captures outputs.
	Run(ctx context.Context, cfg *types.ShellConfig) (map[string]any, error)
}

// TemplateExpander expands templates into steps.
type TemplateExpander interface {
	// Expand loads a template and inserts steps into the workflow.
	Expand(ctx context.Context, wf *types.Run, step *types.Step) error
}

// Orchestrator is the main workflow engine.
// It processes workflows by dispatching ready steps to appropriate executors.
// The Orchestrator is the single owner of workflow state mutations - all state
// changes go through its mutex-protected methods to prevent race conditions.
type Orchestrator struct {
	cfg      *config.Config
	store    RunStore
	agents   AgentManager
	shell    ShellRunner
	expander TemplateExpander
	logger   *slog.Logger

	// Active workflow ID (for single-workflow mode)
	workflowID string

	// Mutex to protect workflow state during concurrent operations.
	// All state-mutating operations (HandleStepDone, processWorkflow)
	// must acquire this mutex before reading/writing workflow state.
	wfMu sync.Mutex

	// Track pending async command executions (branch and shell-as-sugar)
	// Key: stepID (string)
	// Value: context.CancelFunc
	pendingCommands sync.Map

	// Shutdown coordination
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New creates a new Orchestrator.
func New(cfg *config.Config, store RunStore, agents AgentManager, shell ShellRunner, expander TemplateExpander, logger *slog.Logger) *Orchestrator {
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
// (HandleStepDone) for thread-safe state mutations.
// Handles SIGINT/SIGTERM for graceful shutdown with cleanup.
func (o *Orchestrator) Run(ctx context.Context) error {
	ctx, o.cancel = context.WithCancel(ctx)
	defer o.cancel()

	o.logger.Info("orchestrator starting")

	// Set up signal handling
	sigChan := o.setupSignalHandler()
	defer signal.Stop(sigChan)

	ticker := time.NewTicker(o.cfg.Orchestrator.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case sig := <-sigChan:
			o.logger.Info("received signal, initiating cleanup", "signal", sig)

			// Cancel in-flight commands (branch and shell)
			o.cancelPendingCommands()

			// Wait for goroutines (should exit quickly after cancellation)
			o.wg.Wait()

			// Run cleanup for the active workflow
			if o.workflowID != "" {
				if err := o.cleanupOnSignal(ctx); err != nil {
					o.logger.Error("cleanup failed", "error", err)
				}
			}
			return nil

		case <-ctx.Done():
			o.logger.Info("orchestrator shutting down", "reason", ctx.Err())
			o.wg.Wait()
			return ctx.Err()

		case <-ticker.C:
			if err := o.tick(ctx); err != nil {
				if errors.Is(err, ErrAllDone) {
					o.logger.Info("all work complete")
					o.wg.Wait()
					// Cleanup already handled by processWorkflow
					return nil
				}
				o.logger.Error("tick error", "error", err)
				// Continue running on non-fatal errors
			}
		}
	}
}

// cleanupOnSignal handles SIGINT/SIGTERM by optionally running cleanup_on_stop.
// If no cleanup_on_stop is defined, just marks workflow as stopped (preserving agents/state).
func (o *Orchestrator) cleanupOnSignal(ctx context.Context) error {
	if o.workflowID == "" {
		return nil
	}

	wf, err := o.store.Get(ctx, o.workflowID)
	if err != nil {
		return fmt.Errorf("getting workflow: %w", err)
	}
	if wf == nil || wf.Status.IsTerminal() {
		return nil
	}

	// Check if cleanup_on_stop is defined (opt-in cleanup)
	if wf.HasCleanup(types.RunStatusStopped) {
		// Use a background context for cleanup since the original may be cancelled
		cleanupCtx := context.Background()
		return o.RunCleanup(cleanupCtx, wf, types.RunStatusStopped)
	}

	// No cleanup_on_stop defined - just mark as stopped, preserve agents/state
	o.logger.Info("workflow stopped (no cleanup_on_stop defined, preserving agents/state)", "id", wf.ID)
	wf.Stop()
	return o.store.Save(ctx, wf)
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
	workflows, err := o.store.List(ctx, RunFilter{Status: types.RunStatusRunning})
	if err != nil {
		return fmt.Errorf("listing workflows: %w", err)
	}

	// If single-workflow mode, filter to just that workflow
	if o.workflowID != "" {
		filtered := make([]*types.Run, 0, 1)
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
		if wf.Status == types.RunStatusRunning {
			allComplete = false
		}
	}

	if allComplete {
		return ErrAllDone
	}
	return nil
}

// processWorkflow processes a single workflow, dispatching all ready steps.
func (o *Orchestrator) processWorkflow(ctx context.Context, wf *types.Run) error {
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
	timeoutModified := o.checkStepTimeouts(ctx, wf)

	// Check for pending steps that are blocked by failed dependencies
	blockedModified := o.checkBlockedSteps(wf)

	// Check for foreach steps with implicit join that are ready to complete
	o.checkForeachCompletion(wf)

	// Check for branch steps waiting for their expanded children to complete
	branchModified := o.checkBranchCompletion(wf)

	readySteps := wf.GetReadySteps()
	if len(readySteps) == 0 {
		if wf.AllDone() {
			// Determine final status
			finalStatus := types.RunStatusDone
			if wf.HasFailed() {
				finalStatus = types.RunStatusFailed
			}

			// Check if cleanup is defined for this trigger (opt-in cleanup)
			if wf.HasCleanup(finalStatus) {
				o.logger.Info("workflow complete, running cleanup", "id", wf.ID, "reason", finalStatus)
				// Unlock before RunCleanup since it may do I/O
				o.wfMu.Unlock()
				err := o.RunCleanup(ctx, wf, finalStatus)
				o.wfMu.Lock()
				if err != nil {
					o.logger.Error("cleanup failed", "error", err)
				}
				return nil
			}

			// No cleanup defined for this trigger - set terminal status directly
			// State/agents are preserved (opt-in cleanup design)
			if finalStatus == types.RunStatusFailed {
				wf.Fail()
				o.logger.Info("workflow failed (no cleanup defined)", "id", wf.ID)
			} else {
				wf.Complete()
				o.logger.Info("workflow completed (no cleanup defined)", "id", wf.ID)
			}
			return o.store.Save(ctx, wf)
		}
		// Save if timeout handling or blocked step detection modified state
		if timeoutModified || blockedModified || branchModified {
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
		// For agent steps, check idleness unless it's fire_forget mode
		if step.Executor == types.ExecutorAgent {
			if step.Agent == nil {
				o.logger.Error("agent step missing config", "step", step.ID)
				continue
			}
			// fire_forget mode injects prompts without waiting for agent to be idle
			// This is used for nudge prompts in the Ralph Wiggum persistence pattern
			if step.Agent.Mode != "fire_forget" && !wf.AgentIsIdle(step.Agent.Agent) {
				continue
			}
		}

		// Check if step is throttled by parent foreach's max_concurrent
		if o.isForeachThrottled(wf, step) {
			continue
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

			// Always preserve InterruptedAt if set (for timeout tracking)
			if ourStep.InterruptedAt != nil && freshStep.InterruptedAt == nil {
				freshStep.InterruptedAt = ourStep.InterruptedAt
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
func (o *Orchestrator) dispatch(ctx context.Context, wf *types.Run, step *types.Step) error {
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
	case types.ExecutorForeach:
		return o.handleForeach(ctx, wf, step)
	case types.ExecutorAgent:
		return o.handleAgent(ctx, wf, step)
	default:
		return fmt.Errorf("unknown executor: %s", step.Executor)
	}
}

// stepOutputRefPattern matches {{step-id.outputs.field}} references
// Step IDs can contain dots (e.g., "parent.child" from expansion prefixes), so we match
// everything before ".outputs." as the step ID.
// Field names can also contain dots for nested access (e.g., "config.nested").
var stepOutputRefPattern = regexp.MustCompile(`\{\{([a-zA-Z0-9_.-]+)\.outputs\.([a-zA-Z0-9_.]+)\}\}`)

// findStepWithScopeWalk looks up a step by ID, using scope-walk resolution if exact match fails.
// When templates are expanded inside foreach loops, step IDs get prefixed (e.g., "agents.0.shell-step").
// A reference to "shell-step" inside "agents.0.expand-step" should find "agents.0.shell-step".
//
// The algorithm walks up the prefix chain of currentStepID until a match is found:
//   1. Try exact: stepID
//   2. Try with prefix: prefix + "." + stepID (for each level of prefix)
//
// Example: currentStepID="agents.0.track", refStepID="resolve-protocol"
//   - Try: "resolve-protocol" (not found)
//   - Try: "agents.0.resolve-protocol" (found!)
func findStepWithScopeWalk(wf *types.Run, refStepID, currentStepID string) (*types.Step, string, bool) {
	// Try exact match first
	if step, ok := wf.Steps[refStepID]; ok {
		return step, refStepID, true
	}

	// Walk up the prefix chain
	prefix := currentStepID
	for {
		idx := strings.LastIndex(prefix, ".")
		if idx < 0 {
			break // No more prefixes to try
		}
		prefix = prefix[:idx]
		prefixedID := prefix + "." + refStepID
		if step, ok := wf.Steps[prefixedID]; ok {
			return step, prefixedID, true
		}
	}

	return nil, refStepID, false
}

// getNestedOutputValue retrieves a potentially nested value from step outputs.
// Field can be simple ("result") or nested ("config.database.host").
func getNestedOutputValue(outputs map[string]any, field string) (any, bool) {
	// Simple case: no dots in field name
	if !strings.Contains(field, ".") {
		val, ok := outputs[field]
		return val, ok
	}

	// Nested case: walk down the path
	parts := strings.Split(field, ".")
	var val any = outputs

	for _, part := range parts {
		switch v := val.(type) {
		case map[string]any:
			var ok bool
			val, ok = v[part]
			if !ok {
				return nil, false
			}
		default:
			return nil, false
		}
	}

	return val, true
}

// resolveStepOutputRefs substitutes {{step.outputs.field}} references with actual values
// from completed steps in the workflow. Uses scope-walk resolution to find steps within
// foreach-expanded contexts.
func (o *Orchestrator) resolveStepOutputRefs(wf *types.Run, step *types.Step) {
	// Build a resolver function that captures the current step for scope-walk
	resolve := func(s string) string {
		return stepOutputRefPattern.ReplaceAllStringFunc(s, func(match string) string {
			// Extract step ID and field name
			parts := stepOutputRefPattern.FindStringSubmatch(match)
			if len(parts) != 3 {
				return match // Keep original if parse fails
			}
			refStepID := parts[1]
			fieldName := parts[2]

			// Look up the step with scope-walk
			depStep, resolvedID, ok := findStepWithScopeWalk(wf, refStepID, step.ID)
			if !ok {
				o.logger.Warn("step output ref: step not found", "ref", match, "stepID", refStepID, "currentStep", step.ID)
				return match
			}

			// Get the output value
			if depStep.Outputs == nil {
				o.logger.Warn("step output ref: step has no outputs", "ref", match, "stepID", resolvedID)
				return match
			}

			val, ok := getNestedOutputValue(depStep.Outputs, fieldName)
			if !ok {
				o.logger.Warn("step output ref: field not found", "ref", match, "stepID", resolvedID, "field", fieldName)
				return match
			}

			// Convert to string
			return stringifyValue(val)
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
			step.Spawn.Adapter = resolve(step.Spawn.Adapter)
			step.Spawn.Workdir = resolve(step.Spawn.Workdir)
			step.Spawn.ResumeSession = resolve(step.Spawn.ResumeSession)
			step.Spawn.SpawnArgs = resolve(step.Spawn.SpawnArgs)
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
	case types.ExecutorForeach:
		if step.Foreach != nil {
			step.Foreach.Items = resolve(step.Foreach.Items)
			step.Foreach.ItemsFile = resolve(step.Foreach.ItemsFile)
		}
	case types.ExecutorExpand:
		if step.Expand != nil {
			step.Expand.Template = resolve(step.Expand.Template)
			for k, v := range step.Expand.Variables {
				if s, ok := v.(string); ok {
					step.Expand.Variables[k] = resolve(s)
				}
				// Non-string values are preserved as-is
			}
		}
	case types.ExecutorBranch:
		if step.Branch != nil {
			// Note: condition is resolved separately in handleBranch via resolveOutputRefs
			// Here we resolve branch target variables
			if step.Branch.OnTrue != nil {
				for k, v := range step.Branch.OnTrue.Variables {
					if s, ok := v.(string); ok {
						step.Branch.OnTrue.Variables[k] = resolve(s)
					}
				}
			}
			if step.Branch.OnFalse != nil {
				for k, v := range step.Branch.OnFalse.Variables {
					if s, ok := v.(string); ok {
						step.Branch.OnFalse.Variables[k] = resolve(s)
					}
				}
			}
			if step.Branch.OnTimeout != nil {
				for k, v := range step.Branch.OnTimeout.Variables {
					if s, ok := v.(string); ok {
						step.Branch.OnTimeout.Variables[k] = resolve(s)
					}
				}
			}
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

// TimeoutGracePeriod is the duration to wait after sending C-c before marking a step as failed.
const TimeoutGracePeriod = 10 * time.Second

// CleanupTimeout is the maximum duration for cleanup script execution.
const CleanupTimeout = 60 * time.Second

// checkStepTimeouts checks for timed-out agent steps and handles timeout enforcement.
// Per MVP-SPEC-v2: send C-c, wait 10 seconds, then mark as failed.
// Returns true if any step state was modified (requires save).
func (o *Orchestrator) checkStepTimeouts(ctx context.Context, wf *types.Run) bool {
	modified := false
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
			o.logger.Warn("invalid timeout duration", "step", step.ID, "timeout", step.Agent.Timeout, "error", err)
			continue
		}

		if step.StartedAt == nil {
			continue
		}

		elapsed := time.Since(*step.StartedAt)

		// If already interrupted, check if grace period has passed
		if step.InterruptedAt != nil {
			gracePeriodElapsed := time.Since(*step.InterruptedAt)
			if gracePeriodElapsed >= TimeoutGracePeriod {
				// Grace period expired - mark step as failed
				o.logger.Warn("step timeout grace period expired",
					"step", step.ID,
					"timeout", timeout,
					"elapsed", elapsed,
					"gracePeriod", gracePeriodElapsed)

				if err := step.Fail(&types.StepError{
					Message: fmt.Sprintf("Step timed out after %s", elapsed.Round(time.Second)),
				}); err != nil {
					o.logger.Error("failed to mark timed-out step as failed",
						"step", step.ID,
						"error", err)
				}
				modified = true
			}
			continue
		}

		// Check if step has exceeded timeout
		if elapsed > timeout {
			o.logger.Warn("step timed out, sending interrupt",
				"step", step.ID,
				"timeout", timeout,
				"elapsed", elapsed)

			// Send C-c to agent
			if o.agents != nil {
				if err := o.agents.Interrupt(ctx, step.Agent.Agent); err != nil {
					o.logger.Error("failed to send interrupt to agent",
						"step", step.ID,
						"agent", step.Agent.Agent,
						"error", err)
					// Continue anyway - mark as interrupted to start grace period
				}
			}

			// Record when we sent the interrupt
			now := time.Now()
			step.InterruptedAt = &now
			modified = true
		}
	}
	return modified
}

// checkBlockedSteps marks pending steps as skipped if they have failed dependencies.
// A step is blocked if any of its dependencies has failed (and that dependency doesn't have on_error=continue).
// Returns true if any step was modified.
func (o *Orchestrator) checkBlockedSteps(wf *types.Run) bool {
	modified := false
	for _, step := range wf.Steps {
		if step.Status != types.StepStatusPending {
			continue
		}

		// Check if any dependency failed
		for _, depID := range step.Needs {
			dep, ok := wf.Steps[depID]
			if !ok {
				continue
			}
			if dep.Status == types.StepStatusFailed {
				// Dependency failed - this step should be skipped
				reason := fmt.Sprintf("dependency %q failed", depID)
				o.logger.Info("skipping step due to failed dependency",
					"step", step.ID,
					"dependency", depID)
				if err := step.Skip(reason); err != nil {
					o.logger.Error("failed to skip step",
						"step", step.ID,
						"error", err)
				}
				modified = true
				break // No need to check other dependencies
			}
			if dep.Status == types.StepStatusSkipped {
				// Dependency was skipped - cascade the skip
				reason := fmt.Sprintf("dependency %q was skipped", depID)
				o.logger.Info("skipping step due to skipped dependency",
					"step", step.ID,
					"dependency", depID)
				if err := step.Skip(reason); err != nil {
					o.logger.Error("failed to skip step",
						"step", step.ID,
						"error", err)
				}
				modified = true
				break
			}
		}
	}
	return modified
}

// checkForeachCompletion checks for foreach steps with implicit join that are ready to complete.
// When join=true (default) and all child steps are done, the foreach step is marked done.
func (o *Orchestrator) checkForeachCompletion(wf *types.Run) {
	for _, step := range wf.Steps {
		// Only check running foreach steps with join=true
		if step.Status != types.StepStatusRunning {
			continue
		}
		if step.Executor != types.ExecutorForeach {
			continue
		}
		if step.Foreach == nil || !step.Foreach.IsJoin() {
			continue
		}

		// Check if all children are complete
		if IsForeachComplete(step, wf.Steps) {
			// Check if any children failed
			if IsForeachFailed(step, wf.Steps) {
				o.logger.Info("foreach step failed (child failed)",
					"step", step.ID)
				if err := step.Fail(&types.StepError{
					Message: "one or more iterations failed",
				}); err != nil {
					o.logger.Error("failed to fail foreach step",
						"step", step.ID,
						"error", err)
				}
			} else {
				o.logger.Info("foreach step complete (all children done)",
					"step", step.ID,
					"childCount", len(step.ExpandedInto))
				if err := step.Complete(nil); err != nil {
					o.logger.Error("failed to complete foreach step",
						"step", step.ID,
						"error", err)
				}
			}
		}
	}
}

// checkBranchCompletion checks for branch steps that are waiting for their expanded
// children to complete. When all children are done, the branch step is marked done.
// This is analogous to checkForeachCompletion for foreach steps.
// Returns true if any step was modified.
func (o *Orchestrator) checkBranchCompletion(wf *types.Run) bool {
	modified := false
	for _, step := range wf.Steps {
		// Only check running branch steps with expanded children
		if step.Status != types.StepStatusRunning {
			continue
		}
		if step.Executor != types.ExecutorBranch {
			continue
		}
		if len(step.ExpandedInto) == 0 {
			continue
		}

		// Check if all children are complete
		if IsBranchComplete(step, wf.Steps) {
			// Check if any children failed
			if IsBranchFailed(step, wf.Steps) {
				o.logger.Info("branch step failed (child failed)",
					"step", step.ID)
				if err := step.Fail(&types.StepError{
					Message: "branch child step failed",
				}); err != nil {
					o.logger.Error("failed to fail branch step",
						"step", step.ID,
						"error", err)
				}
			} else {
				o.logger.Info("branch step complete (all children done)",
					"step", step.ID,
					"childCount", len(step.ExpandedInto))
				// Use the outcome that was stored during handleBranch
				if err := step.Complete(step.Outputs); err != nil {
					o.logger.Error("failed to complete branch step",
						"step", step.ID,
						"error", err)
				}
			}
			modified = true
		}
	}
	return modified
}

// IsBranchComplete checks if all children of a branch step are done.
func IsBranchComplete(branchStep *types.Step, allSteps map[string]*types.Step) bool {
	if branchStep.ExpandedInto == nil || len(branchStep.ExpandedInto) == 0 {
		return true // No children = complete
	}

	// Check all expanded steps
	for _, childID := range branchStep.ExpandedInto {
		child, ok := allSteps[childID]
		if !ok {
			continue // Step not found - treat as complete (may have been cleaned up)
		}
		if !child.Status.IsTerminal() {
			return false // Still running
		}
	}

	return true
}

// IsBranchFailed checks if any child of a branch step has failed.
func IsBranchFailed(branchStep *types.Step, allSteps map[string]*types.Step) bool {
	if branchStep.ExpandedInto == nil {
		return false
	}

	for _, childID := range branchStep.ExpandedInto {
		child, ok := allSteps[childID]
		if !ok {
			continue
		}
		if child.Status == types.StepStatusFailed {
			return true
		}
	}

	return false
}

// isForeachThrottled checks if a step should be throttled due to its parent
// foreach's max_concurrent limit.
//
// Returns true if:
// 1. The step is expanded from a foreach step with max_concurrent > 0
// 2. The number of currently running iterations >= max_concurrent
//
// This enables fair scheduling of foreach iterations without spawning all at once.
func (o *Orchestrator) isForeachThrottled(wf *types.Run, step *types.Step) bool {
	// Step must have been expanded from a parent step
	if step.ExpandedFrom == "" {
		return false
	}

	// Find the root foreach parent (may be nested)
	parentID := step.ExpandedFrom
	for {
		parent, ok := wf.Steps[parentID]
		if !ok {
			return false
		}

		// Found a foreach parent?
		if parent.Executor == types.ExecutorForeach && parent.Foreach != nil {
			maxConcurrent := parent.Foreach.GetMaxConcurrent()
			if maxConcurrent <= 0 {
				// No limit
				return false
			}

			// Count running iterations
			runningCount := CountRunningIterations(parent, wf.Steps)
			if runningCount >= maxConcurrent {
				o.logger.Debug("step throttled by foreach max_concurrent",
					"step", step.ID,
					"foreachStep", parent.ID,
					"maxConcurrent", maxConcurrent,
					"runningCount", runningCount)
				return true
			}
			return false
		}

		// Keep looking up the chain for a foreach ancestor
		if parent.ExpandedFrom == "" {
			return false
		}
		parentID = parent.ExpandedFrom
	}
}

// --- Cleanup Methods ---

// RunCleanup executes the cleanup sequence for a workflow.
// Cleanup is opt-in: only runs if a cleanup script is defined for the given reason.
// Per MVP-SPEC-v2:
// 1. Set status to cleaning_up
// 2. Persist state
// 3. Kill all agent tmux sessions
// 4. Execute cleanup script (60 second timeout)
// 5. Set final status
// 6. Persist final state
func (o *Orchestrator) RunCleanup(ctx context.Context, wf *types.Run, reason types.RunStatus) error {
	cleanupScript := wf.GetCleanupScript(reason)
	o.logger.Info("starting workflow cleanup",
		"workflow", wf.ID,
		"reason", reason,
		"hasCleanupScript", cleanupScript != "")

	// 1. Set status to cleaning_up
	if err := wf.StartCleanup(reason); err != nil {
		return fmt.Errorf("starting cleanup: %w", err)
	}

	// 2. Persist state (so cleanup survives crash)
	if err := o.store.Save(ctx, wf); err != nil {
		o.logger.Error("failed to save workflow cleanup state", "error", err)
		// Continue with cleanup anyway
	}

	// 3. Kill all agent tmux sessions
	if o.agents != nil {
		if err := o.agents.KillAll(ctx, wf); err != nil {
			o.logger.Error("failed to kill agents during cleanup", "error", err)
			// Continue with cleanup anyway
		}
	}

	// 4. Execute cleanup script (if defined for this trigger)
	if cleanupScript != "" {
		if err := o.runCleanupScript(ctx, wf, cleanupScript); err != nil {
			o.logger.Error("cleanup script failed", "error", err)
			// Cleanup script errors are logged but don't prevent workflow termination
		}
	}

	// 5. Set final status
	wf.FinishCleanup()

	// 6. Persist final state
	if err := o.store.Save(ctx, wf); err != nil {
		return fmt.Errorf("saving final workflow state: %w", err)
	}

	o.logger.Info("workflow cleanup complete",
		"workflow", wf.ID,
		"finalStatus", wf.Status)

	return nil
}

// runCleanupScript executes a cleanup script with timeout.
func (o *Orchestrator) runCleanupScript(ctx context.Context, wf *types.Run, script string) error {
	if script == "" {
		return nil
	}

	// Create a timeout context for cleanup
	cleanupCtx, cancel := context.WithTimeout(ctx, CleanupTimeout)
	defer cancel()

	o.logger.Info("running cleanup script", "workflow", wf.ID)

	// Execute the cleanup script via bash
	cmd := exec.CommandContext(cleanupCtx, "bash", "-c", script)

	// Set environment variables from workflow
	cmd.Env = os.Environ()
	for k, v := range wf.Variables {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, stringifyValue(v)))
	}
	// Add workflow ID as an environment variable
	cmd.Env = append(cmd.Env, fmt.Sprintf("MEOW_WORKFLOW=%s", wf.ID))

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cleanupCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("cleanup script timed out after %s", CleanupTimeout)
		}
		return fmt.Errorf("cleanup script failed: %w (stderr: %s)", err, stderr.String())
	}

	o.logger.Info("cleanup script completed successfully", "workflow", wf.ID)
	return nil
}

// setupSignalHandler sets up SIGINT/SIGTERM handling.
// Returns a channel that receives true when a signal is caught.
func (o *Orchestrator) setupSignalHandler() chan os.Signal {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	return sigChan
}

// --- Crash Recovery ---

// Recover handles crash recovery for workflows on orchestrator startup.
// Per MVP-SPEC-v2:
// 1. Load all running workflows
// 2. Handle workflows in cleaning_up state (resume cleanup)
// 3. For running/completing steps:
//   - Orchestrator executors: reset to pending
//   - Expand steps: delete partial child steps, reset to pending
//   - Agent steps with dead agent: reset to pending
//   - Agent steps with live agent: keep running (wait for stop hook)
func (o *Orchestrator) Recover(ctx context.Context) error {
	o.logger.Info("starting crash recovery")

	// Load all running and cleaning_up workflows
	runningWorkflows, err := o.store.List(ctx, RunFilter{Status: types.RunStatusRunning})
	if err != nil {
		return fmt.Errorf("listing running workflows: %w", err)
	}

	cleaningUpWorkflows, err := o.store.List(ctx, RunFilter{Status: types.RunStatusCleaningUp})
	if err != nil {
		return fmt.Errorf("listing cleaning_up workflows: %w", err)
	}

	// Handle workflows that were in cleaning_up state
	for _, wf := range cleaningUpWorkflows {
		o.logger.Info("resuming cleanup for workflow", "workflow", wf.ID, "priorStatus", wf.PriorStatus)
		// Resume cleanup - this will re-run the cleanup script and set final status
		if err := o.resumeCleanup(ctx, wf); err != nil {
			o.logger.Error("failed to resume cleanup", "workflow", wf.ID, "error", err)
		}
	}

	// Handle running workflows
	for _, wf := range runningWorkflows {
		modified := false

		// First pass: identify partial expansions (steps that expand children and were running)
		partialExpands := make(map[string]bool)
		for _, step := range wf.Steps {
			if (step.Executor == types.ExecutorExpand ||
				step.Executor == types.ExecutorBranch ||
				step.Executor == types.ExecutorForeach) &&
				(step.Status == types.StepStatusRunning || step.Status == types.StepStatusCompleting) {
				partialExpands[step.ID] = true
			}
		}

		// Delete partially-expanded child steps
		for stepID, step := range wf.Steps {
			if step.ExpandedFrom != "" && partialExpands[step.ExpandedFrom] {
				o.logger.Info("deleting partial expansion child",
					"step", stepID,
					"parent", step.ExpandedFrom,
					"workflow", wf.ID)
				delete(wf.Steps, stepID)
				modified = true
			}
		}

		// Second pass: handle running/completing steps
		for _, step := range wf.Steps {
			if step.Status != types.StepStatusRunning && step.Status != types.StepStatusCompleting {
				continue
			}

			// Treat "completing" as "running" for recovery purposes
			// (orchestrator crashed during transition)

			// Handle branch and shell steps specially
			if step.Executor == types.ExecutorBranch || step.Executor == types.ExecutorShell {
				if len(step.ExpandedInto) == 0 {
					// Case 1: Condition was in-flight (not yet expanded)
					// Reset to pending - condition will re-run
					o.logger.Info("resetting branch/shell step (condition was in-flight)",
						"step", step.ID,
						"executor", step.Executor,
						"workflow", wf.ID)
					step.Status = types.StepStatusPending
					step.StartedAt = nil
					step.InterruptedAt = nil
					step.Outputs = nil
					modified = true
				} else {
					// Case 2: Already expanded, waiting for children
					// Keep running - checkBranchCompletion will handle
					o.logger.Info("keeping branch/shell step running (has expanded children)",
						"step", step.ID,
						"executor", step.Executor,
						"childCount", len(step.ExpandedInto),
						"workflow", wf.ID)
					// Don't reset - children are live
				}
				continue // Skip the generic orchestrator reset below
			}

			if step.Executor.IsOrchestrator() {
				// Orchestrator step was mid-execution - reset to pending
				o.logger.Info("resetting orchestrator step",
					"step", step.ID,
					"executor", step.Executor,
					"wasStatus", step.Status,
					"workflow", wf.ID)
				step.Status = types.StepStatusPending
				step.StartedAt = nil
				step.InterruptedAt = nil
				// Clear ExpandedInto for steps that expand (expand, branch, foreach)
				if step.Executor == types.ExecutorExpand ||
					step.Executor == types.ExecutorBranch ||
					step.Executor == types.ExecutorForeach {
					step.ExpandedInto = nil
				}
				modified = true
			} else if step.Executor == types.ExecutorAgent {
				// Check if agent is still alive
				var agentAlive bool
				if o.agents != nil && step.Agent != nil {
					agentAlive, _ = o.agents.IsRunning(ctx, step.Agent.Agent)
				}

				if !agentAlive {
					// Agent dead - reset to pending (will need respawn)
					o.logger.Info("resetting step from dead agent",
						"step", step.ID,
						"agent", step.Agent.Agent,
						"workflow", wf.ID)
					step.Status = types.StepStatusPending
					step.StartedAt = nil
					step.InterruptedAt = nil
					modified = true
				} else {
					// Agent still alive - keep running
					// Don't immediately re-inject prompt!
					// Wait for agent to call meow done (normal completion)
					// This avoids injecting duplicate prompts
					o.logger.Info("keeping step running with live agent",
						"step", step.ID,
						"agent", step.Agent.Agent,
						"workflow", wf.ID)
					if step.Status == types.StepStatusCompleting {
						step.Status = types.StepStatusRunning
						modified = true
					}
				}
			}
		}

		if modified {
			if err := o.store.Save(ctx, wf); err != nil {
				o.logger.Error("failed to save workflow after recovery",
					"workflow", wf.ID,
					"error", err)
			}
		}
	}

	o.logger.Info("crash recovery complete",
		"runningWorkflows", len(runningWorkflows),
		"cleaningUpWorkflows", len(cleaningUpWorkflows))

	return nil
}

// resumeCleanup continues cleanup for a workflow that was in cleaning_up state when crash occurred.
func (o *Orchestrator) resumeCleanup(ctx context.Context, wf *types.Run) error {
	// Kill any remaining agents (some may have survived the crash)
	if o.agents != nil {
		if err := o.agents.KillAll(ctx, wf); err != nil {
			o.logger.Warn("failed to kill agents during resumed cleanup", "error", err)
		}
	}

	// Get the cleanup script for the prior status (reason cleanup was triggered)
	cleanupScript := wf.GetCleanupScript(wf.PriorStatus)

	// Run cleanup script again (should be idempotent)
	if cleanupScript != "" {
		if err := o.runCleanupScript(ctx, wf, cleanupScript); err != nil {
			o.logger.Warn("cleanup script failed during resume", "error", err)
		}
	}

	// Set final status
	wf.FinishCleanup()

	// Persist final state
	return o.store.Save(ctx, wf)
}

// completeBranchCondition finalizes a branch/shell step after its condition completes.
// Called from goroutine - acquires mutex for thread-safe state mutation.
//
// Parameters:
// - workflowID, stepID: identifiers (captured by value)
// - outcome: true/false/timeout
// - target: branch target to expand (may be nil for shell-as-sugar)
// - result: ShellResult containing stdout, stderr, exit_code
// - cfg: BranchConfig for output capture definitions
func (o *Orchestrator) completeBranchCondition(
	ctx context.Context,
	workflowID string,
	stepID string,
	outcome BranchOutcome,
	target *types.BranchTarget,
	result *ShellResult,
	cfg *types.BranchConfig,
) {
	// Acquire mutex for state mutation
	o.wfMu.Lock()
	defer o.wfMu.Unlock()

	// Re-fetch workflow to get fresh state
	wf, err := o.store.Get(ctx, workflowID)
	if err != nil {
		o.logger.Error("re-fetching workflow after command", "error", err)
		return
	}
	if wf == nil || wf.Status.IsTerminal() {
		return
	}

	step, ok := wf.GetStep(stepID)
	if !ok || step.Status != types.StepStatusRunning {
		return
	}

	// Handle expansion for branch with targets
	if target != nil {
		if err := o.expandBranchTarget(ctx, wf, step, target); err != nil {
			if failErr := step.Fail(&types.StepError{Message: fmt.Sprintf("expansion failed: %v", err)}); failErr != nil {
				o.logger.Error("failed to mark step as failed", "step", stepID, "error", failErr)
			}
			o.store.Save(ctx, wf)
			return
		}
	}

	// Build outputs - capture per cfg.Outputs definitions
	outputs := map[string]any{
		"outcome":   string(outcome),
		"exit_code": result.ExitCode,
	}

	// Capture defined outputs (stdout, stderr, file:path)
	// Create a source substitution function that resolves step output references
	substituteSource := func(source string) (string, error) {
		vc := workflow.NewVarContext()
		// Set up step lookup from the workflow
		vc.SetStepLookup(func(lookupStepID string) (*workflow.StepInfo, error) {
			s, ok := wf.GetStep(lookupStepID)
			if !ok {
				return nil, nil // Not found
			}
			return &workflow.StepInfo{
				ID:      s.ID,
				Status:  string(s.Status),
				Outputs: s.Outputs,
			}, nil
		})
		return vc.Substitute(source)
	}

	if cfg.Outputs != nil {
		for name, source := range cfg.Outputs {
			value, err := captureOutput(source.Source, result, substituteSource)
			if err != nil {
				o.logger.Warn("output capture failed", "name", name, "error", err)
				outputs[name] = nil
			} else {
				outputs[name] = value
			}
		}
	}

	// Handle on_error for shell-as-sugar (no expansion targets)
	// Default is "fail" when on_error is empty
	if target == nil && result.ExitCode != 0 {
		if cfg.OnError != "continue" {
			// Default to fail
			if failErr := step.Fail(&types.StepError{
				Message: "command failed",
				Code:    result.ExitCode,
				Output:  result.Stderr,
			}); failErr != nil {
				o.logger.Error("failed to mark step as failed", "step", stepID, "error", failErr)
			}
			o.store.Save(ctx, wf)
			return
		}
		// on_error: continue - include error info in outputs
		outputs["error"] = result.Stderr
	}

	// Complete or stay running based on children
	if len(step.ExpandedInto) > 0 {
		step.Outputs = outputs
	} else {
		if err := step.Complete(outputs); err != nil {
			o.logger.Error("failed to complete step", "step", stepID, "error", err)
			return
		}
	}

	o.store.Save(ctx, wf)
}

// --- Executor Handlers (Stubs) ---
// These will be implemented by the executor track (pivot-402 through pivot-407)

// handleShell executes a shell command.
// Shell is syntactic sugar over branch - this converts the config and delegates.
func (o *Orchestrator) handleShell(ctx context.Context, wf *types.Run, step *types.Step) error {
	if step.Shell == nil {
		return fmt.Errorf("shell step %s missing config", step.ID)
	}

	// Convert shell config to branch config
	step.Branch = &types.BranchConfig{
		Condition: step.Shell.Command,
		Workdir:   step.Shell.Workdir,
		Env:       step.Shell.Env,
		Outputs:   step.Shell.Outputs,
		OnError:   step.Shell.OnError,
		// No on_true/on_false → just run, capture outputs, complete
	}

	// Clear shell config (now using branch)
	step.Shell = nil

	// Delegate to branch handler (which runs async)
	return o.handleBranch(ctx, wf, step)
}

// handleSpawn starts an agent in a tmux session.
func (o *Orchestrator) handleSpawn(ctx context.Context, wf *types.Run, step *types.Step) error {
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
func (o *Orchestrator) handleKill(ctx context.Context, wf *types.Run, step *types.Step) error {
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
func (o *Orchestrator) handleExpand(ctx context.Context, wf *types.Run, step *types.Step) error {
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

// handleForeach expands a template for each item in a list.
func (o *Orchestrator) handleForeach(ctx context.Context, wf *types.Run, step *types.Step) error {
	if step.Foreach == nil {
		return fmt.Errorf("foreach step %s missing config", step.ID)
	}

	if err := step.Start(); err != nil {
		return fmt.Errorf("starting step: %w", err)
	}

	// Build template loader from the expander
	// Use step's SourceModule if set (for nested foreach), otherwise workflow template
	sourceModule := step.SourceModule
	if sourceModule == "" {
		sourceModule = wf.Template
	}
	var loader TemplateLoader
	if o.expander != nil {
		if adapter, ok := o.expander.(*TemplateExpanderAdapter); ok {
			loader = &fileTemplateLoader{
				expander:     adapter.Expander,
				sourceModule: sourceModule,
				workflowID:   wf.ID,
			}
		}
	}
	if loader == nil {
		return fmt.Errorf("foreach executor requires template loader")
	}

	// Execute the foreach expansion
	result, stepErr := ExecuteForeach(ctx, step, loader, wf.Variables, 0, nil)
	if stepErr != nil {
		return fmt.Errorf("foreach expansion failed: %s", stepErr.Message)
	}

	// Add expanded steps to workflow
	if wf.Steps == nil {
		wf.Steps = make(map[string]*types.Step)
	}
	for _, newStep := range result.ExpandedSteps {
		wf.Steps[newStep.ID] = newStep
	}

	// Track expanded steps on the foreach step
	step.ExpandedInto = result.StepIDs

	// Handle join vs fire-and-forget
	if step.Foreach.IsJoin() {
		// Implicit join: step stays running until all children complete
		// The orchestrator will mark it done when IsForeachComplete returns true
		o.logger.Info("foreach expansion complete, waiting for children",
			"step", step.ID,
			"iterations", len(result.IterationIDs),
			"childSteps", len(result.ExpandedSteps))
		// Step stays in "running" state - main loop will check for completion
	} else {
		// Fire-and-forget: mark done immediately after expansion
		if err := step.Complete(nil); err != nil {
			return fmt.Errorf("completing step: %w", err)
		}
		o.logger.Info("foreach expansion complete (fire-and-forget)",
			"step", step.ID,
			"iterations", len(result.IterationIDs),
			"childSteps", len(result.ExpandedSteps))
	}

	return nil
}

// handleBranch evaluates a condition and expands the appropriate branch.
// Launches condition execution asynchronously and returns immediately.
func (o *Orchestrator) handleBranch(ctx context.Context, wf *types.Run, step *types.Step) error {
	if step.Branch == nil {
		return fmt.Errorf("branch step %s missing config", step.ID)
	}

	if err := step.Start(); err != nil {
		return fmt.Errorf("starting step: %w", err)
	}

	cfg := step.Branch
	condition := o.resolveOutputRefs(wf, cfg.Condition, step.ID)

	// Capture IDs by value for goroutine (NOT pointers!)
	workflowID := wf.ID
	stepID := step.ID

	// Create cancellable context for the condition
	var condCtx context.Context
	var cancel context.CancelFunc

	if cfg.Timeout != "" {
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return fmt.Errorf("invalid timeout %q: %v", cfg.Timeout, err)
		}
		condCtx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		condCtx, cancel = context.WithCancel(ctx)
	}

	// Track for cleanup (shared with shell-as-sugar)
	o.pendingCommands.Store(stepID, cancel)

	// Launch async condition execution
	o.wg.Add(1)
	go func() {
		defer o.wg.Done()
		o.executeBranchConditionAsync(condCtx, workflowID, stepID, condition, cfg)
	}()

	// Step is running, return immediately
	return nil
}

// executeBranchConditionAsync runs a branch condition and handles completion.
// Called in a goroutine - does NOT hold any mutex during condition execution.
// Acquires mutex only when calling completeBranchCondition.
func (o *Orchestrator) executeBranchConditionAsync(
	ctx context.Context,
	workflowID string,
	stepID string,
	condition string,
	cfg *types.BranchConfig,
) {
	// Clean up tracking regardless of outcome
	defer o.pendingCommands.Delete(stepID)

	// Execute condition command (may block for seconds/minutes/hours)
	// Pass IPC socket path so condition can use meow event/await-event
	// Also pass workflow ID and step ID for MEOW_WORKFLOW and MEOW_STEP vars
	condExec := &SimpleConditionExecutor{
		SocketPath: ipc.SocketPath(workflowID),
		WorkflowID: workflowID,
		StepID:     stepID,
	}
	exitCode, stdout, stderr, execErr := condExec.Execute(ctx, condition)

	// Check for context cancellation (workflow stopped/shutdown)
	if ctx.Err() == context.Canceled {
		o.logger.Info("branch condition cancelled",
			"step", stepID,
			"reason", "context cancelled")
		// Don't complete - workflow is stopping
		return
	}

	// Determine outcome
	var outcome BranchOutcome
	var target *types.BranchTarget

	if execErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			// Condition timed out
			outcome = BranchOutcomeTimeout
			target = cfg.OnTimeout
			if target == nil {
				target = cfg.OnFalse // Fallback per spec
			}
			o.logger.Info("branch condition timed out",
				"step", stepID)
		} else {
			// Execution error (command failed to run, not non-zero exit)
			outcome = BranchOutcomeFalse
			target = cfg.OnFalse
			o.logger.Warn("branch condition execution error",
				"step", stepID,
				"error", execErr)
		}
	} else if exitCode == 0 {
		// Condition evaluated to true
		outcome = BranchOutcomeTrue
		target = cfg.OnTrue
	} else {
		// Condition evaluated to false (non-zero exit)
		outcome = BranchOutcomeFalse
		target = cfg.OnFalse
	}

	o.logger.Info("branch condition completed",
		"step", stepID,
		"outcome", outcome,
		"exitCode", exitCode,
		"hasTarget", target != nil)

	// Complete the branch (acquires mutex, updates state, saves)
	result := &ShellResult{
		ExitCode: exitCode,
		Stdout:   stdout,
		Stderr:   stderr,
	}
	o.completeBranchCondition(ctx, workflowID, stepID, outcome, target, result, cfg)
}

// cancelPendingCommands cancels all in-flight async command executions.
// Called during cleanup to ensure condition goroutines exit promptly.
//
// This allows wg.Wait() to complete quickly instead of waiting for
// potentially long-running commands (meow await-event, etc.).
func (o *Orchestrator) cancelPendingCommands() {
	count := 0
	o.pendingCommands.Range(func(key, value any) bool {
		stepID, ok := key.(string)
		if !ok {
			return true // skip invalid entry
		}
		cancel, ok := value.(context.CancelFunc)
		if !ok {
			return true // skip invalid entry
		}

		o.logger.Info("cancelling pending command", "step", stepID)
		cancel()
		count++
		return true // continue iteration
	})

	if count > 0 {
		o.logger.Info("cancelled pending commands", "count", count)
	}
}

// expandBranchTarget expands a branch target (template or inline steps).
func (o *Orchestrator) expandBranchTarget(ctx context.Context, wf *types.Run, step *types.Step, target *types.BranchTarget) error {
	if target.Template != "" {
		// Create a temporary expand step to reuse the expander
		// Copy SourceModule from branch step so nested local refs resolve correctly
		expandStep := &types.Step{
			ID:           step.ID,
			Executor:     types.ExecutorExpand,
			SourceModule: step.SourceModule,
			Expand: &types.ExpandConfig{
				Template:  target.Template,
				Variables: target.Variables,
			},
		}

		if o.expander == nil {
			return fmt.Errorf("expander not configured")
		}

		if err := o.expander.Expand(ctx, wf, expandStep); err != nil {
			return fmt.Errorf("expanding template %s: %w", target.Template, err)
		}

		// Copy expanded info to original step
		step.ExpandedInto = expandStep.ExpandedInto
		return nil
	}

	if len(target.Inline) > 0 {
		// Expand inline steps
		childIDs := make([]string, 0, len(target.Inline))
		inlineStepIDs := make(map[string]bool)
		for _, is := range target.Inline {
			inlineStepIDs[is.ID] = true
		}

		for _, is := range target.Inline {
			newID := step.ID + "." + is.ID
			childIDs = append(childIDs, newID)

			newStep := &types.Step{
				ID:           newID,
				Executor:     is.Executor,
				Status:       types.StepStatusPending,
				ExpandedFrom: step.ID,
			}

			// Set executor-specific config
			switch is.Executor {
			case types.ExecutorShell:
				newStep.Shell = &types.ShellConfig{
					Command: is.Command,
				}
			case types.ExecutorAgent:
				newStep.Agent = &types.AgentConfig{
					Agent:  is.Agent,
					Prompt: is.Prompt,
				}
			}

			// Prefix dependencies that are internal to this expansion
			newStep.Needs = make([]string, 0, len(is.Needs))
			for _, need := range is.Needs {
				if inlineStepIDs[need] {
					newStep.Needs = append(newStep.Needs, step.ID+"."+need)
				} else {
					newStep.Needs = append(newStep.Needs, need)
				}
			}

			// Add to workflow
			if wf.Steps == nil {
				wf.Steps = make(map[string]*types.Step)
			}
			wf.Steps[newID] = newStep
		}

		step.ExpandedInto = childIDs
	}

	return nil
}

// resolveOutputRefs resolves {{step.outputs.field}} references in a string.
// Uses scope-walk resolution to find steps within foreach-expanded contexts.
func (o *Orchestrator) resolveOutputRefs(wf *types.Run, s string, currentStepID string) string {
	return stepOutputRefPattern.ReplaceAllStringFunc(s, func(match string) string {
		parts := stepOutputRefPattern.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		refStepID := parts[1]
		fieldName := parts[2]

		// Look up the step with scope-walk
		depStep, _, ok := findStepWithScopeWalk(wf, refStepID, currentStepID)
		if !ok {
			return match
		}
		if depStep.Outputs == nil {
			return match
		}
		val, ok := getNestedOutputValue(depStep.Outputs, fieldName)
		if !ok {
			return match
		}

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

// handleAgent injects a prompt into an agent.
func (o *Orchestrator) handleAgent(ctx context.Context, wf *types.Run, step *types.Step) error {
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
