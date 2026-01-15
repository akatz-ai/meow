package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resetInitFlags(t *testing.T) {
	t.Helper()

	if err := initCmd.Flags().Set("global", "false"); err != nil {
		t.Fatalf("failed to reset global flag: %v", err)
	}
	if err := initCmd.Flags().Set("force", "false"); err != nil {
		t.Fatalf("failed to reset force flag: %v", err)
	}
}

func TestInitGlobalCreatesStructure(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	defer resetInitFlags(t)

	if err := initCmd.Flags().Set("global", "true"); err != nil {
		t.Fatalf("failed to set global flag: %v", err)
	}

	output, err := captureOutput(t, func() error {
		return runInit(initCmd, nil)
	})
	if err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	meowDir := filepath.Join(userHome, ".meow")
	if _, err := os.Stat(meowDir); err != nil {
		t.Fatalf("expected global meow dir: %v", err)
	}

	for _, subdir := range []string{"workflows", filepath.Join("workflows", "lib"), "adapters"} {
		path := filepath.Join(meowDir, subdir)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", path, err)
		}
	}

	configPath := filepath.Join(meowDir, "config.toml")
	configBytes, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	configContent := string(configBytes)
	if !strings.Contains(configContent, "MEOW Global Configuration") {
		t.Fatalf("expected global config header, got: %s", configContent)
	}
	if !strings.Contains(configContent, "[agent]") {
		t.Fatalf("expected agent section, got: %s", configContent)
	}

	if !strings.Contains(output, "Initializing user-global MEOW directory") {
		t.Fatalf("expected init message, got: %s", output)
	}
}

func TestInitGlobalAlreadyExists(t *testing.T) {
	userHome := t.TempDir()
	meowDir := filepath.Join(userHome, ".meow")
	t.Setenv("HOME", userHome)
	defer resetInitFlags(t)

	if err := os.MkdirAll(meowDir, 0755); err != nil {
		t.Fatalf("failed to create existing dir: %v", err)
	}

	if err := initCmd.Flags().Set("global", "true"); err != nil {
		t.Fatalf("failed to set global flag: %v", err)
	}

	output, err := captureOutput(t, func() error {
		return runInit(initCmd, nil)
	})
	if err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	if !strings.Contains(output, "~/.meow/ already exists") {
		t.Fatalf("expected already exists message, got: %s", output)
	}
}

func TestInitGlobalForcePreservesExisting(t *testing.T) {
	userHome := t.TempDir()
	meowDir := filepath.Join(userHome, ".meow")
	t.Setenv("HOME", userHome)
	defer resetInitFlags(t)

	if err := os.MkdirAll(filepath.Join(meowDir, "workflows"), 0755); err != nil {
		t.Fatalf("failed to create workflows: %v", err)
	}

	configPath := filepath.Join(meowDir, "config.toml")
	originalConfig := "custom config"
	if err := os.WriteFile(configPath, []byte(originalConfig), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if err := initCmd.Flags().Set("global", "true"); err != nil {
		t.Fatalf("failed to set global flag: %v", err)
	}
	if err := initCmd.Flags().Set("force", "true"); err != nil {
		t.Fatalf("failed to set force flag: %v", err)
	}

	output, err := captureOutput(t, func() error {
		return runInit(initCmd, nil)
	})
	if err != nil {
		t.Fatalf("runInit failed: %v", err)
	}

	updatedConfig, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}
	if string(updatedConfig) != originalConfig {
		t.Fatalf("expected config preserved, got: %s", string(updatedConfig))
	}

	libDir := filepath.Join(meowDir, "workflows", "lib")
	if _, err := os.Stat(libDir); err != nil {
		t.Fatalf("expected workflows/lib created: %v", err)
	}

	if !strings.Contains(output, "Reinitializing ~/.meow/") {
		t.Fatalf("expected reinit message, got: %s", output)
	}
}
