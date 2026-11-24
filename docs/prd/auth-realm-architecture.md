# Auth Realm Architecture PRD

## Executive Summary

This document defines the authentication realm architecture for Atmos. A realm provides complete credential isolation between different repositories, customers, or environments that may use identical identity and provider names.

**Status:** ✅ **Implemented** - Realm isolation is opt-in by design.

**Key Problem:** Credentials are cached globally using only identity and provider names, causing collisions when engineers work with multiple customer repositories that use the same naming conventions.

**Solution:** Introduce realms as the top-level isolation boundary for all credential storage. Realms are explicitly configured via `auth.realm` in `atmos.yaml` or `ATMOS_AUTH_REALM` environment variable.

**Design Decision:** Realm isolation is **opt-in** rather than automatic. When auth is configured, an explicit realm value is required — this ensures users consciously choose their isolation boundary rather than relying on path-derived hashes that break when directories are moved or renamed. See [GitHub issue #2071](https://github.com/cloudposse/atmos/issues/2071) for rationale.

**Implementation Scope:**
- **AWS:** ✅ Implemented — realm is used for file path isolation and keyring key prefixing
- **GCP:** ✅ Implemented — GCP service account identity enforces non-empty realm
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

Realm isolation is **opt-in**. When auth is configured (providers and/or identities exist in `atmos.yaml`), an explicit realm value is required via either `ATMOS_AUTH_REALM` environment variable or `auth.realm` in `atmos.yaml`. If neither is set, `NewAuthManager` returns `ErrEmptyRealm` and the auth operation fails with a clear error message.

This opt-in design ensures:

- **Explicit isolation boundaries** - Users consciously choose their realm name
- **Portable across machines** - Named realms like `customer-acme` work the same everywhere
- **CI/CD friendly** - `ATMOS_AUTH_REALM` can be set per pipeline/job
- **No surprises from path changes** - Moving a directory doesn't invalidate credentials

### Precedence Order

| Priority | Source | Example | Use Case |
|----------|--------|---------|----------|
| 1 (highest) | `ATMOS_AUTH_REALM` env var | `customer-acme` | CI/CD pipelines, testing |
| 2 | `auth.realm` in `atmos.yaml` | `customer-acme` | Explicit, portable across machines |
| 3 (default) | Empty (error) | — | Fails with `ErrEmptyRealm` when auth is configured |

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

| Cloud | Without Realm | With Realm | Status |
|-------|---------------|------------|--------|
| AWS | `~/.config/atmos/aws/{provider}/` | `~/.config/atmos/{realm}/aws/{provider}/` | ✅ Implemented |
| Azure | `~/.azure/atmos/{provider}/` | `~/.config/atmos/{realm}/azure/{provider}/` | 🚧 Future (Azure auth not yet implemented) |

**Note:** All cloud providers share the same base path (`~/.config/atmos/`) with realm as the top-level directory, followed by cloud type, then provider.

**Azure Note:** Azure authentication is documented in the [Azure Authentication File Isolation PRD](./azure-auth-file-isolation.md) but not yet implemented. When Azure auth is implemented, it will include realm support from the start.

### Directory Structure

```
~/.config/atmos/
├── customer-a/                    # Realm (from auth.realm or ATMOS_AUTH_REALM)
│   └── aws/
│       ├── aws-sso/
│       │   ├── credentials        # INI file with identity profiles
│       │   └── config
│       └── aws-user/
│           ├── credentials
│           └── config
│
├── customer-b/                    # Realm (different customer, same provider names)
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

Keyring keys include realm prefix using underscore separators (underscores instead of colons for Windows filesystem compatibility). The realm and identity are separated by a double underscore: realms cannot contain `__` or end with `_`, so the first `__` always ends the realm, and realm `acme` + identity `prod_admin` never shares a key with realm `acme_prod` + identity `admin`:

| Without Realm | With Realm |
|---------------|------------|
| `atmos_{identity}` | `atmos_{realm}__{identity}` |

**Examples:**
- Without realm: `atmos_core-root/terraform`
- With realm: `atmos_customer-a__core-root/terraform`

Entries written in the previous `atmos_{realm}_{identity}` layout are still read (and deleted on logout) when neither the realm nor the identity contains `_`, the only case in which the old key cannot belong to another realm. Other old entries are ignored and re-created on the next login. The pre-realm `atmos_{identity}` cleanup after a realm-scoped login likewise skips identities containing `_`, whose key another realm can share.

An entry found through that fallback is migrated on read: it is copied unchanged to `atmos_{realm}__{identity}`, so later reads no longer depend on the fallback. The old key is kept, so a downgrade to v1.234.1 (which reads only the old key) still finds the credentials; the new key is always read first, and `Delete` removes both. The migration is best effort and never fails the read. It is skipped when the new key already exists (another process stored newer credentials), and on a write failure the fallback keeps serving the old key. Only entries that decode are migrated. Long-lived `aws/user` keys stored by `atmos auth user configure` are read through the same path, so they migrate on first use. An old key whose realm or identity contains `_` is ambiguous (realm `acme` + identity `prod_admin` and realm `acme_prod` + identity `admin` both wrote `atmos_acme_prod_admin`), so `Retrieve` never reads it and never deletes it. For most identities that only costs one login. Long-lived `aws/user` keys cannot be re-created by a login, so the `aws/user` identity handles that case itself (`pkg/auth/identities/aws/user_legacy_keyring.go`, through the optional `credentials.AmbiguousLegacyRealmStore` interface):

- It applies only when no keys are stored under `atmos_{realm}__{identity}`, the old key exists, decodes to long-lived AWS keys (not session credentials), and does not also parse as a current-layout key (an old key containing `__` could be the live entry of another realm, so it is never offered).
- In an interactive session (stdin and stderr are terminals, not CI, `--interactive` not disabled, and the caller allows prompts), the user is asked whether the entry belongs to this identity. The question names the identity, the realm, the old key, and the other realm and identity pairs that shared it. The default answer is No.
- On Yes, the entry is copied unchanged to the new key (unless the new key appeared meanwhile, whose newer credentials win) and the old key is deleted. Deleting is safe at this point because the user claimed the entry: keeping it would leave a second copy of the secret that logout never removes, and the other realm would keep being asked about an entry that is not its own. Authentication continues with the keys.
- On No, nothing in the keyring changes, the not-configured error is returned with a hint to run `atmos auth user configure`, and the question is not asked again for that identity during the same invocation. It is asked again in a later invocation until the identity has keys under the new key.
- Without an interactive session, no question is asked. The not-configured error names the old key and suggests `atmos auth user configure` or re-running interactively.
- In both No and non-interactive cases the error is the usual `aws/user` not-configured error, so when browser sign-in (`webflow_enabled`, on by default) is enabled the identity still falls back to it. If that also fails, the old-key hints are kept in the error.

Other identity kinds never ask: their keyring entries are session credentials that the next login re-creates.

#### Configuration digests of chain steps

Keyring keys name a step (provider or identity) but not its configuration, and stack or component `auth` overrides can give the same name a different account, role, or start URL. So a cached step is reused across processes only when it was written for the same effective configuration (#3720):

- Each keyring entry stores a `config_digest` next to `type` and `data`. Older versions ignore the field, and entries without a digest have exactly the old format.
- AWS session credentials are never stored in the keyring; they live in the identity's own credential files. For these, a record `{"digest", "expiration", "credential_id"}` is written after that identity's login. The expiration and credential ID tie the record to those exact files, so files that something else rewrote no longer match. The record holds nothing secret and describes files, not keyring entries, so the persistent backends keep it in a file outside the keyring: next to a file keyring's directory (`<keyring dir>.config-digests/`), or in the Atmos XDG cache directory (`auth/config-digests/`) for the system keyring. Writing it to the keyring would add a keyring entry (a macOS Keychain prompt, or the file keyring password prompt) for identities that never otherwise use the keyring. The file name is a hash of `atmos-config-digest_{realm}__{identity}`. Only the memory backend keeps the record under that key. Logout deletes the record with the identity.
- The digest is a plain SHA-256 of canonical JSON over the non-secret configuration of every step from the chain root through that step. For providers it covers `kind`, `start_url`, `url`, `region`, `username` and `spec`. For identities it covers `kind`, `provider`, `via`, `principal`, `spec` and `emulator`. It never includes the provider `password` or identity `credentials`. Inside `principal` and `spec`, keys that name secret material (such as `*secret*`, `*token*`, `*password*`, `*private_key*`, `*access_key*`, `external_id`) are dropped. References such as `*_path`, `*_env`, `*_url` and `*_arn` are kept.
- A step whose digest is missing or different is a cache miss and is authenticated again. Entries written before this change have no digest, so after an upgrade each intermediate step is authenticated once more. The target step is always authenticated again, as before.
- The noop keyring persists nothing, so no digest can be recorded. With it, steps backed by identity files keep the previous name-only reuse.

### Environment Variables

When credentials are set up, file paths include the realm:

```bash
AWS_SHARED_CREDENTIALS_FILE=~/.config/atmos/customer-a/aws/aws-sso/credentials
AWS_CONFIG_FILE=~/.config/atmos/customer-a/aws/aws-sso/config
```

---

## Data Flow Architecture

```
realm.GetRealm(config.Realm, cliConfigPath)
│   1. Check ATMOS_AUTH_REALM env var
│   2. Check config.Realm from atmos.yaml
│   3. Default: empty → ErrEmptyRealm (realm is required)
│
▼
manager.realm (stored in auth manager)
│
├──► credentialStore.Retrieve(identity, realm)
│         └──► keyring key: "atmos_customer-a__core-root/terraform"
│
├──► awsFileManager (realm stored on struct at construction)
│         └──► GetCredentialsPath(provider)
│                   └──► ~/.config/atmos/customer-a/aws/aws-sso/credentials
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

## Implementation Details

### Realm Package

**File:** `pkg/auth/realm/realm.go` — ✅ Implemented

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
// Returns empty realm with SourceAuto when no explicit realm is configured.
// NewAuthManager rejects empty realms — explicit configuration is required.
// Returns error if explicit realm value contains invalid characters.
func GetRealm(configRealm, cliConfigPath string) (RealmInfo, error)

// Validate checks that a realm value contains only allowed characters.
// Returns error describing invalid characters if validation fails.
func Validate(input string) error

// SourceDescription returns a human-readable description of where the realm came from.
func (r RealmInfo) SourceDescription(cliConfigPath string) string
```

### Schema Changes — ✅ Implemented

**File:** `pkg/schema/schema_auth.go`

```go
type AuthConfig struct {
    // Realm provides credential isolation between different repositories.
    // Required when auth providers/identities are configured.
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

### Interface Changes — ✅ Implemented

**File:** `pkg/auth/types/interfaces.go`

**CredentialStore interface:**
```go
type CredentialStore interface {
    Store(alias string, creds ICredentials, realm string) error
    Retrieve(alias string, realm string) (ICredentials, error)
    Delete(alias string, realm string) error
    List(realm string) ([]string, error)
    IsExpired(alias string, realm string) (bool, error)
    Type() string
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
    Realm        string  // Credential isolation realm
}
```

**Provider and Identity interfaces** also include `SetRealm(realm string)` methods,
called by the manager after realm computation to propagate the realm to all components.

### File Manager Changes — ✅ Implemented

**File:** `pkg/auth/cloud/aws/files.go`

The realm is stored on the `AWSFileManager` struct at construction time rather than passed per-call:

```go
type AWSFileManager struct {
    baseDir string  // e.g., ~/.config/atmos
    realm   string  // Realm for credential isolation
}

// NewAWSFileManager creates a file manager with realm stored on the struct.
// Result paths: ~/.config/atmos/{realm}/aws/{provider}/credentials
func NewAWSFileManager(basePath, realm string) (*AWSFileManager, error)

// GetCredentialsPath uses the stored realm.
// With realm: {baseDir}/{realm}/aws/{provider}/credentials
// Without realm: {baseDir}/aws/{provider}/credentials
func (m *AWSFileManager) GetCredentialsPath(providerName string) string {
    return filepath.Join(m.baseDir, m.realm, awsDirName, providerName, "credentials")
}
```

**Note:** `filepath.Join` skips empty segments, so when `m.realm == ""` the path is `{baseDir}/aws/{provider}/credentials` (backward-compatible).

**File:** `pkg/auth/cloud/azure/files.go`

Similar changes for Azure file manager, using `"azure"` as the cloud subdirectory.

### Manager Changes — ✅ Implemented

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
    realm           realm.RealmInfo  // Computed realm
}

func NewAuthManager(
    config *schema.AuthConfig,
    credentialStore types.CredentialStore,
    validator types.Validator,
    stackInfo *schema.ConfigAndStacksInfo,
    cliConfigPath string,  // For realm computation
) (types.AuthManager, error) {
    realmInfo, err := realm.GetRealm(config.Realm, cliConfigPath)
    if err != nil {
        return nil, fmt.Errorf("failed to compute auth realm: %w", err)
    }
    // Empty realm is rejected — explicit configuration is required.
    if realmInfo.Value == "" {
        return nil, fmt.Errorf("%w: realm computation produced an empty value", ErrEmptyRealm)
    }
    // Realm is propagated to all providers and identities via SetRealm().
    // ...
}
```

**File:** `pkg/auth/manager_chain.go`

```go
// All credential lookups use realm for isolation.
keyringCreds, keyringErr := m.credentialStore.Retrieve(identityName, m.realm.Value)
```

### Hooks Integration — ✅ Implemented

**File:** `pkg/auth/hooks.go`

```go
func TerraformPreHook(atmosConfig *schema.AtmosConfiguration, stackInfo *schema.ConfigAndStacksInfo) error {
    // ...
    authManager, err := newAuthManager(&authConfig, stackInfo, atmosConfig.CliConfigPath)
    // ...
}
```

---

## Files Modified

| File | Change | Status |
|------|--------|--------|
| `pkg/auth/realm/realm.go` | Realm computation and validation | ✅ Implemented |
| `pkg/auth/realm/realm_test.go` | Tests for realm computation and validation | ✅ Implemented |
| `pkg/schema/schema_auth.go` | `Realm` field on `AuthConfig` | ✅ Implemented |
| `pkg/auth/types/interfaces.go` | `CredentialStore` interface with realm, `PostAuthenticateParams`, `SetRealm` on Provider/Identity | ✅ Implemented |
| `pkg/auth/types/whoami.go` | `Realm`, `RealmSource` on `WhoamiInfo` | ✅ Implemented |
| `pkg/auth/credentials/store.go` | `buildKeyringKey` with realm prefix (underscore separator) | ✅ Implemented |
| `pkg/auth/credentials/keyring_system.go` | Realm in key format | ✅ Implemented |
| `pkg/auth/credentials/keyring_file.go` | Realm in key format | ✅ Implemented |
| `pkg/auth/credentials/keyring_memory.go` | Realm in key format | ✅ Implemented |
| `pkg/auth/credentials/keyring_noop.go` | Realm in key format (accepts but ignores) | ✅ Implemented |
| `pkg/auth/cloud/aws/files.go` | Realm stored on struct, used in path construction | ✅ Implemented |
| `pkg/auth/cloud/aws/setup.go` | Pass realm to file operations | ✅ Implemented |
| `pkg/auth/cloud/azure/files.go` | Realm as top-level directory in paths | 🚧 Future (Azure auth not yet implemented) |
| `pkg/auth/manager.go` | Store realm, enforce non-empty, propagate via `SetRealm` | ✅ Implemented |
| `pkg/auth/manager_chain.go` | Realm in credential lookups | ✅ Implemented |
| `pkg/auth/identities/aws/user.go` | Realm through PostAuthenticate | ✅ Implemented |
| `pkg/auth/identities/aws/assume_role.go` | Realm through PostAuthenticate | ✅ Implemented |
| `pkg/auth/identities/aws/permission_set.go` | Realm through PostAuthenticate | ✅ Implemented |
| `pkg/auth/identities/aws/assume_root.go` | Realm through PostAuthenticate | ✅ Implemented |
| `pkg/auth/identities/gcp_service_account/` | GCP identity enforces non-empty realm | ✅ Implemented |
| `pkg/auth/identities/azure/subscription.go` | Realm through PostAuthenticate | 🚧 Future (Azure auth not yet implemented) |
| `pkg/auth/hooks.go` | Pass `CliConfigPath` to manager creation | ✅ Implemented |
| `cmd/auth_whoami.go` | Display realm in status output (when non-empty) | ✅ Implemented |

---

## User Experience

### `atmos auth status` Output

**With realm configured via `atmos.yaml`:**
```
Credential Realm: customer-acme (config)

Active Identities:
  ✓ core-root/terraform (aws-sso)
    Expires: 2026-01-28T15:30:00Z
```

**With realm configured via environment variable:**
```
Credential Realm: customer-acme (env)

Active Identities:
  ✓ core-root/terraform (aws-sso)
    Expires: 2026-01-28T15:30:00Z
```

### Error Messages

When realm is not configured but auth is:

```
Error: realm is required for credential isolation but was not set

Set auth.realm in atmos.yaml or ATMOS_AUTH_REALM environment variable.
```

---

## Configuration

### Environment Variable

```bash
# Set realm for CI/CD or testing
export ATMOS_AUTH_REALM="customer-acme"
atmos terraform plan -s plat-ue1-prod
```

### Configuration File

```yaml
# atmos.yaml
auth:
  realm: "customer-acme"  # Required: realm for credential isolation

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

---

## Testing Strategy

### Unit Tests

1. Realm computation with env var, config, and empty default
2. Validation of realm values (valid and invalid cases)
3. Error messages for invalid realm values
4. Path generation with realm
5. Keyring key format with realm (underscore separator)

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
| 2026-02-19 | 1.1 | Updated to reflect implemented opt-in design: realm is required when auth is configured (no auto-hash default), underscore keyring separator for Windows compatibility, realm stored on AWSFileManager struct. Updated status from Proposed to Implemented. |
| 2026-11-22 | 1.2 | Realm-scoped keyring keys use `__` between realm and identity (#3720). Unambiguous previous-layout entries are migrated to the new key on read. Ambiguous previous-layout entries holding `aws/user` long-lived keys are adopted only after the user confirms interactively. Cached chain steps carry a configuration digest and are reused only when it matches. |
