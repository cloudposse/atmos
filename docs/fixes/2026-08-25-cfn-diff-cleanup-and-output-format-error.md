# Fix: `diff`/`plan` no longer leaks changesets; bad `output --format` now fails instead of exiting 0

**Date:** 2026-08-25

## Summary

`diff`/`plan` created a real AWS changeset to render its preview and never deleted it, leaking an
AWS object (against the account's changeset quota) on every single run. Separately, `output` (and
`apply`'s end-of-deploy Outputs summary) silently swallowed a bad `--format` value via `ui.Error` and
returned success — a CI pipeline piping stdout would get an empty file and a green exit code.

## Context

Found during a field-test pass, both confirmed live against Floci: one `deploy` + one `diff` on the
same stack left 2 changesets behind (`changeset list`); `atmos aws cfn output demo -s local
--format=bogus` exited 0 with completely empty stdout.

## Changes

**`diff`/`plan`** (`pkg/component/aws/cloudformation/executor.go`'s `runDiff`): after rendering the
diff summary, deletes the changeset it created via the existing `deleteChangeSet` (already used by
`changeset delete`) — regardless of no-op or real-changes outcome. A delete failure is surfaced via
`ui.Warning`, not propagated as a command error: the diff itself already succeeded and was rendered,
so a cleanup failure shouldn't fail the command.

**`output --format`** (`executor.go`'s `renderOutputsSummary`): changed from `func(...)` (void,
swallowing the format error via `ui.Error`) to `func(...) error`, propagated through both callers
(`runOutput` and `runApply`'s post-apply summary). Uses the existing generic `errUtils.ErrInvalidFlag`
sentinel (not a new one — this is a flag-validation error, not an AWS-API or changeset concern).
Confirmed safe for `apply`: `--format` is only registered on the `output` subcommand
(`cloudformation.go`), never on `apply`/`deploy`, so `runApply`'s `format` always resolves to the
safe default — this change can only newly fail `output` itself when the user passes a bad value.

## Validation

- New/updated tests: `TestRunDiff` (now expects the `DeleteChangeSet` call),
  `TestRunDiff_CleanupFailureIsNonFatal`, `TestOperationHandlers_Dispatch`'s `diff` case (same),
  `TestRunOutput_InvalidFormatPropagatesError`.
- `go test ./pkg/component/aws/cloudformation/...` — all pass.
- `atmos lint --changed` — clean (one pre-existing, unrelated finding remains).
- Live, against Floci: one `deploy` + one `diff` now leaves 0 changesets behind (previously 1 per
  `diff` run); `output --format=bogus` now exits non-zero.

## Follow-ups

None.
