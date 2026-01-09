# Track: Executor Handlers (PHASE 2)

## ⚠️ DEPENDENCY WARNING
This track depends on **pivot-401** (Orchestrator Refactor) being merged first.

**Before starting implementation:**
1. Check if pivot-401 is closed: `bd show pivot-401`
2. If still open, WAIT or coordinate with the orchestrator track
3. Once merged, rebase: `git fetch origin && git rebase origin/main`

## Your Beads (in order)

### Can Be Parallel (after pivot-401):
- **pivot-402**: Implement shell executor handler
- **pivot-403**: Implement spawn executor handler
- **pivot-404**: Implement kill executor handler

### Sequential (each depends on previous):
- **pivot-405**: Implement expand executor handler (depends on pivot-401)
- **pivot-406**: Implement branch executor handler (depends on pivot-405)
- **pivot-407**: Implement agent executor handler (depends on pivot-401)

## Recommended Order
1. Start with shell (pivot-402) - simplest, good pattern to establish
2. Then spawn (pivot-403) and kill (pivot-404) - agent lifecycle
3. Then expand (pivot-405) - template expansion
4. Then branch (pivot-406) - conditional logic
5. Finally agent (pivot-407) - ties to IPC

## What You Own
- `internal/orchestrator/executor_*.go` - New files for each executor
- Tests for each executor

## What You Must NOT Touch
- `internal/types/` - Types are finalized
- `internal/template/` - Phase 1 owns this
- `internal/ipc/` - Already complete (but you'll USE it for agent executor)

## Executor Implementations

### Shell Executor (pivot-402)
```go
func (o *Orchestrator) executeShell(ctx context.Context, step *types.Step) error {
    cfg := step.Shell
    cmd := exec.CommandContext(ctx, "sh", "-c", cfg.Command)
    cmd.Dir = cfg.Workdir
    cmd.Env = mapToEnv(cfg.Env)

    output, err := cmd.CombinedOutput()
    if err != nil {
        if cfg.OnError == "continue" {
            // Log and continue
            step.Complete(map[string]any{"error": err.Error()})
        } else {
            step.Fail(&types.StepError{Message: err.Error(), Output: string(output)})
        }
    } else {
        // Capture outputs per cfg.Outputs
        step.Complete(captureOutputs(cfg.Outputs, output))
    }
    return o.store.Save(ctx, workflow)
}
```

### Spawn Executor (pivot-403)
```go
func (o *Orchestrator) executeSpawn(ctx context.Context, step *types.Step) error {
    cfg := step.Spawn

    // Create tmux session
    session, err := o.agents.Create(cfg.Agent, cfg.Workdir, cfg.Env)
    if err != nil {
        return step.Fail(&types.StepError{Message: err.Error()})
    }

    // Register agent in workflow
    workflow.RegisterAgent(cfg.Agent, &types.AgentInfo{
        TmuxSession: session,
        Status: "active",
        Workdir: cfg.Workdir,
    })

    step.Complete(map[string]any{"session": session})
    return o.store.Save(ctx, workflow)
}
```

### Kill Executor (pivot-404)
```go
func (o *Orchestrator) executeKill(ctx context.Context, step *types.Step) error {
    cfg := step.Kill

    if cfg.Graceful {
        // Send SIGTERM, wait cfg.Timeout seconds
        o.agents.SignalGraceful(cfg.Agent, cfg.Timeout)
    }

    if err := o.agents.Kill(cfg.Agent); err != nil {
        return step.Fail(&types.StepError{Message: err.Error()})
    }

    step.Complete(nil)
    return o.store.Save(ctx, workflow)
}
```

### Expand Executor (pivot-405)
```go
func (o *Orchestrator) executeExpand(ctx context.Context, step *types.Step) error {
    cfg := step.Expand

    // Load and bake the referenced template
    expandedSteps, err := o.expander.Expand(cfg.Template, cfg.Variables)
    if err != nil {
        return step.Fail(&types.StepError{Message: err.Error()})
    }

    // Add expanded steps to workflow with prefixed IDs
    for _, s := range expandedSteps {
        s.ID = step.ID + "." + s.ID
        s.ExpandedFrom = step.ID
        workflow.AddStep(s)
    }

    step.ExpandedInto = /* list of new step IDs */
    step.Complete(nil)
    return o.store.Save(ctx, workflow)
}
```

### Branch Executor (pivot-406)
```go
func (o *Orchestrator) executeBranch(ctx context.Context, step *types.Step) error {
    cfg := step.Branch

    // Run condition command
    cmd := exec.CommandContext(ctx, "sh", "-c", cfg.Condition)
    err := cmd.Run()

    var target *types.BranchTarget
    if err == nil {
        target = cfg.OnTrue
    } else if isTimeout(err) && cfg.OnTimeout != nil {
        target = cfg.OnTimeout
    } else {
        target = cfg.OnFalse
    }

    if target != nil {
        // Expand the target (inline or template)
        expandedSteps := o.expandBranchTarget(target)
        // Add to workflow...
    }

    step.Complete(map[string]any{"branch": branchTaken})
    return o.store.Save(ctx, workflow)
}
```

### Agent Executor (pivot-407)
```go
func (o *Orchestrator) executeAgent(ctx context.Context, step *types.Step) error {
    cfg := step.Agent

    // Agent steps don't complete immediately - they wait for IPC
    // Mark as running and update agent's current step
    step.Start()

    agent := workflow.Agents[cfg.Agent]
    agent.CurrentStep = step.ID
    agent.Status = "active"

    // The step will be completed when agent calls "meow done"
    // which is handled by the IPC server

    return o.store.Save(ctx, workflow)
}
```

## TDD Approach

For each executor:
1. Write tests for success case
2. Write tests for error handling
3. Write tests for edge cases
4. Implement the executor

## When Done (for each)

```bash
bd close pivot-402 --reason "Implemented shell executor"
git add -A && git commit -m "feat(orchestrator): Implement shell executor"

# Repeat for each executor...
```

## Context Files to Read

1. `docs/MVP-SPEC-v2.md` - Executor section
2. `internal/orchestrator/orchestrator.go` - Where executors plug in
3. `internal/types/step.go` - Step and executor configs
4. `internal/agent/` - Agent management interface
5. `bd show pivot-402` through `bd show pivot-407` - Bead descriptions
