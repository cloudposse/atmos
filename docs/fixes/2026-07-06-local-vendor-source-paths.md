# Fix: local vendor source path handling

**Date:** 2026-07-06

## Problem

Local source vendoring had edge cases around relative paths, `file://` URIs,
Windows drive paths, existing targets, nil or empty source specs, and unsafe
target paths. These issues affected both direct vendor operations and workflows
that provision a mutable working directory from a local source.

## Fix

Local source resolution now handles relative paths and file URIs consistently,
including localhost and Windows drive forms. Target preparation enforces replace
semantics, creates required parent directories, rejects unsafe targets, and
wraps source copy/download errors with actionable context.

## Tests

```shell
go test ./pkg/provisioner/source -run 'TestResolveSourceURI|TestVendorSource|TestLocalDirectorySource|TestFileURIPath|TestPrepareVendorTarget|TestCopySourceToTarget|TestCopyToTarget' -count=1
```
