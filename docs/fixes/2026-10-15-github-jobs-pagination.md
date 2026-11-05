# Fix: Bound GitHub Actions job-list page size

**Date:** 2026-10-15

## Summary

Read workflow jobs in pages of 30 so required acceptance checks can validate large reruns without repeatedly hitting GitHub API gateway errors.

## Context

In PR #3100, Tests run `38829287446`, attempt 2, all 30 acceptance shards passed. The required result checks still waited on the jobs API. Repeated requests with `per_page=100` returned HTTP 502; a paginated request with `per_page=30` returned all 98 jobs and confirmed every acceptance shard succeeded.

The earlier macOS shard timeout occurred during Go setup, before tests. Its targeted retry passed setup and the acceptance suite. The pagination failure was a separate problem in the result checks.

## Changes

- Use 30 jobs per page in the shared job reader.
- Continue following every `Link` header, preserving full job enumeration and shard-count validation.
- Update the existing pagination, acceptance-result, and rerun-classification test fixtures to use the smaller page size.
- Preserve retry limits, failure classification, security policy, and required checks.

## Validation

- Live paired API requests reproduced HTTP 502 with 100-job pages and successful enumeration of all 98 jobs with 30-job pages.
- `GOTOOLCHAIN=go1.26.9 go test -race ./internal/ci/rerun ./internal/ci/acceptance` passed.
- `GOTOOLCHAIN=go1.26.9 go test -tags mage ./magefiles/ci` passed, including rerun classification and result checks.
- `GOTOOLCHAIN=go1.26.9 go build ./...` passed.
- The patched `ci:checkShardResults` Mage target passed against the live Linux, macOS, and Windows results for the affected run and attempt.
- All applicable pre-commit checks passed.

## Follow-ups

Verify required result checks and coverage on the published commit. Keep this fix on #3100; update descendants only if a concrete conflict or dependency requires it.
