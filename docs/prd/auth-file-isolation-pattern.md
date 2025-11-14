# Universal Identity Provider File Isolation Pattern PRD

## Executive Summary

This document defines the canonical file isolation pattern that ALL Atmos identity providers (AWS, Azure, GCP, etc.) MUST follow. This pattern ensures credential isolation, clean logout, XDG compliance, and consistency across all cloud providers.

**Key Principle:** All identity provider implementations MUST use XDG-compliant, provider-scoped directories under Atmos management (`~/.config/atmos/{cloud}/{provider}/`) to enable clean logout, prevent credential conflicts, and maintain the security model where authenticated identities are isolated per shell session.

## Design Philosophy

### Core Requirements

Every identity provider implementation MUST satisfy these requirements:

1. **Credential Isolation**: Atmos-managed credentials never modify user's manually configured files
2. **Clean Logout**: Deleting an identity removes all traces without affecting user's personal configuration
3. **Shell Session Scoping**: When authenticated with a specific identity, other identities aren't available
4. **XDG Compliance**: Follow platform conventions for config/data file storage
5. **Multi-Provider Support**: Multiple providers of the same cloud can coexist without conflicts

### Security Model

The authentication system enforces a security boundary between:

- **Atmos-managed credentials** (`~/.config/atmos/{cloud}/`): Enterprise/customer accounts provisioned through Atmos
- **Developer's personal credentials** (`~/.aws/`, `~/.azure/`, `~/.config/gcloud/`): Hobby accounts manually configured with `aws configure`, `az login`, `gcloud init`

**Key Principle:** Atmos never modifies the developer's personal configuration. Most developers have their own hobby accounts for personal use that are manually configured in the default credential locations. Atmos-managed credentials for enterprise/customer accounts must be completely isolated to prevent any interference with personal setups.

**Namespace Separation:**

- **Atmos namespace**: `~/.config/atmos/{cloud}/` contains ONLY Atmos-provisioned enterprise/customer credentials
- **User namespace**: `~/.aws/`, `~/.azure/`, `~/.config/gcloud/` contain ONLY developer's manually configured hobby accounts
- **No cross-contamination**: Atmos operations never modify user namespace
- **Environment isolation**: Shell environment variables ensure SDK uses only Atmos credentials when authenticated

### Why File Isolation Matters

**Real-World Scenario:**

Developers typically have:
1. **Personal hobby accounts**: AWS/Azure/GCP accounts for side projects, learning, experimentation
   - Configured manually: `aws configure`, `az login`, `gcloud init`
   - Stored in default locations: `~/.aws/`, `~/.azure/`, `~/.config/gcloud/`
   - Used outside of work hours for personal projects

2. **Enterprise/customer accounts**: Work-related accounts managed through Atmos
   - Multiple customer environments (e.g., Cloud Posse managing 50+ customer accounts)
   - Provisioned through Atmos authentication
   - Must not interfere with personal accounts

**Without isolation (anti-pattern):**
```
~/.aws/credentials           # Mixed: Personal hobby account + customer accounts
~/.azure/msal_token_cache    # Shared: Personal projects + enterprise work
~/.config/gcloud/            # Can't tell: Which is personal vs work?
```

**Problems:**
- ❌ **Personal setup broken**: `atmos auth logout` deletes your hobby account credentials
- ❌ **Work contamination**: Personal hobby credentials leak into enterprise Terraform runs
- ❌ **Audit nightmare**: Hard to tell which credentials came from Atmos vs manual configuration
- ❌ **Configuration corruption**: Atmos overwrites your carefully configured personal settings

**With isolation (correct pattern):**
```
# Atmos-managed enterprise/customer credentials
~/.config/atmos/aws/customer-a-prod/      # Customer A production account
~/.config/atmos/aws/customer-b-dev/       # Customer B development account
~/.config/atmos/azure/acme-corp/          # ACME Corp Azure subscription

# Developer's personal hobby accounts (never touched by Atmos)
~/.aws/credentials                        # Personal AWS hobby account
~/.azure/msal_token_cache.json            # Personal Azure hobby account
~/.config/gcloud/                         # Personal GCP hobby account
```

**Benefits:**
- ✅ **Personal setup protected**: Your hobby accounts are never modified by Atmos
- ✅ **Clean logout**: `rm -rf ~/.config/atmos/aws/customer-a-prod/` only removes work credentials
- ✅ **Clear separation**: Work credentials in `~/.config/atmos/`, personal in default locations
- ✅ **No interference**: Can use personal accounts outside work without affecting Atmos
- ✅ **Audit trail**: All work credentials isolated under `~/.config/atmos/`

## Universal File Pattern

### Directory Structure

**All providers MUST follow this structure:**

```
~/.config/atmos/{cloud}/       # XDG_CONFIG_HOME/atmos/{cloud}
├── {provider-name-1}/         # Provider-specific subdirectory
│   ├── credentials.*          # Credentials (format varies by provider)
│   ├── config.*               # Config (format varies by provider)
│   └── cache.*                # Token cache (format varies by provider)
└── {provider-name-2}/         # Multiple providers can coexist
    └── ...
```

**Platform-specific defaults:**
- **Linux/macOS**: `~/.config/atmos/{cloud}/` (XDG convention for CLI tools)
- **Windows**: `%APPDATA%\atmos\{cloud}\`

**Examples:**
```
~/.config/atmos/aws/aws-sso/credentials           # AWS SSO provider
~/.config/atmos/aws/aws-sso/config                # AWS config
~/.config/atmos/azure/azure-oidc/msal_token_cache.json   # Azure OIDC provider
~/.config/atmos/azure/azure-oidc/azureProfile.json       # Azure profile
~/.config/atmos/gcp/gcp-sa/credentials.json       # GCP service account (future)
```

### File Permissions

**All providers MUST use these permissions:**

| File/Directory | Permissions | Rationale |
|----------------|-------------|-----------|
| Config directory (`{provider}/`) | `0o700` | Owner-only access for sensitive credential files |
| Credential files | `0o600` | Owner-only read/write for credentials |
| Token cache files | `0o600` | Owner-only read/write for tokens |
| Config files | `0o600` | Owner-only read/write for configuration |

### Environment Variable Patterns

Each cloud provider MUST define and implement:

1. **Primary Isolation Variable(s)** - The environment variable(s) that redirect SDK to Atmos-managed files
   - This is the most critical piece - the variable that makes the SDK use Atmos paths instead of defaults

2. **Credential Variables to Clear** - Variables that would conflict with Atmos-managed credentials
   - Prevents ambient credentials from leaking into Atmos sessions

3. **Configuration Variables to Set** - Subscription/project/tenant IDs, regions, etc.
   - Provider-specific identifiers required for SDK operation

4. **Terraform/Tool Compatibility Variables** - Provider-specific variables for tool integration
   - Ensures Terraform and other tools can use Atmos credentials

**Examples:**

| Provider | Primary Isolation | Credential Clearing | Config Variables | Tool Variables |
|----------|------------------|---------------------|------------------|----------------|
| AWS | `AWS_SHARED_CREDENTIALS_FILE`<br/>`AWS_CONFIG_FILE` | `AWS_ACCESS_KEY_ID`<br/>`AWS_SECRET_ACCESS_KEY` | `AWS_PROFILE`<br/>`AWS_REGION` | `AWS_EC2_METADATA_DISABLED=true` |
| Azure | `AZURE_CONFIG_DIR` | `AZURE_CLIENT_ID`<br/>`AZURE_CLIENT_SECRET`<br/>`ARM_*` | `AZURE_SUBSCRIPTION_ID`<br/>`AZURE_TENANT_ID` | `ARM_USE_CLI=true`<br/>`ARM_SUBSCRIPTION_ID` |
| GCP (future) | `GOOGLE_APPLICATION_CREDENTIALS`<br/>`CLOUDSDK_CONFIG` | `GOOGLE_CLOUD_PROJECT`<br/>`GCP_PROJECT` | `GOOGLE_CLOUD_PROJECT`<br/>`GOOGLE_CLOUD_REGION` | `CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE` |

## Code Architecture Pattern

### Required Components

Every cloud provider implementation MUST provide these three components:

#### 1. File Manager (`pkg/auth/cloud/{cloud}/files.go`)

**Purpose:** Centralize path construction and file operations.

**Required Interface:**
```go
type FileManager interface {
    // GetProviderDir returns the provider-specific directory.
    // Example: ~/.config/atmos/aws/aws-sso
    GetProviderDir(providerName string) string

    // GetBaseDir returns the base directory path.
    // Example: ~/.config/atmos/aws
    GetBaseDir() string

    // GetDisplayPath returns user-friendly display path (with ~ if under home directory).
    GetDisplayPath() string

    // Cleanup removes files for the provider.
    // Deletes the entire provider directory.
    Cleanup(providerName string) error

    // DeleteIdentity removes files for a specific identity.
    // This is called during logout to clean up all auth artifacts.
    DeleteIdentity(ctx context.Context, providerName, identityName string) error

    // CleanupAll removes entire base directory (all providers).
    CleanupAll() error
}
```

**Constructor Pattern:**
```go
// NewFileManager creates a new file manager instance.
// BasePath is optional and can be empty to use defaults.
// Precedence: 1) basePath parameter from provider spec, 2) XDG config directory.
//
// Default path follows XDG Base Directory Specification:
//   - Linux/macOS: $XDG_CONFIG_HOME/atmos/{cloud} (default: ~/.config/atmos/{cloud})
//   - Windows: %APPDATA%\atmos\{cloud}
//
// Respects ATMOS_XDG_CONFIG_HOME and XDG_CONFIG_HOME environment variables.
func NewFileManager(basePath string) (*FileManager, error) {
    var baseDir string

    if basePath != "" {
        // Use configured path from provider spec
        expanded, err := homedir.Expand(basePath)
        if err != nil {
            return nil, fmt.Errorf("invalid base_path %q: %w", basePath, err)
        }
        baseDir = expanded
    } else {
        // Default: Use XDG config directory
        var err error
        baseDir, err = xdg.GetXDGConfigDir("{cloud}", PermissionRWX)
        if err != nil {
            return nil, fmt.Errorf("failed to get XDG config directory: %w", err)
        }
    }

    return &FileManager{baseDir: baseDir}, nil
}
```

#### 2. Setup Functions (`pkg/auth/cloud/{cloud}/setup.go`)

**Purpose:** Coordinate file writing, AuthContext population, and environment derivation.

**Required Functions:**
```go
// SetupFiles sets up credential files for the given identity.
// BasePath specifies the base directory (from provider's files.base_path).
// If empty, uses the default XDG config path.
//
// This function is called during PostAuthenticate to write credential files.
func SetupFiles(providerName, identityName string, creds types.ICredentials, basePath string) error

// SetAuthContextParams contains parameters for SetAuthContext.
type SetAuthContextParams struct {
    AuthContext  *schema.AuthContext
    StackInfo    *schema.ConfigAndStacksInfo
    ProviderName string
    IdentityName string
    Credentials  types.ICredentials
    BasePath     string
}

// SetAuthContext populates the cloud-specific auth context with Atmos-managed paths.
// This enables in-process SDK calls to use Atmos-managed credentials.
//
// This function is called during PostAuthenticate to populate AuthContext,
// which is then used by SetEnvironmentVariables to derive environment variables.
func SetAuthContext(params *SetAuthContextParams) error

// SetEnvironmentVariables derives environment variables from AuthContext.
// This populates ComponentEnvSection/ComponentEnvList for spawned processes.
// The auth context is the single source of truth; this function derives from it.
//
// Uses PrepareEnvironment helper to ensure consistent environment setup.
func SetEnvironmentVariables(authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo) error
```

#### 3. Environment Preparation (`pkg/auth/cloud/{cloud}/env.go`)

**Purpose:** Build environment variable map with proper isolation.

**Required Function:**
```go
// PrepareEnvironmentConfig holds configuration for environment preparation.
type PrepareEnvironmentConfig struct {
    Environ map[string]string // Current environment variables
    // ... cloud-specific fields (ConfigDir, SubscriptionID, etc.)
}

// PrepareEnvironment configures environment variables when using Atmos auth.
//
// This function:
//  1. Clears direct credential env vars to prevent conflicts
//  2. Sets primary isolation variable(s) to point to Atmos-managed paths
//  3. Sets configuration variables (subscription/project/tenant IDs, regions, etc.)
//  4. Sets tool compatibility variables (Terraform provider variables, etc.)
//
// Returns a NEW map with modifications - does not mutate the input.
func PrepareEnvironment(cfg PrepareEnvironmentConfig) map[string]string
```

#### 4. Auth Context Schema (`pkg/schema/schema.go`)

**Purpose:** Define cloud-specific AuthContext structure.

**Required Pattern:**
```go
// {Cloud}AuthContext holds {cloud}-specific authentication context.
// This is populated by the {cloud} auth system and consumed by {cloud} SDK calls.
type {Cloud}AuthContext struct {
    // Primary isolation fields - paths to Atmos-managed files/directories
    // Example: ConfigDir, CredentialsFile, etc.

    // Configuration fields - identifiers required by SDK
    // Example: SubscriptionID, TenantID, Region, etc.
}
```

## Provider Configuration Pattern

All providers MUST support optional `files.base_path` configuration:

```yaml
# atmos.yaml
auth:
  providers:
    {provider-name}:
      kind: {cloud}/{auth-type}
      spec:
        # Provider-specific authentication config
        # ...

        files:
          # Optional: Override default XDG config directory
          # If not specified, uses XDG_CONFIG_HOME/atmos/{cloud}
          # Supports tilde expansion: ~/custom/{cloud}
          # Supports environment variables: ${HOME}/custom/{cloud}
          base_path: ""  # Empty = use XDG default (recommended)
```

## Implementation Workflow

When implementing a new cloud provider, follow these steps:

### 1. Create File Manager

```go
// pkg/auth/cloud/{cloud}/files.go
package {cloud}

type FileManager struct {
    baseDir string
}

func NewFileManager(basePath string) (*FileManager, error) {
    // Use XDG default or custom path
    // See pattern above
}

func (m *FileManager) GetProviderDir(providerName string) string {
    return filepath.Join(m.baseDir, providerName)
}

// Implement remaining FileManager interface methods
```

### 2. Define Auth Context

```go
// pkg/schema/schema.go
type {Cloud}AuthContext struct {
    // Paths to Atmos-managed files/directories
    ConfigDir string `json:"config_dir" yaml:"config_dir"`

    // Cloud-specific identifiers
    // ...
}
```

### 3. Implement Setup Functions

```go
// pkg/auth/cloud/{cloud}/setup.go
package {cloud}

func SetupFiles(providerName, identityName string, creds types.ICredentials, basePath string) error {
    // 1. Create file manager
    // 2. Ensure provider directory exists
    // 3. Write credential files in cloud-specific format
}

func SetAuthContext(params *SetAuthContextParams) error {
    // 1. Create file manager
    // 2. Get provider directory path
    // 3. Populate AuthContext.{Cloud} with paths and identifiers
}

func SetEnvironmentVariables(authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo) error {
    // 1. Extract cloud auth context
    // 2. Call PrepareEnvironment to build env map
    // 3. Replace ComponentEnvSection with prepared environment
}
```

### 4. Implement Environment Preparation

```go
// pkg/auth/cloud/{cloud}/env.go
package {cloud}

var conflicting{Cloud}EnvVars = []string{
    // List all environment variables that would conflict with Atmos credentials
}

func PrepareEnvironment(cfg PrepareEnvironmentConfig) map[string]string {
    // 1. Copy input environment (don't mutate)
    // 2. Clear conflicting credential variables
    // 3. Set primary isolation variable(s)
    // 4. Set configuration variables
    // 5. Set tool compatibility variables
    // 6. Return new map
}
```

### 5. Write Tests

```go
// pkg/auth/cloud/{cloud}/files_test.go
func TestNewFileManager_DefaultPath(t *testing.T) {
    tempDir := t.TempDir()
    t.Setenv("XDG_CONFIG_HOME", tempDir)

    mgr, err := NewFileManager("")
    require.NoError(t, err)

    expected := filepath.Join(tempDir, "atmos", "{cloud}")
    assert.Equal(t, expected, mgr.GetBaseDir())
}

// pkg/auth/cloud/{cloud}/setup_test.go
func TestSetAuthContext_PopulatesContext(t *testing.T) {
    // Test that AuthContext is populated correctly
}

// pkg/auth/cloud/{cloud}/env_test.go
func TestPrepareEnvironment_SetsIsolationVars(t *testing.T) {
    // Test that primary isolation variables are set
}

func TestPrepareEnvironment_ClearsConflictingVars(t *testing.T) {
    // Test that conflicting variables are cleared
}
```

### 6. Document Implementation

Create provider-specific PRD in `docs/prd/`:
- `{cloud}-auth-file-isolation.md` - How this provider implements the pattern
- Reference this universal pattern PRD
- Document cloud-specific environment variables
- Include migration guide if applicable

## Testing Requirements

All provider implementations MUST include:

1. **File Manager Tests**:
   - Test XDG default path resolution
   - Test custom base_path configuration
   - Test provider directory construction
   - Test cleanup operations

2. **Setup Tests**:
   - Test AuthContext population
   - Test file creation
   - Test environment variable derivation

3. **Environment Tests**:
   - Test primary isolation variables are set
   - Test conflicting variables are cleared
   - Test configuration variables are set
   - Test tool compatibility variables are set
   - Test input is not mutated (returns new map)

4. **Integration Tests**:
   - Test full auth flow (setup → context → environment)
   - Test cleanup removes all files
   - Test multi-provider coexistence

## Security Considerations

### File Permissions

All implementations MUST:
- Create directories with `0o700` (owner-only access)
- Create credential files with `0o600` (owner-only read/write)
- Never create world-readable credential files

### Environment Variable Isolation

All implementations MUST:
- Clear ambient credential variables that would override Atmos credentials
- Set primary isolation variables to redirect SDK to Atmos paths
- Return new environment map without mutating input

### Attack Surface Reduction

The pattern provides:
- **No cross-contamination**: User's manual config never modified
- **Clear audit trail**: All Atmos credentials in one directory tree
- **Secure deletion**: Remove entire provider directory for logout
- **Shell session scoping**: Environment variables prevent identity leakage

## Migration Considerations

When implementing this pattern for an existing provider that uses non-XDG paths:

1. **Phase 1: Dual-Path Support**
   - Write to new XDG paths
   - Read from both new and legacy paths (new takes precedence)
   - Warn users about legacy paths

2. **Phase 2: Migration Tool** (optional)
   - Provide `atmos auth migrate {cloud}` command
   - Copy credentials from legacy to XDG paths
   - Preserve original files

3. **Phase 3: XDG-Only** (future)
   - Remove fallback to legacy paths
   - Remove migration warnings
   - Update documentation

## Related Documents

1. **[XDG Base Directory Specification PRD](./xdg-base-directory-specification.md)** - XDG compliance patterns
2. **[Auth Context and Multi-Identity Support PRD](./auth-context-multi-identity.md)** - AuthContext design and usage
3. **Provider-specific PRDs**:
   - [AWS Authentication File Isolation](./aws-auth-file-isolation.md)
   - [Azure Authentication File Isolation](./azure-auth-file-isolation.md)
4. **[XDG Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)** - Official standard

## Success Metrics

A provider implementation successfully follows this pattern when:

1. ✅ **XDG Compliance**: Uses `~/.config/atmos/{cloud}` on all platforms
2. ✅ **User Isolation**: User's manual configuration never modified
3. ✅ **Clean Logout**: Deleting provider directory removes all traces
4. ✅ **Environment Control**: SDK uses only Atmos credentials via environment variables
5. ✅ **Multi-Provider**: Multiple providers coexist without conflicts
6. ✅ **Test Coverage**: >80% coverage for file manager, setup, and environment code
7. ✅ **Documentation**: Provider-specific PRD documents implementation

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2025-01-XX | 1.0 | Initial universal pattern PRD created |
