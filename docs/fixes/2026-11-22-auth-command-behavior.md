# Fix: `atmos auth` command output, exit codes, signals and identity selection

**Date:** 2026-11-22

## Summary

This change fixes how the `atmos auth` subcommands print output, choose an identity, exit, and handle
signals. Most items come from the Atmos Auth pre-release review (#3720). Two (A4 and A5) are regressions
that a regression test found by comparing the 1.235.0 candidate with v1.234.1, and A8 is a correction of
the first `auth list` redaction. Four are bugs that already existed in v1.234.1 and were fixed in the same
pass. Every fix has a test that fails without it.

In short:

- `auth list --format json|yaml` no longer prints secrets, and shows everything else unchanged.
- `auth env --format json` prints the raw credentials it exists to print.
- The UI channel keeps masking registered secrets after the UI formatter is rebuilt.
- `auth console` never prints credentials when federation fails.
- `-h`/`--help` after `--` reaches the child, and `auth exec` rejects arguments placed before `--`.
- `auth logout` prompts on a bare `--identity`, and fails instead of guessing when there is no TTY.
- `--tags` beats `ATMOS_IDENTITY`, `--identity=false` is honored, and identity names are
  case-insensitive in `exec`, `env`, `shell` and `logout`.
- `auth whoami` exits 1 for expired or rejected credentials, and validates against the configured STS
  endpoint.
- `auth exec` and `auth shell` exit 128+N for a signal-killed child, and forward termination signals to it.

Other #3720 and regression-test fixes land in separate PRs.

## Context

The #3720 review findings were re-checked on `main` at `2d7822f214`, and each one still reproduced. The
regression test then compared v1.234.1 (`b148adc244`) with the branch that carried the first round of
#3720 fixes. It ran both binaries on identical isolated fixtures, with local fakes for STS and no live
cloud. Two of the first-round auth command fixes had changed behavior that v1.234.1 users relied on:

- The new non-TTY logout path auto-picked an identity (A4).
- `auth whoami` started exiting 1 for valid credentials when STS was offline or behind a proxy (A5).

Both are fixed here, together with the first-round fixes they belong to. The fixes fall into four themes:
secrets in output, argument and identity selection, `whoami` exit status, and child process exit codes and
signals.

## Changes

### Secrets in output

- **`auth list --format json|yaml` redacts only secret values** (#3720 item, corrected by A8;
  `cmd/auth/list_redact.go`, `cmd/auth/list.go`).
  - Problem: the JSON and YAML formats marshaled `schema.Provider` and `schema.Identity` as they were,
    so a provider `password` and inline `aws/user` access keys (already resolved from `!env`) were
    printed in plain text. The first fix replaced every inline credential value, which also hid
    non-secret values such as `region` and turned booleans such as `webflow_enabled` into strings (A8).
  - Fix: `structuredListOutput` builds deep copies with secret values replaced by `[redacted]`. A value
    is secret when it is a provider `password`, when its key in identity `credentials`, `spec` or `env`
    or provider `spec` contains a denylist fragment (`secret`, `password`, `passwd`, `passphrase`,
    `token`, `access_key`, `private_key`, `api_key`, `apikey`, `client_assertion`; case-insensitive, `-`
    read as `_`), or when it looks like an AWS access key ID (`AKIA`/`ASIA`) under any key. Maps and lists
    are redacted recursively, and every string under a secret key is redacted. Booleans and numbers keep
    their value and type. The input maps are never modified.
  - The output now goes through `data.Write`, so it also passes through the global masker.
  - Tests: `TestRenderStructuredAuthListRedactsSecrets`,
    `TestExecuteAuthListCommand_StructuredOutputUsesDataChannel`,
    `TestRedactIdentitiesForList_KeepsNonSecretValuesAndTypes`,
    `TestRedactIdentitiesForList_AccessKeyIDUnderAnyKey`,
    `TestRedactProvidersForList_RedactsSpecSecrets`, `TestRenderStructuredAuthList_KeepsBooleanTypes`,
    `TestIsSecretListKey`, `TestRedactSecretValue_NestedUnderSecretKey`.
- **`auth env --format json` is unmasked** (#3720; `pkg/env/output.go`).
  - Problem: with an Atmos config and no mask option, JSON went through `u.PrintAsJSON`, which writes
    through the masked data channel. The credentials came out as `<MASKED>`, which broke
    `atmos auth env --format json | ConvertFrom-Json` and similar uses.
  - Fix: without the mask option, JSON is rendered by `unmaskedJSON` (highlighted when a config is given,
    plain indented JSON otherwise) and written with `pkgdata.WriteUnmasked`, the same as the bash,
    dotenv, env and github formats. With the mask option it is still masked.
  - Tests: the two `json format to stdout ... stays raw without mask option` cases in `TestOutput`
    (`pkg/env/output_test.go`), which also check that the output still decodes as JSON.
- **The UI channel keeps the global masker after `ui.ReinitFormatter`** (#3720 suspected item;
  `pkg/ui/formatter.go`).
  - The review suspected that `io.RegisterSecret` does not mask the data and UI channels. That is not
    the case at startup: both channels are bound to the global masker. A guard test now fails if that
    binding is removed (`TestInitializeIOMasksSecretsRegisteredOnTheGlobalMasker`,
    `pkg/startup/io_masking_test.go`).
  - The real gap: `ReinitFormatter`, which runs after a cast recording or `--ai` capture, built a fresh
    I/O context with its own masker. Secrets registered during the command were then printed unmasked on
    stderr. It now reuses the global context's masker and streams.
  - Test: `TestReinitFormatterKeepsGlobalMasker`.
- **`auth console` never prints credentials when federation fails** (pre-existing in v1.234.1;
  `pkg/auth/cloud/aws/console.go`).
  - Problem: the federation request carries the session document, with the access key, secret key and
    session token, in its `Session` query parameter. Transport and status errors repeat that URL, so a
    failed `getSigninToken` call printed the credentials on stderr.
  - Fix: `redactFederationError` rebuilds the error with the query replaced by
    `Action=getSigninToken&Session=[redacted]`, and every raw, query-escaped and path-escaped credential
    value replaced by `[redacted]`. `context.Canceled` and `context.DeadlineExceeded` stay in the error
    chain. `GetConsoleURL` also registers the three credential values with the masker.
  - Tests: `TestGetConsoleURL_FederationErrorDoesNotLeakCredentials`,
    `TestGetConsoleURL_FederationErrorKeepsCause`, `TestGetConsoleURL_RegistersCredentialsWithMasker`.

### Arguments and identity selection

- **`-h`/`--help` after `--` reaches the child command** (#3720; `cmd/auth/helpers.go`).
  - Problem: Cobra strips `--` from `args`, so `handleHelpRequest` never saw the separator, and
    `atmos auth exec -- df -h` printed Atmos help instead of running `df -h`.
  - Fix: the new `helpRequested` truncates `args` at `cmd.ArgsLenAtDash()` before looking for help flags.
    The literal `--` check is kept for callers whose args were not parsed by Cobra.
  - Tests: `TestHelpRequestedIgnoresArgsAfterSeparator`, `TestHelpRequestedDetectsHelpBeforeSeparator`.
- **`auth exec` rejects arguments before `--`** (#3720; `cmd/auth/exec.go`).
  - Problem: `getSeparatedArgsForExec` returns only the args after `--`, so `atmos auth exec aws -- s3 ls`
    silently ran `s3 ls`.
  - Fix: `validateAuthExecArgs` returns `ErrInvalidArguments` with the correct form when any positional
    argument appears without `--` or before it, as `validateAuthShellArgs` already does for `auth shell`.
  - Tests: `TestValidateAuthExecArgs`, `TestExecuteAuthExecCommand_RejectsArgsBeforeSeparator`.
- **`ATMOS_IDENTITY` is ambient, and `--tags` takes precedence over it** (#3720;
  `cmd/auth/identity_request.go`, `cmd/auth/login.go`, `cmd/auth/logout.go`).
  - Problem: `auth shell` exports `ATMOS_IDENTITY`, and `login` and `logout` read it as if `--identity`
    had been passed. Inside an auth shell, `auth login --tags ...` ignored the tags, and
    `auth logout --tags ...` failed with `ErrMutuallyExclusiveFlags`.
  - Fix: `resolveIdentityRequest` keeps the explicit flag and the environment value apart. `login`
    resolves in the order: explicit `--identity`, `--tags`, then `ATMOS_IDENTITY`
    (`requestedLoginIdentity`). For `logout`, only explicit selectors conflict with `--tags`, and
    `ATMOS_IDENTITY` is used only when nothing else selects a target. A `--tags` value that matches
    nothing still fails instead of falling back to `ATMOS_IDENTITY`.
  - Tests: `TestResolveIdentityRequest`, `TestAuthenticateIdentity_TagsWinOverAmbientIdentityEnv`,
    `TestAuthenticateIdentity_TagsWithoutMatchFailInsteadOfUsingAmbientIdentity`,
    `TestAuthenticateIdentity_ExplicitIdentityWinsOverTags`, `TestResolveLogoutTarget_AmbientIdentityEnv`.
- **`--identity=false` / `ATMOS_IDENTITY=false` is honored** (#3720; `cmd/auth/identity_request.go`
  and the `login`, `logout`, `whoami`, `env`, `shell`, `console` and `exec` command files).
  - Problem: the value was looked up as an identity named `false` and failed with a misleading
    "identity not found" error, although the docs say it disables authentication.
  - Fix: `auth exec` runs the command unauthenticated, with the OS environment plus the global `env`
    from `atmos.yaml`, and revokes nothing afterwards. The subcommands that act on an identity (`login`,
    `logout`, `whoami`, `env`, `shell`, `console`) return the new `ErrAuthDisabledIdentityRequired` with
    hints. Any value that `cfg.NormalizeIdentityValue` reads as disabled (`false`, `0`, `no`, `off`, any
    case) counts.
  - Tests: `TestIsIdentityDisabled`, `TestIdentityFalseIsRejectedByIdentityCommands`,
    `TestCheckIdentityNotDisabled_AllowsNamedIdentities`, `TestResolveLogoutTarget_IdentityFalse`,
    `TestExecuteAuthExecCommand_IdentityFalseRunsWithoutAuth`.
- **A bare `auth logout --identity` prompts** (#3720; `cmd/auth/logout_target.go`,
  `cmd/auth/logout_interactive.go`).
  - Problem: a bare `--identity` sets the `__SELECT__` sentinel, which `logout` passed on as an identity
    name, so it reported that identity `__SELECT__` was not found.
  - Fix: target selection moved into `resolveLogoutTarget`, which keeps the order `--all`, `--tags`,
    `--provider`, positional identity, `--identity`, `ATMOS_IDENTITY`, then the interactive menu. A bare
    `--identity` now opens the identity selector (`performSelectIdentityLogout`) and logs out the chosen
    identity. The interactive menu code moved to `logout_interactive.go` unchanged apart from the fixes
    below.
  - Tests: `TestResolveLogoutTarget`, `TestResolveLogoutTarget_TagsAreExclusive`,
    `TestPerformSelectIdentityLogout_SelectedIdentityIsLoggedOut`.
- **Logout never auto-picks an identity without a TTY** (regression A4, found by comparing with
  v1.234.1; `cmd/auth/logout_interactive.go`, `errors/errors_execution.go`).
  - Problem: with the bare `--identity` fix above, a bare `--identity` or a plain `auth logout` in CI read
    EOF from the prompt and logged out the first entry of a randomly ordered list, then exited 0.
    v1.234.1 exited 1.
  - Fix: `performInteractiveLogout` returns the new `ErrLogoutSelectionRequiresTTY` (which wraps
    `ErrTTYRequired`) with hints naming the explicit selectors and the available identities when
    `isInteractiveFn()` is false. A bare `--identity` gets the same TTY-required error from the identity
    selector that other bare `--identity` commands return. The menu, the "not found" lists and the
    `--all --dry-run` file list are sorted by name.
  - Tests: `TestPerformInteractiveLogout_WithoutTTYFailsInsteadOfPicking`,
    `TestPerformInteractiveLogout_NoIdentitiesSkipsTTYCheck`, `TestPerformSelectIdentityLogout_WithoutTTYFails`,
    `TestExecuteAuthLogoutCommand_BareIdentityWithoutTTYFails`,
    `TestExecuteAuthLogoutCommand_PlainLogoutWithoutTTYFails`,
    `TestBuildLogoutOptions_SortedOrder`.
- **Identity names are case-insensitive in `exec`, `env`, `shell` and `logout`** (pre-existing in
  v1.234.1; `pkg/auth/manager.go`, `pkg/auth/manager_environment.go`, `pkg/auth/manager_integrations.go`,
  `cmd/auth/logout.go`).
  - Problem: `Authenticate` resolved names case-insensitively, but `PrepareShellEnvironment`,
    `GetEnvironmentVariables`, `GetProviderForIdentity`, `RevokeEphemeralIntegrations` and
    `performIdentityLogout` used the name as typed. Identity keys are stored lower-cased, so
    `--identity Prod-Admin` worked for `login` and failed elsewhere.
  - Fix: the manager methods resolve the name through the new `canonicalIdentityName` (built on
    `resolveIdentityName`, which also gains a nil-config guard). Logout uses `canonicalIdentityKey`. An
    unknown name is passed through unchanged, so it is still reported as not found. Workflows, hooks and
    custom-command `identity:` go through `PrepareShellEnvironment`, so they gain the same behavior.
  - Tests: `TestPrepareShellEnvironment_CaseInsensitiveIdentity`,
    `TestGetEnvironmentVariables_CaseInsensitiveIdentity`,
    `TestGetProviderForIdentity_CaseInsensitiveIdentity`, `TestCaseInsensitiveIdentity_UnknownNameStillFails`,
    `TestResolveIdentityName_NilConfig`, `TestExecuteAuthExecCommand_CaseInsensitiveIdentity`,
    `TestPerformIdentityLogout_CaseInsensitiveIdentity`, `TestCanonicalIdentityKey`.

### `auth whoami` exit status

- **`auth whoami` exits non-zero for expired or invalid credentials** (#3720 suspected item, confirmed;
  `cmd/auth/whoami.go`).
  - Problem: `whoami` printed "expired" or "invalid" and still exited 0, so scripts could not use it to
    decide whether to run `atmos auth login`.
  - Fix: the new `reportWhoami` prints the status and then returns `ErrExpiredCredentials` joined with a
    silent `ExitCodeError{Code: 1}`, so the status is not repeated in an error box. With `--output json`
    the JSON is still written to stdout and a warning goes to stderr.
  - Test: `TestReportWhoami_ExitStatusReflectsCredentialValidity` (both formats, expired and valid).
- **`auth whoami` validates against the configured STS endpoint** (regression A5, found by comparing with
  v1.234.1; `pkg/auth/types/aws_credentials.go`, `cmd/auth/whoami.go`, `errors/errors_execution.go`).
  - Problem: once `whoami` exited 1 for invalid credentials, any failed STS call counted as invalid. The
    call always went to the public regional STS endpoint, so offline, proxy and emulator users
    (LocalStack, a private endpoint) got exit 1 for valid credentials.
  - Fix:
    - `configureWhoamiValidation` sets the validation endpoint from `spec.endpoint_url` on the identity,
      else on its provider (`awsAuth.GetBaseEndpointConfigOption`), through the new runtime-only
      `AWSCredentials.SetValidationEndpoint`. The field is never serialized.
    - `AWSCredentials.Validate` fails expired credentials with `ErrExpiredCredentials` without a network
      call, bounds the STS call to 15 seconds and 2 attempts, and classifies errors. A known rejection
      code, any client-fault API error, or HTTP 401/403 wraps `ErrAuthenticationFailed`. Anything else
      (network, proxy, timeout, cancellation, local config) wraps the new
      `ErrCredentialValidationUnavailable`.
    - `validateCredentials` treats an unavailable validation like a missing validator: the result comes
      from the cached expiration time, and `reportWhoami` prints a warning on stderr.
  - Expired or rejected credentials still exit 1. Unexpired credentials exit 0 when STS cannot be
    reached.
  - Tests: `TestAWSCredentials_ValidateUsesConfiguredEndpoint`, `TestAWSCredentials_ValidateRejectedCredentials`,
    `TestAWSCredentials_ValidateUnreachableEndpoint`, `TestAWSCredentials_ValidateServerFault`,
    `TestAWSCredentials_ValidateExpiredSkipsNetwork`, `TestAWSCredentials_ValidationEndpointIsNotSerialized`,
    `TestReportWhoami_UnreachableSTSKeepsExitZero`, `TestReportWhoami_UnreachableSTSWithExpiredCredentialsFails`,
    `TestReportWhoami_RejectedCredentialsFail`, `TestReportWhoami_ValidatesAgainstConfiguredEndpoint`,
    `TestConfigureWhoamiValidation`. `TestAWSCredentials_ValidateCanceled` now expects
    `ErrCredentialValidationUnavailable`, and `TestValidateCredentials` reads the new result struct.

### Child process exit codes and signals

- **`auth exec` exits 128+N for a signal-killed child** (#3720 suspected item, confirmed;
  `pkg/shell/exec.go`).
  - Problem: `WaitStatus.ExitStatus()` is -1 for a child terminated by a signal, so Atmos exited 255.
  - Fix: `childExitCode` maps a signaled child to 128+N (for example 137 for `SIGKILL`), as a POSIX shell
    does. Windows has no POSIX signals, so the child's own exit code is used there.
  - Test: `TestRunCommand_SignaledChildExitsWith128PlusSignal`.
- **`auth shell` exits 128+N for a signal-killed shell** (pre-existing in v1.234.1;
  `pkg/shell/interactive.go`). `processExitCode` applies the same mapping to the shell's process state.
  Test: `TestStartInteractive_SignaledShellExitsWith128PlusSignal`.
- **Termination signals reach the `auth exec` and `auth shell` child** (pre-existing in v1.234.1;
  `pkg/shell/supervise.go`, `pkg/shell/supervise_unix.go`, `pkg/shell/supervise_windows.go`).
  - Problem: on `SIGTERM` (a `kill`, a CI cancellation, a container stop) Atmos exited and left the child
    running. Ctrl-C could also end Atmos before the child had finished handling it.
  - Fix: `RunCommand` and `StartInteractive` now start the child and supervise it until it exits
    (`superviseChild`).
    - SIGINT: Atmos does not exit while the child runs. When Atmos is in the foreground process group of
      its terminal, the terminal already delivered Ctrl-C to the child, so it is not sent again;
      otherwise (for example `kill -INT <atmos pid>`) it is forwarded.
    - SIGTERM: an exit cleanup forwards `SIGTERM` to the child and waits up to 30 seconds for it to exit.
      For an interactive shell, which ignores `SIGTERM`, `SIGHUP` follows 2 seconds later.
    - Windows: the console delivers Ctrl-C to every attached process and there is no `SIGTERM`, so only
      the SIGINT suspension applies.
  - `secret exec` and `secret shell` use the same `pkg/shell` functions, so they get this behavior too.
  - Tests: `TestRunCommand_TerminationIsForwardedToChild`,
    `TestRunCommand_InterruptForwardedWhenTerminalDoesNotDeliverIt`,
    `TestRunCommand_InterruptNotForwardedWhenTerminalDeliversIt`,
    `TestStartInteractive_TerminationEscalatesToHangup`,
    `TestStartInteractive_TerminationStopsAtFirstSignal`, `TestRunCommand_TerminationGivesUpAfterGrace`,
    `TestStartInteractive_TerminationGraceEndsEscalation`, `TestTerminalDeliversInterrupt`,
    `TestForwardSignal_FinishedProcess`. The test binary acts as the child through env-gated helpers in
    `pkg/shell/testmain_signal_unix_test.go` (with a Windows stub), so no platform binaries are used.

### New sentinels and docs

- New sentinels in `errors/errors_execution.go`: `ErrAuthDisabledIdentityRequired`,
  `ErrLogoutSelectionRequiresTTY` and `ErrCredentialValidationUnavailable`.
- Website pages updated: `auth exec` (args after `--`, exit status and signals), `auth list` (what is
  redacted), `auth login` (`--tags` precedence), `auth shell` (exit status and signals), `auth whoami`
  (exit status and endpoint), `auth logout` (sorted menu, TTY requirement, bare `--identity`, `--tags`
  with `ATMOS_IDENTITY`), and the auth usage page (`--identity=false`, case-insensitive names).

## Validation

All commands were run in this PR's worktree, on top of `main` at `2d7822f214`, with Go 1.27.4 on
darwin/arm64.

- `go build ./...`: passed.
- `go vet` on the changed packages (`cmd/auth`, `errors`, `pkg/auth`, `pkg/auth/cloud/aws`,
  `pkg/auth/types`, `pkg/env`, `pkg/shell`, `pkg/startup`, `pkg/ui`): passed with the host OS,
  `GOOS=windows` and `GOOS=linux`.
- `go test -count=1 -cover` on the same packages: all 9 packages passed.
- This PR does not touch the root `./cmd` package or `./internal/exec`, so nothing was skipped for them.
  Their full suites run in CI.

Package statement coverage, `main` (`2d7822f214`) to this PR, from `go test -count=1 -cover` in a
temporary worktree of `origin/main` and in this worktree:

| Package | Before | After |
| --- | --- | --- |
| `cmd/auth` | 82.2% | 84.0% |
| `errors` | 87.3% | 87.3% |
| `pkg/auth` | 91.7% | 91.7% |
| `pkg/auth/cloud/aws` | 93.0% | 93.0% |
| `pkg/auth/types` | 28.1% | 30.2% |
| `pkg/env` | 98.2% | 97.7% |
| `pkg/shell` | 88.3% | 91.0% |
| `pkg/ui` | 91.4% | 91.4% |

`errors` only gained variable declarations. In `pkg/auth`, `pkg/auth/cloud/aws` and `pkg/ui` the new
lines are covered, but they are few against the package size, so the rounded figure does not move.
`pkg/env` dropped because the new `unmaskedJSON` helper has two error branches that cannot be reached
with a `map[string]string` (a `GetHighlightedJSON` or `json.MarshalIndent` failure), plus the caller's
error return. `pkg/auth/types` is low because most of it is plain data types.

`pkg/startup` is left out of the table because only a test file changed there.

Not run:

- `golangci-lint` and `gofumpt`. Git hooks are disabled in this clone, so CI lint is the first lint pass.
- Tests on Linux and Windows. Only cross-OS `go vet` ran.
- `go test -race`.
- The `TestCLICommands` snapshot tests in `./tests`.
- The website build (`cd website && npm run build`), although website pages changed.
- The end-to-end A4 and A5 rechecks against the v1.234.1 binary were part of the regression test, not
  repeated for this PR.

## Follow-ups

Choices made in these fixes that can be revisited:

- A child that ignores termination is left running after the 30-second grace period rather than killed.
- The `auth list` secret denylist matches key fragments, so a non-secret key that contains one of them
  (for example a name with `token` in it) is also redacted. A secret stored under a key with none of the
  fragments is only redacted when it looks like an AWS access key ID; the global masker on the data
  channel is the second line of defense.

Compatibility notes for users (intentional changes since v1.234.1 that need release notes):

- `atmos auth whoami` exits 1 for expired or rejected credentials, in both output formats. It exits 0
  with a warning when STS cannot be reached and the credentials have not expired.
- `--identity=false` disables auth for `auth exec`; the other auth subcommands reject it.
- `auth exec` rejects arguments placed before `--`.
- `--tags` takes precedence over `ATMOS_IDENTITY` in `auth login` and `auth logout`.
- A plain `auth logout` or a bare `auth logout --identity` without a TTY fails instead of prompting.
- Ctrl-C during `auth exec` or `auth shell` waits for the child and returns its exit code, and a
  signal-killed child exits 128+N instead of 255.
