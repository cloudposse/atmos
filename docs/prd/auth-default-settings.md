# Auth Default Settings PRD

## Overview

This PRD introduces `auth.defaults` configuration for managing global authentication defaults, including a deterministic selected default identity. This solves the "multiple defaults" problem in non-interactive (CI) environments and provides clear semantics for identity selection.

## Historical Context: Why `identity.default: true` Exists

The `identity.default: true` field was originally designed to mark **provider-specific default identities**. The use case:

> "You might have a default identity based on a given provider."

**Example - Multiple Providers, Multiple Defaults:**
```yaml
auth:
  providers:
    aws-sso-dev:
      kind: aws/sso
      region: us-east-2
    aws-sso-prod:
      kind: aws/sso
      region: us-east-1
    github-oidc:
      kind: github/oidc

  identities:
    dev-default:
      via:
        provider: aws-sso-dev
      default: true  # Default for aws-sso-dev provider

    prod-default:
      via:
        provider: aws-sso-prod
      default: true  # Default for aws-sso-prod provider

    ci-default:
      via:
        provider: github-oidc
      default: true  # Default for github-oidc provider
```text

**The Problem:**
- **Interactive (TTY)**: User selects from 3 defaults (works as intended - provider-based favorites)
- **Non-interactive (CI)**: Error - "multiple default identities" (breaks)

**Current Behavior:**
`identity.default: true` acts as **favorites/shortcuts**, not truly provider-scoped defaults.

## Problem Statement

### Current State

Atmos has two concepts of "default" for identities:

1. **Identity-level `default: true`** - Marks identities as "favorites" (multiple allowed, originally intended for provider-specific defaults)
2. **No global selected default** - Must use `--identity` flag or `ATMOS_IDENTITY` env var to explicitly select

### Gaps

**Gap 1: No Deterministic Default Selection**
- Multiple `identity.default: true` → Interactive selection or error
- CI environments require deterministic behavior (no TTY)
- No way to say "use THIS identity by default"

**Gap 2: Ambiguous Semantics with Profiles**
When a profile sets `default: true` on an identity:
- Is this a profile-scoped selected default?
- Or does it add to the global favorites list?
- Multiple profiles with defaults → Which one wins?

**Gap 3: No Global Auth Defaults**
No mechanism exists for:
- Setting a **single selected default identity** globally
- Configuring global session duration defaults
- Setting global console session defaults
- Configuring global keyring defaults

## Proposed Solution

Add a new `auth.defaults` or `auth.settings` top-level configuration for global auth defaults.

### Option 1: `auth.defaults` (Recommended)

```yaml
auth:
  # NEW: Global defaults for auth behavior
  defaults:
    identity: github-oidc-identity  # Selected default (overrides identity.default: true)
    session:
      duration: "12h"               # Default session duration for all identities
    console:
      session_duration: "8h"        # Default console session duration
    keyring:
      type: "system"                # Default keyring type

  # Existing structure
  providers:
    github-oidc-provider:
      kind: github/oidc
      region: us-east-1

  identities:
    github-oidc-identity:
      kind: aws/assume-role
      default: true  # Still marks as "favorite", but auth.defaults.identity wins
      via:
        provider: github-oidc-provider
      principal:
        assume_role: "arn:aws:iam::123456789012:role/GitHubActionsDeployRole"
```text

**Precedence:**

```text
1. --identity=explicit-name      (CLI flag with value)
2. ATMOS_IDENTITY env var        (environment variable)
3. auth.defaults.identity        (global selected default) ← NEW
4. identity.default: true        (favorites - interactive selection or error)
5. Error: no default identity
```text

### Option 2: `auth.settings` (Alternative)

```yaml
auth:
  # NEW: Global settings for auth behavior
  settings:
    session_duration: "12h"
    console_session_duration: "8h"
    keyring_type: "system"

  providers:
    # ... same as above

  identities:
    github-oidc-identity:
      default: true  # Current implementation uses identity-level default
    # ... same as above
```text

**Note:** Less structured than Option 1, but flatter hierarchy. Default identity selection uses the existing `identity.default: true` field.

## Detailed Design (Option 1 - Recommended)

### Schema Changes

```go
// pkg/schema/schema_auth.go

// AuthConfig defines the authentication configuration structure.
type AuthConfig struct {
    Logs            Logs                `yaml:"logs,omitempty" json:"logs,omitempty" mapstructure:"logs"`
    Keyring         KeyringConfig       `yaml:"keyring,omitempty" json:"keyring,omitempty" mapstructure:"keyring"`
    Defaults        *AuthDefaults       `yaml:"defaults,omitempty" json:"defaults,omitempty" mapstructure:"defaults"` // NEW
    Providers       map[string]Provider `yaml:"providers" json:"providers" mapstructure:"providers"`
    Identities      map[string]Identity `yaml:"identities" json:"identities" mapstructure:"identities"`
    IdentityCaseMap map[string]string   `yaml:"-" json:"-" mapstructure:"-"`
}

// AuthDefaults defines global defaults for auth behavior.
type AuthDefaults struct {
    Identity string          `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"` // Selected default identity name
    Session  *SessionConfig  `yaml:"session,omitempty" json:"session,omitempty" mapstructure:"session"`     // Default session config
    Console  *ConsoleConfig  `yaml:"console,omitempty" json:"console,omitempty" mapstructure:"console"`     // Default console config
    Keyring  *KeyringConfig  `yaml:"keyring,omitempty" json:"keyring,omitempty" mapstructure:"keyring"`     // Default keyring config
}
```text

### Implementation Changes

```go
// pkg/auth/manager.go

// GetDefaultIdentity returns the name of the default identity, if any.
func (m *manager) GetDefaultIdentity(forceSelect bool) (string, error) {
    defer perf.Track(nil, "auth.Manager.GetDefaultIdentity")()

    // If forceSelect is true, user explicitly requested identity selection.
    if forceSelect {
        if !isInteractive() {
            return "", errUtils.ErrIdentitySelectionRequiresTTY
        }
        return m.promptForIdentity("Select an identity:", m.ListIdentities())
    }

    // NEW: Check auth.defaults.identity first (global selected default).
    if m.config.Defaults != nil && m.config.Defaults.Identity != "" {
        selectedDefault := m.config.Defaults.Identity

        // Validate the identity exists.
        if _, exists := m.config.Identities[selectedDefault]; !exists {
            return "", fmt.Errorf("%w: auth.defaults.identity '%s' not found",
                errUtils.ErrDefaultIdentity, selectedDefault)
        }

        return selectedDefault, nil
    }

    // Existing logic: Find all identities with default: true.
    var defaultIdentities []string
    for name, identity := range m.config.Identities {
        if identity.Default {
            defaultIdentities = append(defaultIdentities, name)
        }
    }

    // Handle different scenarios based on number of default identities found.
    switch len(defaultIdentities) {
    case 0:
        if !isInteractive() {
            return "", errUtils.ErrNoDefaultIdentity
        }
        return m.promptForIdentity("No default identity configured. Please choose an identity:", m.ListIdentities())
    case 1:
        return defaultIdentities[0], nil
    default:
        // Multiple defaults found.
        if !isInteractive() {
            return "", fmt.Errorf(errFormatWithString, errUtils.ErrMultipleDefaultIdentities,
                fmt.Sprintf(backtickedFmt, defaultIdentities))
        }
        return m.promptForIdentity("Multiple default identities found. Please choose one:", defaultIdentities)
    }
}
```text

### Profile Usage Examples

#### CI Profile with Selected Default

```yaml
# profiles/ci/auth.yaml
auth:
  defaults:
    identity: github-oidc-identity  # Explicit selection for CI (non-interactive safe)

  identities:
    github-oidc-identity:
      kind: aws/assume-role
      via:
        provider: github-oidc-provider
      principal:
        assume_role: "arn:aws:iam::123456789012:role/GitHubActionsDeployRole"
        role_session_name: '{{ env "GITHUB_RUN_ID" }}'

  providers:
    github-oidc-provider:
      kind: github/oidc
      region: us-east-1
```text

**Usage:**
```bash
# In CI - uses github-oidc-identity automatically (no TTY needed)
ATMOS_PROFILE=ci atmos terraform apply component -s prod

# Override with explicit identity
ATMOS_PROFILE=ci atmos terraform apply component -s prod --identity different-identity
```text

#### Base Config with Favorites

```yaml
# atmos.yaml
auth:
  # No auth.defaults.identity - use favorites pattern

  identities:
    developer-sandbox:
      kind: aws/permission-set
      default: true  # Favorite
      via:
        provider: aws-sso-dev
      principal:
        account_id: "999888777666"
        permission_set: DeveloperAccess

    developer-prod:
      kind: aws/permission-set
      default: true  # Favorite
      via:
        provider: aws-sso-prod
      principal:
        account_id: "123456789012"
        permission_set: ReadOnlyAccess
```text

**Behavior:**
- **Interactive (TTY)**: User chooses from `developer-sandbox` and `developer-prod`
- **Non-interactive (CI)**: Error - multiple defaults without explicit selection

#### Developer Profile Overrides Favorites

```yaml
# profiles/developer/auth.yaml
auth:
  defaults:
    identity: developer-sandbox  # Overrides base config favorites
```text

**Behavior:**
- When `--profile developer` active: Uses `developer-sandbox` automatically
- Without profile: Uses favorites pattern (interactive selection)

#### Global Session Defaults

```yaml
# atmos.yaml
auth:
  defaults:
    identity: default-identity
    session:
      duration: "8h"  # Default for all identities
    console:
      session_duration: "4h"  # Default console session

  identities:
    short-lived:
      kind: aws/permission-set
      session:
        duration: "1h"  # Override global default for this identity
      # ... identity config

    long-lived:
      kind: aws/permission-set
      # Uses global default: 8h
      # ... identity config
```text

**Behavior:**
- `short-lived` identity uses 1h session (identity-level override)
- `long-lived` identity uses 8h session (from auth.defaults.session)

## Disambiguation: Two Types of Defaults

This proposal introduces **two distinct concepts** of defaults, each serving a different purpose:

### 1. `auth.defaults.identity` - Global Selected Default (NEW)

**Purpose:** Single, deterministic identity selection
**Cardinality:** One (singular)
**Use Case:** "Always use THIS identity by default"
**Behavior:** Automatic selection (no user interaction)

```yaml
auth:
  defaults:
    identity: my-chosen-identity  # THE selected default (singular)
```text

**Characteristics:**
- **Deterministic** - Always returns the same identity
- **Non-interactive safe** - Works in CI (no TTY needed)
- **Override semantics** - Wins over `identity.default: true`
- **Profile-friendly** - Each profile can set its own selected default

### 2. `identity.default: true` - Identity-Level Favorites (EXISTING)

**Purpose:** Mark identities as favorites/shortcuts (originally: provider-specific defaults)
**Cardinality:** Many (multiple allowed)
**Use Case:** "These are my frequently-used identities"
**Behavior:** Interactive selection or error if multiple in non-interactive mode

```yaml
auth:
  identities:
    identity-a:
      default: true  # Favorite #1
    identity-b:
      default: true  # Favorite #2
    identity-c:
      default: true  # Favorite #3
```text

**Characteristics:**
- **Non-deterministic** - Requires user choice when multiple exist
- **Interactive required** - Prompts user in TTY, errors in CI
- **Provider-oriented** - Originally designed for "default identity per provider"
- **Multiple allowed** - Creates a shortlist of favorites

### Relationship Between the Two

**When BOTH exist:**
```yaml
auth:
  defaults:
    identity: identity-a  # Selected default (wins)

  identities:
    identity-a:
      default: true  # Also a favorite
    identity-b:
      default: true  # Favorite
```text

**Resolution:**
1. `auth.defaults.identity` is checked first → Returns `identity-a` deterministically
2. `identity.default: true` is only consulted if no `auth.defaults.identity` exists

**Result:** `auth.defaults.identity` **overrides** `identity.default: true` for automatic selection.

### Environment Variable Mapping

**New Environment Variable: `ATMOS_DEFAULTS_IDENTITY`**

Maps to `auth.defaults.identity` configuration:

```bash
# Option 1: Set via environment variable
export ATMOS_DEFAULTS_IDENTITY=my-identity
atmos terraform plan component -s stack

# Option 2: Set via configuration
# atmos.yaml
auth:
  defaults:
    identity: my-identity
```text

**Precedence with environment variables:**
```text
1. --identity=explicit            (CLI flag - highest)
2. ATMOS_IDENTITY                 (explicit identity selection)
3. ATMOS_DEFAULTS_IDENTITY        (selected default via env var) ← NEW
4. auth.defaults.identity         (selected default via config)
5. identity.default: true         (favorites)
6. Error: no default identity
```text

**Note:** `ATMOS_DEFAULTS_IDENTITY` env var takes precedence over `auth.defaults.identity` config, following Viper's environment variable precedence pattern.

**Use Cases:**
- Override config's selected default temporarily: `ATMOS_DEFAULTS_IDENTITY=temp-identity atmos ...`
- Set selected default in CI without config changes: `export ATMOS_DEFAULTS_IDENTITY=ci-identity`
- Profile-specific selected defaults in environment: `ATMOS_PROFILE=ci ATMOS_DEFAULTS_IDENTITY=github-oidc`

### Complete Precedence Chain

```text
Identity Resolution (with all mechanisms):
1. --identity=explicit-name           (CLI flag with value)
2. ATMOS_IDENTITY env var             (explicit identity selection)
3. ATMOS_DEFAULTS_IDENTITY env var    (selected default via environment) ← NEW
4. auth.defaults.identity             (selected default via config) ← NEW
5. identity.default: true             (favorites - interactive or error)
6. Error: no default identity

Session Duration Resolution (per identity):
1. identity.session.duration          (identity-specific override)
2. auth.defaults.session.duration     (global default) ← NEW
3. Provider default                   (provider-level default)

Console Session Resolution (per identity):
1. identity.console.session_duration  (identity-specific override)
2. auth.defaults.console.session_duration (global default) ← NEW
3. Provider console default           (provider-level default)
```text

## Use Cases

### Use Case 1: CI/CD Environment

**Problem:** CI environments are non-interactive. Multiple `default: true` identities cause errors.

**Solution:**
```yaml
# profiles/ci/auth.yaml
auth:
  defaults:
    identity: github-oidc-identity  # Explicit for CI

  identities:
    github-oidc-identity:
      # ... config
```text

**Result:** CI runs without TTY errors, automatically uses `github-oidc-identity`.

### Use Case 2: Developer Workstation

**Problem:** Developers want favorites list for quick switching, but a sensible default.

**Solution:**
```yaml
# atmos.yaml (base config)
auth:
  defaults:
    identity: developer-sandbox  # Sensible default

  identities:
    developer-sandbox:
      default: true  # Also mark as favorite
    developer-prod:
      default: true  # Favorite for quick --identity selection
    platform-admin:
      default: false  # Not a favorite (requires explicit --identity)
```text

**Result:**
- Default commands use `developer-sandbox`
- `atmos terraform plan --identity` shows favorites (sandbox + prod)
- Platform admin requires explicit `--identity platform-admin`

### Use Case 3: Profile-Specific Defaults

**Problem:** Different profiles need different default identities.

**Solution:**
```yaml
# profiles/audit/auth.yaml
auth:
  defaults:
    identity: audit-read-only

  identities:
    audit-read-only:
      kind: aws/permission-set
      # ... config

# profiles/platform-admin/auth.yaml
auth:
  defaults:
    identity: platform-admin

  identities:
    platform-admin:
      kind: aws/permission-set
      # ... config
```text

**Result:**
- `--profile audit` automatically uses `audit-read-only`
- `--profile platform-admin` automatically uses `platform-admin`

### Use Case 4: Global Session Defaults

**Problem:** All identities should have 12h sessions by default, but some need shorter.

**Solution:**
```yaml
auth:
  defaults:
    identity: default-identity
    session:
      duration: "12h"  # Global default

  identities:
    temporary-access:
      session:
        duration: "1h"  # Override for this identity
    # Other identities inherit 12h default
```text

**Result:** Reduces configuration duplication, clear overrides.

## Benefits

### 1. **Deterministic Default for Non-Interactive Environments**
- CI/CD no longer errors with "multiple defaults"
- `auth.defaults.identity` provides single source of truth
- Profiles can override per-environment

### 2. **Backward Compatible**
- Existing `identity.default: true` still works (favorites)
- No breaking changes to existing configurations
- New field is optional

### 3. **Clear Precedence**
- Explicit selection (`--identity` flag) always wins
- Global selected default (`auth.defaults.identity`) wins over favorites
- Favorites (`identity.default: true`) used as fallback for interactive selection

### 4. **Profile-Friendly**
- Profiles can set `auth.defaults.identity` for their use case
- Profile's selected default overrides base config favorites
- Works with profile merge precedence (rightmost wins)

### 5. **Reduces Configuration Duplication**
- Global session/console defaults apply to all identities
- Identity-specific overrides when needed
- Clear hierarchy: identity > defaults > provider

### 6. **Future-Proof for Additional Defaults**
- `auth.defaults.session` - Session configuration
- `auth.defaults.console` - Console configuration
- `auth.defaults.keyring` - Keyring configuration
- Extensible for future auth-related defaults

## Drawbacks and Mitigations

### Drawback 1: Two Ways to Set Default

**Issue:** Both `identity.default: true` and `auth.defaults.identity` exist.

**Mitigation:**
- Clear documentation: "Selected default" vs. "Favorites"
- `auth.defaults.identity` always wins (clear precedence)
- Recommendation: Use `auth.defaults.identity` in profiles, `identity.default: true` in base config

### Drawback 2: Schema Complexity

**Issue:** Adds another layer to auth configuration.

**Mitigation:**
- Optional field - only use when needed
- Clear examples in documentation
- Aligns with existing patterns (providers, identities, now defaults)

### Drawback 3: Validation Complexity

**Issue:** Must validate `auth.defaults.identity` references exist.

**Mitigation:**
- Validate during config loading (existing pattern)
- Clear error message: "auth.defaults.identity 'foo' not found"
- Same validation pattern as `identity.via.identity` references

## Implementation Plan

### Phase 1: Schema and Core Logic (Week 1)

1. Add `AuthDefaults` struct to `pkg/schema/schema_auth.go`
2. Add `Defaults *AuthDefaults` field to `AuthConfig`
3. Update `GetDefaultIdentity()` in `pkg/auth/manager.go`:
   - Check `auth.defaults.identity` first
   - Validate identity exists
   - Fall back to existing favorites logic
4. Add validation for `auth.defaults.identity` references
5. Update JSON schemas in `pkg/datafetcher/schema/`

**Tests:**
- Unit tests for precedence chain
- Validation tests for invalid references
- Multiple profile merge tests

### Phase 2: Documentation and Examples (Week 2)

1. Update identity resolution documentation
2. Add `auth.defaults` examples to PRD
3. Update profiles PRD with `auth.defaults` usage
4. Add CI profile example using `auth.defaults.identity`
5. Document favorites vs. selected default semantics

**Deliverables:**
- Updated auth documentation
- Profile examples with `auth.defaults`
- Migration guide for CI environments

## Alternatives Considered

### Alternative 1: Environment Variable Only

```bash
ATMOS_DEFAULT_IDENTITY=github-oidc-identity
```text

**Pros:**
- Simple, no schema changes
- Works with existing precedence

**Cons:**
- Not profile-aware (can't set per-profile)
- Doesn't solve global session defaults problem
- Environment variable sprawl

**Verdict:** Rejected - doesn't solve profile use case.

### Alternative 2: Profile-Specific Field

```yaml
profiles:
  ci:
    auth:
      identities:
        github-oidc-identity:
          default: true
```text

**Pros:**
- Clear profile scope
- Uses existing identity default mechanism

**Cons:**
- Profile needs to know about auth internals
- Doesn't solve global session defaults

**Verdict:** Rejected - violates separation of concerns.

### Alternative 3: Keep Current Behavior + Validation

Only allow single `identity.default: true` in non-interactive mode.

**Pros:**
- No schema changes
- Enforces best practice

**Cons:**
- Doesn't allow multiple favorites in base config
- Profiles can't override base defaults cleanly
- Doesn't solve global session defaults

**Verdict:** Rejected - too restrictive, breaks existing workflows.

## Recommendation

**Implement Option 1: `auth.defaults`**

**Rationale:**
1. Solves the profile + CI use case cleanly
2. Backward compatible with existing `identity.default: true`
3. Extensible for future defaults (session, console, keyring)
4. Clear precedence chain (selected > favorites)
5. Profile-friendly (each profile can set its selected default)
6. Reduces configuration duplication

**Key Decision:**
- `auth.defaults.identity` is the **selected default** (single choice)
- `identity.default: true` remains **favorites** (multiple allowed, interactive selection)
- Clear precedence ensures no ambiguity

## Open Questions

1. **Should `auth.defaults.identity` support multiple values?**
   - **Recommendation:** No - defeats the purpose of "selected default"
   - Multiple favorites already supported via `identity.default: true`

2. **Should identity-level session config merge with global defaults?**
   - **Recommendation:** No - identity-level overrides completely (simpler)
   - Document: Use global defaults for common cases, override when needed

3. **Should we deprecate `identity.default: true` eventually?**
   - **Recommendation:** No - it serves a different purpose (favorites list)
   - Keep both: selected default (single) + favorites (multiple)

4. **Should providers support `provider.default: true` or `provider.defaults` similar to identities?**
   - **Background:** The original intent of `identity.default: true` was provider-specific defaults
   - **Problem:** "You might have a default identity based on a given provider"
   - **Current Workaround:** Use `identity.default: true` on identities per provider (creates favorites list)

   **Future Consideration: Provider-Level Default Identity**
   ```yaml
   auth:
     providers:
       aws-sso-dev:
         kind: aws/sso
         defaults:
           identity: dev-default  # Default identity for this provider
       aws-sso-prod:
         kind: aws/sso
         defaults:
           identity: prod-default  # Default identity for this provider
   ```

   **Recommendation:** Not in this PRD - defer to future enhancement

   **Rationale:**
   - Current proposal solves the immediate problem (CI + profiles)
   - Provider-level defaults add significant complexity
   - Unclear precedence: global vs. provider-level vs. identity-level
   - Use case needs validation (when would you select by provider vs. by identity?)
   - Can be added later without breaking changes

   **Alternative:** Document pattern using profiles
   ```yaml
   # profiles/use-dev-provider/auth.yaml
   auth:
     defaults:
       identity: dev-default  # Effectively: default for aws-sso-dev
   ```
