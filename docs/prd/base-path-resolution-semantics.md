# PRD: Base Path Resolution Semantics

## Overview

This PRD formalizes the resolution semantics for the `base_path` configuration option in `atmos.yaml`. It defines how different `base_path` values are resolved to absolute paths, ensuring predictable behavior regardless of where atmos is executed from.

## Motivation

Issue #1858 revealed that the resolution behavior for empty `base_path` was undefined when using `ATMOS_CLI_CONFIG_PATH`. This PRD formalizes the correct semantics:

- Empty `base_path` should trigger git root discovery, not resolve to the config directory
- Relative paths (`.`, `..`, `./foo`, `../foo`) should be config-file-relative, following the convention of other config files
- Simple relative paths (`foo`) should search git root first, then config directory

## Requirements

### Functional Requirements

#### FR1: Base Path Resolution

| `base_path` value | Resolves to | Rationale |
|-------------------|-------------|-----------|
| `""`, `~`, `null`, or unset | Search: git root → dirname(atmos.yaml) | Smart default |
| `"."` | dirname(atmos.yaml) | Config-file-relative |
| `".."` | Parent of dirname(atmos.yaml) | Config-file-relative |
| `"./foo"` | dirname(atmos.yaml)/foo | Config-file-relative |
| `"../foo"` | Parent of dirname(atmos.yaml)/foo | Config-file-relative |
| `"foo"` | Search: git root/foo → dirname(atmos.yaml)/foo | Search path |
| `"/absolute/path"` | /absolute/path | Explicit absolute |
| `!repo-root` | Git repository root | Explicit git root tag |
| `!cwd` | Current working directory | Explicit CWD tag |

**Search order for empty and simple relative paths:**
1. Git repo root (most common - standard repo structure)
2. dirname(atmos.yaml) (fallback when not in git repo)

#### FR2: Explicit Path Tags

- `!repo-root` - Resolves to git repository root, with optional default value
- `!cwd` - Resolves to current working directory, with optional relative path

#### FR3: Git Root Discovery

Git root discovery applies:
- When `base_path` is empty/unset
- When `base_path` is a simple relative path (no `./` or `../` prefix)
- Regardless of whether config is found via default discovery or `ATMOS_CLI_CONFIG_PATH`

Git root discovery does NOT apply:
- When `base_path` starts with `.` or `./` (explicit config-relative)
- When `base_path` starts with `../` (explicit config-relative navigation)
- When `base_path` is absolute
- When `ATMOS_GIT_ROOT_BASEPATH=false`

#### FR4: Environment Variable Interaction

- `ATMOS_BASE_PATH` - Overrides `base_path` in config file
- `ATMOS_CLI_CONFIG_PATH` - Specifies config file location
- `ATMOS_GIT_ROOT_BASEPATH=false` - Disables git root discovery

### Non-Functional Requirements

#### NFR1: Testability

- `TEST_GIT_ROOT` environment variable for test isolation (mocks git root path)

---

## Design Rationale

### Why `.` and `..` are config-file-relative

This follows the convention of other configuration files:
- `tsconfig.json` - paths relative to tsconfig location
- `package.json` - paths relative to package.json location
- `.eslintrc` - paths relative to config location
- `Makefile` - includes relative to Makefile location

**Paths in configuration files are relative to where the config is defined, not where you run the command from.**

This is the behavior introduced in v1.201.0 (PR #1774) and is intentional. The commit message states:
> "This is the key fix: when ATMOS_BASE_PATH is relative (e.g., "../../.."), we need to resolve it relative to where atmos.yaml is, not relative to CWD."

Pre-v1.201.0 used CWD-relative (via `filepath.Abs`) which was the bug being fixed.

### Why empty `base_path` triggers git root discovery

Empty/unset values conventionally mean "use sensible defaults":
- Git commands work from anywhere in a repository
- Most repos have `atmos.yaml` at root alongside `stacks/` and `components/`
- Empty = "I don't want to specify, figure it out"

This enables the "run from anywhere" behavior that users expect.

### Why `!cwd` tag exists

Users who need CWD-relative behavior can use `!cwd`:

```yaml
base_path: !cwd
# or with a relative path
base_path: !cwd ./relative/path
```

This provides an explicit escape hatch for users who genuinely need paths relative to where atmos is executed from.

---

## Specification

### Detection Logic

```
if path == "" or path is unset:
    return git_repo_root() or dirname(atmos.yaml)

if path is absolute:
    return path

if path == "." or path starts with "./" or path == ".." or path starts with "../":
    return dirname(atmos.yaml) / path  # Config-file-relative

# Simple relative path (e.g., "foo", "foo/bar")
return git_repo_root() / path or dirname(atmos.yaml) / path
```

### Key Semantic Distinctions

1. **`""` (empty) vs `"."`**:
   - `""` = smart default (git root with fallback to config dir)
   - `"."` = explicit config directory (where atmos.yaml lives)

2. **`"./foo"` vs `"foo"`**:
   - `"./foo"` = explicit config-dir-relative (config-dir/foo)
   - `"foo"` = search path (git-root/foo → config-dir/foo)

3. **`!cwd` vs `"."`**:
   - `!cwd` = current working directory (where command is run from)
   - `"."` = config directory (where atmos.yaml is located)

---

## Test Cases

```
# Scenario: atmos.yaml at /repo/config/atmos.yaml, CWD is /repo/src, git root is /repo

base_path: ""           → /repo                    (git root)
base_path: "."          → /repo/config             (config dir)
base_path: ".."         → /repo                    (parent of config dir)
base_path: "./foo"      → /repo/config/foo         (config-dir-relative)
base_path: "../foo"     → /repo/foo                (parent of config dir)
base_path: "foo"        → /repo/foo                (git root + foo)
base_path: "foo/bar"    → /repo/foo/bar            (git root + foo/bar)
base_path: "/abs/path"  → /abs/path                (absolute)
base_path: !repo-root   → /repo                    (explicit git root)
base_path: !cwd         → /repo/src                (explicit CWD)

# Scenario: Same setup but NOT in a git repo

base_path: ""           → /repo/config             (fallback to config dir)
base_path: "."          → /repo/config             (config dir)
base_path: ".."         → /repo                    (parent of config dir)
base_path: "./foo"      → /repo/config/foo         (config-dir-relative)
base_path: "../foo"     → /repo/foo                (parent of config dir)
base_path: "foo"        → /repo/config/foo         (fallback to config-dir + foo)
base_path: "foo/bar"    → /repo/config/foo/bar     (fallback to config-dir + foo/bar)
base_path: "/abs/path"  → /abs/path                (absolute)
base_path: !cwd         → /repo/src                (explicit CWD)
```

---

## Issue #1858 Resolution

**User's setup:**
- `ATMOS_CLI_CONFIG_PATH=./rootfs/usr/local/etc/atmos`
- `base_path: ""` in their atmos.yaml
- `stacks/` and `components/` at repo root

**Before fix (broken):**
- Empty `base_path` resolved to config directory (`/repo/rootfs/usr/local/etc/atmos/`)
- Atmos looked for `stacks/` at `/repo/rootfs/usr/local/etc/atmos/stacks/`
- Directory doesn't exist → error

**After fix (working):**
- Empty `base_path` triggers git root discovery
- Resolves to `/repo` (git root)
- `stacks/` found at `/repo/stacks/`

---

## References

- Issue #1858: Path resolution regression report
- PR #1774: Path-based component resolution (introduced config-relative behavior)
- PR #1773: Git root discovery for default base path
- Related PRD: `docs/prd/git-root-discovery-default-behavior.md`
- Related PRD: `docs/prd/component-path-resolution.md`
