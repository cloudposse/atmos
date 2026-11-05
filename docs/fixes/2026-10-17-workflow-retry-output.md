# Fix: Workflow retry conditions can match failed command output

**Date:** 2026-10-17

## Summary

Workflow shell and Atmos steps now retry when `retry.conditions` matches a failed
attempt's stdout or stderr. The correction applies to every configuration edition.

## Context

Comparing v1.232.0 (`26edaff2cc461de95e99e48db1583cd08270f8f8`) with main
(`373a6bc02d1a2c83e0e63f0d707c131f65c2383c`) revealed that a workflow which printed
a matching transient error, failed, and would succeed on its next attempt instead
stopped after its first attempt on main. Both unpinned projects and projects pinned
to edition `2026-10-12` were affected.

The workflow command runner captured output but returned the command error without
attaching that output. The retry predicate therefore saw only the exit-status error.
Custom commands already attached captured output correctly. This is a regression,
not an intentional compatibility change; preserving it behind an edition gate would
leave existing projects broken.

## Changes

- Return `step.WithStepOutput(runErr, commandResult)` from the shared workflow
  command runner so retry predicates can inspect captured stdout and stderr.
- Reuse the existing wrapper, which preserves the original error chain and uses
  the already masked captured output.
- Add 20 workflow test cases covering shell and Atmos commands, explicit and
  generated step names, stdout matching, stderr matching, nonmatching conditions,
  error-text matching, and unconditional retries. Assert attempt counts and that
  only successful output is stored as a step result.

## Validation

- Before the fix, the new test failed in all eight stdout/stderr matching cases;
  the twelve control cases passed.
- After the fix, the focused workflow, retry, command-result, timeout, cancellation,
  and masking tests passed in `internal/exec`, `pkg/runner/step`, and `cmd`.
- `GOMAXPROCS=2 CGO_ENABLED=0 go build ./...` passed.
- Package-wide short tests passed in `internal/exec` and `pkg/runner/step` via
  `TEST='./internal/exec ./pkg/runner/step ./cmd' TESTARGS='-count=1' atmos test`.
  The `cmd` package initially failed `TestSetupColorProfileIgnoresScriptFlags`
  because the session exports `NO_COLOR=1`; the entire package passed with
  `env -u NO_COLOR GOMAXPROCS=2 CGO_ENABLED=0 GOFIPS140=latest go test -short ./cmd -count=1 -timeout=5m`.
- Custom golangci-lint against the complete fix patch, including the new test,
  passed with zero issues. `atmos lint --changed` reported six issues in unchanged
  files: `internal/exec/template_funcs_component.go`, `cmd/helm/helm.go`,
  `cmd/kubernetes/kubernetes.go`, and `pkg/terraform/output/format.go`.
- The fix-log validator, regression-test skill metadata validator, and
  `git diff --check` passed.
- Before opening the PR, `atmos test --full` passed with the workflow, retry,
  command-result, timeout, cancellation, and masking test filter across the three
  affected packages; `atmos lint --changed` passed with the complete patch staged.
- `npm run build` in `website` passed, with warnings about existing MDX
  normalization fallbacks, dynamic dependencies, and broken anchors.
- Rebuilt the CLI and replayed the saved fixtures twice against the release,
  original main, and fixed binary: 72 executions matched expectations. Matching
  output now recovers; nonmatching conditions still stop after one attempt on main
  and the fixed binary. Tested unpinned configuration and edition `2026-10-12`.
  Local commands, binary and patch hashes, and observations are retained under
  `.context/regression-test/2026-10-17/fix/` and its sibling `runs/` directory.

## Follow-ups

None.
