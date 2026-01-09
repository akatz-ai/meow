package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/meow-stack/meow-machine/internal/types"
)

// YAMLWorkflowStore persists workflows as YAML files with atomic writes.
type YAMLWorkflowStore struct {
	dir      string   // .meow/workflows
	lockFile *os.File // Exclusive lock to prevent multiple orchestrators
}

// NewYAMLWorkflowStore creates a new store and acquires exclusive lock.
func NewYAMLWorkflowStore(dir string) (*YAMLWorkflowStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating workflow dir: %w", err)
	}

	// Acquire exclusive lock
	lockPath := filepath.Join(dir, ".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening lock file: %w", err)
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("another orchestrator is running (lock held): %w", err)
	}

	// Recover from any interrupted writes
	if err := recoverInterruptedWrites(dir); err != nil {
		syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
		lockFile.Close()
		return nil, fmt.Errorf("recovering interrupted writes: %w", err)
	}

	return &YAMLWorkflowStore{dir: dir, lockFile: lockFile}, nil
}

// Close releases the lock.
func (s *YAMLWorkflowStore) Close() error {
	if s.lockFile != nil {
		syscall.Flock(int(s.lockFile.Fd()), syscall.LOCK_UN)
		return s.lockFile.Close()
	}
	return nil
}

// recoverInterruptedWrites handles .tmp files left from crashed writes.
func recoverInterruptedWrites(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".yaml.tmp") {
			continue
		}

		tmpPath := filepath.Join(dir, entry.Name())
		mainPath := strings.TrimSuffix(tmpPath, ".tmp")

		// Check if main file exists and is valid
		if _, err := os.Stat(mainPath); err == nil {
			// Main file exists, delete orphan temp
			os.Remove(tmpPath)
		} else {
			// Main file missing, promote temp
			os.Rename(tmpPath, mainPath)
		}
	}
	return nil
}

// Create persists a new workflow.
func (s *YAMLWorkflowStore) Create(ctx context.Context, wf *types.Workflow) error {
	path := filepath.Join(s.dir, wf.ID+".yaml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("workflow already exists: %s", wf.ID)
	}
	return s.Save(ctx, wf)
}

// Get retrieves a workflow by ID.
func (s *YAMLWorkflowStore) Get(ctx context.Context, id string) (*types.Workflow, error) {
	path := filepath.Join(s.dir, id+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workflow not found: %s", id)
		}
		return nil, err
	}

	var wf types.Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parsing workflow %s: %w", id, err)
	}
	return &wf, nil
}

// Save persists workflow state atomically (write-then-rename).
func (s *YAMLWorkflowStore) Save(ctx context.Context, wf *types.Workflow) error {
	data, err := yaml.Marshal(wf)
	if err != nil {
		return fmt.Errorf("marshaling workflow: %w", err)
	}

	mainPath := filepath.Join(s.dir, wf.ID+".yaml")
	tmpPath := mainPath + ".tmp"

	// Write to temp file
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, mainPath); err != nil {
		os.Remove(tmpPath) // Clean up on failure
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// Delete removes a workflow.
func (s *YAMLWorkflowStore) Delete(ctx context.Context, id string) error {
	path := filepath.Join(s.dir, id+".yaml")
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("workflow not found: %s", id)
		}
		return err
	}
	return nil
}

// List returns all workflows matching filter.
func (s *YAMLWorkflowStore) List(ctx context.Context, filter WorkflowFilter) ([]*types.Workflow, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var workflows []*types.Workflow
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") || name == ".lock" {
			continue
		}
		if strings.HasSuffix(name, ".yaml.tmp") {
			continue
		}

		id := strings.TrimSuffix(name, ".yaml")
		wf, err := s.Get(ctx, id)
		if err != nil {
			continue // Skip invalid files
		}

		if filter.Status != "" && wf.Status != filter.Status {
			continue
		}
		workflows = append(workflows, wf)
	}
	return workflows, nil
}

// GetByAgent returns workflows with steps assigned to agent.
func (s *YAMLWorkflowStore) GetByAgent(ctx context.Context, agentID string) ([]*types.Workflow, error) {
	all, err := s.List(ctx, WorkflowFilter{})
	if err != nil {
		return nil, err
	}

	var result []*types.Workflow
	for _, wf := range all {
		for _, step := range wf.Steps {
			if step.Agent != nil && step.Agent.Agent == agentID {
				result = append(result, wf)
				break
			}
		}
	}
	return result, nil
}

// Ensure YAMLWorkflowStore implements WorkflowStore
var _ WorkflowStore = (*YAMLWorkflowStore)(nil)
