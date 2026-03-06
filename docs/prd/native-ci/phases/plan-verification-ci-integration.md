# Plan Verification: CI-Integrated Stored vs Fresh Plan Comparison

> Related: [Plan Verification](../terraform-plugin/plan-verification.md) | [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Hooks Integration](../framework/hooks-integration.md) | [Planfile Download Path Resolution](./planfile-download-component-path-resolution.md)

## Problem Statement

The `--verify-plan` flag on `atmos terraform apply` exists but has no CI integration. In CI mode with `--ci`, the planfile is downloaded automatically via the `before.terraform.apply` hook, but verification (`VerifyPlanfile()`) runs later inside `terraformRunWithOptions()` — after the hook has already completed. This means:

1. **Verification requires `--from-plan` or `--planfile`** — but in CI mode the planfile is downloaded automatically and the user shouldn't need to specify it explicitly.
2. **The stored planfile is downloaded to the canonical path** (e.g., `components/terraform/vpc/vpc-prod-abc123.tfplan`), but verification needs a *separate* copy at a `stored.` prefix so terraform can generate a fresh plan at the canonical path and compare.
3. **No dedicated hook event** for verification — it's embedded inline in `terraformRunWithOptions()`.

### Current Apply Flow (CI mode)

```
PreRunE:  before.terraform.apply → downloadPlanfile() to canonical path
RunE:     terraformRunWithOptions()
            → [if --verify-plan && --from-plan] VerifyPlanfile()
            → executeSingleComponent() → terraform apply
PostRunE: after.terraform.apply → writeSummary(), writeOutputs()
```

### Desired Apply Flow (CI mode with verification)

```
PreRunE:  before.terraform.apply → downloadPlanfile() to stored.* prefix
RunE:     terraformRunWithOptions()
            → [if --verify-plan] terraform plan (generates fresh planfile at canonical path)
            → [if --verify-plan] compare stored.* vs fresh planfile via plan-diff
            → [if plans differ] FAIL with drift error
            → [if plans match] terraform apply (using fresh planfile)
PostRunE: after.terraform.apply → writeSummary(), writeOutputs()
```

## Desired State

### 1. Download to `stored.*` prefix when verification is enabled

When `--verify-plan` is active, the `downloadPlanfile()` handler writes files with a `stored.` prefix:
- `plan.tfplan` → `<componentDir>/stored.plan.tfplan`
- `.terraform.lock.hcl` → `<componentDir>/stored..terraform.lock.hcl`

This keeps the canonical planfile path free for the fresh plan that terraform will generate during verification.

**Implementation:** `downloadPlanfile()` in `handlers.go` checks `ctx.Info.VerifyPlan`. When true, it calls `WritePlanfileResults()` with a prefixed planfile path. The `stored.*` prefix is a constant defined in the planfile package.

### 2. Wire `--verify-plan` into `ConfigAndStacksInfo` from CI hook context

The `HookContext.Info` already carries `VerifyPlan bool` from `applyOptionsToInfo()` — but hooks run in `PreRunE`, before `applyOptionsToInfo()`. The fix: set `info.VerifyPlan` from the parsed flag in the hook execution path too.

**Implementation:** In `runHooksWithOutput()` (or the apply `PreRunE`), read `--verify-plan` from viper and set it on `info` before calling `RunCIHooks()`. This way, `downloadPlanfile()` knows whether verification is enabled.

### 3. Refactor `VerifyPlanfile()` to accept the stored planfile path

Current `VerifyPlanfile()` reads `info.PlanFile` as the stored plan. For CI integration, the stored plan is at the `stored.*` path, and the fresh plan should be generated at the canonical path (so apply can use it).

**Refactored signature:**
```go
func VerifyPlanfile(info *schema.ConfigAndStacksInfo, storedPlanFile string) error
```

The function:
1. Validates `storedPlanFile` exists on disk
2. Gets JSON of stored plan via `getTerraformPlanJSON()`
3. Generates a fresh plan at the canonical planfile path via `generateNewPlanFile()` — but output goes to the resolved component dir (not a temp dir)
4. Gets JSON of fresh plan
5. Compares via `generatePlanDiff()`
6. Returns `ErrPlanVerificationFailed` with diff if plans differ
7. Cleans up the `stored.*` file after comparison

### 4. Update `terraformRunWithOptions()` to handle CI verification flow

When `--verify-plan` is set:
1. If `stored.*` planfile exists in the component dir (set by CI download hook), use it as the stored plan
2. Generate fresh plan at the canonical path
3. Compare stored vs fresh
4. If match → proceed to `terraform apply` with the fresh planfile (set `info.PlanFile` and `info.UseTerraformPlan`)
5. If mismatch → return error (blocks apply)
6. Clean up `stored.*` file

When `--verify-plan` is set without CI mode (manual usage):
- Requires `--from-plan` or `--planfile` to specify the stored plan (existing behavior preserved)

### 5. Lock file handling

The downloaded `.terraform.lock.hcl` (at `stored..terraform.lock.hcl`) is not used for verification — it was downloaded for apply. After verification succeeds, the `stored.` lock file should be renamed/moved to the canonical `.terraform.lock.hcl` path so terraform apply uses the correct lock file.

Actually, simpler: download the lock file directly to `.terraform.lock.hcl` (no prefix). Only the planfile needs the `stored.*` prefix since that's what verification compares. The lock file is always the same — it describes provider constraints, not plan state.

## Files to Modify

| File | Changes |
|------|---------|
| `pkg/ci/plugins/terraform/planfile/constants.go` | **Create** — add `StoredPlanPrefix = "stored."` constant |
| `pkg/ci/plugins/terraform/handlers.go` | Modify `downloadPlanfile()` — when `ctx.Info.VerifyPlan`, use `stored.` prefix for planfile, no prefix for lock file |
| `pkg/ci/plugins/terraform/planfile/write.go` | Add `WritePlanfileResultsWithStoredPrefix()` or add a `storedPrefix` parameter option |
| `cmd/terraform/utils.go` | In `runHooksWithOutput()`, read `--verify-plan` from viper and set on `info.VerifyPlan` before `RunCIHooks()` |
| `cmd/terraform/utils.go` | In `terraformRunWithOptions()`, update verification block: detect `stored.*` planfile, call `VerifyPlanfile()` with it, set `info.PlanFile` to fresh plan on success |
| `internal/exec/terraform_verify_plan.go` | Refactor `VerifyPlanfile()` to accept `storedPlanFile` parameter. Generate fresh plan in component dir (not temp dir). Clean up stored file after comparison. |
| `internal/exec/terraform_verify_plan_test.go` | Update tests for new signature |
| `pkg/hooks/event.go` | No change needed — existing `before.terraform.apply` hook is sufficient |

## Edge Cases

### Verification without CI mode

When `--verify-plan` is used manually (no `--ci`, no CI hooks), the current behavior is preserved: `--from-plan` or `--planfile` specifies the stored plan. The `stored.*` prefix is only used when the CI hook downloads the planfile.

### No stored planfile found

If `--verify-plan` is set but no `stored.*` planfile exists and no `--from-plan`/`--planfile` is specified, the verification step is skipped with a warning. This handles the case where CI download failed silently or was disabled.

### Fresh plan shows no changes

If the fresh plan has no changes but the stored plan had changes, this is drift — verification fails. The comparison is structural, not "has changes vs no changes".

### Component with workdir (source vendoring)

`VerifyPlanfile()` already handles workdir via `ProcessStacks()` + `provWorkdir.WorkdirPathKey`. No additional changes needed — the stored plan path and fresh plan path both resolve relative to the workdir.

### Multi-component apply

`--verify-plan` is only supported for single-component execution. Multi-component (`--all`, `--affected`) skips verification (current behavior — the flag check is inside `terraformRunWithOptions()` after the multi-component branch).

## Verification

1. `go build ./...`
2. `go test ./internal/exec/... -run TestVerifyPlanfile` — updated tests pass
3. `go test ./pkg/ci/plugins/terraform/... -count=1` — handler tests pass
4. `go test ./cmd/terraform/... -count=1` — command tests pass
5. Manual: `atmos terraform apply vpc -s prod --ci --verify-plan` — downloads stored plan, generates fresh plan, compares, applies if matching
6. Manual: Modify infrastructure between plan and apply — verification detects drift and fails
