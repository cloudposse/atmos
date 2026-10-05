# Fix: artifact downloads time out only when progress stops

**Date:** 2026-10-05

## Problem

Testing [PR #3249](https://github.com/cloudposse/atmos/pull/3249) with
`atmos --use-version=3249` failed while downloading its 312 MiB Linux build artifact:

```text
failed to download PR artifact: failed to write file: context deadline exceeded
(Client.Timeout or context cancellation while reading body)
```

The artifact downloader set `http.Client.Timeout` to five minutes. That deadline
covered the entire request, including redirects and reading the response body.
A download still receiving data could therefore fail after five minutes. The
`failed to write file` wrapper obscured that the failure came from a network read.

## Changes

- Replace the total request deadline with a five-minute inactivity watchdog.
  Start it before sending the request and reset it on redirect/final response
  headers and every body read that returns bytes. Active downloads have no total
  duration limit imposed by the downloader.
- Cancel stalled requests through a derived context and report
  `artifact download timed out: no download progress for 5m`, preserving the existing
  `ErrPRArtifactDownloadFailed` classification and caller cancellation causes.
- Present the download-specific timeout directly, without a misleading file-write
  error or the HTTP transport's secondary `context canceled` message.
- Keep caller deadlines, redirect token protections, and HTTP error handling.
- Stop the watchdog on completion. Close incomplete temporary files before
  removing them, including on Windows, and check the final file close for errors.
- Show received bytes, total bytes, and percentage in the artifact download
  spinner. Prefer HTTP Content-Length, fall back to GitHub's artifact size, and
  show received bytes alone when neither is known. Throttle terminal updates to
  100 milliseconds and non-interactive updates to five seconds. Show extraction
  as a separate phase and preserve silent mode.

PR, SHA, and branch artifact installs share this downloader and receive the fix.
There are no new configuration settings or public APIs.

## Validation

Regression tests cover a continuously progressing transfer lasting longer than
the idle interval, header and body timer resets, stalls before headers and during
the body, caller deadlines and cancellation causes, partial-file cleanup, stale
timer callbacks, watchdog shutdown, and the default five-minute error message.
Virtual time keeps these tests fast. A local HTTP server also verifies that a
stalled real response body is interrupted by request cancellation.

The focused artifact, progress, extraction, and error-classification tests pass
under `go test -race ./pkg/toolchain`. These include the existing authenticated
HTTPS and redirect token-stripping tests, and download-to-install tests for
visible progress and silent output. Progress tests verify Content-Length
precedence, artifact-size fallback, unknown totals, and log throttling.

`./custom-gcl run --new-from-rev=origin/main ./pkg/toolchain/...` reports no
issues, and the changed Go files pass `gofumpt` formatting checks.

No live GitHub artifact download is required by these tests.

## Follow-up

The regular toolchain asset installer uses a separate HTTP client with a
30-second total download timeout. That issue is outside this artifact-download
fix and needs a separate change; this PR does not change general HTTP defaults
or the shared go-getter timeout contract.
