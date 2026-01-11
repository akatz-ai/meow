# MEOW Stack - Claude Instructions

This document provides instructions for Claude Code when working with the MEOW Stack system.

## Required Reading

Before working on any task, read the authoritative specification:

```
@docs/MVP-SPEC-v2.md
```

This is the **only** architecture document. It defines:
- The 7 executors (shell, spawn, kill, expand, branch, foreach, agent)
- Step and Workflow data structures
- IPC protocol (Unix sockets, single-line JSON)
- Template system (TOML modules)
- CLI commands

## Overview

MEOW (Meow Executors Orchestrate Work) is a coordination language for AI agent orchestration. It is NOT a task tracker - it orchestrates agents through programmable workflows.

## Project Status: Pre-Customer MVP

**This is a greenfield MVP with zero customers.** No legacy code, no deployed users, no backwards compatibility requirements.

**Critical rule:** Do NOT add:
- "Legacy" format support or compatibility shims
- Backwards compatibility code for old versions
- Migration paths from previous formats
- Deprecation warnings or fallback behaviors

Delete old code rather than maintaining parallel paths.

## Current Work: MVP-SPEC-v2 Pivot

We are implementing the workflow-centric model from MVP-SPEC-v2. Key changes from older code:

| Old (Bead-Centric) | New (Workflow-Centric) |
|--------------------|------------------------|
| 8 bead types | 7 executors |
| BeadStore interface | WorkflowStore interface |
| Bead struct | Step struct |
| `meow close` | `meow done` |
| Three-tier visibility | No tiers - just steps |
| .beads/issues.jsonl | .meow/workflows/*.yaml |

**If you see old patterns in the code, you may be looking at code that needs to be refactored as part of this pivot.**

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

## Async Branch Execution

Branch conditions execute **asynchronously** in goroutines. This is critical for enabling parallel monitoring patterns (the "Ralph Wiggum pattern"):

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

1. `handleBranch()` launches condition in goroutine and returns immediately
2. Condition execution holds NO mutex (pure I/O)
3. `completeBranchCondition()` acquires mutex, re-reads workflow, expands, saves
4. Completions serialize through mutex (~100-200/second throughput)
5. `pendingBranches` sync.Map tracks cancel functions for cleanup
6. Recovery resets in-flight branches (no ExpandedInto) to pending

**Files:**
- Implementation: `internal/orchestrator/orchestrator.go` (handleBranch, completeBranchCondition)
- Design: `docs/MVP-SPEC-v2.md` §branch, §Performance Characteristics

**Performance:** Parallel conditions are fine, but completions serialize. For 100+ concurrent branches, expect ~1s completion latency. Use `max_concurrent` on foreach to limit parallelism if needed.

## Step Status Lifecycle

```
pending ──► running ──► completing ──► done
              │             │
              │             └──► (back to running if validation fails)
              │
              └──► failed
```

The `completing` state prevents race conditions during step transitions.

## Working on Beads

When assigned a bead to implement:

1. **Read the bead**: `bd show <bead-id>`
2. **Read relevant spec sections**: The bead description often references specific parts of MVP-SPEC-v2
3. **Implement with TDD**: Write failing tests first, then implement
4. **Commit frequently**: Small, logical commits
5. **Close when done**: `bd close <bead-id>`

## Key Files

| Purpose | Path |
|---------|------|
| Spec | `docs/MVP-SPEC-v2.md` |
| Types (old) | `internal/types/bead.go` |
| Types (new) | `internal/types/step.go` (to be created) |
| Orchestrator | `internal/orchestrator/orchestrator.go` |
| Templates | `internal/template/` |
| CLI | `cmd/meow/cmd/` |

## Commands Reference

### Beads CLI (bd)

```bash
bd ready                  # Show tasks ready to work on
bd show <id>              # View bead details
bd list --status=open     # List open beads
bd close <id>             # Close completed bead
bd update <id> --notes "" # Update notes
```

### MEOW CLI (meow) - Current

```bash
meow run <template>       # Run a workflow
meow prime                # Get current task (for agents)
meow close <id>           # Close a task (being renamed to 'done')
```

### MEOW CLI (meow) - New (MVP-SPEC-v2)

```bash
meow run <template>       # Run a workflow
meow done --output k=v    # Signal step completion
meow prime                # Get current prompt (for stop hook)
meow approve <wf> <gate>  # Approve a gate
meow reject <wf> <gate>   # Reject a gate
```

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
# All tests
go test ./...
go test -v ./...                    # Verbose output
go test -race ./...                 # Race condition detector

# Short tests only (skip E2E)
go test -short ./...

# Specific package
go test ./internal/types/
go test -v ./internal/template/

# Specific test by name
go test -v -run TestExecutorType ./internal/types/
go test -v -run TestShellStep ./internal/testutil/e2e/

# E2E tests only
go test -v ./internal/testutil/e2e/

# Disable test cache
go test -count=1 ./...

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
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

### E2E Test Infrastructure

E2E tests use a harness that provides:
- Isolated temporary directories per test
- Built binaries (`meow` and `meow-agent-sim`)
- Simulator for agent responses

```go
// Example E2E test pattern
func TestMyWorkflow(t *testing.T) {
    h := e2e.NewHarness(t)

    cfg := e2e.NewSimConfigBuilder().
        WithBehavior("do something", e2e.ActionComplete).
        Build()

    run := h.RunWorkflow("my-template.meow.toml", cfg)
    run.WaitForStep("step-id", types.StepStatusDone)
}
```

### Writing New Tests

1. **Unit tests**: Add `*_test.go` in the same package as source
2. **Use subtests**: Group related cases with `t.Run("scenario", ...)`
3. **Use `t.TempDir()`**: For file system isolation
4. **Use `t.Helper()`**: In helper functions for better error locations
5. **Use `t.Cleanup()`**: For resource cleanup

## Best Practices

### 1. Read the Spec First

MVP-SPEC-v2 is comprehensive. If you're unsure about something, it's probably in the spec.

### 2. No Gate Executor

If you're tempted to add a gate executor or GateConfig, stop. Gates are implemented via branch + await-approval.

### 3. 7 Executors Only

The executors are: shell, spawn, kill, expand, branch, foreach, agent. No more, no less.

### 4. Step IDs Cannot Contain Dots

Dots are reserved for expansion prefixes (e.g., `parent-step.child-step`).

### 5. Atomic File Writes

Workflow state files use write-then-rename for atomicity.

### 6. Single-Line JSON for IPC

IPC messages must be single-line JSON (no pretty printing).

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

*The authoritative reference is `docs/MVP-SPEC-v2.md`. When in doubt, read the spec.*
