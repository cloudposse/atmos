# Fix: AWS webflow spinner fallback no longer cancels an in-flight token exchange

**Date:** 2026-10-02

## Summary

When the bubbletea spinner failed to start during AWS browser authentication, the fallback canceled
the same context the authorization-code exchange was using. If the OAuth callback had already
arrived, the in-flight exchange was aborted and authentication failed with
`failed to exchange authorization code for credentials: context canceled`. The callback wait and
the exchange now use separate contexts, so ending the wait never aborts an exchange in progress.

## Context

`TestBrowserWebflowInteractive_OpenURLFailure` failed in the `[race] non-acceptance test suite
(shard 1/4)` job on PR #3244 (unrelated to that PR's changes) and passed locally, including with
the CI shuffle seed. Two problems combined:

- The test meant to take the simple (non-spinner) wait, but its TTY mock returned `true` on the
  first call. Because the test calls `browserWebflowInteractive` directly, that first call happened
  inside `waitForCallbackWithSpinner`, so the spinner branch ran. The `TestMain` stub makes that
  branch fail immediately, and the fallback swallowed the failure.
- `handleSpinnerFallback` then canceled the exchange context. Whether that hit the exchange
  depended on scheduling: locally the cancel landed before the simulated callback (30 ms later);
  under CI race-detector load the callback arrived first and the exchange was canceled mid-request.

The production race is real: any spinner start failure that coincides with the callback aborts the
login. It dates from the original webflow implementation (#2148).

## Changes

- `pkg/auth/identities/aws/webflow_browser.go`: `waitForCallbackWithSpinner` derives a `waitCtx` for
  the callback wait. The spinner's abort and the fallback cancel only `waitCtx`, while the exchange
  runs under the timeout context, which the deferred cancel still ends when the function returns.
  `startSpinnerExchangeGoroutine` takes the two contexts separately. Ctrl-C in the spinner still
  quits immediately with `ErrUserAborted`.
- `pkg/auth/identities/aws/webflow_browser_test.go`:
  - `TestBrowserWebflowInteractive_OpenURLFailure` now reports no TTY, so it deterministically takes
    the simple wait it was written for.
  - New `TestWaitForCallbackWithSpinner_FallbackKeepsInFlightExchange` holds the exchange open
    while the spinner fails and asserts its request context is not canceled.

## Validation

- The new regression test fails on the old behavior (exchange under the wait context) with the same
  `context canceled` error seen in CI, and passes with the fix.
- `go test -race ./pkg/auth/identities/aws/ -shuffle=on`: pass, 3 runs.
- `go test -race ./pkg/auth/identities/aws/ -run 'OpenURLFailure|FallbackKeepsInFlightExchange|SpinnerFallback' -count=30 -shuffle=1790918359171285868`:
  pass.
- `./custom-gcl run --new-from-rev=origin/main ./pkg/auth/...`: 0 issues.

## Follow-ups

None.
