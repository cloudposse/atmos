# Fix: Deduplicate govulncheck SARIF stacks before upload

**Date:** 2026-10-09

## Summary

The govulncheck job now runs through `go tool mage ci:vulncheck`, which removes
exact duplicate stack objects from the SARIF report before upload. GitHub code
scanning accepts the report again, and every finding and distinct stack trace is
preserved.

## Context

[Job 113647222434](https://github.com/cloudposse/atmos/actions/runs/37876824022/job/113647222434)
completed the scan but failed when uploading `govulncheck.sarif`: results 5, 11,
12, 13, and 15 contained duplicate `stacks` entries. The SARIF schema requires
unique objects in that array. This was a report-format failure, not a scanner
execution failure and not a reason to suppress vulnerability findings.

The normalization lives in Go, as a Mage target with unit tests, because the
repository keeps CI logic of this size out of workflow shell and `jq`. See
`magefiles/README.md`.

## Changes

- Add `internal/ci/vulncheck`. `Scan` runs `govulncheck -format sarif` and
  `DedupeStacks` keeps the first occurrence of each complete stack object in its
  original position. Objects compare independently of key order. Numbers and
  text such as `<` and `&` pass through unchanged, and results without stacks
  are untouched.
- Add the `ci:vulncheck <output>` Mage target and list it in
  `magefiles/README.md`.
- Replace the inline `govulncheck ... > govulncheck.sarif` step in
  `.github/workflows/codeql.yml` with the target. The report replaces the output
  file atomically, only after the scan succeeded and the output parsed as SARIF.
  A scanner failure, empty output, or malformed output fails the step and leaves
  any existing report untouched, so an empty upload can never hide
  vulnerabilities.

## Validation

- `go test -race ./internal/ci/vulncheck/` passes at 90.8% coverage. It covers
  duplicates that differ only in key order, distinct messages and frame order,
  first-occurrence ordering, several results and runs, absent, null and
  non-array `stacks`/`runs`/`results`, idempotence, large integers, HTML
  characters, and rejection of empty, truncated, trailing-data and non-object
  input. Scan tests check the command line, replacement of an existing report,
  that every failure mode leaves the previous report untouched with no
  temporary files left behind, and that a missing `govulncheck` is reported.
- `go test -tags mage ./magefiles/` passes.
- `go tool mage ci:vulncheck <file>` run against a stand-in `govulncheck` wrote
  the report with the duplicate stack removed.
- `actionlint .github/workflows/codeql.yml` passes.
- The full govulncheck scan and the GitHub upload need the updated CI run. The
  scan's call-graph analysis is too expensive to repeat locally.

## Follow-ups

None.
