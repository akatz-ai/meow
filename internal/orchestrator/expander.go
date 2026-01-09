// Package orchestrator provides the core workflow execution engine.
package orchestrator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/meow-stack/meow-machine/internal/template"
	"github.com/meow-stack/meow-machine/internal/types"
)

// ExpandResult contains the steps generated from expanding a template.
type ExpandResult struct {
	Steps      []*types.Step
	WorkflowID string
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
	if config == nil {
		return nil, fmt.Errorf("expand config is nil")
	}
	if config.Template == "" {
		return nil, fmt.Errorf("template is empty")
	}

	templateRef := config.Template
	var workflow *template.Workflow
	var workflowID string

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
		module, err := template.ParseModuleFile(modulePath)
		if err != nil {
			return nil, fmt.Errorf("loading source module %s: %w", modulePath, err)
		}

		// Get the referenced workflow (strip the leading dot)
		workflowName := templateRef[1:]
		workflow = module.GetWorkflow(workflowName)
		if workflow == nil {
			return nil, fmt.Errorf("workflow %q not found in %s", workflowName, modulePath)
		}
	}

	// Check for file#workflow format
	if workflow != nil {
		// Already resolved (local reference)
	} else if strings.Contains(templateRef, "#") {
		parts := strings.SplitN(templateRef, "#", 2)
		filePath := parts[0]
		workflowName := parts[1]

		// Resolve file path
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(e.BaseDir, filePath)
		}

		// Load the module
		module, err := template.ParseModuleFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("loading module %s: %w", filePath, err)
		}

		workflow = module.GetWorkflow(workflowName)
		if workflow == nil {
			return nil, fmt.Errorf("workflow %q not found in %s", workflowName, filePath)
		}
	} else if strings.HasSuffix(templateRef, ".toml") || strings.Contains(templateRef, "/") {
		// File path
		filePath := templateRef
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(e.BaseDir, filePath)
		}

		// Load the module
		module, err := template.ParseModuleFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("loading module %s: %w", filePath, err)
		}

		workflow = module.DefaultWorkflow()
		if workflow == nil {
			// Try to get any workflow
			for _, w := range module.Workflows {
				workflow = w
				break
			}
		}
		if workflow == nil {
			return nil, fmt.Errorf("no workflow found in %s", filePath)
		}
	} else {
		// Template name - use loader
		return nil, fmt.Errorf("named template loading not yet supported for expansion: %s", templateRef)
	}

	// Create the baker
	baker := template.NewBaker(workflowID)

	// Bake the workflow
	result, err := baker.BakeWorkflow(workflow, config.Variables)
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

		// Steps with no dependencies should depend on parent completing
		// (This will be handled by the orchestrator when inserting)
	}

	return &ExpandResult{
		Steps:      result.Steps,
		WorkflowID: workflowID,
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
func (a *TemplateExpanderAdapter) Expand(ctx context.Context, wf *types.Workflow, step *types.Step) error {
	if step.Expand == nil {
		return fmt.Errorf("step %s missing expand config", step.ID)
	}

	// Call the underlying expander, passing the workflow's source template for local refs
	result, err := a.Expander.Expand(ctx, step.Expand, step.ID, wf.ID, wf.Template)
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
