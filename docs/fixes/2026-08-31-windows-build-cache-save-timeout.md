# Fix: raise Windows Build job timeout for cold Go cache saves

**Date:** 2026-08-31

## Summary

`Build (windows)` (and everything downstream: the per-OS Acceptance Tests gate jobs and the
k3s matrix, which all `needs:` it) failed on run 33387953802 with the job hitting its
`timeout-minutes: 45` limit. This wasn't a code or test failure — every real build/test step
succeeded; only the trailing `actions/setup-go` cache-save post-step was still running when
the timeout cancelled the job. Raised the job timeout to 60 minutes.

## Context

Investigated via `gh run view`/`gh api repos/.../actions/jobs/<id>` rather than trusting the
attached logs at face value: the four attached "failing" logs (Acceptance Tests
linux/macos/windows, k3s/demo-helmfile) all showed the same shape — a gate step that checks
"did the matrix jobs actually run" failing with "found 0 shard jobs" / "matrix result was
'skipped'". That's the expected, correct behavior of those gates when an upstream `needs:`
job never completes; it isn't evidence of a code bug in the gates themselves.

Per-step timing for job 99474734663 confirmed the real cause: `Set up job` through `Upload
build artifacts` all completed successfully by 12:01:22 (Build itself took only 5m31s).
`Post Set up Go` (the Go module/build cache save) then ran until 12:24:07 — 22m45s and still
in progress — when the job's `timeout-minutes: 45` cancelled it. The run's overall conclusion
was `cancelled`, not `failure`, and `gh run list` showed no newer run for the branch, so this
wasn't superseded by a later push either.

This is the same class of flake the existing comment in `test.yml` already documents (a prior
occurrence killed at 30m10s, timeout raised to 45m as a result) — a large merge from `main`
(58 commits, touching `go.mod`) cold-started the Windows runner's Go cache, and 45 minutes of
headroom wasn't enough for that cache save on top of the build itself this time.

## Changes

- `.github/workflows/test.yml`: `build` job's `timeout-minutes` raised from `45` to `60` for
  the same reason the prior 30→45 bump was made, extending the comment to record this second
  occurrence (run 33387953802, job 99474734663) for future reference.

## Validation

- `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/test.yml'))"` — valid YAML.
- `actionlint .github/workflows/test.yml` — no issues.
- Re-ran the failed jobs on PR #1908 via `gh run rerun --failed` after pushing this change, to
  confirm the branch is unblocked (see PR checks for the outcome).

## Follow-ups

None filed. If this recurs a third time, the next step would be investigating whether the
Windows Go cache save can be made non-blocking (e.g. `actions/cache/save` as a best-effort
background step) rather than continuing to raise the timeout.
