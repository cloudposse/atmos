# Fix: Allow artifact downloads in cast validation

**Date:** 2026-10-26

## Summary

Use the shared Harden Runner policy groups for the committed-cast validation job.

## Context

The job installs Atmos from a build artifact after enabling blocked egress. Its
inline allowlist covered GitHub and telemetry, but omitted the GitHub Actions
artifact service and Azure blob-storage hosts used by artifact downloads.

## Changes

Replace the inline allowlist with the repository's Harden Runner action and its
`github`, `github-actions`, `github-blob-storage`, and `telemetry` policy groups.
Egress remains restricted to the selected groups.

## Validation

- `actionlint -shellcheck='' -pyflakes='' .github/workflows/test.yml` passed.
- `node --test .github/actions/harden-runner/runtime/policy.test.cjs` passed all 39
  tests, including validation of every workflow's selected policy groups.
- `git diff --check` passed.

## Follow-ups

None.
