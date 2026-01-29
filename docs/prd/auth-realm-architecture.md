# Auth Realm Architecture PRD

## Executive Summary

This document defines the authentication realm architecture for Atmos. A realm provides complete credential isolation between different repositories, customers, or environments that may use identical identity and provider names.

**Status:** 📋 **Proposed** - Ready for implementation.

**Key Problem:** Credentials are cached globally using only identity and provider names, causing collisions when engineers work with multiple customer repositories that use the same naming conventions.

**Solution:** Introduce realms as the top-level isolation boundary for all credential storage. By default, realms are automatically derived from the repository path, ensuring zero-configuration isolation.

**Implementation Scope:**
- **AWS:** Will be implemented with this PRD (AWS authentication is currently implemented)
- **Azure:** Will be implemented when Azure authentication is built (Azure auth is documented but not yet implemented—see [Azure Authentication File Isolation PRD](./azure-auth-file-isolation.md))

---

## What is a Realm?

An authentication realm defines a complete, isolated authentication universe in which identities, credential authorities, resolution rules, and authentication semantics are evaluated together. In Atmos, a realm establishes the top-level boundary that determines:

- Which identities exist
- How they authenticate
- Where their credentials are stored and resolved

**Key insight:** Both identity names AND provider names can collide across customers. A realm isolates the entire authentication namespace, not just individual components.

---

## Realm Computation

### Default Behavior

By default, the realm is automatically computed as the SHA256 hash (first 8 characters) of the `CliConfigPath` (the directory containing `atmos.yaml`). This ensures:

- **Zero-configuration isolation** - Works automatically without any setup
- **Repository-scoped credentials** - Each repo gets its own credential space
- **No collisions by default** - Different paths produce different hashes

### Precedence Order

| Priority | Source | Example | Use Case |
|----------|--------|---------|----------|
| 1 (highest) | `ATMOS_AUTH_REALM` env var | `customer-acme` | CI/CD pipelines, testing |
| 2 | `auth.realm` in `atmos.yaml` | `customer-acme` | Explicit, portable across machines |
| 3 (default) | SHA256(`CliConfigPath`)[:8] | `a1b2c3d4` | Automatic isolation |

### Realm Value Requirements

**Allowed characters:**
- ASCII lowercase alphanumeric: `a-z`, `0-9`
- Hyphen: `-`
- Underscore: `_`

**Validation rules (for explicit values):**
1. Must contain only allowed characters (lowercase alphanumeric, hyphen, underscore)
2. Must not be empty
3. Must not start or end with hyphen or underscore
4. Must not contain consecutive hyphens or underscores
5. Maximum 64 characters
6. Must not contain path traversal sequences (`/`, `\`, `..`)

**Error behavior:** Invalid realm values result in an immediate error with a clear message explaining what characters are allowed. No sanitization is performed—the user must provide a valid realm value.

**Example error:**
```
Error: Invalid realm value 'my/realm'

Realm values must contain only lowercase letters, numbers, hyphens, and underscores.
The following characters are not allowed: /

Please update your auth.realm configuration or ATMOS_AUTH_REALM environment variable.
```

**Security:**
- Prevents path traversal attacks (validation rejects `/`, `\`, `..`)
- Cross-platform filesystem compatibility
- Deterministic output

---

## Realm Touchpoints

### Credential File Storage

The realm becomes the **top-level directory** under the atmos base path, with cloud as the next level:

| Cloud | Current Path | With Realm | Status |
|-------|--------------|------------|--------|
| AWS | `~/.config/atmos/aws/{provider}/` | `~/.config/atmos/{realm}/aws/{provider}/` | ✅ Will be implemented |
| Azure | `~/.azure/atmos/{provider}/` | `~/.config/atmos/{realm}/azure/{provider}/` | 🚧 Future (Azure auth not yet implemented) |

**Note:** All cloud providers now share the same base path (`~/.config/atmos/`) with realm as the top-level directory, followed by cloud type, then provider.

**Azure Note:** Azure authentication is documented in the [Azure Authentication File Isolation PRD](./azure-auth-file-isolation.md) but not yet implemented. When Azure auth is implemented, it will include realm support from the start.

### Directory Structure

```
~/.config/atmos/
├── a1b2c3d4/                      # Realm (auto-hash from Customer A's path)
│   └── aws/
│       ├── aws-sso/
│       │   ├── credentials        # INI file with identity profiles
│       │   └── config
│       └── aws-user/
│           ├── credentials
│           └── config
│
├── b5c6d7e8/                      # Realm (auto-hash from Customer B's path)
│   └── aws/
│       └── aws-sso/               # Same provider name, different realm
│           ├── credentials
│           └── config
│
└── customer-acme/                 # Realm (explicit config)
    ├── aws/
    │   └── aws-sso/
    │       ├── credentials
    │       └── config
    └── azure/
        └── azure-cli/
            └── credentials
```

### Keyring Storage

Keyring keys include realm prefix:

| Current | With Realm |
|---------|------------|
| `{identity}` | `atmos:{realm}:{identity}` |

**Examples:**
- Current: `core-root/terraform`
- With realm: `atmos:a1b2c3d4:core-root/terraform`

### Environment Variables

When credentials are set up, file paths include the realm:

```bash
AWS_SHARED_CREDENTIALS_FILE=~/.config/atmos/a1b2c3d4/aws/aws-sso/credentials
AWS_CONFIG_FILE=~/.config/atmos/a1b2c3d4/aws/aws-sso/config
```

---

## Data Flow Architecture

```
atmosConfig.CliConfigPath
│   (e.g., /Users/dev/customer-acme/infrastructure)
│
▼
realm.GetRealm(config.Realm, cliConfigPath)
│   1. Check ATMOS_AUTH_REALM env var
│   2. Check config.Realm from atmos.yaml
│   3. Default: SHA256(cliConfigPath)[:8] → "a1b2c3d4"
│
▼
manager.realm (stored in auth manager)
│
├──► credentialStore.Retrieve(identity, realm)
│         └──► keyring key: "atmos:a1b2c3d4:core-root/terraform"
│
├──► awsFileManager.GetCredentialsPath(provider, realm)
│         └──► ~/.config/atmos/a1b2c3d4/aws/aws-sso/credentials
│
└──► PostAuthenticateParams.Realm
           │
           ▼
     Identity.PostAuthenticate(params)
           │
           └──► SetupFiles(provider, identity, creds, basePath, realm)
                     └──► Writes to realm-scoped path
```

---

## Implementation Requirements

### New Package

**File:** `pkg/auth/realm/realm.go`

```go
package realm

const (
    EnvVarName   = "ATMOS_AUTH_REALM"
    SourceEnv    = "env"
    SourceConfig = "config"
    SourceAuto   = "auto"
    MaxLength    = 64
)

type RealmInfo struct {
    Value  string  // The realm identifier
    Source string  // How it was determined: "env", "config", or "auto"
}

// GetRealm computes the realm with proper precedence.
// Returns error if explicit realm value contains invalid characters.
func GetRealm(configRealm, cliConfigPath string) (RealmInfo, error)

// validate checks that a realm value contains only allowed characters.
// Returns error describing invalid characters if validation fails.
func validate(input string) error
```

### Schema Changes

**File:** `pkg/schema/schema_auth.go`

```go
type AuthConfig struct {
    // Realm provides credential isolation between different repositories.
    // Default: SHA256 hash of atmos.yaml directory path.
    Realm string `yaml:"realm,omitempty" json:"realm,omitempty" mapstructure:"realm"`

    // ... existing fields
}
```

**File:** `pkg/auth/types/whoami.go`

```go
type WhoamiInfo struct {
    Realm       string `json:"realm,omitempty"`
    RealmSource string `json:"realm_source,omitempty"` // "env", "config", "auto"

    // ... existing fields
}
```

### Interface Changes

**File:** `pkg/auth/types/interfaces.go`

**CredentialStore interface:**
```go
type CredentialStore interface {
    Store(alias string, creds ICredentials, realm string) error
    Retrieve(alias string, realm string) (ICredentials, error)
    Delete(alias string, realm string) error
    // ... other methods with realm parameter
}
```

**PostAuthenticateParams:**
```go
type PostAuthenticateParams struct {
    AuthContext  *schema.AuthContext
    StackInfo    *schema.ConfigAndStacksInfo
    ProviderName string
    IdentityName string
    Credentials  ICredentials
    Manager      AuthManager
    Realm        string  // NEW: Credential isolation realm
}
```

### File Manager Changes

**File:** `pkg/auth/cloud/aws/files.go`

```go
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

**Note:** The `baseDir` changes from `~/.config/atmos/aws` to `~/.config/atmos` since realm is now top-level.

**File:** `pkg/auth/cloud/azure/files.go`

Similar changes for Azure file manager, using `"azure"` as the cloud subdirectory.

### Manager Changes

**File:** `pkg/auth/manager.go`

```go
type manager struct {
    config          *schema.AuthConfig
    providers       map[string]types.Provider
    identities      map[string]types.Identity
    credentialStore types.CredentialStore
    validator       types.Validator
    stackInfo       *schema.ConfigAndStacksInfo
    chain           []string
    realm           realm.RealmInfo  // NEW: Computed realm
}

func NewAuthManager(
    config *schema.AuthConfig,
    credentialStore types.CredentialStore,
    validator types.Validator,
    stackInfo *schema.ConfigAndStacksInfo,
    cliConfigPath string,  // NEW: For realm computation
) (types.AuthManager, error) {
    realmInfo, err := realm.GetRealm(config.Realm, cliConfigPath)
    if err != nil {
        return nil, fmt.Errorf("failed to compute auth realm: %w", err)
    }
    // ...
}
```

**File:** `pkg/auth/manager_chain.go`

```go
// Update credential lookups to use realm
keyringCreds, keyringErr := m.credentialStore.Retrieve(identityName, m.realm.Value)
```

### Hooks Integration

**File:** `pkg/auth/hooks.go`

```go
func TerraformPreHook(atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
    // ...
    authManager, err := newAuthManager(&authConfig, stackInfo, atmosConfig.CliConfigPath)
    // ...
}
```

---

## Files Requiring Modification

| File | Change |
|------|--------|
| `pkg/auth/realm/realm.go` | **NEW** - Realm computation and validation |
| `pkg/schema/schema_auth.go` | Add `Realm` field to `AuthConfig` |
| `pkg/auth/types/interfaces.go` | Update `CredentialStore` interface, `PostAuthenticateParams` |
| `pkg/auth/types/whoami.go` | Add `Realm`, `RealmSource` to `WhoamiInfo` |
| `pkg/auth/credentials/keyring_system.go` | Include realm in key format |
| `pkg/auth/credentials/keyring_file.go` | Include realm in key format |
| `pkg/auth/credentials/keyring_memory.go` | Include realm in key format |
| `pkg/auth/credentials/keyring_noop.go` | Include realm in key format |
| `pkg/auth/cloud/aws/files.go` | Realm as top-level directory in paths |
| `pkg/auth/cloud/aws/setup.go` | Pass realm to file operations |
| `pkg/auth/cloud/azure/files.go` | Realm as top-level directory in paths (**Future** - when Azure auth is implemented) |
| `pkg/auth/manager.go` | Store realm, update `NewAuthManager` |
| `pkg/auth/manager_chain.go` | Use realm in credential lookups |
| `pkg/auth/identities/aws/user.go` | Pass realm through PostAuthenticate |
| `pkg/auth/identities/aws/assume_role.go` | Pass realm through PostAuthenticate |
| `pkg/auth/identities/aws/permission_set.go` | Pass realm through PostAuthenticate |
| `pkg/auth/identities/aws/assume_root.go` | Pass realm through PostAuthenticate |
| `pkg/auth/identities/azure/subscription.go` | Pass realm through PostAuthenticate (**Future** - when Azure auth is implemented) |
| `pkg/auth/hooks.go` | Pass `CliConfigPath` to manager creation |
| `cmd/auth_whoami.go` | Display realm in status output |

---

## User Experience

### `atmos auth status` Output

**With explicit realm:**
```
Credential Realm: customer-acme
  Source: atmos.yaml (auth.realm)

Active Identities:
  ✓ core-root/terraform (aws-sso)
    Expires: 2026-01-28T15:30:00Z
```

**With automatic realm:**
```
Credential Realm: a1b2c3d4
  Source: auto-generated from /Users/dev/customer-acme/infrastructure

Active Identities:
  ✓ core-root/terraform (aws-sso)
    Expires: 2026-01-28T15:30:00Z
```

### Error Messages

When credentials are not found:

```
Error: No cached credentials found for identity 'core-root/terraform'

The credential realm 'a1b2c3d4' does not contain cached credentials
for this identity. This may happen when:
  - Switching between different repositories
  - The realm changed (directory moved or renamed)
  - Using a different realm than when you last authenticated

Run 'atmos auth login' to authenticate with this identity.
```

---

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
  realm: "customer-acme"  # Optional: explicit realm for portability

  providers:
    aws-sso:
      kind: aws/sso
      spec:
        # ...
```

---

## Migration Considerations

### Breaking Change

Existing cached credentials will not be found after this update because paths change:

- **Old:** `~/.config/atmos/aws/aws-sso/credentials`
- **New:** `~/.config/atmos/{realm}/aws/aws-sso/credentials`

### Expected Behavior

This is the intended fix. Users re-authenticate in each repository:

```bash
atmos auth login
```

### No Automatic Migration

- Old credentials remain (can be manually cleaned up)
- New credentials stored in realm-scoped paths
- Clean separation ensures no cross-contamination

---

## Security Analysis

### Before (Vulnerable)

| Scenario | Risk |
|----------|------|
| Same identity name across customers | Credential cross-contamination |
| Same provider name across customers | Credentials shared between customers |
| Switch between repos | Wrong AWS account accessed |

### After (Secure)

| Scenario | Outcome |
|----------|---------|
| Same identity name across customers | Isolated by realm |
| Same provider name across customers | Isolated by realm |
| Switch between repos | Different realm, different credentials |

### Residual Risks

1. **Explicit same realm:** If users configure the same `auth.realm` in different repos, credentials will be shared (intentional)
2. **Path changes:** Moving a repository changes the automatic realm, requiring re-authentication

---

## Testing Strategy

### Unit Tests

1. Realm computation with env var, config, and auto-hash
2. Validation of realm values (valid and invalid cases)
3. Error messages for invalid realm values
4. Path generation with realm
5. Keyring key format with realm

### Integration Tests

```go
func TestCredentialRealmIsolation(t *testing.T) {
    // Create two mock repos with same identity name
    repoA := setupMockRepo(t, "/path/to/customer-a")
    repoB := setupMockRepo(t, "/path/to/customer-b")

    // Authenticate in repo A
    authInRepo(t, repoA, "core-root/terraform")

    // Verify repo B doesn't see repo A's credentials
    creds := getCredentials(t, repoB, "core-root/terraform")
    assert.Nil(t, creds)
}
```

---

## Related Documents

1. **[Auth Credential Realm Isolation PRD](./auth-user-credential-realm-isolation.md)** - Original problem statement
2. **[AWS Authentication File Isolation PRD](./aws-auth-file-isolation.md)** - AWS file isolation patterns
3. **[Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md)** - Base pattern

---

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2026-01-28 | 1.0 | Initial auth realm architecture PRD |
