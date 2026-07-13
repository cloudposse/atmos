# List On-Error Graceful Degradation

## Problem

`atmos list stacks`/`list components`/`list settings` aborted the entire command the instant
any one component's `!terraform.state`/`!terraform.output` failed to resolve — most commonly
because the component's Terraform backend had not been provisioned yet. One unprovisioned
component made it impossible to list every other, already-provisioned stack.

The only workaround, `--process-functions=false --process-templates=false`, disabled all
function/template rendering — losing every computed column, not just the unresolvable ones.

## Root Cause

`internal/exec/yaml_func_utils.go`'s `processNodesWithContext` — the single recursive walker
used by every YAML function type — treated the first per-value error as fatal for the whole
call: it set `firstErr` and unwound without processing any remaining keys, stacks, or
components.

## Fix

A new opt-in `--on-error=warn` mode (default remains `strict`, i.e. today's behavior) narrowly
tolerates errors already classified recoverable by `isRecoverableTerraformError` — a backend or
state that has not been provisioned yet (`ErrTerraformStateNotProvisioned` /
`ErrTerraformOutputNotFound`). In `warn` mode, a recoverable per-value error is substituted with
`null`, reported once via `ui.Warningf` with the offending stack/component/function, and
processing continues with the next key, component, and stack. Every other error class — auth
failures, malformed YAML, misconfiguration — still fails the command exactly as before.

This deliberately does *not* reuse or reverse the auth fail-fast decision described in
[list-instances-auth-fail-fast.md](2026-05-15-list-instances-auth-fail-fast.md): that fix made a
fatal *misconfiguration* (a declared default identity that isn't configured) fail fast instead of
silently falling back to the wrong identity. This fix only degrades a fundamentally different
failure class — *infrastructure not yet provisioned* — and only when the caller explicitly opts
in with `--on-error=warn`.

## Expected Behavior

- `atmos list stacks` / `list components` / `list settings` with no `--on-error` flag (or
  `--on-error=strict`): unchanged — the first unresolvable value still aborts the command.
- `--on-error=warn`: an unresolvable `!terraform.state`/`!terraform.output` value becomes `null`
  in the output, a warning naming the stack/component/function is printed to stderr, and the rest
  of the command completes normally with exit code 0.
- Errors unrelated to backend provisioning (auth, malformed YAML, etc.) still fail the command
  even with `--on-error=warn`.
