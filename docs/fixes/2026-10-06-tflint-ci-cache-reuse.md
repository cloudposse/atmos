# Fix: Reuse the cached TFLint installation in CI

**Date:** 2026-10-06

## Summary

Use the shared CI toolchain action in the quick-start lint matrix and TFLint
hook example so these jobs reuse tools installed by the build job.

## Context

The jobs restored the Atmos toolchain cache, then ran separate single-tool
installs for OpenTofu and TFLint. That installation path downloads and verifies
the tools again. GitHub returned HTTP 403 with `API rate limit exceeded for
installation` while fetching TFLint's release attestation, before linting ran.

## Changes

Both jobs now use `.github/actions/ci-toolchain` with `cache: restore-only`.
The action runs `atmos toolchain install` from `.tool-versions`, skips installed
binaries, retries failed installs once, and exports the toolchain PATH. TFLint
remains pinned to `0.64.0`; fresh installations retain signature verification.

The runner allowlists also include Helm and HashiCorp download hosts so the
shared action can restore the complete pinned toolchain on a cache miss.

## Validation

- `actionlint .github/workflows/test.yml` passed.
- `TestInstallOrSkipTool/tool_already_installed_-_skips` passed, verifying that
  an installed tool is skipped without downloading it.
- GitHub Actions will exercise both updated jobs after the commit is pushed.

## Follow-ups

None.
