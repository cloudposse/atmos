# Fix: Match Windows release assets and clear affected-file validation

**Date:** 2026-10-09

## Summary

Correct the mocked Terraform release asset used by the latest-version install test on Windows, and remove trailing whitespace from the docs-generation test case.

## Context

Windows acceptance shard 4 failed because the raw-binary installer requested `terraform.exe`, while the mock registered only `terraform`. Both pre-commit and affected-file validation also rejected a trailing space on the existing `## Providers` expectation in the newly modified docs-generation test-case file.

## Changes

- Register the release asset with the installer's platform-aware executable suffix helper, preserving the end-to-end install and file-content assertions.
- Remove the trailing space without changing the expected text.

## Validation

- Reproduced the EditorConfig failure locally with `atmos validate --affected --base origin/main` and the CI exclusions; the same command passed after the fix.
- The focused latest-version install test and existing cross-platform mock asset URL tests passed, including the Windows `.exe` URL case on macOS.
- Patch-scoped custom golangci-lint passed.
- Native Windows execution is left to the PR CI matrix.
- Production Go code is unchanged, preserving the previously verified 100% patch coverage.

## Follow-ups

None.
