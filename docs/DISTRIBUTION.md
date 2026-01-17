# MEOW Distribution Guide

This guide covers how to distribute MEOW workflows through two complementary channels: **MEOW Collections** and the **Claude Marketplace**.

## Overview

MEOW workflows can reach users through two distribution channels:

| Channel | Best For | Entry Point | Prerequisites |
|---------|----------|-------------|---------------|
| **MEOW Collection** | Users who already have MEOW | `meow collection install` | MEOW installed |
| **Claude Marketplace** | Claude users discovering MEOW | `/plugin install` in Claude | None (self-installing) |

**Key insight:** You maintain a **single source of truth** (your workflow repository) and use MEOW's tooling to package for each channel.

## The Two Channels

### Channel 1: MEOW Collection

**When to use:**
- Distributing to MEOW users
- Workflow-first distribution
- No AI harness integration needed

**How it works:**

1. Users install your collection:
   ```bash
   meow collection install <github-user>/<repo>
   ```

2. MEOW copies workflows to `~/.meow/workflows/`

3. Users run workflows directly:
   ```bash
   meow run your-workflow
   ```

**Pros:**
- Simple setup (just a `meow-collection.toml`)
- Direct workflow installation
- No extra packaging steps

**Cons:**
- Requires MEOW pre-installed
- No AI harness integration

**See also:** [COLLECTIONS.md](COLLECTIONS.md) for collection manifest details.

### Channel 2: Claude Marketplace

**When to use:**
- Reaching Claude Code users
- Providing in-editor documentation
- Self-installing workflow packages

**How it works:**

1. Users discover via Claude marketplace:
   ```
   /plugin marketplace add <user>/<repo>
   /plugin install <skill-name>
   ```

2. Claude copies skill to `~/.claude/skills/<skill-name>/`

3. Skill includes setup instructions that guide Claude through:
   - Checking for MEOW
   - Installing MEOW if needed
   - Copying bundled workflows to `~/.meow/workflows/`

4. User runs workflows via MEOW:
   ```bash
   meow run workflow-from-skill
   ```

**Pros:**
- Reaches Claude users who don't have MEOW yet
- Self-installing (guides Claude through setup)
- Integrated documentation in Claude

**Cons:**
- Requires export step (`meow skill export`)
- Separate repository for marketplace distribution
- More complex setup

## Repository Structure

The recommended structure supports **both channels** from a single source:

```
my-workflow-pack/
├── meow-collection.toml       # Channel 1: Collection manifest
├── skills/                    # Channel 2: Skill definitions
│   └── my-skill/
│       ├── skill.toml         # Skill manifest
│       ├── SKILL.md           # User documentation
│       └── references/        # Additional docs
└── workflows/                 # Single source of truth
    ├── main.meow.toml
    ├── secondary.meow.toml
    └── lib/
        └── helpers.meow.toml
```

**Distribution outputs:**

- **Channel 1:** Users get workflows directly from `workflows/` via collection install
- **Channel 2:** You export to `dist/` using `meow skill export`, which bundles workflows into the skill

## Setting Up a Distribution Repository

Follow these steps to create a repository that supports both distribution channels.

### Step 1: Initialize Repository

```bash
mkdir my-workflow-pack
cd my-workflow-pack
git init
```

### Step 2: Create Workflows

Create your workflows in `workflows/`:

```bash
mkdir -p workflows/lib
```

**Example workflow** (`workflows/sprint.meow.toml`):

```toml
[main]
name = "sprint"
description = "Execute a sprint workflow"

[main.variables]
sprint_name = { required = true }

[[main.steps]]
id = "start"
executor = "spawn"
agent = "claude-1"

[[main.steps]]
id = "plan"
executor = "agent"
agent = "claude-1"
prompt = "Plan sprint: {{sprint_name}}"
needs = ["start"]
```

### Step 3: Create Collection Manifest (Channel 1)

Create `meow-collection.toml` at the repository root:

```toml
[collection]
name = "my-workflows"
description = "My MEOW workflow collection"
version = "1.0.0"

[collection.owner]
name = "Your Name"
url = "https://github.com/your-username"

[collection.repository]
url = "https://github.com/your-username/my-workflow-pack"
license = "MIT"

[[packs]]
name = "core"
description = "Core workflows"
workflows = [
  "workflows/sprint.meow.toml",
  "workflows/lib/helpers.meow.toml",
]
```

**Test Channel 1:**

```bash
# From repo root
meow collection install .

# Verify
meow ls | grep sprint

# Run
meow run sprint --var sprint_name="Test Sprint"
```

### Step 4: Create Skill Definition (Channel 2)

Create the skill directory structure:

```bash
mkdir -p skills/my-skill/references
```

**Create `skills/my-skill/skill.toml`:**

```toml
[skill]
name = "my-skill"
description = "My MEOW workflows for Claude"
version = "1.0.0"

[targets.claude]
# Use default path: ~/.claude/skills/my-skill

[export]
workflows = [
  "workflows/sprint.meow.toml",
  "workflows/lib/helpers.meow.toml",
]
requires = ["meow"]
```

**Create `skills/my-skill/SKILL.md`:**

```markdown
---
name: my-skill
description: My MEOW workflows for Claude
---

# My Skill

## Overview

This skill provides MEOW workflows for sprint planning and execution.

## Prerequisites

This skill uses MEOW for workflow orchestration.

### Check Installation

\`\`\`bash
which meow && meow --version
\`\`\`

### Install MEOW (if needed)

If MEOW is not installed, run:

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/akatz-ai/meow/main/install.sh | sh
\`\`\`

Or with Go:
\`\`\`bash
go install github.com/akatz-ai/meow/cmd/meow@latest
\`\`\`

## Workflow Setup (First Time)

This skill includes bundled workflows. Install them to your MEOW directory:

\`\`\`bash
# Find where this skill is installed (Claude knows this path)
SKILL_DIR="$HOME/.claude/skills/my-skill"

# Copy bundled workflows
mkdir -p ~/.meow/workflows
cp -r "$SKILL_DIR/workflows/"* ~/.meow/workflows/

# Verify installation
meow ls
\`\`\`

**Bundled workflows:**
- `sprint.meow.toml` — Sprint execution
- `lib/helpers.meow.toml` — Helper workflows

## Usage

### Run Sprint

\`\`\`bash
meow run sprint --var sprint_name="Sprint 42"
\`\`\`

## Troubleshooting

If workflows don't appear after installation, verify the copy step completed successfully:

\`\`\`bash
ls -la ~/.meow/workflows/
\`\`\`
```

### Step 5: Export for Marketplace (Channel 2)

Export the skill for marketplace distribution:

```bash
meow skill export my-skill --for-marketplace --output dist/
```

**Output structure:**

```
dist/
├── .claude-plugin/
│   └── marketplace.json
└── plugins/
    └── my-skill/
        ├── plugin.json
        └── skills/
            └── my-skill/
                ├── SKILL.md
                ├── references/
                └── workflows/           # Copied from repo
                    ├── sprint.meow.toml
                    └── lib/
                        └── helpers.meow.toml
```

**Test Channel 2 locally:**

```bash
# Install skill to Claude
meow skill install dist/plugins/my-skill/skills/my-skill --target claude

# Verify
ls ~/.claude/skills/my-skill

# Follow SKILL.md setup instructions
cat ~/.claude/skills/my-skill/SKILL.md
```

### Step 6: Publish to GitHub

**For Channel 1** (collection):

```bash
# Push main repository
git add meow-collection.toml workflows/ skills/
git commit -m "Initial release"
git push origin main
```

Users install with:
```bash
meow collection install your-username/my-workflow-pack
```

**For Channel 2** (marketplace):

```bash
# Create separate repository for marketplace
cd dist/
git init
git add .
git commit -m "Initial marketplace release"
git remote add origin https://github.com/your-username/my-skill-marketplace
git push -u origin main
```

Users install with:
```
/plugin marketplace add your-username/my-skill-marketplace
/plugin install my-skill
```

## The Export Command

The `meow skill export` command packages a skill with bundled workflows for marketplace distribution.

**Usage:**

```bash
meow skill export <skill-name> --for-marketplace --output <dir>
```

**Flags:**

| Flag | Required | Description |
|------|----------|-------------|
| `--for-marketplace` | Yes | Export for Claude marketplace |
| `--output <dir>` | No | Output directory (defaults to `./dist`) |
| `--repo <path>` | No | Path to collection repository (defaults to current directory) |
| `--dry-run` | No | Preview what would be exported without writing files |

**Example:**

```bash
# Export from current directory
meow skill export sprint-planner --for-marketplace --output dist/

# Export from specific repo
meow skill export sprint-planner --repo /path/to/repo --output /tmp/export/

# Preview export
meow skill export sprint-planner --for-marketplace --dry-run
```

**What gets exported:**

1. **Skill files** (from `skills/<skill-name>/`):
   - `SKILL.md` — User documentation
   - `references/` — Additional documentation
   - Any other files (excluding `skill.toml`)

2. **Bundled workflows** (from `export.workflows` in `skill.toml`):
   - Copied to `skills/<skill-name>/workflows/`
   - Directory structure preserved

3. **Generated files**:
   - `.claude-plugin/marketplace.json` — Marketplace metadata
   - `plugins/<skill-name>/plugin.json` — Plugin metadata

**Validation:**

The export command validates:
- `skill.toml` has an `[export]` section
- `export.workflows` is not empty
- All workflow paths exist in the repository

## Self-Installing Skills

Self-installing skills guide the AI through the MEOW installation and workflow setup process. This enables users to install workflows without prior MEOW knowledge.

### SKILL.md Template Pattern

Your `SKILL.md` should include these sections for self-installation:

**1. Prerequisites Section**

```markdown
## Prerequisites

This skill uses MEOW for workflow orchestration.

### Check Installation

\`\`\`bash
which meow && meow --version
\`\`\`

### Install MEOW (if needed)

If MEOW is not installed, run:

\`\`\`bash
curl -fsSL https://raw.githubusercontent.com/akatz-ai/meow/main/install.sh | sh
\`\`\`
```

**2. Workflow Setup Section**

```markdown
## Workflow Setup (First Time)

This skill includes bundled workflows. Install them to your MEOW directory:

\`\`\`bash
# Find where this skill is installed
SKILL_DIR="$HOME/.claude/skills/{{skill-name}}"

# Copy bundled workflows
mkdir -p ~/.meow/workflows
cp -r "$SKILL_DIR/workflows/"* ~/.meow/workflows/

# Verify installation
meow ls
\`\`\`

**Bundled workflows:**
- `workflow1.meow.toml` — Description
- `workflow2.meow.toml` — Description
```

**3. Usage Section**

```markdown
## Usage

Run the main workflow:

\`\`\`bash
meow run main-workflow --var key="value"
\`\`\`
```

### Idempotent Installation

Make setup steps idempotent so they can be run multiple times safely:

```bash
# Good: Creates directory if it doesn't exist
mkdir -p ~/.meow/workflows

# Good: Overwrites safely
cp -r "$SKILL_DIR/workflows/"* ~/.meow/workflows/

# Bad: Fails if directory exists
mkdir ~/.meow/workflows
```

## Complete Example: Step-by-Step

Let's create a complete distribution repository from scratch.

### 1. Create Repository

```bash
mkdir sprint-planner-pack
cd sprint-planner-pack
git init
```

### 2. Add Workflows

```bash
mkdir -p workflows/lib
```

**`workflows/sprint.meow.toml`:**

```toml
[main]
name = "sprint"
description = "Execute a development sprint"

[main.variables]
sprint_name = { required = true }
duration_days = { default = "14" }

[[main.steps]]
id = "init"
executor = "shell"
command = "echo Starting sprint: {{sprint_name}}"

[[main.steps]]
id = "plan"
executor = "spawn"
agent = "claude-1"
needs = ["init"]

[[main.steps]]
id = "execute"
executor = "agent"
agent = "claude-1"
prompt = "Execute {{sprint_name}} over {{duration_days}} days"
needs = ["plan"]
```

### 3. Add Collection Manifest

**`meow-collection.toml`:**

```toml
[collection]
name = "sprint-planner"
description = "Sprint planning and execution workflows"
version = "1.0.0"

[collection.owner]
name = "Example Author"
url = "https://github.com/example"

[collection.repository]
url = "https://github.com/example/sprint-planner-pack"
license = "MIT"

[[packs]]
name = "sprint"
description = "Sprint workflows"
workflows = ["workflows/sprint.meow.toml"]
```

### 4. Add Skill Definition

```bash
mkdir -p skills/sprint-planner/references
```

**`skills/sprint-planner/skill.toml`:**

```toml
[skill]
name = "sprint-planner"
description = "Sprint planning workflows for MEOW"
version = "1.0.0"

[targets.claude]

[export]
workflows = ["workflows/sprint.meow.toml"]
requires = ["meow"]
```

**`skills/sprint-planner/SKILL.md`:**

```markdown
---
name: sprint-planner
description: Sprint planning workflows for MEOW
---

# Sprint Planner

## Overview

Orchestrate development sprints with MEOW workflows.

## Prerequisites

Requires MEOW: https://github.com/akatz-ai/meow

Check installation:
\`\`\`bash
which meow && meow --version
\`\`\`

## Workflow Setup

\`\`\`bash
SKILL_DIR="$HOME/.claude/skills/sprint-planner"
mkdir -p ~/.meow/workflows
cp -r "$SKILL_DIR/workflows/"* ~/.meow/workflows/
meow ls
\`\`\`

## Usage

\`\`\`bash
meow run sprint --var sprint_name="Sprint 1" --var duration_days="14"
\`\`\`
```

### 5. Test Channel 1 (Collection)

```bash
# Install collection
meow collection install .

# Verify
meow ls | grep sprint

# Run
meow run sprint --var sprint_name="Test"
```

### 6. Export for Channel 2 (Marketplace)

```bash
# Export
meow skill export sprint-planner --for-marketplace --output dist/

# Verify output
tree dist/
```

Expected output:
```
dist/
├── .claude-plugin/
│   └── marketplace.json
└── plugins/
    └── sprint-planner/
        ├── plugin.json
        └── skills/
            └── sprint-planner/
                ├── SKILL.md
                └── workflows/
                    └── sprint.meow.toml
```

### 7. Test Channel 2 Locally

```bash
# Install skill
meow skill install dist/plugins/sprint-planner/skills/sprint-planner --target claude

# Verify
cat ~/.claude/skills/sprint-planner/SKILL.md

# Follow setup instructions in SKILL.md
cp -r ~/.claude/skills/sprint-planner/workflows/* ~/.meow/workflows/

# Run workflow
meow run sprint --var sprint_name="Test"
```

### 8. Publish

**Push main repository:**

```bash
git add .
git commit -m "v1.0.0: Initial release"
git tag v1.0.0
git push origin main --tags
```

**Publish marketplace package:**

```bash
cd dist/
git init
git add .
git commit -m "v1.0.0: Marketplace release"
git remote add origin https://github.com/example/sprint-planner-marketplace
git push -u origin main
```

## Troubleshooting

### Export Fails: "no [export] section"

**Problem:** `skill.toml` is missing export configuration.

**Solution:** Add an `[export]` section:

```toml
[export]
workflows = [
  "workflows/main.meow.toml",
]
```

### Export Fails: "workflow not found"

**Problem:** Path in `export.workflows` doesn't exist.

**Solution:** Verify paths are relative to repository root:

```bash
# Check paths exist
ls workflows/sprint.meow.toml
```

### Skill Install Fails: "invalid skill"

**Problem:** `skill.toml` validation failed.

**Solution:** Check required fields:

```toml
[skill]
name = "my-skill"        # Required
description = "..."      # Required

[targets.claude]         # At least one target required
```

### Workflows Not Found After Setup

**Problem:** Workflows weren't copied to `~/.meow/workflows/`.

**Solution:** Verify copy command in SKILL.md setup instructions:

```bash
# Check skill directory
ls ~/.claude/skills/my-skill/workflows/

# Re-run copy command
cp -r ~/.claude/skills/my-skill/workflows/* ~/.meow/workflows/

# Verify
meow ls
```

## Best Practices

### 1. Support Both Channels

Maintain both `meow-collection.toml` and `skills/<name>/skill.toml` in your repository. The overhead is minimal and reaches more users.

### 2. Version Consistently

Keep versions in sync:
- `meow-collection.toml` → `[collection] version`
- `skill.toml` → `[skill] version`
- Git tags

### 3. Test Both Channels

Before releasing, test both:

```bash
# Test Channel 1
meow collection install .

# Test Channel 2
meow skill export skill-name --for-marketplace
meow skill install dist/plugins/skill-name/skills/skill-name --target claude
```

### 4. Document Prerequisites Clearly

Users coming from marketplace may not know MEOW. Make installation instructions copy-paste-able.

### 5. Use Semantic Versioning

Follow semver for both collection and skill versions:
- **Major:** Breaking changes to workflow interfaces
- **Minor:** New workflows or features
- **Patch:** Bug fixes

### 6. Separate Marketplace Repo

Keep marketplace distribution (`dist/`) in a separate repository to avoid confusing users with the source structure.

## See Also

- [SKILLS.md](SKILLS.md) — Skill format and manifest reference
- [COLLECTIONS.md](COLLECTIONS.md) — Collection manifest format
- [ARCHITECTURE.md](ARCHITECTURE.md) — MEOW architecture and design
