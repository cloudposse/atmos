# Fix: Keep SHA verification diagnostics usable during API rate limits

**Date:** 2026-10-06

## Summary

Stop new tag lookups after GitHub reports an exhausted installation quota and
pass verification results between action steps through a temporary file.

## Context

The SHA verification job received rate-limit errors for 235 action references.
Its comment step then failed to start with `Argument list too long` because the
complete JSON diagnostics exceeded the runner's environment-variable limit.
Semver labeling and autofix also failed on API rate limits during the same run.

## Changes

- Stop new tag lookups on HTTP 403 with `x-ratelimit-remaining: 0`. Cached
  successes remain usable; unresolved references still fail verification.
- Write complete results to a unique file under `RUNNER_TEMP` and pass its path
  to the comment step. Skip commenting if verification produced no results file.
- Limit PR tables to 20 problem references and bound field lengths, linking to
  the action log for complete diagnostics.
- Add offline tests that execute the actual composite-action scripts with
  mocked GitHub responses, including a payload over 128 KiB.

## Validation

- All four offline tests passed: quota exhaustion, retained cached successes,
  empty results, and continued checking after an ordinary lookup error.
- `actionlint .github/workflows/verify-sha-pinning.yml` passed.
- Retried the dependency-review, semver-label, and autofix workflows after the
  reported quota-reset time. Their remote results are checked separately.

## Follow-ups

None.
