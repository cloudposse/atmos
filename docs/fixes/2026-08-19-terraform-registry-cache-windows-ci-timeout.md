# Fix: give the Windows terraform-registry-cache CI job enough time to finish

**Date:** 2026-08-19

## Summary

The `terraform-registry-cache` job's `timeout-minutes: 20` (added earlier the same day in
`41af363a875` / #2940) was too tight for the Windows leg: the actual `TestTerraformRegistryCache`
test passed, but the job's post-job `actions/cache` save step got cancelled mid-run when the job
hit its 20-minute wall-clock budget, marking the whole job (and the `Acceptance Tests` gate jobs
that depend on it) as failed. Raised the job's `timeout-minutes` to 30.

## Context

CI on PR #2959 reported `Terraform registry cache test (windows)` as failed, with `Acceptance
Tests (linux)`, `(macos)`, and `(windows)` also failing as a result. Reading the attached job log
(`GitHub Job ID: 96241292192`) showed:

- `--- PASS: TestTerraformRegistryCache (57.65s)` / `ok  github.com/cloudposse/atmos/tests
  58.006s` — the actual test passed.
- Setup (checkout, artifact download, toolchain install, `go build deps`, and the `go test`
  binary compile) consumed ~12.6 of the 20-minute budget before the test even started running,
  leaving ~6.4 minutes for the automatic `actions/cache` post-job save step (Go module/build
  cache).
- `##[error]The operation was canceled.` fired at 22:21:52, almost exactly 20 minutes after the
  job started (22:01:50) — a job-level timeout cancellation, not a test failure.
- The three `Acceptance Tests (*)` jobs each failed in 3-6s via their `needs` gate check
  ("`terraform-registry-cache result was 'cancelled'`"), purely as a downstream consequence.

The Terraform parser changes earlier in this PR (`pkg/ci/plugins/terraform/*`, see
`docs/fixes/2026-08-19-ci-test-summary-fallback-recovers-error-detail.md`) did not cause this
failure — they don't touch CI workflow files, and the actual test passed. This timeout follow-up
itself does change `.github/workflows/test.yml` (see Changes below). Checking recent runs of this
job on `main` confirmed it's a pre-existing, borderline-flaky timing issue: three recent
successful Windows runs took 14m, 14m11s, and 18m38s end-to-end — already close to the 20-minute
ceiling that was added the same day — and this run simply tipped over the edge.

## Changes

- `.github/workflows/test.yml`: `terraform-registry-cache` job's `timeout-minutes` raised from
  `20` to `30`, with a comment recording the observed timings so a future reader doesn't
  re-tighten it without the same context. Left the linux/macos legs alone (they finish in
  6-12 minutes and share the same job-level timeout) and left the inner `Terraform registry cache
  acceptance test` step's own `timeout-minutes: 15` unchanged (the test step itself was never the
  bottleneck).

## Validation

This timeout follow-up itself only changes a workflow YAML file and this doc, so its validation is
scoped accordingly:

- `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/test.yml'))"` — valid YAML.
- `atmos lint --changed` — 0 issues.
- No Go code changed by this follow-up, so no `go build`/`go test` re-run was needed for it;
  verification is the next CI run of PR #2959 actually completing `Terraform registry cache test
  (windows)` within budget.

The earlier Terraform parser changes in this PR (`pkg/ci/plugins/terraform/*`) have their own Go
build/test/lint validation recorded in
`docs/fixes/2026-08-19-ci-test-summary-fallback-recovers-error-detail.md`.

## Follow-ups

None. If Windows timing regresses further (e.g. toward 30 minutes) in the future, the right next
step is investigating why `go test -c` compilation and `atmos build deps` are slow on this runner
image, not just raising the timeout again indefinitely.
