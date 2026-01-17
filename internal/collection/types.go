package collection

// Collection represents a MEOW workflow collection manifest.
type Collection struct {
	Collection CollectionMeta    `toml:"collection"`
	Packs      []Pack            `toml:"packs"`
	Skills     map[string]string `toml:"skills,omitempty"` // name -> manifest path
}

// CollectionMeta contains metadata for the collection.
type CollectionMeta struct {
	Name        string      `toml:"name"`
	Description string      `toml:"description"`
	Version     string      `toml:"version"`
	MeowVersion string      `toml:"meow_version,omitempty"`
	Owner       Owner       `toml:"owner"`
	Repository  *Repository `toml:"repository,omitempty"`
}

// Owner describes the collection author or organization.
type Owner struct {
	Name  string `toml:"name"`
	Email string `toml:"email,omitempty"`
	URL   string `toml:"url,omitempty"`
}

// Repository describes the source repository metadata.
type Repository struct {
	URL     string `toml:"url,omitempty"`
	License string `toml:"license,omitempty"`
}

// Pack groups workflows within a collection.
type Pack struct {
	Name        string   `toml:"name"`
	Description string   `toml:"description"`
	Workflows   []string `toml:"workflows"`
}
