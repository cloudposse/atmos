# Fix AWS Default Profile Interference with SSO Authentication

**Date:** 2026-02-17

**Related Issue:** Misleading error when `[default]` profile in `~/.aws/config` interferes with SSO
device authorization flow.

**Affected Atmos Version:** v1.160.0+ (introduced with Atmos Auth)

**Severity:** Medium — SSO authentication fails with a confusing error when a user has a `[default]`
profile with non-trivial settings in `~/.aws/config`.

## Background

When running `atmos auth login`, the SSO provider calls `LoadIsolatedAWSConfig` to load a clean AWS
config for the OIDC client. This function was intended to completely isolate config loading from the
user's environment. The sanitization had two layers:

1. **Env var isolation** (`WithIsolatedAWSEnv`) — Clears `AWS_PROFILE`, `AWS_CONFIG_FILE`,
   `AWS_SHARED_CREDENTIALS_FILE`, and credential env vars during config loading. This worked correctly.
2. **Shared config file isolation** (`config.WithSharedConfigProfile("")`) — Intended to prevent loading
   from `~/.aws/config`. This did **not** work as expected.

## Symptoms

```text
Error: failed to load AWS config

## Explanation
Failed to load AWS configuration for SSO authentication in region 'us-west-2'

## Hints
💡 Verify that the AWS region is valid and accessible
💡 Check your network connectivity and AWS service availability
```

The error suggests region/network issues, but the actual cause is the `[default]` profile in
`~/.aws/config` being loaded and failing to resolve (e.g., `credential_process` pointing to a
missing binary, or SSO settings that conflict with the Atmos-managed flow).

## Root Cause

The root cause is a gap in the sanitization process: `WithIsolatedAWSEnv` correctly sanitized
environment variables, but the AWS SDK v2 has **two independent mechanisms** to find `~/.aws/config`:

1. **`AWS_CONFIG_FILE` env var** — Sanitized correctly by `WithIsolatedAWSEnv`.
2. **Hardcoded `$HOME/.aws/config` default** — Was **not** blocked by env var cleanup.

The original code attempted to block mechanism #2 with `config.WithSharedConfigProfile("")`, but this
is ineffective due to how the AWS SDK v2 processes these options internally.

### Detailed SDK trace

The following traces through the AWS SDK v2 source code (github.com/aws/aws-sdk-go-v2/config) to show
exactly how `~/.aws/config` leaked through.

#### Step 1: `WithSharedConfigProfile("")` is a no-op

```go
// load_options.go:426-431
func (o LoadOptions) getSharedConfigProfile(ctx context.Context) (string, bool, error) {
    if len(o.SharedConfigProfile) == 0 {
        return "", false, nil     // empty string → found=false
    }
    return o.SharedConfigProfile, true, nil
}
```

Setting `SharedConfigProfile` to `""` causes the getter to return `found=false`, which means
"no profile was specified." The SDK then falls back to the default profile:

```go
// shared_config.go:584-586
profile, ok, err = getSharedConfigProfile(ctx, configs)
if !ok {
    profile = defaultSharedConfigProfile  // falls back to "default"
}
```

So `WithSharedConfigProfile("")` effectively means **"use the `[default]` profile"**, the opposite
of the intent to disable profile loading.

#### Step 2: `SharedConfigFiles` was never set (remains `nil`)

The original code did not call `WithSharedConfigFiles(...)`, so `LoadOptions.SharedConfigFiles`
remained `nil`. The SDK's getter treats `nil` as "not specified":

```go
// load_options.go:448-454
func (o LoadOptions) getSharedConfigFiles(ctx context.Context) ([]string, bool, error) {
    if o.SharedConfigFiles == nil {
        return nil, false, nil    // nil → found=false
    }
    return o.SharedConfigFiles, true, nil
}
```

When `found=false`, the SDK falls back to hardcoded defaults:

```go
// shared_config.go:656-658
if option.ConfigFiles == nil {
    option.ConfigFiles = DefaultSharedConfigFiles  // → []string{"~/.aws/config"}
}
```

`DefaultSharedConfigFiles` is `[]string{filepath.Join(home, ".aws", "config")}`. This path is
resolved from `$HOME`, not from `AWS_CONFIG_FILE`. Unsetting `AWS_CONFIG_FILE` has no effect.

#### Step 3: SDK loads `~/.aws/config` with `[default]` profile

With `profile = "default"` and `configFiles = ["~/.aws/config"]`, the SDK calls `loadIniFiles`
and `setFromIniSections`, which parses the `[default]` profile and attempts to resolve any
settings it contains (credential processes, SSO configuration, etc.).

If the `[default]` profile contains settings that fail to resolve (e.g., `credential_process`
pointing to a missing binary), the SDK returns an error that propagates up as
"failed to load AWS config" with no indication that it came from the user's shared config file.

### Summary

The env var sanitization in `WithIsolatedAWSEnv` was correct but insufficient. The AWS SDK v2
resolves shared config files through a hardcoded `$HOME/.aws/config` path that is independent of
the `AWS_CONFIG_FILE` env var. The only way to prevent this is to explicitly provide an empty
file list via `WithSharedConfigFiles([]string{})`.

## Fix

### Approach

Replace `config.WithSharedConfigProfile("")` with `config.WithSharedConfigFiles([]string{})` and
`config.WithSharedCredentialsFiles([]string{})` for complete filesystem isolation.

### Why this works

`WithSharedConfigFiles([]string{})` sets `LoadOptions.SharedConfigFiles` to a non-nil empty slice.
The SDK's getter returns `found=true` with an empty list:

```go
func (o LoadOptions) getSharedConfigFiles(ctx context.Context) ([]string, bool, error) {
    if o.SharedConfigFiles == nil {   // []string{} is NOT nil
        return nil, false, nil
    }
    return o.SharedConfigFiles, true, nil  // returns empty slice, found=true
}
```

Since `found=true`, the SDK uses the provided (empty) list instead of falling back to defaults:

```go
if option.ConfigFiles == nil {          // NOT nil (it's []string{})
    option.ConfigFiles = DefaultSharedConfigFiles  // SKIPPED
}
```

`loadIniFiles([]string{})` loads nothing. Combined with `WithIsolatedAWSEnv` clearing env vars,
this achieves complete isolation from both environment variables and filesystem config files.

### Implementation

#### Fix `LoadIsolatedAWSConfig` (`pkg/auth/cloud/aws/env.go`)

Replace `config.WithSharedConfigProfile("")` with:
- `config.WithSharedConfigFiles([]string{})`
- `config.WithSharedCredentialsFiles([]string{})`

### Files changed

| File                                         | Change                                                               |
|----------------------------------------------|----------------------------------------------------------------------|
| `pkg/auth/cloud/aws/env.go`                  | Fix `LoadIsolatedAWSConfig` shared config file isolation             |
| `pkg/auth/cloud/aws/env_test.go`             | Tests for isolation fix                                              |
| `pkg/auth/providers/aws/sso.go`              | Remove misleading error hints now that isolation is complete         |

### Tests

| Test                                                    | What it verifies                                                  |
|---------------------------------------------------------|-------------------------------------------------------------------|
| `TestWithIsolatedAWSEnv_ClearsAllProblematicVars`       | All problematic vars are cleared during execution and restored    |
| `TestLoadIsolatedAWSConfig_IgnoresDefaultProfile`       | Default profile in ~/.aws/config does not affect isolated config  |
| `TestLoadIsolatedAWSConfig_DoesNotLoadSharedFiles`      | Shared config file settings do not leak into isolated config      |
