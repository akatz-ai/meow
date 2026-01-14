# MEOW Stack - Claude Instructions

This document provides instructions for Claude Code when working with the MEOW Stack system.

## Documentation Structure

| Document | Purpose | Changes |
|----------|---------|---------|
| `docs/ARCHITECTURE.md` | Philosophy, design decisions, core concepts | Rarely |
| `docs/PATTERNS.md` | Working patterns and examples | When patterns evolve |
| Beads (`bd show <id>`) | What's implemented, in progress, planned | Constantly |
| Code + tests | Actual behavior | With every change |

**The authority on "what does MEOW do today" is beads and code, not docs.**

For architecture and design rationale:
```
@docs/ARCHITECTURE.md
```

For working patterns:
```
@docs/PATTERNS.md
```

## Overview

MEOW (Meow Executors Orchestrate Work) is a coordination language for AI agent orchestration. It is NOT a task tracker - it orchestrates agents through programmable workflows.

**Core principle: Dumb orchestrator, smart templates.**

The orchestrator is generic—it knows nothing about Claude, hooks, or what events mean. All agent-specific logic lives in templates.

## Project Status: Pre-Customer MVP

**This is a greenfield MVP with zero customers.** No legacy code, no deployed users, no backwards compatibility requirements.

**Critical rule:** Do NOT add:
- "Legacy" format support or compatibility shims
- Backwards compatibility code for old versions
- Migration paths from previous formats
- Deprecation warnings or fallback behaviors

Delete old code rather than maintaining parallel paths.

## The 7 Executors

| Executor | Who Runs | Completes When |
|----------|----------|----------------|
| `shell` | Orchestrator | Command exits |
| `spawn` | Orchestrator | Agent session running |
| `kill` | Orchestrator | Agent session terminated |
| `expand` | Orchestrator | Template steps inserted |
| `branch` | Orchestrator | Condition evaluated, branch expanded |
| `foreach` | Orchestrator | All iterations complete (implicit join) |
| `agent` | Agent (Claude) | Agent calls `meow done` |

**Gate is NOT an executor.** Human approval is implemented as: `branch` with `condition = "meow await-approval <gate-id>"`.

## Async Command Execution

Both **branch** and **shell** execute commands **asynchronously** in goroutines. Shell is syntactic sugar over branch—`handleShell()` converts the config and delegates to `handleBranch()`.

This is critical for enabling parallel execution patterns:

```yaml
# Both steps start in the same tick after spawn completes
[[steps]]
id = "main-work"
executor = "agent"
needs = ["spawn"]

[[steps]]
id = "monitor"
executor = "branch"
needs = ["spawn"]
condition = "meow await-event agent-stopped --timeout 30s"
```

**Key implementation details:**

1. `handleBranch()` launches command in goroutine and returns immediately
2. Command execution holds NO mutex (pure I/O)
3. `completeBranchCondition()` acquires mutex, re-reads workflow, expands, saves
4. Completions serialize through mutex (~100-200/second throughput)
5. `pendingCommands` sync.Map tracks cancel functions for both branch and shell
6. Recovery resets in-flight commands (no ExpandedInto) to pending

**Files:**
- Implementation: `internal/orchestrator/orchestrator.go` (handleBranch, handleShell, completeBranchCondition)
- Architecture: `docs/ARCHITECTURE.md`

## Step Status Lifecycle

```
pending ──► running ──► completing ──► done
              │             │
              │             └──► (back to running if validation fails)
              │
              └──► failed
```

The `completing` state prevents race conditions during step transitions.

## Agent Persistence (Ralph Wiggum Pattern)

Agents are kept on task via **events**, not orchestrator logic:

1. Agent's Stop hook emits `meow event agent-stopped`
2. A monitor template waits for events via `meow await-event`
3. On event, monitor injects a nudge prompt via `fire_forget` mode
4. Monitor checks if main task is done; loops if not

This is implemented entirely in `lib/agent-persistence.meow.toml`. The orchestrator just routes events—it doesn't know what "agent-stopped" means.

## Working on Beads

When assigned a bead to implement:

1. **Read the bead**: `bd show <bead-id>`
2. **Check related beads**: Dependencies, blockers, related work
3. **Implement with TDD**: Write failing tests first, then implement
4. **Commit frequently**: Small, logical commits
5. **Close when done**: `bd close <bead-id>`

## Key Files

| Purpose | Path |
|---------|------|
| Architecture | `docs/ARCHITECTURE.md` |
| Patterns | `docs/PATTERNS.md` |
| Types | `internal/types/step.go`, `internal/types/workflow.go` |
| Orchestrator | `internal/orchestrator/orchestrator.go` |
| Workflows | `internal/workflow/` |
| CLI | `cmd/meow/cmd/` |
| Library Workflows | `.meow/workflows/lib/` |

## Commands Reference

### Beads CLI (bd)

```bash
bd ready                  # Show tasks ready to work on
bd show <id>              # View bead details
bd list --status=open     # List open beads
bd close <id>             # Close completed bead
bd update <id> --notes "" # Update notes
bd sync                   # Sync beads with git remote
bd sync --status          # Check sync status
bd sync --flush-only      # Export to JSONL only (no git ops)
```

### Beads Sync Troubleshooting

The `bd sync` command requires a clean git working directory. If sync fails:

1. **Unstaged changes error**: `bd sync` runs `git commit` internally and fails if there are unstaged changes elsewhere. Either commit/stash other changes first, or use `bd sync --flush-only`.

2. **Leftover merge files**: If you see `.beads/beads.left.jsonl` or `.beads/beads.base.jsonl`, these are from interrupted conflict resolution. Clean them up:
   ```bash
   rm -f .beads/beads.left.* .beads/beads.base.*
   ```

3. **Prefix mismatch error**: The remote has issues with a different prefix. Use `--rename-on-import` to fix:
   ```bash
   bd sync --rename-on-import
   ```

4. **Config missing**: Ensure `.beads/config.yaml` has the sync branch set:
   ```yaml
   sync-branch: "main"
   ```

### MEOW CLI (meow)

```bash
meow run <template>       # Run a workflow
meow done --output k=v    # Signal step completion
meow event <type>         # Emit an event
meow await-event <type>   # Wait for an event
meow step-status <id>     # Check step status
meow approve <wf> <gate>  # Approve a gate
meow reject <wf> <gate>   # Reject a gate
```

**Note:** Commands like `meow done` and `meow event` exit silently (no-op) when `MEOW_ORCH_SOCK` is not set. This allows hooks to work both inside and outside MEOW workflows.

## Testing

### Test Types

| Type | Location | Purpose | Runtime |
|------|----------|---------|---------|
| Unit | `*_test.go` alongside source | Test individual functions | ~6s total |
| Integration | `*_integration_test.go` | Test component interactions | Included in unit |
| E2E | `internal/testutil/e2e/` | Full workflow execution | ~48-75s |

### Running Tests

**Makefile targets (preferred):**

```bash
make test              # Run all tests with race detector (~80s)
make test-short        # Skip E2E tests (~6s) - use for quick feedback
make test-cover        # Generate coverage report
make check             # fmt + vet + test
```

**Direct go test commands:**

```bash
go test ./...                           # All tests
go test -race ./...                     # Race condition detector
go test -short ./...                    # Skip E2E tests
go test -v -run TestName ./pkg/         # Specific test
go test -count=1 ./...                  # Disable test cache
```

### Key Test Locations

| Package | Tests For |
|---------|-----------|
| `internal/types/*_test.go` | Step, Workflow, ExecutorType validation |
| `internal/template/*_test.go` | Template parsing, validation, variable expansion |
| `internal/orchestrator/*_test.go` | State management, executor dispatch |
| `internal/agent/*_test.go` | Tmux session management, agent spawning |
| `internal/testutil/e2e/e2e_test.go` | Full workflow execution scenarios |
| `cmd/meow/cmd/*_test.go` | CLI command tests |

## Best Practices

### 1. Dumb Orchestrator

The orchestrator should NOT know about:
- Claude or any specific agent
- What events mean or how to respond
- Stop hooks or persistence patterns

All agent-specific logic belongs in templates.

### 2. No Gate Executor

Gates are implemented via `branch` + `meow await-approval`. Don't add a gate executor.

### 3. 7 Executors Only

The executors are: shell, spawn, kill, expand, branch, foreach, agent. No more, no less.

### 4. Step IDs Cannot Contain Dots

Dots are reserved for expansion prefixes (e.g., `parent-step.child-step`).

### 5. Single Writer Principle

All workflow state mutations go through the orchestrator's mutex-protected methods. The IPC handler delegates to the orchestrator.

### 6. Events, Not Hooks

Agent hook systems (Claude's Stop hook) translate to generic events. The orchestrator routes events; templates implement responses.

## Error Handling

When a step fails:
- `on_error: fail` (default) - workflow fails
- `on_error: continue` - log and continue

Agent step timeout handling:
1. Send C-c to agent's tmux session
2. Wait 10 seconds
3. Mark step failed

## Session Notes

If approaching context limits:

1. **Save progress**: Update bead notes with status
2. **Commit changes**: Don't leave uncommitted work
3. **Document next steps**: What should the next session do?

```bash
bd update <bead-id> --notes "
COMPLETED:
- [what you finished]

IN PROGRESS:
- [current state]

NEXT:
- [what needs to happen]
"
```

---

*For architecture and design rationale, see `docs/ARCHITECTURE.md`. For working patterns, see `docs/PATTERNS.md`. For what's implemented, use `bd list` and `bd show`.*
