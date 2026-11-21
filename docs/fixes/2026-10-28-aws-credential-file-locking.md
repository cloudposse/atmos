# Fix: Serialize AWS credential writes on Windows

**Date:** 2026-10-28

## Summary

Protect shared AWS credentials and configuration files with native file locks on Windows as well as Unix.

## Context

Windows acceptance shard 10 failed in `TestManager_AuthenticateIsolated_Concurrent` while loading a credentials file. AWS file transactions used the cache lock, whose Windows implementation intentionally does not lock. Concurrent load/modify/save transactions could therefore read an incomplete file or overwrite another profile's update.

## Changes

Use `pkg/filelock` for the existing sibling `.lock` file. Preserve the ten-second acquisition timeout and the `ErrFileLockTimeout` and `ErrCacheLocked` error identities. Leave general cache locking unchanged.

Add a concurrent-write regression that verifies every profile survives in both files, and run the existing acquisition-failure assertion on Windows too.

## Validation

- Full tests for `pkg/auth/cloud/aws` and `pkg/auth` pass, including with the race detector.
- The concurrent profile-write and isolated-authentication tests pass 25 repetitions.
- The AWS package test binary cross-compiles for Windows amd64. Native Windows CI remains the platform-specific validation.

## Follow-ups

Confirm Windows acceptance shard 10 passes on the published stack head.
