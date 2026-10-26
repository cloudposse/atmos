# Fix: `toolchain update`/`toolchain lock` version-pinning and lock-file bugs

**Date:** 2026-08-07

## Summary

A field-test pass of the new `atmos toolchain update` and `atmos toolchain lock` commands (on
branch `osterman/toolchain-update-pinning-field-test`) found six real bugs: `update`/`set`/`add
--default` never actually replaced a tool's pinned version (they only prepended, leaving stale
versions behind forever); `atmos toolchain lock` silently discarded a tool's earlier locked
version whenever a second version of the same tool was locked; `install` never verified a fresh
download's checksum against what was already recorded in `toolchain.lock.yaml`, despite the
`lock` command's own warning implying it would; the lock file's default path didn't match the
actual install path; `add`'s error wrapping silently dropped a helpful hint; and `update`'s
`--help` text made a claim about pin immutability that the command's own design had already
walked back elsewhere. All six are fixed, backed by regression tests that were written and
confirmed failing before each fix, then confirmed passing after.

## Context

The bugs were found via a hands-on field-test pass (the `field-test` skill), not code review —
each was reproduced live against real fixtures with real network/registry access, not just
inferred from reading the code. Three of the six were shared-root-cause bugs that surfaced
through multiple commands: the replace-vs-append bug affected `set`, `add --default`, and
`update` simultaneously (all three route through the same `pkg/toolchain/tool_versions.go`
helper), and existing unit tests for all three had encoded the buggy behavior as correct (e.g.
`TestAddToolToVersionsAsDefault`'s assertions checked `versions[0]` but never checked that stale
versions were gone). The lock-file schema bug was corroborated independently: the schema keyed
tool entries by `owner/repo` with a single flat `Version` field, so a `.tool-versions` line
pinning two versions of the same tool (a legitimate, already-shipped example fixture pattern,
`examples/toolchain/.tool-versions`'s `yq 4.45.1 4.50.1`) could never be represented correctly.

Two design decisions were resolved with the user before implementing:
- **Replace semantics:** researched asdf's own `set`/`local` convention (its docs describe
  `asdf set <tool> <version>` as equivalent to `echo "<tool> <version>" > .tool-versions`, i.e.
  full line replacement) and adopted the same "full replace" semantics rather than a
  partial-preserve compromise.
- **Lock-file schema:** chose to restructure `Tool` to hold a nested `Versions` map (Option B)
  over a minimal composite-key change (Option A), since Option A would leave orphaned
  old-format entries on upgrade with no read path ever finding them again.

## Changes

- `pkg/toolchain/tool_versions.go` — `AddVersionToTool(asDefault=true)` now fully replaces the
  version list with the new version, instead of prepending and keeping the old default. Fixes
  `set`, `add --default`, and `update` in one place.
- `pkg/toolchain/lockfile/lockfile.go` — `Tool` restructured from a flat
  `Version`/`Platforms`/`BinaryName`/`InstalledAt` struct to `Versions
  map[string]*VersionEntry`, so multiple locked versions of one tool coexist. Added
  `Tool.GetOrCreateVersion`/`RemoveVersion`, updated `Verify`'s validation to walk the nested
  structure, bumped `lock_file_version` to 2.
- `pkg/toolchain/installer/lockfile_update.go` — `resolveLockFilePath` now falls back to the
  same XDG-cache-first default `toolchain.GetInstallPath()` uses (via `pkg/xdg` directly, since
  `installer` can't import `pkg/toolchain` without a cycle), instead of a hardcoded relative
  `.tools`. `updateLockFile` now compares a freshly downloaded checksum against any existing
  recorded entry before overwriting it.
- `pkg/toolchain/installer/installer.go` — new `verifyAgainstLock` field, distinct from
  `useLockFile`: true for normal config-driven installs, explicitly false for `lock`'s own
  `WithForceLockFile()` path so re-locking/refreshing never fails on its own prior checksum.
- `pkg/toolchain/installer/errors.go` — new `ErrLockfileChecksumMismatch` sentinel.
- `cmd/toolchain/add.go` — replaced a double-`%w` `fmt.Errorf` wrap (which silently discarded
  `cockroachdb/errors` hints/details attached via `errUtils.Build`) with
  `errUtils.Build(...).WithCause(...)`, which explicitly preserves them.
- `cmd/toolchain/update.go` — reworded the `--help` text for pr:/sha:/ref: pins to match the
  corrected language already in `toolchain-update.mdx` (skipped by choice, not immutability).
- Downstream schema consumers updated to compile against the new nested `Tool` type:
  `pkg/sbom/sbom.go` (SBOM component IDs now include the version segment, since a tool can have
  multiple locked versions), `pkg/toolchain/filemanager/lockfile.go` (unused/unwired registry
  implementation, still had to compile).
- Test updates: `pkg/toolchain/tool_versions_test.go`, `set_test.go`, `add_test.go`,
  `update_test.go`, `pkg/toolchain/lock_test.go`, `pkg/toolchain/lockfile/lockfile_test.go`
  (rewritten for the nested schema), `pkg/toolchain/installer/lock_tool_test.go`,
  `lockfile_update_test.go`, `verification_integration_test.go`, `pkg/toolchain/filemanager/
  lockfile_test.go` and `toolversions_test.go` (two more consumers of the replace-semantics bug
  found only while fixing the schema), `pkg/sbom/sbom_test.go`, `cmd/toolchain/add_test.go`,
  `cmd/toolchain/update_test.go`.

## Validation

- Regression tests written first and confirmed failing against pre-fix code (9 tests across the
  six bugs), then confirmed passing after each corresponding fix.
- `go build ./...` — clean.
- `go vet ./...` — clean (full repo, not just the touched packages).
- `go test ./pkg/toolchain/... ./cmd/toolchain/... ./pkg/sbom/...` — all packages pass. One
  transient failure (`TestRunBootstrapVerifierHoldsVersionLockThroughTrustAndExecution`, a
  cosign-bootstrap test unrelated to lock-file code) was observed once under concurrent-suite
  load and confirmed non-reproducing when re-run in isolation and as part of the full
  `pkg/toolchain/installer` package alone.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues (two `godot` findings surfaced and
  were fixed during the pass).
- `gofmt -l` — clean on every touched file.
- Not yet committed, pushed, or opened as a PR.

## Follow-ups

None.
