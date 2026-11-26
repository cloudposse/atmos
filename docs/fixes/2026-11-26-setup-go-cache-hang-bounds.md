# Fix: A hung "Set up Go" step no longer burns the whole job timeout

**Date:** 2026-11-26

## Summary

Issue #3766. The `Set up Go` step (the `./.github/actions/setup-go-cache` composite action) had no time limit
at any call site. On 2026-11-25 one macOS acceptance shard in the merge-queue `Tests` run for #3762 stayed in
`Set up Go` from 21:25:50 until the job was cancelled at 22:09:22, past the 40-minute macOS job timeout. The other
shards finished that step in 2 to 3.5 minutes. The cancelled shard failed the `Acceptance Tests (macos)`
aggregator and the merge queue evicted the PR, so the whole queue cycle had to run again.

Every workflow step that calls the action now sets `timeout-minutes` (15, or less in jobs whose own timeout is
short), except the two calls in `plugin-packages.yaml`, which stay unbounded because that workflow is a plugin
release build input. Each cache restore inside the action now turns a stalled download into a cache miss after
3 minutes per segment instead of 10. A guard test fails if a call site outside `plugin-packages.yaml` omits the
bound, or a restore loses its fallback.

## Context

- The job log of the hung shard is gone (`BlobNotFound`), so it is unknown which part of the action hung. The
  action covers the `actions/setup-go` toolchain download, a `Verify Go installation` check, and an
  `actions/cache/restore` of the ~1.7 GB build/module cache.
- Composite action steps cannot set `timeout-minutes`. The runner's action schema
  (`actions/runner`, `src/Runner.Worker/action_yaml.json`) allows only `continue-on-error`, `env`, `id`, `if`,
  `name`, `run`/`uses`, `shell`, `with` and `working-directory` on them. The bound has to be on the workflow step
  that calls the action, and it then covers the whole composite.
- What the pinned actions already do on a stall:
  - `actions/cache/restore` v5.0.5 downloads the archive in 128 MB segments and aborts a segment after
    `SEGMENT_DOWNLOAD_TIMEOUT_MINS` (default 10). `restoreCacheV2` catches that error, logs
    `Failed to restore: ...` as a warning, and returns no key, so the step succeeds as a cache miss.
  - `actions/setup-go` v7.0.0 downloads the toolchain through `@actions/tool-cache` with a 3-minute socket idle
    timeout and 3 attempts. A download that trickles without going idle, and the archive extraction, have no
    bound of their own.
- `rerun-infra-failures.yml` cannot recover this: it skips `merge_group` runs by design, and a job cancelled by
  its timeout while a step is running is classified `superseded`, which vetoes the rerun.
- Healthy `Set up Go` durations, measured from 448 successful steps in the 20 most recent completed `Tests` and
  `CodeQL` runs: Linux median 3.0, p95 3.8, max 5.1 minutes; Windows median 2.6, p95 3.6, max 4.0; macOS median
  2.8, p95 5.5, max 8.3 (`Build (macos-intel)`).

## Changes

- `.github/workflows/*.yml|yaml` (20 files, 36 of the 38 call sites): every step that uses
  `./.github/actions/setup-go-cache` sets `timeout-minutes`, except the 2 in `plugin-packages.yaml` (below):
  - 15 minutes by default, about 1.8 times the slowest healthy run. A hang now fails the step after 15 minutes
    instead of consuming a 40 to 60 minute job budget.
  - Less in the six jobs whose own timeout is 15 minutes or under, so the configured step bound sits at least
    3 minutes below the configured job timeout:
    7 minutes in the 10-minute jobs (`release-pipeline.yml` `complete`, `test.yml` `race-plan`) and 12 minutes in
    the 15-minute ones (`test.yml` `pact`, `magefiles`, `coverage`, `container-step`). All six run on Linux, where
    the slowest healthy step took 5.1 minutes; the release `complete` job's step took 2.3 minutes on v1.235.0.
    Found in review (CodeRabbit).
  - The 3-minute headroom compares configured durations, so it only holds while the steps before Go setup are
    short. In the last 15 `Tests` runs and recent release runs, those six jobs reached `Set up Go` within 0.2 to
    0.9 minutes of starting. The `coverage` job used to download every shard's artifacts (a step bounded at 10
    minutes) before Go setup; Go setup now runs first, since the download only uses the installed atmos binary
    and `gh`. Found in review (CodeRabbit).
  - `plugin-packages.yaml` (2 call sites) is left unchanged. `plugins/releases.yaml` lists that workflow as a
    build input of every plugin package, so editing it fails the `Official plugin packages / check` job unless
    every plugin version is bumped, which would republish unchanged plugins.
- `.github/actions/setup-go-cache/action.yml`: the three `actions/cache/restore` steps set
  `SEGMENT_DOWNLOAD_TIMEOUT_MINS: "3"`. A healthy 128 MB segment takes seconds (the slowest measured restore,
  Windows, is about 94 seconds for the whole archive), so a segment still running after 3 minutes is stalled. The
  restore then becomes a miss and the job continues, compiling without the cache, well inside the 15-minute step
  bound. The action description documents both bounds.
- New `magefiles/ci/setup_go_cache_bounds_test.go` (runs in the `[magefiles] unit tests` job):
  - `TestEverySetupGoCacheCallBoundsItsDuration` parses every workflow and fails for any step that calls the
    action without a literal `timeout-minutes` between 1 and 15, or with one less than 3 minutes below a literal
    job timeout. Only `plugin-packages.yaml` is exempt, and the test fails if that exemption goes stale.
  - `TestSetupGoCacheBoundMustExpireBeforeTheJobTimeout` checks that headroom rule directly: 15 in a 10-minute job
    and 8 in a 10-minute job are rejected, 7 is accepted, and a matrix-expression job timeout is not evaluated.
  - `TestSetupGoCacheRestoresFallBackToAMissOnAStall` fails if a restore step in the action lacks a numeric
    `SEGMENT_DOWNLOAD_TIMEOUT_MINS` of at most 5, or sets `fail-on-cache-miss: true`, which would turn the
    timed-out restore into a failure.
  - Both tests require that they found at least one call site and one restore step, so a rename can't make them
    pass vacuously.

Considered and not done:

- **Retrying the toolchain download inside the action.** The hang's location is unknown, setup-go already retries
  idle downloads three times, and a pre-fetch step would duplicate setup-go's version resolution. Not justified by
  the evidence available.
- **Teaching the `internal/ci/rerun` classifier that a timed-out `Set up Go` is an infrastructure failure.** The
  classifier only sees step conclusions from the jobs API, which don't reliably separate a step cut off by
  `timeout-minutes` from a real setup failure, and it would not help merge-queue runs anyway.
- **The 9 call sites inside other composite actions** (`release-prepare`, `release-build`, `release-verify`,
  `release-finalize`, `release-milestones`, `release-preview`, `quality-gate`, `classify-infra-failures`,
  `setup-atmos-build`). They can't set `timeout-minutes`. They get the cache-restore bound; the toolchain download
  there remains bounded only by the calling job's timeout.

## Validation

- The new tests failed on `origin/main` before the change: the first reported all 38 call sites without
  `timeout-minutes`, the restore test all 3 restore steps without `SEGMENT_DOWNLOAD_TIMEOUT_MINS`. With a uniform
  15 minutes, the headroom rule reported exactly the six short jobs. All three tests pass after the change.
- `atmos plugin release --inventory plugins/releases.yaml --check --base origin/main` reproduced the
  `Official plugin packages / check` failure ("changed build inputs require metadata.version to increase") while
  `plugin-packages.yaml` was edited.
- `go test -tags=mage ./magefiles/... -count=1` passes, including the release workflow and cache tests that parse
  this action and its callers. `go vet -tags=mage ./magefiles/ci/` passes and `gofumpt` reports no changes.
- `actionlint` on the 20 changed workflows reports no findings, before or after the change.
- Not verified: an actual hang on a hosted runner, or the cache-miss fallback triggered by a real stalled segment.
  The fallback behavior was confirmed by reading the pinned `actions/cache` v5.0.5 `dist/restore-only/index.js`
  (`downloadCacheStorageSDK` and `restoreCacheV2`), not by inducing a stall.
- `golangci-lint` was not run locally (permission denied in this environment); CI runs it.

## Follow-ups

- The toolchain download in the 9 composite-nested call sites, and the 2 call sites in `plugin-packages.yaml`, are
  still bounded only by their jobs' timeouts. Bound the `plugin-packages.yaml` sites the next time a plugin version
  is bumped anyway, and revisit the composite-nested ones if one of those jobs hangs in `Set up Go`.
