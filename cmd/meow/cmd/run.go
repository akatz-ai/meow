package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/template"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <template>",
	Short: "Run a workflow from a template",
	Long: `Start a workflow by baking a template into beads and running the orchestrator.

The template is loaded from .meow/templates/<name>.toml and expanded into
beads in the .beads directory. The orchestrator then executes the workflow.`,
	Args: cobra.ExactArgs(1),
	RunE: runRun,
}

var (
	runDry      bool
	runVars     []string
	runWorkflow string
)

func init() {
	runCmd.Flags().BoolVar(&runDry, "dry-run", false, "validate and show what would be created without executing")
	runCmd.Flags().StringArrayVar(&runVars, "var", nil, "variable values (format: name=value)")
	runCmd.Flags().StringVar(&runWorkflow, "workflow", "main", "workflow name to run (default: main)")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	// For run command, we need .beads directory (not necessarily .meow)
	if err := checkBeadsDir(); err != nil {
		return err
	}

	templatePath := args[0]
	ctx := context.Background()

	// Get working directory
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// If template path is not absolute, resolve it
	if !filepath.IsAbs(templatePath) {
		templatePath = filepath.Join(dir, templatePath)
	}

	// Check template file exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		return fmt.Errorf("template file not found: %s", templatePath)
	}

	// Parse variables from flags
	vars := make(map[string]string)
	for _, v := range runVars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid variable format: %s (expected name=value)", v)
		}
		vars[parts[0]] = parts[1]
	}

	// Load module file
	module, err := template.ParseModuleFile(templatePath)
	if err != nil {
		return fmt.Errorf("parsing template: %w", err)
	}

	// Get workflow (use flag or default to "main")
	workflow := module.GetWorkflow(runWorkflow)
	if workflow == nil {
		// List available workflows for better error message
		var available []string
		for name := range module.Workflows {
			available = append(available, name)
		}
		return fmt.Errorf("workflow %q not found in template. Available: %v", runWorkflow, available)
	}

	// Generate a unique workflow ID
	workflowID := fmt.Sprintf("meow-%d", time.Now().UnixNano())

	// Create baker
	baker := template.NewBaker(workflowID)

	// Bake the workflow into beads
	result, err := baker.BakeWorkflow(workflow, vars)
	if err != nil {
		return fmt.Errorf("baking workflow: %w", err)
	}

	if runDry {
		fmt.Printf("Would create %d beads from template: %s (workflow: %s)\n", len(result.Beads), templatePath, runWorkflow)
		fmt.Printf("Workflow ID: %s\n", result.WorkflowID)
		fmt.Println()
		for _, bead := range result.Beads {
			fmt.Printf("  %s [%s] %s\n", bead.ID, bead.Type, bead.Title)
			if len(bead.Needs) > 0 {
				fmt.Printf("    needs: %v\n", bead.Needs)
			}
		}
		return nil
	}

	// Load bead store
	beadsDir := filepath.Join(dir, ".beads")
	store := orchestrator.NewFileBeadStore(beadsDir)
	if err := store.Load(ctx); err != nil {
		return fmt.Errorf("loading beads: %w", err)
	}

	// Write beads to store
	for _, bead := range result.Beads {
		if err := store.Create(ctx, bead); err != nil {
			return fmt.Errorf("creating bead %s: %w", bead.ID, err)
		}
	}

	// Output success
	fmt.Printf("Created %d beads from template: %s\n", len(result.Beads), filepath.Base(templatePath))
	fmt.Printf("Workflow ID: %s\n", result.WorkflowID)
	if verbose {
		fmt.Println("\nBeads created:")
		for _, bead := range result.Beads {
			fmt.Printf("  %s [%s] %s\n", bead.ID, bead.Type, bead.Title)
		}
	}
	fmt.Println("\nRun 'meow prime' to see your first task.")

	return nil
}
