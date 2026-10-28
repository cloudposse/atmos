# Fix: Validate host-local plugin bundle paths at the archive boundary

**Date:** 2026-10-28

## Summary

Check plugin archive entry names with `filepath.IsLocal` before returning bundle inventory or validating executable content.

## Context

CodeQL reported an archive-name flow through plugin inventory output and a shared YAML buffer pool into unrelated Terraform file operations. The buffer is reset before reuse, so this reported flow does not establish an extraction exploit. Existing bundle validation already rejects traversal, and extraction additionally uses `SafeJoin` and `os.Root` confinement.

The custom bundle validator does not express host-specific path restrictions directly. On Windows, volume-qualified names, alternate data stream syntax, and reserved device names need additional rejection. CodeQL recognizes the standard library's local-path guard directly at the archive boundary.

## Changes

Added `filepath.IsLocal(header.Name)` to the bundle scan before its existing checks. Retained all canonical-path, hidden-file, credential, entry-type, size, and exact-file-set restrictions. Valid names containing embedded `..` remain accepted. Bumped the bundled auth-secrets package to 1.0.1 because the OCI implementation is a declared release input; updated its description test and README pin together.

Regression tests cover traversal, unchanged executable/schema names containing `..`, Windows volume/stream/device rejection, and acceptance of those Windows-specific names on Unix.

## Validation

Passed on macOS with Go 1.26.10:

```shell
go test -race ./pkg/plugin/oci ./pkg/oci -count=1 -timeout=5m
```

Both full package suites passed, including archive traversal and symlink-confinement controls. The fix-log validator and `git diff --check` passed. Windows runtime and hosted CodeQL verification require CI.

## Follow-ups

Verify the Windows-specific regression cases and CodeQL analysis on the published head in CI.
