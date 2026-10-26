# `failed to find import` Persists When Git Root Is Unavailable

**Date:** 2026-03-19

**Related Issues:**
- `terraform-provider-utils` with `ATMOS_BASE_PATH` set to a relative path still fails with
  `failed to find import` even after the v1.210.1 base path resolution fix
- The error occurs on CI workers (e.g., Spacelift) where `.git` directory may not be present
- Absolute paths work; only relative paths fail
- Provider v1.31.0 (Atmos v1.189.0) works; provider v2.3.0 (Atmos v1.210.1) fails

**Previous Fix:** [2026-03-17-failed-to-find-import-base-path-resolution.md](./2026-03-17-failed-to-find-import-base-path-resolution.md)

**Affected Atmos Versions:** v1.202.0+ (including v1.210.1 which attempted to fix this)

**Severity:** High — blocks `terraform plan` on CI workers that don't have `.git` directories

---

## Issue Description

After releasing Atmos v1.210.1 and `terraform-provider-utils` v2.3.0 (which embedded the base
path resolution fix from PR #2215), the same user reported the error persists:

```text
╷
│ Error: failed to find import
│
│   with module.account_map.data.utils_component_config.config[0],
│   on .terraform/modules/account_map/modules/remote-state/main.tf line 1
╵
```

**Key observation:** Setting `ATMOS_BASE_PATH` to an absolute path works. Only relative paths
fail. This confirms the resolution logic is the problem, not the stacks or configuration.

### Environment

- CI worker (Spacelift) — likely no `.git` directory
- `ATMOS_BASE_PATH=.terraform/modules/monorepo` (relative path from CWD)
- CWD is the component directory (e.g., `/workspace/components/terraform/iam-delegated-roles/`)
- The monorepo is cloned inside `.terraform/modules/monorepo` relative to the component directory
- `atmos.yaml` is found by walking up from CWD (e.g., at `/workspace/atmos.yaml`)

### Two Distinct Issues Reported

The user reported two separate issues in the same conversation:

**Issue 1: Stack overflow (`fatal error: stack overflow`)**

Abstract components with `metadata.component` set cause infinite recursion in
`processBaseComponentConfigInternal`. Example:

```yaml
components:
  terraform:
    iam-delegated-roles-defaults:
      metadata:
        component: iam-delegated-roles
        type: abstract    # <-- abstract with metadata.component causes recursion

    iam-delegated-roles:
      metadata:
        component: iam-delegated-roles
        type: real
        inherits:
          - iam-delegated-roles-defaults
```

This was partially addressed in v1.210.0 (PR #2214), but the user still saw it on v1.210.0.
Workaround: remove `metadata.component` from abstract component definitions.

**Issue 2: `failed to find import` with relative `ATMOS_BASE_PATH` (this doc)**

The provider's `data.utils_component_config` fails when `ATMOS_BASE_PATH` is a relative path.
This persists even after v1.210.1 / provider v2.3.0.

---

## Root Cause Analysis

### Why v1.210.1 Fix Doesn't Work on CI Without `.git`

The v1.210.1 fix (PR #2215) added an `os.Stat` fallback to `tryResolveWithGitRoot`: if the
git-root-joined path doesn't exist, try CWD-relative. This works when git root IS available.

However, `tryResolveWithGitRoot` has a critical early exit at `getGitRootOrEmpty`:

```go
func tryResolveWithGitRoot(path string, cliConfigPath string) (string, error) {
    gitRoot := getGitRootOrEmpty()
    if gitRoot == "" {
        return tryResolveWithConfigPath(path, cliConfigPath)  // <-- FALLS HERE
    }
    // ... os.Stat fallback logic (never reached when gitRoot == "") ...
}
```

When `getGitRootOrEmpty()` returns `""` (no `.git` directory found), the function falls through
to `tryResolveWithConfigPath`, which **lacks the `os.Stat` + CWD fallback**:

```go
func tryResolveWithConfigPath(path string, cliConfigPath string) (string, error) {
    if cliConfigPath != "" {
        // Unconditionally joins with config dir — NO os.Stat check
        return absPathOrError(filepath.Join(cliConfigPath, path), ...)
    }
    return absPathOrError(path, ...)
}
```

### The Broken Code Path (Step by Step)

On a CI worker without `.git`:

1. User sets `ATMOS_BASE_PATH=.terraform/modules/monorepo`
2. `processEnvVars` sets `BasePath = ".terraform/modules/monorepo"`, `BasePathSource = "runtime"`
3. `resolveAbsolutePath(".terraform/modules/monorepo", "/workspace", "runtime")` is called
4. `.terraform/modules/monorepo` is a bare path (not dot-prefixed) → goes to `tryResolveWithGitRoot`
5. `getGitRootOrEmpty()` returns `""` — **no `.git` on Spacelift**
6. Falls to `tryResolveWithConfigPath(".terraform/modules/monorepo", "/workspace")`
7. `cliConfigPath == "/workspace"` (where `atmos.yaml` was found walking up from CWD)
8. Returns `filepath.Join("/workspace", ".terraform/modules/monorepo")` =
   `/workspace/.terraform/modules/monorepo` — **WRONG, does not exist**
9. Correct path: `/workspace/components/terraform/iam-delegated-roles/.terraform/modules/monorepo`

### Why Absolute Path Works

When `ATMOS_BASE_PATH` is set to an absolute path (e.g.,
`/workspace/components/terraform/iam-delegated-roles/.terraform/modules/monorepo`),
`resolveAbsolutePath` returns it as-is at the very first check (`filepath.IsAbs(path)`),
bypassing all resolution logic.

### Why Provider v1.31.0 Works

Provider v1.31.0 used Atmos v1.189.0, which predates the v1.202.0 git root discovery change.
In v1.189.0, all relative paths resolved from CWD — no git root search, no config-dir fallback.

### The Asymmetry Between `tryResolveWithGitRoot` and `tryResolveWithConfigPath`

| Function                   | Has `os.Stat` validation? | Has CWD fallback? | Used when          |
|----------------------------|---------------------------|-------------------|--------------------|
| `tryResolveWithGitRoot`    | Yes (v1.210.1)            | Yes (v1.210.1)    | Git root available |
| `tryResolveWithConfigPath` | **No**                    | **No**            | No git root        |

The v1.210.1 fix only added `os.Stat` + CWD fallback to `tryResolveWithGitRoot`. The
`tryResolveWithConfigPath` function was not updated. On CI workers without `.git`, the code
takes the `tryResolveWithConfigPath` path and the fix is bypassed entirely.

---

## Fix (Implemented — Option B)

### Approach: Pass `source` to `tryResolveWithGitRoot` and `tryResolveWithConfigPath`

Both functions are now source-aware. Runtime paths prefer CWD; config paths prefer config dir.
Both have `os.Stat` validation with CWD fallback.

### Changes to `resolveAbsolutePath`

Pass `source` through to `tryResolveWithGitRoot`:

```go
return tryResolveWithGitRoot(path, cliConfigPath, source)
```

### Changes to `tryResolveWithGitRoot`

Accept and forward `source` to `tryResolveWithConfigPath`:

```go
func tryResolveWithGitRoot(path, cliConfigPath, source string) (string, error) {
    gitRoot := getGitRootOrEmpty()
    if gitRoot == "" {
        return tryResolveWithConfigPath(path, cliConfigPath, source)
    }
    // ... existing os.Stat + CWD fallback logic (unchanged) ...
}
```

### Changes to `tryResolveWithConfigPath`

Accept `source` and add `os.Stat` validation with source-aware resolution order:

```go
func tryResolveWithConfigPath(path, cliConfigPath, source string) (string, error) {
    // For runtime sources, try CWD first (user expectation on CI).
    if source == "runtime" && path != "" {
        cwdJoined, err := absPathOrError(path, ...)
        if err == nil {
            if _, statErr := os.Stat(cwdJoined); statErr == nil {
                return cwdJoined, nil
            }
        }
    }

    // Try config-dir-relative.
    if cliConfigPath != "" {
        if path == "" {
            return absPathOrError(cliConfigPath, ...)
        }

        configJoined, err := absPathOrError(filepath.Join(cliConfigPath, path), ...)
        if _, statErr := os.Stat(configJoined); statErr == nil {
            return configJoined, nil
        }

        // Config-dir path doesn't exist — try CWD (if not already tried for runtime).
        if source != "runtime" {
            cwdJoined, _ := absPathOrError(path, ...)
            if _, statErr := os.Stat(cwdJoined); statErr == nil {
                return cwdJoined, nil
            }
        }

        // Neither exists — return config-dir path for consistent error messages.
        return configJoined, nil
    }

    // No config path: resolve relative to CWD.
    return absPathOrError(path, ...)
}
```

### Resolution Order Summary

| Source    | Git Root Available                        | Git Root Unavailable                      |
|-----------|-------------------------------------------|-------------------------------------------|
| `runtime` | git root → CWD (existing)                | **CWD → config dir** (new)                |
| config    | git root → CWD (existing)                | **config dir → CWD** (new)                |

### Why This Is Safe

- `os.Stat` validation is strictly additive — if the first-priority path exists, behavior is
  identical to before
- CWD fallback only activates when the first-priority path doesn't exist
- Empty paths return immediately without hitting `os.Stat` logic
- Config-source paths still prefer config dir (unchanged for normal Atmos CLI usage)

### Files Modified

| File | Change |
|---|---|
| `pkg/config/config.go` | `tryResolveWithGitRoot` and `tryResolveWithConfigPath` accept `source` parameter; `tryResolveWithConfigPath` adds `os.Stat` + CWD fallback with source-aware ordering |
| `pkg/config/base_path_resolution_test.go` | 5 new tests reproducing the CI scenario; updated existing call sites for new signature |
| `pkg/config/config_test.go` | Updated existing call sites for new signature |

---

## Verification

### How to Reproduce

1. Set up a directory structure without `.git`:
   ```text
   /workspace/
   ├── atmos.yaml                # stacks.base_path: "stacks"
   ├── stacks/
   └── components/terraform/vpc/
       └── .terraform/modules/monorepo/
           ├── atmos.yaml
           ├── stacks/
           └── components/terraform/
   ```
2. `cd /workspace/components/terraform/vpc/`
3. `ATMOS_BASE_PATH=.terraform/modules/monorepo atmos describe stacks -s dev`
4. Expected: stacks resolved from `.terraform/modules/monorepo/stacks/`
5. Actual: `failed to find import` (looks at `/workspace/.terraform/modules/monorepo/stacks/`)

### How to Verify the Fix

1. Same setup as above
2. After fix, `tryResolveWithConfigPath` with `source="runtime"` should:
   - Try CWD-relative first: `/workspace/components/terraform/vpc/.terraform/modules/monorepo` → exists → returned
3. Stacks resolve correctly

### Test Cases (Implemented)

| Test                                                           | What It Verifies                                                                       | Status |
|----------------------------------------------------------------|----------------------------------------------------------------------------------------|--------|
| `TestTryResolveWithConfigPath_CWDFallback_BarePathExistsAtCWD` | Runtime source: config-dir path doesn't exist, CWD path does → returns CWD path        | PASS   |
| `TestTryResolveWithConfigPath_ConfigDirPathExists`             | Config-dir path exists → returns it (unchanged behavior)                               | PASS   |
| `TestTryResolveWithConfigPath_NeitherExists`                   | Neither exists → returns config-dir path (consistent errors)                           | PASS   |
| `TestResolveAbsolutePath_BarePathNoGitRoot_CWDFallback`        | End-to-end: no git root, bare runtime path, CWD path exists → resolved                 | PASS   |
| `TestInitCliConfig_BareBasePath_NoGitRoot_CWDFallback`         | Full integration: `ATMOS_BASE_PATH` env var, no `.git`, CI layout → correct resolution | PASS   |

### Test Results

All 47 base path resolution tests pass (42 existing + 5 new), zero regressions:

```text
--- PASS: TestTryResolveWithConfigPath_CWDFallback_BarePathExistsAtCWD (0.00s)
--- PASS: TestTryResolveWithConfigPath_ConfigDirPathExists (0.00s)
--- PASS: TestTryResolveWithConfigPath_NeitherExists (0.00s)
--- PASS: TestResolveAbsolutePath_BarePathNoGitRoot_CWDFallback (0.00s)
--- PASS: TestInitCliConfig_BareBasePath_NoGitRoot_CWDFallback (0.00s)
```

Before the fix, the 3 core bug reproduction tests failed:

```text
--- FAIL: TestTryResolveWithConfigPath_CWDFallback_BarePathExistsAtCWD
    expected: .../workspace/components/terraform/iam-delegated-roles/.terraform/modules/monorepo
    actual:   .../workspace/.terraform/modules/monorepo

--- FAIL: TestResolveAbsolutePath_BarePathNoGitRoot_CWDFallback
    expected: .../workspace/components/terraform/vpc/.terraform/modules/monorepo
    actual:   .../workspace/.terraform/modules/monorepo

--- FAIL: TestInitCliConfig_BareBasePath_NoGitRoot_CWDFallback
    expected: .../workspace/components/terraform/iam-delegated-roles/.terraform/modules/monorepo
    actual:   .../workspace/.terraform/modules/monorepo
```

---

## Related Issue: Stack Overflow with `metadata.component` on Abstract Components

The same user also reported `fatal error: stack overflow` on Atmos versions > v1.200.0. This
is a **separate issue** from `failed to find import`, documented in detail in
[2026-03-16-metadata-component-abstract-stack-overflow.md](./2026-03-16-metadata-component-abstract-stack-overflow.md).

### Summary

Abstract components with `metadata.component` pointing to a real component that inherits from
them cause infinite recursion in `processBaseComponentConfigInternal`. The cycle occurs because
Phase 2 metadata inheritance (PR #1812) re-processes already-processed component maps where
`mergeComponentConfigurations` has added a top-level `"component"` key, creating a circular
reference:

```text
abstract (iam-delegated-roles-defaults)
  → metadata.component: iam-delegated-roles (real)
    → inherits: iam-delegated-roles-defaults (abstract)
      → metadata.component: iam-delegated-roles (real)
        → ... infinite recursion
```

### Fix (PR #2214, v1.210.0)

Two fixes were implemented:
1. **Cycle detection via visited-set** — tracks `(component, baseComponent)` pairs during
   recursion. If a pair is encountered again, returns `ErrCircularComponentInheritance` instead
   of recursing infinitely.
2. **Skip `metadata.component` on abstract components** — when processing the
   `metadata.component` reference, skips component chain resolution if the base component is
   `type: abstract`, since abstract components can't be deployed.

### User Status

The user reported that v1.210.0 did not fully resolve the stack overflow for their specific
configuration. Their workaround was to remove `metadata.component` from abstract component
definitions.

### Verification of Cycle Detection

Additional tests were written to verify the cycle detection works for various patterns:

| Test                                                                   | Pattern                                                       | Result                |
|------------------------------------------------------------------------|---------------------------------------------------------------|-----------------------|
| `TestProcessBaseComponentConfig_MultipleAbstractComponentsCycle`       | Two abstract/real pairs (iam-delegated-roles + eks)           | PASS                  |
| `TestProcessBaseComponentConfig_AbstractWithInheritsCycle`             | Abstract inherits from its own real counterpart               | PASS (cycle detected) |
| `TestProcessBaseComponentConfig_RealComponentSelfReferenceViaAbstract` | Real → abstract → inherits real (cross-cycle)                 | PASS (cycle detected) |
| `TestProcessBaseComponentConfig_DeferDeleteCycleReentry`               | Non-abstract shared-base creating chain back through abstract | PASS (cycle detected) |

The cycle detection and `isAbstract` skip are working correctly for all reproducible patterns.
The user's persistent stack overflow likely involves a configuration pattern we cannot reproduce
without their actual stacks (e.g., multi-file import chains, complex cross-component
dependencies, or interaction with `!terraform.state` YAML functions in inherited definitions).

**Recommendation:** Request the user's stacks to identify the exact cycle pattern not covered
by the current fix.

---

## Timeline

| Date       | Event                                                                                |
|------------|--------------------------------------------------------------------------------------|
| 2026-03-16 | User reports stack overflow + `failed to find import` on Atmos > v1.200.0            |
| 2026-03-16 | Identified: `metadata.component` on abstract components causes stack overflow        |
| 2026-03-16 | Identified: `ATMOS_BASE_PATH` relative path causes `failed to find import`           |
| 2026-03-17 | Released Atmos v1.210.0 (PR #2214) — partial fix for stack overflow                  |
| 2026-03-18 | Released Atmos v1.210.1 (PR #2215) — base path resolution fix                        |
| 2026-03-18 | Released `terraform-provider-utils` v2.3.0 with Atmos v1.210.1                       |
| 2026-03-19 | User confirms: stack overflow workaround works, but `failed to find import` persists |
| 2026-03-19 | User confirms: absolute `ATMOS_BASE_PATH` works, relative fails                      |
| 2026-03-19 | Root cause: `tryResolveWithConfigPath` lacks `os.Stat` + CWD fallback                |

---

## References

- Previous fix doc: [2026-03-17-failed-to-find-import-base-path-resolution.md](./2026-03-17-failed-to-find-import-base-path-resolution.md)
- Stack overflow fix doc: [2026-03-16-metadata-component-abstract-stack-overflow.md](./2026-03-16-metadata-component-abstract-stack-overflow.md)
- Atmos PR #2215: base path resolution fix (v1.210.1)
- Atmos PR #2214: stack overflow fix for `metadata.component` (v1.210.0)
- `tryResolveWithGitRoot` — `pkg/config/config.go` (has `os.Stat` fallback)
- `tryResolveWithConfigPath` — `pkg/config/config.go` (previously missing `os.Stat` fallback before this fix)
- `getGitRootOrEmpty` — `pkg/config/config.go` (returns `""` when no `.git` found)
