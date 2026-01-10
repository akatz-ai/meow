# `meow status` Implementation Beads

## Overview

This document defines the complete bead structure for implementing `meow status`.
Each bead is designed to be:
- **Self-contained**: Includes all context needed to implement
- **Testable**: Has clear success criteria
- **Appropriately sized**: 1-4 hours of work
- **Dependency-aware**: Clear blocking relationships

---

## Bead Hierarchy

```
EPIC: meow-status-epic
├── FEATURE: status-foundation
│   ├── TASK: status-cmd-scaffold
│   ├── TASK: workflow-discovery
│   ├── TASK: workflow-summary-type
│   └── TASK: basic-text-output
├── FEATURE: status-detail-view
│   ├── TASK: single-workflow-detail
│   ├── TASK: step-progress-calc
│   ├── TASK: agent-status-display
│   └── TASK: tmux-verification
├── FEATURE: status-analysis
│   ├── TASK: blocked-step-analysis
│   ├── TASK: failed-step-display
│   ├── TASK: trace-integration
│   └── TASK: orchestrator-liveness
├── FEATURE: status-output-formats
│   ├── TASK: json-output
│   ├── TASK: table-formatting
│   ├── TASK: color-support
│   └── TASK: quiet-mode
├── FEATURE: status-watch-mode
│   ├── TASK: watch-loop
│   ├── TASK: watch-display
│   └── TASK: watch-signals
└── FEATURE: status-polish
    ├── TASK: help-text-examples
    ├── TASK: edge-case-handling
    └── TASK: integration-tests
```

---

## EPIC: meow-status-epic

### Epic Metadata
```yaml
title: "Implement meow status command"
type: epic
priority: 1  # High priority - core observability feature
```

### Background

The `meow status` command is the primary user interface for understanding workflow state in MEOW. Currently, users have no unified way to see:
- What workflows are running
- Progress through workflow steps
- Which agents are active and what they're doing
- How to attach to agent tmux sessions for debugging
- What went wrong when a workflow fails

This gap makes MEOW difficult to use and debug. The `meow status` command fills this observability gap by synthesizing information from workflow YAML files, tmux sessions, and trace logs into an actionable dashboard.

### Success Criteria
1. `meow status` shows all workflows with progress
2. `meow status <id>` shows detailed workflow view
3. Agents display with copy-pasteable tmux attach commands
4. Failed workflows show clear error information
5. JSON output works for scripting
6. Watch mode provides live updates
7. All edge cases handled gracefully

### Design Reference
See: `docs/design/meow-status-implementation.md`

---

## FEATURE: status-foundation

### Feature Metadata
```yaml
title: "Status command foundation"
type: feature
priority: 1
parent: meow-status-epic
```

### Description
Establish the core infrastructure for the status command: command structure, workflow discovery, and basic text output. This is the foundation all other features build upon.

### Rationale
We need a working skeleton before adding detail views, analysis, or formatting. This follows the principle of iterative development - get something minimal working first, then enhance.

### Dependencies
None (first feature)

---

### TASK: status-cmd-scaffold

```yaml
title: "Create status command scaffold with flags"
type: task
priority: 1
parent: status-foundation
```

#### Description
Create the cobra command structure for `meow status` with all planned flags, even if most are initially unimplemented (returning "not yet implemented" errors).

#### Background
The existing `status.go` is a stub that just prints "not yet implemented". We need to:
1. Define all flags according to the spec
2. Set up the command structure
3. Create placeholder functions that will be implemented in later tasks

Having all flags defined upfront ensures:
- Users can see what's planned via `--help`
- We don't forget flags during implementation
- The CLI interface is designed holistically

#### Implementation Details

**File**: `cmd/meow/cmd/status.go`

**Flags to implement**:
- `--json, -j` (bool): JSON output
- `--watch, -w` (bool): Watch mode
- `--interval, -i` (duration, default 2s): Watch interval
- `--status, -s` (string): Filter by status
- `--all-steps` (bool): Show all steps
- `--agents, -a` (bool): Focus on agents
- `--quiet, -q` (bool): Minimal output
- `--no-color` (bool): Disable colors

**Arguments**:
- Optional `[workflow-id]` positional argument

**Exit codes**:
- 0: Success
- 1: No workflows found
- 2: Specified workflow not found
- 3: Error reading state

#### Success Criteria
- [ ] `meow status --help` shows all flags with descriptions
- [ ] Flags can be parsed without error
- [ ] Unknown flags produce helpful error
- [ ] Exit codes are defined as constants

#### Estimated Effort
1-2 hours

---

### TASK: workflow-discovery

```yaml
title: "Implement workflow discovery from filesystem"
type: task
priority: 1
parent: status-foundation
depends_on: [status-cmd-scaffold]
```

#### Description
Implement the ability to find and load all workflow YAML files from `.meow/workflows/`.

#### Background
Workflow state is persisted as YAML files in `.meow/workflows/`. The `YAMLWorkflowStore` already handles individual workflow loading, but we need a non-locking read-only discovery mechanism for the status command.

**Key insight**: The status command must NOT acquire the exclusive lock that `YAMLWorkflowStore` uses. The lock is for the orchestrator to prevent concurrent writes. Status is read-only and should work even while an orchestrator is running.

#### Implementation Details

**New file**: `internal/status/discovery.go`

```go
// DiscoverWorkflows finds all workflow files without locking
func DiscoverWorkflows(meowDir string) ([]*types.Workflow, error)

// DiscoverWorkflowsFiltered applies status filter
func DiscoverWorkflowsFiltered(meowDir string, status types.WorkflowStatus) ([]*types.Workflow, error)

// LoadWorkflowReadOnly loads a single workflow without locking
func LoadWorkflowReadOnly(meowDir, workflowID string) (*types.Workflow, error)
```

**Considerations**:
- Skip `.yaml.tmp` files (incomplete writes)
- Handle corrupted files gracefully (skip with warning)
- Sort by started_at (newest first)
- Don't fail if directory doesn't exist (no workflows yet)

#### Success Criteria
- [ ] Discovers all `.yaml` files in workflows directory
- [ ] Skips `.tmp` files correctly
- [ ] Handles missing directory (returns empty list)
- [ ] Handles corrupted files (logs warning, continues)
- [ ] Respects status filter when provided
- [ ] Unit tests for all cases

#### Estimated Effort
2 hours

---

### TASK: workflow-summary-type

```yaml
title: "Define WorkflowSummary and related types"
type: task
priority: 1
parent: status-foundation
depends_on: [status-cmd-scaffold]
```

#### Description
Define the data types that represent computed workflow status, used between the analysis layer and formatters.

#### Background
The raw `types.Workflow` contains persisted state, but the status command needs computed/derived information:
- Progress percentages
- Elapsed times
- Blocked step lists
- Agent liveness

We need intermediate types that hold this computed data, which can then be formatted for text or JSON output.

#### Implementation Details

**New file**: `internal/status/types.go`

```go
package status

import (
    "time"
    "github.com/meow-stack/meow-machine/internal/types"
)

// WorkflowSummary contains computed status for display
type WorkflowSummary struct {
    // Identity
    ID        string
    Template  string
    Status    types.WorkflowStatus
    StartedAt time.Time
    DoneAt    *time.Time

    // Computed
    ElapsedTime time.Duration
    Variables   map[string]string

    // Progress
    Progress ProgressStats

    // Agents
    Agents []AgentSummary

    // Steps by category
    RunningSteps  []StepSummary
    PendingSteps  []StepSummary
    CompletedSteps []StepSummary
    FailedSteps   []FailedStepSummary

    // Analysis
    BlockedSteps []BlockedStep

    // Infrastructure
    Orchestrator OrchestratorStatus
}

type ProgressStats struct {
    Total    int
    Done     int
    Running  int
    Pending  int
    Failed   int
    Percent  int // 0-100
}

type AgentSummary struct {
    ID           string
    TmuxSession  string
    TmuxAlive    bool  // Verified via tmux has-session
    Status       string
    CurrentStep  string
    StepDuration time.Duration
    Workdir      string
    Mode         string // autonomous/interactive
}

type StepSummary struct {
    ID        string
    Executor  types.ExecutorType
    Agent     string // for agent executor
    StartedAt *time.Time
    Duration  time.Duration
    Mode      string
}

type FailedStepSummary struct {
    StepSummary
    Error    *types.StepError
}

type BlockedStep struct {
    ID         string
    WaitingFor []string // Step IDs blocking this
}

type OrchestratorStatus struct {
    SocketPath string
    Alive      bool
}

// StatusOptions configures status computation
type StatusOptions struct {
    WorkflowID string              // Empty = all workflows
    Status     types.WorkflowStatus // Empty = no filter
    IncludeTrace bool
    TraceLimit   int
}
```

#### Success Criteria
- [ ] All types defined with proper YAML/JSON tags
- [ ] Types documented with godoc comments
- [ ] No dependencies on presentation (pure data)
- [ ] Consistent naming conventions

#### Estimated Effort
1 hour

---

### TASK: basic-text-output

```yaml
title: "Implement basic text output for workflow list"
type: task
priority: 1
parent: status-foundation
depends_on: [workflow-discovery, workflow-summary-type]
```

#### Description
Implement the multi-workflow summary view - the default output when running `meow status` with no arguments.

#### Background
This is the first visible output the user sees. It should be scannable at a glance:
- Which workflows exist
- What's their status
- How much progress has been made
- When they started

We start with a simple text table without colors (colors come later).

#### Implementation Details

**New file**: `internal/status/format_text.go`

```go
func FormatWorkflowList(summaries []*WorkflowSummary, opts FormatOptions) string

func FormatSingleWorkflow(summary *WorkflowSummary, opts FormatOptions) string
```

**Output format** (multi-workflow):
```
MEOW Status
═══════════

Workflows (3 total, 1 running)
  ID           Template                 Status   Progress      Started
  ──────────── ──────────────────────── ──────── ───────────── ─────────
  wf-abc123    work-loop.meow.toml      RUNNING  12/25 (48%)   5m ago
  wf-def456    deploy.meow.toml         done     8/8 (100%)    2h ago
  wf-xyz789    parallel.meow.toml       FAILED   5/12 (41%)    45m ago

Use 'meow status <workflow-id>' for details
```

**Time formatting**:
- < 1 minute: "Xs" (e.g., "45s")
- < 1 hour: "Xm Ys" (e.g., "5m 30s")
- < 1 day: "Xh Ym" (e.g., "2h 15m")
- >= 1 day: "Xd Yh" (e.g., "1d 5h")

#### Success Criteria
- [ ] Shows all workflows in table format
- [ ] Status is visually distinct (RUNNING vs done vs FAILED)
- [ ] Progress shows fraction and percentage
- [ ] Relative time is human-readable
- [ ] Helpful message when no workflows
- [ ] Truncates long template names gracefully

#### Estimated Effort
2-3 hours

---

## FEATURE: status-detail-view

### Feature Metadata
```yaml
title: "Detailed single-workflow view"
type: feature
priority: 1
parent: meow-status-epic
depends_on: [status-foundation]
```

### Description
When viewing a specific workflow (or when only one exists), show comprehensive details including agents, running steps, and progress visualization.

### Rationale
The summary view tells you "what workflows exist". The detail view tells you "what's happening in this workflow". This is where debugging happens - users need to see which agents are active, what step they're on, and how to attach for inspection.

---

### TASK: single-workflow-detail

```yaml
title: "Implement detailed single-workflow view"
type: task
priority: 1
parent: status-detail-view
depends_on: [basic-text-output]
```

#### Description
Create the detailed view shown when specifying a workflow ID or when only one workflow exists.

#### Background
This view is the heart of observability. A developer debugging a stuck workflow needs:
1. **Progress bar**: Visual indication of completion
2. **Status breakdown**: How many steps in each state
3. **Agent info**: Who's doing what, for how long
4. **Running steps**: What's currently executing
5. **Pending steps**: What's waiting (and why)

#### Implementation Details

Enhance `internal/status/format_text.go`:

```go
func FormatDetailedWorkflow(summary *WorkflowSummary, opts FormatOptions) string
```

**Output format**:
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

  done        12  ✓
  running      2  ●
  pending     11  ○
  failed       0

Agents (2)
  worker-1  [active]  impl.write-code  3m 12s
            tmux attach -t meow-wf-abc123-worker-1

  watchdog  [active]  patrol           5m 34s
            tmux attach -t meow-wf-abc123-watchdog

Running Steps
  impl.write-code    worker-1    3m 12s  autonomous
  patrol             watchdog    5m 34s  autonomous
```

**Progress bar implementation**:
- 25 characters wide
- █ for complete portions
- ░ for incomplete portions
- Calculate: `filled = int(percent * 25 / 100)`

#### Success Criteria
- [ ] Detailed view triggered for specific workflow ID
- [ ] Auto-triggers when only one workflow exists
- [ ] Progress bar renders correctly at 0%, 50%, 100%
- [ ] Agent tmux commands are copy-pasteable
- [ ] Variables shown inline
- [ ] Running step durations update correctly

#### Estimated Effort
3-4 hours

---

### TASK: step-progress-calc

```yaml
title: "Implement step progress calculation"
type: task
priority: 1
parent: status-detail-view
depends_on: [workflow-summary-type]
```

#### Description
Create the logic to compute step progress statistics from raw workflow data.

#### Background
The workflow file contains steps with individual statuses. We need to:
1. Count steps by status
2. Calculate percentage complete
3. Handle edge cases (0 steps, all failed, etc.)

#### Implementation Details

**New file**: `internal/status/analysis.go`

```go
// ComputeProgress calculates step statistics
func ComputeProgress(steps map[string]*types.Step) ProgressStats {
    stats := ProgressStats{}

    for _, step := range steps {
        stats.Total++
        switch step.Status {
        case types.StepStatusDone:
            stats.Done++
        case types.StepStatusRunning, types.StepStatusCompleting:
            stats.Running++
        case types.StepStatusPending:
            stats.Pending++
        case types.StepStatusFailed:
            stats.Failed++
        }
    }

    if stats.Total > 0 {
        stats.Percent = stats.Done * 100 / stats.Total
    }

    return stats
}
```

**Edge cases**:
- 0 steps: Percent = 0
- All failed: Percent based on done/(total-failed)?
  - Decision: Use done/total - failure is still "progress" in the sense the step ran

#### Success Criteria
- [ ] Counts all statuses correctly
- [ ] Percentage calculation correct
- [ ] Handles 0 steps (no division by zero)
- [ ] Completing counted as Running (it's still in-flight)
- [ ] Unit tests for edge cases

#### Estimated Effort
1 hour

---

### TASK: agent-status-display

```yaml
title: "Implement agent status extraction and display"
type: task
priority: 1
parent: status-detail-view
depends_on: [workflow-summary-type]
```

#### Description
Extract agent information from workflows and format for display, including the critical tmux attach commands.

#### Background
Agents are the workers in MEOW. Users need to see:
- Which agents exist in the workflow
- What step each is currently working on
- How long they've been on that step
- How to attach to their tmux session for debugging

The tmux attach command is the #1 debugging tool. Making it copy-pasteable is essential.

#### Implementation Details

**In** `internal/status/analysis.go`:

```go
// ExtractAgentSummaries builds agent info from workflow
func ExtractAgentSummaries(wf *types.Workflow) []AgentSummary {
    var agents []AgentSummary

    for agentID, info := range wf.Agents {
        summary := AgentSummary{
            ID:          agentID,
            TmuxSession: info.TmuxSession,
            Status:      info.Status,
            CurrentStep: info.CurrentStep,
            Workdir:     info.Workdir,
        }

        // Find current step to get duration
        if step, ok := wf.Steps[info.CurrentStep]; ok {
            if step.StartedAt != nil {
                summary.StepDuration = time.Since(*step.StartedAt)
            }
            if step.Agent != nil {
                summary.Mode = step.Agent.Mode
            }
        }

        agents = append(agents, summary)
    }

    // Sort alphabetically by ID for consistent output
    sort.Slice(agents, func(i, j int) bool {
        return agents[i].ID < agents[j].ID
    })

    return agents
}
```

**Formatting considerations**:
- Align columns for readability
- Truncate long step IDs if necessary
- Show mode (autonomous/interactive) as it affects behavior

#### Success Criteria
- [ ] All agents from workflow extracted
- [ ] Current step and duration computed
- [ ] Mode extracted from step config
- [ ] Sorted consistently
- [ ] Tmux command formatted correctly

#### Estimated Effort
2 hours

---

### TASK: tmux-verification

```yaml
title: "Implement tmux session liveness verification"
type: task
priority: 2
parent: status-detail-view
depends_on: [agent-status-display]
```

#### Description
Verify whether tmux sessions for agents are actually running, not just recorded in state.

#### Background
The workflow file records `tmux_session: meow-wf-abc123-worker-1` for each agent, but this doesn't mean the session is still alive. The agent could have:
- Crashed (Claude error)
- Been killed manually
- Timed out
- Machine rebooted

Users need to know if they can actually attach to the session.

#### Implementation Details

**New file**: `internal/status/tmux.go`

```go
// VerifyTmuxSession checks if a tmux session exists
func VerifyTmuxSession(sessionName string) bool {
    cmd := exec.Command("tmux", "has-session", "-t", sessionName)
    err := cmd.Run()
    return err == nil
}

// VerifyAllAgentSessions checks all agents in summaries
func VerifyAllAgentSessions(agents []AgentSummary) {
    for i := range agents {
        agents[i].TmuxAlive = VerifyTmuxSession(agents[i].TmuxSession)
    }
}
```

**Display implications**:
```
Agents
  worker-1  [active]  impl.write-code  3m 12s  ✓ tmux alive
            tmux attach -t meow-wf-abc123-worker-1

  backend   [active]  build            7m 45s  ✗ tmux dead
            (session ended - check logs or resume workflow)
```

**Performance consideration**:
- Each `tmux has-session` is a subprocess
- For 3-5 agents, this is fine (<100ms total)
- For many agents, could parallelize with goroutines
- MVP: Sequential is fine

#### Success Criteria
- [ ] `tmux has-session` called for each agent
- [ ] Result stored in AgentSummary.TmuxAlive
- [ ] Display shows alive/dead status
- [ ] Works when tmux not installed (graceful failure)
- [ ] Works when no sessions exist

#### Estimated Effort
1-2 hours

---

## FEATURE: status-analysis

### Feature Metadata
```yaml
title: "Status analysis features"
type: feature
priority: 2
parent: meow-status-epic
depends_on: [status-detail-view]
```

### Description
Advanced analysis features: blocked step detection, failure display, trace integration, and orchestrator liveness.

### Rationale
Beyond showing current state, users need to understand *why* things are in their current state:
- Why is a step pending? (blocked on dependencies)
- What went wrong? (failure details)
- What happened recently? (trace integration)
- Is the orchestrator running? (liveness)

---

### TASK: blocked-step-analysis

```yaml
title: "Implement blocked step dependency analysis"
type: task
priority: 2
parent: status-analysis
depends_on: [step-progress-calc]
```

#### Description
For pending steps, determine which dependencies are blocking them and display this information.

#### Background
A step is "blocked" if:
1. Its status is `pending`
2. At least one of its `needs` dependencies is not `done`

Users debugging slow progress need to know: "Step X is waiting for Y and Z to complete". This helps identify bottlenecks and understand the execution flow.

#### Implementation Details

**In** `internal/status/analysis.go`:

```go
// FindBlockedSteps identifies pending steps and their blockers
func FindBlockedSteps(steps map[string]*types.Step) []BlockedStep {
    var blocked []BlockedStep

    for _, step := range steps {
        if step.Status != types.StepStatusPending {
            continue
        }

        var waitingFor []string
        for _, depID := range step.Needs {
            dep, ok := steps[depID]
            if !ok {
                // Dependency doesn't exist - should not happen in valid workflow
                waitingFor = append(waitingFor, depID+" (missing!)")
                continue
            }
            if dep.Status != types.StepStatusDone {
                waitingFor = append(waitingFor, depID)
            }
        }

        // Only include if actually blocked (has unmet dependencies)
        if len(waitingFor) > 0 {
            blocked = append(blocked, BlockedStep{
                ID:         step.ID,
                WaitingFor: waitingFor,
            })
        }
    }

    // Sort by number of blockers (most blocked first)
    sort.Slice(blocked, func(i, j int) bool {
        return len(blocked[i].WaitingFor) > len(blocked[j].WaitingFor)
    })

    return blocked
}
```

**Display format**:
```
Blocked Steps (3)
  integration        waiting for: frontend.build, backend.build
  deploy             waiting for: integration, approval-gate
  cleanup            waiting for: deploy
```

**Edge cases**:
- Step with no `needs`: Never blocked (ready when pending)
- All dependencies done: Not blocked (should transition to running)
- Circular dependency: Would cause infinite wait (validation issue)

#### Success Criteria
- [ ] Identifies all blocked steps correctly
- [ ] Shows which specific dependencies are blocking
- [ ] Handles missing dependencies gracefully
- [ ] Sorted by importance (most blocked first)
- [ ] Unit tests for dependency scenarios

#### Estimated Effort
2 hours

---

### TASK: failed-step-display

```yaml
title: "Implement failed step and error display"
type: task
priority: 1
parent: status-analysis
depends_on: [step-progress-calc]
```

#### Description
When steps have failed, display comprehensive error information to help users diagnose and fix the issue.

#### Background
A failed workflow is useless without understanding why it failed. The `StepError` type captures:
- `Message`: Human-readable error description
- `Code`: Exit code (for shell commands)
- `Output`: stderr or other context

Users need this information prominently displayed with recovery suggestions.

#### Implementation Details

**In** `internal/status/analysis.go`:

```go
// ExtractFailedSteps gets detailed failure info
func ExtractFailedSteps(steps map[string]*types.Step) []FailedStepSummary {
    var failed []FailedStepSummary

    for _, step := range steps {
        if step.Status != types.StepStatusFailed {
            continue
        }

        summary := FailedStepSummary{
            StepSummary: StepSummary{
                ID:        step.ID,
                Executor:  step.Executor,
                StartedAt: step.StartedAt,
            },
            Error: step.Error,
        }

        if step.StartedAt != nil && step.DoneAt != nil {
            summary.Duration = step.DoneAt.Sub(*step.StartedAt)
        }

        if step.Agent != nil {
            summary.Agent = step.Agent.Agent
        }

        failed = append(failed, summary)
    }

    return failed
}
```

**Display format** (in workflow detail):
```
FAILURE DETAILS
══════════════════════════════════════════════════════════════════════
  Step:      run-tests
  Executor:  shell
  Error:     command_failed
  Exit Code: 1
  Duration:  2m 15s

  Output (last 20 lines):
    FAIL src/auth.test.ts
      ✕ should validate JWT token (15ms)
      ✕ should reject expired token (8ms)

    Tests:       3 failed, 39 passed, 42 total
══════════════════════════════════════════════════════════════════════

Recovery Options
  1. Fix the failing tests
  2. Resume: meow run --resume wf-xyz789
```

**Output truncation**:
- Show last 20 lines of output by default
- `--all-steps` shows full output
- Long lines wrapped or truncated

#### Success Criteria
- [ ] All failed steps identified
- [ ] Error details clearly displayed
- [ ] Output truncated appropriately
- [ ] Recovery suggestions shown
- [ ] Exit code shown for shell steps

#### Estimated Effort
2 hours

---

### TASK: trace-integration

```yaml
title: "Integrate recent trace entries in status output"
type: task
priority: 3
parent: status-analysis
depends_on: [single-workflow-detail]
```

#### Description
Show recent trace entries at the bottom of the detailed view to provide context about what's been happening.

#### Background
The trace file (`.meow/state/trace.jsonl`) contains timestamped execution history. Showing the last few entries gives users a timeline of recent activity without running a separate `meow trace` command.

This is especially useful for:
- Seeing what just completed
- Understanding the execution flow
- Checking if the orchestrator is making progress

#### Implementation Details

**In** `internal/status/trace_reader.go`:

```go
// ReadRecentTrace gets the last N trace entries
func ReadRecentTrace(meowDir string, limit int) ([]TraceEntry, error) {
    tracePath := filepath.Join(meowDir, "state", "trace.jsonl")

    // Read file
    // Parse each line as JSON
    // Return last `limit` entries

    // Note: For efficiency, could read file backwards
    // MVP: Read all, return tail
}
```

**Display format**:
```
Recent Activity (last 5)
  10:28:57  ✓ impl.write-tests completed
  10:29:00  → impl.write-code dispatched
  10:28:45  ✓ setup.worktree completed
  10:28:30  * worker-1 spawned
  10:28:28  > workflow started
```

**Icons** (reuse from trace.go):
- `>` start
- `*` spawn
- `→` dispatch
- `✓` close/complete
- `✗` error
- `||` shutdown

#### Success Criteria
- [ ] Reads last N entries from trace file
- [ ] Handles missing trace file
- [ ] Formats entries compactly
- [ ] Uses consistent icons with trace command
- [ ] Default to 5 entries, configurable

#### Estimated Effort
1-2 hours

---

### TASK: orchestrator-liveness

```yaml
title: "Check orchestrator socket liveness"
type: task
priority: 2
parent: status-analysis
depends_on: [workflow-summary-type]
```

#### Description
Check if the orchestrator's IPC socket exists and is connectable, indicating the orchestrator is actively running.

#### Background
Workflow status might be "running" in the YAML file, but if the orchestrator crashed, no progress will be made. Users need to know:
- Is the orchestrator alive?
- If not, how to resume?

The IPC socket at `/tmp/meow-{workflow_id}.sock` is the indicator.

#### Implementation Details

**In** `internal/status/orchestrator.go`:

```go
// CheckOrchestratorStatus determines if orchestrator is running
func CheckOrchestratorStatus(workflowID string) OrchestratorStatus {
    socketPath := fmt.Sprintf("/tmp/meow-%s.sock", workflowID)

    status := OrchestratorStatus{
        SocketPath: socketPath,
        Alive:      false,
    }

    // Check if socket file exists
    if _, err := os.Stat(socketPath); os.IsNotExist(err) {
        return status
    }

    // Try to connect (with timeout)
    conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
    if err != nil {
        return status
    }
    conn.Close()

    status.Alive = true
    return status
}
```

**Display format**:
```
Orchestrator: ✓ running (socket: /tmp/meow-wf-abc123.sock)
```

or

```
Orchestrator: ✗ not running
              Workflow may be stalled. Resume with:
              meow run --resume wf-abc123
```

#### Success Criteria
- [ ] Detects socket existence
- [ ] Verifies socket is connectable
- [ ] Handles missing socket
- [ ] Handles socket exists but not connectable
- [ ] Timeout prevents hanging

#### Estimated Effort
1 hour

---

## FEATURE: status-output-formats

### Feature Metadata
```yaml
title: "Output format options"
type: feature
priority: 2
parent: meow-status-epic
depends_on: [status-detail-view]
```

### Description
Support multiple output formats: JSON for scripting, enhanced tables, color support, and quiet mode.

### Rationale
Different users have different needs:
- Humans want readable, possibly colored output
- Scripts need structured JSON
- CI systems may need minimal output
- Users on non-color terminals need plain text

---

### TASK: json-output

```yaml
title: "Implement JSON output format"
type: task
priority: 1
parent: status-output-formats
depends_on: [workflow-summary-type]
```

#### Description
Implement `--json` flag that outputs machine-readable JSON instead of human-readable text.

#### Background
JSON output enables:
- Scripting and automation
- Integration with other tools
- Parsing with `jq`
- Monitoring systems

The JSON structure should match the `WorkflowSummary` types closely for consistency.

#### Implementation Details

**In** `internal/status/format_json.go`:

```go
// FormatJSON serializes summaries to JSON
func FormatJSON(summaries []*WorkflowSummary) (string, error) {
    output := JSONOutput{
        Workflows: summaries,
        Summary: JSONSummary{
            Total:   len(summaries),
            Running: countByStatus(summaries, types.WorkflowStatusRunning),
            Done:    countByStatus(summaries, types.WorkflowStatusDone),
            Failed:  countByStatus(summaries, types.WorkflowStatusFailed),
        },
    }

    data, err := json.MarshalIndent(output, "", "  ")
    if err != nil {
        return "", err
    }
    return string(data), nil
}
```

**JSON structure**:
```json
{
  "workflows": [...],
  "summary": {
    "total": 3,
    "running": 1,
    "done": 1,
    "failed": 1
  },
  "generated_at": "2026-01-09T10:35:00Z"
}
```

**Considerations**:
- Include timestamps in ISO 8601
- Durations as seconds (not human-readable strings)
- All fields present (use null, not omit)

#### Success Criteria
- [ ] Valid JSON output
- [ ] All relevant fields included
- [ ] Parseable by `jq`
- [ ] Durations in seconds
- [ ] Timestamps in ISO 8601
- [ ] Pretty-printed (indented)

#### Estimated Effort
2 hours

---

### TASK: table-formatting

```yaml
title: "Implement proper table formatting with alignment"
type: task
priority: 2
parent: status-output-formats
depends_on: [basic-text-output]
```

#### Description
Enhance text tables with proper column alignment, dynamic widths, and consistent formatting.

#### Background
The basic text output uses simple spacing. For professional output, tables should:
- Align columns properly
- Handle variable-width content
- Truncate or wrap long content
- Use box-drawing characters (optional)

#### Implementation Details

**New file**: `internal/status/table.go`

```go
type TableColumn struct {
    Header    string
    Width     int  // 0 = auto
    Align     Alignment
    Truncate  bool
    MaxWidth  int
}

type Table struct {
    Columns []TableColumn
    Rows    [][]string
}

func (t *Table) Render(opts RenderOptions) string
```

**Features**:
- Auto-calculate column widths from content
- Right-align numbers
- Left-align text
- Truncate with "..." for long content
- Optional Unicode box characters vs ASCII

#### Success Criteria
- [ ] Columns align properly
- [ ] Auto-width calculation works
- [ ] Long content truncated gracefully
- [ ] Numbers right-aligned
- [ ] Terminal width respected

#### Estimated Effort
2-3 hours

---

### TASK: color-support

```yaml
title: "Add color support for terminal output"
type: task
priority: 3
parent: status-output-formats
depends_on: [table-formatting]
```

#### Description
Add ANSI color codes to make status visually scannable.

#### Background
Colors dramatically improve readability:
- Green for success/done
- Yellow for in-progress/warning
- Red for failure/error
- Cyan for IDs and commands
- Gray for secondary info

Must respect `--no-color` flag and `NO_COLOR` environment variable.

#### Implementation Details

**New file**: `internal/status/color.go`

```go
const (
    ColorReset  = "\033[0m"
    ColorRed    = "\033[31m"
    ColorGreen  = "\033[32m"
    ColorYellow = "\033[33m"
    ColorBlue   = "\033[34m"
    ColorCyan   = "\033[36m"
    ColorGray   = "\033[90m"
    ColorBold   = "\033[1m"
)

func ShouldUseColor(noColorFlag bool) bool {
    if noColorFlag {
        return false
    }
    if os.Getenv("NO_COLOR") != "" {
        return false
    }
    // Check if stdout is a terminal
    return isatty.IsTerminal(os.Stdout.Fd())
}

func Colorize(text, color string, useColor bool) string {
    if !useColor {
        return text
    }
    return color + text + ColorReset
}
```

**Color mapping**:
- `RUNNING` → Yellow + Bold
- `done` → Green
- `FAILED` → Red + Bold
- Step IDs → Cyan
- Durations → Default
- Commands → Cyan (for copy-paste visibility)

#### Success Criteria
- [ ] Colors applied appropriately
- [ ] `--no-color` disables colors
- [ ] `NO_COLOR` env var respected
- [ ] Non-terminal output has no colors
- [ ] Works on common terminals

#### Estimated Effort
2 hours

---

### TASK: quiet-mode

```yaml
title: "Implement quiet mode for scripting"
type: task
priority: 2
parent: status-output-formats
depends_on: [basic-text-output]
```

#### Description
Implement `--quiet` flag for minimal, script-friendly output.

#### Background
Scripts often just need to know: is it running? What percentage? Quiet mode provides:
- Single-line output
- No headers or decoration
- Easy to parse with basic tools

#### Implementation Details

**Output format**:
```bash
# Single workflow
$ meow status wf-abc123 -q
running 48%

# Multiple workflows
$ meow status -q
wf-abc123 running 48%
wf-def456 done 100%
wf-xyz789 failed 41%
```

**Exit codes** (important for scripts):
- 0: Success (found workflows)
- 1: No workflows
- 2: Specified workflow not found

#### Success Criteria
- [ ] Single-line per workflow
- [ ] Format: `{id} {status} {percent}%`
- [ ] No headers or decoration
- [ ] Exit codes correct
- [ ] Works with status filter

#### Estimated Effort
1 hour

---

## FEATURE: status-watch-mode

### Feature Metadata
```yaml
title: "Watch mode for live updates"
type: feature
priority: 2
parent: meow-status-epic
depends_on: [status-output-formats]
```

### Description
Implement `--watch` flag for continuous status updates, refreshing the display periodically.

### Rationale
Users monitoring a workflow want to see progress without repeatedly running `meow status`. Watch mode provides a live dashboard experience similar to `watch` or `htop`.

---

### TASK: watch-loop

```yaml
title: "Implement watch mode refresh loop"
type: task
priority: 2
parent: status-watch-mode
depends_on: [single-workflow-detail]
```

#### Description
Create the main loop that periodically refreshes status display.

#### Background
Watch mode needs to:
1. Clear the screen
2. Compute fresh status
3. Render output
4. Wait for interval
5. Repeat until Ctrl+C

Must handle graceful shutdown on SIGINT.

#### Implementation Details

**In** `internal/status/watch.go`:

```go
func WatchStatus(ctx context.Context, opts WatchOptions) error {
    ticker := time.NewTicker(opts.Interval)
    defer ticker.Stop()

    // Initial render
    if err := renderWatchView(opts); err != nil {
        return err
    }

    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            if err := renderWatchView(opts); err != nil {
                // Log error but continue
                fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
            }
        }
    }
}

func renderWatchView(opts WatchOptions) error {
    // Clear screen
    fmt.Print("\033[H\033[2J")

    // Compute status
    summaries, err := ComputeStatus(opts)
    if err != nil {
        return err
    }

    // Render with watch header
    output := FormatWatchView(summaries, opts)
    fmt.Print(output)

    return nil
}
```

**Clear screen approach**:
- ANSI escape: `\033[H\033[2J` (move to home + clear)
- Cross-platform: Works on most terminals
- Alternative: Use term library for better compat

#### Success Criteria
- [ ] Screen clears between refreshes
- [ ] Status updates at specified interval
- [ ] Ctrl+C exits cleanly
- [ ] No flickering (full clear, then render)
- [ ] Shows last update timestamp

#### Estimated Effort
2 hours

---

### TASK: watch-display

```yaml
title: "Design watch mode display format"
type: task
priority: 2
parent: status-watch-mode
depends_on: [watch-loop]
```

#### Description
Create a compact display format optimized for watch mode.

#### Background
Watch mode should be more compact than the full detail view:
- Essential info only (can run `meow status` for full details)
- Progress bar is key
- Active agents with durations
- Last few trace entries
- Timestamp of last refresh

#### Implementation Details

**Watch display format**:
```
MEOW Status (watching, refresh: 2s) [Ctrl+C to exit]
═══════════════════════════════════════════════════════

wf-abc123: RUNNING
  ████████████████░░░░░░░░░ 64% (16/25 steps)

  Agents                      Current Step        Duration
  ─────────────────────────── ─────────────────── ────────
  worker-1   ✓ alive          impl.refactor       7m 45s
  watchdog   ✓ alive          patrol             17m 8s

  Recent: ✓ impl.tests (10:35:01) → impl.refactor (10:35:03)

Last refresh: 10:35:23
```

**Compact elements**:
- Single progress bar line
- Agents as simple table
- Recent activity as single line
- Clear timestamp for "is it updating?"

#### Success Criteria
- [ ] Fits in 80x24 terminal
- [ ] Progress bar prominent
- [ ] Agents shown with liveness
- [ ] Last refresh time shown
- [ ] Instructions for exit

#### Estimated Effort
2 hours

---

### TASK: watch-signals

```yaml
title: "Handle signals in watch mode"
type: task
priority: 2
parent: status-watch-mode
depends_on: [watch-loop]
```

#### Description
Properly handle SIGINT (Ctrl+C) and SIGTERM for clean watch mode exit.

#### Background
Users expect Ctrl+C to cleanly exit watch mode:
- Restore terminal state
- Print exit message
- Return appropriate exit code

#### Implementation Details

**In** `cmd/meow/cmd/status.go`:

```go
func runWatchMode(opts WatchOptions) error {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()

    // Handle signals
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

    go func() {
        <-sigChan
        fmt.Println("\nExiting watch mode...")
        cancel()
    }()

    return status.WatchStatus(ctx, opts)
}
```

**Considerations**:
- Don't leave terminal in bad state
- Print newline before exit message
- Clear signal handler on exit

#### Success Criteria
- [ ] Ctrl+C exits cleanly
- [ ] Exit message shown
- [ ] Terminal state restored
- [ ] No orphan goroutines

#### Estimated Effort
1 hour

---

## FEATURE: status-polish

### Feature Metadata
```yaml
title: "Polish and edge cases"
type: feature
priority: 3
parent: meow-status-epic
depends_on: [status-watch-mode]
```

### Description
Final polish: help text, edge case handling, and integration tests.

### Rationale
The difference between a good tool and a great tool is in the polish. This feature covers all the "last 10%" that makes the tool feel complete.

---

### TASK: help-text-examples

```yaml
title: "Write comprehensive help text with examples"
type: task
priority: 2
parent: status-polish
depends_on: [status-cmd-scaffold]
```

#### Description
Write clear, helpful command descriptions and examples for `--help` output.

#### Background
Good help text teaches users how to use the command. It should include:
- Clear description of purpose
- All flags explained
- Usage examples for common scenarios
- Related commands

#### Implementation Details

**In** `cmd/meow/cmd/status.go`:

```go
var statusCmd = &cobra.Command{
    Use:   "status [workflow-id]",
    Short: "Show workflow status",
    Long: `Display the current state of MEOW workflows.

Without arguments, shows a summary of all workflows.
With a workflow ID, shows detailed information for that workflow.

The status command helps you:
  • See which workflows are running
  • Track progress through workflow steps
  • Find agents and their tmux sessions
  • Diagnose failed workflows

Examples:
  # Show all workflows
  meow status

  # Show details for a specific workflow
  meow status wf-abc123

  # Watch status with live updates
  meow status --watch

  # Filter to only running workflows
  meow status --status running

  # Get JSON output for scripting
  meow status --json | jq '.workflows[0].progress'

  # Attach to an agent's tmux session (from status output)
  tmux attach -t meow-wf-abc123-worker-1

See also: meow trace, meow agents`,
    RunE: runStatus,
}
```

#### Success Criteria
- [ ] Short description clear and concise
- [ ] Long description explains value
- [ ] All flags have descriptions
- [ ] Examples cover common use cases
- [ ] Related commands mentioned

#### Estimated Effort
1 hour

---

### TASK: edge-case-handling

```yaml
title: "Handle all edge cases gracefully"
type: task
priority: 2
parent: status-polish
depends_on: [all other tasks]
```

#### Description
Review and handle all edge cases identified in the design document.

#### Background
Edge cases were identified during design:
1. No workflows exist
2. Corrupted workflow file
3. Orphaned tmux sessions
4. Workflow running but orchestrator dead
5. Agent tmux session dead
6. Very long step list
7. Terminal resize in watch mode

Each needs graceful handling with helpful messages.

#### Implementation Details

**Edge case handling**:

1. **No workflows**:
```
No workflows found.

To start a workflow:
  meow run <template.meow.toml>
```

2. **Corrupted file**:
```
Warning: Could not parse wf-xyz.yaml (invalid YAML syntax)

Showing 2 valid workflows:
...
```

3. **Orchestrator dead with running workflow**:
```
wf-abc123: RUNNING (orchestrator not responding)
  Progress stalled. Resume with: meow run --resume wf-abc123
```

4. **Dead tmux session**:
```
worker-1  [active]  impl.code  ✗ tmux dead
          Session ended unexpectedly. Check logs or resume workflow.
```

5. **Long step list** (>20 steps):
```
Pending Steps (47)
  step-1          waiting for: step-0
  step-2          waiting for: step-1
  ... (45 more, use --all-steps to show all)
```

#### Success Criteria
- [ ] All edge cases have user-friendly messages
- [ ] No panics or crashes on bad data
- [ ] Helpful suggestions for resolution
- [ ] Consistent message format

#### Estimated Effort
2-3 hours

---

### TASK: integration-tests

```yaml
title: "Write integration tests for status command"
type: task
priority: 2
parent: status-polish
depends_on: [edge-case-handling]
```

#### Description
Create integration tests that verify the status command works end-to-end.

#### Background
Unit tests verify individual functions. Integration tests verify:
- Command runs successfully
- Flags work correctly
- Output matches expectations
- Edge cases handled

#### Implementation Details

**Test file**: `cmd/meow/cmd/status_test.go`

**Test cases**:
1. `TestStatus_NoWorkflows` - Empty directory
2. `TestStatus_SingleWorkflow` - Auto-detail view
3. `TestStatus_MultipleWorkflows` - Summary table
4. `TestStatus_SpecificWorkflow` - By ID
5. `TestStatus_WorkflowNotFound` - Error message
6. `TestStatus_JSONOutput` - Valid JSON
7. `TestStatus_QuietMode` - Minimal output
8. `TestStatus_StatusFilter` - Filtering works
9. `TestStatus_CorruptedFile` - Graceful handling

**Test approach**:
- Create temp `.meow/workflows/` directory
- Write test workflow YAML files
- Run command with different flags
- Verify output/exit code

#### Success Criteria
- [ ] All major scenarios tested
- [ ] Tests run in CI
- [ ] Test fixtures are realistic
- [ ] Edge cases covered
- [ ] No flaky tests

#### Estimated Effort
3-4 hours

---

## Dependency Graph Summary

```
                            status-cmd-scaffold
                                    │
                    ┌───────────────┼───────────────┐
                    ▼               ▼               ▼
           workflow-discovery  workflow-summary-type
                    │               │
                    └───────┬───────┘
                            ▼
                    basic-text-output
                            │
                            ▼
                 single-workflow-detail
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
step-progress-calc  agent-status-display  (formatters)
        │                   │
        ▼                   ▼
blocked-step-analysis  tmux-verification
        │
        ▼
failed-step-display
        │
        ▼
trace-integration ── orchestrator-liveness
                            │
        ┌───────────────────┼───────────────────┐
        ▼                   ▼                   ▼
   json-output      table-formatting      quiet-mode
                            │
                            ▼
                     color-support
                            │
                            ▼
                      watch-loop
                            │
                    ┌───────┴───────┐
                    ▼               ▼
             watch-display    watch-signals
                    │
                    ▼
            help-text-examples
                    │
                    ▼
           edge-case-handling
                    │
                    ▼
           integration-tests
```

---

## Priority Order (Suggested Implementation Sequence)

### Must Have (P1) - Core functionality
1. status-cmd-scaffold
2. workflow-discovery
3. workflow-summary-type
4. basic-text-output
5. single-workflow-detail
6. step-progress-calc
7. agent-status-display
8. failed-step-display
9. json-output

### Should Have (P2) - Important features
10. tmux-verification
11. blocked-step-analysis
12. orchestrator-liveness
13. table-formatting
14. quiet-mode
15. watch-loop
16. watch-display
17. watch-signals
18. help-text-examples
19. edge-case-handling
20. integration-tests

### Nice to Have (P3) - Polish
21. trace-integration
22. color-support

---

## Effort Estimates

| Priority | Tasks | Total Hours |
|----------|-------|-------------|
| P1 | 9 tasks | 16-21 hours |
| P2 | 11 tasks | 17-22 hours |
| P3 | 2 tasks | 3-4 hours |
| **Total** | **22 tasks** | **36-47 hours** |

Estimated calendar time: **2-3 weeks** with focused work.

---

## Appendix: Bead Creation Commands

To create these beads in your system:

```bash
# Epic
bd create --title="Implement meow status command" --type=epic --priority=1

# Features (adjust parent ID)
bd create --title="Status command foundation" --type=feature --priority=1
bd create --title="Detailed single-workflow view" --type=feature --priority=1
bd create --title="Status analysis features" --type=feature --priority=2
bd create --title="Output format options" --type=feature --priority=2
bd create --title="Watch mode for live updates" --type=feature --priority=2
bd create --title="Polish and edge cases" --type=feature --priority=3

# Tasks (adjust parent and deps)
bd create --title="Create status command scaffold with flags" --type=task --priority=1
# ... etc
```

See the individual bead definitions above for full descriptions to include in the `--notes` or description fields.
