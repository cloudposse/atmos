# Fix: Configuration Profiles Not Loading from CLI Flag

## Problem Statement

When executing Atmos commands with the `--profile` CLI flag, the specified profile configuration is not loaded and
merged with the global configuration. This causes authentication failures and missing configuration even when valid
profiles are defined. The `ATMOS_PROFILE` environment variable works correctly, but the `--profile` CLI flag does not.

## Symptoms

**Error Message:**

```
Error: No valid credential sources found
```

**Command That Should Work:**

```bash
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers
```

**With Comment in Main Config:**

```yaml
# import:
#   - auth.yaml  # ← When commented out, authentication fails
```

**Profile Configuration:**

```yaml
auth:
  providers:
    cplive-sso:
      kind: aws/iam-identity-center
      region: us-east-2
      start_url: https://cplive.awsapps.com/start/
  identities:
    core-identity/managers-team-access:
      kind: aws/permission-set
      default: true
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

## Reproduction Case

### Directory Structure

```plaintext
├── atmos.yaml                           # Main config (auth.yaml import commented out)
├── profiles/
│   └── managers/
│       └── atmos.yaml                   # Profile with auth config
└── auth.yaml                            # Global auth config (not imported)
```

### Main Configuration

```yaml
base_path: "."

# Import shared configuration
# import:
#   - auth.yaml  # ← COMMENTED OUT - should not be required when using profiles

components:
  terraform:
    base_path: "components/terraform"
    command: "tofu"

stacks:
  base_path: "stacks"
  name_pattern: "{tenant}-{environment}-{stage}"
```

### Profile Configuration

**File:** `profiles/managers/atmos.yaml`

```yaml
auth:
  logs:
    level: Info

  providers:
    cplive-saml:
      kind: aws/saml
      url: "..."
      idp_arn: "..."
      profile: ""
      region: us-east-2
    cplive-sso:
      kind: aws/iam-identity-center
      region: us-east-2
      start_url: https://cplive.awsapps.com/start/

  identities:
    core-identity/managers-team-access:
      kind: aws/permission-set
      default: true
      via:
        provider: cplive-sso
      principal:
        name: "IdentityManagersTeamAccess"
        account:
          name: "core-identity"

    core-identity/managers:
      kind: aws/assume-role
      default: false
      via:
        provider: cplive-sso
      principal:
        assume_role: "..."
```

### Test Commands

```bash
# ❌ FAILS: Profile not loaded, auth providers missing
atmos describe config --profile managers

# ❌ FAILS: No authentication available
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers

# ✅ WORKS: Environment variable loads profile correctly
ATMOS_PROFILE=managers atmos describe config

# ✅ WORKS: When auth.yaml import is uncommented
# But defeats the purpose of profiles!
```

## Root Cause Analysis

### Execution Flow

```plaintext
1. User Command
   └─ atmos describe config --profile managers

2. Flag Registration (✅ Works - flags/global_builder.go:119-127)
   └─ Global flag "--profile" is defined
   └─ Flag is registered on RootCmd
   └─ Flag is bound to viper.GetViper() (global singleton)
   └─ Environment variable ATMOS_PROFILE is bound

3. Command Execution (cmd/describe_config.go:18-40)
   └─ RunE function is called by Cobra
   └─ Cobra has parsed flags at this point
   └─ atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
       ↓
       └─ Passes EMPTY ConfigAndStacksInfo struct!
       └─ configAndStacksInfo.ProfilesFromArg = []  // ❌ ALWAYS EMPTY!

4. Profile Loading Check (pkg/config/load.go:105)
   └─ if len(configAndStacksInfo.ProfilesFromArg) > 0 {
       ├─ ❌ Condition is ALWAYS FALSE
       └─ Profile loading code NEVER executes

5. Result
   └─ Configuration loaded WITHOUT profile merging
   └─ auth.providers remains null (from global config)
   └─ Authentication fails
```

### Code Location of First Bug

**File:** `cmd/describe_config.go` (and ALL other commands)
**Line:** 31

```go
atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
//                                     ↑
//                                     └─ ❌ BUG: Empty struct, ProfilesFromArg is never set
```

**Impact:**
Every command in Atmos passes an empty `ConfigAndStacksInfo{}` to `InitCliConfig`, so the `ProfilesFromArg` field is
always empty regardless of what the user passes via `--profile` flag.

### Code Location of Second Bug (Why CLI Flag Doesn't Work)

**File:** `pkg/config/load.go`
**Line:** 105

```go
if len(configAndStacksInfo.ProfilesFromArg) > 0 {
// Load profiles...
}
// ❌ This block never executes because ProfilesFromArg is always empty!
```

**Root Cause of CLI Flag Not Working:**

Even if we were to fix the first bug by reading the profile flag value, there's a **Viper/Cobra flag binding timing
issue**:

1. Global flags are bound to Viper during initialization (root.go:816):
   ```go
   globalParser.BindToViper(viper.GetViper())
   ```

2. `viper.BindPFlag()` creates a **binding** between Viper key and Cobra flag

3. However, **flag VALUES aren't synchronized into Viper until after flag parsing**

4. When `InitCliConfig` is called in command `RunE`, the binding exists but the value is empty:
   ```
   globalViper.IsSet("profile")         → true  (binding exists)
   globalViper.GetStringSlice("profile") → []   (value not synced yet!)
   ```

5. Environment variables work because they're read directly into Viper, not via flag binding

### Why Environment Variable Works

```go
// In global_builder.go:125
EnvVars: []string{"ATMOS_PROFILE"}

// Viper automatically reads ATMOS_PROFILE env var
// and makes it available in globalViper.GetStringSlice("profile")
```

**Env var flow:**

```
ATMOS_PROFILE=managers
    ↓
Viper reads env var during initialization
    ↓
globalViper.GetStringSlice("profile") = ["managers"] ✅
```

**CLI flag flow:**

```
--profile managers
    ↓
Cobra parses flag value
    ↓
viper.BindPFlag creates binding (but value not synced immediately)
    ↓
globalViper.GetStringSlice("profile") = [] ❌
```

## Solution Implemented

### Fix Overview

Since Viper's flag binding doesn't sync values immediately, we implemented a **workaround** that:

1. **Checks Viper first** (for environment variables - works correctly)
2. **Falls back to manual os.Args parsing** (for CLI flags - workaround for Viper timing issue)

This dual approach ensures both `ATMOS_PROFILE` env var and `--profile` CLI flag work correctly.

### Implementation Details

#### Step 1: Add Profile Parsing Helper Function

**File:** `pkg/config/load.go`

Created `parseProfilesFromArgs()` function that manually parses `--profile` flags from `os.Args`:

```go
// parseProfilesFromArgs manually parses --profile flags from os.Args.
// This is a workaround for Viper's BindPFlag not syncing flag values immediately.
// Supports both --profile=value and --profile value syntax.
func parseProfilesFromArgs(args []string) []string {
	var profiles []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--profile" && i+1 < len(args) {
			// --profile value syntax.
			profiles = append(profiles, args[i+1])
			i++ // Skip next arg.
		} else if strings.HasPrefix(arg, "--profile=") {
			// --profile=value syntax.
			value := strings.TrimPrefix(arg, "--profile=")
			// Handle comma-separated values.
			for _, v := range strings.Split(value, ",") {
				if trimmed := strings.TrimSpace(v); trimmed != "" {
					profiles = append(profiles, trimmed)
				}
			}
		}
	}
	return profiles
}
```

**Features:**

- ✅ Supports `--profile value` syntax
- ✅ Supports `--profile=value` syntax
- ✅ Handles comma-separated values: `--profile=dev,staging,prod`
- ✅ Handles multiple flags: `--profile dev --profile staging`
- ✅ Strips whitespace from comma-separated values
- ✅ Filters out empty values

#### Step 2: Update Profile Loading Logic

**File:** `pkg/config/load.go`

Updated the profile loading check to use both Viper (env vars) and os.Args (CLI flags):

```go
// If profiles weren't passed via ConfigAndStacksInfo, check if they were
// specified via --profile flag or ATMOS_PROFILE env var.
// Note: Global flags are bound to viper.GetViper() (global singleton), not the local viper instance.
if len(configAndStacksInfo.ProfilesFromArg) == 0 {
	globalViper := viper.GetViper()

	// WORKAROUND: Viper's BindPFlag doesn't always sync CLI flag values immediately.
	// When using --profile flag, the value may be in os.Args but not yet in Viper.
	// Environment variables work fine (ATMOS_PROFILE).
	// Check both Viper (for env vars) and os.Args (for CLI flags).
	if globalViper.IsSet("profile") && len(globalViper.GetStringSlice("profile")) > 0 {
		// Env var path - value is in Viper.
		configAndStacksInfo.ProfilesFromArg = globalViper.GetStringSlice("profile")
		log.Debug("Profiles loaded from env var", "profiles", configAndStacksInfo.ProfilesFromArg)
	} else {
		// CLI flag path - parse os.Args manually.
		profiles := parseProfilesFromArgs(os.Args)
		if len(profiles) > 0 {
			configAndStacksInfo.ProfilesFromArg = profiles
			log.Debug("Profiles loaded from CLI flag", "profiles", profiles)
		}
	}
}
```

**Logic Flow:**

1. Check if `ProfilesFromArg` is already populated (future-proofing for when commands start passing it explicitly)
2. Get global Viper singleton (where global flags are bound)
3. **First try Viper:** If `profile` key is set AND has non-empty value → use it (env var path)
4. **Fallback to os.Args:** If Viper is empty → manually parse `os.Args` for `--profile` flags
5. Log which method was used for debugging

**Why This Works:**

- **Environment variables:** Immediately available in Viper → first branch executes
- **CLI flags:** Not in Viper yet → second branch parses os.Args directly
- **No breaking changes:** Existing code that might pass ProfilesFromArg explicitly still works

### Testing Strategy

#### Integration Tests

**Manual testing performed:**

1. **CLI flag syntax:**
   ```bash
   atmos describe config --profile managers
   # ✅ Auth providers loaded correctly
   ```

2. **Environment variable:**
   ```bash
   ATMOS_PROFILE=managers atmos describe config
   # ✅ Auth providers loaded correctly
   ```

3. **Comma-separated profiles:**
   ```bash
   atmos describe config --profile=managers,staging
   # ✅ Both profiles loaded and merged
   ```

4. **Original failing command:**
   ```bash
   atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers
   # ✅ Authentication works, terraform plan executes
   ```

5. **All existing tests pass:**
   ```bash
   go test ./pkg/config/... -run TestLoadConfig
   # ✅ All tests pass
   ```

## Success Criteria

All success criteria met:

1. ✅ `--profile` CLI flag loads profile configuration and merges with global config
2. ✅ `ATMOS_PROFILE` environment variable continues to work
3. ✅ `atmos describe config --profile managers` shows merged auth providers and identities
4. ✅ `atmos terraform plan --profile managers` authenticates successfully
5. ✅ Comma-separated profiles work: `--profile=dev,staging,prod`
6. ✅ Multiple profile flags work: `--profile dev --profile staging`
7. ✅ All existing tests continue to pass
8. ✅ New tests provide comprehensive coverage of flag parsing

## Verification Output

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
      "cplive-saml": {
        "kind": "aws/saml",
        "url": "",
        "region": "us-east-2"
      },
      "cplive-sso": {
        "kind": "aws/iam-identity-center",
        "start_url": "https://cplive.awsapps.com/start/",
        "region": "us-east-2"
      }
    },
    "identities": {          # ✅ Identities from managers profile
      "core-identity/managers": {
        "kind": "aws/assume-role",
        "via": {
          "provider": "cplive-sso"
        },
        "principal": {
          "assume_role": ""
        }
      },
      "core-identity/managers-team-access": {
        "kind": "aws/permission-set",
        "default": true,
        ...
      }
    }
}
```

## Files Modified

1. **`pkg/config/load.go`**

- Added `parseProfilesFromArgs()` helper function
- Updated profile loading logic to check both Viper and os.Args

2. **`pkg/config/load_profile_test.go`** (NEW)

- Created comprehensive test suite with 9 test cases
- Tests all flag syntax variations

**Total changes:** 2 files modified/added, ~80 lines of code added

## Why This Workaround is Necessary

### Viper Flag Binding Timing Issue

Viper's `BindPFlag()` creates a **binding** between a Viper key and a Cobra pflag, but the synchronization of values has
timing considerations:

1. **During initialization (root.go:816):**
   ```go
   globalParser.BindToViper(viper.GetViper())
   ```

- Creates binding between "profile" key and --profile flag
- Binding exists but no value yet

2. **When command runs (cmd/describe_config.go:31):**
   ```go
   atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
   ```

- Cobra has parsed flags
- Flag value exists in Cobra's FlagSet
- BUT: Viper hasn't synchronized the value yet
- `globalViper.IsSet("profile")` returns `true` (binding exists)
- `globalViper.GetStringSlice("profile")` returns `[]` (value not synced)

3. **Environment variables work differently:**

- Viper reads env vars directly, not through flag binding
- `ATMOS_PROFILE` is immediately available in Viper
- No synchronization delay

### Alternative Solutions Considered

#### Option 1: Call Viper's ReadInConfig Again

**Rejected:** Would re-read entire config, potentially causing side effects

#### Option 2: Explicitly Sync Flags to Viper

**Rejected:** No official Viper API to force flag value synchronization

#### Option 3: Read Flag from Cobra's FlagSet Directly

**Rejected:** Would require passing `cmd *cobra.Command` to InitCliConfig, changing many function signatures

#### Option 4: Parse os.Args Manually (CHOSEN)

**Accepted:**

- ✅ Minimal code changes
- ✅ No function signature changes
- ✅ Works reliably for all flag syntax variations
- ✅ Easy to test
- ✅ No side effects

## Future Improvements

### Proper Long-Term Fix

The workaround can be replaced with a proper fix once Cobra/Viper timing is better understood:

1. **Option A: Pass cmd to InitCliConfig**
   ```go
   func InitCliConfig(cmd *cobra.Command, configAndStacksInfo *schema.ConfigAndStacksInfo) {
   	if cmd != nil {
   		profile, _ := cmd.Flags().GetStringSlice("profile")
   		configAndStacksInfo.ProfilesFromArg = profile
   	}
   	// ... rest of function
   }
   ```

2. **Option B: Explicit Viper sync method**
   ```go
   // If Viper adds this capability
   viper.GetViper().SyncFlags(cmd.Flags())
   ```

3. **Option C: Commands populate ProfilesFromArg**
   ```go
   // In each command's RunE:
   profile, _ := cmd.Flags().GetStringSlice("profile")
   info := schema.ConfigAndStacksInfo{
   	ProfilesFromArg: profile,
   }
   atmosConfig, err := cfg.InitCliConfig(info, false)
   ```

### Why We Don't Need The Long-Term Fix Yet

The current workaround:

- ✅ Works reliably for all use cases
- ✅ Well-tested
- ✅ No performance impact (os.Args is tiny)
- ✅ Self-contained in one function
- ✅ Easy to replace later if needed

## Related Issue: Identity Flag Not Propagating to Nested Components

### Problem Statement (Follow-up Issue)

After implementing the profile loading fix, a related issue was discovered: when using `--profile` and `--identity`
flags together, the identity selector still appeared during nested component operations (such as `!terraform.state` YAML
functions).

**Command:**

```bash
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers --identity core-identity/managers-team-access
```

**Expected Behavior:**

- Use the explicitly specified identity (`core-identity/managers-team-access`)
- No identity selector should appear

**Actual Behavior:**

- Main component authenticated correctly with specified identity
- Identity selector appeared when processing `!terraform.state` YAML function for nested component (`vpc`)
- Error message: "Multiple default identities found. Please choose one"

### Root Cause Analysis

**Execution Flow:**

1. **Main component authentication** ✅ Works correctly:
   ```
   identity = "core-identity/managers-team-access"
   CreateAndAuthenticateManager(identity, mergedAuthConfig, "__SELECT__")
   ```

2. **Nested component authentication** ❌ Fails:
   ```
   YAML function: !terraform.state vpc vpc_id
   └─ Calls resolveAuthManagerForNestedComponent()
      └─ Calls createComponentAuthManager()
         └─ CreateAndAuthenticateManager("", mergedAuthConfig, "__NO_SELECT__")
            ↑
            └─ Empty identity triggers auto-detection
               └─ Finds multiple defaults in merged profile config
                  └─ Shows selector prompt ❌
   ```

**Code Location:**

**File:** `internal/exec/terraform_nested_auth_helper.go`

```go
componentAuthManager, err := auth.CreateAndAuthenticateManager(
	"",               // ❌ Empty - triggers auto-detection
	mergedAuthConfig, // Contains multiple defaults from profile
	"__NO_SELECT__",
)
```

**Why This Happened:**

When YAML template functions (like `!terraform.state`) need to fetch state from other components, they create
component-specific AuthManagers. The original implementation:

- Did NOT inherit the user's explicitly specified identity
- Always passed empty string for identity
- Relied on auto-detection from component's auth config
- With profiles containing multiple defaults, auto-detection triggered the selector

### Solution Implemented

**Inherit authenticated identity from parent AuthManager to nested components.**

#### Step 1: Extract Identity from Parent AuthManager

**File:** `internal/exec/terraform_nested_auth_helper.go`

```go
// Determine identity to use for component authentication.
// If parent AuthManager exists and is authenticated, inherit its identity.
// This ensures that when user explicitly specifies --identity flag, it propagates to nested components.
var identityName string
if parentAuthManager != nil {
	chain := parentAuthManager.GetChain()
	if len(chain) > 0 {
		// Last element in chain is the authenticated identity.
		identityName = chain[len(chain)-1]
		log.Debug("Inheriting identity from parent AuthManager for component",
			logKeyComponent, component,
			logKeyStack, stack,
			"inheritedIdentity", identityName,
			"chain", chain,
		)
	}
}
```

**Key points:**

- `GetChain()` returns authentication chain: `[providerName, identity1, identity2, ..., targetIdentity]`
- Last element is the authenticated identity
- Extract and use for nested component authentication

#### Step 2: Use Inherited Identity for Component AuthManager

**File:** `internal/exec/terraform_nested_auth_helper.go`

```go
// Create and authenticate new AuthManager with merged config.
// Use inherited identity from parent, or empty string to auto-detect from component's defaults.
componentAuthManager, err := auth.CreateAndAuthenticateManager(
	identityName,     // Inherited from parent, or empty to trigger auto-detection
	mergedAuthConfig, // Merged component + global auth
	cfg.IdentityFlagSelectValue,
)
```

**Behavior:**

- If parent AuthManager exists → inherit its authenticated identity
- If no parent AuthManager → auto-detect from component's defaults (original behavior)
- User's `--identity` choice now propagates to all nested operations

### Testing Strategy

#### Before Fix

```bash
# Command with both --profile and --identity
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers --identity core-identity/managers-team-access

# Output:
# ✅ Main component authenticates with core-identity/managers-team-access
# ❌ When processing !terraform.state vpc vpc_id:
#    ┃ Multiple default identities found. Please choose one:
#    ┃ Press ctrl+c or esc to exit
#    ┃ > core-identity/managers
#    ┃   core-identity/managers-team-access
```

#### After Fix

```bash
# Same command
atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers --identity core-identity/managers-team-access

# Debug output shows identity inheritance:
DEBU  Creating AuthManager with identity identity=core-identity/managers-team-access
DEBU  Inheriting identity from parent AuthManager for component component=vpc inheritedIdentity=core-identity/managers-team-access
DEBU  CreateAndAuthenticateManager called identityName=core-identity/managers-team-access

# ✅ No selector prompt
# ✅ Terraform plan executes successfully
# ✅ All nested component operations use the same identity
```

### Verification

**Manual testing performed:**

1. **With --identity flag:**
   ```bash
   atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers --identity core-identity/managers-team-access
   # ✅ No selector, uses specified identity for all operations
   ```

2. **Without --identity flag (multiple defaults):**
   ```bash
   atmos terraform plan runs-on/cloudposse -s core-ue2-auto --profile managers
   # ✅ Shows selector once for main component
   # ✅ Nested components inherit the selected identity
   ```

3. **Without profile (single default):**
   ```bash
   atmos terraform plan runs-on/cloudposse -s core-ue2-auto
   # ✅ Auto-detects default identity, no selector
   ```

### Success Criteria

All success criteria met:

1. ✅ `--identity` flag propagates to nested component operations
2. ✅ No identity selector appears when identity is explicitly specified
3. ✅ YAML functions (`!terraform.state`, `!terraform.output`) use inherited identity
4. ✅ User's identity choice is consistent throughout entire command execution
5. ✅ Backward compatibility maintained (auto-detection still works when no parent exists)
6. ✅ Existing tests continue to pass

### Files Modified

1. **`internal/exec/terraform_nested_auth_helper.go`**
  - Updated `createComponentAuthManager()` to extract and inherit identity from parent AuthManager
  - Added debug logging for identity inheritance

**Total changes:** 1 file modified, ~15 lines of code added

### Impact

**Benefits:**

- Consistent authentication across main and nested operations
- User's explicit `--identity` choice is respected everywhere
- Reduces confusion and improves user experience
- No breaking changes to existing functionality

**Affected Operations:**

- `!terraform.state` YAML functions
- `!terraform.output` YAML functions
- Any nested component that creates component-specific AuthManager

## Related Documentation

- **Profiles Configuration:** `website/docs/core-concepts/profiles/`
- **Global Flags:** `pkg/flags/global_builder.go`
- **Configuration Loading:** `pkg/config/load.go`
- **Authentication Manager:** `pkg/auth/manager_helpers.go`
- **Nested Component Authentication:** `internal/exec/terraform_nested_auth_helper.go`
