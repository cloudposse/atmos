# Fix: `atmos plugin release --inventory` stops at the first failing package

**Date:** 2026-11-26

## Summary

Issue #3763. `atmos plugin release --inventory` released packages one at a time and returned on the
first package error. The packages after it were never built or published, and with `--catalog` the
catalog was never reconciled, even for packages that had already published.

This turned #3760 into a release problem: re-signing `auth-secrets` (first in `plugins/releases.yaml`)
failed on every push to `main` from 2026-11-22 to 2026-11-25, so `terragrunt` 0.1.2, which had nothing
to do with the failure, was never published and the `cloudposse` catalog went stale. #3761 fixed that
signing failure; this change makes sure a future failure in one package cannot block the others.

An inventory release now continues past a failing package, reconciles the catalog for the packages
that released, reports them, and still exits non-zero with every failure listed. The new `--fail-fast`
flag keeps the previous stop-at-first-failure behavior.

## Context

- `pkg/plugin/release/inventory.go`, `RunInventory`, returned
  `fmt.Errorf("plugin release %s: %w", entry.Name, err)` on the first failure and discarded the
  results gathered so far.
- `cmd/plugin/release_inventory.go`, `runInventoryRelease`, returned that error before
  `reconcileReleasedCatalog` and `presentInventoryReleases` ran.
- `presentInventoryReleases` paired results with plan entries by index
  (`plan.Packages.Include[index].Repository`). That only worked because results were all-or-nothing;
  with partial results the repositories would have been reported against the wrong packages.
- Catalog reconciliation reads the accepted inventory from the current origin default branch and
  merges each selected release into the existing catalog with `catalog.WithSkipMissing()`. Skipping only
  covers versions that are absent from the registry. A failed package can still have a published but
  unsigned or unverified version (for example when publication succeeded and signing or
  `--verify-public` failed), which `WithSkipMissing` would have added to the catalog.

## Changes

- `pkg/plugin/release/inventory.go`:
  - `RunInventory` now continues after a package fails and returns the packages whose release
    completed as `[]InventoryRelease` (entry plus release, in plan order), so each release stays paired
    with the entry that produced it.
  - `run` reports a failed `git worktree remove` separately from the release outcome. A package that
    published (or passed `--check`) but whose worktree could not be removed is still recorded as
    released, so it reaches the catalog and the output, and the cleanup failure is still reported as
    that package's error (and stops the run under fail-fast). Before, the joined error discarded the
    completed release. Found in review (CodeRabbit).
  - The error joins every package failure (`plugin release <name>: ...`) under a summary wrapping the
    new sentinel `ErrPluginInventoryRelease` (`plugin inventory release failed: N of M packages failed or
    did not run`). N counts packages with an error plus packages never started after cancellation or
    fail-fast.
  - Cancellation always stops the run, before the next package and after a package that was cancelled.
  - New `WithInventoryFailFast(bool)` restores returning at the first failure.
- `cmd/plugin/release_inventory.go`:
  - New `--fail-fast` flag (`ATMOS_PLUGIN_RELEASE_FAIL_FAST`), registered through the existing
    `pkg/flags` options; `validateReleaseMode` rejects it without `--inventory`.
  - New `completeInventoryRelease`: after a partial failure it reconciles the catalog (when `--catalog`
    is set) and presents the released packages, then returns the joined package errors so the command
    exits non-zero. Fail-fast, a run in which no package succeeded, and cancellation return the package
    errors without touching the catalog or the output, which matches the previous behavior.
  - Catalog decision: `reconcileReleasedCatalog` keeps only the selections that match a package version
    released in this run (`onlyPackages`): same name, same version after `catalog.NormalizeVersion`, and
    same repository, carried from the plan entries by `releasedPackages`. Packages that failed, never
    started, or were not part of this run's plan keep their existing catalog entry instead of a possibly
    unsigned or unverified version. This matters because reconciliation re-reads the inventory at the
    current default-branch head, which can list a package added after planning or a different version of
    a released package (found in review, CodeRabbit; the first revision only excluded failed packages,
    the second matched by name only). If nothing remains to reconcile, the catalog is not written.
  - `presentInventoryReleases` reports `result.Entry.Repository` for each result instead of indexing the
    plan, and the `--check` summary counts the packages that were checked.
- `errors/plugin.go`: new `ErrPluginInventoryRelease`.
- Docs: `website/docs/cli/commands/plugin/release.mdx` documents `--fail-fast`, the catalog behavior
  after a partial failure, and a new "When a package fails" section. Regenerated
  `website/static/casts/screengrabs/atmos-plugin-release--help.cast`.
- Blog post `website/blog/2026-11-26-plugin-release-continues-past-failures.mdx` and a roadmap milestone
  in the `extensibility` initiative (progress recalculated to 48/61 = 79%), because the new flag makes
  this a `minor` change.
- `.github/workflows/plugin-packages.yaml` is unchanged: CI keeps the new default and does not pass
  `--fail-fast`.

## Validation

- New `pkg/plugin/release/inventory_continue_test.go`:
  - `TestInventoryContinuesPastAFailingPackage`: three packages with the middle one failing. By default
    the first and last are released, the error wraps `ErrPluginInventoryRelease` and `ErrPluginBuild`,
    names only `broken`, and reports `1 of 3` packages failed or did not run. With fail-fast only the first is released, the last never
    starts, and the error reports `2 of 3`. Every started package removes its worktree.
  - `TestInventoryCancellationStopsWithoutFailFast`: cancelling during the second package stops the run
    without fail-fast; the third package never starts and the error wraps `context.Canceled`.
  - `TestInventoryKeepsACompletedReleaseWhenCleanupFails`: `Publish` succeeds and worktree removal
    fails; the release is still in the results, the next package still runs, and the error wraps the
    cleanup failure (`ErrGitCommandExited`).
  - `TestInventoryFailFastStopsAfterACleanupFailure`: with fail-fast the cleanup failure stops the run,
    the completed release is kept, and the next package never starts.
  - `TestInventoryTallyErrorIsNilWithoutFailures`: summary formatting and the no-failure case.
- `pkg/plugin/release/inventory_test.go`: the existing stop-on-failure test now runs with fail-fast
  (renamed `TestInventoryFailFastFailuresAlwaysCleanUpAndStop`) and asserts the new sentinel; in its
  `cleanup` case the completed check is now recorded. The success test asserts entry-paired results.
- New `cmd/plugin/release_inventory_test.go`:
  - `TestCompleteInventoryReleaseAfterPackageFailures` (9 cases): partial failure reconciles only the
    released packages and presents; fail-fast, nothing released, and cancellation skip both;
    catalog failure stops before output and keeps both errors; output errors are returned.
  - `TestReleasedPackagesKeepsNameVersionAndRepository`, `TestOnlyPackagesKeepsExactlyTheReleasedVersions`
    (a failed package, one added after planning, a newer version of a released package, and a released
    package listed under another repository are all dropped; a `v`-prefixed version still matches),
    `TestFailFastRequiresInventory`, `TestReleaseCommandRegistersFailFast`.
  - `TestPresentInventoryReleasesReportsOnlyReleasedPackages` (JSON output keeps each release's own
    repository and omits the failed package) and `TestPresentInventoryReleasesCountsCheckedPackages`.
  - `cmd/plugin/mock_streams_test.go` is generated with mockgen (`pkg/io Streams`).
- Verified the new tests fail against the old behavior: forcing `RunInventory` to stop on every failure
  fails `TestInventoryContinuesPastAFailingPackage/continues_by_default`, and restoring the old early
  return in `completeInventoryRelease` fails the three partial-failure cases.
- `go build ./...` passes. `go test ./pkg/plugin/release/ ./cmd/plugin/ ./errors/ ./pkg/plugin/catalog/`
  passes; `pkg/plugin/release` is at 100% statement coverage, and the new `cmd/plugin` functions
  (`completeInventoryRelease`, `releasedPackages`, `onlyPackages`) are at 100%.
- Not covered by unit tests: `runInventoryRelease` and `reconcileReleasedCatalog` wiring, which need a real
  git checkout and registry (both were already untested before this change).
- `atmos --chdir=demo/casts casts generate screengrabs cli --filter "atmos plugin release --help"` regenerated
  the help cast, and `casts validate screengrabs cli` with the same filter passes.
- `golangci-lint` was not run locally; CI runs it.

## Follow-ups

None.
