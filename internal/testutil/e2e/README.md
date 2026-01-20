# E2E Testing Framework

This directory contains MEOW's end-to-end testing infrastructure. E2E tests verify complete user workflows by running the actual `meow` binary in isolated environments.

## Overview

The E2E framework consists of:

| Component | Purpose |
|-----------|---------|
| `specs/` | YAML specification files documenting what tests verify and why |
| `harness.go` | Test isolation (temp dirs, tmux sockets, simulator config) |
| `registry_helpers.go` | Helpers for registry lifecycle testing |
| `*_test.go` | Actual test implementations |

## Spec-Driven Testing

Tests are driven by **spec files** in `specs/`. Each spec documents:

- **What** is being tested (commands, scenarios)
- **Why** it matters (regression context)
- **Which beads** implement the feature (traceability)
- **Expected behavior** (success criteria)

### Using Specs

When a test fails:

1. Find the test's `// Spec: <scenario>.<test>` comment
2. Look up that ID in `specs/<feature>.yaml`
3. Read the `why:` field to understand original intent
4. Decide: fix the code, or update the expectation

### Spec Format

```yaml
version: "1.0"
feature: feature-name
description: |
  What this feature does and why it exists.

beads:
  component-name: meow-xxxx  # Links to implementing bead

scenarios:
  - id: scenario-name
    description: What this scenario tests
    beads: [meow-xxxx, meow-yyyy]  # Beads exercised

    tests:
      - id: test-name
        name: Human-readable test name
        command: meow <subcommand> <args>
        expect:
          exit_code: 0
          stdout_contains: ["expected", "strings"]
        why: |
          Explanation of why this test exists and what regression
          it prevents. Helps future maintainers decide whether to
          fix code or update expectations.
```

## Test Harness

The `Harness` provides isolated test environments:

```go
h := e2e.NewHarness(t)

// Isolated directories
h.TempDir      // Root temp directory
h.TemplateDir  // .meow/workflows/
h.RunsDir      // .meow/runs/
h.AdapterDir   // .meow/adapters/

// Run meow commands
stdout, stderr, err := runMeow(h, "run", "workflow-name")

// Automatic cleanup via t.Cleanup()
```

### Registry Testing

For tests that need isolated `~/.meow/` state:

```go
h := e2e.NewHarness(t)
r := h.NewRegistryTestSetup()

// Create test registry as git repo
registry := r.CreateTestRegistry("my-registry", "1.0.0")
r.AddCollection(registry, "my-collection")
r.CommitRegistry(registry, "Initial commit")

// Run with isolated HOME
stdout, stderr, err := runMeowWithEnv(h, r.Env(), "registry", "add", registry.Path)

// Check files in isolated home
e2e.FileExists(t, r.RegistriesJSONPath())
e2e.DirExists(t, r.InstalledCollectionPath("my-collection"))
```

## Running Tests

```bash
# All E2E tests (includes build step)
go test ./internal/testutil/e2e/...

# Skip E2E tests (fast feedback)
go test -short ./...

# Specific test
go test -v -run TestE2E_RegistryLifecycle ./internal/testutil/e2e/...

# With race detector
go test -race ./internal/testutil/e2e/...
```

## Writing New Tests

### 1. Start with the Spec

Before writing tests, add the scenario to the appropriate spec file:

```yaml
# specs/my-feature.yaml
scenarios:
  - id: my-scenario
    description: What I'm testing
    beads: [meow-xxxx]
    tests:
      - id: my-test
        name: Human description
        command: meow do-something
        expect:
          exit_code: 0
        why: |
          This prevents regression of <specific behavior>.
```

### 2. Implement the Test

Reference the spec in your test:

```go
// TestE2E_MyFeature tests <description>.
// Spec: my-scenario
func TestE2E_MyFeature(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping E2E test in short mode")
    }

    h := e2e.NewHarness(t)

    // Spec: my-scenario.my-test
    t.Run("my-test", func(t *testing.T) {
        stdout, stderr, err := runMeow(h, "do-something")
        if err != nil {
            t.Fatalf("failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
        }

        // Assertions...
    })
}
```

### 3. Test Categories

| File | Tests | Spec |
|------|-------|------|
| `e2e_test.go` | Core workflow execution (shell, expand, branch, etc.) | `core-orchestrator.yaml` |
| `e2e_test.go` | Crash recovery scenarios | `crash-recovery.yaml` |
| `e2e_test.go` | Agent timeout and crash handling | `agent-lifecycle.yaml` |
| `e2e_test.go` | Event routing and gates | `event-system.yaml` |
| `e2e_test.go` | Output validation and retry | `output-validation.yaml` |
| `typed_vars_test.go` | Typed variable preservation | `typed-variables.yaml` |
| `registry_e2e_test.go` | Collection validation and direct execution | `registry-distribution.yaml` |
| `registry_lifecycle_test.go` | Full registry CLI lifecycle | `registry-distribution.yaml` |

## Best Practices

### Do

- Start with the spec, then write the test
- Include `// Spec: scenario.test` comments
- Use `t.Run()` for sub-tests matching spec test IDs
- Skip in short mode: `if testing.Short() { t.Skip(...) }`
- Use the harness for isolation

### Don't

- Write tests without spec entries
- Hardcode paths (use harness directories)
- Skip the `why:` field in specs
- Leave tests that take > 30s (optimize or mark as slow)

## Debugging Failed Tests

```bash
# Verbose output
go test -v -run TestE2E_RegistryLifecycle ./internal/testutil/e2e/...

# Keep temp directories (add to test)
t.Logf("TempDir: %s", h.TempDir)
// Then don't use t.TempDir() - use os.MkdirTemp() and skip cleanup

# Run single subtest
go test -v -run 'TestE2E_RegistryLifecycle/add-registry-local' ./internal/testutil/e2e/...
```

## Spec Files

| Spec | Coverage | Key Beads |
|------|----------|-----------|
| `core-orchestrator.yaml` | 7 executors (shell, spawn, kill, expand, branch, foreach, agent) | meow-402 to meow-407 |
| `crash-recovery.yaml` | Resume after crash (pending, running, completing states) | meow-202, meow-401 |
| `agent-lifecycle.yaml` | Spawn, timeout, crash detection, Ralph Wiggum | meow-403, meow-404, meow-407 |
| `event-system.yaml` | Event routing, gates, agent-stopped events | meow-507, meow-410 |
| `typed-variables.yaml` | Object/array preservation through expand/foreach | meow-uc85 epic |
| `output-validation.yaml` | Required outputs, type checking, retry | meow-e7.4, meow-icif |
| `registry-distribution.yaml` | Registry add/list/show, install, collections | meow-5zaf to meow-uwt6 |

## Files

```
internal/testutil/e2e/
├── README.md                    # This file
├── specs/
│   ├── core-orchestrator.yaml   # 7 executors spec
│   ├── crash-recovery.yaml      # Crash recovery spec
│   ├── agent-lifecycle.yaml     # Agent timeout/crash spec
│   ├── event-system.yaml        # Event routing spec
│   ├── typed-variables.yaml     # Typed variables spec
│   ├── output-validation.yaml   # Output validation spec
│   └── registry-distribution.yaml  # Registry feature spec
├── harness.go                   # Test harness
├── registry_helpers.go          # Registry test utilities
├── sim_types.go                 # Simulator types
├── workflow_run.go              # Workflow run helpers
├── e2e_test.go                  # Core workflow tests
├── typed_vars_test.go           # Typed variable tests
├── registry_e2e_test.go         # Collection execution tests
└── registry_lifecycle_test.go   # Registry CLI lifecycle tests
```
