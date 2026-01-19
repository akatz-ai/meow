package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	// RegistryFileName is the expected registry file name within .meow/
	RegistryFileName = "registry.json"

	// ManifestFileName is the expected manifest file name within .meow/
	ManifestFileName = "manifest.json"

	// MetaDir is the directory containing registry/manifest metadata
	MetaDir = ".meow"
)

// LoadRegistry loads .meow/registry.json from a directory.
func LoadRegistry(dir string) (*Registry, error) {
	path := filepath.Join(dir, MetaDir, RegistryFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading registry: %w", err)
	}

	var reg Registry
	if err := json.Unmarshal(data, &reg); err != nil {
		return nil, fmt.Errorf("parsing registry: %w", err)
	}

	return &reg, nil
}

// LoadManifest loads .meow/manifest.json from a collection directory.
func LoadManifest(collectionDir string) (*Manifest, error) {
	path := filepath.Join(collectionDir, MetaDir, ManifestFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading manifest: %w", err)
	}

	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	return &m, nil
}

// HasRegistry checks if a directory has .meow/registry.json.
func HasRegistry(dir string) bool {
	path := filepath.Join(dir, MetaDir, RegistryFileName)
	_, err := os.Stat(path)
	return err == nil
}

// HasManifest checks if a directory has .meow/manifest.json.
func HasManifest(dir string) bool {
	path := filepath.Join(dir, MetaDir, ManifestFileName)
	_, err := os.Stat(path)
	return err == nil
}

// ResolveCollectionSource resolves a CollectionEntry's source to an absolute path.
// For path sources, it joins the registry directory, optional collectionRoot, and source path.
// For external sources (github/git), it returns an error (not yet implemented).
func ResolveCollectionSource(entry CollectionEntry, registryDir, collectionRoot string) (string, error) {
	if entry.Source.IsPath() {
		base := registryDir
		if collectionRoot != "" {
			base = filepath.Join(registryDir, collectionRoot)
		}
		return filepath.Join(base, entry.Source.Path), nil
	}

	return "", fmt.Errorf("external sources not yet implemented")
}
