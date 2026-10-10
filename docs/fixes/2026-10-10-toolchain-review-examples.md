# Fix: Refresh toolchain examples and cover startup fallback

**Date:** 2026-10-10

## Summary

Updated the toolchain announcement to use the repository's Terraform versions and added command-level
coverage for startup continuing when the project tool manifest is unusable.

## Context

Review of [PR #3346](https://github.com/cloudposse/atmos/pull/3346#discussion_r4237697169) flagged the
Terraform 1.9.0 example as outdated. The attached Codecov report for `1baf1e98d6` reported 98.98990%
patch coverage, with three missing lines and two partial lines. The repository's configured patch
target is 85%; the uncovered behavior was inspected independently of the passing GitHub check.
CodeRabbit also reported a docstring coverage warning.

## Changes

- The announcement now uses Terraform 1.15.8, matching the repository's `.tool-versions`, and 1.15.6
  for the component override, matching the acceptance-test configuration. Related prose and the
  version-constraint example were updated consistently.
- Added tests through the root command for an unreadable manifest and conflicting aliases. They
  assert that diagnostic commands run, PATH remains unchanged, no tools are provisioned, and scoped
  component execution still reports the manifest error. These exercise the missing and partial
  startup lines in `cmd/root.go`.
- Corrected the `ForComponent` comment to describe `declared` inheriting installed project selections.
  Documented previously undocumented production helpers added by the PR and the startup tests.
- Reviewed the three remaining coverage findings without adding production injection points solely
  for coverage: `ForCommand`'s resolver currently returns no errors; successful Aqua index loading
  always initializes a non-nil package slice; and custom-command PATH assignment cannot receive a
  NUL byte from valid installed paths or the inherited process environment. Their defensive error
  handling remains in place. No coverage thresholds or exclusions changed.

## Validation

- `go test ./cmd -run '^Test(RootCommand.*|InstalledProjectTools.*)$' -count=1
  -coverprofile=.context/review-command-coverage.out`: passed. The profile confirms hits on the
  reported `cmd/root.go` condition and fallback log statement. This package-level profile does not
  reproduce Codecov's merged cross-platform patch percentage.
- `go build ./...`: passed.
- `atmos fix lint`: passed.
- `TEST='./cmd ./pkg/dependencies ./pkg/toolchain/registry/aqua' atmos test`: passed.
- Website `npm run build`: passed, with existing warnings about unrelated anchors, dynamic imports,
  and Markdown normalization.
- CodeRabbit's aggregate docstring percentage requires a fresh remote review; it was not calculated
  locally.

## Follow-ups

None.
