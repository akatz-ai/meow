# MEOW Workflow Syntax Guide

Workflows are TOML files defining agent orchestration. They live in `.meow/workflows/`.

## Basic Structure

```toml
# Workflow metadata
[main]
name = "my-workflow"
description = "What this workflow does"

# Workflow variables
[main.vars]
required_var = { required = true }
optional_var = { default = "value" }

# Steps (execute based on dependencies)
[[main.steps]]
id = "step-1"
executor = "shell"
command = "echo hello"

[[main.steps]]
id = "step-2"
executor = "agent"
needs = ["step-1"]
prompt = "Do something"
```

## Variables

### Declaration

```toml
[main.vars]
# Required - run fails if not provided
task = { required = true }

# Optional with default
timeout = { default = "30m" }
branch = { default = "main" }

# Description for documentation
verbose = { default = false, description = "Enable verbose output" }
```

### Usage

Variables are interpolated with `{{name}}` syntax:

```toml
[[main.steps]]
id = "work"
executor = "agent"
prompt = """
Task: {{task}}
Branch: {{branch}}
"""
```

### Providing Values

```bash
meow run my-workflow --var task="Fix bug #123" --var branch=hotfix
```

## The 7 Executors

### shell

Run a shell command. Orchestrator executes it directly.

```toml
[[main.steps]]
id = "build"
executor = "shell"
command = "go build ./..."
working_dir = "/path/to/project"  # optional
```

**Async execution:** Shell commands run in goroutines. They don't block other steps.

### spawn

Start an agent in a tmux session.

```toml
[[main.steps]]
id = "start-agent"
executor = "spawn"
agent = "main"           # agent name
adapter = "claude"       # which adapter to use
working_dir = "{{dir}}"  # optional
```

Creates tmux session: `meow-<run-id>-<agent-name>`

### kill

Terminate an agent session.

```toml
[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "main"
needs = ["work-complete"]
```

### expand

Inline another workflow's steps.

```toml
[[main.steps]]
id = "setup"
executor = "expand"
workflow = "lib/agent-persistence#monitor"
[main.steps.vars]
agent = "main"
```

Workflow references:
- `.local` - another workflow in same file
- `lib/name` - from .meow/lib/
- `lib/name#workflow` - specific workflow in file
- `path/to/file` - relative to workflows/
- `path/to/file#workflow` - specific workflow

Expanded steps get prefixed: `setup.original-step-id`

### branch

Conditional execution based on command exit code.

```toml
[[main.steps]]
id = "check"
executor = "branch"
condition = "test -f config.toml"
on_true = "use-config"       # expand this if exit 0
on_false = "use-defaults"    # expand this if exit non-0
```

**Async execution:** Condition runs in goroutine.

Common patterns:
```toml
# Human gate
condition = "meow await-approval deploy-gate"

# Event-driven
condition = "meow await-event agent-stopped --timeout 30s"

# Status check
condition = "meow step-status main-work | grep -q done"
```

### foreach

Iterate over a list.

```toml
[[main.steps]]
id = "process-files"
executor = "foreach"
items = ["a.go", "b.go", "c.go"]
item_var = "file"
body = "process-single"  # workflow to expand per item

# Or from previous step output
items_from = "list-files.outputs.files"
```

Creates steps: `process-files.0`, `process-files.1`, etc.

### agent

Send prompt to agent and wait for `meow done`.

```toml
[[main.steps]]
id = "implement"
executor = "agent"
agent = "main"
needs = ["spawn"]
prompt = """
Implement the feature described in TASK.md.

When complete, run: meow done --output status=success
"""
timeout = "30m"  # optional
```

## Output Capture

### Shell Outputs

Capture stdout/stderr from shell commands:

```toml
[[main.steps]]
id = "list-files"
executor = "shell"
command = "find . -name '*.go' -type f"

[main.steps.shell_outputs]
files = { from = "stdout", split = "\n" }
```

### Agent Outputs

Capture values from `meow done --output`:

```toml
[[main.steps]]
id = "analyze"
executor = "agent"
prompt = "Analyze and report. Run: meow done --output count=N --output status=X"

[main.steps.outputs]
count = { type = "int" }
status = { type = "string" }
```

### Referencing Outputs

Use outputs in later steps:

```toml
[[main.steps]]
id = "report"
executor = "shell"
needs = ["analyze"]
command = "echo 'Found {{analyze.outputs.count}} items with status {{analyze.outputs.status}}'"
```

## Dependencies

Steps execute when all `needs` are satisfied:

```toml
[[main.steps]]
id = "step-a"
executor = "shell"
command = "echo A"

[[main.steps]]
id = "step-b"
executor = "shell"
command = "echo B"

[[main.steps]]
id = "step-c"
executor = "shell"
needs = ["step-a", "step-b"]  # waits for both
command = "echo C"
```

Steps with no `needs` start immediately (parallel by default).

## Error Handling

```toml
[[main.steps]]
id = "risky"
executor = "shell"
command = "might-fail.sh"
on_error = "continue"  # or "fail" (default)
```

## Cleanup Scripts

Run commands after run completes:

```toml
[main]
name = "with-cleanup"
cleanup_on_success = "rm -rf /tmp/work"
cleanup_on_failure = "echo 'Failed!' | notify"
cleanup_always = "docker stop test-container"
```

## Complete Example

```toml
[main]
name = "feature-implementation"
description = "Implement a feature with AI assistance"

[main.vars]
task = { required = true }
branch = { default = "feature" }

[[main.steps]]
id = "create-worktree"
executor = "expand"
workflow = "lib/worktree#create"
[main.steps.vars]
branch = "{{branch}}"

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "main"
adapter = "claude"
needs = ["create-worktree"]

[[main.steps]]
id = "setup-hooks"
executor = "expand"
workflow = "lib/claude-events#setup-stop-hook"
needs = ["spawn"]

[[main.steps]]
id = "persist"
executor = "expand"
workflow = "lib/agent-persistence#monitor"
needs = ["spawn"]
[main.steps.vars]
main_step = "implement"

[[main.steps]]
id = "implement"
executor = "agent"
agent = "main"
needs = ["spawn", "setup-hooks"]
prompt = """
Task: {{task}}

Work in the worktree at {{create-worktree.outputs.path}}.

When complete, run:
meow done --output status=success
"""
timeout = "1h"

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "main"
needs = ["implement"]
```

## Step ID Rules

1. Must be unique within workflow
2. **Cannot contain dots** - dots are reserved for expansion prefixes
3. Use kebab-case: `my-step-id`
4. Expanded steps get prefixed: `parent.child`
