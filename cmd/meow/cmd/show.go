package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/akatz-ai/meow/internal/workflow"
	"github.com/spf13/cobra"
)

var (
	showRaw  bool
	showJSON bool
)

var showCmd = &cobra.Command{
	Use:   "show <workflow>",
	Short: "Show workflow details",
	Long: `Display details about a workflow.

By default, shows a summary including:
  - Description and source location
  - Variables (with defaults and descriptions)
  - Steps overview (executor type, dependencies)

Use --raw to output the full TOML file contents.

Examples:
  meow show sprint              # Summary of sprint workflow
  meow show lib/agent-persistence  # Summary of library workflow
  meow show sprint --raw        # Full TOML contents
  meow show sprint --json       # Machine-readable summary`,
	Args: cobra.ExactArgs(1),
	RunE: runShow,
}

func init() {
	showCmd.Flags().BoolVar(&showRaw, "raw", false, "output full TOML file contents")
	showCmd.Flags().BoolVar(&showJSON, "json", false, "output as JSON")
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) error {
	ref := args[0]

	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	loader := workflow.NewLoader(dir)
	location, err := loader.ResolveWorkflow(ref)
	if err != nil {
		return fmt.Errorf("workflow %q not found\n\nUse 'meow ls' to see available workflows", ref)
	}

	// Raw mode: just dump the file
	if showRaw {
		content, err := os.ReadFile(location.Path)
		if err != nil {
			return fmt.Errorf("reading workflow file: %w", err)
		}
		fmt.Print(string(content))
		return nil
	}

	// Parse the module for summary
	module, err := workflow.ParseModuleFile(location.Path)
	if err != nil {
		return fmt.Errorf("parsing workflow: %w", err)
	}

	if showJSON {
		return outputShowJSON(module, location)
	}

	return outputShowText(module, location)
}

// showJSONOutput is the JSON representation of a workflow summary.
type showJSONOutput struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Source      string                `json:"source"`
	Path        string                `json:"path"`
	Workflows   []workflowJSONSummary `json:"workflows"`
}

type workflowJSONSummary struct {
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Internal    bool                  `json:"internal,omitempty"`
	Variables   []variableJSONSummary `json:"variables,omitempty"`
	Steps       []stepJSONSummary     `json:"steps"`
	HasCleanup  bool                  `json:"has_cleanup,omitempty"`
}

type variableJSONSummary struct {
	Name        string `json:"name"`
	Required    bool   `json:"required,omitempty"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description,omitempty"`
}

type stepJSONSummary struct {
	ID       string   `json:"id"`
	Executor string   `json:"executor"`
	Template string   `json:"template,omitempty"`
	Needs    []string `json:"needs,omitempty"`
}

func outputShowJSON(module *workflow.Module, location *workflow.WorkflowLocation) error {
	out := showJSONOutput{
		Name:   location.Name,
		Source: location.Source,
		Path:   location.Path,
	}

	// Get description from the requested workflow
	if wf := module.GetWorkflow(location.Name); wf != nil {
		out.Description = wf.Description
	}

	// Add all workflows in the module
	for name, wf := range module.Workflows {
		wfSummary := workflowJSONSummary{
			Name:        name,
			Description: wf.Description,
			Internal:    wf.Internal,
			HasCleanup:  wf.CleanupOnSuccess != "" || wf.CleanupOnFailure != "" || wf.CleanupOnStop != "",
		}

		// Variables
		for varName, v := range wf.Variables {
			varSummary := variableJSONSummary{
				Name:        varName,
				Required:    v.Required,
				Description: v.Description,
			}
			if v.Default != nil {
				varSummary.Default = fmt.Sprintf("%v", v.Default)
			}
			wfSummary.Variables = append(wfSummary.Variables, varSummary)
		}
		sort.Slice(wfSummary.Variables, func(i, j int) bool {
			// Required variables first, then alphabetical
			if wfSummary.Variables[i].Required != wfSummary.Variables[j].Required {
				return wfSummary.Variables[i].Required
			}
			return wfSummary.Variables[i].Name < wfSummary.Variables[j].Name
		})

		// Steps
		for _, step := range wf.Steps {
			stepSummary := stepJSONSummary{
				ID:       step.ID,
				Executor: string(step.Executor),
				Needs:    step.Needs,
			}
			if step.Template != "" {
				stepSummary.Template = step.Template
			}
			wfSummary.Steps = append(wfSummary.Steps, stepSummary)
		}

		out.Workflows = append(out.Workflows, wfSummary)
	}

	// Sort workflows: requested workflow first, then alphabetically
	sort.Slice(out.Workflows, func(i, j int) bool {
		if out.Workflows[i].Name == location.Name {
			return true
		}
		if out.Workflows[j].Name == location.Name {
			return false
		}
		return out.Workflows[i].Name < out.Workflows[j].Name
	})

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func outputShowText(module *workflow.Module, location *workflow.WorkflowLocation) error {
	// Get the requested workflow, or use first available if main not found
	wf := module.GetWorkflow(location.Name)
	if wf == nil {
		// If no "main" workflow, show module overview instead
		return outputModuleOverview(module, location)
	}

	// Header
	fmt.Printf("Workflow: %s\n", location.Name)
	if wf.Description != "" {
		fmt.Printf("Description: %s\n", wf.Description)
	}
	fmt.Printf("Source: %s (%s)\n", location.Source, formatPath(location.Path))

	// Variables
	if len(wf.Variables) > 0 {
		fmt.Println()
		fmt.Println("Variables:")
		printVariables(wf.Variables)
	}

	// Steps
	fmt.Println()
	fmt.Printf("Steps (%d):\n", len(wf.Steps))
	printSteps(wf.Steps)

	// Cleanup info
	hasCleanup := wf.CleanupOnSuccess != "" || wf.CleanupOnFailure != "" || wf.CleanupOnStop != ""
	if hasCleanup {
		fmt.Println()
		fmt.Print("Cleanup: ")
		var cleanups []string
		if wf.CleanupOnSuccess != "" {
			cleanups = append(cleanups, "on_success")
		}
		if wf.CleanupOnFailure != "" {
			cleanups = append(cleanups, "on_failure")
		}
		if wf.CleanupOnStop != "" {
			cleanups = append(cleanups, "on_stop")
		}
		fmt.Println(strings.Join(cleanups, ", "))
	}

	// Other workflows in module
	if len(module.Workflows) > 1 {
		fmt.Println()
		fmt.Println("Other workflows in module:")
		var others []string
		for name, w := range module.Workflows {
			if name == location.Name {
				continue
			}
			suffix := ""
			if w.Internal {
				suffix = " (internal)"
			}
			others = append(others, fmt.Sprintf("  %s%s", name, suffix))
		}
		sort.Strings(others)
		for _, line := range others {
			fmt.Println(line)
		}
	}

	// Hint about --raw
	fmt.Println()
	fmt.Println("Use --raw to see full TOML contents")

	return nil
}

func printVariables(vars map[string]*workflow.Var) {
	// Sort: required first, then alphabetical
	type varEntry struct {
		name string
		v    *workflow.Var
	}
	var entries []varEntry
	for name, v := range vars {
		entries = append(entries, varEntry{name, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].v.Required != entries[j].v.Required {
			return entries[i].v.Required
		}
		return entries[i].name < entries[j].name
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, e := range entries {
		var parts []string

		// Name with required/default indicator
		if e.v.Required {
			parts = append(parts, fmt.Sprintf("  %s", e.name))
			parts = append(parts, "(required)")
		} else if e.v.Default != nil {
			parts = append(parts, fmt.Sprintf("  %s", e.name))
			defaultStr := fmt.Sprintf("%v", e.v.Default)
			if len(defaultStr) > 30 {
				defaultStr = defaultStr[:27] + "..."
			}
			parts = append(parts, fmt.Sprintf("= %q", defaultStr))
		} else {
			parts = append(parts, fmt.Sprintf("  %s", e.name))
			parts = append(parts, "(optional)")
		}

		// Description
		if e.v.Description != "" {
			desc := e.v.Description
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
			parts = append(parts, desc)
		}

		fmt.Fprintln(w, strings.Join(parts, "\t"))
	}
	w.Flush()
}

func printSteps(steps []*workflow.Step) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, step := range steps {
		var parts []string

		// ID and executor
		parts = append(parts, fmt.Sprintf("  %s", step.ID))
		parts = append(parts, string(step.Executor))

		// Template reference (for expand/foreach) or other key info
		detail := getStepDetail(step)
		if detail != "" {
			parts = append(parts, detail)
		}

		// Dependencies
		if len(step.Needs) > 0 {
			needs := strings.Join(step.Needs, ", ")
			if len(needs) > 40 {
				needs = needs[:37] + "..."
			}
			parts = append(parts, fmt.Sprintf("(needs: %s)", needs))
		}

		fmt.Fprintln(w, strings.Join(parts, "\t"))
	}
	w.Flush()
}

func getStepDetail(step *workflow.Step) string {
	switch step.Executor {
	case workflow.ExecutorExpand:
		if step.Template != "" {
			return fmt.Sprintf("→ %s", step.Template)
		}
	case workflow.ExecutorForeach:
		if step.Template != "" {
			return fmt.Sprintf("→ %s", step.Template)
		}
	case workflow.ExecutorSpawn:
		if step.Agent != "" {
			return fmt.Sprintf("agent=%s", step.Agent)
		}
	case workflow.ExecutorKill:
		if step.Agent != "" {
			return fmt.Sprintf("agent=%s", step.Agent)
		}
	case workflow.ExecutorAgent:
		if step.Agent != "" {
			return fmt.Sprintf("agent=%s", step.Agent)
		}
	case workflow.ExecutorBranch:
		if step.Condition != "" {
			cond := step.Condition
			if len(cond) > 40 {
				cond = cond[:37] + "..."
			}
			return fmt.Sprintf("if: %s", cond)
		}
	case workflow.ExecutorShell:
		if step.Command != "" {
			cmd := step.Command
			// Take first line only
			if idx := strings.Index(cmd, "\n"); idx > 0 {
				cmd = cmd[:idx] + "..."
			}
			if len(cmd) > 40 {
				cmd = cmd[:37] + "..."
			}
			return cmd
		}
	}
	return ""
}

func formatPath(path string) string {
	// Try to make path relative to home or show as-is
	if home, err := os.UserHomeDir(); err == nil {
		if strings.HasPrefix(path, home) {
			return "~" + path[len(home):]
		}
	}
	return path
}

// extractModuleName extracts a short module name from a path.
// e.g., "/path/to/.meow/workflows/lib/agent-persistence.meow.toml" -> "lib/agent-persistence"
func extractModuleName(path string) string {
	// Find the workflows/ part and take everything after
	markers := []string{".meow/workflows/", ".meow\\workflows\\"}
	for _, marker := range markers {
		if idx := strings.Index(path, marker); idx >= 0 {
			name := path[idx+len(marker):]
			return strings.TrimSuffix(name, ".meow.toml")
		}
	}
	// Fallback: just use the filename without extension
	base := strings.TrimSuffix(path, ".meow.toml")
	if idx := strings.LastIndex(base, "/"); idx >= 0 {
		return base[idx+1:]
	}
	return base
}

// outputModuleOverview shows all workflows when no default "main" workflow exists.
func outputModuleOverview(module *workflow.Module, location *workflow.WorkflowLocation) error {
	// Extract module name from path (e.g., "lib/agent-persistence" from full path)
	moduleName := extractModuleName(location.Path)
	fmt.Printf("Module: %s\n", moduleName)
	fmt.Printf("Source: %s (%s)\n", location.Source, formatPath(location.Path))

	// Collect and sort workflows
	var names []string
	for name := range module.Workflows {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Printf("\nWorkflows (%d):\n", len(names))
	for _, name := range names {
		wf := module.Workflows[name]
		internal := ""
		if wf.Internal {
			internal = " (internal)"
		}

		desc := wf.Description
		if len(desc) > 50 {
			desc = desc[:47] + "..."
		}

		if desc != "" {
			fmt.Printf("  %s%s - %s\n", name, internal, desc)
		} else {
			fmt.Printf("  %s%s\n", name, internal)
		}
	}

	fmt.Println()
	fmt.Println("Use 'meow show <file>#<workflow>' to see a specific workflow")
	fmt.Println("Use --raw to see full TOML contents")

	return nil
}
