# MEOW Stack Spec Addendum: Wisps, Modules, and Three-Tier Beads

> **Status**: Draft (Refined)
> **Authors**: Design discussion synthesis
> **Date**: 2026-01-07
> **Depends on**: MVP-SPEC.md

This document extends the MVP specification with three major enhancements:

1. **Three-Tier Bead Architecture** - Work beads, workflow beads (wisps), and orchestrator beads
2. **Module System** - Multi-workflow files with local references and `main` convention
3. **Automatic Wisp Detection** - Smart partitioning of templates into agent-visible and orchestrator-only beads

---

## Table of Contents

1. [Motivation](#motivation)
2. [Beads Fork Note](#beads-fork-note)
3. [Three-Tier Bead Architecture](#three-tier-bead-architecture)
4. [Module System](#module-system)
5. [Automatic Wisp Detection](#automatic-wisp-detection)
6. [Agent Work Bead Access](#agent-work-bead-access)
7. [CRUD Operations by Tier](#crud-operations-by-tier)
8. [Command Visibility Rules](#command-visibility-rules)
9. [Crash Recovery](#crash-recovery)
10. [Implementation Details](#implementation-details)

---

## Motivation

### The Problem

The original MVP spec defines ephemeral beads for cleanup but doesn't address:

1. **Agent visibility** - Agents see ALL assigned beads including infrastructure machinery (start/stop/condition) that they shouldn't care about
2. **Workflow context** - No way to show agents their workflow progression ("step 2 of 4")
3. **Work vs workflow distinction** - Agents conflate "what to work on" (work beads) with "how to work" (workflow steps)
4. **Template organization** - Complex workflows require many files, leading to file explosion

### The Insight: Claude's TODO List Analogy

Claude Code has two distinct tracking systems:
- **Beads** - Persistent work items that survive sessions
- **TODO list** - Ephemeral procedural guidance that disappears

MEOW needs the same distinction:
- **Work beads** - The actual deliverables (persist forever)
- **Wisps** - Procedural guidance for how to do work (burned after completion)

### The Solution

This addendum introduces:
1. A **three-tier visibility model** for beads
2. A **module system** for organizing templates
3. **Automatic wisp detection** during template baking

---

## Beads Fork Note

> **Important**: MEOW Stack will maintain a **fork of the beads CLI/library** to support workflow-specific functionality.

### Rationale

The upstream `beads` project is a general-purpose issue tracking system. MEOW requires:

1. **New bead types** - `start`, `stop`, `condition`, `code`, `expand` as first-class `IssueType` values
2. **Tier field** - Explicit `Tier` enum on the `Issue` struct (not computed from labels)
3. **Workflow metadata** - `HookBead`, `SourceWorkflow`, `WorkflowID` fields
4. **Custom statuses** - Potentially workflow-specific status values
5. **Output storage** - `Outputs map[string]any` for bead-to-bead data flow
6. **Type-specific specs** - `ConditionSpec`, `StartSpec`, etc. on the Issue struct

### Fork Strategy

```
github.com/anthropics/beads           # Upstream (general issue tracking)
github.com/meow-stack/meow-beads      # Fork (workflow orchestration)
```

The fork will:
- Maintain compatibility with upstream beads file format where possible
- Add MEOW-specific fields and types
- Be versioned independently
- Potentially contribute general improvements back upstream

### Fields to Add to Issue Struct

```go
type Issue struct {
    // Existing beads fields...

    // MEOW additions:
    Tier          BeadTier          `json:"tier"`                     // work | wisp | orchestrator
    HookBead      string            `json:"hook_bead,omitempty"`      // Work bead this wisp implements
    SourceWorkflow string           `json:"source_workflow,omitempty"` // Workflow that created this
    WorkflowID    string            `json:"workflow_id,omitempty"`    // Unique workflow instance ID
    Outputs       map[string]any    `json:"outputs,omitempty"`        // Captured outputs

    // Type-specific specs (only one set based on IssueType)
    ConditionSpec *ConditionSpec    `json:"condition_spec,omitempty"`
    StartSpec     *StartSpec        `json:"start_spec,omitempty"`
    StopSpec      *StopSpec         `json:"stop_spec,omitempty"`
    CodeSpec      *CodeSpec         `json:"code_spec,omitempty"`
    ExpandSpec    *ExpandSpec       `json:"expand_spec,omitempty"`
}
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

Three statuses only - no `hooked` status:

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

**Why no `hooked` status?**

The original design had `hooked` to distinguish "agent claimed via meow prime" from "agent actively working." In practice:

1. There's no observable moment when an agent transitions from "claimed" to "working"
2. Crash recovery doesn't need it - we check if the agent process is alive, not what phase they're in
3. It adds state machine complexity without clear benefit

If heartbeat tracking is needed, use a timestamp field instead:

```go
type Bead struct {
    LastSeen time.Time `json:"last_seen,omitempty"` // Optional agent heartbeat
}
```

### Issue Types

The 6 primitives plus `gate` for human approval and `collaborative` for interactive steps:

```go
const (
    // Agent-executable
    TypeTask          IssueType = "task"          // Agent does work, auto-continues
    TypeCollaborative IssueType = "collaborative" // Agent + human conversation, pauses for interaction
    TypeGate          IssueType = "gate"          // Human approval point (no assignee)

    // Orchestrator-executable
    TypeStart     IssueType = "start"     // Spawn agent
    TypeStop      IssueType = "stop"      // Kill agent
    TypeCondition IssueType = "condition" // Branch/loop/wait
    TypeCode      IssueType = "code"      // Run shell command
    TypeExpand    IssueType = "expand"    // Expand template
)
```

**Type behavior summary:**

| Type | Assignee | Auto-continue | Who closes | Use case |
|------|----------|---------------|------------|----------|
| `task` | Required | Yes (Ralph Wiggum loop) | Agent | Normal autonomous work |
| `collaborative` | Required | No (pauses for conversation) | Agent | Design review, clarification |
| `gate` | None | No | Human | Approval checkpoints |

**Note**: `gate` replaces the `orchestrator_task` boolean flag. A gate is explicitly a human-facing task with no assignee that the orchestrator waits on.

### The `collaborative` Type

The `collaborative` type enables **human-in-the-loop interaction** within an otherwise autonomous workflow. When an agent reaches a collaborative step:

1. Agent executes the step (presents information, asks questions)
2. Agent's stop hook fires, BUT...
3. `meow prime --format prompt` returns **empty output** for in-progress collaborative steps
4. No prompt injection → Claude waits naturally for user input
5. User and agent converse freely
6. When done, agent runs `meow close <step-id>`
7. Next stop hook fires, normal flow resumes

**Use cases:**
- Design review before implementation
- Clarification of ambiguous requirements
- Approval of risky operations (with context)
- Interactive debugging sessions

**Example workflow with collaborative step:**

```toml
[implement-with-review]
ephemeral = true
hooks_to = "work_bead"

[[implement-with-review.steps]]
id = "draft-design"
type = "task"
title = "Draft the design"
assignee = "{{agent}}"
instructions = "Create initial design based on requirements"

[[implement-with-review.steps]]
id = "review-design"
type = "collaborative"  # ← Pauses here for conversation!
title = "Review design with user"
assignee = "{{agent}}"
needs = ["draft-design"]
instructions = """
Present the design to the user:
1. Summarize the approach
2. Highlight key decisions and tradeoffs
3. Ask if they have questions or want changes

When the user is satisfied, close this step:
  meow close review-design --notes "User approved"
"""

[[implement-with-review.steps]]
id = "implement"
type = "task"  # ← Back to auto-continue
title = "Implement the approved design"
assignee = "{{agent}}"
needs = ["review-design"]
```

**User experience:**

```
Agent: Here's my design for the auth system:

       ## Approach
       - JWT tokens with 1-hour expiry
       - Refresh tokens stored in httpOnly cookies
       - Rate limiting at 100 req/min per IP

       Do you have any questions or would you like changes?

User: What about rate limiting per user, not just IP?

Agent: Good point! I'd add per-user rate limiting too:
       - 100 req/min per IP (DDoS protection)
       - 1000 req/min per authenticated user (abuse prevention)

       Should I proceed with both?

User: Yes, looks good. Let's go.

Agent: Great! Closing the review step and moving to implementation.
       $ meow close review-design --notes "User approved with per-user rate limiting"

[Stop hook fires, meow prime outputs next step]

Agent: Moving on to implementation. Starting with the JWT middleware...
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

---

## Module System

### File Format

MEOW uses a **module format** where files contain one or more named workflows:

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

[main.steps.outputs]
required = [
    { name = "work_bead", type = "bead_id", description = "The bead to implement" },
    { name = "rationale", type = "string", description = "Why you chose this bead" }
]

[[main.steps]]
id = "do-work"
type = "expand"
template = ".implement"   # Local reference
variables = { work_bead = "{{select-work.outputs.work_bead}}" }
needs = ["select-work"]

[[main.steps]]
id = "check-more"
type = "condition"
condition = "bd list --status=open --type=task | grep -q ."
needs = ["do-work"]

[main.steps.on_true]
template = ".main"        # Recursive loop

[main.steps.on_false]
inline = []               # Done


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

[[implement.steps]]
id = "implement"
type = "task"
title = "Implement to make tests pass"
assignee = "{{agent}}"
needs = ["write-tests"]

[[implement.steps]]
id = "verify"
type = "condition"
condition = "go test ./..."
needs = ["implement"]

[implement.steps.on_true]
inline = [
    { id = "commit", type = "task", title = "Commit changes", assignee = "{{agent}}" }
]

[implement.steps.on_false]
template = ".fix-tests"

[[implement.steps]]
id = "close-work"
type = "code"
code = "bd close {{work_bead}} --notes 'Implemented via TDD workflow'"
needs = ["verify"]


# ═══════════════════════════════════════════════════════════════════════════
# FIX-TESTS WORKFLOW (Helper)
# ═══════════════════════════════════════════════════════════════════════════

[fix-tests]
name = "fix-tests"
description = "Fix failing tests and retry"
ephemeral = true
internal = true
hooks_to = "work_bead"

[[fix-tests.steps]]
id = "analyze-failure"
type = "task"
title = "Analyze test failures"
assignee = "{{agent}}"

[[fix-tests.steps]]
id = "fix"
type = "task"
title = "Fix the implementation"
assignee = "{{agent}}"
needs = ["analyze-failure"]

[[fix-tests.steps]]
id = "retry-verify"
type = "expand"
template = ".implement"
variables = { work_bead = "{{work_bead}}" }
needs = ["fix"]
# Note: This jumps back into implement, starting from verify's dependencies
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
| `/abs/path.meow.toml#workflow` | Absolute path |

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

### Step Schema

Complete step definition with all fields:

```toml
[[workflow.steps]]
# Identity
id = "step-id"                    # Required, unique within workflow
type = "task"                     # task | collaborative | gate | condition | code | start | stop | expand
title = "Human-readable title"    # Required

# Task/Collaborative/Gate fields
instructions = "Detailed instructions for agent"
assignee = "{{agent}}"            # Required for task/collaborative, forbidden for gate

# Dependencies
needs = ["other-step"]            # Step IDs this depends on

# Condition fields
condition = "shell command"       # For type = "condition"
timeout = "5m"                    # Optional timeout

[workflow.steps.on_true]
template = ".other-workflow"
# or
inline = [{ id = "...", type = "task", ... }]

[workflow.steps.on_false]
template = ".error-handler"

# Code fields
code = "shell script"             # For type = "code"
workdir = "/path"                 # Optional working directory

[workflow.steps.code_outputs]
path = "stdout"                   # Capture stdout
commit = "file:/tmp/sha.txt"      # Capture file contents

# Start fields
agent = "claude-2"                # For type = "start"
workdir = "{{setup.outputs.path}}"
prompt = "meow prime"             # Optional, default: "meow prime"
resume_session = "{{save.outputs.session_id}}"  # Optional

[workflow.steps.env]
CUSTOM_VAR = "value"

[workflow.steps.attach_wisp]      # Optional: explicit wisp template
template = "agent-work"
variables = { work_bead = "{{work_bead}}" }

# Stop fields
agent = "claude-2"                # For type = "stop"
graceful = true                   # Optional, default: true
timeout = 10                      # Seconds to wait for graceful shutdown

# Expand fields
template = ".other-workflow"      # For type = "expand"
variables = { key = "value" }

# Output definitions (for task beads)
[workflow.steps.outputs]
required = [
    { name = "result", type = "string", description = "..." }
]
optional = [
    { name = "notes", type = "string" }
]
```

---

## Automatic Wisp Detection

### The Algorithm

When a template is baked, the baker automatically determines each bead's tier:

```go
func (b *Baker) determineTier(step *Step, workflow *Workflow, agentStarts map[string]bool) BeadTier {
    // Non-agent-executable types are always orchestrator
    if step.Type != "task" && step.Type != "collaborative" && step.Type != "gate" {
        return TierOrchestrator
    }

    // Gates are orchestrator (human-facing, no assignee)
    if step.Type == "gate" {
        return TierOrchestrator
    }

    // Tasks and collaborative steps with assignee matching a started agent → wisp
    if step.Assignee != "" && agentStarts[step.Assignee] {
        return TierWisp
    }

    // Tasks/collaborative in ephemeral workflow → wisp
    if workflow.Ephemeral {
        return TierWisp
    }

    // Default: work bead
    return TierWork
}
```

### Simplified Detection

The key insight: **if a workflow is marked `ephemeral = true`, all its tasks become wisps.** The `assignee` field tells us which agent executes them. We don't need to scan for `start` beads.

```toml
[implement]
ephemeral = true  # This is the signal - all tasks below are wisps

[[implement.steps]]
id = "load-context"
type = "task"
assignee = "{{agent}}"  # This tells us WHO executes, not WHAT tier
```

### The `attach_wisp` Override

For complex cases where you want explicit control:

```toml
[[main.steps]]
id = "start-worker"
type = "start"
agent = "claude-2"
workdir = "{{setup-worktree.outputs.path}}"

# Explicit wisp attachment - skips auto-detection for this agent
[main.steps.attach_wisp]
template = "agent-implement"
variables = { work_bead = "{{work_bead}}" }
```

When `attach_wisp` is set:
1. The referenced template is baked as a separate wisp chain
2. Those beads are linked to this `start` bead
3. Auto-detection is SKIPPED for tasks with this agent as assignee

### Validation Rules

At bake time, the baker validates:

1. **Tasks and collaborative steps must have assignee** (in ephemeral workflows):
   ```
   Error: Task 'orphan-task' has no assignee.
          In ephemeral workflows, all tasks must specify an assignee.
   ```

2. **Collaborative steps must have assignee** (always):
   ```
   Error: Collaborative step 'review-design' has no assignee.
          Collaborative steps require an agent to conduct the conversation.
   ```

3. **Gates must NOT have assignee**:
   ```
   Error: Gate 'approval' has assignee 'claude-1'.
          Gates are human-facing and cannot have an agent assignee.
   ```

4. **Assignee must match a known agent** (if start beads exist):
   ```
   Warning: Task 'do-work' assigned to 'claude-3' but no start bead for that agent.
            The agent must be started elsewhere or this task won't execute.
   ```

---

## Agent Work Bead Access

### The Two-View Model

Agents have TWO simultaneous views:

1. **Wisp view** (`meow prime`) - "What step am I on?"
2. **Work view** (`bd ready`, `bv --robot-triage`) - "What work beads exist?"

### Workflow Example

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  AGENT WORKFLOW: Task Selection and Execution                               │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. Agent runs `meow prime`:                                                │
│     ┌─────────────────────────────────────────────────────────────────┐    │
│     │ Your workflow: work-loop (step 1/3)                             │    │
│     │ ────────────────────────────────────                            │    │
│     │   → select-work [in_progress] ← YOU ARE HERE                    │    │
│     │   ○ do-work                                                     │    │
│     │   ○ check-more                                                  │    │
│     │                                                                 │    │
│     │ Instructions:                                                   │    │
│     │   Run bv --robot-triage and select the highest-impact task.     │    │
│     │                                                                 │    │
│     │ Required outputs when closing:                                  │    │
│     │   work_bead (bead_id): The bead to implement                    │    │
│     │   rationale (string): Why you chose this bead                   │    │
│     └─────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  2. Agent queries work beads:                                               │
│     $ bv --robot-triage                                                     │
│     ┌─────────────────────────────────────────────────────────────────┐    │
│     │ READY WORK (ranked by impact):                                  │    │
│     │                                                                 │    │
│     │ 1. [P1] gt-123 "Implement auth endpoint"                        │    │
│     │    Score: 8.5 | Unblocks: 3 | Epic: gt-epic-001                 │    │
│     │                                                                 │    │
│     │ 2. [P2] gt-456 "Add password validation"                        │    │
│     │    Score: 6.2 | Unblocks: 0 | Quick win                         │    │
│     └─────────────────────────────────────────────────────────────────┘    │
│                                                                             │
│  3. Agent closes with outputs:                                              │
│     $ meow close select-work \                                              │
│         --output work_bead=gt-123 \                                         │
│         --output rationale="Highest priority, unblocks 3 others"            │
│                                                                             │
│  4. Orchestrator:                                                           │
│     - Validates outputs (bead_id type checks gt-123 exists)                 │
│     - Stores outputs on select-work bead                                    │
│     - Closes select-work                                                    │
│     - Finds next ready bead: do-work (expand)                               │
│     - Expands with variables: { work_bead = "gt-123" }                      │
│                                                                             │
│  5. Agent runs `meow prime` again:                                          │
│     ┌─────────────────────────────────────────────────────────────────┐    │
│     │ Your workflow: implement (step 1/4)                             │    │
│     │ Work bead: gt-123 "Implement auth endpoint"                     │    │
│     │ ────────────────────────────────────                            │    │
│     │   → load-context [in_progress]                                  │    │
│     │   ○ write-tests                                                 │    │
│     │   ○ implement                                                   │    │
│     │   ○ commit                                                      │    │
│     └─────────────────────────────────────────────────────────────────┘    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Key Insight: Separation of Concerns

| Command | Shows | Agent Action |
|---------|-------|--------------|
| `meow prime` | Current wisp step | Execute step-by-step |
| `bd ready` | Work beads | Select what to work on |
| `bv --robot-triage` | Prioritized work beads | Make informed selection |
| `bd show <id>` | Any bead details | Understand requirements |

The agent can freely query work beads while executing wisp steps. The wisp tells them HOW, the work beads tell them WHAT.

---

## CRUD Operations by Tier

### CREATE

#### Work Beads

```bash
# Human creates via CLI
bd create --title "Implement auth" --type task --priority 2

# PRD breakdown
meow breakdown prd.md

# Programmatic
store.CreateIssue(ctx, &types.Issue{
    Title:     "Implement auth",
    IssueType: types.TypeTask,
    Priority:  2,
    Tier:      types.TierWork,  // Explicit!
})
```

#### Workflow Beads (Wisps)

```go
// Created by template expansion
bead := &types.Issue{
    ID:             generateID(workflow, step),
    Title:          substitute(step.Title, vars),
    IssueType:      types.TypeTask,
    Tier:           types.TierWisp,  // Explicit!
    SourceWorkflow: workflow.Name,
    WorkflowID:     workflowID,
    HookBead:       vars[workflow.HooksTo],  // From hooks_to property
    Assignee:       step.Assignee,
}
```

#### Orchestrator Beads

```go
// Created automatically for non-task primitives
bead := &types.Issue{
    ID:             generateID(workflow, step),
    Title:          step.Title,
    IssueType:      types.IssueType(step.Type),  // start, stop, condition, etc.
    Tier:           types.TierOrchestrator,       // Explicit!
    SourceWorkflow: workflow.Name,
    WorkflowID:     workflowID,
}
```

### READ

#### Work Beads

```go
// bd ready / bv --robot-triage
beads, _ := store.List(ctx, BeadFilter{
    Tier: TierWork,
})
```

#### Workflow Beads (Agent's Wisp)

```go
// meow prime
beads, _ := store.List(ctx, BeadFilter{
    Tier:     TierWisp,
    Assignee: agentID,
})
```

#### Orchestrator Beads

```go
// Orchestrator main loop
beads, _ := store.List(ctx, BeadFilter{
    Status: StatusOpen,
    // No tier filter - sees all
})

// Prioritize by tier
sort.Slice(beads, func(i, j int) bool {
    return tierPriority(beads[i].Tier) < tierPriority(beads[j].Tier)
})
```

### UPDATE

#### Work Beads

```bash
# Standard bd commands
bd update gt-123 --notes "Progress: implemented login"
bd close gt-123 --notes "Completed"
```

#### Workflow Beads

```bash
# Close with validated outputs
meow close select-work \
    --output work_bead=gt-123 \
    --output rationale="Highest priority"

# Or simple close
meow close load-context
```

```go
// Validation before close
func CloseWispStep(ctx context.Context, store Store, stepID string, outputs map[string]string) error {
    step, _ := store.Get(ctx, stepID)

    if err := validateOutputs(step, outputs); err != nil {
        return err  // Reject close if outputs invalid
    }

    step.Outputs = outputs
    step.Status = StatusClosed
    return store.Update(ctx, step)
}
```

#### Orchestrator Beads

```go
// Auto-closed by orchestrator after execution
func (o *Orchestrator) handleCodeBead(ctx context.Context, bead *types.Issue) error {
    output, err := o.executor.Run(bead.CodeSpec.Code)

    bead.Outputs = map[string]any{"stdout": output}
    bead.Status = StatusClosed

    return o.store.Update(ctx, bead)
}
```

### DELETE (Burn)

#### Work Beads

```bash
# Soft-delete (recoverable)
bd delete gt-123 --reason "Duplicate"

# Hard delete
bd delete gt-123 --hard
```

#### Workflow and Orchestrator Beads

```go
// Auto-cleanup on workflow completion
func (o *Orchestrator) cleanupWorkflow(ctx context.Context, workflowID string) error {
    beads, _ := o.store.List(ctx, BeadFilter{
        WorkflowID: workflowID,
    })

    allClosed := true
    for _, bead := range beads {
        if bead.Status != StatusClosed {
            allClosed = false
            break
        }
    }

    if allClosed {
        for _, bead := range beads {
            if bead.Tier != TierWork {
                _ = o.store.Delete(ctx, bead.ID)
            }
        }
    }

    return nil
}
```

**When wisps get burned:**
- On workflow completion (all beads in workflow are closed)
- NOT on agent stop (agent might resume)
- NOT on work bead close (workflow machinery still needed)

---

## Command Visibility Rules

### `meow prime` - Agent's Current Step

Shows ONLY the current agent's wisp step. **Crucially, returns empty output for in-progress collaborative steps** to disable auto-continuation.

```go
func Prime(ctx context.Context, store Store, agentID string, format string) (*PrimeOutput, error) {
    // Check for in-progress collaborative step first
    inProgress, _ := store.List(ctx, BeadFilter{
        Tier:     TierWisp,
        Assignee: agentID,
        Status:   StatusInProgress,
    })

    for _, step := range inProgress {
        if step.IssueType == TypeCollaborative {
            // Collaborative step in progress - don't auto-continue!
            if format == "prompt" {
                return nil, nil  // Empty output = no injection
            }
            // For non-prompt format, show status but indicate conversation mode
            return &PrimeOutput{
                Workflow:        getWorkflowInfo(step),
                WorkBead:        getHookBead(step),
                ConversationMode: true,  // Signal to UI
            }, nil
        }
    }

    // Get ready wisp steps for this agent
    wispSteps, _ := store.List(ctx, BeadFilter{
        Tier:     TierWisp,
        Assignee: agentID,
        Status:   StatusOpen,
    })

    if len(wispSteps) == 0 {
        return nil, nil  // No work for this agent
    }

    // Get all wisp steps for progress display
    allSteps, _ := store.List(ctx, BeadFilter{
        Tier:       TierWisp,
        Assignee:   agentID,
        WorkflowID: wispSteps[0].WorkflowID,
    })

    // Get linked work bead
    var workBead *types.Issue
    if wispSteps[0].HookBead != "" {
        workBead, _ = store.Get(ctx, wispSteps[0].HookBead)
    }

    // Find the first ready step
    current := findFirstReady(wispSteps)

    return &PrimeOutput{
        Workflow: Workflow{
            Name:      current.SourceWorkflow,
            Total:     len(allSteps),
            Completed: countClosed(allSteps),
            Current:   current,
            Steps:     allSteps,
        },
        WorkBead: workBead,
    }, nil
}
```

**Stop hook integration:**

The stop hook relies on `meow prime --format prompt` returning empty for collaborative steps:

```bash
#!/bin/bash
# .claude/hooks/stop-hook.sh

output=$(meow prime --format prompt 2>/dev/null)

if [ -z "$output" ]; then
    # Empty = collaborative mode OR no more work
    # Either way, don't inject anything
    exit 0
fi

# Normal task - inject the next prompt
echo "$output"
```

**Output format:**

```
═══════════════════════════════════════════════════════════════
Your workflow: implement-tdd (step 2/4)
Work bead: gt-123 "Implement auth endpoint"
═══════════════════════════════════════════════════════════════

  ✓ load-context
  → write-tests [in_progress] ← YOU ARE HERE
  ○ implement
  ○ commit

───────────────────────────────────────────────────────────────
Current step: write-tests

Instructions:
  Write failing tests that define the expected behavior.
  Tests MUST fail at this point.

Required outputs: (none)
───────────────────────────────────────────────────────────────
```

### `bd ready` - Work Bead Selection

Shows ONLY work beads:

```go
func GetReadyWork(ctx context.Context, store Store) ([]*types.Issue, error) {
    return store.List(ctx, BeadFilter{
        Tier:   TierWork,
        Status: StatusOpen,
    })
}
```

### `meow status` - Workflow Overview

Shows both wisp progress AND orchestrator state (for debugging):

```
Workflow: work-loop (meow-abc)
Status: running

Agent: claude-2
  Wisp: implement-tdd (step 2/4)
  Current: write-tests [in_progress]

Orchestrator beads:
  ✓ start-worker [closed]
  ○ check-tests [open, waiting on: implement]
  ○ stop-worker [open, waiting on: check-tests]
```

---

## Crash Recovery

### Why All Beads Are Durable

Even ephemeral beads are stored in the database. This enables:

1. **State reconstruction** - `meow continue` can resume from any point
2. **Debugging** - Inspect exactly what happened before crash
3. **No external state** - Beads ARE the state

### The `meow continue` Command

```bash
# Resume a crashed/interrupted workflow
meow continue work-loop

# Or with explicit workflow ID
meow continue --workflow meow-abc
```

**Implementation:**

```go
func ContinueWorkflow(ctx context.Context, store Store, workflowID string) error {
    beads, _ := store.List(ctx, BeadFilter{
        WorkflowID: workflowID,
    })

    // Reconstruct state
    var completed, inProgress, pending []*types.Issue
    for _, bead := range beads {
        switch bead.Status {
        case StatusClosed:
            completed = append(completed, bead)
        case StatusInProgress:
            inProgress = append(inProgress, bead)
        case StatusOpen:
            pending = append(pending, bead)
        }
    }

    // Check if agents for in-progress beads are still alive
    for _, bead := range inProgress {
        if bead.Assignee != "" {
            alive, _ := agents.IsRunning(ctx, bead.Assignee)
            if !alive {
                bead.Status = StatusOpen  // Reset for re-dispatch
                store.Update(ctx, bead)
            }
        }
    }

    // Resume orchestrator loop
    return runOrchestratorLoop(ctx, store, workflowID)
}
```

### State Reconstruction Rules

1. **Closed beads** - Already done, skip
2. **In-progress beads** - Check agent liveness, reset if dead
3. **Open beads** - Wait for dependencies
4. **Outputs** - Preserved on closed beads, available for substitution

---

## Implementation Details

### Schema Changes (Beads Fork)

Add to the Issue struct:

```go
type Issue struct {
    // Existing fields...

    // Tier (explicit, not computed)
    Tier BeadTier `json:"tier"`

    // Workflow tracking
    HookBead       string `json:"hook_bead,omitempty"`
    SourceWorkflow string `json:"source_workflow,omitempty"`
    WorkflowID     string `json:"workflow_id,omitempty"`

    // Output capture
    Outputs map[string]any `json:"outputs,omitempty"`

    // Type-specific specs
    ConditionSpec *ConditionSpec `json:"condition_spec,omitempty"`
    StartSpec     *StartSpec     `json:"start_spec,omitempty"`
    StopSpec      *StopSpec      `json:"stop_spec,omitempty"`
    CodeSpec      *CodeSpec      `json:"code_spec,omitempty"`
    ExpandSpec    *ExpandSpec    `json:"expand_spec,omitempty"`
}
```

### Step Struct (Template Parser)

```go
type Step struct {
    // Identity
    ID          string `toml:"id"`
    Type        string `toml:"type"`
    Title       string `toml:"title"`
    Description string `toml:"description,omitempty"`

    // Task fields
    Instructions string `toml:"instructions,omitempty"`
    Assignee     string `toml:"assignee,omitempty"`

    // Dependencies
    Needs []string `toml:"needs,omitempty"`

    // Condition fields
    Condition string           `toml:"condition,omitempty"`
    Timeout   string           `toml:"timeout,omitempty"`
    OnTrue    *ExpansionTarget `toml:"on_true,omitempty"`
    OnFalse   *ExpansionTarget `toml:"on_false,omitempty"`

    // Code fields
    Code        string            `toml:"code,omitempty"`
    Workdir     string            `toml:"workdir,omitempty"`
    Env         map[string]string `toml:"env,omitempty"`
    CodeOutputs map[string]string `toml:"code_outputs,omitempty"`

    // Start fields
    Agent         string      `toml:"agent,omitempty"`
    Prompt        string      `toml:"prompt,omitempty"`
    ResumeSession string      `toml:"resume_session,omitempty"`
    AttachWisp    *AttachWisp `toml:"attach_wisp,omitempty"`

    // Stop fields
    Graceful bool `toml:"graceful,omitempty"`

    // Expand fields
    Template  string            `toml:"template,omitempty"`
    Variables map[string]string `toml:"variables,omitempty"`

    // Output definitions
    Outputs *OutputSpec `toml:"outputs,omitempty"`
}

type AttachWisp struct {
    Template  string            `toml:"template"`
    Variables map[string]string `toml:"variables,omitempty"`
}

type OutputSpec struct {
    Required []OutputDef `toml:"required,omitempty"`
    Optional []OutputDef `toml:"optional,omitempty"`
}

type OutputDef struct {
    Name        string `toml:"name"`
    Type        string `toml:"type"`
    Description string `toml:"description,omitempty"`
}
```

### Workflow Struct (Module Parser)

```go
type Module struct {
    Workflows map[string]*Workflow
}

type Workflow struct {
    Name        string           `toml:"name"`
    Description string           `toml:"description,omitempty"`
    Ephemeral   bool             `toml:"ephemeral,omitempty"`
    Internal    bool             `toml:"internal,omitempty"`
    HooksTo     string           `toml:"hooks_to,omitempty"`
    Variables   map[string]*Var  `toml:"variables,omitempty"`
    Steps       []*Step          `toml:"steps"`
}

type Var struct {
    Required    bool   `toml:"required,omitempty"`
    Default     any    `toml:"default,omitempty"`
    Type        string `toml:"type,omitempty"`
    Description string `toml:"description,omitempty"`
}
```

### Baker Changes

```go
func (b *Baker) Bake(module *Module, workflowName string, vars map[string]string) (*BakeResult, error) {
    workflow := module.Workflows[workflowName]
    if workflow == nil {
        return nil, fmt.Errorf("workflow %q not found", workflowName)
    }

    // Validate internal workflows aren't called externally
    if workflow.Internal && b.External {
        return nil, fmt.Errorf("workflow %q is internal", workflowName)
    }

    workflowID := b.generateWorkflowID(workflow)

    var beads []*types.Issue
    for _, step := range workflow.Steps {
        bead := b.stepToBead(step, workflow, vars, workflowID)
        beads = append(beads, bead)
    }

    return &BakeResult{
        Beads:      beads,
        WorkflowID: workflowID,
    }, nil
}

func (b *Baker) stepToBead(step *Step, workflow *Workflow, vars map[string]string, workflowID string) *types.Issue {
    bead := &types.Issue{
        ID:             b.generateBeadID(workflowID, step.ID),
        Title:          substitute(step.Title, vars),
        IssueType:      b.determineType(step),
        Tier:           b.determineTier(step, workflow),
        Status:         StatusOpen,
        SourceWorkflow: workflow.Name,
        WorkflowID:     workflowID,
        Assignee:       substitute(step.Assignee, vars),
    }

    // Set HookBead from hooks_to
    if workflow.HooksTo != "" {
        if hookID, ok := vars[workflow.HooksTo]; ok {
            bead.HookBead = hookID
        }
    }

    // Set type-specific specs...

    return bead
}

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

### Orchestrator Changes

```go
func (o *Orchestrator) MainLoop(ctx context.Context) error {
    for {
        bead, err := o.store.GetNextReady(ctx)
        if err != nil {
            return err
        }

        if bead == nil {
            if o.isWorkflowComplete(ctx) {
                o.cleanupWorkflow(ctx)
                return nil
            }
            time.Sleep(100 * time.Millisecond)
            continue
        }

        // Dispatch by type
        switch bead.IssueType {
        case TypeTask:
            // Wait for agent to close (auto-continues via stop hook)
            o.waitForAgentClose(ctx, bead)

        case TypeCollaborative:
            // Wait for agent to close (does NOT auto-continue - conversation mode)
            // The only difference from task is in meow prime's behavior
            o.waitForAgentClose(ctx, bead)

        case TypeGate:
            // Wait for human to close
            o.waitForGateClose(ctx, bead)

        case TypeCondition:
            go o.handleCondition(ctx, bead)

        case TypeExpand:
            o.handleExpand(ctx, bead)

        case TypeCode:
            o.handleCode(ctx, bead)

        case TypeStart:
            o.handleStart(ctx, bead)

        case TypeStop:
            o.handleStop(ctx, bead)
        }
    }
}
```

**Note**: The orchestrator treats `task` and `collaborative` identically - both wait for the agent to close. The difference is in `meow prime`: it returns empty output for in-progress collaborative steps, which prevents the stop hook from injecting the next prompt.

---

## Summary

This addendum introduces:

1. **Three-tier beads with explicit `Tier` field** - Work, wisp, and orchestrator beads distinguished by a direct field, not computed from labels

2. **Simplified status model** - Just `open`, `in_progress`, `closed` (no `hooked`)

3. **Module system** - Multi-workflow files with `main` convention, local references (`.workflow`), and explicit properties (`ephemeral`, `internal`, `hooks_to`)

4. **Explicit HookBead linking** - Via workflow-level `hooks_to` property, not magic variable name detection

5. **Gate type** - Replaces `orchestrator_task` flag for human approval points

6. **Collaborative type** - Enables human-in-the-loop conversation by pausing auto-continuation

7. **Beads fork** - MEOW maintains its own fork of beads with workflow-specific fields

8. **Crash recovery** - All beads durable, `meow continue` reconstructs state

**Type summary:**

| Type | Assignee | Auto-continue | Who closes | Use case |
|------|----------|---------------|------------|----------|
| `task` | Required | Yes | Agent | Normal autonomous work |
| `collaborative` | Required | No | Agent | Design review, clarification, debugging |
| `gate` | None | No | Human | Approval checkpoints |

The key insight: **All beads live in the same store for durability, with tier as an explicit field for fast filtering.** This gives us the best of both worlds - trivial crash recovery AND clean separation of concerns.
