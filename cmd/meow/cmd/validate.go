package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/meow-stack/meow-machine/internal/workflow"
	"github.com/spf13/cobra"
)

var validateCmd = &cobra.Command{
	Use:   "validate <template>",
	Short: "Validate a template",
	Long: `Validate a MEOW template without executing it.

Checks:
- TOML syntax
- Required fields
- Variable references
- Dependency cycles
- Output references`,
	Args: cobra.ExactArgs(1),
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	templatePath := args[0]

	// Get working directory
	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	// Resolve template path if not absolute
	if !filepath.IsAbs(templatePath) {
		templatePath = filepath.Join(dir, templatePath)
	}

	// Check if file exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		return fmt.Errorf("template file not found: %s", templatePath)
	}

	fmt.Printf("Validating template: %s\n", templatePath)

	// Parse the module file
	module, err := workflow.ParseModuleFile(templatePath)
	if err != nil {
		fmt.Printf("\n%s Parsing failed:\n", errorMark())
		fmt.Printf("  %v\n", err)
		return fmt.Errorf("validation failed")
	}

	fmt.Printf("%s Syntax OK\n", checkMark())

	// Run full validation
	result := workflow.ValidateFullModule(module)

	if result.HasErrors() {
		fmt.Printf("\n%s Validation errors:\n", errorMark())
		for _, verr := range result.Errors {
			fmt.Printf("  - %s\n", verr.Error())
		}
		return fmt.Errorf("validation failed with %d error(s)", len(result.Errors))
	}

	fmt.Printf("%s All checks passed\n", checkMark())

	// Show summary
	fmt.Printf("\nWorkflows found:\n")
	for name, wf := range module.Workflows {
		internal := ""
		if wf.IsInternal() {
			internal = " (internal)"
		}
		fmt.Printf("  - %s: %d steps%s\n", name, len(wf.Steps), internal)
	}

	return nil
}

// checkMark returns a check mark for success messages.
func checkMark() string {
	return "[OK]"
}

// errorMark returns an error mark for error messages.
func errorMark() string {
	return "[ERROR]"
}
