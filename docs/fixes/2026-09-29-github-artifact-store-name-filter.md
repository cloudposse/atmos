# Fix: `github/artifacts` store no longer depends on the repo-wide artifact listing

**Date:** 2026-09-29

## Summary

`atmos terraform planfile list` (and every other exact-key operation) failed in CI because the
`github/artifacts` planfile store called the unfiltered `GET /repos/{owner}/{repo}/actions/artifacts`
endpoint, which GitHub answered with an empty-body HTTP 500 for the `cloudposse/atmos` repository.
Exact-key operations now use the server-side `name` filter, and `List` falls back to the current
run's artifacts when the repo-wide listing fails with a server error.

## Context

The unfiltered listing returned HTTP 500 persistently (more than an hour) for one repository while
other repositories and the name-filtered and run-scoped endpoints kept working. Retries with backoff
cannot fix a persistent failure. The `planfile-artifacts-e2e` workflow (job `plan (upload planfile)`,
step `atmos terraform planfile list mycomponent -s prod --format=json`) failed as a result.

`Exists`, `Delete`, `GetMetadata` and `findArtifact` also inspected only page 1 of the repo-wide
listing, that is the newest 100 artifacts of the whole repository, so they could report a
key as missing on a busy repository.

## Changes

- `pkg/ci/artifact/github/store_query.go` (new) holds the list request code moved out of `store.go`:
  a `listArtifactsQuery` (page, per-page, optional exact `name`, optional `runID`) whose query string is
  built with `net/url` so the name is escaped, plus the retry/backoff and single-attempt code unchanged.
- `findArtifact`, `Exists`, `Delete` and `GetMetadata` now query with `name=<artifact name>` and keep the
  `Name ==` equality guard; first match in the returned order still wins and the error sentinels are unchanged.
- `List` keeps the repo-wide paginated listing as the primary path. When it fails with an HTTP 5xx after the
  existing retries and `GITHUB_RUN_ID` is set, it logs one warning and restarts from scratch against
  `GET /repos/{owner}/{repo}/actions/runs/{GITHUB_RUN_ID}/artifacts`, reusing the same retries, pagination,
  prefix filtering and sorting through a swappable page fetcher. Other failures (401/403/404, network errors)
  and a missing `GITHUB_RUN_ID` return the original error unchanged.
- Non-200 list responses now return a typed `listArtifactsStatusError`; its message and
  `errors.Is(err, ErrArtifactListFailed)` behavior match the previous `fmt.Errorf` form, except that an empty
  response body no longer leaves a trailing `: `.
- `pkg/ci/artifact/github/store_query_test.go` (new) covers the behavior with `httptest` servers.

## Validation

- Wrote `store_query_test.go` first; against the previous `store.go` the name-filter tests and the `List`
  fallback tests failed (for example `TestStore_ExactKeyOperations_UseServerSideNameFilter`,
  `TestStore_findArtifact_KeyBeyondFirstHundredRepoArtifacts`, `TestStore_List_FallsBackToRunScopedListing`).
- After the change `go build ./...` and `go test ./pkg/ci/...` pass, and the new tests pass under `-race`.
- `gofumpt -l pkg/ci/artifact/github` reports no files.
- `./custom-gcl run --new-from-rev=origin/main ./pkg/ci/artifact/github/...` reports 0 issues.
- Not run: the `planfile-artifacts-e2e` workflow against the real GitHub API.

## Follow-ups

None.
