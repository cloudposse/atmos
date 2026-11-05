# Fix: Workspace setup failures now show Terraform's error

**Date:** 2026-11-02

## Summary

When the pre-command workspace setup failed, `atmos terraform <cmd>` printed only
`Error: subcommand exited with code 1`. Terraform's actual message (backend authentication,
permissions, state lock, network) was captured and then discarded, so the failure could not be
diagnosed without re-running the commands by hand. Fixes cloudposse/atmos#3607.

## Context

Before running a Terraform command, Atmos runs `terraform workspace select -or-create <workspace>`
and, if that exits with code 1, falls back to `terraform workspace new <workspace>`. Both calls
write into a local buffer so the routine first-run text ("Workspace doesn't exist", "You're now on
a new, empty workspace") stays out of the user-facing output. That buffer was never read, so on
failure nothing was shown. When the fallback also failed, only its error was returned, which hid
the output of the original `select -or-create` call as well.

## Changes

- `internal/exec/terraform_execute_helpers_exec.go`: `runWorkspaceSetup` and
  `createWorkspaceFallback` keep the captured output of each step. Failures return an
  `ErrTerraformWorkspaceOp` error built with the error builder, with the labelled Terraform output
  of the `select -or-create` and `new` steps as the explanation, the workspace as context, and a
  hint pointing at backend credentials and `ATMOS_LOGS_LEVEL=debug`.
- The underlying `ExitCodeError` stays in the error chain, so the process exit code is unchanged.
- Successful setup is still silent, and the "workspace already active" recovery is unchanged.
- Output still passes through the secret-masking writer.

## Validation

- `go test ./internal/exec/ -run 'TestRunWorkspaceSetup|TestExecuteShellCommand'` passes, including
  the new `TestRunWorkspaceSetup_FailureSurfacesTerraformOutput`, which failed before the fix.
- `atmos lint --changed` reports 0 issues.
- Not covered: the streaming TUI path (`--ui`). It does not feed the workspace buffer, so the
  explanation is omitted there and the TUI renders the failure itself.

## Follow-ups

None.
