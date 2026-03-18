# `failed to find import` Due to Base Path Resolution via Git Root

**Date:** 2026-03-17

**Related Issues:**
- `atmos describe affected` fails with `Error: failed to find import` (GitHub issue #2183)
- `terraform-provider-utils` v1.32.0+ fails with `failed to find import` on
  `data.utils_component_config` when `ATMOS_BASE_PATH` env var is set as a relative path
- Both issues affect Atmos versions > v1.200.0

**Affected Atmos Versions:** v1.202.0+ (since the base path behavior change to git root discovery)

**Severity:** High — blocks `describe affected`, `terraform plan` with utils provider, and CI/CD pipelines

---

## Issue Description

Two related failures surface the same error:

### Scenario 1: `terraform-provider-utils` with `ATMOS_BASE_PATH` env var

When using `terraform-provider-utils` v1.32.0+ (which depends on Atmos v1.207.0+), Terraform plans
fail with:

```text
│ Error: failed to find import
│
│   with module.account_map.data.utils_component_config.config[0],
│   on .terraform/modules/account_map/modules/remote-state/main.tf line 1
```

The user sets the `ATMOS_BASE_PATH` environment variable to a relative path on the CI/CD worker
(e.g., `ATMOS_BASE_PATH=.terraform/modules/monorepo`). This env var is read by Viper during config
loading and stored in `atmosConfig.BasePath`. With Atmos v1.200.0 and earlier, this resolved relative
to CWD. With v1.202.0+, `resolveAbsolutePath()` routes simple relative paths (those not starting with
`./` or `../`) through git root discovery, producing a wrong absolute path.

The base path can also be set via:
- The `atmos_base_path` parameter on the `data.utils_component_config` Terraform resource (passed
  to Atmos via `configAndStacksInfo.AtmosBasePath`)
- The `--base-path` CLI flag

### Scenario 2: `atmos describe affected`

Running `atmos describe affected` or `atmos tf plan --affected` fails with:

```text
Error: failed to find import
```

This happens during stack processing when the stacks base path is computed incorrectly due to git root
discovery interfering with the configured `base_path`.

### Error Message Problem

The error `"failed to find import"` provides no context about which import failed or what path was
searched. Users cannot diagnose whether it's a configuration issue, a missing file, or a path
resolution bug.

---

## Root Cause Analysis

### Path Resolution Changed in v1.202.0

The `resolveAbsolutePath()` function in `pkg/config/config.go` was updated in v1.202.0 to use git
root discovery for resolving relative paths. The resolution logic is:

1. If path is absolute → return as-is
2. If path starts with `./` or `../` → resolve relative to `atmos.yaml` location
3. If path is empty or a simple relative path (e.g., `"stacks"`, `".terraform/modules/monorepo"`) →
   **resolve relative to git root**, falling back to `atmos.yaml` dir, then CWD

Step 3 is the problem. Simple relative paths like `.terraform/modules/monorepo` are treated as
git-root-relative, but they are actually CWD-relative (set by the user via `--base-path`,
`ATMOS_BASE_PATH` env var, or the `atmos_base_path` provider parameter).

### Why This Breaks the Provider

The `terraform-provider-utils` runs inside a Terraform provider plugin process. The working directory
is the component directory (e.g., `components/terraform/iam-delegated-roles/`). The user sets
`ATMOS_BASE_PATH=.terraform/modules/monorepo` as an environment variable on the CI/CD worker. Viper
reads this env var and stores it in `atmosConfig.BasePath`. Then `resolveAbsolutePath()` routes it
through git root discovery:

- CWD: `/project/components/terraform/iam-delegated-roles/`
- Git root: `/project/` (the monorepo root)
- `resolveAbsolutePath(".terraform/modules/monorepo")` →
  `filepath.Join("/project/", ".terraform/modules/monorepo")` →
  `/project/.terraform/modules/monorepo` — **WRONG**
- Correct path: `/project/components/terraform/iam-delegated-roles/.terraform/modules/monorepo`

The stacks directory doesn't exist at the git-root-relative path, so `GetGlobMatches()` returns
`ErrFailedToFindImport`.

### Why This Breaks `describe affected`

`describe affected` calls `ExecuteDescribeStacks` which calls `InitCliConfig` →
`AtmosConfigAbsolutePaths`. If `base_path` in `atmos.yaml` is empty (`""`), `resolveAbsolutePath("")`
returns the git root. In most cases this is correct, but in CI environments where the working
directory structure differs from expectations (e.g., GitHub Actions with custom checkout paths), the
git root may not be the expected base path.

Additionally, `FindAllStackConfigsInPathsForStack` (in `pkg/config/utils.go:55-57`) returns the raw
`ErrFailedToFindImport` without wrapping it with context about which path or pattern failed.

### The `GetGlobMatches` Discrepancy

There are two implementations of `GetGlobMatches`:

1. **`pkg/utils/glob_utils.go`** (used by stack processor) — treats `matches == nil` as
   `ErrFailedToFindImport`
2. **`pkg/filesystem/glob.go`** (newer) — treats `matches == nil` as empty result (not an error),
   only returns `ErrFailedToFindImport` when the base directory doesn't exist

The stack processor uses the old implementation (via `u.GetGlobMatches`), which returns an error on
zero matches even when the directory exists but no files match the pattern.

---

## Fix

### Fix 1: Source-Aware Base Path Resolution

Base path values are now classified into four categories (see `docs/prd/base-path-resolution-semantics.md`):

| Category | Pattern | Resolution |
|----------|---------|------------|
| **Empty** | `""`, unset | Git root → config dir → CWD (smart default) |
| **Dot** | `"."`, `"./foo"`, `".."`, `"../foo"` | Source-dependent anchor (see below) |
| **Bare** | `"foo"`, `"foo/bar"`, `".terraform/..."` | Git root search, source-independent |
| **Absolute** | `"/abs/path"` | Pass through unchanged |

**Source-dependent anchoring for dot-prefixed paths:**

A `BasePathSource` field on `AtmosConfiguration` tracks whether the base path came from a runtime
source (env var, CLI flag, provider parameter) or from the config file (`atmos.yaml`). This field is
set to `"runtime"` in three places:

- `InitCliConfig` — when `configAndStacksInfo.AtmosBasePath` is set (struct field from `--base-path`
  or `atmos_base_path`)
- `processEnvVars` — when `ATMOS_BASE_PATH` env var is set

`resolveAbsolutePath()` now accepts a `source` parameter and routes dot-prefixed paths through
`resolveDotPrefixPath()`, which resolves:
- **Runtime source** (`"runtime"`): relative to CWD (shell convention)
- **Config source** (default): relative to the directory containing `atmos.yaml`

Bare paths go through `tryResolveWithGitRoot()` regardless of source — they are source-independent.

**File:** `pkg/config/config.go`

**Tyler's scenario fix:** `ATMOS_BASE_PATH=./.terraform/modules/monorepo` (dot-slash prefix) now
correctly resolves relative to CWD because the env var is marked as a runtime source. The bare form
`ATMOS_BASE_PATH=.terraform/modules/monorepo` goes through git root search with an `os.Stat`
fallback to CWD when the git-root-joined path doesn't exist.

### Fix 2: Improve Error Messages

Wrap `ErrFailedToFindImport` with context wherever it surfaces, using the error builder pattern:
- In `pkg/config/utils.go` — both `FindAllStackConfigsInPathsForStack` and `FindAllStackConfigsInPaths`
  now use `errUtils.Build(err)` with actionable hints and context (pattern, stacks_base_path)
- In `pkg/utils/glob_utils.go` — added actionable hints about checking `base_path` and
  `stacks.base_path` in `atmos.yaml`, and about `ATMOS_BASE_PATH` env var

### Fix 3: Migrate to `pkg/filesystem/glob.go` (Deferred)

The old `pkg/utils/glob_utils.go:GetGlobMatches` treats zero matches as an error. The newer
`pkg/filesystem/glob.go:GetGlobMatches` correctly treats zero matches as empty results. The stack
processor should migrate to the newer implementation to avoid false errors. This is deferred to a
separate PR to minimize the blast radius of this fix.

---

## Files Modified

| File                                       | Change                                                                         |
|--------------------------------------------|--------------------------------------------------------------------------------|
| `pkg/config/config.go`                     | Source-aware resolution: `BasePathSource` tracking, `resolveDotPrefixPath()`, `os.Stat` fallback in `tryResolveWithGitRoot()`, error builder pattern for all path errors |
| `pkg/config/utils.go`                      | Set `BasePathSource = "runtime"` in `processEnvVars()` and `setBasePaths()`, wrap errors with builder pattern |
| `pkg/schema/schema.go`                     | Add `BasePathSource` field to `AtmosConfiguration`                             |
| `pkg/utils/glob_utils.go`                  | Add actionable hints to GetGlobMatches error                                   |
| `pkg/config/base_path_resolution_test.go`  | Tests for source-aware resolution, `BasePathSource` tracking, and error wrapping |

---

## Git Root Discovery Compatibility

A key concern is that these fixes do not break the "run Atmos from any subdirectory" feature
introduced in v1.202.0 via git root discovery. Analysis confirms all code paths remain intact:

### How Git Root Discovery Works

When a user runs `atmos terraform plan vpc -s dev` from a subdirectory (e.g.,
`components/terraform/vpc/`), the flow is:

1. `SearchConfigFile()` walks up from CWD to find `atmos.yaml` (e.g., at `/project/atmos.yaml`)
2. `resolveAbsolutePath()` resolves `stacks.base_path` (typically `"stacks"`) relative to git root
3. `tryResolveWithGitRoot()` joins git root + path → `/project/stacks` — this directory exists

### Why Our Fixes Don't Break It

**Fix 1 (source-aware resolution)**: `resolveAbsolutePath()` now accepts a `source` parameter.
When the base path comes from a runtime source (env var, CLI flag, provider parameter), the
`BasePathSource` field is set to `"runtime"`. This only affects dot-prefixed paths (`"."`,
`"./foo"`, `".."`), which resolve relative to CWD for runtime sources instead of config dir.
Bare paths (`"stacks"`, `"foo/bar"`) go through the same git root search regardless of source.
Normal Atmos CLI usage with `base_path` in atmos.yaml is unaffected.

**Fix 2 (`os.Stat` fallback in `tryResolveWithGitRoot`)**: The added `os.Stat` check validates
the git-root-joined path exists before returning it. For normal projects:

- `resolveAbsolutePath("stacks")` → `tryResolveWithGitRoot("stacks")` →
  `filepath.Join(gitRoot, "stacks")` → `/project/stacks` — **exists** → returned ✅
- `resolveAbsolutePath("")` → returns `gitRoot` directly at line 315 (before `os.Stat`) ✅
- `resolveAbsolutePath("./foo")` → caught by `isExplicitRelative` → never reaches `os.Stat` ✅

The `os.Stat` fallback only triggers when a simple relative path does NOT exist at the git root
but DOES exist relative to CWD — precisely the `ATMOS_BASE_PATH=.terraform/modules/monorepo`
scenario.

### Integration Test Coverage

The following existing integration tests verify "run from any directory" behavior:

- `describe_component_from_nested_dir_discovers_atmos.yaml_in_parent` — runs `describe component`
  from `tests/fixtures/scenarios/complete/components/terraform/weather/` ✅
- `terraform_plan_from_nested_dir_discovers_atmos.yaml_in_parent` — runs `terraform plan` from
  a nested component directory ✅
- `terraform_plan_with_current_directory_(.)` — runs with `base_path: .` ✅

---

## Backward Compatibility

- **Dot-prefixed paths from runtime sources** (`ATMOS_BASE_PATH="."`, `--base-path=./foo`): now
  resolve relative to CWD (shell convention). Previously resolved relative to config dir. This is
  a semantic change but matches user expectations — in a shell, `.` means "here" (CWD)
- **Dot-prefixed paths from config file** (`base_path: "."` in `atmos.yaml`): continue to resolve
  relative to the directory containing `atmos.yaml` — no change
- **Bare paths** (`"stacks"`, `".terraform/modules/monorepo"`): go through git root search
  regardless of source, with `os.Stat` fallback to CWD — source-independent
- **Default/empty `base_path`**: continues to use git root discovery (the v1.202.0 behavior)
- **Error messages**: more informative with actionable hints and error builder pattern — no
  behavioral change, just better diagnostics
- **`ATMOS_GIT_ROOT_BASEPATH=false`**: continues to work as a full opt-out of git root discovery

---

## Test Plan

### Unit Tests (Implemented)

- `TestInitCliConfig_ExplicitBasePath_DotSlash_ResolvesRelativeToCWD` — verify struct field base
  path with dot-slash resolves to CWD (Tyler's scenario)
- `TestInitCliConfig_ExplicitBasePath_AbsolutePassedThrough` — verify absolute path is unchanged
- `TestInitCliConfig_EmptyBasePath_DefaultsToAbsolute` — verify empty base path uses default
  resolution
- `TestInitCliConfig_EnvVarBasePath_DotSlash_ResolvesRelativeToCWD` — verify `ATMOS_BASE_PATH`
  env var with dot-slash resolves to CWD
- `TestInitCliConfig_EnvVarBasePath_Dot_ResolvesToCWD` — verify `ATMOS_BASE_PATH="."` resolves
  to CWD (not config dir) in shell context
- `TestInitCliConfig_BasePathSource_SetForStructField` — verify `BasePathSource` is `"runtime"`
  when `AtmosBasePath` is set via struct field
- `TestInitCliConfig_BasePathSource_SetForEnvVar` — verify `BasePathSource` is `"runtime"` when
  `ATMOS_BASE_PATH` env var is set
- `TestResolveAbsolutePath_DotPrefix_SourceAware` — table-driven test for source-dependent
  dot-prefix resolution (config → config dir, runtime → CWD)
- `TestResolveAbsolutePath_BarePath_SourceIndependent` — verify bare paths resolve identically
  regardless of source
- `TestResolveAbsolutePath_AbsolutePassThrough` — verify absolute paths pass through unchanged
- `TestResolveAbsolutePath_BarePathNoGitRoot` — verify bare path fallback to config dir then CWD
- `TestResolveDotPrefixPath_NoConfigPath_FallsToCWD` — verify dot-prefix fallback to CWD
- `TestResolveDotPrefixPath_DotDotSlash_Runtime` — verify `../c` from runtime resolves to CWD
- `TestTryResolveWithConfigPath_AllBranches` — all branches of config path fallback
- `TestTryResolveWithGitRoot_ExistingPathAtGitRoot` — verify git root discovery works normally
- `TestTryResolveWithGitRoot_CWDFallback` — verify `os.Stat` fallback to CWD
- `TestTryResolveWithGitRoot_NeitherExists` — verify git-root-joined path returned as default
- `TestFindAllStackConfigsInPathsForStack_ErrorWrapping` — verify error wraps `ErrFailedToFindImport`
- `TestFindAllStackConfigsInPaths_ErrorWrapping` — verify error wrapping in non-stack variant

### Integration Tests (Verified Passing)

- `describe_component_from_nested_dir_discovers_atmos.yaml_in_parent` — subdirectory execution ✅
- `terraform_plan_from_nested_dir_discovers_atmos.yaml_in_parent` — nested dir plan ✅
- `terraform_plan_with_current_directory_(.)` — base_path=`.` ✅
- Verify `describe affected` works with default `base_path: ""`
- Verify `describe affected` works with explicit `base_path: "."`
- Verify terraform-provider-utils pattern with relative `ATMOS_BASE_PATH` env var

### Manual Tests

- Test with `ATMOS_GIT_ROOT_BASEPATH=false` to verify opt-out still works
