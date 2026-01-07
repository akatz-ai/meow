# MEOW Stack MVP Refinement

This document specifies the minimal viable implementation of MEOW Stack, focusing on the core value proposition: **durable, nested workflow execution with Go orchestration**.

> **Design Influence**: This architecture borrows heavily from [gastown](../../../gastown)'s proven patterns for agent orchestration, including the hook model, handoff mechanisms, and agent lifecycle management.

## Table of Contents

1. [MVP Scope](#mvp-scope)
2. [Architecture Overview](#architecture-overview)
3. [The Execution Model](#the-execution-model)
4. [The Hook Model](#the-hook-model)
5. [The Orchestrator Model](#the-orchestrator-model)
6. [Session Management](#session-management)
7. [Stop Hook Integration](#stop-hook-integration)
8. [Context Management & Handoff](#context-management--handoff)
9. [CLI Commands](#cli-commands)
10. [Backend Abstraction](#backend-abstraction)
11. [Template System](#template-system)
12. [State Management](#state-management)
13. [Logging & Debugging](#logging--debugging)
14. [Implementation Phases](#implementation-phases)

---

## MVP Scope

### Included

| Feature | Description |
|---------|-------------|
| Go Orchestrator | Binary that manages the execution loop and spawns Claude sessions |
| Beads Backend | Abstracted interface to beads for molecule/step storage |
| Template System | TOML templates baked into molecules |
| Molecule Stack | Nested execution with one Claude session per stack level |
| Hook Model | Molecules are "hooked" to agents, like gastown's hook_bead pattern |
| Stop Hook | Controls iteration, detects completion, triggers handoff |
| Session Resume | Parent sessions resume via `claude --resume` after child completion |
| Context Handoff | Automatic handoff when context window exceeds threshold |
| Bead Assignment | Steps assigned to specific agents via `--assignee` |
| tmux Sessions | Each Claude runs in a named tmux session for observability |
| Human Gates | Blocking steps that pause execution for approval |
| Shell Conditions | Restart/conditional steps using shell command exit codes |
| Debug Logging | Comprehensive logging of stack operations and state transitions |
| **Restart with Fresh Context** | Loop iterations KILL session and spawn fresh for bounded context |
| **times_looped Counter** | Built-in counter available to conditions and templates |
| **Conditional Branching** | Branch steps route to different paths based on conditions |
| **Iteration Digests** | Each loop iteration squashes to a summary before restart |

### Excluded from MVP

| Feature | Reason |
|---------|--------|
| Parallel Agents | Complexity; sequential execution is sufficient for MVP |
| Git Worktrees | Not needed without parallelism |
| beads_viewer (bv) | Claude can decide task importance; templates can specify tools |
| Clone/Fresh Spawn | Single spawn mode (fresh with context injection) |
| Web Dashboard | CLI-only for MVP |
| Notifications | Desktop/Slack notifications deferred |
| Template Inheritance | Simple templates only; extends/override later |

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           MEOW MVP ARCHITECTURE                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                        MEOW CLI (Go Binary)                            │ │
│  │                                                                        │ │
│  │  meow init ──────────▶ Initialize beads + .meow directory              │ │
│  │  meow run <template> ─▶ Bake template, start orchestrator              │ │
│  │  meow prime ─────────▶ Check hook, show assigned work (startup prompt) │ │
│  │  meow done ──────────▶ Signal workflow complete (graceful exit)        │ │
│  │  meow handoff ───────▶ Save context, signal for fresh session          │ │
│  │  meow continue ──────▶ Resume from current stack state                 │ │
│  │  meow status ────────▶ Show stack + current step + session info        │ │
│  │  meow approve ───────▶ Close blocking gate, resume execution           │ │
│  │                                                                        │ │
│  └─────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                         ORCHESTRATOR                                   │ │
│  │                                                                        │ │
│  │  • Manages molecule stack                                              │ │
│  │  • Spawns Claude sessions in tmux with `meow prime` as prompt          │ │
│  │  • Monitors session via stop hook                                      │ │
│  │  • Reads context usage from Claude status line                         │ │
│  │  • Handles push (descend) / pop (ascend) / handoff operations          │ │
│  │  • Resumes parent sessions after child completion                      │ │
│  │                                                                        │ │
│  └─────────────────────────────┬──────────────────────────────────────────┘ │
│                                │                                             │
│              ┌─────────────────┼─────────────────┐                           │
│              │                 │                 │                           │
│              ▼                 ▼                 ▼                           │
│  ┌────────────────┐ ┌────────────────┐ ┌────────────────┐                   │
│  │  TMUX SESSION  │ │  TMUX SESSION  │ │  TMUX SESSION  │                   │
│  │  meow-mol-001  │ │  meow-mol-002  │ │  meow-mol-003  │                   │
│  │                │ │                │ │                │                   │
│  │  Claude for    │ │  Claude for    │ │  Claude for    │                   │
│  │  outer-loop    │ │  meta-mol      │ │  impl-task     │                   │
│  │  (SUSPENDED)   │ │  (SUSPENDED)   │ │  (ACTIVE)      │                   │
│  │                │ │                │ │                │                   │
│  │  Hook: mol-001 │ │  Hook: mol-002 │ │  Hook: mol-003 │                   │
│  └────────────────┘ └────────────────┘ └────────────────┘                   │
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                       BACKEND INTERFACE                                │ │
│  │                                                                        │ │
│  │  CreateBead()      GetBead()       UpdateBead()                        │ │
│  │  GetHookedWork()   GetReadySteps() AssignBead()                        │ │
│  │                                                                        │ │
│  │  Implementation: BeadsBackend (wraps bd CLI + reads .beads files)      │ │
│  │                                                                        │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## The Execution Model

MEOW Stack is a **Turing-complete durable execution engine**. Templates are programs, molecules are execution frames, and the stack is a call stack. This section defines the control flow primitives that make MEOW a true programming language for AI workflows.

### Core Principle: Templates Are Programs

Any template can be:
- **Linear** → Execute steps sequentially, complete, done
- **Looping** → Have a `restart` step that resets to the beginning with fresh context
- **Conditional** → Have branching steps that route to different paths
- **Nested** → Steps with templates push child molecules onto the stack

There is no "special" outer loop template. The outer loop is just a template that happens to have a restart step. Any template can loop.

### Control Flow Primitives

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    MEOW CONTROL FLOW PRIMITIVES                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  SEQUENCE ─────────────────────────────────────────────────────────────────  │
│  Steps execute in dependency order via `needs` field.                        │
│                                                                              │
│      [[steps]]                                                               │
│      id = "step-1"                                                           │
│                                                                              │
│      [[steps]]                                                               │
│      id = "step-2"                                                           │
│      needs = ["step-1"]   # Sequential dependency                            │
│                                                                              │
│  LOOP ─────────────────────────────────────────────────────────────────────  │
│  Restart step resets molecule with fresh Claude context.                     │
│                                                                              │
│      [[steps]]                                                               │
│      id = "restart"                                                          │
│      type = "restart"                                                        │
│      condition = "times_looped < 10 && has_more_work()"                      │
│      # On restart: KILL session, increment counter, spawn FRESH              │
│                                                                              │
│  BRANCH ───────────────────────────────────────────────────────────────────  │
│  Conditional step routes to different next steps.                            │
│                                                                              │
│      [[steps]]                                                               │
│      id = "check-result"                                                     │
│      type = "conditional"                                                    │
│      branches = [                                                            │
│          { condition = "tests_pass()", goto = "deploy" },                    │
│          { condition = "tests_fail()", goto = "fix-tests" },                 │
│          { condition = "true", goto = "manual-review" }                      │
│      ]                                                                       │
│                                                                              │
│  CALL ─────────────────────────────────────────────────────────────────────  │
│  Step with template pushes child molecule onto stack.                        │
│                                                                              │
│      [[steps]]                                                               │
│      id = "implement-feature"                                                │
│      template = "implement"                                                  │
│      variables = { task_id = "{{current_task}}" }                            │
│                                                                              │
│  RETURN ───────────────────────────────────────────────────────────────────  │
│  Implicit: When no more ready steps, molecule returns to parent.             │
│  Explicit: Claude calls `meow done`.                                         │
│                                                                              │
│  HALT ─────────────────────────────────────────────────────────────────────  │
│  Blocking gate pauses execution, awaits external signal.                     │
│                                                                              │
│      [[steps]]                                                               │
│      id = "await-approval"                                                   │
│      type = "blocking-gate"                                                  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### The Restart Semantic: Fresh Context Per Iteration

**Critical design decision**: When a restart step's condition is true, the orchestrator:

1. **KILLS** the current Claude session (not just exits - actually terminates)
2. **Writes handoff notes** to the molecule bead
3. **Increments** the `times_looped` counter
4. **Spawns** a fresh Claude session with clean context
5. **Fresh Claude** runs `meow prime`, reads handoff, continues

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     LOOP WITH FRESH CONTEXT                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  Iteration 1:                                                                │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Claude Session A (fresh context)                                      │ │
│  │  • Starts with: meow prime                                              │ │
│  │  • Sees: times_looped=0                                                 │ │
│  │  • Executes: step-1 → step-2 → step-3 → restart                         │ │
│  │  • Restart evaluates: condition TRUE                                    │ │
│  │  • Action: Write handoff, EXIT SESSION                                  │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                    │                                         │
│                                    ▼                                         │
│  Orchestrator:                                                               │
│  • Sees restart signal with condition=true                                   │
│  • KILLS Session A (tmux kill-session)                                       │
│  • Increments times_looped → 1                                               │
│  • Resets all non-restart steps to "open"                                    │
│  • Spawns Session B (FRESH context)                                          │
│                                    │                                         │
│                                    ▼                                         │
│  Iteration 2:                                                                │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Claude Session B (fresh context)                                      │ │
│  │  • Starts with: meow prime                                              │ │
│  │  • Sees: times_looped=1, handoff notes from Session A                   │ │
│  │  • Executes: step-1 → step-2 → step-3 → restart                         │ │
│  │  • Restart evaluates: condition TRUE                                    │ │
│  │  • Action: Write handoff, EXIT SESSION                                  │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                    │                                         │
│                               ... repeat ...                                 │
│                                    │                                         │
│                                    ▼                                         │
│  Iteration N (condition becomes FALSE):                                      │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │  Claude Session N (fresh context)                                      │ │
│  │  • Executes steps → restart                                             │ │
│  │  • Restart evaluates: condition FALSE (e.g., times_looped >= 10)        │ │
│  │  • Action: Close restart step, MOLECULE COMPLETE                        │ │
│  │  • Pops stack, returns to parent (if any)                               │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Why fresh context per iteration?**

```
WRONG: Same session for all iterations
───────────────────────────────────────
Iteration 1: 20% context used
Iteration 2: 40% context used
Iteration 3: 60% context used  → handoff triggered
Iteration 4: 80% context used  → handoff triggered again
...eventually thrashing on handoffs

RIGHT: Fresh session per iteration
───────────────────────────────────────
Iteration 1: 20% context → restart → KILL → spawn fresh
Iteration 2: 20% context → restart → KILL → spawn fresh
Iteration N: 20% context → condition false → complete

Each iteration is a BOUNDED UNIT with predictable context usage.
```

### The `times_looped` Counter

Every molecule has a built-in `times_looped` counter:

- **Stored on** the molecule bead
- **Starts at** 0 when molecule is created
- **Incremented by** orchestrator on each successful restart
- **Available to** conditions and template variables

```toml
[[steps]]
id = "restart"
type = "restart"
condition = "times_looped < 100"

[[steps]]
id = "log-progress"
description = "Iteration {{times_looped}} of {{max_iterations}}"
```

### Conditional Branching

Conditional steps evaluate conditions and route to different next steps:

```toml
[[steps]]
id = "check-coverage"
type = "conditional"
needs = ["run-tests"]
branches = [
    { condition = "{{coverage_percent}} >= 90", goto = "deploy" },
    { condition = "times_looped >= 10", goto = "give-up" },
    { condition = "true", goto = "improve-coverage" }  # Default
]

[[steps]]
id = "deploy"
description = "Coverage goal achieved, deploy!"

[[steps]]
id = "give-up"
description = "Coverage goal not achievable after 10 iterations"

[[steps]]
id = "improve-coverage"
description = "Add more tests to improve coverage"
template = "write-tests"
```

**Evaluation order**: Branches are evaluated top-to-bottom; first match wins.

### Breaking Out of Loops

Three ways to exit a loop:

**1. Condition on Restart**
```toml
[[steps]]
id = "restart"
type = "restart"
condition = "times_looped < 10 && !goal_achieved()"
# When goal_achieved() returns true, loop exits
```

**2. Conditional Branch Before Restart**
```toml
[[steps]]
id = "check-done"
type = "conditional"
needs = ["main-work"]
branches = [
    { condition = "goal_achieved()", goto = "finalize" },
    { condition = "true", goto = "restart" }
]

[[steps]]
id = "restart"
type = "restart"
needs = ["check-done"]  # Only reached if check-done routes here
condition = "times_looped < 100"  # Safety limit

[[steps]]
id = "finalize"
description = "Goal achieved, clean up"
# This exits the molecule normally
```

**3. Early Exit via `meow done`**
```bash
# Inside any step, Claude can call:
meow done --notes "Goal achieved early after {{times_looped}} iterations"
```

This closes all remaining steps as "skipped" and completes the molecule.

### Example: Goal-Oriented Looping Template

```toml
[meta]
name = "achieve-coverage"
description = "Loop until test coverage >= 90%"

[variables]
target_coverage = { default = 90, description = "Target coverage %" }
max_iterations = { default = 10, description = "Max loop iterations" }

[[steps]]
id = "measure-coverage"
description = "Measure current test coverage"
instructions = """
Run: pytest --cov=src --cov-report=json
Parse coverage.json for total coverage percentage.
Store result: bd update {{step_id}} --notes "coverage={{percent}}"
"""

[[steps]]
id = "check-goal"
type = "conditional"
needs = ["measure-coverage"]
branches = [
    { condition = "{{coverage}} >= {{target_coverage}}", goto = "success" },
    { condition = "times_looped >= {{max_iterations}}", goto = "failure" },
    { condition = "true", goto = "improve" }
]

[[steps]]
id = "improve"
description = "Write more tests to improve coverage"
needs = ["check-goal"]
template = "write-tests"
variables = { focus = "uncovered-code" }

[[steps]]
id = "restart"
type = "restart"
needs = ["improve"]
condition = "true"  # Always restart (check-goal handles exit conditions)

[[steps]]
id = "success"
description = "Coverage goal achieved!"
instructions = "Log success: Coverage at {{coverage}}% after {{times_looped}} iterations"

[[steps]]
id = "failure"
description = "Coverage goal not achievable"
instructions = "Log failure: Gave up at {{coverage}}% after {{max_iterations}} iterations"
```

### Ephemeral vs Persistent: Loop Iterations as Wisps

Loop iterations are **semantically ephemeral**. Each iteration:

1. Runs in fresh context
2. Writes handoff notes (compressed summary)
3. Gets "squashed" on restart

The molecule bead persists, but individual iteration state is transient. This prevents:
- Unbounded step bead accumulation
- Git history bloat
- Query performance degradation

**Iteration digest** (written on restart):
```yaml
# Stored in molecule.iteration_history (array)
- iteration: 3
  started_at: "2026-01-06T10:00:00Z"
  completed_at: "2026-01-06T10:15:00Z"
  summary: "Selected 2 tasks, implemented auth module"
  steps_executed: 4
  outcome: "restart"  # or "complete", "branch-exit"
```

### Turing Completeness Summary

| Construct | MEOW Equivalent |
|-----------|-----------------|
| **Sequence** | Step dependencies (`needs`) |
| **Loop** | `restart` step with condition + fresh context |
| **Conditional** | `conditional` step with branches |
| **Subroutine** | `template` expansion (child molecule) |
| **Variable** | Beads fields, step outputs, `times_looped`, template vars |
| **State** | Git-backed beads storage |
| **Recursion** | Template that references itself (with depth limit) |

This makes MEOW a **durable execution language** for AI agent orchestration, not just "running Claude with prompts."

---

## The Hook Model

Borrowed from gastown: each Claude agent has work "hooked" to it. The agent works on its hooked molecule until complete.

### Hook Concept

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              HOOK MODEL                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  AGENT                          HOOK                          MOLECULE       │
│  ──────                         ────                          ────────       │
│  meow-mol-001      ──────▶    hook_bead    ──────▶         mol-001          │
│  (Claude session)            (molecule ID)              (workflow to execute)│
│                                                                              │
│  The agent's job:                                                            │
│  1. Check what's on its hook (meow prime)                                    │
│  2. Execute the hooked workflow                                              │
│  3. Signal done (meow done) or handoff (meow handoff)                        │
│  4. Clear the hook when complete                                             │
│                                                                              │
│  Agent Bead Fields (stored in beads):                                        │
│  ─────────────────────────────────────                                       │
│  agent_id:      meow-mol-001                                                 │
│  hook_bead:     mol-001                    # What workflow is hooked         │
│  agent_state:   working | done | handoff   # Current state                   │
│  handoff_notes: "Progress: X, Next: Y"     # Context for next session        │
│                                                                              │
│  Molecule Bead Fields:                                                       │
│  ─────────────────────                                                       │
│  assignee:      meow-mol-001               # Which agent owns this           │
│  handoff_notes: "..."                      # Stored on molecule, not agent   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Assignment at Bake Time

When baking a molecule, all steps are assigned to the agent:

```go
func (b *Baker) Bake(templateName string, agentID string, vars map[string]interface{}) (*Molecule, error) {
    // ... create molecule ...

    // Assign molecule to agent
    mol.Assignee = agentID

    // Assign all steps to same agent
    for _, step := range steps {
        step.Assignee = agentID
    }

    // Hook the molecule to the agent
    b.backend.SetHook(agentID, mol.ID)
}
```

### Query Own Work

Claude can query what's assigned to it:

```bash
# What's on my hook?
meow prime  # Returns hooked molecule and ready steps

# What's assigned to me?
bd list --assignee meow-mol-001 --status open
```

---

## The Orchestrator Model

### Core Principle: Go Controls, Claude Executes

The Go orchestrator is the **control plane**:
- Owns the molecule stack
- Spawns Claude sessions with `meow prime` as the initial prompt
- Monitors sessions via stop hook
- Reads context usage from tmux status line
- Triggers handoff when context exceeds threshold
- Handles structural operations (push/pop)

Claude is the **execution plane**:
- Runs `meow prime` to see assigned work
- Executes steps, closing them via `bd close`
- Signals completion via `meow done`
- Signals context handoff via `meow handoff`
- Iterates within a molecule via stop hook (Ralph style)

### The Execution Loop

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         ORCHESTRATOR MAIN LOOP                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  func (o *Orchestrator) Run(templateName string) error {                     │
│                                                                              │
│      // 1. Create agent ID for this level                                    │
│      agentID := fmt.Sprintf("meow-%s", generateID())                         │
│                                                                              │
│      // 2. Bake molecule from template, assign to agent                      │
│      mol := o.baker.Bake(templateName, agentID, vars)                        │
│      o.stack.Push(agentID, mol.ID)                                           │
│                                                                              │
│      // 3. Spawn Claude with "meow prime" as prompt                          │
│      o.spawnSession(agentID)                                                 │
│                                                                              │
│      // 4. Main orchestration loop                                           │
│      for !o.stack.IsEmpty() {                                                │
│          frame := o.stack.Top()                                              │
│                                                                              │
│          // Wait for session to exit (stop hook controls iteration)          │
│          signal := o.waitForExit(frame)                                      │
│                                                                              │
│          switch signal.Type {                                                │
│          case "done":                                                        │
│              // Claude called meow done - workflow complete                  │
│              o.handleMoleculeComplete(frame)                                 │
│                                                                              │
│          case "push":                                                        │
│              // Step has template - spawn child                              │
│              o.handlePush(frame, signal)                                     │
│                                                                              │
│          case "pop":                                                         │
│              // Child completed - resume parent                              │
│              o.handlePop(frame, signal)                                      │
│                                                                              │
│          case "restart":                                                     │
│              // Restart step with true condition - KILL and spawn fresh      │
│              o.handleRestart(frame, signal)                                  │
│                                                                              │
│          case "branch":                                                      │
│              // Conditional step evaluated - route to target                 │
│              o.handleBranch(frame, signal)                                   │
│                                                                              │
│          case "handoff":                                                     │
│              // Context limit - spawn fresh Claude, same molecule            │
│              o.handleHandoff(frame, signal)                                  │
│                                                                              │
│          case "gate":                                                        │
│              // Blocking gate - exit orchestrator                            │
│              return nil                                                      │
│                                                                              │
│          case "continue":                                                    │
│              // Stop hook fed prompt back, session still running             │
│              // This shouldn't reach here (session didn't exit)              │
│          }                                                                   │
│      }                                                                       │
│                                                                              │
│      return nil  // All done                                                 │
│  }                                                                           │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Stack Operations

| Operation | Trigger | Action |
|-----------|---------|--------|
| **PUSH** | Step has template | Bake child molecule with new agent, spawn new Claude |
| **POP** | `meow done` called or all steps closed | Close molecule, resume parent session |
| **RESTART** | Restart step with true condition | KILL session, increment counter, reset steps, spawn fresh |
| **HANDOFF** | Context > 60% | Save notes to molecule, spawn fresh Claude, same molecule |
| **CONTINUE** | Stop hook injects `meow prime` | Claude keeps iterating (doesn't exit) |
| **PAUSE** | Gate step reached | Exit orchestrator, wait for `meow approve` |
| **BRANCH** | Conditional step evaluated | Route to target step, mark others unreachable |

### Restart Handler

The restart handler is crucial for the loop-with-fresh-context model:

```go
func (o *Orchestrator) handleRestart(frame *StackFrame, signal *ExitSignal) {
    mol := o.backend.GetMolecule(frame.MoleculeID)

    // 1. Write iteration digest before reset
    digest := &IterationDigest{
        Iteration:     mol.TimesLooped,
        StartedAt:     frame.IterationStartedAt,
        CompletedAt:   time.Now().UTC().Format(time.RFC3339),
        Summary:       signal.HandoffNotes,
        StepsExecuted: o.countClosedSteps(mol),
        Outcome:       "restart",
    }
    mol.IterationHistory = append(mol.IterationHistory, digest)

    // 2. Increment loop counter
    mol.TimesLooped++

    // 3. Reset all non-restart steps to "open"
    for _, step := range mol.Steps {
        if step.StepType != "restart" {
            step.Status = "open"
        }
    }

    // 4. Store handoff notes for next iteration
    mol.HandoffNotes = signal.HandoffNotes
    o.backend.UpdateMolecule(mol)

    // 5. KILL the current tmux session (critical: fresh context)
    o.tmux.KillSession(frame.AgentID)

    // 6. Spawn fresh Claude session
    o.spawnSession(frame.AgentID)

    o.log.Info("RESTART",
        "molecule", mol.ID,
        "times_looped", mol.TimesLooped,
        "summary", digest.Summary,
    )
}
```

### Branch Handler

The branch handler routes conditional execution:

```go
func (o *Orchestrator) handleBranch(frame *StackFrame, signal *ExitSignal) {
    mol := o.backend.GetMolecule(frame.MoleculeID)
    condStep := o.backend.GetStep(signal.StepID)

    // 1. Mark the conditional step as closed
    condStep.Status = "closed"
    condStep.Notes = fmt.Sprintf("Branched to: %s", signal.BranchTarget)

    // 2. Find target step and mark it ready
    targetStep := o.backend.GetStep(mol.ID + "." + signal.BranchTarget)
    targetStep.Status = "open"

    // 3. Mark unreachable branches as "skipped"
    for _, branch := range condStep.Branches {
        if branch.Goto != signal.BranchTarget {
            skipStep := o.backend.GetStep(mol.ID + "." + branch.Goto)
            if skipStep != nil && skipStep.Status == "open" {
                // Only skip if not reachable via other paths
                if !o.isReachableViaOtherPath(mol, skipStep.ID) {
                    skipStep.Status = "skipped"
                }
            }
        }
    }

    o.backend.UpdateMolecule(mol)

    // 4. Continue execution (session still running, stop hook will iterate)
    o.log.Info("BRANCH",
        "step", condStep.ID,
        "target", signal.BranchTarget,
    )
}
```

---

## Session Management

### One Claude Per Stack Level

Each level of the molecule stack runs in its own Claude session:

```
Stack Level 0: meow-outer-001
├── Claude Session: meow-outer-001 (tmux)
├── Session ID: 550e8400-e29b-41d4-a716-446655440000
├── Hook: outer-loop-001
└── Status: SUSPENDED (waiting for child)

Stack Level 1: meow-meta-001
├── Claude Session: meow-meta-001 (tmux)
├── Session ID: 661f9511-f30c-52e5-b827-557766551111
├── Hook: meta-mol-001
└── Status: SUSPENDED (waiting for child)

Stack Level 2: meow-impl-001
├── Claude Session: meow-impl-001 (tmux)
├── Session ID: 772a0622-a41d-63f6-c938-668877662222
├── Hook: impl-task-1-001
└── Status: ACTIVE (executing write-tests step)
└── Context: 45k/200k (22%)
```

### Spawning a Session

All sessions start with `meow prime` as the prompt:

```go
func (o *Orchestrator) spawnSession(agentID string) error {
    tmuxSession := agentID  // tmux session name = agent ID

    // Build claude command - always starts with "meow prime"
    claudeCmd := fmt.Sprintf(
        "claude -p 'meow prime' --output-format json --allowedTools '%s'",
        o.config.AllowedTools,
    )

    // Create tmux session
    cmd := exec.Command("tmux", "new-session", "-d", "-s", tmuxSession, claudeCmd)
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("failed to create tmux session: %w", err)
    }

    o.log.Info("Spawned session", "tmux", tmuxSession, "agent", agentID)
    return nil
}
```

### Session Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         SESSION LIFECYCLE                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  SPAWN ──────────────────────────────────────────────────────────────────▶  │
│  │                                                                          │
│  │  tmux new-session -d -s "meow-impl-001" "claude -p 'meow prime' ..."     │
│  │                                                                          │
│  ▼                                                                          │
│  PRIME ──────────────────────────────────────────────────────────────────▶  │
│  │                                                                          │
│  │  Claude runs meow prime → sees hooked molecule and ready steps           │
│  │  Begins working on first ready step                                      │
│  │                                                                          │
│  ▼                                                                          │
│  WORKING ────────────────────────────────────────────────────────────────▶  │
│  │                                                                          │
│  │  Claude executes steps, closes them via bd close                         │
│  │  Stop hook monitors after each turn:                                     │
│  │    - Work remaining? → inject "meow prime", continue                     │
│  │    - Context > 60%?  → prompt for "meow handoff"                         │
│  │    - All done?       → allow exit                                        │
│  │                                                                          │
│  ├──▶ More work + context OK ──▶ Stop hook injects "meow prime" (CONTINUE)  │
│  │                                                                          │
│  ├──▶ Context > 60% ──▶ Stop hook prompts for handoff (HANDOFF)             │
│  │                                                                          │
│  ├──▶ Template step next ──▶ Allow exit, write PUSH signal                  │
│  │                                                                          │
│  ├──▶ Gate reached ──▶ Allow exit, write GATE signal                        │
│  │                                                                          │
│  └──▶ meow done called ──▶ Allow exit, write DONE signal                    │
│                                                                              │
│  ▼                                                                          │
│  EXITED ─────────────────────────────────────────────────────────────────▶  │
│  │                                                                          │
│  │  Orchestrator reads .meow/exit-signal.json                               │
│  │  Takes appropriate action (push/pop/handoff/gate)                        │
│  │                                                                          │
│  ▼                                                                          │
│  RESUMED (for parents) ──────────────────────────────────────────────────▶  │
│  │                                                                          │
│  │  claude --resume <session-id> -p "meow prime"                            │
│  │  Parent sees child completed, continues with next step                   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Stop Hook Integration

### Context Detection from Status Line

The orchestrator reads Claude's context usage from the tmux status line:

```
/data/projects/meow-machine │ main │ 🧠 Opus 4.5 │ 100k/200k (50%)
                                                    └── Context usage
```

```go
func (o *Orchestrator) getContextUsage(tmuxSession string) (int, error) {
    // Capture current tmux pane content
    cmd := exec.Command("tmux", "capture-pane", "-t", tmuxSession, "-p")
    output, err := cmd.Output()
    if err != nil {
        return 0, err
    }

    // Parse status line for context percentage
    // Pattern: "100k/200k (50%)" or similar
    re := regexp.MustCompile(`(\d+)k/(\d+)k \((\d+)%\)`)
    matches := re.FindStringSubmatch(string(output))
    if len(matches) >= 4 {
        percent, _ := strconv.Atoi(matches[3])
        return percent, nil
    }

    return 0, nil  // Unknown, assume OK
}
```

### Stop Hook Script

```bash
#!/bin/bash
# .meow/hooks/stop-hook.sh
# Controls Claude iteration within a molecule

set -euo pipefail

HOOK_INPUT=$(cat)
MEOW_STATE=".meow/state.json"

# Check if MEOW is active
if [[ ! -f "$MEOW_STATE" ]]; then
    echo '{"decision":"approve","reason":"No active MEOW workflow"}'
    exit 0
fi

# Prevent infinite loops
STOP_HOOK_ACTIVE=$(echo "$HOOK_INPUT" | jq -r '.stop_hook_active')
if [[ "$STOP_HOOK_ACTIVE" == "true" ]]; then
    echo '{"decision":"approve","reason":"Stop hook already active"}'
    exit 0
fi

# Get current agent from state
AGENT_ID=$(jq -r '.stack[-1].agent_id' "$MEOW_STATE")
TMUX_SESSION="$AGENT_ID"

# Check context usage from tmux status line
CONTEXT_PERCENT=$(tmux capture-pane -t "$TMUX_SESSION" -p 2>/dev/null | \
    grep -oE '\([0-9]+%\)' | tail -1 | tr -d '()%' || echo "0")

# Query MEOW for workflow state
ACTION=$(meow check --agent "$AGENT_ID" --context "$CONTEXT_PERCENT" --json)
DECISION=$(echo "$ACTION" | jq -r '.decision')

case "$DECISION" in
    "continue")
        # More work to do - inject meow prime to keep going
        # This handles cases where Claude asks user for clarification
        # but we just want it to make a decision and continue
        jq -n '{
            "decision": "block",
            "reason": "meow prime",
            "systemMessage": "Continue working on your hooked molecule. Run meow prime to see remaining work."
        }'
        ;;

    "handoff")
        # Context limit approaching - prompt for handoff
        CONTEXT_MSG=$(echo "$ACTION" | jq -r '.context_message')
        jq -n --arg msg "$CONTEXT_MSG" '{
            "decision": "block",
            "reason": $msg,
            "systemMessage": "HANDOFF REQUIRED - Save your context and run: meow handoff"
        }'
        ;;

    "push"|"pop"|"done"|"gate"|"restart"|"branch")
        # Structural change - allow exit, orchestrator handles
        echo "$ACTION" > .meow/exit-signal.json
        jq -n --arg reason "$(echo "$ACTION" | jq -r '.reason')" '{
            "decision": "approve",
            "reason": $reason
        }'
        ;;

    *)
        echo '{"decision":"approve","reason":"Unknown action"}'
        ;;
esac

exit 0
```

### The `meow check` Command

Called by the stop hook to determine next action:

```go
func (c *CLI) Check(agentID string, contextPercent int) (*CheckResult, error) {
    // Get hooked molecule for this agent
    hookBead := c.backend.GetHook(agentID)
    if hookBead == "" {
        return &CheckResult{Decision: "done", Reason: "No work on hook"}, nil
    }

    mol := c.backend.GetMolecule(hookBead)

    // Check context threshold FIRST
    if contextPercent > 60 {
        return &CheckResult{
            Decision: "handoff",
            Reason:   fmt.Sprintf("Context at %d%%, handoff required", contextPercent),
            ContextMessage: fmt.Sprintf(
                "Context window at %d%%. Before continuing:\n"+
                "1. Update current step with progress notes: bd update <step> --notes '...'\n"+
                "2. Run: meow handoff --notes 'Summary of progress and next steps'\n\n"+
                "This will save your context and spawn a fresh session to continue.",
                contextPercent,
            ),
        }, nil
    }

    // Check if meow done was called
    state := c.loadState()
    if state.DoneCalled {
        state.DoneCalled = false
        c.saveState(state)
        return &CheckResult{Decision: "done", Reason: "Workflow complete"}, nil
    }

    // Check ready steps
    readySteps := c.backend.GetReadySteps(mol.ID)

    if len(readySteps) == 0 {
        // All steps closed - molecule complete
        return &CheckResult{Decision: "pop", Reason: "All steps closed"}, nil
    }

    nextStep := readySteps[0]

    if nextStep.Template != "" {
        // Need to push child molecule
        return &CheckResult{
            Decision: "push",
            Reason:   fmt.Sprintf("Step %s has template", nextStep.ID),
            StepID:   nextStep.ID,
            Template: nextStep.Template,
        }, nil
    }

    if nextStep.Type == "blocking-gate" {
        return &CheckResult{
            Decision: "gate",
            Reason:   fmt.Sprintf("Gate %s reached", nextStep.ID),
            StepID:   nextStep.ID,
        }, nil
    }

    // Handle restart steps - evaluate condition
    if nextStep.StepType == "restart" {
        conditionMet, err := c.evaluateCondition(nextStep.Condition, mol)
        if err != nil {
            return nil, fmt.Errorf("failed to evaluate restart condition: %w", err)
        }

        if conditionMet {
            // Condition true - restart with fresh context
            return &CheckResult{
                Decision:     "restart",
                Reason:       fmt.Sprintf("Restart condition met: %s", nextStep.Condition),
                StepID:       nextStep.ID,
                TimesLooped:  mol.TimesLooped,
            }, nil
        } else {
            // Condition false - close restart step, molecule completes
            c.backend.CloseStep(nextStep.ID)
            return &CheckResult{Decision: "pop", Reason: "Restart condition false, molecule complete"}, nil
        }
    }

    // Handle conditional steps - evaluate branches
    if nextStep.StepType == "conditional" {
        for _, branch := range nextStep.Branches {
            conditionMet, err := c.evaluateCondition(branch.Condition, mol)
            if err != nil {
                return nil, fmt.Errorf("failed to evaluate branch condition: %w", err)
            }

            if conditionMet {
                return &CheckResult{
                    Decision:     "branch",
                    Reason:       fmt.Sprintf("Branch %s → %s", nextStep.ID, branch.Goto),
                    StepID:       nextStep.ID,
                    BranchTarget: branch.Goto,
                }, nil
            }
        }
        // No branch matched - this is an error
        return nil, fmt.Errorf("no branch condition matched for step %s", nextStep.ID)
    }

    // More atomic work - continue with meow prime
    return &CheckResult{
        Decision: "continue",
        Reason:   fmt.Sprintf("Ready: %s", nextStep.ID),
    }, nil
}

// evaluateCondition evaluates a shell condition against molecule state
func (c *CLI) evaluateCondition(condition string, mol *Molecule) (bool, error) {
    // Expand template variables
    expanded := c.expandVariables(condition, mol)

    // Run shell condition
    cmd := exec.Command("sh", "-c", fmt.Sprintf("test %s", expanded))
    err := cmd.Run()

    // Exit code 0 = true, non-zero = false
    return err == nil, nil
}

// expandVariables replaces template variables with actual values
func (c *CLI) expandVariables(template string, mol *Molecule) string {
    result := template

    // Built-in variables
    result = strings.ReplaceAll(result, "times_looped", strconv.Itoa(mol.TimesLooped))
    result = strings.ReplaceAll(result, "{{times_looped}}", strconv.Itoa(mol.TimesLooped))

    // Molecule variables
    for key, val := range mol.Variables {
        result = strings.ReplaceAll(result, "{{"+key+"}}", fmt.Sprintf("%v", val))
    }

    return result
}
```

---

## Context Management & Handoff

### Handoff Flow

When context exceeds threshold, the stop hook prompts Claude to handoff:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           HANDOFF FLOW                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  1. Stop hook detects context > 60%                                          │
│     └─▶ Injects prompt: "HANDOFF REQUIRED - run meow handoff"               │
│                                                                              │
│  2. Claude prepares handoff:                                                 │
│     └─▶ Updates current step: bd update <step> --notes "progress..."        │
│     └─▶ Runs: meow handoff --notes "Context for next session"               │
│                                                                              │
│  3. meow handoff:                                                            │
│     └─▶ Writes handoff notes to molecule bead                               │
│     └─▶ Sets agent_state = "handoff"                                        │
│     └─▶ Writes exit signal                                                  │
│     └─▶ Claude session exits                                                │
│                                                                              │
│  4. Orchestrator sees handoff signal:                                        │
│     └─▶ Kills old tmux session                                              │
│     └─▶ Creates NEW agent ID (fresh context)                                │
│     └─▶ Reassigns molecule to new agent                                     │
│     └─▶ Spawns fresh Claude with "meow prime"                               │
│                                                                              │
│  5. Fresh Claude runs meow prime:                                            │
│     └─▶ Sees hooked molecule                                                │
│     └─▶ Sees handoff notes: "Previous session context..."                   │
│     └─▶ Sees ready steps                                                    │
│     └─▶ Continues work from where previous session left off                 │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Handoff Notes Storage

Handoff notes are stored on the **molecule bead** (like gastown):

```go
func (c *CLI) Handoff(notes string) error {
    state := c.loadState()
    frame := state.Stack.Top()

    // Store handoff notes on molecule
    c.backend.UpdateBead(frame.MoleculeID, map[string]interface{}{
        "handoff_notes": notes,
        "handoff_at":    time.Now().UTC().Format(time.RFC3339),
    })

    // Update agent state
    c.backend.UpdateBead(frame.AgentID, map[string]interface{}{
        "agent_state": "handoff",
    })

    // Write exit signal
    signal := &ExitSignal{
        Type:   "handoff",
        Reason: "Context limit, handoff requested",
        Notes:  notes,
    }
    c.writeExitSignal(signal)

    return nil
}
```

### `meow prime` Output

When Claude runs `meow prime`, it sees:

```
═══════════════════════════════════════════════════════════════════════════════
MEOW PRIME - Agent meow-impl-001
═══════════════════════════════════════════════════════════════════════════════

HOOKED MOLECULE: impl-task-1-001
Template: implement
Status: in_progress
Times Looped: 3              ← Loop iteration counter (0 = first run)
Iteration Started: 2026-01-06T10:15:00Z

HANDOFF NOTES (from previous iteration):
───────────────────────────────────────
Iteration 2 completed. Selected 2 tasks, implemented user auth module.
Coverage at 78%. Need 90% target. Continuing to add more tests.
───────────────────────────────────────

ITERATION HISTORY:
  [0] 2026-01-06T09:00 → restart (5 steps, "Initial setup and first task")
  [1] 2026-01-06T09:30 → restart (4 steps, "Second task completed")
  [2] 2026-01-06T10:00 → restart (5 steps, "Coverage at 78%")

READY STEPS:
  [1] impl-task-1-001.implement (in_progress)
      Description: Write implementation
      Instructions: Implement minimum code to pass tests. Follow existing patterns.

  [2] impl-task-1-001.verify-pass (blocked by: implement)
  [3] impl-task-1-001.commit (blocked by: verify-pass)

YOUR TASK:
Work on the ready steps above. When you complete a step, close it:
  bd close <step-id> --notes "what you did"

When ALL steps are complete, signal done:
  meow done --notes "summary of work"

If you're running low on context, save your work and handoff:
  meow handoff --notes "context for next session"

═══════════════════════════════════════════════════════════════════════════════
```

---

## CLI Commands

### `meow init`

Initialize MEOW in the current repository.

```bash
meow init
```

**Actions:**
1. Run `bd init` if `.beads` doesn't exist
2. Create `.meow/` directory structure
3. Create `.meow/hooks/stop-hook.sh`
4. Create `.meow/templates/` with built-in templates
5. Install Claude hook configuration

### `meow run <template>`

Bake a template and start execution.

```bash
meow run outer-loop
meow run implement --var task_id=bd-task-001
```

**Actions:**
1. Generate agent ID
2. Bake template into molecule, assign to agent
3. Hook molecule to agent
4. Initialize stack
5. Spawn Claude with `meow prime`
6. Enter orchestration loop

### `meow prime`

Show hooked work and ready steps. This is the **startup prompt** for all Claude sessions.

```bash
meow prime
meow prime --agent meow-impl-001  # Specific agent
```

**Output:**
- Hooked molecule info
- Handoff notes (if any)
- Ready steps with instructions
- Commands reference

**Key behavior:** If Claude asks for clarification or tries to stop when work remains, the stop hook injects `meow prime` to keep it going.

### `meow done`

Signal graceful workflow completion.

```bash
meow done
meow done --notes "Implemented registration with full test coverage"
```

**Actions:**
1. Verify all steps in hooked molecule are closed
2. Write completion notes to molecule
3. Clear hook from agent
4. Write exit signal for orchestrator

### `meow handoff`

Save context and signal for fresh session.

```bash
meow handoff --notes "Progress: finished tests. Next: implement. Blockers: none"
```

**Actions:**
1. Write handoff notes to molecule bead
2. Set agent state to "handoff"
3. Write exit signal
4. Claude session exits
5. Orchestrator spawns fresh Claude

### `meow continue`

Resume execution from current state.

```bash
meow continue
```

**Actions:**
1. Load state from `.meow/state.json`
2. Reconstruct stack from beads
3. Resume orchestrator loop
4. Either continue active session or spawn fresh

### `meow status`

Show current execution state.

```bash
meow status
meow status --json
```

**Output:**
```
MEOW Stack Status
═════════════════

Stack:
  [0] meow-outer-001 → outer-loop-001 (SUSPENDED)
      Context: 45k/200k (22%)
      Step: run-inner → waiting for child

  [1] meow-meta-001 → meta-mol-001 (SUSPENDED)
      Context: 78k/200k (39%)
      Step: task-1 → waiting for child

  [2] meow-impl-001 → impl-task-1-001 (ACTIVE)
      Context: 120k/200k (60%) ⚠️
      Step: implement (in_progress)

tmux sessions:
  meow-outer-001   (attached: no)
  meow-meta-001    (attached: no)
  meow-impl-001    (attached: yes) ← active

To attach: tmux attach -t meow-impl-001
```

### `meow approve` / `meow reject`

Gate handling (unchanged from previous doc).

### `meow check`

Internal command called by stop hook.

```bash
meow check --agent meow-impl-001 --context 45 --json
```

---

## Backend Abstraction

### Extended Interface

```go
type Backend interface {
    // Bead CRUD
    CreateBead(bead *Bead) (string, error)
    GetBead(id string) (*Bead, error)
    UpdateBead(id string, updates map[string]interface{}) error
    CloseBead(id string) error

    // Queries
    ListBeads(filter *Filter) ([]*Bead, error)
    GetChildren(parentID string) ([]*Bead, error)
    GetReadySteps(moleculeID string) ([]*Bead, error)

    // Hook operations (gastown-style)
    GetHook(agentID string) (string, error)        // Get hooked bead ID
    SetHook(agentID, beadID string) error          // Hook bead to agent
    ClearHook(agentID string) error                // Clear agent's hook

    // Assignment
    AssignBead(beadID, agentID string) error       // Set assignee
    GetAssignedBeads(agentID string) ([]*Bead, error)

    // Molecule-specific
    GetMolecule(id string) (*Molecule, error)
    CreateMolecule(mol *Molecule) error
}

type Bead struct {
    ID           string
    Title        string
    Description  string
    Type         string  // task, epic, molecule, step, gate, agent
    Status       string  // open, in_progress, closed, blocked
    Parent       string
    Dependencies []string
    Labels       []string
    Notes        string

    // Assignment (gastown-style)
    Assignee     string  // Agent ID that owns this bead

    // Hook fields (for agent beads)
    HookBead     string  // Molecule ID on hook
    AgentState   string  // spawning, working, done, handoff

    // Handoff fields (for molecule beads)
    HandoffNotes string  // Context from previous session
    HandoffAt    string  // When handoff occurred

    // Molecule-specific
    Template     string
    CurrentStep  string
    Iteration    int

    // Loop control (for molecules with restart steps)
    TimesLooped      int                // Counter incremented on each restart
    IterationHistory []IterationDigest  // Summary of each completed iteration

    // Step-specific
    StepType     string    // atomic, restart, conditional, blocking-gate
    Condition    string    // Shell condition for restart/conditional steps
    Instructions string
    Branches     []Branch  // For conditional steps
}

type IterationDigest struct {
    Iteration    int       `json:"iteration"`
    StartedAt    string    `json:"started_at"`
    CompletedAt  string    `json:"completed_at"`
    Summary      string    `json:"summary"`
    StepsExecuted int      `json:"steps_executed"`
    Outcome      string    `json:"outcome"`  // restart, complete, branch-exit
}

type Branch struct {
    Condition string `json:"condition"`
    Goto      string `json:"goto"`
}
```

---

## Template System

Templates unchanged from previous doc, but baking now includes assignment:

```go
func (b *Baker) Bake(templateName string, agentID string, vars map[string]interface{}) (*Molecule, error) {
    tmpl, _ := b.registry.Get(templateName)
    molID := fmt.Sprintf("%s-%s", templateName, b.generateShortID())

    // Create molecule with assignee
    mol := &Bead{
        ID:       molID,
        Title:    fmt.Sprintf("Molecule: %s", templateName),
        Type:     "molecule",
        Status:   "open",
        Assignee: agentID,  // ← Assigned to agent
        Template: templateName,
    }
    b.backend.CreateBead(mol)

    // Create steps with assignee
    for _, stepDef := range tmpl.Steps {
        step := &Bead{
            ID:       fmt.Sprintf("%s.%s", molID, stepDef.ID),
            Parent:   molID,
            Assignee: agentID,  // ← Same assignee
            // ... rest of fields
        }
        b.backend.CreateBead(step)
    }

    // Hook molecule to agent
    b.backend.SetHook(agentID, molID)

    return &Molecule{ID: molID}, nil
}
```

---

## State Management

### State File

```json
{
  "active": true,
  "started_at": "2026-01-06T10:00:00Z",
  "root_molecule": "outer-loop-001",

  "stack": [
    {
      "agent_id": "meow-outer-001",
      "molecule_id": "outer-loop-001",
      "session_id": "550e8400-e29b-41d4-a716-446655440000",
      "current_step": "run-inner",
      "status": "suspended"
    },
    {
      "agent_id": "meow-meta-001",
      "molecule_id": "meta-mol-001",
      "session_id": "661f9511-f30c-52e5-b827-557766551111",
      "current_step": "task-1",
      "status": "suspended"
    },
    {
      "agent_id": "meow-impl-001",
      "molecule_id": "impl-task-1-001",
      "session_id": "772a0622-a41d-63f6-c938-668877662222",
      "current_step": "implement",
      "status": "active"
    }
  ],

  "done_called": false
}
```

### Durability via Beads

All critical state is in beads (git-backed):
- Molecule status and assignee
- Step status and assignee
- Agent hook_bead and agent_state
- Handoff notes on molecules

State file is for runtime coordination; can be reconstructed from beads.

---

## Logging & Debugging

### Log Format

```
2026-01-06T10:15:32Z [INFO]  BAKE template=implement agent=meow-impl-001 molecule=impl-task-1-001
2026-01-06T10:15:33Z [INFO]  HOOK agent=meow-impl-001 bead=impl-task-1-001
2026-01-06T10:15:34Z [INFO]  SPAWN agent=meow-impl-001 prompt="meow prime"
2026-01-06T10:16:42Z [INFO]  STEP_CLOSED step=impl-task-1-001.load-context
2026-01-06T10:16:43Z [DEBUG] CHECK agent=meow-impl-001 context=22% decision=continue
2026-01-06T10:25:30Z [WARN]  CHECK agent=meow-impl-001 context=62% decision=handoff
2026-01-06T10:25:45Z [INFO]  HANDOFF agent=meow-impl-001 molecule=impl-task-1-001
2026-01-06T10:25:46Z [INFO]  SPAWN agent=meow-impl-002 prompt="meow prime" (handoff continuation)

# Restart with fresh context (loop iteration)
2026-01-06T11:00:00Z [INFO]  RESTART molecule=achieve-001 times_looped=3 condition="times_looped < 10"
2026-01-06T11:00:00Z [INFO]  DIGEST iteration=2 summary="Coverage at 78%, added 5 tests"
2026-01-06T11:00:01Z [INFO]  KILL session=meow-achieve-001 reason="restart with fresh context"
2026-01-06T11:00:02Z [INFO]  SPAWN agent=meow-achieve-001 prompt="meow prime" iteration=3

# Conditional branch
2026-01-06T11:15:00Z [INFO]  BRANCH step=check-goal condition="coverage >= 90" target=success
2026-01-06T11:15:00Z [INFO]  SKIP step=improve reason="unreachable after branch to success"
2026-01-06T11:15:00Z [INFO]  SKIP step=restart reason="unreachable after branch to success"

# Loop exit (condition false)
2026-01-06T12:00:00Z [INFO]  CHECK step=restart condition="times_looped < 10" result=false
2026-01-06T12:00:00Z [INFO]  CLOSE step=restart reason="condition false, exiting loop"
2026-01-06T12:00:00Z [INFO]  POP molecule=achieve-001 times_looped=10 outcome="loop-complete"
```

### Debug Commands

```bash
# View current state
meow status --verbose

# Tail logs
tail -f .meow/logs/meow.log

# Attach to Claude session
tmux attach -t meow-impl-001

# List all MEOW tmux sessions
tmux ls | grep meow-

# Check what's on an agent's hook
meow prime --agent meow-impl-001

# View molecule with handoff notes
bd show impl-task-1-001

# Check exit signal
cat .meow/exit-signal.json
```

---

## Implementation Phases

### Phase 1: Foundation (Week 1)

- [ ] Project structure (`cmd/meow/`, `internal/`)
- [ ] Backend interface with hook operations
- [ ] Beads backend implementation
- [ ] Template parser
- [ ] Basic logging
- [ ] `meow init`

### Phase 2: Hook Model & Baking (Week 2)

- [ ] Agent ID generation
- [ ] Template baking with assignment
- [ ] Hook set/get/clear operations
- [ ] `meow prime` command
- [ ] Stack data structure
- [ ] State file management

### Phase 3: Session Management (Week 3)

- [ ] tmux session creation with `meow prime`
- [ ] Context detection from status line
- [ ] Stop hook script
- [ ] `meow check` command
- [ ] Exit signal handling
- [ ] Session monitoring

### Phase 4: Full Loop (Week 4)

- [ ] Orchestrator main loop
- [ ] Push handling (child molecules)
- [ ] Pop handling (resume parent)
- [ ] Handoff handling (fresh session)
- [ ] `meow done` command
- [ ] `meow handoff` command
- [ ] Gate handling
- [ ] `meow continue` recovery

### Phase 5: Execution Model (Week 5)

- [ ] Restart step handling with condition evaluation
- [ ] Session KILL on restart (fresh context)
- [ ] `times_looped` counter increment and storage
- [ ] Iteration digest creation before restart
- [ ] Conditional step handling with branch evaluation
- [ ] Branch routing and unreachable step marking
- [ ] Variable expansion in conditions ({{times_looped}}, etc.)
- [ ] Shell condition evaluation (test command)

### Phase 6: Polish (Week 6)

- [ ] Error handling throughout
- [ ] Crash recovery
- [ ] Built-in templates (implement, achieve-coverage, outer-loop)
- [ ] Comprehensive logging with iteration tracking
- [ ] Debug tooling (meow status --verbose with iteration history)
- [ ] Documentation

---

## Success Criteria

### MVP Complete When:

1. **Initialization**: `meow init` sets up project correctly
2. **Prime**: `meow prime` shows hooked work, times_looped, and iteration history
3. **Execution**: Claude works on steps, stop hook keeps it going via `meow prime`
4. **Completion**: `meow done` signals clean completion
5. **Nesting**: Template steps push child molecules with new agents
6. **Resumption**: Parent sessions resume after child completion
7. **Handoff**: Context limit triggers handoff, fresh Claude continues
8. **Gates**: `meow approve` closes gates and resumes execution
9. **Durability**: `meow continue` recovers from any crash state
10. **Observability**: Logs + tmux + context % visible
11. **Loops**: Restart steps with conditions KILL session, increment counter, spawn fresh
12. **Branching**: Conditional steps route to different paths based on conditions
13. **Loop Exit**: `times_looped` counter enables bounded iteration with exit conditions
14. **Iteration Digests**: Each loop iteration creates a digest before restart

### Test Workflow: Linear Template

```bash
# 1. Initialize
meow init

# 2. Create a task
bd create "Implement user registration" --type task

# 3. Run TDD workflow (linear - no loop)
meow run implement --var task_id=bd-xxx

# 4. Watch Claude work
tmux attach -t meow-impl-xxx

# 5. Claude runs meow prime, sees work, executes steps
# 6. If context high, Claude runs meow handoff
# 7. Fresh Claude spawns, runs meow prime, continues

# 8. When done, Claude runs meow done
# 9. Orchestrator pops stack, resumes parent

# 10. If gate reached
meow approve --notes "LGTM"

# 11. If crash, recover
meow continue

# 12. Check status anytime
meow status
```

### Test Workflow: Looping Template

```bash
# 1. Run a goal-oriented loop template
meow run achieve-coverage --var target_coverage=90 --var max_iterations=10

# 2. Watch the loop iterate
tmux attach -t meow-achieve-001

# 3. Observe in logs:
tail -f .meow/logs/meow.log
# 2026-01-06T10:00:00Z [INFO]  SPAWN agent=meow-achieve-001 iteration=0
# 2026-01-06T10:15:00Z [INFO]  RESTART molecule=achieve-001 times_looped=1 summary="Coverage at 65%"
# 2026-01-06T10:15:01Z [INFO]  SPAWN agent=meow-achieve-001 iteration=1 (fresh context)
# 2026-01-06T10:30:00Z [INFO]  RESTART molecule=achieve-001 times_looped=2 summary="Coverage at 78%"
# ...
# 2026-01-06T11:00:00Z [INFO]  BRANCH step=check-goal target=success (condition: coverage >= 90)
# 2026-01-06T11:00:01Z [INFO]  POP molecule=achieve-001 complete

# 4. Check iteration history
meow status --verbose
# ITERATION HISTORY:
#   [0] 10:00 → restart (coverage: 65%)
#   [1] 10:15 → restart (coverage: 78%)
#   [2] 10:30 → restart (coverage: 85%)
#   [3] 10:45 → branch-exit to "success" (coverage: 92%)

# 5. Verify each iteration had fresh context (no context accumulation)
```

---

## Appendix: Gastown Patterns Adopted

| Pattern | Gastown | MEOW |
|---------|---------|------|
| Hook model | `hook_bead` on agent | `HookBead` field, `meow prime` to check |
| Startup prompt | `gt prime` | `meow prime` |
| Completion signal | `gt done` | `meow done` |
| Handoff | `gt handoff` with pinned bead | `meow handoff` with molecule notes |
| Agent state | `agent_state` field | Same |
| Assignment | `assignee` field | Same |
| Work query | `bd list --assignee` | Same |
| Context detection | (various) | tmux status line parsing |

---

## Appendix: Claude CLI Reference

### Key Flags

| Flag | Usage | Purpose |
|------|-------|---------|
| `-p, --prompt` | `claude -p "meow prime"` | Non-interactive with prompt |
| `-r, --resume` | `claude -r <session-id>` | Resume specific session |
| `--output-format` | `--output-format json` | Get session_id in output |
| `--allowedTools` | `--allowedTools "Bash,Read,Edit"` | Auto-approve tools |

### Stop Hook API

**Input:**
```json
{
  "session_id": "uuid",
  "transcript_path": "~/.claude/.../session.jsonl",
  "stop_hook_active": false
}
```

**Output:**
```json
{
  "decision": "block",
  "reason": "meow prime",
  "systemMessage": "Continue working..."
}
```
