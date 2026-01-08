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
// Returns a copy to prevent callers from mutating internal state.
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

	// Return a copy to prevent callers from mutating internal state
	return copyBead(ready[0]), nil
}

// isReadyLocked checks if a bead is ready (caller must hold lock).
func (s *FileBeadStore) isReadyLocked(bead *types.Bead) bool {
	// Must have a valid bead type - issue tracker beads don't have types
	// and should not be processed by the orchestrator
	if bead.Type == "" {
		return false
	}

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
// Priority is determined by tier first, then by type within tier.
func (s *FileBeadStore) beadPriority(bead *types.Bead) int {
	// Tier priority: orchestrator (0-99), wisp (100-199), work (200-299)
	tierBase := s.tierPriority(bead.Tier)

	// Type priority within tier
	typePrio := s.typePriority(bead.Type)

	return tierBase + typePrio
}

// tierPriority returns the base priority for a tier.
// Orchestrator beads are processed first (machinery), then wisps (agent workflow steps),
// then work beads (permanent deliverables - though orchestrator typically doesn't return these).
func (s *FileBeadStore) tierPriority(tier types.BeadTier) int {
	switch tier {
	case types.TierOrchestrator:
		return 0
	case types.TierWisp:
		return 100
	case types.TierWork:
		return 200
	default:
		// Empty tier defaults to work tier behavior
		return 200
	}
}

// typePriority returns priority by bead type within a tier (0-99 range).
func (s *FileBeadStore) typePriority(beadType types.BeadType) int {
	switch beadType {
	case types.BeadTypeCondition:
		return 0
	case types.BeadTypeCode:
		return 10
	case types.BeadTypeExpand:
		return 20
	case types.BeadTypeStart:
		return 30
	case types.BeadTypeStop:
		return 40
	case types.BeadTypeGate:
		return 50
	case types.BeadTypeTask:
		return 60
	case types.BeadTypeCollaborative:
		return 70
	default:
		return 50
	}
}

// Get retrieves a bead by ID.
// Returns a copy to prevent callers from mutating internal state.
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
	return copyBead(bead), nil
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
		// Skip issue tracker beads (no Type) - only check workflow beads
		if bead.Type == "" {
			continue
		}
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

// BeadFilter specifies filter criteria for listing beads.
type BeadFilter struct {
	Status     types.BeadStatus // Filter by status (empty = all)
	Tier       types.BeadTier   // Filter by tier (empty = all)
	Assignee   string           // Filter by assignee (empty = all)
	WorkflowID string           // Filter by workflow ID (empty = all)
	HookBead   string           // Filter by linked work bead (empty = all)
}

// List returns all beads matching the given filter.
func (s *FileBeadStore) List(ctx context.Context, status types.BeadStatus) ([]*types.Bead, error) {
	return s.ListFiltered(ctx, BeadFilter{Status: status})
}

// ListFiltered returns all beads matching the given filter.
func (s *FileBeadStore) ListFiltered(ctx context.Context, filter BeadFilter) ([]*types.Bead, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if !s.loaded {
		return nil, fmt.Errorf("bead store not loaded")
	}

	var result []*types.Bead
	for _, bead := range s.beads {
		if s.matchesFilter(bead, filter) {
			result = append(result, bead)
		}
	}
	return result, nil
}

// matchesFilter returns true if the bead matches all filter criteria.
func (s *FileBeadStore) matchesFilter(bead *types.Bead, filter BeadFilter) bool {
	if filter.Status != "" && bead.Status != filter.Status {
		return false
	}
	if filter.Tier != "" && bead.Tier != filter.Tier {
		return false
	}
	if filter.Assignee != "" && bead.Assignee != filter.Assignee {
		return false
	}
	if filter.WorkflowID != "" && bead.WorkflowID != filter.WorkflowID {
		return false
	}
	if filter.HookBead != "" && bead.HookBead != filter.HookBead {
		return false
	}
	return true
}

// ListByTier returns all beads with the specified tier.
func (s *FileBeadStore) ListByTier(ctx context.Context, tier types.BeadTier) ([]*types.Bead, error) {
	return s.ListFiltered(ctx, BeadFilter{Tier: tier})
}

// ListWispsForAgent returns all wisp-tier beads assigned to the given agent.
func (s *FileBeadStore) ListWispsForAgent(ctx context.Context, agentID string) ([]*types.Bead, error) {
	return s.ListFiltered(ctx, BeadFilter{
		Tier:     types.TierWisp,
		Assignee: agentID,
	})
}

// ListOrchestrator returns all orchestrator-tier beads for a workflow.
func (s *FileBeadStore) ListOrchestrator(ctx context.Context, workflowID string) ([]*types.Bead, error) {
	return s.ListFiltered(ctx, BeadFilter{
		Tier:       types.TierOrchestrator,
		WorkflowID: workflowID,
	})
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

// Delete removes a bead by ID.
func (s *FileBeadStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.beads[id]; !exists {
		return fmt.Errorf("bead %s not found", id)
	}

	delete(s.beads, id)
	return s.writeLocked()
}

// copyBead creates a deep copy of a bead to prevent external mutation.
func copyBead(b *types.Bead) *types.Bead {
	if b == nil {
		return nil
	}
	cp := *b // Shallow copy

	// Deep copy slices
	if b.Needs != nil {
		cp.Needs = make([]string, len(b.Needs))
		copy(cp.Needs, b.Needs)
	}
	if b.Labels != nil {
		cp.Labels = make([]string, len(b.Labels))
		copy(cp.Labels, b.Labels)
	}

	// Deep copy pointer fields
	if b.ClosedAt != nil {
		t := *b.ClosedAt
		cp.ClosedAt = &t
	}

	// Deep copy maps
	if b.Outputs != nil {
		cp.Outputs = make(map[string]any, len(b.Outputs))
		for k, v := range b.Outputs {
			cp.Outputs[k] = v
		}
	}

	// Deep copy specs with their nested structures
	if b.TaskOutputs != nil {
		cp.TaskOutputs = copyTaskOutputSpec(b.TaskOutputs)
	}
	if b.ConditionSpec != nil {
		cp.ConditionSpec = copyConditionSpec(b.ConditionSpec)
	}
	if b.StopSpec != nil {
		spec := *b.StopSpec
		cp.StopSpec = &spec
	}
	if b.StartSpec != nil {
		cp.StartSpec = copyStartSpec(b.StartSpec)
	}
	if b.CodeSpec != nil {
		cp.CodeSpec = copyCodeSpec(b.CodeSpec)
	}
	if b.ExpandSpec != nil {
		cp.ExpandSpec = copyExpandSpec(b.ExpandSpec)
	}

	return &cp
}

// copyTaskOutputSpec creates a deep copy of a TaskOutputSpec.
func copyTaskOutputSpec(spec *types.TaskOutputSpec) *types.TaskOutputSpec {
	if spec == nil {
		return nil
	}
	cp := &types.TaskOutputSpec{}
	if spec.Required != nil {
		cp.Required = make([]types.TaskOutputDef, len(spec.Required))
		copy(cp.Required, spec.Required)
	}
	if spec.Optional != nil {
		cp.Optional = make([]types.TaskOutputDef, len(spec.Optional))
		copy(cp.Optional, spec.Optional)
	}
	return cp
}

// copyConditionSpec creates a deep copy of a ConditionSpec.
func copyConditionSpec(spec *types.ConditionSpec) *types.ConditionSpec {
	if spec == nil {
		return nil
	}
	cp := &types.ConditionSpec{
		Condition: spec.Condition,
		Timeout:   spec.Timeout,
	}
	if spec.OnTrue != nil {
		cp.OnTrue = copyExpansionTarget(spec.OnTrue)
	}
	if spec.OnFalse != nil {
		cp.OnFalse = copyExpansionTarget(spec.OnFalse)
	}
	if spec.OnTimeout != nil {
		cp.OnTimeout = copyExpansionTarget(spec.OnTimeout)
	}
	return cp
}

// copyExpansionTarget creates a deep copy of an ExpansionTarget.
func copyExpansionTarget(target *types.ExpansionTarget) *types.ExpansionTarget {
	if target == nil {
		return nil
	}
	cp := &types.ExpansionTarget{
		Template: target.Template,
	}
	if target.Variables != nil {
		cp.Variables = make(map[string]string, len(target.Variables))
		for k, v := range target.Variables {
			cp.Variables[k] = v
		}
	}
	if target.Inline != nil {
		cp.Inline = make([]json.RawMessage, len(target.Inline))
		for i, raw := range target.Inline {
			cp.Inline[i] = make(json.RawMessage, len(raw))
			copy(cp.Inline[i], raw)
		}
	}
	return cp
}

// copyStartSpec creates a deep copy of a StartSpec.
func copyStartSpec(spec *types.StartSpec) *types.StartSpec {
	if spec == nil {
		return nil
	}
	cp := &types.StartSpec{
		Agent:         spec.Agent,
		Workdir:       spec.Workdir,
		Prompt:        spec.Prompt,
		ResumeSession: spec.ResumeSession,
	}
	if spec.Env != nil {
		cp.Env = make(map[string]string, len(spec.Env))
		for k, v := range spec.Env {
			cp.Env[k] = v
		}
	}
	return cp
}

// copyCodeSpec creates a deep copy of a CodeSpec.
func copyCodeSpec(spec *types.CodeSpec) *types.CodeSpec {
	if spec == nil {
		return nil
	}
	cp := &types.CodeSpec{
		Code:       spec.Code,
		Workdir:    spec.Workdir,
		OnError:    spec.OnError,
		MaxRetries: spec.MaxRetries,
	}
	if spec.Env != nil {
		cp.Env = make(map[string]string, len(spec.Env))
		for k, v := range spec.Env {
			cp.Env[k] = v
		}
	}
	if spec.Outputs != nil {
		cp.Outputs = make([]types.OutputSpec, len(spec.Outputs))
		copy(cp.Outputs, spec.Outputs)
	}
	return cp
}

// copyExpandSpec creates a deep copy of an ExpandSpec.
func copyExpandSpec(spec *types.ExpandSpec) *types.ExpandSpec {
	if spec == nil {
		return nil
	}
	cp := &types.ExpandSpec{
		Template:  spec.Template,
		Assignee:  spec.Assignee,
		Ephemeral: spec.Ephemeral,
	}
	if spec.Variables != nil {
		cp.Variables = make(map[string]string, len(spec.Variables))
		for k, v := range spec.Variables {
			cp.Variables[k] = v
		}
	}
	return cp
}
