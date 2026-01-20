package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/akatz-ai/meow/internal/registry"
)

// setupTestRegistry creates a test registry with a collection
func setupTestRegistry(t *testing.T, registryName, collectionName string) (cache *registry.Cache, cleanup func()) {
	t.Helper()

	// Create cache in temp dir
	tempCache := t.TempDir()
	cache = &registry.Cache{}
	// We'll need to set the cache base dir somehow - for now just use temp

	// Create registry directory structure
	regDir := filepath.Join(tempCache, "registries", registryName)
	if err := os.MkdirAll(regDir, 0755); err != nil {
		t.Fatalf("creating registry dir: %v", err)
	}

	// Create .meow/registry.json
	reg := registry.Registry{
		Name:           registryName,
		Description:    "Test Registry",
		Version:        "1.0.0",
		CollectionRoot: "collections",
		Owner: registry.Owner{
			Name: "Test Owner",
		},
		Collections: []registry.CollectionEntry{
			{
				Name:        collectionName,
				Description: "Test Collection",
				Source: registry.Source{
					Path: collectionName,
				},
			},
		},
	}

	meowDir := filepath.Join(regDir, ".meow")
	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("creating .meow dir: %v", err)
	}

	regData, _ := json.MarshalIndent(reg, "", "  ")
	if err := os.WriteFile(filepath.Join(meowDir, "registry.json"), regData, 0644); err != nil {
		t.Fatalf("writing registry.json: %v", err)
	}

	// Create collection directory
	collectionDir := filepath.Join(regDir, "collections", collectionName)
	if err := os.MkdirAll(collectionDir, 0755); err != nil {
		t.Fatalf("creating collection dir: %v", err)
	}

	// Create .meow/manifest.json
	manifest := registry.Manifest{
		Name:        collectionName,
		Description: "Test collection",
		Entrypoint:  "main.meow.toml",
	}

	collMeowDir := filepath.Join(collectionDir, ".meow")
	if err := os.MkdirAll(collMeowDir, 0755); err != nil {
		t.Fatalf("creating collection .meow dir: %v", err)
	}

	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(collMeowDir, "manifest.json"), manifestData, 0644); err != nil {
		t.Fatalf("writing manifest.json: %v", err)
	}

	// Create main workflow file
	workflowContent := `[[steps]]
id = "test"
executor = "shell"
command = "echo test"
`
	if err := os.WriteFile(filepath.Join(collectionDir, "main.meow.toml"), []byte(workflowContent), 0644); err != nil {
		t.Fatalf("writing workflow file: %v", err)
	}

	// Create a .git directory to test exclusion
	gitDir := filepath.Join(collectionDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("creating .git dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("# git config"), 0644); err != nil {
		t.Fatalf("writing git config: %v", err)
	}

	cleanup = func() {
		os.RemoveAll(tempCache)
	}

	return cache, cleanup
}

func TestParseInstallRef(t *testing.T) {
	tests := []struct {
		name         string
		ref          string
		wantColl     string
		wantRegistry string
		wantErr      bool
	}{
		{
			name:         "valid collection@registry",
			ref:          "sprint@akatz-workflows",
			wantColl:     "sprint",
			wantRegistry: "akatz-workflows",
			wantErr:      false,
		},
		{
			name:         "collection with hyphens",
			ref:          "my-sprint@my-registry",
			wantColl:     "my-sprint",
			wantRegistry: "my-registry",
			wantErr:      false,
		},
		{
			name:    "direct URL not supported yet",
			ref:     "github.com/user/meow-sprint",
			wantErr: true,
		},
		{
			name:    "missing @ separator",
			ref:     "sprint",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			coll, reg, err := parseInstallRef(tt.ref)
			if tt.wantErr {
				if err == nil {
					t.Errorf("parseInstallRef(%q) expected error, got nil", tt.ref)
				}
				return
			}
			if err != nil {
				t.Errorf("parseInstallRef(%q) unexpected error: %v", tt.ref, err)
				return
			}
			if coll != tt.wantColl {
				t.Errorf("parseInstallRef(%q) collection = %q, want %q", tt.ref, coll, tt.wantColl)
			}
			if reg != tt.wantRegistry {
				t.Errorf("parseInstallRef(%q) registry = %q, want %q", tt.ref, reg, tt.wantRegistry)
			}
		})
	}
}

func TestResolveInstallDestination(t *testing.T) {
	tests := []struct {
		name      string
		collName  string
		local     bool
		wantScope string
	}{
		{
			name:      "user scope install",
			collName:  "sprint",
			local:     false,
			wantScope: "user",
		},
		{
			name:      "project scope install",
			collName:  "sprint",
			local:     true,
			wantScope: "project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			destDir, scope, err := resolveInstallDestination(tt.collName, tt.local)
			if err != nil {
				t.Fatalf("resolveInstallDestination() error: %v", err)
			}

			if scope != tt.wantScope {
				t.Errorf("scope = %q, want %q", scope, tt.wantScope)
			}

			if tt.local {
				if !strings.Contains(destDir, ".meow/workflows/"+tt.collName) {
					t.Errorf("local install path should contain .meow/workflows/%s, got: %s", tt.collName, destDir)
				}
			} else {
				if !strings.Contains(destDir, ".meow/workflows/"+tt.collName) {
					t.Errorf("user install path should contain .meow/workflows/%s, got: %s", tt.collName, destDir)
				}
			}
		})
	}
}

// Note: copyDir and copyFile are already tested in adapter_install_test.go
// We rely on those existing implementations which handle .git exclusion

func TestInstallCommand_DryRun(t *testing.T) {
	// This test will verify that --dry-run shows what would be installed without installing
	// We'll need to set up a test registry and verify no files are created

	// Setup will be needed when we implement the full command test
	t.Skip("Requires full install command implementation")
}

func TestInstallCommand_ToUserDirectory(t *testing.T) {
	// Test installing to ~/.meow/workflows/
	t.Skip("Requires full install command implementation")
}

func TestInstallCommand_ToProjectDirectory(t *testing.T) {
	// Test installing to .meow/workflows/ with --local
	t.Skip("Requires full install command implementation")
}

func TestInstallCommand_WithAlias(t *testing.T) {
	// Test installing with --as flag to use different name
	t.Skip("Requires full install command implementation")
}

func TestInstallCommand_Force(t *testing.T) {
	// Test --force flag overwrites existing installation
	t.Skip("Requires full install command implementation")
}

func TestInstallCommand_DuplicateError(t *testing.T) {
	// Test that installing over existing collection without --force produces error
	t.Skip("Requires full install command implementation")
}

func TestInstallCommand_TracksInInstalledStore(t *testing.T) {
	// Test that installation is tracked in ~/.meow/installed.json
	t.Skip("Requires full install command implementation")
}

func TestInstallCommand_CollectionNotFound(t *testing.T) {
	// Test error when collection doesn't exist in registry
	t.Skip("Requires full install command implementation")
}

func TestInstallCommand_RegistryNotFound(t *testing.T) {
	// Test error when registry isn't added/cached
	t.Skip("Requires full install command implementation")
}
