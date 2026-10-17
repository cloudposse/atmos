# Fix: Bound S3 source directory detection to its prefix

**Date:** 2026-10-16

## Summary

Detect an S3 source directory with one slash-delimited, one-object listing after its exact object lookup fails.

## Context

An unsuffixed source such as `s3://bucket/prefix` listed all objects beginning with `prefix`. Sibling names such as `prefix-other` sort before `prefix/` and could force many pagination requests before detecting the directory.

## Changes

- List `prefix/` with `MaxKeys: 1`, matching the prefix used to download the directory.
- Reject a version query on forced directory downloads before listing or fetching objects; S3 version IDs identify individual objects.
- Preserve exact-object precedence, versioned object handling, file fallback, and wrapped authorization or transport failures.
- Make the S3 fixture honor the requested prefix and limit, cover sibling-only results, and verify the bounded request.

## Validation

- The focused client-mode regression failed against the original implementation because the request omitted the slash and object limit.
- Forced directory downloads for bucket roots and prefixes fail with `ErrDownloadFile` without S3 calls or downloaded files; versioned single-object downloads still retain their version ID.
- `GOTOOLCHAIN=go1.26.9 go test -race ./pkg/downloader/s3 -count=1` passed.

## Follow-ups

None.
