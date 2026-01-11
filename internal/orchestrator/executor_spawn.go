package orchestrator

import (
	"context"
	"fmt"

	"github.com/meow-stack/meow-machine/internal/types"
)

// AgentStarter is the interface for starting agents in tmux sessions.
// This decouples the executor from the concrete agent manager implementation.
type AgentStarter interface {
	// Start creates a new agent session with the given configuration.
	Start(ctx context.Context, cfg *AgentStartConfig) error
}

// AgentStartConfig contains the configuration for starting an agent.
type AgentStartConfig struct {
	AgentID       string            // Agent identifier
	WorkflowID    string            // Workflow instance ID
	Workdir       string            // Working directory
	Env           map[string]string // Additional environment variables
	ResumeSession string            // Claude session ID to resume (optional)
	SpawnArgs     string            // Extra CLI args to append to spawn command (optional)
}

// SpawnResult contains the results of spawning an agent.
type SpawnResult struct {
	TmuxSession string // Name of the created tmux session
}

// ExecuteSpawn starts an agent in a tmux session.
// The agentStarter parameter allows dependency injection for testing.
// workflowID is needed to construct the tmux session name.
func ExecuteSpawn(ctx context.Context, step *types.Step, workflowID string, starter AgentStarter) (*SpawnResult, *types.StepError) {
	if step.Spawn == nil {
		return nil, &types.StepError{Message: "spawn step missing config"}
	}

	cfg := step.Spawn

	// Validate required field
	if cfg.Agent == "" {
		return nil, &types.StepError{Message: "spawn step missing agent field"}
	}

	// Build agent start config
	startCfg := &AgentStartConfig{
		AgentID:       cfg.Agent,
		WorkflowID:    workflowID,
		Workdir:       cfg.Workdir,
		Env:           make(map[string]string),
		ResumeSession: cfg.ResumeSession,
		SpawnArgs:     cfg.SpawnArgs,
	}

	// Copy user-provided env vars
	for k, v := range cfg.Env {
		startCfg.Env[k] = v
	}

	// Orchestrator-injected env vars (these take precedence)
	// Per spec: MEOW_* vars are reserved and will override user-provided values
	startCfg.Env["MEOW_AGENT"] = cfg.Agent
	startCfg.Env["MEOW_WORKFLOW"] = workflowID

	// Start the agent
	if err := starter.Start(ctx, startCfg); err != nil {
		return nil, &types.StepError{
			Message: fmt.Sprintf("failed to start agent %s: %v", cfg.Agent, err),
		}
	}

	// Return result with tmux session name
	result := &SpawnResult{
		TmuxSession: fmt.Sprintf("meow-%s-%s", workflowID, cfg.Agent),
	}

	return result, nil
}

// BuildTmuxSessionName constructs the tmux session name for an agent.
// Format: meow-{workflow_id}-{agent_id}
func BuildTmuxSessionName(workflowID, agentID string) string {
	return fmt.Sprintf("meow-%s-%s", workflowID, agentID)
}
