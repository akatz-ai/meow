package types

import (
	"encoding/json"
	"fmt"
	"time"
)

// BeadType represents the 6 primitive bead types in MEOW.
type BeadType string

const (
	BeadTypeTask      BeadType = "task"      // Agent-executed work
	BeadTypeCondition BeadType = "condition" // Branching, looping, waiting
	BeadTypeStop      BeadType = "stop"      // Kill agent session
	BeadTypeStart     BeadType = "start"     // Spawn agent (with optional resume)
	BeadTypeCode      BeadType = "code"      // Execute shell code
	BeadTypeExpand    BeadType = "expand"    // Template expansion
)

// Valid returns true if the bead type is one of the 6 primitives.
func (t BeadType) Valid() bool {
	switch t {
	case BeadTypeTask, BeadTypeCondition, BeadTypeStop, BeadTypeStart, BeadTypeCode, BeadTypeExpand:
		return true
	}
	return false
}

// BeadStatus represents the lifecycle state of a bead.
type BeadStatus string

const (
	BeadStatusOpen       BeadStatus = "open"
	BeadStatusInProgress BeadStatus = "in_progress"
	BeadStatusClosed     BeadStatus = "closed"
)

// Valid returns true if the status is valid.
func (s BeadStatus) Valid() bool {
	switch s {
	case BeadStatusOpen, BeadStatusInProgress, BeadStatusClosed:
		return true
	}
	return false
}

// CanTransitionTo returns true if transitioning from s to target is valid.
func (s BeadStatus) CanTransitionTo(target BeadStatus) bool {
	switch s {
	case BeadStatusOpen:
		return target == BeadStatusInProgress || target == BeadStatusClosed
	case BeadStatusInProgress:
		return target == BeadStatusClosed || target == BeadStatusOpen // Allow reopening
	case BeadStatusClosed:
		return target == BeadStatusOpen // Allow reopening
	}
	return false
}

// OutputType represents the type of output from a code bead.
type OutputType string

const (
	OutputTypeStdout   OutputType = "stdout"
	OutputTypeStderr   OutputType = "stderr"
	OutputTypeFile     OutputType = "file"
	OutputTypeExitCode OutputType = "exit_code"
)

// OutputSpec defines how to capture output from a code bead.
type OutputSpec struct {
	Name   string     `json:"name" toml:"name"`
	Source OutputType `json:"source" toml:"source"`
	Path   string     `json:"path,omitempty" toml:"path,omitempty"` // For file outputs
}

// TaskOutputType represents the data type of a task output.
type TaskOutputType string

const (
	TaskOutputTypeString    TaskOutputType = "string"
	TaskOutputTypeStringArr TaskOutputType = "string[]"
	TaskOutputTypeNumber    TaskOutputType = "number"
	TaskOutputTypeBool      TaskOutputType = "boolean"
	TaskOutputTypeJSON      TaskOutputType = "json"
	TaskOutputTypeBeadID    TaskOutputType = "bead_id"
	TaskOutputTypeBeadIDArr TaskOutputType = "bead_id[]"
	TaskOutputTypeFilePath  TaskOutputType = "file_path"
)

// TaskOutputDef defines a required or optional output from a task bead.
type TaskOutputDef struct {
	Name        string         `json:"name" toml:"name"`
	Type        TaskOutputType `json:"type" toml:"type"`
	Description string         `json:"description,omitempty" toml:"description,omitempty"`
}

// TaskOutputSpec defines the expected outputs from a task bead.
type TaskOutputSpec struct {
	Required []TaskOutputDef `json:"required,omitempty" toml:"required,omitempty"`
	Optional []TaskOutputDef `json:"optional,omitempty" toml:"optional,omitempty"`
}

// ExpansionTarget specifies what to expand for condition branches.
type ExpansionTarget struct {
	Template  string            `json:"template,omitempty" toml:"template,omitempty"`
	Inline    []json.RawMessage `json:"inline,omitempty" toml:"inline,omitempty"`
	Variables map[string]string `json:"variables,omitempty" toml:"variables,omitempty"`
}

// OnErrorAction specifies what to do when a code bead fails.
type OnErrorAction string

const (
	OnErrorContinue OnErrorAction = "continue" // Log and continue (default)
	OnErrorAbort    OnErrorAction = "abort"    // Stop workflow
	OnErrorRetry    OnErrorAction = "retry"    // Retry up to max_retries
)

// ConditionSpec holds fields specific to condition beads.
type ConditionSpec struct {
	Condition string           `json:"condition" toml:"condition"`
	OnTrue    *ExpansionTarget `json:"on_true,omitempty" toml:"on_true,omitempty"`
	OnFalse   *ExpansionTarget `json:"on_false,omitempty" toml:"on_false,omitempty"`
	OnTimeout *ExpansionTarget `json:"on_timeout,omitempty" toml:"on_timeout,omitempty"`
	Timeout   string           `json:"timeout,omitempty" toml:"timeout,omitempty"` // Duration string
}

// StopSpec holds fields specific to stop beads.
type StopSpec struct {
	Agent    string `json:"agent" toml:"agent"`
	Graceful bool   `json:"graceful,omitempty" toml:"graceful,omitempty"`
	Timeout  int    `json:"timeout,omitempty" toml:"timeout,omitempty"` // Seconds
}

// StartSpec holds fields specific to start beads.
type StartSpec struct {
	Agent         string            `json:"agent" toml:"agent"`
	Workdir       string            `json:"workdir,omitempty" toml:"workdir,omitempty"`
	Env           map[string]string `json:"env,omitempty" toml:"env,omitempty"`
	Prompt        string            `json:"prompt,omitempty" toml:"prompt,omitempty"` // Default: "meow prime"
	ResumeSession string            `json:"resume_session,omitempty" toml:"resume_session,omitempty"`
}

// CodeSpec holds fields specific to code beads.
type CodeSpec struct {
	Code       string            `json:"code" toml:"code"`
	Workdir    string            `json:"workdir,omitempty" toml:"workdir,omitempty"`
	Env        map[string]string `json:"env,omitempty" toml:"env,omitempty"`
	Outputs    []OutputSpec      `json:"outputs,omitempty" toml:"outputs,omitempty"`
	OnError    OnErrorAction     `json:"on_error,omitempty" toml:"on_error,omitempty"`
	MaxRetries int               `json:"max_retries,omitempty" toml:"max_retries,omitempty"`
}

// ExpandSpec holds fields specific to expand beads.
type ExpandSpec struct {
	Template  string            `json:"template" toml:"template"`
	Variables map[string]string `json:"variables,omitempty" toml:"variables,omitempty"`
	Assignee  string            `json:"assignee,omitempty" toml:"assignee,omitempty"`
	Ephemeral bool              `json:"ephemeral,omitempty" toml:"ephemeral,omitempty"`
}

// Bead represents a single unit of work in MEOW.
type Bead struct {
	// Common fields
	ID          string     `json:"id" toml:"id"`
	Type        BeadType   `json:"type" toml:"type"`
	Title       string     `json:"title" toml:"title"`
	Description string     `json:"description,omitempty" toml:"description,omitempty"`
	Status      BeadStatus `json:"status" toml:"status"`
	Assignee    string     `json:"assignee,omitempty" toml:"assignee,omitempty"`
	Needs       []string   `json:"needs,omitempty" toml:"needs,omitempty"` // Dependency bead IDs
	Labels      []string   `json:"labels,omitempty" toml:"labels,omitempty"`
	Notes       string     `json:"notes,omitempty" toml:"notes,omitempty"`
	Parent      string     `json:"parent,omitempty" toml:"parent,omitempty"` // Parent bead for nested expansions

	// Timestamps
	CreatedAt time.Time  `json:"created_at" toml:"created_at"`
	ClosedAt  *time.Time `json:"closed_at,omitempty" toml:"closed_at,omitempty"`

	// Stored outputs (set when bead closes)
	Outputs map[string]any `json:"outputs,omitempty" toml:"outputs,omitempty"`

	// Task-specific: required/optional output definitions
	TaskOutputs *TaskOutputSpec `json:"task_outputs,omitempty" toml:"task_outputs,omitempty"`

	// Instructions for task beads
	Instructions string `json:"instructions,omitempty" toml:"instructions,omitempty"`

	// Type-specific specs (only one should be set based on Type)
	ConditionSpec *ConditionSpec `json:"condition_spec,omitempty" toml:"condition_spec,omitempty"`
	StopSpec      *StopSpec      `json:"stop_spec,omitempty" toml:"stop_spec,omitempty"`
	StartSpec     *StartSpec     `json:"start_spec,omitempty" toml:"start_spec,omitempty"`
	CodeSpec      *CodeSpec      `json:"code_spec,omitempty" toml:"code_spec,omitempty"`
	ExpandSpec    *ExpandSpec    `json:"expand_spec,omitempty" toml:"expand_spec,omitempty"`
}

// IsEphemeral returns true if this bead is marked as ephemeral.
func (b *Bead) IsEphemeral() bool {
	for _, label := range b.Labels {
		if label == "meow:ephemeral" {
			return true
		}
	}
	return false
}

// Validate checks that the bead is well-formed.
func (b *Bead) Validate() error {
	if b.ID == "" {
		return fmt.Errorf("bead ID is required")
	}
	if !b.Type.Valid() {
		return fmt.Errorf("invalid bead type: %s", b.Type)
	}
	if !b.Status.Valid() {
		return fmt.Errorf("invalid bead status: %s", b.Status)
	}
	if b.Title == "" {
		return fmt.Errorf("bead title is required")
	}

	// Validate type-specific specs
	switch b.Type {
	case BeadTypeCondition:
		if b.ConditionSpec == nil {
			return fmt.Errorf("condition bead requires condition_spec")
		}
		if b.ConditionSpec.Condition == "" {
			return fmt.Errorf("condition bead requires condition command")
		}
	case BeadTypeStop:
		if b.StopSpec == nil {
			return fmt.Errorf("stop bead requires stop_spec")
		}
		if b.StopSpec.Agent == "" {
			return fmt.Errorf("stop bead requires agent")
		}
	case BeadTypeStart:
		if b.StartSpec == nil {
			return fmt.Errorf("start bead requires start_spec")
		}
		if b.StartSpec.Agent == "" {
			return fmt.Errorf("start bead requires agent")
		}
	case BeadTypeCode:
		if b.CodeSpec == nil {
			return fmt.Errorf("code bead requires code_spec")
		}
		if b.CodeSpec.Code == "" {
			return fmt.Errorf("code bead requires code")
		}
	case BeadTypeExpand:
		if b.ExpandSpec == nil {
			return fmt.Errorf("expand bead requires expand_spec")
		}
		if b.ExpandSpec.Template == "" {
			return fmt.Errorf("expand bead requires template")
		}
	}

	return nil
}

// Close marks the bead as closed with the given outputs.
func (b *Bead) Close(outputs map[string]any) error {
	if !b.Status.CanTransitionTo(BeadStatusClosed) {
		return fmt.Errorf("cannot close bead in status %s", b.Status)
	}
	now := time.Now()
	b.Status = BeadStatusClosed
	b.ClosedAt = &now
	if outputs != nil {
		b.Outputs = outputs
	}
	return nil
}
