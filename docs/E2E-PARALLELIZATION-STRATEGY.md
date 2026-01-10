# E2E Testing Infrastructure: Parallelization Strategy

## Executive Summary

This document analyzes the E2E testing bead structure and proposes an optimal 4-agent parallelization strategy using the `quad-agent-task.meow.toml` template.

## Current Bead Structure

### Epic: meow-qai - E2E Testing Infrastructure with Claude Simulator

```
EPIC meow-qai
├── FEATURE meow-a0z: Core Simulator Binary
│   ├── meow-o3n: CLI entry point and flag parsing
│   ├── meow-086: Implement simulator state machine
│   ├── meow-za1y: Implement behavior matching engine
│   ├── meow-v0bp: Implement YAML config loading
│   ├── meow-vvr9: Implement direct IPC client
│   └── meow-xxv7: Write simulator unit tests
│
├── FEATURE meow-ur9: Hook Emulation System
│   ├── meow-wgjr: Implement stop hook emulation
│   └── meow-oore: Implement tool event emission
│
├── FEATURE meow-4af: Simulator Adapter
│   └── meow-f153: Create simulator adapter configuration
│
├── FEATURE meow-gnf: E2E Test Framework
│   ├── meow-two3: Implement E2E test harness
│   ├── meow-hy8z: Implement workflow run helpers
│   └── meow-tr1q: Implement simulator config builder
│
├── FEATURE meow-ag6: Core E2E Test Suite (P1)
│   ├── meow-jd7d: Write happy path E2E tests
│   ├── meow-03iq: Write stop hook E2E tests
│   ├── meow-r2bw: Write output validation E2E tests
│   └── meow-5y2m: Write execution mode E2E tests
│
├── FEATURE meow-1lh: Advanced E2E Test Suite (P2)
│   └── ... (deferred)
│
└── FEATURE meow-dor: CI Integration (P2)
    └── ... (deferred)
```

## Dependency Analysis

### Natural Dependencies (Sequential)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DEPENDENCY FLOW                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Phase 1: Build Components (PARALLELIZABLE)                                │
│  ─────────────────────────────────────────                                 │
│                                                                             │
│  Track 1          Track 2           Track 3          Track 4               │
│  Simulator Core   Config System     IPC & Events     Framework+Adapter     │
│  ─────────────    ─────────────     ─────────────    ────────────────      │
│  • main.go        • config.go       • ipc.go         • adapter.toml        │
│  • state.go       • sim_config.go   • hooks.go       • harness.go          │
│  • behavior.go                      • events.go      • workflow_run.go     │
│                                                                             │
│      │                │                  │                  │               │
│      └────────────────┴──────────────────┴──────────────────┘               │
│                              │                                              │
│                              ▼                                              │
│  Phase 2: Integration                                                       │
│  ────────────────────                                                       │
│  • Wire components together                                                 │
│  • Write simulator unit tests                                               │
│  • Verify adapter loads correctly                                           │
│                                                                             │
│                              │                                              │
│                              ▼                                              │
│  Phase 3: E2E Tests                                                         │
│  ──────────────────                                                         │
│  • Happy path tests                                                         │
│  • Stop hook tests                                                          │
│  • Output validation tests                                                  │
│  • Execution mode tests                                                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### File Ownership (Critical for Conflict Avoidance)

| Track | Files Created | Package |
|-------|--------------|---------|
| Track 1 | `cmd/meow-agent-sim/main.go`, `state.go`, `behavior.go` | main |
| Track 2 | `cmd/meow-agent-sim/config.go`, `internal/testutil/e2e/sim_config.go` | main, e2e |
| Track 3 | `cmd/meow-agent-sim/ipc.go`, `hooks.go`, `events.go` | main |
| Track 4 | `test/adapters/simulator/adapter.toml`, `internal/testutil/e2e/harness.go`, `workflow_run.go` | e2e |

## Proposed Track Assignments

### Track 1: Simulator Core Logic

**Beads:**
- `meow-o3n`: CLI entry point and flag parsing
- `meow-086`: Implement simulator state machine
- `meow-za1y`: Implement behavior matching engine

**Rationale:** These form the main execution loop of the simulator. The state machine drives behavior, and behavior matching determines responses. They're tightly coupled and should be developed together.

**Output:** Working simulator binary skeleton that:
- Parses CLI flags (`--config`, `--resume`, `--dangerously-skip-permissions`)
- Implements state transitions (STARTING → IDLE → WORKING → IDLE)
- Matches incoming prompts against behavior patterns

### Track 2: Configuration System

**Beads:**
- `meow-v0bp`: Implement YAML config loading
- `meow-tr1q`: Implement simulator config builder

**Rationale:** Configuration is independent of the execution logic. One agent can focus on YAML parsing while another works on the core state machine. The config builder (for tests) can be developed alongside since they share the config schema.

**Output:**
- YAML config parser for simulator behaviors
- Fluent builder API for constructing configs in tests

### Track 3: IPC & Events

**Beads:**
- `meow-vvr9`: Implement direct IPC client
- `meow-wgjr`: Implement stop hook emulation
- `meow-oore`: Implement tool event emission

**Rationale:** IPC, hooks, and events are all about communication with the orchestrator. They share the same IPC client and message types. Developing them together ensures consistency.

**Output:**
- Direct socket client for communicating with orchestrator
- Stop hook that fires on idle transition, calls `meow prime`
- Tool event emission during behavior execution

### Track 4 (Integration): Framework & Adapter

**Beads:**
- `meow-f153`: Create simulator adapter configuration
- `meow-two3`: Implement E2E test harness
- `meow-hy8z`: Implement workflow run helpers
- `meow-xxv7`: Write simulator unit tests

**Rationale:** This is the integration track that:
1. Creates the adapter config (minimal, can be done early)
2. Builds the test harness (needs adapter)
3. Implements workflow helpers (needs harness)
4. Writes unit tests (validates all tracks work)

**Output:**
- `test/adapters/simulator/adapter.toml`
- Complete E2E test framework in `internal/testutil/e2e/`
- Unit tests for the simulator

## Quad-Agent Template Mapping

The `quad-agent-task.meow.toml` template is ideal for this work:

```
┌────────────────────────────────────────────────────────────────────────────┐
│                       WORKFLOW EXECUTION TIMELINE                           │
├────────────────────────────────────────────────────────────────────────────┤
│                                                                            │
│  TIME ──────────────────────────────────────────────────────────────────►  │
│                                                                            │
│  Phase 1: Worktree Creation (Serial)                                       │
│  ─────────────────────────────────────                                     │
│  [create-wt-1] → [create-wt-2] → [create-wt-3] → [create-wt-4]            │
│                                                                            │
│  Phase 2: Parallel Development                                             │
│  ─────────────────────────────────                                         │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐                        │
│  │  Track 1    │  │  Track 2    │  │  Track 3    │                        │
│  │  Core Logic │  │  Config     │  │  IPC/Events │                        │
│  │             │  │             │  │             │                        │
│  │ work-1      │  │ work-2      │  │ work-3      │                        │
│  │ review-1    │  │ review-2    │  │ review-3    │                        │
│  │ verify-1    │  │ verify-2    │  │ verify-3    │                        │
│  │ kill-1      │  │ kill-2      │  │ kill-3      │                        │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘                        │
│         │                │                │                                │
│         └────────────────┼────────────────┘                                │
│                          │                                                 │
│  Phase 3: Merge to Integration                                             │
│  ─────────────────────────────────                                         │
│                   [merge-to-integration]                                   │
│                          │                                                 │
│                          ▼                                                 │
│  Phase 4: Integration Track                                                │
│  ──────────────────────────                                                │
│                   ┌─────────────┐                                          │
│                   │  Track 4    │                                          │
│                   │  Framework  │                                          │
│                   │             │                                          │
│                   │ work-4      │                                          │
│                   │ review-4    │                                          │
│                   │ verify-4    │                                          │
│                   │ kill-4      │                                          │
│                   └──────┬──────┘                                          │
│                          │                                                 │
│  Phase 5: Final Merge                                                      │
│  ─────────────────────────                                                 │
│           [merge-review] → [merge-to-main] → [done]                       │
│                                                                            │
└────────────────────────────────────────────────────────────────────────────┘
```

## Required Interface Contracts

For parallel development to succeed, we need shared interfaces defined upfront.

### Simulator Interface (for Track 4 to mock)

```go
// cmd/meow-agent-sim/types.go (shared by all tracks)
package main

// State represents the simulator's current state
type State string

const (
    StateStarting State = "starting"
    StateIdle     State = "idle"
    StateWorking  State = "working"
    StateAsking   State = "asking"
)

// Config holds the simulator configuration
type Config struct {
    Hooks     HookConfig     `yaml:"hooks"`
    Behaviors []BehaviorDef  `yaml:"behaviors"`
    Defaults  DefaultConfig  `yaml:"defaults"`
}

type HookConfig struct {
    FireStopHook   bool `yaml:"fire_stop_hook"`
    FireToolEvents bool `yaml:"fire_tool_events"`
}

type BehaviorDef struct {
    Match  string `yaml:"match"`
    Action Action `yaml:"action"`
}

type Action struct {
    Type    string         `yaml:"type"` // complete, ask, fail, hang, crash
    Delay   time.Duration  `yaml:"delay"`
    Outputs map[string]any `yaml:"outputs"`
    Events  []EventDef     `yaml:"events"`
}

// IPCClient interface for orchestrator communication
type IPCClient interface {
    StepDone(outputs map[string]any) error
    GetPrompt() (string, error)
    Event(eventType string, data map[string]any) error
    Close() error
}
```

### Test Framework Interface (for E2E tests)

```go
// internal/testutil/e2e/interfaces.go
package e2e

// SimConfig is the configuration for a simulator instance
type SimConfig struct {
    Hooks     SimHookConfig
    Behaviors []SimBehavior
}

// SimBehavior defines how the simulator responds to a prompt pattern
type SimBehavior struct {
    Match  string
    Action SimAction
}

type SimAction struct {
    Type    string
    Delay   time.Duration
    Outputs map[string]any
}
```

## Workflow Command

To run the parallelized workflow:

```bash
meow run .meow/templates/quad-agent-task.meow.toml \
  --var 'track_name_1=sim-core' \
  --var 'task_prompt_1=Implement beads: meow-o3n, meow-086, meow-za1y (simulator core logic)' \
  --var 'track_name_2=sim-config' \
  --var 'task_prompt_2=Implement beads: meow-v0bp, meow-tr1q (configuration system)' \
  --var 'track_name_3=sim-ipc' \
  --var 'task_prompt_3=Implement beads: meow-vvr9, meow-wgjr, meow-oore (IPC and events)' \
  --var 'track_name_4=sim-integration' \
  --var 'task_prompt_4=Implement beads: meow-f153, meow-two3, meow-hy8z, meow-xxv7 (framework and tests)'
```

## Alternative: Custom Template

The quad-agent template is general-purpose. For better control, we could create a specialized template:

### Option A: Two-Phase Workflow (Recommended)

**Phase 1:** Parallel simulator component development (3 agents)
**Phase 2:** Sequential integration + E2E tests (1 agent)

This matches the natural dependency structure better than forcing a 4th parallel track.

### Option B: Merge-Agent-Only Template

For cases where tracks 1-3 are already complete and we just need integration:

```toml
# .meow/templates/merge-and-integrate.meow.toml
[main]
name = "merge-and-integrate"
description = "Merge completed parallel tracks and run integration"

[main.variables]
branches = { required = true, description = "Comma-separated branch names" }
integration_task = { required = true, description = "Integration task prompt" }

# ... steps to merge branches and spawn integration agent
```

## Estimations

| Track | Beads | Est. Lines | Complexity |
|-------|-------|------------|------------|
| Track 1: Core Logic | 3 | ~400 | High (state machine) |
| Track 2: Config | 2 | ~200 | Medium (YAML parsing) |
| Track 3: IPC/Events | 3 | ~300 | High (socket communication) |
| Track 4: Framework | 4 | ~500 | Medium (test utilities) |

**Expected Duration:** If agents work efficiently, Phase 1 (parallel) could complete in ~20-30 minutes of agent time. Phase 2 (integration) adds another ~15-20 minutes.

## Risk Mitigation

### Merge Conflicts

**Risk:** Tracks modifying shared files (go.mod, imports)

**Mitigation:**
1. Each track only creates new files in designated locations
2. Shared types in a single `types.go` file defined before work starts
3. Integration track handles any wiring

### Interface Mismatches

**Risk:** Track 3 (IPC) doesn't match what Track 4 (framework) expects

**Mitigation:**
1. Define interfaces upfront in shared files
2. Integration track verifies compatibility
3. Self-review step catches obvious issues

### Incomplete Work

**Risk:** One track finishes early, another is stuck

**Mitigation:**
1. Kill agents independently via `graceful = true`
2. Merge what's available, integration track handles gaps
3. Document incomplete items for follow-up

## Next Steps

1. **Create shared types file** (`cmd/meow-agent-sim/types.go`)
2. **Run workflow** with task prompts above
3. **Monitor progress** via `meow status`
4. **Review merged code** before integration phase
5. **Run E2E tests** after integration completes
