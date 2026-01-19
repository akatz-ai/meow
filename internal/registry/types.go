// Package registry provides types for MEOW's registry and collection distribution system.
//
// The registry system uses a two-level hierarchy:
//   - Registry: A Git repository containing one or more collections (.meow/registry.json)
//   - Collection: A self-contained, installable workflow bundle (.meow/manifest.json)
//
// For full documentation, see docs/REGISTRY-DISTRIBUTION.md.
package registry

import (
	"encoding/json"
	"time"
)

// Registry represents a .meow/registry.json file.
// A registry is the top-level distribution unit that indexes multiple collections.
type Registry struct {
	// Name is the registry identifier (kebab-case).
	// Users reference this in commands like "meow install collection@registry-name".
	Name string `json:"name"`

	// Description provides a brief human-readable description.
	Description string `json:"description,omitempty"`

	// Version is the registry version (semver).
	// All collections share this version; there are no per-collection versions.
	Version string `json:"version"`

	// Owner describes the registry maintainer.
	Owner Owner `json:"owner"`

	// CollectionRoot is the base path prepended to relative collection sources.
	// For example, if CollectionRoot is "./collections" and a collection source is "sprint",
	// the resolved path is "./collections/sprint/".
	// Default: "."
	CollectionRoot string `json:"collectionRoot,omitempty"`

	// Collections lists the available collections in this registry.
	Collections []CollectionEntry `json:"collections"`
}

// Owner describes a registry maintainer or organization.
type Owner struct {
	// Name is the maintainer or organization name (required).
	Name string `json:"name"`

	// Email is the contact email (optional).
	Email string `json:"email,omitempty"`

	// URL is the website or profile URL (optional).
	URL string `json:"url,omitempty"`
}

// CollectionEntry is a collection listing in registry.json.
// It provides discovery metadata for a collection.
type CollectionEntry struct {
	// Name is the collection identifier (kebab-case).
	Name string `json:"name"`

	// Source specifies where to find the collection.
	// Can be a relative path (string) or a SourceObject for external sources.
	Source Source `json:"source"`

	// Description provides a brief human-readable description.
	Description string `json:"description"`

	// Tags are keywords for discovery and filtering.
	Tags []string `json:"tags,omitempty"`

	// Strict indicates whether the collection must have a .meow/manifest.json.
	// Default: true. When false, the registry entry defines everything.
	Strict *bool `json:"strict,omitempty"`

	// Entrypoint specifies the main workflow file when Strict is false.
	// Only used when Strict is explicitly set to false.
	Entrypoint string `json:"entrypoint,omitempty"`
}

// Source represents a collection source, which can be either a relative path (string)
// or a SourceObject for external repositories.
type Source struct {
	// Path is set when the source is a simple relative path string.
	Path string

	// Object is set when the source is a SourceObject (github or git).
	Object *SourceObject
}

// IsPath returns true if this source is a simple relative path.
func (s Source) IsPath() bool {
	return s.Object == nil
}

// String returns the string representation of the source.
// For path sources, returns the path. For object sources, returns a description.
func (s Source) String() string {
	if s.IsPath() {
		return s.Path
	}
	if s.Object.Type == "github" {
		return "github:" + s.Object.Repo
	}
	return "git:" + s.Object.URL
}

// MarshalJSON implements json.Marshaler.
func (s Source) MarshalJSON() ([]byte, error) {
	if s.IsPath() {
		return json.Marshal(s.Path)
	}
	return json.Marshal(s.Object)
}

// UnmarshalJSON implements json.Unmarshaler.
// Handles both string paths and SourceObject structures.
func (s *Source) UnmarshalJSON(data []byte) error {
	// Try string first
	var path string
	if err := json.Unmarshal(data, &path); err == nil {
		s.Path = path
		s.Object = nil
		return nil
	}

	// Try SourceObject
	var obj SourceObject
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	s.Object = &obj
	s.Path = ""
	return nil
}

// SourceObject represents an external source location (GitHub or Git).
type SourceObject struct {
	// Type must be "github" or "git".
	Type string `json:"type"`

	// Repo is the GitHub repository in "owner/repo" format.
	// Only used when Type is "github".
	Repo string `json:"repo,omitempty"`

	// URL is the Git repository URL.
	// Only used when Type is "git".
	URL string `json:"url,omitempty"`

	// Ref is the Git ref (tag, branch, or commit hash).
	// Optional; defaults to the repository's default branch.
	Ref string `json:"ref,omitempty"`

	// Path is the subdirectory within the repository.
	// Optional; defaults to the repository root.
	Path string `json:"path,omitempty"`
}

// Manifest represents a .meow/manifest.json file within a collection.
// The manifest is the authoritative metadata for an installed collection.
type Manifest struct {
	// Name is the collection identifier (kebab-case).
	// Should match the directory name.
	Name string `json:"name"`

	// Description is a human-readable description.
	Description string `json:"description"`

	// Entrypoint is the main workflow file to run (relative to collection root).
	Entrypoint string `json:"entrypoint"`

	// Author describes the collection author (optional).
	Author *Author `json:"author,omitempty"`

	// License is the SPDX license identifier (MIT, Apache-2.0, etc).
	License string `json:"license,omitempty"`

	// Keywords are tags for search and discovery.
	Keywords []string `json:"keywords,omitempty"`

	// Repository is the source code repository URL.
	Repository string `json:"repository,omitempty"`

	// Homepage is the documentation or homepage URL.
	Homepage string `json:"homepage,omitempty"`
}

// Author describes a collection author.
type Author struct {
	// Name is the author name (required).
	Name string `json:"name"`

	// Email is the contact email (optional).
	Email string `json:"email,omitempty"`

	// URL is the website or profile URL (optional).
	URL string `json:"url,omitempty"`
}

// RegistriesFile represents the ~/.meow/registries.json file.
// This file tracks all registered registries for the user.
type RegistriesFile struct {
	// Registries maps registry names to their registration info.
	Registries map[string]RegisteredRegistry `json:"registries"`
}

// RegisteredRegistry tracks a registered registry in ~/.meow/registries.json.
type RegisteredRegistry struct {
	// Source is the registry source (e.g., "github.com/owner/repo").
	Source string `json:"source"`

	// Version is the cached registry version.
	Version string `json:"version"`

	// AddedAt is when the registry was first added.
	AddedAt time.Time `json:"added_at"`

	// UpdatedAt is when the registry was last updated/fetched.
	UpdatedAt time.Time `json:"updated_at"`
}

// InstalledFile represents the ~/.meow/installed.json file.
// This file tracks all installed collections.
type InstalledFile struct {
	// Collections maps collection names to their installation info.
	Collections map[string]InstalledCollection `json:"collections"`
}

// InstalledCollection tracks an installed collection in ~/.meow/installed.json.
type InstalledCollection struct {
	// Registry is the source registry name (for registry-sourced installs).
	// Empty for direct URL installs.
	Registry string `json:"registry,omitempty"`

	// RegistryVersion is the registry version at install time.
	// Used to detect when updates are available.
	RegistryVersion string `json:"registry_version,omitempty"`

	// Source is the direct source URL (for direct URL installs).
	// Empty for registry-sourced installs.
	Source string `json:"source,omitempty"`

	// InstalledAt is when the collection was installed.
	InstalledAt time.Time `json:"installed_at"`

	// Path is where the collection is installed.
	// For user scope: ~/.meow/workflows/<name>/
	// For project scope: .meow/workflows/<name>/
	Path string `json:"path"`

	// Scope is either "user" or "project".
	Scope string `json:"scope"`
}

// Scope constants for InstalledCollection.
const (
	ScopeUser    = "user"
	ScopeProject = "project"
)

// SourceType constants for SourceObject.
const (
	SourceTypeGitHub = "github"
	SourceTypeGit    = "git"
)
