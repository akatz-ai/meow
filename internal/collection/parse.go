package collection

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ManifestName is the expected collection manifest filename.
const ManifestName = "meow-collection.toml"

// LoadFromDir loads a collection manifest from the repository root.
func LoadFromDir(dir string) (*Collection, error) {
	path := filepath.Join(dir, ManifestName)
	return ParseFile(path)
}

// ParseFile parses a collection manifest from a file path.
func ParseFile(path string) (*Collection, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open collection manifest: %w", err)
	}
	defer file.Close()

	baseDir := filepath.Dir(path)
	return Parse(file, baseDir)
}

// Parse parses a collection manifest from a reader.
func Parse(reader io.Reader, baseDir string) (*Collection, error) {
	var c Collection
	meta, err := toml.NewDecoder(reader).Decode(&c)
	if err != nil {
		return nil, fmt.Errorf("decode collection manifest: %w", err)
	}

	if undecoded := meta.Undecoded(); len(undecoded) > 0 {
		_ = undecoded
	}

	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		baseDir = "."
	}

	result := c.Validate(baseDir)
	if result.HasErrors() {
		return nil, result
	}

	return &c, nil
}

// ParseString parses a collection manifest from a string.
func ParseString(content string, baseDir string) (*Collection, error) {
	return Parse(strings.NewReader(content), baseDir)
}
