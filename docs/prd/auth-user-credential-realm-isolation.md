# Auth Credential Realm Isolation PRD

## Executive Summary

This document describes the credential realm isolation feature that prevents credential collisions when the same identity names are used across different repositories. This is critical for consultants and engineers who work with multiple customers that use identical identity names (e.g., `core-root/terraform`).

**Status:** 📋 **Proposed** - Ready for implementation.

**Key Problem:** AWS credentials are cached globally based only on identity name, causing credential cross-contamination between different repositories with the same identity names.

**Implementation Scope:**
- **AWS:** Will be implemented with this PRD (AWS authentication is currently implemented)
- **Azure:** Will be implemented when Azure authentication is built (Azure auth is documented but not yet implemented—see [Azure Authentication File Isolation PRD](./azure-auth-file-isolation.md))

## Problem Statement

### The Issue

When engineers work with multiple customers, credentials collide:

1. Authenticate to Customer A's `core-root/terraform` identity → credentials cached at `~/.config/atmos/aws/aws-user/credentials`
2. Switch to Customer B's repository, run `atmos terraform plan` with same identity name `core-root/terraform`
3. **Uses Customer A's cached credentials** → wrong AWS account, permission failures, potential security incident

### Root Cause

Current credential storage uses only the provider name and identity name:

```text
~/.config/atmos/aws/{providerName}/credentials
                     └─ "aws-user" (hardcoded for aws/user identities)

[{identityName}]     <- INI profile section
└─ "core-root/terraform" (from identity config)
```

**Missing:** No repository or customer differentiation. Identity names are treated as globally unique, but in reality, multiple customers may use the same identity names.

### Constraints

Any solution must work within these constraints:

1. **No git dependency**: Cannot require a git repository (user may not be in a git repo)
2. **No network calls**: Cannot use remote URLs (no network dependency before auth)
3. **Pre-authentication**: Must work before AWS authentication (cannot use account ID)
4. **Works offline**: Must function without any external services

## Solution: Hybrid Realm Approach

### What is a Realm?

An authentication realm defines a complete, isolated authentication universe in which identities, credential authorities, resolution rules, and authentication semantics are evaluated together. In Atmos, a realm establishes the top-level boundary that determines which identities exist, how they authenticate, and where their credentials are stored and resolved.

### Concept

Introduce a **realm** that differentiates credential storage between different repositories/customers. The realm is derived from:

1. `ATMOS_AUTH_REALM` environment variable (highest priority)
2. `auth.realm` in `atmos.yaml` (explicit configuration)
3. SHA256 hash of `CliConfigPath` (automatic fallback)

### Precedence Order

| Priority | Source | Example | Use Case |
|----------|--------|---------|----------|
| 1 (highest) | `ATMOS_AUTH_REALM` env var | `customer-acme` | CI/CD pipelines, automation |
| 2 | `auth.realm` in `atmos.yaml` | `customer-acme` | Explicit configuration, portable |
| 3 (fallback) | SHA256 hash of `CliConfigPath` | `a1b2c3d4` | Automatic isolation without config |

### Directory Structure Change

**Before (vulnerable):**
```text
~/.config/atmos/aws/aws-user/credentials
                    └─ All customers share this directory
```

**After (isolated):**
```text
~/.config/atmos/{realm}/{cloud}/{provider}/credentials
                 └─ Realm is top-level directory, above cloud
```

**Examples:**
```text
# Customer A (explicit realm)
~/.config/atmos/customer-acme/aws/aws-user/credentials

# Customer B (explicit realm)
~/.config/atmos/customer-beta/aws/aws-user/credentials

# Customer C (automatic hash realm)
~/.config/atmos/a1b2c3d4/aws/aws-user/credentials
```

**Note:** The realm is the top-level directory under `~/.config/atmos/`, followed by cloud type (`aws`, `azure`), then provider name. This ensures complete isolation across realms, cloud providers, and provider configurations. See [Auth Realm Architecture PRD](./auth-realm-architecture.md) for full details.

## Configuration

### Environment Variable

```bash
# Override realm for CI/CD or testing
export ATMOS_AUTH_REALM="customer-acme"
atmos terraform plan -s plat-ue1-prod
```

### Configuration File

```yaml
# atmos.yaml
auth:
  realm: "customer-acme"  # Optional: explicit realm

  providers:
    aws-sso:
      kind: aws/sso
      spec:
        # ... provider configuration
```

### Automatic Realm (Default)

When no explicit realm is configured, the system automatically generates one:

```go
// Pseudocode
func getCredentialRealm(atmosConfig *schema.AtmosConfiguration) (string, error) {
    // Priority 1: Environment variable
    if envRealm := os.Getenv("ATMOS_AUTH_REALM"); envRealm != "" {
        if err := validate(envRealm); err != nil {
            return "", err
        }
        return envRealm, nil
    }

    // Priority 2: Explicit configuration
    if atmosConfig.Auth.Realm != "" {
        if err := validate(atmosConfig.Auth.Realm); err != nil {
            return "", err
        }
        return atmosConfig.Auth.Realm, nil
    }

    // Priority 3: Automatic hash of config path
    hash := sha256.Sum256([]byte(atmosConfig.CliConfigPath))
    return hex.EncodeToString(hash[:])[:8], nil  // First 8 characters
}
```

### Realm Validation

The `validate()` function ensures realm values are safe for filesystem paths. **Invalid values result in an error—no sanitization is performed.**

**Allowed Characters:**
- ASCII lowercase alphanumeric characters: `a-z`, `0-9`
- Hyphen: `-`
- Underscore: `_`

**Validation Rules:**
1. Must contain only allowed characters (lowercase alphanumeric, hyphen, underscore)
2. Must not be empty
3. Must not start or end with hyphen or underscore
4. Must not contain consecutive hyphens or underscores
5. Maximum 64 characters
6. Must not contain path traversal sequences (`/`, `\`, `..`)

**Error Behavior:** Invalid realm values result in an immediate error with a clear message. The user must fix the configuration—no automatic transformation is applied.

```go
// Pseudocode
func validate(input string) error {
    if input == "" {
        return errors.New("realm cannot be empty")
    }

    if len(input) > 64 {
        return fmt.Errorf("realm exceeds maximum length of 64 characters: %d", len(input))
    }

    // Check for invalid characters
    invalidChars := regexp.MustCompile(`[^a-z0-9_-]`).FindAllString(input, -1)
    if len(invalidChars) > 0 {
        return fmt.Errorf("realm contains invalid characters: %v", invalidChars)
    }

    // Check start/end
    if strings.HasPrefix(input, "-") || strings.HasPrefix(input, "_") {
        return errors.New("realm cannot start with hyphen or underscore")
    }
    if strings.HasSuffix(input, "-") || strings.HasSuffix(input, "_") {
        return errors.New("realm cannot end with hyphen or underscore")
    }

    return nil
}
```

**Example error:**
```
Error: Invalid realm value 'My/Realm'

Realm values must contain only lowercase letters, numbers, hyphens, and underscores.
Invalid characters found: /, M, R

Please update your auth.realm configuration or ATMOS_AUTH_REALM environment variable.
```

**Security Considerations:**
- Prevents path traversal attacks (validation rejects `/`, `\`, `..`)
- Ensures cross-platform filesystem compatibility
- Deterministic behavior—same input always produces same result (valid or error)

## Implementation Details

### Schema Changes

```go
// pkg/schema/schema.go
type AuthConfiguration struct {
    // Existing fields...

    // Realm provides credential isolation between different repositories
    // or customer environments that may use the same identity names.
    // If not set, defaults to a hash of the atmos.yaml directory path.
    Realm string `json:"realm,omitempty" yaml:"realm,omitempty"`
}
```

### File Manager Changes

```go
// pkg/auth/cloud/aws/files.go

// GetCredentialsPath returns path with realm as top-level directory.
// Result: ~/.config/atmos/{realm}/aws/{provider}/credentials
func (m *AWSFileManager) GetCredentialsPath(providerName, realm string) string {
    if realm != "" {
        return filepath.Join(m.baseDir, realm, "aws", providerName, "credentials")
    }
    return filepath.Join(m.baseDir, "aws", providerName, "credentials")
}

// GetConfigPath returns path with realm as top-level directory.
// Result: ~/.config/atmos/{realm}/aws/{provider}/config
func (m *AWSFileManager) GetConfigPath(providerName, realm string) string {
    if realm != "" {
        return filepath.Join(m.baseDir, realm, "aws", providerName, "config")
    }
    return filepath.Join(m.baseDir, "aws", providerName, "config")
}
```

**Note:** The `baseDir` is now `~/.config/atmos` (not `~/.config/atmos/aws`) since realm is the top-level directory.

### Identity Implementation Changes

All identity implementations must pass realm through credential storage:

- `pkg/auth/identities/aws/user.go`
- `pkg/auth/identities/aws/assume_role.go`
- `pkg/auth/identities/aws/permission_set.go`
- `pkg/auth/identities/aws/assume_root.go`

### Keyring/Credential Store Changes

```go
// pkg/auth/credentials/store.go

// Key format now includes realm
func createKeyringKey(providerName, identityName, realm string) string {
    if realm != "" {
        return fmt.Sprintf("atmos:%s:%s:%s", realm, providerName, identityName)
    }
    return fmt.Sprintf("atmos:%s:%s", providerName, identityName)
}
```

## User Experience

### Realm Visibility

The realm is displayed in `atmos auth status` output:

```bash
$ atmos auth status

Credential Realm: customer-acme
  Source: atmos.yaml (auth.realm)

Active Identities:
  ✓ core-root/terraform (aws-user)
    Expires: 2026-01-28T15:30:00Z
```

Or with automatic realm:

```bash
$ atmos auth status

Credential Realm: a1b2c3d4
  Source: auto-generated from /Users/dev/customer-acme/infrastructure

Active Identities:
  ✓ core-root/terraform (aws-user)
    Expires: 2026-01-28T15:30:00Z
```

### Clear Messaging

When credentials are not found due to realm mismatch:

```
Error: No cached credentials found for identity 'core-root/terraform'

The credential realm 'customer-beta' does not contain cached credentials
for this identity. This may happen when:
  - Switching between different customer repositories
  - Using a different realm than when you last authenticated

Run 'atmos auth login' to authenticate with this identity.
```

## Migration Considerations

### Breaking Change

Existing cached credentials will not be found after this update because:

1. Old path: `~/.config/atmos/aws/aws-user/credentials`
2. New path: `~/.config/atmos/{realm}/aws/aws-user/credentials`

### Mitigation

- **No automatic migration**: Users simply re-authenticate
- **Expected behavior**: This is the desired outcome - the old shared credentials should not be used
- **Clear documentation**: Release notes explain the change and why it improves security

### Recommended Upgrade Path

1. Deploy new Atmos version
2. Run `atmos auth login` in each repository to re-authenticate
3. (Optional) Configure explicit `auth.realm` for portability

## Testing Strategy

### Unit Tests

1. **Realm generation**:
   - Environment variable override
   - Configuration file value
   - Automatic hash generation
   - Validation of invalid characters (error cases)

2. **Realm validation**:
   - Valid realm values accepted
   - Invalid characters rejected with clear error
   - Edge cases (empty, too long, starts/ends with hyphen)

3. **Path generation**:
   - With explicit realm
   - With automatic realm
   - Correct directory structure (`{realm}/{cloud}/{provider}`)

### Integration Tests

```go
func TestCredentialRealmIsolation(t *testing.T) {
    // Create two mock repositories with same identity name
    repoA := setupMockRepo(t, "customer-a")
    repoB := setupMockRepo(t, "customer-b")

    // Authenticate in repo A
    authInRepo(t, repoA, "core-root/terraform")

    // Switch to repo B
    // Verify credentials are NOT shared
    creds := getCredentials(t, repoB, "core-root/terraform")
    assert.Nil(t, creds, "Should not find repo A credentials in repo B")
}
```

### Manual Testing

1. `ATMOS_PROFILE=superadmin atmos terraform plan` in Customer A's repo
2. Switch to Customer B's repo, run same command
3. Verify no credential cross-contamination (should prompt for re-auth)

## Security Analysis

### Before (Vulnerable)

| Scenario | Risk |
|----------|------|
| Same identity name across customers | Credential cross-contamination |
| Switch between repos | Wrong AWS account accessed |
| Shared INI profile sections | Credentials overwritten silently |

### After (Secure)

| Scenario | Outcome |
|----------|---------|
| Same identity name across customers | Isolated by realm |
| Switch between repos | Different credential directories |
| Unique INI profile sections | No credential collision |

### Residual Risks

1. **Same realm across repos**: If users explicitly configure the same realm in different repos, credentials will still be shared (intentional)
2. **Path changes**: Moving a repository changes the automatic realm, requiring re-authentication

## Related Documents

1. **[Auth Realm Architecture PRD](./auth-realm-architecture.md)** - Complete realm architecture and implementation guide
2. **[AWS Authentication File Isolation PRD](./aws-auth-file-isolation.md)** - Current AWS implementation
3. **[Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md)** - Pattern this extends
4. **[Auth Context and Multi-Identity Support PRD](./auth-context-multi-identity.md)** - AuthContext design

## Success Metrics

This feature is successful when:

1. ✅ **Credential isolation**: Different repositories with same identity names use separate credentials
2. ✅ **Zero configuration default**: Works out-of-the-box without user configuration
3. ✅ **Explicit control**: Users can configure explicit realms for portability
4. ✅ **CI/CD support**: Environment variable allows automation scenarios
5. ✅ **Clear visibility**: Realm displayed in `atmos auth status`
6. ✅ **Test coverage**: >80% coverage for realm-related code

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2026-01-28 | 1.0 | Initial PRD created for credential realm isolation |
