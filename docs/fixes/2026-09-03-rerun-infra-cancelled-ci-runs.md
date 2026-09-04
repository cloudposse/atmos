# Fix: auto-rerun `Tests` runs whose only failures are infrastructure zombies

**Date:** 2026-09-03

## Summary

Added `.github/workflows/rerun-infra-failures.yml`, a `workflow_run` backstop that re-runs a
failed or cancelled `Tests` (test.yml) run when every non-success job is an infrastructure zombie
(a runner that finished all its steps but never reported completion, or a runner that vanished) or
a `Check ... result` aggregator that failed only because of one. Any job with a genuinely failed
step vetoes the rerun. Both the classification and the rerun decision are typed, unit-tested Go in
`internal/ci/rerun`, dispatched through `go tool mage ci:classifyInfraFailures`/`ci:rerunInfraFailures`
(the former via the local composite action `.github/actions/classify-infra-failures`), following
the pattern the acceptance orchestration already uses (`magefiles/acceptance.go`,
`internal/ci/acceptance`). All GitHub REST calls - listing a run attempt's jobs, reading a pull
request's head SHA, and requesting a rerun - go through `github.com/cli/go-gh/v2`'s REST client
(the library `gh` itself is built on) instead of shelling out to the `gh` binary, so the only shell
left in either the composite action or the workflow is a single `go tool mage ...` invocation per
step.

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
  (`DecodeJobs` accepts one `{"jobs":[...]}` page, several concatenated pages, or a bare array;
  `encoding/json` parses the RFC 3339 timestamps with or without fractional seconds).
  `Classify(jobs, Options)` assigns one class per job whose conclusion is not success/skipped/empty:
  `runner-stuck-after-complete` (cancelled, every step completed with success/skipped, job
  completion at least `Options.StuckGap`, default 5 minutes, after the last step), `superseded`
  (any other cancellation, including mid-step cancels and never-started jobs), `runner-lost`
  (failure with no failed step), `check-cascade` (failure whose failed steps all match
  `^Check .* result$`), `real-failure` (everything else). `Verdict(classified)` returns `rerun`
  only when at least one job is stuck/lost and every other one is check-cascade, `no-jobs` when
  nothing failed, `no-rerun` otherwise, with a reason string. `WriteTSV` renders
  `job\tconclusion\tclass`; `WriteMarkdownSummary` renders the same classification as a GitHub
  Actions step-summary block (header linking to the run, Markdown table, verdict line). This file
  stays HTTP-free on purpose, so the classification rules are unit-testable against recorded API
  payloads with no network or mocking involved.
- `internal/ci/rerun/fetch.go` + `actions.go` (new): the package's only network code, behind a
  narrow `RESTClient` interface (`RequestWithContext(ctx, method, path, body) (*http.Response,
  error)`, satisfied by `*api.RESTClient` from `github.com/cli/go-gh/v2/pkg/api` - the library
  `gh` itself is built on, so auth resolves from `GH_TOKEN`/`GITHUB_TOKEN`/gh config exactly as it
  would for the CLI). `FetchJobs` pages `repos/{repo}/actions/runs/{id}/attempts/{n}/jobs`
  (per_page=100), following the `Link: rel="next"` header the way `gh api --paginate` does.
  `PRHeadSHA` reads a pull request's current head commit. `RerunFailedJobs` POSTs
  `repos/{repo}/actions/runs/{id}/rerun-failed-jobs` (what `gh run rerun --failed` calls
  internally) and reports `stillRunning=true` instead of an error when the API's `*api.HTTPError`
  message matches "already running"/"in progress" - the run hasn't finished yet, which isn't a
  failure worth surfacing as one. `mock_gh.go` is a `go.uber.org/mock/mockgen`-generated
  `MockRESTClient`; `fetch_test.go`/`actions_test.go` cover pagination, PR lookup and all three
  rerun outcomes (success, still-running, genuine API error) against it, no live API involved.
- `internal/ci/rerun/classify_test.go` + `testdata/run-<id>.json` (new): table-driven tests for
  every class, the gap boundary (exactly 5 min is stuck, one second less is superseded), custom
  `StuckGap`, missing timestamps, running/success/skipped jobs omitted, paginated and bare-array
  decoding with error cases, every `Verdict` branch, `WriteMarkdownSummary`'s rendering, and the
  four real runs above (fixtures trimmed to their non-success jobs plus two successful ones).
  Package coverage: 98.7%.
- `magefiles/ci_rerun.go` (new, `//go:build mage`): `CI.ClassifyInfraFailures(repo, runID,
  runAttempt)` builds a `go-gh` REST client, fetches the run's jobs itself via
  `rerun.FetchJobs`, prints the TSV table to stdout and `verdict: <outcome> (<reason>)` to stderr;
  when `GITHUB_OUTPUT` is set it appends `verdict=`, `reason=` and a multi-line `table` output;
  when `GITHUB_STEP_SUMMARY` is set it also appends `rerun.WriteMarkdownSummary`'s rendering.
  `CI.RerunInfraFailures(repo, runID, event, headSHA, prNumbers)` is the Go port of the former
  shell "Rerun failed jobs" step: for `pull_request` runs it re-verifies every associated PR
  (space-separated `prNumbers`) is still at `headSHA` via `rerun.PRHeadSHA`, refusing to rerun a
  superseded run or one with no associated PR (a fork PR); otherwise it calls
  `rerun.RerunFailedJobs` and appends the outcome to `GITHUB_STEP_SUMMARY`. Only a genuine API
  failure is returned as an error (and annotated `::error::`); "still finishing" is a `::warning::`
  and a normal (non-error) skip. `ATMOS_CI_STUCK_GAP` (a Go duration, e.g. `300s`) overrides the
  classifier's default gap. `magefiles/ci_rerun_test.go` covers both targets' testable cores
  end to end (against a mocked `RESTClient`, same as the `internal/ci/rerun` tests) - every branch
  of the rerun decision, the `GITHUB_OUTPUT`/`GITHUB_STEP_SUMMARY` formats, and error paths. The
  two `(CI)` methods themselves are thin wrappers (build a client, delegate) left uncovered by
  design, matching `magefiles/acceptance.go`'s existing `(CI)` targets in this same package.
- `.github/actions/classify-infra-failures/action.yml`: down to two steps - `actions/setup-go`
  (pinned as in test.yml) and a "Classify non-success jobs" step that is now nothing but
  `go tool mage ci:classifyInfraFailures "$repo" "$run-id" "$run-attempt"`; the mage target fetches
  the jobs, classifies them, and appends the Markdown summary itself, so no shell step reads or
  reshapes any data anymore. Inputs/outputs (`run-id`, `run-attempt`, `repo`, `github-token`,
  `stuck-gap-seconds`; `verdict`, `reason`, `table`) are unchanged.
- `.github/workflows/rerun-infra-failures.yml`: `on: workflow_run` for `Tests`, top-level
  `permissions: {}`, job-level `actions: write` (rerun), `contents: read` (checkout),
  `pull-requests: read` (PR head lookup). Job-level `if` limits it to `failure`/`cancelled`
  conclusions, `pull_request`/`push` events and `run_attempt < 3`. harden-runner `block` with
  `api.github.com`, `github.com` and the Go module hosts test.yml's build job allows - unchanged,
  since the REST calls this now makes in Go go to the same `api.github.com` the shell's `gh`
  calls did. Full checkout of the default branch (never the PR head), the classify action, then a
  "Rerun failed jobs" step gated on `verdict == 'rerun'` that is now a single
  `go tool mage ci:rerunInfraFailures "$repo" "$run_id" "$event" "$head_sha" "$pr_numbers"` call.
  Restricted to `pull_request` and `push` because a failed required check evicts a PR from the
  merge queue and a rerun does not re-enqueue it (comment in the workflow).
- `go.mod`/`go.sum`: added `github.com/cli/go-gh/v2` (direct) and its transitive dependencies;
  `go mod tidy` bumped `github.com/alecthomas/chroma/v2` from v2.24.1 to v2.27.0 (an existing
  indirect dependency of this repo, forced to go-gh's minimum required version by Go's module
  graph - unrelated to this change otherwise).

## Validation

- `go build ./...`: clean.
- `go test ./internal/ci/rerun/... -cover -race`: `ok ... coverage: 98.7% of statements`.
- `go test -tags=mage ./magefiles/... -run 'TestClassifyInfraFailures|TestRerunInfraFailures|TestStuckGapOptions'`:
  ok (same invocation shape as test.yml's `[magefiles] unit tests` job, which runs the whole
  package with `-tags=mage -race`).
- `go vet -tags=mage ./magefiles/...`: clean.
- `atmos lint --changed` (custom-gcl `--new-from-rev=origin/main`): clean after fixing an
  argument-count violation (`revive`'s `argument-limit: 5`) by bundling `classifyInfraFailures`'s
  and `WriteMarkdownSummary`'s optional inputs into small structs (`classifyConfig`, `rerun.RunRef`),
  a `hugeParam` finding (`gocritic`) by passing `*classifyConfig` by pointer, an unused
  `//nolint:gosec` (`nolintlint`) once `gosec` stopped flagging that line, and one godot rewording.
- `actionlint` on both `.github/workflows/rerun-infra-failures.yml` and (indirectly, via the
  workflow that consumes it) `.github/actions/classify-infra-failures/action.yml`: clean.
- `atmos validate editorconfig --affected --base origin/main`: passed.
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
  the recorded job JSON above instead. `rerun.RerunFailedJobs`'s "still running" detection matches
  `stillRunningPattern` (`(?i)already running|in progress`) directly against `*api.HTTPError.Message`
  - the raw GitHub API error text, not `gh run rerun`'s own wrapping - which was not reproduced
  live either; the pattern is the same one the original shell step grepped for.

## Follow-ups

- Reuse `internal/ci/rerun` in test.yml's `test-required` aggregation to write the same class
  table to the step summary (plan Phase 5). Tracking issue to be opened by a maintainer; none was
  opened here.
- Fork PRs: `workflow_run.pull_requests` is empty for forks, so those runs are never rerun
  (logged as "cannot verify it is current"). Acceptable for now; revisit if fork PRs become common.
