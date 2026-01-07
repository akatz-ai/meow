package cmd

import (
	"fmt"

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
	if err := checkWorkDir(); err != nil {
		return err
	}

	template := args[0]

	// TODO: Load and validate template
	fmt.Printf("Validating template: %s\n", template)
	fmt.Println("(not yet implemented)")

	return nil
}
