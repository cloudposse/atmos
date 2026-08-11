# Fix: CodeRabbit review findings on the toolchain lock-file subsystem

**Date:** 2026-08-11

## Summary

Triaged 11 CodeRabbit findings from PR #2894's review pass (2 meta-notices excluded). Verified
each against current code per the repo's "verify, fix only still-valid, skip the rest with a
reason" convention; 8 were still valid and fixed, 3 were stale/skip. The two most significant
fixes close real, confirmed-reachable bugs:

1. **Lock-file checksum mismatch was rejected too late.** `installFromTool` only compared a freshly downloaded artifact's checksum against the recorded `toolchain.lock.yaml` entry *after* `extractAndInstall`/`os.Chmod` had already placed the binary in the install tree — so a tampered or unexpectedly-changed artifact would already be installed by the time the error returned, defeating the exact supply-chain guarantee `use_lock_file` exists to provide. The existing regression test only asserted an error was returned, never that no binary reached `binDir`, so it never caught this.

2. **Every real user's existing lock file would silently lose its data on upgrade.** Verified directly against the released `v1.226.0-rc.4` tag: every shipped version through it wrote `toolchain.lock.yaml` with `version`/`platforms` directly on the tool (flat, single-version shape, `lock_file_version: 1`). This branch's `Tool` struct changed to a nested `Versions` map (to support locking more than one version of the same tool) without ever handling the old shape on load -- unmarshaling v1 data into the v2 struct silently leaves `Versions` nil for every tool, which would make `Verify` report every tool missing its version and force a silent re-lock on every install for any user upgrading with `use_lock_file: true`.

## Context

This PR (`osterman/toolchain-update-pinning-field-test`) has accumulated substantial toolchain
lock-file work across many sessions; this pass specifically addresses CodeRabbit's review of that
work. Per the repo's fix-log convention, each finding was checked against the code as it exists
now (not just as CodeRabbit described it) before touching anything -- three findings turned out to
already be resolved or to conflict with an established, intentional repo convention, and are
documented below as skipped rather than silently ignored.

## Changes

### Fixed

- `pkg/toolchain/installer/lockfile_update.go` -- split `updateLockFile` into two functions:
  `checkLockFileChecksumMismatch` (new, read-only, compares a freshly verified download against
  any existing lock entry) and `updateLockFile` (now persistence-only, assumes the check already
  passed). Extracted a `lookupLockedChecksum` helper to keep `checkLockFileChecksumMismatch`
  under the repo's cyclomatic-complexity limit.
- `pkg/toolchain/installer/installer.go`, `installFromTool` -- now calls
  `checkLockFileChecksumMismatch` immediately after `verifyDownloadedAsset` and before
  `extractAndInstall`, removing the cached download and returning the error before any extraction
  happens on mismatch.
- `pkg/toolchain/installer/verification_integration_test.go`,
  `TestInstallFromTool_DetectsTamperedLockFileChecksum` -- strengthened to assert `binDir` stays
  empty after a rejected install, not just that an error was returned (this is the assertion gap
  that let the ordering bug ship unnoticed).
- `pkg/toolchain/lockfile/lockfile.go`, `Load` -- added `migrateLegacyTools`, triggered when
  `Metadata.LockFileVersion == 1` (the one real prior schema version -- deliberately not `<
  currentLockFileVersion`, since that would also "fix" a genuinely invalid `0`/missing version and
  defeat `Verify`'s own explicit check for it). Re-parses the raw bytes in the old flat shape and
  folds each tool's single version into a one-entry `Versions` map, preserving
  `Source`/`Platforms`/`BinaryName`/`InstalledAt`.
- `pkg/toolchain/lockfile/lockfile_test.go` -- new `TestLoad_MigratesLegacyV1ToolShape`, using the
  exact v1 YAML shape verified against the `v1.226.0-rc.4` release tag; confirms the recovered
  entry, the bumped `LockFileVersion`, and that `Verify` succeeds post-migration.
- `pkg/sbom/sbom.go`, `appendToolchain` -- added nil guards around the
  tool/version/platform traversal. `toolchainlock.Load` (unlike `Verify`) never validates nested
  entries, so a hand-edited or corrupted lock file can parse successfully with an explicit YAML
  `null` at any of the three levels; the old code dereferenced straight through, which panics.
  Malformed entries are now skipped (best-effort, matching this function's existing "incomplete
  coverage" pattern for missing files) rather than the whole SBOM generation crashing.
- `pkg/sbom/sbom_test.go` -- new `TestAppendToolchainSkipsNilLockEntriesInsteadOfPanicking`,
  exercising a nil entry at each of the three levels plus one fully valid tool in a single lock
  file.
- `pkg/toolchain/filemanager/lockfile.go`:
  - `RemoveTool`'s version-mismatch error now sorts `lockedVersions` before joining them (matches
    `GetTools`'s existing `sort.Strings` for the same reason: map iteration order is randomized,
    so the same failure could previously render different error text on different runs).
  - `AddTool` now rejects an empty `version` up front (new sentinel `ErrLockfileEmptyVersion` in
    `errors/errors.go`) instead of silently creating a lock entry keyed by `""`.  `RemoveTool`
    already gives an empty version a specific meaning (remove every locked version); `AddTool` gave
    it none, so the bogus `""` entry could only ever be removed by wiping the whole tool.
    `SetDefault` forwards straight to `AddTool`, so it's covered by the same guard without a
    separate change.
- `pkg/toolchain/filemanager/lockfile_test.go` -- new
  `TestLockFileManager_RemoveTool_VersionMismatchListsVersionsSorted` (asserts the sorted order via
  `cockroachdb/errors.GetAllSafeDetails` directly, since `errUtils.GetContext`/`HasContext` parse
  safe details as space-separated `key=value` pairs and can't recover a value that itself contains
  spaces) and `TestLockFileManager_AddTool_RejectsEmptyVersion`.
- `cmd/toolchain/list_test.go`, `lock_test.go`, `update_test.go` -- each test's `RunE` fixture now
  saves `toolchain.GetAtmosConfig()` before overwriting it and restores that value in cleanup,
  instead of unconditionally clearing to `nil`. Matches the save/restore pattern already used
  elsewhere in `pkg/toolchain`'s own tests (e.g. `TestRunLock_ForceWritesLockFileWithoutInstalling`)
  -- the previous "set then nil" shortcut made these three tests order-dependent on whatever global
  config an earlier test in the same process happened to leave behind.
- `pkg/toolchain/clean_test.go` -- added a shared `skipIfPermissionChecksAreIneffective` helper
  (checks both `runtime.GOOS == "windows"` and `os.Geteuid() == 0`) and applied it to the three
  `chmod`-based permission tests that previously only guarded against Windows. Root ignores
  permission bits on Linux, so these tests would silently pass for the wrong reason (the forced
  filesystem failure never actually fails) if the suite ever runs as UID 0, which is common for CI
  container images.
- `.claude/skills/field-test/SKILL.md` -- reworded the live-progress verification guidance to
  split it into two explicit checks instead of one command doing double duty: a real,
  unpiped pseudo-TTY run (`script -q /dev/null ... --force-tty --force-color`) for live-progress
  behavior, and a separate piped `--force-color` + `cat -v` run for ANSI-styling verification only.
  The original single piped command made the target command non-TTY, which exercises the
  non-live fallback renderer instead of the actual live-progress path it was meant to check --
  `--force-color` alone does not restore TTY behavior.
- `docs/fixes/2026-08-07-toolchain-update-live-usage-bugs.md` -- corrected a validation-log entry
  from `gofmt -l` to `gofumpt -l`, per CLAUDE.md's formatting mandate.

### Skipped (stale or conflicts with established convention)

- **`cmd/markdown/atmos_toolchain_lock.md` markdownlint findings** (missing `shell` language tag,
  `$` prompts without shown output) -- not fixed. Every sibling
  `cmd/markdown/atmos_toolchain_*.md` file (`install`, `add`, `set`, `remove`, etc.) uses this
  exact same bare-fence-plus-`$`-prompt style, and markdownlint is not wired into this repo's
  pre-commit hooks or CI workflows at all (confirmed by grep). Fixing only the newest file would
  create inconsistency with ~15 established sibling files for a purely advisory, non-blocking
  finding.
- **Mock-runner interface for `cmd/toolchain/lock.go`'s `runLock`** -- not fixed.
  `TestLockCommand_RunE` (added in an earlier coverage-fix pass) already covers the RunE dispatch
  logic via the same real-call pattern used by every sibling command in this package (`add`,
  `list`, `update`); no mock-runner abstraction exists anywhere in `cmd/toolchain`, and introducing
  one only for `lock` would fork an established convention without a concrete benefit.
- **GoDoc comments on `LockCommandProvider`'s exported methods** -- not fixed. Checked
  `InstallCommandProvider`/`UpdateCommandProvider` (the sibling providers): neither has
  method-level doc comments either, and it isn't enforced by this repo's revive config. Adding
  comments only to `LockCommandProvider` would create asymmetry with every other provider in the
  package, not fix a real gap.

## Validation

- `go build ./...` -- clean.
- `go vet ./...` -- clean.
- `go test ./pkg/toolchain/... ./cmd/toolchain/... ./pkg/sbom/...` -- all packages pass, including
  every new regression test above.
- `gofmt -l` -- clean on every touched Go file.
- `./custom-gcl run --new-from-rev=<merge-base>` -- 0 issues (fixed one cyclomatic-complexity
  finding via the `lookupLockedChecksum` extraction, and one `godot` false-positive caused by a
  lowercase Go identifier immediately following a sentence-ending period, same class as prior
  fixes this branch).

## Follow-ups

None.
