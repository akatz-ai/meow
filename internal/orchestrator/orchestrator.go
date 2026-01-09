package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
type Orchestrator struct {
	cfg      *config.Config
	store    WorkflowStore
	agents   AgentManager
	shell    ShellRunner
	expander TemplateExpander
	logger   *slog.Logger

	// IPC channel for receiving messages from agents
	ipcChan chan ipc.Message

	// Active workflow ID (for single-workflow mode)
	workflowID string

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
		ipcChan:  make(chan ipc.Message, 100),
	}
}

// SetWorkflowID sets the active workflow ID for single-workflow mode.
func (o *Orchestrator) SetWorkflowID(id string) {
	o.workflowID = id
}

// IPCChannel returns the channel for receiving IPC messages.
// The IPC server should send messages to this channel.
func (o *Orchestrator) IPCChannel() chan<- ipc.Message {
	return o.ipcChan
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
			o.wg.Wait()
			return ctx.Err()

		case msg := <-o.ipcChan:
			if err := o.handleIPC(ctx, msg); err != nil {
				o.logger.Error("IPC handling error", "error", err)
			}

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
	}

	return o.store.Save(ctx, wf)
}

// dispatch routes a step to the appropriate executor handler.
// IMPORTANT: Exactly 6 executors. Gate is NOT an executor.
func (o *Orchestrator) dispatch(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	o.logger.Info("dispatching step", "id", step.ID, "executor", step.Executor)

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

// handleIPC processes an IPC message from an agent.
func (o *Orchestrator) handleIPC(ctx context.Context, msg ipc.Message) error {
	switch m := msg.(type) {
	case *ipc.StepDoneMessage:
		return o.handleStepDone(ctx, m)
	case *ipc.GetPromptMessage:
		return o.handleGetPrompt(ctx, m)
	case *ipc.ApprovalMessage:
		return o.handleApproval(ctx, m)
	default:
		o.logger.Warn("unknown IPC message type", "type", fmt.Sprintf("%T", msg))
		return nil
	}
}

// handleStepDone processes a meow done message from an agent.
func (o *Orchestrator) handleStepDone(ctx context.Context, msg *ipc.StepDoneMessage) error {
	wf, err := o.store.Get(ctx, msg.Workflow)
	if err != nil {
		return fmt.Errorf("getting workflow %s: %w", msg.Workflow, err)
	}

	step, ok := wf.GetStep(msg.Step)
	if !ok {
		return fmt.Errorf("step %s not found in workflow %s", msg.Step, msg.Workflow)
	}

	// Validate step is in running state
	if step.Status != types.StepStatusRunning {
		return fmt.Errorf("step %s is not running (status: %s)", msg.Step, step.Status)
	}

	// Transition to completing to prevent race with stop hook
	if err := step.SetCompleting(); err != nil {
		return fmt.Errorf("setting step completing: %w", err)
	}

	// Validate outputs if defined
	if step.Agent != nil && len(step.Agent.Outputs) > 0 {
		if err := o.validateOutputs(step, msg.Outputs); err != nil {
			// Validation failed - keep step running so agent can retry
			step.Status = types.StepStatusRunning
			o.logger.Warn("output validation failed", "step", step.ID, "error", err)
			return o.store.Save(ctx, wf)
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

// handleApproval processes a gate approval/rejection.
func (o *Orchestrator) handleApproval(ctx context.Context, msg *ipc.ApprovalMessage) error {
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

	step.Start()

	if o.shell == nil {
		return fmt.Errorf("shell executor not implemented: %w", ErrNotImplemented)
	}

	outputs, err := o.shell.Run(ctx, step.Shell)
	if err != nil {
		if step.Shell.OnError == "continue" {
			o.logger.Warn("shell command failed, continuing", "step", step.ID, "error", err)
			step.Complete(outputs)
			return nil
		}
		return fmt.Errorf("shell command failed: %w", err)
	}

	step.Complete(outputs)
	return nil
}

// handleSpawn starts an agent in a tmux session.
func (o *Orchestrator) handleSpawn(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Spawn == nil {
		return fmt.Errorf("spawn step %s missing config", step.ID)
	}

	step.Start()

	if o.agents == nil {
		return fmt.Errorf("spawn executor not implemented: %w", ErrNotImplemented)
	}

	if err := o.agents.Start(ctx, wf, step); err != nil {
		return fmt.Errorf("starting agent: %w", err)
	}

	// Spawn completes when agent is running
	step.Complete(nil)
	return nil
}

// handleKill stops an agent's tmux session.
func (o *Orchestrator) handleKill(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Kill == nil {
		return fmt.Errorf("kill step %s missing config", step.ID)
	}

	step.Start()

	if o.agents == nil {
		return fmt.Errorf("kill executor not implemented: %w", ErrNotImplemented)
	}

	if err := o.agents.Stop(ctx, wf, step); err != nil {
		return fmt.Errorf("stopping agent: %w", err)
	}

	step.Complete(nil)
	return nil
}

// handleExpand inlines another workflow template.
func (o *Orchestrator) handleExpand(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Expand == nil {
		return fmt.Errorf("expand step %s missing config", step.ID)
	}

	step.Start()

	if o.expander == nil {
		return fmt.Errorf("expand executor not implemented: %w", ErrNotImplemented)
	}

	if err := o.expander.Expand(ctx, wf, step); err != nil {
		return fmt.Errorf("expanding template: %w", err)
	}

	step.Complete(nil)
	return nil
}

// handleBranch evaluates a condition and expands the appropriate branch.
func (o *Orchestrator) handleBranch(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Branch == nil {
		return fmt.Errorf("branch step %s missing config", step.ID)
	}

	step.Start()

	// Branch executor evaluates condition and expands appropriate target
	// This is a stub - will be implemented in pivot-406
	return fmt.Errorf("branch executor not implemented: %w", ErrNotImplemented)
}

// handleAgent injects a prompt into an agent.
func (o *Orchestrator) handleAgent(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Agent == nil {
		return fmt.Errorf("agent step %s missing config", step.ID)
	}

	step.Start()

	if o.agents == nil {
		return fmt.Errorf("agent executor not implemented: %w", ErrNotImplemented)
	}

	// Build prompt for agent
	prompt := step.Agent.Prompt

	// Inject prompt to agent's tmux session
	if err := o.agents.InjectPrompt(ctx, step.Agent.Agent, prompt); err != nil {
		return fmt.Errorf("injecting prompt: %w", err)
	}

	// Agent step stays running until agent calls meow done
	return nil
}
