package e2e

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"
)

// syncBuffer is a thread-safe wrapper around bytes.Buffer.
type syncBuffer struct {
	mu  sync.RWMutex
	buf bytes.Buffer
}

// Write implements io.Writer with mutex protection.
func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns the buffer contents as a string.
func (b *syncBuffer) String() string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.buf.String()
}

// OrchestratorProcess represents a running orchestrator process.
// It provides methods to control and observe the orchestrator for crash testing.
type OrchestratorProcess struct {
	cmd    *exec.Cmd
	pid    int
	stdout *syncBuffer
	stderr *syncBuffer

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

	stdout := &syncBuffer{}
	stderr := &syncBuffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting orchestrator: %w", err)
	}

	proc := &OrchestratorProcess{
		cmd:     cmd,
		pid:     cmd.Process.Pid,
		stdout:  stdout,
		stderr:  stderr,
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
	// Use the resume command to continue the workflow
	return h.StartOrchestrator("resume", workflowID)
}

// findMeowBinary finds the meow binary to use.
// Tries in order: MEOW_BIN env var, E2E test binary, local build.
func (h *Harness) findMeowBinary() (string, error) {
	// Check env var first
	if bin := os.Getenv("MEOW_BIN"); bin != "" {
		if _, err := os.Stat(bin); err == nil {
			return bin, nil
		}
	}

	// Try to find in common locations
	// E2E tests build to /tmp/meow-e2e-bin (see TestMain in e2e_test.go)
	locations := []string{
		"/tmp/meow-e2e-bin", // E2E test binary location
		"./meow",
		"./bin/meow",
		"../../cmd/meow/meow",
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return loc, nil
		}
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
