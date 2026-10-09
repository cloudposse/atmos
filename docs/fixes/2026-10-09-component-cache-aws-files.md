# Fix: Isolate component output caches by AWS credential and config files

**Date:** 2026-10-09

## Summary

Eager `atmos.Component` references using different AWS credential or config files no longer share cached
outputs merely because their profile, region, and endpoint match.

## Context

The process-wide component cache omitted both file paths from its AWS auth key. Different files can
contain the same profile name for different identities, so one caller could receive another's outputs.

## Changes

- Include both quoted AWS file paths alongside the existing profile, region, and endpoint key fields.
- Keep nil-auth and other cloud-provider key behavior unchanged.
- Add an eager CloudFormation component regression that varies each file independently and interleaves
  repeated references, checking returned outputs and exactly one provider request per distinct context.
- Use only existing path metadata; do not read credential contents or add auth details to diagnostics.

## Validation

- The regression failed before the fix: both alternate file contexts received the baseline outputs and
  their provider calls were skipped.
- Focused component cache, CloudFormation output, and auth-resolution tests passed with the race detector.
- `go build ./...` passed.
- Patch-scoped custom golangci-lint for `internal/exec`: 0 issues.
- Fix-log validation and `git diff --check` passed.
- Website build deferred to consolidated stack validation.

## Follow-ups

None.
