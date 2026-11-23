# Fix: ECR and ACR login store Docker credentials the way `docker login` does

**Date:** 2026-11-22

## Summary

`atmos aws ecr login`, `atmos azure acr login` and the `aws/ecr`, `aws/ecr-public` and `azure/acr`
integrations write registry tokens to Docker's `config.json`. That code modeled only
`auths.<host>.auth`, so every login or logout rewrote the file and dropped everything else in it. It also
ignored the credential helpers Docker actually reads tokens from.

This PR makes `pkg/auth/cloud/docker` behave like docker/cli:

- `config.json` is round-tripped losslessly and written atomically.
- Tokens go to the credential helper Docker uses for the registry (`credHelpers`, then `credsStore`),
  and inline `auths` are used only when no helper applies.
- A helper that gets credentials itself, such as `ecr-login`, is left alone and the login succeeds.
- A credential stored in a helper is erased again if `config.json` cannot be saved.
- A warning is printed when `DOCKER_AUTH_CONFIG` shadows the login.
- Registry keys are normalized and matched the way docker/cli does.

The items come from the Atmos Auth pre-release review (#3720) and from the regression test against
v1.234.1. Other #3720 and regression-test fixes land in separate PRs.

## Context

- **#3720, finding 2.2: user files clobbered.** `WriteAuth`/`RemoveAuth` unmarshaled `config.json` into a
  struct with only `auths.<host>.auth` and marshaled it back. Each ECR/ACR login or logout dropped
  `credsStore`, `credHelpers`, `currentContext`, `plugins`, `proxies`, per-registry `identitytoken` and
  `email`, and any other key.
- **Keeping those keys is not enough.** When `credHelpers[host]` or `credsStore` names a helper, Docker
  reads that registry's credentials from the helper and ignores inline `auths`. Once the helper keys are
  preserved, an inline token for such a registry would never be used, so it must go to the helper.
- **Regression found by comparing with v1.234.1 (A6 in the regression test).** The first version of the
  helper routing stored tokens through whatever helper `config.json` named. `docker-credential-ecr-login`
  gets ECR credentials from AWS itself and replies "not implemented" to `store`, so
  `atmos aws ecr login` exited 1 for a registry routed to it. v1.234.1 exited 0.
- **Follow-up review rounds on #3720** found three more gaps in the new code: a credential orphaned in
  the helper when the `config.json` save fails, no hint when `DOCKER_AUTH_CONFIG` overrides
  `config.json`, and exact-match registry keys where docker/cli normalizes them.

## Changes

### Lossless `config.json` round trip (#3720)

- **Problem.** Logins and logouts rewrote `~/.docker/config.json` without the keys Atmos did not model.
- **Cause.** The typed `dockerConfig` struct held only `auths.<host>.auth`.
- **Fix.** A new raw view (`pkg/auth/cloud/docker/rawconfig.go`) keeps every top-level key and every
  `auths` entry as raw JSON. Only `auths`, `credsStore` and `credHelpers` are decoded. Only the target
  registry's `auths` entry changes.
  - Values are written back verbatim, with no HTML escaping.
  - An `auths` key is not added to a file that never had one.
  - Empty, whitespace-only or `null` content is treated as an empty config, as docker/cli does. JSON
    `null` values for `auths`, `credsStore` and `credHelpers` count as unset.
  - `saveConfig` writes atomically (temp file and rename) while holding the existing file lock. It
    writes through a symlinked `config.json`, keeps an existing file's permission bits, and creates a
    new file with `0600`.
  - `GetAuthenticatedRegistries` returns the registries sorted.
- **Main files.** `pkg/auth/cloud/docker/rawconfig.go`, `pkg/auth/cloud/docker/config.go`. The old typed
  view moved to `typed_view_test.go`, where tests use it to assert on output.
- **Tests.** `TestConfigManager_WriteRemove_PreservesUnknownFields`,
  `TestConfigManager_WriteRemove_NoAuthsKey`,
  `TestConfigManager_SaveConfig_OmitsAbsentEmptyAuths`, `TestConfigManager_RemoveAuth_NoFileDoesNotCreate`,
  `TestConfigManager_LoadConfig_WhitespaceFile`, `TestConfigManager_WriteAuth_PreservesFileMode`,
  `TestConfigManager_WriteAuth_FollowsSymlink`, `TestParseRawConfig`,
  `TestConfigManager_SaveConfig_MarshalFailure`, `TestConfigManager_SaveConfig_RenameTargetIsDirectory`.

### Route tokens through Docker credential helpers (#3720)

- **Problem.** With `credHelpers` and `credsStore` preserved, an inline token for a registry routed to a
  helper is never read by Docker, so the login would be silently ineffective.
- **Cause.** `WriteAuth` knew nothing about `credHelpers` or `credsStore`.
- **Fix.** `credentialHelperFor` mirrors docker/cli's `getConfiguredCredentialStore`. A
  `credHelpers[host]` entry wins, even when empty (an empty entry means inline `auths`), then
  `credsStore`.
  - When a helper applies, `WriteAuth` runs `docker-credential-<name> store` with the protocol's JSON
    payload, as `docker login` does. Then it writes only the empty `{}` placeholder to `auths`.
  - A helper failure returns `ErrDockerCredentialHelper` with a hint and leaves `config.json` unchanged.
  - `RemoveAuth` reverses `WriteAuth`. It runs `erase` through the helper, and only for registries that
    have an `auths` entry. If an erase fails, it keeps that registry's entry so the logout can be
    retried, and returns the joined errors. Erasing a credential the helper does not have
    ("credentials not found in native keychain") is not an error.
  - The runner (`execCredentialHelper`) bounds each call with a 60-second timeout, generous enough for
    an OS keychain prompt. Helper stdout and stderr go into the error, not the terminal. A helper name
    that contains a path separator is refused before exec, so a crafted `config.json` cannot run an
    arbitrary relative program.
  - The runner sits behind a `CredentialHelper` interface with a mockgen-generated mock.
- **Main files.** `pkg/auth/cloud/docker/credential_helper.go`, `pkg/auth/cloud/docker/config.go`,
  `pkg/auth/cloud/docker/mock_credential_helper_test.go`, `errors/errors_integrations.go`
  (`ErrDockerCredentialHelper`).
- **Tests.** `TestConfigManager_CredsStore_RoutesThroughHelper`,
  `TestConfigManager_CredHelperEmpty_UsesInlineAuth`,
  `TestConfigManager_WriteAuth_PerRegistryHelperWins`,
  `TestConfigManager_WriteAuth_HelperForOtherRegistryIgnored`,
  `TestConfigManager_WriteAuth_HelperFailureLeavesConfigUnchanged`,
  `TestConfigManager_RemoveAuth_HelperErase`,
  `TestConfigManager_RemoveAuth_HelperEraseFailureKeepsEntry`,
  `TestConfigManager_RemoveAuth_SkipsHelperWithoutEntry`,
  `TestConfigManager_RemoveAuth_SaveFailureIsReturned`, `TestConfigManager_CredentialHelper_DefaultsToExec`,
  `TestWithHelperHint`, `TestExecCredentialHelper` (store and erase payloads, not-found erase, missing
  helper, path separator, timeout), `TestConfigManager_FakeHelperFailure_EndToEnd`. The exec runner is
  tested against a fake helper built from the test binary (`testmain_test.go`). No real
  `docker-credential-*` program runs.
  `TestExecuteExplicitRegistries_UnusableCredentialHelperFailsLoudly` (`cmd/azure/acr`) checks that an
  explicit ACR login fails and leaves `config.json` untouched when the helper is missing.

### Leave helper-managed registries to the helper (regression found by comparing with v1.234.1)

- **Problem.** `atmos aws ecr login` exited 1 when `credHelpers` routed the registry to `ecr-login`.
  `atmos azure acr login --registry` failed the same way for a helper that cannot store.
- **Cause.** Storing through such a helper can never succeed: it replies "not implemented" to `store`
  and `erase`, because it gets registry credentials itself.
- **Fix.** The runner classifies a "not implemented" reply, on stdout or stderr, as
  `ErrDockerCredentialHelperStoreUnsupported`. The match ignores case and allows a `prefix: ` before the
  reply. It does not match the words in the middle of a sentence.
  - `WriteAuth` then leaves `config.json` unchanged and returns a `helperManagedError`.
  - `HelperManagedNotice(err)` recognizes that error and returns the message to show.
  - The `aws/ecr`, `aws/ecr-public` and `azure/acr` integrations and the explicit-registry paths of
    `atmos aws ecr login` (private and public) and `atmos azure acr login` print an info line that Docker
    uses that helper, and succeed.
  - A helper whose store genuinely fails still fails the login. Erasing from a helper that cannot store
    is a no-op, so logout removes the `auths` entry.
  - This applies to any helper Docker resolves for the registry, through `credHelpers` or `credsStore`.
- **Main files.** `pkg/auth/cloud/docker/helper_managed.go`, `pkg/auth/cloud/docker/credential_helper.go`,
  `pkg/auth/integrations/aws/ecr.go`, `pkg/auth/integrations/aws/ecr_public.go`,
  `pkg/auth/integrations/azure/acr.go`, `cmd/aws/ecr/login.go`, `cmd/azure/acr/login.go`,
  `errors/errors_integrations.go` (`ErrDockerCredentialHelperStoreUnsupported`).
- **Tests.** `TestIsNotImplementedReply`, `TestExecCredentialHelper_StoreNotImplemented`,
  `TestExecCredentialHelper_StoreFailureIsNotUnsupported`, `TestExecCredentialHelper_EraseNotImplemented`,
  `TestConfigManager_WriteAuth_HelperManagedRegistry`, `TestHelperManagedNotice_OtherErrors`,
  `TestConfigManager_RemoveAuth_NotImplementedEraseRemovesEntry` (`pkg/auth/cloud/docker`);
  `TestECRIntegrations_EcrLoginHelperManagesRegistry`, `TestECRIntegration_FailingHelperStillFails`
  (`pkg/auth/integrations/aws`); `TestACRIntegration_HelperManagesRegistry`,
  `TestACRIntegration_FailingHelperStillFails` (`pkg/auth/integrations/azure`);
  `TestExecuteExplicitRegistries_EcrLoginHelperManagesRegistry`,
  `TestExecuteExplicitRegistries_FailingHelperStillFails` (`cmd/aws/ecr`, private and ECR Public);
  `TestExecuteExplicitRegistries_HelperManagedRegistrySucceeds`,
  `TestExecuteExplicitRegistries_HelperFailureStillFails` (`cmd/azure/acr`). Each package's `TestMain`
  re-executes the test binary as a fake `docker-credential-*` helper.

### Erase the helper credential when the `config.json` save fails (#3720 follow-up)

- **Problem.** If a helper stored the token but saving `config.json` then failed, no `auths`
  placeholder was written. A later `RemoveAuth` never erased the credential, so it was orphaned in the
  helper (for example the OS keychain).
- **Fix.** `rollbackHelperStore` erases the just-stored credential on a best-effort basis. It returns the
  save error, joined with the erase error when the rollback also fails.
- **Main file.** `pkg/auth/cloud/docker/config.go`.
- **Tests.** `TestConfigManager_WriteAuth_SaveFailureErasesHelperCredential`,
  `TestConfigManager_WriteAuth_SaveFailureRollbackFailure`.

### Warn when `DOCKER_AUTH_CONFIG` shadows the login (#3720 follow-up)

- **Problem.** docker/cli reads credentials from `DOCKER_AUTH_CONFIG` before `config.json`, so a login
  Atmos writes is ignored for any registry that variable lists.
- **Fix.** After a successful `WriteAuth`, a warning goes to stderr through `pkg/ui` when the variable
  is set and non-empty. It names the registry for certain when the variable's `auths` lists it (an exact
  key match, as docker/cli's in-memory store does). Otherwise it gives a general warning, also used when
  the value cannot be parsed. The login still succeeds. No warning is printed when the write fails.
- **Main file.** `pkg/auth/cloud/docker/env_auth_config.go`.
- **Tests.** `TestConfigManager_WriteAuth_WarnsWhenDockerAuthConfigSet`,
  `TestConfigManager_WriteAuth_NoWarningWithoutDockerAuthConfig`,
  `TestConfigManager_WriteAuth_NoWarningOnFailure`,
  `TestConfigManager_Warn_DefaultsToUI`.

### Match registry keys the way docker/cli does (#3720 follow-up)

- **Problem.** Registry keys were matched exactly. A stale legacy entry such as `https://host` or
  `host/v2/` survived `RemoveAuth` and kept the registry logged in for Docker, which falls back to such
  keys.
- **Fix.** `convertToHostname` is a port of docker/cli's `credentials.ConvertToHostname`, including the
  fallback for unbracketed IPv6 literals. It strips the scheme, path and trailing slash and keeps the
  port.
  - `WriteAuth` stores under the normalized host.
  - `RemoveAuth` removes every `auths` key that normalizes to the host. It erases each one from the
    helper Docker resolves for that exact key, as `docker logout` does.
  - `credHelpers` stays an exact match on the host, as in docker/cli.
- **Main files.** `pkg/auth/cloud/docker/hostname.go`, `pkg/auth/cloud/docker/rawconfig.go`
  (`authKeysFor`), `pkg/auth/cloud/docker/config.go`.
- **Tests.** `TestConvertToHostname`, `TestHostFromURLFallback`,
  `TestConfigManager_WriteAuth_NormalizesRegistry`,
  `TestConfigManager_WriteAuth_NormalizedRegistryUsesHostHelper`,
  `TestConfigManager_WriteAuth_SchemeCredHelperKeyIgnored`,
  `TestConfigManager_RemoveAuth_RemovesLegacyKeyVariants`, `TestConfigManager_RemoveAuth_NormalizesRegistry`,
  `TestConfigManager_RemoveAuth_LegacyKeyErasedThroughItsHelper`.

### Coverage-only test

- `TestPromoteAtmosEnvWith_SetFailureSkipsOnlyThatKey` (`pkg/env/promote_set_error_test.go`) checks that
  a variable the environment refuses to set is not reported as promoted, and that the other `ATMOS_*`
  variables are still promoted. It covers existing `pkg/env` code; `pkg/env` itself is not changed.

### Docs

- `website/docs/cli/commands/aws/ecr-login.mdx` gained a "Credential Helpers" section. It covers helper
  routing, the `ecr-login` case with a `credHelpers` example, and that `ecr-login` uses Docker's own AWS
  credentials, not the Atmos identity.
- `website/docs/cli/commands/azure/acr-login.mdx` describes helper routing under "Credential Storage",
  for both the `azure/acr` integration and an explicit `--registry` login.
- `agent-skills/skills/atmos-auth/references/azure-acr-integration.md` describes helper routing,
  helper-managed registries, and the `DOCKER_AUTH_CONFIG` warning.

## Validation

All commands were run in this PR's worktree, one commit on `origin/main` at `2d7822f214`, with Go 1.27.4
on macOS (darwin/arm64).

- `go build ./...`: passed.
- `go vet ./cmd/aws/ecr ./cmd/azure/acr ./errors ./pkg/auth/cloud/docker ./pkg/auth/integrations/aws
  ./pkg/auth/integrations/azure ./pkg/env`: passed.
- `go test -count=1 -cover` on the same seven packages: all passed.
- The PR does not touch the `./cmd` root package or `./internal/exec`.

Package statement coverage, `origin/main` (`2d7822f214`) against this PR. The "before" run used a
temporary detached worktree of `origin/main`, removed afterwards.

| Package | Before | After |
|---|---|---|
| `cmd/aws/ecr` | 87.2% | 89.7% |
| `cmd/azure/acr` | 88.6% | 90.5% |
| `errors` | 87.3% | 87.3% |
| `pkg/auth/cloud/docker` | 95.7% | 99.2% |
| `pkg/auth/integrations/aws` | 93.8% | 94.5% |
| `pkg/auth/integrations/azure` | 93.6% | 94.4% |
| `pkg/env` (test-only change) | 98.2% | 98.7% |

Not run:

- `golangci-lint` and the git hooks; CI lint is the first lint pass.
- `go test -race`.
- The website build, because `website/node_modules` is not installed in this worktree.
- Tests on Linux or Windows.
- A real `docker-credential-*` helper or a live registry; the tests use fake helpers and stubbed token
  exchanges.

## Follow-ups

- **Behavior change for users.** A registry that `credHelpers` or `credsStore` routes to a helper now
  has its token stored through that helper. If the helper cannot store it, the login fails, unless the
  helper replies "not implemented" (such as `ecr-login`). In that case Atmos writes nothing and prints a
  note. Previously Atmos dropped `credHelpers` and `credsStore` from the file and wrote an inline token.
- **`ecr-login` uses Docker's own AWS credentials.** For a registry left to `ecr-login`, Docker gets
  tokens with the AWS credentials in its own environment, not the Atmos identity. The ECR page says to
  run Docker under `atmos auth exec` or `atmos auth shell`, or to remove the registry from `credHelpers`. When `credsStore` is `ecr-login` too, removing the
  entry still selects `ecr-login`; set an explicit empty entry for the registry (`"public.ecr.aws": ""`) to
  override the global store.
