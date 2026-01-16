package skill

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ManifestName is the expected skill manifest filename.
const ManifestName = "skill.toml"

// LoadFromDir loads a skill manifest from a directory.
func LoadFromDir(dir string) (*Skill, error) {
	path := filepath.Join(dir, ManifestName)
	return ParseFile(path)
}

// ParseFile parses a skill manifest from a file path.
func ParseFile(path string) (*Skill, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open skill manifest: %w", err)
	}
	defer file.Close()

	return Parse(file)
}

// Parse parses a skill manifest from a reader.
func Parse(reader io.Reader) (*Skill, error) {
	var s Skill
	_, err := toml.NewDecoder(reader).Decode(&s)
	if err != nil {
		return nil, fmt.Errorf("decode skill manifest: %w", err)
	}

	return &s, nil
}

// ParseString parses a skill manifest from a string.
func ParseString(content string) (*Skill, error) {
	return Parse(strings.NewReader(content))
}
