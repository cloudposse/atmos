# Fix: Apply OCI layer whiteouts before extracting current-layer files

**Date:** 2026-10-26

## Summary

OCI tar layers remove lower-layer files through ordinary and opaque whiteouts without leaving marker files or deleting current-layer additions.

## Context

The extractor treated `.wh.*` entries as regular files. Deleted files remained in the destination, including children hidden by `.wh..wh..opq`. The [OCI image layer specification](https://github.com/opencontainers/image-spec/blob/main/layer.md#whiteouts) requires both forms to affect only lower layers, independently of their position in the tar archive.

## Changes

Spool each uncompressed layer to a temporary file while validating its headers. Apply whiteouts before replaying ordinary entries through the existing extractor. Ordinary whiteouts unlink the named lower entry; opaque whiteouts remove lower children and descendants while preserving the directory itself. Marker files are never materialized.

All deletions use `os.Root`. Reject unsafe marker targets, marker directories, and symlink parents; removing a final symlink or hardlink does not modify its external target. Nonwhiteout link-entry handling remains unchanged. The spool uses disk space for one uncompressed layer on the extraction destination's filesystem, rather than requiring that capacity in global temporary storage. It is created exclusively with mode 0600 through the opened `os.Root`, then closed and removed through that same root on success or failure. Root opaque whiteouts preserve only this active spool; unrelated files with similar names are still removed. The prepass rejects archive entries and whiteout targets that resolve to the spool or descend through it, including symlink and hardlink aliases. Replay rechecks file identity before writing.

## Validation

Disposable three-layer tests reproduced stale files and leaked whiteout markers before the fix. They verify ordinary and recursive removals, root and nested opaque whiteouts, same-layer replacements with markers first or last, later-layer deletion, and unrelated-file preservation. Additional tests cover traversal, symlink escape attempts, external hardlink targets, existing unsupported link entries, malformed markers, truncated streams, and spool cleanup. The full `pkg/oci/...` suite passes with the race detector. The OCI package build, scoped patch lint, fix-record validation, and whitespace checks also pass. No external registries are used.

Additional deterministic tests verify that missing or nondirectory lower parents make ordinary and opaque whiteouts harmless. An unavailable global temporary directory reproduced extraction failures for ordinary, whiteout, and root-opaque layers before destination spooling; all three now pass. Closed extraction roots fail without creating a spool. Collision, descendant, symlink, hardlink, replay-alias, malformed-stream, and renamed-destination tests verify failure propagation, preservation of lower files, and confined cleanup. Existing ordering tests still pass, and similar-looking ordinary filenames remain usable.

Full `pkg/oci` tests with Go 1.26.10 and the race detector pass with 93.0% local statement coverage. The new path guard is fully covered. Windows runtime testing exposed a platform-specific assertion: a closed spool reports an invalid handle rather than Unix’s closed-file errno. The regression now first verifies that an open spool accepts the path, then verifies that closing it propagates a path error naming the spool. Updated hosted checks and Codecov results remain CI validation; the open-directory rename test explicitly skips Windows filesystems where this operation is not portable.

## Follow-ups

None.
