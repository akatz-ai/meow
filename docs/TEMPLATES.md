# MEOW Stack Template Reference

Templates are the building blocks of MEOW workflows. This document provides a complete reference for creating and using templates.

## Table of Contents

1. [Template Basics](#template-basics)
2. [Template Structure](#template-structure)
3. [Step Definitions](#step-definitions)
4. [Variables and Interpolation](#variables-and-interpolation)
5. [Step Types](#step-types)
6. [Dependencies](#dependencies)
7. [Error Handling](#error-handling)
8. [Template Inheritance](#template-inheritance)
9. [Built-in Templates](#built-in-templates)
10. [Creating Custom Templates](#creating-custom-templates)

---

## Template Basics

### What is a Template?

A template is a reusable workflow pattern defined in TOML. When a step references a template, that template is "baked" into a molecule with concrete beads.

```
Template (abstract)          →    Molecule (concrete)
─────────────────────             ───────────────────
implement.toml                    impl-task-1-001
  - load-context                    - impl-task-1-001.load-context
  - write-tests                     - impl-task-1-001.write-tests
  - implement                       - impl-task-1-001.implement
  - commit                          - impl-task-1-001.commit
```

### Template Location

Templates are stored in `.beads/templates/`:

```
.beads/
├── templates/
│   ├── outer-loop.toml
│   ├── implement.toml
│   ├── test-suite.toml
│   ├── human-gate.toml
│   └── custom/
│       └── my-workflow.toml
└── issues.jsonl
```

### Template Discovery

The executor searches for templates in order:

1. `.beads/templates/<name>.toml`
2. `.beads/templates/custom/<name>.toml`
3. `~/.meow/templates/<name>.toml` (global templates)
4. Built-in templates (bundled with MEOW)

---

## Template Structure

### Complete Template Example

```toml
#───────────────────────────────────────────────────────────────────────────────
# Template Metadata
#───────────────────────────────────────────────────────────────────────────────
[meta]
name = "implement"
version = "1.0.0"
description = "TDD implementation workflow for a single task"
author = "meow-stack"

# Execution hints
fits_in_context = true          # Should complete in one Claude session
estimated_minutes = 30          # For scheduling/planning
requires_human = false          # Contains blocking gates?

# Error handling defaults
on_error = "inject-gate"        # What to do when a step fails
max_retries = 2                 # Retry count before error handling
error_gate_template = "error-triage"

# Loop settings (for loop-type templates)
type = "standard"               # "standard" | "loop"
max_iterations = 100            # Only for loop type

#───────────────────────────────────────────────────────────────────────────────
# Variables
#───────────────────────────────────────────────────────────────────────────────
[variables]
task_id = { required = true, description = "The task bead ID to implement" }
epic_id = { required = false, description = "Parent epic if applicable" }
test_framework = { required = false, default = "pytest", description = "Test framework to use" }

#───────────────────────────────────────────────────────────────────────────────
# Steps
#───────────────────────────────────────────────────────────────────────────────

[[steps]]
id = "load-context"
description = "Load relevant files and understand the task"
instructions = """
Read the task description from bead {{task_id}}.
Identify the relevant source files that need modification.
Understand the current state of the codebase.
Document your understanding in this step's notes.
"""

[[steps]]
id = "write-tests"
description = "Write failing tests that define success criteria"
needs = ["load-context"]
instructions = """
Based on the task requirements, write tests using {{test_framework}} that:
1. Define the expected behavior
2. Cover edge cases
3. Will fail until implementation is complete

Create or modify test files as needed.
"""

[[steps]]
id = "verify-fail"
description = "Run tests and verify they fail as expected"
needs = ["write-tests"]
instructions = """
Run the test suite with: {{test_framework}} -v

Verify that:
1. New tests fail (expected - no implementation yet)
2. Existing tests still pass
3. Failures are for the expected reasons
"""
validation = "test_exit_code != 0"

[[steps]]
id = "implement"
description = "Write code to make tests pass"
needs = ["verify-fail"]
instructions = """
Implement the minimum code necessary to make the tests pass.
Follow existing patterns in the codebase.
Do not over-engineer - keep it simple.
"""

[[steps]]
id = "verify-pass"
description = "Run tests and verify they pass"
needs = ["implement"]
instructions = """
Run the full test suite with: {{test_framework}} -v

Verify that:
1. All new tests pass
2. All existing tests still pass
3. No regressions introduced
"""
validation = "test_exit_code == 0"

[[steps]]
id = "review"
description = "Self-review the implementation"
needs = ["verify-pass"]
instructions = """
Review your changes for:
1. Code quality and adherence to project style
2. Edge cases and error handling
3. Performance implications
4. Security considerations
5. Documentation (if needed)

Make any necessary refinements.
"""

[[steps]]
id = "commit"
description = "Commit changes with a descriptive message"
needs = ["review"]
instructions = """
Create a commit with:
1. Clear, concise message following project conventions
2. Reference to task: {{task_id}}
3. Summary of what was implemented

Use conventional commit format if the project uses it.
"""
```

---

## Step Definitions

### Step Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique identifier within template |
| `description` | string | yes | Human-readable description |
| `needs` | array | no | Step IDs this step depends on |
| `instructions` | string | no | Detailed instructions for Claude |
| `type` | string | no | Step type (see [Step Types](#step-types)) |
| `template` | string | no | Template to expand for this step |
| `validation` | string | no | Expression to validate step completion |
| `on_error` | string | no | Override error handling for this step |
| `timeout_minutes` | int | no | Max execution time |
| `retries` | int | no | Retry count for this step |

### Step ID Conventions

```
✓ Good IDs                    ✗ Bad IDs
─────────────                 ─────────
load-context                  step1
write-tests                   s2
verify-fail                   VERIFY_FAIL
implement-feature             do_the_thing
create-pr                     x
```

- Use lowercase with hyphens
- Be descriptive but concise
- Follow verb-noun pattern when possible

---

## Variables and Interpolation

### Defining Variables

```toml
[variables]
# Required variable - must be provided when baking
task_id = { required = true, description = "The task bead ID" }

# Optional with default
test_framework = { required = false, default = "pytest" }

# Optional without default (will be empty string if not provided)
extra_context = { required = false, description = "Additional context" }

# With type hint (for validation)
max_retries = { required = false, default = 3, type = "int" }

# With allowed values
priority = { required = false, default = "medium", enum = ["low", "medium", "high"] }
```

### Using Variables

Variables are interpolated using `{{variable_name}}` syntax:

```toml
[[steps]]
id = "run-tests"
instructions = """
Run tests using {{test_framework}}.
Focus on task: {{task_id}}
{{#if extra_context}}
Additional context: {{extra_context}}
{{/if}}
"""
```

### Special Variables

These are automatically available:

| Variable | Description |
|----------|-------------|
| `{{molecule_id}}` | Current molecule's ID |
| `{{step_id}}` | Current step's ID |
| `{{iteration}}` | Current loop iteration (for loop templates) |
| `{{parent_molecule}}` | Parent molecule ID (if nested) |
| `{{parent_step}}` | Parent step ID (if nested) |
| `{{timestamp}}` | Current ISO timestamp |

### Accessing Outputs

Steps can access outputs from previous steps:

```toml
[[steps]]
id = "analyze"
output = "analysis"  # Declares this step produces output

[[steps]]
id = "use-analysis"
needs = ["analyze"]
instructions = """
Based on the analysis:
{{output.analyze.summary}}

Proceed with implementation.
"""
```

---

## Step Types

### Standard (default)

Regular executable step:

```toml
[[steps]]
id = "implement"
description = "Write the code"
# type = "standard" is implicit
```

### Blocking Gate

Pauses execution until externally closed:

```toml
[[steps]]
id = "await-approval"
description = "Wait for human review"
type = "blocking-gate"
```

### Restart

For loop templates, triggers molecule restart:

```toml
[[steps]]
id = "restart"
description = "Loop back to beginning"
type = "restart"
condition = "has_more_work()"  # Only restart if true
```

### Conditional

Only executes if condition is true:

```toml
[[steps]]
id = "deploy-staging"
description = "Deploy to staging"
type = "conditional"
condition = "{{deploy_to_staging}} == true"
```

### Parallel (Future)

Execute multiple sub-steps in parallel:

```toml
[[steps]]
id = "run-all-tests"
description = "Run all test suites in parallel"
type = "parallel"
sub_steps = ["unit-tests", "integration-tests", "e2e-tests"]
```

---

## Dependencies

### Basic Dependencies

```toml
[[steps]]
id = "step-a"
description = "First step"

[[steps]]
id = "step-b"
description = "Second step"
needs = ["step-a"]  # Runs after step-a completes

[[steps]]
id = "step-c"
description = "Third step"
needs = ["step-a", "step-b"]  # Runs after both complete
```

### Dependency Graph

```
       step-a
      /      \
  step-b    step-c
      \      /
       step-d
```

```toml
[[steps]]
id = "step-a"

[[steps]]
id = "step-b"
needs = ["step-a"]

[[steps]]
id = "step-c"
needs = ["step-a"]

[[steps]]
id = "step-d"
needs = ["step-b", "step-c"]
```

### Ready Front

The executor uses the "ready front" model:
- A step is "ready" when all its dependencies are closed
- Multiple steps can be ready simultaneously
- Executor picks one ready step per iteration

---

## Error Handling

### Template-Level Defaults

```toml
[meta]
on_error = "inject-gate"     # Default for all steps
max_retries = 2              # Retry before error handling
error_gate_template = "error-triage"
```

### Step-Level Overrides

```toml
[[steps]]
id = "risky-operation"
on_error = "retry"
retries = 5
retry_delay_seconds = 30

[[steps]]
id = "optional-step"
on_error = "skip"  # Continue even if this fails

[[steps]]
id = "critical-step"
on_error = "abort"  # Stop everything if this fails
```

### Error Handling Strategies

| Strategy | Behavior |
|----------|----------|
| `inject-gate` | Create error-triage gate, let human/AI decide |
| `retry` | Retry step up to N times with delay |
| `skip` | Mark step as skipped, continue workflow |
| `abort` | Stop execution, require intervention |

### Error Triage Template

```toml
# .beads/templates/error-triage.toml
[meta]
name = "error-triage"
description = "Handle step failures"

[variables]
failed_step = { required = true }
error_message = { required = true }
molecule_id = { required = true }

[[steps]]
id = "analyze"
description = "Analyze the failure"
instructions = """
The step {{failed_step}} in {{molecule_id}} failed with:
{{error_message}}

Analyze what went wrong and why.
"""

[[steps]]
id = "propose"
description = "Propose solutions"
needs = ["analyze"]
instructions = """
Based on the analysis, propose possible solutions:
1. Retry with modifications
2. Skip this step
3. Rework previous steps
4. Abort the workflow
"""

[[steps]]
id = "decide"
description = "Wait for decision"
type = "blocking-gate"
needs = ["propose"]
```

---

## Template Inheritance

### Extending Templates

```toml
[meta]
name = "implement-with-docs"
extends = "implement"
description = "TDD workflow with documentation step"

# Add new step after commit
[[steps]]
id = "update-docs"
description = "Update documentation"
needs = ["commit"]  # Runs after inherited commit step
instructions = """
Update any relevant documentation:
1. README if API changed
2. Inline comments for complex logic
3. API docs if endpoints changed
"""
```

### Overriding Steps

```toml
[meta]
name = "implement-fast"
extends = "implement"

# Override the review step to be faster
[[steps]]
id = "review"
description = "Quick review"
needs = ["verify-pass"]
instructions = """
Quick sanity check:
1. No obvious bugs
2. Tests pass
3. Code runs

Skip detailed review for speed.
"""
```

### Removing Steps

```toml
[meta]
name = "implement-no-commit"
extends = "implement"

# Remove the commit step
[[steps]]
id = "commit"
remove = true  # This step won't be included
```

---

## Built-in Templates

### outer-loop

The master orchestration template:

```toml
[meta]
name = "outer-loop"
type = "loop"
max_iterations = 100

[[steps]]
id = "analyze-pick"
template = "analyze-pick"

[[steps]]
id = "bake-meta"
template = "bake-meta"
needs = ["analyze-pick"]

[[steps]]
id = "run-inner"
template = "{{output.bake_meta.molecule_id}}"
needs = ["bake-meta"]

[[steps]]
id = "restart"
type = "restart"
condition = "not all_epics_closed()"
needs = ["run-inner"]
```

### analyze-pick

Task selection using bv --robot-triage:

```toml
[meta]
name = "analyze-pick"

[[steps]]
id = "run-triage"
instructions = "Run bv --robot-triage and analyze results"

[[steps]]
id = "select-batch"
needs = ["run-triage"]
output = "selected"
```

### implement

TDD workflow (shown in full above).

### test-suite

Comprehensive testing:

```toml
[meta]
name = "test-suite"

[[steps]]
id = "setup"
description = "Prepare test environment"

[[steps]]
id = "unit"
needs = ["setup"]

[[steps]]
id = "integration"
needs = ["unit"]

[[steps]]
id = "e2e"
needs = ["integration"]

[[steps]]
id = "coverage"
needs = ["e2e"]

[[steps]]
id = "report"
needs = ["coverage"]
```

### human-gate

Human review checkpoint:

```toml
[meta]
name = "human-gate"
requires_human = true

[[steps]]
id = "prepare-summary"

[[steps]]
id = "notify"
needs = ["prepare-summary"]

[[steps]]
id = "await-approval"
type = "blocking-gate"
needs = ["notify"]

[[steps]]
id = "record-decision"
needs = ["await-approval"]
```

### close-epic

Epic completion ceremony:

```toml
[meta]
name = "close-epic"

[variables]
epic_id = { required = true }

[[steps]]
id = "verify-deps"
description = "Verify all child tasks are closed"

[[steps]]
id = "update-status"
needs = ["verify-deps"]
instructions = "Mark {{epic_id}} as closed"

[[steps]]
id = "notify"
needs = ["update-status"]
```

---

## Creating Custom Templates

### When to Create a Template

Create a template when:
- You have a repeatable workflow pattern
- Multiple tasks follow the same structure
- You want to enforce workflow discipline
- You need consistent error handling

### Template Design Checklist

- [ ] Clear, descriptive name
- [ ] Comprehensive metadata (description, author, version)
- [ ] Well-defined variables with descriptions
- [ ] Logical step ordering with explicit dependencies
- [ ] Detailed instructions for each step
- [ ] Appropriate error handling
- [ ] Validation expressions where applicable
- [ ] Fits-in-context hint if applicable

### Example: Deploy Template

```toml
[meta]
name = "deploy"
version = "1.0.0"
description = "Deploy to target environment"
author = "your-team"
fits_in_context = true
on_error = "inject-gate"

[variables]
environment = { required = true, enum = ["staging", "production"] }
version = { required = true, description = "Version to deploy" }
rollback_on_fail = { required = false, default = true }

[[steps]]
id = "pre-checks"
description = "Run pre-deployment checks"
instructions = """
Before deploying {{version}} to {{environment}}:
1. Verify all tests pass
2. Check deployment dependencies
3. Validate configuration
"""
validation = "pre_checks_passed == true"

[[steps]]
id = "backup"
description = "Backup current state"
needs = ["pre-checks"]
instructions = """
Create backup of current {{environment}} state:
1. Database snapshot
2. Current config
3. Current version marker
"""

[[steps]]
id = "deploy"
description = "Execute deployment"
needs = ["backup"]
instructions = """
Deploy version {{version}} to {{environment}}:
1. Update container images
2. Run migrations
3. Restart services
"""
on_error = "inject-gate"  # Important: don't auto-retry deploys

[[steps]]
id = "verify"
description = "Verify deployment"
needs = ["deploy"]
instructions = """
Verify {{version}} is running correctly on {{environment}}:
1. Health check endpoints
2. Smoke tests
3. Key user journeys
"""
validation = "health_check == 'ok'"

[[steps]]
id = "notify"
description = "Send deployment notification"
needs = ["verify"]
instructions = """
Notify team of successful deployment:
- Version: {{version}}
- Environment: {{environment}}
- Timestamp: {{timestamp}}
"""
```

### Testing Templates

```bash
# Validate template syntax
meow template validate .beads/templates/my-template.toml

# Dry-run baking (shows what would be created)
meow template bake my-template --dry-run --var task_id=bd-001

# Actually bake into a molecule
meow template bake my-template --var task_id=bd-001
```

---

## Template Best Practices

### 1. Keep Steps Focused

Each step should do one thing well:

```toml
# ✓ Good: Focused steps
[[steps]]
id = "write-tests"
[[steps]]
id = "verify-tests-fail"

# ✗ Bad: Too much in one step
[[steps]]
id = "write-and-verify-tests"
```

### 2. Use Clear Instructions

```toml
# ✓ Good: Clear, actionable
instructions = """
Create unit tests for the UserService class.
Test these methods:
1. create_user() - happy path and validation errors
2. get_user() - found and not found cases
3. update_user() - permissions and validation

Use pytest fixtures for database mocking.
"""

# ✗ Bad: Vague
instructions = "Write some tests"
```

### 3. Declare Dependencies Explicitly

```toml
# ✓ Good: Explicit dependencies
[[steps]]
id = "step-c"
needs = ["step-a", "step-b"]

# ✗ Bad: Implicit ordering
# (Relying on order in file)
```

### 4. Use Variables for Flexibility

```toml
# ✓ Good: Parameterized
[variables]
framework = { default = "pytest" }

[[steps]]
id = "run-tests"
instructions = "Run {{framework}} -v"

# ✗ Bad: Hardcoded
[[steps]]
id = "run-tests"
instructions = "Run pytest -v"
```

### 5. Set Appropriate Error Handling

```toml
# ✓ Good: Thoughtful error handling
[[steps]]
id = "deploy-production"
on_error = "inject-gate"  # Never auto-retry prod deploys

[[steps]]
id = "run-linter"
on_error = "skip"  # Linting is nice-to-have

# ✗ Bad: No error consideration
[[steps]]
id = "dangerous-operation"
# Uses template default, might be wrong
```
