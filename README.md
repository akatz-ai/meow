# MEOW Stack

**Terminal agent orchestration without the framework tax.**

No Python. No cloud. No magic. Just tmux, YAML, and a binary.

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                                                                             │
│   MEOW is the Makefile of agent orchestration.                              │
│                                                                             │
│   Simple. Universal. No magic.                                              │
│   Works with Claude Code, Aider, or any terminal agent.                     │
│   Your workflows are just version-controlled TOML files.                    │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## Why MEOW?

The AI agent orchestration space is exploding with frameworks: LangChain, CrewAI, Claude-Flow, cloud-managed agents. They offer sophisticated features—memory systems, vector databases, MCP tools, visual builders.

**MEOW takes the opposite approach.**

| Framework Approach | MEOW Approach |
|--------------------|---------------|
| Python SDK with dependencies | Single Go binary |
| Cloud services and databases | YAML files on disk |
| Agents as API endpoints | Agents as terminal processes |
| Framework-specific agent code | Any terminal agent, unchanged |
| Visual builders, dashboards | `git diff`-able TOML templates |
| Sophisticated memory/RAG | Bring your own (or don't) |

MEOW is for developers who:
- Use Claude Code or Aider in the terminal
- Want to orchestrate multi-agent workflows
- Don't want to learn a framework ecosystem
- Value understanding exactly what their system does
- Want workflows version-controlled alongside code

## The Model

MEOW templates are **programs**. The orchestrator runs them.

```
Templates = Programs (static, version-controlled TOML)
Steps     = Instructions
Executors = Who runs each instruction
Workflows = Running program instances (runtime state in YAML)
```

The orchestrator **pushes** prompts directly into running terminal sessions via tmux. It doesn't call APIs or invoke functions—it literally types into the agent's terminal and waits for `meow done`.

This means MEOW works with *any* terminal-based agent without modification.

## 7 Executors, Everything Else is Composition

| Executor | Who Runs | Purpose |
|----------|----------|---------|
| `shell` | Orchestrator | Run shell commands, capture outputs |
| `spawn` | Orchestrator | Start an agent in tmux |
| `kill` | Orchestrator | Stop an agent session |
| `expand` | Orchestrator | Inline another template |
| `branch` | Orchestrator | Conditional execution |
| `foreach` | Orchestrator | Iterate over lists |
| `agent` | Agent | Prompt agent, wait for completion |

Loops, parallel execution, human gates, retries, dynamic iteration—all emerge from composing these primitives. No special syntax, no hidden complexity.

## Quick Example

```bash
# Run a workflow
meow run work-loop.meow.toml --var agent=claude-1

# What happens:
# 1. Template parsed into steps
# 2. Agent spawned in tmux session
# 3. Prompts injected as steps become ready
# 4. Agent works, signals completion with `meow done`
# 5. Orchestrator advances, injects next prompt
# 6. Repeat until workflow completes
```

A simple template:

```toml
# work-loop.meow.toml

[main]
name = "work-loop"

[main.variables]
agent = { required = true }

[[main.steps]]
id = "start"
executor = "spawn"
agent = "{{agent}}"

[[main.steps]]
id = "select-task"
executor = "agent"
agent = "{{agent}}"
prompt = "Pick a task from the backlog to work on."
needs = ["start"]
[main.steps.outputs]
task_id = { required = true, type = "string" }

[[main.steps]]
id = "implement"
executor = "agent"
agent = "{{agent}}"
prompt = "Implement task {{select-task.outputs.task_id}} using TDD."
needs = ["select-task"]

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "{{agent}}"
needs = ["implement"]
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
needs = ["select", "get-branch"]
```

## What MEOW Doesn't Do

MEOW is intentionally minimal. It does not include:

- **Memory/RAG systems** — Use your agent's built-in memory, or add a shell step
- **Vector databases** — If you need them, call them from shell steps
- **Visual workflow builders** — Templates are text files; use your editor
- **Cloud orchestration** — Runs locally; you own your state
- **Agent-specific features** — Adapters handle agent differences

This isn't a limitation—it's the point. MEOW coordinates; you build the rest.

## Honest Tradeoffs

**MEOW gives you:**
- Complete understanding of your system
- No vendor lock-in or cloud dependency
- Version-controlled workflows
- Works with any terminal agent
- Simple debugging (it's just tmux sessions and YAML)

**MEOW does NOT give you:**
- Sophisticated agent capabilities out of the box
- Production observability dashboards
- Managed scaling and deployment
- Guaranteed crash recovery (best-effort resume only)

If you need enterprise features, managed infrastructure, or turnkey agent capabilities, look at [Claude-Flow](https://github.com/ruvnet/claude-flow), [LangGraph](https://langchain-ai.github.io/langgraph/), or cloud platforms.

If you want the Makefile of agent orchestration—simple, universal, no magic—MEOW is for you.

## Documentation

**[MVP Specification v2](docs/MVP-SPEC-v2.md)** — The complete technical specification:

- All 7 executors with detailed examples
- Template system and composition patterns
- Agent adapter architecture
- IPC protocol (Unix sockets, JSON)
- Best-effort crash resume
- Multi-agent coordination patterns
- Error handling strategies

## Status

**Active Development** — Core orchestrator and executors functional. See the spec for implementation phases.

## License

MIT

---

*"The Makefile of agent orchestration."*
