# Fix: AWS identities isolate their environment and files, and SSO login is safe under concurrency

**Date:** 2026-11-22

## Summary

This change fixes the AWS identity and AWS SSO items from the Atmos Auth pre-release review (#3720):

- The SSO device-code dialog and the webflow dialog fit an 80-column terminal again.
- `BuildAWSConfigFromCreds` (ECR login, EKS describe-cluster, `aws eks token`) no longer reads the user's
  AWS profile or shared config files.
- Permission-set identities apply `principal.region` to the credentials they return.
- SSO device-token polling stops on cancellation, follows RFC 8628 for `slow_down`, and keeps the AWS
  error as the cause.
- Permission-set, assume-role, assume-root, plugin and mock AWS identities write, report and remove their
  files under the root provider's `spec.files.base_path`.
- The on-disk SSO token cache is written atomically and is read and refreshed under a cross-process lock,
  with an unlocked fallback when the lock is unavailable.
- Every temporary change to the `AWS_*` process environment runs under one process-wide lock.

Every fix has a test. Other #3720 and regression-test fixes land in separate PRs.

## Context

The review findings were recorded against an earlier `main` and re-verified on `main` at `2d7822f214`
before any fix. The items here come from #3720:

- Finding 1.3 (80-column dialogs). The review lists it as new since v1.234.1.
- Finding 3.8 (`BuildAWSConfigFromCreds` and the user's AWS environment).
- Finding 3.9 (permission-set region).
- Finding 4.5 (SSO polling).
- Finding 4.9 (`spec.files.base_path`), including the plugin and mock identities that the first round
  left open.
- Two suspected items, both confirmed real: the non-atomic SSO token cache, and `WithIsolatedAWSEnv`
  running without the credentials loader's `envMu` lock.

None of these items is one of the regressions that the 1.235.0 regression test found against v1.234.1.

## Changes

### Sign-in dialogs fit an 80-column terminal (#3720 finding 1.3)

- **Problem.** The SSO device-code dialog and the webflow dialog rendered 84 to 86 columns wide and
  wrapped in an 80-column terminal.
- **Cause.** Both dialogs accepted a URL up to `link.MaxInlineWidth` (80) without subtracting the box
  border and horizontal padding (6 columns). The SSO instruction line was 79 characters inside that box.
- **Fix.** The inline URL budget is now `link.MaxInlineWidth` minus the box's horizontal frame size
  (`fitsInDialog` in `pkg/auth/providers/aws/sso_ui.go`, and the same check in
  `pkg/auth/identities/aws/webflow_ui.go`). A longer URL is printed outside the dialog. The SSO
  instruction is split over two lines.
- **Tests.** `TestRenderVerificationDialog_FitsStandardTerminal` (`sso_ui_width_test.go`) and
  `TestRenderWebflowDialog_FitsStandardTerminal` (`webflow_ui_width_test.go`). The existing table tests
  in `sso_ui_test.go` and `webflow_ui_test.go` now use the 74-column URL limit.

### `BuildAWSConfigFromCreds` ignores the user's AWS environment (#3720 finding 3.8)

- **Problem.** A stale `AWS_PROFILE`, or an `AWS_CONFIG_FILE` without that profile, made ECR login, EKS
  describe-cluster and `aws eks token` fail with "failed to get shared config profile", even though Atmos
  had already resolved the credentials.
- **Cause.** `BuildAWSConfigFromCreds` called `config.LoadDefaultConfig`, which reads the shared config
  and credentials files and `AWS_PROFILE`.
- **Fix.** It calls `LoadIsolatedAWSConfig`, like the rest of Atmos auth
  (`pkg/auth/cloud/aws/config.go`). The region and static credentials are still passed explicitly.
- **Test.** `TestBuildAWSConfigFromCreds_IgnoresUserAWSEnvironment` (`pkg/auth/cloud/aws/config_test.go`)
  checks that a stale profile and settings in the user's config files neither break nor leak into the
  config.

### Permission sets apply `principal.region` (#3720 finding 3.9)

- **Problem.** `AWS_REGION` in the component environment was the Identity Center (SSO) region, while
  `atmos auth exec` used `principal.region`.
- **Cause.** `permissionSetIdentity.Authenticate` stamped the base credentials' region, which is the SSO
  region, onto the role credentials.
- **Fix.** The credentials region comes from `resolveRegion()` (`principal.region`, inherited through the
  chain). It falls back to the SSO region when no region is configured. The SSO client still talks to the
  Identity Center region (`pkg/auth/identities/aws/permission_set.go`).
- **Test.** `TestPermissionSetIdentity_Authenticate_UsesPrincipalRegion` (`permission_set_region_test.go`).

### SSO polling honors cancellation and RFC 8628 `slow_down` (#3720 finding 4.5)

- **Problem.** After Ctrl+C or a timeout, the poller stayed blocked for a whole interval and then sent
  another `CreateToken` request. `slow_down` doubled the original interval once. A terminal OIDC error
  dropped the AWS error from the chain.
- **Cause.** `pollForAccessToken` waited with `time.Sleep`, computed the slow-down interval from the
  original value, and built the terminal error without `WithCause`.
- **Fix.** The poller moved to `pkg/auth/providers/aws/sso_poll.go`.
  - It waits on a context-aware timer (`waitForPollInterval`), injectable through `ssoProvider.pollWait`
    for tests.
  - Each `slow_down` adds 5 seconds to the interval for this and all later requests (RFC 8628 section
    3.5).
  - Polling is also bounded by the device-code expiry, since longer waits make the attempt count alone
    too loose.
  - Cancellation returns `ErrSSOTokenCreationFailed` with the context error in the chain. A terminal AWS
    error is kept as the cause.
  - The poller takes the `ssoTokenRefresher` interface instead of `*ssooidc.Client`.
- **Tests** (`sso_poll_test.go`): `TestPollForAccessToken_ContextCancellation`,
  `TestPollForAccessToken_CancelledWhilePending`, `TestPollForAccessToken_RequestFailsAfterCancellation`,
  `TestPollForAccessToken_SlowDownAddsFiveSeconds`, `TestPollForAccessToken_TerminalErrorKeepsCause` and
  `TestWaitForPollInterval`. `TestPollForAccessToken_NormalizesZeroInterval` is in
  `sso_cache_lock_test.go`.
- The old `TestPollForAccessToken_ContextCancellation` in `sso_test.go` never called the function under
  test, and `TestSSOProvider_PollForAccessToken_SlowDown` in `sso_extended_test.go` only instantiated the
  error types. Both were removed.

### AWS identities honor the provider's `spec.files.base_path` (#3720 finding 4.9)

- **Problem.** With `spec.files.base_path` set on an SSO or SAML provider, identities wrote credentials
  to the default XDG directory, while the provider reported and deleted files under `base_path`.
- **Cause.** The permission-set, assume-role and assume-root identities, the native plugin AWS
  materializer and the mock AWS identity all passed a hard-coded empty base path.
- **Fix.**
  - A new helper, `filesBasePath`, resolves the root provider's `base_path` through the auth manager and
    returns `""` (the default location) when no manager or `base_path` is available
    (`pkg/auth/identities/aws/files_base_path.go`). It is exported as `FilesBasePath` for identities
    outside that package.
  - Permission-set, assume-role and assume-root identities use it in `PostAuthenticate` (`SetupFiles` and
    the auth context), `Environment`, `PrepareEnvironment`, `CredentialsExist` and `Logout`.
  - The plugin AWS materializer uses it in `Apply` (files and auth context), `Environment`, `Paths` and
    `Logout`; `Load` and `PrepareEnvironment` go through `Environment` (`pkg/plugin/adapter/auth/aws.go`).
    Plugin identities now implement `SetManagerAndProvider`, which the auth manager already calls, so a
    fresh process that reuses cached credentials resolves the same path without a `PostAuthenticate`
    (`pkg/plugin/adapter/auth/identity.go`).
  - The mock AWS identity uses it for `SetupFiles` and its auth context
    (`pkg/auth/providers/mock/aws/identity.go`).
  - `aws/user` and the credential-process identity have no provider config, so they keep the default
    location.
- **Tests.** `TestAWSIdentities_HonorProviderFilesBasePath` (`files_base_path_test.go`),
  `TestFilesBasePath_ResolvesLikeBuiltInIdentities` (`files_base_path_export_test.go`),
  `TestIdentity_PostAuthenticateHonorsProviderFilesBasePath` (`identity_base_path_test.go`), and
  `TestAWSMaterializerHonorsProviderFilesBasePath`, `TestPluginIdentityResolvesBasePathBeforeAuthentication`
  and `TestPluginIdentityWithoutAWSMaterializerIgnoresManager` (`aws_base_path_test.go`).

### The SSO token cache is safe for concurrent processes (#3720 suspected item)

- **Problem.** Parallel `kubectl` exec-plugin calls could fall through to the device flow, which an exec
  plugin cannot run.
- **Cause.** Two problems:
  - The cache was written with `os.WriteFile`, which truncates first, so a concurrent reader could see an
    empty or partial file and treat it as a cache miss.
  - Two processes that found the same expired token both exchanged its refresh token. AWS rotates
    refresh tokens, so the second exchange failed.
- **Fix.**
  - The cache file is replaced atomically with `filesystem.WriteFileAtomic`
    (`pkg/auth/providers/aws/sso_cache.go`).
  - The disk-cache read and the refresh exchange run under a cross-process lock on `<cache>.lock`
    (`loadOrRefreshDiskToken` in `pkg/auth/providers/aws/sso_cache_lock.go`). A waiting process then
    reads the token the first one wrote. Device-flow tokens are saved under the same lock.
  - The lock is best effort. When the cache path cannot be determined, or the lock is not acquired
    within 30 seconds, the work runs once without the lock, which is the previous behavior.
- **Tests.** `TestSSOTokenCache_ReadersNeverSeeTruncatedFile` and
  `TestSSOProviderAuthenticate_ConcurrentProcessesRefreshOnce` (`sso_cache_concurrency_test.go`), and
  `TestWithTokenCacheLock_RunsWithoutCachePath` and `TestWithTokenCacheLock_RunsWhenLockIsHeld`
  (`sso_cache_lock_test.go`).

### All temporary `AWS_*` process-environment changes are serialized (#3720 suspected item)

- **Problem.** An isolated config load could see an identity's `AWS_PROFILE`, the credentials loader
  could lose its files mid-load, and a restore of the wrong snapshot could leave the user's
  `AWS_SHARED_CREDENTIALS_FILE` unset for the rest of the run.
- **Cause.** The identity credentials loader serialized its set, load and restore with a package-local
  `envMu`. `WithIsolatedAWSEnv` (used by `LoadIsolatedAWSConfig` for every SSO, STS and ECR/EKS client)
  and `LoadAtmosManagedAWSConfig` changed the same variables without it.
- **Fix.** `awsCloud.LockAWSProcessEnv` (`pkg/auth/cloud/aws/env.go`) is one process-wide lock, held by
  `WithIsolatedAWSEnv`, `LoadAtmosManagedAWSConfig` and the credentials loader
  (`pkg/auth/identities/aws/credentials_loader.go`), which drops `envMu`. The lock is not reentrant; none
  of the callbacks run under it take it again.
- **Test.** `TestAWSProcessEnv_LoaderAndIsolationDoNotInterleave` (`credentials_loader_env_lock_test.go`).

## Validation

All commands ran in this PR's worktree on darwin.

- `go build ./...` passes.
- `go vet` passes on the five changed packages: `pkg/auth/cloud/aws`, `pkg/auth/identities/aws`,
  `pkg/auth/providers/aws`, `pkg/auth/providers/mock/aws` and `pkg/plugin/adapter/auth`.
- `gofumpt -l` reports no files among the changed Go files.
- `go test -count=1 -cover` passes on the five changed packages.
- `go test -count=1 -race` passes on the same five packages, including the new concurrency tests.

The PR does not touch the `./cmd` root package or `./internal/exec`.

Package statement coverage, `origin/main` (`2d7822f214`) against this PR:

| Package | Before | After |
|---|---|---|
| `pkg/auth/cloud/aws` | 93.0% | 93.0% |
| `pkg/auth/identities/aws` | 90.8% | 90.9% |
| `pkg/auth/providers/aws` | 86.7% | 87.1% |
| `pkg/auth/providers/mock/aws` | 77.9% | 93.2% |
| `pkg/plugin/adapter/auth` | 95.9% | 96.3% |

Not run:

- golangci-lint, so CI lint is the first lint pass.
- Tests on Linux or Windows.
- Interactive TTY sign-in against a real AWS SSO portal. The dialogs are checked by rendering tests.
- The wider test suites (`./cmd`, `./internal/exec`, `TestCLICommands`); they run in CI.
- No documentation pages changed, so the website build was not run.

## Follow-ups

No open work remains for these items. Compatibility notes for release notes:

- ECR and EKS clients built from Atmos credentials no longer read `ca_bundle` or `endpoint_url` from
  `~/.aws/config`. `AWS_CA_BUNDLE` still works.
- With `spec.files.base_path` set, the first `auth env` after upgrading points at files under
  `base_path`, which a re-authentication then creates. Files written by older versions under the default
  XDG directory are not moved.
- For permission sets with a `principal.region` that differs from the SSO region, `AWS_REGION` in the
  component environment changes to `principal.region`.
