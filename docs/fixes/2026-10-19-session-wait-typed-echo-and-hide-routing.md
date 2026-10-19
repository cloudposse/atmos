# Fix: Session `wait` no longer matches the typed command's echo, and YAML `hide`/`show` work

**Date:** 2026-10-19

## Summary

Terminal session casts drive a real shell through a PTY, and the PTY echoes typed input back into the
same output stream a `wait` action searches. A script that typed `sleep 3; echo READY`, pressed Enter,
and waited for `READY` matched the echo of its own command at once, before the command ran. Waits now
skip the echo of a line the script typed and submitted when the wait's text or regex matches that typed
line. Separately, YAML `mode: session` casts and VHS tapes recorded in session mode failed at runtime on
`hide`/`show` with `unknown step type`, even though validation accepted them; these actions now reach
the PTY.

## Context

A field-test pass of the Starlark `cast.recorder` API found both problems. The behavior was reproduced
with real recordings on macOS:

- A YAML session with `write: "sleep 3; echo READY"`, Enter, and `wait: {text: READY, timeout: 1s}`
  succeeded even though `READY` prints after three seconds.
- In the recorder API, `rec.terminal.hide()`, `send("sleep 1; echo TOKEN=...; echo READY")`,
  `wait("READY")`, and `show()` published the hidden command's output: the wait returned on the echo,
  `show()` resumed recording, and the command's output was recorded. Values registered for masking, or
  matching a Gitleaks pattern, were still masked (`TOKEN=<MASKED>`), so the defect is a broken
  `hide()` contract and a non-waiting `wait`, not a masking bypass.
- Visible waits whose text also appeared in the typed command passed even when the command printed
  nothing, so saved-cast validation of those waits proved nothing.
- `type: hide` in a YAML session, and `atmos cast record <tape> --mode=session` for a tape that wraps an
  ordinary command in `Hide … Show`, failed with `unknown step type signal: killed`.
  `isCastSessionPTYAction` in `pkg/runner/step/cast.go` routed only `write`, `key`, `pause`, and `wait`
  to the PTY. The same code is on `main`.

A first implementation cut the whole region from the start of typing to the echoed newline out of the
searched text. Testing against the repository's session demos showed two regressions, and the final
rule avoids both:

- Cutting the typed line left the prompt it was typed at as the end of the buffer, so an end-anchored
  prompt wait such as `\$ $` returned before the command finished.
- Full-screen programs redraw while an answer is typed. The `scaffolding` demo's `huh` form drew its next
  question ("Vendor component version") inside the cut region, and the wait timed out.

## Changes

- `pkg/asciicast/session_echo.go` (new): an `echoTracker` on `sessionState` records each line typed with
  `write` actions and submitted with an Enter key. The line's echo ends at the first newline that
  follows the Enter, or at the Enter after a 500 ms grace period when no newline arrives (programs with
  echo disabled). A wait whose text or regex matches a submitted line's typed text searches only after
  that line's echo; other waits search exactly as before. A write without Enter, and a bare Enter, record
  nothing. Tracking is off on Windows, where sessions use pipes with no terminal echo.
- `pkg/asciicast/session_actions.go`: `runAction` reports writes and key presses to the tracker,
  `outputMatches` applies the tracker's search start, and `waitForOutput` re-checks when an open echo's
  grace period expires. Timeout and regex parsing moved into `parseWaitAction`.
- `pkg/asciicast/session.go`: the tracker field, its Windows switch, and updated wait-buffer comments.
- `pkg/runner/step/cast.go`: `isCastSessionPTYAction` includes `hide` and `show`.
- Tests: `pkg/asciicast/session_echo_test.go` (new) covers scoped and unscoped waits, the end-anchored
  prompt case, a form redraw while typing, open echoes and the grace timer, writes without Enter, bare
  Enter, Enter aliases, and a real `RunSession` whose fake shell prints only after a delay.
  `pkg/runner/step/cast_test.go` checks `hide`/`show` routing, and
  `pkg/runner/step/automation_cast_test.go` checks that a hidden command's output stays out of a
  recorder cast. Session tests that typed before the fake shell started reading now wait for a startup
  banner from the helper shells in both `testmain_test.go` files; several had passed only because their
  wait matched the typed echo.
- Docs: `website/docs/functions/automation/cast.recorder.mdx`, `website/docs/steps/type/cast.mdx`, and
  `agent-skills/skills/atmos-cast/SKILL.md` state that a wait never matches the echo of a submitted line.

## Validation

- `go build ./...` passed.
- `go test -count=1 ./pkg/asciicast/... ./pkg/runner/step/... ./pkg/castplan/... ./pkg/script/starlark/...`
  passed. `go test -race -count=1 ./pkg/asciicast/` passed during implementation.
- The two regression tests failed with the tracker disabled, confirming they exercise the defect.
- `./custom-gcl run --new-from-rev=origin/main ./pkg/asciicast/... ./pkg/runner/step/...` reported
  0 issues; `gofumpt -l` reported no files.
- Real recordings with a fresh `build/atmos` (scratch fixtures under the gitignored `.context/`):
  - YAML `sleep 3; echo READY` with a 1 s wait now times out; with a longer timeout it waits for the
    output.
  - YAML and recorder `hide → send → wait → show` sequences publish no hidden output.
  - A YAML end-anchored prompt wait (`\$ $`) after `sleep 2; echo done` no longer returns early; the cast
    shows `done` before the next typed command.
  - `atmos cast record` of a tape with `Hide`/`Show` around `echo` records the cast without the hidden
    output.
  - Copies of the YAML session demos `cli/interactive-menu`, `examples/interactive-workflows/table-and-build`,
    `examples/scaffolding/generate-example`, and `examples/scaffolding-matrix/generate-example`, with
    outputs redirected to `.context/`, all recorded successfully. A build with the tracker disabled was
    used to confirm earlier scratch-setup failures were not caused by this change.
  - The recorder demo `init-basic` (an interactive `atmos init` session) recorded successfully.
- Not run: Windows (tracking is disabled there by design), and `demo/fixtures/ai-ask/interactive-ask`,
  which needs a logged-in `claude`. Committed casts under `website/static/casts/` were not regenerated.
- `cd website && npm run build` passed with no broken-link errors.

## Follow-ups

The remaining findings from the same `cast.recorder` field-test pass are introduced by this branch and
stay in scope for this PR rather than becoming issues. That includes the demo `finish()` defaulting to
generation in standalone scripts, the demo policy's color check, the terminal last-command exit status,
assertions on replaced values, fingerprint portability, the policy return value, and the error-message
improvements. Re-recording the three screengrab help casts recorded outside the pipeline in #3278
(`atmos-config-get--help`, `atmos-stack-config-get--help`, `atmos-stack-get--help`) is awaiting a
decision.
