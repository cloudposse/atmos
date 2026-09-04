# Fix: streaming UI reported "failed: 0 error(s)" for diagnostic-only failures

**Date:** 2026-09-01

## Summary

`ResourceTracker.GetErrorCount()` only counted resource-state errors, while
`ResourceTracker.HasErrors()` (which decides whether the "failed" banner shows at all) also
counted error-severity diagnostics with no resource address. A failure with no individually
errored resource — e.g. Terraform's own `-out=` planfile write failing — tripped the "failed"
banner via `HasErrors()` but reported `0 error(s)` via the out-of-sync `GetErrorCount()`,
producing a confusing `Plan <stack>/<component> failed: 0 error(s) (Ns)` summary line.

## Context

Reported by the user hitting a real `-out=` write failure (`open
.workdir/terraform/<stack>-<component>-<hash>/.atmos-plan-<ts>.tfplan: no such file or
directory` — the JIT-provisioned workdir was gone by the time Terraform tried to write the
final planfile, root cause still under investigation and not part of this fix) while testing
the streaming UI. The summary line's `0 error(s)` was a second, independent bug spotted while
investigating: `pkg/terraform/ui/model_render.go`'s `renderErrorSummary` calls
`m.tracker.GetErrorCount()` to populate the count, and `finalView` gates on
`m.tracker.HasErrors()` to decide whether to call it — the two were never kept in sync.

## Changes

- `pkg/terraform/ui/resource.go`: `GetErrorCount()` now also counts error-severity
  diagnostics, matching `HasErrors()`'s definition exactly.
- `pkg/terraform/ui/resource_test.go`: extended `TestResourceTracker_HandleDiagnostic` with
  `GetErrorCount()` assertions at both the warning-only and error-diagnostic stages —
  regression coverage for the diagnostic-only-failure case.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/terraform/ui/...` — all pass, including the extended test.
- `gofumpt -l pkg/terraform/ui/resource.go pkg/terraform/ui/resource_test.go` — clean.

## Follow-ups

The underlying `-out=` write failure (workdir directory missing mid-run) is not fixed here —
root cause not yet confirmed. `atmos terraform plan` itself never calls
`pkg/provisioner/workdir/clean.go`'s `CleanWorkdir`/`CleanAllWorkdirs`/`CleanExpiredWorkdirs`
(those are only reachable from the explicit `atmos terraform workdir clean` command), so no
automatic self-cleanup explains it; the most likely explanation is an external actor (a
concurrent `atmos terraform workdir clean`, a second `plan`/`apply` on the same
stack/component racing the same deterministic hash path, or something outside atmos removing
the directory) rather than a bug in the plan/provisioning pipeline itself. None of
`pkg/provisioner/workdir/clean.go`'s removal paths take a lock against an in-flight
provision/execute cycle for the same path.
