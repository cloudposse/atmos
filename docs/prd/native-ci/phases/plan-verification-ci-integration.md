# Plan Verification: Deploy-Based Stored vs Fresh Plan Comparison

> Related: [Plan Verification](../terraform-plugin/plan-verification.md) | [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Hooks Integration](../framework/hooks-integration.md) | [Planfile Download Path Resolution](./planfile-download-component-path-resolution.md)

## Problem Statement

Plan verification needs CI integration, but it belongs on the `deploy` command — not `apply`. The two commands serve different purposes:

- **`atmos terraform apply`** — Runs `terraform apply`. In CI mode, it writes summaries, checks, and outputs. It does NOT interact with planfile storage. It does NOT download or verify planfiles.
- **`atmos terraform deploy`** — The CI-native apply command. In CI mode, it downloads the stored planfile, generates a fresh plan, compares them, and applies only if they match.

### Why `deploy`, not `apply`?

1. **`apply` is a thin wrapper** around `terraform apply`. Adding planfile storage makes it do too much.
2. **`deploy` is the opinionated CI command** — it already adds `-auto-approve`, skips init by default, and is designed for automation.
3. **Separation of concerns** — `apply` stays simple and predictable; `deploy` handles the full CI workflow.

## Current State

### Apply Flow (CI mode)

```
PreRunE:  before.terraform.apply → (no planfile interaction)
RunE:     terraformRunWithOptions() → terraform apply
PostRunE: after.terraform.apply → writeSummary(), writeOutputs(), updateCheckRun()
```

Apply in CI mode only produces cosmetic CI outputs (summaries, checks, outputs). No planfile download, no verification.

### Deploy Flow (CI mode) — Current

```
PreRunE:  before.terraform.apply → downloadPlanfile() to canonical path
RunE:     terraformRunWithOptions() → terraform apply (using downloaded planfile)
PostRunE: after.terraform.apply → writeSummary(), writeOutputs()
```

## Desired State

### Deploy Flow (CI mode with verification)

```
PreRunE:  before.terraform.deploy → downloadPlanfile() to stored.* prefix
RunE:     terraformRunWithOptions()
            → terraform plan (generates fresh planfile — NO CI hooks, no upload, no checks, no summaries)
            → compare stored.* vs fresh planfile via plan-diff
            → [if plans differ] FAIL with drift error
            → [if plans match] terraform apply (using fresh planfile)
PostRunE: after.terraform.deploy → writeSummary(), writeOutputs(), updateCheckRun()
```

Key aspects:
1. **Download to `stored.*` prefix** — The stored planfile is downloaded with a `stored.` prefix so the canonical path is free for the fresh plan.
2. **Internal terraform plan** — The fresh plan runs without any CI side effects. No planfile upload, no status checks, no summaries. The only goal is to produce a planfile for comparison.
3. **Plan-diff comparison** — Structural comparison of stored vs fresh planfiles using the existing plan-diff infrastructure.
4. **Apply fresh planfile** — On match, deploy applies the *fresh* planfile (not the stored one). This ensures terraform applies exactly what was just planned.
5. **Error on mismatch** — If plans differ, deploy fails with a drift error showing the diff.

### Apply Flow (CI mode) — No Change

```
PreRunE:  before.terraform.apply → (no planfile interaction)
RunE:     terraformRunWithOptions() → terraform apply
PostRunE: after.terraform.apply → writeSummary(), writeOutputs(), updateCheckRun()
```

Apply does NOT have `--verify-plan`. Apply does NOT download planfiles. Apply only writes CI cosmetics.

## Implementation

### 1. Remove `--verify-plan` from `apply`

**File:** `cmd/terraform/apply.go`

Remove the `--verify-plan` flag registration from the apply command. Verification is deploy-only.

### 2. Add `--verify-plan` to `deploy` (if not already present)

**File:** `cmd/terraform/deploy.go`

Ensure `--verify-plan` flag is on deploy. Default: `true` in CI mode (always verify when deploying in CI).

### 3. Remove planfile download from `before.terraform.apply`

**File:** `pkg/ci/plugins/terraform/handlers.go`

The `onBeforeApply()` handler should NOT call `downloadPlanfile()`. Apply does not interact with planfile storage.

### 4. Add deploy hook events

**File:** `pkg/hooks/event.go`

Add new events:
```go
BeforeTerraformDeploy HookEvent = "before.terraform.deploy"
AfterTerraformDeploy  HookEvent = "after.terraform.deploy"
```

### 5. Wire deploy to its own hook events

**File:** `cmd/terraform/deploy.go`

Change deploy to fire `before.terraform.deploy` and `after.terraform.deploy` instead of `before.terraform.apply` and `after.terraform.apply`.

### 6. Add deploy hook bindings to terraform plugin

**File:** `pkg/ci/plugins/terraform/plugin.go`

Add bindings:
```go
{Event: "before.terraform.deploy", Handler: p.onBeforeDeploy},
{Event: "after.terraform.deploy",  Handler: p.onAfterDeploy},
```

### 7. Implement `onBeforeDeploy()` handler

**File:** `pkg/ci/plugins/terraform/handlers.go`

```go
func (p *Plugin) onBeforeDeploy(ctx *plugin.HookContext) error {
    // Download planfile to stored.* prefix for verification.
    return p.downloadPlanfileForVerification(ctx)
}
```

Downloads planfile from storage with `stored.` prefix. Sets `ctx.Info.StoredPlanFile` to the stored path.

### 8. Implement `onAfterDeploy()` handler

**File:** `pkg/ci/plugins/terraform/handlers.go`

```go
func (p *Plugin) onAfterDeploy(ctx *plugin.HookContext) error {
    result := p.parseOutputWithError(ctx)

    // Summary — warn-only
    if isSummaryEnabled(ctx.Config) { ... }

    // Output — warn-only
    if isOutputEnabled(ctx.Config) { ... }

    // Check — warn-only
    if isCheckEnabled(ctx.Config) { ... }

    return nil
}
```

Same as `onAfterApply()` — writes summaries, outputs, checks. No planfile upload (that happens on plan).

### 9. Deploy RunE: verification flow

**File:** `cmd/terraform/deploy.go` or `cmd/terraform/utils.go`

In deploy's `RunE`, when CI mode is active:

1. Check for `stored.*` planfile (downloaded by `onBeforeDeploy`)
2. Run `terraform plan -out=<canonical-path>` directly — NOT through the CI hook system. This is an internal plan whose only purpose is to generate a fresh planfile for comparison.
3. Compare stored vs fresh planfile using `VerifyPlanfile()` / plan-diff
4. If match → set `info.PlanFile` to fresh planfile, proceed to `terraform apply`
5. If mismatch → return error with diff

### 10. Download to `stored.*` prefix

**File:** `pkg/ci/plugins/terraform/handlers.go`

New helper `downloadPlanfileForVerification()`:
- Downloads planfile from storage
- Writes planfile to `<componentDir>/stored.plan.tfplan`
- Writes lock file to `<componentDir>/.terraform.lock.hcl` (canonical — no prefix needed for lock file)
- Sets `ctx.Info.StoredPlanFile = storedPlanPath`

### 11. `StoredPlanPrefix` constant

**File:** `pkg/ci/plugins/terraform/planfile/interface.go`

```go
const StoredPlanPrefix = "stored."
```

### 12. `StoredPlanFile` field on `ConfigAndStacksInfo`

**File:** `pkg/schema/schema.go`

```go
StoredPlanFile string `yaml:"stored_plan_file" json:"stored_plan_file" mapstructure:"stored_plan_file"`
```

Carries the stored plan path from the download hook (PreRunE) to the verification step (RunE).

**Note:** PreRunE and RunE may create independent `info` structs via `ProcessCommandLineArgs()`. The `StoredPlanFile` path must survive this boundary. Options:
- Pass it via a package-level variable (like `capturedDeployOutput`)
- Pass it via the cobra command context
- Detect `stored.*` file on disk in RunE (no state passing needed)

The simplest approach: in RunE, check if `stored.*` planfile exists on disk at the expected path. No cross-phase state passing required.

## Files to Modify

| File | Changes |
|------|---------|
| `cmd/terraform/apply.go` | Remove `--verify-plan` flag |
| `cmd/terraform/deploy.go` | Add `--verify-plan` flag, wire `before/after.terraform.deploy` events, add verification flow to RunE |
| `pkg/hooks/event.go` | Add `BeforeTerraformDeploy`, `AfterTerraformDeploy` events |
| `pkg/ci/plugins/terraform/plugin.go` | Add `before/after.terraform.deploy` hook bindings |
| `pkg/ci/plugins/terraform/handlers.go` | Remove `downloadPlanfile()` from `onBeforeApply()`. Add `onBeforeDeploy()` (download with stored prefix), `onAfterDeploy()` (summaries, outputs, checks). Add `downloadPlanfileForVerification()` helper. |
| `pkg/ci/plugins/terraform/planfile/interface.go` | Add `StoredPlanPrefix` constant |
| `pkg/ci/plugins/terraform/planfile/write.go` | Add `WritePlanfileResultsForVerification()` helper |
| `pkg/schema/schema.go` | Add `StoredPlanFile` field |
| `internal/exec/terraform_verify_plan.go` | Refactor `VerifyPlanfile()` — accept stored plan path, generate fresh plan in component dir |

## Edge Cases

### Deploy without CI mode

When `deploy` runs without `--ci` (local usage), no planfile download occurs. Deploy behaves as today: runs `terraform apply` with `-auto-approve`.

### Deploy with CI but no stored planfile

If the CI hook downloads nothing (store empty, plan never ran), deploy falls back to running a fresh plan and applying it directly — same as non-CI deploy. A warning is logged.

### Fresh plan shows no changes but stored plan had changes

This is drift — verification fails. The comparison is structural, not "has changes vs no changes".

### Internal plan must not trigger CI hooks

The `terraform plan` inside deploy's verification flow must NOT trigger CI hooks (no upload, no checks, no summaries). This is achieved by running the plan directly through the terraform CLI, not through the atmos plan command/hooks.

## Verification

1. `go build ./...`
2. `go test ./internal/exec/... -run TestVerifyPlanfile` — updated tests pass
3. `go test ./pkg/ci/plugins/terraform/... -count=1` — handler tests pass
4. `go test ./cmd/terraform/... -count=1` — command tests pass
5. Manual: `atmos terraform deploy vpc -s prod --ci` — downloads stored plan, generates fresh plan, compares, applies if matching
6. Manual: Modify infrastructure between plan and deploy — verification detects drift and fails
7. Manual: `atmos terraform apply vpc -s prod --ci` — does NOT download planfile, does NOT verify, just applies with CI cosmetics
