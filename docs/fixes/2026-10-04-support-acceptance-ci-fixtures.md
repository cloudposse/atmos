# Fix: Align support-layer acceptance fixtures with terminal and transport behavior

**Date:** 2026-10-04

## Summary

Regenerate the terminal snapshot for lazy logger initialization and recognize
remote HTTP/2 response-stream cancellation in existing live GitHub test handling.
Session-cast tests now wait for the shell response rather than the PTY input echo.

## Context

PR #3263 acceptance run `37205411512` failed the `atmos list instances` TTY
snapshot on Linux and macOS. Lazy logger initialization removes redundant terminal
color queries; the displayed component table is unchanged. The same run's macOS
shard 7 saw GitHub cancel a response stream while `TestGetReleases` decoded it.
The existing transient-error classifier handled request-level `*url.Error`
failures but missed net/http's response-body stream error.

A later core race job (`37219051582`, shard 1) failed the session-cast end-to-end
test with `signal: killed` after 2.72 seconds. This was a process teardown failure,
not a race-detector report. Its substring wait for `ready` also matched the PTY's
echo of the typed `printf ready` command, before the helper process had started.
The test consequently entered the production two-second teardown deadline early.

## Changes

- Regenerate the list-instances TTY snapshot through the acceptance harness.
- Extend only the test helper to recognize the observed remote HTTP/2 CANCEL
  message shape. Keep protocol errors, local cancellation, application errors,
  and decoding errors as failures.
- Add positive and negative classifier regressions. No production request or
  retry behavior changes.

- Match the shell's complete `ready` response line in all session fixture
  handshakes. Preserve Windows LF and Unix PTY CRLF output handling.
- Exercise normal and delayed helper startup in the end-to-end test. The
  test-only three-second delay deterministically exposes the premature echo
  match; production session and teardown timeouts remain unchanged.

## Validation

- Reproduced the TTY snapshot mismatch locally; the only difference was the
  redundant terminal-query prefix.
- `go test ./tests -run '^TestCLICommands/atmos_list_instances$' -count=1 -timeout=10m -regenerate-snapshots`
  passed (38.098 seconds) and regenerated only the expected TTY snapshot.
- `go test ./tests -run '^TestCLICommands/atmos_list_instances$' -count=1 -timeout=10m`
  passed against the regenerated snapshot (39.716 seconds).
- The new response-stream classification regression failed before the helper
  fix for both observed peer-cancellation cases.
- `go test ./pkg/github -run '^TestGetReleases$' -count=1 -timeout=5m`
  passed before the fix (12.790 seconds), consistent with a transient failure.
- `go test ./pkg/github -count=1 -timeout=10m` passed after the fix (18.184 seconds).
- `./custom-gcl run --new-from-rev=HEAD ./pkg/github` reported zero issues.

- Thirty repetitions of the original targeted race test passed (52.486 seconds),
  so repetition alone did not reproduce or rule out the CI failure.
- The delayed-start regression then reproduced the same `signal: killed` failure
  against the original substring wait (2.56 seconds); its normal-start case passed.

- `go test -race -count=3 -run 'TestCastHandlerExecutesSessionModeEndToEnd|TestRunCastSessionMode|TestCastHandlerSessionMode|TestCastSession' -timeout=5m ./pkg/runner/step`
  passed with the complete-response waits (49.279 seconds).
- `go test -race -count=1 -timeout=10m ./pkg/runner/step` passed (49.727 seconds).
- `./custom-gcl run --new-from-rev=HEAD ./pkg/runner/step` reported zero issues;
  the Windows step test binary cross-compiled successfully. Native Windows
  execution remains covered by CI.

## Follow-ups

None.
