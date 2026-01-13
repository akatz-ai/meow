![shimmer_meow_logo](https://github.com/user-attachments/assets/7df96ff8-e305-473d-b1ed-8f3c7341ce2e)

**Meow Executors Orchestrate Work** — The Makefile of agent orchestration.

No Python. No cloud. No magic. Just tmux, YAML, and a binary.

## Why MEOW?

The AI agent orchestration space is full of frameworks: LangChain, CrewAI, Claude-Flow, cloud-managed agents. They offer sophisticated features—memory systems, vector databases, MCP tools, visual builders.

**MEOW takes the opposite approach.**

| Framework Approach | MEOW Approach |
|--------------------|---------------|
| Python SDK with dependencies | Single Go binary |
| Cloud services and databases | YAML files on disk |
| Agents as API endpoints | Agents as terminal processes |
| Framework-specific agent code | Any terminal agent, unchanged |
| Visual builders, dashboards | `git diff`-able TOML templates |

MEOW is for developers who use Claude Code or Aider, want multi-agent workflows, and don't want to adopt a framework ecosystem. If you need enterprise features or turnkey agent capabilities, look at [Claude-Flow](https://github.com/ruvnet/claude-flow) or [LangGraph](https://langchain-ai.github.io/langgraph/).

## Installation

**Quick install (Linux/macOS):**
```bash
curl -fsSL https://raw.githubusercontent.com/akatz-ai/meow-machine/main/install.sh | sh
```

**With Go:**
```bash
go install github.com/meow-stack/meow-machine/cmd/meow@latest
```

**From source:**
```bash
git clone https://github.com/akatz-ai/meow-machine
cd meow-machine
make install
```

**Manual download:**
Download pre-built binaries from [Releases](https://github.com/akatz-ai/meow-machine/releases).

## Getting Started

Initialize MEOW in your project:

```bash
cd your-project
meow init
```

This creates the following structure:

```
.meow/
├── config.toml      # Project configuration
├── AGENTS.md        # Guidelines for agents in workflows
├── templates/       # Workflow templates (starter templates included)
│   ├── simple.meow.toml
│   └── tdd.meow.toml
├── lib/             # Standard library templates
│   ├── agent-persistence.meow.toml  # Ralph Wiggum pattern
│   ├── claude-utils.meow.toml       # Context monitoring
│   ├── claude-events.meow.toml      # Hook configuration
│   └── worktree.meow.toml           # Git worktree helper
├── adapters/        # Agent adapter configs
│   ├── claude/
│   ├── codex/
│   └── opencode/
├── runs/            # Runtime state (gitignored)
└── logs/            # Per-run logs (gitignored)
```

The `AGENTS.md` file contains guidelines for AI agents working within workflows—session protocol, completion signals, and best practices. Include it in agent worktrees or reference from your project's `CLAUDE.md`.

Run your first workflow:

```bash
meow run .meow/templates/simple.meow.toml --var agent=my-agent
```

## How It Works

MEOW templates are programs. The orchestrator **pushes** prompts directly into running terminal sessions via tmux—it literally types into the agent's terminal and waits for `meow done`.

```
Templates = Programs (version-controlled TOML)
Steps     = Instructions
Executors = Who runs each instruction
Workflows = Running instances (runtime state in YAML)
```

### 7 Executors

| Executor | Purpose |
|----------|---------|
| `shell` | Run commands, capture outputs |
| `spawn` | Start an agent in tmux |
| `kill` | Stop an agent session |
| `expand` | Inline another template |
| `branch` | Conditional execution |
| `foreach` | Iterate over lists |
| `agent` | Prompt agent, wait for completion |

Loops, parallel execution, human gates, retries—all emerge from composing these primitives.

## Example

```bash
meow run work-loop.meow.toml --var agent=claude-1
```

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
prompt = "Pick a task from the backlog."
needs = ["start"]
[main.steps.outputs]
task_id = { required = true, type = "string" }

[[main.steps]]
id = "implement"
executor = "agent"
agent = "{{agent}}"
prompt = "Implement {{select-task.outputs.task_id}} using TDD."
needs = ["select-task"]

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "{{agent}}"
needs = ["implement"]
```

Steps communicate through outputs (like GitHub Actions):

```toml
[[main.steps]]
id = "get-branch"
executor = "shell"
command = "git branch --show-current"
[main.steps.outputs]
branch = { source = "stdout" }

[[main.steps]]
id = "implement"
executor = "agent"
prompt = "Work on {{get-branch.outputs.branch}}"
needs = ["get-branch"]
```

## Tradeoffs

**MEOW gives you:** Simplicity, no vendor lock-in, version-controlled workflows, works with any terminal agent, easy debugging (just tmux + YAML).

**MEOW does NOT give you:** Built-in memory/RAG, observability dashboards, managed deployment, guaranteed crash recovery (best-effort only).

This isn't a limitation—it's the point. MEOW coordinates; you build the rest.

## Documentation

**[MVP Specification](docs/MVP-SPEC-v2.md)** — Complete technical spec covering all 7 executors, template composition, adapters, IPC, and multi-agent patterns.

## Status

**Active Development** — Core orchestrator functional. See spec for implementation phases.

## License

MIT
