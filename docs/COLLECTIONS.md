# MEOW Collection Manifest

`meow-collection.toml` is the manifest file that defines a workflow collection.
It must live at the repository root.

## Example

```toml
[collection]
name = "akatz-workflows"
description = "Aaron's collection of MEOW workflows"
version = "1.0.0"
meow_version = ">=0.1.0"

[collection.owner]
name = "Aaron Katz"
email = "aaron@example.com"
url = "https://github.com/akatz-ai"

[collection.repository]
url = "https://github.com/akatz-ai/meow-workflows"
license = "MIT"

[[packs]]
name = "agent-utils"
description = "Utilities for agent lifecycle"
workflows = [
  "workflows/lib/agent-persistence.meow.toml",
  "workflows/lib/worktree.meow.toml",
]
```

## Field Reference

### `[collection]`

| Field | Required | Description |
| --- | --- | --- |
| `name` | Yes | Unique identifier (lowercase alphanumeric with hyphens) |
| `description` | Yes | Human-readable description |
| `version` | Yes | Semantic version (X.Y.Z) |
| `meow_version` | No | Semver constraint for minimum MEOW version |

### `[collection.owner]`

| Field | Required | Description |
| --- | --- | --- |
| `name` | Yes | Author or organization name |
| `email` | No | Contact email |
| `url` | No | Website or profile URL |

### `[collection.repository]`

| Field | Required | Description |
| --- | --- | --- |
| `url` | No | Repository URL |
| `license` | No | License identifier (MIT, Apache-2.0, etc.) |

### `[[packs]]`

| Field | Required | Description |
| --- | --- | --- |
| `name` | Yes | Pack identifier (lowercase alphanumeric with hyphens) |
| `description` | Yes | Human-readable description |
| `workflows` | Yes | List of workflow paths relative to repo root |

## Workflow Paths

Workflow paths are relative to the repository root and must point to valid `.meow.toml`
workflow files. The directory structure is preserved when installing a collection.
