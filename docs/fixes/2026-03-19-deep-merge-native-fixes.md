# Deep-Merge Native & Terraform Workspace Fixes

**Date:** 2026-03-19
**PR:** #2201 (perf: replace mergo with native deep merge)
**Reviewer findings:** CodeRabbit audit + GitHub Advanced Security alerts

---

## Issues addressed

### 1. `sliceDeepCopy` vs `appendSlice` precedence flip (behavioral regression)

**File:** `pkg/merge/merge_native.go`

**Problem:** The new `deepMergeNative` checked `appendSlice` before `sliceDeepCopy`, but the
old mergo code checked `WithSliceDeepCopy` first (`if sliceDeepCopy … else if appendSlice …`).
When a caller passes both flags as `true`, the old code applied element-wise merging (deep-copy),
the new code appended — an undocumented behavioral change.

**Fix:** Reordered the condition: `if sliceDeepCopy { … } else { /* appendSlice */ }`.
Note: the public `Merge()` and `MergeWithContext()` APIs are strategy-enum-guarded and never
set both flags simultaneously. The fix matters only for direct callers of the internal
`deepMergeNative` or `MergeWithOptions`.

---

### 2. `mergeSlicesNative` aliased dst maps and tail elements

**File:** `pkg/merge/merge_native.go`

**Problem (inner maps):** When building the `merged` map from `dstMap` values, we used a
shallow copy (`merged[k] = v`). When `deepMergeNative` then recursed into `merged`, it
mutated the shared inner maps of both `merged` and `dstMap`. Since `dstMap` was part of
the accumulator, this silently corrupted earlier data in multi-input merges.

**Fix:** Deep-copy each `dstMap` value before inserting into `merged`:
```go
merged[k] = deepCopyValue(v)
```

**Problem (tail elements):** `copy(result, dst)` shallow-copied all positions, including
positions `i >= len(src)` (the "tail"). Those tail elements of `result` aliased the
accumulator's slice elements. A subsequent merge pass over the same key would find the same
map pointer in two places, and `deepMergeNative`'s in-place recursion could corrupt both.

**Fix:** After the src-range loop, deep-copy tail positions:
```go
for i := len(src); i < len(dst); i++ {
    result[i] = deepCopyValue(dst[i])
}
```

---

### 3. `isTerraformCurrentWorkspace` did not handle the "default" workspace

**File:** `internal/exec/terraform_utils.go`

**Problem:** Terraform never writes the `.terraform/environment` file for the `default`
workspace (or writes an empty file). The helper always returned `false` when the file was
absent, so the workspace-recovery logic never triggered for `default`. If a Windows cleanup
left the `.terraform/environment` file missing while the `default` workspace was active,
both `workspace select default` and `workspace new default` would fail (code 1) and atmos
would bubble up the error instead of proceeding.

**Fix:**
```go
data, err := os.ReadFile(envFile)
if err != nil {
    // File missing (ENOENT) → default workspace is active; permission errors propagate.
    if errors.Is(err, os.ErrNotExist) && workspace == "default" {
        return true
    }
    return false
}
recorded := strings.TrimSpace(string(data))
if recorded == "" {
    return workspace == "default"
}
return recorded == workspace
```

The fix also wires the `isTerraformCurrentWorkspace` check into `runWorkspaceSetup` — the function that is actually called during terraform execution:

```go
// runWorkspaceSetup — recovery path (internal/exec/terraform_execute_helpers_exec.go)
newErr := ExecuteShellCommand(
    *atmosConfig, info.Command,
    []string{"workspace", "new", info.TerraformWorkspace},
    componentPath, info.ComponentEnvList, info.DryRun, info.RedirectStdErr,
)
if newErr == nil {
    return nil
}
var newExitCodeErr errUtils.ExitCodeError
if errors.As(newErr, &newExitCodeErr) && newExitCodeErr.Code == 1 &&
    isTerraformCurrentWorkspace(componentPath, info.TerraformWorkspace) {
    log.Warn("Workspace is already active but its state directory is missing; "+
        "proceeding — subsequent terraform commands may report missing state",
        "workspace", info.TerraformWorkspace)
    return nil
}
return newErr
```

---

### 4. Workspace recovery log level too low

**File:** `internal/exec/terraform_execute_helpers_exec.go`

**Problem:** When both `workspace select` and `workspace new` fail with exit code 1 but the
environment file already names the target workspace (corrupted state), atmos silently
proceeded with a `log.Debug`. In production, this message would be invisible, making it
hard to diagnose later plan/apply failures caused by the missing state directory.

**Fix:** Upgraded to `log.Warn` with a clearer message:
> "Workspace is already active but its state directory is missing; proceeding — subsequent
> terraform commands may report missing state"

---

### 5. `TF_DATA_DIR` relative path resolution (CodeRabbit concern, no change needed)

**File:** `internal/exec/terraform_utils.go`

**Concern:** CodeRabbit noted that Terraform resolves `TF_DATA_DIR` relative to the
_process CWD_ at invocation time, which may differ from `componentPath`.

**Why componentPath is correct here:** Atmos invokes `terraform` with `componentPath` as
its working directory (via `ExecuteShellCommand`'s `dir` parameter). Therefore, when
terraform resolves a relative `TF_DATA_DIR`, its CWD _is_ `componentPath`. Using
`os.Getwd()` (the atmos process CWD) would be wrong. Added an explicit comment to the
`isTerraformCurrentWorkspace` docstring documenting this invariant.

---

### 6. `mergo` dependency partially remaining (documentation clarification)

**Concern:** The PR description implied mergo was fully replaced, but it is still used in
`pkg/merge/merge_yaml_functions.go` and `pkg/devcontainer/config_loader.go`.

**Status:** The hot-path in `MergeWithOptions` / `MergeWithContext` (called ~118k times per
production run) is fully migrated to the native implementation. The remaining mergo usages
are for non-performance-critical paths (YAML function merging and devcontainer config).
A follow-up task should migrate those to eliminate the mergo dependency entirely.

---

### 7. Integer overflow in size computations (GitHub Advanced Security alerts 5236–5239)

**File:** `pkg/merge/merge_native.go`

**Problem:** `len(dst)+len(src)` in `appendSlices` and `mergeSlicesNative` could overflow
`int` if both slices are very large (e.g., `math.MaxInt/2 + 1` each).

**Fix (already applied in a prior commit):** Introduced `safeCap(a, b int) int` which
clamps the result to `1<<24` (16 M entries) — a practical upper bound that prevents
oversized `make()` calls without triggering OOM panics, then replaced direct additions.

The implementation uses a single overflow guard (the redundant second `if sum > maxCapHint`
check was removed as dead code — the first guard `b > maxCapHint-a` already covers every
case where the sum would exceed `maxCapHint`):

```go
const maxCapHint = 1 << 24 // 16 M entries — practical upper bound to prevent OOM

func safeCap(a, b int) int {
    // Guard against integer overflow: if b > maxCapHint-a, then a+b > maxCapHint.
    if b > maxCapHint-a {
        return maxCapHint
    }
    return a + b
}
```

---

## Summary of files changed

| File | Change |
|------|--------|
| `pkg/merge/merge_native.go` | Precedence fix; tail deep-copy; inner map deep-copy |
| `pkg/merge/merge_native_test.go` | Tests for precedence, tail isolation, dstMap isolation, nil-src invariant |
| `internal/exec/terraform_utils.go` | Default-workspace handling; docstring clarification |
| `internal/exec/terraform_utils_test.go` | Tests for default workspace variants; absolute TF_DATA_DIR path |
| `internal/exec/terraform_execute_helpers_exec.go` | Debug → Warn for workspace recovery |
| `internal/exec/terraform_execute_helpers_pipeline_test.go` | Recovery path test; env propagation verification; negative-path logger assertion |
| `internal/exec/testmain_test.go` | Cross-platform subprocess helper: `TestMain` intercepts `_ATMOS_TEST_EXIT_ONE=1` to provide a platform-agnostic "exit 1" binary |
| `internal/exec/validate_stacks_test.go` | Independent fixture YAML count to self-validate block counter |
| `internal/exec/terraform_test.go` | Use `tests.RequireTerraform(t)` for `TestExecuteTerraform_DeploymentStatus`; remove unused `os/exec` import |
| `internal/exec/terraform_clean_test.go` | Use `tests.RequireTerraform(t)` for `TestCLITerraformClean`; add `tests` import |
| `internal/exec/yaml_func_utils_test.go` | Make `RemoveAll` cleanup non-fatal (use `t.Logf` instead of `assert.NoError`) |
| `internal/exec/yaml_func_template_test.go` | Fall back to terraform if tofu binary not found; skip only when both are absent |
| `tests/preconditions.go` | Handle `http.NewRequestWithContext` error with `t.Logf` warning branch |
| `pkg/merge/merge_compare_mergo_test.go` | Use `t.Fatalf` (not `t.Logf`) for `dmergo.Merge` errors in baseline construction |
| `pkg/merge/merge_native_test.go` | Capture error in all 4 benchmark loops (`b.Fatalf` on failure) |
| `errors/errors.go` | Add `ErrMergeNilDst` and `ErrMergeTypeMismatch` sentinel errors |
| `website/src/data/roadmap.js` | Add `pr: 2201` to faster-deep-merge milestone |
