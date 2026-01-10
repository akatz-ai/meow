package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"
)

// OrchestratorProcess represents a running orchestrator process.
// It provides methods to control and observe the orchestrator for crash testing.
type OrchestratorProcess struct {
	cmd    *exec.Cmd
	pid    int
	stdout *bytes.Buffer
	stderr *bytes.Buffer

	// done receives the exit error when the process completes.
	done chan error
	// exited is closed when the process exits, for non-blocking checks.
	exited chan struct{}
	// exitErr stores the error from cmd.Wait() for multiple reads.
	exitErr error

	// harness is the test harness that created this process.
	harness *Harness

	// workflowID is extracted from stderr after startup.
	workflowID string
}

// Kill forcefully terminates the orchestrator process (SIGKILL).
func (p *OrchestratorProcess) Kill() error {
	if p.cmd == nil || p.cmd.Process == nil {
		return fmt.Errorf("process not running")
	}
	return p.cmd.Process.Kill()
}

// Signal sends a signal to the orchestrator process.
func (p *OrchestratorProcess) Signal(sig os.Signal) error {
	if p.cmd == nil || p.cmd.Process == nil {
		return fmt.Errorf("process not running")
	}
	return p.cmd.Process.Signal(sig)
}

// Wait blocks until the process exits and returns the exit error.
func (p *OrchestratorProcess) Wait() error {
	<-p.exited
	return p.exitErr
}

// WaitWithTimeout waits for the process to exit with a timeout.
func (p *OrchestratorProcess) WaitWithTimeout(timeout time.Duration) error {
	select {
	case <-p.exited:
		return p.exitErr
	case <-time.After(timeout):
		return fmt.Errorf("timeout waiting for process to exit")
	}
}

// IsDone returns true if the process has exited.
func (p *OrchestratorProcess) IsDone() bool {
	select {
	case <-p.exited:
		return true
	default:
		return false
	}
}

// PID returns the process ID.
func (p *OrchestratorProcess) PID() int {
	return p.pid
}

// Stdout returns the captured stdout output.
func (p *OrchestratorProcess) Stdout() string {
	return p.stdout.String()
}

// Stderr returns the captured stderr output.
func (p *OrchestratorProcess) Stderr() string {
	return p.stderr.String()
}

// WorkflowID returns the workflow ID extracted from the process output.
// This may be empty if the workflow hasn't started yet.
func (p *OrchestratorProcess) WorkflowID() string {
	return p.workflowID
}

// ExtractWorkflowID attempts to extract the workflow ID from stderr.
// Call this after the workflow has started.
func (p *OrchestratorProcess) ExtractWorkflowID() string {
	// Look for pattern like "workflow_id=wf-xxx" or "Workflow: wf-xxx"
	patterns := []string{
		`workflow[_-]?id[=: ]+([a-zA-Z0-9_-]+)`,
		`Workflow: ([a-zA-Z0-9_-]+)`,
		`Starting workflow ([a-zA-Z0-9_-]+)`,
		`wf-[a-zA-Z0-9]+`,
	}

	output := p.stderr.String()
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			p.workflowID = matches[1]
			return p.workflowID
		} else if len(matches) == 1 {
			// Direct match without capture group
			p.workflowID = matches[0]
			return p.workflowID
		}
	}

	return ""
}

// StartOrchestrator starts the meow orchestrator in the background.
// The process runs until completion, crash, or explicit kill.
func (h *Harness) StartOrchestrator(args ...string) (*OrchestratorProcess, error) {
	// Find the meow binary
	meowBin, err := h.findMeowBinary()
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, meowBin, args...)
	cmd.Dir = h.TempDir
	cmd.Env = h.Env()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting orchestrator: %w", err)
	}

	proc := &OrchestratorProcess{
		cmd:     cmd,
		pid:     cmd.Process.Pid,
		stdout:  &stdout,
		stderr:  &stderr,
		done:    make(chan error, 1),
		exited:  make(chan struct{}),
		harness: h,
	}

	// Wait for process in background
	go func() {
		err := cmd.Wait()
		proc.exitErr = err
		proc.done <- err
		close(proc.exited)
	}()

	// Register cleanup
	h.OnCleanup(func() {
		if !proc.IsDone() {
			_ = proc.Kill()
		}
	})

	return proc, nil
}

// RestartOrchestrator restarts the orchestrator for a specific workflow.
// The workflow state should already exist from a previous run.
func (h *Harness) RestartOrchestrator(workflowID string) (*OrchestratorProcess, error) {
	// The orchestrator should detect and resume the workflow
	return h.StartOrchestrator("run", "--resume", workflowID)
}

// findMeowBinary finds the meow binary to use.
// Tries in order: MEOW_BIN env var, go run, local build.
func (h *Harness) findMeowBinary() (string, error) {
	// Check env var first
	if bin := os.Getenv("MEOW_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin, nil
		}
	}

	// Try to find in common locations
	locations := []string{
		"./meow",
		"./bin/meow",
		"../../cmd/meow/meow",
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc, nil
		}
	}

	// Fall back to go run (slower but always works)
	// We'll use exec.LookPath to see if go is available
	if _, err := exec.LookPath("go"); err == nil {
		return "go", nil
	}

	return "", fmt.Errorf("meow binary not found: set MEOW_BIN or build with 'go build ./cmd/meow'")
}

// StartOrchestratorWithTemplate starts the orchestrator with a template file.
// This is a convenience method that sets up the template and runs it.
func (h *Harness) StartOrchestratorWithTemplate(templateName, templateContent string) (*OrchestratorProcess, error) {
	// Write template
	if err := h.WriteTemplate(templateName, templateContent); err != nil {
		return nil, fmt.Errorf("writing template: %w", err)
	}

	// Start orchestrator
	return h.StartOrchestrator("run", templateName)
}
