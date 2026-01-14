package collection

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalWorkflow = `
[meta]
name = "explore"
version = "1.0.0"

[[steps]]
id = "step-1"
executor = "shell"
command = "echo hello"
`

const minimalManifest = `
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
`

func TestParseMinimalCollection(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "meow-collection.toml")

	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)
	writeFile(t, manifestPath, minimalManifest)

	collection, err := ParseFile(manifestPath)
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if collection.Collection.Name != "my-workflows" {
		t.Errorf("Name = %q, want %q", collection.Collection.Name, "my-workflows")
	}
	if collection.Collection.Description != "My workflows" {
		t.Errorf("Description = %q, want %q", collection.Collection.Description, "My workflows")
	}
	if collection.Collection.Owner.Name != "Test User" {
		t.Errorf("Owner.Name = %q, want %q", collection.Collection.Owner.Name, "Test User")
	}
	if len(collection.Packs) != 1 {
		t.Fatalf("Packs len = %d, want 1", len(collection.Packs))
	}
	if collection.Packs[0].Name != "all" {
		t.Errorf("Packs[0].Name = %q, want %q", collection.Packs[0].Name, "all")
	}
}

func TestParseFullCollection(t *testing.T) {
	dir := t.TempDir()
	manifest := `
[collection]
name = "enterprise-workflows"
description = "Enterprise workflows"
version = "2.1.0"
meow_version = ">=0.2.0"

[collection.owner]
name = "Acme Corp"
email = "devtools@acme.com"
url = "https://acme.com"

[collection.repository]
url = "https://github.com/acme/meow-enterprise"
license = "Apache-2.0"

[[packs]]
name = "ci-cd"
description = "Continuous integration"
workflows = ["ci/build.meow.toml"]

[[packs]]
name = "security"
description = "Security workflows"
workflows = ["security/scan.meow.toml"]
`

	writeFile(t, filepath.Join(dir, "ci", "build.meow.toml"), minimalWorkflow)
	writeFile(t, filepath.Join(dir, "security", "scan.meow.toml"), minimalWorkflow)
	writeFile(t, filepath.Join(dir, "meow-collection.toml"), manifest)

	collection, err := ParseFile(filepath.Join(dir, "meow-collection.toml"))
	if err != nil {
		t.Fatalf("ParseFile() error = %v", err)
	}

	if collection.Collection.MeowVersion != ">=0.2.0" {
		t.Errorf("MeowVersion = %q, want %q", collection.Collection.MeowVersion, ">=0.2.0")
	}
	if collection.Collection.Owner.Email != "devtools@acme.com" {
		t.Errorf("Owner.Email = %q, want %q", collection.Collection.Owner.Email, "devtools@acme.com")
	}
	if collection.Collection.Repository == nil {
		t.Fatalf("Repository should not be nil")
	}
	if collection.Collection.Repository.License != "Apache-2.0" {
		t.Errorf("Repository.License = %q, want %q", collection.Collection.Repository.License, "Apache-2.0")
	}
	if len(collection.Packs) != 2 {
		t.Fatalf("Packs len = %d, want 2", len(collection.Packs))
	}
	if collection.Packs[1].Name != "security" {
		t.Errorf("Packs[1].Name = %q, want %q", collection.Packs[1].Name, "security")
	}
}

func TestValidateMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	result := (&Collection{}).Validate(dir)
	if !result.HasErrors() {
		t.Fatalf("expected validation errors")
	}

	expected := []string{
		"collection.name is required",
		"collection.description is required",
		"collection.version is required",
		"collection.owner.name is required",
		"packs are required",
	}

	for _, msg := range expected {
		if !containsError(result, msg) {
			t.Errorf("expected error containing %q", msg)
		}
	}
}

func TestValidateInvalidNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	collection := &Collection{
		Collection: CollectionMeta{
			Name:        "BadName",
			Description: "desc",
			Version:     "0.1.0",
			Owner:       Owner{Name: "Tester"},
		},
		Packs: []Pack{
			{
				Name:        "bad_name",
				Description: "desc",
				Workflows:   []string{"workflows/explore.meow.toml"},
			},
		},
	}

	result := collection.Validate(dir)
	if !result.HasErrors() {
		t.Fatalf("expected validation errors")
	}
	if !containsError(result, "collection.name must be lowercase") {
		t.Errorf("expected collection name error")
	}
	if !containsError(result, "packs[0].name must be lowercase") {
		t.Errorf("expected pack name error")
	}
}

func TestValidateWorkflowPathsMissing(t *testing.T) {
	dir := t.TempDir()
	collection := &Collection{
		Collection: CollectionMeta{
			Name:        "my-workflows",
			Description: "desc",
			Version:     "0.1.0",
			Owner:       Owner{Name: "Tester"},
		},
		Packs: []Pack{
			{
				Name:        "all",
				Description: "desc",
				Workflows:   []string{"workflows/missing.meow.toml"},
			},
		},
	}

	result := collection.Validate(dir)
	if !result.HasErrors() {
		t.Fatalf("expected validation errors")
	}
	if !containsError(result, "workflow path does not exist") {
		t.Errorf("expected workflow path error")
	}
}

func TestValidateMeowVersionInvalid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)

	collection := &Collection{
		Collection: CollectionMeta{
			Name:        "my-workflows",
			Description: "desc",
			Version:     "0.1.0",
			MeowVersion: ">=bad",
			Owner:       Owner{Name: "Tester"},
		},
		Packs: []Pack{
			{
				Name:        "all",
				Description: "desc",
				Workflows:   []string{"workflows/explore.meow.toml"},
			},
		},
	}

	result := collection.Validate(dir)
	if !result.HasErrors() {
		t.Fatalf("expected validation errors")
	}
	if !containsError(result, "meow_version constraint is invalid") {
		t.Errorf("expected meow_version error")
	}
}

func TestLoadFromDirIntegration(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "workflows", "explore.meow.toml"), minimalWorkflow)
	writeFile(t, filepath.Join(dir, "meow-collection.toml"), minimalManifest)

	collection, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir() error = %v", err)
	}
	if collection.Collection.Name != "my-workflows" {
		t.Errorf("Name = %q, want %q", collection.Collection.Name, "my-workflows")
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
}

func containsError(result *ValidationResult, substr string) bool {
	for _, err := range result.Errors {
		if strings.Contains(err.Error(), substr) {
			return true
		}
	}
	return false
}
