# Fix: streaming UI no longer crashes when output is a forced/real TTY but input isn't

**Date:** 2026-08-25

## Summary

`atmos terraform apply|deploy|plan|init|destroy --ui` crashed with `could not open a new
TTY: open /dev/tty: device not configured` whenever stdout was treated as TTY-capable but
stdin had no real controlling terminal (e.g. `ATMOS_FORCE_TTY=true` with stdin redirected
from `/dev/null`, or any process supervisor that attaches a terminal to stdout/stderr but not
stdin). Bubbletea's program now runs with `tea.WithInput(nil)` in that case, matching a guard
`pkg/ui/spinner` already had — the run no longer needs to read a keypress when
`-auto-approve` is set, so it doesn't need real terminal input either.

## Context

Found while generating a `--ui` demo recording for the streaming-UI changelog post: the
recording (via the cast-recording pipeline's `mode: steps`, which runs commands without a
real pty) failed with the `/dev/tty` error. `pkg/terraform/ui/executor.go`'s `runTUIProgram`
constructed `tea.NewProgram(model, tea.WithOutput(iolib.UI), tea.WithoutSignalHandler())`
unconditionally — Bubbletea's default input reader unconditionally opens `/dev/tty` at
startup regardless of whether the run ever reads a keypress, and that open fails hard when
there's no real controlling terminal. `pkg/ui/spinner`'s own `tea.NewProgram` construction
(`ExecWithSpinner`) already carries a `!terminal.HasRealTTYInput()` guard for exactly this
scenario, added for the same class of forced-TTY/no-real-input environment (screenshots, cast
recordings). `pkg/terraform/ui` never got the equivalent fix when it was built.

`term.IsTTYSupportForStdout()` (used to decide whether to launch streaming UI at all) is
force-TTY-aware and checks stdout only; `terminal.HasRealTTYInput()` is the unforced,
stdin-specific check `pkg/ui/spinner` uses to decide whether it's safe to let Bubbletea touch
`/dev/tty` at all — the two checks answer different questions and both are needed.

Manually verified: with `ATMOS_FORCE_TTY=true` and stdin redirected from `/dev/null`, the
pre-fix binary crashed with the `/dev/tty` error; the post-fix binary completed a full
`terraform deploy vpc -s dev -auto-approve --ui` run cleanly, including the condensed
completion summary and outputs table.

## Changes

- `pkg/terraform/ui/executor.go`: `runTUIProgram` now appends `tea.WithInput(nil)` to the
  Bubbletea program options when `terminal.HasRealTTYInput()` is false, mirroring
  `pkg/ui/spinner`'s `ExecWithSpinner` guard.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/terraform/ui/... ./pkg/ui/spinner/...` — all pass (no regression; the new
  branch isn't independently unit-testable without a real Bubbletea program run, matching the
  same accepted limitation `pkg/ui/spinner` already has for its equivalent guard, and the
  documented TTY-testability limitation in `internal/exec/terraform_streaming_ui_test.go`).
- Manual before/after repro: built the binary pre-fix and post-fix, ran
  `ATMOS_FORCE_TTY=true ... atmos terraform deploy vpc -s dev -auto-approve --ui < /dev/null`
  against the `demo/casts/fixtures/native-terraform` fixture — crashed pre-fix with the exact
  `/dev/tty` error, completed cleanly post-fix.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.
- New `--ui` cast recording (`website/static/casts/demo/fixtures/native-terraform/deploy-ui.cast`,
  generated via `atmos casts generate demo fixtures native-terraform deploy-ui`, `mode:
  session` since Bubbletea still needs a real pty for its own raw-mode setup even with this
  fix) — regenerated and validated after this fix; embedded in the streaming-UI changelog
  post.

## Follow-ups

None. The cast-recording harness's `mode: steps` path (in-process `mvdan/sh` interpreter, not
a plain subprocess spawn) still doesn't pick up this fix for `--ui` specifically — worked
around by using `mode: session` for that one recording, which was already the architecturally
correct choice for a live, timing-sensitive TUI demo. Not filed as a tracked follow-up: no
other `mode: steps` recording has needed this, and `mode: session` is already the documented
right tool for exactly this kind of recording.
