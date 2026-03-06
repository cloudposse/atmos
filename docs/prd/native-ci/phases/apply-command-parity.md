# Apply Command Parity (FR-7) — SHIPPED

> Related: [CI Detection](../framework/ci-detection.md) | [Hooks Integration](../framework/hooks-integration.md) | [Implementation Status](../framework/implementation-status.md)

## Status: SHIPPED

All items implemented: `apply.go` full CI wiring (PreRunE, output capture, error defer, PostRunE with output), `deploy.go` `--ci` flag with identical full CI wiring.

## Problem Statement

The `terraform plan` command had full CI lifecycle wiring (`PreRunE` → `RunE` with output capture + error defer → `PostRunE` with captured output), but `terraform apply` and `terraform deploy` were only partially wired:

1. **`apply.go`**: Had `--ci` flag and `PostRunE` firing `after.terraform.apply`, but:
   - No `PreRunE` — `before.terraform.apply` hooks (planfile download) never triggered.
   - No stdout/stderr capture — CI hooks received empty output.
   - No error defer — check runs not updated to failure status on `RunE` error (Cobra skips `PostRunE` on error).

2. **`deploy.go`**: Had no `--ci` flag at all. `PostRunE` fired `after.terraform.apply` via `runHooks()` but CI hooks only activated if `ci.enabled: true` in config (no flag to force CI mode).

## Solution

Applied the exact same CI wiring pattern from `plan.go` to both `apply.go` and `deploy.go`:

### Pattern (identical for plan/apply/deploy)

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

### CI Mode Detection Chain

Same for all three commands:
1. `--ci` flag from Cobra
2. `viper.GetBool("ci")` (env vars: `ATMOS_CI`, `CI`)
3. `ci.IsCI()` (auto-detection: `GITHUB_ACTIONS=true`, etc.)

## Files Changed

| File | Change |
|------|--------|
| `cmd/terraform/apply.go` | Added `capturedApplyOutput`, `PreRunE` (`before.terraform.apply`), error defer, stdout/stderr capture, updated `PostRunE` to pass output |
| `cmd/terraform/deploy.go` | Added `--ci` flag with `ATMOS_CI`/`CI` env vars, `capturedDeployOutput`, `PreRunE` (`before.terraform.apply`), error defer, stdout/stderr capture, updated `PostRunE` to pass output |

## Verification

1. `go build ./...` — passes.
2. `go test ./cmd/terraform/...` — passes.
