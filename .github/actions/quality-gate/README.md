# Quality Gate

Runs the Mage quality reporter from trusted default-branch code.

- `operation: report` (`quality:report`) refreshes only the PRs the event names:
  a release label or milestone change, or a CI completion for their head. It skips
  repository-wide events and never falls back to every open PR.
- `operation: reconcile` (`quality:reconcile`) trues up PRs after a schedule tick,
  a push to main, a milestone or release change, or a manual dispatch.
- `operation: reportPR` with `pr-number` (`quality:reportPR <repo> <number>`) refreshes
  exactly one PR from live state, independently of the event. Invalid numbers
  fail; missing PRs/API failures fail; closed PRs are successful no-ops. Benchmark
  recovery cannot publish to sibling PRs sharing the same commit.

After this workflow is on main, refresh one PR with:

```sh
atmos dev quality refresh 1234
```

This wraps `gh workflow run quality-report.yml --repo cloudposse/atmos --ref main
-f pr_number=1234` and returns after dispatch; it does not wait for the report.
Wait for that refresh to complete before using its result for a final decision.
The status command can still show the previous snapshot while a refresh is queued.

This refreshes evidence, comments and readiness labels; it does not rerun CI or
override a failing gate. The Actions UI offers the same required input. Dispatch
from the default branch; other refs are skipped. Full manual true-ups still use
`quality-reconcile.yml` with no PR input.

Each successful manual refresh uploads `quality-result.json` in an artifact named
`quality-result-<PR>-<run ID>-<attempt>`. Its schema version is `1`, with
`repository`, `pr_number`, `head_sha`, `base_sha`, `base_ref`, `revision`, `state`,
and an optional `reason`. `state` is `pending`, `ready`, or `blocked`. A blocked
verdict is a successful refresh; API and publication errors fail the operation
and produce no success artifact. Closed PRs return pending with reason
`pr_not_open_or_eligible`; stop waiting for those PRs.

For agents working toward a quality goal, read the same contract from a trusted
checkout containing this tooling:

```sh
atmos dev quality status 1234
```

This wraps `go tool mage quality:status cloudposse/atmos 1234`.

This read-only command prints exactly one JSON object. It reads only the marked
`github-actions[bot]` report and verifies its repository, PR, head/base revision,
draft state, merge SHA, release labels and milestone. Missing, malformed, old
schema or mismatched snapshots return pending. It checks for active same-head
reruns and rechecks live CI before returning a published ready verdict. API
errors exit nonzero without a result. The structured snapshot is embedded in the
sticky comment, so a cancelled replacement reporter does not erase it.

An agent pursuing the goal should:

1. Refresh after a push, CI completion or relevant metadata change. Wait for the
  requested refresh to complete, then read its result.
2. Require the expected repository, PR and head SHA and `state: ready` before
  declaring the goal reached. If the head changes, restart that check.
3. For `pending`, wait for work in progress or request a single-PR refresh if the
  snapshot is missing/stale. For `blocked`, use the human-readable gate comment
  to address the blockers. If the reason is `ci_changed`, refresh the comment
  first so its details reflect the current CI result.
4. Treat a superseded reporter run as scheduling, not failed CI. Do not rerun
  successful tests just to clear a reporter cancellation. Current cancelled
  tests still block; only proven superseded test runs are ignored.

This is a published evaluation with revision/CI freshness checks, not a live
re-evaluation of every review or repository-wide release change. Refresh before
the final decision when those signals may have changed. Do not use workflow
success alone as the verdict, or an old downloaded artifact as a live status.

PR lifecycle, release-label, milestone and review activity goes through the
unprivileged, checkout-free `Quality Gate Signal`, then the default-branch
`workflow_run` coordinator. Quality and release reporters do not subscribe to
`pull_request_target`. Fork approval or merge conflicts can prevent a signal;
manual refresh and scheduled reconciliation remain available. Scheduled quality
recovery retains its existing selection rules, so it is not a guarantee that
every metadata change on an old PR will be detected immediately.

The two operations run in separate workflows (`quality-report.yml` and
`quality-reconcile.yml`) with separate concurrency groups, so a long true-up never
cancels a queued per-PR report. The reporter ignores both workflows' own runs as CI
evidence.

Keep the existing replace-pending policy: #3367 removed a global `queue: max`
after it overflowed and reports waited hours. Timing cancellation is now scoped
to its job; quality is an independent sibling with no timing dependency and does
not cancel an active report. Pending quality jobs can still be superseded. The
repository-wide milestone writer remains globally serialized.

Tests: `go test -tags=mage ./magefiles/qualitygate/...`.
