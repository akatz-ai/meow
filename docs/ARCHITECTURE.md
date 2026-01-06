# MEOW Stack Architecture

This document provides the complete technical specification for the Molecular Expression Of Work (MEOW) Stack — a recursive, composable workflow system for durable AI agent orchestration.

## Table of Contents

1. [Design Philosophy](#design-philosophy)
2. [Core Primitives](#core-primitives)
3. [The Molecule Stack](#the-molecule-stack)
4. [Layer Architecture](#layer-architecture)
5. [Template System](#template-system)
6. [Execution Engine](#execution-engine)
7. [State Management](#state-management)
8. [Human Gates](#human-gates)
9. [Task Selection](#task-selection)
10. [Error Handling](#error-handling)
11. [Parallel Fork-Join](#parallel-fork-join)
12. [Multi-Agent Orchestration](#multi-agent-orchestration)
13. [Git Worktrees](#git-worktrees)
14. [Handoff Context](#handoff-context)
15. [Shell-Based Conditions](#shell-based-conditions)

---

## Design Philosophy

### Core Principles

1. **Everything is a Molecule**
   - Loops are molecules whose final step is "restart"
   - Gates are molecules with blocking steps
   - The outer orchestration layer is itself a molecule
   - Uniform semantics enable arbitrary composition

2. **Molecules Survive Crashes**
   - All state is persisted to beads (git-backed)
   - Any agent can resume where another left off
   - The molecule stack is the source of truth

3. **The Prompt Never Changes, But the World Does**
   - Same prompt each iteration (Ralph Wiggum pattern)
   - Agent sees accumulated work in files and git history
   - Progress is measured by state change, not iteration count

4. **Human Gates Are Workflow Steps**
   - Not interruptions or exceptions
   - First-class beads in molecules
   - Natural checkpoints with full audit trail

5. **Dependency-Driven Execution**
   - No explicit phases or ordering
   - Dependencies define the execution DAG
   - `bd ready` shows the "ready front" — unblocked work

### Design Goals

| Goal | Mechanism |
|------|-----------|
| Durability | Git-backed beads, molecule stack persisted |
| Resumability | `bd mol stack` shows position, `bd ready` shows next step |
| Composability | Templates reference templates, arbitrary nesting |
| Observability | Full audit trail, every step logged |
| Human oversight | Gates as first-class workflow steps |
| Intelligent selection | PageRank/betweenness scoring via bv |

---

## Core Primitives

### Bead

The atomic unit of work. A bead is an issue/task in the beads system:

```yaml
id: "bd-a1b2c3"
title: "Implement user registration endpoint"
description: "Create POST /api/register with validation"
status: open | in_progress | blocked | closed
type: task | feature | bug | epic | gate | molecule
priority: 0-4  # 0 = critical, 4 = backlog
labels: ["template:implement", "backend"]
dependencies:
  - type: blocks
    depends_on: "bd-x1y2z3"
parent: "bd-epic-001"  # For rollup

# Parallel execution (new)
parallel: true | false          # Can run in parallel with siblings
parallel_group: "review-test"   # Group ID for parallel siblings
spawn_mode: clone | fresh       # clone = --resume, fresh = new context

# Conditions (new)
condition: "shell command"      # Shell command that must exit 0 to proceed

# Handoff context (new)
handoff:
  status: success | failure | partial
  summary: "What was accomplished"
  artifacts: ["path/to/file"]
  next_steps: ["What the next agent should do"]
  context: { key: value }       # Arbitrary data for parent
```

### Molecule

A structured workflow containing steps (which are beads). A molecule is a bead with `type: molecule` and child beads representing steps:

```yaml
id: "mol-impl-001"
title: "Implement: User Registration"
type: molecule
template_source: "implement"  # Which template created this
parent_molecule: "meta-mol-001"  # Stack parent
parent_step: "task-1"  # Which step in parent this represents
status: in_progress
current_step: "write-tests"  # Active step
iteration: 1  # For loop molecules
children:
  - "mol-impl-001.load-context"
  - "mol-impl-001.write-tests"
  - "mol-impl-001.verify-fail"
  - "mol-impl-001.implement"
  - "mol-impl-001.commit"

# Session tracking (new)
session:
  id: "session-uuid"            # Claude session ID
  tmux_session: "meow-mol-001"  # tmux session name
  worktree: "/path/to/worktree" # Git worktree path
  branch: "meow/mol-impl-001"   # Auto-generated branch
  checkpoint_id: "checkpoint-X" # For --resume
  spawn_mode: clone | fresh     # How this session was started
  parent_session: "session-Y"   # Session that spawned this one

# Completion signal (new)
terminal_step: "commit"         # Last step - completion triggers parent resume
```

### Template

A reusable workflow pattern defined in TOML:

```toml
[meta]
name = "implement"
description = "TDD implementation workflow"
fits_in_context = true  # Should complete in one session

[[steps]]
id = "load-context"
description = "Load relevant files and understand the task"

[[steps]]
id = "write-tests"
needs = ["load-context"]
```

### Step

A step within a molecule. Steps are beads with parent = molecule:

```yaml
id: "mol-impl-001.write-tests"
title: "Write failing tests"
type: task
parent: "mol-impl-001"
status: open
template: null  # Atomic step, or "sub-template" to expand further
```

---

## The Molecule Stack

The molecule stack is a runtime structure tracking nested execution:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                            MOLECULE STACK                                   │
│                                                                             │
│  Stored in: .beads/molecules/stack.json (or inferred from bead relations)  │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 0: outer-loop-001                                            │   │
│  │  Template: outer-loop                                               │   │
│  │  Status: in_progress                                                │   │
│  │  Current Step: run-inner (step 3 of 4)                              │   │
│  │  Child Molecule: meta-mol-001                                       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 1: meta-mol-001                                              │   │
│  │  Template: (dynamically created)                                    │   │
│  │  Status: in_progress                                                │   │
│  │  Current Step: task-1 (step 1 of 5)                                 │   │
│  │  Child Molecule: impl-task-1-001                                    │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                    │                                        │
│                                    ▼                                        │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │  LEVEL 2: impl-task-1-001                                           │   │
│  │  Template: implement                                                │   │
│  │  Status: in_progress                                                │   │
│  │  Current Step: implement (step 4 of 5) ◀── EXECUTING NOW            │   │
│  │  Child Molecule: (none - atomic step)                               │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Stack Operations

| Operation | Trigger | Effect |
|-----------|---------|--------|
| **PUSH** | Step has template | Create child molecule, descend |
| **EXECUTE** | Step is atomic | Run step directly |
| **POP** | Molecule complete | Return to parent, close parent's step |
| **RESTART** | Step type = restart | Re-instantiate current molecule |
| **PAUSE** | Step type = gate | Stop loop, await external signal |

### Stack State Persistence

The stack is persisted via bead relationships:

```yaml
# Each molecule tracks its position in the stack
mol-impl-001:
  parent_molecule: meta-mol-001
  parent_step: task-1

meta-mol-001:
  parent_molecule: outer-loop-001
  parent_step: run-inner

outer-loop-001:
  parent_molecule: null  # Root of stack
```

To reconstruct stack: traverse `parent_molecule` chain from any active molecule.

---

## Layer Architecture

The system operates in distinct layers, each with specific responsibilities:

### Layer 0: Inception (Human + Claude)

**Input**: Feature idea, PRD, or high-level requirement
**Output**: Structured epics and tasks as beads

```
Human: "I want user authentication with OAuth"
    ↓
Claude (planning): Creates design document
    ↓
Claude (breakdown): Creates beads
    ↓
.beads/issues.jsonl:
  - Epic: User Registration (bd-epic-001)
      - Task: Create registration endpoint (bd-task-001)
      - Task: Add email validation (bd-task-002)
  - Epic: Login/Logout (bd-epic-002)
      - Task: Session management (bd-task-003)
      - Task: JWT implementation (bd-task-004)
  - Epic: OAuth Providers (bd-epic-003)
      - Task: Google OAuth (bd-task-005)
      - Task: GitHub OAuth (bd-task-006)
```

### Layer 1: Outer Loop (Orchestration Molecule)

**Template**: `outer-loop`
**Purpose**: Select work batches, bake meta-molecules, manage iterations

```toml
[meta]
name = "outer-loop"
description = "Master orchestration loop"
type = "loop"

[[steps]]
id = "analyze-pick"
description = "Analyze project and select next high-impact work"
template = "analyze-pick"

[[steps]]
id = "bake-meta"
description = "Create meta-molecule for selected work"
template = "bake-meta"
needs = ["analyze-pick"]

[[steps]]
id = "run-inner"
description = "Execute the meta-molecule"
template = "{{output.bake_meta.molecule_id}}"
needs = ["bake-meta"]

[[steps]]
id = "restart"
description = "Loop to next iteration"
type = "restart"
needs = ["run-inner"]
condition = "not all_epics_closed()"
```

### Layer 2: Meta-Molecule (Work Batch)

**Template**: Dynamically created by `bake-meta` step
**Purpose**: Group related tasks with human gates

Example meta-molecule for "User Registration" epic:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  META-MOLECULE: meta-mol-001                                                │
│  Created for: Epic "User Registration"                                      │
│                                                                             │
│  ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌─────────┐   ┌─────────┐       │
│  │ task-1  │──▶│ task-2  │──▶│ close   │──▶│  test   │──▶│  gate   │       │
│  │         │   │         │   │  epic   │   │  suite  │   │         │       │
│  │template:│   │template:│   │template:│   │template:│   │template:│       │
│  │implement│   │implement│   │close-   │   │test-    │   │human-   │       │
│  │         │   │         │   │epic     │   │suite    │   │gate     │       │
│  └─────────┘   └─────────┘   └─────────┘   └─────────┘   └─────────┘       │
│                                                                             │
│  Gate frequency: Every 1 epic (configurable in bake-meta)                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Layer 3: Step Molecules (Workflow Templates)

**Templates**: `implement`, `test-suite`, `human-gate`, `close-epic`
**Purpose**: Structured workflows for specific step types

Each step in the meta-molecule expands to its template:

```
task-1 → implement template:
  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐
  │  load    │▶│  write   │▶│  verify  │▶│implement │▶│  commit  │
  │ context  │ │  tests   │ │   fail   │ │   code   │ │          │
  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘

test-suite → test-suite template:
  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐
  │  setup   │▶│   unit   │▶│  integ   │▶│   e2e    │▶│  report  │
  │   env    │ │  tests   │ │  tests   │ │  tests   │ │ results  │
  └──────────┘ └──────────┘ └──────────┘ └──────────┘ └──────────┘

human-gate → human-gate template:
  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐
  │ prepare  │▶│  notify  │▶│  await   │▶│  record  │
  │ summary  │ │  human   │ │ approval │ │ decision │
  └──────────┘ └──────────┘ └──────────┘ └──────────┘
                              ↑ BLOCKS
```

### Layer 4: Atomic Execution

**Templates**: None (atomic steps)
**Purpose**: Direct execution by Claude

Atomic steps are executed directly:
- Load files, read context
- Write code, run tests
- Create commits, update status

---

## Template System

### Template Format

Templates are TOML files in `.beads/templates/`:

```toml
[meta]
name = "implement"
description = "TDD implementation workflow"
version = "1.0.0"
author = "meow-stack"

# Execution hints
fits_in_context = true      # Should complete in one session
estimated_minutes = 30      # For scheduling
requires_human = false      # Contains blocking gates

# Error handling
on_error = "inject-gate"    # or: retry, skip, abort
max_retries = 2
error_gate_template = "error-triage"

# Variables (filled when baking)
[variables]
task_id = { required = true, description = "The task bead ID" }
epic_id = { required = false, description = "Parent epic if applicable" }

[[steps]]
id = "load-context"
description = "Load relevant files and understand the task"
instructions = """
Read the task description from {{task_id}}.
Identify relevant source files.
Understand the current state of the codebase.
"""

[[steps]]
id = "write-tests"
description = "Write failing tests that define success criteria"
needs = ["load-context"]
instructions = """
Based on the task requirements, write tests that:
1. Define expected behavior
2. Cover edge cases
3. Will fail until implementation is complete
"""

[[steps]]
id = "verify-fail"
description = "Run tests and verify they fail as expected"
needs = ["write-tests"]
instructions = """
Run the test suite. Verify that:
1. New tests fail (not existing tests)
2. Failures are for the right reasons
3. Test output is clear
"""
validation = "test_exit_code != 0"

[[steps]]
id = "implement"
description = "Write code to make tests pass"
needs = ["verify-fail"]
instructions = """
Implement the minimum code needed to make tests pass.
Follow existing patterns in the codebase.
Do not over-engineer.
"""

[[steps]]
id = "verify-pass"
description = "Run tests and verify they pass"
needs = ["implement"]
validation = "test_exit_code == 0"

[[steps]]
id = "review"
description = "Self-review the implementation"
needs = ["verify-pass"]
instructions = """
Review your changes for:
1. Code quality and style
2. Edge cases and error handling
3. Performance implications
4. Security considerations
"""

[[steps]]
id = "commit"
description = "Commit changes with descriptive message"
needs = ["review"]
instructions = """
Create a commit with:
1. Clear, concise message
2. Reference to task ID: {{task_id}}
3. Summary of changes
"""
```

### Template Expansion (Baking)

When a step has a `template` field, it expands:

```
Before expansion:
  meta-mol-001:
    step: task-1 (template: implement, task_id: bd-task-001)

After expansion:
  meta-mol-001:
    step: task-1 (child_molecule: impl-task-1-001)

  impl-task-1-001:  ← NEW MOLECULE
    parent_molecule: meta-mol-001
    parent_step: task-1
    template_source: implement
    variables: { task_id: bd-task-001 }
    steps:
      - impl-task-1-001.load-context
      - impl-task-1-001.write-tests
      - impl-task-1-001.verify-fail
      - impl-task-1-001.implement
      - impl-task-1-001.verify-pass
      - impl-task-1-001.review
      - impl-task-1-001.commit
```

### Dynamic Template References

Steps can reference templates dynamically:

```toml
[[steps]]
id = "run-inner"
template = "{{output.bake_meta.molecule_id}}"
```

The `{{output.step_id.field}}` syntax accesses outputs from previous steps.

### Template Inheritance

Templates can extend other templates:

```toml
[meta]
name = "implement-with-docs"
extends = "implement"

# Add a step after commit
[[steps]]
id = "update-docs"
description = "Update documentation"
needs = ["commit"]
```

---

## Execution Engine

The executor is the core runtime that drives the molecule stack.

### Executor Algorithm

```python
def execute_iteration():
    # 1. Get current position
    stack = load_molecule_stack()
    if stack.empty():
        return DONE

    current_mol = stack.top()
    current_step = get_ready_step(current_mol)

    if current_step is None:
        # Molecule complete
        return handle_molecule_complete(stack)

    # 2. Dispatch based on step type
    if current_step.has_template():
        # DESCENT - push child molecule
        child = bake_molecule(current_step.template, current_step.variables)
        stack.push(child)
        link_parent_child(current_mol, current_step, child)
        return CONTINUE

    elif current_step.type == "blocking-gate":
        # PAUSE - wait for human
        prepare_gate_summary(current_step)
        notify_human(current_step)
        return PAUSE

    elif current_step.type == "restart":
        # LOOP - restart current molecule
        if evaluate_condition(current_step.condition):
            restart_molecule(current_mol)
            return CONTINUE
        else:
            close_step(current_step)
            return handle_molecule_complete(stack)

    else:
        # ATOMIC - execute directly
        execute_atomic_step(current_step)
        close_step(current_step)
        return CONTINUE

def handle_molecule_complete(stack):
    completed_mol = stack.pop()
    mark_complete(completed_mol)

    if stack.empty():
        return DONE

    # Close parent's step that spawned this molecule
    parent_mol = stack.top()
    parent_step = completed_mol.parent_step
    close_step(parent_step)

    return CONTINUE
```

### Integration with Ralph Wiggum

The executor integrates with Ralph Wiggum's stop-hook:

```bash
#!/bin/bash
# .claude/hooks/meow-stop-hook.sh

# Load MEOW state
MEOW_STATE=".beads/molecules/meow-state.json"

# Run one iteration of executor
result=$(meow-executor iterate)

case "$result" in
  "CONTINUE")
    # Feed prompt back to continue loop
    echo '{"block": true, "reason": "'"$(meow-executor get-prompt)"'"}'
    ;;
  "PAUSE")
    # Gate reached, stop loop
    echo '{"block": false}'
    ;;
  "DONE")
    # All work complete
    echo '{"block": false}'
    ;;
esac
```

### Execution State

The executor maintains state in `.beads/molecules/`:

```json
// .beads/molecules/meow-state.json
{
  "active": true,
  "iteration": 47,
  "max_iterations": 500,
  "started_at": "2026-01-06T10:00:00Z",
  "last_step_at": "2026-01-06T14:32:00Z",
  "stack_root": "outer-loop-001",
  "current_molecule": "impl-task-1-001",
  "current_step": "implement",
  "paused_at_gate": null
}
```

---

## State Management

### Persistence Hierarchy

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         STATE PERSISTENCE                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  .beads/                          ← Git-tracked, source of truth            │
│  ├── issues.jsonl                 ← All beads (tasks, epics, molecules)     │
│  ├── templates/                   ← Workflow templates                      │
│  │   ├── outer-loop.toml                                                    │
│  │   ├── implement.toml                                                     │
│  │   └── ...                                                                │
│  └── molecules/                   ← Runtime molecule state                  │
│      ├── meow-state.json          ← Executor state                          │
│      └── stack-snapshot.json      ← Current stack (derived from beads)      │
│                                                                             │
│  .beads/beads.db                  ← SQLite cache (gitignored)               │
│  .claude/ralph-loop.local.md      ← Ralph Wiggum loop state                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Crash Recovery

On session start:

```bash
# 1. Load molecule stack from beads
bd mol stack
# → outer-loop-001 → meta-mol-001 → impl-task-1-001

# 2. Find current step
bd ready --mol impl-task-1-001
# → implement (step 4 of 5)

# 3. Load step context
bd show impl-task-1-001.implement
# → Shows instructions, previous step notes

# 4. Resume execution
# Claude continues from exactly where it left off
```

### Session Handoff

Each step can store notes for the next session:

```bash
# Before session end
bd update impl-task-1-001.implement --notes "
COMPLETED:
- Created UserService class
- Added validation logic

IN PROGRESS:
- Implementing password hashing

NEXT:
- Add rate limiting
- Handle edge case for existing emails

BLOCKERS: None
"
```

---

## Human Gates

Gates are molecules with blocking steps that pause execution for human review.

### Gate Template

```toml
[meta]
name = "human-gate"
description = "Human review checkpoint"
requires_human = true

[[steps]]
id = "prepare-summary"
description = "Summarize completed work for reviewer"
instructions = """
Create a summary of work completed since the last gate:
1. What tasks were completed
2. Key decisions made
3. Any concerns or questions
4. Test results and coverage
"""

[[steps]]
id = "notify"
description = "Send notification to human reviewer"
needs = ["prepare-summary"]
action = "notify"
channels = ["slack", "email", "desktop"]
message_template = """
🚦 MEOW Gate Reached

Work completed: {{summary.tasks_completed}}
Questions: {{summary.questions}}

Review and approve: meow approve
"""

[[steps]]
id = "await-approval"
description = "Wait for human to review and approve"
needs = ["notify"]
type = "blocking-gate"
# Execution pauses here until human closes this step

[[steps]]
id = "record-decision"
description = "Record human's decision and notes"
needs = ["await-approval"]
instructions = """
Record the human's decision:
- Approval status
- Any feedback or concerns
- Requested changes
"""
```

### Gate Frequency

The `bake-meta` template controls gate frequency:

```toml
[meta]
name = "bake-meta"
description = "Create meta-molecule with human gates"

[variables]
gate_frequency = { default = 2, description = "Insert gate every N tasks" }

# Logic creates: task-1, task-2, GATE, task-3, task-4, GATE, ...
```

### Gate Approval

Human approves via CLI:

```bash
# See current gate
meow status
# → Paused at: meta-mol-001.human-gate (impl-task-1-001 complete)

# Review work
bd show meta-mol-001.human-gate.prepare-summary --notes

# Approve (closes the await-approval step)
meow approve --notes "LGTM, proceed with OAuth implementation"

# Or reject with feedback
meow reject --notes "Need more test coverage for edge cases"
# → Creates rework bead, restarts relevant tasks
```

---

## Task Selection

The outer loop uses intelligent task selection powered by beads_viewer.

### Selection Algorithm

```toml
# .beads/templates/analyze-pick.toml
[meta]
name = "analyze-pick"
description = "Analyze project and select next high-impact work"

[[steps]]
id = "run-triage"
description = "Run bv --robot-triage to get scored recommendations"
instructions = """
Execute: bv --robot-triage

Parse the JSON output to understand:
1. Top recommendations with scores
2. Quick wins (low effort, high impact)
3. Blockers to clear (unblock downstream work)
4. Project health metrics
"""

[[steps]]
id = "analyze-context"
description = "Consider project context beyond scores"
needs = ["run-triage"]
instructions = """
Consider factors bv doesn't know:
1. Current momentum (what's already in progress?)
2. Technical dependencies (does order matter?)
3. Human requests (any explicit priorities?)
4. Risk factors (are there deadlines?)
"""

[[steps]]
id = "select-batch"
description = "Select tasks for next meta-molecule"
needs = ["analyze-context"]
instructions = """
Select 2-4 related tasks that:
1. Can be worked together coherently
2. Complete one or more epics
3. Fit well in a meta-molecule with gates

Record selection in step notes for bake-meta to use.
"""
output = "selected_tasks"  # Available to next steps
```

### Scoring Factors

From beads_viewer (`bv --robot-triage`):

| Factor | Weight | Description |
|--------|--------|-------------|
| PageRank | 0.22 | Recursive dependency importance |
| Betweenness | 0.20 | Bottleneck/bridge importance |
| BlockerRatio | 0.13 | Direct blocking count |
| Priority | 0.10 | Explicit priority (0-4) |
| TimeToImpact | 0.10 | Critical path depth |
| Urgency | 0.10 | Labels + time decay |
| Risk | 0.10 | Volatility signals |
| Staleness | 0.05 | Age-based surfacing |

### Selection Outputs

```json
{
  "selected_tasks": ["bd-task-001", "bd-task-002"],
  "selected_epic": "bd-epic-001",
  "rationale": "Highest PageRank, unblocks 3 downstream tasks",
  "estimated_complexity": "medium",
  "gate_after": true
}
```

---

## Error Handling

### Error Categories

| Category | Example | Default Action |
|----------|---------|----------------|
| Step failure | Tests don't pass | inject-gate |
| Template error | Missing variable | abort |
| System error | Git conflict | pause + notify |
| Timeout | Step exceeds limit | inject-gate |

### Error Handling Strategies

**inject-gate**: Create a triage gate for human/AI decision

```toml
[meta]
on_error = "inject-gate"
error_gate_template = "error-triage"
```

The error-triage template:

```toml
[[steps]]
id = "analyze-error"
description = "Analyze what went wrong"

[[steps]]
id = "propose-solutions"
description = "Propose possible solutions"
needs = ["analyze-error"]

[[steps]]
id = "await-decision"
description = "Wait for decision on how to proceed"
type = "blocking-gate"
needs = ["propose-solutions"]
options = ["retry", "skip", "rework", "abort"]
```

**retry**: Automatically retry the step

```toml
[meta]
on_error = "retry"
max_retries = 3
retry_delay_seconds = 60
```

**skip**: Mark step as skipped, continue

```toml
[meta]
on_error = "skip"
# Only for non-critical steps
```

**abort**: Stop execution, require human intervention

```toml
[meta]
on_error = "abort"
# For critical failures
```

### Error Propagation

Errors bubble up through the molecule stack:

```
impl-task-1-001.implement FAILS
    ↓ (on_error = inject-gate)
impl-task-1-001.error-triage CREATED
    ↓ (human decides: rework)
impl-task-1-001.implement RESET to open
    ↓ (retry)
impl-task-1-001.implement SUCCEEDS
    ↓ (continue normally)
```

If error-triage decides "abort":

```
impl-task-1-001 ABORTED
    ↓ (bubble up)
meta-mol-001.task-1 BLOCKED (error in child)
    ↓ (meta-level error handling)
meta-mol-001.error-triage CREATED
    ↓ (human decides: skip task, continue with task-2)
```

---

## Parallel Fork-Join

MEOW Stack supports true parallel execution where multiple Claude instances work simultaneously on independent tasks.

### Parallel Step Marking

Steps can be marked as parallel-capable in templates or when baking molecules:

```toml
[[steps]]
id = "review"
description = "Review code changes"
template = "review"
parallel = true                  # Can run in parallel
parallel_group = "validation"    # Group ID for joining

[[steps]]
id = "test"
description = "Run test suite"
template = "test"
parallel = true
parallel_group = "validation"    # Same group = parallel siblings

[[steps]]
id = "merge"
description = "Merge changes"
needs = ["review", "test"]       # Join point - waits for both
```

### Fork-Join Execution Model

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          PARALLEL FORK-JOIN MODEL                               │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  Sequential execution:      task-1 ──▶ task-2 ──▶ task-3                        │
│                                                                                 │
│  Parallel fork-join:                                                            │
│                                                                                 │
│                            ┌──▶ review ──┐                                      │
│        task-1 ──▶ fork ────┤             ├──▶ join ──▶ merge                    │
│                            └──▶ test ────┘                                      │
│                                                                                 │
│  Complex DAG:                                                                   │
│                                                                                 │
│                  ┌──▶ review ──┐       ┌──▶ e2e ────┐                           │
│        impl ─────┤             ├──▶ J1 ┤            ├──▶ J2 ──▶ deploy          │
│                  ├──▶ unit ────┤       └──▶ perf ───┘                           │
│                  └──▶ lint ────┘                                                │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Spawn Modes

When forking, each parallel branch can be spawned in different modes:

| Mode | Behavior | Use Case |
|------|----------|----------|
| `clone` | `claude --resume <checkpoint>` | Shared context, agents understand full history |
| `fresh` | `claude --prompt <template>` | Clean context, independent work |

```toml
[[steps]]
id = "review"
parallel = true
spawn_mode = "clone"      # Inherit parent context via --resume

[[steps]]
id = "benchmark"
parallel = true
spawn_mode = "fresh"      # Start fresh with template prompt only
template = "benchmark"    # Required for fresh mode
```

**Default**: `clone` (shared context) unless step has a `template` field, then `fresh`.

### Parallel Execution Trace

```
╔══════════════════════════════════════════════════════════════════════════════╗
║  PARALLEL FORK                                                               ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Session A (parent) at step: fork-validation                                 ║
║  Parallel group: "validation" → [review, test]                               ║
║                                                                              ║
║  1. Save checkpoint: checkpoint-A-after-implement                            ║
║  2. Record fork state:                                                       ║
║     {                                                                        ║
║       "fork_id": "fork-validation-001",                                      ║
║       "parallel_group": "validation",                                        ║
║       "branches": ["review", "test"],                                        ║
║       "parent_session": "session-A",                                         ║
║       "checkpoint": "checkpoint-A-after-implement",                          ║
║       "join_step": "merge"                                                   ║
║     }                                                                        ║
║  3. Spawn Session B (review) in worktree: ~/worktrees/meow-review-001        ║
║  4. Spawn Session C (test) in worktree: ~/worktrees/meow-test-001            ║
║  5. Session A EXITS (not waiting - no token waste)                           ║
║                                                                              ║
║  Status: Parent suspended, children running in parallel                      ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  PARALLEL EXECUTION                                                          ║
╠══════════════════════════════════════════════════════════════════════════════╣
║                                                                              ║
║  Session B (review)              │  Session C (test)                         ║
║  ─────────────────────           │  ─────────────────                        ║
║  Worktree: meow-review-001       │  Worktree: meow-test-001                  ║
║  Branch: meow/review-001         │  Branch: meow/test-001                    ║
║  Mode: clone (--resume)          │  Mode: clone (--resume)                   ║
║                                  │                                           ║
║  Executes review molecule...     │  Executes test molecule...                ║
║  ├─ load-diff                    │  ├─ setup-env                             ║
║  ├─ check-style                  │  ├─ run-unit                              ║
║  ├─ check-security               │  ├─ run-integration                       ║
║  └─ write-review                 │  └─ report-results                        ║
║                                  │                                           ║
║  Completes first!                │  Still running...                         ║
║  Writes handoff, closes terminal │                                           ║
║  Orchestrator: 1/2 complete      │  Completes!                               ║
║                                  │  Writes handoff, closes terminal          ║
║                                  │  Orchestrator: 2/2 complete → JOIN        ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  PARALLEL JOIN                                                               ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  All branches complete. Orchestrator triggers join:                          ║
║                                                                              ║
║  1. Collect handoff context from both branches                               ║
║  2. Merge git branches (or flag conflicts for human)                         ║
║  3. Resume Session A: claude --resume checkpoint-A-after-implement           ║
║  4. Inject handoff context into resumed session                              ║
║  5. Session A continues with "merge" step                                    ║
║                                                                              ║
║  Session A sees:                                                             ║
║    - Review handoff: "LGTM, minor style issues fixed"                        ║
║    - Test handoff: "All 147 tests passing, 92% coverage"                     ║
║    - Merged code from both branches                                          ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝
```

### Join Conditions

The join step can specify different completion conditions:

```toml
[[steps]]
id = "merge"
needs = ["review", "test", "lint"]
join_condition = "all"         # Wait for all (default)

[[steps]]
id = "fast-path"
needs = ["primary", "backup"]
join_condition = "any"         # Continue when first completes

[[steps]]
id = "quorum"
needs = ["node-1", "node-2", "node-3"]
join_condition = "2_of_3"      # Wait for 2 of 3
```

### Failure Handling in Parallel

```toml
[meta]
parallel_failure_mode = "fail_fast"  # or: continue, isolate

# fail_fast: If any branch fails, abort siblings and join
# continue: Let other branches complete, report partial results
# isolate: Failed branch doesn't affect others, join proceeds
```

---

## Multi-Agent Orchestration

MEOW Stack uses a lightweight **orchestrator daemon** (not Claude) to manage multiple Claude sessions.

### Orchestrator Architecture

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                           MEOW ORCHESTRATOR                                     │
│                                                                                 │
│  A lightweight daemon (Go binary) that manages Claude session lifecycle        │
│                                                                                 │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  RESPONSIBILITIES                                                       │   │
│  ├─────────────────────────────────────────────────────────────────────────┤   │
│  │                                                                         │   │
│  │  1. SESSION SPAWNING                                                    │   │
│  │     • Create tmux sessions for molecules                                │   │
│  │     • Launch claude with --resume or fresh prompt                       │   │
│  │     • Create git worktrees for isolation                                │   │
│  │     • Track session → molecule mapping                                  │   │
│  │                                                                         │   │
│  │  2. COMPLETION MONITORING                                               │   │
│  │     • Watch beads for terminal step closure                             │   │
│  │     • Detect parallel join points                                       │   │
│  │     • Handle session crashes/timeouts                                   │   │
│  │                                                                         │   │
│  │  3. PARENT RESUME                                                       │   │
│  │     • When children complete, resume suspended parent                   │   │
│  │     • Inject handoff context into resumed session                       │   │
│  │     • Merge worktree branches                                           │   │
│  │                                                                         │   │
│  │  4. RESOURCE MANAGEMENT                                                 │   │
│  │     • File reservations via mcp_agent_mail                              │   │
│  │     • Worktree cleanup after completion                                 │   │
│  │     • Session limits and throttling                                     │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                 │
│  ┌─────────────────────────────────────────────────────────────────────────┐   │
│  │  STATE STORAGE                                                          │   │
│  ├─────────────────────────────────────────────────────────────────────────┤   │
│  │                                                                         │   │
│  │  .beads/molecules/orchestrator.json:                                    │   │
│  │  {                                                                      │   │
│  │    "sessions": {                                                        │   │
│  │      "session-A": {                                                     │   │
│  │        "molecule": "meta-mol-001",                                      │   │
│  │        "status": "suspended",                                           │   │
│  │        "checkpoint": "checkpoint-A-after-impl",                         │   │
│  │        "tmux": "meow-meta-001",                                         │   │
│  │        "worktree": null,                                                │   │
│  │        "waiting_for": ["review-mol-001", "test-mol-001"],               │   │
│  │        "fork_id": "fork-validation-001"                                 │   │
│  │      },                                                                 │   │
│  │      "session-B": {                                                     │   │
│  │        "molecule": "review-mol-001",                                    │   │
│  │        "status": "active",                                              │   │
│  │        "tmux": "meow-review-001",                                       │   │
│  │        "worktree": "/home/user/worktrees/meow-review-001",              │   │
│  │        "branch": "meow/review-001",                                     │   │
│  │        "parent_session": "session-A"                                    │   │
│  │      }                                                                  │   │
│  │    },                                                                   │   │
│  │    "forks": {                                                           │   │
│  │      "fork-validation-001": {                                           │   │
│  │        "branches": ["session-B", "session-C"],                          │   │
│  │        "completed": ["session-B"],                                      │   │
│  │        "join_step": "merge"                                             │   │
│  │      }                                                                  │   │
│  │    }                                                                    │   │
│  │  }                                                                      │   │
│  │                                                                         │   │
│  └─────────────────────────────────────────────────────────────────────────┘   │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Completion Detection

The orchestrator monitors beads for the **terminal step** of each molecule:

```yaml
# Molecule definition
mol-review-001:
  terminal_step: "write-review"  # Last step in molecule
  children:
    - "mol-review-001.load-diff"
    - "mol-review-001.check-style"
    - "mol-review-001.write-review"  # ← When this closes, molecule is complete
```

**Completion signal**: When `terminal_step` status becomes `closed`:

```bash
# Orchestrator polls or watches beads
bd show mol-review-001.write-review --format json | jq '.status'
# → "closed" means molecule complete
```

Or via file watch:

```bash
# Orchestrator watches for .done file
~/.meow/molecules/mol-review-001.done
```

### Session Lifecycle

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                         SESSION STATES                                          │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  SPAWNING ──▶ ACTIVE ──┬──▶ COMPLETED                                           │
│                        │                                                        │
│                        ├──▶ SUSPENDED (waiting for children)                    │
│                        │        │                                               │
│                        │        └──▶ RESUMED ──▶ ACTIVE                         │
│                        │                                                        │
│                        ├──▶ FAILED                                              │
│                        │                                                        │
│                        └──▶ TIMED_OUT                                           │
│                                                                                 │
│  State transitions:                                                             │
│  • SPAWNING: Orchestrator creating tmux session, worktree                       │
│  • ACTIVE: Claude running, executing steps                                      │
│  • SUSPENDED: Claude exited, checkpoint saved, awaiting children                │
│  • RESUMED: Claude restarted with --resume, children complete                   │
│  • COMPLETED: Molecule finished, handoff written, cleanup pending               │
│  • FAILED: Error during execution, needs intervention                           │
│  • TIMED_OUT: Exceeded time limit, killed                                       │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### Spawning Commands

```bash
# Orchestrator spawns a session
meow-orchestrator spawn \
  --molecule mol-review-001 \
  --mode clone \
  --parent-checkpoint checkpoint-A \
  --worktree ~/worktrees/meow-review-001 \
  --branch meow/review-001

# Internally runs:
tmux new-session -d -s meow-review-001 \
  "cd ~/worktrees/meow-review-001 && claude --resume checkpoint-A --prompt 'Execute review molecule mol-review-001'"
```

### File Reservations

Parallel agents use mcp_agent_mail for file coordination:

```bash
# Session B (review) reserves review-related files
am lease --path "*.md" --path "docs/**" --duration 30m --agent review-001

# Session C (test) reserves test-related files
am lease --path "tests/**" --path "*.test.js" --duration 30m --agent test-001

# No conflicts - different file patterns
```

---

## Git Worktrees

Each spawned Claude session operates in its own **git worktree** for complete isolation.

### Worktree Structure

```
~/                                    # Home directory
├── projects/
│   └── myapp/                        # Main repository
│       ├── .git/                     # Git directory
│       ├── .beads/                   # Beads state
│       └── src/                      # Source code
│
└── worktrees/                        # MEOW worktrees
    └── myapp/                        # Per-project worktree directory
        ├── meow-review-001/          # Session B worktree
        │   ├── .git → ../../../projects/myapp/.git
        │   ├── .beads → ../../../projects/myapp/.beads  # Symlink!
        │   └── src/                  # Own copy of source
        │
        ├── meow-test-001/            # Session C worktree
        │   ├── .git → ...
        │   ├── .beads → ...          # Shared beads state
        │   └── src/
        │
        └── meow-impl-002/            # Another session
            └── ...
```

### Branch Naming Convention

Auto-generated branch names follow the pattern:

```
meow/<molecule-id>[-<suffix>]

Examples:
  meow/mol-impl-001
  meow/review-001
  meow/test-001-retry-1
```

### Worktree Creation

```bash
# Orchestrator creates worktree for new session
WORKTREE_PATH=~/worktrees/myapp/meow-review-001
BRANCH_NAME=meow/review-001

# Create worktree with new branch from current HEAD
git worktree add -b $BRANCH_NAME $WORKTREE_PATH HEAD

# Symlink beads directory (shared state)
ln -s $(pwd)/.beads $WORKTREE_PATH/.beads

# Session operates in worktree
cd $WORKTREE_PATH
claude --resume checkpoint-A
```

### Branch Merging at Join

When parallel branches complete, the orchestrator merges:

```bash
# At join point, merge all parallel branches
git checkout main  # or parent branch

# Merge each branch
git merge --no-ff meow/review-001 -m "Merge review from mol-review-001"
git merge --no-ff meow/test-001 -m "Merge test from mol-test-001"

# If conflicts, create conflict-resolution gate
# Otherwise, cleanup worktrees
git worktree remove ~/worktrees/myapp/meow-review-001
git worktree remove ~/worktrees/myapp/meow-test-001
git branch -d meow/review-001
git branch -d meow/test-001
```

### Conflict Handling

```yaml
# If merge conflicts detected:
conflict_gate:
  type: gate
  title: "Merge Conflict: review-001 + test-001"
  description: |
    Conflict in files:
    - src/auth/login.js
    - tests/auth.test.js

    Branches:
    - meow/review-001: style fixes
    - meow/test-001: new test cases

  resolution_options:
    - keep_review
    - keep_test
    - manual_merge
```

---

## Handoff Context

When a child molecule completes, it writes **handoff context** for the parent to consume.

### Handoff Structure

```yaml
# Written by child session before exit
handoff:
  molecule_id: "mol-review-001"
  status: success | failure | partial

  summary: |
    Code review complete. Found 3 minor issues, all fixed.
    No security concerns. Style is consistent.

  artifacts:
    - path: "review-report.md"
      type: "report"
    - path: "src/auth/login.js"
      type: "modified"

  next_steps:
    - "Proceed with merge"
    - "Run integration tests"

  context:
    issues_found: 3
    issues_fixed: 3
    coverage_delta: "+2.3%"
    review_time_minutes: 12

  warnings:
    - "Large function in auth/login.js:142 - consider refactoring"

  blocking_issues: []
```

### Writing Handoff

Claude writes handoff as the final action before molecule completion:

```bash
# Template instruction for terminal step
[[steps]]
id = "complete"
description = "Write handoff and close molecule"
instructions = """
Before closing this molecule, write handoff context:

bd update {{molecule_id}} --handoff "$(cat <<'EOF'
status: success
summary: |
  [Summarize what was accomplished]
artifacts:
  - path: [files modified/created]
next_steps:
  - [What parent should do next]
context:
  [key]: [value]
EOF
)"

Then close the terminal step:
bd close {{molecule_id}}.{{terminal_step}}
"""
```

### Reading Handoff

Parent session receives handoff when resumed:

```bash
# Orchestrator injects handoff into resume prompt
claude --resume checkpoint-A --prompt "
Children completed. Handoff context:

## mol-review-001 (success)
$(bd show mol-review-001 --handoff)

## mol-test-001 (success)
$(bd show mol-test-001 --handoff)

Continue with step: merge
"
```

### Handoff Aggregation

For parallel joins, handoffs are aggregated:

```yaml
# Aggregated handoff at join
aggregated_handoff:
  fork_id: "fork-validation-001"
  overall_status: success  # all succeeded

  branches:
    - molecule: mol-review-001
      status: success
      summary: "Review complete, 3 issues fixed"

    - molecule: mol-test-001
      status: success
      summary: "All 147 tests passing"

  combined_artifacts:
    - review-report.md
    - test-results.json
    - coverage.html

  combined_context:
    review_issues: 3
    tests_passed: 147
    coverage: "92%"

  merge_status: clean  # or: conflicts
```

---

## Shell-Based Conditions

Conditions in MEOW Stack are **shell commands** that must exit 0 (success) to proceed.

### Condition Syntax

```toml
[[steps]]
id = "deploy"
description = "Deploy to production"
condition = "test $(date +%u) -lt 6"  # Only Mon-Fri

[[steps]]
id = "run-e2e"
condition = "curl -s http://localhost:3000/health | grep -q 'ok'"
```

### Condition Evaluation

The orchestrator/executor evaluates conditions by running them as shell commands:

```bash
# Evaluate condition
if eval "$condition"; then
  # Condition passed, execute step
else
  # Condition failed, skip step
fi
```

### Built-in Condition Helpers

MEOW provides helper scripts for common conditions:

```bash
# .meow/bin/meow-condition

# Check if all tasks in epic are closed
meow-condition all-closed bd-epic-001

# Check if any task is blocked
meow-condition any-blocked bd-epic-001

# Check if file exists
meow-condition file-exists coverage.html

# Check if tests pass
meow-condition tests-pass pytest

# Check if branch is clean
meow-condition git-clean

# Check if API returns success
meow-condition api-ok http://localhost:3000/health
```

### Condition Examples

```toml
# Continue only if no open blockers
[[steps]]
id = "start-feature"
condition = "! bd list --status=blocked --parent={{epic_id}} | grep -q ."

# Only run if coverage above threshold
[[steps]]
id = "approve-pr"
condition = "test $(jq '.coverage' coverage.json) -ge 80"

# Time-based gate
[[steps]]
id = "deploy-prod"
condition = """
  # Only during business hours, not Friday
  hour=$(date +%H)
  dow=$(date +%u)
  test $hour -ge 9 && test $hour -lt 17 && test $dow -lt 5
"""

# External API check
[[steps]]
id = "proceed-if-approved"
condition = "curl -s https://api.example.com/approval/{{pr_id}} | jq -e '.approved'"

# Git-based condition
[[steps]]
id = "merge-to-main"
condition = """
  git fetch origin main
  git merge-base --is-ancestor origin/main HEAD
"""

# Custom script
[[steps]]
id = "run-after-validation"
condition = "./.meow/conditions/validate-schema.sh"
```

### Condition in Loops

For restart steps, condition determines whether to continue looping:

```toml
[[steps]]
id = "restart"
type = "restart"
condition = """
  # Continue while: open epics exist AND under iteration limit
  bd list --type=epic --status=open | grep -q . && \
  test $(cat .beads/molecules/meow-state.json | jq '.iteration') -lt 100
"""
```

### Condition in Gates

Human gates can use conditions for auto-approval:

```toml
[[steps]]
id = "auto-approve-if-green"
type = "conditional-gate"
auto_approve_condition = """
  # Auto-approve if all tests pass and coverage > 90%
  jq -e '.passed == true and .coverage >= 90' test-results.json
"""
# If condition passes: auto-close gate
# If condition fails: wait for human
```

### Condition Timeout

Conditions have a default timeout to prevent hanging:

```toml
[[steps]]
id = "wait-for-service"
condition = "curl -s --max-time 30 http://localhost:3000/health"
condition_timeout = 60  # seconds
```

---

## Summary

MEOW Stack provides:

1. **Recursive molecule architecture** — Everything is a molecule, arbitrary nesting
2. **Durable execution** — Git-backed state, crash recovery, resumption anywhere
3. **Composable templates** — Reusable workflows, template inheritance
4. **Intelligent selection** — PageRank-weighted task scoring
5. **Human-in-the-loop** — First-class gates, not interruptions
6. **Unified semantics** — One execution model for loops, gates, workflows
7. **Parallel fork-join** — True multi-agent parallelism with clone/fresh spawn modes
8. **Git worktree isolation** — Each agent works in its own worktree and branch
9. **Handoff context** — Structured data passing between parent and child agents
10. **Shell-based conditions** — Flexible gating via arbitrary shell commands

The result is a **multi-agent workflow system that thinks in molecules** — from the outer orchestration loop down to parallel execution branches, all using the same primitives, all surviving crashes, all resumable. Parent agents don't waste tokens waiting — they suspend and resume when children complete, with full context handoff.
