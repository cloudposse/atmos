# Fix: retry govulncheck on transient vuln.go.dev fetch failures

**Date:** 2026-08-31

## Summary

The `govulncheck` CI job failed with `HTTP GET https://vuln.go.dev/index/modules.json.gz
returned unexpected status: 403 Forbidden` — a transient failure fetching Go's vulnerability
database, not a real finding or a code problem. Because `golang/govulncheck-action` has no
retry of its own, the job failed on the first attempt, and since no SARIF file was ever
written, the always-run "Upload SARIF file" step then also failed with `Invalid SARIF. JSON
syntax error: Unexpected end of JSON input`. Wrapped the govulncheck step in a 3-attempt retry.

## Context

Same class of problem as `docs/fixes/2026-08-21-cosign-tuf-cdn-403-not-retried.md` (a
transient CDN 403 on a trust/vulnerability-database fetch, not a real security-verdict
failure) but a different fix shape: that one was atmos's own subprocess-retry logic
classifying a cosign error message as retryable; this one is a third-party GitHub Action with
no atmos-owned code to hook into, so the retry has to live at the workflow level instead.

This repo already has an established local pattern for exactly this — no retry action, no
built-in step retry primitive, just a hand-rolled attempt-1/2/3 sequence with
`continue-on-error: true` and `if: steps.<n>.outcome == 'failure'` chaining — in
`.github/actions/download-artifact-retry/action.yml` (added for the same class of problem:
`actions/download-artifact` not retrying connection-level failures). Followed that same shape
rather than introducing a third-party retry-wrapper action or reimplementing govulncheck's
invocation from scratch (which would lose the action's built-in SARIF formatting).

Not extracted into a reusable composite action like `download-artifact-retry`: that one has
many call sites across `setup-atmos-install`; this govulncheck retry has exactly one, so
inlining it as three steps in the job keeps the fix proportionate to where it's actually used.

## Changes

- `.github/workflows/codeql.yml`: the `govulncheck` job's single `Run govulncheck` step is now
  three (`attempt 1/3` through `3/3`), each `continue-on-error: true` except the last, with
  15s waits between attempts on failure. A retry doesn't change what counts as a failure — a
  genuine vulnerability finding still fails the job the same way after exhausting retries; only
  the "did the fetch/scan complete at all" outcome gets a second and third chance.

## Validation

- `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/codeql.yml'))"` — valid YAML.
- `actionlint .github/workflows/codeql.yml` — no issues.
- Not independently verifiable locally (govulncheck's vuln.go.dev fetch and the transient 403
  only reproduce on GitHub's own runners/network path); confirmed via the next PR #1908 CI run
  after pushing this change.

## Follow-ups

None.
