# MEOW Architecture

**MEOW** (Meow Executors Orchestrate Work) is terminal agent orchestration without the framework tax.

> **The Makefile of agent orchestration.** No Python. No cloud. No magic. Just tmux, TOML, and a binary. Works with Claude Code, Aider, or any terminal agent.

---

## Positioning

The AI agent orchestration space is crowded: LangChain, LangGraph, CrewAI, Claude-Flow, AutoGen, cloud-managed agents. These frameworks offer sophisticated features—memory systems, vector databases, MCP tools, visual builders, managed infrastructure.

**MEOW takes the opposite approach.**

| Framework Approach | MEOW Approach |
|--------------------|---------------|
| Python SDK with dependencies | Single Go binary |
| Cloud services and databases | TOML files on disk |
| Agents as API endpoints | Agents as terminal processes |
| Framework-specific agent code | Any terminal agent, unchanged |
| Visual builders, dashboards | `git diff`-able TOML templates |
| Sophisticated memory/RAG | Bring your own (or don't) |

**MEOW is for developers who:**
- Use Claude Code or Aider in the terminal
- Want to orchestrate multi-agent workflows
- Don't want to learn a framework ecosystem
- Value understanding exactly what their system does
- Want workflows version-controlled alongside code

**MEOW is NOT for you if you need:**
- Turnkey agent capabilities and memory systems
- Production observability dashboards
- Managed cloud deployment and scaling
- Visual workflow builders

This is a deliberate tradeoff. MEOW coordinates; everything else is your choice.

---

## Core Principles

### 1. Dumb Orchestrator, Smart Templates

> **The orchestrator is generic. All agent-specific logic lives in templates.**

The orchestrator knows:
- How to execute the 7 executor types
- How to route events between waiters
- How to manage tmux sessions
- How to persist workflow state

The orchestrator does NOT know:
- What Claude is or how its hooks work
- What "agent-stopped" means or how to respond
- How to nudge a stuck agent
- What your task tracking system is

**Why this matters:** MEOW works with any terminal agent. Claude Code, Aider, Cursor, custom agents—they all work because the orchestrator doesn't embed agent-specific assumptions.

### 2. The Propulsion Principle

> **The orchestrator drives agents forward. Agents don't poll—they receive prompts.**

MEOW is a steam engine. The orchestrator is the engineer. Agents are pistons. The orchestrator injects prompts directly into agents via tmux. Agents work until they signal completion with `meow done`.

```
Orchestrator                           Agent (in tmux)
────────────                           ───────────────
  spawn → creates session                  [waiting]
  inject prompt via send-keys →          [working...]
                                         [working...]
                                         [working...]
  ← meow done                            [complete!]
  inject next prompt →                   [working...]
```

### 3. Single Writer Principle

> **The orchestrator is the sole writer to workflow state.**

All state mutations flow through the orchestrator's mutex-protected methods. The IPC handler delegates to the orchestrator—it never writes state directly.

```
Agent → meow done → IPC Server → Orchestrator.HandleStepDone() → Save()
                                       ↓
                                  MUTEX HELD
```

This eliminates race conditions. Lost updates are impossible.

### 4. Events, Not Hooks

> **Agent-specific hook systems translate to generic events.**

The orchestrator doesn't know about Claude's Stop hook or Aider's callbacks. It knows about events:

```bash
# Claude's Stop hook (configured via template, not orchestrator)
meow event agent-stopped

# Orchestrator sees this as a generic event
# Templates decide how to respond
```

This keeps the orchestrator agent-agnostic. The "Ralph Wiggum" persistence pattern (nudging agents that stop unexpectedly) is implemented entirely in templates via event loops—not baked into orchestrator code.

### 5. Persistent State, Best-Effort Resume

> **Workflow state is persisted. On crash, MEOW can resume orchestration.**

MEOW persists workflow state to YAML files. On crash:
1. Reload workflow state
2. Check which agents are still alive
3. Reset interrupted orchestrator steps to pending
4. Resume from where orchestration left off

**What MEOW can recover:** The step dependency graph, which steps are done, captured outputs, agent session existence.

**What MEOW cannot recover:** The effects of interrupted work. If an agent was mid-task when you crashed, MEOW doesn't know if it wrote half a file or corrupted something.

MEOW is not a durable execution engine like Temporal. Think of it like Airflow: it tracks task state, but if a task was mid-execution, recovery means "re-run and hope it's idempotent."

---

## MEOW is a Coordination Language

Templates are **programs** that coordinate agents. They are not task lists, not tickets, not issues. They are executable specifications of how work flows through a system.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         MEOW MENTAL MODEL                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Templates = Programs (static, version-controlled TOML files)               │
│  Steps = Instructions within a program                                      │
│  Executors = Who runs each instruction                                      │
│  Outputs = Data flowing between steps                                       │
│  Runs = Running program instances (runtime state in YAML)                   │
│                                                                             │
│  MEOW doesn't care about your task tracker.                                 │
│  MEOW coordinates agents through programmable workflows.                    │
│  Your workflow tells agents how to find and complete work.                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Task Tracking Agnosticism

MEOW makes **zero assumptions** about how you track work:

| Your System | How MEOW Handles It |
|-------------|---------------------|
| Beads | Workflow prompts agent to run `bd ready` |
| Jira | Workflow prompts agent to query Jira API |
| GitHub Issues | Workflow prompts agent to use `gh issue list` |
| Markdown TODOs | Workflow prompts agent to read `TODO.md` |
| **None** | Workflow can work without any task system |

The **workflow template** encodes how to interact with external systems. MEOW just executes the workflow.

### Agent Agnosticism

MEOW makes **zero assumptions** about which AI agent you use:

| Agent | How MEOW Handles It |
|-------|---------------------|
| Claude Code | Adapter provides start command; templates configure hooks |
| Aider | Adapter provides start command; templates configure callbacks |
| Custom Agent | User writes adapter config and hook templates |

The **adapter system** encapsulates agent-specific runtime behavior (how to start, stop, inject prompts). Agent configuration (like event hooks) belongs in **library templates**, not adapters.

---

## The 7 Executors

Everything in MEOW is a **Step**. A step has an executor that determines who runs it and how.

There are exactly 7 executors—no more, no less:

### Orchestrator Executors (run internally, fast)

| Executor | Purpose | Completes When |
|----------|---------|----------------|
| `shell` | Run a shell command | Command exits |
| `spawn` | Start an agent in tmux | Session is running |
| `kill` | Stop an agent's tmux session | Session is terminated |
| `expand` | Inline another template | Expanded steps are inserted |
| `branch` | Conditional execution | Condition evaluated, branch expanded |
| `foreach` | Iterate over a list | All iterations complete (implicit join) |

### External Executors (wait for external signal)

| Executor | Purpose | Completes When |
|----------|---------|----------------|
| `agent` | Assign work to an agent | Agent runs `meow done` |

### Why Only 7?

This is the minimal set that enables all coordination patterns:

| Pattern | Executors Used |
|---------|----------------|
| Sequential work | `agent` with `needs` |
| Conditional logic | `branch` |
| Loops | `branch` + recursive `expand` |
| Static parallel | Multiple `agent` steps with same `needs` |
| Dynamic parallel | `foreach` with parallel iterations |
| Agent lifecycle | `spawn`, `kill` |
| Setup/teardown | `shell` |
| Human approval | `branch` + `meow await-approval` |
| Composition | `expand` |

**Gate is NOT an executor.** Human approval gates are implemented as `branch` steps with `condition = "meow await-approval <gate-id>"`. This keeps the executor count minimal and makes approval mechanisms user-customizable.

**Gates use the event system.** The `meow approve` and `meow reject` commands emit `gate-approved` and `gate-rejected` events respectively. The `meow await-approval` command waits for either event. This unifies gate handling with the existing event infrastructure—the orchestrator doesn't have special approval logic.

---

## Step Lifecycle

```
pending ──► running ──► completing ──► done
              │             │
              │             └──► (back to running if validation fails)
              │
              └──► failed
```

- `pending`: Waiting for dependencies or orchestrator attention
- `running`: Currently being executed
- `completing`: Agent called `meow done`, orchestrator is handling transition
- `done`: Successfully completed, outputs captured
- `failed`: Execution failed

The `completing` state is critical—it prevents race conditions during step transitions.

---

## Async Execution Model

Both **branch** and **shell** execute commands **asynchronously** in goroutines. This enables parallel execution patterns:

```toml
# Both steps start in the same tick after spawn completes
[[steps]]
id = "main-work"
executor = "agent"
needs = ["spawn"]

[[steps]]
id = "monitor"
executor = "branch"
needs = ["spawn"]
condition = "meow await-event agent-stopped --timeout 30s"
```

Without async execution, `monitor` would block for 30 seconds before `main-work` could start—defeating the purpose of parallel monitoring.

**Shell is syntactic sugar over branch.** Internally, `handleShell()` converts the config and delegates to `handleBranch()`. This means shell steps honor the DAG parallelism contract—shell steps with the same dependencies execute in parallel.

**Completion serialization:** While conditions run in parallel, their completions serialize through the workflow mutex to ensure consistency. This provides correctness at the cost of completion throughput (~100-200 completions/second).

---

## Data Flow Between Steps

Steps communicate through **outputs**. This works like GitHub Actions—pure data flow.

### Capturing Outputs

**From shell:**
```toml
[[steps]]
id = "get-branch"
executor = "shell"
command = "git branch --show-current"

[steps.outputs]
branch = { source = "stdout" }
```

**From agents:**
```toml
[[steps]]
id = "select-task"
executor = "agent"
prompt = "Pick a task from the backlog"

[steps.outputs]
task_id = { required = true, type = "string" }
```

### Referencing Outputs

Use `{{step_id.outputs.field}}` syntax:

```toml
prompt = "Implement task {{select-task.outputs.task_id}}"
```

### Output Types

| Type | Validation |
|------|------------|
| `string` | Non-empty string |
| `number` | Parseable as int or float |
| `boolean` | `true` or `false` |
| `json` | Valid JSON |
| `file_path` | File exists in agent's workdir |

---

## Agent Adapters

Adapters encapsulate agent-specific **runtime behavior**—how to start, stop, and inject prompts.

**What adapters handle:**
- Start command (`claude --dangerously-skip-permissions`)
- Resume command (with session ID)
- Prompt injection (keystrokes to send)
- Graceful shutdown sequence

**What adapters DON'T handle:**
- Hook configuration (lives in templates)
- Event handling (lives in templates)
- Persistence patterns (lives in templates)

This keeps adapters minimal and focused. Different workflows can configure different hooks without needing different adapters.

### Example Adapter (claude)

```toml
# ~/.meow/adapters/claude/adapter.toml

[adapter]
name = "claude"
description = "Claude Code CLI agent"

[spawn]
command = "claude --dangerously-skip-permissions"
resume_command = "claude --dangerously-skip-permissions --resume {{session_id}}"
startup_delay = "3s"

[prompt_injection]
pre_keys = ["Escape"]
method = "literal"
post_keys = ["Enter"]

[graceful_stop]
keys = ["C-c"]
wait = "2s"
```

---

## Events

Events are the generic mechanism for agents to communicate with the orchestrator. Unlike step completion (`meow done`), events are fire-and-forget signals.

### Event Primitives

```bash
# Emit an event (from agent hooks)
meow event <event-type> [--data key=value ...]

# Wait for an event (in branch conditions)
meow await-event <event-type> [--filter key=value ...] [--timeout duration]
```

### Event Flow

```
Agent (Claude)                          Orchestrator             Template
──────────────                          ────────────             ────────
     │                                       │                      │
Stop hook fires (Claude                      │                      │
reaches prompt unexpectedly)                 │                      │
     │                                       │                      │
     │  meow event agent-stopped             │                      │
     │──────────────────────────────────────►│                      │
     │                                       │                      │
     │                                       │ Routes event         │
     │                                       │ to waiters           │
     │                                       │─────────────────────►│
     │                                       │                      │
     │                                       │              await-event
     │                                       │              unblocks
```

**Key insight:** The agent's hooks call `meow event` directly. Hook configuration is done by templates—there's no agent-specific logic in the orchestrator.

### Events Are Optional

Events are an **enhancement**, not a requirement. Workflows work fine without them. If hooks aren't configured, `await-event` just times out and the workflow continues via the timeout branch.

---

## Key Patterns

### Ralph Wiggum Pattern (Agent Persistence)

When an agent stops unexpectedly (reaches prompt without calling `meow done`), nudge it to continue.

**How it works:**
1. Configure agent's Stop hook to emit `meow event agent-stopped`
2. Run an event loop in parallel with the main task
3. On `agent-stopped` event, inject a nudge prompt via `fire_forget` mode
4. Check if main task is done; loop if not

**This is implemented entirely in templates** (`lib/agent-persistence.meow.toml`), not the orchestrator. The orchestrator just routes events—it doesn't know what "agent-stopped" means.

### Human Approval Gates

Human gates are `branch` steps with blocking conditions:

```toml
[[steps]]
id = "notify-reviewer"
executor = "shell"
command = """
echo "Review needed. Approve: meow approve {{workflow_id}} review-gate"
"""

[[steps]]
id = "review-gate"
executor = "branch"
condition = "meow await-approval review-gate --timeout 24h"
needs = ["notify-reviewer"]

[steps.on_true]
inline = []  # Continue on approval

[steps.on_false]
template = ".handle-rejection"
```

### Context Monitoring

Monitor agent context usage and trigger `/compact` when high:

```toml
[[steps]]
id = "check-context"
executor = "branch"
condition = "test $(get-context-percent) -ge 85"

[steps.on_true]
template = ".inject-compact"

[steps.on_false]
template = ".continue-monitoring"
```

Uses `fire_forget` mode to inject `/compact` without waiting for response.

---

## Orchestrator Architecture

The orchestrator is the **central authority**. It:
- Owns all prompt injection
- Manages all agent lifecycles
- Handles all state transitions
- Communicates with agents via tmux and IPC
- Is the **single writer** to the workflow state file

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         MEOW ARCHITECTURE                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│                        ┌─────────────────────┐                              │
│                        │   ORCHESTRATOR      │                              │
│                        │   (meow run)        │                              │
│                        │                     │                              │
│                        │ • Main event loop   │                              │
│                        │ • Workflow state    │                              │
│                        │ • Step dispatch     │                              │
│                        │ • Agent management  │                              │
│                        │ • IPC server        │                              │
│                        └──────────┬──────────┘                              │
│                                   │                                         │
│              ┌────────────────────┼────────────────────┐                    │
│              │                    │                    │                    │
│              ▼                    ▼                    ▼                    │
│     ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐          │
│     │ tmux: meow-wf-a │  │ tmux: meow-wf-b │  │ tmux: meow-wf-c │          │
│     │                 │  │                 │  │                 │          │
│     │ MEOW_AGENT=a    │  │ MEOW_AGENT=b    │  │ MEOW_AGENT=c    │          │
│     │ MEOW_WORKFLOW=1 │  │ MEOW_WORKFLOW=1 │  │ MEOW_WORKFLOW=1 │          │
│     │                 │  │                 │  │                 │          │
│     │    [Claude]     │  │    [Claude]     │  │    [Claude]     │          │
│     └────────┬────────┘  └────────┬────────┘  └────────┬────────┘          │
│              │                    │                    │                    │
│              └────────────────────┴────────────────────┘                    │
│                                   │                                         │
│                         meow done (IPC)                                     │
│                                   │                                         │
│                                   ▼                                         │
│                        ┌─────────────────────┐                              │
│                        │   Orchestrator      │                              │
│                        │   validates, ESC,   │                              │
│                        │   injects next      │                              │
│                        └─────────────────────┘                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Environment Variables

The orchestrator sets these in agent tmux sessions:
- `MEOW_AGENT`: Agent identifier
- `MEOW_WORKFLOW`: Workflow instance ID
- `MEOW_ORCH_SOCK`: Path to IPC socket
- `MEOW_STEP`: Current step ID (updated on each new step)

---

## Template System

Templates are TOML files defining reusable workflows.

### Module Format

A template file can contain multiple named workflows:

```toml
# work-loop.meow.toml

[main]
name = "work-loop"
description = "Continuous work loop"

[[main.steps]]
id = "select"
executor = "agent"
# ...

[tdd]
name = "tdd"
description = "Test-driven implementation"
internal = true  # Can only be called from this file

[[tdd.steps]]
# ...
```

### Template References

| Reference | Resolution |
|-----------|------------|
| `.tdd` | Same file, workflow named `tdd` |
| `main` | Same file, workflow named `main` |
| `helpers#tdd` | File `helpers.meow.toml`, workflow `tdd` |
| `./lib/utils#helper` | Relative path |

### The `main` Convention

When running a module without specifying a workflow, `main` is used:

```bash
meow run work-loop.meow.toml           # Runs [main]
meow run work-loop.meow.toml#tdd       # Runs [tdd] explicitly
```

---

## Variable System

MEOW supports typed variables that preserve structure across template expansion boundaries.

### Variable Types

| Type | Description | Example |
|------|-------------|---------|
| `string` | Default, any text | `"hello"` |
| `int` | Integer number | `42` |
| `bool` | Boolean | `true` |
| `file` | Read file contents | Path to file |
| `json` | Parse JSON string into structure | `'{"key": "value"}'` |
| `object` | Structured value (no strings allowed) | Nested maps/arrays |

### Typed Variable Preservation

Variables maintain their types through expansion chains:

```toml
# foreach sets task to a map: {name: "impl", beads: ["meow-123"]}
[[steps]]
id = "agents"
executor = "foreach"
items_file = "tasks.json"
item_var = "task"
template = "lib/agent-track"

[steps.variables]
task = "{{task}}"  # Preserves the map structure, not JSON string
```

In the expanded template, `{{task.beads}}` returns the array, not a string.

### Pure Reference vs Mixed Content

- **Pure reference** (`{{task}}`): Returns typed value (map, array, etc.)
- **Mixed content** (`prefix-{{task}}`): Returns string (JSON-stringified if needed)

This enables patterns like:
```toml
agent_name = "worker-{{i}}"     # String: "worker-0"
task_data = "{{task}}"          # Map: {name: "...", beads: [...]}
task_id = "{{task.beads.0}}"    # String: "meow-123" (future: array indexing)
```

---

## Design Decisions

### Why Orchestrator-Centric?

The orchestrator owns prompt injection because:
- Single source of truth for "what's happening"
- Single writer to state file (no race conditions)
- Cleaner step transitions (no concurrent modification)
- Enables future distributed orchestration

### Why Events Replace Hooks?

Agent hook systems (Claude's Stop hook, Aider's callbacks) are agent-specific. By translating them to generic events, MEOW stays agent-agnostic. Templates implement the response logic, not the orchestrator.

This also means the "Ralph Wiggum" persistence pattern works with any agent that can emit events—not just Claude.

### Why No Context Management?

Context management is the agent's concern, not MEOW's:
- Users can enable Claude's auto-compact
- Users can add `branch` steps with context-checking scripts
- MEOW stays focused on coordination, not LLM internals

### Why YAML for State?

- Human-readable for debugging
- No database dependency
- Easy to inspect when things go wrong
- Atomic file writes prevent corruption

### Why Templates in TOML?

- Clean syntax for workflow definitions
- Good support for multi-line strings (prompts)
- Distinct from state files (YAML)

### Why Generic Output Types Only?

No `bead_id` or system-specific types because:
- MEOW is task-tracking agnostic
- Users validate external references via `shell` steps if needed
- Generic types (`string`, `number`, `boolean`, `json`, `file_path`) cover all cases

---

## What's NOT in This Document

This architecture document covers **stable philosophy and design decisions**. For details that change frequently, see:

- **Beads** (`bd list`, `bd show <id>`) - What's implemented, in progress, planned
- **Code comments** - Implementation details
- **Workflow files** (`.meow/workflows/`) - Working examples
- **Test files** - Expected behavior

The authority on "what does MEOW do today" is the code and beads, not this document.

---

*MEOW: The coordination language for AI agents.*
