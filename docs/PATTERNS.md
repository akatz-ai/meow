# MEOW Patterns

Common patterns for MEOW workflows. These are battle-tested approaches implemented in the library templates.

---

## Basic Work Loop

A single agent that picks tasks and implements them in a loop.

```toml
[main]
name = "work-loop"

cleanup_on_success = """
git worktree remove .meow/worktrees/{{agent}} --force 2>/dev/null || true
"""

[main.variables]
agent = { required = true }

[[main.steps]]
id = "create-worktree"
executor = "shell"
command = """
git worktree add -b meow/{{agent}} .meow/worktrees/{{agent}} HEAD 2>/dev/null || true
echo ".meow/worktrees/{{agent}}"
"""

[main.steps.outputs]
path = { source = "stdout" }

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "{{agent}}"
workdir = "{{create-worktree.outputs.path}}"
needs = ["create-worktree"]

[[main.steps]]
id = "work"
executor = "agent"
agent = "{{agent}}"
prompt = """
Find and implement the next task.
When done, output whether there are more tasks.
"""
needs = ["spawn"]

[main.steps.outputs]
has_more = { required = true, type = "boolean" }

[[main.steps]]
id = "continue"
executor = "branch"
condition = "test '{{work.outputs.has_more}}' = 'true'"
needs = ["work"]

[main.steps.on_true]
template = ".work-iteration"
variables = { agent = "{{agent}}" }

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "{{agent}}"
needs = ["continue"]
```

---

## Ralph Wiggum (Agent Persistence)

Keep an agent working even when it stops unexpectedly. Uses event-based monitoring.

**Prerequisites:** Agent must emit `agent-stopped` events via its stop hook.

```toml
# Use the library template
[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "worker"
workdir = "{{worktree}}"

[[main.steps]]
id = "main-work"
executor = "agent"
agent = "worker"
prompt = "Implement the feature..."
needs = ["spawn"]

# Start persistence monitor in parallel
[[main.steps]]
id = "persistence"
executor = "expand"
template = "lib/agent-persistence#monitor"
needs = ["spawn"]

[main.steps.variables]
agent = "worker"
nudge_prompt = "Continue working. Run 'meow done' when finished."
check_step = "main-work"

# Kill depends on main work, NOT the monitor
[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "worker"
needs = ["main-work"]
```

**How it works:**
1. Monitor runs in parallel with main work
2. Waits for `agent-stopped` events with timeout
3. On event: re-injects nudge prompt via `fire_forget`
4. Checks if main step is done; loops if not
5. Self-terminates when main step completes

See `lib/agent-persistence.meow.toml` for the full implementation.

---

## Human Approval Gate

Block workflow until a human approves or rejects.

```toml
[[steps]]
id = "notify"
executor = "shell"
command = """
echo "=== REVIEW REQUIRED ==="
echo "Approve: meow approve {{workflow_id}} review-gate"
echo "Reject:  meow reject {{workflow_id}} review-gate --reason '...'"
"""

[[steps]]
id = "review-gate"
executor = "branch"
condition = "meow await-approval review-gate --timeout 24h"
needs = ["notify"]

[steps.on_true]
inline = []  # Continue on approval

[steps.on_false]
template = ".handle-rejection"
```

**Note:** The `await-approval` command blocks until `meow approve` or `meow reject` is called (or timeout).

---

## Parallel Agents with Join

Run multiple agents in parallel, then synchronize.

```toml
# Create worktrees in parallel
[[steps]]
id = "create-frontend-wt"
executor = "shell"
command = "git worktree add ... && echo path"

[steps.outputs]
path = { source = "stdout" }

[[steps]]
id = "create-backend-wt"
executor = "shell"
command = "git worktree add ... && echo path"

[steps.outputs]
path = { source = "stdout" }

# Spawn agents (can run in parallel since worktrees are independent)
[[steps]]
id = "spawn-frontend"
executor = "spawn"
agent = "frontend"
workdir = "{{create-frontend-wt.outputs.path}}"
needs = ["create-frontend-wt"]

[[steps]]
id = "spawn-backend"
executor = "spawn"
agent = "backend"
workdir = "{{create-backend-wt.outputs.path}}"
needs = ["create-backend-wt"]

# Work in parallel
[[steps]]
id = "frontend-work"
executor = "agent"
agent = "frontend"
prompt = "Implement the frontend..."
needs = ["spawn-frontend"]

[[steps]]
id = "backend-work"
executor = "agent"
agent = "backend"
prompt = "Implement the backend..."
needs = ["spawn-backend"]

# JOIN: waits for BOTH agents
[[steps]]
id = "integration"
executor = "agent"
agent = "frontend"
prompt = "Both done. Integrate and verify."
needs = ["frontend-work", "backend-work"]
```

**Key insight:** The join is implicit—any step with multiple `needs` waits for all of them.

---

## Dynamic Parallel (foreach)

Iterate over a runtime-generated list with parallel execution.

```toml
[[steps]]
id = "plan"
executor = "agent"
agent = "planner"
prompt = "Break this feature into subtasks. Output as JSON array."

[steps.outputs]
tasks = { required = true, type = "json" }

[[steps]]
id = "parallel-work"
executor = "foreach"
items = "{{plan.outputs.tasks}}"
item_var = "task"
index_var = "i"
template = ".worker-task"
max_concurrent = "3"
needs = ["plan"]

[steps.variables]
task_description = "{{task.description}}"
agent_id = "worker-{{i}}"

[[steps]]
id = "merge"
executor = "agent"
agent = "planner"
prompt = "All workers done. Merge the results."
needs = ["parallel-work"]  # Waits for ALL iterations (implicit join)
```

**Options:**
- `parallel = false` - Run iterations sequentially
- `join = false` - Don't wait for iterations (fire-and-forget)
- `max_concurrent = "N"` - Limit concurrent iterations

---

## Context Monitoring

Monitor agent context usage and trigger compaction when high.

```toml
[context-monitor]
internal = true

[[context-monitor.steps]]
id = "wait"
executor = "shell"
command = "sleep 30"

[[context-monitor.steps]]
id = "check-session"
executor = "branch"
condition = "tmux has-session -t meow-${MEOW_WORKFLOW}-{{agent}} 2>/dev/null"
needs = ["wait"]

[context-monitor.steps.on_false]
inline = []  # Session gone, exit

[context-monitor.steps.on_true]
template = ".check-context-level"

[check-context-level]
internal = true

[[check-context-level.steps]]
id = "get-percent"
executor = "shell"
command = """
# Screen-scrape tmux for context percentage
SESSION="meow-${MEOW_WORKFLOW}-{{agent}}"
PERCENT=$(tmux capture-pane -p -t "$SESSION" -S -100 2>/dev/null | \
    grep -oE '[0-9]+%' | tail -1 | tr -d '%')
echo "${PERCENT:-0}"
"""

[check-context-level.steps.outputs]
percent = { source = "stdout" }

[[check-context-level.steps]]
id = "check-threshold"
executor = "branch"
condition = "test {{get-percent.outputs.percent}} -ge 85"
needs = ["get-percent"]

[check-context-level.steps.on_true]
template = ".inject-compact"

[check-context-level.steps.on_false]
template = ".context-monitor"  # Loop

[inject-compact]
internal = true

[[inject-compact.steps]]
id = "compact"
executor = "agent"
agent = "{{agent}}"
prompt = "/compact"
mode = "fire_forget"

[[inject-compact.steps]]
id = "wait"
executor = "shell"
command = "sleep 60"
needs = ["compact"]

[[inject-compact.steps]]
id = "continue"
executor = "expand"
template = ".context-monitor"
needs = ["wait"]
```

---

## Fire-and-Forget Commands

Send commands to agents without waiting for completion.

```toml
# Send /compact without waiting
[[steps]]
id = "trigger-compact"
executor = "agent"
agent = "worker"
prompt = "/compact"
mode = "fire_forget"

# Send Escape to interrupt
[[steps]]
id = "interrupt"
executor = "agent"
agent = "worker"
prompt = "Escape"
mode = "fire_forget"
```

**Use cases:**
- Slash commands (`/compact`, `/clear`)
- Key presses (`Escape`)
- Any prompt where you don't need a response

---

## Conditional Branching

Run different paths based on a condition.

```toml
[[steps]]
id = "check-tests"
executor = "branch"
condition = "npm test"

[steps.on_true]
template = ".deploy"  # Tests pass, deploy

[steps.on_false]
template = ".fix-tests"  # Tests fail, fix them
```

**With timeout:**
```toml
[[steps]]
id = "wait-for-ci"
executor = "branch"
condition = "./scripts/wait-for-ci.sh"
timeout = "30m"

[steps.on_true]
inline = []  # CI passed

[steps.on_false]
template = ".handle-ci-failure"

[steps.on_timeout]
template = ".handle-ci-timeout"
```

---

## Idempotent Shell Commands

Shell commands should be safe to re-run (for crash recovery).

**Bad:**
```toml
command = "git worktree add -b meow/agent .meow/worktrees/agent HEAD"
# Fails if worktree exists
```

**Good:**
```toml
command = """
if [ ! -d ".meow/worktrees/agent" ]; then
  git worktree add -b meow/agent .meow/worktrees/agent HEAD
fi
echo ".meow/worktrees/agent"
"""
```

**Also good (suppress errors):**
```toml
command = """
git worktree add -b meow/agent .meow/worktrees/agent HEAD 2>/dev/null || true
echo ".meow/worktrees/agent"
"""
```

---

## Setting Up Claude Events

Configure Claude Code's Stop hook to emit MEOW events.

```toml
[[steps]]
id = "setup-hooks"
executor = "shell"
command = """
mkdir -p {{worktree}}/.claude
cat > {{worktree}}/.claude/settings.json << 'EOF'
{
  "hooks": {
    "Stop": [{"type": "command", "command": "meow event agent-stopped"}]
  }
}
EOF
"""

[[steps]]
id = "spawn"
executor = "spawn"
agent = "worker"
workdir = "{{worktree}}"
needs = ["setup-hooks"]
```

See `lib/claude-events.meow.toml` for a reusable template.

---

## Best Practices

1. **Cleanup on success only** - Preserve state on failure for debugging
2. **Kill depends on work, not monitors** - Let monitors self-terminate
3. **Use `needs` for explicit dependencies** - Don't rely on ordering
4. **Keep prompts goal-oriented** - Tell agents *what* to achieve, not *how*
5. **Test with simple workflows first** - Add complexity incrementally

---

*See `.meow/workflows/` for complete working examples.*
