# MEOW Example Workflows

Ready-to-run workflows demonstrating common patterns.

## Getting Started

Copy any workflow to your project's `.meow/workflows/` directory:

```bash
# Initialize MEOW if you haven't already
meow init

# Copy an example
cp examples/simple.meow.toml .meow/workflows/

# Run it
meow run simple --var task="Fix the login bug"
```

## Examples

| Workflow | Description |
|----------|-------------|
| [`simple.meow.toml`](simple.meow.toml) | Minimal single-agent workflow |
| [`tdd.meow.toml`](tdd.meow.toml) | Test-driven development (red-green-refactor) |
| [`work-loop.meow.toml`](work-loop.meow.toml) | Iterative task selection loop |
| [`parallel-workers.meow.toml`](parallel-workers.meow.toml) | Multi-agent parallel execution with review |

## Patterns Demonstrated

### Single Agent
The simplest pattern—spawn an agent, give it work, clean up.
- See: `simple.meow.toml`

### Sequential Steps
Chain steps with `needs` to create a pipeline.
- See: `tdd.meow.toml` (write-test → make-pass → refactor → commit)

### Iterative Loops
Use `branch` with recursive `expand` to loop until a condition is met.
- See: `work-loop.meow.toml`

### Parallel Execution
Steps with the same dependencies run in parallel. Use multiple `needs` to join.
- See: `parallel-workers.meow.toml`

### Agent Persistence (Ralph Wiggum)
Monitor agents and nudge them if they stop unexpectedly.
- See: `parallel-workers.meow.toml` (uses `lib/agent-persistence`)

## Customization

These are starting points. Common modifications:

1. **Change the agent adapter** — Edit `adapter = "claude"` to use `codex`, `opencode`, etc.
2. **Add variables** — Define inputs in `[main.variables]`
3. **Capture outputs** — Use `[main.steps.outputs]` to pass data between steps
4. **Add persistence** — Expand `lib/agent-persistence#monitor` for long-running tasks

## Prerequisites

- MEOW CLI installed (`meow --version`)
- tmux installed
- An AI agent (Claude Code, Aider, etc.)
