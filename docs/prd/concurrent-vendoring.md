# Concurrent Vendoring with Polished Progress

**Status:** Implemented.

## Summary

Apply the concurrent toolchain installation architecture to vendoring: bounded
parallel downloads and preparation, declaration-ordered destination writes, and a
single progress renderer. Cover all `vendor pull` paths, concurrent upstream
checks in `vendor update` and `vendor update --check`, and `update --pull`.

Toolchain concurrency shipped in PR [#2758](https://github.com/cloudposse/atmos/pull/2758),
commit `580f5c0202`. Its worker-local installers, bounded pool, serialized progress,
permanent completion lines, and advisory locks are the implementation reference.
The previous vendoring installer was independent of Bubble Tea, but fetching,
copying, and receipt recording were combined and its UI dispatched one package
at a time.

Homebrew provides complementary inspiration: concurrent downloads and overlapping
preparation, with permanent results above a bounded live region and terminal
cleanup on interruption. References: [Homebrew 5.0](https://brew.sh/2025/11/12/homebrew-5.0.0/),
[Homebrew 7.0](https://brew.sh/2026/09/13/homebrew-7.0.0/), and
[DownloadQueue](https://docs.brew.sh/rubydoc/Homebrew/DownloadQueue).

## User experience

Use the Atmos toolchain theme, spinner, spacing, icons, and progress bar. Extract
reusable presentation into `pkg/ui/batch`; keep scheduling and vendoring rules
outside it. Preserve toolchain appearance and behavior during extraction.

Each active row has a stable job ID independent of its label. Labels identify
the component name or mixin filename, without destination paths or arrows. Completed
results use `✓ component (version)`, with the version muted; active rows highlight
the component name with a dimmed stage suffix, such as `ipinfo (main) · downloading 42%`.
Download percentages appear only when the backend supplies a total size. Job IDs
distinguish equal labels internally. Phases include checking,
downloading, preparing, ready, installing, retrying, and waiting for another
vendoring operation. Display bytes and total size only when the backend supplies
them, otherwise use an indeterminate spinner.

A prepared package remains ready until destination copying and receipt recording
succeed. Keep completed results above the live region. End pulls with counts of
installed, unchanged, failed, and canceled packages; preserve update discovery's
existing statuses and report order. Selected manifests and component types share
one batch. Update-and-pull presents labeled checking and pulling phases.

Use one renderer on masked stderr; machine-readable update reports remain on
stdout. Respect terminal dimensions, truncate labels safely, and summarize rows
that do not fit. Non-TTY, debug, and unsupported terminals receive plain result
lines without cursor movement. Restore terminal state on all exit paths.

## Configuration and editions

```yaml
vendor:
  max_concurrency: 4
```

```shell
atmos vendor pull --max-concurrency 8
atmos vendor update --check --max-concurrency 8
atmos vendor update --pull --max-concurrency 8
```

Precedence: explicit flag, `ATMOS_VENDOR_MAX_CONCURRENCY`, merged configuration,
then edition-aware default. Reject explicit zero, negative, or malformed values
before work begins. One setting controls upstream checks and pull preparation.

| Configuration | Default workers |
| --- | ---: |
| No edition pin | 4 |
| Pin before feature date | 1 |
| Pin on/after feature date | 4 |
| Explicit setting | Explicit value |

All editions get the progress display. One worker preserves serial execution.
Use the existing edition resolver and end-of-period semantics for partial dates.
Journal `vendor.max_concurrency` as `KindValue`, old `1`, new `4`: the new key
replaces previously implicit sequential behavior. Define its default only in the
configuration defaults layer, allowing `SetDefault` rollback and explicit settings
to win. Include journal listings, descriptions, and default invariant tests.

The implementation is tracked in [PR #3169](https://github.com/cloudposse/atmos/pull/3169)
and uses **2026-09-15** as its feature date. Before merging, align this date and
the corresponding journal, documentation, and tests with the actual merge date.

## Execution and integrity

### Ordered preparation and materialization

Split installation into:

1. **Prepare:** fetch to private temporary storage, apply preparation rules, and
    collect inventory/provenance for an opaque prepared receipt.
2. **Materialize:** copy files, reconcile ownership, prune stale files, and record
    the successful receipt under mutation locks.

Build the entire selection before materialization. Preserve imported-source,
source/target, and component-before-mixin order; sort component-type groups.
Retain selectors, constraints, target overrides, include/exclude rules, refresh,
and strict/warn/silent lock behavior. Strict preflight covers the whole selection.

Use at most N preparation workers, with at most 2N active or prepared packages.
For N=1, commit each package before preparing the next. Start committing as soon
as the next ordered package is ready; a slow early download must not stage the
entire repository. Treat local reads as barriers after preceding writes. Recheck
proposed unchanged results after earlier writes that may affect their targets.

Continue after per-package failures, aggregate errors in plan order, and preserve
successful earlier installations. There is no whole-batch rollback. Neither copy
nor receipt failures may emit success. Clean staging directories on every exit.

### Shared files and processes

Reuse `pkg/filelock`, taking the canonical project mutation lock before the lock
for the configured lockfile path. Protect copying, ownership reconciliation,
pruning, and receipt read-modify-write together. Reload and validate the receipt
inside the transaction before touching targets. Coordinate clean and other
vendoring writers with the same locks. Internal unlocked helpers avoid recursive
acquisition. Downloads and provenance resolution stay outside mutation locks.

Lock waits honor context cancellation and appear in progress. On interruption,
stop scheduling, cancel preparations, join workers, and remove staging. Finish
an already-started materialization before releasing its locks, then stop further
commits and return cancellation. Retain the existing lockfile schema.

### Concurrent update discovery

Resolve selectors and manifest precedence before scheduling checks. Check remote
versions and best-effort archive status concurrently, collecting results by source
position. Apply version edits sequentially with format-preserving YAML setters.
Lock each declaring manifest, reload it, and reject an edit if its relevant source
declaration changed while discovery ran. Preserve constraints, skip rules,
comments, report order, and partial-failure behavior. Check-only mode writes no
manifests or receipts. Update-and-pull retains its selection/reconciliation rules
and passes the same resolved concurrency into its pull plan.

### Interfaces and worker isolation

Provide context-aware batch install/update entrypoints with synchronous
compatibility wrappers, opaque prepared packages, batch options, and structured
progress events. Serialize observers; coalesce byte events while retaining every
state transition and terminal result. Retries are events within a job, never
additional jobs.

Use client-local go-getter detectors instead of mutating the global detector
list. Keep downloader metadata and installer state scoped to a job. Initialize
credentials/configuration before workers. Propagate cancellation through remote
checks, downloads, retry delays, and lock acquisition without replacing caller
contexts with background contexts. Preserve retry policies and timeout limits.

## Verification and release criteria

- Prove preparations/checks overlap and respect N; verify N=1 remains serial.
- Reverse completion order and check identical target contents and receipts,
  including duplicate labels, targets, imports, overlapping destinations, and
  component/mixin order. Verify local barriers and unchanged revalidation.
- Exercise separate processes sharing projects/receipts, including pull plus clean.
- Cover fetch, copy, receipt, stale-manifest, retry, cancellation, and lock-wait
  failures with no leaked staging or missing terminal results.
- Preserve selectors, enforcement modes, refresh, dry run, YAML formatting,
  report order, and structured stdout.
- Test editions before/on/after the feature date, partial dates, no-config loads,
  profiles, environment/flag overrides, and invalid settings.
- Verify renderer output, duplicate labels, unknown sizes, retries, narrow/resized
  terminals, non-TTY output, cancellation, and toolchain compatibility.
- Run focused race tests and achieve at least 85% coverage of new/changed behavior.
- Benchmark eight fetch-bound packages at one/four workers, targeting at least
  2× speedup. Keep timing assertions out of ordinary unit tests.
- Publish command/configuration and edition guidance plus a progress recording.

Outside this release: parallel destination writes, new protocols, persistent
caching, parallel diff execution, and changes to PR publishing behavior.

## Implementation verification

Local verification covers the working-tree changes, including new files:

- Combined short race suites passed for configuration, editions, downloader, OCI,
  shared UI, vendoring, component planning, installation, receipts, update selection,
  and vendor commands. Vendor-focused exec and toolchain adapter race tests passed.
- Changed executable statement coverage is approximately 95%; see
  `.context/coverage-summary.md` for the local profile calculation. Scoped coverage
  is not a replacement for the full CI Codecov upload.
- `custom-gcl` passed with the repository plugin enabled and the complete working
  diff, including untracked Go files.
- `npm run build` in `website` passed. The repeatable download/unchanged recording
  and pull/update help screengrabs were regenerated and validated.
- `BenchmarkInstallBatchFetchBound` measured 595 ms with one worker and 179 ms
  with four (3.33×) in one controlled eight-package run. This is fixture evidence,
  not a promised speedup for every upstream or filesystem.

Regenerate the demo using
`atmos --chdir=demo/casts casts generate demo fixtures concurrent-vendoring pull`.
It serves throttled HTTP downloads locally and shuts the fixture server down even
when recording fails.
