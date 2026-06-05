# Fix `!include` Fails in `atmos describe affected` When Processing Base-Ref Stacks

**Date:** 2026-02-22

**Related Issue:** [GitHub Issue #2090](https://github.com/cloudposse/atmos/issues/2090) — `atmos describe affected`
fails
on `!include` for one file and not the other.

**Affected Atmos Version:** v1.155.0+ (since `!include` with extension-based parsing was introduced)

**Severity:** Medium — `atmos describe affected` fails when processing stack manifests from the base-ref repository
that contain `!include` directives with paths relative to the base path. This breaks CI/CD pipelines that use
`describe affected` to determine which stacks need to be applied.

## Background

The `atmos describe affected` command compares the current branch's stacks against a base reference (typically `main`)
to determine which components are affected by changes. It does this by:

1. Processing the current repo's stacks via `ExecuteDescribeStacks()`
2. Temporarily updating `atmosConfig` paths to point to the base-ref checkout directory
3. Processing the base-ref's stacks via `ExecuteDescribeStacks()`
4. Restoring the original `atmosConfig` paths
5. Comparing the two sets of stacks to find differences

When stack manifests use `!include` to reference files (e.g., `.rego` policy files), the include function resolves
file paths using two strategies:

1. **Relative to the manifest file** — for paths starting with `./` or `../`
2. **Relative to `atmosConfig.BasePath`** — for all other relative paths (most common)

## Symptoms

```bash
$ atmos describe affected --repo-path /path/to/base-ref
```

```text
# Error
invalid stack manifest: the !include function references a file that does not exist:
could not find local file 'stacks/catalog/spacelift/spaces/policies/push-prioritize-non-bot-runs.rego'
(tried relative to manifest '/runner/_work/my-infra/my-infra/base-ref/stacks/catalog/spacelift/spaces/local.yaml'
and base path '')
```

Key indicator: **`base path ''`** — the base path is empty, meaning the file resolution cannot
use the base-path strategy.

The user's stack manifest references two files using `!include`:

```yaml
components:
  terraform:
    spaces:
      vars:
        spaces:
          root:
            policies:
              "NOTIFICATION Failure":
                body: !include stacks/catalog/spacelift/spaces/policies/notification-failure.rego
              "GIT_PUSH Prioritize Non-Bot Runs":
                body: !include stacks/catalog/spacelift/spaces/policies/push-prioritize-non-bot-runs.rego
```

Both files use the same path pattern (relative to repo root / base path), but the second one fails because
it doesn't exist in the base-ref checkout (it was added in the current PR).

## Root Cause

The bug is in `internal/exec/describe_affected_utils.go` in the `executeDescribeAffected()` function.
When switching context to process the base-ref repository's stacks, the function saves and restores several
`atmosConfig` path fields — but **misses `BasePath` and `BasePathAbsolute`**.

### What IS Saved/Restored (lines 70-75, 150-155)

```go
// Save current paths before modification.
currentStacksBaseAbsolutePath := atmosConfig.StacksBaseAbsolutePath
currentStacksTerraformDirAbsolutePath := atmosConfig.TerraformDirAbsolutePath
currentStacksHelmfileDirAbsolutePath := atmosConfig.HelmfileDirAbsolutePath
currentStacksPackerDirAbsolutePath := atmosConfig.PackerDirAbsolutePath
currentStacksStackConfigFilesAbsolutePaths := atmosConfig.StackConfigFilesAbsolutePaths
```

### What is MISSING

```go
// NOT saved/restored — causes the bug:
// atmosConfig.BasePath
// atmosConfig.BasePathAbsolute
```

### How the Bug Manifests

1. `BasePath` is set during initial config loading (e.g., `"./"` or the repo root)
2. `describe affected` updates `StacksBaseAbsolutePath` etc. to point to the remote repo
3. `BasePath` is NOT updated — it still references the original repo (or effectively becomes empty)
4. When `ExecuteDescribeStacks()` processes the base-ref stacks, YAML functions fire
5. `ProcessIncludeTag()` calls `findLocalFile()` which uses `atmosConfig.BasePath` to resolve includes
6. Since `BasePath` doesn't point to the base-ref, files included via base-path-relative paths fail

### The `findLocalFile` Resolution Chain

```go
func findLocalFile(includeFile, manifestFile string, atmosConfig *schema.AtmosConfiguration) string {
// 1. Try relative to the manifest file (only for ./ and ../ prefixed paths)
resolved := ResolveRelativePath(includeFile, manifestFile)
if absPath := resolveAbsolutePath(resolved); absPath != "" {
return absPath
}

// 2. Try relative to the base_path from atmos.yaml — THIS FAILS when BasePath is empty
atmosManifestPath := filepath.Join(atmosConfig.BasePath, includeFile)
return resolveAbsolutePath(atmosManifestPath)
}
```

For paths like `stacks/catalog/.../policy.rego` (not starting with `./`):

- Step 1 returns the path as-is (not relative to manifest) → resolves against CWD → may or may not exist
- Step 2 joins with empty `BasePath` → same as CWD resolution → may or may not exist

### Why One File Works and the Other Doesn't

This is NOT a bug with multiple `!include` tags. The first `!include` succeeds because
`notification-failure.rego` already existed on the main branch (so it exists in the CWD of the current
checkout). The second `!include` fails because `push-prioritize-non-bot-runs.rego` was added in the PR
and may not exist in the CWD when `describe affected` resolves paths during base-ref processing.

## Fix

### Approach

Two fixes applied:

1. **`describe_affected_utils.go`**: Add `BasePath` and `BasePathAbsolute` to the save/restore block
   in `executeDescribeAffected()`, and update them to point to the remote repo directory before
   processing base-ref stacks.

2. **`yaml_include_by_extension.go`**: Change `findLocalFile` to prefer `BasePathAbsolute` over
   `BasePath` for file resolution. `BasePath` can be a relative path (e.g., `"./"` from `atmos.yaml`)
   which resolves incorrectly when the CWD differs from the repo root. `BasePathAbsolute` is always
   the resolved absolute path.

### Files Changed

| File | Change |
|------|--------|
| `internal/exec/describe_affected_utils.go` | Save/restore `BasePath` and `BasePathAbsolute`; update them for remote repo processing |
| `pkg/utils/yaml_include_by_extension.go` | Use `BasePathAbsolute` (with fallback to `BasePath`) in `findLocalFile` and error messages |

### Tests

**Unit tests** (`pkg/utils/yaml_include_by_extension_test.go`):

- `TestFindLocalFileWithEmptyBasePath` — 3 subtests directly testing `findLocalFile`:
  - With correct `BasePath` → files are found
  - With empty `BasePath` → files are NOT found (reproduces the bug)
  - With wrong `BasePath` → files are NOT found
- `TestIncludeMultipleInSameFile` — Verifies multiple `!include` tags in one file all resolve
  correctly (rules out state corruption between consecutive include resolutions)
- `TestIncludeBasePathResolution` — 2 subtests testing full YAML parsing pipeline:
  - With `BasePath` set to repo root → both includes succeed
  - With empty `BasePath` and different CWD → includes fail (reproduces issue #2090)

**Integration tests** (`tests/describe_affected_include_test.go`):

- `TestDescribeAffectedWithInclude` — 2 subtests using real `ExecuteDescribeAffectedWithTargetRepoPath`:
  - Resolves `!include` in both HEAD and BASE stacks
  - Validates all components report correct affected reason (`stack.vars`)
- `TestDescribeAffectedWithIncludeSelfComparison` — Self-comparison produces empty affected list
- `TestDescribeAffectedWithIncludeComponentsLoadCorrectly` — All 3 components load without errors
- `TestDescribeAffectedWithIncludeVerifyIncludedValues` — 2 subtests verifying:
  - `app-with-includes` has correct parsed JSON, YQ-extracted values, raw `.rego` policy, and YAML settings
  - `app-with-raw-includes` has correct raw string values for JSON and `.rego` files

**Fixture** (`tests/fixtures/scenarios/atmos-describe-affected-with-include/`):

- `atmos.yaml` — Standard config with `base_path: "./"`, `name_pattern: "{stage}"`
- `config/vars.json` — JSON data for `!include` testing
- `config/policy.rego` — OPA policy file (raw string by extension)
- `config/settings.yaml` — YAML data for `!include` testing
- `stacks/deploy/nonprod.yaml` — HEAD stack with `!include`, `!include.raw`, YQ expressions
- `stacks-affected/deploy/nonprod.yaml` — BASE stack (same includes, different `environment` var)

Run with:

```bash
go test ./pkg/utils/ -run TestFindLocalFileWithEmptyBasePath -v
go test ./pkg/utils/ -run TestIncludeMultipleInSameFile -v
go test ./pkg/utils/ -run TestIncludeBasePathResolution -v
go test ./tests/ -run TestDescribeAffectedWithInclude -v
```

### Documentation Updated

Updated `website/docs/functions/yaml/include.mdx` and `website/docs/functions/yaml/include.raw.mdx` to make the
path resolution behavior exceptionally clear:

- **Numbered resolution order** — Documents the exact order Atmos tries when resolving a relative path:
  1. **Absolute paths** — used as-is
  2. **Manifest-relative paths** — paths starting with `./` or `../` resolve relative to the directory of the
     manifest file containing the `!include`
  3. **`base_path`-relative paths** — all other relative paths resolve relative to the `base_path` setting in
     `atmos.yaml` (most common pattern)

- **First-match-wins behavior** — Explains that Atmos tries each strategy in order and uses the first match that
  points to an existing file

- **Best practice callout** — A `<Note>` advising users to use `./` or `../` prefixes for explicit manifest-relative
  resolution, and bare paths (no prefix) for `base_path`-relative resolution

- **`!include.raw` cross-reference** — Updated to link back to `!include` docs for full path resolution details,
  with a concise summary of the three path types

This documentation clarifies the behavior that led to the original bug report — users were unaware that bare paths
(like `stacks/catalog/.../policy.rego`) resolve relative to `base_path`, and the error message showing
`base path ''` was confusing because it wasn't clear what `base_path` referred to or why it was empty.
