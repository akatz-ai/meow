package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/meow-stack/meow-machine/internal/config"
	"github.com/meow-stack/meow-machine/internal/types"
)

// Harness provides test isolation for E2E tests.
// Each harness creates an isolated environment with its own:
// - Temporary directory for workflow state
// - Tmux socket for session isolation
// - Simulator configuration
type Harness struct {
	// TempDir is the root temporary directory for this test.
	TempDir string

	// TmuxSocket is the path to the isolated tmux socket.
	TmuxSocket string

	// RunsDir is where run state files are stored.
	RunsDir string

	// LogsDir is where per-run log files are stored.
	LogsDir string

	// TemplateDir is where workflow templates are stored.
	TemplateDir string

	// AdapterDir is where adapter configs are stored.
	AdapterDir string

	// SimConfigPath is the path to the simulator config file.
	SimConfigPath string

	// Config is the test configuration.
	Config *config.Config

	// t is the test context for logging and cleanup.
	t *testing.T

	// cleanupFuncs are called on Cleanup().
	cleanupFuncs []func()
}

// NewHarness creates a new test harness with isolated directories.
func NewHarness(t *testing.T) *Harness {
	t.Helper()

	tempDir := t.TempDir()
	tmuxSocket := filepath.Join(tempDir, "tmux.sock")

	h := &Harness{
		TempDir:       tempDir,
		TmuxSocket:    tmuxSocket,
		RunsDir:       filepath.Join(tempDir, ".meow", "runs"),
		LogsDir:       filepath.Join(tempDir, ".meow", "logs"),
		TemplateDir:   filepath.Join(tempDir, ".meow", "workflows"),
		AdapterDir:    filepath.Join(tempDir, ".meow", "adapters"),
		SimConfigPath: filepath.Join(tempDir, "sim-config.yaml"),
		t:             t,
	}

	// Create directory structure
	dirs := []string{
		h.RunsDir,
		h.LogsDir,
		h.TemplateDir,
		h.AdapterDir,
		filepath.Join(h.AdapterDir, "claude"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
		}
	}

	// Write a "claude" adapter that uses the simulator binary
	simulatorAdapter := `# Test adapter - uses simulator instead of real Claude
[adapter]
name = "claude"
description = "Simulator for E2E testing"

[spawn]
command = "/tmp/meow-agent-sim-e2e"
resume_command = "/tmp/meow-agent-sim-e2e --resume {{session_id}}"
startup_delay = "200ms"

[environment]
TMUX = ""

[prompt_injection]
pre_keys = ["Escape"]
pre_delay = "50ms"
method = "literal"
post_keys = ["Enter"]
post_delay = "10ms"

[graceful_stop]
keys = ["C-c"]
wait = "200ms"
`
	adapterPath := filepath.Join(h.AdapterDir, "claude", "adapter.toml")
	if err := os.WriteFile(adapterPath, []byte(simulatorAdapter), 0644); err != nil {
		t.Fatalf("failed to write simulator adapter: %v", err)
	}

	// Write a minimal config with default adapter for CLI runs
	configContent := `version = "1"

[paths]
workflow_dir = ".meow/workflows"
runs_dir = ".meow/runs"
logs_dir = ".meow/logs"

[agent]
default_adapter = "claude"
`
	configPath := filepath.Join(tempDir, ".meow", "config.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Create default config
	h.Config = h.createConfig()

	// Register cleanup with t.Cleanup
	t.Cleanup(h.Cleanup)

	return h
}

// createConfig creates a test configuration.
func (h *Harness) createConfig() *config.Config {
	cfg := config.Default()
	cfg.Paths.WorkflowDir = h.TemplateDir
	cfg.Paths.RunsDir = h.RunsDir
	cfg.Paths.LogsDir = h.LogsDir
	cfg.Agent.DefaultAdapter = "claude"
	cfg.Logging.Level = config.LogLevelDebug
	return cfg
}

// Cleanup releases resources. Called automatically via t.Cleanup.
func (h *Harness) Cleanup() {
	// Run cleanup functions in reverse order
	for i := len(h.cleanupFuncs) - 1; i >= 0; i-- {
		h.cleanupFuncs[i]()
	}

	// Kill any tmux sessions using our socket
	h.killTmuxSessions()
}

// OnCleanup registers a function to be called during cleanup.
func (h *Harness) OnCleanup(fn func()) {
	h.cleanupFuncs = append(h.cleanupFuncs, fn)
}

// killTmuxSessions kills all tmux sessions on our isolated socket.
func (h *Harness) killTmuxSessions() {
	if _, err := os.Stat(h.TmuxSocket); os.IsNotExist(err) {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tmux", "-S", h.TmuxSocket, "kill-server")
	_ = cmd.Run() // Ignore errors - server may not exist
}

// WriteSimConfig writes a simulator configuration to the harness.
func (h *Harness) WriteSimConfig(cfg SimTestConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal sim config: %w", err)
	}
	if err := os.WriteFile(h.SimConfigPath, data, 0644); err != nil {
		return fmt.Errorf("write sim config: %w", err)
	}
	return nil
}

// WriteTemplate writes a workflow template to the harness.
func (h *Harness) WriteTemplate(name, content string) error {
	path := filepath.Join(h.TemplateDir, name)
	if !strings.HasSuffix(path, ".toml") {
		path += ".toml"
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// WriteAdapterConfig writes an adapter configuration.
func (h *Harness) WriteAdapterConfig(name, content string) error {
	dir := filepath.Join(h.AdapterDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "adapter.toml"), []byte(content), 0644)
}

// Env returns environment variables for subprocess execution.
// The adapter system handles agent spawning - we just need to ensure
// the project adapter directory is used (contains simulator override).
func (h *Harness) Env() []string {
	env := os.Environ()
	env = append(env,
		fmt.Sprintf("MEOW_RUNS_DIR=%s", h.RunsDir),
		fmt.Sprintf("MEOW_WORKFLOW_DIR=%s", h.TemplateDir),
		fmt.Sprintf("MEOW_ADAPTER_DIR=%s", h.AdapterDir),
		fmt.Sprintf("MEOW_SIM_CONFIG=%s", h.SimConfigPath),
		fmt.Sprintf("TMUX_TMPDIR=%s", h.TempDir),
		fmt.Sprintf("MEOW_TMUX_SOCKET=%s", h.TmuxSocket),
	)
	return env
}

// RunWorkflow starts a workflow and returns a WorkflowRun for observation.
// This is a placeholder that will be implemented when the orchestrator
// integration is complete.
func (h *Harness) RunWorkflow(templatePath string) (*WorkflowRun, error) {
	// TODO: Integrate with orchestrator.Run
	// For now, return a stub that can be filled in later
	return &WorkflowRun{
		ID:        fmt.Sprintf("run-%d", time.Now().UnixNano()),
		harness:   h,
		startTime: time.Now(),
	}, nil
}

// LoadWorkflow loads an existing workflow from state.
func (h *Harness) LoadWorkflow(id string) (*types.Run, error) {
	path := filepath.Join(h.RunsDir, id+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read workflow: %w", err)
	}
	var wf types.Run
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("unmarshal workflow: %w", err)
	}
	return &wf, nil
}

// SaveWorkflow saves a workflow to state.
func (h *Harness) SaveWorkflow(wf *types.Run) error {
	if err := os.MkdirAll(h.RunsDir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(wf)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(h.RunsDir, wf.ID+".yaml"), data, 0644)
}

// TmuxNewSession creates a new tmux session using the harness socket.
func (h *Harness) TmuxNewSession(name string) error {
	cmd := exec.Command("tmux", "-S", h.TmuxSocket, "new-session", "-d", "-s", name)
	cmd.Env = h.Env()
	return cmd.Run()
}

// TmuxKillSession kills a tmux session.
func (h *Harness) TmuxKillSession(name string) error {
	cmd := exec.Command("tmux", "-S", h.TmuxSocket, "kill-session", "-t", name)
	return cmd.Run()
}

// TmuxSessionExists checks if a tmux session exists.
func (h *Harness) TmuxSessionExists(name string) bool {
	cmd := exec.Command("tmux", "-S", h.TmuxSocket, "has-session", "-t", name)
	return cmd.Run() == nil
}

// agentSessionName returns the tmux session name for an agent.
// Uses the MEOW naming convention: meow-{workflowID}-{agentID}
// For tests without a workflow ID, just uses meow-{agentID}
func (h *Harness) agentSessionName(agentID string) string {
	return fmt.Sprintf("meow-%s", agentID)
}

// KillAgentSession terminates an agent's tmux session.
func (h *Harness) KillAgentSession(agentID string) error {
	sessionName := h.agentSessionName(agentID)
	return h.TmuxKillSession(sessionName)
}

// IsAgentSessionAlive checks if an agent's tmux session exists.
func (h *Harness) IsAgentSessionAlive(agentID string) bool {
	sessionName := h.agentSessionName(agentID)
	return h.TmuxSessionExists(sessionName)
}

// ListAgentSessions returns all agent session IDs on the test socket.
// Only returns sessions that follow the meow-{agentID} naming convention.
func (h *Harness) ListAgentSessions() ([]string, error) {
	cmd := exec.Command("tmux", "-S", h.TmuxSocket, "list-sessions", "-F", "#{session_name}")
	out, err := cmd.Output()
	if err != nil {
		// No sessions = empty list, not an error
		return nil, nil
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var agents []string
	for _, line := range lines {
		if strings.HasPrefix(line, "meow-") {
			agentID := strings.TrimPrefix(line, "meow-")
			agents = append(agents, agentID)
		}
	}
	return agents, nil
}
