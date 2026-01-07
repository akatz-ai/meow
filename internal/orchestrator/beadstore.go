package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/meow-stack/meow-machine/internal/types"
)

// FileBeadStore implements BeadStore using the beads CLI's file format.
type FileBeadStore struct {
	beadsDir string

	mu     sync.RWMutex
	beads  map[string]*types.Bead
	loaded bool
}

// NewFileBeadStore creates a BeadStore backed by the .beads directory.
func NewFileBeadStore(beadsDir string) *FileBeadStore {
	return &FileBeadStore{
		beadsDir: beadsDir,
		beads:    make(map[string]*types.Bead),
	}
}

// Load reads all beads from the beads directory.
func (s *FileBeadStore) Load(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.loadLocked()
}

// loadLocked performs the actual load (caller must hold the lock).
func (s *FileBeadStore) loadLocked() error {
	issuesPath := filepath.Join(s.beadsDir, "issues.jsonl")
	file, err := os.Open(issuesPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.beads = make(map[string]*types.Bead)
			s.loaded = true
			return nil
		}
		return fmt.Errorf("opening issues file: %w", err)
	}
	defer file.Close()

	s.beads = make(map[string]*types.Bead)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var bead types.Bead
		if err := json.Unmarshal(scanner.Bytes(), &bead); err != nil {
			// Skip malformed lines
			continue
		}
		s.beads[bead.ID] = &bead
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading issues file: %w", err)
	}

	s.loaded = true
	return nil
}

// GetNextReady returns the next bead that is ready to execute.
func (s *FileBeadStore) GetNextReady(ctx context.Context) (*types.Bead, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.loaded {
		return nil, fmt.Errorf("bead store not loaded")
	}

	// Find all ready beads
	var ready []*types.Bead
	for _, bead := range s.beads {
		if s.isReadyLocked(bead) {
			ready = append(ready, bead)
		}
	}

	if len(ready) == 0 {
		return nil, nil
	}

	// Sort by priority
	sort.Slice(ready, func(i, j int) bool {
		return s.beadPriority(ready[i]) < s.beadPriority(ready[j])
	})

	return ready[0], nil
}

// isReadyLocked checks if a bead is ready (caller must hold lock).
func (s *FileBeadStore) isReadyLocked(bead *types.Bead) bool {
	// Must be open
	if bead.Status != types.BeadStatusOpen {
		return false
	}

	// All dependencies must be closed
	for _, depID := range bead.Needs {
		dep, ok := s.beads[depID]
		if !ok {
			// Dependency doesn't exist - treat as not ready
			return false
		}
		if dep.Status != types.BeadStatusClosed {
			return false
		}
	}

	return true
}

// beadPriority returns a priority value (lower = higher priority).
func (s *FileBeadStore) beadPriority(bead *types.Bead) int {
	// Orchestrator beads first
	switch bead.Type {
	case types.BeadTypeCondition:
		return 0
	case types.BeadTypeCode:
		return 1
	case types.BeadTypeExpand:
		return 2
	case types.BeadTypeStart:
		return 3
	case types.BeadTypeStop:
		return 4
	case types.BeadTypeTask:
		return 10 // Tasks last
	default:
		return 5
	}
}

// Get retrieves a bead by ID.
func (s *FileBeadStore) Get(ctx context.Context, id string) (*types.Bead, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.loaded {
		return nil, fmt.Errorf("bead store not loaded")
	}

	bead, ok := s.beads[id]
	if !ok {
		return nil, nil
	}
	return bead, nil
}

// Update saves changes to a bead.
func (s *FileBeadStore) Update(ctx context.Context, bead *types.Bead) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update in-memory
	s.beads[bead.ID] = bead

	// Write to file
	return s.writeLocked()
}

// AllDone returns true if there are no open or in-progress beads.
func (s *FileBeadStore) AllDone(ctx context.Context) (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.loaded {
		return false, fmt.Errorf("bead store not loaded")
	}

	for _, bead := range s.beads {
		if bead.Status == types.BeadStatusOpen || bead.Status == types.BeadStatusInProgress {
			return false, nil
		}
	}
	return true, nil
}

// Create adds a new bead.
func (s *FileBeadStore) Create(ctx context.Context, bead *types.Bead) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.beads[bead.ID]; exists {
		return fmt.Errorf("bead %s already exists", bead.ID)
	}

	s.beads[bead.ID] = bead
	return s.writeLocked()
}

// List returns all beads matching the given filter.
func (s *FileBeadStore) List(ctx context.Context, status types.BeadStatus) ([]*types.Bead, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*types.Bead
	for _, bead := range s.beads {
		if status == "" || bead.Status == status {
			result = append(result, bead)
		}
	}
	return result, nil
}

// writeLocked writes all beads to the issues file (caller must hold lock).
func (s *FileBeadStore) writeLocked() error {
	issuesPath := filepath.Join(s.beadsDir, "issues.jsonl")

	// Ensure directory exists
	if err := os.MkdirAll(s.beadsDir, 0755); err != nil {
		return fmt.Errorf("creating beads directory: %w", err)
	}

	// Write to temp file first
	tmpPath := issuesPath + ".tmp"
	file, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	for _, bead := range s.beads {
		data, err := json.Marshal(bead)
		if err != nil {
			file.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("marshaling bead %s: %w", bead.ID, err)
		}
		if _, err := file.Write(data); err != nil {
			file.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("writing bead %s: %w", bead.ID, err)
		}
		if _, err := file.WriteString("\n"); err != nil {
			file.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("writing newline: %w", err)
		}
	}

	if err := file.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, issuesPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// Reload re-reads beads from disk.
func (s *FileBeadStore) Reload(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loaded = false
	return s.loadLocked()
}
