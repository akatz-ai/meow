# MEOW Stack Implementation Guide

> **Purpose**: This document elaborates on MVP-SPEC-v2 with implementation details, architectural decisions, and migration guidance. It serves as the canonical reference for the pivot from bead-centric to workflow-centric architecture.

---

## Table of Contents

1. [Philosophy & Mental Model](#philosophy--mental-model)
2. [The Step Primitive](#the-step-primitive)
3. [The Seven Executors](#the-seven-executors)
4. [Workflow State Model](#workflow-state-model)
5. [Template System](#template-system)
6. [Agent Interaction Protocol](#agent-interaction-protocol)
7. [CLI Design](#cli-design)
8. [Persistence Architecture](#persistence-architecture)
9. [Crash Recovery](#crash-recovery)
10. [Migration from Bead-Centric Model](#migration-from-bead-centric-model)
11. [Testing Strategy](#testing-strategy)

---

## Philosophy & Mental Model

### MEOW is a Programming Language

MEOW templates are **programs**. They are not configuration files, not task lists, not ticket queues. They are executable specifications that define how work flows through a system of AI agents.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         CORE MENTAL MODEL                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Template    = Program source code                                          │
│  Workflow    = Running program instance (with state)                        │
│  Step        = Single instruction in the program                            │
│  Executor    = Who/what runs each instruction                               │
│  Outputs     = Data flowing between instructions                            │
│  Orchestrator = Runtime that executes programs                              │
│                                                                             │
│  The orchestrator is DUMB. Templates are SMART.                             │
│  Complex behaviors emerge from simple primitives composed.                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Why Task-Tracking Agnostic?

The previous architecture tightly coupled MEOW with beads (our task tracker). This created:
- **Conceptual confusion**: Are we a task tracker or a workflow engine?
- **Integration burden**: Every user needs beads, even if they use Jira/GitHub/Linear
- **Visibility complexity**: Three-tier model (work/wisp/orchestrator) was hard to reason about

The v2 approach: **MEOW doesn't care how you track work**. If you use beads, your template prompts agents to run `bd ready`. If you use Jira, your template prompts agents to query the Jira API. MEOW just executes workflows.

### The Propulsion Principle

> **When an agent receives work via `meow prime`, they execute immediately.**

There is no polling, no status checking, no "is there work for me?" loops. The workflow state IS the instruction. When `meow prime` returns a prompt, the agent acts. When it returns empty, the agent waits (interactive mode) or the workflow is complete.

### Minimal Agent Exposure

Agents should see ONLY:
- The current prompt/instructions
- Expected outputs (if any)
- The command to signal completion (`meow done`)

Agents should NEVER see:
- Internal workflow IDs
- Step dependency graphs
- Orchestrator machinery
- Persistence details
- Other agents' steps

This is not about hiding information—it's about cognitive load. An agent's job is to do the current step, not to understand the entire workflow topology.

---

## The Step Primitive

Everything in MEOW is a **Step**. One primitive, not eight types.

### Step Structure

```go
type Step struct {
    // Identity
    ID       string       `yaml:"id"`        // Unique within workflow
    Executor ExecutorType `yaml:"executor"`  // Who runs this step

    // Lifecycle
    Status    StepStatus `yaml:"status"`     // pending | running | done | failed
    StartedAt *time.Time `yaml:"started_at,omitempty"`
    DoneAt    *time.Time `yaml:"done_at,omitempty"`

    // Dependencies
    Needs []string `yaml:"needs,omitempty"` // Step IDs that must complete first

    // Data
    Outputs map[string]any `yaml:"outputs,omitempty"` // Captured output data
    Error   *StepError     `yaml:"error,omitempty"`   // Error info if failed

    // Executor-specific configuration (one of these is populated)
    Shell  *ShellConfig  `yaml:"shell,omitempty"`
    Spawn  *SpawnConfig  `yaml:"spawn,omitempty"`
    Kill   *KillConfig   `yaml:"kill,omitempty"`
    Expand *ExpandConfig `yaml:"expand,omitempty"`
    Branch *BranchConfig `yaml:"branch,omitempty"`
    Agent  *AgentConfig  `yaml:"agent,omitempty"`
    Gate   *GateConfig   `yaml:"gate,omitempty"`
}
```

### Status Lifecycle

```
pending ──► running ──► done
              │
              └──► failed
```

- **pending**: Waiting for dependencies or orchestrator attention
- **running**: Currently being executed
- **done**: Successfully completed, outputs captured
- **failed**: Execution failed (with error info)

### Dependency Resolution

A step is **ready** when:
1. Status is `pending`
2. All steps in `needs` have status `done`

The orchestrator processes ready steps in priority order:
1. Orchestrator executors first (shell, spawn, kill, expand, branch)
2. Then external executors (agent, gate)
3. Within category: by creation order

This ensures machinery completes before agents receive more work.

---

## The Seven Executors

### Orchestrator Executors (Internal)

These run synchronously within the orchestrator—no external waiting.

#### 1. `shell` — Run Shell Command

Execute a shell command and capture outputs.

```yaml
# In template
[[main.steps]]
id = "setup-branch"
executor = "shell"
command = "git checkout -b feature/{{task_id}} && echo feature/{{task_id}}"
workdir = "/path/to/repo"
env = { GIT_AUTHOR_NAME = "MEOW" }
on_error = "fail"  # or "continue"

[main.steps.outputs]
branch_name = { source = "stdout" }
exit_code = { source = "exit_code" }
```

**Config fields:**
- `command` (required): Shell script to execute
- `workdir` (optional): Working directory
- `env` (optional): Environment variables
- `on_error` (optional): `continue` | `fail` (default: `fail`)
- `outputs` (optional): Output capture definitions

**Output sources:**
- `stdout`: Captured standard output (trimmed)
- `stderr`: Captured standard error (trimmed)
- `exit_code`: Process exit code as integer
- `file:/path`: Contents of a file

#### 2. `spawn` — Start Agent Process

Start an agent in a tmux session with Claude.

```yaml
[[main.steps]]
id = "start-worker"
executor = "spawn"
agent = "worker-1"
workdir = "{{setup-branch.outputs.worktree_path}}"
env = { TASK_ID = "{{task_id}}" }
prompt = "meow prime"  # Default, can customize
resume_session = "{{checkpoint.outputs.session_id}}"  # Optional
```

**Config fields:**
- `agent` (required): Agent identifier (becomes tmux session `meow-{agent}`)
- `workdir` (optional): Working directory for the agent
- `env` (optional): Environment variables (always includes `MEOW_AGENT`)
- `prompt` (optional): Initial prompt to inject (default: `meow prime`)
- `resume_session` (optional): Claude session ID to resume

**Behavior:**
1. Create tmux session `meow-{agent}`
2. Set environment variables
3. Start Claude (with `--resume` if specified)
4. Wait for Claude ready prompt
5. Inject initial prompt
6. Mark step done

#### 3. `kill` — Stop Agent Process

Terminate an agent's tmux session.

```yaml
[[main.steps]]
id = "stop-worker"
executor = "kill"
agent = "worker-1"
graceful = true
timeout = 10
```

**Config fields:**
- `agent` (required): Agent identifier to stop
- `graceful` (optional): Send SIGTERM first (default: `true`)
- `timeout` (optional): Seconds to wait for graceful shutdown (default: `10`)

#### 4. `expand` — Inline Another Workflow

Expand a template's steps into the current workflow.

```yaml
[[main.steps]]
id = "do-implementation"
executor = "expand"
template = ".tdd-impl"
variables = { task_id = "{{select.outputs.task_id}}", agent = "{{agent}}" }
```

**Config fields:**
- `template` (required): Template reference (see Template References)
- `variables` (optional): Variables to pass to the template

**Behavior:**
1. Load and parse referenced template
2. Substitute variables
3. Generate unique step IDs (prefixed with expand step ID)
4. Insert expanded steps into workflow
5. New steps depend on this expand step
6. Mark expand step done

#### 5. `branch` — Conditional Expansion

Evaluate a condition and expand the appropriate branch.

```yaml
[[main.steps]]
id = "check-tests"
executor = "branch"
condition = "npm test"
timeout = "5m"

[main.steps.on_true]
template = ".continue"
variables = { status = "passing" }

[main.steps.on_false]
template = ".fix-tests"

[main.steps.on_timeout]
inline = [
    { id = "notify", executor = "agent", prompt = "Tests timed out, investigate" }
]
```

**Config fields:**
- `condition` (required): Shell command to evaluate (exit 0 = true)
- `on_true` (optional): Expansion if condition passes
- `on_false` (optional): Expansion if condition fails
- `on_timeout` (optional): Expansion if condition times out
- `timeout` (optional): Condition evaluation timeout

**Expansion target:**
```yaml
# Template reference
on_true = { template = "template-ref", variables = { key = "value" } }

# OR inline steps
on_true = { inline = [{ id = "step-1", executor = "agent", prompt = "..." }] }
```

### External Executors (Waiting)

These mark the step as running and wait for external completion.

#### 6. `agent` — Prompt Agent

Assign work to an agent and wait for completion.

```yaml
[[main.steps]]
id = "implement-feature"
executor = "agent"
agent = "worker-1"
mode = "autonomous"  # or "interactive"
prompt = """
Implement the feature described in task {{task_id}}.

Follow TDD:
1. Write failing tests first
2. Implement until tests pass
3. Refactor if needed

When done, provide the path to the main implementation file.
"""

[main.steps.outputs]
main_file = { required = true, type = "string", description = "Path to main file" }
test_file = { required = false, type = "string" }
```

**Config fields:**
- `agent` (required): Agent identifier
- `prompt` (required): Instructions for the agent
- `mode` (optional): `autonomous` (default) or `interactive`
- `outputs` (optional): Expected output definitions

**Modes:**
- `autonomous`: Normal execution with stop-hook auto-continuation
- `interactive`: Pauses auto-continuation for human conversation

**Output definition:**
```yaml
[outputs]
field_name = { required = true, type = "string", description = "..." }
```

Types: `string`, `number`, `boolean`, `json`, `file_path`

#### 7. `gate` — Human Approval

Block until human approves or rejects.

```yaml
[[main.steps]]
id = "review-gate"
executor = "gate"
prompt = """
Review the implementation for {{task_id}}.

Changes:
- {{changes_summary}}

Approve if ready to merge, reject with feedback if changes needed.
"""
timeout = "24h"
```

**Config fields:**
- `prompt` (required): Information for the human reviewer
- `timeout` (optional): How long to wait before timeout action

**Completion:**
- `meow approve <workflow-id> <step-id>` → marks done
- `meow reject <workflow-id> <step-id> --reason "..."` → marks failed

---

## Workflow State Model

### Workflow Structure

```go
type Workflow struct {
    // Identity
    ID       string `yaml:"id"`       // Unique identifier (e.g., "wf-abc123")
    Template string `yaml:"template"` // Source template path

    // Lifecycle
    Status    WorkflowStatus `yaml:"status"`     // pending | running | done | failed
    StartedAt time.Time      `yaml:"started_at"`
    DoneAt    *time.Time     `yaml:"done_at,omitempty"`

    // Configuration
    Variables map[string]string `yaml:"variables,omitempty"` // Resolved variables

    // State
    Steps map[string]*Step `yaml:"steps"` // All steps with their state
}
```

### Workflow Status

```
pending ──► running ──► done
              │
              └──► failed
```

- **pending**: Created but not started
- **running**: Orchestrator is actively processing
- **done**: All steps completed successfully
- **failed**: A step failed with `on_error: fail`

### State File Format

```yaml
# .meow/workflows/wf-abc123.yaml
id: wf-abc123
template: work-loop.meow.toml
status: running
started_at: 2026-01-08T21:00:00Z

variables:
  agent: claude-1
  task_source: beads

steps:
  select:
    executor: agent
    status: done
    started_at: 2026-01-08T21:00:00Z
    done_at: 2026-01-08T21:02:00Z
    agent:
      agent: claude-1
      mode: autonomous
      prompt: "Select the next task..."
    outputs:
      task_id: "PROJ-123"
      has_more: true

  implement:
    executor: expand
    status: done
    done_at: 2026-01-08T21:02:01Z
    expand:
      template: ".tdd"
      variables:
        task_id: "PROJ-123"

  implement.load-context:
    executor: agent
    status: running
    started_at: 2026-01-08T21:05:00Z
    needs: ["implement"]
    agent:
      agent: claude-1
      prompt: "Load context for PROJ-123..."
```

---

## Template System

### Module Format

Templates use TOML with a module format supporting multiple workflows per file.

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
task_source = { default = "beads", description = "How to find tasks" }

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
prompt = "Read and understand task {{task_id}}..."

[[tdd.steps]]
id = "write-tests"
executor = "agent"
agent = "{{agent}}"
prompt = "Write failing tests for {{task_id}}."
needs = ["load-context"]

# ... more steps
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
```

### Variable Substitution

Variables can appear in any string field:

```yaml
prompt = "Work on {{task_id}} in branch {{setup.outputs.branch}}"
command = "git checkout {{branch_name}}"
template = "{{workflow_type}}-impl"
```

**Resolution order:**
1. Workflow variables (from `meow run --var`)
2. Step outputs (from completed steps)
3. Built-in variables (`{{workflow_id}}`, `{{timestamp}}`, `{{date}}`)

---

## Agent Interaction Protocol

### The Agent's View

Agents interact with MEOW through exactly two commands:

1. **`meow prime`** — "What should I do?"
2. **`meow done`** — "I finished, here are outputs"

That's it. Agents don't see workflows, dependencies, or orchestrator machinery.

### `meow prime` Output

```bash
$ meow prime
```

Returns ONLY what the agent needs:

```markdown
## Write Tests

Write failing tests that define the expected behavior for PROJ-123.

### Required Outputs
- `test_file` (string): Path to the test file

### When Done
meow done --output test_file=<path>
```

If no work is assigned, returns empty string (allowing natural conversation to continue in interactive mode, or signaling workflow completion).

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

# With notes (stored but not validated)
meow done --notes "Completed but needs review"
```

### Output Validation

If a step defines required outputs, `meow done` validates:

```bash
$ meow done
Error: Missing required outputs

Required:
  ✗ test_file (string): Not provided

Usage:
  meow done --output test_file=<path>
```

### The Stop-Hook Pattern

Claude Code's stop-hook enables autonomous iteration:

```bash
#!/bin/bash
# .claude/hooks/stop.sh

output=$(meow prime --format prompt 2>/dev/null)

if [ -z "$output" ]; then
    # Empty = interactive mode OR workflow complete
    exit 0  # Don't inject anything
fi

echo "$output"  # Inject next prompt
```

**Flow:**
1. Agent completes work, runs `meow done`
2. Claude Code's stop-hook fires
3. `meow prime` returns next prompt (or empty)
4. If prompt: Claude continues automatically
5. If empty: Claude waits for human input

For `mode: interactive` steps, `meow prime` returns empty while the step is running, breaking the auto-loop.

### Agent Environment

When an agent is spawned, these environment variables are set:

- `MEOW_AGENT` — The agent identifier
- `MEOW_WORKFLOW` — The workflow ID (for debugging, not for agents to use)
- Any custom env vars from the spawn step

The `meow prime` and `meow done` commands use `MEOW_AGENT` to identify which agent is calling.

---

## CLI Design

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

# List workflows
meow list
meow list --status running
```

### Agent Commands

```bash
# See current work (for agents)
meow prime                    # Uses MEOW_AGENT env var
meow prime --agent claude-1   # Explicit agent
meow prime --format prompt    # For stop-hook injection
meow prime --format json      # Machine-readable

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

## Persistence Architecture

### Directory Structure

```
.meow/
├── config.toml              # User configuration
├── agents.yaml              # Active agent sessions
├── orchestrator.lock        # Prevents concurrent instances
└── workflows/
    ├── wf-abc123.yaml       # Workflow instance state
    └── wf-def456.yaml
```

### Config File

```toml
# .meow/config.toml

[orchestrator]
poll_interval = "100ms"      # Main loop frequency
state_dir = ".meow"          # Where to store state

[agent]
default_prompt = "meow prime"
setup_hooks = true           # Auto-configure Claude hooks

[templates]
search_paths = [".meow/templates", "./templates"]
```

### Agent State

```yaml
# .meow/agents.yaml
agents:
  claude-1:
    tmux_session: meow-claude-1
    status: active
    workdir: /data/projects/myapp
    started_at: 2026-01-08T21:00:00Z
    current_workflow: wf-abc123
    current_step: implement.write-tests

  claude-2:
    tmux_session: meow-claude-2
    status: idle
    workdir: /data/projects/myapp
    started_at: 2026-01-08T20:00:00Z
```

---

## Crash Recovery

### Recovery Protocol

On orchestrator startup:

```
1. Acquire exclusive lock (.meow/orchestrator.lock)
2. Load all workflow state from .meow/workflows/*.yaml
3. For each workflow with status: running
   a. Find steps with status: running
   b. For each running step:
      - If orchestrator executor (shell, spawn, kill, expand, branch):
        → Reset to pending (orchestrator crashed mid-execution)
      - If agent executor:
        → Check if assigned agent is alive (tmux session exists)
        → If alive: keep running
        → If dead: reset to pending
      - If gate executor:
        → Keep running (human might still approve)
4. Resume orchestrator loop
```

### Atomic State Updates

All workflow state updates must be atomic:

```go
func (s *WorkflowStore) Update(ctx context.Context, wf *Workflow) error {
    // Write to temp file first
    tmpPath := wf.path + ".tmp"
    if err := s.writeYAML(tmpPath, wf); err != nil {
        return err
    }
    // Atomic rename
    return os.Rename(tmpPath, wf.path)
}
```

### Lock Management

```go
func (p *StatePersister) AcquireLock() error {
    lockPath := filepath.Join(p.stateDir, "orchestrator.lock")

    // Try to create lock file with exclusive flag
    f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
    if err != nil {
        if os.IsExist(err) {
            // Check if lock is stale (process died)
            return p.handleStaleLock(lockPath)
        }
        return err
    }

    // Write our PID
    fmt.Fprintf(f, "%d\n", os.Getpid())
    f.Close()

    p.lockPath = lockPath
    return nil
}
```

---

## Migration from Bead-Centric Model

### What to Remove

1. **Type system** (`internal/types/bead.go`)
   - `BeadType` enum (8 types)
   - `BeadTier` enum (work/wisp/orchestrator)
   - `HookBead`, `SourceWorkflow` fields
   - All type-specific specs (ConditionSpec, etc.) — replaced by executor configs

2. **Bead store** (`internal/orchestrator/beadstore.go`)
   - All bead CRUD operations
   - Tier-aware filtering
   - `.beads/issues.jsonl` integration

3. **Template system cruft** (`internal/template/`)
   - `ephemeral` workflow property
   - `hooks_to` workflow property
   - Tier detection in baker
   - Legacy `[meta]` format support (clean break)

4. **Orchestrator logic** (`internal/orchestrator/orchestrator.go`)
   - `burnWisps()`, `squashWisps()`, `cleanupWorkflow()`
   - Tier-based dispatch priority
   - Bead type switch in dispatch

5. **CLI commands** (`cmd/meow/cmd/`)
   - Rename `close.go` → `done.go`
   - Remove bead-specific output in `prime.go`
   - Remove tier info from `status.go`

### What to Add

1. **New type system** (`internal/types/`)
   - `step.go` — Step struct with executor configs
   - `workflow.go` — Workflow struct with state
   - `executor.go` — ExecutorType enum (7 executors)

2. **Workflow store** (`internal/orchestrator/`)
   - `workflowstore.go` — YAML persistence
   - Interface for workflow CRUD

3. **Updated orchestrator**
   - Executor-based dispatch
   - Workflow-aware state management

4. **Updated template system**
   - Parse `executor` field instead of `type`
   - Remove all tier logic

### Migration Sequence

1. Create new types alongside old (no breaking changes yet)
2. Implement WorkflowStore
3. Update template parser for new format
4. Refactor orchestrator to use new types
5. Update CLI commands
6. Delete old code
7. Update documentation

---

## Testing Strategy

### Unit Tests

Each component should have focused unit tests:

- **Types**: Validation, serialization, status transitions
- **Template parser**: TOML parsing, variable substitution
- **Workflow store**: YAML round-trip, atomic writes
- **Orchestrator**: Dispatch logic, dependency resolution
- **Executors**: Each executor's specific behavior

### Integration Tests

End-to-end workflow execution:

1. **Simple workflow**: spawn → agent → done
2. **Branching**: condition evaluation, branch expansion
3. **Looping**: recursive template expansion
4. **Multi-agent**: parallel execution
5. **Crash recovery**: kill orchestrator, restart, verify continuation

### Test Templates

Create test templates in `testdata/templates/`:

```
testdata/
├── templates/
│   ├── simple.meow.toml      # Basic single-step
│   ├── sequential.meow.toml  # A → B → C
│   ├── parallel.meow.toml    # A → (B, C) → D
│   ├── branching.meow.toml   # Conditional paths
│   ├── looping.meow.toml     # Recursive expansion
│   └── multi-agent.meow.toml # Multiple agents
└── workflows/
    └── fixtures/             # Pre-created workflow states
```

---

*This guide should be read alongside MVP-SPEC-v2.md. The spec defines WHAT; this guide explains HOW.*
