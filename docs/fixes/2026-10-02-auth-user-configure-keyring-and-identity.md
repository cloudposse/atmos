# Fix: auth user configure honors auth.keyring and --identity, and auth failure messages keep their separator

**Date:** 2026-10-02

## Summary

A live test against a real AWS account (an `aws/user` identity with MFA, chained to an MFA-gated
assume-role) found three defects. `atmos auth user configure` ignored `auth.keyring`, so credentials went to
the system keychain while `atmos auth login` read the configured file keyring. It also ignored
`--identity`, even though error hints tell users to run `atmos auth user configure --identity <name>`.
Finally, a failed chained login printed `authentication failed interactive authentication prompt
unavailable: requires a TTY`, with no separator and a repeated sentinel.

## Context

Found while validating the standalone chain-root fix live against a real AWS account: configuring an `aws/user`
identity's keys with `atmos auth user configure` in a fixture that used a file keyring put the keys in the macOS
keychain, so `atmos auth login` could not find them, and the error hints pointed at a `--identity` flag that
`configure` ignored. The same run surfaced the missing separator in the MFA prompt-unavailable message.

- `cmd/auth/user/configure.go` built its store with `credentials.NewCredentialStore()`. That
  no-argument form passes a nil config, so it always resolved the default `system` keyring. Every other
  command, and the auth manager, use `credentials.NewCredentialStoreWithConfig(&atmosConfig.Auth)`.
- `configure` always showed the identity selector and never read the inherited `--identity` flag or
  `ATMOS_IDENTITY`.
- `handleSTSError` in `pkg/auth/identities/aws/user.go` joined the sentinel with `errors.Join`, which
  separates messages with a newline. `buildGetSessionTokenInput` returns the prompt-unavailable error
  through that path, so the message was `authentication failed\ninteractive authentication prompt
  unavailable: requires a TTY`. `WrapAuthenticationFailed` only dropped sentinel segments split on `": "`, so
  the joined sentinel survived inside the detail. The newline flattened to a space in the terminal,
  which produced the live output.

## Changes

- **Keyring.** `configure` now builds its credential store with `NewCredentialStoreWithConfig` from the
  loaded `atmosConfig.Auth`. The store is passed into `promptAndSaveCredentials`, so the type, the file
  path, `password_env`, and `ATMOS_KEYRING_TYPE` all match what `login` uses.
- **`--identity`.** `configure` honors `--identity=<name>` and `ATMOS_IDENTITY`, and skips the selector.
  It validates that the identity exists and has kind `aws/user`; otherwise it returns
  `ErrIdentityNotFound` or `ErrInvalidIdentityKind` with a hint that lists the available `aws/user`
  identities. Without an identity, the selector runs when `interactive.Available()` is true, and
  otherwise the command fails fast with `ErrIdentitySelectionRequiresTTY` and a hint to pass
  `--identity=<name>`. The credential form has the same gate and fails with `ErrAuthPromptUnavailable`.
  `selectAWSUserIdentities` now sorts its result so the selector and hints are deterministic.
- **Hint form.** All `atmos auth user configure --identity %s` hints in `pkg/auth/identities/aws/user.go`
  use `--identity=%s`, as do the matching mentions in the keyring docs and agent skills.
- **Separator.** `dropSentinelSegments` also splits on the newline `errors.Join` produces, so a joined
  sentinel is dropped. `handleSTSError` uses `EnsureAuthenticationFailed` instead of `errors.Join`, so the
  raw error is `authentication failed: <cause>` and keeps matching the sentinels. The chained message now reads
  `authentication failed for identity "ft-role" via identity "ft-user": interactive authentication prompt unavailable: requires a TTY`.
- **Docs.** `website/docs/cli/commands/auth/user/configure.mdx` documents `--identity` and the keyring
  behavior.

## Validation

- `go build ./...`
- `go test ./errors/... ./pkg/auth/... ./cmd/auth/...` passed.
- `go test ./tests -run 'TestCLICommands/atmos_auth'` passed with no snapshot change.
- `./custom-gcl run --new-from-rev=origin/main ./errors/... ./pkg/auth/... ./cmd/auth/...` reported 0 issues.
- `gofumpt` on the touched files; `cd website && npm run build` succeeded.
- New tests: `TestWrapAuthenticationFailed_SeparatorsAndSentinels` and
  `TestUser_generateSessionToken_PromptUnavailableMessage` failed before the fix with the newline
  message and pass now. `TestExecuteAuthUserConfigure_UsesConfiguredKeyring` failed before the fix
  (no keyring directory was written) when `configure` was temporarily switched back to the no-argument
  store, and passes now. That temporary before-fix run wrote a fake entry
  (`atmos_ft-configure-test_ft-configure-user`) to the macOS keychain; it was deleted afterwards, and re-running
  `go test ./cmd/auth/user/ -run Configure` with the fix in place left no keychain entries. `TestResolveIdentityToConfigure` and
  `TestPromptAndSaveCredentials_FailsFastWithoutTerminal` cover the identity and fail-fast paths.
- Live check with a fake-credential fixture (`auth.keyring.type: file`): driving
  `atmos auth user configure --identity=ft-user-b` through a pty wrote
  `atmos_ft-configure_ft-user-b` into the fixture keyring directory, and `security find-generic-password`
  found nothing in the macOS keychain. Without a terminal, no identity, an unknown identity, and an
  `aws/assume-role` identity each fail with the expected message and hints.
- Final combined run on the branch with all of the 2026-10-02 auth fixes applied: `go build ./...`; `go test` for
  `./errors/... ./pkg/auth/... ./cmd/auth/... ./cmd/aws/... ./cmd/azure/... ./cmd/gcp/... ./pkg/devcontainer/...
  ./pkg/store/... ./pkg/process/... ./pkg/io/... ./pkg/data/...`; `go test -short ./cmd/... ./internal/exec/...`;
  `go test ./tests -run 'TestCLICommands/atmos_(auth|aws)'`; `GOOS=windows go vet` on the auth packages; and
  `./custom-gcl run --new-from-rev=origin/main` (0 issues). All passed.

## Follow-ups

None.
