package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/meow-stack/meow-machine/internal/adapter"
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/spf13/cobra"
)

var (
	adapterInstallName  string
	adapterInstallForce bool
)

var adapterInstallCmd = &cobra.Command{
	Use:   "install <source>",
	Short: "Install an adapter from a git URL or local path",
	Long: `Install an adapter from a git repository or local directory.

The source can be:
  - A git URL: https://github.com/user/meow-adapter-aider
  - A local path: ./my-adapter or /path/to/adapter

The adapter directory must contain a valid adapter.toml file.

Examples:
  # Install from git repository
  meow adapter install https://github.com/user/meow-adapter-aider

  # Install from local directory
  meow adapter install ./my-adapter

  # Install with custom name
  meow adapter install https://github.com/user/meow-adapter-aider --name aider

  # Force overwrite existing adapter
  meow adapter install ./my-adapter --force`,
	Args: cobra.ExactArgs(1),
	RunE: runAdapterInstall,
}

func init() {
	adapterInstallCmd.Flags().StringVar(&adapterInstallName, "name", "", "override adapter name (default: from adapter.toml or repo name)")
	adapterInstallCmd.Flags().BoolVar(&adapterInstallForce, "force", false, "overwrite existing adapter")
	adapterCmd.AddCommand(adapterInstallCmd)
}

func runAdapterInstall(cmd *cobra.Command, args []string) error {
	source := args[0]

	var adapterDir string
	var tempDir string
	var err error

	if isGitURL(source) {
		// Clone to temp directory
		tempDir, err = cloneRepo(source)
		if err != nil {
			return err
		}
		defer os.RemoveAll(tempDir)
		adapterDir = tempDir
	} else {
		// Local path
		adapterDir, err = filepath.Abs(source)
		if err != nil {
			return fmt.Errorf("resolving path: %w", err)
		}
		if _, err := os.Stat(adapterDir); os.IsNotExist(err) {
			return fmt.Errorf("source directory not found: %s", adapterDir)
		}
	}

	// Validate adapter.toml exists and is valid
	configPath := filepath.Join(adapterDir, "adapter.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("invalid adapter: adapter.toml not found in %s", adapterDir)
	}

	// Parse and validate adapter config
	cfg, err := loadAdapterConfig(configPath)
	if err != nil {
		return fmt.Errorf("invalid adapter: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid adapter: %w", err)
	}

	// Determine adapter name
	name := adapterInstallName
	if name == "" {
		name = cfg.Adapter.Name
	}
	if name == "" {
		// Fall back to repo name for git URLs
		if isGitURL(source) {
			name = extractRepoName(source)
		} else {
			name = filepath.Base(adapterDir)
		}
	}

	// Validate name
	if !isValidAdapterName(name) {
		return fmt.Errorf("invalid adapter name %q: must contain only alphanumeric characters, hyphens, and underscores", name)
	}

	// Get destination directory
	globalDir := adapter.DefaultGlobalDir()
	if globalDir == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	destDir := filepath.Join(globalDir, name)

	// Check if already exists
	if _, err := os.Stat(destDir); err == nil {
		if !adapterInstallForce {
			return fmt.Errorf("adapter %q already exists at %s. Use --force to overwrite", name, destDir)
		}
		// Remove existing adapter
		if err := os.RemoveAll(destDir); err != nil {
			return fmt.Errorf("removing existing adapter: %w", err)
		}
	}

	// Ensure parent directory exists
	if err := os.MkdirAll(globalDir, 0755); err != nil {
		return fmt.Errorf("creating adapters directory: %w", err)
	}

	// Copy adapter directory
	if err := copyDir(adapterDir, destDir); err != nil {
		return fmt.Errorf("copying adapter: %w", err)
	}

	// Make scripts executable
	scriptsToMakeExecutable := []string{
		"setup.sh",
		"event-translator.sh",
	}
	for _, script := range scriptsToMakeExecutable {
		scriptPath := filepath.Join(destDir, script)
		if _, err := os.Stat(scriptPath); err == nil {
			if err := os.Chmod(scriptPath, 0755); err != nil {
				// Non-fatal: warn but continue
				fmt.Fprintf(os.Stderr, "warning: could not make %s executable: %v\n", script, err)
			}
		}
	}

	fmt.Printf("Installed adapter '%s' to %s\n", name, destDir)
	return nil
}

// isGitURL returns true if the source looks like a git URL.
func isGitURL(source string) bool {
	// Match common git URL patterns
	return strings.HasPrefix(source, "https://") ||
		strings.HasPrefix(source, "http://") ||
		strings.HasPrefix(source, "git@") ||
		strings.HasPrefix(source, "ssh://") ||
		strings.HasPrefix(source, "git://")
}

// cloneRepo clones a git repository to a temporary directory.
func cloneRepo(url string) (string, error) {
	// Check if git is available
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git is required for installing from URL: %w", err)
	}

	// Create temp directory
	tempDir, err := os.MkdirTemp("", "meow-adapter-*")
	if err != nil {
		return "", fmt.Errorf("creating temp directory: %w", err)
	}

	// Clone with depth 1 to save bandwidth
	cmd := exec.Command("git", "clone", "--depth", "1", url, tempDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("cloning repository: %w", err)
	}

	return tempDir, nil
}

// extractRepoName extracts the repository name from a git URL.
func extractRepoName(url string) string {
	// Handle common URL patterns:
	// https://github.com/user/repo-name
	// https://github.com/user/repo-name.git
	// git@github.com:user/repo-name.git

	// Remove trailing .git
	url = strings.TrimSuffix(url, ".git")

	// Get the last path component
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		name := parts[len(parts)-1]
		// For git@ URLs, handle the : separator
		if colonIdx := strings.LastIndex(name, ":"); colonIdx >= 0 {
			name = name[colonIdx+1:]
		}
		return name
	}
	return "adapter"
}

// isValidAdapterName checks if the name is valid for an adapter.
func isValidAdapterName(name string) bool {
	// Allow alphanumeric, hyphens, and underscores
	validName := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
	return validName.MatchString(name)
}

// loadAdapterConfig loads and parses an adapter.toml file.
func loadAdapterConfig(path string) (*types.AdapterConfig, error) {
	var cfg types.AdapterConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return &cfg, nil
}

// copyDir recursively copies a directory tree.
func copyDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		// Skip .git directory
		if entry.Name() == ".git" {
			continue
		}

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
