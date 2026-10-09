# Fix: Isolate acceptance tests from GitHub rate limits

**Date:** 2026-10-09

## Summary

Route the failing include and toolchain acceptance tests through the existing local GitHub mock server, preserving their behavior and snapshot assertions.

## Context

The Linux and Windows acceptance logs on PR #3351 showed exhausted GitHub API quotas. Include tests injected an HTTP client for raw content, but the downloader's separate rate-limit client still contacted GitHub and exhausted the download deadline. Other CLI tests used a live include fixture, while toolchain tests queried live releases, causing installation failures and missing version tables in snapshots.

## Changes

- Route in-process include rate-limit checks to the same mock that serves their raw content.
- Consolidate duplicate CLI include cases onto the existing mock fixture; retain literal public raw-URL coverage in the in-process tests and real-network coverage in the opt-in live GitHub canaries.
- Use the local basic component fixture for the identity-flag success case.
- Serve fixed releases, publication dates, and registry metadata for the three affected toolchain info snapshots, without changing golden files or expected output.
- Resolve and install `terraform@latest` from a local registry and release asset in isolated cache/install directories; assert both the retained `latest` declaration and the installed file contents.

## Validation

- Focused include tests and identity-flag tests passed.
- The CLI include case and all three affected toolchain info snapshots passed without regeneration.
- Focused toolchain install and GitHub mock-server tests passed, including with `-race`.
- Patch-scoped custom golangci-lint passed with zero issues.
- Local execution used macOS; Linux and Windows execution is delegated to the PR CI matrix.

## Follow-ups

None.
