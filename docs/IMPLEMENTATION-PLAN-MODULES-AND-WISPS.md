# Implementation Plan: Module System and Three-Tier Bead Architecture

> **Status**: Approved for Implementation
> **Created**: 2026-01-07
> **Authors**: Design synthesis from architecture discussions
> **Dependencies**: MVP-SPEC.md, SPEC-ADDENDUM-WISPS-AND-MODULES.md, ARCHITECTURE.md

---

## Table of Contents

1. [Executive Summary](#executive-summary)
2. [Vision and Goals](#vision-and-goals)
3. [Current State Analysis](#current-state-analysis)
4. [Architectural Deep Dive](#architectural-deep-dive)
5. [Implementation Phases](#implementation-phases)
6. [Detailed Bead Definitions](#detailed-bead-definitions)
7. [Dependency Graph](#dependency-graph)
8. [Risk Analysis](#risk-analysis)
9. [Success Criteria](#success-criteria)
10. [Future Considerations](#future-considerations)

---

## Executive Summary

This document provides the complete implementation plan for two major enhancements to MEOW Stack:

1. **Module System** - Multi-workflow template files with local references and `main` convention
2. **Three-Tier Bead Architecture** - Work beads, workflow beads (wisps), and orchestrator beads

These features address critical usability issues:
- **File explosion**: Complex workflows currently require many files
- **Agent confusion**: Agents see all beads including infrastructure machinery
- **Missing context**: No way to show agents their workflow progression
- **Work/workflow conflation**: Agents can't distinguish "what to work on" from "how to work"

The implementation spans 7 epics with 45+ tasks, organized into 4 phases over the development timeline.

---

## Vision and Goals

### The Core Problem

When an agent runs `meow prime` today, they see a flat list of beads. Consider this scenario:

```
Agent sees:
  - start-worker (what is this?)
  - load-context (ok, I do this)
  - check-tests (orchestrator thing?)
  - implement (yes, I do this)
  - stop-worker (not my concern)
```

The agent is confused. They see infrastructure beads they shouldn't care about, mixed with their actual work. There's no sense of progression, no connection to the work bead they're implementing.

### The Ideal Experience

After implementation:

```
$ meow prime

═══════════════════════════════════════════════════════════════
Your workflow: implement-tdd (step 2/4)
Work bead: gt-123 "Implement auth endpoint"
═══════════════════════════════════════════════════════════════

  ✓ load-context
  → write-tests [in_progress] ← YOU ARE HERE
  ○ implement
  ○ commit

───────────────────────────────────────────────────────────────
Instructions:
  Write failing tests that define the expected behavior.
  Tests MUST fail at this point.

Required outputs: (none)
───────────────────────────────────────────────────────────────
```

The agent sees:
- Their position in the workflow (step 2/4)
- The work bead they're implementing
- Only their steps (wisps), not orchestrator machinery
- Clear progression with checkmarks

### The Claude TODO Analogy

This maps perfectly to how Claude Code works:

| Claude Code | MEOW Stack |
|-------------|------------|
| Beads in `.beads/` | Work beads (persistent deliverables) |
| Claude's ephemeral TODO list | Wisps (procedural guidance) |
| Internal processing | Orchestrator beads (invisible machinery) |

Just as Claude has a TODO list that guides execution but disappears after completion, agents should have wisps that guide their work but get burned after use.

### Goals

1. **Clarity**: Agents see only what's relevant to them
2. **Progression**: Clear sense of "where am I in my workflow"
3. **Separation**: Work beads (WHAT) vs wisps (HOW) vs orchestrator (MACHINERY)
4. **Composability**: Workflows as functions, modules as libraries
5. **Simplicity**: Convention over configuration (`main` as default)
6. **Durability**: All beads in same store for crash recovery

---

## Current State Analysis

### Parser (`internal/template/parser.go`)

**Current Design:**
```go
type Template struct {
    Meta      Meta            `toml:"meta"`
    Variables map[string]Var  `toml:"variables"`
    Steps     []Step          `toml:"steps"`
}
```

This is a single-workflow-per-file design. The `[meta]` section contains template metadata, `[variables]` defines inputs, and `[[steps]]` defines the workflow steps.

**Limitations:**
1. Cannot define multiple workflows in one file
2. No concept of local references
3. No `internal` visibility modifier
4. Forces file explosion for complex workflows

**Example of Current Format:**
```toml
[meta]
name = "implement-tdd"
description = "TDD workflow"

[variables]
work_bead = { required = true, type = "bead_id" }

[[steps]]
id = "load-context"
type = "task"
title = "Load context"

[[steps]]
id = "write-tests"
type = "task"
needs = ["load-context"]
```

### Loader (`internal/template/loader.go`)

**Current Design:**
```go
func (l *Loader) Load(name string) (*Template, error) {
    // Searches: project/.meow/templates/, user config, embedded
    // Returns single template
}
```

**Limitations:**
1. Only loads by template name (filename without `.toml`)
2. No `#workflow` suffix for selecting named workflows
3. No `.` prefix for local references
4. No concept of "current file context"

### Baker (`internal/template/baker.go`)

**Current Design:**
```go
type Baker struct {
    WorkflowID string
    VarContext *VarContext
    ParentBead string
    Assignee   string
}

func (b *Baker) Bake(tmpl *Template) (*BakeResult, error) {
    // Validates, substitutes variables, creates beads
}
```

**Limitations:**
1. No wisp detection - all task beads treated equally
2. No automatic tier classification
3. No `attach_wisp` handling
4. Labels only set for explicit `ephemeral: true`

### Bead Types (`internal/types/bead.go`)

**Current Fields:**
```go
type Bead struct {
    ID           string
    Type         BeadType      // task, condition, start, stop, code, expand
    Title        string
    Description  string
    Status       BeadStatus    // open, in_progress, closed
    Assignee     string
    Needs        []string
    Labels       []string      // includes "meow:ephemeral"
    Notes        string
    Parent       string
    Outputs      map[string]any
    // ... type-specific specs
}
```

**Missing Fields (from spec):**
- `HookBead` - link to work bead being implemented
- `SourceFormula` - which template created this bead
- `OrchestratorTask` - flag for orchestrator-tracked tasks
- Status `hooked` - on agent's hook, being executed

### Orchestrator (`internal/orchestrator/orchestrator.go`)

**Current Design:**
- Handles all 6 primitives correctly
- Has crash recovery via state persistence
- Has ephemeral cleanup at workflow end
- Condition beads run in goroutines (non-blocking)

**Limitations:**
1. `GetNextReady` returns any ready bead - no tier prioritization
2. `handleStart` doesn't process `attach_wisp`
3. No filtering for agent-specific visibility
4. No wisp lifecycle management (burn/squash)

### Open Beads Affecting This Work

From current backlog:
- `meow-e10.1-e10.4`: CLI commands (init, run, prime, close) - stubs
- `meow-e5.1-e5.3`: Agent system (tmux, state, spawning) - not done
- `meow-e7.2-e7.4`: Output system (storage, resolution, validation) - not done
- `meow-e9.1-e9.7`: Templates (work-loop, call, implement-tdd) - not done

---

## Architectural Deep Dive

### Three-Tier Bead Model

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          BEAD VISIBILITY MODEL                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  TIER 1: WORK BEADS                                                         │
│  ─────────────────                                                          │
│                                                                             │
│  What they are:                                                             │
│    Actual deliverables derived from PRD breakdown, user creation, etc.      │
│                                                                             │
│  Examples:                                                                  │
│    - "Implement user authentication endpoint" (task)                        │
│    - "Fix login timeout bug" (bug)                                          │
│    - "User Management System" (epic)                                        │
│                                                                             │
│  Characteristics:                                                           │
│    - Created by: humans, PRD breakdown, `bd create`                         │
│    - Visible via: `bd ready`, `bv --robot-triage`, `bd list`                │
│    - Agent role: SELECTS from these based on priority/impact                │
│    - Persistence: Permanent, exported to JSONL, git-synced                  │
│    - Ephemeral: NEVER (false always)                                        │
│    - Labels: No meow: prefix                                                │
│                                                                             │
│  Key insight: These represent THE WORK. They exist before any workflow      │
│  runs and persist after workflows complete.                                 │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  TIER 2: WORKFLOW BEADS (WISPS)                                             │
│  ──────────────────────────────                                             │
│                                                                             │
│  What they are:                                                             │
│    Procedural guidance for HOW to do work. Steps an agent follows.          │
│                                                                             │
│  Examples:                                                                  │
│    - "Load context for gt-123" (task, wisp step 1/4)                        │
│    - "Write failing tests" (task, wisp step 2/4)                            │
│    - "Commit changes" (task, wisp step 4/4)                                 │
│                                                                             │
│  Characteristics:                                                           │
│    - Created by: Template expansion (expand, condition branches)            │
│    - Visible via: `meow prime`, `meow steps`                                │
│    - Agent role: EXECUTES step-by-step (on their "hook")                    │
│    - Persistence: Durable in DB, NOT exported to JSONL                      │
│    - Ephemeral: true                                                        │
│    - Labels: ["meow:wisp", "meow:workflow:{id}"]                            │
│                                                                             │
│  Key insight: These are like a TODO list. They guide work but aren't        │
│  THE WORK themselves. They can be burned after completion.                  │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  TIER 3: ORCHESTRATOR BEADS                                                 │
│  ──────────────────────────                                                 │
│                                                                             │
│  What they are:                                                             │
│    Infrastructure machinery. Control flow, agent lifecycle, expansions.     │
│                                                                             │
│  Examples:                                                                  │
│    - "start-worker" (start bead)                                            │
│    - "check-tests" (condition bead)                                         │
│    - "expand-implement" (expand bead)                                       │
│    - "setup-worktree" (code bead)                                           │
│    - "stop-worker" (stop bead)                                              │
│                                                                             │
│  Characteristics:                                                           │
│    - Created by: Template expansion (non-task primitives)                   │
│    - Visible via: NEVER shown to agents, only in debug views                │
│    - Agent role: NONE - orchestrator handles exclusively                    │
│    - Persistence: Durable in DB, NOT exported to JSONL                      │
│    - Ephemeral: true                                                        │
│    - Labels: ["meow:orchestrator", "meow:workflow:{id}"]                    │
│                                                                             │
│  Key insight: Agents should never see these. They're the plumbing.          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Module System Design

#### File Format

**Legacy Single-Workflow (backward compatible):**
```toml
[meta]
name = "simple-task"
description = "A simple task template"

[[steps]]
id = "do-work"
type = "task"
```

**New Module Format:**
```toml
# ═══════════════════════════════════════════════════════════════════════════
# MAIN WORKFLOW (Default Entry Point)
# ═══════════════════════════════════════════════════════════════════════════

[main]
name = "work-loop"
description = "Main work selection and execution loop"

[[main.steps]]
id = "select-work"
type = "task"
# ...

# ═══════════════════════════════════════════════════════════════════════════
# IMPLEMENT WORKFLOW (Helper)
# ═══════════════════════════════════════════════════════════════════════════

[implement]
name = "implement"
description = "TDD implementation workflow"
ephemeral = true
internal = true  # Cannot be called from outside this file

[[implement.steps]]
id = "load-context"
# ...
```

#### Detection Logic

How does the parser know which format a file uses?

```go
func detectFormat(data []byte) FileFormat {
    // Try to decode as module first
    var probe struct {
        Meta *struct{} `toml:"meta"`  // Legacy format has [meta]
    }
    toml.Unmarshal(data, &probe)

    if probe.Meta != nil {
        return FormatLegacy
    }
    return FormatModule
}
```

Key insight: Legacy format ALWAYS has `[meta]` with `name` field. Module format has `[workflow-name]` sections with `[[workflow-name.steps]]`.

#### Reference Resolution

| Reference | Meaning | Resolution |
|-----------|---------|------------|
| `.implement` | Same file, workflow "implement" | Look up in current module's workflows map |
| `.implement.verify` | Same file, workflow "implement", starting at step "verify" | Advanced: partial workflow execution |
| `helpers#tdd` | File "helpers.toml", workflow "tdd" | Load file, extract named workflow |
| `helpers` | File "helpers.toml", workflow "main" | Load file, use default entry point |
| `./subdir/util.toml#helper` | Relative path | Resolve relative to current file |
| `/abs/path.toml#workflow` | Absolute path | Direct file access |

**Current File Context:**

The loader needs to track where it's loading from:

```go
type LoadContext struct {
    CurrentFile string  // e.g., "/project/.meow/templates/work-module.toml"
    Module      *Module // Parsed module for local references
}

func (l *Loader) ResolveReference(ref string, ctx *LoadContext) (*Workflow, error) {
    if strings.HasPrefix(ref, ".") {
        // Local reference
        name := strings.TrimPrefix(ref, ".")
        if ctx.Module == nil {
            return nil, fmt.Errorf("local reference %q but no module context", ref)
        }
        workflow, ok := ctx.Module.Workflows[name]
        if !ok {
            return nil, fmt.Errorf("workflow %q not found in module %q", name, ctx.CurrentFile)
        }
        if workflow.Internal && ctx.External {
            return nil, fmt.Errorf("workflow %q is internal and cannot be called externally", name)
        }
        return workflow, nil
    }
    // ... external reference handling
}
```

### Automatic Wisp Detection Algorithm

When a template is baked, the baker automatically partitions beads:

```go
func (b *Baker) BakeWithWispDetection(module *Module, workflowName string) (*BakeResult, error) {
    workflow := module.Workflows[workflowName]

    // Phase 1: Identify agents (from start beads without attach_wisp)
    agentStarts := make(map[string]*Step)  // agent ID -> start step
    for _, step := range workflow.Steps {
        if step.Type == "start" && step.AttachWisp == nil {
            agentStarts[step.Agent] = step
        }
    }

    // Phase 2: Classify each step
    for _, step := range workflow.Steps {
        bead := b.stepToBead(step)

        if step.Type == "task" {
            if step.Assignee != "" && agentStarts[step.Assignee] != nil {
                // Task assigned to an agent we're starting → WISP
                bead.Labels = append(bead.Labels, "meow:wisp")
                bead.Labels = append(bead.Labels, "meow:workflow:"+b.WorkflowID)
                bead.Ephemeral = true
            } else if step.OrchestratorTask {
                // Explicitly marked as orchestrator task (e.g., human gates)
                bead.Labels = append(bead.Labels, "meow:orchestrator")
                bead.Ephemeral = true
            }
            // else: Regular work bead (no special labels, not ephemeral)
        } else {
            // Non-task primitive → ORCHESTRATOR
            bead.Labels = append(bead.Labels, "meow:orchestrator")
            bead.Labels = append(bead.Labels, "meow:workflow:"+b.WorkflowID)
            bead.Ephemeral = true
        }

        result.Beads = append(result.Beads, bead)
    }

    return result, nil
}
```

### The `attach_wisp` Override

For complex cases where auto-detection isn't sufficient:

```toml
[[main.steps]]
id = "start-worker"
type = "start"
agent = "claude-2"
workdir = "{{setup-worktree.outputs.path}}"

# Explicit wisp attachment
[main.steps.attach_wisp]
template = "agent-implement"
variables = { work_bead = "{{work_bead}}" }
```

When `attach_wisp` is set:
1. The referenced template is baked as a separate wisp
2. Auto-detection is SKIPPED for this agent
3. Inline tasks with `assignee = "claude-2"` become orchestrator beads (not wisps)

This allows splitting orchestrator and agent views into separate templates for maximum flexibility.

### Agent Visibility Filtering

**`meow prime` implementation:**

```go
func Prime(ctx context.Context, store Storage, agentID string) (*PrimeOutput, error) {
    // Get wisp steps for this agent (in progress or ready)
    wispFilter := IssueFilter{
        Assignee: &agentID,
        Labels:   []string{"meow:wisp"},
        Statuses: []string{"open", "in_progress", "hooked"},
    }
    wispSteps, _ := store.Search(ctx, wispFilter)

    if len(wispSteps) == 0 {
        return &PrimeOutput{Message: "No active workflow"}, nil
    }

    // Find the current step (first ready or in_progress)
    var current *Issue
    for _, step := range wispSteps {
        if step.Status == "in_progress" || step.Status == "hooked" {
            current = step
            break
        }
        if step.Status == "open" && isReady(step, store) {
            current = step
            break
        }
    }

    // Get linked work bead
    var workBead *Issue
    if current != nil && current.HookBead != "" {
        workBead, _ = store.Get(ctx, current.HookBead)
    }

    return &PrimeOutput{
        Wisp: WispProgress{
            Name:      current.SourceFormula,
            Total:     len(wispSteps),
            Completed: countClosed(wispSteps),
            Current:   current,
            Steps:     wispSteps,
        },
        WorkBead: workBead,
    }, nil
}
```

**`bd ready` implementation (unchanged but with filtering):**

```go
func GetReadyWork(ctx context.Context, store Storage) ([]*Issue, error) {
    filter := IssueFilter{
        Statuses:      []string{"open"},
        ExcludeLabels: []string{"meow:wisp", "meow:orchestrator"},
        ExcludeTypes:  []string{"start", "stop", "condition", "expand", "code"},
    }
    return store.GetReady(ctx, filter)
}
```

### Wisp Lifecycle

```
┌──────────────────────────────────────────────────────────────────────────┐
│                        WISP LIFECYCLE                                     │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  CREATION (via template expansion)                                       │
│  ──────────────────────────────────                                      │
│                                                                          │
│  1. `meow run workflow.toml` or `expand` bead processes                  │
│  2. Baker identifies task beads with assignee matching start bead        │
│  3. Labels applied: ["meow:wisp", "meow:workflow:{id}"]                  │
│  4. HookBead set to work bead ID (from variables)                        │
│  5. SourceFormula set to template name                                   │
│                                                                          │
│  EXECUTION (agent works through steps)                                   │
│  ──────────────────────────────────────                                  │
│                                                                          │
│  1. Agent runs `meow prime` → sees wisp progression                      │
│  2. Status: open → hooked (when agent claims) → in_progress → closed     │
│  3. Agent runs `meow close <step>` with outputs                          │
│  4. Orchestrator finds next ready wisp step                              │
│  5. Repeat until all wisp steps closed                                   │
│                                                                          │
│  COMPLETION (wisp cleanup)                                               │
│  ─────────────────────────                                               │
│                                                                          │
│  Option A: Auto-burn (default)                                           │
│    - When all wisp steps closed, delete them                             │
│    - Work bead remains with notes                                        │
│                                                                          │
│  Option B: Squash to digest                                              │
│    - Create summary record of what was done                              │
│    - Attach digest to work bead notes                                    │
│    - Delete wisp beads                                                   │
│                                                                          │
│  Option C: Preserve (for debugging)                                      │
│    - Keep wisp beads but mark completed                                  │
│    - Can be manually cleaned later                                       │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Implementation Phases

### Phase 1: Foundation (Parser & Types)

**Goal**: Support parsing module files and new bead metadata.

**Deliverables**:
- Parser can detect and parse module format
- Types have new fields (HookBead, SourceFormula, etc.)
- Backward compatibility with legacy single-workflow files

**Why first**: Everything else depends on being able to parse modules and store the new metadata.

### Phase 2: Resolution & Baking (Loader & Baker)

**Goal**: Resolve references and bake with wisp detection.

**Deliverables**:
- Loader handles `.local`, `file#workflow` references
- Baker partitions beads into tiers automatically
- `attach_wisp` handling in baker

**Why second**: Needs the parser/types from Phase 1. The orchestrator needs correct beads.

### Phase 3: Orchestration (Orchestrator & Agent)

**Goal**: Tier-aware bead processing and agent visibility.

**Deliverables**:
- Orchestrator prioritizes orchestrator beads
- `handleStart` processes `attach_wisp`
- Agent state tracks hook/wisp context

**Why third**: Needs correctly baked beads from Phase 2.

### Phase 4: CLI & Templates (Commands & Examples)

**Goal**: User-facing commands and example templates.

**Deliverables**:
- `meow prime` shows wisp view
- `meow close` with output validation
- `meow continue` for crash recovery
- Example templates in module format

**Why last**: Needs all underlying systems from Phases 1-3.

---

## Detailed Bead Definitions

### Epic Structure

```
meow-modules (P0, Epic)
├── meow-modules-parser (P0, Epic) - Phase 1
│   ├── meow-modules-parser-detect
│   ├── meow-modules-parser-types
│   ├── meow-modules-parser-parse
│   ├── meow-modules-parser-validate
│   └── meow-modules-parser-legacy
│
├── meow-modules-types (P0, Epic) - Phase 1
│   ├── meow-modules-types-fields
│   ├── meow-modules-types-status
│   ├── meow-modules-types-labels
│   └── meow-modules-types-filter
│
├── meow-modules-loader (P0, Epic) - Phase 2
│   ├── meow-modules-loader-context
│   ├── meow-modules-loader-local
│   ├── meow-modules-loader-external
│   └── meow-modules-loader-errors
│
├── meow-modules-baker (P0, Epic) - Phase 2
│   ├── meow-modules-baker-agents
│   ├── meow-modules-baker-partition
│   ├── meow-modules-baker-labels
│   ├── meow-modules-baker-attach
│   └── meow-modules-baker-hookbead
│
├── meow-modules-orch (P0, Epic) - Phase 3
│   ├── meow-modules-orch-priority
│   ├── meow-modules-orch-start
│   ├── meow-modules-orch-filter
│   └── meow-modules-orch-cleanup
│
├── meow-modules-cli (P0, Epic) - Phase 4
│   ├── meow-modules-cli-prime
│   ├── meow-modules-cli-steps
│   ├── meow-modules-cli-close
│   └── meow-modules-cli-continue
│
└── meow-modules-templates (P1, Epic) - Phase 4
    ├── meow-modules-templates-workloop
    ├── meow-modules-templates-implement
    └── meow-modules-templates-call
```

---

### EPIC: meow-modules-parser

```yaml
id: meow-modules-parser
type: epic
priority: P0
title: "Parser: Multi-Workflow Module Support"
description: |
  Extend the template parser to support multi-workflow module files.

  ## Background

  The current parser (`internal/template/parser.go`) only supports single-workflow
  files with `[meta]`, `[variables]`, and `[[steps]]` sections. This limits
  composability and leads to file explosion for complex workflows.

  ## Goal

  Support a new module format where a single file can contain multiple named
  workflows that can reference each other.

  ## Design Decisions

  1. **Detection, not configuration**: The parser auto-detects format based on
     structure (presence of `[meta]` = legacy, presence of `[workflow-name]` = module)

  2. **Backward compatibility**: Legacy single-workflow files continue to work
     unchanged

  3. **Convention over configuration**: `[main]` is the default entry point

  4. **Validation at parse time**: All local references validated when file is parsed

  ## Success Criteria

  - Parse legacy format: PASS
  - Parse module format: PASS
  - Detect format automatically: PASS
  - Validate local references: PASS
  - Error messages are helpful: PASS

labels:
  - "phase:1"
  - "component:parser"
needs: []
```

#### Task: meow-modules-parser-types

```yaml
id: meow-modules-parser-types
type: task
priority: P0
title: "Define module-level parser types"
parent: meow-modules-parser
description: |
  Create the Go types for parsing module files.

  ## Types to Define

  ```go
  // FileFormat indicates the template file format
  type FileFormat int

  const (
      FormatLegacy FileFormat = iota  // [meta] + [[steps]]
      FormatModule                     // [workflow-name] sections
  )

  // ModuleFile represents a parsed module file
  type ModuleFile struct {
      Path      string               // File path for error messages
      Workflows map[string]*Workflow // Named workflows
  }

  // Workflow represents a single workflow within a module
  type Workflow struct {
      Name        string           `toml:"name"`
      Description string           `toml:"description,omitempty"`
      Ephemeral   bool             `toml:"ephemeral,omitempty"`
      Internal    bool             `toml:"internal,omitempty"`
      Variables   map[string]Var   `toml:"variables,omitempty"`
      Steps       []Step           `toml:"steps"`
  }
  ```

  ## File Location

  Create new file: `internal/template/module.go`

  ## Considerations

  - Keep `Template` type for backward compatibility (single-workflow)
  - `Workflow` should be usable standalone or within `ModuleFile`
  - Add helper methods: `GetWorkflow(name)`, `DefaultWorkflow()`, `IsInternal(name)`

  ## Acceptance Criteria

  - [ ] Types defined in `internal/template/module.go`
  - [ ] Types have proper TOML tags
  - [ ] Helper methods implemented
  - [ ] Unit tests for helper methods

instructions: |
  1. Create `internal/template/module.go`
  2. Define `FileFormat` enum
  3. Define `ModuleFile` struct with `Workflows` map
  4. Define `Workflow` struct (similar to current `Template` but with `Internal` field)
  5. Add helper methods:
     - `(m *ModuleFile) GetWorkflow(name string) (*Workflow, bool)`
     - `(m *ModuleFile) DefaultWorkflow() (*Workflow, error)` - returns "main" or error
     - `(m *ModuleFile) IsInternal(name string) bool`
  6. Write unit tests in `internal/template/module_test.go`

needs: []
labels:
  - "phase:1"
  - "component:parser"
```

#### Task: meow-modules-parser-detect

```yaml
id: meow-modules-parser-detect
type: task
priority: P0
title: "Implement format detection logic"
parent: meow-modules-parser
description: |
  Implement logic to detect whether a file is legacy or module format.

  ## Detection Algorithm

  ```go
  func DetectFormat(data []byte) (FileFormat, error) {
      // Strategy: Try to find [meta] section
      // Legacy format ALWAYS has [meta] with name = "..."
      // Module format has [workflow-name] sections

      // Use TOML's table structure to probe
      var probe struct {
          Meta struct {
              Name string `toml:"name"`
          } `toml:"meta"`
      }

      if _, err := toml.Decode(string(data), &probe); err != nil {
          return 0, fmt.Errorf("invalid TOML: %w", err)
      }

      if probe.Meta.Name != "" {
          return FormatLegacy, nil
      }

      return FormatModule, nil
  }
  ```

  ## Edge Cases

  1. Empty file → Error
  2. Invalid TOML → Error with helpful message
  3. Has `[meta]` but no `name` → Treat as module (could be workflow named "meta")
  4. Has both `[meta]` and `[main]` → Ambiguous, error with suggestion

  ## Acceptance Criteria

  - [ ] Detects legacy format correctly
  - [ ] Detects module format correctly
  - [ ] Handles edge cases gracefully
  - [ ] Error messages suggest fixes

instructions: |
  1. Add `DetectFormat(data []byte) (FileFormat, error)` to `module.go`
  2. Implement probing logic
  3. Handle edge cases with specific error types
  4. Write comprehensive tests including edge cases

needs:
  - meow-modules-parser-types
labels:
  - "phase:1"
  - "component:parser"
```

#### Task: meow-modules-parser-parse

```yaml
id: meow-modules-parser-parse
type: task
priority: P0
title: "Implement module parsing"
parent: meow-modules-parser
description: |
  Implement parsing of module-format files into `ModuleFile` struct.

  ## Challenge: Dynamic TOML Parsing

  Unlike legacy format where section names are fixed (`[meta]`, `[[steps]]`),
  module format has dynamic section names (`[workflow-name]`).

  ## Solution: Two-Phase Parse

  ```go
  func ParseModule(data []byte) (*ModuleFile, error) {
      // Phase 1: Decode into map to discover workflow names
      var raw map[string]toml.Primitive
      meta, err := toml.Decode(string(data), &raw)
      if err != nil {
          return nil, err
      }

      module := &ModuleFile{
          Workflows: make(map[string]*Workflow),
      }

      // Phase 2: Decode each workflow
      for name := range raw {
          var workflow Workflow
          if err := meta.PrimitiveDecode(raw[name], &workflow); err != nil {
              return nil, fmt.Errorf("parsing workflow %q: %w", name, err)
          }
          // Default name to section name if not specified
          if workflow.Name == "" {
              workflow.Name = name
          }
          module.Workflows[name] = &workflow
      }

      return module, nil
  }
  ```

  ## TOML Structure Challenge

  The TOML library we use (BurntSushi/toml) can handle this with `Primitive` types
  and `PrimitiveDecode`. We need to decode each top-level table as a workflow.

  ## Acceptance Criteria

  - [ ] Parses module files with multiple workflows
  - [ ] Preserves workflow order (for deterministic behavior)
  - [ ] Handles workflows without explicit `name` field
  - [ ] Reports parsing errors with workflow context

instructions: |
  1. Add `ParseModule(data []byte) (*ModuleFile, error)` to `module.go`
  2. Implement two-phase parsing with `toml.Primitive`
  3. Handle workflow name defaulting
  4. Add integration tests with real module files

needs:
  - meow-modules-parser-types
labels:
  - "phase:1"
  - "component:parser"
```

#### Task: meow-modules-parser-validate

```yaml
id: meow-modules-parser-validate
type: task
priority: P0
title: "Implement module validation"
parent: meow-modules-parser
description: |
  Validate module files after parsing, including local reference validation.

  ## Validations

  1. **At least one workflow**: Module must have at least one workflow

  2. **Unique workflow names**: No duplicates (TOML prevents this, but check anyway)

  3. **Valid step references**: All `needs` reference existing steps within workflow

  4. **Valid local references**: All `.workflow` references exist in module

  5. **No circular references**: Detect workflow A → workflow B → workflow A

  6. **Internal visibility**: External references can't reach `internal = true` workflows

  ## Implementation

  ```go
  func (m *ModuleFile) Validate() *ValidationResult {
      result := &ValidationResult{}

      // Check at least one workflow
      if len(m.Workflows) == 0 {
          result.AddError("module has no workflows")
      }

      // Validate each workflow
      for name, workflow := range m.Workflows {
          result.Merge(workflow.Validate())

          // Validate local references
          for _, step := range workflow.Steps {
              if ref := step.Template; strings.HasPrefix(ref, ".") {
                  targetName := strings.TrimPrefix(ref, ".")
                  if _, ok := m.Workflows[targetName]; !ok {
                      result.AddError("workflow %q references unknown workflow %q", name, targetName)
                  }
              }
          }
      }

      // Check for cycles
      if err := m.detectCycles(); err != nil {
          result.AddError("cycle detected: %v", err)
      }

      return result
  }
  ```

  ## Cycle Detection

  Use DFS with coloring (white/gray/black) to detect cycles in workflow references.

  ## Acceptance Criteria

  - [ ] All validations implemented
  - [ ] Cycle detection works
  - [ ] Error messages include workflow/step context
  - [ ] Validation is called automatically after parse

instructions: |
  1. Add `(m *ModuleFile) Validate() *ValidationResult`
  2. Implement all validation checks
  3. Implement cycle detection using graph coloring
  4. Call validation at end of `ParseModule`
  5. Write tests for each validation type

needs:
  - meow-modules-parser-parse
labels:
  - "phase:1"
  - "component:parser"
```

#### Task: meow-modules-parser-legacy

```yaml
id: meow-modules-parser-legacy
type: task
priority: P0
title: "Maintain legacy format compatibility"
parent: meow-modules-parser
description: |
  Ensure legacy single-workflow files continue to work unchanged.

  ## Strategy

  Create a unified parsing interface that handles both formats:

  ```go
  // ParseAny detects format and parses accordingly
  func ParseAny(data []byte) (*ModuleFile, error) {
      format, err := DetectFormat(data)
      if err != nil {
          return nil, err
      }

      switch format {
      case FormatLegacy:
          // Parse as Template, convert to ModuleFile
          tmpl, err := Parse(bytes.NewReader(data))
          if err != nil {
              return nil, err
          }
          return tmpl.ToModule(), nil

      case FormatModule:
          return ParseModule(data)
      }

      return nil, fmt.Errorf("unknown format")
  }

  // ToModule converts a legacy Template to a ModuleFile
  func (t *Template) ToModule() *ModuleFile {
      return &ModuleFile{
          Workflows: map[string]*Workflow{
              "main": {
                  Name:        t.Meta.Name,
                  Description: t.Meta.Description,
                  Variables:   t.Variables,
                  Steps:       t.Steps,
              },
          },
      }
  }
  ```

  ## Key Insight

  Converting legacy to module format means existing code can work with `ModuleFile`
  uniformly. The legacy `Template` type remains for backward compatibility but
  internally everything uses `ModuleFile`.

  ## Acceptance Criteria

  - [ ] `ParseAny` handles both formats
  - [ ] Legacy files work with existing tests
  - [ ] `Template.ToModule()` conversion works
  - [ ] No breaking changes to existing API

instructions: |
  1. Add `ParseAny(data []byte) (*ModuleFile, error)` as main entry point
  2. Add `(t *Template) ToModule() *ModuleFile` conversion
  3. Ensure all existing parser tests still pass
  4. Add integration tests that use both formats in same test

needs:
  - meow-modules-parser-validate
labels:
  - "phase:1"
  - "component:parser"
```

---

### EPIC: meow-modules-types

```yaml
id: meow-modules-types
type: epic
priority: P0
title: "Types: Three-Tier Bead Metadata"
description: |
  Extend bead types to support three-tier visibility model.

  ## Background

  Currently, beads have minimal metadata for tier classification. The only
  mechanism is the `labels` field with "meow:ephemeral". This isn't enough
  to distinguish wisps from orchestrator beads, or to track which work bead
  a wisp is implementing.

  ## New Fields Needed

  1. `HookBead` - Link to the work bead this wisp is implementing
  2. `SourceFormula` - Which template created this bead
  3. `OrchestratorTask` - Flag for tasks that orchestrator tracks (not agent)

  ## New Status Needed

  `hooked` - When an agent claims a wisp step but hasn't started executing

  ## Success Criteria

  - [ ] New fields added without breaking existing code
  - [ ] Serialization/deserialization works
  - [ ] Migration strategy for existing beads (if any)

labels:
  - "phase:1"
  - "component:types"
needs: []
```

#### Task: meow-modules-types-fields

```yaml
id: meow-modules-types-fields
type: task
priority: P0
title: "Add wisp tracking fields to Bead struct"
parent: meow-modules-types
description: |
  Add new fields to the Bead struct for three-tier tracking.

  ## Fields to Add

  ```go
  type Bead struct {
      // ... existing fields ...

      // Wisp tracking
      HookBead      string `json:"hook_bead,omitempty" toml:"hook_bead,omitempty"`
      SourceFormula string `json:"source_formula,omitempty" toml:"source_formula,omitempty"`

      // Orchestrator task flag
      OrchestratorTask bool `json:"orchestrator_task,omitempty" toml:"orchestrator_task,omitempty"`
  }
  ```

  ## Field Semantics

  ### HookBead

  For wisp beads, this is the ID of the work bead being implemented.

  Example: Wisp step "write-tests" has `HookBead = "gt-123"` where gt-123 is
  "Implement auth endpoint".

  This enables:
  - `meow prime` showing "Work bead: gt-123"
  - Closing work bead when wisp completes
  - Linking audit trail

  ### SourceFormula

  The template name that created this bead.

  Example: Bead created from `implement-tdd.toml` has `SourceFormula = "implement-tdd"`

  This enables:
  - Grouping wisp steps by source
  - Debugging which template created a bead
  - Wisp progress display ("implement-tdd step 2/4")

  ### OrchestratorTask

  For task beads that aren't executed by agents.

  Example: Human approval gates are tasks but not assigned to Claude. They're
  closed by humans via `meow approve`.

  ## Acceptance Criteria

  - [ ] Fields added to Bead struct
  - [ ] JSON/TOML serialization works
  - [ ] Existing bead tests still pass
  - [ ] Helper methods: `(b *Bead) IsWisp() bool`, `(b *Bead) IsOrchestrator() bool`

instructions: |
  1. Add fields to `Bead` struct in `internal/types/bead.go`
  2. Add helper methods for tier detection
  3. Update any existing serialization tests
  4. Add new tests for the helper methods

needs: []
labels:
  - "phase:1"
  - "component:types"
```

#### Task: meow-modules-types-status

```yaml
id: meow-modules-types-status
type: task
priority: P0
title: "Add 'hooked' status for wisp beads"
parent: meow-modules-types
description: |
  Add a new bead status for when a wisp step is claimed by an agent.

  ## Current Statuses

  - `open` - Not started
  - `in_progress` - Being worked on
  - `closed` - Done

  ## New Status: hooked

  When an agent claims a wisp step via `meow prime`, it transitions from
  `open` to `hooked`. When the agent starts actual execution, it becomes
  `in_progress`. This gives visibility into the pipeline:

  ```
  open → hooked → in_progress → closed
         ↑
         └── Agent claimed via meow prime
  ```

  ## Why Needed

  1. **Pipeline visibility**: See what's queued vs actively running
  2. **Crash recovery**: `hooked` beads from dead agents can be reclaimed
  3. **Multi-agent coordination**: Know which agent has which step

  ## Implementation

  ```go
  const (
      BeadStatusOpen       BeadStatus = "open"
      BeadStatusHooked     BeadStatus = "hooked"     // NEW
      BeadStatusInProgress BeadStatus = "in_progress"
      BeadStatusClosed     BeadStatus = "closed"
  )

  func (s BeadStatus) CanTransitionTo(target BeadStatus) bool {
      switch s {
      case BeadStatusOpen:
          return target == BeadStatusHooked || target == BeadStatusInProgress || target == BeadStatusClosed
      case BeadStatusHooked:
          return target == BeadStatusInProgress || target == BeadStatusOpen  // Allow unclaiming
      case BeadStatusInProgress:
          return target == BeadStatusClosed || target == BeadStatusOpen
      case BeadStatusClosed:
          return target == BeadStatusOpen  // Allow reopening
      }
      return false
  }
  ```

  ## Acceptance Criteria

  - [ ] `BeadStatusHooked` constant added
  - [ ] Status transitions updated
  - [ ] `Valid()` method updated
  - [ ] Existing status tests pass

instructions: |
  1. Add `BeadStatusHooked` to `internal/types/bead.go`
  2. Update `Valid()` to include hooked
  3. Update `CanTransitionTo()` with new transitions
  4. Update tests

needs: []
labels:
  - "phase:1"
  - "component:types"
```

#### Task: meow-modules-types-labels

```yaml
id: meow-modules-types-labels
type: task
priority: P0
title: "Define standard label conventions for tiers"
parent: meow-modules-types
description: |
  Define and document the label conventions for bead tier classification.

  ## Label Conventions

  | Label | Meaning |
  |-------|---------|
  | `meow:wisp` | Workflow bead (Tier 2) |
  | `meow:orchestrator` | Infrastructure bead (Tier 3) |
  | `meow:workflow:{id}` | Belongs to workflow with given ID |
  | `meow:ephemeral` | Should be cleaned up (existing) |

  ## Implementation

  Add constants and helper functions:

  ```go
  const (
      LabelWisp         = "meow:wisp"
      LabelOrchestrator = "meow:orchestrator"
      LabelEphemeral    = "meow:ephemeral"
  )

  func LabelWorkflow(id string) string {
      return "meow:workflow:" + id
  }

  func (b *Bead) HasLabel(label string) bool {
      for _, l := range b.Labels {
          if l == label {
              return true
          }
      }
      return false
  }

  func (b *Bead) AddLabel(label string) {
      if !b.HasLabel(label) {
          b.Labels = append(b.Labels, label)
      }
  }

  func (b *Bead) WorkflowID() string {
      for _, l := range b.Labels {
          if strings.HasPrefix(l, "meow:workflow:") {
              return strings.TrimPrefix(l, "meow:workflow:")
          }
      }
      return ""
  }
  ```

  ## Tier Detection

  ```go
  func (b *Bead) Tier() BeadTier {
      if b.HasLabel(LabelOrchestrator) {
          return TierOrchestrator
      }
      if b.HasLabel(LabelWisp) {
          return TierWisp
      }
      // Default: work bead
      return TierWork
  }
  ```

  ## Acceptance Criteria

  - [ ] Label constants defined
  - [ ] Helper functions implemented
  - [ ] `Tier()` method returns correct tier
  - [ ] Documentation comments explain usage

instructions: |
  1. Add label constants to `internal/types/bead.go`
  2. Add `BeadTier` enum type
  3. Implement helper methods
  4. Add tests for tier detection

needs:
  - meow-modules-types-fields
labels:
  - "phase:1"
  - "component:types"
```

#### Task: meow-modules-types-filter

```yaml
id: meow-modules-types-filter
type: task
priority: P0
title: "Add filter types for tier-based queries"
parent: meow-modules-types
description: |
  Add filter types that support tier-based bead queries.

  ## Current State

  The orchestrator uses inline filter logic. We need reusable filter types
  that the bead store can use.

  ## Filter Types

  ```go
  // BeadFilter specifies criteria for bead queries
  type BeadFilter struct {
      // Status filters
      Statuses      []BeadStatus  // Include these statuses
      ExcludeStatuses []BeadStatus

      // Type filters
      Types         []BeadType
      ExcludeTypes  []BeadType

      // Label filters
      Labels        []string     // Must have all these labels
      ExcludeLabels []string     // Must not have any of these

      // Assignment filters
      Assignee      *string      // nil = any, "" = unassigned, "x" = assigned to x

      // Tier shorthand (sets Labels/ExcludeLabels)
      Tier          *BeadTier    // nil = any tier

      // Relationship filters
      Parent        *string      // Filter by parent bead
      HookBead      *string      // Filter by hook bead
      WorkflowID    *string      // Filter by workflow (via label)
  }

  // Convenience constructors
  func WorkBeadFilter() BeadFilter {
      return BeadFilter{
          ExcludeLabels: []string{LabelWisp, LabelOrchestrator},
      }
  }

  func WispFilter(agentID string) BeadFilter {
      return BeadFilter{
          Labels:   []string{LabelWisp},
          Assignee: &agentID,
      }
  }

  func OrchestratorFilter(workflowID string) BeadFilter {
      return BeadFilter{
          Labels: []string{LabelOrchestrator, LabelWorkflow(workflowID)},
      }
  }
  ```

  ## Acceptance Criteria

  - [ ] `BeadFilter` struct defined
  - [ ] Convenience constructors implemented
  - [ ] Filter logic can be used by bead store

instructions: |
  1. Add `BeadFilter` struct to `internal/types/bead.go`
  2. Add convenience constructors
  3. Add `(f BeadFilter) Matches(b *Bead) bool` method
  4. Write tests for filter matching

needs:
  - meow-modules-types-labels
labels:
  - "phase:1"
  - "component:types"
```

---

### EPIC: meow-modules-loader

```yaml
id: meow-modules-loader
type: epic
priority: P0
title: "Loader: Reference Resolution"
description: |
  Extend the template loader to resolve module references.

  ## Background

  Current loader only loads templates by name from predefined paths. It has
  no concept of:
  - Local references (`.implement`)
  - Named workflow selection (`file#workflow`)
  - Current file context

  ## Goal

  Support full reference resolution for module composition.

  ## Reference Types

  | Reference | Example | Resolution |
  |-----------|---------|------------|
  | Local | `.implement` | Same file, workflow "implement" |
  | Local with step | `.implement.verify` | Same file, partial (future) |
  | External default | `work-loop` | File work-loop.toml, workflow "main" |
  | External named | `work-loop#cleanup` | File work-loop.toml, workflow "cleanup" |
  | Relative path | `./helpers.toml#util` | Relative file path |
  | Absolute path | `/path/to.toml#main` | Absolute file path |

  ## Success Criteria

  - [ ] All reference types resolve correctly
  - [ ] Circular reference detection
  - [ ] Helpful error messages
  - [ ] Context tracking for local references

labels:
  - "phase:2"
  - "component:loader"
needs:
  - meow-modules-parser
```

#### Task: meow-modules-loader-context

```yaml
id: meow-modules-loader-context
type: task
priority: P0
title: "Implement load context tracking"
parent: meow-modules-loader
description: |
  The loader needs to track context for resolving local references.

  ## Context Struct

  ```go
  // LoadContext tracks the current loading context
  type LoadContext struct {
      // Current file path (for resolving relative paths)
      CurrentFile string

      // Parsed module (for resolving local references)
      Module *ModuleFile

      // Stack of files being loaded (for cycle detection)
      LoadStack []string

      // Whether this is an external call (for internal visibility check)
      IsExternal bool
  }

  // NewLoadContext creates a root context
  func NewLoadContext() *LoadContext {
      return &LoadContext{
          LoadStack: make([]string, 0),
      }
  }

  // ForFile creates a child context for loading a file
  func (ctx *LoadContext) ForFile(path string, module *ModuleFile) *LoadContext {
      child := &LoadContext{
          CurrentFile: path,
          Module:      module,
          LoadStack:   append(append([]string{}, ctx.LoadStack...), path),
          IsExternal:  true,
      }
      return child
  }

  // ForLocalRef creates a child context for a local reference
  func (ctx *LoadContext) ForLocalRef() *LoadContext {
      child := &LoadContext{
          CurrentFile: ctx.CurrentFile,
          Module:      ctx.Module,
          LoadStack:   ctx.LoadStack,
          IsExternal:  false,  // Local refs can access internal
      }
      return child
  }

  // InStack checks if a path is already being loaded (cycle)
  func (ctx *LoadContext) InStack(path string) bool {
      for _, p := range ctx.LoadStack {
          if p == path {
              return true
          }
      }
      return false
  }
  ```

  ## Acceptance Criteria

  - [ ] `LoadContext` struct implemented
  - [ ] Context propagation methods work
  - [ ] Cycle detection via stack works
  - [ ] External vs local tracking works

instructions: |
  1. Create `internal/template/load_context.go`
  2. Implement `LoadContext` with all methods
  3. Add tests for context propagation and cycle detection

needs: []
labels:
  - "phase:2"
  - "component:loader"
```

#### Task: meow-modules-loader-local

```yaml
id: meow-modules-loader-local
type: task
priority: P0
title: "Implement local reference resolution"
parent: meow-modules-loader
description: |
  Implement resolution of `.workflow` local references.

  ## Algorithm

  ```go
  func (l *Loader) ResolveLocalRef(ref string, ctx *LoadContext) (*Workflow, error) {
      // Extract workflow name
      name := strings.TrimPrefix(ref, ".")

      // Handle step reference (future feature)
      if strings.Contains(name, ".") {
          parts := strings.SplitN(name, ".", 2)
          name = parts[0]
          // parts[1] would be step name
      }

      // Validate context
      if ctx.Module == nil {
          return nil, fmt.Errorf("local reference %q but no module context", ref)
      }

      // Look up workflow
      workflow, ok := ctx.Module.Workflows[name]
      if !ok {
          available := make([]string, 0, len(ctx.Module.Workflows))
          for n := range ctx.Module.Workflows {
              available = append(available, n)
          }
          return nil, fmt.Errorf("workflow %q not found in module; available: %v", name, available)
      }

      // Check internal visibility
      if workflow.Internal && ctx.IsExternal {
          return nil, fmt.Errorf("workflow %q is internal and cannot be referenced externally", name)
      }

      return workflow, nil
  }
  ```

  ## Integration

  Update `Load` to use context:

  ```go
  func (l *Loader) LoadWithContext(ref string, ctx *LoadContext) (*Workflow, error) {
      if strings.HasPrefix(ref, ".") {
          return l.ResolveLocalRef(ref, ctx)
      }
      return l.ResolveExternalRef(ref, ctx)
  }
  ```

  ## Acceptance Criteria

  - [ ] `.workflow` references resolve correctly
  - [ ] Internal workflows blocked for external refs
  - [ ] Error messages include available workflows
  - [ ] Integration with main Load method

instructions: |
  1. Add `ResolveLocalRef` to `internal/template/loader.go`
  2. Update `Load` to delegate to `LoadWithContext`
  3. Add tests for local reference resolution
  4. Add tests for internal visibility

needs:
  - meow-modules-loader-context
labels:
  - "phase:2"
  - "component:loader"
```

#### Task: meow-modules-loader-external

```yaml
id: meow-modules-loader-external
type: task
priority: P0
title: "Implement external reference resolution"
parent: meow-modules-loader
description: |
  Implement resolution of external references with `#workflow` syntax.

  ## Reference Formats

  - `name` → `name.toml#main`
  - `name#workflow` → `name.toml`, workflow "workflow"
  - `./path/to/file.toml#workflow` → relative path
  - `/abs/path.toml#workflow` → absolute path

  ## Algorithm

  ```go
  func (l *Loader) ResolveExternalRef(ref string, ctx *LoadContext) (*Workflow, error) {
      // Parse reference
      filePath, workflowName := parseRef(ref)

      // Resolve relative paths
      if !filepath.IsAbs(filePath) && ctx.CurrentFile != "" {
          dir := filepath.Dir(ctx.CurrentFile)
          filePath = filepath.Join(dir, filePath)
      }

      // Ensure .toml extension
      if !strings.HasSuffix(filePath, ".toml") {
          filePath += ".toml"
      }

      // Check for cycles
      absPath, _ := filepath.Abs(filePath)
      if ctx.InStack(absPath) {
          return nil, fmt.Errorf("circular reference detected: %v -> %s", ctx.LoadStack, absPath)
      }

      // Load the file
      data, err := l.readFile(filePath)
      if err != nil {
          return nil, fmt.Errorf("loading %q: %w", filePath, err)
      }

      // Parse as module
      module, err := ParseAny(data)
      if err != nil {
          return nil, fmt.Errorf("parsing %q: %w", filePath, err)
      }

      // Get requested workflow
      if workflowName == "" {
          return module.DefaultWorkflow()
      }

      workflow, ok := module.Workflows[workflowName]
      if !ok {
          return nil, fmt.Errorf("workflow %q not found in %q", workflowName, filePath)
      }

      // Check internal visibility
      if workflow.Internal {
          return nil, fmt.Errorf("workflow %q in %q is internal", workflowName, filePath)
      }

      return workflow, nil
  }

  func parseRef(ref string) (file, workflow string) {
      if idx := strings.Index(ref, "#"); idx != -1 {
          return ref[:idx], ref[idx+1:]
      }
      return ref, "main"
  }
  ```

  ## Acceptance Criteria

  - [ ] All reference formats work
  - [ ] Relative paths resolve correctly
  - [ ] Cycle detection works
  - [ ] Internal visibility enforced

instructions: |
  1. Add `ResolveExternalRef` to loader
  2. Add `parseRef` helper
  3. Add cycle detection using context
  4. Write comprehensive tests for all formats

needs:
  - meow-modules-loader-local
labels:
  - "phase:2"
  - "component:loader"
```

#### Task: meow-modules-loader-errors

```yaml
id: meow-modules-loader-errors
type: task
priority: P1
title: "Implement helpful error messages for loading"
parent: meow-modules-loader
description: |
  Create a good error experience for reference resolution failures.

  ## Error Types

  ```go
  // WorkflowNotFoundError is returned when a workflow doesn't exist
  type WorkflowNotFoundError struct {
      Reference    string
      File         string
      Workflow     string
      Available    []string
      IsLocal      bool
  }

  func (e *WorkflowNotFoundError) Error() string {
      if e.IsLocal {
          return fmt.Sprintf(
              "workflow %q not found in current module\n"+
              "  Reference: %s\n"+
              "  Available workflows: %v\n"+
              "  Hint: Check spelling or add the workflow to this file",
              e.Workflow, e.Reference, e.Available,
          )
      }
      return fmt.Sprintf(
          "workflow %q not found in file %q\n"+
          "  Reference: %s\n"+
          "  Available workflows: %v",
          e.Workflow, e.File, e.Reference, e.Available,
      )
  }

  // CircularReferenceError is returned on cycle detection
  type CircularReferenceError struct {
      Stack []string
      Next  string
  }

  func (e *CircularReferenceError) Error() string {
      return fmt.Sprintf(
          "circular reference detected:\n"+
          "  Load path: %s\n"+
          "  Attempted: %s\n"+
          "  Hint: Restructure workflows to avoid A → B → A patterns",
          strings.Join(e.Stack, " → "), e.Next,
      )
  }

  // InternalWorkflowError is returned when accessing internal from outside
  type InternalWorkflowError struct {
      Workflow string
      File     string
  }

  func (e *InternalWorkflowError) Error() string {
      return fmt.Sprintf(
          "workflow %q in %q is internal and cannot be accessed externally\n"+
          "  Hint: Remove 'internal = true' or call from within the same file",
          e.Workflow, e.File,
      )
  }
  ```

  ## Acceptance Criteria

  - [ ] Error types defined
  - [ ] Errors include context and hints
  - [ ] Loader uses typed errors

instructions: |
  1. Create `internal/template/errors.go`
  2. Define error types with helpful messages
  3. Update loader to return typed errors
  4. Add tests that verify error messages

needs:
  - meow-modules-loader-external
labels:
  - "phase:2"
  - "component:loader"
```

---

### EPIC: meow-modules-baker

```yaml
id: meow-modules-baker
type: epic
priority: P0
title: "Baker: Wisp Detection and Tier Labeling"
description: |
  Extend the baker to automatically classify beads into tiers.

  ## Background

  The baker transforms template steps into beads. Currently it treats all
  beads uniformly. We need it to:

  1. Identify which agents are being started
  2. Classify tasks by agent assignment
  3. Apply tier-appropriate labels
  4. Handle `attach_wisp` for explicit wisp templates
  5. Set `HookBead` links to work beads

  ## Algorithm Overview

  1. **Phase 1**: Scan for `start` beads without `attach_wisp` → agents map
  2. **Phase 2**: For each step, classify:
     - `type = task` + `assignee in agents` → Wisp (Tier 2)
     - `type = task` + `orchestrator_task = true` → Orchestrator (Tier 3)
     - `type = task` + no assignee in agents → Work (Tier 1)
     - `type != task` → Orchestrator (Tier 3)
  3. **Phase 3**: Apply labels and set metadata

  ## Success Criteria

  - [ ] Auto-detection identifies wisps correctly
  - [ ] Labels applied consistently
  - [ ] HookBead links set from variables
  - [ ] attach_wisp handled

labels:
  - "phase:2"
  - "component:baker"
needs:
  - meow-modules-types
  - meow-modules-parser
```

#### Task: meow-modules-baker-agents

```yaml
id: meow-modules-baker-agents
type: task
priority: P0
title: "Track agents from start beads"
parent: meow-modules-baker
description: |
  First phase of wisp detection: identify which agents are being started.

  ## Implementation

  ```go
  // AgentInfo tracks an agent being started in the workflow
  type AgentInfo struct {
      ID         string    // Agent ID from start spec
      StartStep  *Step     // The start step
      HasWisp    bool      // True if attach_wisp is set
      WispBeads  []string  // Bead IDs that are this agent's wisp (if auto-detected)
  }

  // identifyAgents scans workflow for start beads
  func (b *Baker) identifyAgents(workflow *Workflow) map[string]*AgentInfo {
      agents := make(map[string]*AgentInfo)

      for i := range workflow.Steps {
          step := &workflow.Steps[i]
          if step.Type != "start" {
              continue
          }

          agent := step.StartSpec.Agent
          if agent == "" {
              continue
          }

          agents[agent] = &AgentInfo{
              ID:        agent,
              StartStep: step,
              HasWisp:   step.AttachWisp != nil,
          }
      }

      return agents
  }
  ```

  ## Handling attach_wisp

  If `attach_wisp` is set, that agent's wisp comes from an explicit template,
  not auto-detection. Inline tasks for that agent become orchestrator beads.

  ## Acceptance Criteria

  - [ ] `identifyAgents` correctly finds all start beads
  - [ ] Agents without attach_wisp marked for auto-detection
  - [ ] AgentInfo populated correctly

instructions: |
  1. Add `AgentInfo` struct to `internal/template/baker.go`
  2. Implement `identifyAgents` method
  3. Add tests with various workflow configurations

needs: []
labels:
  - "phase:2"
  - "component:baker"
```

#### Task: meow-modules-baker-partition

```yaml
id: meow-modules-baker-partition
type: task
priority: P0
title: "Implement tier partitioning logic"
parent: meow-modules-baker
description: |
  Second phase: classify each step into a tier.

  ## Classification Logic

  ```go
  type BeadTier int

  const (
      TierWork         BeadTier = 1  // Persistent deliverables
      TierWisp         BeadTier = 2  // Agent workflow steps
      TierOrchestrator BeadTier = 3  // Infrastructure machinery
  )

  func (b *Baker) classifyStep(step *Step, agents map[string]*AgentInfo) BeadTier {
      // Non-task primitives are always orchestrator
      if step.Type != "task" {
          return TierOrchestrator
      }

      // Explicit orchestrator task
      if step.OrchestratorTask {
          return TierOrchestrator
      }

      // Check if assigned to an agent we're starting
      if step.Assignee != "" {
          agent, hasAgent := agents[step.Assignee]
          if hasAgent {
              if agent.HasWisp {
                  // Agent has explicit wisp → this task is orchestrator
                  return TierOrchestrator
              }
              // Agent uses auto-detection → this task is wisp
              return TierWisp
          }
      }

      // No matching agent → work bead
      return TierWork
  }
  ```

  ## Edge Cases

  1. Task with assignee but no matching start → Work bead (external agent)
  2. Task with no assignee → Error if workflow has agents
  3. Task with orchestrator_task = true → Orchestrator even if has assignee

  ## Acceptance Criteria

  - [ ] Classification logic implemented
  - [ ] All edge cases handled
  - [ ] Tests cover each classification path

instructions: |
  1. Add `classifyStep` method to baker
  2. Handle all cases per the logic above
  3. Add validation for unassigned tasks in workflows with agents
  4. Write comprehensive tests

needs:
  - meow-modules-baker-agents
labels:
  - "phase:2"
  - "component:baker"
```

#### Task: meow-modules-baker-labels

```yaml
id: meow-modules-baker-labels
type: task
priority: P0
title: "Apply tier labels and metadata"
parent: meow-modules-baker
description: |
  Third phase: apply appropriate labels based on tier.

  ## Label Application

  ```go
  func (b *Baker) applyTierMetadata(bead *types.Bead, tier BeadTier, workflowID string) {
      // All ephemeral tiers
      if tier != TierWork {
          bead.Ephemeral = true
          bead.AddLabel(types.LabelWorkflow(workflowID))
      }

      // Tier-specific labels
      switch tier {
      case TierWisp:
          bead.AddLabel(types.LabelWisp)
          bead.AddLabel(types.LabelEphemeral)

      case TierOrchestrator:
          bead.AddLabel(types.LabelOrchestrator)
          bead.AddLabel(types.LabelEphemeral)
      }
  }
  ```

  ## Additional Metadata

  - `SourceFormula`: Set to workflow name for all beads from template
  - `Parent`: Set to expand bead ID for nested expansions

  ## Integration

  Update `stepToBead` to use classification:

  ```go
  func (b *Baker) stepToBead(step *Step, stepToID map[string]string, agents map[string]*AgentInfo) (*types.Bead, error) {
      bead := // ... existing logic ...

      // Classify and label
      tier := b.classifyStep(step, agents)
      b.applyTierMetadata(bead, tier, b.WorkflowID)

      // Track wisp beads for agent
      if tier == TierWisp && step.Assignee != "" {
          if agent, ok := agents[step.Assignee]; ok {
              agent.WispBeads = append(agent.WispBeads, bead.ID)
          }
      }

      return bead, nil
  }
  ```

  ## Acceptance Criteria

  - [ ] Labels applied correctly per tier
  - [ ] SourceFormula set on all beads
  - [ ] WispBeads tracked for each agent
  - [ ] Integration with stepToBead complete

instructions: |
  1. Add `applyTierMetadata` method
  2. Update `stepToBead` to classify and apply metadata
  3. Track wisp bead IDs per agent
  4. Add tests verifying labels

needs:
  - meow-modules-baker-partition
labels:
  - "phase:2"
  - "component:baker"
```

#### Task: meow-modules-baker-attach

```yaml
id: meow-modules-baker-attach
type: task
priority: P0
title: "Implement attach_wisp processing"
parent: meow-modules-baker
description: |
  Handle explicit wisp attachment via `attach_wisp` field.

  ## Attach Wisp Spec

  ```toml
  [[main.steps]]
  id = "start-worker"
  type = "start"
  agent = "claude-2"

  [main.steps.attach_wisp]
  template = "agent-implement"
  variables = { work_bead = "{{work_bead}}" }
  ```

  ## Implementation

  When `attach_wisp` is set:

  1. Mark the agent as having explicit wisp
  2. During orchestrator's `handleStart`, load and bake the wisp template
  3. Label wisp beads with agent's ID
  4. Auto-detection skipped for this agent

  ```go
  // In baker
  func (b *Baker) processAttachWisp(step *Step, agent *AgentInfo) error {
      if step.AttachWisp == nil {
          return nil
      }

      agent.HasWisp = true
      agent.WispTemplate = step.AttachWisp.Template
      agent.WispVariables = step.AttachWisp.Variables

      return nil
  }
  ```

  Note: Actual baking of the wisp template happens in the orchestrator's
  `handleStart`, not during initial baking. This allows dynamic variable
  resolution at start time.

  ## StartSpec Extension

  Add wisp info to StartSpec:

  ```go
  type StartSpec struct {
      // ... existing fields ...

      // Wisp attachment (baked at start time)
      AttachWisp *AttachWispSpec `json:"attach_wisp,omitempty"`
  }

  type AttachWispSpec struct {
      Template  string            `json:"template"`
      Variables map[string]string `json:"variables,omitempty"`
  }
  ```

  ## Acceptance Criteria

  - [ ] AttachWispSpec type defined
  - [ ] Baker extracts attach_wisp into StartSpec
  - [ ] Agent marked as having explicit wisp
  - [ ] Tests verify attach_wisp flow

instructions: |
  1. Add `AttachWispSpec` to types
  2. Add to `StartSpec`
  3. Update baker to populate it
  4. Add tests

needs:
  - meow-modules-baker-labels
labels:
  - "phase:2"
  - "component:baker"
```

#### Task: meow-modules-baker-hookbead

```yaml
id: meow-modules-baker-hookbead
type: task
priority: P0
title: "Set HookBead links from variables"
parent: meow-modules-baker
description: |
  Link wisp beads to the work bead they're implementing.

  ## The Hook Bead Pattern

  When a workflow is baked with a `work_bead` variable, wisp beads should
  have `HookBead` set to link them to that work:

  ```toml
  # Called with: variables = { work_bead = "gt-123" }

  [[steps]]
  id = "load-context"
  type = "task"
  assignee = "claude-2"
  title = "Load context for {{work_bead}}"
  ```

  The resulting bead should have `HookBead = "gt-123"`.

  ## Implementation

  Convention: Look for `work_bead` or `hook_bead` variable.

  ```go
  func (b *Baker) setHookBead(bead *types.Bead, tier BeadTier) {
      if tier != TierWisp {
          return
      }

      // Try common hook bead variable names
      for _, name := range []string{"work_bead", "hook_bead", "task_bead"} {
          if val, ok := b.VarContext.Get(name); ok {
              if id, ok := val.(string); ok && id != "" {
                  bead.HookBead = id
                  return
              }
          }
      }
  }
  ```

  ## Benefits

  1. `meow prime` can show "Work bead: gt-123 'Implement auth'"
  2. Closing wisp can auto-update work bead notes
  3. Audit trail links wisps to work

  ## Acceptance Criteria

  - [ ] HookBead set from work_bead variable
  - [ ] Works with variable substitution
  - [ ] Only set for wisp beads

instructions: |
  1. Add `setHookBead` method
  2. Call it during stepToBead for wisp beads
  3. Add tests verifying HookBead is set

needs:
  - meow-modules-baker-labels
labels:
  - "phase:2"
  - "component:baker"
```

---

### EPIC: meow-modules-orch

```yaml
id: meow-modules-orch
type: epic
priority: P0
title: "Orchestrator: Tier-Aware Processing"
description: |
  Update orchestrator to handle three-tier bead model.

  ## Changes Needed

  1. **Priority**: Process orchestrator beads before task beads
  2. **Start handling**: Process attach_wisp when starting agents
  3. **Filtering**: Support tier-based filtering in bead queries
  4. **Cleanup**: Burn wisps when workflow completes

  ## Why Priority Matters

  Consider: `start-worker` and `load-context` are both ready.

  If we process `load-context` first, the agent might not exist yet!
  Orchestrator beads (start, condition, expand, code) must run before
  task beads to set up the environment.

  ## Success Criteria

  - [ ] Orchestrator beads prioritized
  - [ ] attach_wisp baking on start
  - [ ] Wisp cleanup working

labels:
  - "phase:3"
  - "component:orchestrator"
needs:
  - meow-modules-baker
```

#### Task: meow-modules-orch-priority

```yaml
id: meow-modules-orch-priority
type: task
priority: P0
title: "Prioritize orchestrator beads in ready selection"
parent: meow-modules-orch
description: |
  Update GetNextReady to prefer orchestrator beads over task beads.

  ## Current Implementation

  The BeadStore's `GetNextReady` returns any ready bead, ordered by creation time.

  ## New Implementation

  ```go
  func (s *BeadStore) GetNextReady(ctx context.Context) (*types.Bead, error) {
      // Get all ready beads
      ready, err := s.GetReady(ctx, types.BeadFilter{
          Statuses: []types.BeadStatus{types.BeadStatusOpen},
      })
      if err != nil {
          return nil, err
      }

      // Sort by priority: orchestrator first, then by creation time
      sort.Slice(ready, func(i, j int) bool {
          tierI := ready[i].Tier()
          tierJ := ready[j].Tier()

          // Orchestrator before wisp before work
          if tierI != tierJ {
              return tierI > tierJ  // Higher tier number = lower priority
          }

          // Within same tier: earlier creation first
          return ready[i].CreatedAt.Before(ready[j].CreatedAt)
      })

      if len(ready) == 0 {
          return nil, nil
      }
      return ready[0], nil
  }
  ```

  ## Why This Matters

  - `start` must run before tasks can be assigned
  - `condition` must evaluate before branches are taken
  - `expand` must create beads before they're executed
  - `code` setup must complete before dependent tasks

  ## Acceptance Criteria

  - [ ] Orchestrator beads returned first
  - [ ] Same-tier beads ordered by creation time
  - [ ] Tests verify ordering

instructions: |
  1. Update `GetNextReady` in beadstore
  2. Use `Tier()` method for comparison
  3. Add tests with mixed tiers

needs: []
labels:
  - "phase:3"
  - "component:orchestrator"
```

#### Task: meow-modules-orch-start

```yaml
id: meow-modules-orch-start
type: task
priority: P0
title: "Handle attach_wisp in handleStart"
parent: meow-modules-orch
description: |
  When a start bead has attach_wisp, bake the wisp template at start time.

  ## Current handleStart

  ```go
  func (o *Orchestrator) handleStart(ctx context.Context, bead *types.Bead) error {
      if err := o.agents.Start(ctx, bead.StartSpec); err != nil {
          return err
      }
      return bead.Close(nil)
  }
  ```

  ## Updated handleStart

  ```go
  func (o *Orchestrator) handleStart(ctx context.Context, bead *types.Bead) error {
      spec := bead.StartSpec

      // Start the agent
      if err := o.agents.Start(ctx, spec); err != nil {
          return err
      }

      // If attach_wisp is set, bake the wisp template
      if spec.AttachWisp != nil {
          if err := o.attachWispToAgent(ctx, spec.Agent, spec.AttachWisp, bead); err != nil {
              return fmt.Errorf("attaching wisp: %w", err)
          }
      }

      return bead.Close(nil)
  }

  func (o *Orchestrator) attachWispToAgent(ctx context.Context, agentID string, wispSpec *types.AttachWispSpec, parentBead *types.Bead) error {
      // Load the wisp template
      workflow, err := o.loader.LoadWithContext(wispSpec.Template, o.loadContext)
      if err != nil {
          return err
      }

      // Create baker for wisp
      baker := template.NewBaker(o.workflowID + "." + agentID)
      baker.ParentBead = parentBead.ID
      baker.Assignee = agentID

      // Apply variables
      for k, v := range wispSpec.Variables {
          resolved, err := o.resolveVariable(v)
          if err != nil {
              return fmt.Errorf("resolving variable %q: %w", k, err)
          }
          baker.VarContext.Set(k, resolved)
      }

      // Bake with forced wisp tier
      result, err := baker.BakeAsWisp(workflow)
      if err != nil {
          return err
      }

      // Store wisp beads
      for _, bead := range result.Beads {
          bead.HookBead = baker.VarContext.GetString("work_bead")
          if err := o.store.Create(ctx, bead); err != nil {
              return err
          }
      }

      return nil
  }
  ```

  ## Acceptance Criteria

  - [ ] attach_wisp templates baked at start
  - [ ] Wisp beads assigned to agent
  - [ ] HookBead set from variables
  - [ ] Parent bead set correctly

instructions: |
  1. Update `handleStart` to check AttachWisp
  2. Implement `attachWispToAgent`
  3. Add `BakeAsWisp` to baker that forces TierWisp
  4. Add tests for attach_wisp flow

needs:
  - meow-modules-orch-priority
labels:
  - "phase:3"
  - "component:orchestrator"
```

#### Task: meow-modules-orch-filter

```yaml
id: meow-modules-orch-filter
type: task
priority: P0
title: "Add tier-based filtering to bead queries"
parent: meow-modules-orch
description: |
  Enable filtering beads by tier for different use cases.

  ## Use Cases

  1. **Agent's view** (meow prime): Only wisp beads for this agent
  2. **Work selection** (bd ready): Only work beads
  3. **Debug view** (meow status): All beads including orchestrator

  ## BeadStore Interface Extension

  ```go
  type BeadStore interface {
      // ... existing methods ...

      // GetReadyFiltered returns ready beads matching the filter
      GetReadyFiltered(ctx context.Context, filter types.BeadFilter) ([]*types.Bead, error)

      // SearchBeads returns all beads matching the filter
      SearchBeads(ctx context.Context, filter types.BeadFilter) ([]*types.Bead, error)
  }
  ```

  ## Filter Implementation

  ```go
  func (s *BeadStore) GetReadyFiltered(ctx context.Context, filter types.BeadFilter) ([]*types.Bead, error) {
      all, err := s.getAllBeads(ctx)
      if err != nil {
          return nil, err
      }

      var result []*types.Bead
      for _, bead := range all {
          if !s.isReady(ctx, bead) {
              continue
          }
          if !filter.Matches(bead) {
              continue
          }
          result = append(result, bead)
      }

      return result, nil
  }
  ```

  ## Acceptance Criteria

  - [ ] GetReadyFiltered implemented
  - [ ] SearchBeads implemented
  - [ ] Filter.Matches works correctly
  - [ ] Tier shortcuts work (filter.Tier)

instructions: |
  1. Add methods to BeadStore interface
  2. Implement filtering in beadstore.go
  3. Add tests for various filter combinations

needs:
  - meow-modules-orch-priority
labels:
  - "phase:3"
  - "component:orchestrator"
```

#### Task: meow-modules-orch-cleanup

```yaml
id: meow-modules-orch-cleanup
type: task
priority: P0
title: "Implement wisp lifecycle management (burn/squash)"
parent: meow-modules-orch
description: |
  Manage wisp lifecycle: burn when done, optionally squash to digest.

  ## Cleanup Triggers

  1. **Workflow completion**: When all beads are closed
  2. **Agent stop**: When agent is stopped, its wisps can be cleaned
  3. **Manual**: `meow burn <workflow-id>` or `meow squash <workflow-id>`

  ## Burn vs Squash

  - **Burn**: Delete wisp beads, no record
  - **Squash**: Create digest summary, attach to work bead, then delete

  ## Implementation

  ```go
  // BurnWisps deletes all wisp beads for a workflow
  func (o *Orchestrator) BurnWisps(ctx context.Context, workflowID string) error {
      filter := types.BeadFilter{
          Labels: []string{types.LabelWisp, types.LabelWorkflow(workflowID)},
      }

      beads, err := o.store.SearchBeads(ctx, filter)
      if err != nil {
          return err
      }

      for _, bead := range beads {
          if err := o.store.Delete(ctx, bead.ID); err != nil {
              o.logger.Warn("failed to delete wisp", "bead", bead.ID, "error", err)
          }
      }

      o.logger.Info("burned wisps", "workflow", workflowID, "count", len(beads))
      return nil
  }

  // SquashWisps creates a digest and attaches to work bead before burning
  func (o *Orchestrator) SquashWisps(ctx context.Context, workflowID string) error {
      filter := types.BeadFilter{
          Labels: []string{types.LabelWisp, types.LabelWorkflow(workflowID)},
      }

      beads, err := o.store.SearchBeads(ctx, filter)
      if err != nil {
          return err
      }

      // Group by HookBead
      byHook := make(map[string][]*types.Bead)
      for _, bead := range beads {
          byHook[bead.HookBead] = append(byHook[bead.HookBead], bead)
      }

      // Create digest for each work bead
      for hookID, wisps := range byHook {
          if hookID == "" {
              continue
          }

          digest := o.createDigest(wisps)
          workBead, err := o.store.Get(ctx, hookID)
          if err != nil {
              continue
          }

          // Append digest to work bead notes
          workBead.Notes += "\n\n--- Wisp Digest ---\n" + digest
          _ = o.store.Update(ctx, workBead)
      }

      // Now burn
      return o.BurnWisps(ctx, workflowID)
  }

  func (o *Orchestrator) createDigest(wisps []*types.Bead) string {
      var sb strings.Builder
      sb.WriteString(fmt.Sprintf("Completed %d steps:\n", len(wisps)))
      for _, w := range wisps {
          status := "✓"
          if w.Status != types.BeadStatusClosed {
              status = "○"
          }
          sb.WriteString(fmt.Sprintf("  %s %s\n", status, w.Title))
      }
      return sb.String()
  }
  ```

  ## Auto-Cleanup

  Update `cleanupEphemeralBeads` to use `BurnWisps`:

  ```go
  func (o *Orchestrator) cleanupEphemeralBeads(ctx context.Context) error {
      // Use the new burn/squash based on config
      if o.cfg.Cleanup.SquashWisps {
          return o.SquashWisps(ctx, o.state.WorkflowID)
      }
      return o.BurnWisps(ctx, o.state.WorkflowID)
  }
  ```

  ## Acceptance Criteria

  - [ ] BurnWisps deletes all workflow wisps
  - [ ] SquashWisps creates digest first
  - [ ] Digest attached to work bead notes
  - [ ] Auto-cleanup uses new methods

instructions: |
  1. Add `BurnWisps` and `SquashWisps` methods
  2. Add `createDigest` helper
  3. Update `cleanupEphemeralBeads` to use new methods
  4. Add config option for squash vs burn
  5. Add tests

needs:
  - meow-modules-orch-filter
labels:
  - "phase:3"
  - "component:orchestrator"
```

---

### EPIC: meow-modules-cli

```yaml
id: meow-modules-cli
type: epic
priority: P0
title: "CLI: Wisp-Aware Commands"
description: |
  Implement CLI commands that understand the three-tier model.

  ## Commands

  1. `meow prime` - Show current wisp step with progression
  2. `meow steps` - Show all wisp steps for agent
  3. `meow close` - Close wisp step with output validation
  4. `meow continue` - Resume interrupted workflow

  ## Key Insight

  Agents primarily interact through `meow prime`. This command MUST provide:
  - Clear current step
  - Progression context
  - Link to work bead
  - Required outputs

  ## Success Criteria

  - [ ] Prime shows beautiful wisp view
  - [ ] Close validates outputs
  - [ ] Continue resumes correctly

labels:
  - "phase:4"
  - "component:cli"
needs:
  - meow-modules-orch
```

#### Task: meow-modules-cli-prime

```yaml
id: meow-modules-cli-prime
type: task
priority: P0
title: "Implement wisp-aware meow prime"
parent: meow-modules-cli
description: |
  Update `meow prime` to show the wisp progression view.

  ## Current Output (stub)

  ```
  meow prime not yet implemented
  ```

  ## Target Output

  ```
  ═══════════════════════════════════════════════════════════════
  Your workflow: implement-tdd (step 2/4)
  Work bead: gt-123 "Implement auth endpoint"
  ═══════════════════════════════════════════════════════════════

    ✓ load-context
    → write-tests [in_progress] ← YOU ARE HERE
    ○ implement
    ○ commit

  ───────────────────────────────────────────────────────────────
  Current step: write-tests

  Instructions:
    Write failing tests that define the expected behavior.
    Tests MUST fail at this point (TDD principle).

  Required outputs when closing: (none)
  ───────────────────────────────────────────────────────────────

  Commands:
    meow close write-tests              # Complete this step
    meow close write-tests --notes "..."  # With handoff notes
    bd show gt-123                       # View work bead details
  ```

  ## Implementation

  ```go
  func runPrime(cmd *cobra.Command, args []string) error {
      agentID := os.Getenv("MEOW_AGENT")
      if agentID == "" {
          return fmt.Errorf("MEOW_AGENT not set")
      }

      // Get wisp steps for this agent
      filter := types.WispFilter(agentID)
      wispSteps, err := store.SearchBeads(ctx, filter)
      if err != nil {
          return err
      }

      if len(wispSteps) == 0 {
          fmt.Println("No active workflow. Use `bd ready` to find work.")
          return nil
      }

      // Find current step (first ready or in_progress)
      var current *types.Bead
      for _, step := range wispSteps {
          if step.Status == types.BeadStatusInProgress || step.Status == types.BeadStatusHooked {
              current = step
              break
          }
          if step.Status == types.BeadStatusOpen && isReady(step) {
              current = step
              break
          }
      }

      if current == nil {
          fmt.Println("Workflow complete! All steps done.")
          return nil
      }

      // Get work bead if linked
      var workBead *types.Bead
      if current.HookBead != "" {
          workBead, _ = store.Get(ctx, current.HookBead)
      }

      // Render the view
      renderPrimeOutput(wispSteps, current, workBead)

      return nil
  }
  ```

  ## Acceptance Criteria

  - [ ] Shows workflow name and step N/M
  - [ ] Shows work bead if linked
  - [ ] Shows step progression with status icons
  - [ ] Shows current step instructions
  - [ ] Shows required outputs if any
  - [ ] Shows helpful commands

instructions: |
  1. Implement the full prime command in `cmd/meow/cmd/prime.go`
  2. Add `renderPrimeOutput` for formatting
  3. Handle edge cases (no wisp, complete, no agent)
  4. Add tests with mock bead store

needs: []
labels:
  - "phase:4"
  - "component:cli"
```

#### Task: meow-modules-cli-steps

```yaml
id: meow-modules-cli-steps
type: task
priority: P1
title: "Implement meow steps command"
parent: meow-modules-cli
description: |
  Show all wisp steps for the current agent.

  ## Use Case

  Agent wants to see the full workflow, not just current step.

  ## Output

  ```
  $ meow steps

  Workflow: implement-tdd
  Work bead: gt-123 "Implement auth endpoint"

  Steps:
    1. ✓ load-context              [closed]
    2. → write-tests               [in_progress] ← CURRENT
    3. ○ implement                 [open]
    4. ○ commit                    [open]

  Progress: 1/4 complete (25%)
  ```

  ## Flags

  - `--json`: Output as JSON for scripting
  - `--verbose`: Show full step details

  ## Acceptance Criteria

  - [ ] Lists all wisp steps
  - [ ] Shows progress
  - [ ] JSON output works
  - [ ] Handles no-workflow case

instructions: |
  1. Create `cmd/meow/cmd/steps.go`
  2. Implement step listing
  3. Add JSON output option
  4. Add tests

needs:
  - meow-modules-cli-prime
labels:
  - "phase:4"
  - "component:cli"
```

#### Task: meow-modules-cli-close

```yaml
id: meow-modules-cli-close
type: task
priority: P0
title: "Implement meow close with output validation"
parent: meow-modules-cli
description: |
  Update `meow close` to handle output validation for wisp steps.

  ## Current (partial implementation)

  The current `cmd/meow/cmd/close.go` has basic structure but doesn't:
  - Validate required outputs
  - Support `--output key=value`
  - Handle wisp step progression

  ## Target Usage

  ```bash
  # Close step with outputs
  meow close select-task \
      --output work_bead=gt-123 \
      --output rationale="Highest priority"

  # Or with JSON
  meow close select-task --output-json '{"work_bead":"gt-123","rationale":"..."}'

  # With notes
  meow close write-tests --notes "Tests cover happy path and edge cases"
  ```

  ## Validation

  If step has required outputs, validate before closing:

  ```go
  func validateOutputs(bead *types.Bead, outputs map[string]string) error {
      if bead.TaskOutputs == nil {
          return nil
      }

      for _, req := range bead.TaskOutputs.Required {
          val, ok := outputs[req.Name]
          if !ok || val == "" {
              return &MissingOutputError{
                  BeadID:   bead.ID,
                  Output:   req.Name,
                  Type:     req.Type,
                  Required: bead.TaskOutputs.Required,
              }
          }

          // Type-specific validation
          if err := validateOutputType(val, req.Type); err != nil {
              return &InvalidOutputError{
                  BeadID: bead.ID,
                  Output: req.Name,
                  Value:  val,
                  Type:   req.Type,
                  Reason: err.Error(),
              }
          }
      }

      return nil
  }
  ```

  ## Error Output

  ```
  Error: Cannot close select-task - missing required outputs

  Required outputs:
    ✗ work_bead (bead_id): Not provided
    ✗ rationale (string): Not provided

  Usage:
    meow close select-task --output work_bead=<bead_id> --output rationale=<string>
  ```

  ## Acceptance Criteria

  - [ ] `--output key=value` flag works
  - [ ] `--output-json` flag works
  - [ ] Required outputs validated
  - [ ] Type validation (bead_id exists, etc.)
  - [ ] Helpful error messages
  - [ ] Updates bead status and outputs

instructions: |
  1. Update `cmd/meow/cmd/close.go`
  2. Add output flags
  3. Implement validation
  4. Add type-specific validators (use internal/validation)
  5. Add tests

needs:
  - meow-modules-cli-prime
labels:
  - "phase:4"
  - "component:cli"
```

#### Task: meow-modules-cli-continue

```yaml
id: meow-modules-cli-continue
type: task
priority: P1
title: "Implement meow continue for crash recovery"
parent: meow-modules-cli
description: |
  Implement `meow continue` to resume an interrupted workflow.

  ## Use Case

  After a crash, operator runs:
  ```bash
  meow continue work-loop
  # or
  meow continue --workflow meow-abc123
  ```

  ## Implementation

  ```go
  func runContinue(cmd *cobra.Command, args []string) error {
      workflowID := args[0]

      // Find workflow beads
      filter := types.BeadFilter{
          Labels: []string{types.LabelWorkflow(workflowID)},
      }
      beads, err := store.SearchBeads(ctx, filter)
      if err != nil {
          return err
      }

      if len(beads) == 0 {
          return fmt.Errorf("workflow %q not found", workflowID)
      }

      // Show state
      var completed, inProgress, pending int
      for _, b := range beads {
          switch b.Status {
          case types.BeadStatusClosed:
              completed++
          case types.BeadStatusInProgress, types.BeadStatusHooked:
              inProgress++
          default:
              pending++
          }
      }

      fmt.Printf("Workflow: %s\n", workflowID)
      fmt.Printf("  Completed:   %d\n", completed)
      fmt.Printf("  In progress: %d\n", inProgress)
      fmt.Printf("  Pending:     %d\n", pending)
      fmt.Println()

      // Offer to resume
      fmt.Println("Resuming orchestrator...")

      // Start orchestrator with resume
      return runOrchestrator(workflowID, true)
  }
  ```

  ## Acceptance Criteria

  - [ ] Shows workflow state
  - [ ] Resumes orchestrator loop
  - [ ] Handles non-existent workflow

instructions: |
  1. Create `cmd/meow/cmd/continue.go`
  2. Implement state reconstruction
  3. Resume orchestrator
  4. Add tests

needs:
  - meow-modules-cli-prime
labels:
  - "phase:4"
  - "component:cli"
```

---

### EPIC: meow-modules-templates

```yaml
id: meow-modules-templates
type: epic
priority: P1
title: "Templates: Module Format Examples"
description: |
  Create example templates using the new module format.

  ## Templates to Create

  1. **work-loop.toml** - Main loop: select work, implement, repeat
  2. **implement.toml** - TDD implementation workflow
  3. **call.toml** - Parent/child orchestration pattern

  ## Goals

  - Demonstrate module features
  - Serve as templates for users
  - Test the full system end-to-end

  ## Success Criteria

  - [ ] Templates work with new module system
  - [ ] Good examples of wisp vs orchestrator beads
  - [ ] Documented and commented

labels:
  - "phase:4"
  - "component:templates"
needs:
  - meow-modules-cli
```

#### Task: meow-modules-templates-workloop

```yaml
id: meow-modules-templates-workloop
type: task
priority: P1
title: "Create work-loop module template"
parent: meow-modules-templates
description: |
  Create the main work loop template using module format.

  ## Design

  ```toml
  # .meow/templates/work-loop.toml
  #
  # Main work selection and execution loop.
  # This is the outer loop that agents run continuously.
  #
  # Usage:
  #   meow run work-loop --var agent=claude-1

  # ═══════════════════════════════════════════════════════════════════════════
  # MAIN WORKFLOW
  # ═══════════════════════════════════════════════════════════════════════════

  [main]
  name = "work-loop"
  description = "Select work, implement, repeat"

  [main.variables]
  agent = { required = true, description = "Agent ID for this loop" }

  [[main.steps]]
  id = "check-work"
  type = "condition"
  condition = "bd list --status=open --type=task | grep -q ."
  [main.steps.on_true]
  template = ".select-and-implement"
  variables = { agent = "{{agent}}" }
  [main.steps.on_false]
  inline = [
      { id = "summary", type = "task", title = "Write final summary" },
      { id = "done", type = "code", code = "echo 'All work complete'" }
  ]

  # ═══════════════════════════════════════════════════════════════════════════
  # SELECT AND IMPLEMENT (internal helper)
  # ═══════════════════════════════════════════════════════════════════════════

  [select-and-implement]
  name = "select-and-implement"
  internal = true

  [select-and-implement.variables]
  agent = { required = true }

  [[select-and-implement.steps]]
  id = "select-task"
  type = "task"
  title = "Select next task to work on"
  assignee = "{{agent}}"
  instructions = """
  Run `bv --robot-triage` and select the highest-impact task.
  Consider what unblocks the most other work.
  """
  [select-and-implement.steps.outputs]
  required = [
      { name = "work_bead", type = "bead_id", description = "The bead to implement" },
      { name = "rationale", type = "string", description = "Why you chose this" }
  ]

  [[select-and-implement.steps]]
  id = "implement"
  type = "expand"
  template = ".implement"
  variables = { work_bead = "{{select-task.outputs.work_bead}}", agent = "{{agent}}" }
  needs = ["select-task"]

  [[select-and-implement.steps]]
  id = "loop"
  type = "expand"
  template = ".main"  # Recursive!
  variables = { agent = "{{agent}}" }
  needs = ["implement"]

  # ═══════════════════════════════════════════════════════════════════════════
  # IMPLEMENT (TDD workflow)
  # ═══════════════════════════════════════════════════════════════════════════

  [implement]
  name = "implement"
  description = "TDD implementation workflow"
  ephemeral = true  # These are wisps

  [implement.variables]
  work_bead = { required = true, type = "bead_id" }
  agent = { required = true }

  [[implement.steps]]
  id = "load-context"
  type = "task"
  title = "Load context for {{work_bead}}"
  assignee = "{{agent}}"
  instructions = """
  Read the work bead and understand what needs to be done:

  ```bash
  bd show {{work_bead}}
  ```

  Identify relevant source files and understand the codebase structure.
  """

  [[implement.steps]]
  id = "write-tests"
  type = "task"
  title = "Write failing tests"
  assignee = "{{agent}}"
  needs = ["load-context"]
  instructions = """
  Write tests that define the expected behavior.
  Tests MUST fail at this point - you haven't implemented yet!
  """

  [[implement.steps]]
  id = "implement"
  type = "task"
  title = "Implement to make tests pass"
  assignee = "{{agent}}"
  needs = ["write-tests"]
  instructions = """
  Write the minimum code to make tests pass. No gold-plating.
  """

  [[implement.steps]]
  id = "verify"
  type = "condition"
  condition = "go test ./..."
  needs = ["implement"]
  [implement.steps.on_true]
  inline = [
      { id = "commit", type = "task", title = "Commit changes", assignee = "{{agent}}" }
  ]
  [implement.steps.on_false]
  inline = [
      { id = "fix", type = "task", title = "Fix failing tests", assignee = "{{agent}}" },
      { id = "retry", type = "expand", template = ".implement.verify", needs = ["fix"] }
  ]

  [[implement.steps]]
  id = "close-work"
  type = "code"
  code = "bd close {{work_bead}} --notes 'Implemented via TDD'"
  needs = ["verify"]
  ```

  ## Acceptance Criteria

  - [ ] Module format with main, helpers
  - [ ] Local references work
  - [ ] Recursive loop via .main
  - [ ] Wisp beads for agent tasks
  - [ ] Work bead closed at end

instructions: |
  1. Create `.meow/templates/work-loop.toml`
  2. Test with `meow run work-loop --var agent=test`
  3. Verify beads have correct tiers
  4. Add documentation comments

needs: []
labels:
  - "phase:4"
  - "component:templates"
```

---

## Dependency Graph

```
Phase 1: Foundation
─────────────────────────────────────────────────────────────────
                    ┌─────────────────────┐
                    │ meow-modules-parser │
                    │       (Epic)        │
                    └─────────┬───────────┘
                              │
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
   ┌───────────┐        ┌───────────┐        ┌───────────┐
   │  -types   │───────▶│  -detect  │───────▶│  -parse   │
   └───────────┘        └───────────┘        └─────┬─────┘
                                                   │
                              ┌────────────────────┼────────────────────┐
                              ▼                    ▼                    ▼
                        ┌───────────┐        ┌───────────┐        ┌───────────┐
                        │ -validate │───────▶│  -legacy  │        │           │
                        └───────────┘        └───────────┘        │           │
                                                                  │           │
                    ┌─────────────────────┐                       │           │
                    │ meow-modules-types  │                       │           │
                    │       (Epic)        │                       │           │
                    └─────────┬───────────┘                       │           │
                              │                                   │           │
         ┌────────────────────┼────────────────────┐              │           │
         ▼                    ▼                    ▼              │           │
   ┌───────────┐        ┌───────────┐        ┌───────────┐        │           │
   │  -fields  │        │  -status  │        │  -labels  │───────▶│           │
   └───────────┘        └───────────┘        └─────┬─────┘        │           │
                                                   │              │           │
                                                   ▼              │           │
                                             ┌───────────┐        │           │
                                             │  -filter  │────────┘           │
                                             └───────────┘                    │
                                                                              │
Phase 2: Resolution & Baking                                                  │
─────────────────────────────────────────────────────────────────             │
                                                                              │
                    ┌─────────────────────┐                                   │
                    │ meow-modules-loader │◀──────────────────────────────────┘
                    │       (Epic)        │
                    └─────────┬───────────┘
                              │
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
   ┌───────────┐        ┌───────────┐        ┌───────────┐
   │ -context  │───────▶│  -local   │───────▶│ -external │
   └───────────┘        └───────────┘        └─────┬─────┘
                                                   │
                                                   ▼
                                             ┌───────────┐
                                             │  -errors  │
                                             └─────┬─────┘
                                                   │
                                                   ▼
                    ┌─────────────────────┐        │
                    │ meow-modules-baker  │◀───────┘
                    │       (Epic)        │
                    └─────────┬───────────┘
                              │
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
   ┌───────────┐        ┌───────────┐        ┌───────────┐
   │  -agents  │───────▶│-partition │───────▶│  -labels  │
   └───────────┘        └───────────┘        └─────┬─────┘
                                                   │
                              ┌────────────────────┼────────────────────┐
                              ▼                    ▼                    ▼
                        ┌───────────┐        ┌───────────┐              │
                        │  -attach  │        │ -hookbead │──────────────┘
                        └───────────┘        └───────────┘
                                                   │
Phase 3: Orchestration                             │
─────────────────────────────────────────────────────────────────
                                                   │
                    ┌─────────────────────┐        │
                    │  meow-modules-orch  │◀───────┘
                    │       (Epic)        │
                    └─────────┬───────────┘
                              │
         ┌────────────────────┼────────────────────┐
         ▼                    ▼                    ▼
   ┌───────────┐        ┌───────────┐        ┌───────────┐
   │ -priority │        │  -start   │        │  -filter  │
   └─────┬─────┘        └─────┬─────┘        └─────┬─────┘
         │                    │                    │
         └────────────────────┼────────────────────┘
                              ▼
                        ┌───────────┐
                        │ -cleanup  │
                        └─────┬─────┘
                              │
Phase 4: CLI & Templates      │
─────────────────────────────────────────────────────────────────
                              │
                    ┌─────────────────────┐
                    │  meow-modules-cli   │◀───────┘
                    │       (Epic)        │
                    └─────────┬───────────┘
                              │
         ┌────────────────────┼────────────────────┬────────────────────┐
         ▼                    ▼                    ▼                    ▼
   ┌───────────┐        ┌───────────┐        ┌───────────┐        ┌───────────┐
   │  -prime   │        │  -steps   │        │  -close   │        │ -continue │
   └─────┬─────┘        └───────────┘        └───────────┘        └───────────┘
         │
         ▼
   ┌─────────────────────────────────┐
   │    meow-modules-templates       │
   │           (Epic)                │
   └─────────┬───────────────────────┘
             │
    ┌────────┼────────┐
    ▼        ▼        ▼
┌────────┐┌────────┐┌────────┐
│workloop││ impl   ││  call  │
└────────┘└────────┘└────────┘
```

---

## Risk Analysis

### High Risk

1. **TOML Parsing Complexity**
   - Risk: Dynamic table names in TOML are tricky
   - Mitigation: Use `toml.Primitive` and two-phase parsing
   - Fallback: Pre-process TOML to standardize format

2. **Backward Compatibility**
   - Risk: Breaking existing templates
   - Mitigation: Format detection, legacy path
   - Fallback: Migration tool

### Medium Risk

1. **Circular References**
   - Risk: Infinite loops in template expansion
   - Mitigation: Cycle detection in loader
   - Fallback: Max expansion depth

2. **Variable Scoping**
   - Risk: Confusion about which variables are in scope
   - Mitigation: Explicit passing, no inheritance
   - Fallback: Debug logging

### Low Risk

1. **Label Conventions**
   - Risk: Inconsistent labeling
   - Mitigation: Constants, helper functions
   - Fallback: Migration/cleanup tool

2. **Performance**
   - Risk: Slow filtering with many beads
   - Mitigation: Index on labels
   - Fallback: Pagination, caching

---

## Success Criteria

### Functional

1. **Module Parsing**: Parse files with multiple `[workflow]` sections
2. **Local References**: `.workflow` references resolve correctly
3. **Wisp Detection**: Task beads automatically classified by tier
4. **Agent Visibility**: `meow prime` shows only wisp beads
5. **Workflow Progression**: Agents see step N/M progress
6. **Output Validation**: Required outputs enforced on close
7. **Crash Recovery**: `meow continue` resumes correctly

### Performance

1. **Parse Time**: Module file parses in <100ms
2. **Filter Time**: Tier filtering in <10ms for 1000 beads
3. **Memory**: No significant memory increase

### Usability

1. **Error Messages**: Include context, suggestions
2. **Documentation**: All new features documented
3. **Examples**: Templates demonstrate patterns

---

## Future Considerations

### Not In Scope (But Planned)

1. **Partial Workflow Execution**: `.workflow.step` syntax
2. **Template Inheritance**: `extends = "base-template"`
3. **Parallel Execution**: Multiple agents on same workflow
4. **Remote Templates**: `https://...` references

### Open Questions

1. Should modules support shared variables at module level?
2. Should `internal` be the default for helper workflows?
3. What's the max expansion depth before error?

---

## Appendix: Bead Summary

| ID | Title | Priority | Type | Phase |
|----|-------|----------|------|-------|
| meow-modules | Module System and Three-Tier Architecture | P0 | Epic | - |
| meow-modules-parser | Parser: Multi-Workflow Module Support | P0 | Epic | 1 |
| meow-modules-parser-types | Define module-level parser types | P0 | Task | 1 |
| meow-modules-parser-detect | Implement format detection logic | P0 | Task | 1 |
| meow-modules-parser-parse | Implement module parsing | P0 | Task | 1 |
| meow-modules-parser-validate | Implement module validation | P0 | Task | 1 |
| meow-modules-parser-legacy | Maintain legacy format compatibility | P0 | Task | 1 |
| meow-modules-types | Types: Three-Tier Bead Metadata | P0 | Epic | 1 |
| meow-modules-types-fields | Add wisp tracking fields to Bead struct | P0 | Task | 1 |
| meow-modules-types-status | Add 'hooked' status for wisp beads | P0 | Task | 1 |
| meow-modules-types-labels | Define standard label conventions | P0 | Task | 1 |
| meow-modules-types-filter | Add filter types for tier-based queries | P0 | Task | 1 |
| meow-modules-loader | Loader: Reference Resolution | P0 | Epic | 2 |
| meow-modules-loader-context | Implement load context tracking | P0 | Task | 2 |
| meow-modules-loader-local | Implement local reference resolution | P0 | Task | 2 |
| meow-modules-loader-external | Implement external reference resolution | P0 | Task | 2 |
| meow-modules-loader-errors | Implement helpful error messages | P1 | Task | 2 |
| meow-modules-baker | Baker: Wisp Detection and Tier Labeling | P0 | Epic | 2 |
| meow-modules-baker-agents | Track agents from start beads | P0 | Task | 2 |
| meow-modules-baker-partition | Implement tier partitioning logic | P0 | Task | 2 |
| meow-modules-baker-labels | Apply tier labels and metadata | P0 | Task | 2 |
| meow-modules-baker-attach | Implement attach_wisp processing | P0 | Task | 2 |
| meow-modules-baker-hookbead | Set HookBead links from variables | P0 | Task | 2 |
| meow-modules-orch | Orchestrator: Tier-Aware Processing | P0 | Epic | 3 |
| meow-modules-orch-priority | Prioritize orchestrator beads | P0 | Task | 3 |
| meow-modules-orch-start | Handle attach_wisp in handleStart | P0 | Task | 3 |
| meow-modules-orch-filter | Add tier-based filtering | P0 | Task | 3 |
| meow-modules-orch-cleanup | Implement wisp lifecycle (burn/squash) | P0 | Task | 3 |
| meow-modules-cli | CLI: Wisp-Aware Commands | P0 | Epic | 4 |
| meow-modules-cli-prime | Implement wisp-aware meow prime | P0 | Task | 4 |
| meow-modules-cli-steps | Implement meow steps command | P1 | Task | 4 |
| meow-modules-cli-close | Implement meow close with output validation | P0 | Task | 4 |
| meow-modules-cli-continue | Implement meow continue | P1 | Task | 4 |
| meow-modules-templates | Templates: Module Format Examples | P1 | Epic | 4 |
| meow-modules-templates-workloop | Create work-loop module template | P1 | Task | 4 |
| meow-modules-templates-implement | Create implement module (part of workloop) | P1 | Task | 4 |
| meow-modules-templates-call | Create call pattern template | P1 | Task | 4 |

**Total: 7 Epics, 28 Tasks**
