---
name: meow
description: MEOW (Meow Executors Orchestrate Work) agent orchestration tool. Use when working with projects containing a .meow/ directory, creating or editing .meow.toml workflow files, running meow CLI commands (run, status, stop, resume, ls), monitoring agents via tmux sessions, setting up new MEOW projects with meow init, or debugging run execution issues.
---

# MEOW Orchestration Skill

## Overview

MEOW coordinates AI agents through tmux sessions and TOML workflows. Key principles:
- **Dumb orchestrator, smart workflows** - orchestrator routes events; workflows define behavior
- **Local-first** - no cloud, no Python dependencies, just Go + tmux
- **7 executors only** - shell, spawn, kill, expand, branch, foreach, agent

## Quick Start

### Installation

```bash
# Quick install (Linux/macOS)
curl -fsSL https://raw.githubusercontent.com/akatz-ai/meow/main/install.sh | sh

# With Go
go install github.com/akatz-ai/meow/cmd/meow@latest

# Or clone and build
git clone https://github.com/akatz-ai/meow
cd meow && make install
```

### Initialize Project

```bash
meow init
```

Creates:
```
.meow/
├── config.toml      # Project configuration
├── workflows/       # Your workflow definitions
├── lib/             # Standard library (linked)
├── adapters/        # Agent adapters
├── runs/            # Runtime state (gitignored)
└── logs/            # Execution logs (gitignored)
```

### Run a Workflow

```bash
# Foreground (see output)
meow run my-workflow

# Background (daemonized)
meow run my-workflow -d

# With variables
meow run my-workflow --var task="Fix the bug" --var branch=feature-x
```

### Monitor

```bash
# Current runs
meow status

# Watch mode (live updates)
meow status --watch

# List all runs
meow ls

# List with filters
meow ls --status running
meow ls --stale
```

### Stop a Run

```bash
meow stop <run-id>
```

## Project Structure

| Directory | Purpose |
|-----------|---------|
| `workflows/` | Your workflow definitions (.meow.toml files) |
| `lib/` | Standard library workflows (agent-persistence, claude-utils, etc.) |
| `adapters/` | Agent-specific adapters (how to spawn different agents) |
| `runs/` | Active run state (ephemeral, gitignored) |
| `logs/` | Execution logs per run (gitignored) |

## Run Lifecycle

```
Workflow (.meow.toml)
       |
       v
   meow run
       |
       v
  Orchestrator (reads workflow, manages state)
       |
       +---> spawn agent(s) in tmux
       |
       +---> send prompts via agent steps
       |
       +---> wait for `meow done` calls
       |
       +---> route events between steps
       |
       v
  Run complete (all steps done)
```

### Step Status Lifecycle

```
pending --> running --> completing --> done
               |            |
               |            +---> (back to running if validation fails)
               |
               +---> failed
```

## The 7 Executors

| Executor | Run By | Completes When |
|----------|--------|----------------|
| `shell` | Orchestrator | Command exits |
| `spawn` | Orchestrator | Agent session started |
| `kill` | Orchestrator | Agent session terminated |
| `expand` | Orchestrator | Workflow steps inserted |
| `branch` | Orchestrator | Condition evaluated, branch expanded |
| `foreach` | Orchestrator | All iterations complete |
| `agent` | Agent | Agent calls `meow done` |

## Agent Communication

### Environment Variables

When inside a MEOW run, agents have access to:

| Variable | Description |
|----------|-------------|
| `MEOW_AGENT` | Agent name |
| `MEOW_WORKFLOW` | Run ID |
| `MEOW_STEP` | Current step ID |
| `MEOW_ORCH_SOCK` | Socket path for IPC |

### Commands Agents Can Call

```bash
# Signal step completion
meow done

# With output data (passed to downstream steps)
meow done --output result=success --output count=42

# With JSON output
meow done --output-json '{"files": ["a.go", "b.go"]}'

# Emit an event (for event-driven patterns)
meow event task-blocked --data reason="waiting for approval"

# Wait for an event
meow await-event approval-granted --timeout 5m

# Check another step's status
meow step-status setup-step
```

**Note:** `meow done` and `meow event` are no-ops when `MEOW_ORCH_SOCK` is not set. This allows the same hooks to work inside and outside MEOW.

## Monitoring Agents

### Tmux Sessions

Agents run in tmux sessions named: `meow-<run-id>-<agent-name>`

```bash
# List all meow sessions
tmux ls | grep meow-

# Attach to watch an agent
tmux attach -t meow-abc123-main-agent

# Detach without stopping: Ctrl-b d
```

### CLI Commands

```bash
# Active runs with step status
meow status

# Live updates
meow status --watch

# JSON output for scripting
meow status --json

# List agent sessions
meow agents

# Execution trace (debugging)
meow trace <run-id>
```

## Standard Library

The lib/ directory contains reusable workflows.

### Agent Persistence (Ralph Wiggum Pattern)

Keep agents working even when they "finish" prematurely:

```toml
[[main.steps]]
id = "persist"
executor = "expand"
workflow = "lib/agent-persistence#monitor"
needs = ["spawn-agent"]
```

How it works:
1. Agent's stop hook emits `meow event agent-stopped`
2. Monitor workflow waits via `meow await-event`
3. On event, monitor nudges agent to continue
4. Loop until main task completes

### Claude Events Setup

Configure Claude's hooks to emit MEOW events:

```toml
[[main.steps]]
id = "setup-hooks"
executor = "expand"
workflow = "lib/claude-events#setup-stop-hook"
needs = ["spawn"]
```

### Context Monitor

Watch for context size warnings:

```toml
[[main.steps]]
id = "context-watch"
executor = "expand"
workflow = "lib/claude-utils#context-monitor"
needs = ["spawn"]
```

### Worktree Creation

Isolate work in a git worktree:

```toml
[[main.steps]]
id = "create-worktree"
executor = "expand"
workflow = "lib/worktree#create"
[main.steps.vars]
branch = "{{branch}}"
```

## Basic Workflow Structure

```toml
# Workflow metadata
[main]
name = "my-workflow"
description = "Does something useful"

# Variables (required or with defaults)
[main.vars]
task = { required = true }
timeout = { default = "30m" }

# Steps execute based on dependencies (needs)
[[main.steps]]
id = "start-agent"
executor = "spawn"
agent = "main"
adapter = "claude"

[[main.steps]]
id = "do-work"
executor = "agent"
agent = "main"
needs = ["start-agent"]
prompt = """
Your task: {{task}}

When complete, run: meow done --output result=success
"""

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "main"
needs = ["do-work"]
```

## Teaching Your Agent

Create `AGENTS.md` or add to `CLAUDE.md` to teach agents the MEOW protocol:

```markdown
## MEOW Run Protocol

When working in a MEOW run:

1. Check if `MEOW_ORCH_SOCK` is set (indicates MEOW context)
2. Complete your assigned task fully
3. Call `meow done` when finished
4. Use `--output key=value` to pass data to downstream steps
5. If blocked, emit `meow event <type>` with context

Important: ALWAYS call `meow done` when your task is complete.
The orchestrator waits for this signal.
```

## TOML String Escaping

MEOW workflows are TOML files. Understanding TOML string escaping is critical to avoid syntax errors.

### String Types

| Type | Syntax | Escapes Processed | Use For |
|------|--------|-------------------|---------|
| Basic | `"..."` | Yes | Short strings |
| Multi-line Basic | `"""..."""` | Yes | Prompts, scripts |
| Literal | `'...'` | No | Regex, paths |
| Multi-line Literal | `'''...'''` | No | Complex regex |

### Common Pitfall: Invalid Escapes

In basic strings (`"` or `"""`), TOML only recognizes these escapes:
- `\\` → backslash
- `\n` → newline
- `\t` → tab
- `\r` → carriage return
- `\"` → quote
- `\uXXXX` → unicode

**Invalid escapes cause parse errors:**
```toml
# BAD - \? is not a valid TOML escape
command = "grep -E '^\?\?' file.txt"

# GOOD - double the backslash
command = "grep -E '^\\?\\?' file.txt"

# BETTER - use literal string (no escaping needed)
command = 'grep -E "^\?\?" file.txt'
```

### Regex in Shell Commands

When embedding regex in shell commands, you often need double-escaping:

```toml
# In TOML """ string, to get literal \. in bash:
command = """
grep -E '\\.txt$' files.txt       # WRONG - \. is invalid TOML escape
grep -E '\\\\.txt$' files.txt     # RIGHT - \\\\ becomes \\ in bash, which is \.
"""

# Easier: use literal strings where possible
command = '''
grep -E '\.txt$' files.txt        # Works! No TOML escaping in '''
'''
```

### Quick Reference

| You Want | In `"..."` | In `'...'` |
|----------|------------|------------|
| Literal `\` | `\\` | `\` |
| Literal `\n` (not newline) | `\\n` | `\n` |
| Regex `\d+` | `\\d+` | `\d+` |
| Regex `\?` | `\\?` | `\?` |
| Regex `\.` | `\\.` | `\.` |

### Pro Tip

For complex shell commands with regex, prefer multi-line literal strings (`'''`):

```toml
[[main.steps]]
id = "check"
executor = "shell"
command = '''
# No TOML escaping needed in literal strings!
CHANGES=$(git status --porcelain | grep -v -E '^\?\? \.beads/' || true)
if [ -n "$CHANGES" ]; then
    echo "Uncommitted changes found"
    exit 1
fi
'''
```

## Key Constraints

1. **Step IDs cannot contain dots** - dots are reserved for expansion prefixes
2. **Gates are not an executor** - use `branch` with `meow await-approval`
3. **Single writer** - all state changes go through the orchestrator
4. **7 executors only** - no custom executors

## References

- [CLI Commands Reference](references/commands.md)
- [Workflow Syntax Guide](references/workflows.md)
- [Common Patterns](references/patterns.md)
- [Troubleshooting](references/troubleshooting.md)
