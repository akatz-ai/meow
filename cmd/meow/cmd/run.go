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
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/spf13/cobra"
)

var runCmd = &cobra.Command{
	Use:   "run <template>",
	Short: "Run a workflow from a template",
	Long: `Start a workflow by baking a template into steps and running the orchestrator.

The template is loaded and baked into steps which are persisted in
.meow/workflows/<id>.yaml. The orchestrator then executes the workflow.`,
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
	templatePath := args[0]
	ctx := context.Background()

	// Get working directory
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// Ensure .meow/workflows directory exists
	workflowsDir := filepath.Join(dir, ".meow", "workflows")
	if err := os.MkdirAll(workflowsDir, 0755); err != nil {
		return fmt.Errorf("creating workflows dir: %w", err)
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
	templateWorkflow := module.GetWorkflow(runWorkflow)
	if templateWorkflow == nil {
		// List available workflows for better error message
		var available []string
		for name := range module.Workflows {
			available = append(available, name)
		}
		return fmt.Errorf("workflow %q not found in template. Available: %v", runWorkflow, available)
	}

	// Generate a unique workflow ID
	workflowID := fmt.Sprintf("wf-%d", time.Now().UnixNano())

	// Create baker
	baker := template.NewBaker(workflowID)

	// Bake the workflow into steps
	result, err := baker.BakeWorkflow(templateWorkflow, vars)
	if err != nil {
		return fmt.Errorf("baking workflow: %w", err)
	}

	if runDry {
		fmt.Printf("Would create workflow with %d steps from template: %s (workflow: %s)\n", len(result.Steps), templatePath, runWorkflow)
		fmt.Printf("Workflow ID: %s\n", result.WorkflowID)
		fmt.Println()
		for _, step := range result.Steps {
			fmt.Printf("  %s [%s]\n", step.ID, step.Executor)
			if len(step.Needs) > 0 {
				fmt.Printf("    needs: %v\n", step.Needs)
			}
		}
		return nil
	}

	// Create a Workflow object
	wf := types.NewWorkflow(workflowID, templatePath, vars)

	// Add all steps to the workflow
	for _, step := range result.Steps {
		if err := wf.AddStep(step); err != nil {
			return fmt.Errorf("adding step %s: %w", step.ID, err)
		}
	}

	// Create workflow store
	store, err := orchestrator.NewYAMLWorkflowStore(workflowsDir)
	if err != nil {
		return fmt.Errorf("opening workflow store: %w", err)
	}
	defer store.Close()

	// Persist the workflow
	if err := store.Create(ctx, wf); err != nil {
		return fmt.Errorf("creating workflow: %w", err)
	}

	// Output success
	fmt.Printf("Created workflow with %d steps from template: %s\n", len(result.Steps), filepath.Base(templatePath))
	fmt.Printf("Workflow ID: %s\n", result.WorkflowID)
	if verbose {
		fmt.Println("\nSteps created:")
		for _, step := range result.Steps {
			fmt.Printf("  %s [%s]\n", step.ID, step.Executor)
		}
	}

	// TODO: Run the orchestrator once it's refactored to use WorkflowStore and Steps
	// For now, just display information about the workflow
	fmt.Printf("\nWorkflow saved to: %s\n", filepath.Join(workflowsDir, workflowID+".yaml"))
	fmt.Println("Note: Orchestrator execution is pending orchestrator refactor to workflow-centric model.")

	return nil
}
