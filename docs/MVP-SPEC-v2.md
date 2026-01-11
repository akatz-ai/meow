# MEOW MVP Specification v2

**MEOW** (Meow Executors Orchestrate Work) is terminal agent orchestration without the framework tax.

> **The Makefile of agent orchestration.** No Python. No cloud. No magic. Just tmux, YAML, and a binary. Works with Claude Code, Aider, or any terminal agent. Your workflows are just version-controlled TOML files.

---

## Positioning

The AI agent orchestration space is crowded: LangChain, LangGraph, CrewAI, Claude-Flow, AutoGen, cloud-managed agents. These frameworks offer sophisticated features—memory systems, vector databases, MCP tools, visual builders, managed infrastructure.

**MEOW takes the opposite approach.**

| Framework Approach | MEOW Approach |
|--------------------|---------------|
| Python SDK with dependencies | Single Go binary |
| Cloud services and databases | YAML files on disk |
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

## Table of Contents

1. [Positioning](#positioning)
2. [Design Philosophy](#design-philosophy)
3. [The Single Primitive: Step](#the-single-primitive-step)
4. [Executors](#executors)
5. [Data Flow Between Steps](#data-flow-between-steps)
6. [Template System](#template-system)
7. [Agent Adapters](#agent-adapters)
8. [Events](#events)
9. [Orchestrator Architecture](#orchestrator-architecture)
10. [Agent Interaction Model](#agent-interaction-model)
11. [IPC and Communication](#ipc-and-communication)
12. [Persistence and Best-Effort Resume](#persistence-and-best-effort-resume)
13. [Workflow Cleanup](#workflow-cleanup)
14. [Resource Limits](#resource-limits)
15. [CLI Commands](#cli-commands)
16. [Multi-Agent Coordination](#multi-agent-coordination)
17. [Complete Examples](#complete-examples)
18. [Error Handling](#error-handling)
19. [Best Practices](#best-practices)
20. [Implementation Phases](#implementation-phases)

---

## Design Philosophy

> **Core Insight**: MEOW is not a task tracker. It's a programming language for agent coordination. Task tracking is the user's concern—MEOW orchestrates agents through workflows that users program to interact with whatever systems they choose.

### MEOW is a Coordination Language

MEOW templates are **programs** that coordinate agents. They are not task lists, not tickets, not issues. They are executable specifications of how work flows through a system of agents.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         MEOW MENTAL MODEL                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Templates = Programs (static, version-controlled)                          │
│  Steps = Instructions                                                       │
│  Executors = Who runs each instruction                                      │
│  Outputs = Data flowing between steps                                       │
│  Workflows = Running program instances (runtime state)                      │
│                                                                             │
│  MEOW doesn't care about your task tracker.                                 │
│  MEOW coordinates agents through programmable workflows.                    │
│  Your workflow tells agents how to find and complete work.                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Task Tracking Agnosticism

MEOW makes **zero assumptions** about how users track work:

| User's System | How MEOW Handles It |
|---------------|---------------------|
| Beads | Workflow prompts agent to run `bd ready` |
| Jira | Workflow prompts agent to query Jira API |
| GitHub Issues | Workflow prompts agent to use `gh issue list` |
| Markdown TODOs | Workflow prompts agent to read `TODO.md` |
| Sticky notes | Workflow prompts agent to ask the user |
| **None** | Workflow can work without any task system |

The **workflow template** encodes how to interact with external systems. MEOW just executes the workflow. Users can create workflows that have no concept of task tracking at all.

### Agent Agnosticism

MEOW makes **zero assumptions** about which AI agent you use:

| Agent | How MEOW Handles It |
|-------|---------------------|
| Claude Code | Adapter provides start command, prompt injection, hooks |
| Aider | Adapter provides start command, prompt injection |
| Cursor Agent | Adapter provides start command, event translation |
| Custom Agent | User writes adapter config |

The **adapter system** encapsulates agent-specific behavior. MEOW core only knows about generic operations: start a process in tmux, inject text, kill the session. How those translate to a specific agent's interface is the adapter's job.

This means:
- No "Claude" references in MEOW core
- No agent-specific CLI commands in core
- Adapters are configuration + optional scripts (not compiled plugins)
- Users can create adapters for any agent that runs in a terminal

### No Special Cases

MEOW does **not** include:
- Context management (use Claude's auto-compact or explicit `branch` checks)
- Work item tracking integration (pure data flow via variables)
- Special types for external systems (generic types only)
- Agent-specific logic in core (use adapters)

If you need context threshold checks, add a `branch` step with a shell script that detects context usage. If you need to link to external work items, pass IDs as regular string variables.

### Core Principles

#### The Propulsion Principle

> **The orchestrator drives agents forward. Agents don't poll—they receive prompts.**

MEOW is a steam engine. The orchestrator is the engineer. Agents are pistons. The orchestrator injects prompts directly into agents via tmux. Agents work until they signal completion with `meow done`.

#### Minimal Agent Exposure

> **Agents see only what they need: a prompt and how to signal completion.**

Agents should never see:
- Internal workflow IDs
- Step dependency graphs
- Orchestrator machinery
- Persistence details

Agents should only see:
- The current prompt/instructions
- Expected outputs (if any)
- The command to signal completion

#### Persistent State, Best-Effort Resume

> **Workflow orchestration state is persisted. On crash, MEOW can resume orchestration—but interrupted work may need manual recovery.**

MEOW persists workflow state to simple YAML files. On crash:
1. Reload workflow state
2. Check which agents are still alive
3. Attempt to resume from where orchestration left off

**What MEOW can recover:** The step dependency graph, which steps are done, captured outputs, agent session existence.

**What MEOW cannot recover:** The effects of interrupted work. If an agent was mid-task when you crashed, MEOW doesn't know if it wrote half a file, made commits, or corrupted something. Re-running a step assumes the work is idempotent or safe to retry—that's on the user to design.

MEOW is not a durable execution engine like Temporal or Step Functions. Those systems control the execution environment and can replay from event history. MEOW spawns agents that do arbitrary things in the real world. There's no sandbox, no event replay. Think of it like Airflow: it tracks task state, but if a task was mid-execution, recovery means "re-run and hope it's idempotent."

No SQLite. No lock files. Just YAML files and good logging.

#### Composition Over Complexity

> **Complex behaviors emerge from simple primitives composed together.**

MEOW has one primitive (Step) with seven executors. Loops, conditionals, parallel execution, human gates, agent handoffs, dynamic iteration—all emerge from composing these simple building blocks.

---

## The Single Primitive: Step

Everything in MEOW is a **Step**. A step has an executor that determines who runs it and how.

### Step Structure

```yaml
step:
  id: string              # Unique within workflow (no dots allowed)
  executor: ExecutorType  # Who runs this step
  status: Status          # pending | running | completing | done | failed
  needs: [string]         # Step IDs that must complete first
  outputs: map            # Captured output data (populated on completion)

  # Executor-specific fields vary by type
```

### Step ID Rules

Step IDs must follow these rules:
- **No dots (`.`) allowed** — dots are reserved for expansion prefixes
- Valid characters: `[a-zA-Z0-9_-]+`
- Must be unique within the workflow

When templates expand, child step IDs are prefixed with the parent step ID and a dot:
- Expand step `do-impl` expands template with step `load`
- Child step becomes `do-impl.load`

This prevents collisions and makes the expansion hierarchy visible.

### Status Lifecycle

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
- `failed`: Execution failed (with error info)

The `completing` state is critical—it prevents the stop hook from interfering while the orchestrator handles step transitions.

### Dependency Resolution

A step is **ready** when:
1. Status is `pending`
2. All steps in `needs` have status `done`

The orchestrator processes ready steps in order, prioritizing orchestrator executors over agent executors.

### Wildcard Dependencies

The `needs` field supports wildcard patterns for dynamic dependencies, particularly useful with `foreach` expansions:

```yaml
# Wait for all steps matching a pattern
needs: ["parallel-workers.*"]              # All direct children of parallel-workers
needs: ["parallel-workers.*.implement"]    # Only 'implement' steps from each iteration
needs: ["feature-*.test"]                  # All test steps from feature branches
```

**Pattern Syntax:**
- `*` matches any single path segment (like glob)
- Patterns are evaluated at runtime against existing step IDs
- If no steps match the pattern, the dependency is considered satisfied (empty set)

**Use Cases:**

```yaml
# Wait for all foreach iterations to complete their 'build' step
[[steps]]
id = "run-integration-tests"
executor = "shell"
command = "npm run test:integration"
needs = ["parallel-builds.*.build"]  # Wait for build step in each iteration

# Wait for any step that starts with 'validate-'
[[steps]]
id = "final-report"
executor = "agent"
prompt = "Generate a summary of all validation results"
needs = ["validate-*"]  # Matches validate-frontend, validate-backend, etc.
```

**Note:** For most `foreach` use cases, you don't need wildcard patterns—the implicit join means you can simply `need` the foreach step itself. Wildcard patterns are useful when you need finer control, such as waiting for a specific step within each iteration rather than all steps.

---

## Executors

Executors determine **who** runs a step and **how**. There are seven executors in two categories:

### Orchestrator Executors

These run internally—the orchestrator handles them without external interaction.

| Executor | Purpose | Completes When |
|----------|---------|----------------|
| `shell` | Run a shell command | Command exits |
| `spawn` | Start an agent process | Agent session is running |
| `kill` | Stop an agent process | Agent session is terminated |
| `expand` | Inline another workflow | Expanded steps are inserted |
| `branch` | Conditional expansion | Branch is evaluated and expanded |
| `foreach` | Iterate over a list | All iterations complete (implicit join) |

### External Executors

These wait for external completion signals.

| Executor | Purpose | Completes When |
|----------|---------|----------------|
| `agent` | Prompt an agent to do work | Agent runs `meow done` |

**Note:** Human approval gates are implemented using `branch` with blocking conditions. See [Human Approval Pattern](#human-approval-pattern).

---

### `shell` — Run Shell Command

Execute a shell command and capture outputs. Use this for setup tasks like creating git worktrees.

**Implementation Note:** Shell is syntactic sugar over branch. Internally, `handleShell()` converts the config to `BranchConfig` and delegates to `handleBranch()`. This means shell steps run asynchronously and honor the DAG parallelism contract—shell steps with the same dependencies execute in parallel.

```yaml
id: "create-worktree"
executor: "shell"
command: |
  git worktree add -b meow/{{agent}} .meow/worktrees/{{agent}} HEAD
  echo ".meow/worktrees/{{agent}}"
outputs:
  worktree_path:
    source: stdout
workdir: "/path/to/repo"  # Optional
env:                       # Optional
  GIT_AUTHOR_NAME: "MEOW"
```

**Fields:**
- `command` (required): Shell script to execute
- `outputs` (optional): Output capture definitions
- `workdir` (optional): Working directory
- `env` (optional): Environment variables
- `on_error` (optional): `continue` | `fail` (default: `fail`)

**Output Sources:**
- `stdout`: Captured standard output (trimmed)
- `stderr`: Captured standard error (trimmed)
- `exit_code`: Process exit code
- `file:/path`: Contents of a file

**Orchestrator Behavior:**
1. Convert shell config to branch config (with no expansion targets)
2. Execute command asynchronously in goroutine
3. Capture specified outputs
4. If exit code != 0 and `on_error: fail`, mark step `failed`
5. Otherwise, mark step `done` with outputs

---

### `spawn` — Start Agent Process

Start an agent in a tmux session. The working directory is typically created by a prior `shell` step. The `adapter` field determines which agent type to start (defaults to project/global config).

```yaml
id: "start-worker"
executor: "spawn"
agent: "worker-1"
adapter: "claude"                                    # Which adapter to use
workdir: "{{create-worktree.outputs.worktree_path}}"  # From prior shell step
env:
  TASK_ID: "{{task_id}}"
  MY_CUSTOM_VAR: "value"
resume_session: "{{checkpoint.outputs.session_id}}"  # Optional
```

**Fields:**
- `agent` (required): Agent identifier
- `adapter` (optional): Adapter name (defaults to config; see [Agent Adapters](#agent-adapters))
- `workdir` (optional): Working directory for the agent (defaults to current directory)
- `env` (optional): Additional environment variables (beyond orchestrator defaults)
- `resume_session` (optional): Session ID to resume (adapter-specific)

**Orchestrator-Injected Environment:**
The orchestrator always sets these environment variables in the tmux session:
- `MEOW_AGENT`: Agent identifier
- `MEOW_WORKFLOW`: Workflow instance ID
- `MEOW_ORCH_SOCK`: Path to orchestrator's IPC socket
- `MEOW_STEP`: Current step ID (updated on each new step)

User-provided `env` vars are merged with orchestrator defaults. **Orchestrator variables take precedence**—if a user specifies `MEOW_AGENT` in their `env` block, it will be overwritten. All `MEOW_*` environment variables are reserved.

**Orchestrator Behavior:**
1. Load adapter configuration (from step, workflow, or global config)
2. Create tmux session `meow-{workflow_id}-{agent}`
3. Set working directory
4. Set environment variables (user + adapter + orchestrator-reserved)
5. Run adapter's start command (with resume flag if `resume_session` provided)
6. Wait for adapter's startup delay
7. Mark step `done`

**Note:** Worktrees are user-managed. Create them with `shell` steps, pass the path here. The orchestrator doesn't manage worktrees automatically.

**Agent State Tracking:**
The orchestrator tracks agent state in the workflow file:
```yaml
agents:
  worker-1:
    adapter: claude
    tmux_session: meow-wf-abc123-worker-1
    workdir: /data/projects/myapp/.meow/worktrees/worker-1
    session_id: sess-xyz789  # Adapter-specific, for resume
```

This enables session resume for parent-child workflow patterns.

---

### `kill` — Stop Agent Process

Terminate an agent's tmux session.

```yaml
id: "stop-worker"
executor: "kill"
agent: "worker-1"
graceful: true    # Send SIGTERM first, wait, then SIGKILL
timeout: 10       # Seconds to wait for graceful shutdown
```

**Fields:**
- `agent` (required): Agent identifier to stop
- `graceful` (optional): Graceful shutdown (default: `true`)
- `timeout` (optional): Graceful shutdown timeout in seconds (default: `10`)

**Orchestrator Behavior:**
1. If `graceful`: send interrupt, wait up to `timeout`
2. Kill tmux session
3. Remove agent from active agents list
4. Mark step `done`

---

### `expand` — Inline Another Workflow

Expand a template's steps into the current workflow.

```yaml
id: "do-implementation"
executor: "expand"
template: ".tdd-impl"                    # Template reference
variables:
  task_id: "{{select.outputs.task_id}}"
  agent: "{{agent}}"
```

**Fields:**
- `template` (required): Template reference (see [Template References](#template-references))
- `variables` (optional): Variables to pass to the template

**Orchestrator Behavior:**
1. Load and parse referenced template
2. Substitute variables
3. Generate unique step IDs for expanded steps (prefixed with parent step ID)
4. Insert expanded steps into workflow, depending on this expand step
5. Mark expand step `done`

**Workflow Growth:**
The workflow YAML file grows as templates expand. Expanded steps get prefixed IDs:

```yaml
# After expand step "do-impl" runs
steps:
  do-impl:
    status: done
    expanded_into: ["do-impl.load", "do-impl.test", "do-impl.code"]

  do-impl.load:
    status: pending
    expanded_from: do-impl
    # ...
```

---

### `branch` — Conditional Expansion

Evaluate a condition and expand the appropriate branch. This executor also handles human approval gates through blocking conditions.

```yaml
id: "check-tests"
executor: "branch"
condition: "npm test"              # Shell command
on_true:
  template: ".continue"            # Expand if exit code 0
on_false:
  template: ".fix-tests"           # Expand if exit code != 0
on_timeout:                        # Optional
  template: ".handle-timeout"
timeout: "5m"                      # Optional
```

**Fields:**
- `condition` (required): Shell command to evaluate (exit code 0 = true)
- `on_true` (optional): Expansion target if condition is true
- `on_false` (optional): Expansion target if condition is false
- `on_timeout` (optional): Expansion target if condition times out
- `timeout` (optional): Condition evaluation timeout

**Expansion Target:**
```yaml
on_true:
  template: "template-ref"    # Reference to expand
  variables: { key: value }   # Variables for template
# OR
on_true:
  inline:                     # Inline step definitions
    - id: "step-1"
      executor: "agent"
      prompt: "..."
```

**Orchestrator Behavior:**
1. Mark step `running` and launch condition in background goroutine
2. Return immediately (enables parallel execution with other steps)
3. When condition completes: evaluate exit code, select branch
4. Expand selected branch (if any)
5. If no children: mark step `done`
6. If has children: stay `running` until children complete

**Async Execution Model:**

Branch conditions execute **asynchronously** in goroutines, allowing parallel execution of monitoring branches alongside agent work. This enables patterns like:

```yaml
# Both steps start in the same tick after spawn completes
[[steps]]
id = "main-work"
executor = "agent"
needs = ["spawn"]
prompt = "Do the work"

[[steps]]
id = "monitor-for-stop"
executor = "branch"
needs = ["spawn"]
condition = "meow await-event agent-stopped --timeout 30s"
on_true:
  template = ".handle-agent-stopped"
```

Without async execution, `monitor-for-stop` would block for 30 seconds before `main-work` could start—defeating the purpose of parallel monitoring.

**Completion Serialization:**

While conditions run in parallel, their completions serialize through the workflow mutex to ensure consistency. This provides correctness guarantees at the cost of completion throughput (~100-200 branches/second). See [Performance Characteristics](#performance-characteristics) for details.

### Human Approval Pattern

Human gates are implemented using `branch` with blocking conditions:

```yaml
# Notify human and wait for approval
[[steps]]
id = "notify-reviewer"
executor = "shell"
command = """
echo "Review needed for {{task_id}}"
echo "Approve: meow approve {{workflow_id}} review-gate"
echo "Reject:  meow reject {{workflow_id}} review-gate --reason '...'"
# Could also send Slack message, email, etc.
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
variables = { reason = "{{review-gate.outputs.reason}}" }
```

The `meow await-approval` command:
1. Blocks until human runs `meow approve` or `meow reject`
2. Exits 0 on approval, non-zero on rejection
3. Respects the specified timeout (workflow fails on timeout)

This pattern allows custom approval mechanisms:
- Human approval: `meow await-approval`
- CI completion: `./scripts/wait-for-ci.sh`
- API approval: `curl --retry 100 https://api.example.com/approval-status`
- Slack approval: Custom integration script

---

### `agent` — Prompt Agent

Assign work to an agent and wait for completion.

```yaml
id: "implement-feature"
executor: "agent"
agent: "worker-1"                    # Which agent
prompt: |
  Implement the feature described in task {{task_id}}.

  Follow TDD:
  1. Write failing tests first
  2. Implement until tests pass
  3. Refactor if needed

  When done, provide the path to the main implementation file.
outputs:
  main_file:
    required: true
    type: string
    description: "Path to the main implementation file"
  test_file:
    required: false
    type: string
mode: "autonomous"  # autonomous | interactive | fire_forget
timeout: "2h"       # Optional: max time for step
```

**Fields:**
- `agent` (required): Agent identifier
- `prompt` (required): Instructions for the agent
- `outputs` (optional): Expected output definitions (not allowed with `fire_forget` mode)
- `mode` (optional): `autonomous` (default), `interactive`, or `fire_forget`
- `timeout` (optional): Maximum time for step execution (not applicable to `fire_forget`)

**Output Definition:**
```yaml
outputs:
  field_name:
    required: true|false     # Is this output required?
    type: string|number|boolean|json|file_path
    description: "..."       # Shown to agent
```

**Output Types (Generic):**
| Type | Validation | Example |
|------|------------|---------|
| `string` | Non-empty string | `"PROJ-123"` |
| `number` | Parseable as int or float | `42`, `3.14` |
| `boolean` | `true` or `false` | `true` |
| `json` | Valid JSON | `{"key": "value"}` |
| `file_path` | File exists on filesystem | `src/auth.ts` |

**File Path Validation:**
- **Absolute paths** are validated directly
- **Relative paths** are resolved against the **agent's working directory** (the `workdir` from its `spawn` step)
- The orchestrator tracks each agent's spawn workdir and uses it for validation
- If an agent changes directories during execution, relative paths still resolve against the original spawn workdir

**Modes:**
- `autonomous`: Normal execution—waits for agent to call `meow done`, then orchestrator immediately injects next prompt
- `interactive`: Pauses for human conversation—orchestrator returns empty prompt during stop hook, allowing natural Claude-human interaction. Still waits for `meow done`.
- `fire_forget`: Inject prompt and immediately mark step done—does not wait for agent response. Useful for sending commands like `/compact`, `Escape` key presses, or other fire-and-forget instructions. Cannot have outputs.

**Fire-and-Forget Use Cases:**

The `fire_forget` mode leverages the robust prompt injection mechanism (copy mode handling, readiness detection, retry logic) without waiting for completion. Common uses:

```yaml
# Send a slash command to trigger context compaction
[[steps]]
id = "trigger-compact"
executor = "agent"
agent = "worker"
prompt = "/compact"
mode = "fire_forget"

# Send Escape key to pause agent mid-work (aggressive compaction pattern)
[[steps]]
id = "pause-agent"
executor = "agent"
agent = "worker"
prompt = "Escape"  # Special: single key name sends that key
mode = "fire_forget"

# Chain them: Escape then /compact for aggressive compaction
[[steps]]
id = "escape"
executor = "agent"
agent = "worker"
prompt = "Escape"
mode = "fire_forget"

[[steps]]
id = "compact"
executor = "agent"
agent = "worker"
prompt = "/compact"
mode = "fire_forget"
needs = ["escape"]
```

**Note:** For `fire_forget` mode, the prompt injection still uses the full reliability mechanism (cancel copy mode, wait for Claude readiness, send with retry). The only difference is the step completes immediately after successful injection rather than waiting for `meow done`.

**Timeout Behavior:**
If specified, the orchestrator monitors step duration. If the step exceeds the timeout:
1. Send `C-c` (interrupt) to the agent's tmux session
2. Wait 10 seconds for graceful stop
3. Mark step `failed` with error "Step timed out after {duration}"
4. Continue based on `on_error` setting (default: workflow fails)

This handles common stuck scenarios like hung tools or infinite loops.

**Orchestrator Behavior:**
1. Guard: verify step status is `pending` (prevents double-injection)
2. Mark step `running`, record start time
3. Update `MEOW_STEP` environment variable in agent's session
4. Inject prompt directly into agent's tmux session via `send-keys`
5. Start timeout timer (if configured)
6. Wait for agent to signal completion via `meow done` (IPC)
7. Validate outputs against definitions
8. If valid: mark step `done` with outputs, proceed to next step
9. If invalid: return error to agent, keep step `running`, agent retries

---

### `foreach` — Iterate Over a List

Dynamically expand a template for each item in a list, with built-in parallel execution and implicit join semantics.

```yaml
id: "parallel-workers"
executor: "foreach"
items: "{{planner.outputs.tasks}}"   # JSON array to iterate over
item_var: "task"                      # Variable name for current item
index_var: "i"                        # Optional: variable name for index

template: ".worker-task"              # Template to expand for each item
variables:
  agent_id: "worker-{{i}}"
  task_description: "{{task.description}}"
  task_files: "{{task.files}}"

parallel: true                        # Run iterations in parallel (default: true)
max_concurrent: 5                     # Optional: limit concurrent executions
join: true                            # Wait for all iterations (default: true)
```

**Alternative: Read from File**

When a previous step writes JSON to a file (common when JSON contains multi-line text), use `items_file`:

```yaml
id: "parallel-workers"
executor: "foreach"
items_file: ".meow/worktrees/.tasks.json"  # Read JSON array from file
item_var: "task"
template: ".worker-task"
```

**Fields:**
- `items` (required*): JSON array expression to iterate over
- `items_file` (required*): Path to JSON file containing array to iterate over
- `item_var` (required): Variable name that holds the current item in each iteration
- `index_var` (optional): Variable name for the 0-based iteration index
- `template` (required): Template reference to expand for each item
- `variables` (optional): Variables to pass to each expansion (can reference `item_var` and `index_var`)
- `parallel` (optional): Run iterations in parallel (default: `true`)
- `max_concurrent` (optional): Maximum concurrent iterations (only applies when `parallel: true`)
- `join` (optional): Wait for all iterations to complete before marking foreach as done (default: `true`)

*Exactly one of `items` or `items_file` must be specified. Use `items_file` when the JSON array is written to a file by a previous step—this avoids escaping issues that can occur when passing JSON with embedded newlines through variable substitution.

**Child Step Naming:**
Child steps are prefixed with the foreach step ID and the iteration index:
```yaml
# After foreach "parallel-workers" expands over 3 items:
steps:
  parallel-workers:
    status: done  # or running if join: true and children still running
    expanded_into: ["parallel-workers.0", "parallel-workers.1", "parallel-workers.2"]

  parallel-workers.0.create-worktree:
    status: done
    expanded_from: parallel-workers
    # ...

  parallel-workers.0.implement:
    status: running
    # ...

  parallel-workers.1.create-worktree:
    status: done
    # ...
```

**Orchestrator Behavior:**
1. Evaluate `items` expression to get JSON array
2. For each item in array:
   a. Set `item_var` to current item
   b. Set `index_var` to current index (if specified)
   c. Substitute variables
   d. Expand template with prefixed step IDs (`{foreach_id}.{index}.{step_id}`)
3. If `parallel: false`, chain iterations sequentially (each depends on previous)
4. If `join: true` (default): foreach step stays `running` until all child steps complete
5. If `join: false`: foreach step marked `done` after expansion, children run independently

**Implicit Join Semantics:**
By default (`join: true`), the foreach step acts as a synchronization barrier:
- The foreach step is not marked `done` until ALL child steps are `done`
- Downstream steps that `need` the foreach step automatically wait for all iterations
- This eliminates the need for explicit join steps in most cases

```yaml
[[steps]]
id = "parallel-workers"
executor = "foreach"
items = "{{tasks}}"
template = ".worker-task"
# join: true is the default

[[steps]]
id = "merge-all"
executor = "agent"
prompt = "All workers complete. Merge the branches."
needs = ["parallel-workers"]  # Waits for ALL iterations due to implicit join
```

**Disabling Implicit Join:**
Set `join: false` when you want iterations to run independently without blocking:

```yaml
[[steps]]
id = "fire-and-forget"
executor = "foreach"
items = "{{notifications}}"
template = ".send-notification"
join = false  # Don't wait for notifications to complete

[[steps]]
id = "continue-work"
executor = "agent"
prompt = "Continue with main task..."
needs = ["fire-and-forget"]  # Proceeds immediately after expansion
```

**Sequential Iteration:**
For ordered processing where each iteration depends on the previous:

```yaml
[[steps]]
id = "sequential-migrations"
executor = "foreach"
items = "{{migrations}}"
item_var = "migration"
template = ".run-migration"
parallel = false  # Each migration runs after the previous completes
```

**Concurrency Limiting:**
Control resource usage with `max_concurrent`:

```yaml
[[steps]]
id = "parallel-tests"
executor = "foreach"
items = "{{test_suites}}"
template = ".run-test-suite"
max_concurrent = 3  # At most 3 test suites running at once
```

The orchestrator will start up to `max_concurrent` iterations, then wait for one to complete before starting the next.

**Error Handling in foreach:**
If any child step fails:
- With `join: true`: The foreach step fails when the child fails (unless child has `on_error`)
- With `join: false`: Child failure doesn't affect the foreach step (it's already done)

Individual iteration failures can be handled via `on_error` in the expanded template.

---

## Data Flow Between Steps

Steps communicate through **outputs**. This works like GitHub Actions—pure data flow, no special cases.

### Capturing Outputs

**From shell commands:**
```yaml
id: "get-branch"
executor: "shell"
command: "git branch --show-current"
outputs:
  branch:
    source: stdout
```

**From agents:**
```yaml
id: "select-task"
executor: "agent"
prompt: "Pick a task from the backlog"
outputs:
  task_id:
    required: true
    type: string
```

### Referencing Outputs

Use `{{step_id.outputs.field}}` syntax:

```yaml
id: "implement"
executor: "agent"
prompt: "Implement task {{select-task.outputs.task_id}}"
```

### Output Storage

Outputs are stored on the step when it completes:

```yaml
# In workflow state file
steps:
  select-task:
    status: done
    outputs:
      task_id: "PROJ-123"
      priority: "high"
```

### Variable Substitution

Variables can appear in any string field:

```yaml
prompt: "Work on {{task_id}} in branch {{setup.outputs.branch}}"
command: "git checkout {{branch_name}}"
template: "{{workflow_type}}-impl"
```

**Substitution Order:**
1. Workflow variables (from `meow run --var`)
2. Step outputs (from completed steps)
3. Built-in variables

**Built-in Variables:**
- `{{workflow_id}}` — Current workflow instance ID
- `{{timestamp}}` — Current ISO timestamp
- `{{date}}` — Current date (YYYY-MM-DD)

**Missing Variable Handling:**
If a variable reference cannot be resolved (e.g., `{{step.outputs.missing_field}}`), the orchestrator **fails loudly** with an error:
- For orchestrator steps: The step fails immediately
- For agent steps: Substitution fails before prompt injection—the agent never sees a broken prompt

This prevents silent failures from typos or missing outputs. There is no empty-string fallback.

**Shell Context Escaping:**
When variables are substituted into `command` or `condition` fields (shell contexts), values are **automatically shell-escaped** to prevent injection attacks. For example, if an output contains `'; rm -rf /`, it becomes `''\'''; rm -rf /'` when substituted.

If you intentionally need raw shell interpretation, use a `shell` step to prepare the value first.

---

## Template System

Templates are TOML files that define reusable workflows.

### Module Format

A template file can contain multiple workflows:

```toml
# work-loop.meow.toml

# ═══════════════════════════════════════════════════════════════════════════════
# MAIN WORKFLOW
# ═══════════════════════════════════════════════════════════════════════════════

[main]
name = "work-loop"
description = "Continuous work selection and implementation loop"

# Cleanup is opt-in. Only clean up on success (preserve state on failure for debugging)
cleanup_on_success = """
git worktree remove .meow/worktrees/{{agent}} --force 2>/dev/null || true
rm -rf /tmp/meow-{{workflow_id}} 2>/dev/null || true
"""

[main.variables]
agent = { required = true, description = "Agent to assign work to" }

[[main.steps]]
id = "select"
executor = "agent"
agent = "{{agent}}"
prompt = """
Query your project's task system and select the next task to work on.

Store the task identifier and whether there are more tasks available.
"""

[main.steps.outputs]
task_id = { required = true, type = "string" }
has_more = { required = true, type = "boolean" }

[[main.steps]]
id = "implement"
executor = "expand"
template = ".tdd"
variables = { task_id = "{{select.outputs.task_id}}", agent = "{{agent}}" }
needs = ["select"]

[[main.steps]]
id = "loop"
executor = "branch"
condition = "test '{{select.outputs.has_more}}' = 'true'"
needs = ["implement"]

[main.steps.on_true]
template = "main"  # Recursion!
variables = { agent = "{{agent}}" }


# ═══════════════════════════════════════════════════════════════════════════════
# TDD IMPLEMENTATION WORKFLOW
# ═══════════════════════════════════════════════════════════════════════════════

[tdd]
name = "tdd"
description = "Test-driven implementation workflow"
internal = true  # Can only be called from this file

[tdd.variables]
task_id = { required = true }
agent = { required = true }

[[tdd.steps]]
id = "load-context"
executor = "agent"
agent = "{{agent}}"
prompt = """
Read and understand task {{task_id}}.

Identify:
- What needs to be implemented
- Relevant existing code
- Test patterns used in this project
"""

[[tdd.steps]]
id = "write-tests"
executor = "agent"
agent = "{{agent}}"
prompt = "Write failing tests that define the expected behavior for {{task_id}}."
needs = ["load-context"]

[[tdd.steps]]
id = "verify-failing"
executor = "branch"
condition = "! npm test 2>/dev/null"  # Tests should fail
needs = ["write-tests"]

[tdd.steps.on_true]
inline = []  # Good, continue

[tdd.steps.on_false]
inline = [
  { id = "fix-tests", executor = "agent", agent = "{{agent}}",
    prompt = "Tests passed unexpectedly. Revise to actually test new behavior." }
]

[[tdd.steps]]
id = "implement"
executor = "agent"
agent = "{{agent}}"
prompt = "Implement the minimum code to make tests pass."
needs = ["verify-failing"]

[[tdd.steps]]
id = "verify-passing"
executor = "branch"
condition = "npm test"
needs = ["implement"]

[tdd.steps.on_true]
inline = []  # Good, continue

[tdd.steps.on_false]
template = ".tdd"  # Tests still failing, retry from the beginning
variables = { task_id = "{{task_id}}", agent = "{{agent}}" }

[[tdd.steps]]
id = "commit"
executor = "agent"
agent = "{{agent}}"
prompt = "Commit the changes with a descriptive message."
needs = ["verify-passing"]
```

### Template References

| Reference | Resolution |
|-----------|------------|
| `.tdd` | Same file, workflow named `tdd` |
| `main` | Same file, workflow named `main` |
| `helpers#tdd` | File `helpers.meow.toml`, workflow `tdd` |
| `helpers` | File `helpers.meow.toml`, workflow `main` |
| `./lib/utils#helper` | Relative path |

### Workflow Properties

```toml
[workflow-name]
name = "human-readable-name"     # Required
description = "..."              # Optional
internal = true                  # Cannot be called from outside this file

# Cleanup is opt-in (all optional, no cleanup by default)
cleanup_on_success = "..."       # Runs when all steps complete successfully
cleanup_on_failure = "..."       # Runs when a step fails
cleanup_on_stop = "..."          # Runs on SIGINT/SIGTERM/meow stop

[workflow-name.variables]
var_name = { required = true, type = "string", description = "..." }
var_with_default = { default = "value" }

[[workflow-name.steps]]
# Step definitions...
```

### The `main` Convention

When running a module without specifying a workflow, `main` is used:

```bash
meow run work-loop.meow.toml           # Runs [main]
meow run work-loop.meow.toml#tdd       # Runs [tdd] explicitly
```

---

## Agent Adapters

Adapters encapsulate agent-specific **runtime behavior**, keeping MEOW core agent-agnostic. An adapter defines how to start, stop, and inject prompts into a specific agent type.

**Design principle:** Adapters handle only what the orchestrator needs to interact with agent processes at runtime. Agent configuration (like event hooks) belongs in library templates, not adapters. This keeps adapters focused and allows workflows to customize agent configuration as needed.

### Adapter Structure

Adapters are directories containing configuration:

```
~/.meow/adapters/
└── claude/
    ├── adapter.toml        # Required: runtime configuration
    └── README.md           # Optional: documentation
```

### Adapter Configuration

The `adapter.toml` file defines runtime behavior:

```toml
# ~/.meow/adapters/claude/adapter.toml

[adapter]
name = "claude"
description = "Claude Code CLI agent"

# How to start the agent in tmux
[spawn]
command = "claude --dangerously-skip-permissions"
resume_command = "claude --dangerously-skip-permissions --resume {{session_id}}"
startup_delay = "3s"

# Additional environment variables for the agent
[environment]
TMUX = ""  # Prevent Claude from detecting outer tmux

# How to inject prompts
[prompt_injection]
pre_keys = ["Escape"]      # Keys to send before prompt
pre_delay = "100ms"        # Delay after pre_keys
method = "literal"         # "literal" or "keys"
post_keys = ["Enter"]      # Keys to send after prompt

# How to gracefully stop
[graceful_stop]
keys = ["C-c"]
wait = "2s"
```

**Note:** Event hooks (like Claude's Stop/PreToolUse/PostToolUse) are configured via library templates, not adapters. See [Event Hook Configuration](#event-hook-configuration) below.

### Using Adapters in Workflows

Reference adapters in `spawn` steps:

```toml
[[steps]]
id = "spawn"
executor = "spawn"
agent = "worker"
adapter = "claude"        # Use the claude adapter
workdir = "{{worktree}}"
```

If `adapter` is omitted, the default from config is used.

### Adapter Resolution Order

1. Step-level: `adapter` field in spawn step
2. Workflow-level: `default_adapter` in template variables
3. Project-level: `.meow/config.toml` `[agents] default_adapter`
4. Global-level: `~/.meow/config.toml` `[agents] default_adapter`
5. Built-in default: `claude`

### Installing Adapters

```bash
# Install from git repository
meow adapter install github.com/meow-stack/adapter-claude

# Install from local directory
meow adapter install ./my-adapter

# List installed adapters
meow adapter list

# Show adapter details
meow adapter show claude

# Remove adapter
meow adapter remove claude
```

### Creating Custom Adapters

To create an adapter for a new agent type:

1. Create adapter directory: `mkdir -p ~/.meow/adapters/my-agent`
2. Create `adapter.toml` with spawn/injection/stop configuration
3. Test with a simple workflow

For event support, create a corresponding library template (e.g., `lib/my-agent-events.meow.toml`) that configures the agent's hook system. See [Event Hook Configuration](#event-hook-configuration).

### Trust Model

Templates and adapters can contain shell commands. The trust model is consistent:

| Component | User Must Trust |
|-----------|-----------------|
| MEOW core | Yes (it's the tool) |
| Templates | Yes (contains shell commands) |
| Adapters | Yes (defines how agents start) |
| Library templates | Yes (contains shell commands) |
| The agent itself | Yes (it runs on your system) |

**Users should review code before using**, just as they would review any script they download from the internet.

---

## Events

Events are the generic mechanism for agents to communicate with the orchestrator. Unlike step completion (`meow done`), events are fire-and-forget signals that can trigger workflow logic.

### Event Primitives

MEOW core provides two commands for events:

```bash
# Emit an event (called from agent hook configurations)
meow event <event-type> [--data key=value ...]

# Wait for an event (used in branch conditions)
meow await-event <event-type> [--filter key=value ...] [--timeout duration]
```

### Event Types

Event types are strings. MEOW suggests these conventions but doesn't enforce them:

| Event Type | Meaning | Typical Data |
|------------|---------|--------------|
| `agent-stopped` | Agent reached prompt unexpectedly | - |
| `tool-starting` | Agent about to call a tool | `tool`, `args` |
| `tool-completed` | Agent finished calling a tool | `tool`, `result` |
| `error` | Agent encountered an error | `message` |
| `progress` | Agent reporting progress | `message`, `percent` |

Users can define any event types they need.

### Event Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           EVENT FLOW                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Agent (Claude)                          Orchestrator             Workflow  │
│  ──────────────                          ────────────             ────────  │
│       │                                       │                      │      │
│  Calls Write tool                             │                      │      │
│       │                                       │                      │      │
│  PostToolUse hook fires                       │                      │      │
│  (configured by workflow                      │                      │      │
│   via library template)                       │                      │      │
│       │                                       │                      │      │
│       │  meow event tool-completed            │                      │      │
│       │  --data tool=Write                    │                      │      │
│       │──────────────────────────────────────►│                      │      │
│       │                                       │                      │      │
│       │                                       │ Routes event         │      │
│       │                                       │ to waiters           │      │
│       │                                       │─────────────────────►│      │
│       │                                       │                      │      │
│       │                                       │              await-event    │
│       │                                       │              unblocks       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Key insight:** The agent's hooks call `meow event` directly. Hook configuration is done by workflows via library templates—there's no separate "event translator" script in the adapter.

### Using Events in Workflows

Events enable reactive workflow patterns. Common use: monitoring agent behavior.

**Pattern: Alert on Tool Calls**

```toml
# Monitor all Write tool calls during a task

[[steps]]
id = "spawn"
executor = "spawn"
agent = "worker"
adapter = "claude"
workdir = "{{worktree}}"

[[steps]]
id = "main-task"
executor = "agent"
agent = "worker"
prompt = "Implement feature X"
needs = ["spawn"]

# Event monitoring runs in parallel with main-task
[[steps]]
id = "start-monitoring"
executor = "expand"
template = ".event-monitor-loop"
variables = { agent = "worker" }
needs = ["spawn"]

[[steps]]
id = "cleanup"
executor = "kill"
agent = "worker"
needs = ["main-task"]


# Event monitoring loop (internal template)
[event-monitor-loop]
internal = true

[event-monitor-loop.variables]
agent = { required = true }

[[event-monitor-loop.steps]]
id = "wait-for-event"
executor = "branch"
condition = "meow await-event tool-completed --filter agent={{agent}} --timeout 30s"

[event-monitor-loop.steps.on_true]
template = ".handle-event-and-continue"
variables = { agent = "{{agent}}" }

[event-monitor-loop.steps.on_false]
# Timeout - check if main task is done
template = ".check-continue"
variables = { agent = "{{agent}}" }


[handle-event-and-continue]
internal = true

[[handle-event-and-continue.steps]]
id = "log-event"
executor = "shell"
command = "echo '[EVENT] Tool completed for {{agent}}' >> .meow/events.log"

[[handle-event-and-continue.steps]]
id = "check-done"
executor = "branch"
condition = "meow step-status main-task --is-not done"
needs = ["log-event"]

[handle-event-and-continue.steps.on_true]
# Main task still running, keep monitoring
template = ".event-monitor-loop"
variables = { agent = "{{agent}}" }

# on_false: main task done, stop monitoring
```

### Event IPC Message

Events are sent via the standard IPC socket:

```json
{"type":"event","workflow":"wf-abc","agent":"worker-1","event_type":"tool-completed","data":{"tool":"Write","file":"src/main.ts"}}
```

The orchestrator acknowledges receipt:
```json
{"type":"ack","success":true}
```

### Events vs. Step Completion

| Aspect | `meow done` | `meow event` |
|--------|-------------|--------------|
| Purpose | Signal step completion | Signal something happened |
| Workflow effect | Marks step done, triggers next | Routes to await-event waiters |
| Blocking | Waits for validation response | Fire-and-forget |
| Outputs | Validated against step schema | Arbitrary key-value data |
| Frequency | Once per step | Any number of times |

### Events Are Optional

Events are an **enhancement**, not a requirement. Workflows work fine without them:

- If hooks aren't configured → no events fire → `await-event` times out → workflow continues via timeout branch
- Events add observability and reactive patterns, but core orchestration doesn't depend on them

### Event Hook Configuration

Event hooks are configured via **library templates**, not adapters. This keeps workflows self-contained and allows different workflows to configure different hooks.

**Library template for Claude Code hooks (`lib/claude-events.meow.toml`):**

```toml
# Configure Claude Code hooks to emit MEOW events
# Usage: expand template "lib/claude-events#setup-hooks" with worktree path

[setup-hooks]
name = "setup-hooks"
description = "Configure Claude Code hooks for MEOW events"

[setup-hooks.variables]
worktree = { required = true, description = "Path to worktree" }
events = { default = "all", description = "all | stop-only | tools-only" }

[[setup-hooks.steps]]
id = "configure"
executor = "shell"
command = """
mkdir -p {{worktree}}/.claude

case "{{events}}" in
  all)
    cat > {{worktree}}/.claude/settings.json << 'EOF'
{
  "hooks": {
    "Stop": [{"type": "command", "command": "meow event agent-stopped"}],
    "PreToolUse": [{"type": "command", "command": "meow event tool-starting --data tool=$TOOL_NAME"}],
    "PostToolUse": [{"type": "command", "command": "meow event tool-completed --data tool=$TOOL_NAME"}]
  }
}
EOF
    ;;
  stop-only)
    cat > {{worktree}}/.claude/settings.json << 'EOF'
{"hooks": {"Stop": [{"type": "command", "command": "meow event agent-stopped"}]}}
EOF
    ;;
  tools-only)
    cat > {{worktree}}/.claude/settings.json << 'EOF'
{
  "hooks": {
    "PreToolUse": [{"type": "command", "command": "meow event tool-starting --data tool=$TOOL_NAME"}],
    "PostToolUse": [{"type": "command", "command": "meow event tool-completed --data tool=$TOOL_NAME"}]
  }
}
EOF
    ;;
esac
"""
```

**Usage in workflows:**

```toml
[[main.steps]]
id = "create-worktree"
executor = "shell"
command = "git worktree add ... && echo path"
outputs = { path = { source = "stdout" } }

[[main.steps]]
id = "setup-hooks"
executor = "expand"
template = "lib/claude-events#setup-hooks"
variables = { worktree = "{{create-worktree.outputs.path}}", events = "all" }
needs = ["create-worktree"]

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "worker"
adapter = "claude"
workdir = "{{create-worktree.outputs.path}}"
needs = ["setup-hooks"]
```

**Benefits of library templates over adapter-based events:**

1. **Visibility**: Hook configuration is visible in the workflow
2. **Flexibility**: Different workflows can use different event configurations
3. **Self-contained**: Workflows don't depend on adapter installation details
4. **Testable**: Hook setup is just a shell step—easy to debug

**For other agents:** Create similar library templates (e.g., `lib/aider-events.meow.toml`) that configure that agent's hook/callback system.

### Context Monitor Pattern

A common pattern is monitoring an agent's context usage and triggering `/compact` when it gets high. This uses `fire_forget` mode to inject commands without waiting for completion.

**Architecture:**
```
┌─────────────────────────────────────────────────────────────┐
│                      WORKFLOW                               │
│                                                             │
│  spawn-agent ─┬─► work-agent (agent executor, waits)        │
│               │                                             │
│               └─► start-monitor (expand, parallel)          │
│                        │                                    │
│                        ▼                                    │
│                   context-monitor-loop (internal)           │
│                        │                                    │
│                        ├─► sleep 30s                        │
│                        ├─► check session exists             │
│                        ├─► check context %                  │
│                        ├─► if high: fire_forget /compact    │
│                        └─► recurse                          │
│                                                             │
│  kill-agent ◄── needs: [work-agent] (NOT monitor)           │
└─────────────────────────────────────────────────────────────┘
```

**Example Implementation:**

```toml
[main]
name = "work-with-context-monitor"

[[main.steps]]
id = "spawn-worker"
executor = "spawn"
agent = "worker"
workdir = "{{worktree}}"

# Start monitor and work in parallel
[[main.steps]]
id = "start-monitor"
executor = "expand"
template = ".context-monitor-loop"
variables = { agent = "worker", threshold = "85" }
needs = ["spawn-worker"]

[[main.steps]]
id = "do-work"
executor = "agent"
agent = "worker"
prompt = "Implement the feature..."
needs = ["spawn-worker"]

# Kill depends only on work, not monitor
[[main.steps]]
id = "kill-worker"
executor = "kill"
agent = "worker"
needs = ["do-work"]


# Context monitor loop (internal template)
[context-monitor-loop]
internal = true

[context-monitor-loop.variables]
agent = { required = true }
threshold = { default = "90" }
check_interval = { default = "30" }

[[context-monitor-loop.steps]]
id = "wait"
executor = "shell"
command = "sleep {{check_interval}}"

[[context-monitor-loop.steps]]
id = "check-session"
executor = "branch"
condition = "tmux has-session -t meow-${MEOW_WORKFLOW}-{{agent}} 2>/dev/null"
needs = ["wait"]

# Session gone - exit loop gracefully
[context-monitor-loop.steps.on_false]
inline = []

# Session exists - check context
[context-monitor-loop.steps.on_true]
template = ".check-and-maybe-compact"
variables = { agent = "{{agent}}", threshold = "{{threshold}}", check_interval = "{{check_interval}}" }


[check-and-maybe-compact]
internal = true

[[check-and-maybe-compact.steps]]
id = "get-context"
executor = "shell"
description = "Screen-scrape tmux for context percentage"
command = """
SESSION="meow-${MEOW_WORKFLOW}-{{agent}}"
PERCENT=$(tmux capture-pane -p -t "$SESSION" -S -100 2>/dev/null | \
    grep -oE '[0-9]+k/[0-9]+k \([0-9]+%\)' | tail -1 | \
    grep -oE '\([0-9]+%\)' | tr -d '()%')
echo "${PERCENT:-0}"
"""
[check-and-maybe-compact.steps.outputs]
percent = { source = "stdout" }

[[check-and-maybe-compact.steps]]
id = "check-threshold"
executor = "branch"
condition = "test {{get-context.outputs.percent}} -ge {{threshold}}"
needs = ["get-context"]

# Below threshold - just loop
[check-and-maybe-compact.steps.on_false]
template = ".context-monitor-loop"
variables = { agent = "{{agent}}", threshold = "{{threshold}}", check_interval = "{{check_interval}}" }

# Above threshold - compact then loop
[check-and-maybe-compact.steps.on_true]
template = ".compact-and-continue"
variables = { agent = "{{agent}}", threshold = "{{threshold}}", check_interval = "{{check_interval}}" }


[compact-and-continue]
internal = true

[[compact-and-continue.steps]]
id = "inject-compact"
executor = "agent"
agent = "{{agent}}"
prompt = "/compact"
mode = "fire_forget"

[[compact-and-continue.steps]]
id = "wait-for-compact"
executor = "shell"
command = "sleep 60"
needs = ["inject-compact"]

[[compact-and-continue.steps]]
id = "continue-loop"
executor = "expand"
template = ".context-monitor-loop"
variables = { agent = "{{agent}}", threshold = "{{threshold}}", check_interval = "{{check_interval}}" }
needs = ["wait-for-compact"]
```

**Key Points:**
1. Monitor runs in parallel with main work via `expand` without dependencies between them
2. Loop terminates naturally when `tmux has-session` fails (agent killed)
3. `fire_forget` mode uses full injection reliability without waiting for response
4. `kill-agent` depends only on `do-work`, not the monitor—so killing proceeds immediately when work completes
5. All loop state is in the template recursion—no external polling process needed

**Aggressive Compaction (with Escape):**

For cases where you want to interrupt Claude mid-thought before compacting:

```toml
[compact-and-continue]
internal = true

# First send Escape to pause
[[compact-and-continue.steps]]
id = "send-escape"
executor = "agent"
agent = "{{agent}}"
prompt = "Escape"
mode = "fire_forget"

# Small delay for escape to take effect
[[compact-and-continue.steps]]
id = "escape-delay"
executor = "shell"
command = "sleep 0.5"
needs = ["send-escape"]

# Then send /compact
[[compact-and-continue.steps]]
id = "inject-compact"
executor = "agent"
agent = "{{agent}}"
prompt = "/compact"
mode = "fire_forget"
needs = ["escape-delay"]

# ... rest of loop continues ...
```

---

## Orchestrator Architecture

### Orchestrator-Centric Design

The orchestrator is the **central authority**. It:
- Owns all prompt injection (agents never run `meow prime` themselves)
- Manages all agent lifecycles
- Handles all state transitions
- Communicates with agents via tmux and IPC
- Is the **single writer** to the workflow state file (eliminates race conditions)

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
│     │ worktree: wt-a  │  │ worktree: wt-b  │  │ worktree: wt-c  │          │
│     │                 │  │                 │  │                 │          │
│     │ MEOW_AGENT=a    │  │ MEOW_AGENT=b    │  │ MEOW_AGENT=c    │          │
│     │ MEOW_WORKFLOW=1 │  │ MEOW_WORKFLOW=1 │  │ MEOW_WORKFLOW=1 │          │
│     │ MEOW_ORCH_SOCK= │  │ MEOW_ORCH_SOCK= │  │ MEOW_ORCH_SOCK= │          │
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

### Core Loop

The orchestrator processes **all ready steps** in each iteration, enabling parallel agent execution:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           ORCHESTRATOR MAIN LOOP                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  func run():                                                                │
│      start_ipc_server()                                                     │
│                                                                             │
│      while workflow.status != "done":                                       │
│                                                                             │
│          // Handle any pending IPC messages (meow done, etc.)               │
│          process_ipc_queue()                                                │
│                                                                             │
│          // Check timeouts for running agent steps                          │
│          check_step_timeouts()                                              │
│                                                                             │
│          ready_steps = get_all_ready_steps()                                │
│                                                                             │
│          if len(ready_steps) == 0:                                          │
│              if all_steps_done():                                           │
│                  workflow.status = "done"                                   │
│                  break                                                      │
│              sleep(poll_interval)  # Waiting for external completion        │
│              continue                                                       │
│                                                                             │
│          // Process ALL ready steps (enables parallelism)                   │
│          for step in ready_steps:                                           │
│              match step.executor:                                           │
│                  // Orchestrator steps: execute immediately                 │
│                  "shell"  → run_command(step); step.status = "done"         │
│                  "spawn"  → start_agent(step); step.status = "done"         │
│                  "kill"   → stop_agent(step); step.status = "done"          │
│                  "expand" → inline_template(step); step.status = "done"     │
│                  "branch" → eval_and_expand(step); step.status = "done"     │
│                                                                             │
│                  // Agent steps: inject only if agent is idle               │
│                  "agent"  → if agent_is_idle(step.agent):                   │
│                                 inject_prompt(step)                         │
│                                 step.status = "running"                     │
│                                                                             │
│          persist_state()                                                    │
│                                                                             │
│      run_cleanup()                                                          │
│      shutdown_all_agents()                                                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Agent Idle Check

An agent is **idle** if no step assigned to it currently has status `running` or `completing`:

```go
func agentIsIdle(agentID string, workflow Workflow) bool {
    for _, step := range workflow.steps {
        if step.agent == agentID &&
           (step.status == "running" || step.status == "completing") {
            return false
        }
    }
    return true
}
```

The `completing` check prevents injecting a new prompt while the orchestrator is still processing the previous step's completion.

### Priority Order

When multiple steps are ready:

1. **Orchestrator executors first** — `shell`, `spawn`, `kill`, `expand`, `branch`
2. **Then agent executor** — `agent`
3. **Within category**: Earlier creation time, then lexicographic ID

This ensures machinery completes before agents are given more work.

### Step Readiness

```go
func isReady(step Step, workflow Workflow) bool {
    if step.status != "pending" {
        return false
    }
    for _, depID := range step.needs {
        dep := workflow.steps[depID]
        if dep.status != "done" {
            return false
        }
    }
    return true
}

func getAllReadySteps(workflow Workflow) []Step {
    var ready []Step
    for _, step := range workflow.steps {
        if isReady(step, workflow) {
            ready = append(ready, step)
        }
    }
    // Sort by priority: orchestrator executors first, then by creation time
    sort(ready, byPriorityThenCreationTime)
    return ready
}
```

---

## Agent Interaction Model

### Orchestrator Drives Everything

Unlike traditional systems where agents poll for work, MEOW's orchestrator **pushes** work to agents:

1. Orchestrator spawns agent in tmux
2. Orchestrator injects prompt directly via `tmux send-keys`
3. Agent works until calling `meow done`
4. Orchestrator receives completion via IPC
5. Orchestrator sends ESC to pause agent
6. Orchestrator injects next prompt (or kills session)

Agents never need to run `meow prime` themselves—the orchestrator handles all prompt delivery.

### The Step Completion Flow

When an agent calls `meow done`, the orchestrator finds the next step **for that specific agent**:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    STEP COMPLETION SEQUENCE                                  │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Claude        meow done CLI      Orchestrator        tmux session          │
│  ──────        ────────────       ────────────        ────────────          │
│    │                │                  │                   │                │
│    │ $ meow done    │                  │                   │                │
│    │   --output x=y │                  │                   │                │
│    │───────────────►│                  │                   │                │
│    │                │                  │                   │                │
│    │                │ Validate outputs │                   │                │
│    │                │ (locally first)  │                   │                │
│    │                │                  │                   │                │
│    │                │ IPC: StepDone {  │                   │                │
│    │                │   workflow,      │                   │                │
│    │                │   agent, step,   │                   │                │
│    │                │   outputs        │                   │                │
│    │                │ }                │                   │                │
│    │                │─────────────────►│                   │                │
│    │                │                  │                   │                │
│    │                │                  │ Guard: check      │                │
│    │                │                  │ step.status ==    │                │
│    │                │                  │ "running"         │                │
│    │                │                  │                   │                │
│    │                │                  │ Set step.status   │                │
│    │                │                  │ = "completing"    │                │
│    │                │                  │                   │                │
│    │                │                  │ Validate outputs  │                │
│    │                │                  │ (authoritative)   │                │
│    │                │                  │                   │                │
│    │                │    ACK/ERROR     │                   │                │
│    │                │◄─────────────────│                   │                │
│    │                │                  │                   │                │
│    │ "Complete" or  │                  │                   │                │
│    │ "Error: ..."   │                  │                   │                │
│    │◄───────────────│                  │                   │                │
│    │                │                  │                   │                │
│    │                │                  │ (if valid)        │                │
│    │                │                  │ step.status="done"│                │
│    │                │                  │                   │                │
│    │                │                  │ Find next step    │                │
│    │                │                  │ FOR THIS AGENT    │                │
│    │                │                  │                   │                │
│    │                │                  │ Send ESC key      │                │
│    │                │                  │──────────────────►│ PAUSE          │
│    │                │                  │                   │                │
│    │                │                  │ If next found:    │                │
│    │                │                  │ Mark it "running" │                │
│    │                │                  │ Inject prompt     │                │
│    │                │                  │──────────────────►│ NEW PROMPT     │
│    │                │                  │                   │                │
│    │                │                  │ If no next step:  │                │
│    │                │                  │ Agent sits idle   │                │
│    │                │                  │ (stop hook will   │                │
│    │                │                  │  return empty)    │                │
│    │◄───────────────│──────────────────│───────────────────│                │
│    │                │                  │                   │                │
│    │ (continues or  │                  │                   │                │
│    │  waits idle)   │                  │                   │                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Key detail:** The orchestrator looks for the next ready step assigned to *that specific agent*, not just any ready step. This enables parallel agents—each agent only receives prompts for its own work.

```go
func handleStepCompletion(msg StepDoneMessage) {
    step := workflow.steps[msg.step]

    // Guard: only process if currently running
    if step.status != "running" {
        sendError(msg.agent, "Step is not in running state")
        return
    }

    step.status = "completing"  // Block other operations

    if !validateOutputs(step, msg.outputs) {
        step.status = "running"  // Back to running for retry
        sendError(msg.agent, "Invalid outputs...")
        return
    }

    step.status = "done"
    step.outputs = msg.outputs

    // Find next ready step FOR THIS SPECIFIC AGENT
    nextStep := getNextReadyStepForAgent(msg.agent)

    sendEsc(msg.agent)  // Pause agent

    if nextStep != nil {
        nextStep.status = "running"  // Claim it before injection
        injectPrompt(msg.agent, nextStep.prompt)
    }
    // If no next step, agent stays idle (stop hook returns empty)

    persist()  // Single atomic write
}

func getNextReadyStepForAgent(agentID string) *Step {
    for _, step := range workflow.steps {
        if step.agent == agentID && isReady(step, workflow) {
            return &step
        }
    }
    return nil  // No work for this agent right now
}
```

### The Stop Hook (Recovery Mechanism)

The stop hook is a **fallback** for unexpected situations. It fires only when Claude is **at the prompt waiting for user input**—not while Claude is actively working.

**Normal flow (orchestrator-driven):**
1. Agent calls `meow done` with outputs
2. Orchestrator receives via IPC, sets step to `completing`
3. Orchestrator sends **ESC key** to tmux session (forces Claude to stop and reach prompt)
4. Orchestrator validates outputs, marks step `done`
5. Orchestrator injects next prompt via `tmux send-keys`
6. Agent continues working on next step

The ESC key is critical—it ensures Claude reaches the prompt state so the orchestrator can inject the next prompt cleanly. The stop hook is **not** involved in normal operation.

**When the stop hook fires:**
- Claude asked an unexpected question
- Claude is waiting for a command that timed out
- Claude hit an error and stopped
- Any other scenario where Claude reaches the prompt unexpectedly

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

**How `meow prime` works in this context:**
1. Stop hook fires when Claude reaches prompt (waiting for input)
2. `meow prime` reads `MEOW_AGENT` and `MEOW_ORCH_SOCK` from environment
3. `meow prime` asks orchestrator via IPC: "What's the prompt for this agent?"
4. Orchestrator checks step status:
   - If `completing`: return empty (transition in progress, stay quiet)
   - If `running` + `autonomous`: return current step's prompt (nudge to continue)
   - If `running` + `interactive`: return empty (allow human conversation)
   - If no step running (agent idle): return empty (wait for orchestrator)
5. If prompt is returned, Claude continues; if empty, Claude waits naturally

**Key insight:** In autonomous mode, the stop hook re-injects the current prompt as a nudge: "hey, you stopped but shouldn't have—here's your task again." This creates the Ralph Wiggum loop that ensures persistence.

### Interactive Mode

For steps with `mode: interactive`:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         INTERACTIVE STEP FLOW                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. Orchestrator sees next step is 'agent' with mode: interactive           │
│  2. Injects prompt via tmux send-keys                                       │
│  3. Claude works, converses with user                                       │
│  4. Stop hook fires (Claude asked something)                                │
│  5. meow prime queries orchestrator:                                        │
│     • Step is 'running' and mode is 'interactive'                           │
│     • Return "" (empty) ← KEY DIFFERENCE                                    │
│  6. Claude waits naturally for user input                                   │
│  7. User responds, Claude continues                                         │
│  8. Eventually Claude calls: meow done                                      │
│  9. Normal orchestrator takeover (ESC, next step)                           │
│                                                                             │
│  The difference: Interactive steps return "" on stop hook                   │
│  while step is running. Autonomous steps return the prompt again.           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Error Handling in `meow done`

If output validation fails:

```bash
$ meow done --output wrong_field=value
Error: Missing required outputs

Required:
  ✗ task_id (string): Not provided

Usage:
  meow done --output task_id=<value>
```

Claude sees this error message. The step remains `running`. If Claude stops (tries to give up), the stop hook fires, `meow prime` returns the current prompt, and Claude naturally continues trying. The Ralph Wiggum loop ensures persistence.

---

## IPC and Communication

### Unix Domain Socket

Each orchestrator creates a socket for IPC:

```
/tmp/meow-{workflow_id}.sock
```

All `meow` CLI commands in agent sessions communicate through this socket.

**Note:** Workflow IDs should be kept short to avoid hitting the Unix socket path length limit (typically 108 bytes on Linux).

### Message Protocol

Messages are **single-line, newline-delimited JSON**. Each message must be serialized without formatting (no pretty printing) to ensure embedded newlines in strings are properly escaped as `\n`.

**Step completion (meow done → orchestrator):**
```json
{"type":"step_done","workflow":"wf-abc123","agent":"worker-1","step":"impl.write-tests","outputs":{"test_file":"src/test.ts"},"notes":"optional notes"}
```

**Response (orchestrator → meow done):**
```json
{"type":"ack","success":true}
```
```json
{"type":"error","message":"Missing required output: task_id"}
```

**Prompt request (meow prime → orchestrator):**
```json
{"type":"get_prompt","agent":"worker-1"}
```

**Response (orchestrator → meow prime):**
```json
{"type":"prompt","content":"## Write Tests\n\nWrite failing tests..."}
```
```json
{"type":"prompt","content":""}
```

**Session ID request (meow session-id → orchestrator):**
```json
{"type":"get_session_id","agent":"worker-1"}
```

**Response:**
```json
{"type":"session_id","session_id":"sess-xyz789"}
```

**Approval signal (meow approve/reject → orchestrator):**
```json
{"type":"approval","gate_id":"review-gate","approved":true,"notes":"LGTM"}
```
```json
{"type":"approval","gate_id":"review-gate","approved":false,"reason":"Missing tests"}
```

**Event signal (meow event → orchestrator):**
```json
{"type":"event","workflow":"wf-abc123","agent":"worker-1","event_type":"tool-completed","data":{"tool":"Write","file":"src/main.ts"}}
```

**Response:**
```json
{"type":"ack","success":true}
```

**Await event (meow await-event → orchestrator):**
```json
{"type":"await_event","event_type":"tool-completed","filter":{"agent":"worker-1"},"timeout_ms":30000}
```

**Response (event received):**
```json
{"type":"event_received","event_type":"tool-completed","data":{"tool":"Write","file":"src/main.ts"}}
```

**Response (timeout):**
```json
{"type":"timeout","message":"No matching event within 30s"}
```

### Future: Distributed Systems

The same protocol could run over HTTP/gRPC for distributed orchestration. The socket is an implementation detail that can be swapped.

---

## Persistence and Best-Effort Resume

### Directory Structure

```
.meow/
├── config.toml              # User configuration
├── templates/               # Template files (.meow.toml)
└── workflows/
    ├── wf-abc123.yaml       # Workflow instance state
    └── wf-def456.yaml
```

### Workflow State File

The workflow file is the **single source of truth** for runtime state. It grows as templates expand.

```yaml
# .meow/workflows/wf-abc123.yaml
id: wf-abc123
template: work-loop.meow.toml
status: running
started_at: 2026-01-08T21:00:00Z

# Cleanup scripts (from template) - all opt-in
cleanup_on_success: |
  git worktree remove .meow/worktrees/{{agent}} --force 2>/dev/null || true
# cleanup_on_failure and cleanup_on_stop omitted - preserve state on those exits

# Resolved variables
vars:
  agent: claude-1

# Active agents
agents:
  claude-1:
    tmux_session: meow-wf-abc123-claude-1
    claude_session: sess-xyz789  # Captured at spawn for resume
    workdir: /data/projects/myapp/.meow/worktrees/claude-1
    status: active
    current_step: impl.write-tests

# All steps with their state (grows with expansions)
steps:
  select:
    executor: agent
    agent: claude-1
    status: done
    started_at: 2026-01-08T21:00:00Z
    finished_at: 2026-01-08T21:02:00Z
    outputs:
      task_id: "PROJ-123"
      has_more: true

  implement:
    executor: expand
    status: done
    finished_at: 2026-01-08T21:02:01Z
    expanded_into: ["implement.load-context", "implement.write-tests", ...]

  implement.load-context:
    executor: agent
    agent: claude-1
    status: done
    expanded_from: implement
    finished_at: 2026-01-08T21:05:00Z

  implement.write-tests:
    executor: agent
    agent: claude-1
    status: running
    expanded_from: implement
    started_at: 2026-01-08T21:05:30Z
```

### Workflow Lifecycle

```
pending ──► running ──► cleaning_up ──► done
               │             │
               │             └──► stopped (via meow stop or SIGINT)
               │
               └──► failed ──► cleaning_up ──► failed
```

- `pending`: Initial state before orchestrator starts
- `running`: Workflow is executing
- `cleaning_up`: Running cleanup script, killing agents
- `done`: Successfully completed
- `failed`: A step failed (after cleanup)
- `stopped`: Manually stopped (after cleanup)

**On completion:** Workflow file remains (for history/debugging). Users can manually delete or archive.

### Atomic File Writes

The workflow file is written atomically using the **write-then-rename** pattern:

1. Write complete state to `{workflow_id}.yaml.tmp`
2. Atomic rename `{workflow_id}.yaml.tmp` → `{workflow_id}.yaml`

On POSIX systems, rename is atomic. This prevents corruption from mid-write crashes.

**Recovery from interrupted write:**
- If `.yaml.tmp` exists and `.yaml` is valid: delete temp file, use main
- If `.yaml` is corrupt/missing but `.tmp` exists: rename temp to main

### Concurrent Workflow Execution

Multiple workflows can run concurrently. Each orchestrator process locks only its own workflow file, leaving other workflows unaffected.

**Per-Workflow Locking:**
```
.meow/workflows/
├── wf-abc123.yaml        # Workflow state
├── wf-abc123.yaml.lock   # Lock held by orchestrator running wf-abc123
├── wf-def456.yaml        # Another workflow
└── wf-def456.yaml.lock   # Lock held by different orchestrator
```

**Lock Lifecycle:**
1. `meow run` generates a workflow ID
2. Acquires exclusive lock on `{workflow_id}.yaml.lock` using `flock(LOCK_EX|LOCK_NB)`
3. If lock fails → another orchestrator is running this workflow → exit with error
4. Orchestrator holds lock for entire execution
5. On exit (success, failure, or signal) → lock released, lock file deleted

**What This Enables:**
```bash
# Terminal 1: Run a feature implementation workflow
meow run feature-impl.meow.toml --var task=PROJ-123

# Terminal 2: Simultaneously run a different workflow
meow run code-review.meow.toml --var pr=456

# Both run in parallel, each with their own agents and state
```

**What's Still Prevented:**
- Running the **same** workflow ID twice (lock conflict)
- Multiple orchestrators writing to the same workflow file (data corruption)

**Design Rationale:**
- IPC sockets are already per-workflow (`/tmp/meow-{workflow_id}.sock`)
- Tmux sessions are already per-workflow (`meow-{workflow_id}-{agent}`)
- Per-workflow locks complete the isolation model

### Crash Recovery (Best-Effort Resume)

MEOW provides best-effort resume after crashes. This is **not** guaranteed recovery—it's a convenience that works well when steps are designed to be re-runnable.

#### What Recovery Does

On orchestrator startup (fresh or resume):

```
1. Load workflow state from .meow/workflows/{id}.yaml
   (Handle atomic write recovery if needed—see above)

2. If workflow status is "cleaning_up":
   - Resume cleanup (run cleanup script, kill agents)
   - Mark as stopped/failed/done based on prior status
   - Exit

3. Find steps with status: running or completing

4. For each such step:
   a. If status is "completing":
      - Treat as "running" (orchestrator crashed during transition)
      - The step will be re-evaluated below

   b. If executor is orchestrator type (shell, spawn, expand, branch, kill):
      - Reset to pending (orchestrator crashed mid-execution)
      - For "expand" specifically: delete any partially-expanded child steps
        (steps with expanded_from = this step's ID)

   c. If executor is agent:
      - Check if assigned agent is still alive (tmux session exists)
      - If alive: keep running, orchestrator will re-inject prompt on next loop
      - If dead: reset to pending (will need agent respawn)

5. Start IPC server
6. Resume orchestrator loop
```

#### What Recovery Cannot Do

| Scenario | What Happens | User Must Handle |
|----------|--------------|------------------|
| Shell step was mid-execution | Re-runs the command | Make commands idempotent |
| Agent wrote half a file | File may be corrupted | Agent must handle partial state |
| Agent made commits | Commits exist in git | May have duplicate/partial commits |
| Agent called external APIs | Effects already happened | Design for at-least-once delivery |
| Worktree was partially created | May fail or succeed | Use `|| true` guards |

**Key insight:** MEOW tracks *orchestration* state, not *world* state. The step graph is recoverable; the side effects of interrupted work are not.

#### Partial Expansion Recovery

If an `expand` step was `running` when the orchestrator crashed, the workflow file may contain partially-inserted child steps. On recovery:
1. Find all steps with `expanded_from: <expand-step-id>`
2. Delete these partial child steps from the workflow
3. Reset the expand step to `pending`
4. The expand will run cleanly on resume

#### Agent Re-injection

For agent steps that remain `running` after recovery (agent still alive), the orchestrator doesn't immediately re-inject. Instead, it waits for:
- The agent to call `meow done` (normal completion)
- The stop hook to fire (which calls `meow prime` and gets the current prompt)

This avoids injecting duplicate prompts into an agent that may still be working.

#### When Recovery Works Well

Recovery is most reliable when:
- Shell commands are idempotent (`git worktree add ... || true`)
- Agents can detect and handle partial prior work
- External effects are designed for at-least-once delivery
- Steps have clear success criteria that can be re-evaluated

#### When Recovery May Fail

Recovery may cause issues when:
- Shell commands have side effects that can't be repeated
- Agents don't handle being re-prompted gracefully
- External systems don't tolerate duplicate requests
- Prior partial work left corrupted state

---

## Workflow Cleanup (Opt-In)

Cleanup is **opt-in**. By default, no cleanup runs and all agents/state are preserved when a workflow exits. This allows debugging failed workflows and resuming interrupted work.

Templates can define conditional cleanup scripts that run on specific exit conditions.

### Definition

```toml
[main]
name = "my-workflow"

# Cleanup scripts are opt-in - define only the triggers you want
cleanup_on_success = """
# Runs when all steps complete successfully
git worktree remove .meow/worktrees/{{track_name}}-track --force 2>/dev/null || true
"""

cleanup_on_failure = """
# Runs when a step fails (optional - often you want to preserve state for debugging)
echo "Workflow failed - preserving worktree for debugging"
"""

cleanup_on_stop = """
# Runs on SIGINT/SIGTERM or meow stop (optional)
echo "Stopped by user"
"""
```

### Cleanup Triggers

| Trigger | When It Runs | Typical Use |
|---------|--------------|-------------|
| `cleanup_on_success` | All steps complete successfully | Remove worktrees, temp files |
| `cleanup_on_failure` | A step fails with `on_error: fail` | Log state, notify, preserve for debugging |
| `cleanup_on_stop` | User Ctrl+C, SIGTERM, or `meow stop` | Graceful shutdown |

**Default behavior (no cleanup defined):**
- Agents remain running in tmux sessions
- Worktrees and temp files preserved
- Workflow marked as done/failed/stopped without cleanup phase

### Cleanup Sequence

When cleanup is triggered (only if a cleanup script is defined for that trigger):

1. Workflow status set to `cleaning_up`
2. Persist state (so cleanup survives crash)
3. All agent tmux sessions killed (orchestrator-managed)
4. User cleanup script executed (60 second timeout)
5. Workflow status set to final state (`done`, `failed`, `stopped`)
6. Persist final state

### Cleanup Script Environment

- **Working directory:** Orchestrator's working directory
- **Variables:** All workflow variables and step outputs available
- **Timeout:** 60 seconds (SIGKILL if exceeded)
- **Errors:** Logged but don't prevent workflow termination

### Signal Handling

When the orchestrator receives SIGINT or SIGTERM:
- If `cleanup_on_stop` is defined: runs full cleanup sequence
- If `cleanup_on_stop` is **not** defined: just marks workflow as stopped (preserves agents/state)

```go
func handleSignal(wf *Workflow) {
    if wf.HasCleanup(StatusStopped) {
        runCleanup(wf, "stopped")  // Kills agents, runs script
    } else {
        wf.Stop()  // Just mark as stopped, preserve everything
    }
}
```

### Best Practices

**1. Cleanup scripts should be idempotent** (safe to run multiple times):

```bash
# Good: handles already-removed worktree
git worktree remove .meow/worktrees/{{agent}} --force 2>/dev/null || true

# Good: handles missing directory
rm -rf .meow/temp/{{workflow_id}} 2>/dev/null || true
```

**2. Preserve state on failure for debugging:**

```toml
# Only clean up on success - preserve worktree on failure for debugging
cleanup_on_success = """
git worktree remove .meow/worktrees/{{track_name}}-track --force
"""
# cleanup_on_failure intentionally omitted - want to debug
```

**3. Use cleanup_on_stop sparingly:**

```toml
# Most workflows should NOT define cleanup_on_stop
# This preserves agents so you can resume later
```

**Variables in cleanup:** Only reference variables that are guaranteed to exist. Workflow-level variables (from `--var`) are always available. Step outputs may not exist if the workflow failed early.

---

## Resource Limits

To prevent runaway workflows from exhausting system resources, MEOW enforces configurable limits.

### Configuration

```toml
# .meow/config.toml

[limits]
max_expansion_depth = 50       # Maximum nested expand/branch calls
max_total_steps = 1000         # Maximum steps in a single workflow
max_workflow_file_size = "10MB"  # Maximum workflow YAML file size
```

### Enforcement

**Expansion Depth:**
Each `expand` or `branch` step that adds child steps increments a depth counter. If `max_expansion_depth` is exceeded:
- The expand/branch step fails with error: `"max expansion depth exceeded: 50"`
- Workflow continues only if the step has `on_error: continue`

This prevents infinite recursion in templates like:
```toml
# Dangerous: infinite loop if condition always true
[main.steps.on_true]
template = "main"  # Self-reference
```

**Total Steps:**
After each expansion, the orchestrator checks total step count. If `max_total_steps` is exceeded:
- The expand step fails with error: `"max steps exceeded: 1000"`
- This catches runaway template expansions

**Workflow File Size:**
Before writing the workflow file, check its serialized size. If `max_workflow_file_size` is exceeded:
- Persist fails with error: `"workflow file size exceeded: 10MB"`
- The orchestrator should pause and alert rather than corrupt state

### Defaults

If not configured, these defaults apply:
- `max_expansion_depth`: 100
- `max_total_steps`: 10000
- `max_workflow_file_size`: 50MB

These are generous limits that should never trigger in normal operation.

### Performance Characteristics

Understanding MEOW's performance model helps design efficient workflows.

**Parallel Execution:**

| Component | Parallelism | Notes |
|-----------|-------------|-------|
| Agent steps | Fully parallel | Each agent works independently |
| Branch conditions | Fully parallel | Conditions run in goroutines |
| Shell steps | Fully parallel | Shell is sugar over branch, runs async |
| Expand steps | Sequential per tick | Template loading is fast |

**Note:** Shell steps use the same async execution path as branch conditions. Both run their commands in goroutines and honor the DAG parallelism contract.

**Serialization Points:**

Command completions (both branch and shell) serialize through the workflow mutex:

```
Goroutine 1: command completes → acquire mutex → save → release
Goroutine 2: command completes → [wait] → acquire mutex → save → release
Goroutine 3: command completes → [wait] → [wait] → acquire mutex → ...
```

Each completion involves:
- Mutex acquisition (~0-10ms wait, depends on contention)
- Workflow file read (~1-5ms, SSD)
- Output capture / expansion (~0.1ms, in-memory)
- Workflow file write (~1-5ms, SSD)

**Throughput Estimates:**

| Parallel Commands | Completion Time | Impact |
|-------------------|-----------------|--------|
| 10 | ~50-100ms | Imperceptible |
| 50 | ~250-500ms | Minimal |
| 100 | ~0.5-1s | Noticeable |
| 500 | ~2.5-5s | Significant |
| 1000+ | ~5-10s+ | Consider redesign |

**Recommendations for Large Workflows:**

1. **Use `max_concurrent` on foreach** to limit parallel iterations:
   ```yaml
   [[steps]]
   id = "parallel-workers"
   executor = "foreach"
   items = "{{tasks}}"
   template = ".worker"
   max_concurrent = 10  # Process at most 10 in parallel
   ```

2. **Avoid deep branch nesting** where each branch expands more branches. Prefer flat structures.

3. **Break into multiple workflows** for truly massive parallelism. Each workflow has its own state file and mutex.

4. **Accept reasonable latency** for orchestration. MEOW optimizes for correctness and durability over raw throughput.

**Memory Usage:**

- Each step: ~200-500 bytes in workflow state
- Each pending command: ~100 bytes (goroutine + tracking)
- 1000 steps: ~500KB workflow file
- 10000 steps: ~5MB workflow file

Memory is rarely a concern for realistic workflows.

---

## CLI Commands

### Workflow Management

```bash
# Run a workflow (starts orchestrator in foreground)
meow run template.meow.toml
meow run template.meow.toml#workflow-name
meow run template.meow.toml --var agent=claude-1 --var task_id=PROJ-123

# Resume an existing workflow
meow run --resume wf-abc123
meow run --workflow .meow/workflows/wf-abc123.yaml

# Stop workflow (from another terminal)
meow stop wf-abc123           # Graceful: runs cleanup, kills agents
meow stop wf-abc123 --force   # Immediate: SIGKILL agents, then cleanup

# Check workflow status
meow status
meow status wf-abc123
meow status --json

# List all workflows
meow list
meow list --status running
```

### Agent Commands (Run by Claude)

```bash
# Signal completion (for agents)
meow done
meow done --output key=value
meow done --output task_id=PROJ-123 --output priority=high
meow done --json '{"key": "value"}'
meow done --notes "Completion notes for handoff"

# Get current prompt (for stop hook recovery)
meow prime
meow prime --format prompt    # For stop-hook injection (returns raw text)
meow prime --format json      # Structured output

# Get agent's Claude session ID (for parent-child workflows)
meow session-id
meow session-id --agent worker-1  # Query specific agent
```

### Human Interaction

```bash
# List pending approval gates
meow gates
meow gates --workflow wf-abc123

# Approve a gate (unblocks meow await-approval)
meow approve wf-abc123 gate-id
meow approve wf-abc123 gate-id --notes "LGTM"

# Reject a gate (meow await-approval exits non-zero)
meow reject wf-abc123 gate-id
meow reject wf-abc123 gate-id --reason "Needs error handling"

# Block until approval/rejection (used in branch conditions)
meow await-approval gate-id
meow await-approval gate-id --timeout 24h
```

### Events

```bash
# Emit an event (called from agent hook configurations)
meow event <event-type> [--data key=value ...]
meow event tool-completed --data tool=Write --data file=src/main.ts
meow event progress --data percent=50 --data message="Halfway done"

# Wait for an event (used in branch conditions)
meow await-event <event-type> [--filter key=value ...] [--timeout duration]
meow await-event tool-completed --filter tool=Write --timeout 5m
meow await-event agent-stopped --timeout 30s

# Query step status (for event loops to check if main task is done)
meow step-status <step-id>
meow step-status main-task --is done      # Exit 0 if done, 1 otherwise
meow step-status main-task --is-not done  # Exit 0 if NOT done, 1 otherwise
```

### Adapter Management

```bash
# Install an adapter
meow adapter install github.com/meow-stack/adapter-claude
meow adapter install ./local-adapter-dir

# List installed adapters
meow adapter list

# Show adapter details
meow adapter show claude
meow adapter show claude --config   # Show adapter.toml contents

# Remove an adapter
meow adapter remove claude
```

### Debugging

```bash
# Show workflow details
meow show wf-abc123
meow show wf-abc123 --steps
meow show wf-abc123 step-id

# View execution trace
meow trace wf-abc123
meow trace wf-abc123 --follow

# Agent management
meow agents
meow agents --workflow wf-abc123
```

### Initialization

```bash
# Initialize MEOW in a project
meow init
meow init --template work-loop
```

---

## Multi-Agent Coordination

### Assigning Steps to Agents

Each step can specify which agent should execute it:

```yaml
[[steps]]
id = "frontend-work"
executor = "agent"
agent = "frontend-agent"
prompt = "Implement the UI components"

[[steps]]
id = "backend-work"
executor = "agent"
agent = "backend-agent"
prompt = "Implement the API endpoints"
```

### Setting Up Worktrees

Create worktrees with `shell` steps, pass to `spawn`:

```yaml
[[steps]]
id = "create-frontend-worktree"
executor = "shell"
command = |
  git worktree add -b meow/frontend .meow/worktrees/frontend HEAD
  echo ".meow/worktrees/frontend"
outputs = { path: { source: stdout } }

[[steps]]
id = "start-frontend"
executor = "spawn"
agent = "frontend-agent"
workdir = "{{create-frontend-worktree.outputs.path}}"
needs = ["create-frontend-worktree"]
```

### Parallel Execution

Steps without dependencies between them run in parallel across different agents. The orchestrator:
1. Finds **all** ready steps
2. For orchestrator steps: executes them (fast, sequential)
3. For agent steps: injects prompts to **all idle agents** simultaneously

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                      PARALLEL AGENT EXECUTION                                │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Time    Orchestrator              Agent A         Agent B         Agent C  │
│  ────    ────────────              ───────         ───────         ───────  │
│                                                                             │
│   t0     spawn-a, spawn-b,                                                  │
│          spawn-c (sequential,                                               │
│          fast)                                                              │
│                                                                             │
│   t1     inject prompt to A        Working...                               │
│          inject prompt to B                        Working...               │
│          inject prompt to C                                        Working..│
│          (all three now running)                                            │
│                                                                             │
│   t2     waiting...                Working...      Working...      Working..│
│                                                                             │
│   t3     B calls meow done                         Done!                    │
│          → no next step for B                      (idle)                   │
│          (join not ready)                                                   │
│                                                                             │
│   t4     waiting...                Working...      (idle)          Working..│
│                                                                             │
│   t5     A calls meow done         Done!                                    │
│          → no next step for A      (idle)                                   │
│          (join still not ready)                                             │
│                                                                             │
│   t6     C calls meow done                                         Done!    │
│          → join now ready!                                                  │
│          → inject join prompt to A Working...                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Joins (Waiting for Multiple Agents)

A **join** is simply a step with multiple entries in `needs`. The step becomes ready only when **all** dependencies are `done`:

```yaml
[[steps]]
id = "integration"
executor = "agent"
agent = "coordinator"
prompt = "All work complete. Integrate and verify."
needs = ["frontend-work", "backend-work", "test-work"]  # Waits for ALL three
```

The join is **implicit**—no special syntax needed. Dependency resolution handles it naturally.

### Full Parallel Example

```yaml
[[steps]]
id = "create-frontend-worktree"
executor = "shell"
command = "git worktree add ... && echo path"
outputs = { path: { source: stdout } }

[[steps]]
id = "create-backend-worktree"
executor = "shell"
command = "git worktree add ... && echo path"
outputs = { path: { source: stdout } }

[[steps]]
id = "start-frontend"
executor = "spawn"
agent = "frontend-agent"
workdir = "{{create-frontend-worktree.outputs.path}}"
needs = ["create-frontend-worktree"]

[[steps]]
id = "start-backend"
executor = "spawn"
agent = "backend-agent"
workdir = "{{create-backend-worktree.outputs.path}}"
needs = ["create-backend-worktree"]

[[steps]]
id = "frontend-work"
executor = "agent"
agent = "frontend-agent"
prompt = "..."
needs = ["start-frontend"]

[[steps]]
id = "backend-work"
executor = "agent"
agent = "backend-agent"
prompt = "..."
needs = ["start-backend"]

# JOIN: waits for BOTH agents to complete
[[steps]]
id = "integration"
executor = "agent"
agent = "frontend-agent"
prompt = "Both frontend and backend are complete. Integrate and test."
needs = ["frontend-work", "backend-work"]
```

### What Happens to Idle Agents?

When an agent completes its work but the join isn't ready yet:
1. Orchestrator finds no next step for that agent
2. Agent receives ESC (pause) but no new prompt
3. Stop hook fires, `meow prime` returns empty (no running step for this agent)
4. Agent waits naturally (Claude idle at tmux prompt)
5. When join becomes ready, main loop picks it up and assigns to designated agent
6. Orchestrator injects prompt to the designated agent via `tmux send-keys`

**Key insight:** The orchestrator can inject prompts to an idle agent at any time—the agent is just sitting at a tmux prompt waiting. This is different from the stop hook flow; here the orchestrator proactively pushes work via `send-keys`, not via the hook.

**Agent Assignment:**
The join step specifies which agent should execute it. Other agents that completed their pre-join work remain idle until the workflow either:
- Gives them more work (later steps assigned to them)
- Reaches completion (cleanup kills all agents)
- Fails (cleanup kills all agents)

### Parent-Child Workflows (Call Pattern)

```yaml
# Checkpoint parent, spawn child, resume parent when done

[[steps]]
id = "checkpoint"
executor = "shell"
command = "meow session-id --agent {{parent}}"
outputs = { session_id: { source: stdout } }

[[steps]]
id = "stop-parent"
executor = "kill"
agent = "{{parent}}"
needs = ["checkpoint"]

[[steps]]
id = "create-child-worktree"
executor = "shell"
command = "git worktree add ... && echo path"
outputs = { path: { source: stdout } }
needs = ["stop-parent"]

[[steps]]
id = "start-child"
executor = "spawn"
agent = "{{child}}"
workdir = "{{create-child-worktree.outputs.path}}"
needs = ["create-child-worktree"]

[[steps]]
id = "child-work"
executor = "expand"
template = "{{work_template}}"
variables = { agent = "{{child}}" }
needs = ["start-child"]

[[steps]]
id = "stop-child"
executor = "kill"
agent = "{{child}}"
needs = ["child-work"]

[[steps]]
id = "cleanup-worktree"
executor = "shell"
command = "git worktree remove ... || true"
needs = ["stop-child"]

[[steps]]
id = "resume-parent"
executor = "spawn"
agent = "{{parent}}"
resume_session = "{{checkpoint.outputs.session_id}}"
needs = ["cleanup-worktree"]
```

---

## Complete Examples

### Example 1: Simple Work Loop

```toml
# simple-loop.meow.toml

[main]
name = "simple-loop"

# Only clean up on success - preserve worktree on failure for debugging
cleanup_on_success = """
git worktree remove .meow/worktrees/{{agent}} --force 2>/dev/null || true
"""

[main.variables]
agent = { required = true }

[[main.steps]]
id = "create-worktree"
executor = "shell"
command = """
git worktree add -b meow/{{agent}} .meow/worktrees/{{agent}} HEAD 2>/dev/null || true
echo ".meow/worktrees/{{agent}}"
"""

[main.steps.outputs]
path = { source = "stdout" }

[[main.steps]]
id = "start"
executor = "spawn"
agent = "{{agent}}"
workdir = "{{create-worktree.outputs.path}}"
needs = ["create-worktree"]

[[main.steps]]
id = "find-work"
executor = "agent"
agent = "{{agent}}"
prompt = """
Find the next task to work on using your project's task system.

If there are no more tasks, output has_more=false.
"""
needs = ["start"]

[main.steps.outputs]
task_id = { required = false, type = "string" }
has_more = { required = true, type = "boolean" }

[[main.steps]]
id = "do-work"
executor = "agent"
agent = "{{agent}}"
prompt = "Implement task {{find-work.outputs.task_id}}"
timeout = "2h"
needs = ["find-work"]

[[main.steps]]
id = "continue"
executor = "branch"
condition = "test '{{find-work.outputs.has_more}}' = 'true'"
needs = ["do-work"]

[main.steps.on_true]
template = ".work-iteration"
variables = { agent = "{{agent}}" }

# Internal workflow for subsequent iterations (no spawn needed)
[work-iteration]
name = "work-iteration"
internal = true

[work-iteration.variables]
agent = { required = true }

[[work-iteration.steps]]
id = "find-work"
executor = "agent"
agent = "{{agent}}"
prompt = "Find the next task to work on. Output has_more=false if none remain."

[work-iteration.steps.outputs]
task_id = { required = false, type = "string" }
has_more = { required = true, type = "boolean" }

[[work-iteration.steps]]
id = "do-work"
executor = "agent"
agent = "{{agent}}"
prompt = "Implement task {{find-work.outputs.task_id}}"
timeout = "2h"
needs = ["find-work"]

[[work-iteration.steps]]
id = "continue"
executor = "branch"
condition = "test '{{find-work.outputs.has_more}}' = 'true'"
needs = ["do-work"]

[work-iteration.steps.on_true]
template = ".work-iteration"
variables = { agent = "{{agent}}" }
```

### Example 2: Human-Gated Deployment

```toml
# deploy.meow.toml

[main]
name = "deploy"

cleanup_on_success = """
git worktree remove .meow/worktrees/{{agent}} --force 2>/dev/null || true
"""

[main.variables]
agent = { required = true }
environment = { default = "staging" }

[[main.steps]]
id = "run-tests"
executor = "shell"
command = "npm test"

[[main.steps]]
id = "build"
executor = "shell"
command = "npm run build"
needs = ["run-tests"]

[[main.steps]]
id = "deploy-staging"
executor = "shell"
command = "kubectl apply -f k8s/staging/"
needs = ["build"]

[[main.steps]]
id = "create-worktree"
executor = "shell"
command = "git worktree add -b meow/{{agent}} .meow/worktrees/{{agent}} HEAD && echo .meow/worktrees/{{agent}}"
needs = ["deploy-staging"]

[main.steps.outputs]
path = { source = "stdout" }

[[main.steps]]
id = "start-agent"
executor = "spawn"
agent = "{{agent}}"
workdir = "{{create-worktree.outputs.path}}"
needs = ["create-worktree"]

[[main.steps]]
id = "smoke-test"
executor = "agent"
agent = "{{agent}}"
prompt = """
Run smoke tests against staging environment.
Report any issues found.
"""
timeout = "30m"
needs = ["start-agent"]

[main.steps.outputs]
issues_found = { required = true, type = "boolean" }
issues = { required = false, type = "json" }

[[main.steps]]
id = "notify-reviewer"
executor = "shell"
command = """
echo "Staging deployment complete for {{workflow_id}}"
echo "Issues found: {{smoke-test.outputs.issues_found}}"
echo ""
echo "To approve: meow approve {{workflow_id}} approval-gate"
echo "To reject:  meow reject {{workflow_id}} approval-gate --reason '...'"
"""
needs = ["smoke-test"]

[[main.steps]]
id = "approval-gate"
executor = "branch"
condition = "meow await-approval approval-gate --timeout 24h"
needs = ["notify-reviewer"]

[main.steps.on_true]
inline = []  # Continue on approval

[main.steps.on_false]
inline = [
  { id = "log-rejection", executor = "shell",
    command = "echo 'Deployment rejected' >> deploy.log" }
]

[[main.steps]]
id = "deploy-prod"
executor = "shell"
command = "kubectl apply -f k8s/production/"
needs = ["approval-gate"]

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "{{agent}}"
needs = ["deploy-prod"]
```

### Example 3: Parallel Implementation with Watchdog

```toml
# parallel-with-watchdog.meow.toml

[main]
name = "parallel-impl"

cleanup_on_success = """
for agent in planner frontend backend watchdog; do
    git worktree remove ".meow/worktrees/$agent" --force 2>/dev/null || true
done
"""

[main.variables]
task_id = { required = true }

# ... worktree creation steps ...

[[main.steps]]
id = "start-watchdog"
executor = "spawn"
agent = "watchdog"
workdir = "{{create-watchdog-worktree.outputs.path}}"
needs = ["create-watchdog-worktree"]

[[main.steps]]
id = "watchdog-patrol"
executor = "agent"
agent = "watchdog"
prompt = """
You are a watchdog agent. Monitor the other agents for stuck behavior.

Every 5 minutes:
1. Check .meow/workflows/{{workflow_id}}.yaml for step status
2. For any step that's been "running" for >30 minutes with no file changes:
   - Send interrupt: tmux send-keys -t meow-{{workflow_id}}-<agent> C-c
   - Log the intervention

Commands available:
- tmux send-keys -t meow-{{workflow_id}}-frontend C-c
- tmux send-keys -t meow-{{workflow_id}}-backend C-c
- git -C .meow/worktrees/<agent> log --oneline -1

When the workflow completes (all other steps done), call meow done.
"""
needs = ["start-watchdog"]
# No timeout - watchdog runs until workflow ends

# ... rest of parallel workflow ...

[[main.steps]]
id = "cleanup-watchdog"
executor = "kill"
agent = "watchdog"
needs = ["integration"]
```

---

## Error Handling

### Step Failure

When a step fails:

```yaml
steps:
  my-step:
    status: failed
    error:
      type: "command_failed"          # Error classification
      message: "Command exited with code 1"
      code: 1
      output: "npm ERR! ..."
```

### On-Error Handling

The `on_error` field controls what happens when a step fails. It accepts three types of values:

| Value | Behavior |
|-------|----------|
| `"fail"` (default) | Workflow fails immediately |
| `"continue"` | Log error, skip step, continue to next |
| `".template-ref"` | Expand recovery template with error context |

**Simple modes:**

```yaml
# Fail workflow on error (default)
id = "critical-step"
executor = "shell"
command = "npm test"
on_error = "fail"

# Continue on error (for optional steps)
id = "optional-notification"
executor = "shell"
command = "curl https://slack.com/webhook/..."
on_error = "continue"
```

**Template-based recovery:**

```yaml
id = "implement-feature"
executor = "agent"
agent = "worker"
prompt = "Implement feature X"
timeout = "2h"
on_error = ".impl-recovery"  # Expand this template on failure
```

When a step fails and `on_error` references a template:
1. Step is marked `failed`
2. Error context is captured in `_failed_step` variables
3. Recovery template is expanded
4. Workflow continues from the recovery template

### Recovery Context (`_failed_step`)

When a recovery template is expanded, these variables are available:

| Variable | Description |
|----------|-------------|
| `_failed_step.id` | ID of the step that failed |
| `_failed_step.executor` | Executor type (`agent`, `shell`, etc.) |
| `_failed_step.error_type` | Error classification (see below) |
| `_failed_step.error_message` | Human-readable error description |
| `_failed_step.started_at` | When the step started |
| `_failed_step.duration` | How long the step ran |
| `_failed_step.retries` | Number of recovery attempts so far |
| `_failed_step.agent` | Agent ID (for agent steps) |
| `_failed_step.prompt` | Original prompt (for agent steps) |

**Recovery template example:**

```toml
# Retry with escalation after 3 failures
[impl-recovery]
internal = true

[[impl-recovery.steps]]
id = "check-retries"
executor = "branch"
condition = "test {{_failed_step.retries}} -lt 3"

[impl-recovery.steps.on_true]
template = ".retry-implementation"

[impl-recovery.steps.on_false]
template = ".escalate-to-human"


[retry-implementation]
internal = true

[[retry-implementation.steps]]
id = "kill-stuck-agent"
executor = "kill"
agent = "{{_failed_step.agent}}"

[[retry-implementation.steps]]
id = "spawn-fresh"
executor = "spawn"
agent = "retry-{{_failed_step.retries}}"
workdir = "{{worktree}}"
needs = ["kill-stuck-agent"]

[[retry-implementation.steps]]
id = "retry"
executor = "agent"
agent = "retry-{{_failed_step.retries}}"
prompt = """
Previous attempt failed: {{_failed_step.error_message}}

Please try again with a simpler approach:
{{_failed_step.prompt}}
"""
timeout = "2h"
needs = ["spawn-fresh"]


[escalate-to-human]
internal = true

[[escalate-to-human.steps]]
id = "notify"
executor = "shell"
command = """
echo "Task failed after {{_failed_step.retries}} retries"
echo "Error: {{_failed_step.error_message}}"
echo "Approve to skip: meow approve {{workflow_id}} skip-gate"
"""

[[escalate-to-human.steps]]
id = "skip-gate"
executor = "branch"
condition = "meow await-approval skip-gate --timeout 24h"
needs = ["notify"]

[escalate-to-human.steps.on_true]
inline = []  # Human approved, continue workflow

[escalate-to-human.steps.on_false]
inline = [
  { id = "abort", executor = "shell", command = "exit 1", on_error = "fail" }
]
```

### Error Types by Executor

Each executor can produce specific error types:

**Agent Executor:**
| Error Type | Cause |
|------------|-------|
| `timeout` | Step duration exceeded `timeout` limit |
| `agent_not_found` | Agent's tmux session doesn't exist |
| `agent_crashed` | Agent's tmux session died during execution |

**Shell Executor:**
| Error Type | Cause |
|------------|-------|
| `command_failed` | Command exited with non-zero code |
| `timeout` | Command exceeded timeout (if specified) |

**Spawn Executor:**
| Error Type | Cause |
|------------|-------|
| `spawn_failed` | tmux session creation failed, command failed to start, or workdir doesn't exist |

**Kill Executor:**
The `kill` executor is **idempotent**—killing a non-existent session succeeds. This matches the goal: "ensure agent is not running."

**Expand Executor:**
| Error Type | Cause |
|------------|-------|
| `template_not_found` | Referenced template doesn't exist |
| `expansion_limit` | Max expansion depth or step count exceeded |

**Branch Executor:**
| Error Type | Cause |
|------------|-------|
| `condition_error` | Condition command crashed (not just returned false) |
| `expansion_limit` | Branch expansion exceeded limits |

**Foreach Executor:**
| Error Type | Cause |
|------------|-------|
| `invalid_items` | Items expression didn't evaluate to a JSON array |
| `expansion_limit` | Max expansion depth or step count exceeded |
| `child_failed` | A child step failed (with `join: true`) |

### Agent Step Errors (Validation vs Failure)

**Output validation failures are NOT step failures.** When an agent calls `meow done` with invalid outputs:

1. Error message returned to agent
2. Step remains `running` (not failed!)
3. Agent sees error and should retry
4. If agent stops, stop hook fires
5. `meow prime` re-injects current prompt
6. Agent continues naturally (Ralph Wiggum loop)

The step only fails on:
- **Timeout**: External time limit reached
- **Agent crash**: tmux session died
- **Agent not found**: tmux session doesn't exist

This preserves the Ralph Wiggum loop—agents can't give up on validation, they must keep trying until they succeed or timeout.

### Agent Step Timeout

If an agent step exceeds its `timeout`:
1. Orchestrator sends C-c to agent's tmux session
2. Waits 10 seconds for graceful stop
3. Step marked `failed` with `error_type: timeout`
4. If `on_error` is a template, expand it
5. Otherwise, workflow continues based on `on_error` setting

### Workflow Failure

When a step fails and `on_error` is `"fail"`:
1. Workflow status becomes `failed`
2. No more steps are executed
3. Cleanup runs (kills agents, runs cleanup script)
4. Human intervention required (fix and resume, or abandon)

### Recovery Depth Limit

Recovery templates can themselves have `on_error` handlers, enabling nested recovery. To prevent infinite recovery loops, there's a maximum recovery depth (default: 3). If recovery keeps failing beyond this depth, the workflow fails.

### Retry Support (Future)

Built-in retry with backoff may be added as syntactic sugar:

```yaml
id = "flaky-step"
executor = "shell"
command = "..."
retry:
  max_attempts: 3
  delay: "10s"
  backoff: "exponential"
on_error = ".handle-final-failure"  # After all retries exhausted
```

Until then, retry logic can be implemented via recovery templates as shown above.

---

## Best Practices

### Design for Re-Execution

Since MEOW provides best-effort resume (not guaranteed durability), steps should be designed to handle being re-run after a crash. This applies to both shell commands and agent prompts.

**Why this matters:** If the orchestrator crashes while a step is running, that step will be reset to `pending` and re-executed on resume. Your workflow should handle this gracefully.

### Idempotent Shell Commands

Shell commands should be **idempotent**—safe to run multiple times. This is critical for crash recovery, where orchestrator steps may be re-executed.

**Bad: Fails if worktree exists**
```bash
git worktree add -b meow/agent .meow/worktrees/agent HEAD
```

**Good: Idempotent version**
```bash
if [ ! -d ".meow/worktrees/agent" ]; then
  git worktree add -b meow/agent .meow/worktrees/agent HEAD
fi
echo ".meow/worktrees/agent"
```

**Alternative: Use error suppression carefully**
```bash
git worktree add -b meow/agent .meow/worktrees/agent HEAD 2>/dev/null || true
echo ".meow/worktrees/agent"
```

This works but swallows all errors. The explicit check is safer.

### Common Idempotent Patterns

| Operation | Idempotent Pattern |
|-----------|-------------------|
| Create directory | `mkdir -p path` |
| Create worktree | Check if exists first |
| Create branch | `git branch branch_name 2>/dev/null \|\| true` |
| Write file | Just write (overwrites) |
| Remove directory | `rm -rf path` (already idempotent) |

### Re-Runnable Agent Prompts

Agent prompts may be re-injected after a crash. Design prompts so agents can handle being asked to do something they may have already partially completed.

**Fragile: Assumes fresh start**
```
Create the user authentication module from scratch.
```

**Robust: Handles partial prior work**
```
Implement user authentication. Check if auth files already exist—if so,
review and complete them rather than starting over. If starting fresh,
create the module from scratch.
```

**Even better: Define success criteria**
```
Ensure user authentication is fully implemented and tested.

Success criteria:
- src/auth/login.ts exists with working login function
- src/auth/logout.ts exists with working logout function
- Tests in src/auth/*.test.ts pass

If any of these are missing or broken, fix them. If all criteria are met,
you're done.
```

The more clearly you define "what done looks like," the better agents can handle being re-prompted mid-task.

### Worktree Management

Worktrees are user-managed. Use opt-in cleanup to handle teardown on success:

```toml
[main]
# Only clean up worktrees on success - preserve on failure for debugging
cleanup_on_success = """
git worktree remove .meow/worktrees/{{agent}} --force 2>/dev/null || true
"""
```

For multi-agent workflows:
```toml
cleanup_on_success = """
for agent in frontend backend coordinator; do
    git worktree remove ".meow/worktrees/$agent" --force 2>/dev/null || true
done
"""
```

### Template Organization

**Single-file modules:**
For related workflows, keep them in one file:
```toml
# work-loop.meow.toml
[main]           # Entry point
[tdd]            # TDD implementation (internal)
[work-iteration] # Loop iteration (internal)
```

**Shared utilities:**
For common patterns used across workflows:
```toml
# lib/common.meow.toml
[setup-agent]    # Worktree + spawn pattern
[cleanup-agent]  # Kill + worktree removal
```

Reference with: `template = "lib/common#setup-agent"`

### Error Handling Strategies

**Fail fast (default):**
Most steps should fail the workflow on error. Fix and resume.

**Continue on failure:**
Use sparingly for optional/cleanup steps:
```yaml
on_error = "continue"
```

**Conditional handling with branch:**
For complex error flows, use a branch to check state:
```yaml
[[steps]]
id = "check-tests"
executor = "branch"
condition = "npm test"
on_false:
  template = ".handle-test-failure"
```

### Parallel Agent Guidelines

**Keep agents isolated:**
- Each agent should have its own worktree
- Avoid agents modifying the same files
- Use the integration step to merge work

**Plan for join delays:**
When agents finish at different times, idle agents wait. Design workflows so this wait time is acceptable.

**Limit concurrent agents:**
More agents = more tmux sessions = more resource usage. Start with 2-3 parallel agents and scale up based on need.

### User-Space Monitoring

For sophisticated stuck detection beyond simple timeouts, implement a **watchdog agent**:

```toml
[[steps]]
id = "watchdog-patrol"
executor = "agent"
agent = "watchdog"
prompt = """
Monitor other agents. Every 5 minutes:
1. Check workflow state for stuck steps
2. Check git logs for progress
3. Send C-c interrupt if agent seems stuck

Use tmux send-keys -t meow-{{workflow_id}}-<agent> C-c to interrupt.
"""
```

This pattern allows custom "stuck" definitions and intervention strategies without adding complexity to the orchestrator.

---

## Implementation Phases

### Phase 1: Core Engine

- [ ] Step model and status management
- [ ] Step ID validation (no dots allowed)
- [ ] Template parsing (module format)
- [ ] `shell` executor
- [ ] `agent` executor with prompt injection
- [ ] Agent modes: `autonomous`, `interactive`, `fire_forget`
- [ ] Basic persistence (workflow YAML files)
- [ ] IPC server (Unix socket, single-line JSON)
- [ ] `meow run`, `meow prime`, `meow done` commands
- [ ] Variable substitution

### Phase 2: Full Executors

- [ ] `spawn` executor with tmux management and session ID capture
- [ ] `kill` executor
- [ ] `expand` executor with template resolution
- [ ] `branch` executor with condition evaluation
- [ ] `meow await-approval` command for human gates
- [ ] `meow approve`/`meow reject` commands

### Phase 3: Robustness

- [ ] Best-effort resume (resume workflows after crash)
- [ ] Output validation (all types)
- [ ] Error handling (`on_error` modes: fail, continue)
- [ ] `completing` state handling with guards
- [ ] Atomic file writes (write-then-rename)
- [ ] Partial expansion recovery
- [ ] Resource limits enforcement
- [ ] Shell context escaping for variable substitution
- [ ] Workflow cleanup hook
- [ ] Signal handling (SIGINT/SIGTERM)
- [ ] Agent step timeouts
- [ ] Error type classification per executor

### Phase 4: Multi-Agent

- [ ] Parallel step execution
- [ ] Multiple agents per workflow
- [ ] `meow session-id` command
- [ ] Session resume support (`resume_session`)

### Phase 5: Agent Adapters

- [ ] Adapter configuration format (`adapter.toml`)
- [ ] Adapter loading and resolution
- [ ] `adapter` field in spawn steps
- [ ] `meow adapter install/list/show/remove` commands
- [ ] Built-in claude adapter
- [ ] Adapter setup script execution

### Phase 6: Events

- [ ] Event IPC message handling
- [ ] `meow event` command
- [ ] `meow await-event` command with timeout
- [ ] Event routing to waiters
- [ ] `meow step-status` command for event loops
- [ ] Event translator script pattern documentation

### Phase 7: Dynamic Iteration

- [ ] `foreach` executor
- [ ] Items expression evaluation (JSON arrays)
- [ ] `item_var` and `index_var` substitution
- [ ] Parallel iteration with `max_concurrent` limiting
- [ ] Sequential iteration (`parallel: false`)
- [ ] Implicit join semantics (`join: true` default)
- [ ] `join: false` fire-and-forget mode
- [ ] Child step ID prefixing (`foreach-id.index.step-id`)
- [ ] Wildcard patterns in `needs` field

### Phase 8: Advanced Error Handling

- [ ] Template-based `on_error` recovery
- [ ] `_failed_step` context variables
- [ ] Recovery depth limiting
- [ ] Error type propagation to recovery templates
- [ ] Retry count tracking across recovery attempts

### Phase 9: Developer Experience

- [ ] `meow run --resume` for workflow continuation
- [ ] Template validation command
- [ ] Workflow visualization
- [ ] Debug mode / trace logging

---

## Appendix: Design Decisions

### Why Orchestrator-Centric?

The orchestrator owns prompt injection because:
- Single source of truth for "what's happening"
- Single writer to state file (no race conditions)
- Cleaner step transitions (no concurrent modification)
- Stop hook is just recovery, not primary mechanism
- Enables future distributed orchestration

### Why No Tiers?

The original design had three "tiers" (work, wisp, orchestrator). This was complexity from trying to integrate with external task tracking. With task tracking as the user's concern, tiers disappear. There's just: steps and who executes them.

### Why No Beads Integration?

MEOW is task-tracking agnostic. Users can use any system—beads, Jira, GitHub, markdown files, sticky notes, or nothing at all. The workflow template tells agents how to interact with whatever system exists. This is maximum flexibility.

### Why No Context Management?

Context management is Claude's concern, not MEOW's:
- Users can enable Claude's auto-compact
- Users can add explicit `branch` steps with context-checking scripts
- MEOW stays focused on coordination, not LLM internals

### Why Adapters?

MEOW needs to be agent-agnostic—it should work with Claude Code, Aider, Cursor, or any future AI agent that runs in a terminal. Agent-specific concerns include:

- **Start command**: Different flags, options, permissions
- **Prompt injection**: Different keystroke sequences
- **Graceful shutdown**: Different interrupt handling
- **Session resume**: Different mechanisms for continuing work
- **Event systems**: Hooks, callbacks, watchers (varies by agent)

Without adapters, these would be hardcoded in MEOW core, creating tight coupling and limiting extensibility. Adapters allow:

- Users to add support for new agents without modifying MEOW
- Different configurations for the same agent (e.g., claude with different models)
- Agent-specific setup scripts without polluting core
- Clean separation of concerns

The adapter format is intentionally simple—TOML config plus optional shell scripts—because:
- Shell scripts are auditable (users can read them before installing)
- No compiled plugins or binary blobs that could hide malicious code
- Same trust model as templates (both contain shell code)
- Easy to create and share

### Why Events?

Events provide a generic mechanism for agents to signal the orchestrator without completing a step. Use cases:

- **Monitoring**: React to tool calls, errors, or progress
- **Reactive workflows**: Trigger actions based on agent behavior
- **Watchdog patterns**: Alert on specific conditions

Events are explicitly **optional and fire-and-forget** because:
- Core orchestration shouldn't depend on agent-specific event systems
- Workflows must work even if events aren't configured
- Event handling adds complexity; not all workflows need it

The event system is built on existing primitives (`branch` + `await-event`), not a new executor, keeping the core minimal.

### Why Generic Output Types Only?

No `bead_id` or other system-specific types because:
- MEOW is task-tracking agnostic
- Users validate external references via `shell` steps if needed
- Generic types (`string`, `number`, `boolean`, `json`, `file_path`) cover all cases

### Why User-Managed Worktrees?

Worktrees are created via `shell` steps because:
- Full user control over git operations
- No hidden magic
- Works with any git workflow
- Cleanup is explicit (via cleanup hook)

### Why YAML for State?

- Human-readable for debugging and forensics
- No database dependency
- Easy to inspect when things go wrong
- Atomic file writes prevent corruption
- Enables best-effort resume (not full durability—see [Crash Recovery](#crash-recovery-best-effort-resume))

### Why Templates in TOML?

- Clean syntax for workflow definitions
- Good support for multi-line strings (prompts)
- Established in the ecosystem
- Distinct from state files (YAML)

### Why Seven Executors?

This is the minimal set that enables all coordination patterns:

| Pattern | Executors Used |
|---------|----------------|
| Sequential work | `agent` with `needs` |
| Conditional | `branch` |
| Loops | `branch` + recursive `expand` |
| Static parallel | Multiple `agent` steps |
| Dynamic parallel | `foreach` with parallel iterations |
| Agent lifecycle | `spawn`, `kill` |
| Setup/teardown | `shell` |
| Human approval | `branch` + `meow await-approval` |
| Composition | `expand` |
| Iteration over lists | `foreach` |

Human gates were originally a separate executor, but they're just blocking conditions—`branch` with a command that waits for approval. Removing the dedicated `gate` executor simplifies the system and makes approval mechanisms user-customizable.

The `foreach` executor was added because dynamic iteration over runtime-generated lists is a common pattern that's cumbersome to express with `branch` + recursive `expand`. While technically possible without `foreach`, the ergonomic benefit of native support for iteration justifies the additional executor.

### Why No Built-in Stuck Detection?

Detecting "stuck" is hard:
- How long is too long? Complex tasks take hours.
- "No progress" requires understanding what progress means.
- False positives are worse than false negatives.

Instead, MEOW provides:
- Simple `timeout` field for agent steps (handles hung tools)
- Cleanup hook for guaranteed teardown
- Watchdog agent pattern for sophisticated monitoring

Users compose the monitoring behavior they need.

### How Parallel Agents Work

Parallelism emerges naturally from the dependency model:
- Steps with no dependencies between them become ready simultaneously
- The orchestrator processes **all** ready agent steps, injecting prompts to each idle agent
- Each agent works independently until calling `meow done`
- **Joins** are just steps with multiple `needs` entries—ready when all dependencies are `done`
- Idle agents (no next step available) sit at the tmux prompt; the orchestrator can inject new work at any time

No special "parallel" or "fork/join" syntax needed—it's all implicit in the dependency graph.

### Why ESC Before Prompt Injection?

After an agent calls `meow done`, the orchestrator sends ESC before injecting the next prompt. This is intentional:

1. **Clean state:** ESC ensures Claude reaches the prompt, regardless of what it was doing
2. **Predictable injection:** `tmux send-keys` works reliably when Claude is at the prompt
3. **Stop hook coordination:** If no next prompt is available, the stop hook fires and calls `meow prime`, which returns empty—Claude waits naturally

The ESC key is the orchestrator's way of "taking control" of the agent session between steps.

### Stop Hook vs. Direct Injection

There are two ways prompts reach agents:

1. **Direct injection (normal flow):** Orchestrator sends ESC, then injects prompt via `tmux send-keys`. Stop hook is not involved.

2. **Stop hook recovery:** If Claude stops unexpectedly (not from `meow done`), the stop hook fires and calls `meow prime` to get the current prompt. This is a fallback mechanism.

In normal operation, the stop hook rarely fires. It's insurance against edge cases.

### Why Single-Line JSON for IPC?

JSON allows embedded newlines in strings (`"line1\nline2"`), but newline-delimited protocols assume one message per line. By requiring single-line JSON (no pretty printing), we ensure:
- Simple parsing with `readline()`
- Embedded newlines are properly escaped
- No ambiguity about message boundaries

---

*MEOW: The coordination language for AI agents.*
