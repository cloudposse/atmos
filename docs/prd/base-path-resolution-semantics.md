# PRD: Base Path Resolution Semantics

## Overview

This PRD formalizes the resolution semantics for the `base_path` configuration option in `atmos.yaml`. It defines how different `base_path` values are resolved to absolute paths, ensuring predictable behavior regardless of where atmos is executed from.

## Core Convention: Empty vs Dot vs Bare

The entire resolution system rests on one principle: **the value you provide tells Atmos _how_ to resolve, and the source tells Atmos _where_ "here" is.**

There are four categories of `base_path` value:

| Category | Pattern | Meaning | Example |
|----------|---------|---------|---------|
| **Empty** | `""`, unset, `~`, `null` | "I have no opinion — use smart defaults" | `base_path: ""` |
| **Dot** | `"."`, `"./"`, `"./foo"`, `".."`, `"../foo"` | "HERE" — an explicit anchor to a contextual location | `base_path: "."` |
| **Bare** | `"foo"`, `"foo/bar"`, `".hidden"`, `".terraform/modules/x"` | "Find this name" — a search via git root → config dir | `base_path: "stacks"` |
| **Absolute** | `"/abs/path"` | "This exact location" — no resolution needed | `base_path: "/opt/atmos"` |

### `""` ≠ `"."`

This distinction is fundamental and intentional:

- **`""`** means "I didn't specify anything." Atmos applies its smart default: discover the git root, fall back to config dir. The user has **no opinion** about where the base path should be.
- **`"."`** means "I explicitly said HERE." The user has an **opinion** — they want the base path anchored to a specific location. Where "here" is depends on context (see below).

These are never interchangeable. An empty string is the absence of a signal. A dot is a signal.

### `"./foo"` ≠ `"foo"`

- **`"./foo"`** has a dot-slash prefix — it's an explicit anchor. "HERE, then foo." It resolves relative to context.
- **`"foo"`** has no prefix — it's a name to search for. Atmos looks for `foo` at the git root first, then the config directory.

### `".terraform/..."` ≠ `"./.terraform/..."`

This catches people. `.terraform` starts with a dot, but it's a **directory name** (`.t...`), not the `./` prefix. It's a bare path — a name to search for. To make it CWD-relative, use `"./.terraform/..."` (dot-slash prefix).

## Where "Here" Is: Source-Dependent Anchoring

The dot-prefix (`"."`, `"./foo"`, `".."`, `"../foo"`) always means "here." But where "here" is depends on where the value came from:

| Source | "Here" means | Convention |
|--------|-------------|------------|
| `atmos.yaml` (config file) | Directory containing `atmos.yaml` | Like `tsconfig.json`, `package.json`, `.eslintrc` |
| `ATMOS_BASE_PATH` (env var) | CWD (current working directory) | Like every other shell tool |
| `--base-path` (CLI flag) | CWD | Like every other CLI tool |
| `atmos_base_path` (provider param) | CWD | Provider runs in a component directory |

This isn't special Atmos logic. It's how dots work everywhere:
- In a Makefile, `./scripts/build.sh` means relative to the Makefile.
- In a shell, `./scripts/build.sh` means relative to CWD.
- Same dot, different context, same intuition.

**Bare paths and empty values are source-independent.** `ATMOS_BASE_PATH=stacks` and `base_path: stacks` in atmos.yaml go through the exact same git root search. The source only matters for dot-prefixed values.

## Comprehensive Resolution Table

**Setup:** `atmos.yaml` at `/repo/config/atmos.yaml`, git root is `/repo`, CWD is `/repo/components/terraform/vpc`

| # | Config (`atmos.yaml`) | Env var (`ATMOS_BASE_PATH`) | Resolves to | Category | Why |
|---|----------------------|----------------------------|-------------|----------|-----|
| 1 | `base_path: ""` | *(not set)* | `/repo` | Empty | No opinion → git root discovery |
| 2 | *(not set)* | `""` | `/repo` | Empty | Empty env var = not set → config/default |
| 3 | `base_path: "."` | — | `/repo/config` | Dot | "Here" in a config file = config dir |
| 4 | — | `ATMOS_BASE_PATH=.` | `/repo/components/terraform/vpc` | Dot | "Here" in a shell = CWD |
| 5 | `base_path: "./stacks"` | — | `/repo/config/stacks` | Dot | Config dir + relative path |
| 6 | — | `ATMOS_BASE_PATH=./stacks` | `/repo/components/terraform/vpc/stacks` | Dot | CWD + relative path |
| 7 | `base_path: ".."` | — | `/repo` | Dot | Up from config dir |
| 8 | — | `ATMOS_BASE_PATH=..` | `/repo/components/terraform` | Dot | Up from CWD |
| 9 | `base_path: "../../.."` | — | *(3 levels up from `/repo/config`)* | Dot | Navigate up from config dir |
| 10 | — | `ATMOS_BASE_PATH=../../..` | `/repo` | Dot | Navigate up from CWD (3 levels from `vpc`) |
| 11 | `base_path: "stacks"` | — | `/repo/stacks` | Bare | Search: git root/stacks |
| 12 | — | `ATMOS_BASE_PATH=stacks` | `/repo/stacks` | Bare | **Same search** — source-independent |
| 13 | `base_path: "custom/path"` | — | `/repo/custom/path` | Bare | Search: git root/custom/path |
| 14 | — | `ATMOS_BASE_PATH=custom/path` | `/repo/custom/path` | Bare | **Same search** — source-independent |
| 15 | — | `ATMOS_BASE_PATH=./.terraform/modules/monorepo` | `/repo/components/terraform/vpc/.terraform/modules/monorepo` | Dot | Dot-slash anchors to CWD |
| 16 | — | `ATMOS_BASE_PATH=.terraform/modules/monorepo` | `/repo/.terraform/modules/monorepo` | Bare | `.terraform` is a name, not `./` prefix → search |
| 17 | `base_path: "/abs/path"` | — | `/abs/path` | Absolute | Absolute paths pass through |
| 18 | — | `ATMOS_BASE_PATH=/abs/path` | `/abs/path` | Absolute | Same — absolute is absolute |

### Verifying Consistency

The table demonstrates three properties:

1. **Empty is always the same** — Rows 1-2: `""` always means git root discovery, regardless of source.
2. **Bare is always the same** — Rows 11-14: `"stacks"` always goes through git root search, regardless of source.
3. **Dot adapts to context** — Rows 3-10, 15: `"."` means "here", where "here" depends on whether you're in a file or a shell.

No row contradicts another. No value has surprise behavior based on its source. The only thing that changes is the anchor for dot-prefix, which matches universal convention.

### Simple Pseudocode

```
# Config-file source (base_path in atmos.yaml):
if path == "" or path is unset:
    return git_repo_root() or dirname(atmos.yaml) or CWD
if path is absolute:
    return path
if path == "." or starts with "./" or ".." or "../":
    return dirname(atmos.yaml) / path       # config-dir-relative
else:  # bare path ("stacks", "foo/bar")
    return git_repo_root() / path           # git-root search (os.Stat fallback to CWD)

# Runtime source (ATMOS_BASE_PATH, --base-path) — only dot-prefix differs:
if path == "." or starts with "./" or ".." or "../":
    return CWD / path                       # CWD-relative (shell convention)
# All other categories (empty, bare, absolute) resolve identically to above.
```

### Quick Reference

```
# Setup: atmos.yaml at /repo/config/atmos.yaml, git root is /repo,
#         CWD is /repo/components/terraform/vpc

# ── Config file (base_path in atmos.yaml) ──────────────────────────
# Dot-prefixed paths anchor to config dir (dirname of atmos.yaml)
base_path: ""           → /repo                    (empty → git root discovery)
base_path: "."          → /repo/config             (dot → config dir)
base_path: ".."         → /repo                    (parent of config dir)
base_path: "./foo"      → /repo/config/foo         (config dir + foo)
base_path: "../foo"     → /repo/foo                (parent of config dir + foo)
base_path: "stacks"     → /repo/stacks             (bare → git root search)
base_path: "foo/bar"    → /repo/foo/bar            (bare → git root search)
base_path: "/abs/path"  → /abs/path                (absolute → pass through)
base_path: !repo-root   → /repo                    (explicit git root tag)
base_path: !cwd         → /repo/components/terraform/vpc  (explicit CWD tag)

# ── Environment variable / CLI flag (runtime source) ───────────────
# Dot-prefixed paths anchor to CWD (shell convention)
ATMOS_BASE_PATH=""                              → /repo                    (empty → git root discovery)
ATMOS_BASE_PATH=.                               → /repo/components/terraform/vpc  (dot → CWD)
ATMOS_BASE_PATH=..                              → /repo/components/terraform      (parent of CWD)
ATMOS_BASE_PATH=./foo                           → /repo/components/terraform/vpc/foo  (CWD + foo)
ATMOS_BASE_PATH=../foo                          → /repo/components/terraform/foo  (parent of CWD + foo)
ATMOS_BASE_PATH=stacks                          → /repo/stacks             (bare → git root search)
ATMOS_BASE_PATH=./.terraform/modules/monorepo   → /repo/components/terraform/vpc/.terraform/modules/monorepo  (dot-slash → CWD)
ATMOS_BASE_PATH=.terraform/modules/monorepo     → /repo/.terraform/modules/monorepo  (bare → git root search, os.Stat fallback to CWD)
ATMOS_BASE_PATH=/abs/path                       → /abs/path                (absolute → pass through)

# ── Same setup but NOT in a git repo ───────────────────────────────
# Bare paths fall back to config dir instead of git root
base_path: ""           → /repo/config             (empty → fallback to config dir)
base_path: "."          → /repo/config             (dot → config dir, unchanged)
base_path: "stacks"     → /repo/config/stacks      (bare → fallback to config dir)
ATMOS_BASE_PATH=.       → /repo/components/terraform/vpc  (dot → CWD, unchanged)
ATMOS_BASE_PATH=stacks  → /repo/config/stacks      (bare → fallback to config dir)
```

## Motivation

Issue #1858 revealed that the resolution behavior for empty `base_path` was undefined when using `ATMOS_CLI_CONFIG_PATH`. Issue #2183 (Tyler Rankin / Spacelift) revealed that `ATMOS_BASE_PATH=.terraform/modules/monorepo` broke in v1.202.0 when git root discovery was extended to all simple relative paths.

The original PRD did not distinguish between the four value categories or formalize the source-dependent anchoring for dot-prefix. This led to two conflicting fixes — one making env vars config-relative, the other making them CWD-relative — because the underlying convention was never stated.

## Requirements

### Functional Requirements

#### FR1: Value Category Classification

Every `base_path` value is classified into exactly one category:

| Category | Detection | Resolution strategy |
|----------|-----------|-------------------|
| Empty | `""`, unset, `~`, `null` | Git root → config dir → CWD |
| Dot | Starts with `"./"`, `"../"`, equals `"."` or `".."` | Anchor-relative (source-dependent) |
| Bare | Non-empty, non-absolute, no dot prefix | Git root search → config dir → CWD fallback |
| Absolute | Starts with `/` (Unix) or drive letter (Windows) | Pass through |

Classification is independent of source. A value is Dot or Bare based on its content alone.

#### FR2: Empty Value Resolution

Empty values trigger smart default resolution:

1. Discover git root → use it as base path
2. No git root → use dirname(atmos.yaml)
3. No config file → use CWD

Empty env var (`ATMOS_BASE_PATH=""`) is treated as unset and does not override config-file values.

#### FR3: Dot-Prefix Resolution (Source-Dependent Anchoring)

Dot-prefixed values resolve relative to an anchor. The anchor depends on the source:

- **Config-file source** (atmos.yaml): anchor = dirname(atmos.yaml)
- **Runtime source** (env var, CLI flag, provider param): anchor = CWD

Resolution: `filepath.Join(anchor, path)` → `filepath.Abs()`

#### FR4: Bare Path Resolution (Source-Independent Search)

Bare paths go through git root search regardless of source:

1. If git root exists and `git_root/path` exists on disk → return `git_root/path`
2. If `git_root/path` doesn't exist but `CWD/path` does → return `CWD/path` (with trace log)
3. If neither exists → return `git_root/path` (for consistent error messages)
4. If no git root → resolve relative to config dir, then CWD

The `os.Stat` validation in step 1-2 prevents silent misresolution.

#### FR5: Absolute Path Resolution

Absolute paths are returned as-is. No search, no anchoring.

#### FR6: Explicit Path Tags

- `!repo-root` — Resolves to git repository root
- `!cwd` — Resolves to current working directory (useful in atmos.yaml when you genuinely need CWD)

#### FR7: Environment Variable Interaction

- `ATMOS_BASE_PATH` — Overrides `base_path` in config file. Marked as runtime source.
- `ATMOS_CLI_CONFIG_PATH` — Specifies config file location (does not affect base path source).
- `ATMOS_GIT_ROOT_BASEPATH=false` — Disables git root discovery.

#### FR8: Config File Search Order

| Priority | Source | Description |
|----------|--------|-------------|
| 1 | CLI flags | `--config`, `--config-path` |
| 2 | Environment variable | `ATMOS_CLI_CONFIG_PATH` |
| 3 | Current directory | `./atmos.yaml` (CWD only, no parent search) |
| 4 | Git repository root | `repo-root/atmos.yaml` |
| 5 | Parent directory search | Walks up from CWD looking for `atmos.yaml` |
| 6 | Home directory | `~/.atmos/atmos.yaml` |
| 7 | System directory | `/usr/local/etc/atmos/atmos.yaml` |

#### FR9: Base Path Source Priority

| Priority | Source | Marked as |
|----------|--------|-----------|
| 1 | `configAndStacksInfo.AtmosBasePath` (struct field from `--base-path` or `atmos_base_path`) | `runtime` |
| 2 | `ATMOS_BASE_PATH` environment variable | `runtime` |
| 3 | `base_path` in `atmos.yaml` | (default, config-file) |
| 4 | Default (empty) | (default) |

### Non-Functional Requirements

#### NFR1: Testability

- `ATMOS_GIT_ROOT_BASEPATH=false` to disable git root discovery in tests
- Tests must cover all four value categories for both config and runtime sources

---

## Design Rationale

### Why dot-prefix in config files is config-dir-relative

This follows the convention of other configuration files:
- `tsconfig.json` — paths relative to tsconfig location
- `package.json` — paths relative to package.json location
- `.eslintrc` — paths relative to config location
- `Makefile` — includes relative to Makefile location

**Paths in configuration files are relative to where the config is defined, not where you run the command from.**

### Why dot-prefix in env vars/CLI flags is CWD-relative

This follows the convention of every shell tool:
- `cd ./subdir` — relative to CWD
- `ls ../parent` — relative to CWD
- `SOME_PATH=./local/dir some-tool` — relative to CWD

When a user types a path in their shell, they expect it to resolve from where they are.

### Why bare paths are source-independent

A bare path like `"stacks"` is a name, not a relative reference. There's no dot-anchor, so there's nothing to be "relative to." Instead, it's a search: "find `stacks` starting from the git root."

Making bare paths source-dependent would be confusing: `ATMOS_BASE_PATH=stacks` resolving to CWD/stacks while `base_path: stacks` resolves to git-root/stacks. Two different locations for the same value? That's the incongruence this PRD eliminates.

### Why empty is different from dot

Empty means "I didn't specify." It's the default. Atmos gets to be smart about it — discover the git root, find the project, do the right thing.

Dot means "I specified HERE." The user made a choice. Atmos should respect that choice and resolve "here" based on context.

If empty and dot were the same, then `ATMOS_BASE_PATH=` and `ATMOS_BASE_PATH=.` would be identical. But the first means "use defaults" and the second means "use my current directory." Very different intent.

### Why `!cwd` tag exists

Users who need CWD-relative behavior in `atmos.yaml` can use `!cwd`:

```yaml
base_path: !cwd
# or with a relative path
base_path: !cwd ./relative/path
```

This provides an explicit escape hatch. Note that runtime sources (env vars, CLI flags) already get CWD-relative behavior for dot-prefixed paths, so `!cwd` is primarily useful in `atmos.yaml`.

---

## Specification

### Resolution Algorithm

```
function resolve(value, source, configDir):

    # 1. Classify the value
    category = classify(value)

    # 2. Resolve based on category
    switch category:

        case EMPTY:
            return gitRoot() ?? configDir ?? CWD

        case ABSOLUTE:
            return value

        case DOT:
            if source == "runtime":
                anchor = CWD
            else:
                anchor = configDir
            return abs(join(anchor, value))

        case BARE:
            # Source-independent: always search git root first
            root = gitRoot()
            if root != "":
                if exists(join(root, value)):
                    return join(root, value)
                if exists(abs(value)):          # CWD fallback
                    return abs(value)
                return join(root, value)        # default for error messages
            # No git root: config dir fallback
            if configDir != "":
                return abs(join(configDir, value))
            return abs(value)                   # last resort: CWD
```

### Classification Function

```
function classify(value):
    if value == "" or value is unset:
        return EMPTY
    if isAbsolute(value):
        return ABSOLUTE
    if value == "." or value == ".."
       or startsWith(value, "./") or startsWith(value, "../")
       or startsWith(value, ".\\") or startsWith(value, "..\\"):  # Windows
        return DOT
    return BARE
```

### Source Tracking

The `AtmosConfiguration` struct carries a `BasePathSource` field (`yaml:"-"`) that is set to `"runtime"` when the base path comes from an env var, CLI flag, or provider parameter. Config-file values leave it empty (default).

---

## Issue Resolutions

### Issue #1858

**Setup:** `ATMOS_CLI_CONFIG_PATH=./rootfs/usr/local/etc/atmos`, `base_path: ""`, stacks at repo root.

**Resolution:** Empty base_path → git root discovery → `/repo` → finds `stacks/`.

### Issue #2183 (Tyler Rankin / Spacelift)

**Setup:** `ATMOS_BASE_PATH=.terraform/modules/monorepo`, CWD is component directory.

**Resolution with this PRD:** `.terraform/modules/monorepo` is a **bare path** (no dot-slash prefix). It goes through git root search. `os.Stat` at `git_root/.terraform/modules/monorepo` fails. CWD fallback finds it at `CWD/.terraform/modules/monorepo`.

**Recommended migration:** Use `ATMOS_BASE_PATH=./.terraform/modules/monorepo` (dot-slash prefix). This explicitly anchors to CWD, making intent clear and avoiding the search-then-fallback path.

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
