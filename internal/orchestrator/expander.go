// Package orchestrator provides the core workflow execution engine.
package orchestrator

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/meow-stack/meow-machine/internal/workflow"
)

func isExplicitWorkflowPath(ref string) bool {
	return filepath.IsAbs(ref) || strings.HasPrefix(ref, ".") || strings.HasSuffix(ref, ".toml")
}

func resolveWorkflowPath(baseDir, ref string) (string, error) {
	path := ref
	if !filepath.IsAbs(path) {
		path = filepath.Join(baseDir, path)
	}

	if !strings.HasSuffix(path, ".toml") {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			path = path + ".meow.toml"
		}
	}

	if _, err := os.Stat(path); err != nil {
		return "", err
	}

	return path, nil
}

// ExpandResult contains the steps generated from expanding a template.
type ExpandResult struct {
	Steps      []*types.Step
	WorkflowID string
}

// ExpandOptions configures template expansion behavior.
type ExpandOptions struct {
	// DeferUndefinedVariables leaves undefined {{variable}} placeholders as-is
	// instead of causing an error. Used by foreach to defer item_var substitution.
	DeferUndefinedVariables bool
}

// FileTemplateExpander expands templates using the template package.
// It loads templates from the filesystem and bakes them into steps.
type FileTemplateExpander struct {
	// BaseDir is the project root directory
	BaseDir string
}

// NewFileTemplateExpander creates a new FileTemplateExpander.
func NewFileTemplateExpander(baseDir string) *FileTemplateExpander {
	return &FileTemplateExpander{
		BaseDir: baseDir,
	}
}

// Expand loads a template and bakes it into steps.
// The caller is responsible for inserting the steps into the workflow.
// sourceModule is the path to the source module file, used to resolve local workflow references.
func (e *FileTemplateExpander) Expand(ctx context.Context, config *types.ExpandConfig, parentStepID string, parentWorkflowID string, sourceModule string) (*ExpandResult, error) {
	return e.ExpandWithOptions(ctx, config, parentStepID, parentWorkflowID, sourceModule, nil)
}

// ExpandWithOptions loads a template and bakes it into steps with configurable options.
func (e *FileTemplateExpander) ExpandWithOptions(ctx context.Context, config *types.ExpandConfig, parentStepID string, parentWorkflowID string, sourceModule string, opts *ExpandOptions) (*ExpandResult, error) {
	if config == nil {
		return nil, fmt.Errorf("expand config is nil")
	}
	if config.Template == "" {
		return nil, fmt.Errorf("template is empty")
	}

	templateRef := config.Template
	var wf *workflow.Workflow
	var workflowID string
	var resolvedModulePath string // Track which module file was resolved, for local refs in nested templates

	// Determine the workflow ID
	if parentWorkflowID != "" {
		workflowID = parentWorkflowID + "." + sanitizeID(templateRef)
	} else {
		workflowID = "expanded-" + sanitizeID(templateRef)
	}

	// Handle different reference formats
	if strings.HasPrefix(templateRef, ".") && !strings.Contains(templateRef, "/") && !strings.Contains(templateRef, "\\") {
		// Local workflow reference like ".agent-lifecycle"
		if sourceModule == "" {
			return nil, fmt.Errorf("local workflow references (.name) require module context, but no source module provided")
		}

		// Resolve source module path
		modulePath := sourceModule
		if !filepath.IsAbs(modulePath) {
			modulePath = filepath.Join(e.BaseDir, modulePath)
		}

		// Load the source module
		module, err := workflow.ParseModuleFile(modulePath)
		if err != nil {
			return nil, fmt.Errorf("loading source module %s: %w", modulePath, err)
		}

		// Get the referenced workflow (strip the leading dot)
		workflowName := templateRef[1:]
		wf = module.GetWorkflow(workflowName)
		if wf == nil {
			return nil, fmt.Errorf("workflow %q not found in %s", workflowName, modulePath)
		}
		resolvedModulePath = modulePath
	}

	// Check for file#workflow format
	if wf != nil {
		// Already resolved (local reference)
	} else if strings.Contains(templateRef, "#") {
		parts := strings.SplitN(templateRef, "#", 2)
		fileRef := strings.TrimSpace(parts[0])
		workflowName := strings.TrimSpace(parts[1])
		if fileRef == "" || workflowName == "" {
			return nil, fmt.Errorf("invalid workflow reference: %s", templateRef)
		}

		if isExplicitWorkflowPath(fileRef) {
			modulePath, err := resolveWorkflowPath(e.BaseDir, fileRef)
			if err != nil {
				return nil, fmt.Errorf("resolving module path %s: %w", fileRef, err)
			}

			module, err := workflow.ParseModuleFile(modulePath)
			if err != nil {
				return nil, fmt.Errorf("loading module %s: %w", modulePath, err)
			}

			wf = module.GetWorkflow(workflowName)
			if wf == nil {
				return nil, fmt.Errorf("workflow %q not found in %s", workflowName, modulePath)
			}
			resolvedModulePath = modulePath
		} else {
			loader := workflow.NewLoader(e.BaseDir)
			loaded, err := loader.LoadWorkflow(templateRef)
			if err != nil {
				return nil, err
			}
			wf = loaded.Workflow
			resolvedModulePath = loaded.Path
		}
	} else if isExplicitWorkflowPath(templateRef) {
		modulePath, err := resolveWorkflowPath(e.BaseDir, templateRef)
		if err != nil {
			return nil, fmt.Errorf("resolving module path %s: %w", templateRef, err)
		}

		module, err := workflow.ParseModuleFile(modulePath)
		if err != nil {
			return nil, fmt.Errorf("loading module %s: %w", modulePath, err)
		}

		wf = module.GetWorkflow("main")
		if wf == nil {
			var available []string
			for name := range module.Workflows {
				available = append(available, name)
			}
			return nil, fmt.Errorf("workflow %q not found in %s (available: %v)", "main", modulePath, available)
		}
		resolvedModulePath = modulePath
	} else {
		loader := workflow.NewLoader(e.BaseDir)
		loaded, err := loader.LoadWorkflow(templateRef)
		if err != nil {
			return nil, err
		}
		wf = loaded.Workflow
		resolvedModulePath = loaded.Path
	}

	// Create the baker
	baker := workflow.NewBaker(workflowID)

	// Apply options
	if opts != nil && opts.DeferUndefinedVariables {
		baker.VarContext.DeferUndefinedVariables = true
	}

	// Bake the workflow
	result, err := baker.BakeWorkflow(wf, config.Variables)
	if err != nil {
		return nil, fmt.Errorf("baking workflow: %w", err)
	}

	// Prefix step IDs with parent step ID and set up dependencies
	for _, step := range result.Steps {
		// Prefix the step ID: parent.child
		step.ID = parentStepID + "." + step.ID

		// Update internal dependencies to use prefixed IDs
		for i, need := range step.Needs {
			step.Needs[i] = parentStepID + "." + need
		}

		// Set the source module for local reference resolution in nested templates
		step.SourceModule = resolvedModulePath

		// Steps with no dependencies should depend on parent completing
		// (This will be handled by the orchestrator when inserting)
	}

	return &ExpandResult{
		Steps:      result.Steps,
		WorkflowID: workflowID,
	}, nil
}

// resolveExpandConfig resolves any step output references in the expand config's variables.
// This is called at runtime when the expand step executes, so step outputs are available.
func (a *TemplateExpanderAdapter) resolveExpandConfig(wf *types.Run, config *types.ExpandConfig) (*types.ExpandConfig, error) {
	if len(config.Variables) == 0 {
		return config, nil
	}

	// Create a VarContext with access to step outputs
	vc := workflow.NewVarContext()

	// Add workflow variables
	for k, v := range wf.Variables {
		vc.Set(k, v)
	}

	// Add step outputs from all done steps
	for stepID, step := range wf.Steps {
		if step.Status == types.StepStatusDone && step.Outputs != nil {
			vc.SetOutputs(stepID, step.Outputs)
		}
	}

	// Resolve variables using EvalMap which preserves types for pure references
	resolvedVars, err := vc.EvalMap(config.Variables)
	if err != nil {
		return nil, err
	}

	// Return a copy with resolved variables
	return &types.ExpandConfig{
		Template:  config.Template,
		Variables: resolvedVars,
	}, nil
}

// sanitizeID creates a safe ID component from a template reference.
func sanitizeID(ref string) string {
	// Remove path separators and extensions
	ref = filepath.Base(ref)
	ref = strings.TrimSuffix(ref, ".toml")
	ref = strings.ReplaceAll(ref, "#", "-")
	ref = strings.ReplaceAll(ref, ".", "-")
	return ref
}

// TemplateExpanderAdapter adapts FileTemplateExpander to the TemplateExpander interface.
// It wraps Expand to work directly with workflow and step objects.
type TemplateExpanderAdapter struct {
	Expander *FileTemplateExpander
}

// NewTemplateExpanderAdapter creates a new adapter wrapping a FileTemplateExpander.
func NewTemplateExpanderAdapter(baseDir string) *TemplateExpanderAdapter {
	return &TemplateExpanderAdapter{
		Expander: NewFileTemplateExpander(baseDir),
	}
}

// Expand implements TemplateExpander interface.
// It expands a template and inserts the resulting steps into the workflow.
func (a *TemplateExpanderAdapter) Expand(ctx context.Context, wf *types.Run, step *types.Step) error {
	if step.Expand == nil {
		return fmt.Errorf("step %s missing expand config", step.ID)
	}

	// Use step's SourceModule if set (for nested expansions), otherwise fall back to workflow template
	sourceModule := step.SourceModule
	if sourceModule == "" {
		sourceModule = wf.Template
	}

	// Resolve any step output references in variables at runtime
	// This is needed because references like "{{init.outputs.config}}" are deferred at bake time
	resolvedConfig, err := a.resolveExpandConfig(wf, step.Expand)
	if err != nil {
		return fmt.Errorf("resolving expand variables: %w", err)
	}

	// Call the underlying expander, passing the source module for local refs
	result, err := a.Expander.Expand(ctx, resolvedConfig, step.ID, wf.ID, sourceModule)
	if err != nil {
		return err
	}

	// Insert expanded steps into workflow
	if len(result.Steps) > 0 {
		// Collect child step IDs
		childIDs := make([]string, len(result.Steps))
		for i, s := range result.Steps {
			childIDs[i] = s.ID
		}

		// Mark the expand step with what it expanded into
		step.ExpandedInto = childIDs

		// Insert expanded steps into workflow's step map
		if wf.Steps == nil {
			wf.Steps = make(map[string]*types.Step)
		}
		for _, s := range result.Steps {
			wf.Steps[s.ID] = s
		}
	}

	return nil
}
