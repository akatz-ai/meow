package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateValidSkill(t *testing.T) {
	tmpDir := t.TempDir()

	// Create skill directory with matching name
	skillDir := filepath.Join(tmpDir, "my-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "my-skill",
			Description: "A valid test skill",
			Version:     "1.0.0",
		},
		Targets: map[string]Target{
			"claude": {},
		},
	}

	result := skill.Validate(skillDir)

	if result.HasErrors() {
		t.Errorf("expected no errors for valid skill, got: %v", result.Error())
	}
}

func TestValidateMissingName(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "",
			Description: "Missing name",
		},
		Targets: map[string]Target{
			"claude": {},
		},
	}

	result := skill.Validate(skillDir)

	if !result.HasErrors() {
		t.Fatal("expected error for missing name")
	}

	if !strings.Contains(result.Error(), "skill.name") {
		t.Errorf("expected error about skill.name, got: %v", result.Error())
	}
}

func TestValidateInvalidNameFormat(t *testing.T) {
	tests := []struct {
		name    string
		invalid bool
	}{
		{"valid-name", false},
		{"also-valid", false},
		{"simple", false},
		{"UPPERCASE", true},
		{"has_underscore", true},
		{"has.dot", true},
		{"has space", true},
		{"has@symbol", true},
		{"-starts-with-hyphen", true},
		{"ends-with-hyphen-", true},
		{"has--double-hyphen", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			skillDir := filepath.Join(tmpDir, tt.name)
			if err := os.MkdirAll(skillDir, 0755); err != nil {
				t.Fatal(err)
			}

			skill := &Skill{
				Skill: SkillMeta{
					Name:        tt.name,
					Description: "Test skill",
				},
				Targets: map[string]Target{
					"claude": {},
				},
			}

			result := skill.Validate(skillDir)

			if tt.invalid && !result.HasErrors() {
				t.Errorf("expected error for invalid name %q", tt.name)
			}

			if !tt.invalid && result.HasErrors() {
				t.Errorf("expected no error for valid name %q, got: %v", tt.name, result.Error())
			}
		})
	}
}

func TestValidateNameMismatchDirectory(t *testing.T) {
	tmpDir := t.TempDir()

	// Create skill directory with different name
	skillDir := filepath.Join(tmpDir, "actual-dir-name")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "different-name",
			Description: "Name doesn't match directory",
		},
		Targets: map[string]Target{
			"claude": {},
		},
	}

	result := skill.Validate(skillDir)

	if !result.HasErrors() {
		t.Fatal("expected error when name doesn't match directory")
	}

	errStr := result.Error()
	if !strings.Contains(errStr, "skill.name") || !strings.Contains(errStr, "directory") {
		t.Errorf("expected error about name/directory mismatch, got: %v", errStr)
	}
}

func TestValidateMissingDescription(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "test-skill",
			Description: "",
		},
		Targets: map[string]Target{
			"claude": {},
		},
	}

	result := skill.Validate(skillDir)

	if !result.HasErrors() {
		t.Fatal("expected error for missing description")
	}

	if !strings.Contains(result.Error(), "skill.description") {
		t.Errorf("expected error about skill.description, got: %v", result.Error())
	}
}

func TestValidateDescriptionTooLong(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create description longer than 1024 chars
	longDesc := strings.Repeat("a", 1025)

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "test-skill",
			Description: longDesc,
		},
		Targets: map[string]Target{
			"claude": {},
		},
	}

	result := skill.Validate(skillDir)

	if !result.HasErrors() {
		t.Fatal("expected error for description > 1024 chars")
	}

	if !strings.Contains(result.Error(), "skill.description") {
		t.Errorf("expected error about skill.description, got: %v", result.Error())
	}
}

func TestValidateInvalidVersion(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		version string
		valid   bool
	}{
		{"1.0.0", true},
		{"0.0.1", true},
		{"10.20.30", true},
		{"", true}, // Empty version is allowed (optional)
		{"1.0", false},
		{"1", false},
		{"v1.0.0", false},
		{"1.0.0-beta", false},
		{"not-semver", false},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			skill := &Skill{
				Skill: SkillMeta{
					Name:        "test-skill",
					Description: "Test skill",
					Version:     tt.version,
				},
				Targets: map[string]Target{
					"claude": {},
				},
			}

			result := skill.Validate(skillDir)

			if tt.valid && result.HasErrors() {
				t.Errorf("expected no error for version %q, got: %v", tt.version, result.Error())
			}

			if !tt.valid && !result.HasErrors() {
				t.Errorf("expected error for invalid version %q", tt.version)
			}
		})
	}
}

func TestValidateNoTargets(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "test-skill",
			Description: "No targets",
		},
		Targets: map[string]Target{},
	}

	result := skill.Validate(skillDir)

	if !result.HasErrors() {
		t.Fatal("expected error when no targets defined")
	}

	if !strings.Contains(result.Error(), "targets") {
		t.Errorf("expected error about targets, got: %v", result.Error())
	}
}

func TestValidateCustomPathWithoutPlaceholder(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "test-skill",
			Description: "Custom path without {{name}}",
		},
		Targets: map[string]Target{
			"custom": {
				Path: "/custom/path/without/placeholder",
			},
		},
	}

	result := skill.Validate(skillDir)

	if !result.HasErrors() {
		t.Fatal("expected error when custom path doesn't contain {{name}}")
	}

	if !strings.Contains(result.Error(), "{{name}}") {
		t.Errorf("expected error about {{name}} placeholder, got: %v", result.Error())
	}
}

func TestValidateCustomPathWithPlaceholder(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "test-skill",
			Description: "Custom path with placeholder",
		},
		Targets: map[string]Target{
			"custom": {
				Path: "/custom/path/{{name}}",
			},
		},
	}

	result := skill.Validate(skillDir)

	// Should not error specifically about the path placeholder
	for _, err := range result.Errors {
		if strings.Contains(err.Message, "{{name}}") {
			t.Errorf("unexpected error about {{name}}: %v", err)
		}
	}
}

func TestValidateMissingFiles(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "test-skill",
			Description: "Has files list with missing file",
			Files:       []string{"exists.md", "missing.md"},
		},
		Targets: map[string]Target{
			"claude": {},
		},
	}

	// Create only one of the files
	if err := os.WriteFile(filepath.Join(skillDir, "exists.md"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	result := skill.Validate(skillDir)

	if !result.HasErrors() {
		t.Fatal("expected error for missing file")
	}

	if !strings.Contains(result.Error(), "missing.md") {
		t.Errorf("expected error about missing.md, got: %v", result.Error())
	}
}

func TestValidateFilesExist(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "test-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "test-skill",
			Description: "Has files list with existing files",
			Files:       []string{"file1.md", "file2.md"},
		},
		Targets: map[string]Target{
			"claude": {},
		},
	}

	// Create both files
	if err := os.WriteFile(filepath.Join(skillDir, "file1.md"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "file2.md"), []byte("content"), 0644); err != nil {
		t.Fatal(err)
	}

	result := skill.Validate(skillDir)

	if result.HasErrors() {
		t.Errorf("expected no errors when all files exist, got: %v", result.Error())
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "wrong-name")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	skill := &Skill{
		Skill: SkillMeta{
			Name:        "INVALID_NAME",
			Description: "",
			Version:     "not-semver",
			Files:       []string{"missing.md"},
		},
		Targets: map[string]Target{},
	}

	result := skill.Validate(skillDir)

	if !result.HasErrors() {
		t.Fatal("expected multiple errors")
	}

	// Should aggregate all errors, not fail fast
	errStr := result.Error()
	errorChecks := []string{
		"skill.name",       // Invalid format AND doesn't match directory
		"skill.description", // Missing
		"skill.version",    // Invalid semver
		"targets",          // No targets
	}

	for _, check := range errorChecks {
		if !strings.Contains(errStr, check) {
			t.Errorf("expected error to contain %q, got: %v", check, errStr)
		}
	}
}

func TestValidationResultInterface(t *testing.T) {
	t.Run("HasErrors returns false for empty result", func(t *testing.T) {
		result := &ValidationResult{}
		if result.HasErrors() {
			t.Error("HasErrors should return false for empty result")
		}
	})

	t.Run("HasErrors returns true when errors exist", func(t *testing.T) {
		result := &ValidationResult{}
		result.Add("field", "message")
		if !result.HasErrors() {
			t.Error("HasErrors should return true when errors exist")
		}
	})

	t.Run("Error returns empty string for no errors", func(t *testing.T) {
		result := &ValidationResult{}
		if result.Error() != "" {
			t.Errorf("Error() should return empty string, got: %q", result.Error())
		}
	})

	t.Run("Error formats multiple errors", func(t *testing.T) {
		result := &ValidationResult{}
		result.Add("field1", "message1")
		result.Add("field2", "message2")

		errStr := result.Error()
		if !strings.Contains(errStr, "field1") || !strings.Contains(errStr, "field2") {
			t.Errorf("Error() should contain all field names, got: %q", errStr)
		}
	})
}
