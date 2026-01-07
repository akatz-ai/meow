# MEOW Stack MVP Specification

This document specifies the minimal viable implementation of MEOW Stack, based on a **primitives-first architecture** where a small set of orthogonal bead types enables arbitrary workflow composition.

> **Design Principle**: The orchestrator is dumb; the templates are smart. All workflow complexity lives in composable templates, not hardcoded orchestrator logic.

## Table of Contents

1. [Design Philosophy](#design-philosophy)
2. [The 8 Primitive Bead Types](#the-8-primitive-bead-types)
3. [Orchestrator Architecture](#orchestrator-architecture)
4. [Template System](#template-system)
5. [Agent Management](#agent-management)
6. [Execution Model](#execution-model)
7. [Composable Templates](#composable-templates)
8. [Complete Execution Trace](#complete-execution-trace)
9. [CLI Commands](#cli-commands)
10. [State Management](#state-management)
11. [Future: Parallelization](#future-parallelization)
12. [Implementation Phases](#implementation-phases)

---

## Design Philosophy

### Primitives Over Special Cases

MEOW Stack is built on a fundamental principle: **minimize orchestrator complexity by maximizing template expressiveness**. Instead of the orchestrator having special knowledge of:

- Context thresholds and automatic handoff
- Loop semantics and iteration counters
- Clone vs fresh spawn modes
- Worktree management

We provide **8 primitive bead types** that users compose into templates. The orchestrator is a simple state machine that recognizes these primitives and executes them. Everything else—including context management, refresh logic, call/return semantics, worktree handling, and even human approval gates—is **user-defined template composition**.

### Benefits

| Benefit | How It's Achieved |
|---------|-------------------|
| **Maximum user control** | Users define exactly what happens via templates |
| **No magic** | Orchestrator behavior is trivially understandable |
| **Debuggable** | All state is beads, all actions are explicit |
| **Composable** | Templates can include templates via `expand` |
| **Parallelizable** | Multiple agents via bead assignment (future) |
| **Extensible** | New patterns = new templates, not orchestrator changes |

### The Key Insight

Instead of:
```
orchestrator.handleRefresh()  // Complex, opinionated
orchestrator.handleCall()     // Complex, opinionated
orchestrator.handleLoop()     // Complex, opinionated
```

We have:
```
orchestrator.handlePrimitive(bead)  // Simple dispatch
```

And `refresh`, `call`, `loop` are just templates that compose primitives.

---

## The 8 Primitive Bead Types

The orchestrator recognizes exactly 8 bead types. Everything else is built from these.

### Overview

| Primitive | Executor | Description |
|-----------|----------|-------------|
| `task` | Claude | Regular work bead—Claude executes and closes |
| `condition` | Orchestrator | Evaluate shell command (can block!), expand on_true or on_false |
| `stop` | Orchestrator | Kill an agent's tmux session |
| `start` | Orchestrator | Spawn an agent in tmux with `meow prime` |
| `checkpoint` | Orchestrator | Save agent's session for later resume |
| `resume` | Orchestrator | Resume agent from saved checkpoint |
| `code` | Orchestrator | Execute arbitrary shell code |
| `expand` | Orchestrator | Expand a template into beads at this point |

### Detailed Specifications

#### `task` — Claude-Executed Work

The default bead type. Claude receives it via `meow prime`, executes the work, and closes it.

```yaml
id: "bd-abc123"
type: task
title: "Implement user registration endpoint"
description: "Create POST /api/register with validation"
assignee: "claude-1"
status: open
```

**Orchestrator behavior**: Ensure assigned agent is running, wait for Claude to close the bead.

#### `condition` — Branching, Looping, and Waiting

Evaluate a shell command. If exit code is 0 (true), expand `on_true` template. Otherwise expand `on_false` template.

**Key insight**: Shell commands can block. The orchestrator doesn't care if a condition takes 1ms or 1 hour to evaluate. This means `condition` subsumes what other systems call "gates" or "waits"—human approval, API polling, timers, webhooks—are all just blocking shell commands.

```yaml
# Simple branch
id: "bd-cond-001"
type: condition
condition: "bd list --type=task --status=open | grep -q ."
on_true:
  template: "work-iteration"
on_false:
  template: "finalize"
```

```yaml
# Human approval gate (blocking)
id: "bd-approve-001"
type: condition
condition: "meow wait-approve --bead {{bead_id}} --timeout 24h"
on_true:
  inline: []  # Approved, continue
on_false:
  template: "handle-rejection"
```

```yaml
# Wait for CI (blocking)
id: "bd-ci-001"
type: condition
condition: "gh run watch $RUN_ID --exit-status"
on_true:
  inline: []  # CI passed
on_false:
  template: "handle-ci-failure"
```

```yaml
# Timer/cooldown (blocking)
id: "bd-wait-001"
type: condition
condition: "sleep 300 && exit 0"
on_true:
  inline: []  # Continue after 5 minutes
on_false:
  inline: []  # Never reached
```

**Orchestrator behavior**:
1. Pause the current agent (don't advance it)
2. Execute `condition` as shell command (may block indefinitely)
3. Based on exit code, expand appropriate template
4. Auto-close this bead
5. Continue (agent will see newly expanded beads)

**Use cases**: Loops, branching, context checks, goal-oriented workflows, human approval gates, API waits, timers, webhooks.

**Helper CLIs for common waits**:
| Command | Behavior |
|---------|----------|
| `meow wait-approve --bead <id>` | Block until `meow approve <id>` is run |
| `meow wait-file <path>` | Block until file exists |
| `meow wait-api <url> --until "jq expr"` | Poll API until condition met |

These are conveniences—any blocking shell command works.

#### `stop` — Kill Agent Session

Terminate an agent's tmux session.

```yaml
id: "bd-stop-001"
type: stop
agent: "claude-1"
```

**Orchestrator behavior**:
1. Kill tmux session for specified agent
2. Update agent state to "stopped"
3. Auto-close this bead

**Use cases**: End of workflow, before refresh, cleanup.

#### `start` — Spawn Agent

Start an agent in a new tmux session, running `meow prime` as the initial prompt.

```yaml
id: "bd-start-001"
type: start
agent: "claude-2"
workdir: "/path/to/worktree"  # Optional, defaults to current
```

**Orchestrator behavior**:
1. Create tmux session named `meow-{agent}`
2. Start Claude with `meow prime` as prompt
3. Update agent state to "active"
4. Auto-close this bead

**Use cases**: Initial startup, spawning child agents, refresh.

#### `checkpoint` — Save for Resume

Save an agent's Claude session checkpoint for later `resume`.

```yaml
id: "bd-ckpt-001"
type: checkpoint
agent: "claude-1"
```

**Orchestrator behavior**:
1. Capture Claude's session/conversation ID
2. Store in agent state for later resume
3. Auto-close this bead

**Note**: This does NOT stop the agent. Pair with `stop` if needed.

**Use cases**: Before spawning child (for later resume), before potential failure.

#### `resume` — Resume from Checkpoint

Resume an agent from a previously saved checkpoint using `claude --resume`.

```yaml
id: "bd-resume-001"
type: resume
agent: "claude-1"
```

**Orchestrator behavior**:
1. Retrieve saved checkpoint from agent state
2. Start Claude with `claude --resume {checkpoint}` plus `meow prime`
3. Update agent state to "active"
4. Auto-close this bead

**Use cases**: After child completes, resuming parent context.

#### `code` — Arbitrary Shell Execution

Execute arbitrary shell code. This is the escape hatch for user-defined orchestrator actions.

```yaml
id: "bd-code-001"
type: code
code: |
  git worktree add -b meow/claude-2 ~/worktrees/claude-2 HEAD
  echo "Worktree created"
```

**Orchestrator behavior**:
1. Execute `code` as shell script
2. Capture output (for logging/debugging)
3. If exit code != 0, handle as error
4. Auto-close this bead

**Use cases**: Worktree management, environment setup, git operations, notifications, any custom action.

#### `expand` — Template Slot

Expand a template into beads at this point in the workflow. This is the **composition mechanism**.

```yaml
id: "bd-expand-001"
type: expand
template: "implement-tdd"
variables:
  task_id: "bd-task-123"
assignee: "claude-2"  # Assign expanded beads to this agent
```

**Orchestrator behavior**:
1. Load template from `.meow/templates/{template}.toml`
2. Substitute variables
3. Create beads for each step, inserted after this bead
4. Set `assignee` on created beads (if specified)
5. Auto-close this bead

**Use cases**: Template composition, call semantics, slot filling.

---

## Orchestrator Architecture

### Core Design

The orchestrator is a **simple event loop** that:
1. Finds the next ready bead (any agent)
2. Checks its type
3. Dispatches to the appropriate handler
4. Repeats

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           MEOW ORCHESTRATOR                                      │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ┌────────────────────────────────────────────────────────────────────────────┐ │
│  │                           MAIN LOOP                                        │ │
│  │                                                                            │ │
│  │  while true:                                                               │ │
│  │      bead = getNextReadyBead()    // Any agent, any ready bead             │ │
│  │                                                                            │ │
│  │      if bead is None:                                                      │ │
│  │          if allDone(): return     // No open beads, no active agents       │ │
│  │          sleep(100ms)             // Waiting on condition or agent         │ │
│  │          continue                                                          │ │
│  │                                                                            │ │
│  │      switch bead.type:                                                     │ │
│  │          case "task":       waitForClaudeToClose(bead)                     │ │
│  │          case "condition":  evalAndExpand(bead)  // may block!             │ │
│  │          case "stop":       killAgent(bead.agent); close(bead)             │ │
│  │          case "start":      spawnAgent(bead.agent); close(bead)            │ │
│  │          case "checkpoint": saveCheckpoint(bead.agent); close(bead)        │ │
│  │          case "resume":     resumeAgent(bead.agent); close(bead)           │ │
│  │          case "code":       execShell(bead.code); close(bead)              │ │
│  │          case "expand":     expandTemplate(bead); close(bead)              │ │
│  │                                                                            │ │
│  └────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
│  ┌────────────────────────────────────────────────────────────────────────────┐ │
│  │                         AGENT STATE                                        │ │
│  │                                                                            │ │
│  │  agents: map[string]*AgentState                                            │ │
│  │                                                                            │ │
│  │  AgentState {                                                              │ │
│  │      ID:          string          // e.g., "claude-1"                      │ │
│  │      TmuxSession: string          // e.g., "meow-claude-1"                 │ │
│  │      Status:      string          // active | stopped | checkpointed       │ │
│  │      Checkpoint:  string          // Session ID for resume                 │ │
│  │      Workdir:     string          // Current working directory             │ │
│  │  }                                                                         │ │
│  │                                                                            │ │
│  └────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Bead Readiness

A bead is "ready" when:
1. Status is `open`
2. All beads in its `needs` array are `closed`
3. Its `assignee` agent exists (or assignee is null for orchestrator beads)

### How Claude Closes Beads

Claude uses `meow close <bead-id>` (or `bd close` with MEOW integration) to close a task bead. This:
1. Updates bead status to `closed`
2. Triggers orchestrator to check what's next

The orchestrator can detect this via:
- File watch on `.beads/issues.jsonl`
- Polling bead status
- Claude calling `meow signal` explicitly

---

## Template System

### Template Format

Templates are TOML files in `.meow/templates/`:

```toml
[meta]
name = "implement-tdd"
description = "TDD implementation workflow"
version = "1.0.0"

[variables]
task_id = { required = true, description = "The task bead ID to implement" }

[[steps]]
id = "load-context"
type = "task"
title = "Load relevant files and understand the task"
description = "Read {{task_id}} and identify relevant source files"

[[steps]]
id = "write-tests"
type = "task"
title = "Write failing tests"
needs = ["load-context"]

[[steps]]
id = "implement"
type = "task"
title = "Implement code to make tests pass"
needs = ["write-tests"]

[[steps]]
id = "validate"
type = "task"
title = "Run tests and verify they pass"
needs = ["implement"]

[[steps]]
id = "commit"
type = "task"
title = "Commit changes"
needs = ["validate"]
```

### Variable Substitution

Templates support `{{variable}}` syntax:

```toml
description = "Implement task {{task_id}}"
condition = "test {{threshold}} -gt 60"
code = "git checkout {{branch}}"
```

Variables come from:
1. Template `[variables]` defaults
2. `expand` bead's `variables` field
3. Built-in variables (see below)

### Built-in Variables

| Variable | Description |
|----------|-------------|
| `{{agent}}` | Current agent ID |
| `{{bead_id}}` | Current bead ID |
| `{{workdir}}` | Current working directory |
| `{{timestamp}}` | Current ISO timestamp |

### Template Baking

When `meow run <template>` is called or an `expand` bead is processed:

1. Load template TOML
2. Validate required variables are provided
3. Substitute all `{{variable}}` references
4. Create bead for each step
5. Set up `needs` dependencies (translated to bead IDs)
6. Insert beads into storage

---

## Agent Management

### Agent Lifecycle

```
            ┌──────────────────────────────────────────────────────────────────┐
            │                    AGENT LIFECYCLE                               │
            ├──────────────────────────────────────────────────────────────────┤
            │                                                                  │
            │      (not exist)                                                 │
            │           │                                                      │
            │           │ start                                                │
            │           ▼                                                      │
            │      ┌─────────┐                                                 │
            │      │ ACTIVE  │◀──────────────────────────┐                     │
            │      └────┬────┘                           │                     │
            │           │                                │                     │
            │      ┌────┴────┐                           │                     │
            │      │         │                           │                     │
            │      │ stop    │ checkpoint                │ resume              │
            │      ▼         ▼                           │                     │
            │  ┌─────────┐  ┌──────────────┐             │                     │
            │  │ STOPPED │  │ CHECKPOINTED │─────────────┘                     │
            │  └─────────┘  └──────────────┘                                   │
            │      │                                                           │
            │      │ start (fresh)                                             │
            │      ▼                                                           │
            │  ┌─────────┐                                                     │
            │  │ ACTIVE  │  (new session, no context)                          │
            │  └─────────┘                                                     │
            │                                                                  │
            └──────────────────────────────────────────────────────────────────┘
```

### Agent State Storage

Agent state is stored in `.meow/agents.json`:

```json
{
  "claude-1": {
    "id": "claude-1",
    "tmux_session": "meow-claude-1",
    "status": "checkpointed",
    "checkpoint": "conv_abc123",
    "workdir": "/data/projects/myapp"
  },
  "claude-2": {
    "id": "claude-2",
    "tmux_session": "meow-claude-2",
    "status": "active",
    "checkpoint": null,
    "workdir": "/data/projects/myapp/worktrees/claude-2"
  }
}
```

### Bead Assignment

Every bead has an optional `assignee` field:

```yaml
assignee: "claude-1"  # This bead is for claude-1
assignee: null        # Orchestrator handles (for primitives)
```

`meow prime` shows only beads assigned to the calling agent:

```bash
# Claude-1 runs this, sees only its beads
meow prime
```

---

## Execution Model

### How Claude Interacts with MEOW

1. **Startup**: Claude runs `meow prime` to see its current work
2. **Execution**: Claude works on the task
3. **Completion**: Claude runs `meow close <bead-id>`
4. **Loop**: `meow prime` shows next task (if any)

The stop-hook (Ralph Wiggum pattern) can inject `meow prime` automatically:

```bash
# .claude/hooks/stop-hook.sh
meow prime --format prompt
```

### Orchestrator-Claude Coordination

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                    ORCHESTRATOR-CLAUDE COORDINATION                              │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  ORCHESTRATOR                           CLAUDE (in tmux)                         │
│  ─────────────                          ────────────────                         │
│                                                                                  │
│  1. getNextReadyBead()                                                           │
│     → returns task bead                                                          │
│     → bead.assignee = "claude-1"                                                 │
│                                                                                  │
│  2. ensureAgentRunning("claude-1")                                               │
│     → checks tmux session exists                                                 │
│     → if not, error (should have been started)                                   │
│                                                                                  │
│  3. waitForBeadClose(bead)              4. (Claude working...)                   │
│     → poll bead.status                     - reads task via `meow prime`         │
│     → or watch file                        - does the work                       │
│                                            - runs `meow close bd-xyz`            │
│                                                                                  │
│  5. bead.status == closed!                                                       │
│     → loop continues                                                             │
│     → getNextReadyBead()                                                         │
│                                                                                  │
│  6. Next bead is "condition"                                                     │
│     → orchestrator handles it                                                    │
│     → Claude not involved                                                        │
│                                                                                  │
│  7. After condition expands beads:                                               │
│     → getNextReadyBead()                                                         │
│     → returns next task                  8. (Claude's stop-hook fires)           │
│                                            - `meow prime` shows new task         │
│                                            - Claude continues                    │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## Composable Templates

The power of MEOW comes from composing primitives into reusable templates.

### Example: `refresh` Template

Kill current agent and start fresh (no context preservation):

```toml
# .meow/templates/refresh.toml
[meta]
name = "refresh"
description = "Kill agent and start fresh with clean context"

[variables]
agent = { required = true }

[[steps]]
id = "stop"
type = "stop"
agent = "{{agent}}"

[[steps]]
id = "start"
type = "start"
agent = "{{agent}}"
needs = ["stop"]
```

### Example: `handoff` Template

Write notes, then refresh:

```toml
# .meow/templates/handoff.toml
[meta]
name = "handoff"
description = "Write handoff notes and refresh agent"

[variables]
agent = { required = true }

[[steps]]
id = "write-notes"
type = "task"
title = "Write handoff notes"
description = "Document progress, decisions, and next steps in the bead notes"
assignee = "{{agent}}"

[[steps]]
id = "stop"
type = "stop"
agent = "{{agent}}"
needs = ["write-notes"]

[[steps]]
id = "start"
type = "start"
agent = "{{agent}}"
needs = ["stop"]
```

### Example: `call` Template

The full call semantics—checkpoint parent, spawn child, run template, cleanup, resume:

```toml
# .meow/templates/call.toml
[meta]
name = "call"
description = "Spawn child agent for sub-workflow, then resume parent"

[variables]
parent = { required = true, description = "Parent agent ID" }
child = { required = true, description = "Child agent ID" }
template = { required = true, description = "Template for child to execute" }
template_vars = { default = {}, description = "Variables for child template" }
use_worktree = { default = false }

# --- Pause parent ---
[[steps]]
id = "checkpoint-parent"
type = "checkpoint"
agent = "{{parent}}"

[[steps]]
id = "stop-parent"
type = "stop"
agent = "{{parent}}"
needs = ["checkpoint-parent"]

# --- Setup child environment (optional worktree) ---
[[steps]]
id = "setup-worktree"
type = "code"
needs = ["stop-parent"]
code = """
if [ "{{use_worktree}}" = "true" ]; then
    git worktree add -b meow/{{child}} ~/worktrees/{{child}} HEAD
    echo "~/worktrees/{{child}}" > /tmp/meow-{{child}}-workdir
else
    pwd > /tmp/meow-{{child}}-workdir
fi
"""

# --- Start child ---
[[steps]]
id = "start-child"
type = "start"
agent = "{{child}}"
needs = ["setup-worktree"]
# workdir is read from the temp file by orchestrator, or we enhance start

# --- Child does the work ---
[[steps]]
id = "child-work"
type = "expand"
template = "{{template}}"
variables = "{{template_vars}}"
assignee = "{{child}}"
needs = ["start-child"]

# --- Stop child ---
[[steps]]
id = "stop-child"
type = "stop"
agent = "{{child}}"
needs = ["child-work"]

# --- Cleanup worktree ---
[[steps]]
id = "cleanup-worktree"
type = "code"
needs = ["stop-child"]
code = """
if [ "{{use_worktree}}" = "true" ]; then
    git worktree remove ~/worktrees/{{child}} || true
    git branch -d meow/{{child}} || true
fi
"""

# --- Resume parent ---
[[steps]]
id = "resume-parent"
type = "resume"
agent = "{{parent}}"
needs = ["cleanup-worktree"]
```

### Example: `human-gate` Template

Human approval gate using blocking condition:

```toml
# .meow/templates/human-gate.toml
[meta]
name = "human-gate"
description = "Wait for human approval before continuing"

[variables]
bead_id = { required = true }
timeout = { default = "24h" }

[[steps]]
id = "prepare-summary"
type = "task"
title = "Prepare work summary for review"
description = "Document what was done and what approval is needed"
assignee = "{{agent}}"

[[steps]]
id = "await-approval"
type = "condition"
needs = ["prepare-summary"]
condition = "meow wait-approve --bead {{bead_id}} --timeout {{timeout}}"
on_true:
  inline = []  # Approved, continue workflow
on_false:
  inline = [
    { id = "handle-rejection", type = "task",
      title = "Address rejection feedback and retry" }
  ]
```

### Example: `context-check` Template

User-defined context management:

```toml
# .meow/templates/context-check.toml
[meta]
name = "context-check"
description = "Check context usage and refresh if above threshold"

[variables]
agent = { required = true }
threshold = { default = 60, description = "Context % threshold" }

[[steps]]
id = "check"
type = "condition"
condition = "meow context-usage --agent {{agent}} | awk '{exit ($1 > {{threshold}} ? 0 : 1)}'"
on_true:
  template = "handoff"
  variables = { agent = "{{agent}}" }
on_false:
  inline = []  # Empty, just continue
```

### Example: `work-loop` Template

A complete outer loop with context management:

```toml
# .meow/templates/work-loop.toml
[meta]
name = "work-loop"
description = "Main work loop - select task, implement, check context, repeat"

[variables]
agent = { required = true }

# --- Check if work remains ---
[[steps]]
id = "check-work"
type = "condition"
condition = "bd list --type=task --status=open | grep -q ."
on_true:
  inline = [
    { id = "select-task", type = "task", title = "Select next task to work on" },
    { id = "implement", type = "expand", template = "call", needs = ["select-task"],
      variables = { parent = "{{agent}}", child = "{{agent}}-worker",
                    template = "implement-tdd", use_worktree = true } },
    { id = "context-check", type = "expand", template = "context-check",
      needs = ["implement"], variables = { agent = "{{agent}}" } },
    { id = "loop", type = "expand", template = "work-loop",
      needs = ["context-check"], variables = { agent = "{{agent}}" } }
  ]
on_false:
  inline = [
    { id = "summary", type = "task", title = "Write final summary" },
    { id = "done", type = "stop", agent = "{{agent}}", needs = ["summary"] }
  ]
```

---

## Complete Execution Trace

Let's trace through `meow run work-loop --var agent=claude-1`:

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 1: meow run work-loop --var agent=claude-1                                │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  Orchestrator:                                                                   │
│  1. Bakes work-loop template → creates bead: check-work (condition)              │
│  2. Spawns claude-1 in tmux "meow-claude-1"                                      │
│  3. Enters main loop                                                             │
│                                                                                  │
│  Beads: [check-work(condition, open)]                                            │
│  Agents: {claude-1: active}                                                      │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 2: Orchestrator processes check-work condition                            │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Runs: bd list --type=task --status=open | grep -q .                          │
│  2. Exit code 0 (TRUE) - there are open tasks                                    │
│  3. Expands on_true inline beads                                                 │
│  4. Closes check-work                                                            │
│                                                                                  │
│  Beads: [check-work(closed), select-task(task, open), implement(expand, open),   │
│          context-check(expand, open), loop(expand, open)]                        │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 3: Claude-1 executes select-task                                          │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  Claude-1:                                                                       │
│  1. `meow prime` shows select-task                                               │
│  2. Claude analyzes open tasks, picks bd-task-001                                │
│  3. Claude runs `meow close select-task`                                         │
│                                                                                  │
│  Orchestrator:                                                                   │
│  4. Sees select-task closed                                                      │
│  5. Next ready: implement (expand)                                               │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 4: Orchestrator expands implement (call template)                         │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Loads "call" template                                                        │
│  2. Substitutes: parent=claude-1, child=claude-1-worker,                         │
│                  template=implement-tdd, use_worktree=true                       │
│  3. Creates beads: checkpoint-parent, stop-parent, setup-worktree,               │
│                    start-child, child-work(expand), stop-child,                  │
│                    cleanup-worktree, resume-parent                               │
│  4. Closes implement bead                                                        │
│                                                                                  │
│  Beads now include the full call ceremony                                        │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 5: Orchestrator processes checkpoint-parent                               │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Saves claude-1's session checkpoint                                          │
│  2. Closes checkpoint-parent                                                     │
│                                                                                  │
│  Agents: {claude-1: active, checkpoint: "conv_xyz"}                              │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 6: Orchestrator processes stop-parent                                     │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Kills tmux session "meow-claude-1"                                           │
│  2. Updates claude-1 status to "checkpointed"                                    │
│  3. Closes stop-parent                                                           │
│                                                                                  │
│  Agents: {claude-1: checkpointed}                                                │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 7: Orchestrator processes setup-worktree (code)                           │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Executes shell: git worktree add -b meow/claude-1-worker ...                 │
│  2. Closes setup-worktree                                                        │
│                                                                                  │
│  Git: new worktree at ~/worktrees/claude-1-worker                                │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 8: Orchestrator processes start-child                                     │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Creates tmux session "meow-claude-1-worker"                                  │
│  2. Starts Claude with `meow prime`                                              │
│  3. Updates claude-1-worker status to "active"                                   │
│  4. Closes start-child                                                           │
│                                                                                  │
│  Agents: {claude-1: checkpointed, claude-1-worker: active}                       │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 9: Orchestrator expands child-work (implement-tdd template)               │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Loads implement-tdd template                                                 │
│  2. Creates beads: load-context, write-tests, implement, validate, commit        │
│  3. All assigned to claude-1-worker                                              │
│  4. Closes child-work                                                            │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 10-14: Claude-1-worker executes implement-tdd steps                       │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  Claude-1-worker (in worktree):                                                  │
│  - load-context: reads task, identifies files                                    │
│  - write-tests: creates test file                                                │
│  - implement: writes code                                                        │
│  - validate: runs tests                                                          │
│  - commit: git commit                                                            │
│                                                                                  │
│  Each: `meow prime` → work → `meow close`                                        │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 15: Orchestrator processes stop-child                                     │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Kills tmux session "meow-claude-1-worker"                                    │
│  2. Updates claude-1-worker status to "stopped"                                  │
│  3. Closes stop-child                                                            │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 16: Orchestrator processes cleanup-worktree (code)                        │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Executes: git worktree remove ...; git branch -d ...                         │
│  2. Closes cleanup-worktree                                                      │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 17: Orchestrator processes resume-parent                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Retrieves claude-1's checkpoint                                              │
│  2. Starts: claude --resume conv_xyz                                             │
│  3. Injects `meow prime`                                                         │
│  4. Updates claude-1 status to "active"                                          │
│  5. Closes resume-parent                                                         │
│                                                                                  │
│  Claude-1 wakes up with full context, sees next task                             │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 18: Orchestrator expands context-check                                    │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Expands context-check template                                               │
│  2. Creates condition bead                                                       │
│  3. Evaluates: meow context-usage --agent claude-1 → 45%                         │
│  4. 45 > 60? FALSE                                                               │
│  5. Expands on_false: empty (nothing added)                                      │
│  6. Closes context-check condition                                               │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  STEP 19: Orchestrator expands loop (work-loop recursion)                       │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  1. Expands work-loop template again                                             │
│  2. Creates new check-work condition                                             │
│  3. Closes loop bead                                                             │
│                                                                                  │
│  Cycle repeats from STEP 2...                                                    │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
                                        │
                                        ▼
┌─────────────────────────────────────────────────────────────────────────────────┐
│  EVENTUALLY: No more open tasks                                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                  │
│  check-work condition:                                                           │
│  1. Runs: bd list --type=task --status=open | grep -q .                          │
│  2. Exit code 1 (FALSE) - no open tasks                                          │
│  3. Expands on_false: summary (task), done (stop)                                │
│                                                                                  │
│  Claude-1 writes summary, then orchestrator stops it.                            │
│  No more ready beads. Workflow complete.                                         │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

---

## CLI Commands

### User Commands

| Command | Description |
|---------|-------------|
| `meow init` | Initialize .meow directory with default templates |
| `meow run <template>` | Bake template and start orchestrator |
| `meow status` | Show current state: agents, active beads |
| `meow approve <bead>` | Unblock a `meow wait-approve` condition |
| `meow reject <bead>` | Reject a `meow wait-approve` condition (with optional reason) |
| `meow stop` | Gracefully stop orchestrator |

### Agent Commands (Claude uses these)

| Command | Description |
|---------|-------------|
| `meow prime` | Show next task for current agent |
| `meow close <bead>` | Close a task bead |
| `meow context-usage` | Report context window usage |

### Debug Commands

| Command | Description |
|---------|-------------|
| `meow agents` | List all agents and their states |
| `meow beads` | List all beads with status |
| `meow trace` | Show execution trace/history |

---

## State Management

### File Structure

```
.meow/
├── config.toml          # MEOW configuration
├── agents.json          # Agent states (active, checkpointed, etc.)
├── templates/           # User-defined templates
│   ├── work-loop.toml
│   ├── implement-tdd.toml
│   ├── call.toml
│   ├── refresh.toml
│   └── ...
└── state/
    ├── orchestrator.json # Orchestrator state (for crash recovery)
    └── trace.jsonl       # Execution trace log

.beads/
├── issues.jsonl         # All beads (tasks, conditions, etc.)
└── beads.db             # SQLite cache
```

### Crash Recovery

If the orchestrator crashes:
1. On restart, load `.meow/state/orchestrator.json`
2. Rebuild agent states from `.meow/agents.json`
3. Find all open beads from `.beads/`
4. Resume main loop

All state is persisted; no in-memory-only state.

---

## Future: Parallelization

The architecture supports parallelization via:

### 1. Multiple Agents

Multiple agents can be active simultaneously:

```yaml
# Two agents working in parallel
claude-1: active (working on bd-task-001)
claude-2: active (working on bd-task-002)
```

### 2. Bead Assignment

Beads specify which agent handles them:

```yaml
id: bd-task-001
assignee: claude-1

id: bd-task-002
assignee: claude-2
```

### 3. `foreach` Directive (Future Enhancement)

Templates can generate parallel beads:

```toml
[[steps]]
id = "work-{{item.id}}"
type = "task"
assignee = "{{item.agent}}"
foreach = "item in work_items"
```

### 4. Join Semantics

Wait for multiple beads:

```toml
[[steps]]
id = "merge-results"
type = "task"
needs = ["work-task-1", "work-task-2", "work-task-3"]
```

The `needs` array creates implicit join points.

---

## Implementation Phases

### Phase 1: Core Orchestrator (MVP)

**Goal**: Single-agent execution with all 8 primitives.

- [ ] Go binary: `meow` CLI
- [ ] Template parser (TOML → beads)
- [ ] Orchestrator main loop
- [ ] All 8 primitive handlers
- [ ] Agent state management
- [ ] tmux session management
- [ ] `meow prime` / `meow close` integration
- [ ] `meow wait-approve` / `meow approve` for human gates
- [ ] Basic crash recovery

**Deliverable**: Can run a simple loop workflow with one agent.

### Phase 2: Composition Library

**Goal**: Useful default templates.

- [ ] `refresh` template
- [ ] `handoff` template
- [ ] `call` template (with worktree support)
- [ ] `context-check` template
- [ ] `work-loop` template
- [ ] Template documentation

**Deliverable**: Users can run complex workflows out of the box.

### Phase 3: Robustness

**Goal**: Production-ready single-agent.

- [ ] Comprehensive error handling
- [ ] Detailed logging and tracing
- [ ] `meow status` / `meow agents` / `meow trace`
- [ ] Condition timeout handling
- [ ] Integration tests

**Deliverable**: Reliable single-agent orchestration.

### Phase 4: Parallelization (Future)

**Goal**: Multi-agent parallel execution.

- [ ] `foreach` directive
- [ ] Parallel `start` handling
- [ ] Join point detection
- [ ] Branch merge conflict handling
- [ ] Resource contention (file locks via mcp_agent_mail)

**Deliverable**: Multiple Claude instances working in parallel.

---

## Summary

MEOW Stack MVP is built on **8 primitive bead types**:

| # | Primitive | Purpose |
|---|-----------|---------|
| 1 | `task` | Claude-executed work |
| 2 | `condition` | Branching, looping, and waiting (can block for gates) |
| 3 | `stop` | Kill agent session |
| 4 | `start` | Spawn agent |
| 5 | `checkpoint` | Save for resume |
| 6 | `resume` | Resume from checkpoint |
| 7 | `code` | Arbitrary shell execution |
| 8 | `expand` | Template composition |

Everything else—`refresh`, `handoff`, `call`, loops, context management, human approval gates—is **template composition** using these primitives.

The orchestrator is a simple state machine. The templates are the programs. Users have complete control.
