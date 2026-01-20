package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const InstalledFileName = "installed.json"

// InstalledStore manages ~/.meow/installed.json
type InstalledStore struct {
	path string
}

// NewInstalledStore creates a store at the default location (~/.meow/installed.json)
func NewInstalledStore() (*InstalledStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home dir: %w", err)
	}

	return &InstalledStore{
		path: filepath.Join(home, ".meow", InstalledFileName),
	}, nil
}

// NewInstalledStoreWithPath creates a store at a custom path (for testing)
func NewInstalledStoreWithPath(path string) *InstalledStore {
	return &InstalledStore{path: path}
}

// Load reads the installed file, returning empty struct if not exists
func (s *InstalledStore) Load() (*InstalledFile, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return &InstalledFile{Collections: make(map[string]InstalledCollection)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading installed file: %w", err)
	}

	var f InstalledFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing installed file: %w", err)
	}

	if f.Collections == nil {
		f.Collections = make(map[string]InstalledCollection)
	}

	return &f, nil
}

// Save writes the installed file
func (s *InstalledStore) Save(f *InstalledFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling installed: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("writing installed file: %w", err)
	}

	return nil
}

// Add tracks a newly installed collection
func (s *InstalledStore) Add(name string, info InstalledCollection) error {
	f, err := s.Load()
	if err != nil {
		return err
	}

	info.InstalledAt = time.Now()
	f.Collections[name] = info

	return s.Save(f)
}

// Remove untracks an installed collection
func (s *InstalledStore) Remove(name string) error {
	f, err := s.Load()
	if err != nil {
		return err
	}

	if _, exists := f.Collections[name]; !exists {
		return fmt.Errorf("collection %q not installed", name)
	}

	delete(f.Collections, name)
	return s.Save(f)
}

// Get returns info about an installed collection
func (s *InstalledStore) Get(name string) (*InstalledCollection, error) {
	f, err := s.Load()
	if err != nil {
		return nil, err
	}

	c, exists := f.Collections[name]
	if !exists {
		return nil, nil // Not an error, just not installed
	}

	return &c, nil
}

// Exists checks if a collection is installed
func (s *InstalledStore) Exists(name string) (bool, error) {
	c, err := s.Get(name)
	if err != nil {
		return false, err
	}
	return c != nil, nil
}

// List returns all installed collections
func (s *InstalledStore) List() (map[string]InstalledCollection, error) {
	f, err := s.Load()
	if err != nil {
		return nil, err
	}
	return f.Collections, nil
}
