# Fix: Refresh one PR with trusted quality reporting

**Date:** 2026-11-09

## Summary

Add an explicit single-PR quality refresh and remove `pull_request_target` from
the quality-report, quality-reconcile and release-milestones workflows. Privileged
reporters continue to execute default-branch tooling; PR activity reaches them
through the unprivileged signal workflow. Separate timing cancellation from gate
evaluation and expose a machine-readable result for agents pursuing a quality goal.

## Context

Manual quality dispatch previously selected every open PR. Recovering one report
therefore required a repository-wide true-up. Benchmark recovery could also call
the publisher for other PRs sharing the selected PR's head commit.

The concurrency history matters: [PR #3367](https://github.com/cloudposse/atmos/pull/3367)
removed a global `queue: max` after bursts filled its queue, delayed reports for
hours, and failed overflow runs. Simply restoring that queue is not a fix for
cancelled-run noise. Release milestone enforcement also writes every open PR's
and merge group's statuses, so splitting its lock by triggering PR would allow
competing repository-wide writers.

The [fork signal fix](2026-10-31-quality-gate-signal-approval-blocker.md) documents
another constraint: unprivileged fork signals may await approval. Replacing a
privileged PR event with a signal does not remove that limitation.

## Changes

- Add required `pr_number` to manual `quality-report.yml` dispatch and the Mage
  `quality:reportPR <repo> <number>` entrypoint. Reject noncanonical or nonpositive
  integers before making API calls. Explicit refresh ignores the event payload
  and cannot fall back to all open PRs.
- Restrict benchmark-recovery publication to the selected PR, even when the
  benchmark commit is associated with multiple PRs. Preserve existing live
  head/base checks, bounded artifact handling, and unavailable-evidence behavior.
- Route PR lifecycle, label and milestone signals through `pull_request` with
  no permissions, checkout or secrets. The trusted completion coordinator still
  evaluates live evidence instead of trusting a signal's reported verdict.
- Wake globally serialized release reconciliation from PR signal completions;
  review signals do not run release enforcement or previews. Keep the separate
  merge-group trigger and scheduled/manual recovery.
- Trigger quality and release true-ups on pushes to main, replacing the
  privileged PR-close trigger. Exclude those push-triggered reporter runs from
  quality evidence.
- Restrict manual quality dispatch to the default branch. Keep all reporter
  checkouts explicitly on the repository's default branch with persisted Git
  credentials disabled.
- Move timing cancellation to its job and remove quality's dependency on timing.
  Both jobs admit the same PR and signal completions, including fork review
  signals without PR associations. Keep per-PR pending replacement and global
  milestone serialization; do not reintroduce the queue that overflowed.
- Add a stable version-1 JSON snapshot to the trusted bot report and a read-only
  `quality:status` command. Missing or mismatched snapshots return pending. Check
  mutable PR metadata as well as head/base, detect same-head reruns, and use the
  existing CI supersession logic before returning ready. API errors emit no verdict.
- Upload the JSON result after successful single-PR refresh. Document how agents
  should use the revision-matched gate result instead of requiring every reporter
  run to succeed. Snapshots do not re-evaluate all reviews or release metadata;
  refresh before a final decision when these signals change.
- Add `atmos dev quality refresh <PR>` and `atmos dev quality status <PR>`.
  Refresh dispatches the main-branch workflow; status prints JSON through the
  local helper. Starlark passes arguments directly to executables without shell
  interpolation, and rejects invalid PR numbers before starting either tool.

## Validation

- Quality and release package tests pass, including malformed input, missing PR,
  event-independent single-PR selection, shared-head benchmark recovery, and
  reporter-run exclusion.
- Workflow trust-boundary tests check that the three privileged workflows have
  no PR event triggers, deny permissions by default, and check out only trusted
  tooling; the signal has no permissions or checkout and only runs `true`.
- `actionlint` passes for all five changed workflows; `git diff --check` passes.
- `go vet -tags=mage ./magefiles/qualitygate ./magefiles/release/...` passes;
  `atmos lint --changed` reports zero issues. `go tool mage -l` exposes the new
  `quality:reportPR` command.
- Result tests cover schema/metadata mismatches, spoofed comments, same-head
  reruns, cancelled reporters versus cancelled CI, external pending statuses,
  concurrent head changes, API failures and JSON-only stdout. Workflow tests
  prohibit workflow-wide timing cancellation and a timing dependency for quality.
- Exercised both `atmos dev quality` commands with stubbed executables: exact
  dispatch/status arguments, JSON-only status stdout, required/invalid argument
  rejection, and underlying tool failure propagation pass. Command help renders;
  no hosted workflow was dispatched by this validation.

## Follow-ups

The workflow changes take effect after landing on main. Then run:

```sh
gh workflow run quality-report.yml --ref main -f pr_number=1234
```

This updates evidence and readiness, not CI execution. Missing/API-inaccessible
PRs fail; closed PRs are successful no-ops. Fork approval delays and merge
conflicts may suppress signals. Manual refresh remains available; scheduled
quality recovery retains its bounded selection rules and does not guarantee
immediate detection of every metadata change on old PRs.

Pending replacement still produces cancelled runs; it no longer cancels an active
gate via timing or defines the agent-facing verdict. Verify sibling-job cancellation
behavior on hosted Actions after merge. Other workflows using `pull_request_target`
are outside this quality/release change.
