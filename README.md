# MEOW Stack

**Meow Executors Orchestrate Work** — A durable, recursive coordination language for AI agent orchestration.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Templates are programs. Steps are instructions. Executors run them.        │
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

MEOW provides **7 executors** that compose into arbitrary workflows:

| Executor | Who Runs | Purpose |
|----------|----------|---------|
| `shell` | Orchestrator | Run shell commands, capture outputs |
| `spawn` | Orchestrator | Start an agent in tmux |
| `kill` | Orchestrator | Stop an agent session |
| `expand` | Orchestrator | Inline another template |
| `branch` | Orchestrator | Conditional execution |
| `foreach` | Orchestrator | Iterate over lists |
| `agent` | Agent | Prompt agent, wait for `meow done` |

**Everything else—loops, parallel execution, human gates, context management, checkpoint/resume—emerges from composing these primitives.**

## Design Philosophy

> **MEOW is a coordination language, not a task tracker.**

MEOW templates are **programs** that coordinate agents. They are not task lists, not tickets, not issues. They are executable specifications of how work flows through a system of agents.

```
Templates = Programs (static, version-controlled)
Steps     = Instructions
Executors = Who runs each instruction
Outputs   = Data flowing between steps
Workflows = Running program instances
```

### Core Principles

**The Propulsion Principle** — The orchestrator drives agents forward. Agents don't poll—they receive prompts directly via tmux injection.

**Minimal Agent Exposure** — Agents see only what they need: a prompt and how to signal completion (`meow done`).

**Durable Execution** — All workflow state survives crashes. Simple YAML files, no database.

**Composition Over Complexity** — Complex behaviors emerge from simple primitives composed together.

### Agnosticism

**Task Tracking Agnostic** — MEOW doesn't care how you track work. Your workflow template tells agents how to interact with Jira, GitHub Issues, Beads, or nothing at all.

**Agent Agnostic** — MEOW supports any terminal-based AI agent through adapters. Claude Code, Aider, Cursor—just configuration.

## Quick Example

```bash
# Start a workflow
meow run work-loop --var agent=claude

# The orchestrator:
# 1. Parses template into steps
# 2. Spawns agent in tmux
# 3. Injects prompts as steps become ready
# 4. Agent works, calls `meow done` when finished
# 5. Orchestrator advances to next step
# 6. Loop continues until workflow completes
```

A simple work loop template:

```toml
# work-loop.meow.toml

[main]
name = "work-loop"

[main.variables]
agent = { required = true }

[[main.steps]]
id = "start-agent"
executor = "spawn"
agent = "{{agent}}"

[[main.steps]]
id = "select-task"
executor = "agent"
agent = "{{agent}}"
prompt = "Run `bd ready` and pick a task to work on."
needs = ["start-agent"]
[main.steps.outputs]
task_id = { required = true, type = "string" }

[[main.steps]]
id = "implement"
executor = "agent"
agent = "{{agent}}"
prompt = "Implement task {{select-task.outputs.task_id}} using TDD."
needs = ["select-task"]

[[main.steps]]
id = "check-more-work"
executor = "branch"
condition = "bd list --status=open | grep -q ."
needs = ["implement"]
[main.steps.on_true]
template = ".continue-loop"
[main.steps.on_false]
template = ".finalize"
```

## Data Flow

Steps communicate through outputs, like GitHub Actions:

```toml
# Capture from shell
[[main.steps]]
id = "get-branch"
executor = "shell"
command = "git branch --show-current"
[main.steps.outputs]
branch = { source = "stdout" }

# Capture from agent
[[main.steps]]
id = "select"
executor = "agent"
prompt = "Pick a task"
[main.steps.outputs]
task_id = { required = true, type = "string" }

# Reference in later steps
[[main.steps]]
id = "implement"
executor = "agent"
prompt = "Implement {{select.outputs.task_id}} on branch {{get-branch.outputs.branch}}"
```

## Documentation

**[MVP Specification v2](docs/MVP-SPEC-v2.md)** — The complete technical specification:

- All 7 executors with examples
- Template system and composition
- Agent adapter architecture
- IPC protocol
- Crash recovery
- Multi-agent coordination patterns

## Status

**Active Development** — Core orchestrator and executors being implemented.

## License

MIT

---

*"7 executors. Everything else is composition."*
