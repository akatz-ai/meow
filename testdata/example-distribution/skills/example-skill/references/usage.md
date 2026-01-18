# Example Skill Usage Guide

This guide provides detailed examples for using the example-skill workflows.

## Quick Start

After installing the skill and MEOW, you can run the example workflow:

```bash
meow run example-task --var task="Your task description"
```

## Variables

The `example-task` workflow accepts the following variables:

### Required Variables

- `task` - Task description to execute (string)

### Optional Variables

- `priority` - Task priority level (default: "medium")
  - Valid values: "low", "medium", "high"

## Examples

### Basic Task Execution

```bash
meow run example-task --var task="Implement user authentication"
```

Output:
```
[medium] Starting task: Implement user authentication
Executing: Implement user authentication
Task execution completed
[COMPLETE] Task completed: Implement user authentication
```

### High Priority Task

```bash
meow run example-task --var task="Fix security vulnerability" --var priority="high"
```

Output:
```
[high] Starting task: Fix security vulnerability
Executing: Fix security vulnerability
Task execution completed
[COMPLETE] Task completed: Fix security vulnerability
```

### Low Priority Task

```bash
meow run example-task --var task="Update documentation" --var priority="low"
```

## Workflow Structure

The `example-task` workflow demonstrates several key MEOW patterns:

1. **Input Validation** - Inline shell-based validation
2. **Sequential Steps** - Demonstrates step dependencies with `needs`
3. **Variable Substitution** - Uses `{{variable}}` syntax in commands
4. **Shell Executor** - Shows how to use shell commands in workflow steps

## Helper Workflows

The skill includes a `lib/example-helpers.meow.toml` workflow that demonstrates reusable validation patterns. This can be used as a template for creating your own helper workflows.

### example-helpers

Validates that required inputs are not empty.

```toml
[meta]
name = "example-helpers"
description = "Helper workflow for input validation"

[variables]
input = { description = "Input value to validate" }
type = { description = "Type of validation to perform" }

[[steps]]
id = "check-empty"
executor = "shell"
command = """if [ -z "{{input}}" ]; then
  echo "Error: {{type}} input cannot be empty" >&2
  exit 1
fi
echo "Input validation passed"
"""
```

## Integration with Claude

When using this skill with Claude Code, you can:

1. Ask Claude to run workflows for you
2. Claude can interpret task descriptions and set appropriate variables
3. Claude can monitor workflow execution and handle errors

Example conversation:
```
User: Can you run a high priority task to fix the login bug?
Claude: I'll run the example workflow with high priority:
  meow run example-task --var task="Fix login bug" --var priority="high"
```

## Troubleshooting

### Workflow Not Found

If you get "workflow not found" errors:

```bash
# Verify workflows are installed
meow ls

# If missing, reinstall from skill directory
SKILL_DIR="path/to/skill"
cp -r "$SKILL_DIR/workflows/"* ~/.meow/workflows/
```

### Validation Errors

If validation fails:
- Ensure the `task` variable is not empty
- Check that variable names are spelled correctly
- Verify quote escaping in shell commands

### MEOW Not Found

If `meow` command is not found:

```bash
# Check installation
which meow

# Install if missing
go install github.com/akatz-ai/meow/cmd/meow@latest
```

## Advanced Usage

### Modifying Workflows

You can customize the workflows after installation:

```bash
# Edit the main workflow
$EDITOR ~/.meow/workflows/example-task.meow.toml

# Edit helpers
$EDITOR ~/.meow/workflows/lib/example-helpers.meow.toml
```

### Creating Derived Workflows

Use the example workflows as templates for your own:

```bash
cp ~/.meow/workflows/example-task.meow.toml ~/.meow/workflows/my-task.meow.toml
$EDITOR ~/.meow/workflows/my-task.meow.toml
```

## Next Steps

- Explore other MEOW executors: `agent`, `branch`, `foreach`
- Create your own library workflows
- Integrate workflows with CI/CD pipelines
- Build skills for your team

For more information, see the [MEOW documentation](https://github.com/akatz-ai/meow).
