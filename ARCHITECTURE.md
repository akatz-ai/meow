# MEOW Stack Architecture

> **For implementers**: This document describes how all pieces fit together at MVP completion. Keep this mental model as you work on individual components.

## The One Sentence

MEOW is a dumb orchestrator that dispatches 6 primitive bead types while smart TOML templates define all workflow logic.

## Core Insight

```
┌──────────────────────────────────────────────────────────────────────┐
│  Orchestrator: Simple dispatch loop (recognizes 6 primitives)        │
│  Templates:    All workflow intelligence (loops, gates, handoffs)    │
│  Beads:        Single source of truth (git-backed, survives crashes) │
└──────────────────────────────────────────────────────────────────────┘
```

## The 6 Primitives

| Primitive   | Executor      | Purpose                                      |
|-------------|---------------|----------------------------------------------|
| `task`      | Claude        | Work unit — agent executes and closes        |
| `condition` | Orchestrator  | Branch/loop/wait — shell command (can block) |
| `stop`      | Orchestrator  | Kill agent's tmux session                    |
| `start`     | Orchestrator  | Spawn agent (with optional `--resume`)       |
| `code`      | Orchestrator  | Shell execution with output capture          |
| `expand`    | Orchestrator  | Inline template into beads                   |

**Everything else is template composition**: refresh, handoff, call/return, human gates, context management.

## Component Diagram

```
                    USER
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         meow CLI                                     │
│  init │ run │ prime │ close │ status │ approve │ agents │ ...       │
└───────────────────────────────┬─────────────────────────────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        ▼                       ▼                       ▼
┌───────────────┐    ┌───────────────────┐    ┌─────────────────┐
│   Template    │    │   Orchestrator    │    │  Agent Manager  │
│   System      │    │                   │    │                 │
│               │    │   Main Loop       │    │  tmux spawn     │
│  TOML parse   │◄───│   Primitive       │───►│  tmux stop      │
│  Variables    │    │   dispatch        │    │  session track  │
│  Baking       │    │   State persist   │    │  heartbeat      │
└───────────────┘    └─────────┬─────────┘    └─────────────────┘
                               │
        ┌──────────────────────┼──────────────────────┐
        ▼                      ▼                      ▼
┌───────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  BeadStore    │    │  CodeExecutor   │    │  Condition      │
│               │    │                 │    │  Evaluator      │
│  .beads/      │    │  Shell exec     │    │                 │
│  bd commands  │    │  Output capture │    │  Blocking exec  │
│  Readiness    │    │  Error handling │    │  Goroutines     │
└───────────────┘    └─────────────────┘    └─────────────────┘
```

## Data Flow

### 1. Workflow Start (`meow run <template>`)

```
Template TOML ──► Bake into Beads ──► Create initial beads ──► Enter main loop
                 (variable subst)     (in .beads/)
```

### 2. Main Loop (every 100ms)

```
┌─► GetNextReadyBead()
│        │
│        ├─► None + all done ──► Exit "complete"
│        │
│        ├─► None + waiting ──► Sleep, continue
│        │
│        └─► Found bead ──► Dispatch by type:
│                                │
│                                ├─► task      ──► Mark in_progress, wait for Claude
│                                ├─► condition ──► Spawn goroutine, eval shell, expand branch
│                                ├─► stop      ──► Kill tmux session, auto-close
│                                ├─► start     ──► Create tmux session, auto-close
│                                ├─► code      ──► Exec shell, capture outputs, auto-close
│                                └─► expand    ──► Load template, create child beads, auto-close
│
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

### 3. Task Execution (Claude side)

```
Claude receives task ◄── meow prime
Claude does work
Claude closes task   ──► meow close <id> --output key=value
                              │
                              ▼
                     Bead status → closed
                     Orchestrator sees on next tick
```

## Output Binding

Data flows between beads via output binding:

```yaml
# Code bead captures output
id: "create-worktree"
type: code
code: |
  git worktree add ~/wt/worker HEAD
  echo "~/wt/worker"
outputs:
  path: stdout                    # ◄── Captured

# Subsequent bead references it
id: "start-worker"
type: start
workdir: "{{create-worktree.outputs.path}}"  # ◄── Used
needs: ["create-worktree"]
```

Task beads can also produce validated outputs via `meow close --output key=value`.

## Agent Lifecycle

```
┌─────────────────────────────────────────────────────────────────────┐
│                        tmux session: meow-claude-1                   │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ $ claude --dangerously-skip-permissions                       │  │
│  │                                                               │  │
│  │ > meow prime                     ◄── Injected on start        │  │
│  │ [Claude sees task, executes]                                  │  │
│  │ > meow close bd-task-001                                      │  │
│  │ [Stop hook fires]                                             │  │
│  │ > meow prime                     ◄── Ralph Wiggum loop        │  │
│  │ [Continues...]                                                │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

**Resume**: `start` bead with `resume_session` spawns Claude with `--resume <session_id>`.

## State & Crash Recovery

```
.meow/
├── config.toml              # Configuration
├── state/
│   ├── orchestrator.json    # Current state (workflow_id, tick_count)
│   ├── orchestrator.lock    # Prevents concurrent instances
│   ├── heartbeat.json       # Health check (updated every 30s)
│   └── trace.jsonl          # Execution log
└── templates/               # User templates

.beads/                      # Source of truth (git-backed)
└── issues.jsonl             # All bead state
```

**On crash**: Restart `meow run` — orchestrator reloads state, validates tmux sessions, continues from last position. Beads never lost.

## Template Composition

Templates are the smart layer. Complex patterns are just primitives composed:

```
"call" pattern:
  1. code      ──► meow session-id (capture session)
  2. stop      ──► Kill parent
  3. start     ──► Spawn child
  4. expand    ──► Child's work
  5. stop      ──► Kill child
  6. start     ──► Resume parent (with session_id)

"human-gate" pattern:
  1. task      ──► Prepare summary
  2. condition ──► meow wait-approve --bead <id> (BLOCKS)
  3. ...       ──► Continue or handle rejection

"work-loop" pattern:
  1. condition ──► Any open tasks?
     on_true   ──► expand work-iteration
     on_false  ──► expand finalize
```

## Key Interfaces

```go
// BeadStore — access bead state
type BeadStore interface {
    GetNextReady(ctx) (*Bead, error)  // Next executable bead
    Get(ctx, id) (*Bead, error)       // By ID
    Update(ctx, bead) error           // Save changes
    AllDone(ctx) (bool, error)        // Completion check
}

// AgentManager — tmux lifecycle
type AgentManager interface {
    Start(ctx, *StartSpec) error      // Spawn in tmux
    Stop(ctx, *StopSpec) error        // Kill tmux session
    IsRunning(ctx, agentID) (bool, error)
}

// TemplateExpander — bake templates into beads
type TemplateExpander interface {
    Expand(ctx, *ExpandSpec, parentBead) error
}

// CodeExecutor — shell execution
type CodeExecutor interface {
    Execute(ctx, *CodeSpec) (outputs map[string]any, error)
}
```

## Bead Readiness Rules

A bead is "ready" when:
1. Status is `open`
2. All `needs` dependencies are `closed`
3. Assignee agent exists (or null for orchestrator beads)

**Priority**: orchestrator beads before task beads, then by creation time.

## Ephemeral Beads

Template steps can be marked `ephemeral: true`. These:
- Execute normally
- Get labeled `meow:ephemeral`
- Auto-cleaned after workflow completes
- Don't clutter `bd list` output

Use for workflow machinery (the "how"), not actual work (the "what").

## File Layout

```
meow-machine/
├── cmd/meow/                    # CLI entry point
│   └── cmd/                     # Cobra commands
├── internal/
│   ├── config/                  # Config loading
│   ├── types/                   # Bead, Agent, specs
│   ├── orchestrator/            # Main loop, state, bead store
│   ├── template/                # TOML parsing, baking
│   ├── agent/                   # tmux management
│   └── executor/                # Shell execution
└── templates/                   # Default templates (embedded)
```

## Key Principles

1. **Orchestrator is dumb** — 6 primitives, simple dispatch, no workflow opinions
2. **Templates are smart** — all patterns are user-defined composition
3. **Beads are truth** — survives crashes, git-backed, inspectable
4. **Conditions can block** — unifies gates, waits, loops, timers
5. **Output binding** — dynamic data flow between beads
6. **Crash recovery** — state persisted, resume where you left off

---

*When implementing: your piece should fit this puzzle. If unclear how, check the interfaces.*
