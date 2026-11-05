# Fix: Publish project coverage only from the complete test suite

**Date:** 2026-10-24

## Summary

Keep focused release-rehearsal coverage as a workflow artifact and publish Codecov
coverage only from the aggregate Tests job.

## Context

PR #3474 added a separate Codecov upload from Local action tests. That upload
finishes before the acceptance matrix and covers only a small set of packages.
On PR #3509 at `2c6052160b`, Codecov reported 96.45% project coverage for 34 files
and 3,410 lines, compared with 2,622 files and 246,749 lines on main. The full-suite
upload had not run yet. The percentage described the partial report, not the
repository, and appeared as an improvement in the PR comment.

## Changes

- Replace the focused Codecov upload with a downloadable `release-rehearsal-coverage`
  artifact, retaining the race-enabled tests and real GoReleaser rehearsal.
- Keep `test.yml` as the sole Codecov publisher, after its build, acceptance, and
  Mage test dependencies succeed. It uploads the combined acceptance, Mage, and
  notice-generator reports; the Mage report already includes release packages.
- Preserve coverage thresholds, flags, exclusions, and the existing aggregate
  report scope. Rehearsal-specific extra hits remain available in the diagnostic
  artifact rather than contributing a separate Codecov upload.
- Extend workflow contract tests to reject the focused upload and require the
  aggregate publisher's dependencies and complete report list.

## Validation

- Passed `go test -tags=mage ./magefiles/release -run 'Test(ReleaseRehearsal|RepositoryCoverage)' -count=1`.
- Passed actionlint for both changed workflows with ShellCheck and Pyflakes disabled.
- Confirmed only `test.yml` references the Codecov upload action.
- Passed the fix-document validator and `git diff --check`.
- Live Codecov behavior remains to be confirmed by the next CI run.

## Follow-ups

Confirm the next CI run publishes project coverage only after the aggregate test
job completes. The current-head Codecov comment will update after a new complete
report is uploaded; changing this workflow does not rewrite older reports.
