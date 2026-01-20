# MEOW CLI Commands Reference

## Workflow Management

### meow init

Initialize a MEOW project in the current directory.

```bash
meow init
```

Creates `.meow/` directory structure with config, workflows, lib, and adapters.

### meow run

Run a workflow from a workflow.

```bash
meow run <workflow> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-d, --daemon` | Run in background (daemonized) |
| `--var key=value` | Set workflow variable (repeatable) |
| `--workflow <id>` | Use specific run ID (default: generated) |
| `--dry-run` | Parse and validate without executing |
| `--no-resume` | Start fresh even if workflow exists |

**Examples:**
```bash
# Run in foreground
meow run fix-bug

# Run in background with variables
meow run fix-bug -d --var issue=123 --var branch=hotfix

# Dry run to validate workflow
meow run complex-workflow --dry-run
```

### meow status

Show status of active workflows.

```bash
meow status [run-id] [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-w, --watch` | Live updates (refresh every second) |
| `-a, --all` | Include completed workflows |
| `--json` | Output as JSON |
| `--filter <status>` | Filter by step status |

**Examples:**
```bash
# All active workflows
meow status

# Specific workflow
meow status abc123

# Watch mode
meow status --watch

# JSON for scripting
meow status --json | jq '.workflows[0].steps'
```

### meow stop

Stop a running workflow.

```bash
meow stop <run-id> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--force` | Force stop (kill agents immediately) |
| `--timeout <duration>` | Wait for graceful shutdown (default: 10s) |

Sends SIGTERM to agents, waits for graceful shutdown, then SIGKILL if needed.

### meow resume

Resume a stopped or failed workflow.

```bash
meow resume <run-id> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--from <step-id>` | Resume from specific step |
| `--reset-failed` | Reset failed steps to pending |

### meow ls

List workflows.

```bash
meow ls [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `-a, --all` | Include completed workflows |
| `--stale` | Show only stale workflows (no activity) |
| `--status <status>` | Filter: running, stopped, failed, done |
| `--json` | Output as JSON |
| `--since <duration>` | Only workflows started within duration |

**Examples:**
```bash
# Active workflows
meow ls

# Find stale workflows to clean up
meow ls --stale

# All failed workflows
meow ls --status failed --all
```

### meow agents

List active agent sessions.

```bash
meow agents [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--workflow <id>` | Filter by workflow |
| `--json` | Output as JSON |

Shows tmux sessions, their workflow, and status.

### meow trace

Show execution trace for debugging.

```bash
meow trace <run-id> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--step <id>` | Show only specific step |
| `--follow` | Follow new events |
| `--json` | Output as JSON |

Shows chronological events: step transitions, commands, outputs, errors.

## Agent Commands

These commands are called BY agents running inside a workflow.

### meow done

Signal that the current step is complete.

```bash
meow done [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--output key=value` | Pass output to downstream steps (repeatable) |
| `--output-json <json>` | Pass JSON object as outputs |
| `--notes <text>` | Add notes to step completion |

**Examples:**
```bash
# Simple completion
meow done

# With outputs
meow done --output status=success --output files_changed=3

# With JSON
meow done --output-json '{"results": ["a", "b", "c"]}'
```

**Note:** No-op when `MEOW_ORCH_SOCK` is not set.

### meow event

Emit an event to the orchestrator.

```bash
meow event <type> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--data key=value` | Attach data to event (repeatable) |
| `--data-json <json>` | Attach JSON data |

**Examples:**
```bash
# Simple event
meow event task-blocked

# With context
meow event need-review --data file=main.go --data line=42
```

### meow await-event

Wait for an event of a specific type.

```bash
meow await-event <type> [flags]
```

**Flags:**
| Flag | Description |
|------|-------------|
| `--filter key=value` | Only match events with this data |
| `--timeout <duration>` | Maximum wait time (default: no timeout) |

**Examples:**
```bash
# Wait for approval
meow await-event approved --timeout 1h

# Wait for specific event
meow await-event file-changed --filter path=config.toml
```

Exit codes:
- 0: Event received
- 1: Timeout or error

### meow step-status

Check the status of a step.

```bash
meow step-status <step-id>
```

Returns: `pending`, `running`, `completing`, `done`, `failed`

Useful for conditional logic in shell steps.

## Human Approval

### meow approve

Approve a gate.

```bash
meow approve <run-id> <gate-id>
```

### meow reject

Reject a gate.

```bash
meow reject <run-id> <gate-id> [--reason <text>]
```

**Example workflow with gate:**
```toml
[[main.steps]]
id = "deploy-gate"
executor = "branch"
condition = "meow await-approval deploy-gate"
on_true = "proceed-deploy"
on_false = "cancel-deploy"
```

Then from outside:
```bash
meow approve abc123 deploy-gate
# or
meow reject abc123 deploy-gate --reason "Not ready for prod"
```

## Environment Variables

| Variable | Description |
|----------|-------------|
| `MEOW_CONFIG` | Override config file location |
| `MEOW_DEBUG` | Enable debug logging |
| `MEOW_ORCH_SOCK` | (Set by orchestrator) IPC socket path |
| `MEOW_WORKFLOW` | (Set by orchestrator) Current run ID |
| `MEOW_STEP` | (Set by orchestrator) Current step ID |
| `MEOW_AGENT` | (Set by orchestrator) Current agent name |
