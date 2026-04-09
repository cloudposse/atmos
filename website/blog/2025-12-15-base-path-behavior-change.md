---
slug: base-path-behavior-change
title: "Breaking Change: Empty base_path No Longer Defaults to Current Directory"
authors:
  - aknysh
tags:
  - breaking-change
date: 2025-12-15T00:00:00.000Z
release: v1.202.0
---

Starting with Atmos v1.202.0, empty or omitted `base_path` values in `atmos.yaml` now trigger git root discovery instead of defaulting to the current directory. Users with multiple Atmos projects in a single repository, or where the Atmos project root differs from the git root, must explicitly set `base_path: "."`.

<!--truncate-->

## What Changed

The interpretation of `base_path` in `atmos.yaml` has changed:

**Before (v1.201.x and earlier):**
```yaml
# Both treated as "current directory"
base_path: ""
base_path: "."
```

**After (v1.202.0+):**
```yaml
# Triggers git root discovery with fallback
base_path: ""

# Explicitly uses config file directory relative to the location of the `atmos.yaml`
base_path: "."
```

These are no longer equivalent. An empty `base_path` now means "find the git repository root and use that", while `"."` explicitly means "use the directory where `atmos.yaml` is located".

## Who Is Affected

You may be affected if:

1. **Multiple Atmos projects in one repository** - Each project has its own `atmos.yaml` in a subdirectory
2. **Atmos project root differs from git root** - Your `atmos.yaml` is not at the repository root
3. **Using refarch-scaffold or similar patterns** - Where `atmos.yaml` is in a nested directory

### Symptoms

If affected, you'll see errors like:

```text
The atmos.yaml CLI config file specifies the directory for Atmos stacks
as stacks, but the directory does not exist.
```

Or:

```text
atmos exited with code 1
```

## How to Fix

### Option 1: Explicitly Set base_path (Recommended)

Update your `atmos.yaml` to explicitly specify the base path:

```yaml
# Before (no longer works as expected)
base_path: ""

# After (explicit current directory)
base_path: "."
```

### Option 2: Use Relative Paths

If your stacks and components are relative to the config file:

```yaml
base_path: "."

stacks:
  base_path: "stacks"

components:
  terraform:
    base_path: "components/terraform"
```

### Option 3: Navigate to Project Root

If you prefer empty `base_path`, ensure you run Atmos from the git repository root where `stacks/` and `components/` directories exist.

## Path Resolution Semantics

For reference, here's how different `base_path` values are now interpreted:

| `base_path` value | Resolves to | Use case |
|-------------------|-------------|----------|
| `""` (empty/unset) | Git repo root, fallback to config dir | Default - single project at repo root |
| `"."` | Directory containing `atmos.yaml` | Explicit config-relative paths |
| `".."` | Parent of config directory | Config in subdirectory |
| `"./foo"` | config-dir/foo | Explicit relative path |
| `"foo"` | git-root/foo with fallback | Simple relative path |
| `"/absolute/path"` | As specified | Absolute path override |

## Why This Change?

This change was made to support running `atmos` commands from anywhere within a repository, similar to how `git` commands work. The git root discovery enables:

- Running `atmos terraform plan vpc -s dev` from any subdirectory
- Consistent behavior regardless of current working directory
- Better alignment with developer workflows

For users with non-standard project layouts, the explicit `base_path: "."` provides the previous behavior.

## References

- [PR #1872: Correct base path resolution semantics](https://github.com/cloudposse/atmos/pull/1872)
- [PR #1868: Fix base path resolution and fallback order](https://github.com/cloudposse/atmos/pull/1868)
- [Issue #1858: Path resolution regression](https://github.com/cloudposse/atmos/issues/1858)
- [CLI Configuration Documentation](https://atmos.tools/cli/configuration)
