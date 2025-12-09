# Identity Flag Behavior and Default Identity Resolution

## Executive Summary

This document defines the behavior of the `--identity` flag across all Atmos commands and how default identity resolution works. It clarifies the interaction between explicit flag usage, environment variables, default identities, and the authentication system.

## Problem Statement

### Background

A critical regression was reported in Atmos v1.196.0 where `atmos terraform plan` would fail in CI environments with the error "interactive identity selection requires a TTY" even when:
- No authentication was configured
- No `--identity` flag was provided
- No identity-related environment variables were set

The root cause was twofold:
1. **Viper global state pollution**: `viper.BindPFlag()` in `cmd/auth_shell.go` created two-way binding that persisted the `__SELECT__` value across commands
2. **Incorrect fallback logic**: Code would read from viper when the flag wasn't provided, getting polluted values from previous commands

### Success Criteria

- ✅ Default identity authentication works when no `--identity` flag provided
- ✅ Interactive identity selection works in TTY when `--identity` flag used without value
- ✅ Fast failure with clear error in CI when interactive selection attempted
- ✅ Graceful handling when no identities are configured
- ✅ No viper global state pollution between commands
- ✅ Explicit identity selection works when `--identity=name` provided

## Core Requirements

### FR-001: Identity Flag Behavior Matrix

The `--identity` flag has four distinct modes of operation:

#### Mode 1: Flag Not Provided (Default Identity)

**User Command:**
```bash
atmos terraform plan component --stack mystack
# No --identity flag provided
```

**Behavior:**
1. `ProcessCommandLineArgs()` checks for `ATMOS_IDENTITY` environment variable
2. If env var present: `info.Identity = $ATMOS_IDENTITY`
3. If env var absent: `info.Identity = ""`
4. Later, `auth.TerraformPreHook()` is called
5. If `info.Identity` is empty, `authManager.GetDefaultIdentity(false)` retrieves default identity from configuration
6. If default identity configured: authentication proceeds automatically
7. If no default identity: error with message "Use the identity flag or specify an identity as default"

**Key Point:** Default identity authentication is automatic and does NOT require the `--identity` flag.

**Acceptance Criteria:**
- ✅ When no `--identity` flag and default identity configured → Use default identity
- ✅ When `ATMOS_IDENTITY=my-id` set → Use that identity (env var precedence)
- ✅ When no flag, no env var, no default → Error: "no default identity"
- ✅ Authentication happens transparently in `TerraformPreHook`

#### Mode 2: Flag Without Value (Interactive Selection)

**User Command:**
```bash
atmos terraform plan component --stack mystack --identity
# OR
atmos terraform plan component --stack mystack -i
```

**Behavior:**
1. Cobra's `NoOptDefVal` mechanism sets flag value to `__SELECT__`
2. `flags.Changed("identity")` returns `true`
3. Code enters identity flag handling block
4. Checks if TTY available:
   - **TTY available + identities configured:** Show interactive identity selector
   - **No TTY (CI):** Error: "interactive identity selection requires a TTY"
   - **TTY available + no identities:** Debug log: "Identity selection skipped: no authentication configured"

**Key Point:** Interactive selection is ONLY triggered when flag explicitly provided without value.

**Acceptance Criteria:**
- ✅ `--identity` + TTY + identities → Interactive selector
- ✅ `--identity` + no TTY → Fail fast with clear error
- ✅ `--identity` + TTY + no identities → Skip with debug log (no error)
- ✅ `--identity` without value NEVER reads from viper global state

#### Mode 3: Flag With Explicit Value

**User Command:**
```bash
atmos terraform plan component --stack mystack --identity=prod-admin
# OR
atmos terraform plan component --stack mystack -i prod-admin
```

**Behavior:**
1. `flags.Changed("identity")` returns `true`
2. `flags.GetString("identity")` returns `"prod-admin"`
3. `info.Identity = "prod-admin"`
4. Later, `auth.TerraformPreHook()` authenticates with `prod-admin`
5. If identity doesn't exist: Error from auth manager: "identity not found: prod-admin"

**Key Point:** Explicit identity overrides default identity and environment variables.

**Acceptance Criteria:**
- ✅ `--identity=my-id` + identity exists → Use that identity
- ✅ `--identity=my-id` + identity doesn't exist → Error: "identity not found"
- ✅ Explicit identity takes precedence over `ATMOS_IDENTITY` env var
- ✅ Explicit identity takes precedence over default identity

#### Mode 4: Flag With Disable Value (NEW)

**User Command:**
```bash
atmos terraform plan component --stack mystack --identity=false
# OR
atmos terraform plan component --stack mystack --identity=0
# OR via environment variable
export ATMOS_IDENTITY=false
atmos terraform plan component --stack mystack
```

**Behavior:**
1. `flags.Changed("identity")` returns `true` (if via flag)
2. `normalizeIdentityValue()` converts `"false"` → `"__DISABLED__"` sentinel
3. `info.Identity = "__DISABLED__"`
4. Later, `auth.TerraformPreHook()` checks `isAuthenticationDisabled(stackInfo.Identity)`
5. If disabled: Returns early, skipping all authentication
6. Cloud provider SDK uses default credential resolution (e.g., for AWS: env vars, shared credentials, IMDS, OIDC)

**Key Point:** Explicit disable overrides all identity configurations, skipping Atmos authentication entirely.

**Accepted Boolean False Representations:**
- `false`, `False`, `FALSE` (case-insensitive)
- `0` (zero string)
- `no`, `No`, `NO` (case-insensitive)
- `off`, `Off`, `OFF` (case-insensitive)

**Acceptance Criteria:**
- ✅ `--identity=false` → Skip authentication, use cloud provider SDK defaults
- ✅ `ATMOS_IDENTITY=false` → Skip authentication
- ✅ Works with identities configured in `atmos.yaml`
- ✅ Works when no identities configured
- ✅ Disable flag takes precedence over default identity
- ✅ Disable flag takes precedence over `ATMOS_IDENTITY` env var (if both set)

### FR-002: Precedence Order

Identity resolution follows this precedence (highest to lowest):

1. **Explicit `--identity=false` flag** (disables authentication)
2. **Explicit `--identity` flag with value** (e.g., `--identity=prod-admin`)
3. **Interactive selection** (when `--identity` flag used without value in TTY)
4. **`ATMOS_IDENTITY=false` environment variable** (disables authentication)
5. **`ATMOS_IDENTITY` environment variable** (with identity name)
6. **Default identity from configuration** (atmos.yaml or component config)
7. **Error if none of above** ("no default identity found")

**Acceptance Criteria:**
- ✅ Flag with `false` disables authentication (highest priority)
- ✅ Flag with value overrides all other sources
- ✅ Environment variable with `false` disables authentication
- ✅ Environment variable with name overrides default from config
- ✅ Default identity used only when no other source specified
- ✅ Clear error when no identity source available

### FR-003: Authentication Flow

Authentication happens in two places depending on the scenario:

#### Scenario A: Explicit Flag Handling (terraform_utils.go)

**When:** User provides `--identity` flag (with or without value)

**Location:** `cmd/terraform_utils.go` lines 78-127

**Flow:**
```go
if flags.Changed("identity") {
    identityFlag := flags.GetString("identity")

    if identityFlag == "__SELECT__" {
        // Interactive selection requested
        if !TTY {
            return Error("requires TTY")
        }
        if len(atmosConfig.Auth.Identities) == 0 {
            log.Debug("no identities, skipping")
        } else {
            info.Identity = showInteractiveSelector()
        }
    } else {
        // Explicit value provided
        info.Identity = identityFlag
    }
}
// If flag not provided, info.Identity already set by ProcessCommandLineArgs
```

#### Scenario B: Default Identity Authentication (TerraformPreHook)

**When:** No `--identity` flag provided OR flag provided with explicit value

**Location:** `pkg/auth/hooks.go` `TerraformPreHook()`

**Flow:**
```go
func TerraformPreHook(atmosConfig, stackInfo) error {
    // Skip if no auth configured
    if len(authConfig.Providers) == 0 && len(authConfig.Identities) == 0 {
        return nil // No error
    }

    // Resolve identity
    targetIdentity := resolveTargetIdentityName(stackInfo, authManager)

    // Authenticate
    credentials := authManager.Authenticate(ctx, targetIdentity)

    // Write to environment
    stackInfo.ComponentEnvSection = credentials.ToEnv()

    return nil
}

func resolveTargetIdentityName(stackInfo, authManager) string {
    // If identity already set (from flag or env var)
    if stackInfo.Identity != "" {
        return stackInfo.Identity
    }

    // Get default identity from config
    return authManager.GetDefaultIdentity(false) // false = no interactive
}
```

**Acceptance Criteria:**
- ✅ `TerraformPreHook` always called for terraform commands
- ✅ Hook skips gracefully when no auth configured
- ✅ Hook uses `stackInfo.Identity` if set (from flag/env)
- ✅ Hook falls back to default identity if `stackInfo.Identity` empty
- ✅ Hook never prompts interactively (non-interactive context)

### FR-004: No Viper Global State Pollution

**Problem:** `viper.BindPFlag()` creates two-way binding where flag values sync to viper global state, causing pollution across commands.

**Solution:**

1. **Identity flag defined ONCE** as `PersistentFlags` in parent `authCmd` (cmd/auth.go:27)
2. **Child commands inherit** identity flag (auth shell, auth exec) - no redefinition
3. **No `viper.BindPFlag()`** for identity flag - use `viper.BindEnv()` for environment variables only
4. **Flag reading** via `flags.Changed()` and `flags.GetString()` - NEVER via `viper.GetString()`

**Code Changes:**

**cmd/auth.go:**
```go
// Define identity flag ONCE as PersistentFlags
authCmd.PersistentFlags().StringP("identity", "i", "", "Specify the target identity")
identityFlag.NoOptDefVal = "__SELECT__"

// BindEnv for environment variables (one-way binding)
viper.BindEnv("identity", "ATMOS_IDENTITY", "IDENTITY")
```

**cmd/auth_shell.go and cmd/auth_exec.go:**
```go
func init() {
    // NOTE: --identity flag inherited from parent authCmd (PersistentFlags).
    // DO NOT redefine here - that creates duplicate local flag that shadows parent.

    // DO NOT use viper.BindPFlag() - causes global state pollution

    authCmd.AddCommand(authShellCmd)
}
```

**cmd/terraform_utils.go:**
```go
// Only check if flag explicitly provided - don't read from viper
if flags.Changed("identity") {
    identityFlag := flags.GetString("identity")
    // Handle flag value...
}
// If flag not provided, info.Identity already set by ProcessCommandLineArgs
```

**Acceptance Criteria:**
- ✅ Identity flag defined once in parent command
- ✅ No duplicate flag definitions in child commands
- ✅ No `viper.BindPFlag()` anywhere for identity flag
- ✅ Use `viper.BindEnv()` for environment variable binding only
- ✅ Flag reading via Cobra APIs, not viper
- ✅ No viper pollution between command executions

### FR-005: Error Messages

Clear, actionable error messages for each failure scenario:

| Scenario | Error Message | Exit Code |
|----------|--------------|-----------|
| `--identity` in CI (no TTY) | `interactive identity selection requires a TTY` | 1 |
| Identity not found | `identity not found: <name>` | 1 |
| No default identity | `no default identity found. Use the identity flag or specify an identity as default.` | 1 |
| Authentication failed | `failed to authenticate with identity "<name>": <reason>` | 1 |

**Acceptance Criteria:**
- ✅ All error messages include context (which identity, why it failed)
- ✅ TTY error clearly states interactive selection requires TTY
- ✅ Missing identity error suggests using `--identity` flag or setting default
- ✅ Authentication errors include underlying provider error details

## Non-Functional Requirements

### NFR-001: Performance

- Identity flag parsing adds < 1ms overhead
- No performance impact when flag not provided
- Viper state checks avoided in hot path

### NFR-002: Backward Compatibility

- Existing behavior preserved for all valid use cases
- Environment variable precedence unchanged
- Default identity behavior unchanged
- Breaking change: Invalid viper pollution scenarios now work correctly (this is a bug fix)

### NFR-003: Testing

All scenarios covered by comprehensive tests:

**Test Files:**
- `cmd/viper_bindings_test.go` - Tests viper BindPFlag pollution
- `cmd/viper_identity_flag_test.go` - Tests identity flag resolution via viper
- `cmd/terraform_identity_flag_test.go` - Tests terraform identity flag behavior
- `cmd/cobra_flag_defaults_test.go` - Tests Cobra NoOptDefVal behavior
- `cmd/auth_exec_test.go` - Tests auth exec command flag inheritance
- `cmd/auth_shell_test.go` - Tests auth shell command flag inheritance

**Test Coverage:**
- ✅ No flag + default identity configured
- ✅ No flag + no default identity → error
- ✅ `--identity` + TTY + identities → interactive selector
- ✅ `--identity` + no TTY → error
- ✅ `--identity` + no identities → skip with debug log
- ✅ `--identity=value` + identity exists → use that identity
- ✅ `--identity=value` + identity missing → error
- ✅ `ATMOS_IDENTITY` env var precedence
- ✅ Flag precedence over env var
- ✅ Viper pollution scenarios (proof bug is fixed)
- ✅ Flag inheritance from parent command

## Implementation Summary

### Files Modified

1. **cmd/terraform_utils.go** (lines 75-128)
   - Only process `--identity` flag when explicitly provided
   - Check TTY before interactive selection
   - Skip selection with debug log when no identities configured
   - Never read from viper global state

2. **cmd/auth_shell.go** (init function)
   - Remove duplicate identity flag definition
   - Remove `viper.BindPFlag()` call
   - Add comment explaining inheritance from parent

3. **cmd/auth_exec.go** (init function)
   - Remove duplicate identity flag definition
   - Add comment explaining inheritance from parent

4. **cmd/auth.go** (existing)
   - Identity flag already correctly defined as PersistentFlags
   - Already uses `viper.BindEnv()` (not BindPFlag)
   - No changes needed

### Test Files Created

1. **cmd/viper_bindings_test.go** - Proves BindPFlag pollution mechanism
2. **cmd/viper_identity_flag_test.go** - Tests viper scenarios
3. **cmd/terraform_identity_flag_test.go** - Tests terraform command behavior
4. **cmd/cobra_flag_defaults_test.go** - Tests Cobra NoOptDefVal

### Test Files Updated

1. **cmd/auth_exec_test.go** - Updated to check inherited flag
2. **cmd/auth_shell_test.go** - Updated to check inherited flag

## Decision Log

### Decision 1: Where Authentication Happens

**Decision:** Keep authentication in `TerraformPreHook()`, not in `terraform_utils.go`

**Rationale:**
- Separation of concerns: terraform_utils handles CLI flag parsing, hooks handle authentication
- Consistent location: all terraform commands use the same hook
- Default identity works automatically without flag handling
- Interactive selection is the ONLY case that needs special handling

### Decision 2: Viper Usage

**Decision:** Never use `viper.BindPFlag()` for identity flag, only use `viper.BindEnv()`

**Rationale:**
- `BindPFlag()` creates two-way binding that pollutes global state
- `BindEnv()` is one-way (env → viper) which is safe
- Flag reading should use Cobra APIs (`flags.Changed()`, `flags.GetString()`)
- Viper should only be used for environment variable resolution

### Decision 3: Flag Inheritance

**Decision:** Define identity flag once in parent command, children inherit via PersistentFlags

**Rationale:**
- Single source of truth prevents duplicate definitions
- Cobra PersistentFlags designed for this use case
- No need to redefine or re-bind in child commands
- Simpler code, less opportunity for bugs

### Decision 4: Interactive Selection Guard

**Decision:** Skip interactive selection with debug log (not error) when no identities configured

**Rationale:**
- User explicitly requested selection by providing `--identity` flag
- No identities means nothing to select - this is not an error condition
- Debug log provides visibility for troubleshooting
- Allows commands to continue without failing

## Future Enhancements

1. **Multiple Identity Support**: Support `--identity` flag multiple times for concurrent provider credentials (AWS + GitHub + Azure)
2. **Identity Aliases**: Support short aliases for commonly-used identities
3. **Identity Validation**: Pre-flight validation that identity exists before showing selector
4. **Better Error Context**: Include stack and component name in identity resolution errors

## References

- [Atmos Auth PRD](./PRD/PRD-Atmos-Auth.md)
- [Auth Context and Multi-Identity Support](../../../docs/prd/auth-context-multi-identity.md)
- [Command Registry Pattern](../../../docs/prd/command-registry-pattern.md)
