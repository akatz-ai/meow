package cmd

import (
	"github.com/spf13/cobra"
)

var skillCmd = &cobra.Command{
	Use:   "skill",
	Short: "Manage skills for AI harnesses",
	Long: `Manage skills that can be installed into AI harnesses like Claude Code or OpenCode.

Skills are bundles of files (prompts, documentation, helpers) that extend
an AI harness's capabilities. They can be installed globally or per-project.

Supported harnesses:
  - claude:   Claude Code (~/.claude/skills/<name>/)
  - opencode: OpenCode    (~/.config/opencode/skill/<name>/)`,
}

func init() {
	rootCmd.AddCommand(skillCmd)
}
