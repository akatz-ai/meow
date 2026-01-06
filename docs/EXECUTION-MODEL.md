# MEOW Stack Execution Model

This document provides a detailed walkthrough of how the MEOW Stack executes workflows, including complete traces and state transitions.

## Table of Contents

1. [Execution Overview](#execution-overview)
2. [The Execution Loop](#the-execution-loop)
3. [Complete Execution Trace](#complete-execution-trace)
4. [State Transitions](#state-transitions)
5. [Stack Operations](#stack-operations)
6. [Crash Recovery](#crash-recovery)
7. [Gate Handling](#gate-handling)
8. [Loop Restart Semantics](#loop-restart-semantics)

---

## Execution Overview

MEOW Stack execution follows a simple loop:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          EXECUTION LOOP                                     │
│                                                                             │
│    ┌──────────────────────────────────────────────────────────────────┐    │
│    │                                                                  │    │
│    │    ┌─────────────┐                                               │    │
│    │    │  GET READY  │ ← What's the next unblocked step?             │    │
│    │    │    STEP     │                                               │    │
│    │    └──────┬──────┘                                               │    │
│    │           │                                                      │    │
│    │           ▼                                                      │    │
│    │    ┌─────────────┐     Has template?                             │    │
│    │    │  DISPATCH   │────────────────────┐                          │    │
│    │    │    STEP     │                    │                          │    │
│    │    └──────┬──────┘                    │                          │    │
│    │           │                           ▼                          │    │
│    │           │                    ┌─────────────┐                   │    │
│    │           │                    │    PUSH     │ Descend into      │    │
│    │           │                    │   CHILD     │ child molecule    │    │
│    │           │                    │  MOLECULE   │                   │    │
│    │           │                    └──────┬──────┘                   │    │
│    │           │                           │                          │    │
│    │           │ Atomic step               │                          │    │
│    │           ▼                           │                          │    │
│    │    ┌─────────────┐                    │                          │    │
│    │    │   EXECUTE   │                    │                          │    │
│    │    │    STEP     │                    │                          │    │
│    │    └──────┬──────┘                    │                          │    │
│    │           │                           │                          │    │
│    │           ▼                           │                          │    │
│    │    ┌─────────────┐                    │                          │    │
│    │    │   CLOSE     │                    │                          │    │
│    │    │    STEP     │                    │                          │    │
│    │    └──────┬──────┘                    │                          │    │
│    │           │                           │                          │    │
│    │           ▼                           │                          │    │
│    │    ┌─────────────┐                    │                          │    │
│    │    │  MOLECULE   │ Yes ──────────────────────────┐               │    │
│    │    │ COMPLETE?   │                    │          │               │    │
│    │    └──────┬──────┘                    │          ▼               │    │
│    │           │ No                        │   ┌─────────────┐        │    │
│    │           │                           │   │    POP      │        │    │
│    │           │                           │   │   STACK     │        │    │
│    │           │                           │   └──────┬──────┘        │    │
│    │           │                           │          │               │    │
│    │           │                           │          │               │    │
│    │           └───────────────────────────┴──────────┘               │    │
│    │                           │                                      │    │
│    │                           │                                      │    │
│    └───────────────────────────┘                                      │    │
│                                                                             │
│    Special cases:                                                           │
│    • Gate step → PAUSE loop, await human                                    │
│    • Restart step → Re-instantiate molecule, continue                       │
│    • Stack empty → DONE, all work complete                                  │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## The Execution Loop

### Pseudocode

```python
def meow_loop():
    while True:
        result = execute_one_iteration()

        if result == DONE:
            print("All work complete!")
            break

        elif result == PAUSE:
            print("Paused at gate. Waiting for human approval.")
            wait_for_gate_closure()
            # Loop continues after gate closes

        elif result == CONTINUE:
            # Ralph Wiggum stop-hook feeds prompt back
            continue

def execute_one_iteration():
    # 1. Load current position from beads
    stack = get_molecule_stack()

    if stack.is_empty():
        return DONE

    current_mol = stack.top()
    ready_step = get_ready_step(current_mol)

    # 2. No ready steps = molecule complete
    if ready_step is None:
        return handle_molecule_complete(stack)

    # 3. Dispatch based on step type
    return dispatch_step(stack, current_mol, ready_step)

def dispatch_step(stack, mol, step):
    if step.template:
        # DESCENT: Expand template into child molecule
        child = bake_molecule_from_template(step.template, step.variables)
        set_child_molecule(mol, step, child)
        stack.push(child)
        return CONTINUE

    elif step.type == "blocking-gate":
        # PAUSE: Wait for human
        prepare_gate(step)
        return PAUSE

    elif step.type == "restart":
        # LOOP: Check condition and restart
        if evaluate_condition(step.condition):
            increment_iteration(mol)
            reset_molecule_steps(mol)
            return CONTINUE
        else:
            close_step(step)
            return handle_molecule_complete(stack)

    else:
        # ATOMIC: Execute directly
        execute_atomic(step)
        close_step(step)
        return CONTINUE

def handle_molecule_complete(stack):
    completed = stack.pop()
    mark_molecule_complete(completed)

    if stack.is_empty():
        return DONE

    # Close the parent step that spawned this molecule
    parent_mol = stack.top()
    parent_step = completed.parent_step
    close_step(parent_step)

    # Check if parent is now complete
    if all_steps_closed(parent_mol):
        return handle_molecule_complete(stack)  # Recursive ascent

    return CONTINUE
```

---

## Complete Execution Trace

This section provides a complete, iteration-by-iteration trace of a MEOW workflow.

### Scenario

- **Feature**: User authentication
- **Epics**: 2 (Registration, Login)
- **Tasks**: 3 (2 for Registration, 1 for Login)
- **Gate frequency**: Every 1 epic

### Initial State

```yaml
# .beads/issues.jsonl (abbreviated)
bd-epic-001:
  title: User Registration
  type: epic
  status: open

bd-task-001:
  title: Create registration endpoint
  type: task
  parent: bd-epic-001
  status: open

bd-task-002:
  title: Add email validation
  type: task
  parent: bd-epic-001
  status: open
  needs: [bd-task-001]

bd-epic-002:
  title: Login/Logout
  type: epic
  status: open
  needs: [bd-epic-001]

bd-task-003:
  title: Implement session management
  type: task
  parent: bd-epic-002
  status: open
```

### Trace

```
╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 1                                                                 ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001]                                                     ║
║  Current Molecule: outer-loop-001                                            ║
║  Ready Step: analyze-pick                                                    ║
║                                                                              ║
║  Action: analyze-pick has template → PUSH                                    ║
║  Result: analyze-pick-001 created, pushed to stack                           ║
║                                                                              ║
║  Stack after: [outer-loop-001 → analyze-pick-001]                            ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 2                                                                 ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → analyze-pick-001]                                  ║
║  Current Molecule: analyze-pick-001                                          ║
║  Ready Step: run-bv-triage                                                   ║
║                                                                              ║
║  Action: Atomic step → EXECUTE                                               ║
║          Claude runs: bv --robot-triage                                      ║
║          Result: bd-task-001 recommended (highest PageRank)                  ║
║          Step closed                                                         ║
║                                                                              ║
║  Stack after: [outer-loop-001 → analyze-pick-001]                            ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 3                                                                 ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → analyze-pick-001]                                  ║
║  Current Molecule: analyze-pick-001                                          ║
║  Ready Step: select-batch                                                    ║
║                                                                              ║
║  Action: Atomic step → EXECUTE                                               ║
║          Claude selects: [bd-task-001, bd-task-002] (complete bd-epic-001)   ║
║          Output recorded in step notes                                       ║
║          Step closed                                                         ║
║                                                                              ║
║  Stack after: [outer-loop-001 → analyze-pick-001]                            ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 4                                                                 ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → analyze-pick-001]                                  ║
║  Current Molecule: analyze-pick-001                                          ║
║  Ready Step: (none - all steps closed)                                       ║
║                                                                              ║
║  Action: Molecule complete → POP                                             ║
║          analyze-pick-001 marked complete                                    ║
║          Parent step (outer-loop-001.analyze-pick) closed                    ║
║                                                                              ║
║  Stack after: [outer-loop-001]                                               ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 5                                                                 ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001]                                                     ║
║  Current Molecule: outer-loop-001                                            ║
║  Ready Step: bake-meta                                                       ║
║                                                                              ║
║  Action: bake-meta has template → PUSH                                       ║
║  Result: bake-meta-001 created                                               ║
║                                                                              ║
║  Stack after: [outer-loop-001 → bake-meta-001]                               ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATIONS 6-8 (bake-meta molecule)                                         ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Claude creates meta-mol-001 with steps:                                     ║
║    - task-1 (template: implement, target: bd-task-001)                       ║
║    - task-2 (template: implement, target: bd-task-002)                       ║
║    - close-epic (template: close-epic, target: bd-epic-001)                  ║
║    - test-suite (template: test-suite)                                       ║
║    - human-gate (template: human-gate)                                       ║
║                                                                              ║
║  bake-meta-001 complete → POP                                                ║
║                                                                              ║
║  Stack after: [outer-loop-001]                                               ║
║  Output: molecule_id = meta-mol-001                                          ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 9                                                                 ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001]                                                     ║
║  Current Molecule: outer-loop-001                                            ║
║  Ready Step: run-inner                                                       ║
║                                                                              ║
║  Action: run-inner template = meta-mol-001 → PUSH                            ║
║                                                                              ║
║  Stack after: [outer-loop-001 → meta-mol-001]                                ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 10                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → meta-mol-001]                                      ║
║  Current Molecule: meta-mol-001                                              ║
║  Ready Step: task-1                                                          ║
║                                                                              ║
║  Action: task-1 has template (implement) → PUSH                              ║
║  Result: impl-task-1-001 created                                             ║
║                                                                              ║
║  Stack after: [outer-loop-001 → meta-mol-001 → impl-task-1-001]              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 11                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → meta-mol-001 → impl-task-1-001]                    ║
║  Current Molecule: impl-task-1-001                                           ║
║  Ready Step: load-context                                                    ║
║                                                                              ║
║  Action: Atomic step → EXECUTE                                               ║
║          Claude reads task bd-task-001 description                           ║
║          Claude identifies relevant files                                    ║
║          Step closed                                                         ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 12                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → meta-mol-001 → impl-task-1-001]                    ║
║  Ready Step: write-tests                                                     ║
║                                                                              ║
║  Action: Atomic step → EXECUTE                                               ║
║          Claude writes test_registration.py                                  ║
║          Tests define expected behavior                                      ║
║          Step closed                                                         ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 13                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Ready Step: verify-fail                                                     ║
║                                                                              ║
║  Action: Atomic step → EXECUTE                                               ║
║          Claude runs pytest                                                  ║
║          Tests fail as expected (no implementation yet)                      ║
║          Step closed                                                         ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 14                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Ready Step: implement                                                       ║
║                                                                              ║
║  Action: Atomic step → EXECUTE                                               ║
║          Claude implements registration endpoint                             ║
║          Creates src/routes/register.py                                      ║
║          Step closed                                                         ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 15                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Ready Step: verify-pass                                                     ║
║                                                                              ║
║  Action: Atomic step → EXECUTE                                               ║
║          Claude runs pytest                                                  ║
║          All tests pass                                                      ║
║          Step closed                                                         ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATIONS 16-17 (review, commit)                                           ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Claude reviews implementation                                               ║
║  Claude creates commit: "feat: add user registration endpoint"               ║
║  Steps closed                                                                ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 18                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → meta-mol-001 → impl-task-1-001]                    ║
║  Current Molecule: impl-task-1-001                                           ║
║  Ready Step: (none - all steps closed)                                       ║
║                                                                              ║
║  Action: Molecule complete → POP                                             ║
║          impl-task-1-001 marked complete                                     ║
║          bd-task-001 marked closed (implementation done)                     ║
║          Parent step (meta-mol-001.task-1) closed                            ║
║                                                                              ║
║  Stack after: [outer-loop-001 → meta-mol-001]                                ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 19                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → meta-mol-001]                                      ║
║  Current Molecule: meta-mol-001                                              ║
║  Ready Step: task-2                                                          ║
║                                                                              ║
║  Action: task-2 has template (implement) → PUSH                              ║
║  Result: impl-task-2-001 created                                             ║
║                                                                              ║
║  Stack after: [outer-loop-001 → meta-mol-001 → impl-task-2-001]              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATIONS 20-27 (impl-task-2-001 - email validation)                       ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Similar to iterations 11-18:                                                ║
║    - load-context                                                            ║
║    - write-tests (email validation tests)                                    ║
║    - verify-fail                                                             ║
║    - implement (email validation logic)                                      ║
║    - verify-pass                                                             ║
║    - review                                                                  ║
║    - commit                                                                  ║
║                                                                              ║
║  impl-task-2-001 complete → POP                                              ║
║  bd-task-002 marked closed                                                   ║
║                                                                              ║
║  Stack after: [outer-loop-001 → meta-mol-001]                                ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 28                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → meta-mol-001]                                      ║
║  Current Molecule: meta-mol-001                                              ║
║  Ready Step: close-epic                                                      ║
║                                                                              ║
║  Action: close-epic has template → PUSH                                      ║
║  Result: close-epic-001 created                                              ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATIONS 29-31 (close-epic-001)                                           ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  - verify-deps: Confirm all epic tasks closed                                ║
║  - update-status: Mark bd-epic-001 as closed                                 ║
║  - notify: Log epic completion                                               ║
║                                                                              ║
║  close-epic-001 complete → POP                                               ║
║                                                                              ║
║  Stack after: [outer-loop-001 → meta-mol-001]                                ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 32                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → meta-mol-001]                                      ║
║  Ready Step: test-suite                                                      ║
║                                                                              ║
║  Action: test-suite has template → PUSH                                      ║
║  Result: test-suite-001 created                                              ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATIONS 33-38 (test-suite-001)                                           ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  - setup: Prepare test environment                                           ║
║  - unit-tests: Run all unit tests                                            ║
║  - integration-tests: Run integration tests                                  ║
║  - e2e-tests: Run end-to-end tests                                           ║
║  - coverage: Generate coverage report                                        ║
║  - report: Summarize test results                                            ║
║                                                                              ║
║  test-suite-001 complete → POP                                               ║
║                                                                              ║
║  Stack after: [outer-loop-001 → meta-mol-001]                                ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 39                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → meta-mol-001]                                      ║
║  Ready Step: human-gate                                                      ║
║                                                                              ║
║  Action: human-gate has template → PUSH                                      ║
║  Result: human-gate-001 created                                              ║
║                                                                              ║
║  Stack after: [outer-loop-001 → meta-mol-001 → human-gate-001]               ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATIONS 40-41 (human-gate-001)                                           ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  - prepare-summary: Claude summarizes completed work                         ║
║    "Completed: User Registration epic (2 tasks)"                             ║
║    "Tests: 47 passing, 92% coverage"                                         ║
║  - notify: Sends notification to human                                       ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 42                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001 → meta-mol-001 → human-gate-001]                     ║
║  Ready Step: await-approval                                                  ║
║  Step Type: blocking-gate                                                    ║
║                                                                              ║
║  Action: PAUSE                                                               ║
║          Loop stops                                                          ║
║          Human notification sent                                             ║
║          Awaiting: meow approve                                              ║
║                                                                              ║
║  ═══════════════════════════════════════════════════════════════════════════ ║
║  ║                     LOOP PAUSED - AWAITING HUMAN                       ║  ║
║  ═══════════════════════════════════════════════════════════════════════════ ║
║                                                                              ║
║  Human reviews work...                                                       ║
║  Human runs: meow approve --notes "LGTM, proceed with login"                 ║
║  await-approval step closed                                                  ║
║                                                                              ║
║  ═══════════════════════════════════════════════════════════════════════════ ║
║  ║                         LOOP RESUMED                                   ║  ║
║  ═══════════════════════════════════════════════════════════════════════════ ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 43                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Ready Step: record-decision                                                 ║
║                                                                              ║
║  Action: Atomic step → EXECUTE                                               ║
║          Claude records: "Approved by human. Notes: LGTM, proceed"           ║
║          Step closed                                                         ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 44                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  human-gate-001 complete → POP                                               ║
║  meta-mol-001.human-gate closed                                              ║
║                                                                              ║
║  Stack: [outer-loop-001 → meta-mol-001]                                      ║
║  Ready Step: (none - all steps closed)                                       ║
║                                                                              ║
║  meta-mol-001 complete → POP                                                 ║
║  outer-loop-001.run-inner closed                                             ║
║                                                                              ║
║  Stack after: [outer-loop-001]                                               ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 45                                                                ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001]                                                     ║
║  Ready Step: restart                                                         ║
║  Step Type: restart                                                          ║
║  Condition: not all_epics_closed()                                           ║
║                                                                              ║
║  Evaluation:                                                                 ║
║    bd-epic-001: closed ✓                                                     ║
║    bd-epic-002: open ✗                                                       ║
║    Result: TRUE (not all closed)                                             ║
║                                                                              ║
║  Action: RESTART                                                             ║
║          outer-loop-001 steps reset to open                                  ║
║          Iteration counter incremented                                       ║
║                                                                              ║
║  Stack after: [outer-loop-001] (fresh iteration)                             ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  ITERATION 46+                                                               ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Outer loop continues:                                                       ║
║    - analyze-pick selects bd-epic-002 (Login)                                ║
║    - bake-meta creates meta-mol-002                                          ║
║    - run-inner executes login implementation                                 ║
║    - ... (similar flow for login epic)                                       ║
║                                                                              ║
╚══════════════════════════════════════════════════════════════════════════════╝

╔══════════════════════════════════════════════════════════════════════════════╗
║  FINAL ITERATION                                                             ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Stack: [outer-loop-001]                                                     ║
║  Ready Step: restart                                                         ║
║  Condition: not all_epics_closed()                                           ║
║                                                                              ║
║  Evaluation:                                                                 ║
║    bd-epic-001: closed ✓                                                     ║
║    bd-epic-002: closed ✓                                                     ║
║    Result: FALSE (all closed!)                                               ║
║                                                                              ║
║  Action: Close restart step, molecule complete                               ║
║          outer-loop-001 complete → POP                                       ║
║          Stack empty → DONE                                                  ║
║                                                                              ║
║  ═══════════════════════════════════════════════════════════════════════════ ║
║  ║                       ALL WORK COMPLETE                                ║  ║
║  ═══════════════════════════════════════════════════════════════════════════ ║
╚══════════════════════════════════════════════════════════════════════════════╝
```

---

## State Transitions

### Step States

```
     ┌─────────┐
     │  OPEN   │ ← Initial state
     └────┬────┘
          │ Execution starts
          ▼
     ┌─────────┐
     │IN_PROG  │ ← Currently executing
     └────┬────┘
          │
     ┌────┴────┬────────────┐
     │         │            │
     ▼         ▼            ▼
┌─────────┐ ┌─────────┐ ┌─────────┐
│ CLOSED  │ │ BLOCKED │ │ FAILED  │
└─────────┘ └────┬────┘ └────┬────┘
                 │           │
                 │ Unblocked │ Retried
                 ▼           ▼
            ┌─────────┐ ┌─────────┐
            │  OPEN   │ │  OPEN   │
            └─────────┘ └─────────┘
```

### Molecule States

```
     ┌─────────┐
     │ PENDING │ ← Template not yet baked
     └────┬────┘
          │ Template baked
          ▼
     ┌─────────┐
     │  OPEN   │ ← Ready to execute
     └────┬────┘
          │ First step starts
          ▼
     ┌─────────┐
     │IN_PROG  │ ← Steps being executed
     └────┬────┘
          │
     ┌────┴────┬────────────┐
     │         │            │
     ▼         ▼            ▼
┌─────────┐ ┌─────────┐ ┌─────────┐
│COMPLETE │ │ PAUSED  │ │ FAILED  │
└─────────┘ └────┬────┘ └─────────┘
                 │
                 │ Gate closed
                 ▼
            ┌─────────┐
            │IN_PROG  │
            └─────────┘
```

---

## Stack Operations

### PUSH (Descend)

Triggered when a step has a template:

```
Before:
  Stack: [A → B]
  B.current_step = "task-1"
  B.task-1.template = "implement"

After PUSH:
  Stack: [A → B → C]
  C = baked from "implement" template
  C.parent_molecule = B
  C.parent_step = "task-1"
  B.task-1.child_molecule = C
```

### POP (Ascend)

Triggered when molecule has no more ready steps:

```
Before:
  Stack: [A → B → C]
  C: all steps closed

After POP:
  Stack: [A → B]
  C.status = complete
  B.task-1.status = closed
  B.task-1.child_molecule = C (preserved for audit)
```

### RESTART (Loop)

Triggered by restart-type step with true condition:

```
Before:
  Stack: [A]
  A.restart.condition = "has_more_work()"
  A.iteration = 3

After RESTART:
  Stack: [A]
  A.iteration = 4
  A.analyze-pick.status = open
  A.bake-meta.status = open
  A.run-inner.status = open
  A.restart.status = open
```

---

## Crash Recovery

### Scenario: Crash Mid-Step

```
State at crash:
  Stack: [outer-loop-001 → meta-mol-001 → impl-task-1-001]
  impl-task-1-001.implement: in_progress
  Files modified but not committed
```

### Recovery Process

```bash
# 1. New session starts, loads molecule stack
$ bd mol stack
outer-loop-001 → meta-mol-001 → impl-task-1-001

# 2. Find current position
$ bd mol current
impl-task-1-001 (step: implement, status: in_progress)

# 3. Load step context
$ bd show impl-task-1-001.implement
Title: Implement code to make tests pass
Status: in_progress
Notes: "Started implementing UserService..."

# 4. Check file state
$ git status
modified: src/services/user_service.py (uncommitted)

# 5. Resume execution
# Claude sees:
#   - The step is in_progress
#   - Partial implementation exists
#   - Notes from before crash
# Claude continues from where it left off
```

### Key Durability Properties

1. **Stack reconstructible from beads** — Parent/child links are persisted
2. **Step notes preserve context** — What was done, what's next
3. **Git tracks file changes** — Even uncommitted work visible via git status
4. **Molecules are immutable once baked** — Structure doesn't change mid-execution

---

## Gate Handling

### Gate Lifecycle

```
┌───────────────────────────────────────────────────────────────────────────┐
│                          GATE LIFECYCLE                                   │
├───────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  1. Gate step becomes ready                                               │
│     └─▶ Executor encounters blocking-gate type                            │
│                                                                           │
│  2. Prepare summary                                                       │
│     └─▶ Previous step in gate molecule creates summary                    │
│                                                                           │
│  3. Notify human                                                          │
│     └─▶ Send via configured channels (Slack, email, desktop)              │
│                                                                           │
│  4. PAUSE execution                                                       │
│     └─▶ Ralph Wiggum stop-hook returns {block: false}                     │
│     └─▶ Loop stops, awaiting external signal                              │
│                                                                           │
│  5. Human reviews                                                         │
│     └─▶ Reads summary, checks work, runs tests                            │
│                                                                           │
│  6. Human approves/rejects                                                │
│     └─▶ meow approve --notes "..."                                        │
│         OR                                                                │
│     └─▶ meow reject --notes "..." --rework-steps [...]                    │
│                                                                           │
│  7. Gate step closed                                                      │
│     └─▶ Decision recorded in step notes                                   │
│                                                                           │
│  8. RESUME execution                                                      │
│     └─▶ Next iteration picks up from gate completion                      │
│                                                                           │
│  (If rejected: rework beads created, loop handles them)                   │
│                                                                           │
└───────────────────────────────────────────────────────────────────────────┘
```

### Gate Commands

```bash
# View current gate status
meow status
# → Paused at: meta-mol-001.human-gate
# → Summary: Completed User Registration (2 tasks, 92% coverage)
# → Waiting since: 2026-01-06 14:32:00 (2 hours ago)

# Approve and continue
meow approve --notes "LGTM, good work on the validation"

# Reject with rework
meow reject --notes "Need more edge case tests" \
  --rework-steps impl-task-1-001.write-tests

# Reject and abort
meow reject --abort --notes "Wrong approach, need to redesign"
```

---

## Loop Restart Semantics

### Restart Step

```toml
[[steps]]
id = "restart"
description = "Loop to next iteration"
type = "restart"
needs = ["run-inner"]
condition = "not all_epics_closed()"
```

### Restart Behavior

When executor encounters a restart step:

1. **Evaluate condition**
   ```python
   if evaluate_condition(step.condition):
       # Continue looping
   else:
       # Exit loop (molecule completes)
   ```

2. **If true: Reset molecule**
   ```python
   molecule.iteration += 1
   for step in molecule.steps:
       if step.id != "restart":
           step.status = "open"
           step.child_molecule = None
   ```

3. **If false: Complete molecule**
   ```python
   close_step(restart_step)
   handle_molecule_complete(stack)
   ```

### Condition Expressions

Conditions are evaluated against the beads state:

```python
# Built-in conditions
"not all_epics_closed()"      # Any epic still open?
"has_unblocked_work()"        # Any ready tasks?
"iteration < 10"              # Iteration limit
"time_elapsed < 3600"         # Time limit (seconds)

# Combined
"has_unblocked_work() and iteration < 100"
```

### Infinite Loop Prevention

```toml
[meta]
max_iterations = 100  # Hard limit

[[steps]]
id = "restart"
type = "restart"
condition = "has_work() and iteration < max_iterations"
```

If max iterations reached, molecule completes with warning.

---

## Summary

The MEOW execution model provides:

1. **Deterministic execution** — Same state produces same behavior
2. **Full durability** — Crash anywhere, resume exactly
3. **Recursive composition** — Molecules within molecules
4. **Clean semantics** — Push/pop/restart/pause as primitives
5. **Human-in-the-loop** — Gates as natural workflow steps
6. **Observable state** — Every transition logged and auditable
