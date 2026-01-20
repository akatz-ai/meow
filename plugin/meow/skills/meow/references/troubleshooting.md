# MEOW Troubleshooting

## Stale Workflows

**Symptom:** Old workflows listed, no activity.

**Diagnose:**
```bash
meow ls --stale
```

**Fix:**
```bash
# Stop specific workflow
meow stop <run-id>

# Or clean up all stale
meow ls --stale --json | jq -r '.[].id' | xargs -I{} meow stop {}
```

**Prevention:** Always clean up with `meow stop` when done.

## Workflow Already Running

**Symptom:** `workflow already running` error when starting.

**Cause:** Previous run didn't clean up properly.

**Fix:**
```bash
# Check what's running
meow ls

# Stop the existing one
meow stop <run-id>

# Then start new
meow run my-workflow
```

**Alternative:** Force new run ID:
```bash
meow run my-workflow --workflow new-unique-id
```

## Agent Not Responding

**Symptom:** Step stuck in `running`, agent not doing anything.

**Diagnose:**
```bash
# Find the tmux session
meow agents

# Attach to see what's happening
tmux attach -t meow-<workflow>-<agent>
```

**Common causes:**

1. **Agent waiting for input:** Send Escape key to unstick
   ```bash
   tmux send-keys -t meow-<wf>-<agent> Escape
   ```

2. **Agent crashed:** Check for errors in session

3. **Prompt not displayed:** Agent might not have received the prompt
   ```bash
   # Check workflow trace
   meow trace <run-id>
   ```

**Fix:** If agent is truly stuck:
```bash
# Kill just the agent
tmux kill-session -t meow-<workflow>-<agent>

# Then either resume or restart
meow resume <run-id>
```

## meow done Not Working

**Symptom:** Agent runs `meow done` but step doesn't complete.

**Diagnose:**
```bash
# Check if MEOW_ORCH_SOCK is set (inside agent)
echo $MEOW_ORCH_SOCK

# Check socket exists
ls -la $MEOW_ORCH_SOCK
```

**Common causes:**

1. **Not in MEOW context:** `MEOW_ORCH_SOCK` not set. This is a no-op by design.

2. **Orchestrator crashed:** Socket exists but no listener.
   ```bash
   # Check if orchestrator is running
   meow status
   ```

3. **Wrong step:** Agent calling `meow done` for a step it's not assigned to.

**Fix:**
```bash
# Restart the workflow
meow stop <run-id>
meow run <workflow>
```

## Workflow Failed

**Symptom:** Workflow status shows `failed`.

**Diagnose:**
```bash
# Check status for which step failed
meow status <run-id>

# Check execution trace
meow trace <run-id>

# Check logs
cat .meow/logs/<run-id>.log
```

**Common causes:**

1. **Shell command failed:** Non-zero exit code
   - Check command output in trace
   - Test command manually

2. **Agent timeout:** Step took too long
   - Increase timeout in workflow
   - Check if agent got stuck

3. **Workflow error:** Invalid variable reference, missing dependency
   - Dry run to validate: `meow run <workflow> --dry-run`

**Fix:**
```bash
# Reset failed steps and resume
meow resume <run-id> --reset-failed

# Or start fresh
meow stop <run-id>
meow run <workflow>
```

## Workflow Parsing Errors

**Symptom:** `workflow parse error` on `meow run`.

**Diagnose:**
```bash
meow run <workflow> --dry-run
```

**Common issues:**

1. **Missing required variable:**
   ```
   variable 'task' is required but not provided
   ```
   Fix: Add `--var task="value"`

2. **Invalid TOML syntax:**
   - Check for unquoted strings with special chars
   - Verify array syntax: `[[main.steps]]`
   - Use a TOML validator

3. **Step ID with dot:**
   ```
   step ID 'my.step' cannot contain dots
   ```
   Fix: Use kebab-case: `my-step`

4. **Unknown executor:**
   ```
   unknown executor 'custom'
   ```
   Fix: Use only the 7 executors: shell, spawn, kill, expand, branch, foreach, agent

5. **Invalid escape sequence:**
   ```
   invalid escape in string '\?'
   ```
   This happens when using regex or special characters in `"..."` or `"""..."""` strings.

   **Problem:** TOML only recognizes specific escapes (`\\`, `\n`, `\t`, `\r`, `\"`, `\uXXXX`).
   Characters like `\?`, `\.`, `\d` are invalid.

   **Fix options:**
   ```toml
   # Option 1: Double the backslash
   command = "grep -E '^\\?\\?' file.txt"

   # Option 2: Use literal strings (no escaping)
   command = 'grep -E "^\?\?" file.txt'

   # Option 3: Use multi-line literal for complex commands
   command = '''
   grep -E '^\?\? \.beads/' file.txt
   '''
   ```

   See the **TOML String Escaping** section in the main skill doc for details.

## Tmux Session Issues

**Symptom:** Can't find agent session, or session exists but workflow doesn't know.

**Check sessions:**
```bash
tmux ls | grep meow-
```

**Orphaned sessions** (session exists, workflow doesn't):
```bash
# Kill orphaned session
tmux kill-session -t meow-old-<workflow>-<agent>
```

**Missing session** (workflow expects it):
```bash
# Workflow state is inconsistent, restart
meow stop <run-id>
meow run <workflow>
```

## Lock File Issues

**Symptom:** `lock file exists` or similar errors.

**Location:** `.meow/runs/<run-id>/lock`

**Fix:**
```bash
# Check if orchestrator is really running
meow status

# If not, remove stale lock
rm .meow/runs/<run-id>/lock

# Or stop the workflow properly
meow stop <run-id>
```

## Event Timeout

**Symptom:** `meow await-event` times out.

**Diagnose:**
```bash
# Check if event was emitted
meow trace <run-id> | grep event
```

**Common causes:**

1. **Event never emitted:** Check if the emitting step ran
2. **Event type mismatch:** Case-sensitive, check spelling
3. **Filter too strict:** Event data doesn't match filter

**Fix:**
```bash
# Manually emit event (for testing)
MEOW_ORCH_SOCK=<socket> meow event <type>
```

## Debug Mode

For detailed debugging:
```bash
MEOW_DEBUG=1 meow run <workflow>
```

This logs:
- Step state transitions
- Command execution
- IPC messages
- Event routing
