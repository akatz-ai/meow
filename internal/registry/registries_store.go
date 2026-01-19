package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const RegistriesFileName = "registries.json"

// RegistriesStore manages ~/.meow/registries.json
type RegistriesStore struct {
	path string
}

// NewRegistriesStore creates a store at the default location (~/.meow/registries.json)
func NewRegistriesStore() (*RegistriesStore, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home dir: %w", err)
	}

	return &RegistriesStore{
		path: filepath.Join(home, ".meow", RegistriesFileName),
	}, nil
}

// NewRegistriesStoreWithPath creates a store at a custom path (for testing)
func NewRegistriesStoreWithPath(path string) *RegistriesStore {
	return &RegistriesStore{path: path}
}

// Path returns the store's file path
func (s *RegistriesStore) Path() string {
	return s.path
}

// Load reads the registries file, returning empty struct if not exists
func (s *RegistriesStore) Load() (*RegistriesFile, error) {
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return &RegistriesFile{Registries: make(map[string]RegisteredRegistry)}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading registries file: %w", err)
	}

	var f RegistriesFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parsing registries file: %w", err)
	}

	if f.Registries == nil {
		f.Registries = make(map[string]RegisteredRegistry)
	}

	return &f, nil
}

// Save writes the registries file
func (s *RegistriesStore) Save(f *RegistriesFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}

	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling registries: %w", err)
	}

	if err := os.WriteFile(s.path, data, 0644); err != nil {
		return fmt.Errorf("writing registries file: %w", err)
	}

	return nil
}

// Add registers a new registry
func (s *RegistriesStore) Add(name, source, version string) error {
	f, err := s.Load()
	if err != nil {
		return err
	}

	if _, exists := f.Registries[name]; exists {
		return fmt.Errorf("registry %q already registered", name)
	}

	now := time.Now()
	f.Registries[name] = RegisteredRegistry{
		Source:    source,
		Version:   version,
		AddedAt:   now,
		UpdatedAt: now,
	}

	return s.Save(f)
}

// Remove unregisters a registry
func (s *RegistriesStore) Remove(name string) error {
	f, err := s.Load()
	if err != nil {
		return err
	}

	if _, exists := f.Registries[name]; !exists {
		return fmt.Errorf("registry %q not found", name)
	}

	delete(f.Registries, name)
	return s.Save(f)
}

// Get returns a registered registry
func (s *RegistriesStore) Get(name string) (*RegisteredRegistry, error) {
	f, err := s.Load()
	if err != nil {
		return nil, err
	}

	reg, exists := f.Registries[name]
	if !exists {
		return nil, fmt.Errorf("registry %q not found", name)
	}

	return &reg, nil
}

// Update marks a registry as updated with new version
func (s *RegistriesStore) Update(name, version string) error {
	f, err := s.Load()
	if err != nil {
		return err
	}

	reg, exists := f.Registries[name]
	if !exists {
		return fmt.Errorf("registry %q not found", name)
	}

	reg.Version = version
	reg.UpdatedAt = time.Now()
	f.Registries[name] = reg

	return s.Save(f)
}

// List returns all registered registries
func (s *RegistriesStore) List() (map[string]RegisteredRegistry, error) {
	f, err := s.Load()
	if err != nil {
		return nil, err
	}
	return f.Registries, nil
}
