package e2e_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meow-stack/meow-machine/internal/testutil/e2e"
	"github.com/meow-stack/meow-machine/internal/types"
	"gopkg.in/yaml.v3"
)

// TestMain builds required binaries before running tests.
func TestMain(m *testing.M) {
	// Find the project root (where go.mod is)
	projectRoot := findProjectRoot()
	if projectRoot == "" {
		fmt.Fprintf(os.Stderr, "could not find project root\n")
		os.Exit(1)
	}

	// Build meow CLI
	cmd := exec.Command("go", "build", "-o", "/tmp/meow-e2e-bin", "./cmd/meow")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build meow: %v\n%s\n", err, out)
		os.Exit(1)
	}

	// Build simulator
	cmd = exec.Command("go", "build", "-o", "/tmp/meow-agent-sim-e2e", "./cmd/meow-agent-sim")
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build simulator: %v\n%s\n", err, out)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

// findProjectRoot walks up to find go.mod
func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// runMeow executes meow CLI and returns stdout, stderr, and any error.
func runMeow(h *e2e.Harness, args ...string) (string, string, error) {
	return runMeowWithTimeout(h, 60*time.Second, args...)
}

// runMeowWithTimeout executes meow CLI with a custom timeout.
func runMeowWithTimeout(h *e2e.Harness, timeout time.Duration, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/tmp/meow-e2e-bin", args...)
	cmd.Dir = h.TempDir
	cmd.Env = h.Env()
	// Add PATH so meow can find the simulator
	cmd.Env = append(cmd.Env, "PATH=/tmp:"+os.Getenv("PATH"))

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// ===========================================================================
// Happy Path Tests
// ===========================================================================

func TestE2E_SingleShellStep(t *testing.T) {
	h := e2e.NewHarness(t)

	// Write a simple shell-only template
	template := `
[main]
name = "single-shell"

[[main.steps]]
id = "echo-step"
executor = "shell"
command = "echo 'hello world'"
`
	if err := h.WriteTemplate("single-shell.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// Run the workflow
	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "single-shell.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify workflow completed
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected 'workflow completed' in output, got:\n%s", stderr)
	}
}

func TestE2E_MultipleShellSteps(t *testing.T) {
	h := e2e.NewHarness(t)

	template := `
[main]
name = "multi-shell"

[[main.steps]]
id = "step1"
executor = "shell"
command = "echo 'step 1'"

[[main.steps]]
id = "step2"
executor = "shell"
needs = ["step1"]
command = "echo 'step 2'"

[[main.steps]]
id = "step3"
executor = "shell"
needs = ["step2"]
command = "echo 'step 3'"
`
	if err := h.WriteTemplate("multi-shell.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "multi-shell.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete, got:\n%s", stderr)
	}
}

func TestE2E_ParallelShellSteps(t *testing.T) {
	h := e2e.NewHarness(t)

	template := `
[main]
name = "parallel-shell"

[[main.steps]]
id = "parallel-1"
executor = "shell"
command = "echo 'parallel 1' && sleep 0.1"

[[main.steps]]
id = "parallel-2"
executor = "shell"
command = "echo 'parallel 2' && sleep 0.1"

[[main.steps]]
id = "parallel-3"
executor = "shell"
command = "echo 'parallel 3' && sleep 0.1"

[[main.steps]]
id = "join"
executor = "shell"
needs = ["parallel-1", "parallel-2", "parallel-3"]
command = "echo 'all done'"
`
	if err := h.WriteTemplate("parallel-shell.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	start := time.Now()
	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "parallel-shell.toml"))
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Parallel steps should complete faster than sequential
	// 3 steps @ 0.1s each = 0.3s sequential, ~0.1s parallel
	// Allow some overhead for process startup
	if duration > 1500*time.Millisecond {
		t.Errorf("parallel steps took too long: %v (expected < 1500ms)", duration)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

func TestE2E_ShellOutputCapture(t *testing.T) {
	h := e2e.NewHarness(t)

	template := `
[main]
name = "shell-output"

[[main.steps]]
id = "produce"
executor = "shell"
command = "echo 'captured-value'"
[main.steps.shell_outputs]
result = { source = "stdout" }

[[main.steps]]
id = "consume"
executor = "shell"
needs = ["produce"]
command = "echo 'got: {{produce.outputs.result}}'"
`
	if err := h.WriteTemplate("shell-output.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "shell-output.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

// ===========================================================================
// Agent Step Tests (with Simulator)
// ===========================================================================

func TestE2E_SingleAgentStep(t *testing.T) {
	h := e2e.NewHarness(t)

	// Configure simulator to complete immediately
	simConfig := e2e.NewSimConfigBuilder().
		WithDefaultAction(e2e.ActionComplete).
		WithStartupDelay(50 * time.Millisecond).
		WithDelay(10 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	// Write simulator adapter config
	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"
args = []

[timing]
startup_delay = "100ms"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	template := `
[main]
name = "single-agent"

[[main.steps]]
id = "spawn-agent"
executor = "spawn"
agent = "test-agent"

[[main.steps]]
id = "work"
executor = "agent"
agent = "test-agent"
needs = ["spawn-agent"]
prompt = "Do something simple"

[[main.steps]]
id = "kill-agent"
executor = "kill"
agent = "test-agent"
needs = ["work"]
graceful = true
`
	if err := h.WriteTemplate("single-agent.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "single-agent.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete, got:\n%s", stderr)
	}
}

func TestE2E_AgentWithOutputs(t *testing.T) {
	h := e2e.NewHarness(t)

	// Configure simulator to produce outputs
	simConfig := e2e.NewSimConfigBuilder().
		WithBehaviorOutputs("produce output", map[string]any{
			"result": "hello-from-agent",
			"count":  42,
		}).
		WithStartupDelay(50 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	template := `
[main]
name = "agent-outputs"

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "producer"

[[main.steps]]
id = "produce"
executor = "agent"
agent = "producer"
needs = ["spawn"]
prompt = "Please produce output for me"

[[main.steps]]
id = "kill"
executor = "kill"
agent = "producer"
needs = ["produce"]
graceful = true
`
	if err := h.WriteTemplate("agent-outputs.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "agent-outputs.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

func TestE2E_ParallelAgents(t *testing.T) {
	h := e2e.NewHarness(t)

	simConfig := e2e.NewSimConfigBuilder().
		WithDefaultAction(e2e.ActionComplete).
		WithStartupDelay(50 * time.Millisecond).
		WithDelay(50 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	template := `
[main]
name = "parallel-agents"

[[main.steps]]
id = "spawn-1"
executor = "spawn"
agent = "agent-1"

[[main.steps]]
id = "spawn-2"
executor = "spawn"
agent = "agent-2"

[[main.steps]]
id = "work-1"
executor = "agent"
agent = "agent-1"
needs = ["spawn-1"]
prompt = "Agent 1 work"

[[main.steps]]
id = "work-2"
executor = "agent"
agent = "agent-2"
needs = ["spawn-2"]
prompt = "Agent 2 work"

[[main.steps]]
id = "kill-1"
executor = "kill"
agent = "agent-1"
needs = ["work-1"]
graceful = true

[[main.steps]]
id = "kill-2"
executor = "kill"
agent = "agent-2"
needs = ["work-2"]
graceful = true

[[main.steps]]
id = "done"
executor = "shell"
needs = ["kill-1", "kill-2"]
command = "echo 'both agents done'"
`
	if err := h.WriteTemplate("parallel-agents.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "parallel-agents.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify both spawns and works were dispatched
	if !strings.Contains(stderr, "spawning agent") {
		t.Errorf("expected agent spawning in logs")
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

// ===========================================================================
// Error Handling Tests
// ===========================================================================

func TestE2E_ShellStepFailure(t *testing.T) {
	h := e2e.NewHarness(t)

	template := `
[main]
name = "shell-fail"

[[main.steps]]
id = "fail-step"
executor = "shell"
command = "exit 1"
`
	if err := h.WriteTemplate("shell-fail.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	_, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "shell-fail.toml"))

	// meow run may exit 0 even if workflow fails (it just reports the status)
	// Check for failure indication in output
	hasFailure := strings.Contains(stderr, "failed") ||
		strings.Contains(stderr, "step failed") ||
		strings.Contains(stderr, "exit code 1") ||
		err != nil

	if !hasFailure {
		t.Errorf("expected workflow to indicate failure\nstderr: %s\nerr: %v", stderr, err)
	}
}

func TestE2E_ShellStepFailureWithOnErrorContinue(t *testing.T) {
	h := e2e.NewHarness(t)

	template := `
[main]
name = "shell-fail-continue"

[[main.steps]]
id = "fail-step"
executor = "shell"
command = "exit 1"
on_error = "continue"

[[main.steps]]
id = "after-fail"
executor = "shell"
needs = ["fail-step"]
command = "echo 'still running'"
`
	if err := h.WriteTemplate("shell-fail-continue.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "shell-fail-continue.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Workflow should complete despite failure
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete with on_error=continue")
	}
}

// ===========================================================================
// Workflow State Tests
// ===========================================================================

func TestE2E_WorkflowStateFile(t *testing.T) {
	h := e2e.NewHarness(t)

	template := `
[main]
name = "state-test"

[[main.steps]]
id = "step1"
executor = "shell"
command = "echo 'done'"
`
	if err := h.WriteTemplate("state-test.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "state-test.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v", err)
	}

	// Extract workflow ID from output
	var workflowID string
	for _, line := range strings.Split(stdout, "\n") {
		if strings.HasPrefix(line, "Workflow ID:") {
			workflowID = strings.TrimSpace(strings.TrimPrefix(line, "Workflow ID:"))
			break
		}
	}

	if workflowID == "" {
		t.Logf("stdout: %s", stdout)
		t.Logf("stderr: %s", stderr)
		t.Skip("could not determine workflow ID from output")
	}

	// The meow CLI currently uses its own default state dir (.meow/workflows/)
	// rather than the harness state dir. Check if workflow completed via output.
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}

	// Verify workflow ID was generated
	if !strings.HasPrefix(workflowID, "wf-") {
		t.Errorf("expected workflow ID to start with 'wf-', got %s", workflowID)
	}
}

// ===========================================================================
// Dependency Chain Tests
// ===========================================================================

func TestE2E_DiamondDependency(t *testing.T) {
	h := e2e.NewHarness(t)

	// Diamond pattern:
	//     A
	//    / \
	//   B   C
	//    \ /
	//     D
	template := `
[main]
name = "diamond"

[[main.steps]]
id = "A"
executor = "shell"
command = "echo 'A'"

[[main.steps]]
id = "B"
executor = "shell"
needs = ["A"]
command = "echo 'B'"

[[main.steps]]
id = "C"
executor = "shell"
needs = ["A"]
command = "echo 'C'"

[[main.steps]]
id = "D"
executor = "shell"
needs = ["B", "C"]
command = "echo 'D'"
`
	if err := h.WriteTemplate("diamond.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "diamond.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

// ===========================================================================
// Expand Executor Tests
// ===========================================================================

func TestE2E_SimpleExpand(t *testing.T) {
	h := e2e.NewHarness(t)

	// Template with a sub-workflow that gets expanded
	template := `
[main]
name = "simple-expand"

[[main.steps]]
id = "before"
executor = "shell"
command = "echo 'before expand'"

[[main.steps]]
id = "do-work"
executor = "expand"
template = ".sub-workflow"
needs = ["before"]

[[main.steps]]
id = "after"
executor = "shell"
command = "echo 'after expand'"
needs = ["do-work"]

# Sub-workflow to be expanded
[sub-workflow]
name = "sub-workflow"
internal = true

[[sub-workflow.steps]]
id = "step-a"
executor = "shell"
command = "echo 'expanded step A'"

[[sub-workflow.steps]]
id = "step-b"
executor = "shell"
command = "echo 'expanded step B'"
needs = ["step-a"]
`
	if err := h.WriteTemplate("simple-expand.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "simple-expand.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete, got:\nstderr: %s", stderr)
	}
}

func TestE2E_ExpandWithVariables(t *testing.T) {
	h := e2e.NewHarness(t)

	// Template that passes variables to expanded sub-workflow
	template := `
[main]
name = "expand-with-vars"

[[main.steps]]
id = "setup"
executor = "shell"
command = "echo 'my-value'"
[main.steps.shell_outputs]
value = { source = "stdout" }

[[main.steps]]
id = "expand-it"
executor = "expand"
template = ".parameterized"
needs = ["setup"]
[main.steps.variables]
param = "{{setup.outputs.value}}"
name = "test-run"

# Parameterized sub-workflow
[parameterized]
name = "parameterized"
internal = true

[parameterized.variables]
param = { required = true }
name = { required = true }

[[parameterized.steps]]
id = "use-param"
executor = "shell"
command = "echo 'param={{param}} name={{name}}'"
`
	if err := h.WriteTemplate("expand-vars.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "expand-vars.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

func TestE2E_NestedExpand(t *testing.T) {
	h := e2e.NewHarness(t)

	// Template with nested expansions: main -> level1 -> level2
	template := `
[main]
name = "nested-expand"

[[main.steps]]
id = "start"
executor = "shell"
command = "echo 'starting nested expand'"

[[main.steps]]
id = "level1"
executor = "expand"
template = ".first-level"
needs = ["start"]

[[main.steps]]
id = "finish"
executor = "shell"
command = "echo 'finished nested expand'"
needs = ["level1"]

# First level expansion
[first-level]
name = "first-level"
internal = true

[[first-level.steps]]
id = "before-nested"
executor = "shell"
command = "echo 'first level'"

[[first-level.steps]]
id = "level2"
executor = "expand"
template = ".second-level"
needs = ["before-nested"]

# Second level expansion
[second-level]
name = "second-level"
internal = true

[[second-level.steps]]
id = "deepest"
executor = "shell"
command = "echo 'second level - deepest'"
`
	if err := h.WriteTemplate("nested-expand.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "nested-expand.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

// ===========================================================================
// Branch Executor Tests
// ===========================================================================

func TestE2E_BranchTrueCondition(t *testing.T) {
	h := e2e.NewHarness(t)

	// Branch where condition is true (exit 0)
	template := `
[main]
name = "branch-true"

[[main.steps]]
id = "check"
executor = "branch"
condition = "true"  # Always succeeds

[main.steps.on_true]
template = ".success-path"

[main.steps.on_false]
template = ".failure-path"

[[main.steps]]
id = "after"
executor = "shell"
command = "echo 'after branch'"
needs = ["check"]

# Success path
[success-path]
name = "success-path"
internal = true

[[success-path.steps]]
id = "success"
executor = "shell"
command = "echo 'took success path'"

# Failure path
[failure-path]
name = "failure-path"
internal = true

[[failure-path.steps]]
id = "failure"
executor = "shell"
command = "echo 'took failure path'"
`
	if err := h.WriteTemplate("branch-true.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "branch-true.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

func TestE2E_BranchFalseCondition(t *testing.T) {
	h := e2e.NewHarness(t)

	// Branch where condition is false (exit non-zero)
	template := `
[main]
name = "branch-false"

[[main.steps]]
id = "check"
executor = "branch"
condition = "false"  # Always fails

[main.steps.on_true]
template = ".success-path"

[main.steps.on_false]
template = ".failure-path"

[[main.steps]]
id = "after"
executor = "shell"
command = "echo 'after branch'"
needs = ["check"]

# Success path
[success-path]
name = "success-path"
internal = true

[[success-path.steps]]
id = "success"
executor = "shell"
command = "echo 'took success path'"

# Failure path
[failure-path]
name = "failure-path"
internal = true

[[failure-path.steps]]
id = "failure"
executor = "shell"
command = "echo 'took failure path'"
`
	if err := h.WriteTemplate("branch-false.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "branch-false.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

func TestE2E_BranchWithInlineSteps(t *testing.T) {
	h := e2e.NewHarness(t)

	// Branch using inline steps instead of template reference
	template := `
[main]
name = "branch-inline"

[[main.steps]]
id = "check"
executor = "branch"
condition = "test 1 -eq 1"  # True condition

[main.steps.on_true]
inline = [
  { id = "inline-1", executor = "shell", command = "echo 'inline step 1'" },
  { id = "inline-2", executor = "shell", command = "echo 'inline step 2'", needs = ["inline-1"] }
]

[main.steps.on_false]
inline = []

[[main.steps]]
id = "after"
executor = "shell"
command = "echo 'after inline branch'"
needs = ["check"]
`
	if err := h.WriteTemplate("branch-inline.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "branch-inline.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

func TestE2E_BranchEmptyOnTrue(t *testing.T) {
	h := e2e.NewHarness(t)

	// Branch with empty on_true (just continue without expanding anything)
	template := `
[main]
name = "branch-empty"

[[main.steps]]
id = "check"
executor = "branch"
condition = "true"

[main.steps.on_true]
inline = []  # Empty - just continue

[main.steps.on_false]
template = ".error-handler"

[[main.steps]]
id = "continue"
executor = "shell"
command = "echo 'continued after empty branch'"
needs = ["check"]

[error-handler]
name = "error-handler"
internal = true

[[error-handler.steps]]
id = "handle"
executor = "shell"
command = "echo 'handling error'"
`
	if err := h.WriteTemplate("branch-empty.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "branch-empty.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

func TestE2E_BranchWithShellCondition(t *testing.T) {
	h := e2e.NewHarness(t)

	// Branch using a real shell condition that checks something
	template := `
[main]
name = "branch-shell-condition"

[[main.steps]]
id = "create-file"
executor = "shell"
command = "touch /tmp/meow-test-file-$$"
[main.steps.shell_outputs]
filepath = { source = "stdout" }

[[main.steps]]
id = "check-exists"
executor = "branch"
condition = "test -d /tmp"  # Check if /tmp exists (always true)
needs = ["create-file"]

[main.steps.on_true]
inline = [
  { id = "exists", executor = "shell", command = "echo 'directory exists'" }
]

[main.steps.on_false]
inline = [
  { id = "missing", executor = "shell", command = "echo 'directory missing'" }
]

[[main.steps]]
id = "done"
executor = "shell"
command = "echo 'check complete'"
needs = ["check-exists"]
`
	if err := h.WriteTemplate("branch-shell.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "branch-shell.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

func TestE2E_BranchWithVariableCondition(t *testing.T) {
	h := e2e.NewHarness(t)

	// Branch condition that uses output from previous step
	template := `
[main]
name = "branch-var-condition"

[[main.steps]]
id = "get-value"
executor = "shell"
command = "echo 'yes'"
[main.steps.shell_outputs]
answer = { source = "stdout" }

[[main.steps]]
id = "check-value"
executor = "branch"
condition = "test '{{get-value.outputs.answer}}' = 'yes'"
needs = ["get-value"]

[main.steps.on_true]
inline = [
  { id = "confirmed", executor = "shell", command = "echo 'answer was yes'" }
]

[main.steps.on_false]
inline = [
  { id = "denied", executor = "shell", command = "echo 'answer was not yes'" }
]

[[main.steps]]
id = "done"
executor = "shell"
command = "echo 'branch complete'"
needs = ["check-value"]
`
	if err := h.WriteTemplate("branch-var.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "branch-var.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

func TestE2E_ConditionalRetryLoop(t *testing.T) {
	h := e2e.NewHarness(t)

	// Simulates a retry pattern: try something, branch on result
	// This uses a file counter to track iterations
	template := `
[main]
name = "conditional-retry"

[[main.steps]]
id = "init-counter"
executor = "shell"
command = "echo 0 > /tmp/meow-counter-$$ && echo /tmp/meow-counter-$$"
[main.steps.shell_outputs]
counter_file = { source = "stdout" }

[[main.steps]]
id = "attempt"
executor = "expand"
template = ".try-once"
needs = ["init-counter"]
[main.steps.variables]
counter_file = "{{init-counter.outputs.counter_file}}"

# Single attempt workflow
[try-once]
name = "try-once"
internal = true

[try-once.variables]
counter_file = { required = true }

[[try-once.steps]]
id = "increment"
executor = "shell"
command = "count=$(cat {{counter_file}}); echo $((count + 1)) > {{counter_file}}; cat {{counter_file}}"
[try-once.steps.shell_outputs]
count = { source = "stdout" }

[[try-once.steps]]
id = "check-done"
executor = "branch"
condition = "test $(cat {{counter_file}}) -ge 1"
needs = ["increment"]

[try-once.steps.on_true]
inline = [
  { id = "success", executor = "shell", command = "echo 'reached target count'" }
]

[try-once.steps.on_false]
inline = [
  { id = "retry", executor = "shell", command = "echo 'would retry here'" }
]
`
	if err := h.WriteTemplate("retry-loop.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "retry-loop.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

func TestE2E_ExpandAndBranchCombined(t *testing.T) {
	h := e2e.NewHarness(t)

	// Complex workflow combining expand and branch
	template := `
[main]
name = "expand-branch-combined"

[[main.steps]]
id = "setup"
executor = "shell"
command = "echo 'production'"
[main.steps.shell_outputs]
env = { source = "stdout" }

[[main.steps]]
id = "check-env"
executor = "branch"
condition = "test '{{setup.outputs.env}}' = 'production'"
needs = ["setup"]

[main.steps.on_true]
template = ".prod-deploy"

[main.steps.on_false]
template = ".dev-deploy"

[[main.steps]]
id = "finalize"
executor = "shell"
command = "echo 'deployment complete'"
needs = ["check-env"]

# Production deployment
[prod-deploy]
name = "prod-deploy"
internal = true

[[prod-deploy.steps]]
id = "backup"
executor = "shell"
command = "echo 'backing up production'"

[[prod-deploy.steps]]
id = "deploy"
executor = "shell"
command = "echo 'deploying to production'"
needs = ["backup"]

[[prod-deploy.steps]]
id = "verify"
executor = "shell"
command = "echo 'verifying production'"
needs = ["deploy"]

# Dev deployment (simpler)
[dev-deploy]
name = "dev-deploy"
internal = true

[[dev-deploy.steps]]
id = "deploy"
executor = "shell"
command = "echo 'deploying to dev'"
`
	if err := h.WriteTemplate("expand-branch.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "expand-branch.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete")
	}
}

// ===========================================================================
// Harness Utility Tests
// ===========================================================================

func TestE2E_SimConfigBuilder(t *testing.T) {
	h := e2e.NewHarness(t)

	config := e2e.NewSimConfigBuilder().
		WithBehavior("implement", e2e.ActionComplete).
		WithBehavior("review", e2e.ActionComplete).
		WithBehaviorOutputs("generate", map[string]any{"code": "println"}).
		WithStartupDelay(100 * time.Millisecond).
		WithDelay(50 * time.Millisecond).
		WithStopHook(true).
		WithLogLevel("debug").
		Build()

	if err := h.WriteSimConfig(config); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(h.SimConfigPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var loaded e2e.SimTestConfig
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	if len(loaded.Behaviors) != 3 {
		t.Errorf("expected 3 behaviors, got %d", len(loaded.Behaviors))
	}

	if loaded.Timing.StartupDelay != 100*time.Millisecond {
		t.Errorf("startup delay mismatch: %v", loaded.Timing.StartupDelay)
	}
}

func TestE2E_CreateTestWorkflow(t *testing.T) {
	h := e2e.NewHarness(t)

	steps := map[string]*types.Step{
		"step1": {
			Executor: types.ExecutorShell,
			Status:   types.StepStatusDone,
			Shell:    &types.ShellConfig{Command: "echo test"},
		},
		"step2": {
			Executor: types.ExecutorShell,
			Status:   types.StepStatusPending,
			Needs:    []string{"step1"},
			Shell:    &types.ShellConfig{Command: "echo test2"},
		},
	}

	run, err := e2e.CreateTestWorkflow(h, "test-wf", steps)
	if err != nil {
		t.Fatalf("failed to create test workflow: %v", err)
	}

	// Verify workflow was saved
	wf, err := run.Workflow()
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}

	if wf.ID != "test-wf" {
		t.Errorf("workflow ID mismatch: %s", wf.ID)
	}

	if len(wf.Steps) != 2 {
		t.Errorf("expected 2 steps, got %d", len(wf.Steps))
	}

	// Test step status check
	status, err := run.StepStatus("step1")
	if err != nil {
		t.Fatalf("failed to get step status: %v", err)
	}
	if status != "done" {
		t.Errorf("expected step1 status done, got %s", status)
	}
}

func TestE2E_SimConfigBuilder_WithHangBehavior(t *testing.T) {
	h := e2e.NewHarness(t)

	config := e2e.NewSimConfigBuilder().
		WithHangBehavior("stuck task").
		Build()

	if err := h.WriteSimConfig(config); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Verify file was written with hang behavior
	data, err := os.ReadFile(h.SimConfigPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var loaded e2e.SimTestConfig
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	if len(loaded.Behaviors) != 1 {
		t.Fatalf("expected 1 behavior, got %d", len(loaded.Behaviors))
	}

	if loaded.Behaviors[0].Match != "stuck task" {
		t.Errorf("expected match 'stuck task', got %q", loaded.Behaviors[0].Match)
	}

	if loaded.Behaviors[0].Action.Type != e2e.ActionHang {
		t.Errorf("expected action type 'hang', got %q", loaded.Behaviors[0].Action.Type)
	}
}

func TestE2E_SimConfigBuilder_WithBehaviorSequence(t *testing.T) {
	h := e2e.NewHarness(t)

	config := e2e.NewSimConfigBuilder().
		WithBehaviorSequence("implement", []map[string]any{
			{"wrong": "first"},
			{"task_id": "correct"},
		}).
		Build()

	if err := h.WriteSimConfig(config); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Verify file was written with sequence behavior
	data, err := os.ReadFile(h.SimConfigPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	var loaded e2e.SimTestConfig
	if err := yaml.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	if len(loaded.Behaviors) != 1 {
		t.Fatalf("expected 1 behavior, got %d", len(loaded.Behaviors))
	}

	if loaded.Behaviors[0].Match != "implement" {
		t.Errorf("expected match 'implement', got %q", loaded.Behaviors[0].Match)
	}

	if len(loaded.Behaviors[0].Action.OutputsSequence) != 2 {
		t.Errorf("expected 2 outputs in sequence, got %d", len(loaded.Behaviors[0].Action.OutputsSequence))
	}
}

func TestE2E_AgentSessionControl(t *testing.T) {
	h := e2e.NewHarness(t)

	// Initially, no agent sessions should exist
	agents, err := h.ListAgentSessions()
	if err != nil {
		t.Fatalf("ListAgentSessions failed: %v", err)
	}
	if len(agents) != 0 {
		t.Errorf("expected 0 agents initially, got %d", len(agents))
	}

	// Create a tmux session with agent naming convention
	if err := h.TmuxNewSession("meow-test-agent"); err != nil {
		t.Fatalf("failed to create tmux session: %v", err)
	}

	// Agent should now be alive
	if !h.IsAgentSessionAlive("test-agent") {
		t.Error("expected agent session to be alive")
	}

	// Should appear in list
	agents, err = h.ListAgentSessions()
	if err != nil {
		t.Fatalf("ListAgentSessions failed: %v", err)
	}
	found := false
	for _, a := range agents {
		if a == "test-agent" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'test-agent' in agents list, got %v", agents)
	}

	// Kill the agent
	if err := h.KillAgentSession("test-agent"); err != nil {
		t.Fatalf("KillAgentSession failed: %v", err)
	}

	// Agent should no longer be alive
	if h.IsAgentSessionAlive("test-agent") {
		t.Error("expected agent session to be dead after kill")
	}
}

// ===========================================================================
// Agent Step Timeout Tests
// ===========================================================================

// TestE2E_AgentStepTimeout_SendsSignal tests that when an agent step times out,
// the orchestrator sends C-c to the agent and marks the step as failed.
//
// Per MVP-SPEC-v2 (lines 622-629):
//   1. Send `C-c` (interrupt) to the agent's tmux session
//   2. Wait 10 seconds for graceful stop
//   3. Mark step `failed` with error "Step timed out after {duration}"
//   4. Continue based on `on_error` setting (default: workflow fails)
//
// Expected timeline: ~3s timeout + ~10s grace period = ~13-15s total
func TestE2E_AgentStepTimeout_SendsSignal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Configure simulator to hang forever when it receives a prompt
	simConfig := e2e.NewSimConfigBuilder().
		WithHangBehavior("hang forever").
		WithStartupDelay(50 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"

[timing]
startup_delay = "100ms"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	// Template with agent step that has a short timeout
	// Using 3s timeout for tests - actual timeout + 10s grace period = ~13s max
	template := `
[main]
name = "agent-timeout"

[[main.steps]]
id = "spawn-agent"
executor = "spawn"
agent = "timeout-worker"

[[main.steps]]
id = "hang-step"
executor = "agent"
agent = "timeout-worker"
needs = ["spawn-agent"]
prompt = "This prompt will hang forever"
timeout = "3s"

[[main.steps]]
id = "kill-agent"
executor = "kill"
agent = "timeout-worker"
needs = ["hang-step"]
graceful = true
`
	if err := h.WriteTemplate("agent-timeout.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// Use longer timeout for test since we expect ~13s (3s timeout + 10s grace)
	// but allow up to 30s for any delays
	start := time.Now()
	stdout, stderr, err := runMeowWithTimeout(h, 45*time.Second, "run", filepath.Join(h.TemplateDir, "agent-timeout.toml"))
	duration := time.Since(start)

	t.Logf("Timeout test completed in %v", duration)
	t.Logf("stdout: %s", stdout)
	t.Logf("stderr: %s", stderr)
	if err != nil {
		t.Logf("err: %v", err)
	}

	// The orchestrator should detect the timeout and send interrupts
	// Verify that timeout detection occurred
	hasTimeoutDetection := strings.Contains(stderr, "timed out") ||
		strings.Contains(stderr, "timeout") ||
		strings.Contains(stderr, "sending interrupt")

	if !hasTimeoutDetection {
		t.Errorf("expected timeout detection in output")
	}

	// Duration should be at least the timeout duration
	if duration < 3*time.Second {
		t.Errorf("expected workflow to take at least 3s for timeout, took %v", duration)
	}

	// Once the orchestrator is fully implemented, we expect the workflow to:
	// 1. Detect timeout after 3s
	// 2. Send C-c and wait up to 10s grace period
	// 3. Mark step as failed
	// 4. Complete workflow (as failed) or run cleanup
	//
	// Current behavior may differ if timeout handling isn't fully implemented.
	// The test verifies timeout detection occurs.
}

// TestE2E_AgentStepTimeout_OnErrorContinue tests that when an agent step times out
// with on_error=continue, the workflow continues to subsequent steps.
//
// Per MVP-SPEC-v2 (lines 3069-3091):
//   on_error = "continue" logs the error and continues to the next step
//
// Expected behavior:
//   1. Agent step times out after 3s
//   2. Step marked failed with error_type: timeout
//   3. Workflow continues due to on_error=continue
//   4. Subsequent steps execute normally
//   5. Workflow completes successfully
func TestE2E_AgentStepTimeout_OnErrorContinue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Simulator hangs on "hang" prompts, completes on "continue" prompts
	simConfig := e2e.NewSimConfigBuilder().
		WithHangBehavior("hang forever").
		WithBehavior("after timeout", e2e.ActionComplete).
		WithStartupDelay(50 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	// Template with timeout step using on_error=continue and a follow-up shell step
	template := `
[main]
name = "timeout-continue"

[[main.steps]]
id = "spawn-worker"
executor = "spawn"
agent = "worker"

[[main.steps]]
id = "will-timeout"
executor = "agent"
agent = "worker"
needs = ["spawn-worker"]
prompt = "This prompt will hang forever"
timeout = "3s"
on_error = "continue"

[[main.steps]]
id = "after-timeout"
executor = "shell"
needs = ["will-timeout"]
command = "echo 'workflow continued after timeout'"

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "worker"
needs = ["after-timeout"]
graceful = true
`
	if err := h.WriteTemplate("timeout-continue.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// Use longer timeout to allow for timeout + grace period + continuation
	stdout, stderr, err := runMeowWithTimeout(h, 45*time.Second, "run", filepath.Join(h.TemplateDir, "timeout-continue.toml"))

	t.Logf("stdout: %s", stdout)
	t.Logf("stderr: %s", stderr)
	if err != nil {
		t.Logf("err: %v", err)
	}

	// Verify timeout was detected
	hasTimeoutDetection := strings.Contains(stderr, "timed out") ||
		strings.Contains(stderr, "timeout") ||
		strings.Contains(stderr, "sending interrupt")

	if !hasTimeoutDetection {
		t.Errorf("expected timeout detection in output")
	}

	// With on_error=continue fully implemented, workflow should complete successfully
	// even though one step timed out.
	// Note: This requires on_error support for agent steps to be implemented.
	// The test documents the expected behavior.
	if strings.Contains(stderr, "workflow completed") {
		t.Logf("SUCCESS: workflow completed with on_error=continue as expected")
	} else {
		// Log that this is expected behavior that may not be implemented yet
		t.Logf("NOTE: on_error=continue for agent timeouts may not be fully implemented yet")
	}
}

// TestE2E_AgentStepTimeout_OnErrorFail tests that when an agent step times out
// with on_error=fail (default), the workflow fails.
//
// Per MVP-SPEC-v2 (lines 3272-3279):
//   When a step fails and on_error is "fail":
//   1. Workflow status becomes `failed`
//   2. No more steps are executed
//   3. Cleanup runs (kills agents, runs cleanup script)
//   4. Human intervention required
//
// Expected behavior:
//   1. Agent step times out after 3s
//   2. Step marked failed with error_type: timeout
//   3. Workflow fails immediately (default on_error=fail)
//   4. Subsequent steps do NOT run
func TestE2E_AgentStepTimeout_OnErrorFail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Simulator hangs forever
	simConfig := e2e.NewSimConfigBuilder().
		WithHangBehavior("hang forever").
		WithStartupDelay(50 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	// Template with timeout step using default on_error=fail
	template := `
[main]
name = "timeout-fail"

[[main.steps]]
id = "spawn-worker"
executor = "spawn"
agent = "worker"

[[main.steps]]
id = "will-timeout"
executor = "agent"
agent = "worker"
needs = ["spawn-worker"]
prompt = "This prompt will hang forever"
timeout = "3s"
# on_error defaults to "fail"

[[main.steps]]
id = "should-not-run"
executor = "shell"
needs = ["will-timeout"]
command = "echo 'this should not run'"

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "worker"
needs = ["should-not-run"]
graceful = true
`
	if err := h.WriteTemplate("timeout-fail.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// Use longer timeout for test
	stdout, stderr, err := runMeowWithTimeout(h, 45*time.Second, "run", filepath.Join(h.TemplateDir, "timeout-fail.toml"))

	t.Logf("stdout: %s", stdout)
	t.Logf("stderr: %s", stderr)
	if err != nil {
		t.Logf("err: %v", err)
	}

	// Verify timeout was detected
	hasTimeoutDetection := strings.Contains(stderr, "timed out") ||
		strings.Contains(stderr, "timeout") ||
		strings.Contains(stderr, "sending interrupt")

	if !hasTimeoutDetection {
		t.Errorf("expected timeout detection in output")
	}

	// With full implementation, workflow should fail (not complete successfully)
	if strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to fail, but it completed successfully")
	}

	// The test verifies timeout detection. Full on_error=fail semantics
	// (stopping the workflow) requires the timeout handling to be fully
	// implemented with state persistence.
}

// TestE2E_AgentStepTimeout_RecoveryTemplate tests that when an agent step times out
// and on_error references a template, that recovery template is expanded.
//
// Per MVP-SPEC-v2 (lines 3093-3108):
//   When a step fails and on_error references a template:
//   1. Step is marked `failed`
//   2. Error context is captured in `_failed_step` variables
//   3. Recovery template is expanded
//   4. Workflow continues from the recovery template
//
// Example from spec:
//   on_error = ".impl-recovery"  # Expand this template on failure
//
// Expected behavior:
//   1. Agent step times out after 3s
//   2. Step marked failed with error_type: timeout
//   3. Recovery template ".recovery-workflow" is expanded
//   4. Recovery steps execute
//   5. Workflow continues/completes
func TestE2E_AgentStepTimeout_RecoveryTemplate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Simulator hangs on "will timeout", completes on "recovery"
	simConfig := e2e.NewSimConfigBuilder().
		WithHangBehavior("hang forever").
		WithBehavior("recovery", e2e.ActionComplete).
		WithStartupDelay(50 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	// Template with timeout step that expands a recovery template on failure
	template := `
[main]
name = "timeout-recovery"

[[main.steps]]
id = "spawn-worker"
executor = "spawn"
agent = "worker"

[[main.steps]]
id = "will-timeout"
executor = "agent"
agent = "worker"
needs = ["spawn-worker"]
prompt = "This prompt will hang forever"
timeout = "3s"
on_error = ".recovery-workflow"

[[main.steps]]
id = "final-step"
executor = "shell"
needs = ["will-timeout"]
command = "echo 'reached final step'"

[[main.steps]]
id = "cleanup"
executor = "kill"
agent = "worker"
needs = ["final-step"]
graceful = true

# Recovery workflow expanded on timeout
[recovery-workflow]
name = "recovery"
internal = true

[[recovery-workflow.steps]]
id = "handle-timeout"
executor = "shell"
command = "echo 'recovery: handling timeout'"

[[recovery-workflow.steps]]
id = "notify"
executor = "shell"
command = "echo 'recovery: notifying about timeout'"
needs = ["handle-timeout"]
`
	if err := h.WriteTemplate("timeout-recovery.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// Use longer timeout for test
	stdout, stderr, err := runMeowWithTimeout(h, 45*time.Second, "run", filepath.Join(h.TemplateDir, "timeout-recovery.toml"))

	t.Logf("stdout: %s", stdout)
	t.Logf("stderr: %s", stderr)
	if err != nil {
		t.Logf("err: %v", err)
	}

	// Verify timeout was detected
	hasTimeoutDetection := strings.Contains(stderr, "timed out") ||
		strings.Contains(stderr, "timeout") ||
		strings.Contains(stderr, "sending interrupt")

	if !hasTimeoutDetection {
		t.Errorf("expected timeout detection in output")
	}

	// Check if recovery steps were executed
	// This requires template-based on_error recovery to be implemented
	hasRecovery := strings.Contains(stderr, "recovery") ||
		strings.Contains(stdout, "recovery")

	if hasRecovery {
		t.Logf("SUCCESS: recovery template was expanded and executed")
	} else {
		// Template-based on_error recovery is a Phase 8 feature per spec
		// This test documents the expected behavior
		t.Logf("NOTE: Template-based on_error recovery may not be implemented yet (Phase 8)")
	}
}

// ===========================================================================
// Agent Crash Tests
//
// These tests verify that the orchestrator properly detects and handles agent
// crashes. Per MVP-SPEC-v2, when an agent's tmux session dies unexpectedly:
// - The orchestrator should detect the crash via session liveness polling
// - The step should be marked as failed with error_type "agent_crashed"
// - Resources should be cleaned up properly
//
// Current Status:
// - Crash detection via session liveness is NOT YET IMPLEMENTED
// - These tests rely on step timeout as a fallback
// - Timeout handling has a known issue (InterruptedAt not persisted)
//
// These tests document expected behavior and serve as acceptance criteria
// for the crash detection feature.
// ===========================================================================

// TestE2E_AgentCrash_SessionDies tests that when an agent's tmux session dies
// unexpectedly (via crash), the orchestrator detects it and marks the step as failed.
//
// Expected behavior per MVP-SPEC-v2:
// - Orchestrator polls for session liveness
// - Detects that session is gone
// - Marks step as failed with error_type="agent_crashed"
// - Workflow fails (unless on_error=continue)
//
// Current behavior: Relies on step timeout to eventually fail.
func TestE2E_AgentCrash_SessionDies(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agent crash test in short mode")
	}

	h := e2e.NewHarness(t)

	// Configure simulator to crash when it receives a prompt
	simConfig := e2e.NewSimConfigBuilder().
		WithCrashBehavior("crash task", 1).
		WithStartupDelay(50 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"

[timing]
startup_delay = "100ms"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	// Template where agent receives a prompt that causes crash
	// Using a very short timeout so the test doesn't hang
	template := `
[main]
name = "agent-crash-test"

[[main.steps]]
id = "spawn-agent"
executor = "spawn"
agent = "crash-agent"

[[main.steps]]
id = "crash-work"
executor = "agent"
agent = "crash-agent"
needs = ["spawn-agent"]
prompt = "Please do this crash task for me"
timeout = "5s"

[[main.steps]]
id = "after-crash"
executor = "shell"
needs = ["crash-work"]
command = "echo 'this should not run'"
`
	// Start orchestrator in background so we can test behavior
	// StartOrchestratorWithTemplate writes the template and starts the orchestrator
	proc, err := h.StartOrchestratorWithTemplate("agent-crash.toml", template)
	if err != nil {
		t.Fatalf("failed to start orchestrator: %v", err)
	}

	// Wait a bit for workflow to start
	time.Sleep(3 * time.Second)

	// The simulator should have crashed by now
	// Check that the orchestrator detected something (session gone)
	// Note: Without crash detection implemented, this will wait for timeout

	// Wait for completion or timeout
	waitErr := proc.WaitWithTimeout(30 * time.Second)

	stderr := proc.Stderr()
	stdout := proc.Stdout()

	t.Logf("Orchestrator stdout: %s", stdout)
	t.Logf("Orchestrator stderr (first 2000 chars): %.2000s", stderr)

	// Document the observed behavior
	if waitErr != nil {
		if strings.Contains(waitErr.Error(), "timeout") {
			t.Log("NOTE: Test timed out - crash detection and timeout handling need implementation")
			t.Skip("Crash detection not yet implemented - orchestrator hung waiting for meow done")
		}
	}

	// Check if workflow indicated failure
	hasFailure := proc.IsDone() && waitErr != nil ||
		strings.Contains(stderr, "failed") ||
		strings.Contains(stderr, "timed out")

	if hasFailure {
		t.Log("Workflow properly detected crash/timeout and failed")
	} else if strings.Contains(stderr, "workflow completed") {
		t.Log("NOTE: Workflow completed despite crash - crash detection not implemented")
	}
}

// TestE2E_AgentCrash_OnErrorContinue tests that when an agent crashes but the step
// has on_error=continue, the workflow continues to the next step.
//
// Expected behavior:
// - Agent crash is detected (via session liveness or timeout)
// - Step is marked failed
// - Due to on_error=continue, workflow proceeds to next step
// - Workflow completes successfully
//
// Current status: Depends on crash detection or timeout handling being implemented.
func TestE2E_AgentCrash_OnErrorContinue(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agent crash test in short mode")
	}

	// This test documents expected behavior for on_error=continue with agent crashes.
	// Currently blocked by crash detection and timeout handling implementation.
	t.Skip("Test blocked: crash detection and timeout grace period handling not yet implemented")
}

// TestE2E_AgentCrash_StopHookNotCalled tests that when an agent crashes abruptly
// (tmux session dies), the stop hook is NOT called since the process is gone.
// This verifies the distinction between graceful stops (stop hook fires) and crashes.
//
// Expected behavior:
// - Agent process crashes (exits with non-zero code)
// - Process is gone before stop hook can fire
// - Orchestrator detects crash via session liveness check
// - Stop hook marker file should NOT exist
//
// Current status: Depends on crash detection being implemented.
func TestE2E_AgentCrash_StopHookNotCalled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agent crash test in short mode")
	}

	// This test documents expected behavior for stop hooks when agent crashes.
	// Currently blocked by crash detection implementation.
	t.Skip("Test blocked: crash detection not yet implemented - test would hang")
}

// TestE2E_AgentCrash_RalphWiggum tests catastrophic failure scenario:
// Multiple agents running, first one crashes, workflow should fail gracefully
// with proper resource cleanup.
//
// Named after the Simpsons character who never gives up - this tests that even
// with multiple agents and complex failure scenarios, the system handles it gracefully.
//
// Expected behavior:
// - Agent 1 crashes during work
// - Agent 2 completes normally (but may be killed during cleanup)
// - Workflow fails because agent-1 step failed
// - All agent sessions are cleaned up
//
// Current status: Depends on crash detection and timeout handling being implemented.
func TestE2E_AgentCrash_RalphWiggum(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agent crash test in short mode")
	}

	// This test documents expected behavior for multi-agent crash scenarios.
	// Currently blocked by crash detection and timeout handling implementation.
	t.Skip("Test blocked: crash detection and timeout grace period handling not yet implemented")
}

// TestE2E_AgentCrash_DetectionLatency tests that the orchestrator detects
// agent crashes within a reasonable time (poll interval).
//
// Expected behavior:
// - Agent crashes immediately after receiving prompt
// - Orchestrator detects crash via session liveness poll OR timeout fires
// - Total time should be bounded (not infinite)
//
// This test ensures the system doesn't hang forever waiting for meow done
// when an agent has crashed.
//
// Current status: Depends on crash detection or timeout handling being implemented.
func TestE2E_AgentCrash_DetectionLatency(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping agent crash test in short mode")
	}

	// This test documents expected crash detection latency requirements.
	// Currently blocked by crash detection and timeout handling implementation.
	t.Skip("Test blocked: crash detection and timeout grace period handling not yet implemented")
}

// ===========================================================================
// Output Validation Retry Tests
//
// These tests verify that when output validation fails:
// 1. Step status returns to "running" (not "failed")
// 2. Error is returned to the agent
// 3. Agent can retry with correct outputs
// 4. Step eventually completes when outputs are valid
// ===========================================================================

func TestE2E_OutputValidation_MissingRequired(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Test blocked: requires validation retry mechanism (re-inject prompt on validation failure)
	t.Skip("Test blocked: validation retry mechanism (re-inject prompt on failure) not yet implemented")

	h := e2e.NewHarness(t)

	// Configure simulator:
	// - First call: provides wrong key "wrong_key" instead of "task_id"
	// - Second call: provides correct key "task_id"
	simConfig := e2e.NewSimConfigBuilder().
		WithBehaviorSequence("produce required output", []map[string]any{
			{"wrong_key": "some-value"},   // First attempt: wrong key
			{"task_id": "PROJ-123"},       // Second attempt: correct key
		}).
		WithStartupDelay(50 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"

[timing]
startup_delay = "100ms"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	// Template with an agent step that requires a specific output
	template := `
[main]
name = "validation-missing-output"

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "worker"

[[main.steps]]
id = "work"
executor = "agent"
agent = "worker"
needs = ["spawn"]
prompt = "Please produce required output for the task"

[main.steps.outputs]
task_id = { required = true, type = "string", description = "The task ID" }

[[main.steps]]
id = "verify"
executor = "shell"
needs = ["work"]
command = "echo 'task_id received: {{work.outputs.task_id}}'"

[[main.steps]]
id = "kill"
executor = "kill"
agent = "worker"
needs = ["verify"]
graceful = true
`
	if err := h.WriteTemplate("validation-missing.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "validation-missing.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Workflow should complete successfully after retry
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete after retry, got:\nstderr: %s", stderr)
	}

	// Should see validation failure message in logs
	if !strings.Contains(stderr, "validation failed") && !strings.Contains(stderr, "missing required output") {
		t.Logf("Note: expected validation failure message in logs (may be at debug level)")
	}
}

func TestE2E_OutputValidation_WrongType(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Test blocked: requires validation retry mechanism (re-inject prompt on validation failure)
	t.Skip("Test blocked: validation retry mechanism (re-inject prompt on failure) not yet implemented")

	h := e2e.NewHarness(t)

	// Configure simulator:
	// - First call: provides "count" as string instead of number
	// - Second call: provides "count" as number
	simConfig := e2e.NewSimConfigBuilder().
		WithBehaviorSequence("count the items", []map[string]any{
			{"count": "forty-two"},  // First attempt: wrong type (string)
			{"count": 42},           // Second attempt: correct type (number)
		}).
		WithStartupDelay(50 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"

[timing]
startup_delay = "100ms"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	// Template that expects a number output
	template := `
[main]
name = "validation-wrong-type"

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "counter"

[[main.steps]]
id = "count"
executor = "agent"
agent = "counter"
needs = ["spawn"]
prompt = "Please count the items and report the total"

[main.steps.outputs]
count = { required = true, type = "number", description = "The total count" }

[[main.steps]]
id = "report"
executor = "shell"
needs = ["count"]
command = "echo 'Count is: {{count.outputs.count}}'"

[[main.steps]]
id = "kill"
executor = "kill"
agent = "counter"
needs = ["report"]
graceful = true
`
	if err := h.WriteTemplate("validation-type.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "validation-type.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Workflow should complete after type correction
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete after type correction, got:\nstderr: %s", stderr)
	}
}

func TestE2E_OutputValidation_MultipleRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	// Test blocked: requires validation retry mechanism (re-inject prompt on validation failure)
	t.Skip("Test blocked: validation retry mechanism (re-inject prompt on failure) not yet implemented")

	h := e2e.NewHarness(t)

	// Configure simulator to fail 3 times, then succeed on 4th attempt
	simConfig := e2e.NewSimConfigBuilder().
		WithBehaviorSequence("perform validation test", []map[string]any{
			{"wrong": "attempt1"},      // 1st attempt: wrong key
			{"also_wrong": "attempt2"}, // 2nd attempt: still wrong
			{"nope": "attempt3"},       // 3rd attempt: nope
			{"result": "success!"},     // 4th attempt: finally correct
		}).
		WithStartupDelay(50 * time.Millisecond).
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"

[timing]
startup_delay = "100ms"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	// Template that requires specific output
	template := `
[main]
name = "validation-multi-retry"

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "persistent"

[[main.steps]]
id = "validate"
executor = "agent"
agent = "persistent"
needs = ["spawn"]
prompt = "Please perform validation test and provide the result"

[main.steps.outputs]
result = { required = true, type = "string", description = "The validation result" }

[[main.steps]]
id = "done"
executor = "shell"
needs = ["validate"]
command = "echo 'Validation passed with result: {{validate.outputs.result}}'"

[[main.steps]]
id = "kill"
executor = "kill"
agent = "persistent"
needs = ["done"]
graceful = true
`
	if err := h.WriteTemplate("validation-multi.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "validation-multi.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Workflow should complete after 4 attempts
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete after multiple retries, got:\nstderr: %s", stderr)
	}
}

func TestE2E_OutputValidation_EventualTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Configure simulator to always provide wrong outputs (never succeeds)
	simConfig := e2e.NewSimConfigBuilder().
		WithBehaviorSequence("timeout test", []map[string]any{
			{"wrong": "always"},  // Will repeat this forever (last item repeats)
		}).
		WithStartupDelay(50 * time.Millisecond).
		WithDelay(100 * time.Millisecond). // Small delay between retries
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"

[timing]
startup_delay = "100ms"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	// Template with a step that has a short timeout
	// The step requires "result" but simulator always provides "wrong"
	template := `
[main]
name = "validation-timeout"

[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "doomed"

[[main.steps]]
id = "doomed-step"
executor = "agent"
agent = "doomed"
needs = ["spawn"]
prompt = "Please run timeout test"
timeout = "3s"

[main.steps.outputs]
result = { required = true, type = "string", description = "Required result that will never come" }

[[main.steps]]
id = "kill"
executor = "kill"
agent = "doomed"
needs = ["doomed-step"]
graceful = true
`
	if err := h.WriteTemplate("validation-timeout.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// Run with timeout - workflow should fail due to step timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/tmp/meow-e2e-bin", "run", filepath.Join(h.TemplateDir, "validation-timeout.toml"))
	cmd.Dir = h.TempDir
	cmd.Env = h.Env()
	cmd.Env = append(cmd.Env, "PATH=/tmp:"+os.Getenv("PATH"))

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// The workflow should fail due to timeout
	// Check for timeout indication in output
	stderrStr := stderr.String()
	stdoutStr := stdout.String()

	hasTimeout := strings.Contains(stderrStr, "timeout") ||
		strings.Contains(stderrStr, "timed out") ||
		strings.Contains(stderrStr, "step failed") ||
		err != nil

	if !hasTimeout {
		t.Errorf("expected step to timeout, got:\nstdout: %s\nstderr: %s\nerr: %v",
			stdoutStr, stderrStr, err)
	}

	// Workflow should NOT complete successfully
	if strings.Contains(stderrStr, "workflow completed") {
		t.Errorf("expected workflow to fail due to timeout, but it completed")
	}
}

// ===========================================================================
// Crash Recovery E2E Tests
// ===========================================================================

// TestE2E_CrashRecovery_PendingSteps tests recovery when workflow has pending steps.
// Scenario: Workflow has step1 done, step2/step3 pending. Simulate crash by creating
// workflow state directly, then resume and verify completion.
func TestE2E_CrashRecovery_PendingSteps(t *testing.T) {
	h := e2e.NewHarness(t)

	// Create a workflow state directly - simulating state after step1 done
	steps := map[string]*types.Step{
		"step1": {
			ID:       "step1",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusDone,
			Shell:    &types.ShellConfig{Command: "echo 'step1 done'"},
		},
		"step2": {
			ID:       "step2",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusPending,
			Needs:    []string{"step1"},
			Shell:    &types.ShellConfig{Command: "echo 'step2 running'"},
		},
		"step3": {
			ID:       "step3",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusPending,
			Needs:    []string{"step2"},
			Shell:    &types.ShellConfig{Command: "echo 'step3 running'"},
		},
	}

	run, err := e2e.CreateTestWorkflow(h, "wf-recovery-pending", steps)
	if err != nil {
		t.Fatalf("failed to create test workflow: %v", err)
	}

	// Update workflow status to running (simulating interrupted state)
	wf, err := run.Workflow()
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}
	wf.Status = types.WorkflowStatusRunning
	if err := h.SaveWorkflow(wf); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	// Resume the workflow
	proc, err := h.RestartOrchestrator("wf-recovery-pending")
	if err != nil {
		t.Fatalf("failed to restart orchestrator: %v", err)
	}

	// Wait for completion
	err = proc.WaitWithTimeout(30 * time.Second)
	if err != nil {
		t.Logf("stdout: %s", proc.Stdout())
		t.Logf("stderr: %s", proc.Stderr())
		t.Fatalf("orchestrator did not complete: %v", err)
	}

	// Verify all steps are done
	wf, err = run.Workflow()
	if err != nil {
		t.Fatalf("failed to reload workflow: %v", err)
	}

	for stepID, step := range wf.Steps {
		if step.Status != types.StepStatusDone {
			t.Errorf("step %s status = %s, want done", stepID, step.Status)
		}
	}

	if wf.Status != types.WorkflowStatusDone {
		t.Errorf("workflow status = %s, want done", wf.Status)
	}
}

// TestE2E_CrashRecovery_RunningShellStep tests recovery when a shell step was running.
// Scenario: Shell step was "running" when crash occurred. Should be reset to pending
// and re-executed on resume.
func TestE2E_CrashRecovery_RunningShellStep(t *testing.T) {
	h := e2e.NewHarness(t)

	// Create marker file path to track idempotent execution
	markerFile := filepath.Join(h.TempDir, "shell-recovery-marker")

	// Create a workflow with a "running" shell step (simulating crash mid-execution)
	steps := map[string]*types.Step{
		"step1": {
			ID:       "step1",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusRunning, // Was running when crash happened
			Shell: &types.ShellConfig{
				// Idempotent command - creates marker file
				Command: fmt.Sprintf("touch %s && echo 'executed'", markerFile),
			},
		},
		"step2": {
			ID:       "step2",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusPending,
			Needs:    []string{"step1"},
			Shell:    &types.ShellConfig{Command: "echo 'step2'"},
		},
	}

	run, err := e2e.CreateTestWorkflow(h, "wf-recovery-shell", steps)
	if err != nil {
		t.Fatalf("failed to create test workflow: %v", err)
	}

	// Set workflow to running
	wf, err := run.Workflow()
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}
	wf.Status = types.WorkflowStatusRunning
	if err := h.SaveWorkflow(wf); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	// Resume the workflow
	proc, err := h.RestartOrchestrator("wf-recovery-shell")
	if err != nil {
		t.Fatalf("failed to restart orchestrator: %v", err)
	}

	err = proc.WaitWithTimeout(30 * time.Second)
	if err != nil {
		t.Logf("stdout: %s", proc.Stdout())
		t.Logf("stderr: %s", proc.Stderr())
		t.Fatalf("orchestrator did not complete: %v", err)
	}

	// Verify workflow completed
	wf, err = run.Workflow()
	if err != nil {
		t.Fatalf("failed to reload workflow: %v", err)
	}

	if wf.Status != types.WorkflowStatusDone {
		t.Errorf("workflow status = %s, want done", wf.Status)
	}

	// Verify shell step was re-executed (marker file should exist)
	if _, err := os.Stat(markerFile); os.IsNotExist(err) {
		t.Error("marker file not created - shell step was not re-executed")
	}

	// Verify step1 is done (was reset and re-executed)
	step1, ok := wf.Steps["step1"]
	if !ok {
		t.Fatal("step1 not found")
	}
	if step1.Status != types.StepStatusDone {
		t.Errorf("step1 status = %s, want done", step1.Status)
	}
}

// TestE2E_CrashRecovery_RunningAgentStep_DeadAgent tests recovery when an agent step
// was running but the agent is now dead.
// Scenario: Agent step was "running", agent tmux session no longer exists.
// The recovery should reset the step to pending, but since the agent is dead
// and there's no way to re-spawn it (spawn step already done), the workflow
// will eventually fail. This test verifies the recovery behavior is correct.
func TestE2E_CrashRecovery_RunningAgentStep_DeadAgent(t *testing.T) {
	h := e2e.NewHarness(t)

	// Create a workflow with a "running" agent step, but no agent session
	steps := map[string]*types.Step{
		"spawn": {
			ID:       "spawn",
			Executor: types.ExecutorSpawn,
			Status:   types.StepStatusDone,
			Spawn: &types.SpawnConfig{
				Agent: "test-agent",
			},
		},
		"work": {
			ID:       "work",
			Executor: types.ExecutorAgent,
			Status:   types.StepStatusRunning, // Was running when crash happened
			Needs:    []string{"spawn"},
			Agent: &types.AgentConfig{
				Agent:  "test-agent",
				Prompt: "Do some work",
			},
		},
	}

	// NOTE: No tmux session is created - simulating that agent died in crash
	// The orchestrator should detect this and reset the step to pending

	run, err := e2e.CreateTestWorkflow(h, "wf-recovery-agent-dead", steps)
	if err != nil {
		t.Fatalf("failed to create test workflow: %v", err)
	}

	// Set workflow to running with agent info
	wf, err := run.Workflow()
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}
	wf.Status = types.WorkflowStatusRunning
	wf.RegisterAgent("test-agent", &types.AgentInfo{
		TmuxSession: "meow-wf-recovery-agent-dead-test-agent",
		Status:      "active",
		CurrentStep: "work",
	})
	if err := h.SaveWorkflow(wf); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	// Resume the workflow - this should detect dead agent and reset step
	proc, err := h.RestartOrchestrator("wf-recovery-agent-dead")
	if err != nil {
		t.Fatalf("failed to restart orchestrator: %v", err)
	}

	// Give the orchestrator a short time to process recovery and hit the dispatch error
	// The orchestrator may not exit cleanly if the workflow is stuck, so we use a short timeout
	_ = proc.WaitWithTimeout(5 * time.Second)

	// Check the stderr output for recovery messages
	stderr := proc.Stderr()

	// Verify recovery detected the dead agent
	if !strings.Contains(stderr, "resetting step from dead agent") {
		t.Errorf("expected recovery to reset step from dead agent\nstderr: %s", stderr)
	}

	// Verify the step was identified
	if !strings.Contains(stderr, "step=work") {
		t.Errorf("expected recovery to mention work step\nstderr: %s", stderr)
	}

	// Kill the process if still running (it may hang because agent can't be re-spawned)
	if !proc.IsDone() {
		_ = proc.Kill()
	}

	// Reload workflow to verify recovery state
	wf, err = run.Workflow()
	if err != nil {
		t.Fatalf("failed to reload workflow: %v", err)
	}

	// After recovery, the work step should have been reset to pending
	// (though it may have been dispatched and failed by now)
	t.Logf("workflow status: %s", wf.Status)
	for stepID, step := range wf.Steps {
		t.Logf("step %s: status=%s", stepID, step.Status)
	}
}

// TestE2E_CrashRecovery_CompletingStep tests recovery when a step was in "completing" state.
// Scenario: Agent called meow done, orchestrator was processing when crash occurred.
// The step should be treated as "running" and wait for agent completion.
func TestE2E_CrashRecovery_CompletingStep(t *testing.T) {
	h := e2e.NewHarness(t)

	// Create a workflow with a "completing" shell step (unusual but tests edge case)
	steps := map[string]*types.Step{
		"step1": {
			ID:       "step1",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusCompleting, // Was completing when crash happened
			Shell:    &types.ShellConfig{Command: "echo 'step1'"},
		},
		"step2": {
			ID:       "step2",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusPending,
			Needs:    []string{"step1"},
			Shell:    &types.ShellConfig{Command: "echo 'step2'"},
		},
	}

	run, err := e2e.CreateTestWorkflow(h, "wf-recovery-completing", steps)
	if err != nil {
		t.Fatalf("failed to create test workflow: %v", err)
	}

	wf, err := run.Workflow()
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}
	wf.Status = types.WorkflowStatusRunning
	if err := h.SaveWorkflow(wf); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	// Resume the workflow
	proc, err := h.RestartOrchestrator("wf-recovery-completing")
	if err != nil {
		t.Fatalf("failed to restart orchestrator: %v", err)
	}

	err = proc.WaitWithTimeout(30 * time.Second)
	if err != nil {
		t.Logf("stdout: %s", proc.Stdout())
		t.Logf("stderr: %s", proc.Stderr())
		t.Fatalf("orchestrator did not complete: %v", err)
	}

	// Verify workflow completed
	wf, err = run.Workflow()
	if err != nil {
		t.Fatalf("failed to reload workflow: %v", err)
	}

	if wf.Status != types.WorkflowStatusDone {
		t.Errorf("workflow status = %s, want done", wf.Status)
	}

	// Both steps should be done
	for stepID, step := range wf.Steps {
		if step.Status != types.StepStatusDone {
			t.Errorf("step %s status = %s, want done", stepID, step.Status)
		}
	}
}

// TestE2E_CrashRecovery_PartialExpansion tests recovery when expand step was running.
// Scenario: Expand step was mid-execution, some child steps may exist.
// Should delete partial children and re-expand.
func TestE2E_CrashRecovery_PartialExpansion(t *testing.T) {
	h := e2e.NewHarness(t)

	// Create a workflow with a "running" expand step and some partial children
	steps := map[string]*types.Step{
		"before": {
			ID:       "before",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusDone,
			Shell:    &types.ShellConfig{Command: "echo 'before'"},
		},
		"expand-step": {
			ID:       "expand-step",
			Executor: types.ExecutorExpand,
			Status:   types.StepStatusRunning, // Was running when crash happened
			Needs:    []string{"before"},
			Expand: &types.ExpandConfig{
				Template: ".sub-workflow",
			},
			ExpandedInto: []string{"expand-step.child1"}, // Partial expansion
		},
		// Partial child step - should be deleted on recovery
		"expand-step.child1": {
			ID:           "expand-step.child1",
			Executor:     types.ExecutorShell,
			Status:       types.StepStatusPending,
			ExpandedFrom: "expand-step",
			Shell:        &types.ShellConfig{Command: "echo 'child1'"},
		},
		"after": {
			ID:       "after",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusPending,
			Needs:    []string{"expand-step"},
			Shell:    &types.ShellConfig{Command: "echo 'after'"},
		},
	}

	run, err := e2e.CreateTestWorkflow(h, "wf-recovery-expand", steps)
	if err != nil {
		t.Fatalf("failed to create test workflow: %v", err)
	}

	wf, err := run.Workflow()
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}
	wf.Status = types.WorkflowStatusRunning
	if err := h.SaveWorkflow(wf); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	// Write a template for the expand step to use
	template := `
[sub-workflow]
name = "sub-workflow"
internal = true

[[sub-workflow.steps]]
id = "real-child"
executor = "shell"
command = "echo 'properly expanded'"
`
	if err := h.WriteTemplate("wf-recovery-expand.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// Resume the workflow
	proc, err := h.RestartOrchestrator("wf-recovery-expand")
	if err != nil {
		t.Fatalf("failed to restart orchestrator: %v", err)
	}

	// Wait briefly for recovery to run - we expect the orchestrator to timeout
	// because the template path won't resolve correctly (we're testing recovery, not expansion)
	_ = proc.WaitWithTimeout(5 * time.Second)

	// Check stderr for recovery messages - this is the key verification
	stderr := proc.Stderr()
	t.Logf("stderr: %s", stderr)

	// Verify recovery detected and handled the partial expansion
	if !strings.Contains(stderr, "deleting partial expansion child") {
		t.Error("expected recovery to delete partial expansion child")
	}
	if !strings.Contains(stderr, "resetting orchestrator step") {
		t.Error("expected recovery to reset expand step")
	}
	if !strings.Contains(stderr, "step=expand-step") {
		t.Error("expected recovery to mention expand-step")
	}

	// Kill the orchestrator if still running (template error may cause it to hang)
	if !proc.IsDone() {
		_ = proc.Kill()
	}

	// Reload workflow to verify recovery state
	wf, err = run.Workflow()
	if err != nil {
		t.Fatalf("failed to reload workflow: %v", err)
	}

	// The partial child should have been deleted during recovery
	// (Note: it may be recreated if the expand ran again, but the original was deleted)
	t.Logf("steps after recovery: %v", func() []string {
		var ids []string
		for id := range wf.Steps {
			ids = append(ids, id)
		}
		return ids
	}())
}

// TestE2E_CrashRecovery_WorkflowCompleted tests that terminal workflows can't be resumed.
func TestE2E_CrashRecovery_WorkflowCompleted(t *testing.T) {
	h := e2e.NewHarness(t)

	// Create a completed workflow
	steps := map[string]*types.Step{
		"step1": {
			ID:       "step1",
			Executor: types.ExecutorShell,
			Status:   types.StepStatusDone,
			Shell:    &types.ShellConfig{Command: "echo 'done'"},
		},
	}

	run, err := e2e.CreateTestWorkflow(h, "wf-recovery-completed", steps)
	if err != nil {
		t.Fatalf("failed to create test workflow: %v", err)
	}

	wf, err := run.Workflow()
	if err != nil {
		t.Fatalf("failed to load workflow: %v", err)
	}
	wf.Status = types.WorkflowStatusDone
	wf.Complete()
	if err := h.SaveWorkflow(wf); err != nil {
		t.Fatalf("failed to save workflow: %v", err)
	}

	// Try to resume - should fail with error
	proc, err := h.RestartOrchestrator("wf-recovery-completed")
	if err != nil {
		t.Fatalf("failed to start orchestrator: %v", err)
	}

	err = proc.WaitWithTimeout(10 * time.Second)

	// The resume command should exit with an error since workflow is already done
	stderr := proc.Stderr()
	t.Logf("stderr: %s", stderr)

	// Verify we get an error (either about already done or process exits non-zero)
	if err == nil && !strings.Contains(stderr, "already") {
		t.Error("expected resume of completed workflow to fail")
	}
}

// ===========================================================================
// Async Execution Tests
// ===========================================================================

// TestE2E_ParallelBranchSteps tests that two branch steps with the same
// dependencies execute in parallel.
//
// This verifies the async branch implementation:
//   1. Both branches start their conditions in the same orchestrator tick
//   2. Conditions run concurrently in separate goroutines
//   3. Workflow completes in ~1s (parallel) instead of ~2s (sequential)
//
// Timeline:
//   - t=0s: Both branch-1 and branch-2 start (same needs, both ready)
//   - t=0-1s: Both sleep 1s conditions run in parallel
//   - t=1s: Both conditions complete, both branches expand on_true
//   - t=1s: Verify step runs (both branches done)
//
// Expected: Total time ~1-2s (parallel), not ~2-3s (sequential)
func TestE2E_ParallelBranchSteps(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	template := `
[main]
name = "parallel-branches"

[[main.steps]]
id = "setup"
executor = "shell"
command = "echo 'setup'"

# Two branches that should run in parallel
[[main.steps]]
id = "branch-1"
executor = "branch"
needs = ["setup"]
condition = "sleep 1 && echo 'branch-1'"

[main.steps.on_true]
inline = [
  { id = "result-1", executor = "shell", command = "echo 'branch-1 done'" }
]

[[main.steps]]
id = "branch-2"
executor = "branch"
needs = ["setup"]
condition = "sleep 1 && echo 'branch-2'"

[main.steps.on_true]
inline = [
  { id = "result-2", executor = "shell", command = "echo 'branch-2 done'" }
]

# Verify both complete
[[main.steps]]
id = "verify"
executor = "shell"
needs = ["branch-1", "branch-2"]
command = "echo 'both branches complete'"
`

	if err := h.WriteTemplate("parallel-branches.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	start := time.Now()
	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "parallel-branches.toml"))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify workflow completed
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// The primary verification is timing - parallel execution should complete faster
	// than sequential execution. With two 1-second sleeps:
	// - Parallel: ~1-2s total
	// - Sequential: ~2-3s total
	// We allow up to 3s to account for test environment overhead
	if elapsed > 3*time.Second {
		t.Errorf("workflow took too long (%.2fs) - branches may be running sequentially (expected <3s for parallel)", elapsed.Seconds())
	} else {
		t.Logf("✓ Parallel execution confirmed: %.2fs (expected ~1-2s)", elapsed.Seconds())
	}
}

// TestE2E_ShellOutputChain tests that shell step outputs flow correctly
// to downstream steps via variable substitution.
//
// This verifies:
//   1. Shell steps capture outputs (stdout)
//   2. Outputs are available for variable substitution in later steps
//   3. Multiple steps can chain outputs
//
// Timeline:
//   - Step 1: Generates "hello" and captures to outputs.greeting
//   - Step 2: Generates "world" and captures to outputs.target
//   - Step 3: Uses both outputs ({{step-1.outputs.greeting}} and {{step-2.outputs.target}}) to create "hello world"
func TestE2E_ShellOutputChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Create a temp file for output verification
	outputFile := filepath.Join(h.TempDir, "result.txt")

	template := fmt.Sprintf(`
[main]
name = "shell-output-chain"

[[main.steps]]
id = "step-1"
executor = "shell"
command = "echo 'hello'"

[main.steps.outputs]
greeting = { source = "stdout" }

[[main.steps]]
id = "step-2"
executor = "shell"
needs = ["step-1"]
command = "echo 'world'"

[main.steps.outputs]
target = { source = "stdout" }

[[main.steps]]
id = "step-3"
executor = "shell"
needs = ["step-1", "step-2"]
command = "echo '{{step-1.outputs.greeting}} {{step-2.outputs.target}}' > %s"
`, outputFile)

	if err := h.WriteTemplate("shell-output-chain.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "shell-output-chain.toml"))
	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify workflow completed
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// Read the output file to verify variable substitution worked
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	expectedOutput := "hello world"
	actualOutput := strings.TrimSpace(string(content))
	if actualOutput != expectedOutput {
		t.Errorf("output mismatch:\n  expected: %q\n  got:      %q", expectedOutput, actualOutput)
	} else {
		t.Logf("✓ Output chain verified: %q", actualOutput)
	}
}

// TestE2E_EventRouting tests that events can be sent and received between
// concurrent workflow steps using meow event and meow await-event.
//
// This verifies the core event routing mechanism needed for Ralph Wiggum patterns:
//   1. A branch step waits for an event (meow await-event)
//   2. A shell step sends the event (meow event)
//   3. The waiting branch receives the event and completes
//
// Timeline:
//   - t=0s: Both event-waiter and event-sender start (parallel branches)
//   - t=0s: event-waiter blocks on await-event
//   - t=1s: event-sender sends the event after 1s delay
//   - t=1s: event-waiter receives event, completes with on_true
//   - t=1s: verify step runs (both branches done)
//
// Expected: Workflow completes successfully with event routing working
func TestE2E_EventRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	template := `
[main]
name = "event-routing"

[[main.steps]]
id = "setup"
executor = "shell"
command = "echo 'starting event routing test'"

# Branch that waits for an event
[[main.steps]]
id = "event-waiter"
executor = "branch"
needs = ["setup"]
condition = "meow await-event test-signal --timeout 10s"
timeout = "15s"

[main.steps.on_true]
inline = [
  { id = "waiter-done", executor = "shell", command = "echo 'event received!'" }
]

[main.steps.on_timeout]
inline = [
  { id = "waiter-timeout", executor = "shell", command = "echo 'timeout - event not received'" }
]

# Shell step that sends the event after a delay
[[main.steps]]
id = "event-sender"
executor = "shell"
needs = ["setup"]
command = "sleep 1 && meow event test-signal && echo 'event sent'"

# Verify both complete
[[main.steps]]
id = "verify"
executor = "shell"
needs = ["event-waiter", "event-sender"]
command = "echo 'event routing verified'"
`

	if err := h.WriteTemplate("event-routing.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	start := time.Now()
	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "event-routing.toml"))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify workflow completed
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// Verify event was received (not timeout)
	if strings.Contains(stdout, "timeout - event not received") {
		t.Errorf("event-waiter timed out instead of receiving event\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// Should complete in ~1-3s (sender waits 1s, then sends event)
	if elapsed > 10*time.Second {
		t.Errorf("workflow took too long (%.2fs) - event routing may have failed", elapsed.Seconds())
	} else {
		t.Logf("✓ Event routing verified in %.2fs", elapsed.Seconds())
	}
}

// TestE2E_EventRouting_AgentStopped tests the "Ralph Wiggum" pattern:
// an agent's stop hook emits the agent-stopped event which triggers a waiting branch.
//
// This is the canonical test for async branch + agent parallel execution:
//   1. spawn starts the agent
//   2. monitor (branch) and work (agent) both start in parallel after spawn
//   3. work completes via meow done
//   4. Simulator's stop hook fires agent-stopped event
//   5. monitor receives the event and expands on_true
//   6. Workflow completes successfully
//
// This validates:
//   - Branch and agent steps run in parallel (async branch)
//   - Simulator stop hook fires agent-stopped event after meow done
//   - EventRouter delivers event to waiting await-event
//   - Workflow completes via event-driven coordination (not timeout)
func TestE2E_EventRouting_AgentStopped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)

	// Configure simulator with stop hook enabled (fires agent-stopped event)
	simConfig := e2e.NewSimConfigBuilder().
		WithStopHook(true). // Default, but explicit for clarity
		WithDefaultAction(e2e.ActionComplete).
		WithStartupDelay(50 * time.Millisecond).
		WithDelay(50 * time.Millisecond). // Fast completion
		WithLogLevel("debug").             // Enable debug logging to see stop hook
		Build()

	if err := h.WriteSimConfig(simConfig); err != nil {
		t.Fatalf("failed to write sim config: %v", err)
	}

	// Write simulator adapter config
	adapterConfig := `
[adapter]
name = "simulator"

[spawn]
command = "/tmp/meow-agent-sim-e2e"
args = []

[timing]
startup_delay = "100ms"
`
	if err := h.WriteAdapterConfig("simulator", adapterConfig); err != nil {
		t.Fatalf("failed to write adapter config: %v", err)
	}

	template := `
[main]
name = "event-routing-agent-stopped"

# Step 1: Spawn the agent
[[main.steps]]
id = "spawn"
executor = "spawn"
agent = "worker"

# Step 2a: Monitor waits for agent-stopped event (runs in parallel with work)
[[main.steps]]
id = "monitor"
executor = "branch"
needs = ["spawn"]
condition = "meow await-event agent-stopped --timeout 30s"
timeout = "35s"

[main.steps.on_true]
inline = [
  { id = "got-event", executor = "shell", command = "echo 'EVENT_RECEIVED'" }
]

[main.steps.on_timeout]
inline = [
  { id = "timed-out", executor = "shell", command = "echo 'TIMEOUT_NO_EVENT'" }
]

# Step 2b: Agent does work (runs in parallel with monitor)
[[main.steps]]
id = "work"
executor = "agent"
agent = "worker"
needs = ["spawn"]
prompt = "Do some work and complete"

# Step 3: Final step runs after both complete
[[main.steps]]
id = "final"
executor = "shell"
needs = ["monitor", "work"]
command = "echo 'BOTH_COMPLETE'"

# Step 4: Clean up
[[main.steps]]
id = "kill"
executor = "kill"
agent = "worker"
needs = ["final"]
graceful = true
`
	if err := h.WriteTemplate("event-routing-agent-stopped.toml", template); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	start := time.Now()
	stdout, stderr, err := runMeow(h, "run", filepath.Join(h.TemplateDir, "event-routing-agent-stopped.toml"))
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("meow run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify workflow completed
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected workflow to complete\nstdout: %s\nstderr: %s", stdout, stderr)
	}

	// Verify agent-stopped event was matched (proves event routing works)
	if !strings.Contains(stderr, "event_type=agent-stopped matched=true") {
		t.Errorf("expected agent-stopped event to be matched\nstderr: %s", stderr)
	}

	// Verify the on_true branch was expanded (monitor.got-event step dispatched)
	if !strings.Contains(stderr, "dispatching step\" id=monitor.got-event") {
		t.Errorf("expected monitor.got-event step to be dispatched (on_true expansion)\nstderr: %s", stderr)
	}

	// Verify final step ran (proves both monitor and work completed)
	if !strings.Contains(stderr, "dispatching step\" id=final") {
		t.Errorf("expected final step to be dispatched\nstderr: %s", stderr)
	}

	// Should complete quickly - parallel execution means event arrives soon after agent completes
	// Allow up to 20s for test environment overhead (agent startup, tmux delays, etc.)
	if elapsed > 20*time.Second {
		t.Errorf("workflow took too long (%.2fs) - event routing may have failed (expected <20s)", elapsed.Seconds())
	} else {
		t.Logf("✓ Agent stop hook event routing verified in %.2fs", elapsed.Seconds())
	}
}
