# Fix: Authentication failures are printed once and read cleanly

**Date:** 2026-10-02

## Summary

When `atmos auth` (and the commands built on it, such as `atmos aws credential-process`) failed to
authenticate, the same error was printed two or three times, and each message stuttered, for example
`authentication failed: authentication failed: failed to authenticate via credential chain ...`.
A failure now produces exactly one error block, the message names the identity once and keeps the real
cause, and hints and explanations attached by providers (for example the AWS SSO "requires an interactive
terminal" guidance) are still shown.

## Context

Three independent problems combined:

- The `pkg/auth` library printed errors itself with `errUtils.CheckErrorAndPrint` at several layers
  (manager, chain, provider, identity-chain step, constructor, prompt, `GetProviderKindForIdentity`,
  the Terraform pre-hook) and also returned them, and the command layer then printed the returned error
  again. A `SuppressAuthErrors` context flag existed only to gate some of those prints.
- Layers wrapped errors with two `%w` verbs (`fmt.Errorf("%w: ...: %w", ErrAuthenticationFailed, ..., err)`).
  The resulting multi-cause error (`Unwrap() []error`) is a leaf for cockroachdb `errors.GetAllHints` and
  `GetAllDetails`, so hints and explanations attached below it were unreachable by the formatter. This is
  why the library printed eagerly in the first place: removing the inner prints alone would have dropped
  the SSO hints.
- Every layer re-added the `ErrAuthenticationFailed` text and its own "failed to authenticate ..." prefix,
  and the command layers prefixed the manager's error once more.

## Changes

- `errors/multicause.go`: new `AllHints` and `AllDetails` that follow the single-cause chain and also
  descend into multi-cause errors (`errors.Join`, multiple `%w`), de-duplicated in encounter order. The
  formatter, `CheckErrorAndPrint` helpers, `ErrorBuilder.WithCause`, and the Sentry reporter use them
  instead of calling cockroachdb directly. `JoinPreservingHints` is unchanged.
- `errors/auth_failure.go`: new `WrapAuthenticationFailed` (states the sentinel once, accumulates scopes such
  as `identity "dev" via provider "sso"`, drops repeated sentinel text, keeps a single-cause `Unwrap` chain so
  hints stay reachable and `errors.Is` matches both the sentinel and the cause), `EnsureAuthenticationFailed`,
  `WrapIdentityAuthFailed`, and `MarkAs` (extra sentinels for `errors.Is` without changing the message).
- `pkg/auth`: removed every in-library print on authentication paths (`manager.go`, `manager_chain.go`,
  `hooks.go`); the pre-hook now returns hinted errors instead of printing and discarding the cause.
  Authentication layers use `WrapAuthenticationFailed`, and the standalone identities
  (`aws/user`, `aws/ambient`, `aws/credential-process`, `ambient`, emulator) no longer add their own
  "authentication failed" prefix. Removed the dead `types.WithSuppressAuthErrors` / `SuppressAuthErrors`
  context plumbing and its tests.
- Command and caller layers no longer re-prefix the manager's error: `cmd/auth` (`env`, `shell`, `exec`,
  `login`, `console`), custom commands in `cmd/cmd_utils.go`, workflow step identities, `credentialprocess`,
  the EKS/ECR/AKS/GKE helpers, and the integration paths in `pkg/auth/manager_integrations.go`.
- `pkg/devcontainer/identity.go`: previously discarded the manager's error and relied on the manager having
  printed it. It now keeps the manager's error as the base of the rendered error.
- `internal/exec/terraform_execute_helpers_exec.go`: the pre-hook failure is logged at debug level because
  the returned error is rendered once by the command boundary.
- Tests: `errors/multicause_test.go`, `errors/auth_failure_test.go`, `pkg/auth/manager_error_output_test.go`
  and `pkg/auth/hooks_error_output_test.go` capture stderr and assert nothing is printed by the library, that
  hints and explanations survive the manager's wrapping and render through the formatter, and that the
  sentinel and identity each appear once. Existing assertions on the old wording were updated.

Caller audit (every call to the manager's `Authenticate`, `GetCachedCredentials`, `Whoami`,
`AuthenticateProvider`, `GetProviderKindForIdentity`, and `NewAuthManager`): all return or render the error,
except `authenticateAdditionalIdentities` in `pkg/auth/hooks.go` (intentionally non-fatal, logs a warning that
includes the error) and the credential-cache probes that fall through to `Authenticate`. `pkg/devcontainer`
was the one caller that relied on the manager's print and is fixed above.

## Validation

- Before the fix, `go test ./pkg/auth/ -run 'PrintsNothing|PrintNothing'` and
  `go test ./errors/ -run 'HintSurvivesMultiCauseWrap|PreservesHintsAcrossMultiCauseCause'` failed.
- After the fix, `go test ./errors/... ./pkg/auth/... ./cmd/auth/... ./cmd/aws/... ./cmd/azure/... ./cmd/gcp/...
  ./pkg/devcontainer/... ./pkg/store/...` passes.
- Rebuilt `build/atmos` and re-ran the real reproductions: `atmos aws credential-process -i cp-fail`,
  `atmos auth env -i cp-fail --login`, and the non-TTY SSO case. Each now prints one `**Error:**` block
  (previously 2, 2, and 3), and the SSO case still shows its hints.
- Final combined run on the branch with all of the 2026-10-02 auth fixes applied: `go build ./...`; `go test` for
  `./errors/... ./pkg/auth/... ./cmd/auth/... ./cmd/aws/... ./cmd/azure/... ./cmd/gcp/... ./pkg/devcontainer/...
  ./pkg/store/... ./pkg/process/... ./pkg/io/... ./pkg/data/...`; `go test -short ./cmd/... ./internal/exec/...`;
  `go test ./tests -run 'TestCLICommands/atmos_(auth|aws)'`; `GOOS=windows go vet` on the auth packages; and
  `./custom-gcl run --new-from-rev=origin/main` (0 issues). All passed.

## Follow-ups

None.
