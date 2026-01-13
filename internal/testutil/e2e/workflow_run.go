package e2e

import (
	"context"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/meow-stack/meow-machine/internal/types"
)

// WorkflowRun represents a running workflow for testing.
// It provides methods to wait for state changes and inspect outputs.
type WorkflowRun struct {
	// ID is the workflow identifier.
	ID string

	// harness is the test harness that created this run.
	harness *Harness

	// startTime records when the workflow started.
	startTime time.Time

	// cancel can be called to stop the workflow.
	cancel context.CancelFunc
}

// WaitForStep waits for a step to reach the given status.
// Returns an error if the timeout expires before the step reaches the status.
func (r *WorkflowRun) WaitForStep(stepID string, status string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	targetStatus := types.StepStatus(status)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for step %s to reach status %s", stepID, status)
		case <-ticker.C:
			wf, err := r.loadWorkflow()
			if err != nil {
				continue // Workflow file may not exist yet
			}
			step, ok := wf.GetStep(stepID)
			if !ok {
				continue // Step may not exist yet
			}
			if step.Status == targetStatus {
				return nil
			}
			// If step is in a terminal state that's not what we want, fail fast
			if step.Status.IsTerminal() && step.Status != targetStatus {
				return fmt.Errorf("step %s reached terminal status %s instead of %s",
					stepID, step.Status, status)
			}
		}
	}
}

// WaitForDone waits for the workflow to complete (done or failed).
func (r *WorkflowRun) WaitForDone(timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for workflow %s to complete", r.ID)
		case <-ticker.C:
			wf, err := r.loadWorkflow()
			if err != nil {
				continue
			}
			if wf.Status.IsTerminal() {
				return nil
			}
		}
	}
}

// WaitForStatus waits for the workflow to reach a specific status.
func (r *WorkflowRun) WaitForStatus(status types.RunStatus, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for workflow %s to reach status %s", r.ID, status)
		case <-ticker.C:
			wf, err := r.loadWorkflow()
			if err != nil {
				continue
			}
			if wf.Status == status {
				return nil
			}
			// Fail fast if workflow is in a terminal state that's not what we want
			if wf.Status.IsTerminal() && wf.Status != status {
				return fmt.Errorf("workflow %s reached terminal status %s instead of %s",
					r.ID, wf.Status, status)
			}
		}
	}
}

// Status returns the current workflow status.
func (r *WorkflowRun) Status() string {
	wf, err := r.loadWorkflow()
	if err != nil {
		return "unknown"
	}
	return string(wf.Status)
}

// StepStatus returns the status of a specific step.
func (r *WorkflowRun) StepStatus(stepID string) (string, error) {
	wf, err := r.loadWorkflow()
	if err != nil {
		return "", err
	}
	step, ok := wf.GetStep(stepID)
	if !ok {
		return "", fmt.Errorf("step %s not found", stepID)
	}
	return string(step.Status), nil
}

// StepOutput retrieves a specific output from a step.
func (r *WorkflowRun) StepOutput(stepID, key string) (any, error) {
	wf, err := r.loadWorkflow()
	if err != nil {
		return nil, err
	}
	step, ok := wf.GetStep(stepID)
	if !ok {
		return nil, fmt.Errorf("step %s not found", stepID)
	}
	if step.Outputs == nil {
		return nil, fmt.Errorf("step %s has no outputs", stepID)
	}
	val, ok := step.Outputs[key]
	if !ok {
		return nil, fmt.Errorf("output %s not found in step %s", key, stepID)
	}
	return val, nil
}

// StepOutputs returns all outputs from a step.
func (r *WorkflowRun) StepOutputs(stepID string) (map[string]any, error) {
	wf, err := r.loadWorkflow()
	if err != nil {
		return nil, err
	}
	step, ok := wf.GetStep(stepID)
	if !ok {
		return nil, fmt.Errorf("step %s not found", stepID)
	}
	return step.Outputs, nil
}

// StepError returns the error from a failed step.
func (r *WorkflowRun) StepError(stepID string) (*types.StepError, error) {
	wf, err := r.loadWorkflow()
	if err != nil {
		return nil, err
	}
	step, ok := wf.GetStep(stepID)
	if !ok {
		return nil, fmt.Errorf("step %s not found", stepID)
	}
	return step.Error, nil
}

// Workflow returns the current workflow state.
func (r *WorkflowRun) Workflow() (*types.Run, error) {
	return r.loadWorkflow()
}

// loadWorkflow loads the workflow from state.
func (r *WorkflowRun) loadWorkflow() (*types.Run, error) {
	return r.harness.LoadWorkflow(r.ID)
}

// Duration returns how long the workflow has been running.
func (r *WorkflowRun) Duration() time.Duration {
	return time.Since(r.startTime)
}

// Cancel stops the workflow execution.
func (r *WorkflowRun) Cancel() {
	if r.cancel != nil {
		r.cancel()
	}
}

// AssertStepDone asserts that a step is in done status.
// Returns an error if the step is not done or doesn't exist.
func (r *WorkflowRun) AssertStepDone(stepID string) error {
	status, err := r.StepStatus(stepID)
	if err != nil {
		return err
	}
	if status != string(types.StepStatusDone) {
		return fmt.Errorf("step %s is %s, expected done", stepID, status)
	}
	return nil
}

// AssertStepFailed asserts that a step is in failed status.
func (r *WorkflowRun) AssertStepFailed(stepID string) error {
	status, err := r.StepStatus(stepID)
	if err != nil {
		return err
	}
	if status != string(types.StepStatusFailed) {
		return fmt.Errorf("step %s is %s, expected failed", stepID, status)
	}
	return nil
}

// AssertWorkflowDone asserts that the workflow completed successfully.
func (r *WorkflowRun) AssertWorkflowDone() error {
	status := r.Status()
	if status != string(types.RunStatusDone) {
		return fmt.Errorf("workflow is %s, expected done", status)
	}
	return nil
}

// AssertWorkflowFailed asserts that the workflow failed.
func (r *WorkflowRun) AssertWorkflowFailed() error {
	status := r.Status()
	if status != string(types.RunStatusFailed) {
		return fmt.Errorf("workflow is %s, expected failed", status)
	}
	return nil
}

// WorkflowRunFromID creates a WorkflowRun from an existing workflow ID.
// Useful for attaching to workflows started by other means.
func WorkflowRunFromID(h *Harness, id string) *WorkflowRun {
	return &WorkflowRun{
		ID:        id,
		harness:   h,
		startTime: time.Now(),
	}
}

// WorkflowRunFromFile loads a workflow state file and creates a WorkflowRun.
func WorkflowRunFromFile(h *Harness, path string) (*WorkflowRun, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wf types.Run
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}

	// Also save to harness state dir so subsequent loads work
	if err := h.SaveWorkflow(&wf); err != nil {
		return nil, err
	}

	return &WorkflowRun{
		ID:        wf.ID,
		harness:   h,
		startTime: wf.StartedAt,
	}, nil
}

// CreateTestWorkflow creates a workflow with the given steps for testing.
// This is useful for tests that need to set up specific workflow state.
func CreateTestWorkflow(h *Harness, id string, steps map[string]*types.Step) (*WorkflowRun, error) {
	wf := types.NewRun(id, "test-template", nil)
	for stepID, step := range steps {
		step.ID = stepID
		if err := wf.AddStep(step); err != nil {
			return nil, fmt.Errorf("add step %s: %w", stepID, err)
		}
	}

	// Ensure runs directory exists
	if err := os.MkdirAll(h.RunsDir, 0755); err != nil {
		return nil, err
	}

	if err := h.SaveWorkflow(wf); err != nil {
		return nil, err
	}

	return &WorkflowRun{
		ID:        id,
		harness:   h,
		startTime: wf.StartedAt,
	}, nil
}
