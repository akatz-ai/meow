# Common MEOW Patterns

## Basic Work Loop

The simplest pattern: spawn agent, do work, kill agent.

```toml
[main]
name = "simple-task"

[main.vars]
task = { required = true }

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "worker"
adapter = "claude"

[[main.steps]]
id = "work"
executor = "agent"
agent = "worker"
needs = ["spawn"]
prompt = """
{{task}}

When done: meow done
"""

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "worker"
needs = ["work"]
```

## Ralph Wiggum (Agent Persistence)

Keep agents working even when they try to stop prematurely. Named after the Simpsons character who keeps going despite confusion.

```toml
[main]
name = "persistent-task"

[main.vars]
task = { required = true }

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "worker"
adapter = "claude"

[[main.steps]]
id = "setup-hooks"
executor = "expand"
workflow = "lib/claude-events#setup-stop-hook"
needs = ["spawn"]

[[main.steps]]
id = "monitor"
executor = "expand"
workflow = "lib/agent-persistence#monitor"
needs = ["spawn"]
[main.steps.vars]
main_step = "work"
agent = "worker"

[[main.steps]]
id = "work"
executor = "agent"
agent = "worker"
needs = ["spawn", "setup-hooks"]
prompt = "{{task}}\n\nWhen complete: meow done"

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "worker"
needs = ["work"]
```

How it works:
1. Claude's Stop hook runs `meow event agent-stopped`
2. Monitor step waits for this event
3. On event, monitor checks if main work step is done
4. If not done, nudges agent to continue
5. Loop until work step completes

## Worktree Isolation

Run work in an isolated git worktree.

```toml
[main]
name = "isolated-work"

[main.vars]
branch = { required = true }
task = { required = true }

[[main.steps]]
id = "worktree"
executor = "expand"
workflow = "lib/worktree#create"
[main.steps.vars]
branch = "{{branch}}"

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "worker"
adapter = "claude"
working_dir = "{{worktree.outputs.path}}"
needs = ["worktree"]

[[main.steps]]
id = "work"
executor = "agent"
agent = "worker"
needs = ["spawn"]
prompt = """
You are working in: {{worktree.outputs.path}}

{{task}}

When done: meow done
"""

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "worker"
needs = ["work"]

[main]
cleanup_always = "git worktree remove {{worktree.outputs.path}} --force"
```

## Context Monitoring

Watch for agent context size warnings and handle gracefully.

```toml
[[main.steps]]
id = "context-monitor"
executor = "expand"
workflow = "lib/claude-utils#context-monitor"
needs = ["spawn"]
[main.steps.vars]
agent = "worker"
main_step = "work"
threshold_percent = "80"
```

## Parallel Agents with Join

Run multiple agents in parallel, wait for all to complete.

```toml
[main]
name = "parallel-work"

[[main.steps]]
id = "spawn-a"
executor = "spawn"
agent = "agent-a"
adapter = "claude"

[[main.steps]]
id = "spawn-b"
executor = "spawn"
agent = "agent-b"
adapter = "claude"

[[main.steps]]
id = "work-a"
executor = "agent"
agent = "agent-a"
needs = ["spawn-a"]
prompt = "Do part A. When done: meow done --output result=X"

[[main.steps]]
id = "work-b"
executor = "agent"
agent = "agent-b"
needs = ["spawn-b"]
prompt = "Do part B. When done: meow done --output result=Y"

# Join: waits for both
[[main.steps]]
id = "combine"
executor = "shell"
needs = ["work-a", "work-b"]
command = "echo 'A: {{work-a.outputs.result}}, B: {{work-b.outputs.result}}'"

[[main.steps]]
id = "cleanup-a"
executor = "kill"
agent = "agent-a"
needs = ["combine"]

[[main.steps]]
id = "cleanup-b"
executor = "kill"
agent = "agent-b"
needs = ["combine"]
```

## Human Approval Gate

Pause workflow until human approves.

```toml
[main]
name = "with-approval"

[[main.steps]]
id = "prepare"
executor = "shell"
command = "generate-changes.sh"

[[main.steps]]
id = "review-gate"
executor = "branch"
needs = ["prepare"]
condition = "meow await-approval review"
on_true = "apply-changes"
on_false = "rollback"

[apply-changes]
[[apply-changes.steps]]
id = "apply"
executor = "shell"
command = "apply-changes.sh"

[rollback]
[[rollback.steps]]
id = "cancel"
executor = "shell"
command = "rm -rf pending-changes/"
```

Then from outside the workflow:
```bash
# Check what needs approval
meow status <run-id>

# Approve
meow approve <run-id> review

# Or reject
meow reject <run-id> review --reason "Changes look wrong"
```

## Output Chaining

Pass data between steps.

```toml
[[main.steps]]
id = "analyze"
executor = "agent"
agent = "analyzer"
needs = ["spawn"]
prompt = """
Analyze the codebase and identify issues.

When done: meow done --output issues=N --output priority=high/medium/low
"""

[main.steps.outputs]
issues = { type = "int" }
priority = { type = "string" }

[[main.steps]]
id = "decide"
executor = "branch"
needs = ["analyze"]
condition = "test {{analyze.outputs.issues}} -gt 0"
on_true = "fix-issues"
on_false = "report-clean"

[fix-issues]
[[fix-issues.steps]]
id = "fix"
executor = "agent"
agent = "fixer"
prompt = """
Found {{analyze.outputs.issues}} issues at {{analyze.outputs.priority}} priority.
Fix them all.
"""
```

## Foreach Iteration

Process a list of items.

```toml
[[main.steps]]
id = "list-files"
executor = "shell"
command = "find . -name '*.go' -type f"

[main.steps.shell_outputs]
files = { from = "stdout", split = "\n" }

[[main.steps]]
id = "process-each"
executor = "foreach"
needs = ["list-files"]
items_from = "list-files.outputs.files"
item_var = "file"
body = "process-file"

[process-file]
[[process-file.steps]]
id = "lint"
executor = "shell"
command = "golint {{file}}"
```

## Event-Driven Coordination

Coordinate steps via events rather than direct dependencies.

```toml
# Main work
[[main.steps]]
id = "work"
executor = "agent"
prompt = """
Work on the task.
If you need a review, run: meow event need-review --data file=X
Then wait for: meow await-event review-complete
"""

# Reviewer (runs in parallel)
[[main.steps]]
id = "reviewer"
executor = "branch"
condition = "meow await-event need-review"
on_true = "do-review"

[do-review]
[[do-review.steps]]
id = "review"
executor = "agent"
agent = "reviewer"
prompt = "Review the file. When done: meow event review-complete"
```
