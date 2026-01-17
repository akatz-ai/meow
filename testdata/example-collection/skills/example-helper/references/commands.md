# MEOW Command Reference

This document provides a quick reference for common MEOW commands used in the example workflow.

## Core Commands

### Running Workflows

```bash
# Run a workflow by name
meow run <workflow-name>

# Run with variables
meow run <workflow-name> --var key=value

# View workflow status
meow status <run-id>
```

### Agent Commands

These commands are used within agent workflows:

```bash
# Signal step completion
meow done

# Signal completion with outputs
meow done --output key=value

# Emit an event
meow event <event-type>

# Wait for an event
meow await-event <event-type>
```

## Collection Commands

```bash
# Validate a collection
meow collection validate <path>

# Install a collection
meow collection install <path>

# Install with skills
meow collection install <path> --skill claude
```

## Example Workflow

The example workflow demonstrates:

1. **Shell executor** - Runs a simple shell command (`hello` step)
2. **Spawn executor** - Starts an agent in a tmux session (`spawn-agent` step)
3. **Agent executor** - Agent completes a task and calls `meow done` (`agent-task` step)
4. **Kill executor** - Gracefully stops the agent session (`cleanup` step)
5. **Dependencies** - Each step depends on the previous step completing

## Learn More

For complete documentation, see the main MEOW docs:
- Architecture: `docs/ARCHITECTURE.md`
- Patterns: `docs/PATTERNS.md`
- Collections: `docs/COLLECTIONS.md`
