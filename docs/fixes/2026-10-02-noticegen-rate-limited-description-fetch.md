# Fix: NOTICE generation no longer fails on a rate-limited GitHub description fetch

**Date:** 2026-10-02

## Summary

`go tool mage notice:generate` failed the **Review Dependency Licenses** job with
`fetch repo description: fetch https://api.github.com/repos/cloudposse/atmos: unexpected status 403
Forbidden`. When the GitHub API description fetch fails, noticegen now keeps the tagline already
committed in `NOTICE` (with a warning) instead of failing. It still errors when there is no existing
tagline to reuse.

## Context

`tools/noticegen` fetches the repository description for NOTICE's tagline (#3035). The dependency
review workflow deliberately runs that step without a token, because the job executes
PR-controlled code. Its comment assumed the unauthenticated API was fine at one call per push.
Since #3175 moved public builds to GitHub-hosted runners, the unauthenticated limit
(60 requests/hour) is per IP and shared with every other job on the runner, so a 403 is routine and
unrelated to the PR under test. Adding a token was rejected for the same exfiltration reason the
workflow already documents.

## Changes

- `tools/noticegen/generate.go`: `resolveDescription` uses the live description when the fetch
  succeeds, and otherwise falls back to `existingDescription`. That function parses the tagline
  from a `NOTICE` rendered by `buildHeader` (title, blank line, tagline, copyright line) and
  validates it with the same single-line check as the API response.
- `tools/noticegen/generate_test.go`: covers the fallback, an unrecognized existing `NOTICE` (still
  an error), and the tagline parser (rendered header, CRLF, malformed shapes, control characters,
  missing file).
- `.github/workflows/dependency-review.yml` and `tools/noticegen/README.md`: document the fallback
  in place of the stale "works fine unauthenticated" claim.

## Validation

- `go test ./...` in `tools/noticegen`: pass.
- `go run -C tools/noticegen . <repo> <temp copy of NOTICE>` with network access: identical to the
  committed `NOTICE`.
- The same run with `GH_TOKEN=invalid-token-for-test` (GitHub answers 401) printed the fallback
  warning and produced an identical `NOTICE`.

## Follow-ups

None.
