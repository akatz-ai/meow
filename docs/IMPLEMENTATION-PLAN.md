# MEOW Stack MVP Implementation Plan

> **Version**: 1.0.0
> **Status**: Design Complete, Ready for Implementation
> **Last Updated**: 2026-01-07

This document serves as both architectural specification AND implementation task breakdown for the MEOW Stack MVP. It is designed to be self-contained—any future developer (human or AI) should be able to understand the full context, reasoning, and implementation path from this document alone.

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Vision & Goals](#vision--goals)
3. [Architecture Overview](#architecture-overview)
4. [The 8 Primitives](#the-8-primitives)
5. [Key Design Decisions](#key-design-decisions)
6. [Epic Breakdown](#epic-breakdown)
7. [Implementation Phases](#implementation-phases)
8. [Beads Task Structure](#beads-task-structure)

---

## Executive Summary

**MEOW Stack** (Molecular Expression Of Work) is a primitives-first workflow orchestration system for durable AI agent execution. It solves the fundamental problems of AI coding agents:

| Problem | MEOW Solution |
|---------|---------------|
| Context amnesia | Beads persist state; agents resume via `meow prime` |
| No durability | All state in git-backed beads; crash recovery automatic |
| Unstructured execution | Molecules provide step-by-step workflow discipline |
| Human oversight gaps | `condition` primitive enables blocking gates |
| Scaling chaos | Templates enable reproducible, composable workflows |

### The Core Insight

> **"The orchestrator is dumb; the templates are smart."**

Instead of building complex orchestrator logic for every workflow pattern (refresh, handoff, call, loops, gates), we provide **8 orthogonal primitives** that compose into arbitrary workflows via TOML templates. The orchestrator is a simple dispatch loop; all intelligence lives in user-controllable templates.

### MVP Scope

- Single-agent execution (parallelization deferred)
- All 8 primitive handlers
- Template system with variable substitution
- Basic default templates (work-loop, implement, human-gate)
- Integration with beads and Claude Code
- Crash recovery

### Out of Scope for MVP

- Multi-agent parallel execution
- `foreach` directive for dynamic parallelization
- Built-in context awareness (user defines via `condition`)
- Web dashboard
- Remote agent execution

---

## Vision & Goals

### Why MEOW Exists

Current AI agent orchestration approaches have fundamental limitations:

1. **Opinionated orchestrators**: Systems like Gas Town have built-in knowledge of agent types, lifecycle management, and workflow patterns. This limits flexibility—users can't easily define new patterns.

2. **Ephemeral state**: Most agent systems lose context on restart. Work must be re-explained, progress is lost.

3. **No durability**: If an agent crashes mid-task, there's often no way to resume cleanly.

4. **Magic thresholds**: Systems decide when to refresh context, when to handoff, when to gate. Users have little control.

MEOW addresses these by:

1. **Primitives over opinions**: 8 primitives compose into any workflow. New patterns are templates, not orchestrator changes.

2. **Beads as memory**: All state persists in git-backed beads. Any agent can resume where another left off.

3. **Crash-proof design**: Orchestrator state is checkpointed. Agents can be respawned. Nothing is lost.

4. **User control**: Context thresholds, gate conditions, handoff triggers—all user-defined via templates.

### The Gas Town Relationship

Gas Town is an existing multi-agent orchestrator with rich functionality. MEOW Stack is designed as a **lower-level foundation** that could:

1. **Replace Gas Town's orchestration layer** with a more flexible, primitives-based approach
2. **Enable Gas Town patterns as templates**: Mayor, Witness, Polecat lifecycles could be expressed as MEOW templates
3. **Provide the durability layer** that Gas Town uses underneath

For MVP, we focus on single-agent workflows. Multi-agent support (required for full Gas Town replacement) is Phase 2.

### Success Criteria

1. **Single-agent work loop**: `meow run work-loop` executes tasks until complete
2. **TDD workflow**: `implement` template guides Claude through test-first development
3. **Human gates**: `human-gate` template blocks until `meow approve` is run
4. **Crash recovery**: Kill orchestrator mid-workflow, restart, continue from last state
5. **Template composition**: Templates can `expand` other templates recursively

---

## Architecture Overview

### System Components

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              MEOW STACK                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────┐    ┌──────────────────┐    ┌──────────────────┐       │
│  │   meow CLI       │    │   Orchestrator   │    │   Template       │       │
│  │                  │    │                  │    │   System         │       │
│  │  - init          │    │  - Main loop     │    │                  │       │
│  │  - run           │───▶│  - Primitive     │◀───│  - TOML parser   │       │
│  │  - prime         │    │    handlers      │    │  - Variables     │       │
│  │  - close         │    │  - State mgmt    │    │  - Baking        │       │
│  │  - status        │    │  - Recovery      │    │  - Validation    │       │
│  │  - approve       │    │                  │    │                  │       │
│  └──────────────────┘    └────────┬─────────┘    └──────────────────┘       │
│                                   │                                          │
│                    ┌──────────────┼──────────────┐                          │
│                    │              │              │                          │
│                    ▼              ▼              ▼                          │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐          │
│  │  Agent Manager   │  │  Condition       │  │  Beads           │          │
│  │                  │  │  Evaluator       │  │  Integration     │          │
│  │  - tmux control  │  │                  │  │                  │          │
│  │  - spawn/stop    │  │  - Shell exec    │  │  - bd commands   │          │
│  │  - checkpoint    │  │  - Blocking      │  │  - State queries │          │
│  │  - resume        │  │  - Timeout       │  │  - Bead CRUD     │          │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────────────┐
│                              EXTERNAL                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐          │
│  │  .beads/         │  │  tmux            │  │  Claude Code     │          │
│  │                  │  │                  │  │                  │          │
│  │  - issues.jsonl  │  │  - Sessions      │  │  - --resume      │          │
│  │  - beads.db      │  │  - Panes         │  │  - Hooks         │          │
│  │  - bd CLI        │  │  - Send-keys     │  │  - Session IDs   │          │
│  └──────────────────┘  └──────────────────┘  └──────────────────┘          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### File Structure

```
project/
├── .meow/
│   ├── config.toml           # MEOW configuration
│   ├── agents.json           # Agent states (active, checkpointed, etc.)
│   ├── templates/            # User-defined templates
│   │   ├── work-loop.toml
│   │   ├── implement.toml
│   │   └── ...
│   └── state/
│       ├── orchestrator.json # Current orchestrator state
│       ├── orchestrator.pid  # Process ID for lock
│       ├── orchestrator.lock # Prevent concurrent instances
│       ├── heartbeat.json    # Health check
│       └── trace.jsonl       # Execution trace log
│
├── .beads/                   # Beads storage (existing)
│   ├── issues.jsonl
│   └── beads.db
│
└── .claude/                  # Claude Code config
    └── settings.json         # Hooks configuration
```

### Data Flow

```
1. User runs: meow run work-loop
       │
       ▼
2. Template System: Bakes work-loop.toml into beads
       │
       ▼
3. Orchestrator: Enters main loop
       │
       ├───────────────────────────────────────────────────────────────┐
       ▼                                                               │
4. getNextReadyBead()                                                  │
       │                                                               │
       ├── None + all done → Exit "Workflow complete"                  │
       │                                                               │
       ├── None + waiting → Sleep, retry                               │
       │                                                               │
       └── Found bead → Dispatch by type:                              │
               │                                                       │
               ├── task      → Wait for Claude to close               │
               │                     │                                 │
               │                     ├── Claude runs meow prime        │
               │                     ├── Claude executes work          │
               │                     └── Claude runs meow close        │
               │                                                       │
               ├── condition → Run shell command                       │
               │                     │                                 │
               │                     ├── Exit 0 → Expand on_true       │
               │                     └── Exit ≠0 → Expand on_false     │
               │                                                       │
               ├── expand    → Load template, create child beads       │
               │                                                       │
               ├── code      → Execute shell script                    │
               │                                                       │
               ├── stop      → Kill tmux session                       │
               │                                                       │
               ├── start     → Spawn tmux session with Claude          │
               │                                                       │
               ├── checkpoint → Save session ID                        │
               │                                                       │
               └── resume    → Start Claude with --resume              │
               │                                                       │
               └── Auto-close bead, loop ───────────────────────────────┘
```

---

## The 8 Primitives

### Overview Table

| # | Primitive | Executor | Blocking? | Description |
|---|-----------|----------|-----------|-------------|
| 1 | `task` | Claude | Yes (waits) | Work Claude executes and closes |
| 2 | `condition` | Orchestrator | Yes (shell) | Evaluate shell, expand on_true/on_false |
| 3 | `stop` | Orchestrator | No | Kill agent's tmux session |
| 4 | `start` | Orchestrator | No | Spawn agent in tmux |
| 5 | `checkpoint` | Orchestrator | No | Save session for later resume |
| 6 | `resume` | Orchestrator | No | Resume from checkpoint |
| 7 | `code` | Orchestrator | Yes (shell) | Execute arbitrary shell |
| 8 | `expand` | Orchestrator | No | Inline template expansion |

### Detailed Specifications

#### 1. `task` — Agent-Executed Work

**Purpose**: The default bead type. Claude receives it, executes the work, closes it.

**Bead Fields**:
```yaml
type: task
title: "Implement user registration"
description: "Create POST /api/register endpoint"
instructions: |
  1. Read existing auth module
  2. Create endpoint following existing patterns
  3. Add validation
  4. Write tests
assignee: "claude-1"  # Which agent handles this
```

**Orchestrator Behavior**:
1. Verify assigned agent is running (spawn if needed via error)
2. Wait for bead status to become `closed`
3. Detection via file watch on `.beads/issues.jsonl` or polling

**Agent Behavior**:
1. `meow prime` shows this task
2. Claude executes work following instructions
3. Claude runs `meow close <bead-id>` with optional notes

**Why This Design**: Tasks are the fundamental unit of work. The orchestrator is passive—it waits for Claude to close. This respects agent autonomy while maintaining workflow structure.

---

#### 2. `condition` — Branching, Looping, and Waiting

**Purpose**: Evaluate a shell command. Based on exit code, expand different templates.

**The Key Insight**: Shell commands can **block indefinitely**. This means:
- Human gates: `meow wait-approve --bead xyz` (blocks until human runs `meow approve`)
- CI waits: `gh run watch $RUN_ID --exit-status`
- Timers: `sleep 300 && exit 0`
- API polling: `curl ... | jq ... | grep ...`
- Context checks: `meow context-usage --threshold 70 --format exit-code`

The orchestrator doesn't care if a condition takes 1ms or 24 hours.

**Bead Fields**:
```yaml
type: condition
condition: "bd list --type=task --status=open | grep -q ."
on_true:
  template: "work-iteration"
  variables: { ... }
on_false:
  template: "finalize"
# OR inline:
on_true:
  inline:
    - { id: "next-step", type: "task", ... }
on_false:
  inline: []  # Empty = nothing
timeout: "1h"  # Optional
on_timeout:
  template: "handle-timeout"
```

**Orchestrator Behavior**:
1. Execute `condition` as shell command (runs in background goroutine to avoid blocking main loop)
2. Capture exit code, stdout, stderr
3. If exit 0: expand `on_true`
4. If exit ≠ 0: expand `on_false`
5. If timeout: expand `on_timeout` (or treat as on_false)
6. Auto-close this bead
7. Continue main loop (new beads are now ready)

**Why This Design**: Instead of separate primitives for gates, loops, waits, timers—we unify them. Any blocking operation is just a shell command. This keeps the orchestrator simple while enabling arbitrary control flow.

---

#### 3. `stop` — Kill Agent Session

**Purpose**: Terminate an agent's tmux session.

**Bead Fields**:
```yaml
type: stop
agent: "claude-1"
graceful: true  # Optional: SIGTERM first, wait, then SIGKILL
timeout: 10     # Seconds for graceful shutdown
```

**Orchestrator Behavior**:
1. If graceful: send interrupt (Ctrl-C), wait timeout, force kill
2. Kill tmux session `meow-{agent}`
3. Update agent state to `stopped`
4. Auto-close this bead

**Note**: Does NOT delete workspace/worktree. Use `code` bead for cleanup if needed.

**Why This Design**: Clean separation between session lifecycle and workspace lifecycle. Stopping an agent is about the Claude process, not about the work artifact.

---

#### 4. `start` — Spawn Agent

**Purpose**: Start an agent in a new tmux session.

**Bead Fields**:
```yaml
type: start
agent: "claude-1"
workdir: "/path/to/project"  # Optional
env:
  MEOW_AGENT: "claude-1"
  MEOW_ROLE: "worker"
prompt: "meow prime"  # Optional, default
```

**Orchestrator Behavior**:
1. Create tmux session: `tmux new-session -d -s meow-claude-1 -c {workdir}`
2. Set environment variables
3. Start Claude: `claude --dangerously-skip-permissions`
4. Wait for Claude ready
5. Inject prompt: `meow prime`
6. Update agent state to `active`
7. Auto-close this bead

**Why This Design**: tmux provides persistent sessions that survive SSH disconnects. The agent name is the identity; the session is ephemeral.

---

#### 5. `checkpoint` — Save for Resume

**Purpose**: Save an agent's Claude session checkpoint for later `resume`.

**Bead Fields**:
```yaml
type: checkpoint
agent: "claude-1"
label: "pre-child-spawn"  # Optional identifier
```

**Orchestrator Behavior**:
1. Discover session ID (from `$MEOW_SESSION_ID` or Claude's session file)
2. Store checkpoint in agent state:
   ```json
   {
     "checkpoint_id": "session_abc123",
     "label": "pre-child-spawn",
     "timestamp": "...",
     "bead_context": ["bd-001", "bd-002"]
   }
   ```
3. Auto-close this bead

**Note**: Does NOT stop the agent. Pair with `stop` if needed.

**Why This Design**: Checkpointing is separate from stopping. You might checkpoint as insurance without stopping, or checkpoint then stop for handoff.

---

#### 6. `resume` — Resume from Checkpoint

**Purpose**: Resume an agent from a saved checkpoint using `claude --resume`.

**Bead Fields**:
```yaml
type: resume
agent: "claude-1"
checkpoint: "pre-child-spawn"  # Optional: specific label, or latest
```

**Orchestrator Behavior**:
1. Retrieve checkpoint from agent state
2. Create tmux session
3. Start Claude with: `claude --resume {checkpoint_id}`
4. Wait for ready
5. Inject `meow prime`
6. Update agent state to `active`
7. Auto-close this bead

**Why This Design**: Claude Code's `--resume` flag enables session continuity. The agent wakes up with full prior context, sees `meow prime`, and continues work.

---

#### 7. `code` — Arbitrary Shell Execution

**Purpose**: Execute arbitrary shell code. The escape hatch for custom orchestrator actions.

**Bead Fields**:
```yaml
type: code
code: |
  git worktree add -b meow/claude-2 ~/worktrees/claude-2 HEAD
  echo "Worktree created"
workdir: "/path/to/repo"  # Optional
env:
  GIT_AUTHOR_NAME: "MEOW Bot"
on_error: "continue"  # continue | abort | retry
max_retries: 3
```

**Orchestrator Behavior**:
1. Execute `code` as shell script
2. Set workdir and environment
3. Capture stdout/stderr, log to trace
4. If exit ≠ 0:
   - `continue`: log error, continue (default)
   - `abort`: stop workflow
   - `retry`: retry up to max_retries
5. Auto-close this bead

**Why This Design**: Templates can't anticipate every need. `code` provides an escape hatch for git operations, environment setup, notifications, etc.

---

#### 8. `expand` — Template Composition

**Purpose**: Expand a template into beads at this point. The composition mechanism.

**Bead Fields**:
```yaml
type: expand
template: "implement-tdd"
variables:
  task_id: "bd-task-123"
  test_framework: "pytest"
assignee: "claude-1"  # Assign expanded beads to this agent
```

**Orchestrator Behavior**:
1. Load template from `.meow/templates/{template}.toml`
2. Validate required variables
3. Substitute all `{{variable}}` references
4. For each step in template:
   - Generate unique bead ID
   - Translate `needs` to actual bead IDs
   - Set assignee if specified
   - Create bead
5. Wire dependencies: expanded beads depend on this bead's position
6. Auto-close this bead

**Nesting**: Expanded templates can contain `expand` beads, enabling recursive composition.

**Why This Design**: This is how complex workflows are built from simple primitives. A `call` template is just: checkpoint → stop → start → expand → stop → resume. Templates compose templates.

---

## Key Design Decisions

### Decision 1: 8 Primitives, No More

**Context**: We could add convenience primitives like `gate`, `loop`, `call`, `handoff`.

**Decision**: Keep it to 8. Everything else is template composition.

**Reasoning**:
- Fewer primitives = simpler orchestrator = fewer bugs
- Templates are user-visible and modifiable
- New patterns don't require orchestrator changes
- Forces us to think in terms of composition

**Trade-off**: Users need to understand template composition for advanced patterns. We mitigate this with good default templates.

---

### Decision 2: Conditions Can Block

**Context**: Many systems have separate primitives for waits, gates, timers.

**Decision**: `condition` is just a shell command that can block arbitrarily.

**Reasoning**:
- Unified model: loops, gates, waits, timers are all conditions
- Shell is universal: any blocking operation can be wrapped
- No orchestrator complexity for different wait types
- Helper CLIs (`meow wait-approve`, etc.) provide convenience

**Trade-off**: Less type safety (it's all shell). Mitigated by clear documentation and helper commands.

---

### Decision 3: Orchestrator Handles Primitives, Claude Handles Tasks

**Context**: Who executes what?

**Decision**:
- Orchestrator handles 7 primitives directly (condition, stop, start, checkpoint, resume, code, expand)
- Claude handles `task` beads
- Orchestrator waits passively for Claude to close tasks

**Reasoning**:
- Clear separation of concerns
- Respects agent autonomy
- Tasks can be arbitrarily complex—Claude decides how to execute
- Primitives are mechanical—orchestrator can handle deterministically

---

### Decision 4: Templates Are TOML

**Context**: Could use YAML, JSON, or custom DSL.

**Decision**: TOML for templates.

**Reasoning**:
- Human-readable and writable
- Good support in Go
- Natural for configuration-like data
- Multiline strings work well for instructions
- Comments supported (unlike JSON)

---

### Decision 5: State in Beads, Not Separate Database

**Context**: Where does workflow state live?

**Decision**: Beads are the source of truth. Orchestrator state (`.meow/state/`) is cache/coordination only.

**Reasoning**:
- Beads already solve persistence, git-backing, crash recovery
- No second source of truth to synchronize
- Agents use `bd` commands they already know
- Human can inspect/modify state with familiar tools

---

### Decision 6: tmux for Agent Sessions

**Context**: How do we manage Claude processes?

**Decision**: tmux sessions.

**Reasoning**:
- Survives SSH disconnects
- Named sessions enable targeting
- Send-keys for injection
- Standard tool, well-understood
- Supports future multi-agent (multiple sessions)

**Trade-off**: Requires tmux installed. Acceptable for CLI-based workflows.

---

### Decision 7: No Built-in Context Awareness

**Context**: Should the orchestrator automatically refresh when context is high?

**Decision**: No. User defines context checks via `condition` beads.

**Reasoning**:
- Different projects have different tolerances
- "Context is high" is hard to measure accurately
- User control is a core principle
- Easy to add as template: `condition: "meow context-usage --threshold 70 --format exit-code"`

**Trade-off**: User must remember to include context checks. Mitigated by including in default work-loop template.

---

### Decision 8: Single Agent for MVP

**Context**: Should MVP support multi-agent parallelization?

**Decision**: No. Single agent execution only.

**Reasoning**:
- Simpler to implement and debug
- Validates core primitives before adding complexity
- Most workflows can be single-agent
- Architecture supports multi-agent (beads can have assignee)

**Future**: Add `foreach` directive, parallel `start` handling, join detection in Phase 2.

---

## Epic Breakdown

The implementation is organized into 11 epics, each representing a coherent capability area.

### Epic Overview

| # | Epic | Description | Dependencies |
|---|------|-------------|--------------|
| 1 | Foundation | Project setup, core types, CLI skeleton | None |
| 2 | Template System | Parser, variables, validation | E1 |
| 3 | Orchestrator Core | Main loop, readiness, state | E1, E2 |
| 4 | Primitive Handlers | All 8 primitive implementations | E3 |
| 5 | Agent Management | tmux, spawn, stop, lifecycle | E3 |
| 6 | Condition System | Shell execution, blocking, gates | E4 |
| 7 | Checkpoint/Resume | Session persistence | E4, E5 |
| 8 | Integration | Beads, hooks, recovery | E3, E4 |
| 9 | Default Templates | MVP template library | E2 |
| 10 | CLI Completion | All commands | E3, E4, E5 |
| 11 | Testing & Docs | Integration tests, documentation | All |

### Dependency Graph

```
E1 (Foundation)
 │
 ├──▶ E2 (Templates)
 │     │
 │     └──▶ E9 (Default Templates)
 │
 └──▶ E3 (Orchestrator Core)
       │
       ├──▶ E4 (Primitives)
       │     │
       │     ├──▶ E6 (Conditions)
       │     │
       │     └──▶ E7 (Checkpoint/Resume)
       │
       ├──▶ E5 (Agent Management)
       │     │
       │     └──▶ E7 (Checkpoint/Resume)
       │
       └──▶ E8 (Integration)
             │
             └──▶ E10 (CLI Completion)
                   │
                   └──▶ E11 (Testing & Docs)
```

---

## Implementation Phases

### Phase 1: Core Infrastructure (E1, E2, E3)
**Goal**: Runnable orchestrator that can bake templates and loop on beads.

- Project structure, Go modules, build system
- Core types (Bead, Agent, Template)
- TOML template parser with variable substitution
- Orchestrator main loop (without full primitive handlers)
- State persistence

**Milestone**: `meow run simple-template` starts and loops (even if primitives are stubs).

### Phase 2: Essential Primitives (E4, E5)
**Goal**: Task and control flow primitives working.

- `task` handler (wait for Claude to close)
- `condition` handler (shell + expansion)
- `expand` handler (template composition)
- `code` handler (shell execution)
- `start` and `stop` handlers (tmux)

**Milestone**: `meow run work-loop` with basic tasks executes end-to-end.

### Phase 3: Advanced Primitives & Integration (E6, E7, E8)
**Goal**: Full primitive set and external integration.

- Blocking condition support (human gates)
- `checkpoint` and `resume` handlers
- Beads integration (bd commands)
- Claude Code hooks
- Crash recovery

**Milestone**: Human gate works. Crash mid-workflow, restart, continue.

### Phase 4: Templates & Polish (E9, E10, E11)
**Goal**: Usable out-of-box with good templates.

- Default templates (work-loop, implement, human-gate, refresh, handoff)
- All CLI commands complete
- Integration tests
- Documentation

**Milestone**: New project can `meow init && meow run work-loop` with good experience.

---

## Beads Task Structure

Below is the complete task breakdown in beads-importable format. Each task includes:
- **Title**: Short description
- **Type**: epic | task
- **Priority**: 0 (critical) to 3 (low)
- **Notes**: Full context, reasoning, acceptance criteria

This can be imported via `bd import` or used as implementation guide.

---

# =============================================================================
# EPIC 1: FOUNDATION
# =============================================================================

```yaml
# -----------------------------------------------------------------------------
# EPIC: Foundation
# -----------------------------------------------------------------------------
# PURPOSE:
# Establish the project structure, core types, build system, and CLI skeleton.
# This is the foundation everything else builds on.
#
# CONTEXT:
# MEOW Stack is a Go project. We use standard Go project layout, Cobra for CLI,
# and BurntSushi/toml for template parsing. The foundation must be solid because
# everything depends on it.
#
# SUCCESS CRITERIA:
# - `go build` produces working `meow` binary
# - `meow --version` and `meow --help` work
# - Core types are defined and usable
# - Basic project structure matches architecture doc
# -----------------------------------------------------------------------------

- id: meow-e1
  type: epic
  title: "Foundation: Project setup and core infrastructure"
  priority: 0
  labels: [epic, foundation, phase-1]
  notes: |
    ## Overview
    Establish the foundational infrastructure for MEOW Stack.

    ## Deliverables
    1. Go project structure with modules
    2. Core types (Bead, Agent, Template, Config)
    3. CLI skeleton using Cobra
    4. Build system (Makefile)
    5. Basic configuration handling

    ## Why This Matters
    A clean foundation prevents technical debt. Every component depends on
    these core types and patterns. Getting this right saves refactoring later.

# -----------------------------------------------------------------------------
# TASK 1.1: Project Structure
# -----------------------------------------------------------------------------
- id: meow-e1.1
  type: task
  title: "Initialize Go project with module structure"
  priority: 0
  parent: meow-e1
  labels: [foundation, setup]
  notes: |
    ## Task
    Create the Go project structure for MEOW Stack.

    ## Directory Structure
    ```
    meow-machine/
    ├── cmd/
    │   └── meow/
    │       └── main.go           # CLI entry point
    ├── internal/
    │   ├── config/               # Configuration handling
    │   ├── types/                # Core data types
    │   ├── orchestrator/         # Orchestrator engine
    │   ├── template/             # Template system
    │   ├── agent/                # Agent management
    │   └── primitive/            # Primitive handlers
    ├── pkg/
    │   └── meow/                 # Public API (if needed)
    ├── templates/                # Default templates (embedded)
    ├── go.mod
    ├── go.sum
    ├── Makefile
    └── README.md
    ```

    ## Commands
    ```bash
    go mod init github.com/your-org/meow-machine
    mkdir -p cmd/meow internal/{config,types,orchestrator,template,agent,primitive} pkg/meow templates
    ```

    ## Acceptance Criteria
    - [ ] `go mod init` creates valid go.mod
    - [ ] Directory structure matches plan
    - [ ] `.gitignore` includes Go artifacts
    - [ ] Basic `main.go` compiles

# -----------------------------------------------------------------------------
# TASK 1.2: Core Types
# -----------------------------------------------------------------------------
- id: meow-e1.2
  type: task
  title: "Define core data types (Bead, Agent, Template, Config)"
  priority: 0
  parent: meow-e1
  needs: [meow-e1.1]
  labels: [foundation, types]
  notes: |
    ## Task
    Define the fundamental data structures used throughout MEOW.

    ## Types to Define

    ### Bead (internal/types/bead.go)
    ```go
    type BeadType string
    const (
        BeadTask       BeadType = "task"
        BeadCondition  BeadType = "condition"
        BeadStop       BeadType = "stop"
        BeadStart      BeadType = "start"
        BeadCheckpoint BeadType = "checkpoint"
        BeadResume     BeadType = "resume"
        BeadCode       BeadType = "code"
        BeadExpand     BeadType = "expand"
    )

    type BeadStatus string
    const (
        StatusOpen       BeadStatus = "open"
        StatusInProgress BeadStatus = "in_progress"
        StatusClosed     BeadStatus = "closed"
    )

    type Bead struct {
        ID          string            `json:"id"`
        Type        BeadType          `json:"type"`
        Title       string            `json:"title"`
        Description string            `json:"description,omitempty"`
        Instructions string           `json:"instructions,omitempty"`
        Status      BeadStatus        `json:"status"`
        Assignee    string            `json:"assignee,omitempty"`
        Needs       []string          `json:"needs,omitempty"`
        Labels      []string          `json:"labels,omitempty"`
        Notes       string            `json:"notes,omitempty"`
        Meta        map[string]any    `json:"meta,omitempty"`
        CreatedAt   time.Time         `json:"created_at"`
        ClosedAt    *time.Time        `json:"closed_at,omitempty"`

        // Primitive-specific fields (populated based on Type)
        Condition   *ConditionSpec    `json:"condition_spec,omitempty"`
        AgentSpec   *AgentSpec        `json:"agent_spec,omitempty"`
        CodeSpec    *CodeSpec         `json:"code_spec,omitempty"`
        ExpandSpec  *ExpandSpec       `json:"expand_spec,omitempty"`
    }
    ```

    ### Agent State (internal/types/agent.go)
    ```go
    type AgentStatus string
    const (
        AgentActive       AgentStatus = "active"
        AgentStopped      AgentStatus = "stopped"
        AgentCheckpointed AgentStatus = "checkpointed"
    )

    type Agent struct {
        ID           string      `json:"id"`
        TmuxSession  string      `json:"tmux_session"`
        Status       AgentStatus `json:"status"`
        Workdir      string      `json:"workdir"`
        CurrentBead  string      `json:"current_bead,omitempty"`
        Checkpoint   *Checkpoint `json:"checkpoint,omitempty"`
        StartedAt    time.Time   `json:"started_at"`
        LastActivity time.Time   `json:"last_activity"`
    }

    type Checkpoint struct {
        SessionID   string    `json:"session_id"`
        Label       string    `json:"label,omitempty"`
        Timestamp   time.Time `json:"timestamp"`
        BeadContext []string  `json:"bead_context,omitempty"`
    }
    ```

    ### Config (internal/config/config.go)
    ```go
    type Config struct {
        Version     string            `toml:"version"`
        ProjectName string            `toml:"project_name"`
        TemplateDir string            `toml:"template_dir"`
        BeadsDir    string            `toml:"beads_dir"`
        Defaults    DefaultsConfig    `toml:"defaults"`
    }

    type DefaultsConfig struct {
        Agent           string `toml:"agent"`
        StopGracePeriod int    `toml:"stop_grace_period"`
    }
    ```

    ## Design Decisions

    ### Why separate spec structs?
    Each primitive has different fields. Using embedded structs keeps the
    main Bead type clean while allowing type-safe access to primitive-specific
    data. Only the relevant spec is populated based on Type.

    ### Why time.Time for timestamps?
    Go's time.Time handles timezones and serialization well. JSON marshaling
    uses RFC3339 format, compatible with beads.

    ## Acceptance Criteria
    - [ ] All types compile without errors
    - [ ] JSON marshaling/unmarshaling works correctly
    - [ ] Types have appropriate comments/godoc
    - [ ] Unit tests for marshaling edge cases

# -----------------------------------------------------------------------------
# TASK 1.3: CLI Skeleton
# -----------------------------------------------------------------------------
- id: meow-e1.3
  type: task
  title: "Create CLI skeleton with Cobra"
  priority: 0
  parent: meow-e1
  needs: [meow-e1.1]
  labels: [foundation, cli]
  notes: |
    ## Task
    Set up the CLI framework using Cobra. Create stub commands for all planned
    functionality.

    ## Commands to Stub

    ### Root Command
    ```go
    var rootCmd = &cobra.Command{
        Use:   "meow",
        Short: "MEOW Stack - Molecular Expression Of Work",
        Long:  `Durable, composable workflow orchestration for AI agents.`,
    }
    ```

    ### User Commands
    - `meow init` - Initialize .meow directory
    - `meow run <template>` - Start workflow
    - `meow status` - Show current state
    - `meow approve [bead]` - Approve a gate
    - `meow reject [bead]` - Reject a gate
    - `meow pause` - Pause orchestrator
    - `meow resume` - Resume orchestrator (different from resume primitive!)
    - `meow stop` - Stop orchestrator

    ### Agent Commands
    - `meow prime` - Show next task for agent
    - `meow close <bead>` - Close a task
    - `meow handoff` - Request context refresh
    - `meow context-usage` - Report context usage

    ### Debug Commands
    - `meow agents` - List agents
    - `meow beads` - List workflow beads
    - `meow trace` - Show execution trace
    - `meow peek <agent>` - Show agent output
    - `meow attach <agent>` - Attach to agent session
    - `meow validate <template>` - Validate template

    ## Implementation

    Each command gets its own file in `cmd/meow/`:
    ```
    cmd/meow/
    ├── main.go
    ├── root.go
    ├── init.go
    ├── run.go
    ├── status.go
    ├── approve.go
    ├── reject.go
    ├── prime.go
    ├── close.go
    └── ...
    ```

    For MVP, commands can print "Not implemented" but must parse arguments
    correctly.

    ## Acceptance Criteria
    - [ ] `meow --help` shows all commands
    - [ ] `meow <cmd> --help` shows command-specific help
    - [ ] `meow --version` shows version
    - [ ] Commands parse arguments correctly (even if stub)
    - [ ] Consistent flag naming across commands

# -----------------------------------------------------------------------------
# TASK 1.4: Build System
# -----------------------------------------------------------------------------
- id: meow-e1.4
  type: task
  title: "Create Makefile and build scripts"
  priority: 1
  parent: meow-e1
  needs: [meow-e1.1]
  labels: [foundation, build]
  notes: |
    ## Task
    Create build automation for the project.

    ## Makefile Targets

    ```makefile
    .PHONY: build clean test lint install

    VERSION ?= $(shell git describe --tags --always --dirty)
    LDFLAGS := -ldflags "-X main.version=$(VERSION)"

    build:
        go build $(LDFLAGS) -o bin/meow ./cmd/meow

    install:
        go install $(LDFLAGS) ./cmd/meow

    test:
        go test -v ./...

    test-short:
        go test -short ./...

    lint:
        golangci-lint run ./...

    clean:
        rm -rf bin/

    # Embed default templates
    generate:
        go generate ./...
    ```

    ## Version Embedding

    ```go
    // cmd/meow/version.go
    var version = "dev"

    var versionCmd = &cobra.Command{
        Use:   "version",
        Short: "Print version",
        Run: func(cmd *cobra.Command, args []string) {
            fmt.Printf("meow version %s\n", version)
        },
    }
    ```

    ## Acceptance Criteria
    - [ ] `make build` produces binary
    - [ ] `make test` runs tests
    - [ ] `make lint` runs linter (with golangci-lint)
    - [ ] `make install` installs to GOPATH/bin
    - [ ] Version is embedded correctly

# -----------------------------------------------------------------------------
# TASK 1.5: Configuration System
# -----------------------------------------------------------------------------
- id: meow-e1.5
  type: task
  title: "Implement configuration loading and defaults"
  priority: 1
  parent: meow-e1
  needs: [meow-e1.2]
  labels: [foundation, config]
  notes: |
    ## Task
    Implement configuration file loading and sensible defaults.

    ## Config File Location

    Priority order:
    1. `.meow/config.toml` (project-local)
    2. `~/.config/meow/config.toml` (user global)
    3. Built-in defaults

    ## Default Configuration

    ```toml
    # .meow/config.toml
    version = "1"
    project_name = ""  # Auto-detected from directory

    [paths]
    template_dir = ".meow/templates"
    beads_dir = ".beads"
    state_dir = ".meow/state"

    [defaults]
    agent = "claude-1"
    stop_grace_period = 10  # seconds
    condition_timeout = "1h"

    [orchestrator]
    poll_interval = "100ms"
    heartbeat_interval = "30s"
    stuck_threshold = "15m"

    [hooks]
    session_start = "meow prime --hook"
    stop = "meow prime --format prompt"
    ```

    ## Implementation

    ```go
    // internal/config/config.go

    func Load() (*Config, error) {
        cfg := DefaultConfig()

        // Try project-local first
        if data, err := os.ReadFile(".meow/config.toml"); err == nil {
            if err := toml.Unmarshal(data, cfg); err != nil {
                return nil, fmt.Errorf("parse .meow/config.toml: %w", err)
            }
        }

        // Then user global
        home, _ := os.UserHomeDir()
        globalPath := filepath.Join(home, ".config", "meow", "config.toml")
        if data, err := os.ReadFile(globalPath); err == nil {
            // Merge, don't overwrite
            if err := toml.Unmarshal(data, cfg); err != nil {
                return nil, fmt.Errorf("parse global config: %w", err)
            }
        }

        return cfg, nil
    }
    ```

    ## Acceptance Criteria
    - [ ] Default config works with no file present
    - [ ] Project-local config overrides defaults
    - [ ] Unknown fields are ignored (forward compatibility)
    - [ ] Invalid config produces helpful error message
```

# =============================================================================
# EPIC 2: TEMPLATE SYSTEM
# =============================================================================

```yaml
# -----------------------------------------------------------------------------
# EPIC: Template System
# -----------------------------------------------------------------------------
# PURPOSE:
# Parse TOML templates, substitute variables, validate structure, and "bake"
# templates into beads. This is where workflow definitions become executable.
#
# CONTEXT:
# Templates are the "programs" in MEOW. They define workflows using the 8
# primitives. The orchestrator is dumb; templates are smart. This epic makes
# templates usable.
#
# SUCCESS CRITERIA:
# - Can parse any valid MEOW template
# - Variable substitution works with nesting
# - Validation catches common errors
# - Baking creates correct beads with dependencies
# -----------------------------------------------------------------------------

- id: meow-e2
  type: epic
  title: "Template System: Parser, variables, and baking"
  priority: 0
  labels: [epic, templates, phase-1]
  needs: [meow-e1]
  notes: |
    ## Overview
    The template system transforms TOML workflow definitions into executable
    beads.

    ## Key Components
    1. TOML parser with template-specific handling
    2. Variable substitution engine
    3. Template validation
    4. Baking (template → beads)

    ## Design Philosophy
    Templates should be:
    - Human-readable and writable
    - Composable (templates can include templates)
    - Validated at parse time, not runtime
    - Version-controlled alongside code

# -----------------------------------------------------------------------------
# TASK 2.1: Template Parser
# -----------------------------------------------------------------------------
- id: meow-e2.1
  type: task
  title: "Implement TOML template parser"
  priority: 0
  parent: meow-e2
  needs: [meow-e1.2]
  labels: [templates, parser]
  notes: |
    ## Task
    Parse MEOW template files from TOML format.

    ## Template Structure

    ```toml
    [meta]
    name = "implement"
    version = "1.0.0"
    description = "TDD implementation workflow"
    author = "meow-stack"

    # Optional metadata
    fits_in_context = true
    estimated_minutes = 30
    on_error = "inject-gate"
    max_retries = 2

    [variables]
    task_id = { required = true, description = "Task bead ID" }
    test_framework = { required = false, default = "auto" }
    skip_tests = { required = false, default = false, type = "bool" }

    [[steps]]
    id = "load-context"
    type = "task"  # Default if not specified
    title = "Load context"
    description = "..."
    instructions = """
    Multiline instructions here.
    Can reference {{task_id}}.
    """

    [[steps]]
    id = "write-tests"
    type = "task"
    title = "Write tests"
    needs = ["load-context"]
    condition = "not {{skip_tests}}"  # Skip if skip_tests is true
    instructions = "..."

    [[steps]]
    id = "check-something"
    type = "condition"
    condition = "test -f somefile"
    on_true:
      template = "handle-exists"
    on_false:
      inline = [
        { id = "create-file", type = "code", code = "touch somefile" }
      ]
    ```

    ## Go Types

    ```go
    type Template struct {
        Meta      TemplateMeta             `toml:"meta"`
        Variables map[string]VariableDef   `toml:"variables"`
        Steps     []TemplateStep           `toml:"steps"`
    }

    type TemplateMeta struct {
        Name             string `toml:"name"`
        Version          string `toml:"version"`
        Description      string `toml:"description"`
        Author           string `toml:"author"`
        FitsInContext    bool   `toml:"fits_in_context"`
        EstimatedMinutes int    `toml:"estimated_minutes"`
        OnError          string `toml:"on_error"`
        MaxRetries       int    `toml:"max_retries"`
    }

    type VariableDef struct {
        Required    bool   `toml:"required"`
        Default     any    `toml:"default"`
        Type        string `toml:"type"`  // string, bool, int, array
        Description string `toml:"description"`
        Enum        []any  `toml:"enum"`
    }

    type TemplateStep struct {
        ID           string                 `toml:"id"`
        Type         string                 `toml:"type"`
        Title        string                 `toml:"title"`
        Description  string                 `toml:"description"`
        Instructions string                 `toml:"instructions"`
        Needs        []string               `toml:"needs"`
        Condition    string                 `toml:"condition"`
        Labels       []string               `toml:"labels"`

        // Primitive-specific
        ConditionCmd string                 `toml:"condition"`  // for type=condition
        OnTrue       *ExpansionTarget       `toml:"on_true"`
        OnFalse      *ExpansionTarget       `toml:"on_false"`
        Code         string                 `toml:"code"`       // for type=code
        Agent        string                 `toml:"agent"`      // for start/stop
        Template     string                 `toml:"template"`   // for expand
        Variables    map[string]any         `toml:"variables"`
    }

    type ExpansionTarget struct {
        Template  string         `toml:"template"`
        Variables map[string]any `toml:"variables"`
        Inline    []TemplateStep `toml:"inline"`
    }
    ```

    ## Acceptance Criteria
    - [ ] Parse valid template without error
    - [ ] Return helpful error for invalid TOML
    - [ ] Handle all step types correctly
    - [ ] Support inline and template expansion targets
    - [ ] Parse variable definitions with all options

# -----------------------------------------------------------------------------
# TASK 2.2: Variable Substitution
# -----------------------------------------------------------------------------
- id: meow-e2.2
  type: task
  title: "Implement variable substitution engine"
  priority: 0
  parent: meow-e2
  needs: [meow-e2.1]
  labels: [templates, variables]
  notes: |
    ## Task
    Implement {{variable}} substitution throughout templates.

    ## Substitution Rules

    ### Basic Substitution
    ```
    {{task_id}} → bd-abc123
    {{test_framework}} → pytest
    ```

    ### Nested Object Access
    ```
    {{output.analyze-pick.selected_tasks}} → [bd-001, bd-002]
    ```

    ### Built-in Variables
    ```
    {{agent}} → Current agent ID
    {{bead_id}} → Current bead ID
    {{workdir}} → Current working directory
    {{timestamp}} → ISO timestamp
    {{parent_bead}} → Parent bead ID (if expanded)
    {{expand_depth}} → Nesting depth
    {{workflow_id}} → Root workflow ID
    ```

    ### Conditional Expressions (simple)
    ```
    {{#if skip_tests}}skipped{{/if}}
    not {{skip_tests}} → evaluates to bool
    ```

    ## Implementation

    ```go
    type Substitutor struct {
        Variables map[string]any
        Builtins  map[string]func() string
    }

    func (s *Substitutor) Substitute(input string) (string, error) {
        // Regex: \{\{([^}]+)\}\}
        // For each match:
        //   1. Check builtins
        //   2. Check variables (with dot notation)
        //   3. Error if not found and no default
    }

    func (s *Substitutor) SubstituteAll(tmpl *Template) (*Template, error) {
        // Deep clone template
        // Substitute in all string fields
        // Handle nested structures
    }
    ```

    ## Edge Cases

    - Unset required variable → error
    - Unset optional variable with default → use default
    - Nested access on non-object → error
    - Recursive substitution → detect and error

    ## Acceptance Criteria
    - [ ] Basic {{var}} substitution works
    - [ ] Nested {{a.b.c}} access works
    - [ ] Built-in variables populated correctly
    - [ ] Missing required variable produces clear error
    - [ ] Default values used when variable not provided
    - [ ] Type coercion (string to bool for conditions)

# -----------------------------------------------------------------------------
# TASK 2.3: Template Validation
# -----------------------------------------------------------------------------
- id: meow-e2.3
  type: task
  title: "Implement template validation"
  priority: 1
  parent: meow-e2
  needs: [meow-e2.1]
  labels: [templates, validation]
  notes: |
    ## Task
    Validate templates for correctness before baking.

    ## Validations

    ### Structure Validation
    - [ ] `meta.name` is required
    - [ ] Each step has unique `id`
    - [ ] Step `type` is one of 8 primitives (or "task" default)
    - [ ] Required fields present for each primitive type

    ### Dependency Validation
    - [ ] `needs` references exist as step IDs
    - [ ] No circular dependencies
    - [ ] All steps are reachable from entry points

    ### Variable Validation
    - [ ] All `{{var}}` references are defined in variables or builtins
    - [ ] Required variables have no defaults
    - [ ] Enum values match defined options

    ### Type-Specific Validation

    **condition**:
    - Must have `condition` command
    - Must have `on_true` and/or `on_false`

    **expand**:
    - Must have `template` reference
    - Template must exist (warning if not found)

    **start/stop**:
    - Must have `agent` field

    **code**:
    - Must have `code` field

    ## Error Messages

    ```
    template "implement" validation failed:
      - step "write-tests": needs "load-contxt" not found (did you mean "load-context"?)
      - step "check-ci": missing required field "on_true" for condition type
      - variable "{{taks_id}}": undefined (did you mean "task_id"?)
    ```

    ## Implementation

    ```go
    type ValidationError struct {
        Template string
        Step     string // empty if template-level
        Field    string
        Message  string
        Suggestion string // "did you mean..."
    }

    func Validate(tmpl *Template) ([]ValidationError, error)
    ```

    ## Acceptance Criteria
    - [ ] Catches missing required fields
    - [ ] Detects circular dependencies
    - [ ] Suggests corrections for typos
    - [ ] Returns all errors, not just first
    - [ ] Clear, actionable error messages

# -----------------------------------------------------------------------------
# TASK 2.4: Template Baking
# -----------------------------------------------------------------------------
- id: meow-e2.4
  type: task
  title: "Implement template baking (template → beads)"
  priority: 0
  parent: meow-e2
  needs: [meow-e2.2, meow-e2.3]
  labels: [templates, baking]
  notes: |
    ## Task
    Transform a parsed, validated template into executable beads.

    ## Baking Process

    1. **Load template** from file or embedded
    2. **Validate** structure and dependencies
    3. **Substitute variables** with provided values
    4. **Generate bead IDs** for each step
    5. **Translate dependencies** from step IDs to bead IDs
    6. **Create beads** via bd commands or direct JSONL append

    ## ID Generation

    Bead IDs must be unique and traceable:
    ```
    {workflow_id}.{step_id}-{hash}

    Examples:
    meow-run-001.load-context-a3f8
    meow-run-001.write-tests-b2c9
    ```

    ## Dependency Translation

    Template step:
    ```toml
    [[steps]]
    id = "write-tests"
    needs = ["load-context"]
    ```

    Becomes bead:
    ```json
    {
      "id": "meow-run-001.write-tests-b2c9",
      "needs": ["meow-run-001.load-context-a3f8"]
    }
    ```

    ## Handling `expand` Steps

    When baking encounters an `expand` step:
    1. Create the expand bead itself
    2. The orchestrator handles actual expansion at runtime
    3. This enables dynamic template selection ({{output.step.template}})

    ## Handling `condition` Steps

    `on_true` and `on_false` can be:
    - `template`: reference to external template (runtime expansion)
    - `inline`: array of steps (baked immediately as child beads)

    For inline, create child beads with parent reference.

    ## Implementation

    ```go
    type BakeResult struct {
        WorkflowID string
        Beads      []*Bead
        EntryPoint string  // First bead ID
    }

    func Bake(tmpl *Template, vars map[string]any) (*BakeResult, error) {
        // 1. Validate
        if errs, err := Validate(tmpl); len(errs) > 0 {
            return nil, &ValidationErrors{errs}
        }

        // 2. Generate workflow ID
        workflowID := generateWorkflowID()

        // 3. Substitute variables
        sub := NewSubstitutor(vars, workflowID)
        tmpl, err := sub.SubstituteAll(tmpl)

        // 4. Create beads for each step
        beadIDMap := make(map[string]string)
        var beads []*Bead

        for _, step := range tmpl.Steps {
            beadID := generateBeadID(workflowID, step.ID)
            beadIDMap[step.ID] = beadID

            bead := stepToBead(step, beadID)
            beads = append(beads, bead)
        }

        // 5. Translate dependencies
        for _, bead := range beads {
            bead.Needs = translateNeeds(bead.Needs, beadIDMap)
        }

        // 6. Handle inline expansions
        beads = expandInlineSteps(beads, beadIDMap)

        return &BakeResult{workflowID, beads, beads[0].ID}, nil
    }
    ```

    ## Acceptance Criteria
    - [ ] Simple template bakes to correct beads
    - [ ] Dependencies translate correctly
    - [ ] Inline steps become child beads
    - [ ] Bead IDs are unique and traceable
    - [ ] Variables fully substituted
    - [ ] Round-trip test: template → beads → execute → verify

# -----------------------------------------------------------------------------
# TASK 2.5: Template Loading
# -----------------------------------------------------------------------------
- id: meow-e2.5
  type: task
  title: "Implement template loading from filesystem and embedded"
  priority: 1
  parent: meow-e2
  needs: [meow-e2.1]
  labels: [templates, loading]
  notes: |
    ## Task
    Load templates from multiple sources with precedence.

    ## Load Order

    1. **Project templates**: `.meow/templates/{name}.toml`
    2. **User templates**: `~/.config/meow/templates/{name}.toml`
    3. **Embedded defaults**: Built into binary

    Project overrides user overrides embedded.

    ## Embedded Templates

    Use Go embed for default templates:

    ```go
    //go:embed templates/*.toml
    var embeddedTemplates embed.FS

    func LoadEmbedded(name string) (*Template, error) {
        data, err := embeddedTemplates.ReadFile("templates/" + name + ".toml")
        if err != nil {
            return nil, fmt.Errorf("no embedded template %q", name)
        }
        return Parse(data)
    }
    ```

    ## Template Resolution

    ```go
    func (l *Loader) Load(name string) (*Template, error) {
        // 1. Try project
        path := filepath.Join(".meow", "templates", name+".toml")
        if data, err := os.ReadFile(path); err == nil {
            return Parse(data)
        }

        // 2. Try user
        home, _ := os.UserHomeDir()
        path = filepath.Join(home, ".config", "meow", "templates", name+".toml")
        if data, err := os.ReadFile(path); err == nil {
            return Parse(data)
        }

        // 3. Try embedded
        return LoadEmbedded(name)
    }
    ```

    ## Acceptance Criteria
    - [ ] Embedded templates load correctly
    - [ ] Project templates override embedded
    - [ ] User templates work
    - [ ] Helpful error when template not found
    - [ ] `meow validate` can validate any loadable template
```

# =============================================================================
# EPIC 3: ORCHESTRATOR CORE
# =============================================================================

```yaml
# -----------------------------------------------------------------------------
# EPIC: Orchestrator Core
# -----------------------------------------------------------------------------
# PURPOSE:
# The heart of MEOW. Main loop, bead readiness detection, state management,
# and recovery. This is deliberately simple—complexity lives in templates.
#
# CONTEXT:
# The orchestrator is a dispatch loop:
#   1. Find next ready bead
#   2. Check type
#   3. Dispatch to handler
#   4. Repeat
#
# It doesn't know about workflows, context, refresh, gates—just 8 primitives.
#
# SUCCESS CRITERIA:
# - Main loop runs continuously
# - Correctly identifies ready beads
# - Dispatches to appropriate handlers
# - Persists state for recovery
# - Handles concurrent conditions
# -----------------------------------------------------------------------------

- id: meow-e3
  type: epic
  title: "Orchestrator Core: Main loop and state management"
  priority: 0
  labels: [epic, orchestrator, phase-1]
  needs: [meow-e1, meow-e2]
  notes: |
    ## Overview
    The orchestrator is intentionally simple. It:
    1. Finds ready beads
    2. Dispatches by type
    3. Manages agent state
    4. Persists for recovery

    ## Design Philosophy
    > "The orchestrator is dumb; the templates are smart."

    The orchestrator knows ONLY the 8 primitives. Everything else—loops,
    gates, refresh, call semantics—is template composition.

# -----------------------------------------------------------------------------
# TASK 3.1: Main Loop
# -----------------------------------------------------------------------------
- id: meow-e3.1
  type: task
  title: "Implement orchestrator main loop"
  priority: 0
  parent: meow-e3
  needs: [meow-e1.2]
  labels: [orchestrator, main-loop]
  notes: |
    ## Task
    Implement the core orchestrator loop.

    ## Loop Structure

    ```go
    func (o *Orchestrator) Run(ctx context.Context) error {
        for {
            select {
            case <-ctx.Done():
                return ctx.Err()
            default:
            }

            // 1. Find next ready bead
            bead, err := o.getNextReadyBead()
            if err != nil {
                return fmt.Errorf("get ready bead: %w", err)
            }

            // 2. No ready bead
            if bead == nil {
                if o.allDone() {
                    o.log.Info("Workflow complete")
                    return nil
                }
                // Waiting on agent or condition
                time.Sleep(o.cfg.PollInterval)
                continue
            }

            // 3. Dispatch by type
            o.log.Info("Processing bead", "id", bead.ID, "type", bead.Type)

            if err := o.dispatch(bead); err != nil {
                return o.handleError(bead, err)
            }

            // 4. Update heartbeat
            o.updateHeartbeat()
        }
    }

    func (o *Orchestrator) dispatch(bead *Bead) error {
        switch bead.Type {
        case BeadTask:
            return o.handleTask(bead)
        case BeadCondition:
            return o.handleCondition(bead)
        case BeadStop:
            return o.handleStop(bead)
        case BeadStart:
            return o.handleStart(bead)
        case BeadCheckpoint:
            return o.handleCheckpoint(bead)
        case BeadResume:
            return o.handleResume(bead)
        case BeadCode:
            return o.handleCode(bead)
        case BeadExpand:
            return o.handleExpand(bead)
        default:
            return fmt.Errorf("unknown bead type: %s", bead.Type)
        }
    }
    ```

    ## Concurrency

    Conditions run in goroutines to avoid blocking the main loop:

    ```go
    func (o *Orchestrator) handleCondition(bead *Bead) error {
        // Mark as in-progress
        o.markInProgress(bead)

        // Run in background
        go func() {
            result := o.evalCondition(bead)
            o.conditionResults <- conditionResult{bead.ID, result}
        }()

        return nil  // Don't wait
    }
    ```

    The main loop also listens to conditionResults channel.

    ## Acceptance Criteria
    - [ ] Loop runs until no ready beads and all done
    - [ ] Dispatches correctly to all 8 handlers
    - [ ] Conditions don't block main loop
    - [ ] Graceful shutdown on context cancel
    - [ ] Heartbeat updates regularly

# -----------------------------------------------------------------------------
# TASK 3.2: Bead Readiness
# -----------------------------------------------------------------------------
- id: meow-e3.2
  type: task
  title: "Implement bead readiness detection"
  priority: 0
  parent: meow-e3
  needs: [meow-e3.1]
  labels: [orchestrator, readiness]
  notes: |
    ## Task
    Determine which beads are ready for execution.

    ## Readiness Rules

    A bead is "ready" when:
    1. Status is `open`
    2. All beads in `needs` array are `closed`
    3. Agent exists (for task beads) or assignee is null (orchestrator beads)
    4. Not already being processed

    ## Priority Ordering

    When multiple beads are ready:
    1. **Orchestrator beads first**: condition, expand, code, stop, start, checkpoint, resume
    2. **Then task beads**: let orchestrator complete setup before agent work
    3. **Within same type**: earlier creation time
    4. **Tie-breaker**: lexicographic bead ID

    This ensures the orchestrator completes control flow before giving agents
    more work.

    ## Implementation

    ```go
    func (o *Orchestrator) getNextReadyBead() (*Bead, error) {
        // Get all open beads
        beads, err := o.beads.ListOpen()
        if err != nil {
            return nil, err
        }

        var ready []*Bead
        for _, bead := range beads {
            if o.isReady(bead) {
                ready = append(ready, bead)
            }
        }

        if len(ready) == 0 {
            return nil, nil
        }

        // Sort by priority
        sort.Slice(ready, func(i, j int) bool {
            return o.beadPriority(ready[i]) < o.beadPriority(ready[j])
        })

        return ready[0], nil
    }

    func (o *Orchestrator) isReady(bead *Bead) bool {
        if bead.Status != StatusOpen {
            return false
        }

        // Check dependencies
        for _, needID := range bead.Needs {
            need, _ := o.beads.Get(needID)
            if need == nil || need.Status != StatusClosed {
                return false
            }
        }

        // Check if already being processed (in-flight condition, etc.)
        if o.inFlight[bead.ID] {
            return false
        }

        return true
    }

    func (o *Orchestrator) beadPriority(bead *Bead) int {
        switch bead.Type {
        case BeadCondition, BeadExpand, BeadCode:
            return 0  // Highest: control flow
        case BeadStop, BeadStart, BeadCheckpoint, BeadResume:
            return 1  // Agent lifecycle
        case BeadTask:
            return 2  // Agent work
        default:
            return 3
        }
    }
    ```

    ## Change Detection

    For efficiency, watch `.beads/issues.jsonl` for changes rather than
    polling the full list every iteration:

    ```go
    func (o *Orchestrator) watchBeads(ctx context.Context) {
        watcher, _ := fsnotify.NewWatcher()
        watcher.Add(".beads/issues.jsonl")

        for {
            select {
            case <-ctx.Done():
                return
            case <-watcher.Events:
                o.beadsChanged <- struct{}{}
            }
        }
    }
    ```

    ## Acceptance Criteria
    - [ ] Correctly identifies ready beads
    - [ ] Dependencies checked transitively
    - [ ] Priority ordering correct
    - [ ] Handles empty needs array
    - [ ] File watching triggers re-check
    - [ ] Performance acceptable with 100+ beads

# -----------------------------------------------------------------------------
# TASK 3.3: State Persistence
# -----------------------------------------------------------------------------
- id: meow-e3.3
  type: task
  title: "Implement orchestrator state persistence"
  priority: 0
  parent: meow-e3
  needs: [meow-e3.1]
  labels: [orchestrator, state]
  notes: |
    ## Task
    Persist orchestrator state for crash recovery.

    ## State to Persist

    ### .meow/state/orchestrator.json
    ```json
    {
      "workflow_id": "meow-run-001",
      "started_at": "2026-01-07T10:00:00Z",
      "status": "running",
      "current_bead": "meow-run-001.write-tests-b2c9",
      "in_flight_conditions": ["meow-run-001.check-ci-a1b2"],
      "iteration": 42,
      "last_heartbeat": "2026-01-07T12:30:00Z"
    }
    ```

    ### .meow/agents.json
    ```json
    {
      "claude-1": {
        "id": "claude-1",
        "tmux_session": "meow-claude-1",
        "status": "active",
        "workdir": "/data/projects/myapp",
        "current_bead": "meow-run-001.implement-c3d4",
        "checkpoint": null,
        "started_at": "2026-01-07T10:00:00Z",
        "last_activity": "2026-01-07T12:30:00Z"
      }
    }
    ```

    ### .meow/state/heartbeat.json
    ```json
    {
      "pid": 12345,
      "timestamp": "2026-01-07T12:30:00Z",
      "status": "healthy"
    }
    ```

    ## Atomic Writes

    All state writes must be atomic (write to temp, then rename):

    ```go
    func atomicWrite(path string, data []byte) error {
        tmp := path + ".tmp"
        if err := os.WriteFile(tmp, data, 0644); err != nil {
            return err
        }
        return os.Rename(tmp, path)
    }
    ```

    ## Lock File

    Prevent concurrent orchestrators:

    ```go
    func (o *Orchestrator) acquireLock() error {
        lockPath := ".meow/state/orchestrator.lock"

        // Try to create exclusively
        f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
        if err != nil {
            if os.IsExist(err) {
                // Check if holder is alive
                return o.checkStaleLock(lockPath)
            }
            return err
        }

        // Write our PID
        fmt.Fprintf(f, "%d", os.Getpid())
        f.Close()

        o.lockPath = lockPath
        return nil
    }
    ```

    ## Acceptance Criteria
    - [ ] State persists across iterations
    - [ ] Atomic writes prevent corruption
    - [ ] Lock prevents concurrent instances
    - [ ] Stale lock detection works
    - [ ] Agent state updates on changes

# -----------------------------------------------------------------------------
# TASK 3.4: Startup and Recovery
# -----------------------------------------------------------------------------
- id: meow-e3.4
  type: task
  title: "Implement orchestrator startup and crash recovery"
  priority: 0
  parent: meow-e3
  needs: [meow-e3.3]
  labels: [orchestrator, recovery]
  notes: |
    ## Task
    Handle orchestrator startup, including recovery from crashes.

    ## Startup Flow

    ```go
    func (o *Orchestrator) Start(templateName string, vars map[string]any) error {
        // 1. Acquire lock
        if err := o.acquireLock(); err != nil {
            return fmt.Errorf("acquire lock: %w", err)
        }

        // 2. Check for existing state
        if o.hasExistingState() {
            return o.resume()
        }

        // 3. Fresh start
        return o.freshStart(templateName, vars)
    }

    func (o *Orchestrator) freshStart(templateName string, vars map[string]any) error {
        // Load template
        tmpl, err := o.templates.Load(templateName)
        if err != nil {
            return err
        }

        // Bake into beads
        result, err := o.templates.Bake(tmpl, vars)
        if err != nil {
            return err
        }

        // Create beads
        for _, bead := range result.Beads {
            if err := o.beads.Create(bead); err != nil {
                return err
            }
        }

        // Save initial state
        o.state.WorkflowID = result.WorkflowID
        o.state.Status = "running"
        o.saveState()

        // Spawn initial agent
        if err := o.spawnAgent(o.cfg.DefaultAgent); err != nil {
            return err
        }

        // Enter main loop
        return o.Run(context.Background())
    }
    ```

    ## Recovery Flow

    ```go
    func (o *Orchestrator) resume() error {
        o.log.Info("Resuming from previous state")

        // 1. Load orchestrator state
        if err := o.loadState(); err != nil {
            return err
        }

        // 2. Load agent states
        if err := o.loadAgents(); err != nil {
            return err
        }

        // 3. Reconcile with reality
        if err := o.reconcileAgents(); err != nil {
            return err
        }

        // 4. Reset in-progress beads from dead agents
        if err := o.resetOrphanedBeads(); err != nil {
            return err
        }

        // 5. Resume main loop
        return o.Run(context.Background())
    }

    func (o *Orchestrator) reconcileAgents() error {
        for id, agent := range o.agents {
            if agent.Status == AgentActive {
                // Check if tmux session exists
                if !o.tmux.SessionExists(agent.TmuxSession) {
                    o.log.Warn("Agent session died", "agent", id)
                    agent.Status = AgentStopped
                }
            }
        }
        return o.saveAgents()
    }

    func (o *Orchestrator) resetOrphanedBeads() error {
        beads, _ := o.beads.ListByStatus(StatusInProgress)
        for _, bead := range beads {
            agent := o.agents[bead.Assignee]
            if agent == nil || agent.Status != AgentActive {
                o.log.Info("Resetting orphaned bead", "id", bead.ID)
                bead.Status = StatusOpen
                o.beads.Update(bead)
            }
        }
        return nil
    }
    ```

    ## Acceptance Criteria
    - [ ] Fresh start creates beads and spawns agent
    - [ ] Resume loads existing state
    - [ ] Dead agents detected via tmux
    - [ ] In-progress beads from dead agents reset
    - [ ] Main loop continues from correct position

# -----------------------------------------------------------------------------
# TASK 3.5: Trace Logging
# -----------------------------------------------------------------------------
- id: meow-e3.5
  type: task
  title: "Implement execution trace logging"
  priority: 1
  parent: meow-e3
  needs: [meow-e3.1]
  labels: [orchestrator, logging]
  notes: |
    ## Task
    Log all orchestrator actions for debugging and audit.

    ## Trace Format

    `.meow/state/trace.jsonl`:
    ```json
    {"ts":"2026-01-07T10:00:00Z","action":"start","workflow":"meow-run-001"}
    {"ts":"2026-01-07T10:00:01Z","action":"bake","template":"work-loop","beads":["bd-001","bd-002"]}
    {"ts":"2026-01-07T10:00:02Z","action":"spawn","agent":"claude-1","session":"meow-claude-1"}
    {"ts":"2026-01-07T10:00:03Z","action":"dispatch","bead":"bd-001","type":"condition"}
    {"ts":"2026-01-07T10:00:04Z","action":"condition_eval","bead":"bd-001","command":"bd list...","exit":0}
    {"ts":"2026-01-07T10:00:05Z","action":"expand","bead":"bd-001","template":"work-iteration","created":["bd-003","bd-004"]}
    {"ts":"2026-01-07T10:00:06Z","action":"close","bead":"bd-001"}
    {"ts":"2026-01-07T10:05:00Z","action":"close","bead":"bd-003","actor":"claude-1"}
    ```

    ## Implementation

    ```go
    type Tracer struct {
        file *os.File
        mu   sync.Mutex
    }

    func (t *Tracer) Log(action string, data map[string]any) {
        t.mu.Lock()
        defer t.mu.Unlock()

        entry := map[string]any{
            "ts":     time.Now().Format(time.RFC3339),
            "action": action,
        }
        for k, v := range data {
            entry[k] = v
        }

        json.NewEncoder(t.file).Encode(entry)
    }
    ```

    ## CLI Access

    ```bash
    meow trace              # Show recent entries
    meow trace --follow     # Stream in real-time
    meow trace --json       # Raw JSON output
    meow trace --since 1h   # Last hour
    ```

    ## Acceptance Criteria
    - [ ] All dispatch actions logged
    - [ ] Condition eval results logged
    - [ ] Agent close events logged
    - [ ] Trace survives crash (file-based)
    - [ ] meow trace command works
```

# =============================================================================
# EPIC 4: PRIMITIVE HANDLERS
# =============================================================================

```yaml
# -----------------------------------------------------------------------------
# EPIC: Primitive Handlers
# -----------------------------------------------------------------------------
# PURPOSE:
# Implement the handlers for all 8 primitive bead types. These are the
# building blocks that templates compose.
#
# CONTEXT:
# The orchestrator dispatches beads to handlers based on type. Each handler
# knows how to execute its primitive and close the bead when done.
#
# SUCCESS CRITERIA:
# - All 8 primitives have working handlers
# - Handlers are isolated and testable
# - Error handling is consistent
# - Beads are closed correctly
# -----------------------------------------------------------------------------

- id: meow-e4
  type: epic
  title: "Primitive Handlers: All 8 primitive implementations"
  priority: 0
  labels: [epic, primitives, phase-2]
  needs: [meow-e3]
  notes: |
    ## Overview
    Each primitive gets its own handler. The orchestrator just dispatches.

    ## Handler Interface
    ```go
    type Handler interface {
        Handle(ctx context.Context, bead *Bead) error
    }
    ```

    Most handlers auto-close the bead. Task handler waits for agent to close.

# -----------------------------------------------------------------------------
# TASK 4.1: Task Handler
# -----------------------------------------------------------------------------
- id: meow-e4.1
  type: task
  title: "Implement task primitive handler"
  priority: 0
  parent: meow-e4
  needs: [meow-e3.2]
  labels: [primitives, task]
  notes: |
    ## Task
    Handle `task` beads by waiting for Claude to close them.

    ## Behavior

    1. Verify assigned agent is running
    2. Mark bead as in_progress (if not already)
    3. Wait for bead status to become `closed`
    4. Return (main loop finds next ready bead)

    ## Implementation

    ```go
    func (h *TaskHandler) Handle(ctx context.Context, bead *Bead) error {
        // Verify agent
        agent := h.agents.Get(bead.Assignee)
        if agent == nil || agent.Status != AgentActive {
            return fmt.Errorf("agent %q not active", bead.Assignee)
        }

        // Mark in progress
        if bead.Status == StatusOpen {
            bead.Status = StatusInProgress
            h.beads.Update(bead)
        }

        // Wait for close
        // The agent runs `meow close <bead-id>` when done
        // We detect this via file watch or polling
        for {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-h.beadChanged:
                current, _ := h.beads.Get(bead.ID)
                if current.Status == StatusClosed {
                    return nil
                }
            case <-time.After(h.pollInterval):
                current, _ := h.beads.Get(bead.ID)
                if current.Status == StatusClosed {
                    return nil
                }
            }
        }
    }
    ```

    ## Error Cases

    - Agent not running → error (should have been started)
    - Agent dies while task in progress → reset to open, respawn
    - Bead doesn't exist → error (shouldn't happen)

    ## Acceptance Criteria
    - [ ] Waits for agent to close bead
    - [ ] Marks bead in_progress
    - [ ] Returns when bead is closed
    - [ ] Handles agent death gracefully

# -----------------------------------------------------------------------------
# TASK 4.2: Condition Handler
# -----------------------------------------------------------------------------
- id: meow-e4.2
  type: task
  title: "Implement condition primitive handler"
  priority: 0
  parent: meow-e4
  needs: [meow-e3.2]
  labels: [primitives, condition]
  notes: |
    ## Task
    Handle `condition` beads by evaluating shell commands and expanding
    results.

    ## Behavior

    1. Mark bead in_progress
    2. Execute condition shell command (in background)
    3. Based on exit code, expand on_true or on_false
    4. Close bead

    ## Implementation

    ```go
    func (h *ConditionHandler) Handle(ctx context.Context, bead *Bead) error {
        spec := bead.Condition
        if spec == nil {
            return fmt.Errorf("condition bead missing spec")
        }

        // Mark in progress
        bead.Status = StatusInProgress
        h.beads.Update(bead)

        // Execute condition (may block!)
        h.tracer.Log("condition_start", map[string]any{"bead": bead.ID, "cmd": spec.Command})

        result, err := h.execWithTimeout(ctx, spec.Command, spec.Timeout)
        if err != nil && !isExitError(err) {
            return fmt.Errorf("condition exec: %w", err)
        }

        h.tracer.Log("condition_result", map[string]any{
            "bead": bead.ID,
            "exit": result.ExitCode,
            "stdout": result.Stdout,
        })

        // Expand based on result
        var target *ExpansionTarget
        if result.ExitCode == 0 {
            target = spec.OnTrue
        } else if result.IsTimeout {
            target = spec.OnTimeout
            if target == nil {
                target = spec.OnFalse
            }
        } else {
            target = spec.OnFalse
        }

        if target != nil {
            if err := h.expand(bead, target); err != nil {
                return fmt.Errorf("expand: %w", err)
            }
        }

        // Close condition bead
        return h.closeBead(bead)
    }
    ```

    ## Blocking Conditions

    Conditions can block arbitrarily (human gates, CI waits, etc.). They run
    in goroutines and signal completion to the main loop:

    ```go
    // In main loop:
    case BeadCondition:
        h.handleCondition(bead)  // Starts goroutine, returns immediately

    // Goroutine signals completion:
    o.conditionDone <- bead.ID
    ```

    ## Acceptance Criteria
    - [ ] Shell command executes correctly
    - [ ] Exit code 0 → on_true
    - [ ] Exit code ≠0 → on_false
    - [ ] Timeout → on_timeout (or on_false)
    - [ ] Expansion creates correct child beads
    - [ ] Bead auto-closes

# -----------------------------------------------------------------------------
# TASK 4.3: Code Handler
# -----------------------------------------------------------------------------
- id: meow-e4.3
  type: task
  title: "Implement code primitive handler"
  priority: 0
  parent: meow-e4
  needs: [meow-e3.2]
  labels: [primitives, code]
  notes: |
    ## Task
    Handle `code` beads by executing arbitrary shell scripts.

    ## Behavior

    1. Execute code in shell
    2. Capture stdout/stderr
    3. Handle errors based on on_error policy
    4. Close bead

    ## Implementation

    ```go
    func (h *CodeHandler) Handle(ctx context.Context, bead *Bead) error {
        spec := bead.CodeSpec
        if spec == nil {
            return fmt.Errorf("code bead missing spec")
        }

        bead.Status = StatusInProgress
        h.beads.Update(bead)

        // Build command
        cmd := exec.CommandContext(ctx, "sh", "-c", spec.Code)
        if spec.Workdir != "" {
            cmd.Dir = spec.Workdir
        }
        cmd.Env = append(os.Environ(), mapToEnv(spec.Env)...)

        // Execute
        var stdout, stderr bytes.Buffer
        cmd.Stdout = &stdout
        cmd.Stderr = &stderr

        err := cmd.Run()

        h.tracer.Log("code_exec", map[string]any{
            "bead":   bead.ID,
            "exit":   cmd.ProcessState.ExitCode(),
            "stdout": stdout.String(),
            "stderr": stderr.String(),
        })

        // Handle error
        if err != nil {
            switch spec.OnError {
            case "continue":
                // Log and continue
                h.log.Warn("Code failed, continuing", "bead", bead.ID, "err", err)
            case "abort":
                return fmt.Errorf("code failed: %w", err)
            case "retry":
                // Retry logic
                if spec.Retries < spec.MaxRetries {
                    spec.Retries++
                    h.beads.Update(bead)
                    return h.Handle(ctx, bead)
                }
                return fmt.Errorf("code failed after %d retries: %w", spec.MaxRetries, err)
            }
        }

        return h.closeBead(bead)
    }
    ```

    ## Acceptance Criteria
    - [ ] Shell script executes
    - [ ] Working directory set correctly
    - [ ] Environment variables passed
    - [ ] on_error: continue works
    - [ ] on_error: abort stops workflow
    - [ ] on_error: retry retries
    - [ ] Output logged to trace

# -----------------------------------------------------------------------------
# TASK 4.4: Expand Handler
# -----------------------------------------------------------------------------
- id: meow-e4.4
  type: task
  title: "Implement expand primitive handler"
  priority: 0
  parent: meow-e4
  needs: [meow-e2.4, meow-e3.2]
  labels: [primitives, expand]
  notes: |
    ## Task
    Handle `expand` beads by loading and baking templates.

    ## Behavior

    1. Load referenced template
    2. Merge variables from expand bead with template defaults
    3. Bake template into child beads
    4. Wire dependencies (children depend on expand bead's position)
    5. Set assignee on children if specified
    6. Close expand bead

    ## Implementation

    ```go
    func (h *ExpandHandler) Handle(ctx context.Context, bead *Bead) error {
        spec := bead.ExpandSpec
        if spec == nil {
            return fmt.Errorf("expand bead missing spec")
        }

        // Load template
        tmpl, err := h.templates.Load(spec.Template)
        if err != nil {
            return fmt.Errorf("load template %q: %w", spec.Template, err)
        }

        // Merge variables
        vars := make(map[string]any)
        // First, template defaults
        for name, def := range tmpl.Variables {
            if def.Default != nil {
                vars[name] = def.Default
            }
        }
        // Then, expand bead variables
        for k, v := range spec.Variables {
            vars[k] = v
        }
        // Add builtins
        vars["parent_bead"] = bead.ID
        vars["expand_depth"] = bead.Meta["expand_depth"].(int) + 1

        // Bake
        result, err := h.templates.Bake(tmpl, vars)
        if err != nil {
            return fmt.Errorf("bake template: %w", err)
        }

        // Wire dependencies: children depend on beads that this expand depends on
        for _, child := range result.Beads {
            // Child needs what expand needed, plus expand itself is "done"
            // Actually, children should execute AFTER expand, so their
            // dependencies should include anything in expand's position
            child.Meta["parent_expand"] = bead.ID

            // Set assignee if specified
            if spec.Assignee != "" {
                child.Assignee = spec.Assignee
            }

            h.beads.Create(child)
        }

        h.tracer.Log("expand", map[string]any{
            "bead":     bead.ID,
            "template": spec.Template,
            "created":  result.BeadIDs(),
        })

        return h.closeBead(bead)
    }
    ```

    ## Recursive Expansion

    Expanded templates can contain expand beads. The orchestrator handles
    this naturally—the child expand beads become ready after the parent
    expand closes, and they get processed in subsequent iterations.

    ## Acceptance Criteria
    - [ ] Template loads correctly
    - [ ] Variables merge correctly
    - [ ] Child beads created
    - [ ] Assignee propagates
    - [ ] Recursive expansion works
    - [ ] Expand bead auto-closes

# -----------------------------------------------------------------------------
# TASK 4.5: Stop Handler
# -----------------------------------------------------------------------------
- id: meow-e4.5
  type: task
  title: "Implement stop primitive handler"
  priority: 0
  parent: meow-e4
  needs: [meow-e5.1]  # Needs agent management
  labels: [primitives, stop]
  notes: |
    ## Task
    Handle `stop` beads by killing agent tmux sessions.

    ## Behavior

    1. If graceful: send interrupt, wait, then force kill
    2. Kill tmux session
    3. Update agent state to stopped
    4. Close bead

    ## Implementation

    ```go
    func (h *StopHandler) Handle(ctx context.Context, bead *Bead) error {
        spec := bead.AgentSpec
        if spec == nil {
            return fmt.Errorf("stop bead missing agent spec")
        }

        agent := h.agents.Get(spec.Agent)
        if agent == nil {
            // Agent doesn't exist, nothing to stop
            return h.closeBead(bead)
        }

        if agent.Status != AgentActive {
            // Already stopped
            return h.closeBead(bead)
        }

        // Graceful shutdown
        if spec.Graceful {
            h.log.Info("Graceful stop", "agent", spec.Agent)

            // Send Ctrl-C
            h.tmux.SendKeys(agent.TmuxSession, "C-c")

            // Wait for clean exit
            deadline := time.Now().Add(time.Duration(spec.Timeout) * time.Second)
            for time.Now().Before(deadline) {
                if !h.tmux.SessionExists(agent.TmuxSession) {
                    break
                }
                time.Sleep(100 * time.Millisecond)
            }
        }

        // Force kill
        if h.tmux.SessionExists(agent.TmuxSession) {
            h.tmux.KillSession(agent.TmuxSession)
        }

        // Update agent state
        agent.Status = AgentStopped
        h.agents.Update(agent)

        h.tracer.Log("stop", map[string]any{"agent": spec.Agent})

        return h.closeBead(bead)
    }
    ```

    ## Acceptance Criteria
    - [ ] Graceful stop sends interrupt first
    - [ ] Force kill after timeout
    - [ ] Agent state updated to stopped
    - [ ] Works even if agent not running
    - [ ] Bead auto-closes

# -----------------------------------------------------------------------------
# TASK 4.6: Start Handler
# -----------------------------------------------------------------------------
- id: meow-e4.6
  type: task
  title: "Implement start primitive handler"
  priority: 0
  parent: meow-e4
  needs: [meow-e5.1]  # Needs agent management
  labels: [primitives, start]
  notes: |
    ## Task
    Handle `start` beads by spawning agents in tmux.

    ## Behavior

    1. Create tmux session
    2. Set environment
    3. Start Claude
    4. Wait for ready
    5. Inject prompt (meow prime)
    6. Update agent state
    7. Close bead

    ## Implementation

    ```go
    func (h *StartHandler) Handle(ctx context.Context, bead *Bead) error {
        spec := bead.AgentSpec
        if spec == nil {
            return fmt.Errorf("start bead missing agent spec")
        }

        sessionName := "meow-" + spec.Agent
        workdir := spec.Workdir
        if workdir == "" {
            workdir, _ = os.Getwd()
        }

        // Create tmux session
        if err := h.tmux.NewSession(sessionName, workdir); err != nil {
            return fmt.Errorf("create tmux session: %w", err)
        }

        // Set environment
        env := map[string]string{
            "MEOW_AGENT": spec.Agent,
        }
        for k, v := range spec.Env {
            env[k] = v
        }
        for k, v := range env {
            h.tmux.SetEnv(sessionName, k, v)
        }

        // Start Claude
        claudeCmd := "claude --dangerously-skip-permissions"
        h.tmux.SendKeys(sessionName, claudeCmd)
        h.tmux.SendKeys(sessionName, "Enter")

        // Wait for Claude ready (detect prompt)
        if err := h.waitForReady(ctx, sessionName); err != nil {
            return fmt.Errorf("wait for claude ready: %w", err)
        }

        // Inject prompt
        prompt := spec.Prompt
        if prompt == "" {
            prompt = "meow prime"
        }
        h.tmux.SendKeys(sessionName, prompt)
        h.tmux.SendKeys(sessionName, "Enter")

        // Update agent state
        agent := &Agent{
            ID:           spec.Agent,
            TmuxSession:  sessionName,
            Status:       AgentActive,
            Workdir:      workdir,
            StartedAt:    time.Now(),
            LastActivity: time.Now(),
        }
        h.agents.Set(agent)

        h.tracer.Log("start", map[string]any{
            "agent":   spec.Agent,
            "session": sessionName,
            "workdir": workdir,
        })

        return h.closeBead(bead)
    }
    ```

    ## Claude Ready Detection

    Wait for Claude to be ready to accept input. This is tricky—we look for
    the prompt or a known pattern in output:

    ```go
    func (h *StartHandler) waitForReady(ctx context.Context, session string) error {
        timeout := time.After(30 * time.Second)
        ticker := time.NewTicker(500 * time.Millisecond)
        defer ticker.Stop()

        for {
            select {
            case <-ctx.Done():
                return ctx.Err()
            case <-timeout:
                return fmt.Errorf("timeout waiting for Claude ready")
            case <-ticker.C:
                output := h.tmux.CapturePane(session)
                if strings.Contains(output, "Claude") || strings.Contains(output, ">") {
                    return nil
                }
            }
        }
    }
    ```

    ## Acceptance Criteria
    - [ ] Creates tmux session
    - [ ] Sets environment correctly
    - [ ] Starts Claude
    - [ ] Detects when Claude is ready
    - [ ] Injects meow prime
    - [ ] Agent state reflects active

# -----------------------------------------------------------------------------
# TASK 4.7: Checkpoint Handler
# -----------------------------------------------------------------------------
- id: meow-e4.7
  type: task
  title: "Implement checkpoint primitive handler"
  priority: 0
  parent: meow-e4
  needs: [meow-e5.1]
  labels: [primitives, checkpoint]
  notes: |
    ## Task
    Handle `checkpoint` beads by saving session IDs for later resume.

    ## Behavior

    1. Discover session ID
    2. Store checkpoint in agent state
    3. Close bead

    ## Session ID Discovery

    Claude Code exposes session ID via:
    - Environment variable: `$CLAUDE_SESSION_ID` (if we set a hook)
    - Session file: `~/.claude/sessions/<latest>`

    For MVP, we'll inject a hook that captures the session ID.

    ## Implementation

    ```go
    func (h *CheckpointHandler) Handle(ctx context.Context, bead *Bead) error {
        spec := bead.AgentSpec
        if spec == nil {
            return fmt.Errorf("checkpoint bead missing agent spec")
        }

        agent := h.agents.Get(spec.Agent)
        if agent == nil {
            return fmt.Errorf("agent %q not found", spec.Agent)
        }

        // Discover session ID
        sessionID, err := h.discoverSessionID(agent)
        if err != nil {
            return fmt.Errorf("discover session ID: %w", err)
        }

        // Store checkpoint
        agent.Checkpoint = &Checkpoint{
            SessionID:   sessionID,
            Label:       spec.Label,
            Timestamp:   time.Now(),
            BeadContext: h.getCurrentBeadContext(agent),
        }
        h.agents.Update(agent)

        h.tracer.Log("checkpoint", map[string]any{
            "agent":      spec.Agent,
            "session_id": sessionID,
            "label":      spec.Label,
        })

        return h.closeBead(bead)
    }

    func (h *CheckpointHandler) discoverSessionID(agent *Agent) (string, error) {
        // Option 1: Read from environment (if we injected it)
        // meow prime --hook writes session ID to a known file
        sessionFile := filepath.Join(".meow", "state", "sessions", agent.ID)
        if data, err := os.ReadFile(sessionFile); err == nil {
            return strings.TrimSpace(string(data)), nil
        }

        // Option 2: Find latest Claude session
        home, _ := os.UserHomeDir()
        sessionsDir := filepath.Join(home, ".claude", "sessions")
        // Find most recent session file
        // ...

        return "", fmt.Errorf("could not discover session ID")
    }
    ```

    ## Acceptance Criteria
    - [ ] Discovers session ID
    - [ ] Stores checkpoint with label
    - [ ] Works with running agent
    - [ ] Bead auto-closes

# -----------------------------------------------------------------------------
# TASK 4.8: Resume Handler
# -----------------------------------------------------------------------------
- id: meow-e4.8
  type: task
  title: "Implement resume primitive handler"
  priority: 0
  parent: meow-e4
  needs: [meow-e4.7]
  labels: [primitives, resume]
  notes: |
    ## Task
    Handle `resume` beads by starting Claude with --resume flag.

    ## Behavior

    1. Retrieve checkpoint
    2. Create tmux session
    3. Start Claude with --resume <session_id>
    4. Inject meow prime
    5. Update agent state
    6. Close bead

    ## Implementation

    ```go
    func (h *ResumeHandler) Handle(ctx context.Context, bead *Bead) error {
        spec := bead.AgentSpec
        if spec == nil {
            return fmt.Errorf("resume bead missing agent spec")
        }

        agent := h.agents.Get(spec.Agent)
        if agent == nil {
            return fmt.Errorf("agent %q not found", spec.Agent)
        }

        // Get checkpoint
        checkpoint := agent.Checkpoint
        if spec.CheckpointLabel != "" {
            // Find checkpoint by label
            checkpoint = h.findCheckpointByLabel(agent, spec.CheckpointLabel)
        }
        if checkpoint == nil {
            return fmt.Errorf("no checkpoint found for agent %q", spec.Agent)
        }

        sessionName := "meow-" + spec.Agent

        // Create tmux session
        if err := h.tmux.NewSession(sessionName, agent.Workdir); err != nil {
            return fmt.Errorf("create tmux session: %w", err)
        }

        // Start Claude with --resume
        claudeCmd := fmt.Sprintf("claude --resume %s", checkpoint.SessionID)
        h.tmux.SendKeys(sessionName, claudeCmd)
        h.tmux.SendKeys(sessionName, "Enter")

        // Wait for ready
        if err := h.waitForReady(ctx, sessionName); err != nil {
            return fmt.Errorf("wait for claude ready: %w", err)
        }

        // Inject meow prime
        h.tmux.SendKeys(sessionName, "meow prime")
        h.tmux.SendKeys(sessionName, "Enter")

        // Update agent state
        agent.TmuxSession = sessionName
        agent.Status = AgentActive
        agent.LastActivity = time.Now()
        h.agents.Update(agent)

        h.tracer.Log("resume", map[string]any{
            "agent":      spec.Agent,
            "session_id": checkpoint.SessionID,
        })

        return h.closeBead(bead)
    }
    ```

    ## Acceptance Criteria
    - [ ] Retrieves checkpoint correctly
    - [ ] Starts Claude with --resume
    - [ ] Agent wakes up with prior context
    - [ ] meow prime shows next work
    - [ ] Agent state updated
```

# =============================================================================
# EPIC 5: AGENT MANAGEMENT
# =============================================================================

```yaml
# -----------------------------------------------------------------------------
# EPIC: Agent Management
# -----------------------------------------------------------------------------
# PURPOSE:
# Manage agent lifecycle via tmux. Spawn, stop, monitor sessions.
#
# CONTEXT:
# Agents run in tmux sessions, which provides persistence across SSH
# disconnects and enables send-keys for injection.
#
# SUCCESS CRITERIA:
# - Can create/kill tmux sessions
# - Can send commands to sessions
# - Can detect if sessions are alive
# - Agent state accurately reflects reality
# -----------------------------------------------------------------------------

- id: meow-e5
  type: epic
  title: "Agent Management: tmux and lifecycle"
  priority: 0
  labels: [epic, agents, phase-2]
  needs: [meow-e3]
  notes: |
    ## Overview
    Agents are Claude Code instances running in tmux sessions. This epic
    provides the infrastructure to manage them.

# -----------------------------------------------------------------------------
# TASK 5.1: tmux Wrapper
# -----------------------------------------------------------------------------
- id: meow-e5.1
  type: task
  title: "Implement tmux wrapper library"
  priority: 0
  parent: meow-e5
  needs: [meow-e1.2]
  labels: [agents, tmux]
  notes: |
    ## Task
    Create a Go wrapper around tmux commands.

    ## Interface

    ```go
    type Tmux interface {
        NewSession(name, workdir string) error
        KillSession(name string) error
        SessionExists(name string) bool
        ListSessions() ([]string, error)

        SendKeys(session, keys string) error
        SendKeysLiteral(session, text string) error

        CapturePane(session string) string
        SetEnv(session, key, value string) error
    }
    ```

    ## Implementation

    ```go
    type tmuxImpl struct{}

    func (t *tmuxImpl) NewSession(name, workdir string) error {
        cmd := exec.Command("tmux", "new-session", "-d", "-s", name, "-c", workdir)
        return cmd.Run()
    }

    func (t *tmuxImpl) KillSession(name string) error {
        cmd := exec.Command("tmux", "kill-session", "-t", name)
        return cmd.Run()
    }

    func (t *tmuxImpl) SessionExists(name string) bool {
        cmd := exec.Command("tmux", "has-session", "-t", name)
        return cmd.Run() == nil
    }

    func (t *tmuxImpl) SendKeys(session, keys string) error {
        cmd := exec.Command("tmux", "send-keys", "-t", session, keys)
        return cmd.Run()
    }

    func (t *tmuxImpl) CapturePane(session string) string {
        cmd := exec.Command("tmux", "capture-pane", "-t", session, "-p")
        out, _ := cmd.Output()
        return string(out)
    }
    ```

    ## Error Handling

    - Session already exists → return specific error type
    - tmux not installed → helpful error message
    - Session not found → specific error type

    ## Acceptance Criteria
    - [ ] Create session in specific directory
    - [ ] Kill session cleanly
    - [ ] Check if session exists
    - [ ] Send text to session
    - [ ] Capture current pane output
    - [ ] Handle tmux not installed

# -----------------------------------------------------------------------------
# TASK 5.2: Agent State Store
# -----------------------------------------------------------------------------
- id: meow-e5.2
  type: task
  title: "Implement agent state storage"
  priority: 0
  parent: meow-e5
  needs: [meow-e1.2]
  labels: [agents, state]
  notes: |
    ## Task
    Manage persistent agent state.

    ## Storage

    `.meow/agents.json`:
    ```json
    {
      "claude-1": {
        "id": "claude-1",
        "tmux_session": "meow-claude-1",
        "status": "active",
        "workdir": "/data/projects/myapp",
        "current_bead": "bd-task-001",
        "checkpoint": {
          "session_id": "conv_abc123",
          "label": "pre-child",
          "timestamp": "2026-01-07T10:00:00Z"
        },
        "started_at": "2026-01-07T10:00:00Z",
        "last_activity": "2026-01-07T12:30:00Z"
      }
    }
    ```

    ## Interface

    ```go
    type AgentStore interface {
        Get(id string) *Agent
        Set(agent *Agent) error
        Update(agent *Agent) error
        Delete(id string) error
        List() []*Agent
        ListByStatus(status AgentStatus) []*Agent

        Load() error
        Save() error
    }
    ```

    ## Implementation

    ```go
    type agentStore struct {
        agents map[string]*Agent
        path   string
        mu     sync.RWMutex
    }

    func (s *agentStore) Load() error {
        s.mu.Lock()
        defer s.mu.Unlock()

        data, err := os.ReadFile(s.path)
        if os.IsNotExist(err) {
            s.agents = make(map[string]*Agent)
            return nil
        }
        if err != nil {
            return err
        }

        return json.Unmarshal(data, &s.agents)
    }

    func (s *agentStore) Save() error {
        s.mu.RLock()
        defer s.mu.RUnlock()

        data, err := json.MarshalIndent(s.agents, "", "  ")
        if err != nil {
            return err
        }

        return atomicWrite(s.path, data)
    }
    ```

    ## Acceptance Criteria
    - [ ] Persist agent state to disk
    - [ ] Atomic writes for safety
    - [ ] Thread-safe access
    - [ ] Handle missing file gracefully

# -----------------------------------------------------------------------------
# TASK 5.3: Agent Spawning
# -----------------------------------------------------------------------------
- id: meow-e5.3
  type: task
  title: "Implement agent spawning logic"
  priority: 0
  parent: meow-e5
  needs: [meow-e5.1, meow-e5.2]
  labels: [agents, spawn]
  notes: |
    ## Task
    Higher-level agent spawning that coordinates tmux and state.

    ## Spawn Flow

    1. Check if agent already exists
    2. Create tmux session
    3. Configure environment
    4. Start Claude
    5. Wait for ready
    6. Inject initial prompt
    7. Update agent state

    ## Implementation

    ```go
    type AgentManager struct {
        tmux   Tmux
        store  AgentStore
        tracer *Tracer
    }

    func (m *AgentManager) Spawn(cfg *SpawnConfig) (*Agent, error) {
        // Check existing
        if existing := m.store.Get(cfg.ID); existing != nil {
            if existing.Status == AgentActive {
                return nil, fmt.Errorf("agent %q already active", cfg.ID)
            }
        }

        sessionName := "meow-" + cfg.ID

        // Create session
        if err := m.tmux.NewSession(sessionName, cfg.Workdir); err != nil {
            return nil, fmt.Errorf("create session: %w", err)
        }

        // Set environment
        for k, v := range cfg.Env {
            m.tmux.SetEnv(sessionName, k, v)
        }
        m.tmux.SetEnv(sessionName, "MEOW_AGENT", cfg.ID)

        // Start Claude
        m.tmux.SendKeys(sessionName, "claude --dangerously-skip-permissions")
        m.tmux.SendKeys(sessionName, "Enter")

        // Wait for ready
        if err := m.waitForReady(sessionName, 30*time.Second); err != nil {
            m.tmux.KillSession(sessionName)
            return nil, fmt.Errorf("wait ready: %w", err)
        }

        // Inject prompt
        if cfg.Prompt != "" {
            m.tmux.SendKeys(sessionName, cfg.Prompt)
            m.tmux.SendKeys(sessionName, "Enter")
        }

        // Create agent record
        agent := &Agent{
            ID:           cfg.ID,
            TmuxSession:  sessionName,
            Status:       AgentActive,
            Workdir:      cfg.Workdir,
            StartedAt:    time.Now(),
            LastActivity: time.Now(),
        }
        m.store.Set(agent)
        m.store.Save()

        m.tracer.Log("agent_spawn", map[string]any{
            "agent":   cfg.ID,
            "session": sessionName,
        })

        return agent, nil
    }
    ```

    ## Acceptance Criteria
    - [ ] Creates complete agent setup
    - [ ] Handles existing agent
    - [ ] Cleans up on failure
    - [ ] Updates state correctly

# -----------------------------------------------------------------------------
# TASK 5.4: Agent Monitoring
# -----------------------------------------------------------------------------
- id: meow-e5.4
  type: task
  title: "Implement agent health monitoring"
  priority: 1
  parent: meow-e5
  needs: [meow-e5.3]
  labels: [agents, monitoring]
  notes: |
    ## Task
    Monitor agent health and detect crashes/stuck agents.

    ## Health Checks

    1. **Session exists**: tmux has-session
    2. **Activity check**: Has the agent made progress recently?
    3. **Stuck detection**: Same bead in_progress for too long

    ## Implementation

    ```go
    func (m *AgentManager) HealthCheck() []AgentHealth {
        var results []AgentHealth

        for _, agent := range m.store.List() {
            health := AgentHealth{
                AgentID: agent.ID,
                Status:  "healthy",
            }

            if agent.Status == AgentActive {
                // Check session exists
                if !m.tmux.SessionExists(agent.TmuxSession) {
                    health.Status = "crashed"
                    health.Issues = append(health.Issues, "tmux session died")
                }

                // Check stuck
                if time.Since(agent.LastActivity) > m.cfg.StuckThreshold {
                    health.Status = "stuck"
                    health.Issues = append(health.Issues,
                        fmt.Sprintf("no activity for %v", time.Since(agent.LastActivity)))
                }
            }

            results = append(results, health)
        }

        return results
    }

    func (m *AgentManager) HandleCrash(agentID string) error {
        agent := m.store.Get(agentID)
        if agent == nil {
            return nil
        }

        // Update state
        agent.Status = AgentStopped
        m.store.Update(agent)

        // Reset in-progress beads
        // (handled by orchestrator, not here)

        return m.store.Save()
    }
    ```

    ## Acceptance Criteria
    - [ ] Detects crashed sessions
    - [ ] Detects stuck agents
    - [ ] Updates state on crash
    - [ ] Integrates with orchestrator recovery
```

# =============================================================================
# EPIC 6-11: Remaining Epics (Abbreviated)
# =============================================================================

```yaml
# -----------------------------------------------------------------------------
# EPIC 6: Condition System
# -----------------------------------------------------------------------------
- id: meow-e6
  type: epic
  title: "Condition System: Blocking, timeouts, and gates"
  priority: 0
  labels: [epic, conditions, phase-3]
  needs: [meow-e4]
  notes: |
    ## Overview
    Deep implementation of blocking conditions, including human gates.

    ## Key Tasks
    - meow-e6.1: Blocking shell execution
    - meow-e6.2: Timeout handling
    - meow-e6.3: Helper commands (meow wait-approve, etc.)
    - meow-e6.4: Gate approval flow (meow approve/reject)

- id: meow-e6.1
  type: task
  title: "Implement blocking shell execution with cancellation"
  priority: 0
  parent: meow-e6
  labels: [conditions, blocking]
  notes: |
    Shell commands that block indefinitely (human gates, CI waits).
    Must support cancellation via context.
    Run in goroutines to not block main loop.

- id: meow-e6.2
  type: task
  title: "Implement condition timeout handling"
  priority: 1
  parent: meow-e6
  needs: [meow-e6.1]
  labels: [conditions, timeout]
  notes: |
    Timeouts specified on conditions. On timeout, expand on_timeout
    template (or on_false if not specified).

- id: meow-e6.3
  type: task
  title: "Implement helper commands (wait-approve, wait-file, etc.)"
  priority: 1
  parent: meow-e6
  labels: [conditions, helpers]
  notes: |
    Convenience commands for common blocking patterns:
    - meow wait-approve --bead <id>
    - meow wait-file <path>
    - meow wait-bead <id> --status closed
    - meow context-usage --threshold N --format exit-code

- id: meow-e6.4
  type: task
  title: "Implement gate approval flow"
  priority: 0
  parent: meow-e6
  needs: [meow-e6.1]
  labels: [conditions, gates]
  notes: |
    Human runs `meow approve <bead>` to unblock a waiting condition.
    Implementation: creates a marker file or writes to a pipe that
    the waiting `meow wait-approve` command detects.

# -----------------------------------------------------------------------------
# EPIC 7: Checkpoint/Resume
# -----------------------------------------------------------------------------
- id: meow-e7
  type: epic
  title: "Checkpoint/Resume: Session persistence"
  priority: 0
  labels: [epic, checkpoint, phase-3]
  needs: [meow-e4, meow-e5]
  notes: |
    ## Overview
    Enable checkpoint and resume of Claude sessions.

    ## Key Tasks
    - meow-e7.1: Session ID discovery mechanism
    - meow-e7.2: Checkpoint storage and retrieval
    - meow-e7.3: Resume with --resume flag

- id: meow-e7.1
  type: task
  title: "Implement session ID discovery"
  priority: 0
  parent: meow-e7
  labels: [checkpoint, session]
  notes: |
    Need to discover Claude's session ID for checkpoint.
    Options:
    - Inject via meow prime --hook (writes to file)
    - Find latest in ~/.claude/sessions/
    - Parse from Claude output

- id: meow-e7.2
  type: task
  title: "Implement checkpoint storage"
  priority: 0
  parent: meow-e7
  needs: [meow-e7.1]
  labels: [checkpoint, storage]
  notes: |
    Store checkpoints with labels in agent state.
    Support multiple checkpoints per agent.

- id: meow-e7.3
  type: task
  title: "Implement resume with context"
  priority: 0
  parent: meow-e7
  needs: [meow-e7.2]
  labels: [checkpoint, resume]
  notes: |
    Start Claude with --resume <session_id>.
    Inject meow prime after resume.
    Verify context is restored.

# -----------------------------------------------------------------------------
# EPIC 8: Integration
# -----------------------------------------------------------------------------
- id: meow-e8
  type: epic
  title: "Integration: Beads, hooks, recovery"
  priority: 0
  labels: [epic, integration, phase-3]
  needs: [meow-e3, meow-e4]
  notes: |
    ## Overview
    Integration with external systems.

    ## Key Tasks
    - meow-e8.1: Beads integration (bd commands)
    - meow-e8.2: Claude Code hooks setup
    - meow-e8.3: Crash recovery testing
    - meow-e8.4: Git worktree support

- id: meow-e8.1
  type: task
  title: "Implement beads integration"
  priority: 0
  parent: meow-e8
  labels: [integration, beads]
  notes: |
    Use bd commands for bead operations:
    - bd create for new beads
    - bd update for status changes
    - bd show for reading
    - bd list for queries

    Alternatively, direct JSONL manipulation for performance.

- id: meow-e8.2
  type: task
  title: "Implement Claude Code hooks setup"
  priority: 1
  parent: meow-e8
  labels: [integration, hooks]
  notes: |
    Configure .claude/settings.json with MEOW hooks:
    - SessionStart: meow prime --hook
    - Stop: meow prime --format prompt

    meow init should set this up.

- id: meow-e8.3
  type: task
  title: "Test and validate crash recovery"
  priority: 1
  parent: meow-e8
  needs: [meow-e3.4]
  labels: [integration, recovery]
  notes: |
    Integration test:
    1. Start workflow
    2. Kill orchestrator mid-execution
    3. Restart
    4. Verify continues correctly
    5. Verify no duplicate work

- id: meow-e8.4
  type: task
  title: "Implement git worktree support for code beads"
  priority: 2
  parent: meow-e8
  labels: [integration, git]
  notes: |
    Code beads can create worktrees for isolated work.
    Support in start bead workdir specification.

# -----------------------------------------------------------------------------
# EPIC 9: Default Templates
# -----------------------------------------------------------------------------
- id: meow-e9
  type: epic
  title: "Default Templates: MVP template library"
  priority: 1
  labels: [epic, templates, phase-4]
  needs: [meow-e2]
  notes: |
    ## Overview
    Ship useful default templates.

    ## Templates
    - work-loop: Outer loop
    - implement: TDD workflow
    - human-gate: Blocking approval
    - refresh: Kill and restart
    - handoff: Notes and refresh
    - context-check: Check and maybe refresh

- id: meow-e9.1
  type: task
  title: "Create work-loop template"
  priority: 0
  parent: meow-e9
  labels: [templates, work-loop]
  notes: |
    Main orchestration loop.
    Check for work → select → implement → check context → loop.

- id: meow-e9.2
  type: task
  title: "Create implement template"
  priority: 0
  parent: meow-e9
  labels: [templates, implement]
  notes: |
    TDD workflow:
    load-context → write-tests → verify-fail → implement →
    verify-pass → review → commit

- id: meow-e9.3
  type: task
  title: "Create human-gate template"
  priority: 0
  parent: meow-e9
  labels: [templates, gate]
  notes: |
    Blocking approval:
    prepare-summary → notify → await-approval → record-decision

- id: meow-e9.4
  type: task
  title: "Create refresh and handoff templates"
  priority: 1
  parent: meow-e9
  labels: [templates, refresh]
  notes: |
    refresh: stop → start
    handoff: write-notes → stop → start

- id: meow-e9.5
  type: task
  title: "Create context-check template"
  priority: 1
  parent: meow-e9
  labels: [templates, context]
  notes: |
    condition: meow context-usage --threshold N
    on_true: expand handoff
    on_false: nothing

# -----------------------------------------------------------------------------
# EPIC 10: CLI Completion
# -----------------------------------------------------------------------------
- id: meow-e10
  type: epic
  title: "CLI Completion: All commands"
  priority: 1
  labels: [epic, cli, phase-4]
  needs: [meow-e3, meow-e4, meow-e5]
  notes: |
    ## Overview
    Complete all CLI commands.

    ## User Commands
    - meow init
    - meow run
    - meow status
    - meow approve/reject
    - meow pause/resume/stop

    ## Agent Commands
    - meow prime
    - meow close
    - meow handoff
    - meow context-usage

    ## Debug Commands
    - meow agents
    - meow beads
    - meow trace
    - meow peek/attach
    - meow validate

- id: meow-e10.1
  type: task
  title: "Implement meow init command"
  priority: 0
  parent: meow-e10
  labels: [cli, init]
  notes: |
    Initialize .meow directory:
    - Create directory structure
    - Copy default templates
    - Create config.toml
    - Setup Claude Code hooks

- id: meow-e10.2
  type: task
  title: "Implement meow run command"
  priority: 0
  parent: meow-e10
  needs: [meow-e3.4]
  labels: [cli, run]
  notes: |
    Start workflow execution.
    Usage: meow run <template> [--var key=value]...

- id: meow-e10.3
  type: task
  title: "Implement meow prime command"
  priority: 0
  parent: meow-e10
  labels: [cli, prime]
  notes: |
    Show next task for agent.
    Reads $MEOW_AGENT, finds assigned ready bead.
    Outputs formatted context.

- id: meow-e10.4
  type: task
  title: "Implement meow close command"
  priority: 0
  parent: meow-e10
  labels: [cli, close]
  notes: |
    Close a task bead.
    Usage: meow close <bead-id> [--notes "..."]
    Wraps bd close with MEOW context.

- id: meow-e10.5
  type: task
  title: "Implement meow status command"
  priority: 1
  parent: meow-e10
  labels: [cli, status]
  notes: |
    Show workflow status.
    Agents, beads, progress, current activity.

- id: meow-e10.6
  type: task
  title: "Implement debug commands (agents, beads, trace, peek)"
  priority: 1
  parent: meow-e10
  labels: [cli, debug]
  notes: |
    Various debugging commands for visibility.

# -----------------------------------------------------------------------------
# EPIC 11: Testing & Documentation
# -----------------------------------------------------------------------------
- id: meow-e11
  type: epic
  title: "Testing & Documentation"
  priority: 1
  labels: [epic, testing, phase-4]
  needs: [meow-e10]
  notes: |
    ## Overview
    Integration tests and user documentation.

- id: meow-e11.1
  type: task
  title: "Create integration test suite"
  priority: 0
  parent: meow-e11
  labels: [testing]
  notes: |
    End-to-end tests:
    - Simple template execution
    - Condition branching
    - Human gate flow
    - Crash recovery
    - Template composition

- id: meow-e11.2
  type: task
  title: "Write user documentation"
  priority: 1
  parent: meow-e11
  labels: [docs]
  notes: |
    - Quick start guide
    - Template authoring guide
    - CLI reference
    - Architecture overview

- id: meow-e11.3
  type: task
  title: "Create example workflows"
  priority: 2
  parent: meow-e11
  labels: [docs, examples]
  notes: |
    Example projects demonstrating MEOW:
    - Basic task list processing
    - TDD development workflow
    - Multi-step deployment
```

---

## Dependency Summary

```
Phase 1 (Foundation):
  E1 → E2 → E3

Phase 2 (Primitives):
  E3 → E4
  E3 → E5

Phase 3 (Advanced):
  E4 → E6 (Conditions)
  E4 + E5 → E7 (Checkpoint/Resume)
  E3 + E4 → E8 (Integration)

Phase 4 (Polish):
  E2 → E9 (Templates)
  E3 + E4 + E5 → E10 (CLI)
  E10 → E11 (Testing)
```

---

## Acceptance Criteria (MVP)

The MVP is complete when:

1. **`meow init`** creates proper directory structure and hooks
2. **`meow run work-loop`** executes tasks until completion
3. **`meow prime`** shows correct next task to agent
4. **`meow close`** properly closes beads
5. **Human gates** block until `meow approve` is run
6. **Crash recovery** works (kill orchestrator, restart, continue)
7. **TDD workflow** (`implement` template) guides through test-first development
8. **Documentation** covers basic usage

---

## Future Considerations

### Multi-Agent (Phase 2)
- Bead assignee routing
- Parallel `start` beads
- `foreach` directive
- Join point detection
- Resource locks

### Context Awareness (Optional)
- Built-in context threshold checking
- Automatic handoff triggers
- Token counting integration

### Gas Town Compatibility
- Template library for Gas Town patterns
- Polecat lifecycle as templates
- Witness/Refinery roles as templates
- Convoy tracking integration

---

*This document is the source of truth for MEOW Stack MVP implementation.*
