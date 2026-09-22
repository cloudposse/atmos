# Fix: `vendor clean --prune-lock` to forget lock entries

**Date:** 2026-09-22

## Summary

Add a `--prune-lock` flag to `atmos vendor clean` that also removes the cleaned components' entries
from `vendor.lock.yaml`. This restores a supported way to permanently remove a vendored component
after #3169 made `vendor clean` preserve lock entries by default. Fixes cloudposse/atmos#3196.

## Context

Since v1.229.0 (#3169, "`vendor clean` now preserves lockfile entries for future reinstalls"), there
was no supported path to remove a vendored source that uses the native lock:

1. `vendor clean --component other` removes files but keeps `other` in `vendor.lock.yaml` (intentional).
2. Deleting `other` from `vendor.yaml` and running `vendor pull --refresh-lock` vendors the remaining
   sources but leaves the orphan `other` artifact in the lock.
3. `vendor verify` then fails: `other`'s recorded files are reported as `missing`.

The issue offered three candidate fixes. Auto-pruning on `--refresh-lock` was rejected because
`vendor.yaml` and legacy `component.yaml` vendoring **share one `vendor.lock.yaml`**
(both call `lockfile.Record`/`PrepareRecord`), so a full `vendor pull` blindly pruning
non-declared artifacts could delete legitimate `component.yaml` lock entries. The surgical,
zero-over-pruning fix is an explicit opt-in flag on `vendor clean` that forgets only the entries the
same selectors already matched.

## Changes

- `pkg/vendoring/lockfile/lockfile.go`:
  - Added `CleanOptions{Force, DryRun, PruneLock}` and changed `CleanSelectedContext` to take it
    (replacing the `force, dryRun bool` pair). `Clean`/`CleanSelected` keep their bool signatures and
    delegate with `PruneLock: false`, so existing callers are unaffected.
  - Added `pruneSelectedLockEntries`: after files are removed, delete the **selected** artifacts'
    entries from the lock and `Save`. It only touches artifacts the component/tag/stack/label
    selectors already chose, so it never prunes entries the caller didn't target (e.g. shared
    `component.yaml` artifacts). A dry run reports the would-be-forgotten names without writing.
  - Added `CleanReport.Forgotten` (the forgotten component names).
- `cmd/vendor/clean.go`: registered the `--prune-lock` bool flag (via the existing
  `flags.StandardParser`), passed it through `CleanOptions`, and printed `Forgot lock entry <name>`
  (or `Would forget…` on `--dry-run`).
- `website/docs/cli/commands/vendor/vendor-clean.mdx`: documented the flag, synopsis, an example, and
  the permanent-removal workflow.

## Validation

```bash
# New tests fail without the flag (the feature didn't exist) and pass with it.
go test ./pkg/vendoring/lockfile/ -run TestCleanSelected_ -v
go test ./cmd/vendor/ -run TestVendorCleanCmd_PruneLock -v

# No regressions; existing clean tests (default preservation) still pass.
go build ./...
go test ./pkg/vendoring/lockfile/ ./cmd/vendor/
cd website && npm run build
```

Coverage: `CleanSelectedContext` 100%, `pruneSelectedLockEntries` covered including the empty-selection
guard and dry-run path. `atmos lint --changed` reports 0 issues.

## Follow-ups

None. Auto-pruning orphans on `vendor pull --refresh-lock` was intentionally not implemented due to
the shared-lock data-loss risk described above; `--prune-lock` is the safe, targeted alternative.
