package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Test fixtures for registry and collection validation

const validRegistry = `{
  "name": "test-registry",
  "description": "A test registry",
  "version": "1.0.0",
  "owner": {
    "name": "Test Owner"
  },
  "collections": [
    {
      "name": "valid-collection",
      "source": "collections/valid",
      "description": "A valid collection"
    }
  ]
}`

const invalidRegistryMissingName = `{
  "description": "Missing name",
  "version": "1.0.0",
  "owner": {
    "name": "Test Owner"
  },
  "collections": []
}`

const invalidRegistryBadCollectionName = `{
  "name": "test-registry",
  "version": "1.0.0",
  "owner": {
    "name": "Test Owner"
  },
  "collections": [
    {
      "name": "Invalid_Name",
      "source": "collections/test",
      "description": "Bad name format"
    }
  ]
}`

const invalidRegistryDuplicateCollection = `{
  "name": "test-registry",
  "version": "1.0.0",
  "owner": {
    "name": "Test Owner"
  },
  "collections": [
    {
      "name": "duplicate",
      "source": "collections/dup1",
      "description": "First"
    },
    {
      "name": "duplicate",
      "source": "collections/dup2",
      "description": "Second"
    }
  ]
}`

const validManifest = `{
  "name": "valid-collection",
  "description": "A valid collection",
  "entrypoint": "workflows/main.meow.toml"
}`

const invalidManifestMissingName = `{
  "description": "Missing name",
  "entrypoint": "workflows/main.meow.toml"
}`

const invalidManifestBadEntrypoint = `{
  "name": "test-collection",
  "description": "Test collection",
  "entrypoint": "main.txt"
}`

const validateTestWorkflow = `
[meta]
name = "test-workflow"
version = "1.0.0"

[[steps]]
id = "start"
executor = "shell"
command = "echo test"
`

// TestRegistryValidate_ValidRegistry tests validating a valid registry
func TestRegistryValidate_ValidRegistry(t *testing.T) {
	// Create temp registry directory
	registryDir := t.TempDir()

	// Create .meow/registry.json
	meowDir := filepath.Join(registryDir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meowDir, "registry.json"), []byte(validRegistry), 0644); err != nil {
		t.Fatal(err)
	}

	// Create the collection directory with manifest
	collectionDir := filepath.Join(registryDir, "collections", "valid")
	collectionMeowDir := filepath.Join(collectionDir, ".meow")
	if err := os.MkdirAll(collectionMeowDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collectionMeowDir, "manifest.json"), []byte(validManifest), 0644); err != nil {
		t.Fatal(err)
	}

	// Create entrypoint file
	workflowsDir := filepath.Join(collectionDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowsDir, "main.meow.toml"), []byte(validateTestWorkflow), 0644); err != nil {
		t.Fatal(err)
	}

	// Set up command output capture
	var out bytes.Buffer
	registryValidateCmd.SetOut(&out)
	registryValidateCmd.SetErr(&out)

	// Run validate command
	if err := runRegistryValidate(registryValidateCmd, []string{registryDir}); err != nil {
		t.Fatalf("Expected valid registry to pass, got error: %v\nOutput: %s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "✓") {
		t.Errorf("Expected success indicator in output, got: %s", output)
	}
	if !strings.Contains(output, "valid") {
		t.Errorf("Expected 'valid' in output, got: %s", output)
	}
}

// TestRegistryValidate_InvalidRegistry tests validating an invalid registry
func TestRegistryValidate_InvalidRegistry(t *testing.T) {
	tests := []struct {
		name           string
		registryJSON   string
		expectedInErr  string
	}{
		{
			name:          "missing name",
			registryJSON:  invalidRegistryMissingName,
			expectedInErr: "name",
		},
		{
			name:          "bad collection name format",
			registryJSON:  invalidRegistryBadCollectionName,
			expectedInErr: "kebab-case",
		},
		{
			name:          "duplicate collection",
			registryJSON:  invalidRegistryDuplicateCollection,
			expectedInErr: "duplicate",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registryDir := t.TempDir()

			meowDir := filepath.Join(registryDir, ".meow")
			if err := os.MkdirAll(meowDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(meowDir, "registry.json"), []byte(tt.registryJSON), 0644); err != nil {
				t.Fatal(err)
			}

			var out bytes.Buffer
			registryValidateCmd.SetOut(&out)
			registryValidateCmd.SetErr(&out)

			err := runRegistryValidate(registryValidateCmd, []string{registryDir})
			if err == nil {
				t.Fatalf("Expected error for invalid registry, got nil\nOutput: %s", out.String())
			}

			errMsg := err.Error() + out.String()
			if !strings.Contains(errMsg, tt.expectedInErr) {
				t.Errorf("Expected error to contain %q, got: %s", tt.expectedInErr, errMsg)
			}
		})
	}
}

// TestRegistryValidate_CurrentDir tests validating current directory
func TestRegistryValidate_CurrentDir(t *testing.T) {
	registryDir := t.TempDir()

	// Change to registry directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(registryDir); err != nil {
		t.Fatal(err)
	}

	// Create .meow/registry.json
	meowDir := filepath.Join(registryDir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meowDir, "registry.json"), []byte(validRegistry), 0644); err != nil {
		t.Fatal(err)
	}

	// Create collection
	collectionDir := filepath.Join(registryDir, "collections", "valid")
	collectionMeowDir := filepath.Join(collectionDir, ".meow")
	if err := os.MkdirAll(collectionMeowDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(collectionMeowDir, "manifest.json"), []byte(validManifest), 0644); err != nil {
		t.Fatal(err)
	}

	workflowsDir := filepath.Join(collectionDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowsDir, "main.meow.toml"), []byte(validateTestWorkflow), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	registryValidateCmd.SetOut(&out)
	registryValidateCmd.SetErr(&out)

	// Run validate without args (should default to ".")
	if err := runRegistryValidate(registryValidateCmd, []string{}); err != nil {
		t.Fatalf("Expected valid registry to pass, got error: %v\nOutput: %s", err, out.String())
	}
}

// TestCollectionValidate_ValidCollection tests validating a valid collection
func TestCollectionValidate_ValidCollection(t *testing.T) {
	collectionDir := t.TempDir()

	// Create .meow/manifest.json
	meowDir := filepath.Join(collectionDir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meowDir, "manifest.json"), []byte(validManifest), 0644); err != nil {
		t.Fatal(err)
	}

	// Create entrypoint file
	workflowsDir := filepath.Join(collectionDir, "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workflowsDir, "main.meow.toml"), []byte(validateTestWorkflow), 0644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	collectionValidateCmd.SetOut(&out)
	collectionValidateCmd.SetErr(&out)

	// Run validate command
	if err := runRegistryValidate(collectionValidateCmd, []string{collectionDir}); err != nil {
		t.Fatalf("Expected valid collection to pass, got error: %v\nOutput: %s", err, out.String())
	}

	output := out.String()
	if !strings.Contains(output, "✓") {
		t.Errorf("Expected success indicator in output, got: %s", output)
	}
	if !strings.Contains(output, "valid-collection") {
		t.Errorf("Expected collection name in output, got: %s", output)
	}
}

// TestCollectionValidate_InvalidCollection tests validating an invalid collection
func TestCollectionValidate_InvalidCollection(t *testing.T) {
	tests := []struct {
		name          string
		manifestJSON  string
		createEntry   bool
		expectedInErr string
	}{
		{
			name:          "missing name",
			manifestJSON:  invalidManifestMissingName,
			createEntry:   true,
			expectedInErr: "name",
		},
		{
			name:          "bad entrypoint extension",
			manifestJSON:  invalidManifestBadEntrypoint,
			createEntry:   true,
			expectedInErr: ".meow.toml",
		},
		{
			name:          "missing entrypoint file",
			manifestJSON:  validManifest,
			createEntry:   false, // Don't create the entrypoint file
			expectedInErr: "not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collectionDir := t.TempDir()

			meowDir := filepath.Join(collectionDir, ".meow")
			if err := os.MkdirAll(meowDir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(meowDir, "manifest.json"), []byte(tt.manifestJSON), 0644); err != nil {
				t.Fatal(err)
			}

			if tt.createEntry {
				workflowsDir := filepath.Join(collectionDir, "workflows")
				if err := os.MkdirAll(workflowsDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(workflowsDir, "main.meow.toml"), []byte(testWorkflow), 0644); err != nil {
					t.Fatal(err)
				}
			}

			var out bytes.Buffer
			collectionValidateCmd.SetOut(&out)
			collectionValidateCmd.SetErr(&out)

			err := runRegistryValidate(collectionValidateCmd, []string{collectionDir})
			if err == nil {
				t.Fatalf("Expected error for invalid collection, got nil\nOutput: %s", out.String())
			}

			errMsg := err.Error() + out.String()
			if !strings.Contains(errMsg, tt.expectedInErr) {
				t.Errorf("Expected error to contain %q, got: %s", tt.expectedInErr, errMsg)
			}
		})
	}
}

// TestValidate_NoManifestOrRegistry tests error when neither file exists
func TestValidate_NoManifestOrRegistry(t *testing.T) {
	emptyDir := t.TempDir()

	var out bytes.Buffer
	registryValidateCmd.SetOut(&out)
	registryValidateCmd.SetErr(&out)

	err := runRegistryValidate(registryValidateCmd, []string{emptyDir})
	if err == nil {
		t.Fatal("Expected error when no registry.json or manifest.json found")
	}

	errMsg := err.Error()
	if !strings.Contains(errMsg, "no registry.json or manifest.json") {
		t.Errorf("Expected error about missing files, got: %s", errMsg)
	}
}

// TestRegistryValidate_MissingCollectionManifest tests strict validation
func TestRegistryValidate_MissingCollectionManifest(t *testing.T) {
	registryDir := t.TempDir()

	// Registry with a collection that has no manifest
	registryWithMissingManifest := `{
  "name": "test-registry",
  "version": "1.0.0",
  "owner": {
    "name": "Test Owner"
  },
  "collections": [
    {
      "name": "no-manifest",
      "source": "collections/missing",
      "description": "Collection without manifest"
    }
  ]
}`

	meowDir := filepath.Join(registryDir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(meowDir, "registry.json"), []byte(registryWithMissingManifest), 0644); err != nil {
		t.Fatal(err)
	}

	// Create collection dir but no manifest
	collectionDir := filepath.Join(registryDir, "collections", "missing")
	if err := os.MkdirAll(collectionDir, 0755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	registryValidateCmd.SetOut(&out)
	registryValidateCmd.SetErr(&out)

	err := runRegistryValidate(registryValidateCmd, []string{registryDir})
	if err == nil {
		t.Fatal("Expected error for missing manifest with strict=true")
	}

	errMsg := err.Error() + out.String()
	if !strings.Contains(errMsg, "missing") && !strings.Contains(errMsg, "manifest") {
		t.Errorf("Expected error about missing manifest, got: %s", errMsg)
	}
}
