# Concurrency Model Analysis: Workflow File Locking and Mutex Logic

**Date:** 2026-01-09
**Author:** Claude (Analysis Session)
**Status:** Analysis Complete, Beads Created

---

## Executive Summary

The MEOW orchestrator has a **critical architectural flaw** in its concurrency model: there are two independent code paths for handling IPC messages from agents (`IPCHandler` and `Orchestrator.handleIPC`), and they are **not coordinated**. This creates race conditions where both paths can read-modify-write the workflow file concurrently, leading to lost updates.

Additionally, there are several medium and low severity issues around merge logic, lock consolidation, and file persistence hardening.

---

## Architecture Overview

The system has **three layers** of concurrency protection:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       CONCURRENCY PROTECTION LAYERS                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Layer 1: File-System Lock (syscall.Flock) - PER WORKFLOW                   │
│  ├── Purpose: Prevent multiple orchestrators on SAME workflow               │
│  ├── Location: .meow/workflows/{id}.yaml.lock                               │
│  └── Scope: Per workflow, inter-process                                     │
│  └── Note: Different workflows can run concurrently!                        │
│                                                                             │
│  Layer 2: In-Memory Mutex (sync.Mutex)                                      │
│  ├── Purpose: Coordinate GOROUTINES within one process                      │
│  ├── Location: orchestrator.wfMu                                            │
│  └── Scope: Per orchestrator instance, intra-process                        │
│                                                                             │
│  Layer 3: Atomic File Writes (write-then-rename)                            │
│  ├── Purpose: Prevent file corruption from crashes                          │
│  ├── Location: YAMLWorkflowStore.Save()                                     │
│  └── Scope: Per file operation                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Critical Issue: Orphan IPC Handler (Unsynchronized Dual Code Paths)

### The Problem

Looking at `cmd/meow/cmd/run.go`:

```go
// Line 184: Creates orchestrator with its own IPC channel and mutex
orch := orchestrator.New(cfg, store, agentManager, shellRunner, expander, logger)

// Line 200: Creates SEPARATE IPC handler that talks directly to store
ipcHandler := orchestrator.NewIPCHandler(store, agentManager, logger)

// Line 203: IPC server uses the handler, NOT the orchestrator's channel
ipcServer := ipc.NewServer(workflowID, ipcHandler, logger)
```

The result:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    CURRENT (BROKEN) ARCHITECTURE                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│    Agent (Claude)                                                           │
│         │                                                                   │
│         │ meow done --output x=y                                            │
│         ▼                                                                   │
│    ┌─────────────┐                                                          │
│    │ IPC Server  │                                                          │
│    └──────┬──────┘                                                          │
│           │                                                                 │
│           ▼                                                                 │
│    ┌─────────────────┐     ┌─────────────────┐                              │
│    │  IPCHandler     │     │  Orchestrator   │                              │
│    │  (NO MUTEX!)    │     │  (HAS MUTEX)    │                              │
│    └────────┬────────┘     └────────┬────────┘                              │
│             │                       │                                       │
│             │ store.Get()           │ store.Get()                           │
│             │ modify workflow       │ modify workflow                       │
│             │ store.Save()          │ store.Save()                          │
│             │                       │                                       │
│             ▼                       ▼                                       │
│    ┌─────────────────────────────────────────┐                              │
│    │        YAMLWorkflowStore                │                              │
│    │        (SHARED, NO COORDINATION)        │                              │
│    └─────────────────────────────────────────┘                              │
│                                                                             │
│    RACE CONDITION: Both paths can read-modify-write simultaneously!         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Why This Is Critical

1. **Lost Updates:** IPCHandler.HandleStepDone() marks a step as `done` and saves. Meanwhile, Orchestrator.processWorkflow() read the workflow before that save, makes changes, and overwrites.

2. **State Corruption:** The merge logic in processWorkflow tries to handle this but is insufficient (see below).

3. **Silent Failures:** These races are intermittent and may only manifest under load or with specific timing.

### The Evidence

**IPCHandler.HandleStepDone** (`ipc_handler.go:40-145`):
```go
func (h *IPCHandler) HandleStepDone(ctx context.Context, msg *ipc.StepDoneMessage) any {
    // NO MUTEX ACQUISITION
    wf, err := h.store.Get(ctx, msg.Workflow)  // Read
    // ... modify step status ...
    if err := h.store.Save(ctx, wf); err != nil {  // Write
        // ...
    }
}
```

**Orchestrator.handleStepDone** (`orchestrator.go:448-490`):
```go
func (o *Orchestrator) handleStepDone(ctx context.Context, msg *ipc.StepDoneMessage) error {
    o.wfMu.Lock()  // HAS MUTEX
    defer o.wfMu.Unlock()
    wf, err := o.store.Get(ctx, msg.Workflow)
    // ...
}
```

The orchestrator's version has the mutex, but **it's never called** because the IPC server routes messages to `IPCHandler`, not `Orchestrator`.

### Manifestation Scenarios

**Scenario 1: Agent completes step while orchestrator is ticking**
```
T0: Orchestrator.tick() starts, acquires wfMu
T1: Orchestrator reads workflow (step X is "running")
T2: Agent calls "meow done" → IPCHandler.HandleStepDone
T3: IPCHandler reads workflow, sees step X "running"
T4: IPCHandler marks step X "done", saves workflow
T5: Orchestrator processes steps, marks step Y "running"
T6: Orchestrator saves workflow (overwrites T4's save!)
T7: Step X is back to "running" - LOST UPDATE
```

**Scenario 2: Multiple agents completing simultaneously**
```
T0: Agent A calls "meow done" → IPCHandler starts
T1: Agent B calls "meow done" → IPCHandler starts (parallel)
T2: Both read workflow at same state
T3: A marks step-A done, saves
T4: B marks step-B done, saves (overwrites A's change!)
T5: step-A completion is lost
```

---

## Medium Issue: Flawed Merge Logic in processWorkflow

### The Problem

The orchestrator attempts to handle concurrent modifications with merge logic (`orchestrator.go:269-303`):

```go
// Merge step states between our copy and fresh copy.
// Strategy: use the more "advanced" state for each step.
for stepID, ourStep := range wf.Steps {
    freshStep, ok := freshWf.GetStep(stepID)
    // ...
    ourRank := stepStatusRank(ourStep.Status)
    freshRank := stepStatusRank(freshStep.Status)

    if ourRank > freshRank {
        // Our step is more advanced - copy our state to fresh
        freshStep.Status = ourStep.Status
        // ... copy other fields
    }
}
```

### Issues with This Approach

1. **Status rank treats `done` and `failed` as equal:**
   ```go
   func stepStatusRank(status types.StepStatus) int {
       case types.StepStatusDone:
           return 3
       case types.StepStatusFailed:
           return 3 // Same as done - both terminal
   }
   ```
   This means a `done` step could overwrite a `failed` step's error information, or vice versa.

2. **Only copies selected fields:** The merge only copies `Status`, `StartedAt`, `DoneAt`, `Outputs`, `Error`, and `ExpandedInto`. Other fields could be lost.

3. **Doesn't handle new outputs from IPC:** If IPCHandler sets outputs and the orchestrator's copy has no outputs, the merge keeps the orchestrator's (empty) outputs because the status ranks might be equal.

4. **No timestamp-based resolution:** Without comparing modification times, there's no way to know which change is "newer".

5. **Doesn't handle Agents map:** The workflow's `Agents` map tracking agent state isn't merged at all.

### Why This Exists

This merge logic is a band-aid for the orphan IPC handler problem. If the IPC handler were properly coordinated with the orchestrator (via mutex or channel), this complex merge logic would be unnecessary.

---

## Medium Issue: Unused StatePersister Lock

### The Problem

There are **two different lock mechanisms** defined:

1. `YAMLWorkflowStore.AcquireWorkflowLock()` uses `.meow/workflows/{id}.yaml.lock` (per-workflow lock)
2. `StatePersister` uses `.meow/orchestrator.lock` (acquired in `AcquireLock`)

Looking at `run.go`:
```go
// Line 135: Creates workflow store (no lock acquired here)
store, err := orchestrator.NewYAMLWorkflowStore(workflowsDir)

// Line 142: Acquires per-workflow lock
lock, err := store.AcquireWorkflowLock(workflowID)
defer lock.Release()

// StatePersister is NEVER created or used!
```

### Implications

1. The `StatePersister` code in `state.go` is completely unused
2. Its lock, heartbeat, and state persistence features aren't active
3. This is dead code that may confuse future maintainers

### What StatePersister Was Intended For

Based on the code, `StatePersister` was designed to:
- Track orchestrator process state (PID, tick count, active conditions)
- Provide heartbeat for liveness detection
- Enable another process to check if orchestrator is alive
- Support crash detection via stale heartbeat

These are useful features that aren't being used.

---

## Low Issue: No fsync Before Rename in Atomic Writes

### The Problem

`YAMLWorkflowStore.Save()`:
```go
// Write to temp file
if err := os.WriteFile(tmpPath, data, 0644); err != nil {
    return fmt.Errorf("writing temp file: %w", err)
}

// Atomic rename
if err := os.Rename(tmpPath, mainPath); err != nil {
    // ...
}
```

`os.WriteFile` doesn't guarantee the data is on disk before returning. In a power failure:
1. Data may be in kernel buffers but not on disk
2. Rename succeeds (rename is metadata, often flushed sooner)
3. Power fails
4. On recovery: main file exists but may be empty/truncated

### The Fix

```go
// Open file with explicit control
f, err := os.Create(tmpPath)
if err != nil { return err }

// Write data
if _, err := f.Write(data); err != nil {
    f.Close()
    return err
}

// Sync to disk
if err := f.Sync(); err != nil {
    f.Close()
    return err
}

// Close before rename
if err := f.Close(); err != nil {
    return err
}

// Now rename is safe
if err := os.Rename(tmpPath, mainPath); err != nil {
    // ...
}
```

### Severity Assessment

This is **low severity** because:
- Power failures during writes are rare
- Most filesystems have battery-backed write caches
- The impact is one workflow's state loss, not system-wide corruption

---

## Low Issue: Temp File Promotion Without Validation

### The Problem

`recoverInterruptedWrites()`:
```go
if _, err := os.Stat(mainPath); err == nil {
    // Main file exists, delete orphan temp
    os.Remove(tmpPath)
} else {
    // Main file missing, promote temp
    os.Rename(tmpPath, mainPath)  // NO VALIDATION!
}
```

If the write was interrupted mid-write, the temp file could be:
- Truncated (partial YAML)
- Corrupted (invalid YAML)
- Empty (0 bytes)

Promoting such a file replaces potentially good state with garbage.

### The Fix

```go
} else {
    // Main file missing - validate temp before promoting
    data, err := os.ReadFile(tmpPath)
    if err != nil {
        // Can't read temp - delete it
        os.Remove(tmpPath)
        continue
    }

    var wf types.Workflow
    if err := yaml.Unmarshal(data, &wf); err != nil {
        // Invalid YAML - delete it
        os.Remove(tmpPath)
        continue
    }

    // Valid - promote
    os.Rename(tmpPath, mainPath)
}
```

---

## Recommended Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    PROPOSED (FIXED) ARCHITECTURE                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Option A: Channel-Based Coordination                                       │
│  ─────────────────────────────────────                                      │
│                                                                             │
│    Agent → IPC Server → IPCHandler → orchestrator.ipcChan → Orchestrator    │
│                                              │                              │
│                                              └── All state changes go       │
│                                                  through orchestrator's     │
│                                                  single event loop          │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Option B: Shared Mutex                                                     │
│  ──────────────────────                                                     │
│                                                                             │
│    IPCHandler gets reference to orchestrator's mutex:                       │
│                                                                             │
│    ipcHandler := orchestrator.NewIPCHandler(store, agents, orch.Mutex())    │
│                                                                             │
│    Both code paths acquire same mutex before store operations               │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Option C: Delegate to Orchestrator (Recommended)                           │
│  ───────────────────────────────────────────────                            │
│                                                                             │
│    IPCHandler holds reference to Orchestrator and delegates:                │
│                                                                             │
│    func (h *IPCHandler) HandleStepDone(ctx, msg) any {                      │
│        return h.orch.HandleStepDone(ctx, msg)  // Delegate                  │
│    }                                                                        │
│                                                                             │
│    Orchestrator.HandleStepDone has the mutex and all logic                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**Recommendation: Option C** - It's the simplest change with the clearest ownership model. The Orchestrator owns all workflow state mutations.

---

## Impact on MVP Goals

These concurrency bugs directly threaten MEOW's core value proposition:

1. **Durable Execution:** The spec promises "All workflow state survives crashes, restarts, and session boundaries." Race conditions can lose step completions, violating this.

2. **Orchestrator as Single Authority:** The spec states "The orchestrator is the central authority... Is the single writer to the workflow state file." The current dual-path architecture violates single-writer semantics.

3. **Agent Trust:** If an agent's `meow done` is silently lost, the agent thinks the step is complete but the workflow doesn't advance. This breaks the fundamental contract.

---

## Related Spec Sections

- MVP-SPEC-v2.md §11 "Persistence and Crash Recovery"
- MVP-SPEC-v2.md §8 "Orchestrator Architecture"
- MVP-SPEC-v2.md §10 "IPC and Communication"
