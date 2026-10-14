# Fix: ghtest records requests on arrival, not after the handler returns

**Date:** 2026-10-07

## Summary

The fake GitHub API used by the CI provider tests appended a request to its log only after the handler returned. A large response reaches the client before that, so a client that immediately sends the next request could be logged first, or a test could read the log before the first request was in it. `TestProvider_PostComment_PaginatesListSearch` failed intermittently on the macOS acceptance shard with one GET recorded instead of two. The server now records a request as soon as it arrives and fills in the status when the handler finishes.

## Context

The failing assertion reported the recorded requests as the page-2 GET followed by the PATCH, with the page-1 GET absent. The page-1 handler had served its response (the client could not have asked for page 2 otherwise), so the record was being appended too late rather than never.

Two properties combine to open the window. A 100-comment page is larger than net/http's 4 KiB write buffer, so the JSON body is written to the connection during the handler, not at its return. And go-github decodes a page with `json.Decoder.Decode`, which returns once the JSON value is complete rather than at end of body. The client therefore has the full first page while the server goroutine is still unwinding, opens a second connection for page 2, edits the match, and returns to the test, which reads `Requests()` before the deferred append on the first goroutine has run. Widening that window with a 50 ms delay after the body write reproduced the CI failure locally on every run.

## Changes

- `Server.serve` in `pkg/ci/providers/github/ghtest/server.go` appends the record under the lock before dispatching and stores its index; the deferred function only sets `Status`.
- `RecordedRequest.Status` is documented as zero while the handler is still running.
- An unexported `afterRoute` hook runs after a handler returns, for internal tests only.
- `TestServer_RecordsRequestsOnArrival` holds a handler open with the hook, decodes the first page on the client, and asserts the request is already listed with a zero status, then that the status becomes 200 once released. The test fails against the previous recorder.

## Validation

- `go test ./pkg/ci/providers/github/... -count=1`
- The pagination test passes repeatedly with the 50 ms delay experiment applied on top of the fix, and fails on every run with the delay applied to the previous recorder.
- `./custom-gcl run --new-from-rev=HEAD ./pkg/ci/providers/github/...`

## Follow-ups

None.
