package cmd

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/meow-stack/meow-machine/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	lsAll  bool
	lsJSON bool
)

type workflowEntry struct {
	Name        string
	Description string
	Source      string
	Path        string
}

var lsCmd = &cobra.Command{
	Use:   "ls [path]",
	Short: "List available workflows",
	Long: `List workflows from .meow/workflows and ~/.meow/workflows.

By default, lists only top-level workflows. Use --all to include
subdirectories, or pass a path to list a specific subdirectory.

Examples:
  meow ls           # Top-level workflows only
  meow ls -a        # Include subdirectories
  meow ls lib/      # List workflows under lib/`,
	Args: cobra.MaximumNArgs(1),
	RunE: runLs,
}

func init() {
	lsCmd.Flags().BoolVarP(&lsAll, "all", "a", false, "include subdirectories")
	lsCmd.Flags().BoolVar(&lsJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(lsCmd)
}

func runLs(cmd *cobra.Command, args []string) error {
	if err := checkWorkDir(); err != nil {
		return err
	}

	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	prefix := ""
	if len(args) == 1 {
		prefix = filepath.Clean(args[0])
		if filepath.IsAbs(prefix) {
			return fmt.Errorf("path must be relative: %s", args[0])
		}
		if prefix == "." {
			prefix = ""
		}
	}

	projectEntries, err := collectWorkflowEntries(filepath.Join(dir, ".meow", "workflows"), prefix, lsAll, "project")
	if err != nil {
		return err
	}

	userEntries := []workflowEntry{}
	if home, err := os.UserHomeDir(); err == nil {
		entries, err := collectWorkflowEntries(filepath.Join(home, ".meow", "workflows"), prefix, lsAll, "user")
		if err != nil {
			return err
		}
		userEntries = entries
	}

	entries := mergeWorkflowEntries(projectEntries, userEntries)
	if len(entries) == 0 {
		fmt.Println("No workflows found")
		return nil
	}

	if lsJSON {
		return printWorkflowsJSON(entries)
	}

	return printWorkflowsTable(entries)
}

func collectWorkflowEntries(workflowsDir, prefix string, recursive bool, source string) ([]workflowEntry, error) {
	if prefix != "" {
		prefix = filepath.Clean(prefix)
		if prefix == "." {
			prefix = ""
		}
	}

	baseDir := workflowsDir
	targetDir := workflowsDir
	if prefix != "" {
		targetDir = filepath.Join(workflowsDir, prefix)
	}

	if _, err := os.Stat(targetDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("checking workflows dir: %w", err)
	}

	var entries []workflowEntry
	if recursive {
		err := filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			entry, err := buildWorkflowEntry(baseDir, path, source)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				return nil
			}
			if entry.Name != "" {
				entries = append(entries, entry)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		dirEntries, err := os.ReadDir(targetDir)
		if err != nil {
			return nil, err
		}
		for _, entry := range dirEntries {
			if entry.IsDir() {
				continue
			}
			fullPath := filepath.Join(targetDir, entry.Name())
			workflowEntry, err := buildWorkflowEntry(baseDir, fullPath, source)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				continue
			}
			if workflowEntry.Name != "" {
				entries = append(entries, workflowEntry)
			}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries, nil
}

func buildWorkflowEntry(baseDir, path, source string) (workflowEntry, error) {
	if !strings.HasSuffix(path, ".meow.toml") {
		return workflowEntry{}, nil
	}

	rel, err := filepath.Rel(baseDir, path)
	if err != nil {
		return workflowEntry{}, err
	}

	name := strings.TrimSuffix(filepath.ToSlash(rel), ".meow.toml")
	module, err := workflow.ParseModuleFile(path)
	if err != nil {
		return workflowEntry{}, fmt.Errorf("parsing workflow %s: %w", path, err)
	}

	description := workflowDescription(module)

	return workflowEntry{
		Name:        name,
		Description: description,
		Source:      source,
		Path:        path,
	}, nil
}

func workflowDescription(module *workflow.Module) string {
	if module == nil {
		return ""
	}

	if wf := module.DefaultWorkflow(); wf != nil {
		return wf.Description
	}

	for _, wf := range module.Workflows {
		return wf.Description
	}

	return ""
}

func mergeWorkflowEntries(projectEntries, userEntries []workflowEntry) []workflowEntry {
	seen := make(map[string]bool)
	entries := make([]workflowEntry, 0, len(projectEntries)+len(userEntries))

	for _, entry := range projectEntries {
		if seen[entry.Name] {
			continue
		}
		seen[entry.Name] = true
		entries = append(entries, entry)
	}

	for _, entry := range userEntries {
		if seen[entry.Name] {
			continue
		}
		seen[entry.Name] = true
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name < entries[j].Name
	})

	return entries
}

func printWorkflowsTable(entries []workflowEntry) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "WORKFLOW\tDESCRIPTION\tSOURCE")

	for _, entry := range entries {
		fmt.Fprintf(w, "%s\t%s\t%s\n", entry.Name, entry.Description, entry.Source)
	}

	return w.Flush()
}

func printWorkflowsJSON(entries []workflowEntry) error {
	type workflowJSON struct {
		Workflow    string `json:"workflow"`
		Description string `json:"description,omitempty"`
		Source      string `json:"source"`
		Path        string `json:"path"`
	}

	out := make([]workflowJSON, len(entries))
	for i, entry := range entries {
		out[i] = workflowJSON{
			Workflow:    entry.Name,
			Description: entry.Description,
			Source:      entry.Source,
			Path:        entry.Path,
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
