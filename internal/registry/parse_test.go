package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Sample JSON content for testing
const validRegistryJSON = `{
  "name": "test-registry",
  "description": "A test registry",
  "version": "1.0.0",
  "owner": {
    "name": "Test Owner",
    "email": "test@example.com"
  },
  "collectionRoot": "./collections",
  "collections": [
    {
      "name": "sprint",
      "source": "sprint",
      "description": "Sprint planning workflow"
    },
    {
      "name": "external",
      "source": {
        "type": "github",
        "repo": "owner/repo"
      },
      "description": "External collection"
    }
  ]
}`

const validManifestJSON = `{
  "name": "sprint",
  "description": "Sprint planning workflow",
  "entrypoint": "sprint.meow.toml",
  "author": {
    "name": "Sprint Author",
    "email": "author@example.com"
  },
  "license": "MIT",
  "keywords": ["sprint", "planning"]
}`

const malformedJSON = `{invalid json`

func TestLoadRegistry(t *testing.T) {
	t.Run("valid registry", func(t *testing.T) {
		dir := t.TempDir()
		registryPath := filepath.Join(dir, MetaDir, RegistryFileName)
		writeTestFile(t, registryPath, validRegistryJSON)

		reg, err := LoadRegistry(dir)
		if err != nil {
			t.Fatalf("LoadRegistry() error = %v", err)
		}

		if reg.Name != "test-registry" {
			t.Errorf("Name = %q, want %q", reg.Name, "test-registry")
		}
		if reg.Description != "A test registry" {
			t.Errorf("Description = %q, want %q", reg.Description, "A test registry")
		}
		if reg.Version != "1.0.0" {
			t.Errorf("Version = %q, want %q", reg.Version, "1.0.0")
		}
		if reg.Owner.Name != "Test Owner" {
			t.Errorf("Owner.Name = %q, want %q", reg.Owner.Name, "Test Owner")
		}
		if reg.Owner.Email != "test@example.com" {
			t.Errorf("Owner.Email = %q, want %q", reg.Owner.Email, "test@example.com")
		}
		if reg.CollectionRoot != "./collections" {
			t.Errorf("CollectionRoot = %q, want %q", reg.CollectionRoot, "./collections")
		}
		if len(reg.Collections) != 2 {
			t.Fatalf("Collections len = %d, want 2", len(reg.Collections))
		}

		// Check first collection (relative path source)
		if reg.Collections[0].Name != "sprint" {
			t.Errorf("Collections[0].Name = %q, want %q", reg.Collections[0].Name, "sprint")
		}
		if !reg.Collections[0].Source.IsPath() {
			t.Errorf("Collections[0].Source should be a path")
		}
		if reg.Collections[0].Source.Path != "sprint" {
			t.Errorf("Collections[0].Source.Path = %q, want %q", reg.Collections[0].Source.Path, "sprint")
		}

		// Check second collection (github source)
		if reg.Collections[1].Name != "external" {
			t.Errorf("Collections[1].Name = %q, want %q", reg.Collections[1].Name, "external")
		}
		if reg.Collections[1].Source.IsPath() {
			t.Errorf("Collections[1].Source should not be a path")
		}
		if reg.Collections[1].Source.Object.Type != "github" {
			t.Errorf("Collections[1].Source.Object.Type = %q, want %q", reg.Collections[1].Source.Object.Type, "github")
		}
		if reg.Collections[1].Source.Object.Repo != "owner/repo" {
			t.Errorf("Collections[1].Source.Object.Repo = %q, want %q", reg.Collections[1].Source.Object.Repo, "owner/repo")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()
		_, err := LoadRegistry(dir)
		if err == nil {
			t.Fatal("LoadRegistry() expected error for missing file")
		}
		if !strings.Contains(err.Error(), "reading registry") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "reading registry")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		registryPath := filepath.Join(dir, MetaDir, RegistryFileName)
		writeTestFile(t, registryPath, malformedJSON)

		_, err := LoadRegistry(dir)
		if err == nil {
			t.Fatal("LoadRegistry() expected error for malformed JSON")
		}
		if !strings.Contains(err.Error(), "parsing registry") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "parsing registry")
		}
	})
}

func TestLoadManifest(t *testing.T) {
	t.Run("valid manifest", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, MetaDir, ManifestFileName)
		writeTestFile(t, manifestPath, validManifestJSON)

		m, err := LoadManifest(dir)
		if err != nil {
			t.Fatalf("LoadManifest() error = %v", err)
		}

		if m.Name != "sprint" {
			t.Errorf("Name = %q, want %q", m.Name, "sprint")
		}
		if m.Description != "Sprint planning workflow" {
			t.Errorf("Description = %q, want %q", m.Description, "Sprint planning workflow")
		}
		if m.Entrypoint != "sprint.meow.toml" {
			t.Errorf("Entrypoint = %q, want %q", m.Entrypoint, "sprint.meow.toml")
		}
		if m.Author == nil {
			t.Fatalf("Author should not be nil")
		}
		if m.Author.Name != "Sprint Author" {
			t.Errorf("Author.Name = %q, want %q", m.Author.Name, "Sprint Author")
		}
		if m.License != "MIT" {
			t.Errorf("License = %q, want %q", m.License, "MIT")
		}
		if len(m.Keywords) != 2 {
			t.Fatalf("Keywords len = %d, want 2", len(m.Keywords))
		}
		if m.Keywords[0] != "sprint" {
			t.Errorf("Keywords[0] = %q, want %q", m.Keywords[0], "sprint")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		dir := t.TempDir()
		_, err := LoadManifest(dir)
		if err == nil {
			t.Fatal("LoadManifest() expected error for missing file")
		}
		if !strings.Contains(err.Error(), "reading manifest") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "reading manifest")
		}
	})

	t.Run("malformed JSON", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, MetaDir, ManifestFileName)
		writeTestFile(t, manifestPath, malformedJSON)

		_, err := LoadManifest(dir)
		if err == nil {
			t.Fatal("LoadManifest() expected error for malformed JSON")
		}
		if !strings.Contains(err.Error(), "parsing manifest") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "parsing manifest")
		}
	})
}

func TestHasRegistry(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		dir := t.TempDir()
		registryPath := filepath.Join(dir, MetaDir, RegistryFileName)
		writeTestFile(t, registryPath, validRegistryJSON)

		if !HasRegistry(dir) {
			t.Error("HasRegistry() = false, want true")
		}
	})

	t.Run("does not exist", func(t *testing.T) {
		dir := t.TempDir()
		if HasRegistry(dir) {
			t.Error("HasRegistry() = true, want false")
		}
	})
}

func TestHasManifest(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		dir := t.TempDir()
		manifestPath := filepath.Join(dir, MetaDir, ManifestFileName)
		writeTestFile(t, manifestPath, validManifestJSON)

		if !HasManifest(dir) {
			t.Error("HasManifest() = false, want true")
		}
	})

	t.Run("does not exist", func(t *testing.T) {
		dir := t.TempDir()
		if HasManifest(dir) {
			t.Error("HasManifest() = true, want false")
		}
	})
}

func TestResolveCollectionSource(t *testing.T) {
	t.Run("relative path without collectionRoot", func(t *testing.T) {
		entry := CollectionEntry{
			Name:   "sprint",
			Source: Source{Path: "sprint"},
		}
		registryDir := "/home/user/registry"

		path, err := ResolveCollectionSource(entry, registryDir, "")
		if err != nil {
			t.Fatalf("ResolveCollectionSource() error = %v", err)
		}
		expected := "/home/user/registry/sprint"
		if path != expected {
			t.Errorf("path = %q, want %q", path, expected)
		}
	})

	t.Run("relative path with collectionRoot", func(t *testing.T) {
		entry := CollectionEntry{
			Name:   "sprint",
			Source: Source{Path: "sprint"},
		}
		registryDir := "/home/user/registry"
		collectionRoot := "collections"

		path, err := ResolveCollectionSource(entry, registryDir, collectionRoot)
		if err != nil {
			t.Fatalf("ResolveCollectionSource() error = %v", err)
		}
		expected := "/home/user/registry/collections/sprint"
		if path != expected {
			t.Errorf("path = %q, want %q", path, expected)
		}
	})

	t.Run("relative path with ./collectionRoot", func(t *testing.T) {
		entry := CollectionEntry{
			Name:   "sprint",
			Source: Source{Path: "sprint"},
		}
		registryDir := "/home/user/registry"
		collectionRoot := "./collections"

		path, err := ResolveCollectionSource(entry, registryDir, collectionRoot)
		if err != nil {
			t.Fatalf("ResolveCollectionSource() error = %v", err)
		}
		// filepath.Join normalizes the path, removing ./
		expected := "/home/user/registry/collections/sprint"
		if path != expected {
			t.Errorf("path = %q, want %q", path, expected)
		}
	})

	t.Run("external source returns error", func(t *testing.T) {
		entry := CollectionEntry{
			Name: "external",
			Source: Source{
				Object: &SourceObject{
					Type: "github",
					Repo: "owner/repo",
				},
			},
		}
		registryDir := "/home/user/registry"

		_, err := ResolveCollectionSource(entry, registryDir, "")
		if err == nil {
			t.Fatal("ResolveCollectionSource() expected error for external source")
		}
		if !strings.Contains(err.Error(), "external sources not yet implemented") {
			t.Errorf("error = %q, want to contain %q", err.Error(), "external sources not yet implemented")
		}
	})
}

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll error = %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile error = %v", err)
	}
}
