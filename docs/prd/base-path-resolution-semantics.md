# PRD: Base Path Resolution Semantics

## Overview

This PRD formalizes the resolution semantics for the `base_path` configuration option in `atmos.yaml`. It defines how different `base_path` values are resolved to absolute paths, ensuring predictable behavior regardless of where atmos is executed from.

**Important:** Resolution semantics differ based on the **source** of the `base_path` value. Config-file values resolve relative to the config file location, while values provided by the user at runtime (environment variables, CLI flags, provider parameters) resolve relative to the current working directory.

## Motivation

Issue #1858 revealed that the resolution behavior for empty `base_path` was undefined when using `ATMOS_CLI_CONFIG_PATH`. This PRD formalizes the correct semantics:

- Empty `base_path` should trigger git root discovery, not resolve to the config directory
- Relative paths (`.`, `..`, `./foo`, `../foo`) should be config-file-relative, following the convention of other config files
- Simple relative paths (`foo`) should search git root first, then config directory

Issue #2183 and a related `terraform-provider-utils` regression revealed that the original PRD did not distinguish between base path sources. When `ATMOS_BASE_PATH` is set as an environment variable or `--base-path` is passed as a CLI flag, the user intends the path to resolve relative to CWD, not relative to git root or the config file. The PRD has been updated to formalize source-dependent resolution semantics.

## Requirements

### Functional Requirements

#### FR1: Base Path Resolution (Config-File Source)

These semantics apply when `base_path` is set in `atmos.yaml`:

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
- When `base_path` is a simple relative path (no `./` or `../` prefix) **from a config-file source**
- Regardless of whether config is found via default discovery or `ATMOS_CLI_CONFIG_PATH`

Git root discovery does NOT apply:
- When `base_path` starts with `.` or `./` (explicit config-relative)
- When `base_path` starts with `../` (explicit config-relative navigation)
- When `base_path` is absolute
- When `ATMOS_GIT_ROOT_BASEPATH=false`
- When `base_path` is a simple relative path from a **runtime source** (env var, CLI flag, provider parameter) — see FR6

#### FR4: Environment Variable Interaction

- `ATMOS_BASE_PATH` - Overrides `base_path` in config file (see FR6 for resolution semantics)
- `ATMOS_CLI_CONFIG_PATH` - Specifies config file location
- `ATMOS_GIT_ROOT_BASEPATH=false` - Disables git root discovery

#### FR5: Config File Search Order

Atmos searches for `atmos.yaml` in the following order (highest to lowest priority):

| Priority | Source | Description |
|----------|--------|-------------|
| 1 | CLI flags | `--config`, `--config-path` |
| 2 | Environment variable | `ATMOS_CLI_CONFIG_PATH` |
| 3 | Current directory | `./atmos.yaml` (CWD only, no parent search) |
| 4 | Git repository root | `repo-root/atmos.yaml` |
| 5 | Parent directory search | Walks up from CWD looking for `atmos.yaml` |
| 6 | Home directory | `~/.atmos/atmos.yaml` |
| 7 | System directory | `/usr/local/etc/atmos/atmos.yaml` |

**Note:** Viper deep-merges configurations, so settings from higher-priority sources override those from lower-priority sources.

#### FR6: Runtime Source Resolution (CLI Flag, Env Var, Provider Parameter)

When `base_path` is provided at runtime — via `--base-path` CLI flag, `ATMOS_BASE_PATH` environment variable, or the `atmos_base_path` parameter in `terraform-provider-utils` — resolution semantics differ from config-file values:

| `base_path` value | Resolves to | Rationale |
|-------------------|-------------|-----------|
| `""` (empty) | No override (config-file value used) | Empty means "not set" |
| `"/absolute/path"` | /absolute/path | Explicit absolute |
| `"."` | dirname(atmos.yaml) | Config-file-relative (preserves existing behavior) |
| `"./foo"` | dirname(atmos.yaml)/foo | Config-file-relative (preserves existing behavior) |
| `"../foo"` | Parent of dirname(atmos.yaml)/foo | Config-file-relative (preserves existing behavior) |
| `"foo"` | CWD/foo | **CWD-relative** |
| `"foo/bar"` | CWD/foo/bar | **CWD-relative** |

**Key difference from FR1:** Simple relative paths (`"foo"`, `"foo/bar"`) resolve relative to CWD, **not** via git root search. This is because:

1. **User intent**: When a user types `ATMOS_BASE_PATH=.terraform/modules/monorepo` in their shell or CI config, they mean "relative to where I am right now"
2. **Shell convention**: Environment variables and CLI flags operate in the user's shell context, where relative paths are always CWD-relative
3. **CI/CD compatibility**: CI workers (Spacelift, GitHub Actions) set env vars in component directories where CWD differs from git root

**Priority order for base path sources (highest wins):**

| Priority | Source | Resolution |
|----------|--------|------------|
| 1 | `configAndStacksInfo.AtmosBasePath` (struct field from `--base-path` or `atmos_base_path`) | FR6 (CWD-relative for simple paths) |
| 2 | `ATMOS_BASE_PATH` environment variable (via Viper) | FR6 (CWD-relative for simple paths) |
| 3 | `base_path` in `atmos.yaml` | FR1 (config-file-relative / git root search) |
| 4 | Default (empty) | Git root → config dir → CWD |

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

**Note:** The v1.201.0 commit message specifically references `"../../.."` — a dot-relative path. This fix was correct for dot-relative paths. The regression occurred when v1.202.0 extended git root discovery to *all* simple relative paths regardless of source, which broke CWD-relative env var and CLI flag values.

### Why empty `base_path` triggers git root discovery

Empty/unset values conventionally mean "use sensible defaults":
- Git commands work from anywhere in a repository
- Most repos have `atmos.yaml` at root alongside `stacks/` and `components/`
- Empty = "I don't want to specify, figure it out"

This enables the "run from anywhere" behavior that users expect.

### Why `!cwd` tag exists

Users who need CWD-relative behavior in `atmos.yaml` can use `!cwd`:

```yaml
base_path: !cwd
# or with a relative path
base_path: !cwd ./relative/path
```

This provides an explicit escape hatch for users who genuinely need paths relative to where atmos is executed from within a config file. Note that runtime sources (env vars, CLI flags) already resolve simple relative paths relative to CWD, so `!cwd` is primarily useful in `atmos.yaml`.

### Why runtime sources use CWD-relative resolution

Config-file values and runtime values have fundamentally different contexts:

- **Config files** exist at a fixed location in the repository. Paths in them are authored relative to that location. The user who writes `base_path: stacks` in `atmos.yaml` at `/repo/atmos.yaml` means `/repo/stacks`.
- **Runtime values** are provided in the user's shell context. The user who types `ATMOS_BASE_PATH=.terraform/modules/monorepo` means "relative to my current directory." They may be in `/repo/components/terraform/vpc/` and expect the path to resolve to `/repo/components/terraform/vpc/.terraform/modules/monorepo`.

Applying config-file resolution (git root search) to runtime values breaks the principle of least surprise. When a user sets an env var with a relative path, every other tool resolves it relative to CWD. Atmos should too.

**Dot-relative paths** (`"."`, `"./foo"`, `"../foo"`) from runtime sources still resolve config-file-relative. This preserves backward compatibility with `ATMOS_BASE_PATH="../../.."` (the v1.201.0 fix scenario) and is consistent: dot-prefixed paths always mean "relative to the config file."

---

## Specification

### Detection Logic

```
# Step 1: Determine the source and apply source-specific pre-processing
if source is runtime (CLI flag, env var, provider parameter):
    if path is a simple relative path (not empty, not absolute, no "." or ".." prefix):
        return CWD / path   # CWD-relative for runtime sources

# Step 2: Standard config-file resolution
if path == "" or path is unset:
    return git_repo_root() or dirname(atmos.yaml)

if path is absolute:
    return path

if path == "." or path starts with "./" or path == ".." or path starts with "../":
    return dirname(atmos.yaml) / path  # Config-file-relative

# Simple relative path from config file (e.g., "foo", "foo/bar")
# Try git root first, validate with os.Stat, fall back to CWD
if exists(git_repo_root() / path):
    return git_repo_root() / path
if exists(CWD / path):
    return CWD / path
return git_repo_root() / path  # Default to git root for consistent error messages
```

### Key Semantic Distinctions

1. **`""` (empty) vs `"."`**:
   - `""` = smart default (git root with fallback to config dir)
   - `"."` = explicit config directory (where atmos.yaml lives)

2. **`"./foo"` vs `"foo"` (in atmos.yaml)**:
   - `"./foo"` = explicit config-dir-relative (config-dir/foo)
   - `"foo"` = search path (git-root/foo → config-dir/foo)

3. **`"foo"` in atmos.yaml vs `ATMOS_BASE_PATH=foo`**:
   - In atmos.yaml: search path (git-root/foo → config-dir/foo)
   - As env var: CWD-relative (CWD/foo)

4. **`!cwd` vs `"."`**:
   - `!cwd` = current working directory (where command is run from)
   - `"."` = config directory (where atmos.yaml is located)

---

## Test Cases

### Config-File Source (`base_path` in atmos.yaml)

```
# Scenario: atmos.yaml at /repo/config/atmos.yaml, CWD is /repo/src, git root is /repo

base_path: ""           → /repo                    (git root)
base_path: "."          → /repo/config             (config dir)
base_path: ".."         → /repo                    (parent of config dir)
base_path: "./foo"      → /repo/config/foo         (config-dir-relative)
base_path: "../foo"     → /repo/foo                (parent of config dir)
base_path: "foo"        → /repo/foo                (git root + foo, if exists)
base_path: "foo/bar"    → /repo/foo/bar            (git root + foo/bar, if exists)
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

### Runtime Source (env var, CLI flag, provider parameter)

```
# Scenario: CWD is /repo/components/terraform/vpc, git root is /repo

ATMOS_BASE_PATH=""                                → (no override, config-file value used)
ATMOS_BASE_PATH="."                               → /repo/config             (config-dir-relative)
ATMOS_BASE_PATH="./foo"                           → /repo/config/foo         (config-dir-relative)
ATMOS_BASE_PATH="../foo"                          → /repo/config/../foo      (config-dir-relative)
ATMOS_BASE_PATH=".terraform/modules/monorepo"     → /repo/components/terraform/vpc/.terraform/modules/monorepo (CWD-relative)
ATMOS_BASE_PATH="stacks"                          → /repo/components/terraform/vpc/stacks (CWD-relative)
ATMOS_BASE_PATH="/abs/path"                       → /abs/path                (absolute)
--base-path=".terraform/modules/monorepo"         → /repo/components/terraform/vpc/.terraform/modules/monorepo (CWD-relative)
--base-path="/abs/path"                           → /abs/path                (absolute)
atmos_base_path=".terraform/modules/monorepo"     → /repo/components/terraform/vpc/.terraform/modules/monorepo (CWD-relative)
```

---

## Implementation

### Two-Pronged Approach

The implementation handles the two runtime source code paths separately:

**Prong 1: Struct field (`configAndStacksInfo.AtmosBasePath`)**

Set by `--base-path` CLI flag or `atmos_base_path` provider parameter. In `InitCliConfig`, before `AtmosConfigAbsolutePaths` runs, `resolveSimpleRelativeBasePath()` converts simple relative paths to absolute via `filepath.Abs()` (CWD-relative). The resulting absolute path passes through `resolveAbsolutePath()` unchanged.

**Prong 2: Env var (`ATMOS_BASE_PATH`)**

Read by Viper into `atmosConfig.BasePath`. The value reaches `resolveAbsolutePath()` → `tryResolveWithGitRoot()`. The `os.Stat` validation check tries the git-root-joined path first; if it doesn't exist, falls back to CWD-relative. This handles the case where `ATMOS_BASE_PATH=.terraform/modules/monorepo` is set from a component directory where the path exists relative to CWD but not at git root.

### `resolveSimpleRelativeBasePath()` Function

Classifies paths and converts simple relative paths to absolute:
- Empty → return as-is (no override)
- Absolute → return as-is
- Dot-relative (`.`, `./foo`, `..`, `../foo`) → return as-is (for config-file-relative resolution)
- Simple relative (`foo`, `foo/bar`) → `filepath.Abs()` (CWD-relative)

### `tryResolveWithGitRoot()` `os.Stat` Fallback

For simple relative paths from config-file source:
1. Try `git_root / path` — if exists, return it
2. Try `CWD / path` — if exists, return it (with trace log)
3. Neither exists — return git root path (for consistent error messages)

This also handles the env var case: if `ATMOS_BASE_PATH` sets a path that exists at CWD but not at git root, the fallback finds it.

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

## Issue #2183 Resolution

**User's setup (Tyler Rankin / Spacelift):**
- `ATMOS_BASE_PATH=.terraform/modules/monorepo` (environment variable on CI worker)
- CWD is component directory: `/project/components/terraform/iam-delegated-roles/`
- Git root is `/project/`

**Before fix (broken, v1.202.0+):**
- Viper reads `ATMOS_BASE_PATH` into `atmosConfig.BasePath`
- `resolveAbsolutePath(".terraform/modules/monorepo")` routes through git root discovery
- Resolves to `/project/.terraform/modules/monorepo` — **WRONG** (doesn't exist)
- `GetGlobMatches` returns `ErrFailedToFindImport`
- Error message provides no context about which path failed

**After fix (working):**
- `tryResolveWithGitRoot` tries `/project/.terraform/modules/monorepo` → doesn't exist (`os.Stat` fails)
- Falls back to CWD-relative: `/project/components/terraform/iam-delegated-roles/.terraform/modules/monorepo` → exists
- Stacks and components found at the correct location
- Error messages now include actionable hints and context (pattern, stacks base path)

---

## References

- Issue #1858: Path resolution regression report
- Issue #2183: `failed to find import` with `ATMOS_BASE_PATH` env var
- PR #1774: Path-based component resolution (introduced config-relative behavior)
- PR #1773: Git root discovery for default base path
- PR #2215: Fix explicit base paths resolving relative to CWD
- Fix doc: `docs/fixes/2026-03-17-failed-to-find-import-base-path-resolution.md`
- Related PRD: `docs/prd/git-root-discovery-default-behavior.md`
- Related PRD: `docs/prd/component-path-resolution.md`
