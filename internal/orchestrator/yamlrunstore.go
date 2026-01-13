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

// WorkflowLock represents an exclusive lock on a specific workflow.
// This enables multiple orchestrators to run different workflows concurrently.
type WorkflowLock struct {
	workflowID string
	lockFile   *os.File
	lockPath   string
}

// Release releases the workflow lock and cleans up the lock file.
func (l *WorkflowLock) Release() error {
	if l.lockFile == nil {
		return nil
	}
	syscall.Flock(int(l.lockFile.Fd()), syscall.LOCK_UN)
	err := l.lockFile.Close()
	l.lockFile = nil
	// Clean up lock file (best effort)
	os.Remove(l.lockPath)
	return err
}

// YAMLRunStore persists workflows as YAML files with atomic writes.
// Multiple stores can be created for the same directory - locking is per-workflow.
type YAMLRunStore struct {
	dir string // .meow/workflows
}

// NewYAMLRunStore creates a new store.
// The store does not acquire any locks - use AcquireWorkflowLock for per-workflow locking.
func NewYAMLRunStore(dir string) (*YAMLRunStore, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating workflow dir: %w", err)
	}

	// Recover from any interrupted writes
	if err := recoverInterruptedWrites(dir); err != nil {
		return nil, fmt.Errorf("recovering interrupted writes: %w", err)
	}

	return &YAMLRunStore{dir: dir}, nil
}

// AcquireWorkflowLock acquires an exclusive lock for a specific workflow.
// This prevents multiple orchestrators from running the same workflow concurrently.
// Other workflows are not affected and can run in parallel.
func (s *YAMLRunStore) AcquireWorkflowLock(workflowID string) (*WorkflowLock, error) {
	lockPath := filepath.Join(s.dir, workflowID+".yaml.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("opening workflow lock file: %w", err)
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("workflow %s is already being orchestrated (lock held): %w", workflowID, err)
	}

	return &WorkflowLock{
		workflowID: workflowID,
		lockFile:   lockFile,
		lockPath:   lockPath,
	}, nil
}

// IsLocked checks if a workflow is currently locked (orchestrator running).
// Returns true if another process holds the lock, false otherwise.
func (s *YAMLRunStore) IsLocked(workflowID string) bool {
	lockPath := filepath.Join(s.dir, workflowID+".yaml.lock")

	// Try to open the lock file
	lockFile, err := os.OpenFile(lockPath, os.O_RDWR, 0644)
	if err != nil {
		return false // No lock file = not locked
	}
	defer lockFile.Close()

	// Try non-blocking lock
	err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		return true // Couldn't acquire = someone has it
	}

	// We got the lock, release it immediately
	syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	return false
}

// Close is a no-op for compatibility. Use WorkflowLock.Release() to release locks.
func (s *YAMLRunStore) Close() error {
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
func (s *YAMLRunStore) Create(ctx context.Context, wf *types.Run) error {
	path := filepath.Join(s.dir, wf.ID+".yaml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("workflow already exists: %s", wf.ID)
	}
	return s.Save(ctx, wf)
}

// Get retrieves a workflow by ID.
func (s *YAMLRunStore) Get(ctx context.Context, id string) (*types.Run, error) {
	path := filepath.Join(s.dir, id+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("workflow not found: %s", id)
		}
		return nil, err
	}

	var wf types.Run
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, fmt.Errorf("parsing workflow %s: %w", id, err)
	}
	return &wf, nil
}

// Save persists workflow state atomically (write-then-rename).
func (s *YAMLRunStore) Save(ctx context.Context, wf *types.Run) error {
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
func (s *YAMLRunStore) Delete(ctx context.Context, id string) error {
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
func (s *YAMLRunStore) List(ctx context.Context, filter RunFilter) ([]*types.Run, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}

	var workflows []*types.Run
	for _, entry := range entries {
		name := entry.Name()
		// Skip non-YAML files (including .yaml.tmp which ends in .tmp, not .yaml)
		if !strings.HasSuffix(name, ".yaml") {
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
func (s *YAMLRunStore) GetByAgent(ctx context.Context, agentID string) ([]*types.Run, error) {
	all, err := s.List(ctx, RunFilter{})
	if err != nil {
		return nil, err
	}

	var result []*types.Run
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

// Ensure YAMLRunStore implements RunStore
var _ RunStore = (*YAMLRunStore)(nil)
