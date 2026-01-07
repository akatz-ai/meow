# MEOW Stack Architecture

> **For implementers**: This document describes how all pieces fit together at MVP completion. Keep this mental model as you work on individual components.

## The One Sentence

MEOW is a dumb orchestrator that dispatches 8 primitive bead types across three visibility tiers, while smart TOML module templates define all workflow logic.

## Core Insight

```
┌──────────────────────────────────────────────────────────────────────┐
│  Orchestrator: Simple dispatch loop (recognizes 8 primitives)        │
│  Templates:    All workflow intelligence (modules with workflows)    │
│  Beads:        Three-tiered truth (work, wisp, orchestrator)         │
│  Tiers:        Visibility control (agents see wisps, not machinery)  │
└──────────────────────────────────────────────────────────────────────┘
```

---

## Three-Tier Bead Model

**The key architectural insight**: Not all beads are equal. Agents should see their workflow steps (wisps), not infrastructure machinery. Work beads (deliverables) are separate from how those deliverables get implemented.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          BEAD VISIBILITY MODEL                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  TIER 1: WORK BEADS                     Tier field: "work"                  │
│  ─────────────────                                                          │
│  Purpose:     Actual deliverables from PRD breakdown                        │
│  Examples:    "Implement auth endpoint", "Fix login bug"                    │
│  Created by:  Humans, PRD breakdown, `bd create`                            │
│  Visible via: `bd ready`, `bv --robot-triage`, `bd list`                    │
│  Agent role:  SELECTS from these based on priority/impact                   │
│  Persistence: Permanent, exported to JSONL, git-synced                      │
│  ID prefix:   bd-*                                                          │
│                                                                             │
│  TIER 2: WORKFLOW BEADS (WISPS)         Tier field: "wisp"                  │
│  ──────────────────────────────                                             │
│  Purpose:     Procedural guidance - HOW to do work                          │
│  Examples:    "Load context", "Write tests", "Commit changes"               │
│  Created by:  Template expansion (ephemeral workflows)                      │
│  Visible via: `meow prime`, `meow steps`                                    │
│  Agent role:  EXECUTES step-by-step                                         │
│  Persistence: Durable in DB until burned, NOT exported to JSONL             │
│  ID prefix:   meow-*                                                        │
│                                                                             │
│  TIER 3: ORCHESTRATOR BEADS             Tier field: "orchestrator"          │
│  ──────────────────────────                                                 │
│  Purpose:     Workflow machinery - control flow, agent lifecycle            │
│  Examples:    "start-worker", "check-tests", "expand-implement"             │
│  Created by:  Template expansion (non-task primitives)                      │
│  Visible via: NEVER shown to agents, only debug views                       │
│  Agent role:  NONE - orchestrator handles exclusively                       │
│  Persistence: Durable in DB until burned, NOT exported to JSONL             │
│  ID prefix:   meow-*                                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### The Claude TODO Analogy

This maps to how Claude Code works:

| Claude Code | MEOW Stack |
|-------------|------------|
| Beads in `.beads/` | Work beads (persistent deliverables) |
| Claude's ephemeral TODO list | Wisps (procedural guidance, burned after use) |
| Internal processing | Orchestrator beads (invisible machinery) |

### Tier as Explicit Field

The tier is stored directly on the bead, not computed from labels:

```go
type BeadTier string

const (
    TierWork        BeadTier = "work"        // Permanent deliverables
    TierWisp        BeadTier = "wisp"        // Agent workflow steps
    TierOrchestrator BeadTier = "orchestrator" // Infrastructure machinery
)
```

Benefits: O(1) lookup, self-documenting, validated at creation time.

---

## The 8 Primitive Bead Types

| Primitive | Executor | Purpose | Auto-continue |
|-----------|----------|---------|---------------|
| `task` | Agent | Work unit — agent executes and closes | Yes (Ralph Wiggum loop) |
| `collaborative` | Agent | Interactive work — pauses for human conversation | No (conversation mode) |
| `gate` | Human | Approval checkpoint — no assignee, human closes | No (blocks workflow) |
| `condition` | Orchestrator | Branch/loop/wait — shell command (can block) | N/A |
| `stop` | Orchestrator | Kill agent's tmux session | N/A |
| `start` | Orchestrator | Spawn agent (with optional `--resume`) | N/A |
| `code` | Orchestrator | Shell execution with output capture | N/A |
| `expand` | Orchestrator | Inline template into beads | N/A |

### Agent-Executable Types

| Type | Assignee | Auto-continue | Who closes | Use case |
|------|----------|---------------|------------|----------|
| `task` | Required | Yes | Agent | Normal autonomous work |
| `collaborative` | Required | No | Agent | Design review, clarification, debugging |
| `gate` | None | No | Human | Approval checkpoints |

**The `collaborative` type** enables human-in-the-loop interaction. When an agent reaches a collaborative step:
1. Agent executes the step (presents information, asks questions)
2. Agent's stop hook fires, BUT...
3. `meow prime --format prompt` returns **empty output** for in-progress collaborative steps
4. No prompt injection → Claude waits naturally for user input
5. User and agent converse freely
6. When done, agent runs `meow close <step-id>`
7. Next stop hook fires, normal flow resumes

**The `gate` type** replaces the `orchestrator_task` pattern. Gates have no assignee — the orchestrator waits for a human to run `meow approve` or `meow reject`.

---

## Module System

Templates use a **module format** where files contain one or more named workflows.

### File Format

```toml
# work-module.meow.toml

# ═══════════════════════════════════════════════════════════════════════════
# MAIN WORKFLOW (Default Entry Point)
# ═══════════════════════════════════════════════════════════════════════════

[main]
name = "work-loop"
description = "Main work selection and execution loop"

[main.variables]
agent = { required = true, type = "string", description = "Agent ID" }

[[main.steps]]
id = "select-work"
type = "task"
title = "Select next work bead"
assignee = "{{agent}}"

[main.steps.outputs]
required = [
    { name = "work_bead", type = "bead_id", description = "The bead to implement" }
]

[[main.steps]]
id = "do-work"
type = "expand"
template = ".implement"   # ← Local reference (same file)
variables = { work_bead = "{{select-work.outputs.work_bead}}" }
needs = ["select-work"]


# ═══════════════════════════════════════════════════════════════════════════
# IMPLEMENT WORKFLOW (Helper - Called by main)
# ═══════════════════════════════════════════════════════════════════════════

[implement]
name = "implement"
description = "TDD implementation workflow"
ephemeral = true          # All steps from this workflow are wisps
internal = true           # Cannot be called from outside this file
hooks_to = "work_bead"    # Links all wisps to this variable's value

[implement.variables]
work_bead = { required = true, type = "bead_id" }

[[implement.steps]]
id = "load-context"
type = "task"
title = "Load context for {{work_bead}}"
assignee = "{{agent}}"

[[implement.steps]]
id = "write-tests"
type = "task"
title = "Write failing tests"
assignee = "{{agent}}"
needs = ["load-context"]
```

### The `main` Convention

```bash
meow run work-module.meow.toml           # Runs [main] workflow
meow run work-module.meow.toml#implement # Runs [implement] workflow explicitly
```

### Reference Syntax

| Reference | Resolution |
|-----------|------------|
| `.implement` | Same file, workflow named `implement` |
| `.implement.verify` | Same file, workflow `implement`, step `verify` (partial) |
| `helpers#tdd` | File `helpers.meow.toml`, workflow named `tdd` |
| `helpers` | File `helpers.meow.toml`, workflow `main` |
| `./subdir/util#helper` | Relative path |

### Workflow Properties

| Property | Type | Description |
|----------|------|-------------|
| `name` | string | Human-readable name (required) |
| `description` | string | Documentation |
| `ephemeral` | bool | All expanded beads are wisps (default: false) |
| `internal` | bool | Cannot be called from outside this file (default: false) |
| `hooks_to` | string | Variable name whose value becomes HookBead for all wisps |

### Legacy Format (Backward Compatible)

Single-workflow files with `[meta]` section are still supported:

```toml
[meta]
name = "simple-task"
description = "A simple task template"

[[steps]]
id = "do-work"
type = "task"
```

Detection: if file has `[meta]` section → legacy format. Otherwise → module format.

---

## Bead Structure

### Core Fields (Compatible with upstream beads)

```go
type Bead struct {
    ID          string     `json:"id"`
    Title       string     `json:"title"`
    Description string     `json:"description,omitempty"`
    Status      BeadStatus `json:"status"`           // open | in_progress | closed
    Assignee    string     `json:"assignee,omitempty"`
    Labels      []string   `json:"labels,omitempty"`
    Notes       string     `json:"notes,omitempty"`
    CreatedAt   time.Time  `json:"created_at"`
    ClosedAt    *time.Time `json:"closed_at,omitempty"`
    Needs       []string   `json:"needs,omitempty"`  // Dependency bead IDs
}
```

### MEOW-Specific Fields (ignored by upstream `bd`, used by `meow`)

```go
type Bead struct {
    // ... core fields above ...

    // Type and tier
    Type BeadType `json:"type"`                    // task | collaborative | gate | condition | ...
    Tier BeadTier `json:"tier,omitempty"`          // work | wisp | orchestrator

    // Workflow tracking
    HookBead       string `json:"hook_bead,omitempty"`        // Work bead this wisp implements
    SourceWorkflow string `json:"source_workflow,omitempty"`  // Template that created this
    WorkflowID     string `json:"workflow_id,omitempty"`      // Unique workflow instance ID

    // Captured outputs (set when bead closes)
    Outputs map[string]any `json:"outputs,omitempty"`

    // Task output definitions (what outputs are required/optional)
    TaskOutputs *TaskOutputSpec `json:"task_outputs,omitempty"`

    // Type-specific specs
    ConditionSpec *ConditionSpec `json:"condition_spec,omitempty"`
    StartSpec     *StartSpec     `json:"start_spec,omitempty"`
    StopSpec      *StopSpec      `json:"stop_spec,omitempty"`
    CodeSpec      *CodeSpec      `json:"code_spec,omitempty"`
    ExpandSpec    *ExpandSpec    `json:"expand_spec,omitempty"`
}
```

### Status Transitions

```
open → in_progress → closed
  ↑                    │
  └────────────────────┘  (reopen if needed)
```

Three statuses only. No `hooked` status — crash recovery checks if agent process is alive, not what phase they're in.

---

## Component Diagram

```
                    USER
                      │
                      ▼
┌─────────────────────────────────────────────────────────────────────┐
│                         meow CLI                                     │
│  init │ run │ prime │ close │ status │ approve │ agents │ steps     │
└───────────────────────────────┬─────────────────────────────────────┘
                                │
        ┌───────────────────────┼───────────────────────┐
        ▼                       ▼                       ▼
┌───────────────┐    ┌───────────────────┐    ┌─────────────────┐
│   Template    │    │   Orchestrator    │    │  Agent Manager  │
│   System      │    │                   │    │                 │
│               │    │   Main Loop       │    │  tmux spawn     │
│  Module parse │◄───│   Tier-aware      │───►│  tmux stop      │
│  Loader       │    │   dispatch        │    │  session track  │
│  Baker        │    │   State persist   │    │  heartbeat      │
└───────────────┘    └─────────┬─────────┘    └─────────────────┘
                               │
        ┌──────────────────────┼──────────────────────┐
        ▼                      ▼                      ▼
┌───────────────┐    ┌─────────────────┐    ┌─────────────────┐
│  BeadStore    │    │  CodeExecutor   │    │  Condition      │
│               │    │                 │    │  Evaluator      │
│  .beads/      │    │  Shell exec     │    │                 │
│  issues.jsonl │    │  Output capture │    │  Blocking exec  │
│  Tier filter  │    │  Error handling │    │  Goroutines     │
└───────────────┘    └─────────────────┘    └─────────────────┘
```

---

## Data Flow

### 1. Workflow Start (`meow run <module>#workflow`)

```
Module TOML ──► Parse workflows ──► Select workflow ──► Bake into beads ──► Enter loop
               (multi-workflow)     (main or #name)    (tier detection)
```

### 2. Main Loop (every 100ms)

```
┌─► GetNextReadyBead()
│        │
│        ├─► None + all done ──► Burn wisps ──► Exit "complete"
│        │
│        ├─► None + waiting ──► Sleep, continue
│        │
│        └─► Found bead ──► Dispatch by type:
│                                │
│                                ├─► task         ──► Mark in_progress, wait for agent
│                                ├─► collaborative──► Mark in_progress, wait (no auto-continue)
│                                ├─► gate         ──► Wait for human approval
│                                ├─► condition    ──► Spawn goroutine, eval, expand branch
│                                ├─► stop         ──► Kill tmux session, auto-close
│                                ├─► start        ──► Create tmux session, auto-close
│                                ├─► code         ──► Exec shell, capture outputs, auto-close
│                                └─► expand       ──► Bake template, create child beads, auto-close
│
└─────────────────────────────────────────────────────────────────────────────────────────┘
```

**Tier-aware dispatch priority**: orchestrator beads before wisps before work beads.

### 3. Agent Sees Work (`meow prime`)

```
$ meow prime

═══════════════════════════════════════════════════════════════
Your workflow: implement-tdd (step 2/4)
Work bead: bd-123 "Implement auth endpoint"
═══════════════════════════════════════════════════════════════

  ✓ load-context
  → write-tests [in_progress] ← YOU ARE HERE
  ○ implement
  ○ commit

───────────────────────────────────────────────────────────────
Instructions:
  Write failing tests that define the expected behavior.
  Tests MUST fail at this point.

Required outputs: (none)
───────────────────────────────────────────────────────────────
```

**Key behaviors**:
- Shows only **wisps** assigned to this agent (not orchestrator beads)
- Shows **workflow progression** (step 2/4)
- Shows linked **work bead** (via `HookBead` field)
- For **collaborative** steps in_progress: returns **empty output** to disable auto-continuation

---

## Output Binding

Data flows between beads via output binding.

### Code Bead Outputs

```yaml
id: "create-worktree"
type: code
code: |
  git worktree add ~/wt/worker HEAD
  echo "~/wt/worker"
outputs:
  path: stdout                    # ◄── Captured

id: "start-worker"
type: start
workdir: "{{create-worktree.outputs.path}}"  # ◄── Referenced
needs: ["create-worktree"]
```

### Task Outputs with Validation

Tasks can require validated outputs:

```yaml
id: "select-task"
type: task
title: "Select next task to work on"
outputs:
  required:
    - name: "work_bead"
      type: "bead_id"           # ◄── Validates bead exists!
      description: "The bead to implement"
    - name: "rationale"
      type: "string"
```

Agent closes with outputs:
```bash
meow close select-task \
  --output work_bead=bd-task-123 \
  --output rationale="Highest priority, unblocks 3 others"
```

If required outputs missing or invalid, close fails:
```
Error: Cannot close select-task - missing required outputs
  ✗ work_bead (bead_id): Not provided
```

### Output Types

| Type | Validation |
|------|------------|
| `string` | Non-empty string |
| `string[]` | Array of strings |
| `number` | Parseable as number |
| `boolean` | `true` or `false` |
| `json` | Valid JSON |
| `bead_id` | **Validates bead exists** |
| `bead_id[]` | Array of valid bead IDs |
| `file_path` | File exists |

The `bead_id` type is special — it catches typos and hallucinated bead IDs.

---

## Wisp Lifecycle

### Detection

When a template is baked, the baker automatically determines each bead's tier:

```go
func (b *Baker) determineTier(step *Step, workflow *Workflow) BeadTier {
    switch step.Type {
    case "task", "collaborative":
        if workflow.Ephemeral {
            return TierWisp
        }
        return TierWork
    case "gate":
        return TierOrchestrator  // Gates are orchestrator-tracked
    default:
        return TierOrchestrator  // condition, code, start, stop, expand
    }
}
```

**Key insight**: if a workflow is marked `ephemeral = true`, all its tasks become wisps.

### HookBead Linking

The `hooks_to` property explicitly links wisps to the work bead they implement:

```toml
[implement]
hooks_to = "work_bead"  # All wisps link to {{work_bead}}
```

When the baker creates wisps:
```go
bead.HookBead = vars["work_bead"]  // e.g., "bd-123"
```

### Burning (Cleanup)

Wisps are **durable in DB** until explicitly burned. Burning happens:
- On **workflow completion** (all beads in workflow are closed)
- Via `meow clean --ephemeral` (manual)
- Per config: `[cleanup] ephemeral = "on_complete" | "manual" | "never"`

**NOT** on agent stop (agent might resume).

### Visibility

| Command | Shows |
|---------|-------|
| `meow prime` | Current wisp step for this agent |
| `meow steps` | All wisp steps in current workflow |
| `meow status` | Workflow overview (wisps + orchestrator for debug) |
| `bd ready` | Work beads only |
| `bd list` | Work beads only (wisps hidden by default) |
| `bd list --include-ephemeral` | All beads |

---

## Agent Lifecycle

```
┌─────────────────────────────────────────────────────────────────────┐
│                        tmux session: meow-claude-1                   │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ $ claude --dangerously-skip-permissions                       │  │
│  │                                                               │  │
│  │ > meow prime                     ◄── Injected on start        │  │
│  │ [Claude sees wisp, executes]                                  │  │
│  │ > meow close meow-abc.write-tests                             │  │
│  │ [Stop hook fires]                                             │  │
│  │ > meow prime                     ◄── Ralph Wiggum loop        │  │
│  │ [Next wisp...]                                                │  │
│  └───────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

### Stop Hook for Auto-Continuation

```bash
#!/bin/bash
# .claude/hooks/stop-hook.sh

output=$(meow prime --format prompt 2>/dev/null)

if [ -z "$output" ]; then
    # Empty = collaborative mode OR no more work
    exit 0  # Don't inject anything → Claude waits for user
fi

echo "$output"  # Normal task → inject next prompt
```

### Resume

`start` bead with `resume_session` spawns Claude with `--resume <session_id>`:

```yaml
id: "resume-parent"
type: start
agent: "claude-1"
resume_session: "{{save-session.outputs.session_id}}"
```

---

## Beads Integration (Overlay Approach)

MEOW **layers on top** of upstream `beads` rather than forking:

```
┌─────────────────────────────────────────────────────────────────────┐
│                    .beads/issues.jsonl                              │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │ {"id":"bd-task-123","title":"Implement auth",...}              │ │
│  │ {"id":"meow-abc.load-context","tier":"wisp","hook_bead":...}   │ │
│  │ {"id":"meow-abc.start-worker","tier":"orchestrator",...}       │ │
│  │ {"id":"bd-task-456","title":"Fix bug",...}                     │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                    │                     │                          │
│                    ▼                     ▼                          │
│  ┌────────────────────────┐   ┌────────────────────────┐           │
│  │     bd CLI (upstream)  │   │     meow CLI           │           │
│  ├────────────────────────┤   ├────────────────────────┤           │
│  │ • bd ready             │   │ • meow prime           │           │
│  │ • bd list              │   │ • meow close           │           │
│  │ • bd create            │   │ • meow run             │           │
│  │ • bd close             │   │ • meow status          │           │
│  ├────────────────────────┤   ├────────────────────────┤           │
│  │ Sees: bd-* beads       │   │ Sees: All beads        │           │
│  │ Ignores: MEOW fields   │   │ Understands: Tiers,    │           │
│  │ (tier, hook_bead, etc) │   │ specs, workflows       │           │
│  └────────────────────────┘   └────────────────────────┘           │
└─────────────────────────────────────────────────────────────────────┘
```

**ID prefix separation**:
- `bd-*` — Work beads (created by `bd create`, humans)
- `meow-*` — Workflow beads (created by template expansion)

**Why not fork?** JSON is schema-flexible. MEOW extends fields without breaking upstream.

---

## Key Interfaces

```go
// BeadStore — tier-aware bead access
type BeadStore interface {
    GetNextReady(ctx context.Context) (*Bead, error)  // Tier-prioritized
    Get(ctx context.Context, id string) (*Bead, error)
    Update(ctx context.Context, bead *Bead) error
    Create(ctx context.Context, bead *Bead) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, filter BeadFilter) ([]*Bead, error)
    AllDone(ctx context.Context) (bool, error)
}

// BeadFilter for tier-aware queries
type BeadFilter struct {
    Tier     BeadTier   // Filter by tier
    Status   BeadStatus // Filter by status
    Assignee string     // Filter by agent
    WorkflowID string   // Filter by workflow
}

// AgentManager — tmux lifecycle
type AgentManager interface {
    Start(ctx context.Context, spec *StartSpec) error
    Stop(ctx context.Context, spec *StopSpec) error
    IsRunning(ctx context.Context, agentID string) (bool, error)
}

// ModuleLoader — load and resolve template references
type ModuleLoader interface {
    Load(ctx context.Context, ref string) (*Module, *Workflow, error)
    ResolveLocal(ctx context.Context, module *Module, name string) (*Workflow, error)
}

// Baker — transform workflows into beads with tier detection
type Baker interface {
    Bake(ctx context.Context, workflow *Workflow, vars map[string]string) (*BakeResult, error)
}

// CodeExecutor — shell execution
type CodeExecutor interface {
    Execute(ctx context.Context, spec *CodeSpec) (map[string]any, error)
}
```

---

## Template Composition Patterns

Templates are the smart layer. Complex patterns are primitives composed:

### Call Pattern (Parent/Child Orchestration)

```
1. code      ──► meow session-id (capture parent session)
2. stop      ──► Kill parent
3. code      ──► git worktree add (capture path)
4. start     ──► Spawn child in worktree
5. expand    ──► Child's work (wisps)
6. stop      ──► Kill child
7. code      ──► git worktree remove (cleanup)
8. start     ──► Resume parent (with session_id)
```

### Human Gate Pattern

```
1. task      ──► Prepare summary for review
2. gate      ──► Human approval (BLOCKS until meow approve)
3. ...       ──► Continue or handle rejection
```

### Work Loop Pattern

```
1. condition ──► Any open tasks? (bd list --status=open)
   on_true   ──► task: select work (requires work_bead output)
                 expand: .implement (with work_bead)
                 expand: .work-loop (recursive)
   on_false  ──► task: write summary
                 stop: agent
```

---

## State & Crash Recovery

```
.meow/
├── config.toml              # Configuration
├── state/
│   ├── orchestrator.json    # Current state (workflow_id, tick_count)
│   ├── orchestrator.lock    # Prevents concurrent instances
│   ├── heartbeat.json       # Health check (updated every 30s)
│   └── trace.jsonl          # Execution log
└── templates/               # User templates (module format)

.beads/                      # Source of truth (git-backed)
└── issues.jsonl             # All beads (work + wisp + orchestrator)
```

**On crash**: Restart `meow run` — orchestrator reloads state, validates tmux sessions, resets in-progress beads from dead agents to open, continues. All beads (including wisps) are durable.

---

## File Layout

```
meow-machine/
├── cmd/meow/                    # CLI entry point
│   └── cmd/                     # Cobra commands
├── internal/
│   ├── config/                  # Config loading
│   ├── types/                   # Bead (with Tier, HookBead), Agent, specs
│   ├── orchestrator/            # Main loop, tier-aware dispatch, state
│   ├── template/
│   │   ├── parser.go            # Module format parsing
│   │   ├── loader.go            # Reference resolution
│   │   └── baker.go             # Tier detection, wisp creation
│   ├── agent/                   # tmux management
│   ├── executor/                # Shell execution
│   └── validation/              # Output type validators (bead_id, etc.)
└── templates/                   # Default templates (embedded)
```

---

## Key Principles

1. **Orchestrator is dumb** — 8 primitives, simple tier-aware dispatch
2. **Templates are smart** — modules with workflows, all patterns user-defined
3. **Beads are truth** — three tiers, all durable, git-backed, inspectable
4. **Tiers control visibility** — agents see wisps, not machinery
5. **Wisps are TODO lists** — procedural guidance, burned after use
6. **Output binding** — dynamic data flow, validated task outputs
7. **Conditions can block** — unifies gates, waits, loops, timers
8. **Crash recovery** — all state persisted, resume where you left off

---

## Quick Reference: Bead Types by Tier

| Type | Typical Tier | Assignee | Closes |
|------|--------------|----------|--------|
| `task` | work or wisp | Agent | Agent |
| `collaborative` | wisp | Agent | Agent (pauses auto-continue) |
| `gate` | orchestrator | None | Human |
| `condition` | orchestrator | None | Orchestrator |
| `start` | orchestrator | None | Orchestrator |
| `stop` | orchestrator | None | Orchestrator |
| `code` | orchestrator | None | Orchestrator |
| `expand` | orchestrator | None | Orchestrator |

---

*When implementing: your piece should fit this puzzle. Check the tier model, the module system, and the interfaces. If a bead has `tier: "wisp"`, it's agent-visible workflow guidance. If `tier: "orchestrator"`, agents never see it.*
