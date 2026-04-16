# AWS_PROFILE Environment Variable Breaks All Atmos Commands (Issue #2076)

**GitHub Issue:** https://github.com/cloudposse/atmos/issues/2076

**Affected Versions:** v1.206.0, v1.206.1

**Severity:** Critical - affects ALL commands when `AWS_PROFILE` is set

## Symptoms

Any Atmos command (including `atmos version`) fails with a "profile not found" error when
`AWS_PROFILE` is set in the environment:

```text
WARN Could not load config error="profile \"my-aws-profile\" not found in directories [...]"
```

The error occurs because Atmos interprets `AWS_PROFILE` (an AWS-specific environment variable)
as an Atmos configuration profile name.

## Root Cause

The `aws eks update-kubeconfig` command's `init()` function in `cmd/aws/eks/update_kubeconfig.go`
registered a local "profile" flag (for AWS CLI authentication) with environment variable bindings
`["ATMOS_AWS_PROFILE", "AWS_PROFILE"]`. These bindings were applied to the **global** Viper instance
via `BindToViper(viper.GetViper())`.

Since the flag name "profile" matches the global `--profile` flag (used for Atmos configuration
profiles), the EKS command's `BindEnv("profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE")` call
**overwrites** the global binding `BindEnv("profile", "ATMOS_PROFILE")`. In Viper, the last
`BindEnv` call for a key wins.

This caused two problems:

1. **Env var pollution:** `viper.GetViper().IsSet("profile")` returns true when `AWS_PROFILE` is
   set, triggering Atmos to treat it as a configuration profile name.
2. **Default type change:** The EKS parser's `SetDefault("profile", "")` overwrites the global
   parser's `SetDefault("profile", []string(nil))`, changing the expected type from StringSlice
   to String.

The collision happens at **init time** (not at command execution time), meaning it affects ALL
commands, not just `aws eks update-kubeconfig`.

### Call Chain

1. `cmd/root.go` init() -> `globalParser.BindToViper(viper.GetViper())` -> binds `"profile"` to `"ATMOS_PROFILE"`
2. `cmd/aws/eks/update_kubeconfig.go` init() -> `updateKubeconfigParser.BindToViper(viper.GetViper())` -> overwrites `"profile"` to `["ATMOS_AWS_PROFILE", "AWS_PROFILE"]`
3. Any command -> `getProfilesFromFlagsOrEnv()` -> `viper.GetViper().GetStringSlice("profile")` -> returns `AWS_PROFILE` value
4. `loadProfiles()` -> tries to load AWS profile name as Atmos config profile -> "profile not found"

## Fix

Added `flags.WithViperPrefix("eks")` to the `NewStandardParser()` call in `cmd/aws/eks/update_kubeconfig.go`.
This namespaces all EKS Viper keys under `eks.*` (e.g., `eks.profile`, `eks.stack`, `eks.region`)
so they don't collide with global keys.

The EKS command reads flags from Cobra pflags (via `cmd.Flags().GetString("profile")`), not from
Viper, so the prefix has no functional impact on the command itself.

**Before (buggy):**
```go
updateKubeconfigParser = flags.NewStandardParser(
    flags.WithStringFlag("profile", "", "", "AWS CLI profile"),
    flags.WithEnvVars("profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"),
)
updateKubeconfigParser.BindToViper(viper.GetViper())
// Result: viper key "profile" -> env vars ["ATMOS_AWS_PROFILE", "AWS_PROFILE"]
// OVERWRITES global "profile" -> "ATMOS_PROFILE"
```

**After (fixed):**
```go
updateKubeconfigParser = flags.NewStandardParser(
    flags.WithViperPrefix("eks"),
    flags.WithStringFlag("profile", "", "", "AWS CLI profile"),
    flags.WithEnvVars("profile", "ATMOS_AWS_PROFILE", "AWS_PROFILE"),
)
updateKubeconfigParser.BindToViper(viper.GetViper())
// Result: viper key "eks.profile" -> env vars ["ATMOS_AWS_PROFILE", "AWS_PROFILE"]
// Global "profile" -> "ATMOS_PROFILE" is PRESERVED
```

**Files changed:** `cmd/aws/eks/update_kubeconfig.go`

## Test Coverage

All tests in `cmd/aws/eks/update_kubeconfig_test.go`:

- `TestUpdateKubeconfigParser_ViperPrefix` - Verifies EKS keys are namespaced under `eks.*`
- `TestUpdateKubeconfigParser_AwsProfileDoesNotAffectGlobalProfile` - Core regression test: AWS_PROFILE does not pollute global profile
- `TestUpdateKubeconfigParser_WithoutPrefix_KeyCollision` - Demonstrates the bug without the fix
- `TestUpdateKubeconfigParser_BindFlagsToViper` - Verifies runtime flag binding uses prefixed keys
- `TestUpdateKubeconfigParser_AtmosProfileStillWorks` - Verifies ATMOS_PROFILE still works correctly
- `TestUpdateKubeconfigParser_BothEnvVarsSet` - Verifies correct isolation when both AWS_PROFILE and ATMOS_PROFILE are set

## Relationship to Previous Fixes

This issue is a continuation of the v1.206.0 auth realm isolation issues documented in
`docs/fixes/2026-02-12-auth-realm-isolation-issues.md`. While the previous fixes addressed
ATMOS_PROFILE/identity confusion and realm credential paths, this issue is a separate Viper
namespace collision introduced by the `aws eks update-kubeconfig` command, which predates v1.206.0
but was masked until the profile loading logic was tightened by the auth realm work.
