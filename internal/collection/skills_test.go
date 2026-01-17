package collection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for meow-fxtb: Extend collection types to support skills section

const validSkillTOML = `
[skill]
name = "sprint-planner"
description = "Plan and execute sprints"
version = "1.0.0"

[targets.claude]
`

func TestParseCollectionWithSkills(t *testing.T) {
	dir := t.TempDir()

	// Create a valid workflow
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	// Create a valid skill
	skillDir := filepath.Join(dir, "skills", "sprint-planner")
	os.MkdirAll(skillDir, 0755)
	writeFile(t, filepath.Join(skillDir, "skill.toml"), validSkillTOML)

	// Create collection manifest with skills section
	manifest := `
[collection]
name = "my-collection"
description = "My collection with skills"
version = "0.1.0"

[collection.owner]
name = "Test User"

[[packs]]
name = "all"
description = "All workflows"
workflows = ["workflows/explore.meow.toml"]

[skills]
sprint-planner = "skills/sprint-planner/skill.toml"
`
	writeFile(t, filepath.Join(dir, "meow-collection.toml"), manifest)

	collection, err := ParseFile(filepath.Join(dir, "meow-collection.toml"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Verify skills section was parsed
	if collection.Skills == nil {
		t.Fatal("Skills should not be nil")
	}
	if len(collection.Skills) != 1 {
		t.Fatalf("Skills len = %d, want 1", len(collection.Skills))
	}
	if collection.Skills["sprint-planner"] != "skills/sprint-planner/skill.toml" {
		t.Errorf("Skills[sprint-planner] = %q, want %q",
			collection.Skills["sprint-planner"], "skills/sprint-planner/skill.toml")
	}
}

func TestParseCollectionWithMultipleSkills(t *testing.T) {
	dir := t.TempDir()

	// Create workflows
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	// Create multiple skills
	for _, name := range []string{"sprint-planner", "workflow-helper", "code-review"} {
		skillDir := filepath.Join(dir, "skills", name)
		os.MkdirAll(skillDir, 0755)
		skillToml := strings.Replace(validSkillTOML, "sprint-planner", name, 1)
		writeFile(t, filepath.Join(skillDir, "skill.toml"), skillToml)
	}

	manifest := `
[collection]
name = "multi-skill-collection"
description = "Collection with multiple skills"
version = "0.1.0"

[collection.owner]
name = "Test User"

[[packs]]
name = "all"
description = "All workflows"
workflows = ["workflows/explore.meow.toml"]

[skills]
sprint-planner = "skills/sprint-planner/skill.toml"
workflow-helper = "skills/workflow-helper/skill.toml"
code-review = "skills/code-review/skill.toml"
`
	writeFile(t, filepath.Join(dir, "meow-collection.toml"), manifest)

	collection, err := ParseFile(filepath.Join(dir, "meow-collection.toml"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if len(collection.Skills) != 3 {
		t.Errorf("Skills len = %d, want 3", len(collection.Skills))
	}
}

func TestParseCollectionWithoutSkills(t *testing.T) {
	// Existing collections without skills section should still work
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)
	writeFile(t, filepath.Join(dir, "meow-collection.toml"), minimalManifest)

	collection, err := ParseFile(filepath.Join(dir, "meow-collection.toml"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	// Skills should be nil or empty map when not specified
	if collection.Skills != nil && len(collection.Skills) != 0 {
		t.Errorf("Skills should be nil or empty when not specified, got: %v", collection.Skills)
	}
}

func TestValidateCollectionSkillPathExists(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	// Create collection with skill path that doesn't exist
	collection := &Collection{
		Collection: CollectionMeta{
			Name:        "my-collection",
			Description: "desc",
			Version:     "0.1.0",
			Owner:       Owner{Name: "Tester"},
		},
		Packs: []Pack{
			{
				Name:        "all",
				Description: "desc",
				Workflows:   []string{"workflows/explore.meow.toml"},
			},
		},
		Skills: map[string]string{
			"missing-skill": "skills/missing-skill/skill.toml",
		},
	}

	result := collection.Validate(dir)
	if !result.HasErrors() {
		t.Fatal("expected validation errors for missing skill path")
	}
	if !containsError(result, "skill path does not exist") {
		t.Errorf("expected 'skill path does not exist' error, got: %v", result.Errors)
	}
}

func TestValidateCollectionSkillManifestInvalid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	// Create skill with invalid manifest (missing required fields)
	skillDir := filepath.Join(dir, "skills", "bad-skill")
	os.MkdirAll(skillDir, 0755)
	invalidSkillTOML := `
[skill]
# Missing name and description
version = "1.0.0"

[targets.claude]
`
	writeFile(t, filepath.Join(skillDir, "skill.toml"), invalidSkillTOML)

	collection := &Collection{
		Collection: CollectionMeta{
			Name:        "my-collection",
			Description: "desc",
			Version:     "0.1.0",
			Owner:       Owner{Name: "Tester"},
		},
		Packs: []Pack{
			{
				Name:        "all",
				Description: "desc",
				Workflows:   []string{"workflows/explore.meow.toml"},
			},
		},
		Skills: map[string]string{
			"bad-skill": "skills/bad-skill/skill.toml",
		},
	}

	result := collection.Validate(dir)
	if !result.HasErrors() {
		t.Fatal("expected validation errors for invalid skill manifest")
	}
	// Should report skill validation errors
	errStr := result.Error()
	if !strings.Contains(errStr, "skill") {
		t.Errorf("expected skill validation error, got: %v", result.Errors)
	}
}

func TestValidateCollectionSkillNameMustMatchKey(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	// Create skill where manifest name doesn't match the map key
	skillDir := filepath.Join(dir, "skills", "mismatched")
	os.MkdirAll(skillDir, 0755)
	// skill.toml says name is "different-name"
	mismatchedSkillTOML := `
[skill]
name = "mismatched"
description = "Skill with mismatched name"
version = "1.0.0"

[targets.claude]
`
	writeFile(t, filepath.Join(skillDir, "skill.toml"), mismatchedSkillTOML)

	collection := &Collection{
		Collection: CollectionMeta{
			Name:        "my-collection",
			Description: "desc",
			Version:     "0.1.0",
			Owner:       Owner{Name: "Tester"},
		},
		Packs: []Pack{
			{
				Name:        "all",
				Description: "desc",
				Workflows:   []string{"workflows/explore.meow.toml"},
			},
		},
		Skills: map[string]string{
			// Key is "wrong-key" but skill.toml says "mismatched"
			"wrong-key": "skills/mismatched/skill.toml",
		},
	}

	result := collection.Validate(dir)
	if !result.HasErrors() {
		t.Fatal("expected validation error when skill name doesn't match key")
	}
	if !containsError(result, "name") && !containsError(result, "mismatch") {
		t.Errorf("expected name mismatch error, got: %v", result.Errors)
	}
}

func TestValidateCollectionEmptySkillsIsValid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	collection := &Collection{
		Collection: CollectionMeta{
			Name:        "my-collection",
			Description: "desc",
			Version:     "0.1.0",
			Owner:       Owner{Name: "Tester"},
		},
		Packs: []Pack{
			{
				Name:        "all",
				Description: "desc",
				Workflows:   []string{"workflows/explore.meow.toml"},
			},
		},
		Skills: map[string]string{}, // Empty skills map
	}

	result := collection.Validate(dir)
	if result.HasErrors() {
		t.Errorf("empty skills section should be valid, got errors: %v", result.Errors)
	}
}

func TestValidateCollectionSkillAbsolutePathInvalid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	collection := &Collection{
		Collection: CollectionMeta{
			Name:        "my-collection",
			Description: "desc",
			Version:     "0.1.0",
			Owner:       Owner{Name: "Tester"},
		},
		Packs: []Pack{
			{
				Name:        "all",
				Description: "desc",
				Workflows:   []string{"workflows/explore.meow.toml"},
			},
		},
		Skills: map[string]string{
			"bad-path": "/absolute/path/skill.toml",
		},
	}

	result := collection.Validate(dir)
	if !result.HasErrors() {
		t.Fatal("expected validation error for absolute skill path")
	}
	if !containsError(result, "relative") || !containsError(result, "path") {
		t.Errorf("expected relative path error, got: %v", result.Errors)
	}
}

func TestValidateCollectionSkillPathMustEndWithSkillToml(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	collection := &Collection{
		Collection: CollectionMeta{
			Name:        "my-collection",
			Description: "desc",
			Version:     "0.1.0",
			Owner:       Owner{Name: "Tester"},
		},
		Packs: []Pack{
			{
				Name:        "all",
				Description: "desc",
				Workflows:   []string{"workflows/explore.meow.toml"},
			},
		},
		Skills: map[string]string{
			"wrong-extension": "skills/my-skill/manifest.toml",
		},
	}

	result := collection.Validate(dir)
	if !result.HasErrors() {
		t.Fatal("expected validation error for wrong skill manifest filename")
	}
	if !containsError(result, "skill.toml") {
		t.Errorf("expected 'skill.toml' error, got: %v", result.Errors)
	}
}

func TestValidateCollectionWithValidSkill(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	// Create a fully valid skill
	skillDir := filepath.Join(dir, "skills", "valid-skill")
	os.MkdirAll(skillDir, 0755)
	writeFile(t, filepath.Join(skillDir, "skill.toml"), `
[skill]
name = "valid-skill"
description = "A perfectly valid skill"
version = "1.0.0"

[targets.claude]
`)

	collection := &Collection{
		Collection: CollectionMeta{
			Name:        "my-collection",
			Description: "desc",
			Version:     "0.1.0",
			Owner:       Owner{Name: "Tester"},
		},
		Packs: []Pack{
			{
				Name:        "all",
				Description: "desc",
				Workflows:   []string{"workflows/explore.meow.toml"},
			},
		},
		Skills: map[string]string{
			"valid-skill": "skills/valid-skill/skill.toml",
		},
	}

	result := collection.Validate(dir)
	if result.HasErrors() {
		t.Errorf("valid skill should pass validation, got errors: %v", result.Errors)
	}
}

func TestLoadFromDirWithSkills(t *testing.T) {
	dir := t.TempDir()

	// Create workflow
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	// Create skill
	skillDir := filepath.Join(dir, "skills", "my-skill")
	os.MkdirAll(skillDir, 0755)
	writeFile(t, filepath.Join(skillDir, "skill.toml"), `
[skill]
name = "my-skill"
description = "My skill"

[targets.claude]
`)

	// Create manifest with skills
	manifest := `
[collection]
name = "my-workflows"
description = "My workflows"
version = "0.1.0"

[collection.owner]
name = "Test User"

[[packs]]
name = "all"
description = "All workflows"
workflows = ["workflows/explore.meow.toml"]

[skills]
my-skill = "skills/my-skill/skill.toml"
`
	writeFile(t, filepath.Join(dir, "meow-collection.toml"), manifest)

	collection, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}
	if collection.Skills == nil || collection.Skills["my-skill"] == "" {
		t.Errorf("Skills should contain 'my-skill', got: %v", collection.Skills)
	}
}
