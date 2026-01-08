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

// FileTemplateExpander expands templates using the template package.
// It loads templates from the filesystem and bakes them into beads.
type FileTemplateExpander struct {
	// BaseDir is the project root directory
	BaseDir string

	// Store is used to create the resulting beads
	Store BeadStore
}

// NewFileTemplateExpander creates a new FileTemplateExpander.
func NewFileTemplateExpander(baseDir string, store BeadStore) *FileTemplateExpander {
	return &FileTemplateExpander{
		BaseDir: baseDir,
		Store:   store,
	}
}

// Expand implements TemplateExpander.
// It loads a template and creates beads from it, linking them to the parent bead.
func (e *FileTemplateExpander) Expand(ctx context.Context, spec *types.ExpandSpec, parentBead *types.Bead) error {
	if spec == nil {
		return fmt.Errorf("expand spec is nil")
	}
	if spec.Template == "" {
		return fmt.Errorf("template is empty")
	}

	// Parse the template reference
	// Formats:
	//   - "name" - load by name from standard locations
	//   - "./path/file.toml" - load from relative path
	//   - "/abs/path/file.toml" - load from absolute path
	//   - ".workflow" - local reference (module format)
	//   - "file.toml#workflow" - file with specific workflow

	templateRef := spec.Template
	var workflow *template.Workflow
	var workflowID string

	// Determine the workflow ID
	if parentBead != nil && parentBead.WorkflowID != "" {
		// Use parent's workflow ID with extension
		workflowID = parentBead.WorkflowID + "." + sanitizeID(templateRef)
	} else {
		workflowID = "expanded-" + sanitizeID(templateRef)
	}

	// Handle different reference formats
	if strings.HasPrefix(templateRef, ".") && !strings.Contains(templateRef, "/") && !strings.Contains(templateRef, "\\") {
		// Local workflow reference like ".check-loop" - currently not supported without context
		return fmt.Errorf("local workflow references (.name) require module context, not supported in direct expansion")
	}

	// Check for file#workflow format
	if strings.Contains(templateRef, "#") {
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
			return fmt.Errorf("loading module %s: %w", filePath, err)
		}

		workflow = module.GetWorkflow(workflowName)
		if workflow == nil {
			return fmt.Errorf("workflow %q not found in %s", workflowName, filePath)
		}
	} else if strings.HasSuffix(templateRef, ".toml") || strings.Contains(templateRef, "/") {
		// File path
		filePath := templateRef
		if !filepath.IsAbs(filePath) {
			filePath = filepath.Join(e.BaseDir, filePath)
		}

		// Load the module (prefer module format)
		module, err := template.ParseModuleFile(filePath)
		if err != nil {
			return fmt.Errorf("loading module %s: %w", filePath, err)
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
			return fmt.Errorf("no workflow found in %s", filePath)
		}
	} else {
		// Template name - use loader
		return fmt.Errorf("named template loading not yet supported for expansion: %s", templateRef)
	}

	// Create the baker
	baker := template.NewBaker(workflowID)

	// Merge variables from spec with any defaults
	vars := make(map[string]string)
	for k, v := range spec.Variables {
		vars[k] = v
	}

	// Bake the workflow
	result, err := baker.BakeWorkflow(workflow, vars)
	if err != nil {
		return fmt.Errorf("baking workflow: %w", err)
	}

	// Check if store supports creation
	creator, ok := e.Store.(interface {
		Create(context.Context, *types.Bead) error
	})
	if !ok {
		return fmt.Errorf("store does not support creation")
	}

	// Link beads to parent and create them
	for _, bead := range result.Beads {
		if parentBead != nil {
			bead.Parent = parentBead.ID
			// First bead(s) without dependencies should depend on parent
			if len(bead.Needs) == 0 {
				bead.Needs = []string{parentBead.ID}
			}
		}

		if err := creator.Create(ctx, bead); err != nil {
			return fmt.Errorf("creating bead %s: %w", bead.ID, err)
		}
	}

	return nil
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
