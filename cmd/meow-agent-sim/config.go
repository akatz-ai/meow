package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

// LoadConfig loads simulator configuration from a YAML file.
func LoadConfig(path string) (SimConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SimConfig{}, err
	}

	config := NewDefaultSimConfig()
	if err := yaml.Unmarshal(data, &config); err != nil {
		return SimConfig{}, err
	}

	return config, nil
}
