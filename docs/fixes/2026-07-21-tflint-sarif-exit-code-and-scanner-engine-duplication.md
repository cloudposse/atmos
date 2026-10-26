# Fix: tflint SARIF exit-code regression; documented scanner/hook engine duplication

**Date:** 2026-07-21

## Problem

A code-hygiene review of the `osterman/tf-lint-hook` branch found that
`pkg/scanners/sarif.NewResultHandler` reported "no findings" whenever tflint's
SARIF output file was missing, without checking the scanner's exit code. The
sibling `pkg/hooks/sarif.NewResultHandler` (used by checkov/trivy/kics)
already had the correct check. Because `pkg/scanners/sarif` was a
copy-forked package, the fix present in the original was never carried over:
if tflint crashed or was misconfigured and never wrote a SARIF file, `atmos
terraform lint` (and the `type: tflint` step / `kind: tflint` hook) reported a
clean pass instead of a failure.

The review also asked whether `pkg/scanners`'s runner/subprocess engine
(`runner.go`, `component.go`, `sarif_report.go` — all new in this branch, ~500
lines) could be merged into the pre-existing `pkg/hooks/command_engine.go`,
which it duplicates almost line-for-line.

## Fix

`pkg/scanners/sarif/handler.go`'s `NewResultHandler` now checks
`ctx.ExitCode` when the SARIF file is missing, reporting `StatusFailure` /
"scan failed" instead of a false "no findings" — mirroring
`pkg/hooks/sarif`'s already-correct `missingReportSummary`. Regression test:
`TestHandler_MissingReportAfterScannerFailure`.

Separately, `pkg/hooks/kinds/tflint/kind.go`'s hook registration (`kind:
tflint`, distinct from the `atmos terraform lint` CLI path) now sets
`CaptureStdout: true` and a `ResultHandler`, and its `tflintEngine.Run`
resolves tflint's dynamic `--config` argument before delegating to the shared
`hooks.CommandEngine` — closing a gap where that lifecycle-hook path had no
working SARIF capture at all.

## Why the two engines were not merged

Investigating the merge surfaced a real Go import cycle, not a style
preference: `pkg/hooks/step_engine.go` directly imports `pkg/runner/step`
(to bridge `kind: step`/`kind: steps` hooks to the step registry), and
`pkg/runner/step/tflint.go` imports `pkg/scanners/tflint` (for the `type:
tflint` workflow step). So:

```
pkg/hooks → pkg/runner/step → pkg/scanners/tflint → (would need) pkg/hooks
```

`pkg/scanners` (and its subpackages) can never import `pkg/hooks` as a
result. This isn't incidental — `pkg/hooks`'s dependency on the step
registry is core lifecycle-hook machinery, not something specific to tflint.

**Decision:** keep `pkg/hooks/command_engine.go` and `pkg/scanners/runner.go`
as two separate engine implementations. `pkg/scanners` (used by `atmos
terraform lint` and the `type: tflint` step) and `pkg/hooks` (used by
lifecycle hooks — checkov/trivy/kics/tflint-as-`kind:-tflint`) each own their
own subprocess/PATH/env/CI-reporting mechanics, and each own SARIF handler
package (`pkg/scanners/sarif`, `pkg/hooks/sarif`). Merging them would require
first decoupling `pkg/hooks`'s direct dependency on `pkg/runner/step` — a
materially larger, riskier change touching lifecycle-hook execution for every
hook kind, not just tflint, and out of scope for this PR.

Future work that wants a single engine should start from that decoupling,
not from `pkg/scanners`/`pkg/hooks` directly.

## Tests

```shell
go test ./pkg/scanners/sarif/... -run TestHandler_MissingReportAfterScannerFailure -v
go test ./pkg/hooks/kinds/tflint/... -v
go test ./pkg/hooks/... ./pkg/scanners/...
```
