# Fix: auto-rerun `Tests` runs whose only failures are infrastructure zombies

**Date:** 2026-09-03

## Summary

Added `.github/workflows/rerun-infra-failures.yml`, a `workflow_run` backstop that re-runs a
failed or cancelled `Tests` (test.yml) run with `gh run rerun --failed` when every non-success job
is an infrastructure zombie (a runner that finished all its steps but never reported completion,
or a runner that vanished) or a `Check ... result` aggregator that failed only because of one. Any
job with a genuinely failed step vetoes the rerun. The classification is typed, unit-tested Go in
`internal/ci/rerun`, dispatched through `go tool mage ci:classifyInfraFailures` by the local
composite action `.github/actions/classify-infra-failures`, following the pattern the acceptance
orchestration already uses (`magefiles/acceptance.go`, `internal/ci/acceptance`).

## Context

Phase 3 of the CI-stability plan. Since harden-runner moved to Windows/macOS legs (Aug 31) and to
`block` mode (Sept 2), a growing share of `Tests` runs (23 jobs, roughly 27% of runs on Sept 3)
end with a Windows job whose every step, including `Complete job`, is `success`, yet the job is
`cancelled` 30 to 40 minutes later. The harden-runner Windows post step kills its agent mid
DNS-restore, leaving the runner on a dead `127.0.0.1` resolver; the runner uploads its logs but
its final completion call never resolves. Phase 1 adds an in-job DNS guard; this workflow is the
backstop that keeps PR authors from having to click "Re-run failed jobs" by hand while the guard
and the upstream fix land.

Real-run evidence used to design the classifier (all `pull_request`, Sept 3):

| Run | Non-success jobs | Note |
|---|---|---|
| 33774798945 | `Build (windows)` cancelled, all 19 steps success, reaped 34 min after `Complete job`; `[k3s] demo-helmfile` and the three `Acceptance Tests (<os>)` aliases `failure` with only `Check ... result` failed | zombie build stalls the entire run |
| 33769749251 | Windows shards 2/10 and 6/10 cancelled, all steps success, reaped 50 min later; `Acceptance Tests (windows)` alias fails on shard count | zombie shards |
| 33751647765 | Windows shard 6/10 zombie; a manual `--failed` rerun re-executed only the shard, its alias and dependents, carried the other 86 jobs over, and succeeded | the exact behaviour this workflow automates |
| 33775397630 | `[magefiles] unit tests`, `[mock-linux] examples/demo-atlantis` failed on real steps | must never be rerun |

Every observed zombie run also has `failure` jobs whose only failed step is an aggregator
(`Check per-OS test matrix result`, `Check k3s matrix result`): those `if: always()` jobs count
their shards via the jobs API and fail when a shard never reported. A classifier that treated
them as real failures would never fire, so they get their own `check-cascade` class, tolerated
only alongside at least one genuine zombie.

## Changes

- `internal/ci/rerun/classify.go` (new): `Job`/`Step` model decoded from the GitHub jobs API JSON
  (`DecodeJobs` accepts one `{"jobs":[...]}` page, several concatenated pages as emitted by
  `gh api --paginate`, or a bare array; `encoding/json` parses the RFC 3339 timestamps with or
  without fractional seconds). `Classify(jobs, Options)` assigns one class per job whose
  conclusion is not success/skipped/empty: `runner-stuck-after-complete` (cancelled, every step
  completed with success/skipped, job completion at least `Options.StuckGap`, default 5 minutes,
  after the last step), `superseded` (any other cancellation, including mid-step cancels and
  never-started jobs), `runner-lost` (failure with no failed step), `check-cascade` (failure whose
  failed steps all match `^Check .* result$`), `real-failure` (everything else).
  `Verdict(classified)` returns `rerun` only when at least one job is stuck/lost and every other
  one is check-cascade, `no-jobs` when nothing failed, `no-rerun` otherwise, with a reason string.
  `WriteTSV` renders `job\tconclusion\tclass`. The package does no HTTP on purpose: the action
  fetches the jobs with `gh api ... --paginate` into a file, which keeps the Go a pure function of
  the API payload with no new client plumbing.
- `internal/ci/rerun/classify_test.go` + `testdata/run-<id>.json` (new): table-driven tests for
  every class, the gap boundary (exactly 5 min is stuck, one second less is superseded), custom
  `StuckGap`, missing timestamps, running/success/skipped jobs omitted, paginated and bare-array
  decoding with error cases, every `Verdict` branch, and the four real runs above (fixtures
  trimmed to their non-success jobs plus two successful ones).
- `magefiles/ci_rerun.go` (new, `//go:build mage`): `CI.ClassifyInfraFailures(jobsFile)` prints
  the TSV table to stdout and `verdict: <outcome> (<reason>)` to stderr, and when `GITHUB_OUTPUT`
  is set appends `verdict=`, `reason=` and a multi-line `table` output. `ATMOS_CI_STUCK_GAP`
  (a Go duration, e.g. `300s`) overrides the default gap. `magefiles/ci_rerun_test.go` covers the
  target end to end, including the `GITHUB_OUTPUT` format and error paths.
- `.github/actions/classify-infra-failures/action.yml` (new composite action): inputs `run-id`,
  `run-attempt`, `repo`, `github-token`, `stuck-gap-seconds` (default `300`); outputs `verdict`,
  `reason`, `table`. Steps: `actions/setup-go` (pinned as in test.yml) → `gh api
  .../attempts/{n}/jobs --paginate > $RUNNER_TEMP/jobs.json` → `go tool mage
  ci:classifyInfraFailures` → a Markdown table plus the verdict in the step summary.
- `.github/workflows/rerun-infra-failures.yml` (new): `on: workflow_run` for `Tests`, top-level
  `permissions: {}`, job-level `actions: write` (rerun), `contents: read` (checkout),
  `pull-requests: read` (PR head lookup). Job-level `if` limits it to `failure`/`cancelled`
  conclusions, `pull_request`/`push` events and `run_attempt < 3`. harden-runner `block` with
  `api.github.com`, `github.com` and the Go module hosts test.yml's build job allows. Full checkout
  of the default branch (never the PR head), the classify action, then a rerun step gated on
  `verdict == 'rerun'` that re-checks the PR head SHA immediately before calling `gh run rerun
  "$RUN_ID" --failed`. Restricted to `pull_request` and `push` because a failed required check
  evicts a PR from the merge queue and a rerun does not re-enqueue it (comment in the workflow).
  A `gh run rerun` failure mentioning "already running"/"in progress" is a warning and exit 0; any
  other failure fails the job so it is visible.

## Validation

- `go build ./...`: clean.
- `go test ./internal/ci/rerun/... -cover`: `ok ... coverage: 98.9% of statements`.
- `go test -tags=mage ./magefiles/... -run 'TestClassifyInfraFailures|TestStuckGapOptions'`: ok
  (same invocation shape as test.yml's `[magefiles] unit tests` job, which runs the whole package
  with `-tags=mage -race`).
- `atmos lint --changed` (custom-gcl `--new-from-rev=origin/main`): clean after one godot
  rewording and three inline `//nolint:gosec` annotations on the GITHUB_OUTPUT/stderr writes
  (plain-text sinks, paths set by the Actions runner), matching the convention in
  `internal/ci/acceptance/coverage.go`.
- `actionlint .github/workflows/rerun-infra-failures.yml`: clean.
- `atmos validate editorconfig --affected --base origin/main`: passed. (The first push carried
  two space-indented `.sh` scripts that failed `[*.sh] indent_style = tab`; they were replaced by
  the Go implementation, so no shell scripts remain in the PR.)
- `go tool mage ci:classifyInfraFailures` against the full, untrimmed job listings of the four
  real runs:

  ```text
  == 33774798945 ==
  Build (windows)	cancelled	runner-stuck-after-complete
  [k3s] demo-helmfile	failure	check-cascade
  Acceptance Tests (windows)	failure	check-cascade
  Acceptance Tests (macos)	failure	check-cascade
  Acceptance Tests (linux)	failure	check-cascade
  verdict: rerun (all 5 non-success job(s) are infrastructure zombies or cascades of one)
  == 33769749251 ==
  Acceptance Tests (windows, shard 6/10)	cancelled	runner-stuck-after-complete
  Acceptance Tests (windows, shard 2/10)	cancelled	runner-stuck-after-complete
  Acceptance Tests (windows)	failure	check-cascade
  verdict: rerun (all 3 non-success job(s) are infrastructure zombies or cascades of one)
  == 33775397630 ==
  [magefiles] unit tests	failure	real-failure
  [mock-linux] examples/demo-atlantis	failure	real-failure
  verdict: no-rerun (no runner-stuck-after-complete or runner-lost jobs)
  == 33771957449 ==
  Acceptance Tests (linux, shard 4/10)	failure	real-failure
  Acceptance Tests (linux)	failure	check-cascade
  verdict: no-rerun (no runner-stuck-after-complete or runner-lost jobs)
  ```

- Does `gh run rerun --failed` re-queue cancelled jobs? Verified live on this PR's own `Tests`
  run 33788174036: attempt 1 was cancelled while queued (14 cancelled, 1 failed, 42 successful,
  29 never started); after `gh run rerun --failed`, attempt 2 listed the 43 completed jobs carried
  over with their original start times and re-queued the failed job, all 14 cancelled jobs and
  their never-started dependents (42 in progress/queued). Attempt 2 was then cancelled to free the
  runners. So the cancelled zombies are re-executed and the run is not restarted from scratch.
- `test-required` compatibility with `--failed` reruns: the attempt-2 job listing of run
  33778050850 (`.../attempts/2/jobs`) and the attempt-5 listing of run 33690277521 both return all
  92 jobs, including the carried-over successful ones (new job ids, `run_attempt` equal to the new
  attempt, original `started_at`), with 10 shards per OS. `test-required`'s `TEST_SHARD_COUNT`
  guard therefore passes on a rerun without changes; run 33751647765 attempt 2 is a real example
  of a `--failed` rerun going green.
- Not exercised: an end-to-end trigger. `workflow_run` workflows only run from the default
  branch, so the first live execution happens after merge; the decision logic was validated on
  the recorded job JSON above instead. The "already running" text of `gh run rerun` was taken
  from the gh source (`run %d cannot be rerun; <API message>`) and not reproduced live.

## Follow-ups

- Reuse `internal/ci/rerun` in test.yml's `test-required` aggregation to write the same class
  table to the step summary (plan Phase 5). Tracking issue to be opened by a maintainer; none was
  opened here.
- Fork PRs: `workflow_run.pull_requests` is empty for forks, so those runs are never rerun
  (logged as "cannot verify it is current"). Acceptable for now; revisit if fork PRs become common.
