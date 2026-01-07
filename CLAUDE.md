# MEOW Stack - Claude Instructions

This document provides instructions for Claude Code when working with the MEOW Stack system.

## Required Reading

Before working on any task, read the architecture document to understand how your piece fits the whole:

```
@ARCHITECTURE.md
```

This document explains the 6 primitives, component relationships, data flow, and key interfaces.

## Overview

MEOW Stack (Molecular Expression Of Work) is a recursive, composable workflow system for durable AI agent orchestration. You are executing within this system.

## Core Concepts

### You Are in a Molecule

When executing MEOW workflows, you are always within a **molecule** — a structured workflow with steps. Each step you complete advances the workflow.

### The Molecule Stack

There is a stack of active molecules. You might be:
- In the **outer-loop** (selecting work, baking molecules)
- In a **meta-molecule** (executing a batch of tasks)
- In a **step molecule** (e.g., implementing a task with TDD)

Check your position:
```bash
bd mol stack    # Show full stack
bd mol current  # Show current molecule and step
bd ready        # Show next step to execute
```

### Steps Have Templates

When you encounter a step with a `template` label, that step expands into its own molecule. The executor handles this — you just need to complete the current step.

## Execution Protocol

### At Session Start

1. **Check position**:
   ```bash
   bd mol current
   ```

2. **Read step context**:
   ```bash
   bd show <current-step-id>
   ```

3. **Check for notes from previous session**:
   Look for handoff notes in the step's notes field.

4. **Resume work**:
   Continue from where the last session stopped.

### During Execution

1. **Follow step instructions**:
   Each step has detailed instructions. Follow them carefully.

2. **Complete one step at a time**:
   Don't skip ahead. The dependency graph enforces order.

3. **Close steps when done**:
   ```bash
   bd close <step-id> --continue
   ```
   The `--continue` flag advances to the next ready step.

4. **Record context in notes**:
   If the step is complex, update notes as you work:
   ```bash
   bd update <step-id> --notes "PROGRESS: ..."
   ```

### At Session End (or Context Limit)

If you're about to hit context limits or the session is ending:

1. **Save your progress**:
   ```bash
   bd update <current-step> --notes "
   COMPLETED:
   - [what you finished]

   IN PROGRESS:
   - [what you're working on]

   NEXT:
   - [what needs to happen next]

   CONTEXT:
   - [important decisions, file locations, etc.]
   "
   ```

2. **Commit any code changes**:
   Don't leave uncommitted work. Future sessions see git history.

3. **The loop continues**:
   The Ralph Wiggum loop will restart with the same prompt. You'll see your notes.

## Special Step Types

### Atomic Steps

Most steps are atomic — execute them directly following the instructions.

### Gate Steps

When you reach a step with `type: blocking-gate`:
1. Complete any preparation steps (summary, notification)
2. The loop will PAUSE
3. Wait for human to run `meow approve` or `meow reject`
4. Continue when the gate is closed

### Restart Steps

When you reach a step with `type: restart`:
1. Check the condition (usually `not all_epics_closed()`)
2. If true: molecule resets, loop continues
3. If false: molecule completes, stack pops

## Task Selection

When in the **analyze-pick** step:

1. **Run triage**:
   ```bash
   bv --robot-triage
   ```

2. **Analyze the JSON output**:
   - `recommendations` — ranked by composite score
   - `score_breakdown` — why each is ranked
   - `unblocks_ids` — what completing this enables
   - `quick_wins` — low effort, high impact
   - `project_health` — overall status

3. **Consider context**:
   - What's already in progress?
   - Would this complete an epic?
   - Any dependencies bv doesn't know about?

4. **Select coherent batch**:
   - 1-3 related tasks
   - Same epic or area
   - Natural review boundary

## Template-Based Steps

When executing a step that expanded from a template:

### implement (TDD workflow)

1. **load-context**: Read task, identify files
2. **write-tests**: Write failing tests first
3. **verify-fail**: Ensure tests fail correctly
4. **implement**: Write minimum code to pass
5. **verify-pass**: All tests must pass
6. **review**: Self-review for quality
7. **commit**: Descriptive commit message

### test-suite

1. **setup**: Prepare test environment
2. **unit-tests**: Run unit tests
3. **integration-tests**: Run integration tests
4. **e2e-tests**: Run E2E tests (if configured)
5. **coverage**: Generate coverage report
6. **report**: Summarize results

### human-gate

1. **prepare-summary**: Summarize completed work
2. **notify**: Send notification
3. **await-approval**: STOP — wait for human
4. **record-decision**: Log human's decision

## Error Handling

### If a Step Fails

1. **Don't panic**. Errors are expected.
2. **Document the error**:
   ```bash
   bd update <step> --notes "ERROR: ..."
   ```
3. **Check error handling**:
   - `on_error = retry` → try again
   - `on_error = skip` → move on
   - `on_error = inject-gate` → human/AI triages
   - `on_error = abort` → stop execution

### If Tests Fail

1. Read the error carefully
2. Fix the issue in implementation (not the test)
3. Re-run tests
4. Don't close verify-pass until tests actually pass

### If You're Stuck

1. Update notes with your understanding of the problem
2. If human gate is available, request review
3. Don't thrash — better to pause than waste iterations

## Commands Reference

### Beads CLI (bd)

```bash
# View state
bd mol stack              # Show molecule stack
bd mol current            # Current molecule and step
bd ready                  # Next executable step
bd show <id>              # View bead details
bd list --status=open     # List open beads

# Update state
bd close <id>             # Close a step
bd close <id> --continue  # Close and advance
bd update <id> --notes "" # Update notes
bd update <id> --status in_progress

# Dependencies
bd dep tree <id>          # Show dependency tree
bd dep add <a> <b>        # A depends on B
```

### Beads Viewer (bv)

```bash
bv --robot-triage         # Full triage with scoring
bv --robot-next           # Just top recommendation
bv --robot-plan           # Parallel execution tracks
```

### MEOW CLI (meow)

```bash
meow status               # Current position and state
meow approve              # Close current gate (human)
meow reject --notes ""    # Reject with feedback (human)
```

## Best Practices

### 1. Trust the System

The molecule stack and dependency graph ensure correct ordering. Don't try to skip ahead or work out of order.

### 2. Write Good Notes

Future sessions (including yourself) depend on good notes. Be specific:
- What was done
- What's in progress
- What's next
- Key decisions made
- File locations

### 3. Commit Frequently

Each logical unit of work should be committed. The git history is context for future sessions.

### 4. Follow TDD Discipline

When in the implement template:
- Tests first, always
- Verify they fail before implementing
- Minimum viable implementation
- Don't over-engineer

### 5. Be Honest About Gates

When reaching a human gate:
- Provide accurate summary
- Flag concerns honestly
- Don't try to skip or auto-approve

### 6. Respect Context Limits

If you're approaching context limits:
- Save progress in notes
- Commit changes
- Let the loop restart cleanly

## Integration with Other Systems

### mcp_agent_mail

For multi-agent scenarios, coordinate via agent mail:
```bash
am send --to "<agent>" --subject "..." --body "..."
am inbox
am lease --path "src/auth/**" --duration 1h
```

### beads_viewer

For task prioritization:
```bash
bv --robot-triage
bv --robot-next
bv --robot-insights
```

## Troubleshooting

### "No ready steps"

The molecule is complete. Check if you should:
- Pop to parent molecule (`bd mol stack`)
- Or if outer-loop should restart

### "Cycle detected"

Dependency cycle in beads. Run:
```bash
bv --robot-insights  # Will show cycle details
```

### "Template not found"

Check template exists:
```bash
ls .beads/templates/
```

### "Step already closed"

You're trying to close a step that's already done. Check:
```bash
bd show <step-id>
bd mol current
```

---

*Remember: You are one session in a durable workflow. Do your step well, leave good notes, and trust the system to continue.*
