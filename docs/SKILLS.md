# MEOW Skills

Skills are bundles of files that can be installed into AI harnesses like Claude Code or OpenCode. They package MEOW workflows with documentation, making it easy to distribute and share workflow automation.

## What are Skills?

A skill is a directory containing:

- **`skill.toml`** — Metadata and configuration
- **`SKILL.md`** — User-facing documentation for the AI harness
- **`references/`** (optional) — Additional reference documentation
- **`workflows/`** (optional, for marketplace export) — Bundled workflow files

Skills bridge the gap between MEOW workflows and AI harnesses. When you install a skill, the files (excluding `skill.toml`) are copied to the harness's skill directory, where the AI can read them to understand available workflows and how to use them.

## For Users

### Installing Skills

Install a skill to an AI harness using the `meow skill install` command:

```bash
# Install to Claude Code globally
meow skill install ./my-skill --target claude

# Install to OpenCode globally
meow skill install ./my-skill --target opencode

# Install to all supported targets
meow skill install ./my-skill --target all

# Preview what would be installed
meow skill install ./my-skill --target claude --dry-run

# Force overwrite existing skill
meow skill install ./my-skill --target claude --force
```

**Global vs Project Installation:**

By default, skills are installed globally. The harness determines where the skill is read from (global vs project paths).

### Listing Installed Skills

List skills installed in your harnesses:

```bash
# List skills in Claude Code
ls ~/.claude/skills/

# List skills in OpenCode
ls ~/.config/opencode/skill/
```

### Removing Skills

Remove installed skills manually:

```bash
# Remove from Claude Code
rm -rf ~/.claude/skills/my-skill

# Remove from OpenCode
rm -rf ~/.config/opencode/skill/my-skill
```

### Supported Harnesses

MEOW supports the following AI harnesses out of the box:

| Harness | Identifier | Global Path | Project Path |
|---------|------------|-------------|--------------|
| **Claude Code** | `claude` | `~/.claude/skills/<name>/` | `.claude/skills/<name>/` |
| **OpenCode** | `opencode` | `~/.config/opencode/skill/<name>/` | `.opencode/skill/<name>/` |

When you install a skill with `--target claude`, MEOW copies the skill files to `~/.claude/skills/<skill-name>/`. The AI harness can then read these files to understand the skill's capabilities.

## For Authors

### Skill Directory Structure

A typical skill directory looks like this:

```
my-skill/
├── skill.toml          # Manifest (required)
├── SKILL.md            # User documentation (required)
└── references/         # Additional docs (optional)
    ├── examples.md
    └── api-reference.md
```

When exported for marketplace distribution, workflows are bundled:

```
my-skill/
├── skill.toml
├── SKILL.md
├── references/
└── workflows/          # Bundled workflows (added during export)
    ├── main.meow.toml
    └── lib/
        └── helpers.meow.toml
```

### skill.toml Reference

The `skill.toml` file defines your skill's metadata and configuration.

**Minimal example:**

```toml
[skill]
name = "sprint-planner"
description = "Plan and execute sprints with MEOW"

[targets.claude]  # Enable Claude Code support
```

**Complete example with all fields:**

```toml
[skill]
name = "sprint-planner"
description = "Plan and execute sprints with MEOW workflows"
version = "1.2.0"
files = [
  "SKILL.md",
  "references/",
]

# Supported AI harnesses
[targets.claude]
# Empty section = use default path (~/.claude/skills/sprint-planner)

[targets.opencode]
path = "~/.config/custom/sprint-planner"  # Custom installation path

# Export configuration (for marketplace distribution)
[export]
workflows = [
  "workflows/sprint.meow.toml",
  "workflows/lib/agent-persistence.meow.toml",
]
requires = ["meow"]

[export.marketplace]
plugin_name = "sprint-planner-plugin"
version = "1.2.0"
```

**Field descriptions:**

#### `[skill]` Section

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `name` | **Yes** | string | Unique identifier (lowercase, alphanumeric, hyphens) |
| `description` | **Yes** | string | Human-readable description of the skill |
| `version` | No | string | Semantic version (e.g., "1.2.0") |
| `files` | No | string[] | Specific files to include. If empty, all files in the skill directory are included (excluding `skill.toml`) |

#### `[targets.<harness>]` Sections

Define which AI harnesses this skill supports. The harness name (e.g., `claude`, `opencode`) must match a known target.

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `path` | No | string | Custom installation path. If empty, the harness's default path is used. Supports `{{name}}` placeholder and `~` expansion. |

**Example targets:**

```toml
# Use default path for Claude
[targets.claude]

# Use custom path for OpenCode
[targets.opencode]
path = "~/.config/custom-opencode/skills/{{name}}"
```

#### `[export]` Section (optional)

Configuration for exporting to Claude marketplace. Only needed if you plan to distribute via marketplace.

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `workflows` | No | string[] | Workflow files to bundle (relative to repo root). Required for `meow skill export`. |
| `requires` | No | string[] | Dependencies to document in SKILL.md (e.g., `["meow"]`) |

#### `[export.marketplace]` Section (optional)

Marketplace-specific metadata. Overrides defaults from `[skill]`.

| Field | Required | Type | Description |
|-------|----------|------|-------------|
| `plugin_name` | No | string | Plugin name for marketplace. Defaults to skill name. |
| `version` | No | string | Plugin version. Defaults to skill version or "1.0.0". |

### SKILL.md Format

The `SKILL.md` file is what users see when they activate the skill in their AI harness. It should be written in Markdown and follow this structure:

**Frontmatter (required):**

```markdown
---
name: sprint-planner
description: Plan and execute sprints with MEOW workflows
---
```

**Content sections:**

```markdown
# Sprint Planner Skill

## Overview

This skill helps you plan and execute development sprints using MEOW workflows.

## Prerequisites

This skill requires MEOW to be installed. Check if MEOW is available:

\`\`\`bash
which meow && meow --version
\`\`\`

If not installed, see https://github.com/akatz-ai/meow for installation instructions.

## Usage

Run the sprint workflow:

\`\`\`bash
meow run sprint --var sprint_name="Sprint 42"
\`\`\`

## Available Workflows

- **sprint** — Main sprint execution workflow
- **sprint-review** — Review completed work

## Configuration

[Optional: Document any configuration needed]

## References

- [Examples](references/examples.md)
- [API Reference](references/api.md)
```

**For marketplace distribution**, the export command can inject setup instructions. See [DISTRIBUTION.md](DISTRIBUTION.md) for details.

### Best Practices

#### 1. Keep Skills Focused

A skill should represent a cohesive set of related workflows. Don't bundle unrelated workflows into a single skill.

**Good:**
- `sprint-planner` — Sprint planning and execution
- `code-review` — Code review workflows
- `tdd-workflow` — TDD development workflow

**Bad:**
- `everything` — All your workflows in one skill

#### 2. Document Prerequisites

Always document what the user needs to have installed or configured for the skill to work.

#### 3. Include Examples

Provide clear, copy-paste-able examples in your SKILL.md.

#### 4. Version Your Skills

Use semantic versioning in your `skill.toml`. Update the version when you change workflows or documentation.

#### 5. Test Installation

Before distributing, test installing to a clean environment:

```bash
meow skill install ./my-skill --target claude --dry-run
```

#### 6. Use References for Detailed Docs

Keep SKILL.md concise. Put detailed documentation, API references, and examples in the `references/` directory.

## Distribution Channels

Skills can be distributed through two channels:

1. **MEOW Collections** — Users with MEOW installed can install from collections
2. **Claude Marketplace** — Users discover through Claude's plugin system

For details on distribution, see [DISTRIBUTION.md](DISTRIBUTION.md).

## Troubleshooting

### Skill Not Found by Harness

If the AI harness doesn't see your installed skill:

1. **Check the installation path:**
   ```bash
   ls ~/.claude/skills/my-skill  # For Claude
   ls ~/.config/opencode/skill/my-skill  # For OpenCode
   ```

2. **Verify SKILL.md exists:**
   ```bash
   cat ~/.claude/skills/my-skill/SKILL.md
   ```

3. **Restart the harness** — Some harnesses may cache skill lists

### Installation Fails

If `meow skill install` fails:

1. **Check skill.toml is valid:**
   ```bash
   cat my-skill/skill.toml
   ```

2. **Verify target is supported:**
   ```bash
   # skill.toml must have a [targets.<target>] section
   ```

3. **Check file permissions** — Ensure you have write access to the target directory

### Custom Paths Not Working

If you set a custom path in `[targets.<harness>]` and it's not working:

1. **Verify path expansion** — `~` should expand to your home directory
2. **Check `{{name}}` placeholder** — Should be replaced with skill name
3. **Ensure directory exists** — Create parent directories if needed

## See Also

- [DISTRIBUTION.md](DISTRIBUTION.md) — Distribution strategies and marketplace export
- [COLLECTIONS.md](COLLECTIONS.md) — Creating workflow collections
- [ARCHITECTURE.md](ARCHITECTURE.md) — MEOW architecture and design
