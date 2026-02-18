# Fix AWS Default Profile Interference with SSO Authentication

**Date:** 2026-02-17

**Related Issue:** Misleading error when `AWS_PROFILE` or `[default]` profile in `~/.aws/config` interferes
with SSO device authorization flow.

**Affected Atmos Version:** v1.160.0+ (introduced with Atmos Auth)

**Severity:** Medium — SSO authentication fails with a misleading error message when a user has a default AWS
profile configured, making the root cause difficult to diagnose.

## Background

When running `atmos auth login`, the SSO provider loads an AWS config to initialize the OIDC client for
device authorization. The `LoadIsolatedAWSConfig` function was intended to completely isolate this config
loading from the user's existing AWS environment. However, the implementation had a gap:

- `WithIsolatedAWSEnv` correctly unsets `AWS_PROFILE`, `AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE`,
  and credential env vars during config loading.
- `config.WithSharedConfigProfile("")` was used with the intent to "disable shared config loading," but
  in the AWS SDK v2, an empty string means "use the default profile."
- The AWS SDK still loads `~/.aws/config` and `~/.aws/credentials` from their default filesystem paths
  even when the corresponding env vars are unset.

This means if the user has a `[default]` profile in `~/.aws/config` that references SSO configuration,
credential processes, or other non-trivial settings, the SDK attempts to resolve those during
`LoadDefaultConfig` and may fail with a confusing error.

## Symptoms

```
Error: failed to load AWS config

## Explanation
Failed to load AWS configuration for SSO authentication in region 'us-west-2'

## Hints
💡 Verify that the AWS region is valid and accessible
💡 Check your network connectivity and AWS service availability
```

The hints suggest region/network issues, but the actual cause is the `[default]` profile in
`~/.aws/config` (or `AWS_PROFILE` env var) interfering with config loading.

## Root Cause

Two issues:

### 1. Incomplete isolation in `LoadIsolatedAWSConfig`

`config.WithSharedConfigProfile("")` does **not** disable shared config file loading. In the AWS SDK v2,
an empty profile name resolves to the default profile (`[default]`). The SDK still reads `~/.aws/config`
and `~/.aws/credentials` from their default paths (`$HOME/.aws/config` and `$HOME/.aws/credentials`).

The correct approach is to use `config.WithSharedConfigFiles([]string{})` and
`config.WithSharedCredentialsFiles([]string{})` to provide empty file lists, which prevents the SDK
from loading any shared config files.

### 2. Misleading error message

The error at `sso.go:153-161` only hints at region/network issues. It does not mention:
- The `AWS_PROFILE` environment variable as a potential cause.
- The `~/.aws/config` default profile as a potential cause.
- That Atmos auth isolates from external AWS configuration (so the user knows this was attempted).

## Fix

### Approach

1. Replace `config.WithSharedConfigProfile("")` with `config.WithSharedConfigFiles([]string{})` and
   `config.WithSharedCredentialsFiles([]string{})` for complete filesystem isolation.
2. Add a warning log when `AWS_PROFILE` is set or `~/.aws/config` exists with a default profile,
   informing the user that these will be ignored during SSO auth.
3. Improve the error message in the SSO provider to include hints about `AWS_PROFILE` and default
   profiles in `~/.aws/config`.

### Implementation

#### 1. Fix `LoadIsolatedAWSConfig` (`pkg/auth/cloud/aws/env.go`)

Replace `config.WithSharedConfigProfile("")` with:
- `config.WithSharedConfigFiles([]string{})`
- `config.WithSharedCredentialsFiles([]string{})`

This completely prevents the AWS SDK from reading any shared config files during isolated operations.

#### 2. Add `WarnIfAWSProfileSet` helper (`pkg/auth/cloud/aws/env.go`)

New function that logs a warning when `AWS_PROFILE` is set, informing the user that it will be
ignored during Atmos auth.

#### 3. Improve SSO error message (`pkg/auth/providers/aws/sso.go`)

Add hints about:
- Checking if `AWS_PROFILE` environment variable is set.
- Checking for a `[default]` profile in `~/.aws/config`.
- The fact that Atmos auth operates in an isolated AWS environment.

#### 4. Call warning from SSO `Authenticate` method

Before loading the isolated config, call `WarnIfAWSProfileSet` to emit a debug-level warning.

### Files changed

| File                                         | Change                                                               |
|----------------------------------------------|----------------------------------------------------------------------|
| `pkg/auth/cloud/aws/env.go`                  | Fix `LoadIsolatedAWSConfig` isolation; add `WarnIfAWSProfileSet`     |
| `pkg/auth/providers/aws/sso.go`              | Improve error hints; call `WarnIfAWSProfileSet` before config load   |
| `pkg/auth/cloud/aws/env_test.go`             | Tests for isolation fix and warning detection                        |
| `pkg/auth/providers/aws/sso_test.go`         | Tests for improved error messages                                    |

### Tests

| Test                                                    | What it verifies                                                  |
|---------------------------------------------------------|-------------------------------------------------------------------|
| `TestWithIsolatedAWSEnv_ClearsAllProblematicVars`       | All problematic vars are cleared during execution and restored    |
| `TestLoadIsolatedAWSConfig_IgnoresDefaultProfile`       | Default profile in ~/.aws/config does not affect isolated config  |
| `TestWarnIfAWSProfileSet_LogsWarning`                   | Warning is logged when AWS_PROFILE is set                         |
| `TestWarnIfAWSProfileSet_NoWarningWhenUnset`            | No warning when AWS_PROFILE is not set                            |
| `TestSSOProvider_Authenticate_ErrorIncludesProfileHint` | Error message includes hint about AWS_PROFILE                     |
