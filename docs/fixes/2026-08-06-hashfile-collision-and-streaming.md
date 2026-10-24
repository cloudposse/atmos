# Fix: `HashFiles` no longer has path/content collisions and streams large files

**Date:** 2026-08-06

## Summary

`pkg/hashfile.HashFiles` (the freshness checker's `checksum.changed` primitive) hashed
`filepath.Base(p)` followed by raw file content, concatenated with no boundary between records or
between the path and its content. Two distinct real collisions followed: (1) a path `"a"` with
content `"bc"` hashed identically to a path `"ab"` with content `"c"`, since both produce the byte
stream `"abc"`; (2) hashing only the base name (not the full path) meant a file moved between two
directories that share a filename was invisible to the hash if content was unchanged. It also
loaded each file fully into memory via `os.ReadFile` before hashing.

## Context

Flagged in two PR #2882 review threads (`discussion_r3729516454`, `discussion_r3729516446`).
CodeRabbit's own analysis included a runnable Python probe reproducing both collisions against a
model of the original implementation before proposing the fix.

## Changes

- `pkg/hashfile/hashfile.go`: each record (path, then content) is now written into the digest via
  `writeRecord`, which prefixes the data with its own fixed 8-byte big-endian length -- so no
  concatenation of two records can ever produce the same byte stream as a different pair of
  path/content values. The full path (not just its base name) is hashed, so a same-filename move
  between directories now changes the hash. File content is streamed via `os.Open` + `io.Copy`
  directly into the sha256 digest (with a length-prefix computed from `Stat()`) instead of
  `os.ReadFile`, avoiding a full in-memory copy for large task-runner inputs.
- Existing hash values are not preserved across this change (a different byte layout naturally
  produces different digests for the same inputs) -- freshness state persisted under
  `.atmos/cache/freshness/` will see a one-time "changed" on the first run after upgrading,
  exactly like any other cache-key format change. Not a correctness issue.

## Validation

- New tests in `pkg/hashfile/hashfile_test.go`:
  `TestHashFiles_ConcatenationCollisionDoesNotMatch` and
  `TestHashFiles_SameBasenameDifferentDirectoryDiffersFromHash` reproduce both collisions against
  the real file system and assert they no longer match. `TestHashFiles_LargeFileStreamsWithoutError`
  sanity-checks a 5MB file hashes correctly and deterministically.
- Existing tests (`Deterministic`, `ContentChangeChangesHash`, `RenameChangesHash`, `EmptyInput`,
  `MissingFileErrors`) all pass unchanged -- they assert relative properties (equality/inequality),
  not hardcoded digest values, so the format change doesn't affect them.
- `go test ./pkg/hashfile/... ./pkg/runner/freshness/...`: full suites pass.

## Follow-ups

None.
