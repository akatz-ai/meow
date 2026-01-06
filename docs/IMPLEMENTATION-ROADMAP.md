# MEOW Stack Implementation Roadmap

This document outlines the phased implementation plan for MEOW Stack, from MVP to full system.

## Overview

The implementation is divided into 5 phases:

1. **Phase 1**: Template Expansion — Basic template baking in beads
2. **Phase 2**: Molecule Stack — Stack management and navigation
3. **Phase 3**: Loop Semantics — Restart, conditions, outer loop
4. **Phase 4**: Gate Integration — Human gates and pause/resume
5. **Phase 5**: Claude Code Skill — Full integration and polish

Each phase builds on the previous, creating an incrementally usable system.

---

## Phase 1: Template Expansion

**Goal**: Add template support to beads, enabling molecules to be baked from templates.

### 1.1 Template Parser

**Location**: `beads/internal/template/`

```go
// template/parser.go
type Template struct {
    Meta      TemplateMeta
    Variables map[string]Variable
    Steps     []StepDef
}

type TemplateMeta struct {
    Name           string
    Version        string
    Description    string
    FitsInContext  bool
    OnError        string
    MaxRetries     int
}

type Variable struct {
    Required    bool
    Default     interface{}
    Description string
    Type        string
    Enum        []string
}

type StepDef struct {
    ID           string
    Description  string
    Needs        []string
    Template     string
    Instructions string
    Type         string
    Condition    string
    Validation   string
}

func ParseTemplate(path string) (*Template, error)
func ValidateTemplate(t *Template) error
```

### 1.2 Template Registry

**Location**: `beads/internal/template/`

```go
// template/registry.go
type Registry struct {
    searchPaths []string
    cache       map[string]*Template
}

func NewRegistry(paths ...string) *Registry
func (r *Registry) Get(name string) (*Template, error)
func (r *Registry) List() []string
func (r *Registry) Validate(name string) error
```

Search order:
1. `.beads/templates/`
2. `.beads/templates/custom/`
3. `~/.meow/templates/`
4. Built-in (embedded)

### 1.3 Template Baking

**Location**: `beads/internal/template/`

```go
// template/baker.go
type BakeOptions struct {
    Template   string
    Variables  map[string]interface{}
    ParentMol  string
    ParentStep string
}

type BakeResult struct {
    MoleculeID string
    StepIDs    []string
}

func Bake(ctx context.Context, db *sql.DB, opts BakeOptions) (*BakeResult, error)
```

Baking creates:
1. Parent molecule bead (type: molecule)
2. Child step beads (with dependencies)
3. Links to parent molecule/step if nested

### 1.4 CLI Commands

```bash
# New commands
bd template list                     # List available templates
bd template show <name>              # Show template details
bd template validate <name>          # Validate template syntax

bd mol pour <template> [--var key=val]...  # Bake template into molecule
bd mol pour <template> --dry-run           # Show what would be created
```

### 1.5 Deliverables

- [ ] Template TOML parser with validation
- [ ] Template registry with search paths
- [ ] Template baking into molecule + steps
- [ ] `bd template *` CLI commands
- [ ] `bd mol pour` command
- [ ] Unit tests for all components
- [ ] Integration test: bake implement template

---

## Phase 2: Molecule Stack

**Goal**: Track nested molecule execution with stack semantics.

### 2.1 Stack Data Model

Add to molecule beads:

```yaml
# New fields on molecule-type beads
parent_molecule: mol-parent-001  # ID of parent molecule
parent_step: task-1              # Step in parent that spawned this
current_step: implement          # Currently executing step
status: in_progress              # open, in_progress, complete, paused
iteration: 1                     # For loop molecules
```

### 2.2 Stack Operations

**Location**: `beads/internal/molecule/`

```go
// molecule/stack.go
type Stack struct {
    db *sql.DB
}

type StackFrame struct {
    MoleculeID  string
    CurrentStep string
    ParentMol   string
    ParentStep  string
    Iteration   int
}

func (s *Stack) Load(rootMolID string) ([]StackFrame, error)
func (s *Stack) Push(parentMol, parentStep, childMol string) error
func (s *Stack) Pop(molID string) error
func (s *Stack) Current() (*StackFrame, error)
func (s *Stack) SetCurrentStep(molID, stepID string) error
```

Stack is reconstructed from bead relationships:
```sql
WITH RECURSIVE stack AS (
    SELECT id, parent_molecule, parent_step, current_step, 0 as depth
    FROM issues WHERE id = :root
    UNION ALL
    SELECT i.id, i.parent_molecule, i.parent_step, i.current_step, s.depth + 1
    FROM issues i JOIN stack s ON i.parent_molecule = s.id
)
SELECT * FROM stack ORDER BY depth;
```

### 2.3 Ready Front

**Location**: `beads/internal/molecule/`

```go
// molecule/ready.go
func GetReadySteps(db *sql.DB, molID string) ([]Issue, error)
// Returns steps where:
//   - All dependencies are closed
//   - Step itself is not closed
//   - Parent molecule is active
```

### 2.4 CLI Commands

```bash
bd mol stack              # Show current stack (all levels)
bd mol current            # Show current molecule + step
bd mol push <mol>         # Manually push (usually automatic)
bd mol pop                # Pop current molecule (mark complete)
```

### 2.5 Deliverables

- [ ] Extended molecule bead schema
- [ ] Stack load/push/pop operations
- [ ] Ready front calculation for molecules
- [ ] `bd mol stack/current/push/pop` commands
- [ ] Integration test: nested molecule navigation

---

## Phase 3: Loop Semantics

**Goal**: Support restart steps for looping workflows.

### 3.1 Step Types

```go
type StepType string

const (
    StepTypeStandard     StepType = "standard"      // Normal execution
    StepTypeBlockingGate StepType = "blocking-gate" // Pause until closed
    StepTypeRestart      StepType = "restart"       // Loop molecule
    StepTypeConditional  StepType = "conditional"   // Skip if condition false
)
```

### 3.2 Condition Evaluation

**Location**: `beads/internal/molecule/`

```go
// molecule/condition.go
type ConditionContext struct {
    Iteration      int
    MaxIterations  int
    MoleculeID     string
    Variables      map[string]interface{}
}

func EvaluateCondition(expr string, ctx *ConditionContext) (bool, error)

// Built-in functions:
// - all_epics_closed() bool
// - has_unblocked_work() bool
// - iteration < N
// - variable == "value"
```

### 3.3 Restart Execution

**Location**: `beads/internal/molecule/`

```go
// molecule/restart.go
func RestartMolecule(db *sql.DB, molID string) error
// 1. Increment iteration counter
// 2. Reset all steps to "open" (except restart step)
// 3. Clear child molecule references
// 4. Update molecule status to "in_progress"
```

### 3.4 CLI Commands

```bash
bd mol restart [mol-id]   # Restart molecule (increment iteration)
bd mol status             # Show molecule status including iteration
```

### 3.5 Deliverables

- [ ] Step type field and handling
- [ ] Condition expression evaluator
- [ ] Restart molecule operation
- [ ] Built-in condition functions
- [ ] Integration test: outer-loop restart

---

## Phase 4: Gate Integration

**Goal**: Human gates that pause execution and await approval.

### 4.1 Gate State

```yaml
# Fields on gate-type steps
gate_status: pending | approved | rejected
gate_decision_by: human@example.com
gate_decision_at: 2026-01-06T10:00:00Z
gate_notes: "LGTM, proceed"
```

### 4.2 Executor Gate Handling

**Location**: `meow/internal/executor/`

```go
// executor/gate.go
func HandleGateStep(step *Issue) ExecutionResult {
    if step.GateStatus == "pending" {
        // Prepare summary (previous step should have done this)
        // Send notification
        return PAUSE
    }
    if step.GateStatus == "approved" {
        return CONTINUE
    }
    if step.GateStatus == "rejected" {
        // Handle rejection (rework, abort, etc.)
        return handleRejection(step)
    }
}
```

### 4.3 Notification System

**Location**: `meow/internal/notify/`

```go
// notify/notify.go
type Notifier interface {
    Send(ctx context.Context, msg Notification) error
}

type Notification struct {
    Title   string
    Body    string
    Actions []Action  // approve, reject, view
}

// Implementations:
// - DesktopNotifier (notify-send, osascript)
// - SlackNotifier (webhook)
// - EmailNotifier (SMTP)
```

### 4.4 CLI Commands

```bash
meow status                  # Show current gate status
meow approve                 # Approve current gate
meow approve --notes "..."   # Approve with notes
meow reject --notes "..."    # Reject with notes
meow reject --rework step-id # Reject and reset step
```

### 4.5 Deliverables

- [ ] Gate status fields on beads
- [ ] Gate handling in executor
- [ ] Notification system (at least desktop)
- [ ] `meow approve/reject` commands
- [ ] Integration test: full gate flow

---

## Phase 5: Claude Code Skill

**Goal**: Package as Claude Code skill with full integration.

### 5.1 Skill Structure

```
~/.claude/skills/meow-stack/
├── skill.json
├── commands/
│   ├── meow-start.md
│   ├── meow-status.md
│   ├── meow-approve.md
│   └── meow-stop.md
├── hooks/
│   ├── hooks.json
│   └── meow-stop-hook.sh
├── scripts/
│   └── meow-executor.sh
└── resources/
    └── meow-instructions.md
```

### 5.2 skill.json

```json
{
  "name": "meow-stack",
  "version": "1.0.0",
  "description": "Molecular Expression Of Work - Durable workflow orchestration",
  "commands": ["meow-start", "meow-status", "meow-approve", "meow-stop"],
  "hooks": {
    "Stop": ["meow-stop-hook.sh"]
  }
}
```

### 5.3 Stop Hook Integration

```bash
#!/bin/bash
# hooks/meow-stop-hook.sh

# Check if MEOW is active
if [ ! -f ".beads/molecules/meow-state.json" ]; then
    echo '{"block": false}'
    exit 0
fi

# Run one iteration of executor
result=$(meow-executor iterate 2>/dev/null)

case "$result" in
    "CONTINUE")
        # Get prompt and feed back
        prompt=$(meow-executor get-prompt)
        echo "{\"block\": true, \"reason\": \"$prompt\"}"
        ;;
    "PAUSE")
        echo '{"block": false}'  # Stop loop for gate
        ;;
    "DONE")
        echo '{"block": false}'  # All work complete
        ;;
esac
```

### 5.4 Executor Binary

**Location**: `meow/cmd/meow-executor/`

```go
// Main executor that integrates all components
func main() {
    switch os.Args[1] {
    case "iterate":
        result := executor.RunOneIteration()
        fmt.Println(result)  // CONTINUE, PAUSE, DONE
    case "get-prompt":
        prompt := executor.GetCurrentPrompt()
        fmt.Println(prompt)
    case "status":
        status := executor.GetStatus()
        json.NewEncoder(os.Stdout).Encode(status)
    }
}
```

### 5.5 Commands

#### /meow-start

```markdown
Start a MEOW workflow loop.

Usage: /meow-start [template] [--var key=val]...

Examples:
  /meow-start                       # Start outer-loop
  /meow-start implement --var task_id=bd-001
```

#### /meow-status

```markdown
Show current MEOW status.

Displays:
- Current molecule stack
- Current step
- Gate status (if paused)
- Iteration count
```

### 5.6 Deliverables

- [ ] Skill package structure
- [ ] Stop hook integration
- [ ] meow-executor binary
- [ ] Command definitions
- [ ] Installation script
- [ ] End-to-end test: full workflow

---

## Implementation Order

### Milestone 1: Basic Templates (Phase 1)

Duration: ~1 week

- Template parser
- Template registry
- Baking into molecules
- CLI commands

**Demo**: Bake `implement` template, see molecule + steps created.

### Milestone 2: Nested Execution (Phase 2)

Duration: ~1 week

- Molecule stack
- Push/pop operations
- Ready front
- Navigation commands

**Demo**: Bake nested molecules, navigate stack.

### Milestone 3: Looping (Phase 3)

Duration: ~3-4 days

- Step types
- Condition evaluation
- Restart operation

**Demo**: outer-loop restarts until condition false.

### Milestone 4: Human Gates (Phase 4)

Duration: ~1 week

- Gate handling
- Pause/resume
- Notifications
- Approval CLI

**Demo**: Loop pauses at gate, resumes after approval.

### Milestone 5: Claude Integration (Phase 5)

Duration: ~1 week

- Skill packaging
- Stop hook
- Executor binary
- Commands

**Demo**: Full /meow-start → work → gate → approve → complete.

---

## Technical Decisions

### Language Choice

- **beads extensions**: Go (matches existing beads codebase)
- **meow executor**: Go (single binary, cross-platform)
- **Claude skill**: Bash/JS (standard skill format)

### Storage

- **Template registry**: File-based (TOML in .beads/templates/)
- **Molecule state**: SQLite (existing beads.db) + JSONL (existing issues.jsonl)
- **Executor state**: JSON file (.beads/molecules/meow-state.json)

### Dependencies

- beads CLI (required, base)
- beads_viewer (optional, for intelligent task selection)
- mcp_agent_mail (optional, for multi-agent)

---

## Success Criteria

### Phase 1 Complete When:
- [ ] `bd template list/show/validate` work
- [ ] `bd mol pour implement --var task_id=bd-001` creates molecule
- [ ] Molecule has all steps with correct dependencies

### Phase 2 Complete When:
- [ ] Nested molecules track parent/child relationships
- [ ] `bd mol stack` shows full nesting
- [ ] `bd ready` respects molecule context

### Phase 3 Complete When:
- [ ] Restart step resets molecule
- [ ] Conditions evaluate correctly
- [ ] outer-loop template can loop

### Phase 4 Complete When:
- [ ] Gate steps pause execution
- [ ] `meow approve` resumes execution
- [ ] Rejection triggers rework

### Phase 5 Complete When:
- [ ] `/meow-start` initiates loop
- [ ] Loop continues across iterations
- [ ] Gates pause and resume correctly
- [ ] Workflow completes end-to-end

---

## Future Enhancements

### Post-MVP Features

- **Parallel execution**: Multiple steps executing simultaneously
- **Template inheritance**: Extend/override templates
- **Web dashboard**: Visual molecule stack and status
- **Metrics**: Execution time, iteration counts, gate durations
- **Multi-agent**: Multiple Claude instances, work partitioning
- **Rollback**: Undo molecules to previous state

### Integration Opportunities

- **CI/CD**: Trigger MEOW workflows from CI
- **GitHub**: Create issues from PRD, sync with beads
- **Slack**: Rich gate notifications
- **Monitoring**: Prometheus metrics for observability

---

## Getting Started

To begin implementation:

1. Fork/clone the beads repository
2. Create feature branch: `git checkout -b feat/meow-templates`
3. Start with Phase 1.1 (Template Parser)
4. Write tests first (TDD!)
5. Create PR for each phase

Let's build it!
