# Auth Credential Namespace Isolation PRD

## Executive Summary

This document describes the credential namespace isolation feature that prevents credential collisions when the same identity names are used across different repositories. This is critical for consultants and engineers who work with multiple customers that use identical identity names (e.g., `core-root/terraform`).

**Status:** 📋 **Proposed** - Ready for implementation.

**Key Problem:** AWS credentials are cached globally based only on identity name, causing credential cross-contamination between different repositories with the same identity names.

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

## Solution: Hybrid Namespace Approach

### Concept

Introduce a **namespace** that differentiates credential storage between different repositories/customers. The namespace is derived from:

1. `ATMOS_AUTH_NAMESPACE` environment variable (highest priority)
2. `auth.namespace` in `atmos.yaml` (explicit configuration)
3. SHA256 hash of `CliConfigPath` (automatic fallback)

### Precedence Order

| Priority | Source | Example | Use Case |
|----------|--------|---------|----------|
| 1 (highest) | `ATMOS_AUTH_NAMESPACE` env var | `customer-acme` | CI/CD pipelines, automation |
| 2 | `auth.namespace` in `atmos.yaml` | `customer-acme` | Explicit configuration, portable |
| 3 (fallback) | SHA256 hash of `CliConfigPath` | `a1b2c3d4` | Automatic isolation without config |

### Directory Structure Change

**Before (vulnerable):**
```text
~/.config/atmos/aws/aws-user/credentials
                    └─ All customers share this directory
```

**After (isolated):**
```text
~/.config/atmos/aws/aws-user-{namespace}/credentials
                            └─ Unique per repository/customer
```

**Examples:**
```text
# Customer A (explicit namespace)
~/.config/atmos/aws/aws-user-customer-acme/credentials

# Customer B (explicit namespace)
~/.config/atmos/aws/aws-user-customer-beta/credentials

# Customer C (automatic hash namespace)
~/.config/atmos/aws/aws-user-a1b2c3d4/credentials
```

## Configuration

### Environment Variable

```bash
# Override namespace for CI/CD or testing
export ATMOS_AUTH_NAMESPACE="customer-acme"
atmos terraform plan -s plat-ue1-prod
```

### Configuration File

```yaml
# atmos.yaml
auth:
  namespace: "customer-acme"  # Optional: explicit namespace

  providers:
    aws-sso:
      kind: aws/sso
      spec:
        # ... provider configuration
```

### Automatic Namespace (Default)

When no explicit namespace is configured, the system automatically generates one:

```go
// Pseudocode
func getCredentialNamespace(atmosConfig *schema.AtmosConfiguration) string {
    // Priority 1: Environment variable
    if envNS := os.Getenv("ATMOS_AUTH_NAMESPACE"); envNS != "" {
        return sanitize(envNS)
    }

    // Priority 2: Explicit configuration
    if atmosConfig.Auth.Namespace != "" {
        return sanitize(atmosConfig.Auth.Namespace)
    }

    // Priority 3: Automatic hash of config path
    hash := sha256.Sum256([]byte(atmosConfig.CliConfigPath))
    return hex.EncodeToString(hash[:])[:8]  // First 8 characters
}
```

## Implementation Details

### Schema Changes

```go
// pkg/schema/schema.go
type AuthConfiguration struct {
    // Existing fields...

    // Namespace provides credential isolation between different repositories
    // or customer environments that may use the same identity names.
    // If not set, defaults to a hash of the atmos.yaml directory path.
    Namespace string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
}
```

### File Manager Changes

```go
// pkg/auth/cloud/aws/files.go

// GetCredentialsPath now includes namespace in the path
func (m *AWSFileManager) GetCredentialsPath(providerName, namespace string) string {
    dirName := providerName
    if namespace != "" {
        dirName = fmt.Sprintf("%s-%s", providerName, namespace)
    }
    return filepath.Join(m.baseDir, dirName, "credentials")
}

// GetConfigPath now includes namespace in the path
func (m *AWSFileManager) GetConfigPath(providerName, namespace string) string {
    dirName := providerName
    if namespace != "" {
        dirName = fmt.Sprintf("%s-%s", providerName, namespace)
    }
    return filepath.Join(m.baseDir, dirName, "config")
}
```

### Identity Implementation Changes

All identity implementations must pass namespace through credential storage:

- `pkg/auth/identities/aws/user.go`
- `pkg/auth/identities/aws/assume_role.go`
- `pkg/auth/identities/aws/permission_set.go`
- `pkg/auth/identities/aws/assume_root.go`

### Keyring/Credential Store Changes

```go
// pkg/auth/credentials/store.go

// Key format now includes namespace
func createKeyringKey(providerName, identityName, namespace string) string {
    if namespace != "" {
        return fmt.Sprintf("atmos:%s:%s:%s", namespace, providerName, identityName)
    }
    return fmt.Sprintf("atmos:%s:%s", providerName, identityName)
}
```

## User Experience

### Namespace Visibility

The namespace is displayed in `atmos auth status` output:

```bash
$ atmos auth status

Credential Namespace: customer-acme
  Source: atmos.yaml (auth.namespace)

Active Identities:
  ✓ core-root/terraform (aws-user)
    Expires: 2026-01-28T15:30:00Z
```

Or with automatic namespace:

```bash
$ atmos auth status

Credential Namespace: a1b2c3d4
  Source: auto-generated from /Users/dev/customer-acme/infrastructure

Active Identities:
  ✓ core-root/terraform (aws-user)
    Expires: 2026-01-28T15:30:00Z
```

### Clear Messaging

When credentials are not found due to namespace mismatch:

```
Error: No cached credentials found for identity 'core-root/terraform'

The credential namespace 'customer-beta' does not contain cached credentials
for this identity. This may happen when:
  - Switching between different customer repositories
  - Using a different namespace than when you last authenticated

Run 'atmos auth login' to authenticate with this identity.
```

## Migration Considerations

### Breaking Change

Existing cached credentials will not be found after this update because:

1. Old path: `~/.config/atmos/aws/aws-user/credentials`
2. New path: `~/.config/atmos/aws/aws-user-{namespace}/credentials`

### Mitigation

- **No automatic migration**: Users simply re-authenticate
- **Expected behavior**: This is the desired outcome - the old shared credentials should not be used
- **Clear documentation**: Release notes explain the change and why it improves security

### Recommended Upgrade Path

1. Deploy new Atmos version
2. Run `atmos auth login` in each repository to re-authenticate
3. (Optional) Configure explicit `auth.namespace` for portability

## Testing Strategy

### Unit Tests

1. **Namespace generation**:
   - Environment variable override
   - Configuration file value
   - Automatic hash generation
   - Sanitization of invalid characters

2. **Path generation**:
   - With explicit namespace
   - With automatic namespace
   - Without namespace (backward compatibility testing)

### Integration Tests

```go
func TestCredentialNamespaceIsolation(t *testing.T) {
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
| Same identity name across customers | Isolated by namespace |
| Switch between repos | Different credential directories |
| Unique INI profile sections | No credential collision |

### Residual Risks

1. **Same namespace across repos**: If users explicitly configure the same namespace in different repos, credentials will still be shared (intentional)
2. **Path changes**: Moving a repository changes the automatic namespace, requiring re-authentication

## Related Documents

1. **[AWS Authentication File Isolation PRD](./aws-auth-file-isolation.md)** - Current AWS implementation
2. **[Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md)** - Pattern this extends
3. **[Auth Context and Multi-Identity Support PRD](./auth-context-multi-identity.md)** - AuthContext design

## Success Metrics

This feature is successful when:

1. ✅ **Credential isolation**: Different repositories with same identity names use separate credentials
2. ✅ **Zero configuration default**: Works out-of-the-box without user configuration
3. ✅ **Explicit control**: Users can configure explicit namespaces for portability
4. ✅ **CI/CD support**: Environment variable allows automation scenarios
5. ✅ **Clear visibility**: Namespace displayed in `atmos auth status`
6. ✅ **Test coverage**: >80% coverage for namespace-related code

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2026-01-28 | 1.0 | Initial PRD created for credential namespace isolation |
