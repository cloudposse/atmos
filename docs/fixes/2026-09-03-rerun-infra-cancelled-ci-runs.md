# Fix: auto-rerun `Tests` runs whose only failures are infrastructure zombies

**Date:** 2026-09-03

## Summary

Added `.github/workflows/rerun-infra-failures.yml`, a `workflow_run` backstop that re-runs a
failed or cancelled `Tests` (test.yml) run with `gh run rerun --failed` when every non-success job
is an infrastructure zombie (a runner that finished all its steps but never reported completion,
or a runner that vanished) or a `Check ... result` aggregator that failed only because of one. Any
job with a genuinely failed step vetoes the rerun. The classification lives in a standalone,
self-tested script, `.github/scripts/classify-infra-failures.sh`, so `test-required` can reuse it.

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

- `.github/workflows/rerun-infra-failures.yml` (new): `on: workflow_run` for `Tests`, top-level
  `permissions: {}`, job-level `actions: write` only, harden-runner `block` with
  `api.github.com:443` and `github.com:443` (checkout of the script from the default branch,
  never the PR head). Skips unless the conclusion is `failure`/`cancelled`, unless
  `run_attempt < 3`, and for `pull_request` runs unless `head_sha` is still the PR head. Restricted
  to `pull_request` and `push`: a failed required check evicts a PR from the merge queue and a
  rerun does not re-enqueue it, so `merge_group` runs are out of scope (comment in the workflow).
  Prints a job/conclusion/class table to the log and `$GITHUB_STEP_SUMMARY`; reruns only when
  there is at least one `runner-stuck-after-complete` or `runner-lost` job and nothing else but
  `check-cascade`. A `gh run rerun` failure mentioning "already running"/"in progress" is a
  warning and exit 0; any other failure fails the job so it is visible.
- `.github/scripts/classify-infra-failures.sh` (new): reads the paginated `.../attempts/{n}/jobs`
  JSON from stdin, prints TSV `job\tconclusion\tclass`. Classes: `runner-stuck-after-complete`
  (cancelled, all steps success/skipped, job completion at least `STUCK_GAP_SECONDS` (default 300)
  after the last step), `runner-lost` (failure, no failed step), `check-cascade` (failure whose
  failed steps all match `^Check .* result$`), `superseded` (any other cancellation, including
  jobs cancelled mid-step or never started), `real-failure` (everything else). Uses a
  `def ts: sub("\\.[0-9]+Z$"; "Z") | fromdate;` helper so fractional-second timestamps parse.
- `.github/scripts/classify-infra-failures-test.sh` (new): plain-bash self-test with one fixture
  per class plus ordinary-cancel, never-started, aggregator-plus-real-step, configurable gap and
  paginated-input cases.

## Validation

- Self-test (`bash .github/scripts/classify-infra-failures-test.sh`):

  ```text
  PASS  runner-stuck-after-complete  Build (windows)
  PASS  superseded                   Build (windows)
  PASS  runner-lost                  Acceptance Tests (macos, shard 4/10)
  PASS  check-cascade                Acceptance Tests (windows)
  PASS  superseded                   Acceptance Tests (linux, shard 2/10)
  PASS  superseded                   Acceptance Tests (linux, shard 3/10)
  PASS  real-failure                 Acceptance Tests (linux, shard 4/10)
  PASS  real-failure                 [k3s] demo-helmfile
  PASS  superseded                   Build (windows) STUCK_GAP_SECONDS=10
  PASS  runner-stuck-after-complete  Build (windows) STUCK_GAP_SECONDS=3
  PASS  paginated input, success omitted
  all assertions passed
  ```

- Classifier against real runs (`gh api .../attempts/1/jobs --paginate | bash .github/scripts/classify-infra-failures.sh`):
  33774798945 → `Build (windows)` runner-stuck-after-complete, four check-cascade (would rerun);
  33769749251 → two runner-stuck-after-complete shards plus one check-cascade (would rerun);
  33775397630 → two real-failure (no rerun); 33771957449 → one real-failure plus one
  check-cascade (no rerun, correctly).
- `actionlint .github/workflows/rerun-infra-failures.yml`: clean.
- `shellcheck` on both scripts: clean.
- `test-required` compatibility with `--failed` reruns, checked on GitHub: the attempt-2 job
  listing of run 33778050850 (`.../attempts/2/jobs`) and the attempt-5 listing of run 33690277521
  both return all 92 jobs, including the carried-over successful ones (new job ids, `run_attempt`
  equal to the new attempt, original `started_at`), with 10 shards per OS. `test-required`'s
  `TEST_SHARD_COUNT` guard therefore passes on a rerun without changes; run 33751647765 attempt 2
  is a real example of a `--failed` rerun going green.
- Not exercised: an end-to-end trigger. `workflow_run` workflows only run from the default
  branch, so the first live execution happens after merge; the decision logic was validated on
  the recorded job JSON above instead. The "already running" text of `gh run rerun` was taken
  from the gh source (`run %d cannot be rerun; <API message>`) and not reproduced live, since
  that would have rerun someone's PR.

## Follow-ups

- Reuse `classify-infra-failures.sh` in test.yml's `test-required` aggregation to write the same
  class table to the step summary (plan Phase 5). Tracking issue to be opened by a maintainer;
  none was opened here.
- Fork PRs: `workflow_run.pull_requests` is empty for forks, so those runs are never rerun
  (logged as "cannot verify it is current"). Acceptable for now; revisit if fork PRs become common.
