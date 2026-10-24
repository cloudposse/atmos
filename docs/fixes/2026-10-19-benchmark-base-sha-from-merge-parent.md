# Fix: Benchmarks record the measured base from the merge revision, not the event payload

**Date:** 2026-10-19

## Summary

The Quality Gate's "Benchmark results" check failed on PR #3411 with "measured merge revision does not match the recorded base and PR head" even though the Benchmarks workflow itself succeeded. The benchmark manifest recorded `github.event.pull_request.base.sha` as the base, and that payload value lagged the real base branch by over an hour. The workflow now records the merge revision's first parent, which is the main commit that was actually measured.

## Context

The reporter in `magefiles/bench/bench_github.go` validates a pull-request artifact by fetching the measured merge commit from GitHub and requiring its first parent to equal the manifest's `BaseSHA` and its second parent to equal the run's head SHA. For run 39196516326 the merge commit `c0759b698c` had parents `f400a34a95` (main at 02:50 UTC) and `8b8b9ccb92` (the PR head), but the manifest's `BaseSHA` was `323db151b8`, main as of 01:36 UTC. The measurement was correct; only the recorded base was stale.

The stale value came from `ATMOS_BENCH_BASE_SHA: ${{ github.event.pull_request.base.sha || github.sha }}` in `.github/workflows/benchmarks.yml`. GitHub refreshes `pull_request.base.sha` in event payloads lazily. This PR is pushed from a Conductor worktree, whose pushes never emit `synchronize`, so its workflows are triggered by closing and reopening the PR, and the `reopened` payload still carried the old base. PR #3413 hit the same rejection in the same gate run. Other open PRs that receive normal `synchronize` events passed.

## Changes

Single file, `.github/workflows/benchmarks.yml`:

- Add a `Record the measured base revision` step after checkout, for `pull_request` events only, that reads the first `parent` line of `git cat-file -p HEAD` and writes it to `ATMOS_BENCH_BASE_SHA` in `GITHUB_ENV`. `HEAD^1` does not resolve on the depth-1 checkout, but `cat-file` prints the raw commit object including its parents. The step fails loudly if the value is not a 40-character SHA.
- Keep the job-level `ATMOS_BENCH_BASE_SHA` expression as the `push` value and as a placeholder for pull requests, with a comment explaining the override.

No change to the reporter or its validation: the first parent of the merge revision is exactly what the validation compares against, so a correct manifest now passes by construction while a tampered or mismatched artifact is still rejected.

## Validation

- `actionlint` with shellcheck passed on the workflow file.
- `git cat-file -p HEAD | sed -n 's/^parent //p'` was verified against a fresh `--depth 1` clone of this repository, where it printed the parent SHA while `git rev-parse HEAD^1` failed with "unknown revision".
- The gate itself only re-evaluates on a new pull-request Benchmarks run of this PR, so the fix is confirmed when the Quality Gate comment on PR #3411 shows "Benchmark results" passed for a head that includes this change.

## Follow-ups

The Quality Gate also counts check-runs left by a duplicate `test.yml` run that the concurrency group cancelled, because GitHub's `filter=latest` hides only older attempts of the same run. That is a separate limitation in `magefiles/qualitygate/quality_checks.go`, worked around on this PR by re-running the cancelled duplicate, and is not addressed here.
