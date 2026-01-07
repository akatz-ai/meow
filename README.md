# MEOW Stack

**Molecular Expression Of Work** — A primitives-first workflow system for durable AI agent orchestration.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  "6 primitives. Everything else is template composition."                   │
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

MEOW Stack provides **6 primitive bead types** that compose into arbitrary workflows:

| # | Primitive | Purpose |
|---|-----------|---------|
| 1 | `task` | Claude-executed work (with optional validated outputs) |
| 2 | `condition` | Branching, looping, and waiting (can block for gates) |
| 3 | `stop` | Kill agent session |
| 4 | `start` | Spawn agent (with optional resume from session) |
| 5 | `code` | Arbitrary shell execution (with output capture) |
| 6 | `expand` | Template composition |

**Everything else—`refresh`, `handoff`, `call`, loops, context management, human approval gates, checkpoint/resume—is template composition using these primitives.**

## Design Philosophy

> **The orchestrator is dumb; the templates are smart.**

The orchestrator is a simple dispatch loop that recognizes 6 bead types. All workflow complexity lives in composable TOML templates that users define and control.

- **No magic context thresholds** — Users define when to check context via `condition` beads
- **No hardcoded call semantics** — `call` is a template: checkpoint → stop → start → expand → stop → resume
- **No special gate type** — Gates are blocking `condition` beads: `meow wait-approve --bead xyz`
- **No built-in worktree management** — Users add `code` beads for git operations

## Quick Example

```bash
# Start a workflow
meow run work-loop

# The orchestrator:
# 1. Bakes template into beads
# 2. Spawns Claude in tmux
# 3. Watches for special bead types
# 4. Claude executes task beads, closes them
# 5. Orchestrator handles condition/stop/start/etc
# 6. Loop continues until all beads complete
```

A simple work loop template:

```toml
# .meow/templates/work-loop.toml
[meta]
name = "work-loop"

[[steps]]
id = "check-work"
type = "condition"
condition = "bd list --type=task --status=open | grep -q ."
on_true:
  template = "work-iteration"
on_false:
  template = "finalize"
```

## Documentation

**[MVP Specification](docs/MVP-SPEC.md)** — The complete technical specification including:

- All 6 primitive types with examples
- Orchestrator architecture and main loop
- Template system and composition patterns
- Agent lifecycle management
- Complete execution traces
- Implementation roadmap

## Related Projects

MEOW Stack builds on:

- **[Beads](https://github.com/steveyegge/beads)** — Git-backed issue tracker
- **[Beads Viewer](https://github.com/Dicklesworthstone/beads_viewer)** — TUI + robot mode for task scoring
- **[Ralph Wiggum](https://ghuntley.com/ralph/)** — Persistent iteration loop technique
- **[Gas Town](https://github.com/...)** — Multi-agent orchestrator (propulsion principle inspiration)

## Philosophy

> **6 primitives compose into any workflow.** Simple orchestrator, smart templates.

> **Beads survive crashes.** Any agent can resume where another left off.

> **The prompt never changes, but the world does.** Each iteration sees accumulated work.

> **Users control everything.** Context management, call semantics, gates—all user-defined.

## Status

**Design Phase** — This repository contains the architectural specification. See [MVP-SPEC.md](docs/MVP-SPEC.md) for implementation roadmap.

## License

MIT

---

*"It's just 6 primitives. But they compose into anything."*
