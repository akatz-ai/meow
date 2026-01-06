# MEOW Stack

**Molecular Expression Of Work** — A recursive, composable workflow system for durable AI agent orchestration.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  "Everything is a molecule. Loops are molecules. Gates are molecules.      │
│   The whole system is molecules all the way down."                         │
└─────────────────────────────────────────────────────────────────────────────┘
```

## The Problem

AI coding agents are powerful but fragile:

- **Context amnesia**: Sessions end, context is lost, work must be re-explained
- **No durability**: Crashes lose state; resumption requires human intervention
- **Unstructured execution**: Agents wander without clear workflow discipline
- **Human oversight gaps**: No natural checkpoints for review
- **Scaling chaos**: Multiple agents step on each other without coordination

## The Solution

MEOW Stack introduces a **recursive molecule architecture** where:

1. **Everything is a molecule** — Even loops and gates are molecules
2. **Molecules contain beads** — Each bead is a step in a workflow
3. **Beads can expand into molecules** — Recursive composition
4. **State is git-backed** — Survives crashes, enables resumption anywhere
5. **Human gates are first-class** — Built into the workflow, not bolted on

## Core Concepts

### The Molecule Stack

At any point during execution, there's a **stack** of active molecules (like a call stack):

```
┌─────────────────────────────────────────┐
│ outer-loop-001        [step: run-inner] │ ← Orchestration loop
├─────────────────────────────────────────┤
│ meta-mol-001          [step: task-1]    │ ← Batch of related work
├─────────────────────────────────────────┤
│ impl-task-1-001       [step: implement] │ ← TDD workflow (CURRENT)
└─────────────────────────────────────────┘
```

When `impl-task-1-001` completes → pop → resume `meta-mol-001` → continue to `task-2`

### Four Layers of Work

```
LAYER 0: Feature Idea
    ↓ (human + Claude planning session)
LAYER 1: Epics & Tasks (beads with dependencies)
    ↓ (outer loop selects work, bakes meta-molecule)
LAYER 2: Meta-Molecule (task batches + human gates)
    ↓ (each task expands via template)
LAYER 3: Step Molecules (TDD, test-suite, etc.)
    ↓ (atomic steps executed directly)
LAYER 4: Atomic Execution
```

### Templates

Reusable workflow patterns defined in TOML:

```toml
# .beads/templates/implement.toml
[meta]
name = "implement"
description = "TDD implementation workflow"

[[steps]]
id = "load-context"
description = "Load relevant files and understand the task"

[[steps]]
id = "write-tests"
description = "Write failing tests that define success criteria"
needs = ["load-context"]

[[steps]]
id = "verify-fail"
description = "Run tests and verify they fail"
needs = ["write-tests"]

[[steps]]
id = "implement"
description = "Write code to make tests pass"
needs = ["verify-fail"]

[[steps]]
id = "commit"
description = "Commit with descriptive message"
needs = ["implement"]
```

### The Execution Loop

Built on [Ralph Wiggum](https://ghuntley.com/ralph/) — a persistent iteration loop:

1. Claude receives prompt + sees accumulated work in files
2. Executor checks molecule stack → finds current step
3. If step has template → push child molecule, descend
4. If step is atomic → execute directly
5. If step is gate → pause loop, await human
6. If molecule complete → pop stack, ascend to parent
7. Loop continues until all work done

## Quick Example

```bash
# 1. Human provides feature idea
meow plan "Add user authentication with OAuth support"

# 2. Claude breaks into epics + tasks (stored as beads)
#    → Epic: User registration
#    → Epic: Login/logout
#    → Epic: OAuth providers
#    Each with 2-3 child tasks

# 3. Start the MEOW loop
meow start

# 4. Outer loop runs:
#    - Analyzes project with `bv --robot-triage`
#    - Picks highest-impact work (Epic 1: registration)
#    - Bakes meta-molecule: [task-1, task-2, close-epic, test-suite, human-gate]
#    - Descends into task-1 → expands to implement template
#    - Executes TDD steps: load-context → write-tests → verify-fail → implement → commit
#    - Ascends, continues to task-2
#    - ...eventually hits human-gate

# 5. Human reviews at gate
meow approve  # or: bd close <gate-id>

# 6. Loop continues with next batch of work
#    Until all epics complete
```

## Why This Architecture?

### Durability
The molecule stack lives in beads (git-backed). After a crash:
```bash
bd mol stack  # Shows exactly where you were
bd ready      # Shows next step to execute
# Resume immediately
```

### Composability
Templates can reference other templates. New workflow = new template file:
```toml
[[steps]]
id = "implement-feature"
template = "implement"  # Expands to TDD workflow

[[steps]]
id = "deploy-staging"
template = "deploy"     # Expands to deployment workflow
needs = ["implement-feature"]
```

### Human-in-the-Loop
Gates aren't special — they're molecules with blocking steps:
```toml
# human-gate template
[[steps]]
id = "await-approval"
type = "blocking-gate"  # Pauses loop until human closes
```

### Intelligent Task Selection
The outer loop uses [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer) for scoring:
- PageRank-weighted importance
- Betweenness centrality (bottleneck detection)
- Unblock count (what does this enable?)
- Critical path analysis

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           MEOW STACK ARCHITECTURE                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                         MOLECULE EXECUTOR                             │ │
│  │  • Manages molecule stack (push/pop)                                  │ │
│  │  • Dispatches steps to templates                                      │ │
│  │  • Handles loop restart semantics                                     │ │
│  │  • Integrates with Ralph Wiggum stop-hook                             │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                    ↓                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                         TEMPLATE REGISTRY                             │ │
│  │  .beads/templates/                                                    │ │
│  │  ├── outer-loop.toml      # Master orchestration loop                 │ │
│  │  ├── implement.toml       # TDD workflow                              │ │
│  │  ├── test-suite.toml      # Comprehensive testing                     │ │
│  │  ├── human-gate.toml      # Human review checkpoint                   │ │
│  │  ├── analyze-pick.toml    # Task selection with bv                    │ │
│  │  └── bake-meta.toml       # Dynamic meta-molecule creation            │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                    ↓                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                         BEADS STORAGE                                 │ │
│  │  .beads/                                                              │ │
│  │  ├── issues.jsonl         # All beads (git-backed)                    │ │
│  │  ├── beads.db             # SQLite cache (gitignored)                 │ │
│  │  └── molecules/           # Active molecule state                     │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                    ↓                                        │
│  ┌───────────────────────────────────────────────────────────────────────┐ │
│  │                      BEADS VIEWER (bv)                                │ │
│  │  • bv --robot-triage      # Ranked task recommendations               │ │
│  │  • bv --robot-next        # Single top pick                           │ │
│  │  • PageRank, betweenness, critical path analysis                      │ │
│  └───────────────────────────────────────────────────────────────────────┘ │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Documentation

- **[Architecture](docs/ARCHITECTURE.md)** — Full technical design
- **[Execution Model](docs/EXECUTION-MODEL.md)** — How the executor works
- **[Templates](docs/TEMPLATES.md)** — Template system reference
- **[Implementation Roadmap](docs/IMPLEMENTATION-ROADMAP.md)** — Build plan

## Related Projects

MEOW Stack builds on:

- **[Beads](https://github.com/steveyegge/beads)** — Git-backed issue tracker with molecule support
- **[Beads Viewer](https://github.com/Dicklesworthstone/beads_viewer)** — TUI + robot mode for task scoring
- **[Ralph Wiggum](https://ghuntley.com/ralph/)** — Persistent iteration loop technique
- **[Gas Town](https://github.com/...)** — Multi-agent orchestrator (inspiration for propulsion principle)

## Philosophy

> **Molecules survive crashes. Any agent can resume where another left off.**

> **The prompt never changes, but the world does.** Each iteration sees accumulated work.

> **Human gates aren't interruptions — they're workflow steps.** Built-in, not bolted on.

> **Everything is a molecule.** Uniform semantics enable arbitrary composition.

## Status

🚧 **Design Phase** — This repository contains the architectural specification. Implementation is planned in phases:

1. Template expansion in beads CLI
2. Molecule stack management
3. Loop/restart semantics
4. Gate integration
5. Claude Code skill packaging

## License

MIT

---

*"It's a Bash loop. But it's also a molecule. Which contains molecules. All the way down."*
