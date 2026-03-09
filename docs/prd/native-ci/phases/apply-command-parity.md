# Apply Command Parity (FR-7) — SHIPPED

> Related: [CI Detection](../framework/ci-detection.md) | [Hooks Integration](../framework/hooks-integration.md) | [Implementation Status](../framework/implementation-status.md) | [Plan Verification](./plan-verification-ci-integration.md)

## Status: SHIPPED

All items implemented: `apply.go` full CI wiring (PreRunE, output capture, error defer, PostRunE with output), `deploy.go` with own `before/after.terraform.deploy` hook events, `--ci` flag, and full CI wiring.

### Command Responsibility

| Command | CI Planfile Download | Plan Verification | CI Summaries/Checks/Outputs |
|---------|---------------------|-------------------|----------------------------|
| `plan` | N/A | N/A | Yes (upload planfile, write summary, checks, outputs) |
| `apply` | **No** | **No** | Yes (write summary, checks, outputs only) |
| `deploy` | **Yes** | **Yes** | Yes (write summary, checks, outputs) |

- **`apply`** is a thin wrapper around `terraform apply`. In CI mode, it only writes summaries, checks, and outputs. It does NOT interact with planfile storage.
- **`deploy`** is the CI-native apply command. In CI mode, it downloads stored planfiles, verifies them against fresh plans, and applies only if they match.

## Problem Statement

The `terraform plan` command had full CI lifecycle wiring (`PreRunE` → `RunE` with output capture + error defer → `PostRunE` with captured output), but `terraform apply` and `terraform deploy` were only partially wired:

1. **`apply.go`**: Had `--ci` flag and `PostRunE` firing `after.terraform.apply`, but:
   - No `PreRunE` — `before.terraform.apply` hooks (planfile download) never triggered.
   - No stdout/stderr capture — CI hooks received empty output.
   - No error defer — check runs not updated to failure status on `RunE` error (Cobra skips `PostRunE` on error).

2. **`deploy.go`**: Had no `--ci` flag at all. `PostRunE` fired `after.terraform.apply` via `runHooks()` but CI hooks only activated if `ci.enabled: true` in config (no flag to force CI mode).

## Solution

### Plan/Apply: PreRunE/PostRunE Pattern

```go
var capturedOutput string

var cmd = &cobra.Command{
    PreRunE: func(cmd *cobra.Command, args []string) error {
        return runHooks(h.BeforeTerraform*, cmd, args)
    },
    RunE: func(cmd *cobra.Command, args []string) (runErr error) {
        capturedOutput = ""
        defer func() {
            if runErr != nil {
                runHooksOnErrorWithOutput(h.AfterTerraform*, cmd, args, runErr, capturedOutput)
            }
        }()
        // ... CI mode detection, output capture via bytes.Buffer ...
        err := terraformRunWithOptions(...)
        if ciMode {
            capturedOutput = ansi.Strip(combined)
        }
        return err
    },
    PostRunE: func(cmd *cobra.Command, args []string) error {
        return runHooksWithOutput(h.AfterTerraform*, cmd, args, capturedOutput)
    },
}
```

### Deploy: Deferred Hook Firing

Deploy fires `before.terraform.deploy` from **inside** `terraformRunWithOptions` (after `ProcessCommandLineArgs` resolves stacks) rather than from `PreRunE`. This is necessary because `PreRunE` calls `ProcessCommandLineArgs` which eagerly resolves ALL stacks including `!store` YAML functions — if component B depends on component A's outputs via `!store`, and component A hasn't been deployed yet, `PreRunE` would fail.

The `after.terraform.deploy` hook fires from `PostRunE` normally. `onAfterDeploy` sets `ctx.Command = "apply"` and delegates to `onAfterApply` since deploy is semantically apply for CI purposes (uses `apply.md` template, fetches terraform outputs, sets `success` variable).

### CI Mode Detection

Same for all three commands:
1. `--ci` flag from Cobra
2. `viper.GetBool("ci")` (env vars: `ATMOS_CI`, `CI`)
3. `ci.IsCI()` (auto-detection: `GITHUB_ACTIONS=true`, etc.)

**Important**: `ci.enabled: true` must be set in `atmos.yaml` for any CI hooks to fire. This is a hard kill switch — even `--ci` flag cannot override it. See [CI Detection](../framework/ci-detection.md).

The `--ci` flag is bound to both `ATMOS_CI` and the generic `CI` environment variable (set by all CI runners). This means `--ci` is implicitly `true` on every CI run. This is safe because `ci.enabled` in `atmos.yaml` is the hard kill switch — when `ci.enabled` is `false` (or not set), CI hooks are disabled unconditionally.

## Files Changed

| File | Change |
|------|--------|
| `cmd/terraform/apply.go` | Added `capturedApplyOutput`, `PreRunE` (`before.terraform.apply`), error defer, stdout/stderr capture, updated `PostRunE` to pass output |
| `cmd/terraform/deploy.go` | Added `--ci` flag with `ATMOS_CI`/`CI` env vars, `capturedDeployOutput`, `before/after.terraform.deploy` events (not apply events), `before.terraform.deploy` fires from `terraformRunWithOptions`, error defer, stdout/stderr capture |
| `cmd/terraform/utils.go` | Added `runCIHooksForDeploy` helper for firing `before.terraform.deploy` inside `terraformRunWithOptions` |
| `pkg/hooks/event.go` | Added `BeforeTerraformDeploy`, `AfterTerraformDeploy` constants |
| `pkg/ci/plugins/terraform/plugin.go` | Added deploy hook bindings (4 → 6 total) |
| `pkg/ci/plugins/terraform/handlers.go` | Added `onBeforeDeploy`, `onAfterDeploy`, `downloadPlanfileForVerification` |

## Verification

1. `go build ./...` — passes.
2. `go test ./cmd/terraform/...` — passes.
3. `go test ./pkg/ci/plugins/terraform/...` — passes.
