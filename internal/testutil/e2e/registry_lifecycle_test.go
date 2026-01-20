package e2e_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/akatz-ai/meow/internal/testutil/e2e"
)

// ===========================================================================
// Registry Lifecycle E2E Tests
// ===========================================================================
//
// Spec: specs/registry-distribution.yaml
//
// These tests verify the complete registry lifecycle:
// - registry add -> list -> show -> update -> remove
// - install -> collection list -> show -> run -> remove
//
// Unlike the execution-focused tests in registry_e2e_test.go, these tests
// verify the full CLI workflow using isolated HOME directories.

// TestE2E_RegistryLifecycle tests the complete registry add -> list -> show -> update -> remove cycle.
// Spec: registry-lifecycle
func TestE2E_RegistryLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)
	r := h.NewRegistryTestSetup()

	// Create test registry with a collection
	registry := r.CreateTestRegistry("lifecycle-registry", "1.0.0")
	r.AddCollection(registry, "test-collection")
	r.CommitRegistry(registry, "Initial commit with test-collection")

	// Spec: registry-lifecycle.add-registry-local
	// Add registry from local git repo
	t.Run("add-registry-local", func(t *testing.T) {
		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "registry", "add", registry.Path)
		if err != nil {
			t.Fatalf("registry add failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		output := stdout + stderr
		if !strings.Contains(output, "Added registry:") {
			t.Errorf("expected 'Added registry:' in output, got:\n%s", output)
		}
		if !strings.Contains(output, "collections available") {
			t.Errorf("expected 'collections available' in output, got:\n%s", output)
		}

		// Verify registries.json was created
		if !e2e.FileExists(t, r.RegistriesJSONPath()) {
			t.Errorf("expected registries.json to be created at %s", r.RegistriesJSONPath())
		}
	})

	// Spec: registry-lifecycle.list-registries
	// List shows registered registries
	t.Run("list-registries", func(t *testing.T) {
		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "registry", "list")
		if err != nil {
			t.Fatalf("registry list failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		output := stdout + stderr
		if !strings.Contains(output, "lifecycle-registry") {
			t.Errorf("expected registry name in list output, got:\n%s", output)
		}
		if !strings.Contains(output, "1.0.0") {
			t.Errorf("expected version in list output, got:\n%s", output)
		}
	})

	// Spec: registry-lifecycle.show-registry
	// Show displays registry details and collections
	t.Run("show-registry", func(t *testing.T) {
		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "registry", "show", "lifecycle-registry")
		if err != nil {
			t.Fatalf("registry show failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		output := stdout + stderr
		if !strings.Contains(output, "Registry:") || !strings.Contains(output, "lifecycle-registry") {
			t.Errorf("expected registry details in show output, got:\n%s", output)
		}
		if !strings.Contains(output, "test-collection") {
			t.Errorf("expected collection name in show output, got:\n%s", output)
		}
	})

	// Spec: registry-lifecycle.update-registry
	// Update fetches new version from remote
	t.Run("update-registry", func(t *testing.T) {
		// First, update the source registry to a new version
		r.UpdateRegistryVersion(registry, "1.1.0")

		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "registry", "update", "lifecycle-registry")
		if err != nil {
			t.Fatalf("registry update failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		output := stdout + stderr
		if !strings.Contains(output, "1.1.0") {
			t.Errorf("expected new version in update output, got:\n%s", output)
		}
	})

	// Spec: registry-lifecycle.remove-registry
	// Remove unregisters and cleans up
	t.Run("remove-registry", func(t *testing.T) {
		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "registry", "remove", "lifecycle-registry")
		if err != nil {
			t.Fatalf("registry remove failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		output := stdout + stderr
		if !strings.Contains(output, "Removed registry") {
			t.Errorf("expected 'Removed registry' in output, got:\n%s", output)
		}

		// Verify list now shows no registries
		stdout, _, _ = runMeowWithEnv(h, r.Env(), "registry", "list")
		if !strings.Contains(stdout, "No registries") {
			t.Errorf("expected empty registry list after remove, got:\n%s", stdout)
		}
	})
}

// TestE2E_CollectionInstallCycle tests the complete install -> list -> show -> run -> remove cycle.
// Spec: collection-install
func TestE2E_CollectionInstallCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)
	r := h.NewRegistryTestSetup()

	// Setup: Create and add a registry
	registry := r.CreateTestRegistry("install-test-registry", "1.0.0")
	r.AddCollection(registry, "installable-collection")
	r.CommitRegistry(registry, "Initial commit")

	// Add the registry first
	stdout, stderr, err := runMeowWithEnv(h, r.Env(), "registry", "add", registry.Path)
	if err != nil {
		t.Fatalf("setup: registry add failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Spec: collection-install.install-from-registry
	// Install collection from registered registry
	t.Run("install-from-registry", func(t *testing.T) {
		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "install", "installable-collection@install-test-registry")
		if err != nil {
			t.Fatalf("install failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		output := stdout + stderr
		if !strings.Contains(output, "Installed") {
			t.Errorf("expected 'Installed' in output, got:\n%s", output)
		}

		// Verify collection directory was created
		collPath := r.InstalledCollectionPath("installable-collection")
		if !e2e.DirExists(t, collPath) {
			t.Errorf("expected collection directory at %s", collPath)
		}

		// Verify manifest exists
		manifestPath := collPath + "/.meow/manifest.json"
		if !e2e.FileExists(t, manifestPath) {
			t.Errorf("expected manifest.json at %s", manifestPath)
		}

		// Verify installed.json was created
		if !e2e.FileExists(t, r.InstalledJSONPath()) {
			t.Errorf("expected installed.json at %s", r.InstalledJSONPath())
		}
	})

	// Spec: collection-install.collection-list
	// Collection list shows installed with metadata
	t.Run("collection-list", func(t *testing.T) {
		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "collection", "list")
		if err != nil {
			t.Fatalf("collection list failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		output := stdout + stderr
		if !strings.Contains(output, "installable-collection") {
			t.Errorf("expected collection name in list output, got:\n%s", output)
		}
		if !strings.Contains(output, "install-test-registry") {
			t.Errorf("expected registry name in list output, got:\n%s", output)
		}
	})

	// Spec: collection-install.collection-show
	// Collection show displays manifest and workflows
	t.Run("collection-show", func(t *testing.T) {
		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "collection", "show", "installable-collection")
		if err != nil {
			t.Fatalf("collection show failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		output := stdout + stderr
		if !strings.Contains(output, "Collection:") {
			t.Errorf("expected 'Collection:' in output, got:\n%s", output)
		}
		if !strings.Contains(output, "Entrypoint:") {
			t.Errorf("expected 'Entrypoint:' in output, got:\n%s", output)
		}
	})

	// Spec: collection-execution.run-entrypoint
	// Run collection by name uses entrypoint
	t.Run("run-entrypoint", func(t *testing.T) {
		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "run", "installable-collection")
		if err != nil {
			t.Fatalf("run failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		if !strings.Contains(stderr, "workflow completed") {
			t.Errorf("expected 'workflow completed' in output, got:\n%s", stderr)
		}
	})

	// Spec: workflow-discovery.ls-shows-collections
	// meow ls displays installed collections
	t.Run("ls-shows-collections", func(t *testing.T) {
		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "ls")
		if err != nil {
			t.Fatalf("ls failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		output := stdout + stderr
		if !strings.Contains(output, "installable-collection") {
			t.Errorf("expected collection in ls output, got:\n%s", output)
		}
		if !strings.Contains(output, "collection") {
			t.Errorf("expected '(collection)' marker in ls output, got:\n%s", output)
		}
	})

	// Spec: collection-install.collection-remove
	// Collection remove uninstalls cleanly
	t.Run("collection-remove", func(t *testing.T) {
		stdout, stderr, err := runMeowWithEnv(h, r.Env(), "collection", "remove", "installable-collection")
		if err != nil {
			t.Fatalf("collection remove failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}

		output := stdout + stderr
		if !strings.Contains(output, "Removed collection") {
			t.Errorf("expected 'Removed collection' in output, got:\n%s", output)
		}

		// Verify directory was removed
		collPath := r.InstalledCollectionPath("installable-collection")
		if e2e.DirExists(t, collPath) {
			t.Errorf("expected collection directory to be removed: %s", collPath)
		}
	})
}

// TestE2E_CollectionExpandWithinCollection tests that expand resolves within collection.
// Spec: collection-expand.expand-resolves-within-collection
//
// This is the KEY FEATURE that makes collections self-contained.
func TestE2E_CollectionExpandWithinCollection(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)
	r := h.NewRegistryTestSetup()

	// Create registry with expand-using collection
	registry := r.CreateTestRegistry("expand-registry", "1.0.0")
	r.AddCollectionWithExpand(registry, "expand-test")
	r.CommitRegistry(registry, "Initial commit with expand collection")

	// Add registry and install collection
	stdout, stderr, err := runMeowWithEnv(h, r.Env(), "registry", "add", registry.Path)
	if err != nil {
		t.Fatalf("registry add failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	stdout, stderr, err = runMeowWithEnv(h, r.Env(), "install", "expand-test@expand-registry")
	if err != nil {
		t.Fatalf("install failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Run the collection - this exercises collection-relative expand
	stdout, stderr, err = runMeowWithEnv(h, r.Env(), "run", "expand-test")
	if err != nil {
		t.Fatalf("run with expand failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	// Verify workflow completed
	if !strings.Contains(stderr, "workflow completed") {
		t.Errorf("expected 'workflow completed' in output, got:\n%s", stderr)
	}

	// Verify the expand step was dispatched (check for expanded step ID)
	if !strings.Contains(stderr, "expand-helper") {
		t.Errorf("expected 'expand-helper' step to be dispatched, got:\n%s", stderr)
	}
}

// TestE2E_CollectionLsJSONMetadata tests that meow ls --json includes proper collection metadata.
// Spec: workflow-discovery.ls-json-includes-metadata
func TestE2E_CollectionLsJSONMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)
	r := h.NewRegistryTestSetup()

	// Setup: Create, add registry, and install collection
	registry := r.CreateTestRegistry("json-registry", "1.0.0")
	r.AddCollection(registry, "json-test-collection")
	r.CommitRegistry(registry, "Initial commit")

	runMeowWithEnv(h, r.Env(), "registry", "add", registry.Path)
	runMeowWithEnv(h, r.Env(), "install", "json-test-collection@json-registry")

	// Run ls --json
	stdout, _, err := runMeowWithEnv(h, r.Env(), "ls", "--json")
	if err != nil {
		t.Fatalf("ls --json failed: %v", err)
	}

	// Parse JSON
	var entries []struct {
		Workflow     string `json:"workflow"`
		IsCollection bool   `json:"isCollection"`
		Entrypoint   string `json:"entrypoint"`
	}
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("failed to parse JSON: %v\noutput: %s", err, stdout)
	}

	// Find our collection
	found := false
	for _, e := range entries {
		if e.Workflow == "json-test-collection" {
			found = true
			if !e.IsCollection {
				t.Errorf("expected isCollection=true")
			}
			if e.Entrypoint == "" {
				t.Errorf("expected entrypoint to be set")
			}
			break
		}
	}

	if !found {
		t.Errorf("collection not found in ls --json output")
	}
}

// TestE2E_ErrorHandling tests graceful error handling.
// Spec: error-handling
func TestE2E_ErrorHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	h := e2e.NewHarness(t)
	r := h.NewRegistryTestSetup()

	// Spec: error-handling.install-nonexistent-registry
	t.Run("install-nonexistent-registry", func(t *testing.T) {
		_, stderr, err := runMeowWithEnv(h, r.Env(), "install", "something@nonexistent-registry")

		// Should fail
		if err == nil {
			t.Errorf("expected install to fail for nonexistent registry")
		}

		output := stderr
		if !strings.Contains(strings.ToLower(output), "not found") && !strings.Contains(strings.ToLower(output), "error") {
			t.Errorf("expected error message about registry not found, got:\n%s", output)
		}
	})

	// Setup: add a registry for the next test
	registry := r.CreateTestRegistry("error-test-registry", "1.0.0")
	r.AddCollection(registry, "real-collection")
	r.CommitRegistry(registry, "Initial commit")
	runMeowWithEnv(h, r.Env(), "registry", "add", registry.Path)

	// Spec: error-handling.install-nonexistent-collection
	t.Run("install-nonexistent-collection", func(t *testing.T) {
		_, stderr, err := runMeowWithEnv(h, r.Env(), "install", "nonexistent@error-test-registry")

		// Should fail
		if err == nil {
			t.Errorf("expected install to fail for nonexistent collection")
		}

		output := stderr
		if !strings.Contains(strings.ToLower(output), "not found") && !strings.Contains(strings.ToLower(output), "error") {
			t.Errorf("expected error message about collection not found, got:\n%s", output)
		}
	})
}

// ===========================================================================
// Helper: Run meow with custom environment
// ===========================================================================

// runMeowWithEnv runs meow with custom environment variables.
// This is needed to test registry commands which use HOME for ~/.meow/.
func runMeowWithEnv(h *e2e.Harness, env []string, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "/tmp/meow-e2e-bin", args...)
	cmd.Dir = h.TempDir
	cmd.Env = env
	// Add PATH so meow can find git and other tools
	cmd.Env = append(cmd.Env, "PATH=/usr/local/bin:/usr/bin:/bin")

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
