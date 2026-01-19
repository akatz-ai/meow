package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRegistry_RequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		registry Registry
		wantErr  string
	}{
		{
			name:     "empty registry",
			registry: Registry{},
			wantErr:  "name is required",
		},
		{
			name: "missing name",
			registry: Registry{
				Version: "1.0.0",
				Owner:   Owner{Name: "test"},
			},
			wantErr: "name is required",
		},
		{
			name: "missing version",
			registry: Registry{
				Name:  "test-registry",
				Owner: Owner{Name: "test"},
			},
			wantErr: "version is required",
		},
		{
			name: "missing owner.name",
			registry: Registry{
				Name:    "test-registry",
				Version: "1.0.0",
				Owner:   Owner{},
			},
			wantErr: "owner.name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateRegistry(&tt.registry)
			if !result.HasErrors() {
				t.Fatal("expected validation errors")
			}
			if !containsError(result.Errors, tt.wantErr) {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, result.Errors)
			}
		})
	}
}

func TestValidateRegistry_NameFormat(t *testing.T) {
	tests := []struct {
		name    string
		regName string
		wantErr bool
	}{
		{"valid kebab-case", "my-registry", false},
		{"valid simple", "registry", false},
		{"valid with numbers", "registry-v2", false},
		{"invalid uppercase", "My-Registry", true},
		{"invalid underscore", "my_registry", true},
		{"invalid starts with number", "2-registry", true},
		{"invalid starts with hyphen", "-registry", true},
		{"invalid ends with hyphen", "registry-", true},
		{"invalid spaces", "my registry", true},
		{"invalid special chars", "my@registry", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Registry{
				Name:    tt.regName,
				Version: "1.0.0",
				Owner:   Owner{Name: "test"},
			}
			result := ValidateRegistry(r)
			hasNameError := containsError(result.Errors, "name must be kebab-case")
			if tt.wantErr && !hasNameError {
				t.Errorf("expected name format error for %q", tt.regName)
			}
			if !tt.wantErr && hasNameError {
				t.Errorf("unexpected name format error for %q", tt.regName)
			}
		})
	}
}

func TestValidateRegistry_DuplicateCollections(t *testing.T) {
	r := &Registry{
		Name:    "test-registry",
		Version: "1.0.0",
		Owner:   Owner{Name: "test"},
		Collections: []CollectionEntry{
			{Name: "collection-a", Source: Source{Path: "./a"}, Description: "First"},
			{Name: "collection-b", Source: Source{Path: "./b"}, Description: "Second"},
			{Name: "collection-a", Source: Source{Path: "./c"}, Description: "Duplicate"},
		},
	}

	result := ValidateRegistry(r)
	if !result.HasErrors() {
		t.Fatal("expected validation errors for duplicate collection names")
	}
	if !containsError(result.Errors, "duplicate collection name") {
		t.Errorf("expected duplicate collection error, got %v", result.Errors)
	}
}

func TestValidateRegistry_CollectionErrors(t *testing.T) {
	tests := []struct {
		name       string
		collection CollectionEntry
		wantErr    string
	}{
		{
			name:       "missing collection name",
			collection: CollectionEntry{Source: Source{Path: "./col"}, Description: "test"},
			wantErr:    "name is required",
		},
		{
			name:       "missing collection source",
			collection: CollectionEntry{Name: "test-col", Description: "test"},
			wantErr:    "source is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Registry{
				Name:        "test-registry",
				Version:     "1.0.0",
				Owner:       Owner{Name: "test"},
				Collections: []CollectionEntry{tt.collection},
			}
			result := ValidateRegistry(r)
			if !result.HasErrors() {
				t.Fatal("expected validation errors")
			}
			if !containsError(result.Errors, tt.wantErr) {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, result.Errors)
			}
		})
	}
}

func TestValidateRegistry_NoCollectionsWarning(t *testing.T) {
	r := &Registry{
		Name:        "test-registry",
		Version:     "1.0.0",
		Owner:       Owner{Name: "test"},
		Collections: []CollectionEntry{},
	}

	result := ValidateRegistry(r)
	if result.HasErrors() {
		t.Errorf("empty collections should not be an error, got %v", result.Errors)
	}
	if !containsWarning(result.Warnings, "no collections") {
		t.Errorf("expected warning about no collections, got %v", result.Warnings)
	}
}

func TestValidateRegistry_ValidRegistry(t *testing.T) {
	r := &Registry{
		Name:        "my-registry",
		Description: "A test registry",
		Version:     "1.0.0",
		Owner:       Owner{Name: "Test Owner", Email: "test@example.com"},
		Collections: []CollectionEntry{
			{
				Name:        "sprint",
				Source:      Source{Path: "./collections/sprint"},
				Description: "Sprint workflows",
				Tags:        []string{"agile", "planning"},
			},
		},
	}

	result := ValidateRegistry(r)
	if result.HasErrors() {
		t.Errorf("valid registry should not have errors, got %v", result.Errors)
	}
}

func TestValidateManifest_RequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		manifest Manifest
		wantErr  string
	}{
		{
			name:     "empty manifest",
			manifest: Manifest{},
			wantErr:  "name is required",
		},
		{
			name: "missing name",
			manifest: Manifest{
				Description: "Test",
				Entrypoint:  "main.meow.toml",
			},
			wantErr: "name is required",
		},
		{
			name: "missing description",
			manifest: Manifest{
				Name:       "test-manifest",
				Entrypoint: "main.meow.toml",
			},
			wantErr: "description is required",
		},
		{
			name: "missing entrypoint",
			manifest: Manifest{
				Name:        "test-manifest",
				Description: "Test",
			},
			wantErr: "entrypoint is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ValidateManifest(&tt.manifest)
			if !result.HasErrors() {
				t.Fatal("expected validation errors")
			}
			if !containsError(result.Errors, tt.wantErr) {
				t.Errorf("expected error containing %q, got %v", tt.wantErr, result.Errors)
			}
		})
	}
}

func TestValidateManifest_NameFormat(t *testing.T) {
	tests := []struct {
		name       string
		maniName   string
		wantErr    bool
	}{
		{"valid kebab-case", "my-collection", false},
		{"valid simple", "collection", false},
		{"invalid uppercase", "My-Collection", true},
		{"invalid underscore", "my_collection", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				Name:        tt.maniName,
				Description: "Test",
				Entrypoint:  "main.meow.toml",
			}
			result := ValidateManifest(m)
			hasNameError := containsError(result.Errors, "name must be kebab-case")
			if tt.wantErr && !hasNameError {
				t.Errorf("expected name format error for %q", tt.maniName)
			}
			if !tt.wantErr && hasNameError {
				t.Errorf("unexpected name format error for %q", tt.maniName)
			}
		})
	}
}

func TestValidateManifest_EntrypointFormat(t *testing.T) {
	tests := []struct {
		name       string
		entrypoint string
		wantErr    bool
	}{
		{"valid meow.toml", "main.meow.toml", false},
		{"valid nested path", "workflows/main.meow.toml", false},
		{"invalid extension", "main.toml", true},
		{"invalid no extension", "main", true},
		{"invalid yaml", "main.yaml", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Manifest{
				Name:        "test-collection",
				Description: "Test",
				Entrypoint:  tt.entrypoint,
			}
			result := ValidateManifest(m)
			hasEntryError := containsError(result.Errors, "entrypoint must be a .meow.toml file")
			if tt.wantErr && !hasEntryError {
				t.Errorf("expected entrypoint format error for %q", tt.entrypoint)
			}
			if !tt.wantErr && hasEntryError {
				t.Errorf("unexpected entrypoint format error for %q", tt.entrypoint)
			}
		})
	}
}

func TestValidateManifest_ValidManifest(t *testing.T) {
	m := &Manifest{
		Name:        "my-collection",
		Description: "A test collection",
		Entrypoint:  "workflows/main.meow.toml",
		Author:      &Author{Name: "Test Author"},
		License:     "MIT",
		Keywords:    []string{"test", "example"},
	}

	result := ValidateManifest(m)
	if result.HasErrors() {
		t.Errorf("valid manifest should not have errors, got %v", result.Errors)
	}
}

func TestValidateCollection_EntrypointExists(t *testing.T) {
	// Create a temp directory with manifest
	dir := t.TempDir()

	m := &Manifest{
		Name:        "test-collection",
		Description: "Test",
		Entrypoint:  "main.meow.toml",
	}

	// Test entrypoint not found
	result := ValidateCollection(dir, m)
	if !containsError(result.Errors, "entrypoint not found") {
		t.Errorf("expected entrypoint not found error, got %v", result.Errors)
	}

	// Create entrypoint file
	entryPath := filepath.Join(dir, "main.meow.toml")
	if err := os.WriteFile(entryPath, []byte("[workflow]\nname = \"test\"\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Test entrypoint found
	result = ValidateCollection(dir, m)
	if containsError(result.Errors, "entrypoint not found") {
		t.Errorf("entrypoint should be found, got %v", result.Errors)
	}
}

func TestValidationResult_Error(t *testing.T) {
	result := &ValidationResult{
		Errors: []ValidationError{
			{Field: "name", Message: "is required"},
			{Field: "version", Message: "is required"},
		},
	}

	errStr := result.Error()
	if errStr == "" {
		t.Error("expected non-empty error string")
	}
	// Should contain both errors
	if !containsSubstring(errStr, "name") || !containsSubstring(errStr, "version") {
		t.Errorf("error string should contain both errors, got %q", errStr)
	}
}

func TestValidationResult_EmptyError(t *testing.T) {
	result := &ValidationResult{}
	if result.Error() != "" {
		t.Error("empty result should return empty error string")
	}
}

// Helper functions

func containsError(errors []ValidationError, substr string) bool {
	for _, e := range errors {
		if containsSubstring(e.Error(), substr) {
			return true
		}
	}
	return false
}

func containsWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if containsSubstring(w, substr) {
			return true
		}
	}
	return false
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
