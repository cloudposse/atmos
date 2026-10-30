# Fix: Native CI metadata no longer pollutes stdout inside legacy GitHub Actions

**Date:** 2026-10-07

## Summary

Atmos emitted GitHub Actions workflow-command metadata on **stdout** (the data
channel), which corrupts the structured JSON/YAML that downstream consumers
capture from stdout. The most visible symptom: the CI startup banner's
"Deprecated GitHub Action" `::warning` annotation broke
`cloudposse/github-action-atmos-get-settings@v1`, which parses the stdout of
`atmos describe ...` as JSON and failed with
`SyntaxError: Unexpected token '`', "`::warning "... is not valid JSON`.

Two distinct emitters are involved, and they need different fixes because
annotations and log groups behave differently:

- **Annotations** (`::warning`/`::error`/`::notice`) now go to **stderr**. The
  runner parses workflow commands from the combined stdout+stderr stream, and
  annotations are standalone, so stderr is safe and keeps stdout clean.
- **Log groups** (`::group::`/`::endgroup::`) are now **disabled entirely when a
  legacy action is detected**. Group markers must bracket the stdout content
  they fold, and the runner does not guarantee ordering across stdout and
  stderr, so they cannot be moved to stderr. Outside legacy actions, grouping
  keeps its existing stdout behavior unchanged.

## Context

- Reported in issue #3309. The legacy-action startup banner framework landed in
  #3004 (`feat(ci): startup banner for CI/Pro status and legacy-action
  detection`, v1.228.0), but that version emitted the deprecation notice only to
  stderr. The specific stdout-polluting line — the `::warning` annotation
  emitted via the data channel — was added later in #3177 (`fix(ci): improve
  legacy action migration guidance and docs release labels`), which first
  shipped in v1.230.0.
- Affected versions: every release from v1.230.0 onward (the reporter hit it on
  1.238.1); last known good 1.227.0. v1.228.0-v1.229.x carry the banner but not
  the stdout annotation, so they are unaffected.
- Design discussion on PR #3310 (superseded by this change) concluded: keep
  annotations on stderr always; disable grouping in detected legacy actions
  rather than moving group markers to stderr (which would desynchronize the
  markers from the stdout logs they wrap).

## Changes

- `pkg/ci/providers/github/annotations.go`: `Annotate` now writes each workflow
  command through `ui.Writeln` (stderr) instead of `data.Writeln` (stdout). The
  method still satisfies the `provider.Annotator` `error` contract by returning
  `nil`; the write is fire-and-forget, matching every other UI-channel write.
- `pkg/ci/internal/provider/types.go`: new optional capability
  `LogGroupingSuppressor { SuppressLogGrouping() bool }`, mirroring the existing
  `LogGrouper`/`DebugModeDetector` optional-capability pattern.
- `pkg/ci/providers/github/log_group.go`: the GitHub provider implements
  `SuppressLogGrouping()` via the existing `LegacyActionRepo()` detection, plus
  compile-time assertions for both grouping capabilities.
- `pkg/ci/loggroup.go` and `pkg/ci/log_group.go`: both grouping entry points
  (`grouper()`, which feeds `ci.Group`, `GroupingEnabled`, and
  `ShouldPropagateLogGroupSentinel`; and `ci.StartLogGroup`) consult a shared
  `groupingSuppressed(provider)` helper and emit no group markers when the
  detected provider suppresses grouping. `pkg/ci` cannot import the github
  provider (import cycle), so the capability interface is the decoupling seam.
- `pkg/ci/startup/status.go`: updated the legacy-action comment to note the
  annotation is on stderr.
- Tests: annotation tests now assert stderr with stdout empty
  (`pkg/ci/providers/github/annotations_test.go`,
  `pkg/ci/startup/status_test.go`); new `SuppressLogGrouping` provider test
  (`pkg/ci/providers/github/log_group_test.go`); new grouping-suppression tests
  for both entry points plus `GroupingEnabled`/`ShouldPropagateLogGroupSentinel`
  and a non-suppressed negative control
  (`pkg/ci/loggroup_suppress_test.go`); added coverage for previously-untested
  GitHub status helpers (`pkg/ci/providers/github/status_prs_test.go`).
- Audited the writer-based `StartGroup`/`EndGroup` methods in
  `pkg/ci/providers/github/loggroup.go` that CodeRabbit flagged: they have no
  live callers and emit nothing today, so they pose no stdout-pollution risk and
  were left unchanged. If they are ever wired up, they must route through the
  same legacy-action grouping suppression added here.

## Validation

- `go build ./...` - passes.
- `go test ./pkg/ci/... ./cmd/ci/... ./pkg/hooks/... ./pkg/scanners/... ./internal/exec/ ./pkg/runner/...` - passes.
- Coverage: `pkg/ci` 92.3%, `pkg/ci/providers/github` 94.1%, `pkg/ci/startup` 100%.
- `bash .claude/skills/fix-log/scripts/validate-fix-doc.sh` on this file - passes.
- Regression guards: startup-banner and annotation tests fail if any workflow
  command is written back to stdout; grouping-suppression tests fail if a
  legacy-action provider ever emits group markers.

## Follow-ups

None.
