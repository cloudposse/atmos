# Fix: Apply OCI layer whiteouts before extracting current-layer files

**Date:** 2026-10-26

## Summary

OCI tar layers remove lower-layer files through ordinary and opaque whiteouts without leaving marker files or deleting current-layer additions.

## Context

The extractor treated `.wh.*` entries as regular files. Deleted files remained in the destination, including children hidden by `.wh..wh..opq`. The [OCI image layer specification](https://github.com/opencontainers/image-spec/blob/main/layer.md#whiteouts) requires both forms to affect only lower layers, independently of their position in the tar archive.

## Changes

Spool each uncompressed layer to a temporary file while validating its headers. Apply whiteouts before replaying ordinary entries through the existing extractor. Ordinary whiteouts unlink the named lower entry; opaque whiteouts remove lower children and descendants while preserving the directory itself. Marker files are never materialized.

All deletions use `os.Root`. Reject unsafe marker targets, marker directories, and symlink parents; removing a final symlink or hardlink does not modify its external target. Nonwhiteout link-entry handling remains unchanged. The spool uses disk space for one uncompressed layer and is removed on success and failure.

## Validation

Disposable three-layer tests reproduced stale files and leaked whiteout markers before the fix. They verify ordinary and recursive removals, root and nested opaque whiteouts, same-layer replacements with markers first or last, later-layer deletion, and unrelated-file preservation. Additional tests cover traversal, symlink escape attempts, external hardlink targets, existing unsupported link entries, malformed markers, truncated streams, and spool cleanup. The full `pkg/oci/...` suite passes with the race detector. The OCI package build, scoped patch lint, fix-record validation, and whitespace checks also pass. No external registries are used.

## Follow-ups

None.
