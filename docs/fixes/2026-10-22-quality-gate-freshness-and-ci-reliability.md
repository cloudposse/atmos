# Fix: Keep quality reports current without repeated pending updates

**Date:** 2026-10-22

## Summary

[PR #3476](https://github.com/cloudposse/atmos/pull/3476) fixes stale quality
verdicts, adds report sections and a job-summary link, and addresses the CI
failures encountered while validating those changes. The reporter publishes one
pending snapshot for the current head/base revision, waits for CI completion
before publishing the full report, and avoids edits caused only by a new
reporter run's summary URL.

## Context

- On [PR #3426](https://github.com/cloudposse/atmos/pull/3426), an older failed
  semver-label check overwrote a newer successful check under the same app/name
  key. The checks API's `filter=latest` still returned results from multiple
  check suites, newest first.
- [PR #3457](https://github.com/cloudposse/atmos/pull/3457) retained a ready report
  while current tests failed. Both the timing coordinator and the reporter
  deferred quality publication until all workflows completed, allowing stale
  readiness to survive a push or rerun.
- PR #3476 initially had no quality comment for the same reason. The
  [scheduled reporter](https://github.com/cloudposse/atmos/actions/runs/39790514328)
  explicitly deferred it at 13:56 UTC. Reporters load trusted code from `main`,
  so unmerged reporter changes cannot affect their own PR automatically.
- Refreshing on every workflow completion would create unnecessary comment
  edits as pending jobs finished. A different job-summary URL on each reporter
  run also made otherwise identical reports compare unequal.
- The attached CI logs showed an
  [OpenTofu signature download returning HTTP 502](https://github.com/cloudposse/atmos/actions/runs/39775051378/job/119812106486),
  a [blocked Windows artifact storage host](https://github.com/cloudposse/atmos/actions/runs/39776523423/job/119825363021),
  live latest-release API 404s on
  [Linux](https://github.com/cloudposse/atmos/actions/runs/39782257359/job/119844161169)
  and [macOS](https://github.com/cloudposse/atmos/actions/runs/39782257359/job/119844160649),
  and [three refused internal artifact API connections](https://github.com/cloudposse/atmos/actions/runs/39782257359/job/119844159899)
  before demo linting could start.
- The [consumer contract job](https://github.com/cloudposse/atmos/actions/runs/39799519472/job/119897869185)
  failed before running tests when the Pact native library download was reset by
  GitHub's release-assets host. The pinned installer creates the destination
  file before downloading and can accept that leftover file on a normal retry.

- The [Floci Go E2E job](https://github.com/cloudposse/atmos/actions/runs/39805844063/job/119953543543)
  failed during APT installation, before tests ran. Azure's Ubuntu mirror returned
  HTTP 503s and closed connections. The existing direct-URL replacement did not
  match the runner's `mirror+file:/etc/apt/apt-mirrors.txt` source, and its HTTPS
  fallback archives were outside this job's network allowlist.

## Changes

- Select the newest result per app/name across all check pages, including
  Codecov checks, by `started_at` rather than API response order. Check IDs
  break timestamp ties; if any duplicate lacks a start time, use IDs for that
  entire group so selection remains independent of page order. Check-run
  responses do not expose the `created_at` field suggested in review.
- Allow quality reconciliation when the timing coordinator succeeds without
  publishing final timing data. Publish an initial pending snapshot and clear
  stale readiness once, then retain that snapshot while CI remains incomplete.
  A new head, base commit, or target branch requires a new snapshot. A same-head
  rerun replaces a previously final report with a pending snapshot.
- Keep pending snapshots eligible for scheduled recovery beyond the 24-hour
  recent-commit window. Also check for active current-head workflows before
  applying the age cutoff, recovering same-head reruns whose events were missed.
  Empty workflow listings do not keep inactive old PRs selected. Publish final evidence after CI completes, including
  failures, and continue reconciling later review and external-check changes.
- Compare report content independently of its reporter-run summary URL. Keep
  the existing link when evidence is unchanged, while still producing a job
  summary on each publishing pass. Genuine report changes get the new link.
- Update the reporter README to describe pending snapshots, recovery selection,
  check ordering, and summary-link deduplication.
- Group the report into Readiness, Reviews, and Measures; link to the existing
  Actions job summary only when running in a summary-producing invocation.
- Use the shared toolchain action's bounded installation retry for race shards,
  retaining signature verification and restore-only caching.
- Allow GitHub's Azure artifact storage hosts in the multi-OS test jobs; the
  artifact account can differ from the cache account detected by runner hardening.
- Download demo lint build artifacts through the supported REST path, scoped to
  the current workflow run with an `actions: read` token. Allow the Azure storage
  redirect hosts in this job too, so runner hardening permits the REST download.
  Retain bounded retries.
- Run the complete consumer contract suite locally and in CI with
  `atmos test pact`. It shares installation with `atmos test pact-install`, using
  native retries (three attempts with 10-second delays) and a five-minute install
  timeout. The local default is `build/pact`; CI passes
  `--lib-dir "$RUNNER_TEMP/pact"`. Forced downloads prevent partial libraries
  from satisfying the installer's existence check. The command enables CGO and
  configures absolute linker and Linux/macOS loader paths before running tests.
  Exhausted retries fail the job; contract test failures are not retried.
- Rewrite both direct Azure and GitHub mirror-list APT sources to the allowed
  primary HTTP Ubuntu archive in the Floci and race dependency setup steps.
  Keep APT's bounded retries, timeouts, and package signature verification.
- Replace live latest-release tests with local API fixtures and a TLS server
  that checks authenticated and unauthenticated requests. Preserve real 404
  error coverage instead of treating missing releases as transient errors.

## Validation

- Duplicate-check regression cases cover both response orders across multiple
  suites/pages, successful and failed replacements, queued/running reruns,
  timestamp ties, and missing timestamps. Reversed-order cases failed before
  replacing the initial first-result selection with explicit age comparison.
- New update-frequency regressions failed before the fix: intermediate CI
  progress and a changed summary URL both caused comment PATCH requests.
- Tests cover initial publication, repeated pending events, final success and
  failure, one-time stale-readiness removal even when review evidence is
  unavailable, revision changes, and scheduled
  recovery of pending snapshots older than 24 hours.
- `go test -tags=mage -race ./magefiles/...` passed during this PR's validation.
- `go test -race ./pkg/github -count=1` passed after replacing the live release tests.
- The patch-scoped coverage skill passed for the reporter and GitHub packages;
  all 91 changed executable lines were covered locally after the review fix.
  This is a touched-package check, not full-suite coverage; Codecov remains
  authoritative. Invalid summary run IDs are tested explicitly.
- Executed `atmos test pact-install` with a simulated installer: first-attempt
  success, recovery after two failed partial downloads, and failure after all
  three attempts. Verified forced downloads and a library path containing spaces.
  The command help and workflow configuration also validated.
- `atmos test pact` passed with the real native library and consumer tests on
  macOS, both with the default directory and a relative path containing spaces.
  A simulated test failure propagated after one test invocation without retry.
  Regenerated contracts matched the committed content, ignoring Pact's omitted
  final newline.
- Old-head rerun selection regressions failed before the recovery fix and cover
  active statuses, terminal runs, empty listings, other heads, reporter workflows,
  API errors, and both stale and recent PR update timestamps.
- Executed both APT rewrite commands against mirror-list, direct Azure, and
  already-primary source fixtures. Verified idempotence, preserved suites and
  signing keyrings, and the Floci host/port allowlist match. Hosted package
  installation remains a CI check.
- Workflow validation with `actionlint`, the fix-log format validator,
  `git diff --check`, and the commit hooks passed.
- Hosted CI must validate the artifact download path; a local unit test cannot
  reproduce the hosted runner's refused connection or network allowlist.

## Follow-ups

Verify the final PR CI run before merging. Reporter behavior becomes active
after the changes reach `main`; the scheduled reconciler then recovers pending
reports. No separate issue is needed for this unmerged work.
