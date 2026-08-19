# Fix: `Terraform registry cache test (windows)` CI job timed out during cache save, not the test

**Date:** 2026-08-19

## Summary

The `Terraform registry cache test (windows)` CI check failed with `The operation was canceled.`,
but the actual `TestTerraformRegistryCache` Go test had already passed. The job's own
`timeout-minutes: 20` expired while the post-job Go module/build cache save step (`tar` + `zstd`)
was still running — a step that runs notably slower on Windows runners than on macOS/Linux.

## Context

User attached a failing-CI log for job `Terraform registry cache test (windows)` (GitHub job ID
96243114852) on PR #2961 and asked to fix the failing CI actions.

Reading the log: `--- PASS: TestTerraformRegistryCache (58.69s)` / `ok
github.com/cloudposse/atmos/tests 59.000s` at 22:24:00, well inside the test step's own 15-minute
sub-step timeout. The job then moved into `Post job cleanup` and started `tar.exe ... zstd -T0`
to build the `actions/cache` save archive at 22:24:12. That step was still running when
`##[error]The operation was canceled.` was logged at 22:29:01 — exactly matching the job's
20-minute deadline from its 22:08:57 start (22:08:57 + 20m = 22:28:57, plus a few seconds of
reporting lag). No `concurrency:` block cancels in-progress runs in `.github/workflows/test.yml`,
so this was conclusively the job's own timeout, not an external cancellation. The corresponding
Linux (6m39s) and macOS (11m15s) legs of the same job both finished comfortably inside the 20m
budget — Windows cache-save is the outlier, a pattern this same workflow file already accounts for
elsewhere (the `Acceptance tests` step grants Windows 40m vs 30m for macOS/Linux, with a comment
documenting the same Windows-is-slower-at-caching behavior).

## Changes

### Code

| File                            | Change                                                                 |
|-----------------------------------|-------------------------------------------------------------------------|
| `.github/workflows/test.yml`     | `terraform-registry-cache` job's `timeout-minutes` changed from a flat 20 minutes to 30 minutes on Windows and 20 minutes on other targets, matching the existing per-target timeout pattern used by the `Acceptance tests` step in the same file |

## Validation

- YAML parses (`python3 -c "import yaml; yaml.safe_load(open('.github/workflows/test.yml'))"`).
- Pushed to PR #2961. The `Terraform registry cache test (windows)` check for this change is
  **pending** at the time this record was written; this section has not yet been updated with a
  pass/fail outcome.

## Follow-ups

None.
