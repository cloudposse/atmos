# Fix: Configuration Profiles Not Loading from CLI Flag

## Problem Statement

When executing Atmos commands with the `--profile` CLI flag, the specified profile configuration is not loaded and merged with the global configuration. This causes authentication failures and missing configuration even when valid profiles are defined. The `ATMOS_PROFILE` environment variable works correctly, but the `--profile` CLI flag does not.

## Symptoms

**Error Message:**

```
Error: No valid credential sources found
```

**Command That Should Work:**

```bash
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers
```

**Expected Behavior:**
- Profile configuration should be loaded and merged with global config
- Authentication should work using profile's auth providers and identities
- `atmos describe config --profile managers` should show merged configuration

**Actual Behavior:**
- Profile configuration is not loaded when using `--profile` flag
- `atmos describe config --profile managers` shows `"providers": null`
- Authentication fails with "No valid credential sources found"
- `ATMOS_PROFILE=managers atmos describe config` works correctly (env var path)

## Root Cause

### Viper Flag Binding Timing Issue

Viper's `BindPFlag()` creates a binding between a Viper key and a Cobra pflag, but **flag values aren't immediately synced to Viper** after Cobra parses them:

1. **During initialization:**
   - Global flags are bound to Viper: `viper.BindPFlag("profile", cmd.Flags().Lookup("profile"))`
   - Binding exists but no value yet

2. **When command runs:**
   - Cobra has parsed flags
   - Flag value exists in Cobra's `FlagSet`
   - BUT: Viper hasn't synchronized the value yet
   - `viper.IsSet("profile")` returns `true` (binding exists)
   - `viper.GetStringSlice("profile")` returns `[]` (value not synced)

3. **Environment variables work differently:**
   - Viper reads env vars directly, not through flag binding
   - `ATMOS_PROFILE` is immediately available in Viper
   - No synchronization delay

**This is why `ATMOS_PROFILE=managers` works but `--profile managers` doesn't.**

## Solution Implemented

### Overview

The fix addresses the Viper timing issue by **explicitly syncing flag values to Viper** in `cmd/root.go` before calling `InitCliConfig()`. This ensures both CLI flags and environment variables work correctly.

### Implementation

#### 1. Sync Flags in root.go (Main Solution)

**File:** `cmd/root.go`

Added `syncGlobalFlagsToViper()` function that runs in `PersistentPreRun` before `InitCliConfig()`:

```go
// syncGlobalFlagsToViper synchronizes global flags from Cobra's FlagSet to Viper.
// This is necessary because Viper's BindPFlag doesn't immediately sync values when flags are parsed.
func syncGlobalFlagsToViper(cmd *cobra.Command) {
	v := viper.GetViper()

	// Sync profile flag if explicitly set
	if cmd.Flags().Changed("profile") {
		if profiles, err := cmd.Flags().GetStringSlice("profile"); err == nil {
			v.Set("profile", profiles)
		}
	}

	// Sync identity flag if explicitly set
	if cmd.Flags().Changed("identity") {
		if identity, err := cmd.Flags().GetString("identity"); err == nil {
			v.Set("identity", identity)
		}
	}
}

// In PersistentPreRun (called before every command):
syncGlobalFlagsToViper(cmd)
atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, false)
```

**How it works:**
1. After Cobra parses flags, `PersistentPreRun` calls `syncGlobalFlagsToViper()`
2. Function reads changed flags from Cobra's `FlagSet` using `cmd.Flags().Get*()`
3. Writes values directly to Viper using `v.Set()`
4. `InitCliConfig()` can now read flag values from Viper immediately

#### 2. Fallback for DisableFlagParsing Commands

**File:** `pkg/config/load.go`

For commands with `DisableFlagParsing=true` (terraform/helmfile/packer), Cobra never parses flags, so `syncGlobalFlagsToViper()` can't read from the FlagSet. We use pflag as a fallback:

```go
// parseProfilesFromOsArgs parses --profile flags from os.Args using pflag.
// This is a fallback for commands with DisableFlagParsing=true (terraform, helmfile, packer).
func parseProfilesFromOsArgs(args []string) []string {
	fs := pflag.NewFlagSet("profile-parser", pflag.ContinueOnError)
	fs.ParseErrorsAllowlist.UnknownFlags = true

	profiles := fs.StringSlice("profile", []string{}, "Configuration profiles")
	_ = fs.Parse(args)

	// Trim whitespace and filter empty values
	result := make([]string, 0, len(*profiles))
	for _, profile := range *profiles {
		if trimmed := strings.TrimSpace(profile); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
```

**Note:** This fallback is only needed until terraform/helmfile/packer migrate to the command registry pattern.

#### 3. Simplified Profile Loading

**File:** `pkg/config/load.go`

`getProfilesFromFlagsOrEnv()` reads from Viper (which now has synced values):

```go
func getProfilesFromFlagsOrEnv() ([]string, string) {
	globalViper := viper.GetViper()

	// Check if profile is set in Viper (from either flag or env var)
	// syncGlobalFlagsToViper() ensures CLI flag values are synced before InitCliConfig
	if globalViper.IsSet("profile") && len(globalViper.GetStringSlice("profile")) > 0 {
		profiles := globalViper.GetStringSlice("profile")

		// Determine source based on whether ATMOS_PROFILE env var is explicitly set
		if _, envSet := os.LookupEnv("ATMOS_PROFILE"); envSet {
			return profiles, "env"
		}
		return profiles, "flag"
	}

	return nil, ""
}
```

### Why This Solution Works

**For normal commands:**
1. Cobra parses flags → stores values in FlagSet
2. `syncGlobalFlagsToViper()` reads FlagSet → writes to Viper
3. `getProfilesFromFlagsOrEnv()` reads from Viper → returns profiles
4. Profile loading works ✅

**For DisableFlagParsing commands:**
1. Cobra skips flag parsing
2. `parseProfilesFromOsArgs()` parses `os.Args` with pflag
3. Values eventually get into Viper
4. Profile loading works ✅

**For environment variables:**
1. Viper reads `ATMOS_PROFILE` directly (no flag binding)
2. Values immediately available in Viper
3. Profile loading works ✅

### Benefits

- ✅ **Fixes root cause** - Viper has flag values available immediately
- ✅ **Clean API** - No need to pass `cmd` parameter through functions
- ✅ **Centralized** - All flag syncing happens in one place (root.go)
- ✅ **Minimal changes** - Only affects root.go and config loading
- ✅ **Future-proof** - Works for all commands

## Verification

### Before Fix

```bash
$ atmos describe config --profile managers | grep -A 10 '"auth"'
"auth": {
    "logs": {},
    "keyring": {},
    "providers": null,      # ❌ Profile not loaded
    "identities": null
}
```

### After Fix

```bash
$ atmos describe config --profile managers | grep -A 30 '"auth"'
"auth": {
    "logs": {
      "file": "",
      "level": "Info"
    },
    "keyring": {},
    "providers": {           # ✅ Providers from managers profile
      "cplive-sso": {
        "kind": "aws/iam-identity-center",
        "start_url": "https://cplive.awsapps.com/start/",
        "region": "us-east-2"
      }
    },
    "identities": {          # ✅ Identities from managers profile
      "core-identity/managers-team-access": {
        "kind": "aws/permission-set",
        "default": true,
        ...
      }
    }
}
```

## Testing

### Test Coverage

**Unit tests** in `pkg/config/load_flags_test.go`:
- Profiles from environment variable
- Profiles from CLI flag (`--profile value` syntax)
- Profiles from CLI flag (`--profile=value` syntax)
- No profiles specified
- Empty profile slice

**Manual testing performed:**
1. CLI flag syntax: `atmos describe config --profile managers` ✅
2. Environment variable: `ATMOS_PROFILE=managers atmos describe config` ✅
3. Comma-separated profiles: `atmos describe config --profile=managers,staging` ✅
4. Original failing command: `atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers` ✅

## Files Modified

1. **`cmd/root.go`**
   - Added `syncGlobalFlagsToViper()` function
   - Called in `PersistentPreRun` before `InitCliConfig()`

2. **`pkg/config/load.go`**
   - Added `parseProfilesFromOsArgs()` fallback for DisableFlagParsing commands
   - Simplified `getProfilesFromFlagsOrEnv()` to read from Viper

3. **`pkg/config/load_flags_test.go`** (NEW)
   - Comprehensive test suite for flag parsing

## Related Fix: Identity Flag Propagation to Nested Components

### Problem

When using `--profile` and `--identity` flags together, the identity selector appeared during nested component operations (like `!terraform.state` YAML functions).

```bash
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers --identity core-identity/managers-team-access
# ❌ Identity selector appeared for nested components
```

### Root Cause

When YAML template functions fetch state from other components, they create component-specific AuthManagers. The original implementation:
- Did NOT inherit the user's explicitly specified identity
- Always passed empty string for identity
- Relied on auto-detection from component's auth config
- With profiles containing multiple defaults, auto-detection triggered the selector

### Solution

**File:** `internal/exec/terraform_nested_auth_helper.go`

Updated `createComponentAuthManager()` to inherit authenticated identity from parent AuthManager:

```go
// Determine identity to use for component authentication
var identityName string
if parentAuthManager != nil {
	chain := parentAuthManager.GetChain()
	if len(chain) > 0 {
		// Last element in chain is the authenticated identity
		identityName = chain[len(chain)-1]
		log.Debug("Inheriting identity from parent AuthManager",
			"component", component,
			"inheritedIdentity", identityName)
	}
}

// Create AuthManager with inherited identity
componentAuthManager, err := auth.CreateAndAuthenticateManager(
	identityName,     // Inherited from parent, or empty to auto-detect
	mergedAuthConfig,
	cfg.IdentityFlagSelectValue,
)
```

### Result

- ✅ `--identity` flag propagates to nested component operations
- ✅ No identity selector appears when identity is explicitly specified
- ✅ YAML functions use inherited identity consistently
- ✅ Backward compatibility maintained (auto-detection still works when no parent exists)

## Future Improvements

### After Terraform/Helmfile/Packer Migration

Once terraform/helmfile/packer migrate to the command registry pattern, the `parseProfilesFromOsArgs()` fallback can be removed:

**Removable code:**
```go
// ❌ DELETE: No longer needed after migration
func parseProfilesFromOsArgs(args []string) []string {
    // pflag parsing fallback for DisableFlagParsing commands
}
```

**Migration checklist:**
- [ ] Remove `DisableFlagParsing: true` from command definitions
- [ ] Use standard flag parser (`flags.NewStandardParser()`)
- [ ] Delete `parseProfilesFromOsArgs()` function
- [ ] Update tests to remove os.Args parsing test cases
- [ ] Update documentation

## Related Documentation

- **Profiles Configuration:** `website/docs/core-concepts/profiles/`
- **Global Flags:** `pkg/flags/global_builder.go`
- **Configuration Loading:** `pkg/config/load.go`
- **Authentication Manager:** `pkg/auth/manager_helpers.go`
- **Nested Component Authentication:** `internal/exec/terraform_nested_auth_helper.go`
