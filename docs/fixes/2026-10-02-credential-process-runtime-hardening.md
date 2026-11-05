# Fix: Credential process runtime hardening

**Date:** 2026-10-02

## Summary

A field test of the `aws/credential-process` identity found invisible prompts that hang, keyring
persistence of helper output, a logout that did not log out, a helper timeout that overran by 12
seconds, validation that ran too late or not at all, error hints that the docs promised but did not
exist, and a Windows-only test bug. This record covers the fixes for all of them.

## Context

- `isInteractive()` only checked stdin. The identity selector and the MFA and credential forms are
  drawn on stderr, so when a parent process captures stderr (the AWS CLI running a `credential_process`
  helper) the form was invisible and the process hung waiting for input nobody could see.
- The SSO device-flow hint told users to run `aws sso login`. Atmos keeps its own SSO token cache, so
  that never helped.
- `auth whoami -i cp-noexp` stored long-lived helper output in the keyring through
  `cacheWhoamiCredentials`, contradicting the documented "never stored in the keyring" contract.
- `auth logout` deliberately preserves keyring entries unless `--keychain` is given (documented for
  `aws/user` access keys). Entries for helper-owned credentials were therefore never removed, and chains
  kept reusing them.
- `cp-slow` (a helper that runs `sleep 70`) took 72 seconds instead of the documented minute. Killing
  the shell on timeout left the grandchild holding the stdout pipe, and `exec.Cmd.WaitDelay` was unset,
  so `Wait` blocked until the grandchild exited.
- `via` on an `aws/credential-process` identity was rejected only after the upstream identity's helper
  already ran. `session:` and `principal:` were silently ignored.
- The PRD error table promised hints for `ErrCredentialProcessFailed`, `ErrCredentialProcessInvalidOutput`
  and `ErrCredentialProcessRecursion`; none existed.
- `TestDefaultCredentialProcessCommandBuilder` expected `Args == [cmd.exe /C <cmd>]` on Windows, but
  `pkg/process` builds `Args=[%COMSPEC%]` and carries the command line in `SysProcAttr.CmdLine`.

## Changes

- **Shared interactivity predicate.** New package `pkg/auth/interactive` decides whether a prompt can be
  shown: interactive mode enabled, stdin AND stderr are terminals (honoring `--force-tty`), and not CI.
  It takes an injectable `Environment`. `pkg/auth/manager.go` (`isInteractive`) and `cmd/auth/login.go`
  now delegate to it. The production MFA prompt and credential prompt in `pkg/auth/identities/aws` fail
  fast with the new `ErrAuthPromptUnavailable` instead of running huh. The Azure PIM justification prompt
  also requires stderr to be a terminal. When a prompt is needed but impossible, the errors explain why
  and hint `atmos auth login --identity=<name>` (identity selection errors also hint `--identity=<name>`).
- **SSO hints.** The device-flow hint, the device-authorization failure hint, the timeout hint, and the
  provisioning hint now point at `atmos auth login --provider=<name>` and say Atmos has its own token
  cache. The CI and OIDC hints stay.
- **No keyring persistence for helper credentials.** New optional interface
  `types.CredentialPersistence` (`PersistsCredentials() bool`, helper `types.PersistsCredentialsInKeyring`).
  `aws/credential-process` returns false. The manager asks the identity, never the kind, and skips every
  keyring write (whoami cache, chain steps) and read (`loadCredentialsWithFallback`) for such identities.
  Entries written by older versions are purged on whoami and chain authentication.
- **Logout really logs out.** `Logout` treats a non-persisting identity like an ambient chain: it deletes the
  keyring entry (best effort, no `--keychain`, no partial-logout error when nothing exists). Regular
  identities keep the documented behavior of preserving keyring credentials unless `--keychain` is passed.
- **Helper timeout.** `DefaultCredentialProcessCommandBuilder` sets `WaitDelay` to 2 seconds, and
  `RetrieveProcessCredentials` applies the same bound to custom builders. The timeout now returns
  promptly. Grandchildren that outlive the killed shell are not terminated; they only stop holding Atmos.
- **Validation.** The manager validates every identity in the chain before authenticating any step
  (`validateChainIdentities`), so a bad later identity no longer runs an upstream helper first.
  `aws/credential-process` now rejects `session` and `principal` with hints.
- **Error hints.** `ErrCredentialProcessFailed`, `ErrCredentialProcessInvalidOutput` and
  `ErrCredentialProcessRecursion` are built with the error builder (single cause, identity in the message
  and context) and carry the hints from the PRD. Hints never include helper stdout.
- **Windows test.** The shell-invocation assertion moved to `credential_process_shell_unix_test.go` and
  `credential_process_shell_windows_test.go` (the Windows one asserts `SysProcAttr.CmdLine`).
- **Test assertions.** `require.Error` became `require.ErrorIs` in the credential-process identity tests,
  and the lifecycle test asserts the SDK error text because no sentinel exists for "no files".
- **Docs.** `website/docs/cli/configuration/auth/identities.mdx` lists the rejected settings, the logout
  cleanup, and the bounded timeout.

## Validation

- `go build ./...` and `go test ./pkg/auth/... ./pkg/process/... ./cmd/auth/...` pass.
- `GOOS=windows go vet ./pkg/auth/cloud/aws/ ./pkg/process/ ./pkg/auth/ ./pkg/auth/identities/... ./cmd/auth/` is clean.
- `GOTOOLCHAIN=local ./custom-gcl run --new-from-rev=origin/main ./pkg/auth/... ./pkg/process/... ./cmd/auth/...` reports 0 issues.
- Before-fix failures confirmed: the grandchild timeout test took 12.09s against an 8s limit without
  `WaitDelay`; the non-persistence tests (whoami, chain step, keyring fallback, logout) fail when the
  identity reports `PersistsCredentials() == true`.
- Live field test against the fixture: `auth whoami`/`auth login -i cp-noexp` leave the keyring empty; a
  planted stale entry is removed by `auth logout -i cp-noexp` (no `--keychain`) and by `auth whoami`;
  `cp-slow` returned after 63.9s total instead of 72s; the `via2` fixture reports the `via` error with
  the `other` helper counter unchanged; `session.duration: 12h` is rejected with a hint; under a pty
  with stderr redirected to a file, `auth env` returns the prompt-unavailable error with the login hint
  instead of hanging.
- `cd website && npm run build` succeeded after the follow-on docs pass that also covered this change's prose edits.
- Final combined run on the branch with all of the 2026-10-02 auth fixes applied: `go build ./...`; `go test` for
  `./errors/... ./pkg/auth/... ./cmd/auth/... ./cmd/aws/... ./cmd/azure/... ./cmd/gcp/... ./pkg/devcontainer/...
  ./pkg/store/... ./pkg/process/... ./pkg/io/... ./pkg/data/...`; `go test -short ./cmd/... ./internal/exec/...`;
  `go test ./tests -run 'TestCLICommands/atmos_(auth|aws)'`; `GOOS=windows go vet` on the auth packages; and
  `./custom-gcl run --new-from-rev=origin/main` (0 issues). All passed.

## Follow-ups

None.
