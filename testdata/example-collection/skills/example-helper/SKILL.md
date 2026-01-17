---
name: example-helper
description: Example skill for testing MEOW skill bundling
---

# Example Helper

This skill helps Claude/OpenCode understand how to use the example MEOW workflow collection.

## When to Use

Use this skill when working with the example MEOW collection. It provides context and guidance for:

- Understanding basic MEOW workflow structure
- Learning how skills integrate with workflows
- Testing skill installation and bundling

## Overview

This is a minimal example skill that demonstrates:

1. **Skill bundling** - How skills are packaged with collections
2. **Multi-harness support** - Works with both Claude Code and OpenCode
3. **Reference files** - Includes additional documentation in `references/`

## MEOW Commands

The example workflow uses standard MEOW commands. See `@references/commands.md` for details.

## Example Usage

When working with the example collection:

1. Install the collection with skills:
   ```bash
   meow collection install /path/to/example-collection --skill claude
   ```

2. Run the example workflow:
   ```bash
   meow run example
   ```

3. The workflow demonstrates:
   - Shell executor (`hello` step)
   - Agent executor (`agent-task` step)
   - Basic step dependencies

## Files in This Skill

- `SKILL.md` (this file) - Main skill documentation
- `references/commands.md` - MEOW command reference
- `skill.toml` - Skill manifest

## Integration with Workflows

This skill is referenced by workflows in the `example-collection`. When installed, it provides context to AI assistants about how to work with MEOW workflows.
