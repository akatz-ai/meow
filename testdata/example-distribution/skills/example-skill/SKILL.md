---
name: example-skill
description: Example skill for MEOW workflow orchestration. Use when learning MEOW or testing distribution.
---

# Example MEOW Skill

## Overview

This skill demonstrates how to bundle MEOW workflows with a Claude skill for marketplace distribution. It includes example workflows for task execution with validation and logging.

## Prerequisites

This skill uses MEOW for workflow orchestration.

### Check Installation

```bash
which meow && meow --version
```

### Install MEOW (if needed)

If MEOW is not installed, run:

```bash
curl -fsSL https://raw.githubusercontent.com/akatz-ai/meow/main/install.sh | sh
```

Or with Go:

```bash
go install github.com/akatz-ai/meow/cmd/meow@latest
```

## Workflow Setup (First Time)

This skill includes bundled workflows. Install them to your MEOW directory:

```bash
# Create MEOW directory if needed
mkdir -p ~/.meow/workflows/lib

# Find where this skill is installed
# (Claude: use the path where you found this SKILL.md)
SKILL_DIR="$(dirname "$(realpath "$0")")"

# Copy bundled workflows
cp -r "$SKILL_DIR/workflows/"* ~/.meow/workflows/

# Verify installation
meow ls
```

**Bundled workflows:**
- `workflows/example-task.meow.toml` - Main task workflow
- `workflows/lib/example-helpers.meow.toml` - Helper utilities

## Usage

Run the example workflow to execute a task:

```bash
# Basic usage
meow run example-task --var task="Build a TODO app"

# With priority
meow run example-task --var task="Fix auth bug" --var priority="high"
```

The workflow will:
1. Validate the task input
2. Log the task start with priority
3. Execute the task
4. Log completion

## References

- [Usage Guide](references/usage.md) - Detailed usage examples and patterns
