# Fix: CI startup banner no longer pollutes stdout with `::warning` annotations

**Date:** 2026-10-07

## Summary

The CI startup banner emitted its "Deprecated GitHub Action" annotation as a
`::warning` GitHub Actions workflow command on **stdout** (the data channel).
That corrupted the structured JSON/YAML that downstream consumers capture from
stdout - most visibly `cloudposse/github-action-atmos-get-settings@v1`, which
parses the stdout of `atmos describe ...` as JSON and failed with
`SyntaxError: Unexpected token '`', "`::warning "... is not valid JSON`.

The annotation now goes to **stderr** (the UI channel), where the GitHub
Actions runner still parses it, leaving stdout pristine for data consumers.

## Context

- Reported in issue #3309.
- Introduced by the startup banner in #3904.
- Affected versions: broken in 1.238.1, last known good 1.227.0.

`pkg/ci/providers/github.(*Provider).Annotate` wrote each workflow command via
`data.Writeln` (stdout). GitHub Actions annotations are diagnostic metadata for
the runner, not program output, so per Atmos's two-channel I/O model
(stdout = pipeable data, stderr = human/diagnostic UI) they belong on stderr.
The runner parses workflow commands from the step's combined log stream, which
captures both stdout and stderr, so annotations keep rendering in the PR
Checks/Files UI after the move.

## Changes

- `pkg/ci/providers/github/annotations.go`: `Annotate` now writes each workflow
  command through `ui.Writeln` (stderr) instead of `data.Writeln` (stdout).
  Updated the doc comment to explain why stderr is correct and to reference
  #3309. The write is fire-and-forget (matching every other UI-channel write in
  Atmos); the method still satisfies the `provider.Annotator` `error` contract
  by returning `nil`.
- `pkg/ci/startup/status.go`: updated the legacy-action comment to note the
  annotation is emitted on stderr, not stdout.
- `pkg/ci/providers/github/annotations_test.go`: the previous tests asserted the
  annotation landed on stdout (they codified the bug). Reworked to assert it
  lands on stderr with stdout left empty, plus an empty-input case. Kept the
  shared `errWriter`/`testStreams` helpers used by other tests in the package.
- `pkg/ci/startup/status_test.go`: the two legacy-action tests now assert the
  `::warning` annotation is on stderr and that stdout receives nothing.
- `pkg/ci/providers/github/status_prs_test.go` (new): added coverage for the
  previously untested `getStatus` opt-in PR paths, `getPRsCreatedByUser`,
  `getPRsRequestingReview`, and `searchPRsWithQuery`, including the
  user-lookup-fails, search-fails, and bare-PR (full-fetch-fails) paths.

## Validation

- `go build ./...` - passes.
- `go test ./pkg/ci/providers/github/ ./pkg/ci/startup/` - passes.
- Coverage: `pkg/ci/providers/github` 88.8% -> 94.1%; `pkg/ci/startup` 100%
  (the two changed source functions, `Annotate` and `printStatusLines`, are at
  100%).
- `bash .claude/skills/fix-log/scripts/validate-fix-doc.sh` on this file - passes.
- Regression guards added: the startup-banner and annotation tests now fail if
  any workflow command is ever written back to stdout.

## Follow-ups

None.
