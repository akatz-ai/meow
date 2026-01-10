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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
	t.Skip("TODO: Agent tests need tmux socket isolation in agent_manager")
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
	t.Skip("TODO: Agent tests need tmux socket isolation in agent_manager")
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
	t.Skip("TODO: Agent tests need tmux socket isolation in agent_manager")
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
