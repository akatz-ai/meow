# MEOW Stack MVP Specification v2

**MEOW** (Molecular Expression Of Work) is a durable, recursive, composable coordination language for AI agent orchestration.

> **Core Insight**: MEOW is not a task tracker. It's a programming language for agent coordination. Task tracking is the user's concern—MEOW orchestrates agents through workflows that users program to interact with whatever systems they choose.

---

## Table of Contents

1. [Design Philosophy](#design-philosophy)
2. [The Single Primitive: Step](#the-single-primitive-step)
3. [Executors](#executors)
4. [Data Flow Between Steps](#data-flow-between-steps)
5. [Template System](#template-system)
6. [Orchestrator Architecture](#orchestrator-architecture)
7. [Agent Interaction Model](#agent-interaction-model)
8. [IPC and Communication](#ipc-and-communication)
9. [Persistence and Crash Recovery](#persistence-and-crash-recovery)
10. [Resource Limits](#resource-limits)
11. [CLI Commands](#cli-commands)
12. [Multi-Agent Coordination](#multi-agent-coordination)
13. [Complete Examples](#complete-examples)
14. [Error Handling](#error-handling)
15. [Best Practices](#best-practices)
16. [Implementation Phases](#implementation-phases)

---

## Design Philosophy

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

### No Special Cases

MEOW does **not** include:
- Context management (use Claude's auto-compact or explicit `branch` checks)
- Work item tracking integration (pure data flow via variables)
- Special types for external systems (generic types only)

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

#### Durable Execution

> **All workflow state survives crashes, restarts, and session boundaries.**

MEOW persists workflow state to simple YAML files. On crash:
1. Reload workflow state
2. Check which agents are still alive
3. Reset orphaned running steps to pending
4. Resume orchestration

No SQLite. No lock files. Just files.

#### Composition Over Complexity

> **Complex behaviors emerge from simple primitives composed together.**

MEOW has one primitive (Step) with seven executors. Loops, conditionals, parallel execution, agent handoffs—all emerge from composing these simple building blocks.

---

## The Single Primitive: Step

Everything in MEOW is a **Step**. A step has an executor that determines who runs it and how.

### Step Structure

```yaml
step:
  id: string              # Unique within workflow
  executor: ExecutorType  # Who runs this step
  status: Status          # pending | running | completing | done | failed
  needs: [string]         # Step IDs that must complete first
  outputs: map            # Captured output data (populated on completion)

  # Executor-specific fields vary by type
```

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

### External Executors

These wait for external completion signals.

| Executor | Purpose | Completes When |
|----------|---------|----------------|
| `agent` | Prompt an agent to do work | Agent runs `meow done` |
| `gate` | Wait for human approval | Human runs `meow approve` |

---

### `shell` — Run Shell Command

Execute a shell command and capture outputs. Use this for setup tasks like creating git worktrees.

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
1. Execute command in shell
2. Capture specified outputs
3. If exit code != 0 and `on_error: fail`, mark step `failed`
4. Otherwise, mark step `done` with outputs

---

### `spawn` — Start Agent Process

Start an agent in a tmux session. The working directory is typically created by a prior `shell` step.

```yaml
id: "start-worker"
executor: "spawn"
agent: "worker-1"
workdir: "{{create-worktree.outputs.worktree_path}}"  # From prior shell step
env:
  TASK_ID: "{{task_id}}"
  MY_CUSTOM_VAR: "value"
resume_session: "{{checkpoint.outputs.session_id}}"  # Optional
```

**Fields:**
- `agent` (required): Agent identifier
- `workdir` (optional): Working directory for the agent (defaults to current directory)
- `env` (optional): Additional environment variables (beyond orchestrator defaults)
- `resume_session` (optional): Claude session ID to resume

**Orchestrator-Injected Environment:**
The orchestrator always sets these environment variables in the tmux session:
- `MEOW_AGENT`: Agent identifier
- `MEOW_WORKFLOW`: Workflow instance ID
- `MEOW_ORCH_SOCK`: Path to orchestrator's IPC socket
- `MEOW_STEP`: Current step ID (updated on each new step)

User-provided `env` vars are merged with orchestrator defaults. **Orchestrator variables take precedence**—if a user specifies `MEOW_AGENT` in their `env` block, it will be overwritten. All `MEOW_*` environment variables are reserved.

**Orchestrator Behavior:**
1. Create tmux session `meow-{workflow_id}-{agent}`
2. Set working directory
3. Set environment variables
4. Start Claude (with `--resume {session_id}` if `resume_session` provided)
5. Wait for Claude ready prompt
6. Inject first step's prompt directly via `tmux send-keys`
7. Mark step `done`

**Note:** Worktrees are user-managed. Create them with `shell` steps, pass the path here. The orchestrator doesn't manage worktrees automatically.

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
3. Mark step `done`

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

Evaluate a condition and expand the appropriate branch.

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
1. Execute condition as shell command (may block)
2. Based on exit code, select appropriate branch
3. Expand selected branch (if any)
4. Mark branch step `done`

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
mode: "autonomous"  # autonomous | interactive
```

**Fields:**
- `agent` (required): Agent identifier
- `prompt` (required): Instructions for the agent
- `outputs` (optional): Expected output definitions
- `mode` (optional): `autonomous` (default) or `interactive`

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
- The orchestrator tracks each agent's workdir and uses it for validation

**Modes:**
- `autonomous`: Normal execution—on step completion, orchestrator immediately injects next prompt
- `interactive`: Pauses for human conversation—orchestrator returns empty prompt during stop hook, allowing natural Claude-human interaction

**Orchestrator Behavior:**
1. Mark step `running`
2. Update `MEOW_STEP` environment variable in agent's session
3. Inject prompt directly into agent's tmux session via `send-keys`
4. Wait for agent to signal completion via `meow done` (IPC)
5. Validate outputs against definitions
6. If valid: mark step `done` with outputs, proceed to next step
7. If invalid: return error to agent, keep step `running`, agent retries

---

### `gate` — Human Approval

Block until human approves or rejects.

```yaml
id: "review-gate"
executor: "gate"
prompt: |
  Review the implementation for {{task_id}}.

  Changes:
  - {{changes_summary}}

  Approve if ready to merge, reject with feedback if changes needed.
timeout: "24h"
```

**Fields:**
- `prompt` (required): Information for the human reviewer
- `timeout` (optional): How long to wait before auto-action

**Orchestrator Behavior:**
1. Mark step `running`
2. Wait for human to run `meow approve` or `meow reject`
3. On approve: mark step `done`
4. On reject: mark step `failed` with rejection reason

**Human Experience:**
```bash
$ meow gates
WORKFLOW      STEP          PROMPT
wf-abc123     review-gate   Review the implementation for PROJ-123...

$ meow approve wf-abc123 review-gate
Approved: review-gate

# OR

$ meow reject wf-abc123 review-gate --reason "Missing error handling"
Rejected: review-gate
```

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

## Orchestrator Architecture

### Orchestrator-Centric Design

The orchestrator is the **central authority**. It:
- Owns all prompt injection (agents never run `meow prime` themselves)
- Manages all agent lifecycles
- Handles all state transitions
- Communicates with agents via tmux and IPC

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
│                  "gate"   → step.status = "running"                         │
│                                                                             │
│          persist_state()                                                    │
│                                                                             │
│      shutdown_all_agents()                                                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Agent Idle Check

An agent is **idle** if no step assigned to it currently has status `running`:

```go
func agentIsIdle(agentID string, workflow Workflow) bool {
    for _, step := range workflow.steps {
        if step.agent == agentID && step.status == "running" {
            return false
        }
    }
    return true
}
```

This prevents injecting multiple prompts to the same agent simultaneously.

### Priority Order

When multiple steps are ready:

1. **Orchestrator executors first** — `shell`, `spawn`, `kill`, `expand`, `branch`
2. **Then external executors** — `agent`, `gate`
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
    step.status = "completing"

    if !validateOutputs(step, msg.outputs) {
        step.status = "running"  // Retry
        sendError(msg.agent, "Invalid outputs...")
        return
    }

    step.status = "done"
    step.outputs = msg.outputs

    // Find next ready step FOR THIS SPECIFIC AGENT
    nextStep := getNextReadyStepForAgent(msg.agent)

    sendEsc(msg.agent)  // Pause agent

    if nextStep != nil {
        injectPrompt(msg.agent, nextStep.prompt)
        nextStep.status = "running"
    }
    // If no next step, agent stays idle (stop hook returns empty)
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

### Message Protocol

Messages are newline-delimited JSON:

**Step completion (meow done → orchestrator):**
```json
{
  "type": "step_done",
  "workflow": "wf-abc123",
  "agent": "worker-1",
  "step": "impl.write-tests",
  "outputs": {"test_file": "src/test.ts"},
  "notes": "optional notes"
}
```

**Response (orchestrator → meow done):**
```json
{"type": "ack", "success": true}
// OR
{"type": "error", "message": "Missing required output: task_id"}
```

**Prompt request (meow prime → orchestrator):**
```json
{
  "type": "get_prompt",
  "agent": "worker-1"
}
```

**Response (orchestrator → meow prime):**
```json
{"type": "prompt", "content": "## Write Tests\n\nWrite failing tests..."}
// OR (during transition or interactive mode)
{"type": "prompt", "content": ""}
```

### Future: Distributed Systems

The same protocol could run over HTTP/gRPC for distributed orchestration. The socket is an implementation detail that can be swapped.

---

## Persistence and Crash Recovery

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

# Resolved variables
vars:
  agent: claude-1

# Active agents
agents:
  claude-1:
    tmux_session: meow-wf-abc123-claude-1
    status: active
    workdir: /data/projects/myapp/.meow/worktrees/claude-1
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
pending ──► running ──► done
               │
               └──► failed
```

**On completion:** Workflow file remains (for history/debugging). Users can manually delete or archive.

**Optional checkpointing:** Users who want git-based recovery can add `shell` steps:
```yaml
id: "checkpoint"
executor: "shell"
command: |
  cp .meow/workflows/{{workflow_id}}.yaml .meow/checkpoints/{{workflow_id}}-{{timestamp}}.yaml
```

### Atomic File Writes

The workflow file is written atomically using the **write-then-rename** pattern:

1. Write complete state to `{workflow_id}.yaml.tmp`
2. Atomic rename `{workflow_id}.yaml.tmp` → `{workflow_id}.yaml`

On POSIX systems, rename is atomic. This prevents corruption from mid-write crashes.

**Recovery from interrupted write:**
- If `.yaml.tmp` exists and `.yaml` is valid: delete temp file, use main
- If `.yaml` is corrupt/missing but `.tmp` exists: rename temp to main

### Crash Recovery

On orchestrator startup (fresh or resume):

```
1. Load workflow state from .meow/workflows/{id}.yaml
   (Handle atomic write recovery if needed—see above)

2. Find steps with status: running or completing

3. For each such step:
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

   d. If executor is gate:
      - Keep running (human might still approve)

4. Start IPC server
5. Resume orchestrator loop
```

**Partial Expansion Recovery:**
If an `expand` step was `running` when the orchestrator crashed, the workflow file may contain partially-inserted child steps. On recovery:
1. Find all steps with `expanded_from: <expand-step-id>`
2. Delete these partial child steps from the workflow
3. Reset the expand step to `pending`
4. The expand will run cleanly on resume

**Agent Re-injection:**
For agent steps that remain `running` after recovery (agent still alive), the orchestrator doesn't immediately re-inject. Instead, it waits for:
- The agent to call `meow done` (normal completion)
- The stop hook to fire (which calls `meow prime` and gets the current prompt)

This avoids injecting duplicate prompts into an agent that may still be working.

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
meow stop wf-abc123
meow stop wf-abc123 --force  # Kill immediately

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
```

### Human Interaction

```bash
# List pending gates
meow gates
meow gates --workflow wf-abc123

# Approve a gate
meow approve wf-abc123 step-id
meow approve wf-abc123 step-id --notes "LGTM"

# Reject a gate
meow reject wf-abc123 step-id
meow reject wf-abc123 step-id --reason "Needs error handling"
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
- Reaches completion (orchestrator kills all agents)
- Fails (agents left running, may need manual cleanup)

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
command = "git worktree add ... && echo path"
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
needs = ["start-agent"]

[main.steps.outputs]
issues_found = { required = true, type = "boolean" }
issues = { required = false, type = "json" }

[[main.steps]]
id = "approval"
executor = "gate"
prompt = """
Staging deployment complete. Smoke test results:
- Issues found: {{smoke-test.outputs.issues_found}}

Approve to deploy to production.
"""
needs = ["smoke-test"]

[[main.steps]]
id = "deploy-prod"
executor = "shell"
command = "kubectl apply -f k8s/production/"
needs = ["approval"]

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "{{agent}}"
needs = ["deploy-prod"]
```

### Example 3: Parallel Implementation

```toml
# parallel.meow.toml

[main]
name = "parallel-impl"

[main.variables]
task_id = { required = true }

[[main.steps]]
id = "create-planner-worktree"
executor = "shell"
command = "git worktree add -b meow/planner .meow/worktrees/planner HEAD && echo .meow/worktrees/planner"

[main.steps.outputs]
path = { source = "stdout" }

[[main.steps]]
id = "start-planner"
executor = "spawn"
agent = "planner"
workdir = "{{create-planner-worktree.outputs.path}}"
needs = ["create-planner-worktree"]

[[main.steps]]
id = "plan"
executor = "agent"
agent = "planner"
prompt = "Break down {{task_id}} into frontend and backend tasks."
needs = ["start-planner"]

[main.steps.outputs]
frontend_spec = { required = true, type = "string" }
backend_spec = { required = true, type = "string" }

[[main.steps]]
id = "create-frontend-worktree"
executor = "shell"
command = "git worktree add -b meow/frontend .meow/worktrees/frontend HEAD && echo .meow/worktrees/frontend"
needs = ["plan"]

[main.steps.outputs]
path = { source = "stdout" }

[[main.steps]]
id = "create-backend-worktree"
executor = "shell"
command = "git worktree add -b meow/backend .meow/worktrees/backend HEAD && echo .meow/worktrees/backend"
needs = ["plan"]

[main.steps.outputs]
path = { source = "stdout" }

[[main.steps]]
id = "start-frontend"
executor = "spawn"
agent = "frontend"
workdir = "{{create-frontend-worktree.outputs.path}}"
needs = ["create-frontend-worktree"]

[[main.steps]]
id = "start-backend"
executor = "spawn"
agent = "backend"
workdir = "{{create-backend-worktree.outputs.path}}"
needs = ["create-backend-worktree"]

[[main.steps]]
id = "impl-frontend"
executor = "agent"
agent = "frontend"
prompt = "Implement: {{plan.outputs.frontend_spec}}"
needs = ["start-frontend"]

[[main.steps]]
id = "impl-backend"
executor = "agent"
agent = "backend"
prompt = "Implement: {{plan.outputs.backend_spec}}"
needs = ["start-backend"]

[[main.steps]]
id = "integrate"
executor = "agent"
agent = "planner"
prompt = "Integrate frontend and backend, run full test suite."
needs = ["impl-frontend", "impl-backend"]

[[main.steps]]
id = "cleanup-frontend"
executor = "kill"
agent = "frontend"
needs = ["integrate"]

[[main.steps]]
id = "cleanup-backend"
executor = "kill"
agent = "backend"
needs = ["integrate"]

[[main.steps]]
id = "cleanup-planner"
executor = "kill"
agent = "planner"
needs = ["cleanup-frontend", "cleanup-backend"]
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
      message: "Command exited with code 1"
      code: 1
      output: "npm ERR! ..."
```

### On-Error Handling

For `shell` executor:

```yaml
id = "risky-command"
executor = "shell"
command = "npm test"
on_error = "continue"  # continue | fail (default: fail)
```

### Agent Step Errors

If `meow done` fails validation:
1. Error message returned to agent
2. Step remains `running`
3. Agent sees error and should retry
4. If agent stops, stop hook fires
5. `meow prime` re-injects current prompt
6. Agent continues naturally (Ralph Wiggum loop)

### Workflow Failure

When a step fails and `on_error` is `fail`:
1. Workflow status becomes `failed`
2. No more steps are executed
3. Human intervention required (fix and resume, or abandon)

### Retry Support (Future)

```yaml
id = "flaky-step"
executor = "shell"
command = "..."
retry:
  max_attempts: 3
  delay: "10s"
  backoff: "exponential"
```

---

## Best Practices

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

### Worktree Management

Worktrees are user-managed. Recommended patterns:

**Unique worktree per agent:**
```yaml
[[steps]]
id = "create-worktree"
executor = "shell"
command = """
WORKTREE=".meow/worktrees/{{agent}}"
if [ ! -d "$WORKTREE" ]; then
  git worktree add -b "meow/{{agent}}" "$WORKTREE" HEAD
fi
echo "$WORKTREE"
"""
```

**Cleanup on workflow completion:**
```yaml
[[steps]]
id = "cleanup-worktree"
executor = "shell"
command = "git worktree remove .meow/worktrees/{{agent}} --force || true"
needs = ["final-step"]
```

**Important:** Always clean up worktrees in success paths. For failed workflows, manual cleanup may be needed.

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

---

## Implementation Phases

### Phase 1: Core Engine

- [ ] Step model and status management
- [ ] Template parsing (module format)
- [ ] `shell` executor
- [ ] `agent` executor with prompt injection
- [ ] Basic persistence (workflow YAML files)
- [ ] IPC server (Unix socket)
- [ ] `meow run`, `meow prime`, `meow done` commands
- [ ] Variable substitution

### Phase 2: Full Executors

- [ ] `spawn` executor with tmux management
- [ ] `kill` executor
- [ ] `expand` executor with template resolution
- [ ] `branch` executor with condition evaluation
- [ ] `gate` executor with `meow approve`/`meow reject`

### Phase 3: Robustness

- [ ] Crash recovery (resume workflows)
- [ ] Output validation (all types)
- [ ] Error handling (on_error modes)
- [ ] `completing` state handling
- [ ] Atomic file writes (write-then-rename)
- [ ] Partial expansion recovery
- [ ] Resource limits enforcement
- [ ] Shell context escaping for variable substitution

### Phase 4: Multi-Agent

- [ ] Parallel step execution
- [ ] Multiple agents per workflow
- [ ] Session resume support (`resume_session`)

### Phase 5: Developer Experience

- [ ] `meow run --resume` for workflow continuation
- [ ] Template validation command
- [ ] Workflow visualization
- [ ] Debug mode / trace logging

---

## Appendix: Design Decisions

### Why Orchestrator-Centric?

The orchestrator owns prompt injection because:
- Single source of truth for "what's happening"
- Cleaner step transitions (no race conditions)
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
- Cleanup is explicit

### Why YAML for State?

- Human-readable for debugging
- No database dependency
- Simple crash recovery
- Atomic file writes

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
| Parallel | Multiple `agent` steps |
| Agent lifecycle | `spawn`, `kill` |
| Setup/teardown | `shell` |
| Human approval | `gate` |
| Composition | `expand` |

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

---

*MEOW: The coordination language for AI agents.*
