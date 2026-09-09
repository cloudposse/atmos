# Fix: streaming UI printed dozens of duplicate progress lines instead of redrawing in place

**Date:** 2026-09-01

## Summary

`atmos terraform plan/apply/deploy/init/destroy --ui` (or `components.terraform.ui.enabled:
true`) in a real terminal printed a new `plan <stack>/<component>` line on every tick instead
of updating one line in place — dozens of duplicate lines scrolling by, each with its own
spinner/progress bar frame, before finally landing on the completion summary. Root cause:
Bubbletea's output writer didn't expose `Fd()`, so Bubbletea silently treated a real terminal
as "not a TTY" and disabled its own line-clearing.

## Context

Reported by the user with three screenshots from a real terminal session running this
worktree's own `build/atmos terraform plan --chdir=~/Dev/cloudposse/infra/infra-live`: healthy
`Init` output, followed by ~25 duplicate `plan core-ue2-auto/vpc` lines each with an
incrementing spinner frame and (later) a colored progress bar, before a correct final "Plan
... completed (no changes)" summary. The bug is purely visual/rendering — the underlying
terraform run completes correctly — but makes the streaming UI unusable in this configuration.

`pkg/terraform/ui/executor.go`'s `runTUIProgram` constructed the Bubbletea program with
`tea.WithOutput(iolib.UI)` — the package-level global UI writer. With secret masking enabled
(the default), `iolib.UI`'s concrete type is `*dynamicMaskedWriter` (`pkg/io/streams.go`),
which implements only `Write()` — no `Fd()`, `Read()`, or `Close()`. Bubbletea's TTY detection
type-asserts its configured output writer for an `Fd()`-capable interface
(`github.com/charmbracelet/x/term`'s `term.File`) to distinguish a real terminal from a
pipe/buffer; failing that assertion, it silently disables cursor movement and line-clearing —
exactly matching the observed symptom.

This exact failure mode was already identified and fixed once before, for a different
Bubbletea program: `pkg/io/global.go`'s `maskedWriter.Fd()` doc comment explicitly describes it
("Without this, wrapping os.Stdout with MaskWriter makes it indistinguishable from a
non-terminal writer to callers like Bubble Tea... See the Bubble Tea program setup in
internal/exec/vendor_model.go for the motivating case"), and `internal/exec/vendor_model.go`
correctly uses `iolib.MaskWriter(os.Stdout)` — which returns a `*maskedWriter` that forwards
`Fd()` to the real underlying `*os.File`. `pkg/terraform/ui/executor.go` was written
independently and used the raw global `iolib.UI` instead of going through `iolib.MaskWriter`,
missing the same fix.

## Changes

- `pkg/terraform/ui/executor.go`: `runTUIProgram` now uses
  `tea.WithOutput(iolib.MaskWriter(os.Stderr))` instead of `tea.WithOutput(iolib.UI)` — same
  masking behavior (writes still go through the global masker), but now via `*maskedWriter`,
  which forwards `Fd()`/`Read()`/`Close()` to the real `os.Stderr`, matching the pattern
  already established in `internal/exec/vendor_model.go`.

## Validation

- `go build ./...` — clean.
- `go test ./pkg/terraform/ui/...` — all pass, no regression.
- `gofumpt -l pkg/terraform/ui/executor.go` — clean.
- Regenerated `website/static/casts/demo/fixtures/native-terraform/deploy-ui.cast` via the
  real-pty cast-recording harness (`atmos casts generate demo fixtures native-terraform
  deploy-ui`) and inspected the raw asciicast stream: each frame now emits the correct
  cursor-up count matching its own line count (`[2A`, `[3A`, `[4A`, ...), i.e. in-place
  redraw, not appended duplicate lines.
- Not reproduced by an automated regression test: the bug only manifests with secret masking
  enabled (the default) against a real terminal file descriptor, which the existing
  `runTeaProgram` test-injection seam bypasses entirely (see
  `internal/exec/terraform_streaming_ui_test.go`'s documented TTY-testability limitation, and
  this session's `2026-08-25-streaming-ui-no-real-tty-input.md` fix for the same class of gap).

## Follow-ups

None.
