# Fix: Pro exception events keep the error message instead of `×`

**Date:** 2026-11-03

## Summary

Exceptions that Atmos reports to Atmos Pro now carry the failure's own error text, prefixed with the command
that failed. Before this change, a failed `terraform plan` appeared as an exception whose entire diagnostic was
`×\n(1) ×\nError types: (1) errors.ExitCodeError`, which does not say what failed.

## Context

The exception event is built from `cockroachdb/errors` `BuildSentryReport`. That function redacts every message
it cannot prove is safe, so only static sentinel errors keep their text. `ExitCodeError` (a subcommand exiting
non-zero) and any `fmt.Errorf` wrapper with dynamic context were reduced to `×`.

Atmos already masks every string in the event before it leaves the process (`maskedExceptionEvent`, using the
same masking as terminal output), so the library's blanket redaction was a second, coarser layer that removed
useful text without adding protection. A `SafeFormatError` method on `ExitCodeError` is not enough: even then
the exit code itself prints as `×` unless it is wrapped in `redact.Safe`.

## Changes

- `pkg/pro/exception_event.go`: `buildExceptionEvent` now sets the event message and the top exception value
  from `err.Error()`, prefixed with `<component type> <subcommand>: ` when both are known. The error-type
  breakdown that `cockroachdb/errors` appends to the message is kept, since it names types and no message text.
  The text is masked first and then truncated to 8 KiB on a rune boundary, so one verbose error cannot push the
  event over the 1 MiB envelope limit and drop it, and a secret split by the cut is still masked. `buildExceptionEvent` also tolerates a nil `info`.
- `pkg/pro/exception_event_message_test.go`: tests for the message, the command prefix, wrapped context, the
  retained error-type diagnostics, masking of a registered secret in the built event, a secret that straddles
  the truncation cut, truncation on a rune boundary, the encoded size of a worst-case event, and nil `info`.
- `website/docs/cli/configuration/settings/pro.mdx`: documents the event message, the 8 KiB limit, and that the
  failed command's own output is not part of the event.

Behavior change to be aware of: the message was `×` for all exit-code failures, so they were likely grouped
together by message. They now differ by text. Both the Atmos Pro destination and any separately configured
`errors.sentry` destination receive the same masked event, so both see the new message.

Not changed: `errors.CaptureError` in `errors/sentry.go`, the path used only when no reporter is installed,
still uses `cockroachdb/errors` redaction.

## Validation

- `go test ./pkg/pro`: passes, including the new tests. The new tests failed before the change with the message
  equal to `×`. The straddling-secret test failed against the first version of the change, which truncated
  before masking and sent the first five characters of the secret (`hunte… [truncated]`).
- `atmos lint --changed`: 0 issues, after fixing three `godot` comment findings in the new code.
- `cd website && npm run build`: succeeds. The build reports broken anchors on pages unrelated to this change;
  none involve the edited Pro settings page.
- A scratch program outside the repository, using a copy of `ExitCodeError`, reproduced the `×\n(1) ×\n...`
  message exactly and showed that a `SafeFormatError` method alone still prints `subcommand exited with code ×`.

## Follow-ups

None.
