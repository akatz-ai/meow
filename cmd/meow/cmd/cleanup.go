package cmd

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/meow-stack/meow-machine/internal/agent"
	"github.com/meow-stack/meow-machine/internal/cli"
	"github.com/meow-stack/meow-machine/internal/orchestrator"
	"github.com/meow-stack/meow-machine/internal/types"
	"github.com/spf13/cobra"
)

// Cleanup command flags
var (
	cleanupDryRun bool
	cleanupYes    bool
	cleanupList   bool
)

// CleanupTimeout is the maximum time allowed for cleanup script execution.
const CleanupTimeout = 60 * time.Second

// Maximum number of workflows to show in interactive selection
const maxCleanupOptions = 15

var cleanupCmd = &cobra.Command{
	Use:   "cleanup [workflow-id]",
	Short: "Clean up resources from a workflow run",
	Long: `Clean up tmux sessions and other resources from a workflow run.

When run without arguments, shows a list of recent stopped/failed workflows
that may need cleanup and lets you select one interactively.

By default shows a preview and asks for confirmation.
Use --dry-run to just show what would be cleaned.
Use -y to skip confirmation.

Examples:
  meow cleanup                      # Interactive: select from recent runs
  meow cleanup --list               # Just list cleanable workflows
  meow cleanup run-123456789        # Clean specific workflow
  meow cleanup run-123456789 -y     # Skip confirmation
  meow cleanup run-123456789 --dry-run  # Preview only`,
	Args: cobra.MaximumNArgs(1),
	RunE: runCleanup,
}

func init() {
	rootCmd.AddCommand(cleanupCmd)

	cleanupCmd.Flags().BoolVar(&cleanupDryRun, "dry-run", false, "Show what would be cleaned without doing it")
	cleanupCmd.Flags().BoolVarP(&cleanupYes, "yes", "y", false, "Skip confirmation prompt")
	cleanupCmd.Flags().BoolVarP(&cleanupList, "list", "l", false, "List cleanable workflows without prompting")
}

// cleanupResources holds discovered resources to clean up.
type cleanupResources struct {
	// Tmux sessions to kill
	Sessions []string
	// Sessions found in run state
	SessionsFromState []string
	// Sessions found via tmux query (may include orphans)
	SessionsFromTmux []string
	// Cleanup script to run (if any)
	CleanupScript string
	// Which trigger the script is from
	CleanupTrigger string
}

func runCleanup(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	dir, err := getWorkDir()
	if err != nil {
		return err
	}

	runsDir := filepath.Join(dir, ".meow", "runs")
	store, err := orchestrator.NewYAMLRunStore(runsDir)
	if err != nil {
		return fmt.Errorf("opening workflow store: %w", err)
	}
	defer store.Close()

	// Determine workflow ID - either from args or interactive selection
	var workflowID string
	if len(args) > 0 {
		workflowID = args[0]
	} else {
		// Interactive mode: show list of cleanable workflows
		selected, err := selectCleanableWorkflow(ctx, store)
		if err != nil {
			return err
		}
		if selected == "" {
			// User cancelled
			return nil
		}
		workflowID = selected
		fmt.Println() // blank line after selection
	}

	// Load workflow state
	wf, err := store.Get(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("workflow not found: %s", workflowID)
	}

	// Check if workflow is still running
	if !wf.Status.IsTerminal() {
		return fmt.Errorf("workflow %s is still %s (use 'meow stop' first)", workflowID, wf.Status)
	}

	// Discover resources to clean up
	resources, err := discoverResources(ctx, wf, dir)
	if err != nil {
		return fmt.Errorf("discovering resources: %w", err)
	}

	// Print preview
	printCleanupPreview(wf, resources)

	// Check if there's anything to do
	if len(resources.Sessions) == 0 && resources.CleanupScript == "" {
		fmt.Println("\nNothing to clean up.")
		return nil
	}

	// Dry run - don't execute
	if cleanupDryRun {
		fmt.Println("\n--dry-run specified, not executing cleanup.")
		return nil
	}

	// Confirm unless -y
	if !cleanupYes {
		fmt.Print("\nProceed with cleanup? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Cleanup cancelled.")
			return nil
		}
	}

	// Execute cleanup
	fmt.Println()
	if err := executeCleanup(ctx, wf, resources, dir); err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}

	fmt.Printf("\n✓ Cleanup complete for %s\n", workflowID)
	return nil
}

// selectCleanableWorkflow shows a list of workflows that may need cleanup.
// Returns the selected workflow ID, or empty string if cancelled/none available.
func selectCleanableWorkflow(ctx context.Context, store *orchestrator.YAMLRunStore) (string, error) {
	// Get all workflows
	all, err := store.List(ctx, orchestrator.RunFilter{})
	if err != nil {
		return "", fmt.Errorf("listing workflows: %w", err)
	}

	// Filter to terminal (cleanable) workflows, prioritizing failed/stopped
	var cleanable []*types.Run
	for _, wf := range all {
		if wf.Status.IsTerminal() {
			cleanable = append(cleanable, wf)
		}
	}

	if len(cleanable) == 0 {
		fmt.Println("No workflows available for cleanup.")
		fmt.Println("(Only stopped, failed, or done workflows can be cleaned up)")
		return "", nil
	}

	// Sort by end time (most recent first), then by status priority
	sort.Slice(cleanable, func(i, j int) bool {
		// Priority: failed > stopped > done
		statusPriority := func(s types.RunStatus) int {
			switch s {
			case types.RunStatusFailed:
				return 0
			case types.RunStatusStopped:
				return 1
			default:
				return 2
			}
		}

		pi, pj := statusPriority(cleanable[i].Status), statusPriority(cleanable[j].Status)
		if pi != pj {
			return pi < pj
		}

		// Then by end time (most recent first)
		ti := cleanable[i].StartedAt
		if cleanable[i].DoneAt != nil {
			ti = *cleanable[i].DoneAt
		}
		tj := cleanable[j].StartedAt
		if cleanable[j].DoneAt != nil {
			tj = *cleanable[j].DoneAt
		}
		return ti.After(tj)
	})

	// Limit to max options
	if len(cleanable) > maxCleanupOptions {
		cleanable = cleanable[:maxCleanupOptions]
	}

	// If --list flag, just print and exit
	if cleanupList {
		fmt.Println("Workflows available for cleanup:")
		fmt.Println()
		for _, wf := range cleanable {
			printWorkflowSummaryLine(wf)
		}
		return "", nil
	}

	// Build options for interactive selection
	options := make([]cli.SelectOption, len(cleanable))
	for i, wf := range cleanable {
		options[i] = cli.SelectOption{
			Value: wf.ID,
			Label: formatWorkflowOption(wf),
		}
	}

	return cli.Select("Select a workflow to clean up:", options)
}

// formatWorkflowOption formats a workflow for the selection list.
func formatWorkflowOption(wf *types.Run) string {
	// Extract template name from path
	templateName := filepath.Base(wf.Template)
	templateName = strings.TrimSuffix(templateName, ".meow.toml")
	templateName = strings.TrimSuffix(templateName, ".toml")

	// Format time
	timeStr := wf.StartedAt.Format("Jan 02 15:04")
	if wf.DoneAt != nil {
		timeStr = wf.DoneAt.Format("Jan 02 15:04")
	}

	// Status indicator
	statusIcon := ""
	switch wf.Status {
	case types.RunStatusFailed:
		statusIcon = "[FAILED]"
	case types.RunStatusStopped:
		statusIcon = "[STOPPED]"
	case types.RunStatusDone:
		statusIcon = "[done]"
	}

	return fmt.Sprintf("%s %s - %s %s", statusIcon, timeStr, templateName, wf.ID)
}

// printWorkflowSummaryLine prints a single-line summary for --list mode.
func printWorkflowSummaryLine(wf *types.Run) {
	templateName := filepath.Base(wf.Template)
	templateName = strings.TrimSuffix(templateName, ".meow.toml")
	templateName = strings.TrimSuffix(templateName, ".toml")

	timeStr := wf.StartedAt.Format("2006-01-02 15:04:05")
	if wf.DoneAt != nil {
		timeStr = wf.DoneAt.Format("2006-01-02 15:04:05")
	}

	fmt.Printf("  %-8s  %s  %-20s  %s\n", wf.Status, timeStr, templateName, wf.ID)
}

// discoverResources finds all resources that need cleanup for a workflow.
func discoverResources(ctx context.Context, wf *types.Run, workdir string) (*cleanupResources, error) {
	resources := &cleanupResources{}

	// 1. Get sessions from run state
	for _, agentInfo := range wf.Agents {
		if agentInfo.TmuxSession != "" {
			resources.SessionsFromState = append(resources.SessionsFromState, agentInfo.TmuxSession)
		}
	}
	sort.Strings(resources.SessionsFromState)

	// 2. Query tmux for sessions matching this workflow
	tmux := agent.NewTmuxWrapper()
	prefix := fmt.Sprintf("meow-%s-", wf.ID)
	tmuxSessions, err := tmux.ListSessions(ctx, prefix)
	if err != nil {
		// Non-fatal - tmux might not be running
		tmuxSessions = nil
	}
	resources.SessionsFromTmux = tmuxSessions
	sort.Strings(resources.SessionsFromTmux)

	// 3. Union of both sources (deduplicated)
	sessionSet := make(map[string]bool)
	for _, s := range resources.SessionsFromState {
		sessionSet[s] = true
	}
	for _, s := range resources.SessionsFromTmux {
		sessionSet[s] = true
	}
	for s := range sessionSet {
		resources.Sessions = append(resources.Sessions, s)
	}
	sort.Strings(resources.Sessions)

	// 4. Determine cleanup script based on workflow status
	// Use the script that matches the actual final status
	switch wf.Status {
	case types.RunStatusDone:
		if wf.CleanupOnSuccess != "" {
			resources.CleanupScript = wf.CleanupOnSuccess
			resources.CleanupTrigger = "cleanup_on_success"
		}
	case types.RunStatusFailed:
		if wf.CleanupOnFailure != "" {
			resources.CleanupScript = wf.CleanupOnFailure
			resources.CleanupTrigger = "cleanup_on_failure"
		}
	case types.RunStatusStopped:
		if wf.CleanupOnStop != "" {
			resources.CleanupScript = wf.CleanupOnStop
			resources.CleanupTrigger = "cleanup_on_stop"
		}
	}

	return resources, nil
}

// printCleanupPreview displays what will be cleaned up.
func printCleanupPreview(wf *types.Run, resources *cleanupResources) {
	fmt.Printf("Workflow: %s (%s)\n", wf.Template, wf.ID)
	fmt.Printf("Status: %s\n", wf.Status)
	fmt.Printf("Started: %s\n", wf.StartedAt.Format("2006-01-02 15:04:05"))
	if wf.DoneAt != nil {
		fmt.Printf("Ended: %s\n", wf.DoneAt.Format("2006-01-02 15:04:05"))
	}

	fmt.Println("\nResources to clean up:")

	// Tmux sessions
	if len(resources.Sessions) > 0 {
		fmt.Printf("\n  tmux sessions (%d):\n", len(resources.Sessions))
		for _, session := range resources.Sessions {
			// Mark orphans (found via tmux but not in state)
			inState := contains(resources.SessionsFromState, session)
			inTmux := contains(resources.SessionsFromTmux, session)

			marker := ""
			if inTmux && !inState {
				marker = " (orphan - not in run state)"
			} else if inState && !inTmux {
				marker = " (in state but session not found)"
			}
			fmt.Printf("    - %s%s\n", session, marker)
		}
	} else {
		fmt.Println("\n  tmux sessions: none found")
	}

	// Cleanup script
	fmt.Println()
	if resources.CleanupScript != "" {
		fmt.Printf("  cleanup script: %s (will run)\n", resources.CleanupTrigger)
	} else {
		fmt.Printf("  cleanup script: not defined for '%s' status\n", wf.Status)
		if wf.Status == types.RunStatusFailed || wf.Status == types.RunStatusStopped {
			fmt.Println("\n  ⚠️  No cleanup script for this status. Only tmux sessions will be killed.")
			fmt.Println("     You may need to manually clean up worktrees in .meow/worktrees/")
		}
	}
}

// executeCleanup performs the actual cleanup.
func executeCleanup(ctx context.Context, wf *types.Run, resources *cleanupResources, workdir string) error {
	tmux := agent.NewTmuxWrapper()

	// 1. Kill tmux sessions
	for _, session := range resources.Sessions {
		fmt.Printf("Killing tmux session: %s... ", session)

		// Send C-c first for graceful shutdown
		_ = tmux.SendKeysSpecial(ctx, session, "C-c")
		time.Sleep(200 * time.Millisecond)

		// Kill the session
		if err := tmux.KillSession(ctx, session); err != nil {
			fmt.Printf("warning: %v\n", err)
		} else {
			fmt.Println("done")
		}
	}

	// 2. Run cleanup script if defined
	if resources.CleanupScript != "" {
		fmt.Printf("\nRunning %s...\n", resources.CleanupTrigger)
		if err := runCleanupScript(ctx, wf, resources.CleanupScript, workdir); err != nil {
			return fmt.Errorf("cleanup script failed: %w", err)
		}
	}

	return nil
}

// runCleanupScript executes a cleanup script with timeout.
func runCleanupScript(ctx context.Context, wf *types.Run, script string, workdir string) error {
	if script == "" {
		return nil
	}

	// Create a timeout context for cleanup
	cleanupCtx, cancel := context.WithTimeout(ctx, CleanupTimeout)
	defer cancel()

	// Execute the cleanup script via bash
	cmd := exec.CommandContext(cleanupCtx, "bash", "-c", script)
	cmd.Dir = workdir

	// Set environment variables from workflow
	cmd.Env = os.Environ()
	for k, v := range wf.Variables {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	// Add workflow ID as an environment variable
	cmd.Env = append(cmd.Env, fmt.Sprintf("MEOW_WORKFLOW=%s", wf.ID))
	// Mark this as a manual cleanup run
	cmd.Env = append(cmd.Env, "MEOW_CLEANUP=manual")

	// Capture output for display
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if cleanupCtx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("cleanup script timed out after %s", CleanupTimeout)
		}
		return fmt.Errorf("%w\nstderr: %s", err, stderr.String())
	}

	// Print script output
	if stdout.Len() > 0 {
		fmt.Print(stdout.String())
	}

	return nil
}

// contains checks if a string slice contains a value.
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
