package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// OrchestratorState represents the persistent state of the orchestrator.
type OrchestratorState struct {
	// Version for state file format
	Version string `json:"version"`

	// Current workflow being executed
	WorkflowID   string `json:"workflow_id,omitempty"`
	TemplateName string `json:"template_name,omitempty"`

	// Execution tracking
	StartedAt   time.Time  `json:"started_at"`
	LastTickAt  *time.Time `json:"last_tick_at,omitempty"`
	TickCount   int64      `json:"tick_count"`

	// Active condition goroutines
	ActiveConditions []string `json:"active_conditions,omitempty"`

	// PID for crash detection
	PID int `json:"pid"`
}

// StatePersister handles persistent orchestrator state.
type StatePersister struct {
	stateDir string
	lockFile *os.File
}

// NewStatePersister creates a new state persister.
func NewStatePersister(stateDir string) *StatePersister {
	return &StatePersister{
		stateDir: stateDir,
	}
}

// AcquireLock acquires an exclusive lock to prevent concurrent orchestrators.
func (p *StatePersister) AcquireLock() error {
	if err := os.MkdirAll(p.stateDir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	lockPath := filepath.Join(p.stateDir, "orchestrator.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("opening lock file: %w", err)
	}

	// Try to acquire exclusive lock (non-blocking)
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		file.Close()
		if err == syscall.EWOULDBLOCK {
			return fmt.Errorf("another orchestrator is already running")
		}
		return fmt.Errorf("acquiring lock: %w", err)
	}

	// Write our PID to the lock file
	file.Truncate(0)
	file.Seek(0, 0)
	fmt.Fprintf(file, "%d\n", os.Getpid())

	p.lockFile = file
	return nil
}

// ReleaseLock releases the exclusive lock.
func (p *StatePersister) ReleaseLock() error {
	if p.lockFile == nil {
		return nil
	}

	if err := syscall.Flock(int(p.lockFile.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("releasing lock: %w", err)
	}

	if err := p.lockFile.Close(); err != nil {
		return fmt.Errorf("closing lock file: %w", err)
	}

	// Remove lock file
	lockPath := filepath.Join(p.stateDir, "orchestrator.lock")
	os.Remove(lockPath)

	p.lockFile = nil
	return nil
}

// SaveState atomically writes orchestrator state.
func (p *StatePersister) SaveState(state *OrchestratorState) error {
	if err := os.MkdirAll(p.stateDir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	statePath := filepath.Join(p.stateDir, "orchestrator.json")
	tmpPath := statePath + ".tmp"

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := os.Rename(tmpPath, statePath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// LoadState reads orchestrator state from disk.
func (p *StatePersister) LoadState() (*OrchestratorState, error) {
	statePath := filepath.Join(p.stateDir, "orchestrator.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading state file: %w", err)
	}

	var state OrchestratorState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state file: %w", err)
	}

	return &state, nil
}

// UpdateHeartbeat writes a heartbeat file for health checks.
func (p *StatePersister) UpdateHeartbeat() error {
	if err := os.MkdirAll(p.stateDir, 0755); err != nil {
		return fmt.Errorf("creating state directory: %w", err)
	}

	heartbeatPath := filepath.Join(p.stateDir, "heartbeat.json")
	data := fmt.Sprintf(`{"pid":%d,"timestamp":"%s"}`, os.Getpid(), time.Now().Format(time.RFC3339))

	if err := os.WriteFile(heartbeatPath, []byte(data), 0644); err != nil {
		return fmt.Errorf("writing heartbeat: %w", err)
	}

	return nil
}

// CheckStaleHeartbeat checks if the heartbeat is stale (no updates for given duration).
func (p *StatePersister) CheckStaleHeartbeat(maxAge time.Duration) (bool, error) {
	heartbeatPath := filepath.Join(p.stateDir, "heartbeat.json")
	info, err := os.Stat(heartbeatPath)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, fmt.Errorf("stat heartbeat: %w", err)
	}

	return time.Since(info.ModTime()) > maxAge, nil
}

// ClearState removes the state file.
func (p *StatePersister) ClearState() error {
	statePath := filepath.Join(p.stateDir, "orchestrator.json")
	if err := os.Remove(statePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing state file: %w", err)
	}
	return nil
}

// IsLockHeld checks if another process holds the lock.
func (p *StatePersister) IsLockHeld() (bool, int, error) {
	lockPath := filepath.Join(p.stateDir, "orchestrator.lock")

	file, err := os.Open(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, 0, nil
		}
		return false, 0, fmt.Errorf("opening lock file: %w", err)
	}
	defer file.Close()

	// Try to get a shared lock (non-blocking)
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		// Lock is held exclusively
		var pid int
		fmt.Fscanf(file, "%d", &pid)
		return true, pid, nil
	}

	// We got the shared lock, release it
	syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	return false, 0, nil
}
