package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBeadType_Valid(t *testing.T) {
	tests := []struct {
		beadType BeadType
		want     bool
	}{
		{BeadTypeTask, true},
		{BeadTypeCollaborative, true},
		{BeadTypeGate, true},
		{BeadTypeCondition, true},
		{BeadTypeStop, true},
		{BeadTypeStart, true},
		{BeadTypeCode, true},
		{BeadTypeExpand, true},
		{BeadType("invalid"), false},
		{BeadType(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.beadType), func(t *testing.T) {
			if got := tt.beadType.Valid(); got != tt.want {
				t.Errorf("BeadType.Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBeadTier_Valid(t *testing.T) {
	tests := []struct {
		tier BeadTier
		want bool
	}{
		{TierWork, true},
		{TierWisp, true},
		{TierOrchestrator, true},
		{BeadTier(""), true}, // Empty is valid (defaults to work)
		{BeadTier("invalid"), false},
	}

	for _, tt := range tests {
		name := string(tt.tier)
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			if got := tt.tier.Valid(); got != tt.want {
				t.Errorf("BeadTier.Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBeadStatus_Valid(t *testing.T) {
	tests := []struct {
		status BeadStatus
		want   bool
	}{
		{BeadStatusOpen, true},
		{BeadStatusInProgress, true},
		{BeadStatusClosed, true},
		{BeadStatus("invalid"), false},
		{BeadStatus(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.Valid(); got != tt.want {
				t.Errorf("BeadStatus.Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBeadStatus_CanTransitionTo(t *testing.T) {
	tests := []struct {
		from BeadStatus
		to   BeadStatus
		want bool
	}{
		{BeadStatusOpen, BeadStatusInProgress, true},
		{BeadStatusOpen, BeadStatusClosed, true},
		{BeadStatusOpen, BeadStatusOpen, false},
		{BeadStatusInProgress, BeadStatusClosed, true},
		{BeadStatusInProgress, BeadStatusOpen, true}, // Reopen
		{BeadStatusInProgress, BeadStatusInProgress, false},
		{BeadStatusClosed, BeadStatusOpen, true}, // Reopen
		{BeadStatusClosed, BeadStatusInProgress, false},
		{BeadStatusClosed, BeadStatusClosed, false},
		// Invalid status transitions
		{BeadStatus("invalid"), BeadStatusOpen, false},
		{BeadStatus("invalid"), BeadStatusInProgress, false},
		{BeadStatus("invalid"), BeadStatusClosed, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			if got := tt.from.CanTransitionTo(tt.to); got != tt.want {
				t.Errorf("BeadStatus.CanTransitionTo(%s) = %v, want %v", tt.to, got, tt.want)
			}
		})
	}
}

func TestBead_JSONRoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	bead := &Bead{
		ID:          "bd-test-001",
		Type:        BeadTypeTask,
		Title:       "Test task",
		Description: "A test task",
		Status:      BeadStatusOpen,
		Assignee:    "claude-1",
		Needs:       []string{"bd-dep-001"},
		Labels:      []string{"test", "priority:high"},
		Notes:       "Some notes",
		CreatedAt:   now,
		TaskOutputs: &TaskOutputSpec{
			Required: []TaskOutputDef{
				{Name: "result", Type: TaskOutputTypeString},
			},
		},
	}

	data, err := json.Marshal(bead)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded Bead
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.ID != bead.ID {
		t.Errorf("ID mismatch: got %s, want %s", decoded.ID, bead.ID)
	}
	if decoded.Type != bead.Type {
		t.Errorf("Type mismatch: got %s, want %s", decoded.Type, bead.Type)
	}
	if decoded.Title != bead.Title {
		t.Errorf("Title mismatch: got %s, want %s", decoded.Title, bead.Title)
	}
	if decoded.Status != bead.Status {
		t.Errorf("Status mismatch: got %s, want %s", decoded.Status, bead.Status)
	}
	if len(decoded.Needs) != len(bead.Needs) {
		t.Errorf("Needs length mismatch: got %d, want %d", len(decoded.Needs), len(bead.Needs))
	}
	if len(decoded.TaskOutputs.Required) != 1 {
		t.Errorf("TaskOutputs.Required length mismatch: got %d, want 1", len(decoded.TaskOutputs.Required))
	}
}

func TestBead_Validate(t *testing.T) {
	tests := []struct {
		name    string
		bead    *Bead
		wantErr bool
	}{
		{
			name: "valid task bead",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeTask,
				Title:  "Test",
				Status: BeadStatusOpen,
			},
			wantErr: false,
		},
		{
			name: "missing ID",
			bead: &Bead{
				Type:   BeadTypeTask,
				Title:  "Test",
				Status: BeadStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadType("invalid"),
				Title:  "Test",
				Status: BeadStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "missing title",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeTask,
				Status: BeadStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "condition without spec",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeCondition,
				Title:  "Test",
				Status: BeadStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "valid condition bead",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeCondition,
				Title:  "Test",
				Status: BeadStatusOpen,
				ConditionSpec: &ConditionSpec{
					Condition: "test -f file.txt",
				},
			},
			wantErr: false,
		},
		{
			name: "valid code bead",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeCode,
				Title:  "Test",
				Status: BeadStatusOpen,
				CodeSpec: &CodeSpec{
					Code: "echo hello",
				},
			},
			wantErr: false,
		},
		{
			name: "valid start bead",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeStart,
				Title:  "Test",
				Status: BeadStatusOpen,
				StartSpec: &StartSpec{
					Agent: "claude-1",
				},
			},
			wantErr: false,
		},
		{
			name: "valid stop bead",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeStop,
				Title:  "Test",
				Status: BeadStatusOpen,
				StopSpec: &StopSpec{
					Agent: "claude-1",
				},
			},
			wantErr: false,
		},
		{
			name: "valid expand bead",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeExpand,
				Title:  "Test",
				Status: BeadStatusOpen,
				ExpandSpec: &ExpandSpec{
					Template: "implement-tdd",
				},
			},
			wantErr: false,
		},
		{
			name: "valid collaborative bead",
			bead: &Bead{
				ID:       "bd-001",
				Type:     BeadTypeCollaborative,
				Title:    "Test",
				Status:   BeadStatusOpen,
				Assignee: "claude-1",
			},
			wantErr: false,
		},
		{
			name: "collaborative without assignee",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeCollaborative,
				Title:  "Test",
				Status: BeadStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "valid gate bead",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeGate,
				Title:  "Test",
				Status: BeadStatusOpen,
			},
			wantErr: false,
		},
		{
			name: "gate with assignee",
			bead: &Bead{
				ID:       "bd-001",
				Type:     BeadTypeGate,
				Title:    "Test",
				Status:   BeadStatusOpen,
				Assignee: "claude-1",
			},
			wantErr: true,
		},
		{
			name: "invalid tier",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeTask,
				Title:  "Test",
				Status: BeadStatusOpen,
				Tier:   BeadTier("invalid"),
			},
			wantErr: true,
		},
		{
			name: "invalid status",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeTask,
				Title:  "Test",
				Status: BeadStatus("invalid"),
			},
			wantErr: true,
		},
		{
			name: "condition without condition command",
			bead: &Bead{
				ID:            "bd-001",
				Type:          BeadTypeCondition,
				Title:         "Test",
				Status:        BeadStatusOpen,
				ConditionSpec: &ConditionSpec{},
			},
			wantErr: true,
		},
		{
			name: "stop without spec",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeStop,
				Title:  "Test",
				Status: BeadStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "stop without agent",
			bead: &Bead{
				ID:       "bd-001",
				Type:     BeadTypeStop,
				Title:    "Test",
				Status:   BeadStatusOpen,
				StopSpec: &StopSpec{},
			},
			wantErr: true,
		},
		{
			name: "start without spec",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeStart,
				Title:  "Test",
				Status: BeadStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "start without agent",
			bead: &Bead{
				ID:        "bd-001",
				Type:      BeadTypeStart,
				Title:     "Test",
				Status:    BeadStatusOpen,
				StartSpec: &StartSpec{},
			},
			wantErr: true,
		},
		{
			name: "code without spec",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeCode,
				Title:  "Test",
				Status: BeadStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "code without code",
			bead: &Bead{
				ID:       "bd-001",
				Type:     BeadTypeCode,
				Title:    "Test",
				Status:   BeadStatusOpen,
				CodeSpec: &CodeSpec{},
			},
			wantErr: true,
		},
		{
			name: "expand without spec",
			bead: &Bead{
				ID:     "bd-001",
				Type:   BeadTypeExpand,
				Title:  "Test",
				Status: BeadStatusOpen,
			},
			wantErr: true,
		},
		{
			name: "expand without template",
			bead: &Bead{
				ID:         "bd-001",
				Type:       BeadTypeExpand,
				Title:      "Test",
				Status:     BeadStatusOpen,
				ExpandSpec: &ExpandSpec{},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.bead.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBead_IsEphemeral(t *testing.T) {
	tests := []struct {
		name   string
		labels []string
		want   bool
	}{
		{"no labels", nil, false},
		{"other labels", []string{"test", "priority:high"}, false},
		{"ephemeral label", []string{"meow:ephemeral"}, true},
		{"mixed labels", []string{"test", "meow:ephemeral", "other"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bead := &Bead{Labels: tt.labels}
			if got := bead.IsEphemeral(); got != tt.want {
				t.Errorf("IsEphemeral() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBead_Close(t *testing.T) {
	t.Run("close from in_progress with outputs", func(t *testing.T) {
		bead := &Bead{
			ID:     "bd-001",
			Type:   BeadTypeTask,
			Title:  "Test",
			Status: BeadStatusInProgress,
		}

		outputs := map[string]any{"result": "success"}
		if err := bead.Close(outputs); err != nil {
			t.Fatalf("Close failed: %v", err)
		}

		if bead.Status != BeadStatusClosed {
			t.Errorf("Status = %s, want closed", bead.Status)
		}
		if bead.ClosedAt == nil {
			t.Error("ClosedAt should be set")
		}
		if bead.Outputs["result"] != "success" {
			t.Errorf("Outputs[result] = %v, want success", bead.Outputs["result"])
		}
	})

	t.Run("close from open", func(t *testing.T) {
		bead := &Bead{
			ID:     "bd-001",
			Type:   BeadTypeTask,
			Title:  "Test",
			Status: BeadStatusOpen,
		}

		if err := bead.Close(nil); err != nil {
			t.Fatalf("Close from open failed: %v", err)
		}

		if bead.Status != BeadStatusClosed {
			t.Errorf("Status = %s, want closed", bead.Status)
		}
		if bead.ClosedAt == nil {
			t.Error("ClosedAt should be set")
		}
		if bead.Outputs != nil {
			t.Errorf("Outputs should be nil, got %v", bead.Outputs)
		}
	})

	t.Run("close already closed", func(t *testing.T) {
		bead := &Bead{
			ID:     "bd-001",
			Type:   BeadTypeTask,
			Title:  "Test",
			Status: BeadStatusClosed,
		}

		if err := bead.Close(nil); err == nil {
			t.Error("Expected error when closing already closed bead")
		}
	})
}

func TestCodeSpec_JSONRoundTrip(t *testing.T) {
	spec := &CodeSpec{
		Code:    "echo hello",
		Workdir: "/tmp",
		Env:     map[string]string{"FOO": "bar"},
		Outputs: []OutputSpec{
			{Name: "result", Source: OutputTypeStdout},
			{Name: "log", Source: OutputTypeFile, Path: "/tmp/log.txt"},
		},
		OnError:    OnErrorRetry,
		MaxRetries: 3,
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded CodeSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Code != spec.Code {
		t.Errorf("Code mismatch: got %s, want %s", decoded.Code, spec.Code)
	}
	if len(decoded.Outputs) != 2 {
		t.Errorf("Outputs length mismatch: got %d, want 2", len(decoded.Outputs))
	}
	if decoded.Outputs[1].Path != "/tmp/log.txt" {
		t.Errorf("Output path mismatch: got %s, want /tmp/log.txt", decoded.Outputs[1].Path)
	}
}

func TestTaskOutputSpec_JSONRoundTrip(t *testing.T) {
	spec := &TaskOutputSpec{
		Required: []TaskOutputDef{
			{Name: "work_bead", Type: TaskOutputTypeBeadID, Description: "The bead to implement"},
			{Name: "rationale", Type: TaskOutputTypeString},
		},
		Optional: []TaskOutputDef{
			{Name: "alternative", Type: TaskOutputTypeBeadID},
		},
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded TaskOutputSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if len(decoded.Required) != 2 {
		t.Errorf("Required length mismatch: got %d, want 2", len(decoded.Required))
	}
	if decoded.Required[0].Type != TaskOutputTypeBeadID {
		t.Errorf("Required[0].Type = %s, want bead_id", decoded.Required[0].Type)
	}
	if len(decoded.Optional) != 1 {
		t.Errorf("Optional length mismatch: got %d, want 1", len(decoded.Optional))
	}
}
