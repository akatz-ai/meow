package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsGitURL(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"https://github.com/user/repo", true},
		{"http://github.com/user/repo", true},
		{"git@github.com:user/repo.git", true},
		{"ssh://git@github.com/user/repo", true},
		{"git://github.com/user/repo", true},
		{"./my-adapter", false},
		{"/path/to/adapter", false},
		{"../relative/path", false},
		{"my-adapter", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isGitURL(tt.input)
			if result != tt.expected {
				t.Errorf("isGitURL(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestExtractRepoName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"https://github.com/user/meow-adapter-aider", "meow-adapter-aider"},
		{"https://github.com/user/meow-adapter-aider.git", "meow-adapter-aider"},
		{"git@github.com:user/my-adapter.git", "my-adapter"},
		{"https://gitlab.com/user/adapter", "adapter"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractRepoName(tt.input)
			if result != tt.expected {
				t.Errorf("extractRepoName(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsValidAdapterName(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"claude", true},
		{"aider-v2", true},
		{"my_adapter", true},
		{"Adapter123", true},
		{"a", true},
		{"", false},
		{"-invalid", false},
		{"_invalid", false},
		{"has space", false},
		{"has.dot", false},
		{"has/slash", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isValidAdapterName(tt.input)
			if result != tt.expected {
				t.Errorf("isValidAdapterName(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCopyDir(t *testing.T) {
	// Create source directory structure
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "dest")

	// Create files in source
	if err := os.WriteFile(filepath.Join(srcDir, "adapter.toml"), []byte("[adapter]\nname = \"test\""), 0644); err != nil {
		t.Fatalf("creating adapter.toml: %v", err)
	}

	// Create subdirectory
	subDir := filepath.Join(srcDir, "scripts")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("creating subdirectory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "setup.sh"), []byte("#!/bin/bash\necho hello"), 0755); err != nil {
		t.Fatalf("creating setup.sh: %v", err)
	}

	// Create .git directory (should be skipped)
	gitDir := filepath.Join(srcDir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatalf("creating .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("git config"), 0644); err != nil {
		t.Fatalf("creating git config: %v", err)
	}

	// Copy
	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir: %v", err)
	}

	// Verify files were copied
	if _, err := os.Stat(filepath.Join(dstDir, "adapter.toml")); err != nil {
		t.Errorf("adapter.toml not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dstDir, "scripts", "setup.sh")); err != nil {
		t.Errorf("scripts/setup.sh not copied: %v", err)
	}

	// Verify .git was skipped
	if _, err := os.Stat(filepath.Join(dstDir, ".git")); !os.IsNotExist(err) {
		t.Errorf(".git directory should not be copied")
	}
}

func TestLoadAdapterConfig(t *testing.T) {
	// Create temp adapter.toml
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "adapter.toml")

	content := `[adapter]
name = "test-adapter"
description = "A test adapter"

[spawn]
command = "test-agent --run"
startup_delay = "3s"

[prompt_injection]
method = "literal"
pre_keys = ["Escape"]
post_keys = ["Enter"]

[graceful_stop]
keys = ["C-c"]
wait = "2s"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("writing adapter.toml: %v", err)
	}

	cfg, err := loadAdapterConfig(configPath)
	if err != nil {
		t.Fatalf("loadAdapterConfig: %v", err)
	}

	if cfg.Adapter.Name != "test-adapter" {
		t.Errorf("name = %q, expected %q", cfg.Adapter.Name, "test-adapter")
	}
	if cfg.Spawn.Command != "test-agent --run" {
		t.Errorf("spawn.command = %q, expected %q", cfg.Spawn.Command, "test-agent --run")
	}
	if cfg.PromptInjection.Method != "literal" {
		t.Errorf("prompt_injection.method = %q, expected %q", cfg.PromptInjection.Method, "literal")
	}

	// Validate should pass
	if err := cfg.Validate(); err != nil {
		t.Errorf("validate failed: %v", err)
	}
}

func TestLoadAdapterConfigInvalid(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "adapter.toml")

	// Missing required fields
	content := `[adapter]
description = "Missing name field"
`
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		t.Fatalf("writing adapter.toml: %v", err)
	}

	cfg, err := loadAdapterConfig(configPath)
	if err != nil {
		t.Fatalf("loadAdapterConfig: %v", err)
	}

	// Validate should fail due to missing name
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation to fail for missing name")
	}
}

func TestAdapterInstallLocalPath(t *testing.T) {
	// Create a source adapter directory
	srcDir := t.TempDir()

	adapterContent := `[adapter]
name = "test-local"
description = "Test local adapter"

[spawn]
command = "test-agent"
startup_delay = "1s"

[prompt_injection]
method = "literal"
post_keys = ["Enter"]
`
	if err := os.WriteFile(filepath.Join(srcDir, "adapter.toml"), []byte(adapterContent), 0644); err != nil {
		t.Fatalf("creating adapter.toml: %v", err)
	}

	// Create a script that should be made executable
	if err := os.WriteFile(filepath.Join(srcDir, "setup.sh"), []byte("#!/bin/bash\necho setup"), 0644); err != nil {
		t.Fatalf("creating setup.sh: %v", err)
	}

	// Override home directory for test
	originalHome := os.Getenv("HOME")
	testHome := t.TempDir()
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Run the install command
	adapterInstallName = ""
	adapterInstallForce = false

	err := runAdapterInstall(adapterInstallCmd, []string{srcDir})
	if err != nil {
		t.Fatalf("runAdapterInstall: %v", err)
	}

	// Verify adapter was installed
	destDir := filepath.Join(testHome, ".meow", "adapters", "test-local")
	if _, err := os.Stat(filepath.Join(destDir, "adapter.toml")); err != nil {
		t.Errorf("adapter.toml not installed: %v", err)
	}

	// Verify setup.sh was made executable
	info, err := os.Stat(filepath.Join(destDir, "setup.sh"))
	if err != nil {
		t.Errorf("setup.sh not installed: %v", err)
	} else if info.Mode()&0100 == 0 {
		t.Errorf("setup.sh not executable: mode=%o", info.Mode())
	}
}

func TestAdapterInstallWithNameOverride(t *testing.T) {
	// Create a source adapter directory
	srcDir := t.TempDir()

	adapterContent := `[adapter]
name = "original-name"
description = "Test adapter"

[spawn]
command = "test-agent"
`
	if err := os.WriteFile(filepath.Join(srcDir, "adapter.toml"), []byte(adapterContent), 0644); err != nil {
		t.Fatalf("creating adapter.toml: %v", err)
	}

	// Override home directory for test
	originalHome := os.Getenv("HOME")
	testHome := t.TempDir()
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Run the install command with --name override
	adapterInstallName = "custom-name"
	adapterInstallForce = false
	defer func() { adapterInstallName = "" }()

	err := runAdapterInstall(adapterInstallCmd, []string{srcDir})
	if err != nil {
		t.Fatalf("runAdapterInstall: %v", err)
	}

	// Verify adapter was installed with custom name
	destDir := filepath.Join(testHome, ".meow", "adapters", "custom-name")
	if _, err := os.Stat(filepath.Join(destDir, "adapter.toml")); err != nil {
		t.Errorf("adapter not installed at custom-name: %v", err)
	}
}

func TestAdapterInstallExistingNoForce(t *testing.T) {
	// Create a source adapter directory
	srcDir := t.TempDir()

	adapterContent := `[adapter]
name = "existing"
description = "Test adapter"

[spawn]
command = "test-agent"
`
	if err := os.WriteFile(filepath.Join(srcDir, "adapter.toml"), []byte(adapterContent), 0644); err != nil {
		t.Fatalf("creating adapter.toml: %v", err)
	}

	// Override home directory for test
	originalHome := os.Getenv("HOME")
	testHome := t.TempDir()
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Create existing adapter
	existingDir := filepath.Join(testHome, ".meow", "adapters", "existing")
	if err := os.MkdirAll(existingDir, 0755); err != nil {
		t.Fatalf("creating existing adapter dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "adapter.toml"), []byte("old content"), 0644); err != nil {
		t.Fatalf("creating existing adapter.toml: %v", err)
	}

	// Run the install command without --force
	adapterInstallName = ""
	adapterInstallForce = false

	err := runAdapterInstall(adapterInstallCmd, []string{srcDir})
	if err == nil {
		t.Fatal("expected error for existing adapter without --force")
	}
}

func TestAdapterInstallExistingWithForce(t *testing.T) {
	// Create a source adapter directory
	srcDir := t.TempDir()

	adapterContent := `[adapter]
name = "existing"
description = "New adapter"

[spawn]
command = "new-agent"
`
	if err := os.WriteFile(filepath.Join(srcDir, "adapter.toml"), []byte(adapterContent), 0644); err != nil {
		t.Fatalf("creating adapter.toml: %v", err)
	}

	// Override home directory for test
	originalHome := os.Getenv("HOME")
	testHome := t.TempDir()
	os.Setenv("HOME", testHome)
	defer os.Setenv("HOME", originalHome)

	// Create existing adapter
	existingDir := filepath.Join(testHome, ".meow", "adapters", "existing")
	if err := os.MkdirAll(existingDir, 0755); err != nil {
		t.Fatalf("creating existing adapter dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(existingDir, "adapter.toml"), []byte("old content"), 0644); err != nil {
		t.Fatalf("creating existing adapter.toml: %v", err)
	}

	// Run the install command with --force
	adapterInstallName = ""
	adapterInstallForce = true
	defer func() { adapterInstallForce = false }()

	err := runAdapterInstall(adapterInstallCmd, []string{srcDir})
	if err != nil {
		t.Fatalf("runAdapterInstall with force: %v", err)
	}

	// Verify new content was installed
	content, err := os.ReadFile(filepath.Join(existingDir, "adapter.toml"))
	if err != nil {
		t.Fatalf("reading installed adapter.toml: %v", err)
	}
	if string(content) == "old content" {
		t.Error("adapter was not overwritten with --force")
	}
}

func TestAdapterInstallMissingAdapterToml(t *testing.T) {
	// Create an empty source directory (no adapter.toml)
	srcDir := t.TempDir()

	adapterInstallName = ""
	adapterInstallForce = false

	err := runAdapterInstall(adapterInstallCmd, []string{srcDir})
	if err == nil {
		t.Fatal("expected error for missing adapter.toml")
	}
}

func TestAdapterInstallInvalidAdapterToml(t *testing.T) {
	// Create a source with invalid adapter.toml
	srcDir := t.TempDir()

	// Missing spawn.command (required field)
	adapterContent := `[adapter]
name = "invalid"
`
	if err := os.WriteFile(filepath.Join(srcDir, "adapter.toml"), []byte(adapterContent), 0644); err != nil {
		t.Fatalf("creating adapter.toml: %v", err)
	}

	adapterInstallName = ""
	adapterInstallForce = false

	err := runAdapterInstall(adapterInstallCmd, []string{srcDir})
	if err == nil {
		t.Fatal("expected error for invalid adapter.toml")
	}
}
