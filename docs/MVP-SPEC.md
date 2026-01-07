# MEOW Stack MVP Specification

This document specifies the implementation of MEOW Stack (Molecular Expression Of Work), a durable, recursive, composable workflow system for AI agent orchestration. Built on a **primitives-first architecture** where 6 orthogonal primitive types compose into arbitrary workflows. The type system includes 8 total bead types: the 6 primitives plus `gate` (human approval) and `collaborative` (interactive conversation) as specialized task variants.

> **Design Principle**: The orchestrator is dumb; the templates are smart. All workflow complexity lives in composable templates, not hardcoded orchestrator logic.

## Table of Contents

1. [Design Philosophy](#design-philosophy)
2. [Beads Integration Strategy](#beads-integration-strategy)
3. [The 6 Primitive Bead Types](#the-6-primitive-bead-types)
4. [Output Binding](#output-binding)
5. [Task Outputs with Validation](#task-outputs-with-validation)
6. [Three-Tier Bead Architecture](#three-tier-bead-architecture)
7. [Module System](#module-system)
8. [Orchestrator Architecture](#orchestrator-architecture)
9. [Template System](#template-system)
10. [Agent Management](#agent-management)
11. [Execution Model](#execution-model)
12. [Composable Templates](#composable-templates)
13. [Complete Execution Trace](#complete-execution-trace)
14. [CLI Commands](#cli-commands)
15. [State Management](#state-management)
16. [Future: Parallelization](#future-parallelization)
17. [Implementation Phases](#implementation-phases)
18. [Error Handling](#error-handling)
19. [Crash Recovery](#crash-recovery)
20. [Debugging and Observability](#debugging-and-observability)
21. [Integration Patterns](#integration-patterns)

---

## Design Philosophy

### Primitives Over Special Cases

MEOW Stack is built on a fundamental principle: **minimize orchestrator complexity by maximizing template expressiveness**.

Instead of the orchestrator having special knowledge of:

- Context thresholds and automatic handoff
- Loop semantics and iteration counters
- Clone vs fresh spawn modes
- Worktree management

We provide **6 primitive bead types** that users compose into templates. The orchestrator is a simple state machine that recognizes these primitives and executes them. Everything else—including context management, refresh logic, call/return semantics, worktree handling, and even human approval gates—is **user-defined template composition**.

### Core Principles

#### The Propulsion Principle

> **If you find work assigned to you, YOU RUN IT.**

MEOW is a steam engine. Agents are pistons. The system's throughput depends on one thing: when an agent finds work, they EXECUTE immediately. There is no supervisor polling "did you start yet?" The bead assignment IS the instruction.

#### Zero Friendly Conjecture (ZFC)

> **Don't reason about other agents.**

Agents should not try to infer what other agents are doing, thinking, or planning. Trust the bead state. If you need coordination, use explicit signals (beads, mail, files). The orchestrator handles coordination; agents handle execution.

#### Three-Layer Durability

Agent state has three layers that operate independently:

| Layer | Component | Lifecycle | Persistence |
|-------|-----------|-----------|-------------|
| **Session** | Claude process | Ephemeral | Cycles per step/handoff |
| **Workspace** | Git worktree | Persistent | Until explicit cleanup |
| **Identity** | Agent ID/slot | Persistent | Until workflow complete |

**Key insight**: Session cycling is **normal operation**, not failure. The workspace persists across sessions. Handoff notes provide continuity.

#### Beads as Source of Truth

All workflow state lives in beads:
- Task assignments
- Progress tracking
- Dependencies
- Notes and context
- Error state

The orchestrator doesn't maintain separate state—it reads beads. Agents update beads. Everything is observable and recoverable.

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

### The Condition Insight

The `condition` primitive is deceptively powerful because **shell commands can block**. This means human approval gates, CI waits, timers, webhooks, inter-agent signals—anything that "waits"—is just a blocking condition. The orchestrator doesn't care if a condition takes 1ms or 24 hours.

---

## Beads Integration Strategy

> **Decision**: MEOW Stack layers its workflow system **on top of upstream beads** rather than maintaining a fork. Both CLIs coexist and operate on the same data.

### Rationale

Forking beads is unnecessary because:

1. **JSON is schema-flexible** - MEOW can add fields to `issues.jsonl` that upstream `bd` will ignore but preserve
2. **ID prefix separation** - MEOW workflow beads use `meow-*` prefix, naturally separating them from work beads (`bd-*`)
3. **CLI separation** - `meow` CLI handles workflow operations; `bd` CLI handles traditional issue tracking

### How Coexistence Works

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         BEADS INTEGRATION MODEL                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  .beads/issues.jsonl  ←──────────────────────────────────────────────┐      │
│  ┌─────────────────────────────────────────────────────────────────┐ │      │
│  │ {"id":"bd-task-123","title":"Implement auth",...}               │ │      │
│  │ {"id":"meow-abc.load-context","tier":"wisp","hook_bead":...}    │ │      │
│  │ {"id":"meow-abc.write-tests","tier":"wisp",...}                 │ │      │
│  └─────────────────────────────────────────────────────────────────┘ │      │
│              ┌───────────────┴───────────────┐                       │      │
│              ▼                               ▼                       │      │
│  ┌───────────────────────┐      ┌───────────────────────┐           │      │
│  │     bd CLI            │      │     meow CLI          │           │      │
│  │  (upstream beads)     │      │  (MEOW orchestrator)  │           │      │
│  ├───────────────────────┤      ├───────────────────────┤           │      │
│  │ • bd ready            │      │ • meow prime          │           │      │
│  │ • bd list             │      │ • meow close          │           │      │
│  │ • bd create           │      │ • meow run            │           │      │
│  ├───────────────────────┤      ├───────────────────────┤           │      │
│  │ Sees: All beads       │      │ Sees: All beads       │           │      │
│  │ Ignores: MEOW fields  │      │ Understands: Tiers,   │           │      │
│  │ (tier, condition_spec)│      │ specs, workflows      │           │      │
│  └───────────────────────┘      └───────────────────────┘           │      │
└──────────────────────────────────────────────────────────────────────┘
```

### ID Prefix Convention

| Prefix | Source | Visible in `bd ready` | Visible in `meow prime` |
|--------|--------|----------------------|------------------------|
| `bd-*` | Traditional beads (`bd create`) | Yes | No (unless selected as work bead) |
| `meow-*` | MEOW workflows | No (filtered by prefix) | Yes (wisps and orchestrator beads) |

### What Each CLI Handles

| Operation | Use `bd` | Use `meow` |
|-----------|----------|------------|
| Create work beads | `bd create --title "..."` | - |
| View work backlog | `bd ready`, `bv --robot-triage` | - |
| Run workflow on work bead | - | `meow run implement-tdd --work-bead bd-123` |
| See current workflow step | - | `meow prime` |
| Close workflow step | - | `meow close meow-abc.step-1` |
| Close work bead | `bd close bd-123` | (or via workflow's final step) |

### MEOW-Specific Fields

MEOW extends the JSON schema with fields that upstream `bd` ignores:

```json
{
  "id": "meow-abc123.load-context",
  "title": "Load context for bd-task-456",
  "status": "open",
  "tier": "wisp",
  "hook_bead": "bd-task-456",
  "source_workflow": "implement-tdd",
  "workflow_id": "meow-abc123",
  "condition_spec": null,
  "outputs": {}
}
```

---

## The 6 Primitive Bead Types

The orchestrator recognizes exactly 6 primitive bead types. Everything else is built from these. Additionally, `gate` and `collaborative` are specialized task variants that modify agent behavior.

### Overview

| Type | Executor | Description |
|------|----------|-------------|
| `task` | Agent | Regular work bead—agent executes and closes, auto-continues via stop hook |
| `collaborative` | Agent | Interactive step—agent pauses for human conversation, no auto-continue |
| `gate` | Human | Approval checkpoint—no assignee, human closes via `meow approve` |
| `condition` | Orchestrator | Evaluate shell command (can block!), expand on_true or on_false |
| `stop` | Orchestrator | Kill an agent's tmux session |
| `start` | Orchestrator | Spawn an agent in tmux (with optional resume from session) |
| `code` | Orchestrator | Execute arbitrary shell code (with output capture) |
| `expand` | Orchestrator | Expand a template into beads at this point |

### Task Variants: gate and collaborative

The `gate` and `collaborative` types are specialized task variants that modify agent behavior:

| Type | Assignee | Auto-continue | Who closes | Use case |
|------|----------|---------------|------------|----------|
| `task` | Required | Yes (Ralph Wiggum loop) | Agent | Normal autonomous work |
| `collaborative` | Required | No (pauses for conversation) | Agent | Design review, clarification |
| `gate` | None | No | Human | Approval checkpoints |

**The `collaborative` type** enables human-in-the-loop interaction. When an agent reaches a collaborative step:
1. Agent executes the step (presents information, asks questions)
2. Agent's stop hook fires, BUT...
3. `meow prime --format prompt` returns **empty output** for in-progress collaborative steps
4. No prompt injection → Claude waits naturally for user input
5. User and agent converse freely
6. When done, agent runs `meow close <step-id>`
7. Next stop hook fires, normal flow resumes

**The `gate` type** is for human approval points. Gates have no assignee—the orchestrator waits for a human to run `meow approve` or `meow reject`.

### Why 6 Primitives + 2 Variants?

The original design had `checkpoint` and `resume` as separate primitives. After analysis, we collapsed these:

- **`checkpoint`** → `code` bead that runs `meow session-id --save`
- **`resume`** → `start` bead with `resume_session` parameter

This follows the principle of **composition over specialization**. The `start` primitive now optionally takes a session ID for resumption, making it more flexible while reducing the primitive count.

### Common Bead Fields

All beads share these fields:

```yaml
id: "bd-abc123"           # Unique identifier (hash-based)
type: "task"              # One of the 8 primitive types
title: "Short description"
description: "Detailed instructions"
status: "open"            # open | in_progress | closed
assignee: "claude-1"      # Agent ID (null for orchestrator beads)
needs: ["bd-xyz"]         # Dependencies (bead IDs that must be closed first)
labels: ["priority:high"] # Optional labels for filtering
notes: ""                 # Handoff notes, progress updates
created_at: "2026-01-06T12:00:00Z"
closed_at: null
```

### Detailed Specifications

#### `task` — Agent-Executed Work

The default bead type. Claude receives it via `meow prime`, executes the work, and closes it. Tasks can optionally require **validated outputs** that subsequent beads can reference.

```yaml
id: "bd-abc123"
type: task
title: "Implement user registration endpoint"
description: "Create POST /api/register with validation"
instructions: |
  1. Read the existing auth module at src/auth/
  2. Create new endpoint following existing patterns
  3. Add input validation for email and password
  4. Write tests in src/auth/__tests__/
  5. Run `npm test` to verify
assignee: "claude-1"
status: open
notes: ""
```

**With required outputs** (see [Task Outputs with Validation](#task-outputs-with-validation)):

```yaml
id: "bd-select-001"
type: task
title: "Select next task to work on"
description: "Run bv --robot-triage and pick the highest priority task"
assignee: "claude-1"
outputs:
  required:
    - name: "work_bead"
      type: "bead_id"
      description: "The bead ID to implement"
    - name: "rationale"
      type: "string"
      description: "Why you chose this bead"
```

**Orchestrator behavior**:
1. Ensure assigned agent is running (spawn if not)
2. Wait for Claude to close the bead
3. **If outputs are required**: validate outputs before allowing close
4. On close, store outputs on the bead for subsequent reference
5. Check for next ready bead

**Agent behavior**:
1. `meow prime` shows this task (including required outputs)
2. Agent executes the work
3. Agent runs `meow close <bead-id>` with notes and **required outputs**:
   ```bash
   meow close bd-select-001 \
     --output work_bead=bd-task-123 \
     --output rationale="Highest priority, unblocks 3 others"
   ```

#### `condition` — Branching, Looping, and Waiting

Evaluate a shell command. If exit code is 0 (true), expand `on_true` template. Otherwise expand `on_false` template.

**Key insight**: Shell commands can block. The orchestrator doesn't care if a condition takes 1ms or 1 hour to evaluate. This means `condition` subsumes what other systems call "gates" or "waits"—human approval, API polling, timers, webhooks—are all just blocking shell commands.

##### Examples

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
```

```yaml
# Context threshold check
id: "bd-context-001"
type: condition
condition: "meow context-usage --agent {{agent}} --format exit-code --threshold 70"
on_true:
  template: "handoff"
  variables: { agent: "{{agent}}" }
on_false:
  inline: []  # Context OK, continue
```

```yaml
# Inter-agent signal wait
id: "bd-signal-001"
type: condition
condition: "meow wait-signal --from claude-2 --signal task-complete --timeout 1h"
on_true:
  inline: []
on_false:
  template: "handle-timeout"
```

**Orchestrator behavior**:
1. Execute `condition` as shell command (may block indefinitely)
2. Capture stdout/stderr for logging
3. Based on exit code, expand appropriate template
4. Insert expanded beads after this bead in dependency chain
5. Auto-close this bead
6. Continue main loop (agent will see newly expanded beads via `meow prime`)

**Timeout handling**: Conditions can specify `timeout` field. If exceeded:
- Condition exits with code 124 (timeout)
- `on_timeout` template expanded (if specified), otherwise treated as `on_false`

**Use cases**: Loops, branching, context checks, goal-oriented workflows, human approval gates, API waits, timers, webhooks.

**Helper CLIs for common waits**:
| Command | Behavior |
|---------|----------|
| `meow wait-approve --bead <id>` | Block until `meow approve <id>` is run |
| `meow wait-file <path>` | Block until file exists |
| `meow wait-api <url> --until "jq expr"` | Poll API until condition met |
| `meow wait-signal --from <agent> --signal <name>` | Wait for inter-agent signal |
| `meow wait-bead <id> --status closed` | Wait for specific bead to close |
| `meow context-usage --threshold N --format exit-code` | Exit 0 if above threshold |

These are conveniences—any blocking shell command works.

**Writing custom wait commands**: Any script that:
1. Blocks until condition is met (or timeout)
2. Exits 0 for "true", non-zero for "false"

#### `stop` — Kill Agent Session

Terminate an agent's tmux session.

```yaml
id: "bd-stop-001"
type: stop
agent: "claude-1"
graceful: true  # Optional: send SIGTERM first, wait, then SIGKILL
timeout: 10     # Seconds to wait for graceful shutdown
```

**Orchestrator behavior**:
1. If `graceful`:
   a. Send interrupt to Claude (Ctrl-C equivalent)
   b. Wait for clean exit up to `timeout`
   c. Force kill if still running
2. Kill tmux session for specified agent
3. Update agent state to "stopped"
4. Auto-close this bead

**Note**: Does NOT delete the workspace/worktree. Use `code` bead for cleanup if needed.

**Use cases**: End of workflow, before refresh, cleanup.

#### `start` — Spawn Agent (with Optional Resume)

Start an agent in a new tmux session. If `resume_session` is provided, starts Claude with `--resume` to restore prior context.

```yaml
id: "bd-start-001"
type: start
agent: "claude-2"
workdir: "/path/to/worktree"  # Optional, can be dynamic from code bead output
env:
  MEOW_AGENT: "claude-2"
  MEOW_ROLE: "worker"
  CUSTOM_VAR: "value"
prompt: "meow prime"  # Optional, defaults to "meow prime"
```

**With session resume** (replaces the old `resume` primitive):

```yaml
id: "bd-resume-parent-001"
type: start
agent: "claude-1"
workdir: "."
resume_session: "{{save-session.outputs.session_id}}"  # From prior code bead
```

**Orchestrator behavior**:
1. Create tmux session named `meow-{agent}`
2. Set working directory (defaults to current, can use output from prior bead)
3. Set environment variables (MEOW_AGENT always set)
4. **If `resume_session` is set**: start Claude with `--resume {session_id}`
5. Otherwise: start Claude fresh
6. Wait for Claude to be ready (detect prompt)
7. Inject initial prompt (default: `meow prime`)
8. Update agent state to "active"
9. Auto-close this bead

**Fresh start sequence**:
```bash
tmux new-session -d -s meow-claude-2 -c /path/to/workdir
tmux send-keys -t meow-claude-2 "MEOW_AGENT=claude-2 claude --dangerously-skip-permissions" Enter
# Wait for Claude ready prompt
tmux send-keys -t meow-claude-2 "meow prime" Enter
```

**Resume sequence**:
```bash
tmux new-session -d -s meow-claude-1 -c /path/to/workdir
tmux send-keys -t meow-claude-1 "MEOW_AGENT=claude-1 claude --resume session_abc123" Enter
# Wait for Claude ready prompt
tmux send-keys -t meow-claude-1 "meow prime" Enter
```

**Use cases**: Initial startup, spawning child agents, refresh, resuming parent after child completes.

#### Checkpoint and Resume via Composition

> **Note**: The original design had `checkpoint` and `resume` as separate primitives. These are now composed from `code` and `start` beads, reducing the primitive count from 8 to 6.

**Saving a checkpoint** (using `code` bead):

```yaml
id: "save-session"
type: code
code: |
  meow session-id --agent {{agent}}
outputs:
  session_id: stdout  # Capture for later use
```

**Resuming from checkpoint** (using `start` bead with `resume_session`):

```yaml
id: "resume-parent"
type: start
agent: "{{agent}}"
workdir: "."
resume_session: "{{save-session.outputs.session_id}}"
needs: ["save-session", "cleanup"]
```

This composition approach is more flexible:
- Session ID capture is explicit and inspectable
- Works with any session discovery mechanism
- Resume can happen anywhere, not just after checkpoint

#### `code` — Arbitrary Shell Execution (with Output Capture)

Execute arbitrary shell code. This is the escape hatch for user-defined orchestrator actions. **Outputs can be captured and used by subsequent beads**.

```yaml
id: "bd-code-001"
type: code
code: |
  git worktree add -b meow/claude-2 ~/worktrees/claude-2 HEAD
  echo "~/worktrees/claude-2"
workdir: "/path/to/repo"  # Optional
env:
  GIT_AUTHOR_NAME: "MEOW Bot"
outputs:
  worktree_path: stdout  # Capture stdout as named output
```

**With file-based output capture**:

```yaml
id: "bd-code-002"
type: code
code: |
  git rev-parse HEAD > /tmp/commit-sha.txt
  npm test 2>&1 | tee /tmp/test-output.txt
outputs:
  commit_sha: file:/tmp/commit-sha.txt
  test_log: file:/tmp/test-output.txt
```

**Using captured outputs in subsequent beads**:

```yaml
id: "start-worker"
type: start
agent: "claude-2"
workdir: "{{bd-code-001.outputs.worktree_path}}"  # Uses output from code bead
needs: ["bd-code-001"]
```

**Orchestrator behavior**:
1. Execute `code` as shell script
2. Set working directory and environment
3. Capture stdout and stderr
4. **If `outputs` defined**: capture specified outputs (stdout, file paths)
5. Store outputs on the bead for subsequent reference
6. Log to trace file
7. If exit code != 0:
   - Check `on_error` field
   - Default: log error and continue (non-fatal)
   - If `on_error: abort`: stop workflow
   - If `on_error: retry`: retry up to `max_retries` times
8. Auto-close this bead

**Output types**:
| Type | Syntax | Description |
|------|--------|-------------|
| `stdout` | `outputs: { name: stdout }` | Capture stdout (trimmed) |
| `stderr` | `outputs: { name: stderr }` | Capture stderr (trimmed) |
| `file` | `outputs: { name: file:/path }` | Read file contents |
| `exit_code` | `outputs: { name: exit_code }` | Capture exit code as string |

**Use cases**: Worktree management, environment setup, git operations, notifications, session ID discovery, any custom action with data passing.

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

**With ephemeral beads** (see [Ephemeral Beads](#ephemeral-beads-wisps)):

```yaml
id: "bd-expand-002"
type: expand
template: "call-implement"
ephemeral: true  # Expanded beads are operational, not work items
variables:
  parent: "claude-1"
  child: "claude-1-worker"
  work_bead: "{{select-task.outputs.work_bead}}"  # Uses output from prior task
```

**Orchestrator behavior**:
1. Load template from `.meow/templates/{template}.toml`
2. Validate required variables are provided
3. Substitute all `{{variable}}` references (including outputs from prior beads)
4. For each step in template:
   a. Generate unique bead ID: `bd-{step.id}-{random}`
   b. Translate `needs` to actual bead IDs
   c. Set `assignee` if specified
   d. **If `ephemeral: true`**: mark beads with `labels: ["meow:ephemeral"]`
   e. Create bead
5. Wire up dependencies so expanded beads execute after this bead's position
6. Auto-close this bead

**Nesting**: Expanded templates can contain `expand` beads, enabling recursive composition.

**Use cases**: Template composition, call semantics, slot filling, ephemeral workflow machinery.

---

## Output Binding

Output binding allows data to flow between beads. This is essential for dynamic workflows where one bead's output becomes another's input.

### The Problem

Without output binding, workflows are static:

```yaml
# Bad: Hardcoded path
id: "start-worker"
type: start
workdir: "/hardcoded/path"  # Can't adapt to runtime decisions
```

### The Solution

Beads can capture outputs that subsequent beads reference:

```yaml
# Good: Dynamic path from code bead
id: "create-worktree"
type: code
code: |
  git worktree add -b meow/worker ~/worktrees/worker HEAD
  echo "~/worktrees/worker"
outputs:
  path: stdout

id: "start-worker"
type: start
workdir: "{{create-worktree.outputs.path}}"  # Dynamic!
needs: ["create-worktree"]
```

### Output Sources

| Bead Type | Output Sources |
|-----------|----------------|
| `code` | stdout, stderr, file contents, exit code |
| `task` | Validated outputs from `meow close --output` |
| `condition` | None (outputs are branch selection) |
| `expand` | None (outputs are the expanded beads) |
| `start`, `stop` | None |

### Reference Syntax

Outputs are referenced using `{{bead_id.outputs.field}}`:

```yaml
# From code bead
workdir: "{{setup-worktree.outputs.path}}"

# From task bead
work_bead: "{{select-task.outputs.work_bead}}"

# Nested reference
resume_session: "{{save-session.outputs.session_id}}"
```

### Storage

Outputs are stored on the bead itself:

```json
{
  "id": "create-worktree",
  "type": "code",
  "status": "closed",
  "outputs": {
    "path": "~/worktrees/worker"
  }
}
```

This ensures:
- Outputs survive orchestrator restarts
- Outputs are inspectable (`bd show create-worktree`)
- Outputs are part of the audit trail

---

## Task Outputs with Validation

Tasks can require **validated outputs** that Claude must provide when closing. This enables reliable data flow from Claude's decisions to subsequent beads.

### The Problem

When Claude needs to make a decision (like selecting a task), how does that decision get passed forward?

```yaml
# Claude selects a task, but how do we know which one?
id: "select-task"
type: task
title: "Select next task to work on"
# ... Claude picks one, but where does it go?
```

### The Solution: Required Outputs

Tasks can declare required outputs with types:

```yaml
id: "select-task"
type: task
title: "Select next task to work on"
description: "Run bv --robot-triage and pick the highest priority task"
outputs:
  required:
    - name: "work_bead"
      type: "bead_id"
      description: "The bead ID to implement"
    - name: "rationale"
      type: "string"
      description: "Why you chose this bead"
  optional:
    - name: "alternative"
      type: "bead_id"
      description: "Second choice if first is blocked"
```

### Closing with Outputs

Claude provides outputs when closing:

```bash
# This works
meow close bd-select-001 \
  --output work_bead=bd-task-123 \
  --output rationale="Highest priority, unblocks 3 others"

# Or with JSON
meow close bd-select-001 --output-json '{
  "work_bead": "bd-task-123",
  "rationale": "Highest priority, unblocks 3 others"
}'
```

### Validation

If Claude doesn't provide required outputs, the close fails:

```bash
$ meow close bd-select-001
Error: Cannot close bd-select-001 - missing required outputs

Required outputs:
  ✗ work_bead (bead_id): Not provided
  ✗ rationale (string): Not provided

Usage:
  meow close bd-select-001 --output work_bead=<bead_id> --output rationale=<string>
```

### Output Types

| Type | Validation | Example |
|------|------------|---------|
| `string` | Non-empty string | `"bd-task-123"` |
| `string[]` | Array of strings | `["bd-1", "bd-2"]` |
| `number` | Parseable as number | `42`, `3.14` |
| `boolean` | `true` or `false` | `true` |
| `json` | Valid JSON | `{"key": "value"}` |
| `bead_id` | Exists in .beads/ | `bd-task-123` (validated!) |
| `bead_id[]` | Array of valid bead IDs | `["bd-1", "bd-2"]` |
| `file_path` | File exists | `src/auth/register.go` |

### The `bead_id` Type

The `bead_id` type is special—it validates that the bead exists:

```bash
$ meow close bd-select-001 --output work_bead=bd-task-999
Error: Output validation failed

  work_bead: "bd-task-999"
    ✗ Type: bead_id
    ✗ Error: Bead 'bd-task-999' does not exist

    Available beads matching 'bd-task-*':
      - bd-task-001 "Implement user auth"
      - bd-task-002 "Add password validation"

Please provide valid outputs and try again.
```

This catches typos and hallucinated bead IDs.

### Using Outputs in Templates

Subsequent beads reference task outputs:

```toml
[[steps]]
id = "select-task"
type = "task"
title = "Select next task"
[steps.outputs]
required = [
  { name = "work_bead", type = "bead_id" }
]

[[steps]]
id = "implement"
type = "expand"
template = "call-implement"
variables = { work_bead = "{{select-task.outputs.work_bead}}" }
needs = ["select-task"]
```

### Prime Shows Expected Outputs

`meow prime` shows Claude what outputs are expected:

```
=== YOUR CURRENT TASK ===
Bead: bd-select-001
Title: Select next task to work on

Instructions:
  Run: bv --robot-triage
  Pick the highest priority ready task.

REQUIRED OUTPUTS when closing:
  work_bead (bead_id): The bead ID to implement
  rationale (string): Why you chose this bead

Example:
  meow close bd-select-001 \
    --output work_bead=bd-task-XXX \
    --output rationale="..."
```

---

## Three-Tier Bead Architecture

All beads live in the same storage for durability and crash recovery. Tiers are distinguished by an **explicit `Tier` field**, not computed from labels.

### Tier Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          BEAD VISIBILITY MODEL                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  TIER 1: WORK BEADS (Persistent Deliverables)                               │
│  ────────────────────────────────────────────                               │
│  Purpose:     Actual deliverables from PRD breakdown                        │
│  Examples:    "Implement auth endpoint", "Fix login bug"                    │
│  Created by:  Humans, PRD breakdown, `bd create`                            │
│  Visible via: `bd ready`, `bv --robot-triage`, `bd list`                    │
│  Agent role:  SELECTS from these based on priority/impact                   │
│  Persistence: Permanent, exported to JSONL, git-synced                      │
│  Tier value:  "work"                                                        │
│                                                                             │
│  TIER 2: WORKFLOW BEADS (Wisps - Ephemeral Guidance)                        │
│  ───────────────────────────────────────────────────                        │
│  Purpose:     Procedural guidance - HOW to do work                          │
│  Examples:    "Load context", "Write tests", "Commit changes"               │
│  Created by:  Template expansion (`expand`, `condition` branches)           │
│  Visible via: `meow prime`, `meow steps`                                    │
│  Agent role:  EXECUTES these step-by-step                                   │
│  Persistence: Durable in DB until burned, NOT exported to JSONL             │
│  Tier value:  "wisp"                                                        │
│                                                                             │
│  TIER 3: ORCHESTRATOR BEADS (Infrastructure - Invisible)                    │
│  ───────────────────────────────────────────────────────                    │
│  Purpose:     Workflow machinery - control flow, agent lifecycle            │
│  Examples:    "start-worker", "check-tests", "expand-implement"             │
│  Created by:  Template expansion (non-task primitives)                      │
│  Visible via: NEVER shown to agents, only debug views                       │
│  Agent role:  NONE - orchestrator handles exclusively                       │
│  Persistence: Durable in DB until burned, NOT exported to JSONL             │
│  Tier value:  "orchestrator"                                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

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

Benefits:
- O(1) tier lookup (vs O(n) label scanning)
- Self-documenting and explicit
- Validated at creation time
- Simpler filtering logic everywhere

### Bead Status (Simplified)

Three statuses only:

```go
const (
    StatusOpen       BeadStatus = "open"        // Ready to be worked on
    StatusInProgress BeadStatus = "in_progress" // Currently being executed
    StatusClosed     BeadStatus = "closed"      // Completed
)
```

Status transitions:
```
open → in_progress → closed
  ↑                    │
  └────────────────────┘  (reopen if needed)
```

### Metadata Fields by Tier

#### Tier 1: Work Beads

```yaml
id: "gt-123"
tier: "work"
issue_type: task | feature | bug | epic | chore
status: open | in_progress | closed
# No source_workflow - not from a template
# No hook_bead - IS a work bead
```

#### Tier 2: Workflow Beads (Wisps)

```yaml
id: "meow-abc.load-context"
tier: "wisp"
issue_type: task
status: open | in_progress | closed
source_workflow: "implement-tdd"
workflow_id: "meow-abc"
hook_bead: "gt-123"           # The work bead being implemented
assignee: "claude-2"          # The agent executing this wisp
```

#### Tier 3: Orchestrator Beads

```yaml
id: "meow-abc.start-worker"
tier: "orchestrator"
issue_type: start | stop | condition | expand | code
status: open | closed         # Auto-closed by orchestrator
source_workflow: "implement-tdd"
workflow_id: "meow-abc"
# No assignee - orchestrator handles
```

### Automatic Tier Detection

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
        return TierOrchestrator
    default:
        return TierOrchestrator
    }
}
```

The key insight: **if a workflow is marked `ephemeral = true`, all its tasks become wisps.**

### The Two-View Model

Agents have TWO simultaneous views:

1. **Wisp view** (`meow prime`) - "What step am I on?"
2. **Work view** (`bd ready`, `bv --robot-triage`) - "What work beads exist?"

The agent can freely query work beads while executing wisp steps. The wisp tells them HOW, the work beads tell them WHAT.

### Cleanup (Burning Wisps)

When wisps get burned:
- On workflow completion (all beads in workflow are closed)
- NOT on agent stop (agent might resume)
- NOT on work bead close (workflow machinery still needed)

```bash
# Manual cleanup
meow clean --ephemeral

# Automatic cleanup (in config)
[cleanup]
ephemeral = "on_complete"  # on_complete | manual | never
```

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
instructions = "Run bv --robot-triage and pick the highest priority task"

[[main.steps]]
id = "do-work"
type = "expand"
template = ".implement"   # Local reference
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
instructions = "Read the task and identify relevant files"
assignee = "{{agent}}"

[[implement.steps]]
id = "write-tests"
type = "task"
title = "Write failing tests"
assignee = "{{agent}}"
needs = ["load-context"]
```

### The `main` Convention

When running a module file:

```bash
meow run work-module.meow.toml           # Runs [main] workflow
meow run work-module.meow.toml#implement # Runs [implement] workflow explicitly
```

- `[main]` is the default entry point
- If `[main]` doesn't exist, error unless explicit `#workflow` specified

### Reference Syntax

| Reference | Resolution |
|-----------|------------|
| `.implement` | Same file, workflow named `implement` |
| `.implement.verify` | Same file, workflow `implement`, step `verify` (partial execution) |
| `helpers#tdd` | File `helpers.meow.toml`, workflow named `tdd` |
| `helpers` | File `helpers.meow.toml`, workflow `main` |
| `./subdir/util.meow.toml#helper` | Relative path |

### Workflow Properties

```toml
[workflow-name]
name = "human-readable-name"     # Required
description = "..."              # Optional
ephemeral = true                 # All expanded beads are wisps (default: false)
internal = true                  # Cannot be called from outside this file (default: false)
hooks_to = "variable_name"       # Links all wisps to this variable's bead ID value

[workflow-name.variables]
var_name = { required = true, type = "bead_id", description = "..." }
var_with_default = { default = "value", type = "string" }

[[workflow-name.steps]]
# ... step definitions
```

### The `hooks_to` Property

**Explicit HookBead linking** via workflow-level declaration:

```toml
[implement]
hooks_to = "work_bead"  # All wisps from this workflow link to {{work_bead}}
```

When the baker creates wisps from this workflow, it sets:
```go
bead.HookBead = vars["work_bead"]  // e.g., "gt-123"
```

This replaces magic variable name detection. The link is explicit and documented in the workflow definition.

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
│  │          if allDone():                                                     │ │
│  │              log("Workflow complete")                                      │ │
│  │              return                                                        │ │
│  │          sleep(100ms)             // Waiting on agent                      │ │
│  │          continue                                                          │ │
│  │                                                                            │ │
│  │      switch bead.type:                                                     │ │
│  │          case "task":       waitForClaudeToClose(bead)                     │ │
│  │          case "condition":  evalAndExpand(bead)  // may block!             │ │
│  │          case "stop":       killAgent(bead.agent); close(bead)             │ │
│  │          case "start":      spawnAgent(bead); close(bead)  // handles resume│ │
│  │          case "code":       execShell(bead); close(bead)   // captures output│ │
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
│  ┌────────────────────────────────────────────────────────────────────────────┐ │
│  │                    CONCURRENT CONDITION HANDLING                           │ │
│  │                                                                            │ │
│  │  // Conditions run in background goroutines to avoid blocking main loop   │ │
│  │  // This allows multiple conditions to be evaluated concurrently          │ │
│  │                                                                            │ │
│  │  func handleCondition(bead):                                               │ │
│  │      go func():                                                            │ │
│  │          result = execShell(bead.condition)  // May block!                 │ │
│  │          if result.exitCode == 0:                                          │ │
│  │              expandTemplate(bead.on_true)                                  │ │
│  │          else:                                                             │ │
│  │              expandTemplate(bead.on_false)                                 │ │
│  │          closeBead(bead)                                                   │ │
│  │          signalMainLoop()                                                  │ │
│  │      ()                                                                    │ │
│  │                                                                            │ │
│  └────────────────────────────────────────────────────────────────────────────┘ │
│                                                                                  │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Orchestrator Startup

When `meow run <template>` is called:

```
1. Load config from .meow/config.toml
2. Acquire lock file (.meow/orchestrator.lock)
3. Load or create agent state from .meow/agents.json
4. Load existing beads from .beads/
5. If resuming:
   a. Load state from .meow/state/orchestrator.json
   b. Validate agents match tmux sessions
   c. Continue main loop
6. If fresh start:
   a. Bake template into initial beads
   b. Spawn initial agent(s)
   c. Enter main loop
```

### Orchestrator Health

The orchestrator maintains its own health:

```
.meow/state/
├── orchestrator.json     # Current state
├── orchestrator.pid      # Process ID
├── heartbeat.json        # Updated every 30s
└── trace.jsonl           # Execution log
```

External monitoring can:
- Check PID file for process liveness
- Check heartbeat age for stuck detection
- Parse trace for debugging

### Bead Readiness

A bead is "ready" when:
1. Status is `open`
2. All beads in its `needs` array are `closed`
3. Its `assignee` agent exists (or assignee is null for orchestrator beads)

**Priority ordering** (when multiple beads are ready):
1. Orchestrator beads (condition, expand, code) before task beads
2. Earlier creation time
3. Lower bead ID (lexicographic)

This ensures orchestrator actions complete before agents are given more work.

### How Claude Closes Beads

Claude uses `meow close <bead-id>` (or `bd close` with MEOW integration) to close a task bead. This:
1. Updates bead status to `closed`
2. Triggers orchestrator to check what's next

The orchestrator can detect this via:
- **File watch** on `.beads/issues.jsonl` (preferred—instant response)
- **Polling** bead status every 100ms (fallback)

### The Stop-Hook Pattern (Ralph Wiggum Loop)

Claude Code's stop-hook enables the persistent iteration loop:

```json
// .claude/settings.json
{
  "hooks": {
    "Stop": [
      {"type": "command", "command": "meow prime --format prompt"}
    ]
  }
}
```

When Claude finishes a task:
1. Claude runs `meow close <bead-id>`
2. Claude's stop-hook fires
3. `meow prime` shows next task
4. Claude continues automatically

---

## Template System

### Template Format

Templates are TOML files in `.meow/templates/`:

```toml
[meta]
name = "implement-tdd"
description = "TDD implementation workflow"
tags = ["implementation", "tdd"]
version = "1.0.0"

[variables]
task_id = { required = true, description = "The task bead ID to implement" }

[[steps]]
id = "load-context"
type = "task"
title = "Load context and understand the task"
description = |
  Read the task bead and understand what needs to be done:

  ```bash
  bd show {{task_id}}
  ```

  Identify relevant source files and understand the codebase structure.
  Update notes with your understanding before proceeding.

[[steps]]
id = "write-tests"
type = "task"
title = "Write failing tests"
description = |
  Write tests that define the expected behavior.
  Tests MUST fail at this point (you haven't implemented yet).

  If tests pass, you're either:
  - Testing something already implemented (revise tests)
  - Tests are wrong (fix them)
needs = ["load-context"]

[[steps]]
id = "verify-fail"
type = "condition"
condition = "! npm test 2>/dev/null"  # Expect tests to fail
on_true:
  inline: []  # Good, tests fail as expected
on_false:
  inline:
    - { id: "fix-tests", type: "task", title: "Tests passed unexpectedly - revise test strategy" }
needs = ["write-tests"]

[[steps]]
id = "implement"
type = "task"
title = "Implement code to make tests pass"
description = "Write the minimum code necessary to make tests pass. No gold-plating."
needs = ["verify-fail"]

[[steps]]
id = "verify-pass"
type = "task"
title = "Run tests and verify they pass"
needs = ["implement"]

[[steps]]
id = "commit"
type = "task"
title = "Commit changes"
needs = ["verify-pass"]
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
| `{{parent_bead}}` | Parent bead ID (if expanded from another bead) |
| `{{expand_depth}}` | Nesting depth of template expansion |
| `{{workflow_id}}` | Root workflow ID |

### Template Baking

When `meow run <template>` is called or an `expand` bead is processed:

1. Load template TOML
2. Validate required variables are provided
3. Substitute all `{{variable}}` references
4. Create bead for each step
5. Set up `needs` dependencies (translated to bead IDs)
6. Insert beads into storage

### Inline vs Template Expansion

`on_true` and `on_false` can specify:

```yaml
# Reference an external template
on_true:
  template: "handle-success"
  variables: { result: "passed" }

# Define steps inline
on_false:
  inline:
    - { id: "log-failure", type: "code", code: "echo 'Failed' >> log.txt" }
    - { id: "retry", type: "expand", template: "retry-logic" }

# Empty (no additional steps)
on_true:
  inline: []
```

### Template Inheritance (Future)

Templates can extend other templates:

```toml
[meta]
name = "implement-tdd-strict"
extends = "implement-tdd"
# Overrides or additions here
```

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

### Agent Identity

Every agent has a unique ID set via environment:

```bash
MEOW_AGENT=claude-1    # Primary identifier
MEOW_ROLE=worker       # Optional role hint
MEOW_WORKFLOW=wf-abc   # Workflow this agent belongs to
```

The agent reads its ID from `$MEOW_AGENT` when executing `meow prime` or `meow close`.

### Agent Discovery

```bash
meow agents              # List all agents
meow agents --active     # Only active agents
meow agents --json       # Machine-readable
```

Output:
```
AGENT       STATUS        TMUX SESSION      CURRENT BEAD
claude-1    active        meow-claude-1     bd-task-001
claude-2    checkpointed  -                 -
claude-3    stopped       -                 -
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
    "workdir": "/data/projects/myapp/worktrees/claude-2",
    "status": "active",
    "checkpoint": null,
    "current_bead": "bd-task-002",
    "started_at": "2026-01-06T12:00:00Z",
    "last_activity": "2026-01-06T12:05:00Z"
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

### Handoff Notes System

When an agent hands off (stops with notes for the next session):

1. **Agent writes notes** to the current bead or a handoff bead:
   ```bash
   bd update bd-task-001 --notes "PROGRESS: Completed steps 1-3
   NEXT: Continue with step 4
   CONTEXT: Using the new API from auth-v2 branch"
   ```

2. **Agent triggers handoff**:
   ```bash
   meow handoff --notes "Stopping for context refresh"
   ```
   This creates a handoff record and stops the session.

3. **New session sees notes** via `meow prime`:
   ```
   === HANDOFF NOTES ===
   PROGRESS: Completed steps 1-3
   NEXT: Continue with step 4
   CONTEXT: Using the new API from auth-v2 branch
   === END NOTES ===

   Your next task: bd-task-001 "Implement feature X"
   ...
   ```

### Hook Integration

MEOW integrates with Claude Code hooks:

```json
// .claude/settings.json
{
  "hooks": {
    "SessionStart": [
      {"type": "command", "command": "meow prime --hook"}
    ],
    "Stop": [
      {"type": "command", "command": "meow prime --format prompt"}
    ]
  }
}
```

**SessionStart hook** (`meow prime --hook`):
1. Reads session metadata from stdin (session_id, transcript_path)
2. Persists session ID for checkpoint/resume
3. Outputs agent context and current task

**Stop hook** (`meow prime --format prompt`):
1. Outputs next task as prompt
2. Enables automatic continuation

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

### Prime Output Format

`meow prime` outputs context for the agent:

```
=== MEOW AGENT CONTEXT ===
Agent: claude-1
Workflow: wf-abc123
Session: session_xyz789

=== HANDOFF NOTES ===
(Previous session notes if any)

=== CURRENT TASK ===
Bead: bd-task-001
Title: Implement user registration endpoint
Status: in_progress

Instructions:
1. Read the existing auth module at src/auth/
2. Create new endpoint following existing patterns
...

=== WORKFLOW PROGRESS ===
Completed: 3/7 steps
Next after this: bd-task-002 "Write tests"

=== COMMANDS ===
- meow close bd-task-001           # Complete this task
- meow close bd-task-001 --notes   # Complete with handoff notes
- meow status                      # Check workflow status
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

The full call semantics—pause parent, spawn child, run template, cleanup, resume:

```toml
# .meow/templates/call.toml
[meta]
name = "call"
description = "Spawn child agent for sub-workflow, then resume parent"
ephemeral = true  # All these steps are operational machinery

[variables]
parent = { required = true, description = "Parent agent ID" }
child = { required = true, description = "Child agent ID" }
template = { required = true, description = "Template for child to execute" }
template_vars = { default = {}, description = "Variables for child template" }
use_worktree = { default = false }

# --- Save parent session (replaces checkpoint primitive) ---
[[steps]]
id = "save-parent-session"
type = "code"
code = |
  meow session-id --agent {{parent}}
outputs:
  session_id: stdout  # Captured for resume

[[steps]]
id = "stop-parent"
type = "stop"
agent = "{{parent}}"
needs = ["save-parent-session"]

# --- Setup child environment (optional worktree) ---
[[steps]]
id = "setup-worktree"
type = "code"
needs = ["stop-parent"]
code = |
  if [ "{{use_worktree}}" = "true" ]; then
      git worktree add -b meow/{{child}} .meow/worktrees/{{child}} HEAD
      echo ".meow/worktrees/{{child}}"
  else
      pwd
  fi
outputs:
  workdir: stdout  # Captured for start

# --- Start child ---
[[steps]]
id = "start-child"
type = "start"
agent = "{{child}}"
workdir = "{{setup-worktree.outputs.workdir}}"  # Uses captured output!
needs = ["setup-worktree"]

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
code = |
  if [ "{{use_worktree}}" = "true" ]; then
      git worktree remove .meow/worktrees/{{child}} || true
      git branch -d meow/{{child}} || true
  fi

# --- Resume parent (uses start with resume_session) ---
[[steps]]
id = "resume-parent"
type = "start"
agent = "{{parent}}"
workdir = "."
resume_session = "{{save-parent-session.outputs.session_id}}"  # Uses captured session!
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

### Example: `agent-patrol` Template

A persistent loop pattern (like Gas Town's patrol agents):

```toml
# .meow/templates/agent-patrol.toml
[meta]
name = "agent-patrol"
description = "Continuous patrol loop with periodic checks"

[variables]
agent = { required = true }
check_interval = { default = "5m" }
patrol_action = { required = true, description = "Template to run each patrol" }

[[steps]]
id = "patrol-cycle"
type = "expand"
template = "{{patrol_action}}"
variables = { agent = "{{agent}}" }

[[steps]]
id = "cooldown"
type = "condition"
needs = ["patrol-cycle"]
condition = "sleep {{check_interval}} && exit 0"
on_true:
  inline: []

[[steps]]
id = "context-check"
type = "expand"
needs = ["cooldown"]
template = "context-check"
variables = { agent = "{{agent}}", threshold = 70 }

[[steps]]
id = "continue-patrol"
type = "condition"
needs = ["context-check"]
condition = "exit 0"  # Always true - loop continues
on_true:
  template = "agent-patrol"
  variables = { agent = "{{agent}}", check_interval = "{{check_interval}}", patrol_action = "{{patrol_action}}" }
```

### Example: `retry-with-backoff` Template

Error handling with exponential backoff:

```toml
# .meow/templates/retry-with-backoff.toml
[meta]
name = "retry-with-backoff"
description = "Retry a task with exponential backoff"

[variables]
task_template = { required = true }
task_vars = { default = {} }
max_retries = { default = 3 }
initial_delay = { default = "10s" }

[[steps]]
id = "attempt"
type = "expand"
template = "{{task_template}}"
variables = "{{task_vars}}"
on_error:
  inline:
    - { id = "check-retries", type = "condition",
        condition = "test $MEOW_RETRY_COUNT -lt {{max_retries}}",
        on_true = { template = "backoff-delay", variables = { delay = "{{initial_delay}}" } },
        on_false = { template = "escalate-failure" } }
```

### Example: `parallel-work` Template

Parallel execution pattern:

```toml
# .meow/templates/parallel-work.toml
[meta]
name = "parallel-work"
description = "Execute multiple tasks in parallel with different agents"

[variables]
tasks = { required = true, type = "array" }  # Array of {template, vars, agent}

[[steps]]
id = "spawn-workers"
type = "code"
code = |
  # This would be handled specially by orchestrator
  # to create multiple start beads dynamically
  echo "Spawning {{tasks | length}} workers"

# Each task becomes a parallel branch
# {{#foreach task in tasks}}
[[steps]]
id = "work-{{task.agent}}"
type = "expand"
template = "{{task.template}}"
variables = "{{task.vars}}"
assignee = "{{task.agent}}"
needs = ["spawn-workers"]
# {{/foreach}}

[[steps]]
id = "join"
type = "task"
title = "Merge results from parallel workers"
needs = ["work-*"]  # Wait for all work-* beads
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

A complete outer loop with context management and **validated task outputs**:

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
    { id = "select-task", type = "task",
      title = "Select next task to work on",
      description = "Run bv --robot-triage and pick the highest priority task",
      outputs = {
        required = [
          { name = "work_bead", type = "bead_id", description = "The bead to implement" }
        ]
      }
    },
    { id = "implement", type = "expand", template = "call", needs = ["select-task"],
      ephemeral = true,
      variables = {
        parent = "{{agent}}",
        child = "{{agent}}-worker",
        template = "implement-tdd",
        template_vars = { work_bead = "{{select-task.outputs.work_bead}}" },
        use_worktree = true
      }
    },
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

**Key features**:
- `select-task` requires a `work_bead` output (validated as `bead_id`)
- `implement` uses `{{select-task.outputs.work_bead}}` to pass the selection
- `ephemeral = true` on the call template to keep work bead history clean

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

### Trace Visualization

The trace can be visualized as a DAG:

```
meow trace --graph

check-work ─┬─► select-task ─► implement ─┬─► checkpoint-parent
            │                             ├─► stop-parent
            │                             ├─► setup-worktree
            │                             ├─► start-child
            │                             └─► child-work ─┬─► load-context
            │                                             ├─► write-tests
            │                                             ├─► implement
            │                                             ├─► validate
            │                                             └─► commit
            │
            └─► (on_false) ─► summary ─► done
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
| `meow pause` | Pause orchestrator (no new beads picked up) |
| `meow resume` | Resume paused orchestrator |
| `meow stop` | Gracefully stop orchestrator |

### Agent Commands (Claude uses these)

| Command | Description |
|---------|-------------|
| `meow prime` | Show next task for current agent (includes required outputs) |
| `meow close <bead>` | Close a task bead (fails if required outputs missing) |
| `meow close <bead> --notes "..."` | Close with handoff notes |
| `meow close <bead> --output key=value` | Close with validated output |
| `meow close <bead> --output-json '{...}'` | Close with JSON outputs |
| `meow handoff` | Write notes and request context refresh |
| `meow signal <name>` | Send inter-agent signal |
| `meow context-usage` | Report context window usage |
| `meow session-id` | Get current Claude session ID (for checkpoint/resume) |

### Debug Commands

| Command | Description |
|---------|-------------|
| `meow agents` | List all agents and their states |
| `meow agents --active` | Only active agents |
| `meow beads` | List all beads with status |
| `meow beads --ready` | Only ready beads |
| `meow beads --include-ephemeral` | Include ephemeral beads in listing |
| `meow trace` | Show execution trace/history |
| `meow trace --follow` | Stream trace in real-time |
| `meow peek <agent>` | Show agent's current output |
| `meow attach <agent>` | Attach to agent's tmux session |
| `meow nudge <agent> <msg>` | Send interrupt + message to agent |
| `meow validate <template>` | Validate template syntax |
| `meow clean --ephemeral` | Clean up completed ephemeral beads |

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
    ├── orchestrator.pid  # Process ID
    ├── orchestrator.lock # Lock file
    ├── heartbeat.json    # Health check
    └── trace.jsonl       # Execution trace log

.beads/
├── issues.jsonl         # All beads (tasks, conditions, etc.)
└── beads.db             # SQLite cache
```

### Crash Recovery

If the orchestrator crashes:
1. On restart, load `.meow/state/orchestrator.json`
2. Rebuild agent states from `.meow/agents.json`
3. Reconcile agent states with tmux reality
4. Reset `in_progress` beads from dead agents to `open`
5. Find all open beads from `.beads/`
6. Resume main loop

**Invariant**: All state is persisted; no in-memory-only state.

---

## Error Handling

### Error Types

| Error Type | Source | Handling |
|------------|--------|----------|
| **Bead error** | Task execution fails | `on_error` field on bead |
| **Condition error** | Shell command fails unexpectedly | Log and treat as `on_false` |
| **Agent error** | tmux/Claude crash | Orchestrator detects, respawns |
| **Template error** | Invalid template syntax | Fail baking, report to user |
| **System error** | Disk full, network down | Pause workflow, alert |

### Bead-Level Error Handling

Every bead can specify `on_error`:

```yaml
id: "bd-task-001"
type: task
on_error:
  action: "retry"          # retry | skip | abort | inject
  max_retries: 3
  retry_delay: "30s"
  inject_template: "triage-error"  # If action=inject
```

### Error Actions

| Action | Behavior |
|--------|----------|
| `retry` | Retry the bead up to `max_retries` times |
| `skip` | Mark bead as closed (with error note), continue |
| `abort` | Stop the workflow |
| `inject` | Expand `inject_template` for error triage |

### Agent Crash Detection

The orchestrator monitors agents:

```go
func monitorAgents() {
    for agent := range agents {
        if agent.status == "active" {
            if !tmux.SessionExists(agent.tmuxSession) {
                handleAgentCrash(agent)
            } else if agent.lastActivity.Before(time.Now().Add(-stuckThreshold)) {
                handleStuckAgent(agent)
            }
        }
    }
}
```

**Crash recovery**:
1. Detect session died
2. Check if agent had in-progress bead
3. If yes:
   - Mark bead as `open` (reset from `in_progress`)
   - Respawn agent
   - Agent sees bead via `meow prime`, continues

**Stuck detection**:
1. No activity for `stuck_threshold` (default: 15min)
2. Options:
   - Nudge: Send interrupt + message
   - Kill and respawn
   - Escalate to human

### Stuck Agent Nudging

```bash
meow nudge claude-1 "Are you stuck? Current task: bd-task-001"
```

This sends an interrupt to the Claude session and injects a message.

### Error Logging

All errors are logged to `.meow/state/errors.jsonl`:

```json
{"timestamp":"...","type":"agent_crash","agent":"claude-1","bead":"bd-task-001","details":"tmux session died"}
{"timestamp":"...","type":"bead_error","bead":"bd-code-001","exit_code":1,"stderr":"git: not a repository"}
```

---

## Crash Recovery

### Orchestrator Crash Recovery

If the orchestrator crashes:

1. **On restart**, check for existing state:
   ```bash
   if [ -f .meow/state/orchestrator.json ]; then
       meow resume  # Resume from state
   else
       meow run <template>  # Fresh start
   fi
   ```

2. **Load state**:
   - Agent states from `.meow/agents.json`
   - All beads from `.beads/`
   - Orchestrator position from `.meow/state/orchestrator.json`

3. **Reconcile with reality**:
   - Check each "active" agent's tmux session exists
   - If session dead, mark agent stopped
   - Reset any `in_progress` beads from dead agents to `open`

4. **Resume main loop**

### Agent Crash Recovery

When an agent crashes mid-task:

1. **Workspace preserved**: Git worktree still exists with any changes
2. **Bead state preserved**: The bead records progress
3. **On respawn**:
   - `meow prime` shows the same task
   - Notes field contains previous progress
   - Agent can see uncommitted changes in workspace

### Preventing State Corruption

**Atomic writes**: All state files are written atomically:
```go
func atomicWrite(path string, data []byte) error {
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, data, 0644); err != nil {
        return err
    }
    return os.Rename(tmp, path)
}
```

**Lock files**: Prevent concurrent orchestrator instances:
```
.meow/orchestrator.lock
```

**Journaling**: Critical operations logged before execution:
```json
{"op":"expand_template","bead":"bd-expand-001","template":"call","status":"starting"}
{"op":"expand_template","bead":"bd-expand-001","template":"call","status":"complete","created":["bd-ckpt-001","bd-stop-001"]}
```

---

## Debugging and Observability

### Trace Log

Every orchestrator action is logged to `.meow/state/trace.jsonl`:

```json
{"ts":"2026-01-06T12:00:00Z","action":"start","bead":"bd-start-001","agent":"claude-1"}
{"ts":"2026-01-06T12:00:05Z","action":"prime","agent":"claude-1","bead":"bd-task-001"}
{"ts":"2026-01-06T12:05:00Z","action":"close","bead":"bd-task-001","agent":"claude-1"}
{"ts":"2026-01-06T12:05:01Z","action":"condition_start","bead":"bd-cond-001"}
{"ts":"2026-01-06T12:05:02Z","action":"condition_result","bead":"bd-cond-001","exit_code":0}
{"ts":"2026-01-06T12:05:02Z","action":"expand","bead":"bd-cond-001","template":"work-iteration","created":["bd-select-001","bd-impl-001"]}
```

### Status Commands

```bash
meow status              # Overall workflow status
meow status --verbose    # Detailed status with all beads
meow agents              # Agent status
meow beads               # All beads with status
meow beads --ready       # Only ready beads
meow trace               # Recent trace entries
meow trace --follow      # Stream trace in real-time
```

### Status Output

```
$ meow status

MEOW Workflow: wf-abc123
Started: 2026-01-06T10:00:00Z
Duration: 2h 5m

AGENTS
  claude-1    active        bd-task-005    meow-claude-1
  claude-2    checkpointed  -              -

BEADS
  Total: 15
  Closed: 8
  In Progress: 1
  Open: 6

CURRENT
  claude-1 is working on: bd-task-005 "Write integration tests"

NEXT
  bd-task-006 "Update documentation" (waiting on bd-task-005)
```

### Debug Mode

```bash
MEOW_DEBUG=1 meow run work-loop
```

Enables:
- Verbose logging to stderr
- Shell command output shown
- Pause before each bead for inspection (`MEOW_DEBUG=step`)

### Tmux Session Inspection

```bash
meow peek claude-1           # Show Claude's current output
meow attach claude-1         # Attach to Claude's tmux session
```

---

## Integration Patterns

### With Beads (`bd` command)

MEOW builds on top of the beads issue tracker. Integration:

```bash
# MEOW uses beads for storage
export BEADS_DIR=.beads

# Agents use bd commands for bead operations
bd show bd-task-001          # View bead
bd update bd-task-001 --notes "Progress..."
bd close bd-task-001         # Or meow close (which wraps bd close)

# MEOW extends beads with workflow metadata
bd list --type=task --label=meow:workflow-abc
```

### With Git

MEOW integrates with git for:

1. **Worktrees**: Parallel agents can work in isolated worktrees
   ```bash
   # code bead in call template
   git worktree add -b meow/claude-2 ~/worktrees/claude-2 HEAD
   ```

2. **Commits**: Agents commit work to preserve progress
   ```bash
   # In task instructions
   git add -A && git commit -m "feat: implement X (bd-task-001)"
   ```

3. **Sync**: Beads are git-tracked for collaboration
   ```bash
   bd sync  # Commits and pushes .beads/
   ```

### With Claude Code

MEOW is designed for Claude Code integration:

1. **Hooks**: SessionStart and Stop hooks for automatic continuation
2. **Resume**: `checkpoint` + `resume` use `claude --resume`
3. **Session ID**: Captured from Claude Code's session metadata
4. **Workspace**: Each agent runs in its own directory with `.claude/` config

### With External Systems

Via `code` and `condition` beads:

```yaml
# Slack notification
type: code
code: |
  curl -X POST $SLACK_WEBHOOK -d '{"text": "Workflow complete!"}'

# GitHub PR creation
type: code
code: |
  gh pr create --title "{{pr_title}}" --body "{{pr_body}}"

# Wait for external API
type: condition
condition: |
  status=$(curl -s https://api.example.com/job/$JOB_ID | jq -r .status)
  [ "$status" = "complete" ]
```

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

### 5. Resource Contention

When multiple agents might conflict:

```toml
[[steps]]
id = "exclusive-db-migration"
type = "task"
lock = "db-schema"  # Only one agent can hold this lock
lock_timeout = "30m"
```

Locks are advisory and managed via `.meow/locks/`:

```json
{
  "db-schema": {
    "holder": "claude-2",
    "acquired": "2026-01-06T12:00:00Z",
    "expires": "2026-01-06T12:30:00Z"
  }
}
```

---

## Implementation Phases

### Phase 1: Core Orchestrator (MVP)

**Goal**: Single-agent execution with all 6 primitives + output binding.

- [ ] Go binary: `meow` CLI
- [ ] Template parser (TOML → beads)
- [ ] Orchestrator main loop
- [ ] All 6 primitive handlers (task, condition, stop, start, code, expand)
- [ ] **Output binding** for `code` beads (stdout, file capture)
- [ ] **Task outputs with validation** (required/optional outputs, type checking)
- [ ] Agent state management
- [ ] Agent identity (MEOW_AGENT env var)
- [ ] tmux session management
- [ ] `meow prime` / `meow close` integration (with `--output` flag)
- [ ] `meow session-id` for checkpoint/resume composition
- [ ] `meow wait-approve` / `meow approve` for human gates
- [ ] Basic crash recovery

**Deliverable**: Can run a simple loop workflow with one agent, with data passing between beads.

### Phase 2: Composition Library

**Goal**: Useful default templates.

- [ ] `refresh` template
- [ ] `handoff` template
- [ ] `call` template (with worktree support)
- [ ] `context-check` template
- [ ] `work-loop` template
- [ ] Template documentation

**Deliverable**: Users can run complex single-agent workflows out of the box.

### Phase 3: Robustness

**Goal**: Production-ready single-agent.

- [ ] Comprehensive error handling
- [ ] Agent crash detection and recovery
- [ ] Stuck agent detection and nudging
- [ ] Detailed logging and tracing
- [ ] `meow status` / `meow agents` / `meow trace`
- [ ] `meow peek` / `meow attach` / `meow nudge`
- [ ] Condition timeout handling
- [ ] Atomic state writes
- [ ] Lock files for orchestrator
- [ ] Integration tests
- [ ] Documentation

**Deliverable**: Reliable single-agent orchestration.

### Phase 4: Parallelization (Future)

**Goal**: Multi-agent parallel execution.

- [ ] `foreach` directive
- [ ] Parallel `start` handling
- [ ] Join point detection
- [ ] Worktree management
- [ ] Branch merge conflict handling
- [ ] Resource locks
- [ ] Inter-agent signaling

**Deliverable**: Multiple Claude instances working in parallel.

### Phase 5: Advanced Features (Future)

**Goal**: Enterprise-ready orchestration.

- [ ] Web dashboard for monitoring
- [ ] Metrics and alerting
- [ ] Template marketplace
- [ ] Multi-project workflows
- [ ] Remote agent execution
- [ ] Cost tracking and budgets
- [ ] Approval workflows with RBAC
- [ ] Audit logging

**Deliverable**: Full-featured workflow orchestration platform.

---

## Summary

MEOW Stack MVP is built on **6 primitive bead types** + **3 key capabilities**:

### The 6 Primitives

| # | Primitive | Purpose |
|---|-----------|---------|
| 1 | `task` | Claude-executed work (with optional validated outputs) |
| 2 | `condition` | Branching, looping, and waiting (can block for gates) |
| 3 | `stop` | Kill agent session |
| 4 | `start` | Spawn agent (with optional session resume) |
| 5 | `code` | Arbitrary shell execution (with output capture) |
| 6 | `expand` | Template composition (with ephemeral option) |

### Key Capabilities

| Capability | Description |
|------------|-------------|
| **Output Binding** | `code` beads capture stdout/files; subsequent beads reference via `{{bead.outputs.field}}` |
| **Task Outputs** | Tasks can require validated outputs; Claude provides via `meow close --output` |
| **Ephemeral Beads** | Template steps can be marked ephemeral for auto-cleanup after completion |

### Composition Examples

- `checkpoint` → `code` bead that runs `meow session-id`
- `resume` → `start` bead with `resume_session` parameter
- Worktree path passing → `code` captures path, `start` uses `{{code.outputs.path}}`
- Task selection → `task` requires `work_bead` output, `expand` uses `{{task.outputs.work_bead}}`

Everything else—`refresh`, `handoff`, `call`, loops, context management, human approval gates—is **template composition** using these primitives.

The orchestrator is a simple state machine. The templates are the programs. Users have complete control.

> **Remember**: You are one session in a durable workflow. Do your step well, leave good notes, and trust the system to continue.

---

## Implementation Tracking

For detailed implementation tasks, acceptance criteria, and progress tracking:

- **[IMPLEMENTATION-PLAN.md](IMPLEMENTATION-PLAN.md)** — Comprehensive architectural specification with 11 epics and ~50 tasks, full design decisions, and rationale
- **[IMPLEMENTATION-BEADS.yaml](IMPLEMENTATION-BEADS.yaml)** — Beads-importable task breakdown for tracking progress

### Import Tasks to Beads

```bash
# Import the implementation tasks
bd import -i docs/IMPLEMENTATION-BEADS.yaml

# View epics
bd list --type=epic

# View ready tasks
bd ready
```

### Implementation Phases Summary

| Phase | Epics | Description |
|-------|-------|-------------|
| **Phase 1** | E1, E2, E3 | Foundation, templates, orchestrator core |
| **Phase 2** | E4, E5 | Primitive handlers (6 types), agent management |
| **Phase 3** | E6, E7, E8 | Output binding, task outputs, ephemeral beads |
| **Phase 4** | E9, E10, E11 | Default templates, CLI, testing |

MVP completion: End of Phase 4 (~50 tasks total)

**Key architectural decisions**:
- 6 primitives instead of 8 (checkpoint/resume composed from code/start)
- Output binding enables dynamic data flow between beads
- Validated task outputs ensure reliable Claude → orchestrator communication
- Ephemeral beads keep work history clean while tracking workflow steps
