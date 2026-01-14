# MEOW Stack - LLM Context Config
# Core files for understanding the codebase
# Usage: llmd | clipboard

WHITELIST:

# Documentation
CLAUDE.md
docs/ARCHITECTURE.md
docs/PATTERNS.md

# Core Types (the foundation)
internal/types/step.go
internal/types/workflow.go
internal/types/agent.go
internal/types/adapter.go

# Orchestrator (main execution engine)
internal/orchestrator/orchestrator.go
internal/orchestrator/expander.go
internal/orchestrator/event_router.go
internal/orchestrator/agent_manager.go

# Executors (the 7 executors)
internal/orchestrator/executor_shell.go
internal/orchestrator/executor_spawn.go
internal/orchestrator/executor_kill.go
internal/orchestrator/executor_agent.go
internal/orchestrator/executor_branch.go
internal/orchestrator/executor_expand.go
internal/orchestrator/executor_foreach.go

# Template System
internal/template/parser.go
internal/template/loader.go
internal/template/vars.go
internal/template/validate.go
internal/template/baker.go
internal/template/module.go

# IPC (agent communication)
internal/ipc/messages.go
internal/ipc/server.go
internal/ipc/client.go

# Agent Management
internal/agent/store.go
internal/agent/tmux_wrapper.go

# Adapter System
internal/adapter/registry.go
internal/adapter/loader.go

# Config
internal/config/config.go

# Key Template Examples
.meow/workflows/lib/agent-persistence.meow.toml
.meow/workflows/lib/agent-track.meow.toml
.meow/workflows/sprint.meow.toml

EXCLUDE:
**/*_test.go
**/testutil/**

OPTIONS:
respect_gitignore: true
include_hidden: true
include_binary: false
