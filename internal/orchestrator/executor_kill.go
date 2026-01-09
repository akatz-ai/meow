package orchestrator

import (
	"context"
	"fmt"
	"strings"

	"github.com/meow-stack/meow-machine/internal/types"
)

// AgentStopper is the interface for stopping agents.
// This decouples the executor from the concrete agent manager implementation.
type AgentStopper interface {
	// Stop terminates an agent session.
	Stop(ctx context.Context, cfg *AgentStopConfig) error

	// IsRunning checks if an agent is currently active.
	IsRunning(ctx context.Context, agentID string) (bool, error)
}

// AgentStopConfig contains the configuration for stopping an agent.
type AgentStopConfig struct {
	AgentID    string // Agent identifier
	WorkflowID string // Workflow instance ID (to construct tmux session name)
	Graceful   bool   // Whether to attempt graceful shutdown
	Timeout    int    // Seconds to wait for graceful shutdown
}

// KillResult contains the results of killing an agent.
type KillResult struct {
	WasRunning bool // Whether the agent was running before kill
}

// DefaultKillTimeout is the default timeout for graceful shutdown in seconds.
const DefaultKillTimeout = 10

// ExecuteKill terminates an agent's tmux session.
// The stopper parameter allows dependency injection for testing.
func ExecuteKill(ctx context.Context, step *types.Step, workflowID string, stopper AgentStopper) (*KillResult, *types.StepError) {
	if step.Kill == nil {
		return nil, &types.StepError{Message: "kill step missing config"}
	}

	cfg := step.Kill

	// Validate required field
	if cfg.Agent == "" {
		return nil, &types.StepError{Message: "kill step missing agent field"}
	}

	result := &KillResult{}

	// Check if agent is running (optional - for informational purposes)
	wasRunning, err := stopper.IsRunning(ctx, cfg.Agent)
	if err != nil {
		// Log but don't fail - agent status check is informational
		wasRunning = false
	}
	result.WasRunning = wasRunning

	// Build stop config
	// Note: In Go, we can't distinguish "unset" from "explicitly false".
	// The spec says graceful defaults to true, so we pass through the value
	// and let the AgentStopper handle default behavior.
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = DefaultKillTimeout
	}

	stopCfg := &AgentStopConfig{
		AgentID:    cfg.Agent,
		WorkflowID: workflowID,
		Graceful:   cfg.Graceful,
		Timeout:    timeout,
	}

	// Stop the agent
	if err := stopper.Stop(ctx, stopCfg); err != nil {
		// Per bead description: don't fail if agent already dead
		// The stopper implementation should handle this gracefully
		// We only fail on actual errors, not "already stopped"
		if !isAgentAlreadyStoppedError(err) {
			return result, &types.StepError{
				Message: fmt.Sprintf("failed to stop agent %s: %v", cfg.Agent, err),
			}
		}
		// Agent already stopped - not an error
	}

	return result, nil
}

// isAgentAlreadyStoppedError checks if the error indicates the agent was already stopped.
// Per the spec: "Doesn't fail if agent already dead" - we're lenient with stop errors.
func isAgentAlreadyStoppedError(err error) bool {
	if err == nil {
		return false
	}
	// Be lenient - treat most stop errors as "already stopped"
	// Common cases: session not found, process not running, etc.
	// The AgentStopper should return specific error types for truly fatal errors.
	errMsg := err.Error()
	return strings.Contains(errMsg, "not found") ||
		strings.Contains(errMsg, "no such") ||
		strings.Contains(errMsg, "does not exist") ||
		strings.Contains(errMsg, "not running") ||
		strings.Contains(errMsg, "already")
}
