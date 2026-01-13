# MEOW Agent Guidelines

This document is for AI agents working within MEOW workflows. Include this in your agent's worktree CLAUDE.md or reference it from your project's agent configuration.

## Session Protocol

When working in a MEOW workflow, your shell environment includes:

| Variable | Purpose |
|----------|---------|
| `MEOW_AGENT` | Your agent identifier |
| `MEOW_WORKFLOW` | Current workflow ID |
| `MEOW_STEP` | Current step you're executing |
| `MEOW_ORCH_SOCK` | IPC socket (if set, you're in a MEOW context) |

## Completion Protocol

**CRITICAL:** Always signal completion when your task is done:

```bash
meow done
```

The orchestrator waits for this signal. Without it, the workflow hangs.

### Passing Output Data

If downstream steps need data from your work:

```bash
# Key-value pairs
meow done --output result=success --output count=42

# JSON for complex data
meow done --output-json '{"files": ["a.go", "b.go"], "status": "clean"}'

# With notes (for logging)
meow done --notes "Implemented feature X with 3 new functions"
```

### What Counts as "Done"

- Task completed successfully
- Task failed but you've done what you can
- You're blocked and need external input

When in doubt, call `meow done`. The workflow can handle what comes next.

## Event Protocol

For event-driven coordination patterns:

```bash
# Emit an event (non-blocking, fire-and-forget)
meow event task-blocked --data reason="need API key"

# Wait for an event (blocking until event or timeout)
meow await-event approval-granted --timeout 5m
```

Events are useful for:
- Signaling to parallel monitoring processes
- Coordinating between agents
- Human approval gates

## Working in Worktrees

If you're in a git worktree (common for isolated work):

1. Your working directory is separate from main repo
2. Changes are on a feature branch
3. Commits stay local until merged
4. The workflow handles cleanup

Check `.meow-branch` in your working directory to see which branch you're on.

## Common Patterns

### Standard Task Completion

```
1. Understand the task from your prompt
2. Implement the solution
3. Run tests if applicable
4. Commit changes with clear messages
5. Call: meow done
```

### Task with Output

```
1. Do the analysis/work
2. Identify key results
3. Call: meow done --output key=value
```

### Blocked or Uncertain

```
1. Describe what you've done
2. Call: meow done --notes "Blocked on X, completed Y"
```

## Troubleshooting

### meow done does nothing

Check if `MEOW_ORCH_SOCK` is set:
```bash
echo $MEOW_ORCH_SOCK
```

If empty, you're not in a MEOW workflow. The command is a no-op by design.

### Agent keeps getting nudged

The persistence monitor (Ralph Wiggum) nudges you when you stop without calling `meow done`. Make sure to call it when your task is complete.

### Lost context on what to do

Check your step ID and look at the workflow:
```bash
echo $MEOW_STEP
echo $MEOW_WORKFLOW
```

## Best Practices

1. **Call meow done promptly** - Don't leave the orchestrator waiting
2. **Use outputs for data flow** - Pass structured data to downstream steps
3. **Commit before done** - Ensure your work is saved
4. **Check environment first** - Verify you're in a MEOW context if behavior seems odd
5. **Keep prompts goal-oriented** - Focus on the task, not the workflow mechanics
