# E2E Testing Infrastructure Design

## Executive Summary

This document specifies the design for a comprehensive E2E testing infrastructure for MEOW, centered around a **Claude Simulator** that behaves identically to Claude Code from the orchestrator's perspective. This enables deterministic, fast, token-free testing of complex multi-agent workflows.

**Problem Statement:** MEOW orchestrates AI agents through tmux sessions, IPC sockets, and hook-based event systems. Current unit tests mock these interactions but cannot verify the full integration works correctly. Testing with real Claude Code burns tokens, is slow, and introduces non-determinism.

**Solution:** A simulator binary that implements Claude Code's behavioral contract, configurable to simulate various scenarios (happy path, questions, errors, tool events, crashes).

---

## Table of Contents

1. [Background & Motivation](#background--motivation)
2. [Current Testing Gaps](#current-testing-gaps)
3. [Simulator Architecture](#simulator-architecture)
4. [Behavioral Contract](#behavioral-contract)
5. [Configuration System](#configuration-system)
6. [Adapter Integration](#adapter-integration)
7. [Test Framework Design](#test-framework-design)
8. [Test Scenarios](#test-scenarios)
9. [Non-Flakiness Strategies](#non-flakiness-strategies)
10. [Implementation Phases](#implementation-phases)

---

## Background & Motivation

### The MEOW Agent Interaction Model

MEOW orchestrates agents through a "propulsion" model:

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    ORCHESTRATOR ←→ AGENT INTERACTION                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Orchestrator                                    Agent (Claude Code)        │
│  ────────────                                    ──────────────────        │
│                                                                             │
│  1. spawn step → tmux new-session + claude --dangerously-skip-permissions  │
│                                                                             │
│  2. agent step → inject prompt via tmux send-keys -l                       │
│                      │                                                      │
│                      └──────────────────────────► Agent receives prompt    │
│                                                   Agent works...           │
│                                                   Agent calls meow done    │
│                      ◄──────────────────────────┘      │                   │
│                                                        │                   │
│  3. IPC server receives step_done message              │                   │
│     - Validates outputs                                │                   │
│     - Marks step done                                  │                   │
│     - Finds next step for this agent                   │                   │
│     - Sends ESC to agent                               │                   │
│     - Injects next prompt                              │                   │
│                                                                             │
│  ALTERNATIVE: Agent stops unexpectedly (asks question, error, etc.)        │
│                                                                             │
│  4. Agent reaches prompt → Stop hook fires                                 │
│     - Runs: meow event agent-stopped                                       │
│     - Templates use await-event to detect and respond to agent stops       │
│     - Orchestrator may inject nudge prompt via tmux send-keys              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Why E2E Testing Matters

The orchestrator's correctness depends on:

1. **Timing**: `waitForClaudeReady()` must detect Claude's prompt indicator
2. **IPC Protocol**: Single-line JSON over Unix sockets must parse correctly
3. **Hook Behavior**: Stop hook must emit `agent-stopped` event
4. **Event Routing**: Events must match waiters with correct filtering
5. **State Transitions**: Steps must move through pending→running→completing→done
6. **Crash Recovery**: Interrupted workflows must resume correctly

Unit tests mock these interactions but cannot verify the **integration** works. For example:
- Does `send-keys -l` correctly handle multiline prompts with unicode?
- Does the stop hook fire at the right time?
- Does the event system correctly route `agent-stopped` events?

### Token Cost Problem

Testing with real Claude Code:
- Burns ~$0.01-0.10 per workflow run
- Takes 30s-5min per test
- Introduces LLM non-determinism
- Requires internet connectivity
- Rate-limited by API

With a simulator:
- Zero token cost
- Tests run in seconds
- Fully deterministic
- Works offline
- Parallelizable

---

## Current Testing Gaps

### What's Well Tested (Unit Level)

| Component | Coverage | Notes |
|-----------|----------|-------|
| Type validation | ✅ High | Step, Workflow, Agent, Adapter types |
| Orchestrator dispatch | ✅ High | All 6 executors have dedicated test files |
| Template parsing | ✅ High | TOML parsing, validation, variable expansion |
| IPC messages | ✅ High | Serialization, parsing |
| Dependency resolution | ✅ High | GetReadySteps, wildcard matching |

### What's Not Tested (Integration/E2E Level)

| Scenario | Gap | Risk |
|----------|-----|------|
| Stop hook → agent-stopped event | No test | Agent might get stuck |
| Event emission → await-event | No test | Branch conditions might fail |
| Prompt injection reliability | No test | Prompts might not arrive |
| Parallel agent synchronization | No test | Race conditions |
| Crash recovery with live agents | No test | State corruption |
| Output validation rejection + retry | No test | Agent might loop forever |
| fire_forget mode injection | No test | Commands might be lost |
| Context monitor pattern | No test | /compact might not work |

### Template Patterns Without E2E Coverage

The workflows in `.meow/workflows/` demonstrate patterns that have no E2E tests:

1. **ralph-wiggum-demo.meow.toml** - Agent persistence via event monitoring
2. **adapter-events-impl.meow.toml** - 4-agent parallel with template expansion
3. **dual-agent-task.meow.toml** - Serialized worktrees, parallel work tracks
4. **test-multi-agent-join.meow.toml** - Fork-join with 3 agents

---

## Simulator Architecture

### Design Principles

1. **Contract Fidelity**: Simulator MUST behave exactly like Claude Code from MEOW's perspective
2. **Configurability**: All behaviors scriptable via YAML config
3. **Observability**: All actions logged for test assertions
4. **Determinism**: No randomness in default mode (optional jitter for stress tests)
5. **Speed**: 10-100x faster than real Claude

### Component Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           meow-agent-sim                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                         INPUT LAYER                                  │   │
│  │                                                                      │   │
│  │  stdin ◄─────── tmux send-keys -l "prompt text" Enter              │   │
│  │                                                                      │   │
│  │  Handles:                                                            │   │
│  │  - Multiline prompts (newlines embedded)                            │   │
│  │  - Unicode characters                                                │   │
│  │  - Special characters (&, |, ;, etc.)                               │   │
│  │  - Long prompts (>4KB)                                              │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                              │                                              │
│                              ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                       STATE MACHINE                                  │   │
│  │                                                                      │   │
│  │  ┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐   │   │
│  │  │ STARTING │────►│  IDLE    │────►│ WORKING  │────►│  DONE    │   │   │
│  │  └──────────┘     └──────────┘     └──────────┘     └──────────┘   │   │
│  │       │                │                │                │          │   │
│  │       │                │                │                │          │   │
│  │       ▼                ▼                ▼                ▼          │   │
│  │  Show startup     Show prompt      Execute action   Call meow done  │   │
│  │  messages         indicator        (configurable)   with outputs    │   │
│  │  Wait startup_    "> "             Sleep work_delay Fire hooks      │   │
│  │  delay                             Emit events      Return to IDLE  │   │
│  │                                                                      │   │
│  │                   ┌──────────┐                                      │   │
│  │                   │ ASKING   │◄───── action: ask_question           │   │
│  │                   └──────────┘                                      │   │
│  │                        │                                             │   │
│  │                        ▼                                             │   │
│  │                   Print question                                     │   │
│  │                   Show prompt "> "                                  │   │
│  │                   Fire Stop hook (→ emits agent-stopped event)      │   │
│  │                   Wait for user input                               │   │
│  │                        │                                             │   │
│  │                        ▼                                             │   │
│  │                   Process response → back to WORKING or DONE        │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                              │                                              │
│                              ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      BEHAVIOR ENGINE                                 │   │
│  │                                                                      │   │
│  │  Loaded from: sim-config.yaml (or embedded defaults)                │   │
│  │                                                                      │   │
│  │  For each prompt received:                                          │   │
│  │  1. Match against behavior patterns (regex or substring)            │   │
│  │  2. If match found → execute configured action                      │   │
│  │  3. If no match → execute default action                            │   │
│  │                                                                      │   │
│  │  Actions:                                                            │   │
│  │  - complete: Call meow done with specified outputs                  │   │
│  │  - ask: Print question, wait for response                           │   │
│  │  - fail: Print error, don't call meow done (tests retry)           │   │
│  │  - hang: Do nothing (tests timeout handling)                        │   │
│  │  - crash: Exit process (tests crash recovery)                       │   │
│  │  - events: Emit specified events during work                        │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                              │                                              │
│                              ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                       HOOK EMULATION                                 │   │
│  │                                                                      │   │
│  │  Stop Hook (fires when transitioning to IDLE or ASKING):            │   │
│  │  1. Call: meow event agent-stopped                                  │   │
│  │  2. Orchestrator handles prompt injection via tmux send-keys        │   │
│  │                                                                      │   │
│  │  Tool Hooks (during WORKING state, if configured):                  │   │
│  │  1. Before action: meow event tool-starting --data tool=$TOOL       │   │
│  │  2. After action: meow event tool-completed --data tool=$TOOL       │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                              │                                              │
│                              ▼                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                       OUTPUT LAYER                                   │   │
│  │                                                                      │   │
│  │  stdout ─────────► Terminal output (for tmux capture-pane)          │   │
│  │                    - Prompt indicator "> "                          │   │
│  │                    - Work progress messages                          │   │
│  │                    - Questions                                       │   │
│  │                                                                      │   │
│  │  stderr ─────────► Debug/trace output                               │   │
│  │                    - State transitions                               │   │
│  │                    - Hook invocations                                │   │
│  │                    - IPC calls                                       │   │
│  │                                                                      │   │
│  │  IPC ────────────► meow CLI commands                                │   │
│  │                    - meow done --output k=v                         │   │
│  │                    - meow event <type> --data k=v                   │   │
│  │                                                                      │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Behavioral Contract

The simulator MUST implement these behaviors to be a valid Claude Code substitute:

### Startup Behavior

1. Accept `--dangerously-skip-permissions` flag (ignore it)
2. Accept `--resume <session_id>` flag (store for later reference)
3. Print startup messages (configurable)
4. Wait `startup_delay` (default 100ms, real Claude is 3s)
5. Show prompt indicator `> ` on stdout

### Prompt Reception

1. Read lines from stdin until Enter
2. Accumulate multiline input (tmux send-keys -l embeds newlines)
3. Trigger state transition IDLE → WORKING

### Work Execution

1. Execute configured behavior based on prompt match
2. During work, optionally emit tool events
3. After work_delay, complete the action

### Completion Signaling

1. Call `meow done --output key=value` via shell
2. Read response (ack or error)
3. If error: stay in WORKING, may retry or give up based on config
4. If ack: transition to IDLE

### Stop Hook Behavior

When transitioning to IDLE (showing prompt):

1. Run: `meow event agent-stopped` (fire and forget)
2. Wait for external input (orchestrator injects via tmux send-keys)

### Environment Variables

The simulator MUST respect:
- `MEOW_AGENT` - Agent identifier
- `MEOW_WORKFLOW` - Workflow ID
- `MEOW_ORCH_SOCK` - IPC socket path
- `MEOW_STEP` - Current step ID (updated by orchestrator)

---

## Configuration System

### Configuration File Format

```yaml
# sim-config.yaml
# Claude Simulator behavior configuration

# Timing configuration
timing:
  startup_delay: 100ms      # Time before showing first prompt
  default_work_delay: 500ms # Default time to "think" before completing
  prompt_delay: 50ms        # Delay before showing prompt after completion

# Hook configuration
hooks:
  fire_stop_hook: true      # Whether to emit agent-stopped event on idle
  fire_tool_events: true    # Whether to emit PreToolUse/PostToolUse events

# Behavior definitions (evaluated in order, first match wins)
behaviors:
  # Pattern matching with specific outputs
  - match: "Write failing tests"
    type: regex  # or "contains" (default)
    action:
      type: complete
      delay: 1s
      outputs:
        test_file: "src/__tests__/feature.test.ts"
      events:
        - type: tool-starting
          data: {tool: "Write"}
          when: before  # before, after, or during
        - type: tool-completed
          data: {tool: "Write"}
          when: after

  # Ask a question (triggers interactive mode)
  - match: "implement.*feature"
    type: regex
    action:
      type: ask
      question: "I found 3 potential approaches. Which should I use?"
      options:
        - "Use approach A (fastest)"
        - "Use approach B (most maintainable)"
        - "Use approach C (most flexible)"

  # Simulate failure (tests retry behavior)
  - match: "fail-on-first-attempt"
    action:
      type: fail_then_succeed
      fail_count: 1
      fail_message: "Error: Could not parse configuration"
      success_outputs:
        result: "fixed"

  # Simulate hang (tests timeout)
  - match: "hang-forever"
    action:
      type: hang
      # No completion, no meow done

  # Simulate crash (tests recovery)
  - match: "crash-immediately"
    action:
      type: crash
      exit_code: 137  # SIGKILL

  # Emit specific event patterns
  - match: "complex-operation"
    action:
      type: complete
      delay: 2s
      events:
        - {type: tool-starting, data: {tool: Read}, when: 0ms}
        - {type: tool-completed, data: {tool: Read}, when: 200ms}
        - {type: tool-starting, data: {tool: Write}, when: 500ms}
        - {type: tool-completed, data: {tool: Write}, when: 1500ms}
      outputs:
        files_modified: 3

# Default behavior when no pattern matches
default:
  action:
    type: complete
    delay: 500ms
    outputs: {}  # No outputs

# Logging configuration
logging:
  level: debug  # debug, info, warn, error
  format: json  # json or text
  file: ""      # Empty = stderr, otherwise file path
```

### Environment Variable Overrides

```bash
MEOW_SIM_CONFIG=/path/to/config.yaml  # Config file path
MEOW_SIM_DELAY=100ms                   # Override all delays
MEOW_SIM_LOG_LEVEL=debug               # Override log level
MEOW_SIM_DETERMINISTIC=true            # Disable any randomization
```

---

## Adapter Integration

### Simulator Adapter Configuration

```toml
# ~/.meow/adapters/simulator/adapter.toml

[adapter]
name = "simulator"
description = "Claude Code simulator for E2E testing"

[spawn]
# The simulator binary, with config path from workflow variable
command = "meow-agent-sim --config {{sim_config}}"
# Resume is a no-op for simulator but must be accepted
resume_command = "meow-agent-sim --config {{sim_config}} --resume {{session_id}}"
# Much faster startup than real Claude
startup_delay = "100ms"

[environment]
# Same as claude adapter - prevent tmux detection
TMUX = ""

[prompt_injection]
# No pre-keys needed - simulator is always ready
pre_keys = []
pre_delay = "0ms"
method = "literal"
post_keys = ["Enter"]
post_delay = "50ms"

[graceful_stop]
keys = ["C-c"]
wait = "500ms"

[events]
# Use the same event translator as claude adapter
# Simulator will call meow commands directly, but this
# provides consistency if hooks are configured
translator = "./event-translator.sh"

[events.agent_config]
Stop = "{{adapter_dir}}/event-translator.sh Stop"
PreToolUse = "{{adapter_dir}}/event-translator.sh PreToolUse $TOOL_NAME"
PostToolUse = "{{adapter_dir}}/event-translator.sh PostToolUse $TOOL_NAME"
```

### Using Simulator in Workflows

Test workflows specify the simulator adapter:

```toml
# test-e2e-stop-hook.meow.toml

[main]
name = "e2e-stop-hook-test"
description = "E2E test for stop hook behavior"

[main.variables]
sim_config = { required = true, description = "Path to simulator config" }

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "test-agent"
adapter = "simulator"  # Use simulator instead of claude
workdir = "{{workdir}}"

[[main.steps]]
id = "work"
executor = "agent"
agent = "test-agent"
prompt = "implement the feature"  # Matches ask pattern in config
mode = "interactive"  # Test interactive mode
needs = ["spawn"]

[main.steps.outputs]
result = { required = true, type = "string" }

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "test-agent"
needs = ["work"]
```

---

## Test Framework Design

### Test Harness Structure

```go
// internal/testutil/e2e/harness.go

// Harness provides E2E test infrastructure
type Harness struct {
    t           *testing.T
    tempDir     string          // Isolated test directory
    tmuxServer  string          // Dedicated tmux socket
    orchSocket  string          // Orchestrator IPC socket
    simConfigs  map[string]string // Agent ID -> config path
    workflows   []*Workflow     // Running workflows
    logs        *LogCapture     // Captured logs for assertions
}

// NewHarness creates an isolated E2E test environment
func NewHarness(t *testing.T) *Harness {
    t.Helper()

    tempDir := t.TempDir()
    tmuxSocket := filepath.Join(tempDir, "tmux.sock")

    return &Harness{
        t:          t,
        tempDir:    tempDir,
        tmuxServer: tmuxSocket,
        simConfigs: make(map[string]string),
    }
}

// ConfigureAgent sets up a simulator config for an agent
func (h *Harness) ConfigureAgent(agentID string, config SimConfig) {
    configPath := filepath.Join(h.tempDir, agentID+"-sim.yaml")
    writeYAML(configPath, config)
    h.simConfigs[agentID] = configPath
}

// RunWorkflow starts a workflow with the simulator adapter
func (h *Harness) RunWorkflow(templatePath string, vars map[string]string) *WorkflowRun {
    // Inject sim_config variables for each agent
    for agentID, configPath := range h.simConfigs {
        vars["sim_config_"+agentID] = configPath
    }

    // Start orchestrator in background
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    run := &WorkflowRun{
        harness: h,
        ctx:     ctx,
        cancel:  cancel,
    }

    go run.execute(templatePath, vars)

    return run
}

// Cleanup tears down all test resources
func (h *Harness) Cleanup() {
    // Kill any remaining tmux sessions
    exec.Command("tmux", "-S", h.tmuxServer, "kill-server").Run()
}
```

### Workflow Run Helpers

```go
// internal/testutil/e2e/workflow_run.go

type WorkflowRun struct {
    harness    *Harness
    ctx        context.Context
    cancel     context.CancelFunc
    workflowID string
    status     string
    steps      map[string]*StepState
    events     []Event
    mu         sync.RWMutex
}

// WaitForStep blocks until a step reaches the expected status
func (r *WorkflowRun) WaitForStep(stepID string, status string, timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        r.mu.RLock()
        step, ok := r.steps[stepID]
        r.mu.RUnlock()

        if ok && step.Status == status {
            return nil
        }
        time.Sleep(50 * time.Millisecond)
    }
    return fmt.Errorf("step %s did not reach status %s within %v", stepID, status, timeout)
}

// WaitForEvent blocks until a matching event is received
func (r *WorkflowRun) WaitForEvent(eventType string, filter map[string]string, timeout time.Duration) (*Event, error) {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        r.mu.RLock()
        for _, e := range r.events {
            if e.Type == eventType && matchesFilter(e, filter) {
                r.mu.RUnlock()
                return &e, nil
            }
        }
        r.mu.RUnlock()
        time.Sleep(50 * time.Millisecond)
    }
    return nil, fmt.Errorf("event %s with filter %v not received within %v", eventType, filter, timeout)
}

// WaitForDone blocks until workflow completes
func (r *WorkflowRun) WaitForDone(timeout time.Duration) error {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        r.mu.RLock()
        status := r.status
        r.mu.RUnlock()

        if status == "done" || status == "failed" || status == "stopped" {
            return nil
        }
        time.Sleep(100 * time.Millisecond)
    }
    return fmt.Errorf("workflow did not complete within %v", timeout)
}

// AssertStepOutput checks a step's output value
func (r *WorkflowRun) AssertStepOutput(t *testing.T, stepID, key string, expected any) {
    t.Helper()
    r.mu.RLock()
    defer r.mu.RUnlock()

    step, ok := r.steps[stepID]
    if !ok {
        t.Errorf("step %s not found", stepID)
        return
    }

    actual, ok := step.Outputs[key]
    if !ok {
        t.Errorf("output %s not found on step %s", key, stepID)
        return
    }

    if actual != expected {
        t.Errorf("step %s output %s: got %v, want %v", stepID, key, actual, expected)
    }
}
```

### Simulator Configuration Builder

```go
// internal/testutil/e2e/sim_config.go

type SimConfig struct {
    Timing    TimingConfig    `yaml:"timing"`
    Hooks     HooksConfig     `yaml:"hooks"`
    Behaviors []Behavior      `yaml:"behaviors"`
    Default   DefaultBehavior `yaml:"default"`
    Logging   LoggingConfig   `yaml:"logging"`
}

type Behavior struct {
    Match  string         `yaml:"match"`
    Type   string         `yaml:"type,omitempty"` // regex or contains
    Action BehaviorAction `yaml:"action"`
}

type BehaviorAction struct {
    Type        string            `yaml:"type"` // complete, ask, fail, hang, crash
    Delay       string            `yaml:"delay,omitempty"`
    Outputs     map[string]any    `yaml:"outputs,omitempty"`
    Events      []EventDef        `yaml:"events,omitempty"`
    Question    string            `yaml:"question,omitempty"`
    FailCount   int               `yaml:"fail_count,omitempty"`
    FailMessage string            `yaml:"fail_message,omitempty"`
    ExitCode    int               `yaml:"exit_code,omitempty"`
}

// Builder pattern for fluent config creation

func NewSimConfig() *SimConfigBuilder {
    return &SimConfigBuilder{
        config: SimConfig{
            Timing: TimingConfig{
                StartupDelay:     "100ms",
                DefaultWorkDelay: "100ms",
                PromptDelay:      "10ms",
            },
            Hooks: HooksConfig{
                FireStopHook:   true,
                FireToolEvents: true,
            },
            Default: DefaultBehavior{
                Action: BehaviorAction{
                    Type:  "complete",
                    Delay: "100ms",
                },
            },
            Logging: LoggingConfig{
                Level:  "debug",
                Format: "json",
            },
        },
    }
}

func (b *SimConfigBuilder) OnPrompt(pattern string) *BehaviorBuilder {
    return &BehaviorBuilder{parent: b, behavior: Behavior{Match: pattern}}
}

func (bb *BehaviorBuilder) Complete(outputs map[string]any) *SimConfigBuilder {
    bb.behavior.Action = BehaviorAction{Type: "complete", Outputs: outputs}
    bb.parent.config.Behaviors = append(bb.parent.config.Behaviors, bb.behavior)
    return bb.parent
}

func (bb *BehaviorBuilder) Ask(question string) *SimConfigBuilder {
    bb.behavior.Action = BehaviorAction{Type: "ask", Question: question}
    bb.parent.config.Behaviors = append(bb.parent.config.Behaviors, bb.behavior)
    return bb.parent
}

func (bb *BehaviorBuilder) Fail(message string) *SimConfigBuilder {
    bb.behavior.Action = BehaviorAction{Type: "fail", FailMessage: message}
    bb.parent.config.Behaviors = append(bb.parent.config.Behaviors, bb.behavior)
    return bb.parent
}

// Usage example:
// config := NewSimConfig().
//     OnPrompt("Write tests").Complete(map[string]any{"file": "test.ts"}).
//     OnPrompt("implement").Ask("Which approach?").
//     OnPrompt("fail-test").Fail("Intentional failure").
//     Build()
```

---

## Test Scenarios

### 1. Happy Path Workflow

**Objective:** Verify basic orchestration works end-to-end

```go
func TestE2E_HappyPath(t *testing.T) {
    h := e2e.NewHarness(t)
    defer h.Cleanup()

    // Configure simulator to complete all prompts
    h.ConfigureAgent("worker", e2e.NewSimConfig().
        OnPrompt(".*").Complete(map[string]any{"result": "success"}).
        Build())

    // Run workflow
    run := h.RunWorkflow("templates/simple.meow.toml", nil)

    // Wait for completion
    require.NoError(t, run.WaitForDone(10*time.Second))
    assert.Equal(t, "done", run.Status())
    run.AssertStepOutput(t, "main-task", "result", "success")
}
```

### 2. Stop Hook Recovery

**Objective:** Verify stop hook re-injects prompt when agent stops unexpectedly

```go
func TestE2E_StopHookRecovery(t *testing.T) {
    h := e2e.NewHarness(t)
    defer h.Cleanup()

    // First attempt: fail (triggers stop hook)
    // Second attempt: succeed
    h.ConfigureAgent("worker", e2e.NewSimConfig().
        OnPrompt("implement").FailThenSucceed(1, "Temporary error",
            map[string]any{"result": "fixed"}).
        Build())

    run := h.RunWorkflow("templates/agent-demo.meow.toml", nil)

    // Should eventually succeed despite first failure
    require.NoError(t, run.WaitForDone(15*time.Second))
    assert.Equal(t, "done", run.Status())
    run.AssertStepOutput(t, "work", "result", "fixed")

    // Verify stop hook was called
    events := run.EventsOfType("agent-stopped")
    assert.GreaterOrEqual(t, len(events), 1)
}
```

### 3. Interactive Mode

**Objective:** Verify interactive mode stops at questions without re-injecting

```go
func TestE2E_InteractiveMode(t *testing.T) {
    h := e2e.NewHarness(t)
    defer h.Cleanup()

    // Simulator asks a question
    h.ConfigureAgent("worker", e2e.NewSimConfig().
        OnPrompt("review the code").Ask("Found 3 issues. Continue anyway?").
        OnPrompt("yes|continue").Complete(map[string]any{"reviewed": true}).
        Build())

    run := h.RunWorkflow("templates/interactive-review.meow.toml", nil)

    // Wait for question state
    require.NoError(t, run.WaitForStep("review", "running", 5*time.Second))

    // Verify simulator is at prompt (asking)
    time.Sleep(500 * time.Millisecond)
    assert.True(t, h.SimulatorAtPrompt("worker"))

    // Stop hook should have fired agent-stopped event
    events := run.EventsOfType("agent-stopped")
    assert.GreaterOrEqual(t, len(events), 1)

    // Inject user response
    h.InjectToSimulator("worker", "yes, continue")

    // Should complete
    require.NoError(t, run.WaitForDone(5*time.Second))
    run.AssertStepOutput(t, "review", "reviewed", true)
}
```

### 4. Event Routing

**Objective:** Verify events are routed to await-event waiters

```go
func TestE2E_EventRouting(t *testing.T) {
    h := e2e.NewHarness(t)
    defer h.Cleanup()

    // Simulator emits tool events during work
    h.ConfigureAgent("worker", e2e.NewSimConfig().
        OnPrompt("process files").CompleteWithEvents(
            map[string]any{"processed": 5},
            []e2e.EventDef{
                {Type: "tool-starting", Data: map[string]any{"tool": "Read"}, When: "0ms"},
                {Type: "tool-completed", Data: map[string]any{"tool": "Read"}, When: "100ms"},
            }).
        Build())

    run := h.RunWorkflow("templates/event-monitoring.meow.toml", nil)

    // Wait for tool-completed event
    event, err := run.WaitForEvent("tool-completed",
        map[string]string{"tool": "Read"}, 5*time.Second)
    require.NoError(t, err)
    assert.Equal(t, "Read", event.Data["tool"])

    // Workflow should complete
    require.NoError(t, run.WaitForDone(10*time.Second))
}
```

### 5. Parallel Agents

**Objective:** Verify parallel execution and join synchronization

```go
func TestE2E_ParallelAgents(t *testing.T) {
    h := e2e.NewHarness(t)
    defer h.Cleanup()

    // Configure 3 agents with different delays
    for i, delay := range []string{"100ms", "200ms", "150ms"} {
        agentID := fmt.Sprintf("worker-%d", i)
        h.ConfigureAgent(agentID, e2e.NewSimConfig().
            OnPrompt(".*").CompleteWithDelay(delay,
                map[string]any{"agent": agentID}).
            Build())
    }

    run := h.RunWorkflow("templates/test-multi-agent-join.meow.toml", nil)

    // All agents should start nearly simultaneously
    for i := 0; i < 3; i++ {
        stepID := fmt.Sprintf("task-%d", i)
        require.NoError(t, run.WaitForStep(stepID, "running", 2*time.Second))
    }

    // Verify they ran in parallel (all started before any finished)
    startTimes := run.StepStartTimes("task-0", "task-1", "task-2")
    endTimes := run.StepEndTimes("task-0", "task-1", "task-2")

    // All starts should be before any end (parallel execution)
    latestStart := maxTime(startTimes)
    earliestEnd := minTime(endTimes)
    assert.True(t, latestStart.Before(earliestEnd),
        "agents should run in parallel")

    // Join step should wait for all
    require.NoError(t, run.WaitForStep("aggregate", "done", 5*time.Second))
    require.NoError(t, run.WaitForDone(5*time.Second))
}
```

### 6. Crash Recovery

**Objective:** Verify orchestrator recovers from mid-workflow crash

```go
func TestE2E_CrashRecovery(t *testing.T) {
    h := e2e.NewHarness(t)
    defer h.Cleanup()

    // Simulator completes step 1, crashes on step 2, then succeeds
    crashCounter := &atomic.Int32{}
    h.ConfigureAgent("worker", e2e.NewSimConfig().
        OnPrompt("step-1").Complete(map[string]any{"result": "step1-done"}).
        OnPrompt("step-2").DynamicAction(func(prompt string) BehaviorAction {
            if crashCounter.Add(1) == 1 {
                return BehaviorAction{Type: "crash", ExitCode: 137}
            }
            return BehaviorAction{Type: "complete", Outputs: map[string]any{"result": "step2-done"}}
        }).
        Build())

    // Start workflow
    run := h.RunWorkflow("templates/multi-step.meow.toml", nil)

    // Wait for step 1 to complete
    require.NoError(t, run.WaitForStep("step-1", "done", 5*time.Second))

    // Wait for crash (step 2 running then fails)
    require.NoError(t, run.WaitForStep("step-2", "running", 5*time.Second))
    time.Sleep(500 * time.Millisecond) // Let crash happen

    // Orchestrator should detect dead agent
    // In real scenario, orchestrator would crash too
    // For test, we simulate orchestrator restart
    h.RestartOrchestrator()

    // Recovery should respawn agent and retry step 2
    require.NoError(t, run.WaitForStep("step-2", "done", 10*time.Second))
    require.NoError(t, run.WaitForDone(5*time.Second))

    run.AssertStepOutput(t, "step-2", "result", "step2-done")
}
```

### 7. Output Validation

**Objective:** Verify output validation rejects bad outputs and allows retry

```go
func TestE2E_OutputValidation(t *testing.T) {
    h := e2e.NewHarness(t)
    defer h.Cleanup()

    // First attempt: wrong output type
    // Second attempt: correct output
    h.ConfigureAgent("worker", e2e.NewSimConfig().
        OnPrompt("validate output").SequentialActions([]BehaviorAction{
            {Type: "complete", Outputs: map[string]any{
                "count": "not-a-number", // Wrong type
            }},
            {Type: "complete", Outputs: map[string]any{
                "count": 42, // Correct type
            }},
        }).
        Build())

    run := h.RunWorkflow("templates/output-validation.meow.toml", nil)

    require.NoError(t, run.WaitForDone(10*time.Second))
    run.AssertStepOutput(t, "validate", "count", 42)

    // Verify error was returned for first attempt
    ipcLogs := run.IPCLogs()
    assert.True(t, containsError(ipcLogs, "type validation failed"))
}
```

### 8. Fire-and-Forget Mode

**Objective:** Verify fire_forget mode injects without waiting

```go
func TestE2E_FireForget(t *testing.T) {
    h := e2e.NewHarness(t)
    defer h.Cleanup()

    // Simulator receives /compact and continues
    h.ConfigureAgent("worker", e2e.NewSimConfig().
        OnPrompt("/compact").NoOp(). // Acknowledge but don't call meow done
        OnPrompt("main task").Complete(map[string]any{"result": "done"}).
        Build())

    run := h.RunWorkflow("templates/fire-forget-compact.meow.toml", nil)

    // Fire-forget step should complete immediately after injection
    require.NoError(t, run.WaitForStep("send-compact", "done", 2*time.Second))

    // Main task should proceed
    require.NoError(t, run.WaitForStep("main-task", "done", 5*time.Second))
    require.NoError(t, run.WaitForDone(5*time.Second))
}
```

---

## Non-Flakiness Strategies

### 1. Isolated Test Environments

Each test gets:
- Unique temp directory
- Dedicated tmux socket (`tmux -S /tmp/test-xxx.sock`)
- Unique orchestrator IPC socket
- No shared state

```go
func NewHarness(t *testing.T) *Harness {
    tempDir := t.TempDir() // Automatically cleaned up
    tmuxSocket := filepath.Join(tempDir, "tmux.sock")
    os.Setenv("TMUX_TMPDIR", tempDir) // Isolate tmux
    // ...
}
```

### 2. Deterministic Timing

- No randomization in default test mode
- Fixed delays (not ranges)
- Configurable via environment for debugging

```yaml
timing:
  startup_delay: 100ms  # Fixed, not "100-500ms"
  default_work_delay: 100ms
```

### 3. Explicit Synchronization

Tests wait for specific states rather than sleeping:

```go
// BAD: Flaky
time.Sleep(2 * time.Second)
assert.Equal(t, "done", run.Status())

// GOOD: Explicit wait
require.NoError(t, run.WaitForDone(5*time.Second))
```

### 4. Retry-Safe Operations

Operations that might fail transiently have built-in retries:

```go
func (h *Harness) InjectToSimulator(agentID, text string) error {
    var lastErr error
    for attempt := 0; attempt < 3; attempt++ {
        if err := h.sendKeys(agentID, text); err != nil {
            lastErr = err
            time.Sleep(100 * time.Millisecond)
            continue
        }
        return nil
    }
    return lastErr
}
```

### 5. Comprehensive Logging

All simulator actions logged with timestamps:

```json
{"time":"2026-01-09T10:00:00.123Z","level":"debug","msg":"state_transition","from":"idle","to":"working","prompt":"implement feature"}
{"time":"2026-01-09T10:00:00.234Z","level":"debug","msg":"ipc_call","command":"meow done","outputs":{"result":"success"}}
```

### 6. Timeout Assertions

All waits have explicit timeouts:

```go
func (r *WorkflowRun) WaitForDone(timeout time.Duration) error {
    select {
    case <-r.doneCh:
        return nil
    case <-time.After(timeout):
        return fmt.Errorf("workflow did not complete within %v\nCurrent state: %s\nStep states: %v",
            timeout, r.status, r.stepStates())
    }
}
```

---

## Implementation Phases

### Phase 1: Core Simulator Binary

1. Create `cmd/meow-agent-sim/main.go`
2. Implement state machine (STARTING → IDLE → WORKING → DONE)
3. Implement stdin reading and prompt detection
4. Implement `meow done` calling
5. Implement basic behavior matching

### Phase 2: Hook Emulation

1. Implement stop hook (meow event agent-stopped)
2. Implement tool event emission
3. Test with real orchestrator

### Phase 3: Simulator Adapter

1. Create `simulator` adapter config
2. Test adapter loading and spawn
3. Verify prompt injection works

### Phase 4: Test Framework

1. Create `internal/testutil/e2e/` package
2. Implement Harness, WorkflowRun, SimConfig
3. Create builder patterns for fluent config

### Phase 5: Core Test Suite

1. Happy path tests
2. Stop hook tests
3. Interactive mode tests
4. Output validation tests

### Phase 6: Advanced Tests

1. Parallel agent tests
2. Event routing tests
3. Crash recovery tests
4. Template pattern coverage

### Phase 7: CI Integration

1. Add E2E tests to CI pipeline
2. Configure timeouts and retries
3. Add coverage reporting

---

## Appendix: Quick Reference

### Simulator CLI

```bash
# Start simulator with config
meow-agent-sim --config sim-config.yaml

# With resume (for testing resume flows)
meow-agent-sim --config sim-config.yaml --resume session-123

# Debug mode
MEOW_SIM_LOG_LEVEL=debug meow-agent-sim --config sim-config.yaml
```

### Key Files

| File | Purpose |
|------|---------|
| `cmd/meow-agent-sim/main.go` | Simulator binary entry point |
| `internal/testutil/e2e/harness.go` | E2E test harness |
| `internal/testutil/e2e/sim_config.go` | Simulator config types |
| `test/e2e/*_test.go` | E2E test suites |
| `~/.meow/adapters/simulator/` | Simulator adapter |

### Environment Variables

| Variable | Purpose |
|----------|---------|
| `MEOW_AGENT` | Agent ID (set by orchestrator) |
| `MEOW_WORKFLOW` | Workflow ID (set by orchestrator) |
| `MEOW_ORCH_SOCK` | IPC socket path (set by orchestrator) |
| `MEOW_SIM_CONFIG` | Simulator config path |
| `MEOW_SIM_LOG_LEVEL` | Logging verbosity |

---

*This document serves as the authoritative specification for E2E testing infrastructure. Implementation should follow this design closely.*
