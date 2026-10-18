# Fix: Remove partial S3 source downloads

**Date:** 2026-10-16

## Summary

Remove a destination file when copying its S3 response fails.

## Context

An interrupted S3 response left a partially written source file on disk. Later consumers could mistake that truncated file for a complete download.

## Changes

- Close the destination before removing a failed download so cleanup also works on Windows.
- Preserve the copy and close errors, and include any cleanup failure in the returned joined error.
- Add a regression whose response contains partial template content before returning an unexpected EOF.

## Validation

- The new regression failed against the original implementation because the partial destination still existed.
- `GOTOOLCHAIN=go1.26.9 go test -race ./pkg/downloader/s3 -count=1` passed.

## Follow-ups

None.
