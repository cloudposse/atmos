# Fix: `test-required`/`k3s-required` reported failure instead of passing through on a cancelled run

**Date:** 2026-08-31

## Summary

Five attached CI failure logs (`Acceptance Tests (linux/macos/windows)`, `[k3s] demo-helmfile`,
`Build (windows)`) all turned out to be one event: workflow run `33394180592` on PR #2878
(`head_sha: 0a8e4e4974`, this branch) was **cancelled**, not failed -- confirmed via `gh api
repos/cloudposse/atmos/actions/runs/33394180592` (`conclusion: cancelled`) and per-job status
(`Build (windows)`: `cancelled`; the `test` matrix template job: `skipped`). The two `-required`
gate jobs still ran (`if: always()`) and mis-reported the cancellation as a genuine failure:

```text
expected 10 'linux' shard jobs, found 0
##[error]Process completed with exit code 1.
```

```text
k3s matrix result was 'skipped'
##[error]Process completed with exit code 1.
```

## Context

`test-required` and `k3s-required` gate the sharded `test`/`k3s` matrix jobs and intentionally use
`if: always()` so the required check always resolves rather than staying pending forever. When the
whole workflow run is cancelled (this run's `head_sha` matches a commit pushed mid-session; the very
next push, made shortly after, is the likely trigger), GitHub skips the `test`/`k3s` matrix before
it ever dispatches jobs -- but the gate jobs still execute and, finding 0 shard jobs (or
`needs.k3s.result == 'skipped'`), correctly-by-their-own-logic-but-misleadingly report a hard
failure. `test-required`'s own comment already documents a deliberate design choice to "fail loudly
... rather than silently treating a missing shard as a pass" for genuine anomalies (a renamed job,
an API hiccup, or a real upstream `build` failure that legitimately produces the same "skipped"
result) -- that property needed to stay intact, so the fix could not simply treat any "skipped"/
non-success result as acceptable.

`needs.test.result` and `needs.k3s.result` both report `"skipped"` for two different situations
that must be told apart: a genuine upstream failure (should still fail loudly) and a whole-run
cancellation (should not). The `cancelled()` expression function distinguishes them -- it reflects
the run's own cancellation state, not any specific job's result -- so it's the correct signal here.
`cancelled()` is only valid in a job or step `if:`, not inside an interpolated `run:` script
(confirmed by `actionlint`), so the fix uses step-level `if: ${{ !cancelled() }}` guards plus a
small explicit "Skip verification" step for a clear log line, rather than an early `exit 0` inside
the existing script bodies.

No other workflow in `.github/workflows/` has a `concurrency:` block referencing `test.yml`'s jobs,
and `test.yml` itself declares none -- the cancellation was not GitHub's automatic
`concurrency.cancel-in-progress` behavior (that requires an explicit `concurrency:` key, which this
workflow doesn't have). The exact cancelling actor (manual, Mergify, or another integration) wasn't
identified and doesn't change the fix; a run being superseded by a newer push to the same PR is
normal and expected regardless of the mechanism.

## Changes

`.github/workflows/test.yml`:

- `test-required`: added a "Skip verification (workflow run was cancelled)" step
  (`if: ${{ cancelled() }}`) and gated both existing check steps ("Check per-OS test matrix result",
  "Check terraform-registry-cache result") with `if: ${{ !cancelled() }}` (the latter combined with
  its existing `matrix.check != 'Acceptance Tests (macos)'` condition), so a cancelled run resolves
  as a passing job whose verification steps are skipped, instead of a hard failure. The per-shard/API-based
  verification logic itself is unchanged.
- `k3s-required`: same pattern -- an explicit skip step plus `if: ${{ !cancelled() }}` on the
  existing "Check k3s matrix result" step.

No changes to `test`, `k3s`, `build`, or any other job -- this only affects how the two gate jobs
report a cancellation that already happened upstream.

## Validation

- `python3 -c "import yaml; yaml.safe_load(open('.github/workflows/test.yml'))"` -- valid YAML.
- `actionlint .github/workflows/test.yml` -- 0 issues (an earlier draft using `${{ cancelled() }}`
  directly inside a `run:` script was caught and corrected by this check: "calling function
  'cancelled' is not allowed here").
- Confirmed via `gh api` that all five attached failures share `run_id: 33394180592`, and that this
  run's own conclusion is `cancelled` (not `failure`), ruling out a genuine test/build regression.
- Not exercised end-to-end (no way to trigger and cancel a real Actions run from this session); the
  fix is a narrow, `actionlint`-verified expression-logic correction with an unambiguous mechanism.

## Follow-ups

None.
