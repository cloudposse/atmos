# Fix: Keep tool-version coordination locks outside projects

**Date:** 2026-10-04

## Summary

Reading or updating `.tool-versions` no longer leaves an empty `.tool-versions.lock`
in the project. Coordination files live in the Atmos XDG data directory under
`locks/tool-versions`.

## Context

Both readers and writers previously opened a sibling lock file. Reading an
existing manifest therefore modified its directory and failed in read-only
projects. The lock must remain stable across independent Atmos processes, even
when the manifest or its parent directories do not exist yet.

## Changes

- Derive the lock filename from the SHA-256 hash of the canonical absolute manifest
  path. Resolve directory and manifest symlinks, including dangling manifest links,
  so aliases share the same lock before and after creation.
- Preserve case where parent filesystem metadata establishes case-sensitive
  lookups; fold names on case-insensitive or unknown filesystems. Darwin metadata,
  Windows per-directory flags, and Linux casefold flags avoid probe files and
  preserve distinct paths on known case-sensitive filesystems.
  Missing suffixes inherit the nearest existing directory's behavior. CodeRabbit
  identified that symlink resolution alone preserved caller casing on macOS,
  producing different lock hashes for the same manifest.
- Pass a properly aligned `uint32` buffer to Windows' case-sensitivity query.
  The original four-byte array guaranteed only byte alignment; the native API
  rejected it with `ERROR_NOACCESS`. Preserve error propagation rather than
  treating memory-access failures as a case-insensitive filesystem fallback.
- Retain shared reader locks and exclusive writer locks. Only writers create
  manifest directories; missing-manifest reads create neither project nor data
  directories. Lock-directory and lock-acquisition failures prevent the operation.
- Keep XDG lock files after release to preserve their inode identity. Leave legacy
  project-local lock files untouched. Processes coordinating an update must use the
  same XDG data directory and lock convention.
- Reject an existing file used as a parent directory during canonicalization.
  Windows CI exposed that a missing-path error could otherwise allow the shared
  lock callback to run for an invalid manifest path. Retain the original path
  error and leave the callback uncalled on every platform.
- Add regressions for project cleanliness, legacy lock preservation, missing
  paths, read-only projects, path aliases, failure propagation, reader/writer
  exclusion, concurrent updates/readers in five separate processes, and a synchronized
  cross-process shared-lock test that excludes writers until release. Process
  fixtures use explicit owner/repo tool names to avoid registry network access.

## Validation

- `go test -race -count=3 ./pkg/toolchain -run 'TestToolVersions|TestAddToolToVersionsConcurrent' -timeout=5m` passed.
- `./custom-gcl run --new-from-rev=HEAD ./pkg/toolchain` reported zero issues.
- The lock regressions passed three race-enabled repetitions after the Windows
  path correction (8.754 seconds); Windows execution remains a CI check.

- Case-variant lock regressions failed before the fix for missing parent
  directories, missing manifests, and existing manifests on macOS. The corrected
  tests verify key stability before/after creation, shared/exclusive exclusion,
  and project cleanliness. Three race-enabled repetitions of the tool-version
  tests passed (81.645 seconds).
- Linux container execution passed the case-identity, native-detection, symlink,
  missing-read, and read-only regressions on a case-sensitive filesystem. Linux
  and Windows toolchain test binaries cross-compiled successfully; Windows runtime
  behavior remains a CI check.
- The full toolchain package passed in 93.589 seconds with 82.7% package coverage.
  The macOS executable lines added by the case-alias correction had 20/22
  covered (90.91%); this local estimate excludes platform-specific files not
  compiled on macOS, so CI coverage remains authoritative. After including the
  Linux container profile (including the procfs unsupported-ioctl fallback), the
  full core patch estimate was 703/812 (86.58%). Windows invalid-path and missing-
  directory regressions are included for CI execution; its native API fallback
  for older filesystems is not exercised locally.

- Core CI run `37216028955`, Windows build job `111476990631`, reproduced
  `resolve .tool-versions path casing: Invalid access to memory location.` twice
  while running `atmos toolchain install` (including its retry). Existing
  `TestToolVersionsCaseSensitivityMatchesFilesystem` and
  `TestToolVersionsLockCaseAliases` exercise the native query and manifest locks;
  they remain the behavioral regressions for the aligned-buffer correction.

- After the Windows buffer correction,
  `GOOS=windows GOARCH=amd64 go test -c -o ../toolchain-windows-aligned.test.exe ./pkg/toolchain`
  passed. Both macOS and Windows-targeted
  `./custom-gcl run --new-from-rev=HEAD ./pkg/toolchain` reported zero issues.
  Native Windows build job `111479530832` subsequently passed the same toolchain
  installation in CI, confirming the corrected native call executes successfully.

- Follow-up review found that unsupported Linux inode flags and generic Unix
  metadata do not establish case-sensitive lookups (exFAT and FreeBSD msdosfs
  are counterexamples). Unknown filesystems now use deterministic folded lock
  keys. Linux virtual filesystems and FreeBSD UFS retain their proven sensitive
  behavior; Darwin and Windows retain their native case queries. The policy does
  not inspect directory contents or create probe files, so creating a manifest
  cannot change a held lock's key. Unknown case-sensitive filesystems may
  serialize distinct case-variant manifests; manifest I/O retains original paths.
- A disposable Linux exFAT image reproduced the old binary's alias-lock failures
  for missing parents, missing manifests, and existing manifests. The corrected
  binary passed those cases, key stability while the creating writer held its
  lock, missing-read cleanliness, native metadata policy, and five separate
  processes updating/reading the same manifest through case aliases. The image
  was unmounted and its container removed after the run.
- Final affected lock tests passed three race-enabled repetitions (9.609 seconds).
  Linux, Windows, and FreeBSD toolchain tests cross-compiled; Darwin, Linux,
  Windows, and FreeBSD lint passed. FreeBSD runtime execution is unavailable locally.
- Windows CI also exposed an incorrect comparison of opaque `os.DirEntry`
  metadata: reads can change access timestamps. Project-cleanliness assertions
  now compare recursive path names/types and file bytes, covering the intended
  no-project-writes contract without assuming access timestamps remain fixed.

## Follow-ups

None.
