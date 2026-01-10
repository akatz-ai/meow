# `meow status` Implementation Design

## Document Purpose

This document provides a comprehensive design for implementing the `meow status` CLI command. It serves as:
1. A complete specification for implementation
2. Background and rationale for design decisions
3. A self-contained reference for future development
4. The basis for creating granular implementation beads

---

## Strategic Context

### How `meow status` Serves MEOW's Core Goals

MEOW is a **coordination language for AI agent orchestration**. Its design philosophy emphasizes:

1. **Observability**: Users need to understand what's happening at any moment
2. **Debuggability**: When things go wrong, diagnosis should be straightforward
3. **Durability**: State survives crashes; users can always check current state
4. **Single Source of Truth**: The workflow YAML file is authoritative

The `meow status` command is the **primary user interface for observability**. Without it, users would have to:
- Manually read YAML files to understand workflow state
- Use `tmux ls` to find agent sessions
- Piece together information from multiple sources
- Lack easy access to debugging information

### The Observability Gap

Currently, MEOW has several partial observability tools:
- `meow trace` - Historical execution trace (what happened)
- `meow agents` - Agent registry (may be stale)
- `meow prime` - Current prompt for an agent (IPC-based)
- `meow step-status` - Single step query (IPC-based)

**What's missing**: A unified, real-time view of workflow state that synthesizes all available information into an actionable dashboard. This is `meow status`.

---

## Requirements Analysis

### User Personas and Use Cases

**Persona 1: Developer Running a Workflow**
- Wants to see: Is my workflow running? How far along is it?
- Actions: Monitor progress, estimate completion time
- Output needed: Progress bar, step counts, elapsed time

**Persona 2: Developer Debugging a Stuck Workflow**
- Wants to see: Which step is stuck? Which agent? What's the error?
- Actions: Attach to tmux session, inspect logs, understand blocking
- Output needed: Running step details, tmux attach commands, dependency info

**Persona 3: Developer Investigating a Failed Workflow**
- Wants to see: What failed? Why? What was the state at failure?
- Actions: Read error messages, decide on fix, potentially resume
- Output needed: Failure details, error output, resume instructions

**Persona 4: Developer Managing Multiple Workflows**
- Wants to see: What workflows are active? Any failures?
- Actions: Triage across workflows, focus on problems
- Output needed: Summary table of all workflows

**Persona 5: Script/Automation**
- Wants to see: Machine-readable state for integration
- Actions: Parse status, trigger actions based on state
- Output needed: JSON output with complete state

### Functional Requirements

1. **List all workflows** in the current project
2. **Show detailed status** for a specific workflow
3. **Display agent information** including tmux session attachment commands
4. **Show step progress** with counts by status
5. **Identify blocked steps** and their dependencies
6. **Display errors** for failed workflows/steps
7. **Verify tmux session liveness** for agents
8. **Support JSON output** for scripting
9. **Support filtering** by workflow status
10. **Provide watch mode** for continuous monitoring

### Non-Functional Requirements

1. **Performance**: Status check should complete in <500ms
2. **Reliability**: Must work even if orchestrator has crashed
3. **Accuracy**: Must reflect true state (verify tmux sessions)
4. **Usability**: Output should be scannable at a glance
5. **Actionability**: Include copy-pasteable commands for debugging

---

## Data Model Analysis

### Primary Data Source: Workflow YAML Files

Location: `.meow/workflows/{workflow_id}.yaml`

The workflow file contains:
```yaml
id: wf-abc123
template: work-loop.meow.toml
status: running  # pending | running | done | failed
started_at: 2026-01-09T10:23:45Z
done_at: null  # set on completion

variables:
  agent: worker-1

agents:
  worker-1:
    tmux_session: meow-wf-abc123-worker-1
    status: active  # active | idle
    workdir: .meow/worktrees/worker-1
    current_step: impl.write-code
    claude_session: sess-xyz789

steps:
  step-id:
    id: step-id
    executor: agent  # shell | spawn | kill | expand | branch | agent
    status: running  # pending | running | completing | done | failed
    started_at: 2026-01-09T10:25:00Z
    done_at: null
    needs: [dependency-1, dependency-2]
    outputs: {}
    error: null  # populated on failure
    agent:  # for agent executor
      agent: worker-1
      prompt: "..."
      mode: autonomous
```

### Secondary Data Sources

**Tmux Sessions**: Verify agent liveness
- Command: `tmux has-session -t {session_name} 2>/dev/null`
- Purpose: Detect if agent process is actually running

**IPC Socket**: Check orchestrator liveness
- Path: `/tmp/meow-{workflow_id}.sock`
- Purpose: Indicate if orchestrator is actively running

**Trace File**: Recent activity
- Path: `.meow/state/trace.jsonl`
- Purpose: Show recent actions for context

### Derived Information

From the raw data, we can derive:

1. **Step Progress**
   - Total steps, done count, running count, pending count, failed count
   - Percentage complete: `done / total * 100`

2. **Blocked Steps**
   - Steps where status=pending AND some dependency is not done
   - The specific dependencies blocking each step

3. **Agent Activity**
   - Which step each agent is currently working on
   - How long they've been on that step (now - step.started_at)

4. **Workflow Health**
   - Any failed steps?
   - Any steps running unusually long? (>1h warning)
   - Orchestrator alive? (socket exists and connectable)

5. **Elapsed Time**
   - Workflow duration: now - started_at (or done_at - started_at if complete)
   - Per-step duration for running steps

---

## Visual Design Specification

### Design Principles

1. **Information Hierarchy**: Most important info first, details on request
2. **Visual Scanning**: Status icons, alignment, grouping
3. **Actionable Output**: Commands are copy-pasteable
4. **Color Coding** (optional, terminal-dependent):
   - Green: done, success
   - Yellow: running, in-progress
   - Red: failed, error
   - Gray: pending, not started

### Output Formats

#### Format 1: Multi-Workflow Summary (Default)

When multiple workflows exist or no workflow ID specified:

```
MEOW Status
═══════════

Workflows (3 total, 1 running)
┌─────────────┬─────────────────────────┬─────────┬──────────────┬───────────┐
│ ID          │ Template                │ Status  │ Progress     │ Started   │
├─────────────┼─────────────────────────┼─────────┼──────────────┼───────────┤
│ wf-abc123   │ work-loop.meow.toml     │ RUNNING │ 12/25 (48%)  │ 5m ago    │
│ wf-def456   │ deploy.meow.toml        │ done    │ 8/8 (100%)   │ 2h ago    │
│ wf-xyz789   │ parallel.meow.toml      │ FAILED  │ 5/12 (41%)   │ 45m ago   │
└─────────────┴─────────────────────────┴─────────┴──────────────┴───────────┘

Active Agents (2)
  worker-1   wf-abc123  impl.write-code    5m 23s
             └─ tmux attach -t meow-wf-abc123-worker-1
  watchdog   wf-abc123  patrol             12m 8s
             └─ tmux attach -t meow-wf-abc123-watchdog

Errors (1)
  wf-xyz789  Step 'run-tests' failed: command_failed (exit 1)
             └─ Use: meow status wf-xyz789 --detail

Hint: Use 'meow status <workflow-id>' for detailed view
```

#### Format 2: Single Workflow Detail

When a specific workflow ID is given or only one workflow exists:

```
MEOW Status: wf-abc123
══════════════════════

Overview
  Template:   work-loop.meow.toml
  Status:     RUNNING
  Started:    2026-01-09 10:23:45 (5m 34s ago)
  Variables:  agent=worker-1

Progress
  ████████████░░░░░░░░░░░░░ 48% (12/25 steps)

  ✓ done        12
  ● running      2
  ○ pending     11
  ✗ failed       0

Agents (2)
  ┌──────────┬────────┬──────────────────────────────────┬─────────────┐
  │ Agent    │ Status │ Current Step                     │ Duration    │
  ├──────────┼────────┼──────────────────────────────────┼─────────────┤
  │ worker-1 │ active │ impl.write-code                  │ 3m 12s      │
  │ watchdog │ active │ patrol                           │ 5m 34s      │
  └──────────┴────────┴──────────────────────────────────┴─────────────┘

  Attach commands:
    tmux attach -t meow-wf-abc123-worker-1
    tmux attach -t meow-wf-abc123-watchdog

Running Steps (2)
  impl.write-code    [worker-1]  3m 12s  autonomous
  patrol             [watchdog]  5m 34s  autonomous

Pending Steps (11)
  impl.verify        waiting for: impl.write-code
  integration        waiting for: impl.verify, backend.complete
  ... (9 more, use --all-steps to show)

Recent Activity
  10:28:57  ✓ impl.write-tests completed
  10:29:00  → impl.write-code dispatched
  10:28:45  ✓ setup.worktree completed
  10:28:30  * worker-1 spawned
  10:28:28  > workflow started

Orchestrator: ✓ running (socket: /tmp/meow-wf-abc123.sock)
```

#### Format 3: Failed Workflow Detail

```
MEOW Status: wf-xyz789
══════════════════════

Overview
  Template:   deploy.meow.toml
  Status:     FAILED
  Started:    2026-01-09 09:38:21
  Ended:      2026-01-09 09:45:42 (duration: 7m 21s)
  Variables:  environment=staging

Progress at Failure
  ████████████░░░░░░░░░░░░░ 41% (5/12 steps)

  ✓ done         5
  ✗ failed       1
  ○ not run      6

FAILURE DETAILS
══════════════════════════════════════════════════════════════════════
  Step:      run-tests
  Executor:  shell
  Error:     command_failed
  Exit Code: 1
  Duration:  2m 15s

  Command:
    npm test

  Output (last 20 lines):
    ...
    FAIL src/auth.test.ts
      ✕ should validate JWT token (15ms)
      ✕ should reject expired token (8ms)

    Tests:       3 failed, 39 passed, 42 total
    npm ERR! Test failed. See above for more details.
══════════════════════════════════════════════════════════════════════

Steps Not Run Due to Failure (6)
  deploy-staging     would have run after: run-tests
  approval-gate      would have run after: deploy-staging
  deploy-prod        would have run after: approval-gate
  ... (3 more)

Recovery Options
  1. Fix the failing tests
  2. Resume: meow run --resume wf-xyz789

  Or start fresh: meow run deploy.meow.toml --var environment=staging
```

#### Format 4: JSON Output

```json
{
  "workflows": [
    {
      "id": "wf-abc123",
      "template": "work-loop.meow.toml",
      "status": "running",
      "started_at": "2026-01-09T10:23:45Z",
      "done_at": null,
      "elapsed_seconds": 334,
      "variables": {
        "agent": "worker-1"
      },
      "progress": {
        "total": 25,
        "done": 12,
        "running": 2,
        "pending": 11,
        "failed": 0,
        "percent": 48
      },
      "agents": [
        {
          "id": "worker-1",
          "tmux_session": "meow-wf-abc123-worker-1",
          "tmux_alive": true,
          "status": "active",
          "current_step": "impl.write-code",
          "step_duration_seconds": 192,
          "workdir": ".meow/worktrees/worker-1"
        }
      ],
      "running_steps": [
        {
          "id": "impl.write-code",
          "executor": "agent",
          "agent": "worker-1",
          "started_at": "2026-01-09T10:26:03Z",
          "duration_seconds": 192,
          "mode": "autonomous"
        }
      ],
      "blocked_steps": [
        {
          "id": "impl.verify",
          "waiting_for": ["impl.write-code"]
        }
      ],
      "failed_steps": [],
      "orchestrator": {
        "socket_path": "/tmp/meow-wf-abc123.sock",
        "alive": true
      }
    }
  ],
  "summary": {
    "total_workflows": 3,
    "running": 1,
    "done": 1,
    "failed": 1,
    "total_active_agents": 2
  }
}
```

### Watch Mode Design

```
MEOW Status (watching, refresh: 2s, Ctrl+C to exit)
════════════════════════════════════════════════════

wf-abc123: RUNNING  ████████████████░░░░░░░░░ 64% (16/25)

Agents
  worker-1   impl.refactor        7m 45s  ✓ tmux alive
  watchdog   patrol              17m 8s   ✓ tmux alive

Last Update: 10:35:23 (+2s)
```

---

## Command Interface Specification

### Synopsis

```
meow status [workflow-id] [flags]
```

### Arguments

| Argument | Description |
|----------|-------------|
| `workflow-id` | Optional. Specific workflow to show. If omitted, shows all. |

### Flags

| Flag | Short | Type | Default | Description |
|------|-------|------|---------|-------------|
| `--json` | `-j` | bool | false | Output in JSON format |
| `--watch` | `-w` | bool | false | Continuous refresh mode |
| `--interval` | `-i` | duration | 2s | Refresh interval for watch mode |
| `--status` | `-s` | string | "" | Filter by workflow status (running/done/failed) |
| `--all-steps` | | bool | false | Show all steps (not just summary) |
| `--agents` | `-a` | bool | false | Focus on agent details |
| `--quiet` | `-q` | bool | false | Minimal output (just workflow ID and status) |
| `--no-color` | | bool | false | Disable color output |

### Examples

```bash
# Show all workflows
meow status

# Show specific workflow in detail
meow status wf-abc123

# Watch mode with custom interval
meow status --watch --interval 5s

# Filter to only running workflows
meow status --status running

# JSON output for scripting
meow status --json | jq '.workflows[].progress.percent'

# Quiet mode for scripts
meow status -q wf-abc123  # Output: "running 48%"

# Focus on agents
meow status --agents
```

### Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success (workflows found, status displayed) |
| 0 | Success with `--quiet` and workflow done |
| 1 | No workflows found |
| 2 | Specified workflow not found |
| 3 | Error reading workflow state |

---

## Implementation Architecture

### Package Structure

```
cmd/meow/cmd/
  status.go              # Command definition, flag handling

internal/status/
  status.go              # Main status logic, data aggregation
  workflow_status.go     # Per-workflow status computation
  agent_status.go        # Agent status and tmux verification
  step_analysis.go       # Step progress, blocking analysis
  formatters/
    text.go              # Human-readable text output
    json.go              # JSON output
    table.go             # Table rendering utilities
  watch.go               # Watch mode implementation
```

### Core Types

```go
// WorkflowSummary contains computed status for a workflow
type WorkflowSummary struct {
    ID           string
    Template     string
    Status       types.WorkflowStatus
    StartedAt    time.Time
    DoneAt       *time.Time
    ElapsedTime  time.Duration
    Variables    map[string]string

    Progress     ProgressStats
    Agents       []AgentSummary
    RunningSteps []StepSummary
    BlockedSteps []BlockedStep
    FailedSteps  []FailedStep

    Orchestrator OrchestratorStatus
    RecentTrace  []TraceEntry
}

type ProgressStats struct {
    Total    int
    Done     int
    Running  int
    Pending  int
    Failed   int
    Percent  int
}

type AgentSummary struct {
    ID             string
    TmuxSession    string
    TmuxAlive      bool
    Status         string
    CurrentStep    string
    StepDuration   time.Duration
    Workdir        string
    Mode           string  // autonomous/interactive
}

type BlockedStep struct {
    ID         string
    WaitingFor []string  // Step IDs this is blocked on
}

type FailedStep struct {
    ID        string
    Executor  string
    Error     *types.StepError
    StartedAt time.Time
    Duration  time.Duration
}

type OrchestratorStatus struct {
    SocketPath string
    Alive      bool
}
```

### Key Algorithms

#### 1. Workflow Discovery

```go
func DiscoverWorkflows(meowDir string, filter StatusFilter) ([]*types.Workflow, error) {
    workflowDir := filepath.Join(meowDir, "workflows")

    entries, err := os.ReadDir(workflowDir)
    // Filter to .yaml files (excluding .yaml.tmp)
    // Parse each workflow
    // Apply status filter if specified
    // Sort by started_at (most recent first)

    return workflows, nil
}
```

#### 2. Tmux Session Verification

```go
func VerifyTmuxSession(sessionName string) bool {
    cmd := exec.Command("tmux", "has-session", "-t", sessionName)
    err := cmd.Run()
    return err == nil
}
```

#### 3. Blocked Step Analysis

```go
func FindBlockedSteps(steps map[string]*types.Step) []BlockedStep {
    var blocked []BlockedStep

    for _, step := range steps {
        if step.Status != types.StepStatusPending {
            continue
        }

        var waitingFor []string
        for _, depID := range step.Needs {
            dep := steps[depID]
            if dep == nil || dep.Status != types.StepStatusDone {
                waitingFor = append(waitingFor, depID)
            }
        }

        if len(waitingFor) > 0 {
            blocked = append(blocked, BlockedStep{
                ID:         step.ID,
                WaitingFor: waitingFor,
            })
        }
    }

    return blocked
}
```

#### 4. Watch Mode Loop

```go
func WatchStatus(ctx context.Context, opts WatchOptions) error {
    ticker := time.NewTicker(opts.Interval)
    defer ticker.Stop()

    for {
        // Clear screen
        fmt.Print("\033[H\033[2J")

        // Render status
        status, err := ComputeStatus(opts.WorkflowID)
        if err != nil {
            return err
        }

        RenderWatchView(status, opts.Interval)

        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            continue
        }
    }
}
```

---

## Edge Cases and Error Handling

### Edge Case 1: No Workflows Exist

**Scenario**: User runs `meow status` but no workflows have been created.

**Handling**:
```
No workflows found.

To start a workflow:
  meow run <template.meow.toml>

To initialize a MEOW project:
  meow init
```

### Edge Case 2: Corrupted Workflow File

**Scenario**: A workflow YAML file exists but is corrupted/unparseable.

**Handling**:
- Skip the corrupted file
- Show warning: `Warning: Could not parse wf-xyz.yaml (corrupted?)`
- Continue showing other workflows
- In JSON mode, include in an `errors` array

### Edge Case 3: Orphaned Tmux Sessions

**Scenario**: Tmux sessions exist but workflow is gone (manual deletion, crash).

**Handling**:
- Not shown in status (status is workflow-centric)
- Future: `meow cleanup` command to find and kill orphaned sessions

### Edge Case 4: Workflow Running But Orchestrator Dead

**Scenario**: Workflow status is "running" but orchestrator has crashed.

**Handling**:
- Show workflow as "running" (file says so)
- Show "Orchestrator: ✗ not running (crash recovery available)"
- Suggest: `meow run --resume wf-abc123`

### Edge Case 5: Agent Tmux Session Dead

**Scenario**: Workflow shows agent as active, but tmux session doesn't exist.

**Handling**:
- Show agent with: `worker-1 [active] ✗ tmux session dead`
- This indicates a crash or kill outside MEOW

### Edge Case 6: Very Long Step List

**Scenario**: Workflow has hundreds of steps (via foreach expansion).

**Handling**:
- Default: Show summary + first 10 of each category
- `--all-steps`: Show all (with pagination in watch mode)
- In tables, truncate step IDs if too long

### Edge Case 7: Watch Mode Terminal Resize

**Scenario**: User resizes terminal during watch mode.

**Handling**:
- Detect terminal size on each refresh
- Adjust output accordingly
- Truncate tables if too wide

---

## Testing Strategy

### Unit Tests

1. **Progress calculation**: Given step statuses, verify counts and percentage
2. **Blocked step analysis**: Given dependency graph, verify correct blocking
3. **Time formatting**: Elapsed time display (2s, 5m 30s, 1h 23m)
4. **JSON serialization**: All fields present and correctly typed

### Integration Tests

1. **Workflow discovery**: Create temp workflows, verify discovery
2. **Tmux verification**: Mock tmux command, verify detection
3. **Watch mode**: Verify refresh loop and cancellation
4. **CLI flags**: All flags work correctly

### Manual Testing Checklist

- [ ] No workflows → helpful message
- [ ] Single workflow → auto-detailed view
- [ ] Multiple workflows → summary table
- [ ] Running workflow → progress bar updates
- [ ] Failed workflow → clear error display
- [ ] `--json` → valid JSON output
- [ ] `--watch` → refreshes correctly
- [ ] Ctrl+C in watch → clean exit
- [ ] `--status running` → filters correctly
- [ ] Tmux attach command → works when copied

---

## Future Enhancements (Not in MVP)

1. **Remote status**: Query orchestrator on remote machines
2. **Historical view**: Show status at a point in time
3. **Dependency graph visualization**: ASCII art or link to web viewer
4. **Notifications**: Desktop notification on workflow completion
5. **Comparison**: Compare two workflow runs
6. **Performance metrics**: Step timing statistics

---

## Implementation Phases

### Phase 1: Foundation
- Command structure and flag parsing
- Workflow discovery and parsing
- Basic text output (summary view)

### Phase 2: Detail Views
- Single workflow detailed view
- Step progress calculation
- Agent status with tmux verification

### Phase 3: Analysis Features
- Blocked step analysis
- Failed step display
- Recent trace integration

### Phase 4: Output Formats
- JSON output
- Table formatting improvements
- Color support

### Phase 5: Watch Mode
- Continuous refresh
- Terminal handling
- Clean shutdown

---

## Appendix: Related Spec Sections

From MVP-SPEC-v2.md:
- Section 11: Persistence and Crash Recovery (workflow file format)
- Section 8: Orchestrator Architecture (agent tracking)
- Section 14: CLI Commands (`meow status` mentioned)
- Section 17: Error Handling (error types by executor)
