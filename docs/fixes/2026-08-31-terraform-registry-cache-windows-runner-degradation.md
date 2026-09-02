# Fix: `Terraform registry cache test (windows)` CI job cancelled again despite the test passing

**Date:** 2026-08-31

## Summary

The `Terraform registry cache test (windows)` CI check was reported failing (`GitHub Job ID:
99475416637`), but the actual `TestTerraformRegistryCache` Go test had already passed. The job's
`timeout-minutes: 30` (raised from 20 in #2959, see
`docs/fixes/2026-08-19-terraform-registry-cache-windows-ci-timeout.md`) still wasn't enough this
time: three unrelated steps were all running roughly 10x slower than a normal run, and their
combined time exceeded even the raised budget. Raised `timeout-minutes` for this job's windows
leg from 30 to 45.

## Context

The attached failure log (`.context/attachments/lrwozZ/...log`) contained only Windows runner
diagnostic noise (`pid reused`, `existing process not stopped`, `Cleaning up orphan processes`) —
no test output at all, because the actual per-step job log had already scrolled past the
"last 1000 lines" window captured for the attachment. Reading the real per-step timing via
`gh api repos/cloudposse/atmos/actions/jobs/99475416637` showed the job's true conclusion was
`cancelled`, not a test failure:

- `Get dependencies`: 11:43:59 → 11:49:02 (~5m) — a normal successful run of this same job
  (`32400222717`, 2026-08-20) took ~5s for the equivalent step.
- `Terraform registry cache acceptance test`: 11:49:02 → 11:59:30 (~10.5m), **completed
  successfully** — well inside its own 15m sub-step timeout, but ~10x the ~1m a normal run takes.
- `Post Set up Go` (the `actions/setup-go` cache-save step): 11:59:30 → 12:10:08 (~10.5m),
  **also completed successfully** — a normal run finishes this step near-instantly.
- `Post Cache Atmos toolchain` (normally ~42s): started 12:10:08, was still running when the
  job's 30-minute budget expired at 12:11:50 (job started 11:41:50), and got cancelled at
  12:11:54.

Comparing against four other `Terraform registry cache test (windows)` runs from 2026-08-20 (all
`success`, total job duration 7-21 minutes, `Post Set up Go` consistently near-instant) confirmed
this was not a step regressing in isolation — every measured operation in the failing run,
regardless of what it actually does (module download, running a Go test binary, saving a build
cache), was uniformly ~10x slower than normal. That signature points to a degraded or throttled
runner/network that day, not a code-level regression in this repository: a real regression would
slow only the affected step, not dependency download, test execution, and cache save all equally.

This is the second time this exact job has hit its own timeout after its test step had already
passed (see the 2026-08-19 fix above, which raised 20→30); 30 minutes evidently still doesn't
leave enough headroom for an occasional fully-degraded run.

## Changes

- `.github/workflows/test.yml`: `terraform-registry-cache` job's windows-leg `timeout-minutes`
  raised from 30 to 45, with the comment updated to record this incident's measured timings
  alongside the original 2026-08-19 incident. Linux/macOS remain at 20 (unaffected; both
  historically finish this job comfortably under budget).

## Verification

- `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/test.yml'))"` parses cleanly.
- `actionlint .github/workflows/test.yml` reports no issues.
