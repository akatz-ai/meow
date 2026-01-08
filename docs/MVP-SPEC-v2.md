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
8. [Persistence and Crash Recovery](#persistence-and-crash-recovery)
9. [CLI Commands](#cli-commands)
10. [Multi-Agent Coordination](#multi-agent-coordination)
11. [Complete Examples](#complete-examples)
12. [Error Handling](#error-handling)
13. [Implementation Phases](#implementation-phases)

---

## Design Philosophy

### MEOW is a Coordination Language

MEOW templates are **programs** that coordinate agents. They are not task lists, not tickets, not issues. They are executable specifications of how work flows through a system of agents.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         MEOW MENTAL MODEL                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Templates = Programs                                                       │
│  Steps = Instructions                                                       │
│  Executors = Who runs each instruction                                      │
│  Outputs = Data flowing between steps                                       │
│  Workflows = Running program instances                                      │
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

The **workflow template** encodes how to interact with external systems. MEOW just executes the workflow.

### Core Principles

#### The Propulsion Principle

> **When an agent finds work assigned to them, they execute immediately.**

MEOW is a steam engine. Agents are pistons. When `meow prime` returns a prompt, the agent acts. There's no polling, no checking—the workflow state is the instruction.

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
  status: Status          # pending | running | done | failed
  needs: [string]         # Step IDs that must complete first
  outputs: map            # Captured output data (populated on completion)

  # Executor-specific fields vary by type
```

### Status Lifecycle

```
pending ──► running ──► done
              │
              └──► failed
```

- `pending`: Waiting for dependencies or orchestrator attention
- `running`: Currently being executed
- `done`: Successfully completed, outputs captured
- `failed`: Execution failed (with error info)

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

Execute a shell command and capture outputs.

```yaml
id: "setup-branch"
executor: "shell"
command: |
  git checkout -b feature/{{task_id}}
  echo "feature/{{task_id}}"
outputs:
  branch_name:
    source: stdout
  exit_code:
    source: exit_code
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

Start an agent in a tmux session.

```yaml
id: "start-worker"
executor: "spawn"
agent: "worker-1"
workdir: "{{setup-branch.outputs.worktree_path}}"
env:
  TASK_ID: "{{task_id}}"
prompt: "meow prime"  # Initial prompt injection (default)
resume_session: "{{checkpoint.outputs.session_id}}"  # Optional
```

**Fields:**
- `agent` (required): Agent identifier
- `workdir` (optional): Working directory for the agent
- `env` (optional): Environment variables
- `prompt` (optional): Initial prompt to inject (default: `meow prime`)
- `resume_session` (optional): Claude session ID to resume

**Orchestrator Behavior:**
1. Create tmux session `meow-{agent}`
2. Set environment variables (always includes `MEOW_AGENT={agent}`)
3. Start Claude (with `--resume` if `resume_session` provided)
4. Wait for Claude ready prompt
5. Inject initial prompt
6. Mark step `done`

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
3. Generate unique step IDs for expanded steps
4. Insert expanded steps into workflow, depending on this expand step
5. Mark expand step `done`

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

**Modes:**
- `autonomous`: Normal execution with stop-hook auto-continuation
- `interactive`: Pauses auto-continuation for human conversation

**Orchestrator Behavior:**
1. Mark step `running`
2. Wait for agent to execute `meow done`
3. Validate outputs against definitions
4. If valid, mark step `done` with outputs
5. If invalid, reject and wait for retry

**Agent Experience:**
```bash
$ meow prime
## Implement Feature

Implement the feature described in task PROJ-123.

Follow TDD:
1. Write failing tests first
2. Implement until tests pass
3. Refactor if needed

### Required Outputs
- `main_file` (string): Path to the main implementation file

### Optional Outputs
- `test_file` (string)

### When Done
meow done --output main_file=<path>
```

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

Steps communicate through **outputs**. This works like GitHub Actions.

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

### Core Loop

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           ORCHESTRATOR MAIN LOOP                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  while workflow.status != "done":                                           │
│                                                                             │
│      step = get_next_ready_step()                                           │
│                                                                             │
│      if step is None:                                                       │
│          if all_steps_done():                                               │
│              workflow.status = "done"                                       │
│              break                                                          │
│          sleep(poll_interval)  # Waiting for external completion            │
│          continue                                                           │
│                                                                             │
│      match step.executor:                                                   │
│          "shell"  → run_command(step); step.status = "done"                 │
│          "spawn"  → start_agent(step); step.status = "done"                 │
│          "kill"   → stop_agent(step); step.status = "done"                  │
│          "expand" → inline_template(step); step.status = "done"             │
│          "branch" → eval_and_expand(step); step.status = "done"             │
│          "agent"  → step.status = "running"; wait_for_meow_done()           │
│          "gate"   → step.status = "running"; wait_for_approval()            │
│                                                                             │
│      persist_state()                                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

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
```

---

## Agent Interaction Model

### The Agent's View

Agents interact with MEOW through two commands:

1. **`meow prime`** — "What should I do?"
2. **`meow done`** — "I finished, here are outputs"

That's it. Agents don't see workflows, dependencies, or orchestrator machinery.

### `meow prime` Output

```bash
$ meow prime
```

Returns only what the agent needs:

```markdown
## Write Tests

Write failing tests that define the expected behavior for PROJ-123.

### Required Outputs
- `test_file` (string): Path to the test file

### When Done
```
meow done --output test_file=<path>
```
```

If no work is assigned, returns empty (allowing natural conversation to continue).

### `meow done` Command

```bash
# Simple completion
meow done

# With outputs
meow done --output test_file=src/test/auth.test.ts

# With multiple outputs
meow done --output task_id=PROJ-123 --output priority=high

# With JSON outputs
meow done --output-json '{"task_id": "PROJ-123", "tags": ["auth", "api"]}'

# With notes
meow done --notes "Completed but needs review"
```

### Output Validation

If a step defines required outputs, `meow done` validates them:

```bash
$ meow done
Error: Missing required outputs

Required:
  ✗ test_file (string): Not provided

Usage:
  meow done --output test_file=<path>
```

### The Stop-Hook Pattern (Ralph Wiggum Loop)

Claude Code's stop-hook enables autonomous iteration:

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

When Claude finishes work:
1. Claude runs `meow done`
2. Stop-hook fires
3. `meow prime` returns next prompt
4. Claude continues automatically

For `mode: interactive` steps, `meow prime` returns empty, breaking the loop for human conversation.

---

## Persistence and Crash Recovery

### Directory Structure

```
.meow/
├── config.toml              # User configuration
├── agents.json              # Active agent sessions
└── workflows/
    ├── wf-abc123.yaml       # Workflow instance state
    └── wf-def456.yaml
```

### Workflow State File

```yaml
# .meow/workflows/wf-abc123.yaml
id: wf-abc123
template: work-loop.meow.toml
status: running
started_at: 2026-01-08T21:00:00Z

# Resolved variables
vars:
  agent: claude-1

# All steps with their state
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
    expanded_steps: ["impl.load-context", "impl.write-tests", ...]

  impl.load-context:
    executor: agent
    agent: claude-1
    status: done
    finished_at: 2026-01-08T21:05:00Z

  impl.write-tests:
    executor: agent
    agent: claude-1
    status: running
    started_at: 2026-01-08T21:05:30Z
```

### Crash Recovery

On orchestrator startup:

```
1. Load workflow state from .meow/workflows/{id}.yaml
2. Find steps with status: running
3. For each running step:
   a. If executor is orchestrator type (shell, spawn, etc.):
      - Reset to pending (orchestrator crashed mid-execution)
   b. If executor is agent:
      - Check if assigned agent is still alive (tmux session exists)
      - If alive: keep running
      - If dead: reset to pending
   c. If executor is gate:
      - Keep running (human might still approve)
4. Resume orchestrator loop
```

### Agent State

```yaml
# .meow/agents.json
{
  "claude-1": {
    "tmux_session": "meow-claude-1",
    "status": "active",
    "workdir": "/data/projects/myapp",
    "started_at": "2026-01-08T21:00:00Z",
    "current_step": "impl.write-tests"
  }
}
```

---

## CLI Commands

### Workflow Management

```bash
# Run a workflow
meow run template.meow.toml
meow run template.meow.toml#workflow-name
meow run template.meow.toml --var agent=claude-1 --var task_id=PROJ-123

# Check workflow status
meow status
meow status wf-abc123
meow status --json

# List all workflows
meow list
meow list --status running
```

### Agent Commands

```bash
# See current work (for agents)
meow prime
meow prime --agent claude-1
meow prime --format prompt    # For stop-hook injection
meow prime --format json

# Signal completion (for agents)
meow done
meow done --output key=value
meow done --output-json '{"key": "value"}'
meow done --notes "Completion notes"
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
meow agents --active
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

### Parallel Execution

Steps without dependencies can run in parallel across agents:

```yaml
[[steps]]
id = "start-frontend"
executor = "spawn"
agent = "frontend-agent"

[[steps]]
id = "start-backend"
executor = "spawn"
agent = "backend-agent"

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

[[steps]]
id = "integration"
executor = "agent"
agent = "frontend-agent"
prompt = "..."
needs = ["frontend-work", "backend-work"]  # Waits for both
```

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
id = "start-child"
executor = "spawn"
agent = "{{child}}"
needs = ["stop-parent"]

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
id = "resume-parent"
executor = "spawn"
agent = "{{parent}}"
resume_session = "{{checkpoint.outputs.session_id}}"
needs = ["stop-child"]
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
id = "start"
executor = "spawn"
agent = "{{agent}}"

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
template = "main"
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
id = "smoke-test"
executor = "agent"
agent = "{{agent}}"
prompt = """
Run smoke tests against staging environment.
Report any issues found.
"""
needs = ["deploy-staging"]

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
```

### Example 3: Parallel Implementation

```toml
# parallel.meow.toml

[main]
name = "parallel-impl"

[main.variables]
task_id = { required = true }

[[main.steps]]
id = "plan"
executor = "agent"
agent = "planner"
prompt = "Break down {{task_id}} into frontend and backend tasks."

[main.steps.outputs]
frontend_spec = { required = true, type = "string" }
backend_spec = { required = true, type = "string" }

[[main.steps]]
id = "start-frontend"
executor = "spawn"
agent = "frontend"
needs = ["plan"]

[[main.steps]]
id = "start-backend"
executor = "spawn"
agent = "backend"
needs = ["plan"]

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

### Workflow Failure

When a step fails and `on_error` is `fail`:
1. Workflow status becomes `failed`
2. No more steps are executed
3. Human intervention required

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

## Implementation Phases

### Phase 1: Core Engine

- [ ] Step model and status management
- [ ] Template parsing (module format)
- [ ] `shell` executor
- [ ] `agent` executor
- [ ] Basic persistence (workflow YAML files)
- [ ] `meow run`, `meow prime`, `meow done` commands
- [ ] Variable substitution

### Phase 2: Full Executors

- [ ] `spawn` executor with tmux management
- [ ] `kill` executor
- [ ] `expand` executor with template resolution
- [ ] `branch` executor with condition evaluation
- [ ] `gate` executor with `meow approve`/`meow reject`

### Phase 3: Robustness

- [ ] Crash recovery
- [ ] Output validation
- [ ] Error handling
- [ ] Trace logging

### Phase 4: Multi-Agent

- [ ] Parallel step execution
- [ ] Agent state management
- [ ] Session resume support
- [ ] Worktree management helpers

### Phase 5: Developer Experience

- [ ] Template validation command
- [ ] Workflow visualization
- [ ] Debug mode
- [ ] VSCode extension

---

## Appendix: Design Decisions

### Why No Tiers?

The original design had three "tiers" (work, wisp, orchestrator). This was complexity from trying to integrate with external task tracking. With task tracking as the user's concern, tiers disappear. There's just: steps and who executes them.

### Why No Beads Integration?

MEOW is task-tracking agnostic. Users can use any system—beads, Jira, GitHub, markdown files, sticky notes. The workflow template tells agents how to interact with whatever system exists. This is maximum flexibility.

### Why YAML for State?

- Human-readable for debugging
- No database dependency
- Git-trackable
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

---

*MEOW: The coordination language for AI agents.*
