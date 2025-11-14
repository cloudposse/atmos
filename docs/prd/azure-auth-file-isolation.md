# Azure Authentication File Isolation PRD

## Executive Summary

This document defines the implementation of Azure authentication file isolation following the [Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md). Azure must implement the same XDG-compliant credential isolation pattern as [AWS](./aws-auth-file-isolation.md) to ensure consistency across all cloud providers.

**Status:** 🚧 **Planned** - This PRD defines the implementation plan for Azure file isolation.

**Goal:** Migrate Azure authentication from writing to `~/.azure/` (user's directory) to `~/.config/atmos/azure/` (Atmos-managed directory) for credential isolation, clean logout, and XDG compliance.

## Problem Statement

### Background

Atmos authentication was designed with a security model that protects developer's personal credentials:

**Typical Developer Setup:**
- **Personal hobby accounts**: Developers have their own Azure subscriptions for personal projects, learning, experimentation
  - Configured with: `az login` (stores in `~/.azure/`)
  - Used for: Side projects, tutorials, certification practice
  - Must remain untouched: Atmos should never interfere with personal accounts

- **Enterprise/customer accounts**: Work-related Azure subscriptions managed through Atmos
  - Provisioned through: `atmos auth login` (should store in `~/.config/atmos/azure/`)
  - Used for: Customer deployments, enterprise infrastructure
  - Must be isolated: Separate from personal accounts

**Security Model Requirements:**
1. **Credential isolation**: Atmos-managed enterprise credentials never modify developer's personal `~/.azure/` files
2. **Clean logout**: Deleting an Atmos identity removes all work traces without affecting personal hobby account
3. **Shell session scoping**: When authenticated with a specific identity, other identities aren't available
4. **XDG compliance**: Follow platform conventions for config/data file storage

The AWS authentication implementation successfully implements this model using XDG-compliant paths and environment variable control. However, Azure authentication currently writes to user's default directories.

### Current Azure Behavior (Incorrect)

**Files Modified:**
- `~/.azure/msal_token_cache.json` - Azure CLI MSAL cache (shared with user's manual `az login`)
- `~/.azure/azureProfile.json` - Azure subscription configuration (shared with user's `az` commands)

**Problems:**
1. **Personal hobby account broken**: Atmos overwrites files the developer manually configured with `az login` for their personal Azure subscription
2. **Can't clean logout**: `atmos auth logout` deletes developer's personal Azure CLI credentials for hobby projects
3. **Work contamination**: Personal hobby account credentials leak into enterprise Terraform runs
4. **Security model violation**: Multiple enterprise identities can potentially access each other's credentials
5. **Not XDG compliant**: Uses hardcoded `~/.azure` instead of XDG config directory
6. **Inconsistent with AWS**: Different pattern from established AWS implementation

**Real Impact on Developers:**

```bash
# Developer's personal setup for hobby projects
$ az login  # Personal Azure subscription for side projects
# Credentials stored in: ~/.azure/msal_token_cache.json

# Developer starts using Atmos for work
$ atmos auth login customer-a-azure  # ❌ OVERWRITES ~/.azure/ files!

# Personal hobby project now broken
$ az account show  # ❌ Shows customer-a instead of personal account

# Logout from work
$ atmos auth logout customer-a-azure  # ❌ DELETES personal credentials too!

# Personal hobby account is gone
$ az account show  # ❌ No longer logged in - must re-login manually
```text

This is unacceptable. Developers should be able to maintain their personal Azure hobby accounts alongside Atmos-managed enterprise accounts without interference.

### Azure SDK Environment Variable Support

The Azure SDK and Azure CLI natively support environment variables for configuration, similar to AWS. Understanding these is critical for implementing file isolation.

**Native Azure SDK Environment Variables:**

| Variable | Purpose | Default | Atmos Usage |
|----------|---------|---------|-------------|
| `AZURE_CONFIG_DIR` | Azure CLI config directory | `~/.azure` | **Primary isolation mechanism** - Point to Atmos-managed directory |
| `AZURE_TENANT_ID` | Azure AD tenant ID | (none) | Set to identity's tenant |
| `AZURE_SUBSCRIPTION_ID` | Default subscription | (none) | Set to identity's subscription |
| `AZURE_CLIENT_ID` | Service principal client ID | (none) | **Cleared** (conflicts with CLI auth) |
| `AZURE_CLIENT_SECRET` | Service principal secret | (none) | **Cleared** (conflicts with CLI auth) |
| `AZURE_CLIENT_CERTIFICATE_PATH` | Certificate path | (none) | **Cleared** (conflicts with CLI auth) |
| `AZURE_LOCATION` | Default region/location | (none) | Set to identity's location |
| `ARM_TENANT_ID` | Terraform: Azure AD tenant | (none) | Set for Terraform compatibility |
| `ARM_SUBSCRIPTION_ID` | Terraform: Subscription ID | (none) | Set for Terraform compatibility |
| `ARM_USE_CLI` | Terraform: Use Azure CLI auth | `false` | **Set to `true`** (enables CLI auth) |
| `ARM_CLIENT_ID`, `ARM_CLIENT_SECRET` | Terraform: Service principal | (none) | **Cleared** (conflicts with CLI auth) |

**Key Insight:** `AZURE_CONFIG_DIR` is the Azure equivalent of AWS's `AWS_SHARED_CREDENTIALS_FILE` + `AWS_CONFIG_FILE`. Setting this single variable redirects both Azure SDK and Azure CLI to use a different configuration directory.

**How it works:**
```bash
# Without Atmos (default behavior)
AZURE_CONFIG_DIR=~/.azure
# Azure SDK/CLI reads:
#   - ~/.azure/msal_token_cache.json
#   - ~/.azure/azureProfile.json

# With Atmos (isolated)
AZURE_CONFIG_DIR=~/.config/atmos/azure/azure-oidc
# Azure SDK/CLI reads:
#   - ~/.config/atmos/azure/azure-oidc/msal_token_cache.json
#   - ~/.config/atmos/azure/azure-oidc/azureProfile.json
# User's ~/.azure/ is never touched!
```

### Pattern to Follow

Azure must implement the same file isolation pattern as AWS:

- **Isolated storage**: `~/.config/atmos/azure/{provider-name}/` (XDG-compliant)
- **Environment control**: `AZURE_CONFIG_DIR` redirects SDK to Atmos paths
- **Clean logout**: Delete provider directory without touching user's `~/.azure/` files
- **Multi-provider**: Multiple Azure providers coexist in separate directories

See **[Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md)** for requirements and **[AWS Authentication File Isolation](./aws-auth-file-isolation.md)** for reference implementation.

## Design Goals

### Primary Goals

1. **Match AWS isolation pattern**: Use identical architecture for consistency
2. **XDG compliance**: Follow [XDG Base Directory Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)
3. **Environment variable control**: Use Azure SDK environment variables to direct SDK to Atmos-managed files
4. **Clean logout**: Delete provider directory to remove all traces
5. **Security model preservation**: Maintain shell session identity scoping

### Secondary Goals

6. **Establish universal pattern**: This becomes the reference for GCP, GitHub, and all future providers
7. **Provider configurability**: Support `files.base_path` for custom locations
8. **Testing isolation**: Enable test hermetic isolation with environment overrides
9. **Backward compatibility**: Continue reading from legacy paths as fallback with migration warnings

## Related PRDs

This PRD implements patterns defined in:

1. **[Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md)** - **REQUIRED READING**
   - Defines canonical file isolation pattern for all providers
   - Establishes XDG compliance requirements
   - Documents code architecture pattern (FileManager, Setup, Environment)
   - Provides implementation workflow

2. **[AWS Authentication File Isolation](./aws-auth-file-isolation.md)** - **Reference Implementation**
   - Shows how AWS implements the universal pattern
   - Demonstrates environment variable strategy
   - Provides working example of file manager, setup, and environment preparation

3. **[XDG Base Directory Specification PRD](./xdg-base-directory-specification.md)**
   - Establishes XDG compliance across Atmos
   - Defines platform-aware directory resolution
   - CLI tools use `~/.config` on all platforms

4. **[Auth Context and Multi-Identity Support PRD](./auth-context-multi-identity.md)**
   - Defines `AuthContext` as runtime execution context
   - Establishes pattern for passing credentials to in-process SDK calls
   - Explains why environment variables alone don't work for SDK calls

## Technical Specification

### Universal Identity Provider File Pattern

**All identity providers MUST follow this structure:**

```text
~/.config/atmos/{cloud}/       # XDG_CONFIG_HOME/atmos/{cloud}
├── {provider-name-1}/         # Provider-specific subdirectory
│   ├── credentials.*          # Credentials (format varies by provider)
│   ├── config.*               # Config (format varies by provider)
│   └── cache.*                # Token cache (format varies by provider)
└── {provider-name-2}/         # Multiple providers can coexist
    └── ...
```

**Examples:**
```text
~/.config/atmos/aws/aws-sso/credentials           # AWS SSO provider
~/.config/atmos/aws/aws-sso/config                # AWS config
~/.config/atmos/azure/azure-oidc/msal_token_cache.json   # Azure OIDC provider
~/.config/atmos/azure/azure-oidc/azureProfile.json       # Azure profile
~/.config/atmos/gcp/gcp-sa/credentials.json       # GCP service account (future)
~/.config/atmos/github/github-app/token.json      # GitHub app (future)
```

### Azure-Specific Implementation

#### File Structure

```text
~/.config/atmos/azure/         # XDG_CONFIG_HOME/atmos/azure
├── azure-oidc/                # OIDC/Device Code provider
│   ├── msal_token_cache.json  # MSAL token cache
│   └── azureProfile.json      # Azure CLI profile
└── azure-sp/                  # Service Principal provider (future)
    ├── credentials.json       # Service principal credentials
    └── config.json            # Azure config
```

#### Azure SDK Environment Variables

The Azure SDK and Azure CLI respect these environment variables:

**Primary Control Variables:**

1. **`AZURE_CONFIG_DIR`** - Azure CLI config directory (contains `msal_token_cache.json` and `azureProfile.json`)
   - Default: `~/.azure` on Unix/macOS, `%USERPROFILE%\.azure` on Windows
   - **Atmos sets this to**: `~/.config/atmos/azure/{provider-name}`
   - This is the **primary isolation mechanism** for Azure

2. **Authentication Credentials** (cleared to prevent conflicts):
   - `AZURE_CLIENT_ID` - Service principal client ID
   - `AZURE_CLIENT_SECRET` - Service principal client secret
   - `AZURE_CLIENT_CERTIFICATE_PATH` - Certificate path for cert-based auth
   - `AZURE_TENANT_ID` - Azure AD tenant ID
   - `AZURE_SUBSCRIPTION_ID` - Default subscription
   - `AZURE_FEDERATED_TOKEN_FILE` - For workload identity/OIDC
   - `AZURE_USERNAME`, `AZURE_PASSWORD` - Username/password auth

3. **Terraform/ARM Variables** (for Terraform provider compatibility):
   - `ARM_CLIENT_ID`, `ARM_CLIENT_SECRET`, `ARM_TENANT_ID`, `ARM_SUBSCRIPTION_ID`
   - `ARM_USE_CLI=true` - Enable Azure CLI authentication for Terraform (REQUIRED)
   - `ARM_USE_OIDC`, `ARM_USE_MSI` - Other auth methods (cleared)

**Key Insight:** Setting `AZURE_CONFIG_DIR` redirects both Azure SDK and Azure CLI to use Atmos-managed MSAL cache and profile, just like `AWS_SHARED_CREDENTIALS_FILE` redirects AWS SDK to Atmos-managed credentials.

### AWS to Azure Implementation Mapping

This table shows the direct parallels between AWS and Azure implementations:

| Aspect | AWS Implementation | Azure Implementation | Notes |
|--------|-------------------|---------------------|-------|
| **Base Directory** | `~/.config/atmos/aws/` | `~/.config/atmos/azure/` | Both use XDG config directory |
| **Provider Directory** | `aws-sso/`, `aws-user/` | `azure-oidc/`, `azure-sp/` | Provider name from `atmos.yaml` |
| **Credential Files** | `credentials` (INI)<br/>`config` (INI) | `msal_token_cache.json` (JSON)<br/>`azureProfile.json` (JSON) | Format differs by SDK convention |
| **Primary Isolation Env Var** | `AWS_SHARED_CREDENTIALS_FILE`<br/>`AWS_CONFIG_FILE` | `AZURE_CONFIG_DIR` | Azure uses directory, AWS uses files |
| **Config Env Vars** | `AWS_PROFILE`<br/>`AWS_REGION` | `AZURE_SUBSCRIPTION_ID`<br/>`AZURE_TENANT_ID`<br/>`AZURE_LOCATION` | Azure needs tenant + subscription |
| **Cleared Env Vars** | `AWS_ACCESS_KEY_ID`<br/>`AWS_SECRET_ACCESS_KEY` | `AZURE_CLIENT_ID`<br/>`AZURE_CLIENT_SECRET`<br/>`ARM_*` | Prevent credential conflicts |
| **Terraform Compat** | `AWS_PROFILE`<br/>`AWS_REGION` | `ARM_USE_CLI=true`<br/>`ARM_SUBSCRIPTION_ID`<br/>`ARM_TENANT_ID` | Enable provider auth |
| **Security Enhancement** | `AWS_EC2_METADATA_DISABLED=true` | N/A | Disable ambient credentials |
| **File Manager** | `pkg/auth/cloud/aws/files.go` | `pkg/auth/cloud/azure/files.go` | Same structure |
| **Setup Functions** | `pkg/auth/cloud/aws/setup.go` | `pkg/auth/cloud/azure/setup.go` | Same pattern |
| **Env Preparation** | `pkg/auth/cloud/aws/env.go` | `pkg/auth/cloud/azure/env.go` | Same logic flow |
| **Auth Context** | `AWSAuthContext{...}` | `AzureAuthContext{...}` | Parallel schemas |
| **Clean Logout** | `rm -rf ~/.config/atmos/aws/{provider}` | `rm -rf ~/.config/atmos/azure/{provider}` | Identical |
| **User Directory** | `~/.aws/` (never modified) | `~/.azure/` (never modified) | Isolation guarantee |

**Key Differences:**

1. **File Format**: AWS uses INI (AWS SDK convention), Azure uses JSON (Azure CLI convention)
2. **Isolation Mechanism**: AWS points to specific files, Azure points to a directory
3. **Identifier Requirements**: AWS uses profile+region, Azure uses tenant+subscription+location
4. **Terraform Variables**: AWS reuses SDK vars, Azure needs `ARM_USE_CLI=true` to enable CLI auth

**Parallel Design Principles:**

1. **XDG Compliance**: Both use `~/.config/atmos/{cloud}/` for credential storage
2. **Provider Scoping**: Both use provider-specific subdirectories for multi-provider support
3. **Environment Variable Control**: Both use environment variables to redirect SDKs
4. **Clean Logout**: Both enable logout by deleting a single directory
5. **User Isolation**: Both never modify user's manual configuration (`~/.aws/` or `~/.azure/`)

### Code Architecture

#### 1. Azure File Manager (`pkg/auth/cloud/azure/files.go`) - NEW FILE

Create file manager matching AWS pattern:

```go
package azure

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/cloudposse/atmos/pkg/auth/types"
    "github.com/cloudposse/atmos/pkg/config/homedir"
    log "github.com/cloudposse/atmos/pkg/logger"
    "github.com/cloudposse/atmos/pkg/perf"
    "github.com/cloudposse/atmos/pkg/xdg"
)

const (
    PermissionRWX = 0o700
    PermissionRW  = 0o600
)

// AzureFileManager provides helpers to manage Azure config/cache files.
// Follows the same pattern as pkg/auth/cloud/aws/files.go for consistency.
type AzureFileManager struct {
    baseDir string
}

// NewAzureFileManager creates a new Azure file manager instance.
// BasePath is optional and can be empty to use defaults.
// Precedence: 1) basePath parameter from provider spec, 2) XDG config directory.
//
// Default path follows XDG Base Directory Specification:
//   - Linux: $XDG_CONFIG_HOME/atmos/azure (default: ~/.config/atmos/azure)
//   - macOS: $XDG_CONFIG_HOME/atmos/azure (default: ~/.config/atmos/azure)
//   - Windows: %APPDATA%\atmos\azure
//
// Respects ATMOS_XDG_CONFIG_HOME and XDG_CONFIG_HOME environment variables.
func NewAzureFileManager(basePath string) (*AzureFileManager, error) {
    defer perf.Track(nil, "azure.files.NewAzureFileManager")()

    var baseDir string

    if basePath != "" {
        // Use configured path from provider spec.
        expanded, err := homedir.Expand(basePath)
        if err != nil {
            return nil, fmt.Errorf("invalid base_path %q: %w", basePath, err)
        }
        baseDir = expanded
    } else {
        // Default: Use XDG config directory for Azure files.
        // This keeps Atmos-managed Azure credentials under Atmos's namespace,
        // following the same pattern as AWS and the XDG specification.
        var err error
        baseDir, err = xdg.GetXDGConfigDir("azure", PermissionRWX)
        if err != nil {
            return nil, fmt.Errorf("failed to get XDG config directory for Azure: %w", err)
        }
    }

    return &AzureFileManager{
        baseDir: baseDir,
    }, nil
}

// GetProviderDir returns the provider-specific directory path.
// Example: ~/.config/atmos/azure/azure-oidc
func (m *AzureFileManager) GetProviderDir(providerName string) string {
    return filepath.Join(m.baseDir, providerName)
}

// GetMSALCachePath returns the path to the MSAL token cache file.
// Example: ~/.config/atmos/azure/azure-oidc/msal_token_cache.json
func (m *AzureFileManager) GetMSALCachePath(providerName string) string {
    return filepath.Join(m.GetProviderDir(providerName), "msal_token_cache.json")
}

// GetProfilePath returns the path to the Azure profile file.
// Example: ~/.config/atmos/azure/azure-oidc/azureProfile.json
func (m *AzureFileManager) GetProfilePath(providerName string) string {
    return filepath.Join(m.GetProviderDir(providerName), "azureProfile.json")
}

// GetCredentialsPath returns the path to the credentials file (for service principals).
// Example: ~/.config/atmos/azure/azure-sp/credentials.json
func (m *AzureFileManager) GetCredentialsPath(providerName string) string {
    return filepath.Join(m.GetProviderDir(providerName), "credentials.json")
}

// GetConfigPath returns the path to the config file.
// Example: ~/.config/atmos/azure/azure-oidc/config.json
func (m *AzureFileManager) GetConfigPath(providerName string) string {
    return filepath.Join(m.GetProviderDir(providerName), "config.json")
}

// GetBaseDir returns the base directory path.
func (m *AzureFileManager) GetBaseDir() string {
    return m.baseDir
}

// GetDisplayPath returns a user-friendly display path (with ~ if under home directory).
func (m *AzureFileManager) GetDisplayPath() string {
    homeDir, err := homedir.Dir()
    if err == nil && homeDir != "" && strings.HasPrefix(m.baseDir, homeDir) {
        return strings.Replace(m.baseDir, homeDir, "~", 1)
    }
    return m.baseDir
}

// Cleanup removes Azure files for the provider.
// Deletes the entire provider directory.
func (m *AzureFileManager) Cleanup(providerName string) error {
    defer perf.Track(nil, "azure.files.Cleanup")()

    providerDir := m.GetProviderDir(providerName)

    log.Debug("Cleaning up Azure files directory",
        "provider", providerName,
        "directory", providerDir)

    if err := os.RemoveAll(providerDir); err != nil {
        // If directory doesn't exist, that's not an error (already cleaned up).
        if os.IsNotExist(err) {
            log.Debug("Azure files directory already removed (does not exist)",
                "provider", providerName,
                "directory", providerDir)
            return nil
        }
        return fmt.Errorf("failed to cleanup Azure files: %w", err)
    }

    log.Debug("Successfully removed Azure files directory",
        "provider", providerName,
        "directory", providerDir)

    return nil
}

// DeleteIdentity removes the Azure config directory for a specific identity.
// This is called during logout to clean up all auth artifacts.
func (m *AzureFileManager) DeleteIdentity(ctx context.Context, providerName, identityName string) error {
    defer perf.Track(nil, "azure.files.DeleteIdentity")()

    // For Azure, each provider gets its own directory and identities are scoped within.
    // Deleting the provider directory removes all identities for that provider.
    // This matches the AWS pattern where identities are profiles within provider files.
    return m.Cleanup(providerName)
}

// CleanupAll removes entire base directory (all providers).
func (m *AzureFileManager) CleanupAll() error {
    defer perf.Track(nil, "azure.files.CleanupAll")()

    if err := os.RemoveAll(m.baseDir); err != nil {
        // If directory doesn't exist, that's not an error (already cleaned up).
        if os.IsNotExist(err) {
            return nil
        }
        return fmt.Errorf("failed to cleanup all Azure files: %w", err)
    }

    return nil
}
```

#### 2. Azure Auth Context (`pkg/schema/schema.go`) - UPDATE EXISTING

Extend `AzureAuthContext` to include `ConfigDir`:

```go
// AzureAuthContext holds Azure-specific authentication context.
// This is populated by the Azure auth system and consumed by Azure SDK calls.
type AzureAuthContext struct {
    // ConfigDir is the absolute path to the Azure config directory managed by Atmos.
    // This directory contains msal_token_cache.json and azureProfile.json.
    // Maps to AZURE_CONFIG_DIR environment variable.
    // Example: /home/user/.config/atmos/azure/azure-oidc
    ConfigDir string `json:"config_dir" yaml:"config_dir"`

    // CredentialsFile is the absolute path to the Azure credentials file (optional).
    // Used for service principal authentication.
    // Example: /home/user/.config/atmos/azure/azure-sp/credentials.json
    CredentialsFile string `json:"credentials_file,omitempty" yaml:"credentials_file,omitempty"`

    // SubscriptionID is the Azure subscription ID.
    SubscriptionID string `json:"subscription_id" yaml:"subscription_id"`

    // TenantID is the Azure Active Directory tenant ID.
    TenantID string `json:"tenant_id" yaml:"tenant_id"`

    // Location is the Azure region/location (optional, e.g., "eastus").
    Location string `json:"location,omitempty" yaml:"location,omitempty"`
}
```

#### 3. Azure Setup Functions (`pkg/auth/cloud/azure/setup.go`) - NEW FILE

Create setup functions matching AWS pattern:

```go
package azure

import (
    "fmt"
    "os"

    "github.com/cloudposse/atmos/pkg/auth/types"
    log "github.com/cloudposse/atmos/pkg/logger"
    "github.com/cloudposse/atmos/pkg/schema"
)

// SetupFiles sets up Azure config files for the given identity.
// BasePath specifies the base directory for Azure files (from provider's files.base_path).
// If empty, uses the default XDG config path.
//
// This function is called during PostAuthenticate to write credential files.
func SetupFiles(providerName, identityName string, creds types.ICredentials, basePath string) error {
    azureCreds, ok := creds.(*types.AzureCredentials)
    if !ok {
        return nil // No Azure credentials to setup
    }

    // Create Azure file manager with configured or default path.
    fileManager, err := NewAzureFileManager(basePath)
    if err != nil {
        return fmt.Errorf("failed to create Azure file manager: %w", err)
    }

    // Ensure provider directory exists.
    providerDir := fileManager.GetProviderDir(providerName)
    if err := os.MkdirAll(providerDir, PermissionRWX); err != nil {
        return fmt.Errorf("failed to create provider directory: %w", err)
    }

    log.Debug("Azure files setup complete",
        "provider", providerName,
        "identity", identityName,
        "config_dir", providerDir,
    )

    // Note: Actual MSAL cache and Azure profile writing is handled by
    // the device code provider's updateAzureCLICache function.
    // This setup function just ensures the directory structure exists.

    return nil
}

// SetAuthContextParams contains parameters for SetAuthContext.
type SetAuthContextParams struct {
    AuthContext  *schema.AuthContext
    StackInfo    *schema.ConfigAndStacksInfo
    ProviderName string
    IdentityName string
    Credentials  types.ICredentials
    BasePath     string
}

// SetAuthContext populates the Azure auth context with Atmos-managed paths.
// This enables in-process Azure SDK calls to use Atmos-managed credentials.
//
// This function is called during PostAuthenticate to populate AuthContext,
// which is then used by SetEnvironmentVariables to derive environment variables.
func SetAuthContext(params *SetAuthContextParams) error {
    if params == nil {
        return fmt.Errorf("SetAuthContext parameters cannot be nil")
    }

    authContext := params.AuthContext
    if authContext == nil {
        return nil // No auth context to populate.
    }

    azureCreds, ok := params.Credentials.(*types.AzureCredentials)
    if !ok {
        return nil // No Azure credentials to setup.
    }

    fileManager, err := NewAzureFileManager(params.BasePath)
    if err != nil {
        return fmt.Errorf("failed to create Azure file manager: %w", err)
    }

    providerDir := fileManager.GetProviderDir(params.ProviderName)

    // Start with location from credentials.
    location := azureCreds.Location

    // Check for component-level location override from merged auth config.
    // Stack inheritance allows components to override identity configuration.
    if locationOverride := getComponentLocationOverride(params.StackInfo, params.IdentityName); locationOverride != "" {
        location = locationOverride
        log.Debug("Using component-level location override",
            "identity", params.IdentityName,
            "location", location,
        )
    }

    // Populate Azure auth context as the single source of truth.
    authContext.Azure = &schema.AzureAuthContext{
        ConfigDir:      providerDir,
        SubscriptionID: azureCreds.SubscriptionID,
        TenantID:       azureCreds.TenantID,
        Location:       location,
    }

    log.Debug("Set Azure auth context",
        "provider", params.ProviderName,
        "identity", params.IdentityName,
        "config_dir", providerDir,
        "subscription", azureCreds.SubscriptionID,
        "tenant", azureCreds.TenantID,
        "location", location,
    )

    return nil
}

// getComponentLocationOverride extracts location override from component auth config.
func getComponentLocationOverride(stackInfo *schema.ConfigAndStacksInfo, identityName string) string {
    if stackInfo == nil || stackInfo.ComponentAuthSection == nil {
        return ""
    }

    identities, ok := stackInfo.ComponentAuthSection["identities"].(map[string]any)
    if !ok {
        return ""
    }

    identityCfg, ok := identities[identityName].(map[string]any)
    if !ok {
        return ""
    }

    locationOverride, ok := identityCfg["location"].(string)
    if !ok {
        return ""
    }

    return locationOverride
}

// SetEnvironmentVariables derives Azure environment variables from AuthContext.
// This populates ComponentEnvSection/ComponentEnvList for spawned processes.
// The auth context is the single source of truth; this function derives from it.
//
// Uses PrepareEnvironment helper to ensure consistent environment setup across all commands.
// This clears conflicting credential env vars and sets AZURE_CONFIG_DIR.
//
// Parameters:
//   - authContext: Runtime auth context containing Azure credentials
//   - stackInfo: Stack configuration to populate with environment variables
func SetEnvironmentVariables(authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo) error {
    if authContext == nil || authContext.Azure == nil {
        return nil // No auth context to derive from.
    }

    if stackInfo == nil {
        return nil // No stack info to populate.
    }

    azureAuth := authContext.Azure

    // Convert existing environment section to map for PrepareEnvironment.
    environMap := make(map[string]string)
    if stackInfo.ComponentEnvSection != nil {
        for k, v := range stackInfo.ComponentEnvSection {
            if str, ok := v.(string); ok {
                environMap[k] = str
            }
        }
    }

    // Use shared PrepareEnvironment helper to get properly configured environment.
    // This clears conflicting credentials and sets AZURE_CONFIG_DIR.
    environMap = PrepareEnvironment(PrepareEnvironmentConfig{
        Environ:        environMap,
        ConfigDir:      azureAuth.ConfigDir,
        SubscriptionID: azureAuth.SubscriptionID,
        TenantID:       azureAuth.TenantID,
        Location:       azureAuth.Location,
    })

    // Replace ComponentEnvSection with prepared environment.
    // IMPORTANT: We must completely replace, not merge, to ensure deleted keys stay deleted.
    stackInfo.ComponentEnvSection = make(map[string]any, len(environMap))
    for k, v := range environMap {
        stackInfo.ComponentEnvSection[k] = v
    }

    return nil
}
```text

#### 4. Update `PrepareEnvironment` Function (`pkg/auth/cloud/azure/env.go`) - UPDATE EXISTING

Extend to accept and set `AZURE_CONFIG_DIR`:

```go
// PrepareEnvironmentConfig holds configuration for Azure environment preparation.
type PrepareEnvironmentConfig struct {
    Environ        map[string]string // Current environment variables
    ConfigDir      string            // Azure config directory (contains MSAL cache and profile)
    SubscriptionID string            // Azure subscription ID
    TenantID       string            // Azure tenant ID
    Location       string            // Azure location/region (optional)
}

// PrepareEnvironment configures environment variables for Azure SDK when using Atmos auth.
//
// This function:
//  1. Clears direct Azure credential env vars to prevent conflicts with Atmos-managed credentials
//  2. Sets AZURE_CONFIG_DIR to point to Atmos-managed directory
//  3. Sets AZURE_SUBSCRIPTION_ID, AZURE_TENANT_ID, AZURE_LOCATION
//  4. Sets ARM_* variables for Terraform provider compatibility
//  5. Sets ARM_USE_CLI=true to enable Azure CLI authentication
//
// This matches how 'az login' works - Atmos updates the MSAL cache and Azure profile,
// then Terraform providers automatically detect and use those credentials via ARM_USE_CLI.
//
// Note: Other cloud provider credentials (AWS, GCP) are NOT cleared to support multi-cloud
// scenarios such as using S3 backend for Terraform state while deploying to Azure.
//
// Returns a NEW map with modifications - does not mutate the input.
func PrepareEnvironment(cfg PrepareEnvironmentConfig) map[string]string {
    defer perf.Track(nil, "pkg/auth/cloud/azure.PrepareEnvironment")()

    log.Debug("Preparing Azure environment for Atmos-managed credentials",
        "config_dir", cfg.ConfigDir,
        "subscription", cfg.SubscriptionID,
        "tenant", cfg.TenantID,
        "location", cfg.Location,
    )

    // Create a copy to avoid mutating the input.
    result := make(map[string]string, len(cfg.Environ)+10)
    for k, v := range cfg.Environ {
        result[k] = v
    }

    // Clear problematic Azure credential environment variables.
    // These would override Atmos-managed credentials.
    // Note: We do NOT clear AWS/GCP credentials to support multi-cloud scenarios.
    for _, key := range problematicAzureEnvVars {
        if _, exists := result[key]; exists {
            log.Debug("Clearing Azure credential environment variable", "key", key)
            delete(result, key)
        }
    }

    // Set AZURE_CONFIG_DIR to Atmos-managed directory.
    // This directs Azure CLI and SDK to use Atmos-managed MSAL cache and profile.
    // This is the PRIMARY isolation mechanism for Azure, analogous to
    // AWS_SHARED_CREDENTIALS_FILE and AWS_CONFIG_FILE for AWS.
    if cfg.ConfigDir != "" {
        result["AZURE_CONFIG_DIR"] = cfg.ConfigDir
        log.Debug("Set AZURE_CONFIG_DIR to Atmos-managed directory",
            "config_dir", cfg.ConfigDir,
        )
    }

    // Set Azure subscription and tenant for Terraform providers.
    // These are required for azurerm, azuread, and azapi providers to work correctly.
    if cfg.SubscriptionID != "" {
        result["AZURE_SUBSCRIPTION_ID"] = cfg.SubscriptionID
        result["ARM_SUBSCRIPTION_ID"] = cfg.SubscriptionID
    }

    if cfg.TenantID != "" {
        result["AZURE_TENANT_ID"] = cfg.TenantID
        result["ARM_TENANT_ID"] = cfg.TenantID
    }

    if cfg.Location != "" {
        result["AZURE_LOCATION"] = cfg.Location
        result["ARM_LOCATION"] = cfg.Location
    }

    // Always use Azure CLI authentication for Terraform providers.
    // This matches how 'az login' works - it updates the MSAL cache and Azure profile,
    // then the providers automatically detect and use those credentials.
    // This approach works for all three providers: azurerm, azapi, and azuread.
    result["ARM_USE_CLI"] = "true"
    log.Debug("Set ARM_USE_CLI=true for Azure CLI authentication",
        "note", "Providers will use MSAL cache populated by Atmos at AZURE_CONFIG_DIR")

    log.Debug("Azure auth active - Terraform will use Azure CLI credentials from Atmos-managed MSAL cache",
        "config_dir", cfg.ConfigDir,
        "subscription", cfg.SubscriptionID,
        "tenant", cfg.TenantID,
    )

    return result
}
```

#### 5. Update Device Code Provider (`pkg/auth/providers/azure/device_code_cache.go`) - UPDATE EXISTING

Modify cache path functions to use file manager and respect `basePath` configuration:

```go
// updateAzureCLICache updates the Azure CLI MSAL cache and azureProfile.json
// using the provider-specific config directory from the file manager.
func (p *deviceCodeProvider) updateAzureCLICache(update cliCacheUpdate) error {
    // Get provider-specific config directory using file manager.
    // This respects the files.base_path configuration from provider spec.
    fileManager, err := NewAzureFileManager(p.basePath)
    if err != nil {
        log.Debug("Failed to create Azure file manager, using legacy path", "error", err)
        // Fallback to legacy behavior for backward compatibility
        return p.updateAzureCLICacheLegacy(update)
    }

    configDir := fileManager.GetProviderDir(p.name)

    // Ensure directory exists.
    if err := os.MkdirAll(configDir, 0o700); err != nil {
        log.Debug("Failed to create Azure config directory", "error", err, "path", configDir)
        return nil // Non-fatal
    }

    log.Debug("Updating Azure CLI cache with Atmos-managed paths",
        "provider", p.name,
        "config_dir", configDir,
    )

    // Update MSAL cache.
    msalCachePath := filepath.Join(configDir, "msal_token_cache.json")
    // ... existing cache update logic using msalCachePath ...

    // Update Azure profile.
    azureProfilePath := filepath.Join(configDir, "azureProfile.json")
    // ... existing profile update logic using azureProfilePath ...

    return nil
}

// updateAzureCLICacheLegacy handles the legacy behavior of writing to ~/.azure
// for backward compatibility and migration warnings.
func (p *deviceCodeProvider) updateAzureCLICacheLegacy(update cliCacheUpdate) error {
    // Existing implementation that writes to ~/.azure
    // This is kept as fallback during migration period
    log.Warn("Using legacy Azure config path ~/.azure - " +
        "Consider migrating to XDG-compliant paths. " +
        "Run 'atmos auth login' again to use new paths.")
    // ... existing implementation ...
}
```text

Also update the provider initialization to accept `basePath`:

```go
// deviceCodeProvider implements the azure/device-code provider.
type deviceCodeProvider struct {
    name         string
    tenantID     string
    clientID     string
    scope        string
    basePath     string  // NEW: Base path for file storage
    cacheStorage CacheStorage
}

// NewDeviceCodeProvider creates a new Azure device code provider.
func NewDeviceCodeProvider(name string, spec map[string]any) (types.Provider, error) {
    // ... existing validation ...

    // Extract base_path from spec.files.base_path (optional)
    basePath := ""
    if filesSpec, ok := spec["files"].(map[string]any); ok {
        if basePathValue, ok := filesSpec["base_path"].(string); ok {
            basePath = basePathValue
        }
    }

    return &deviceCodeProvider{
        name:         name,
        tenantID:     tenantID,
        clientID:     clientID,
        scope:        scope,
        basePath:     basePath,  // NEW
        cacheStorage: &defaultCacheStorage{},
    }, nil
}
```

### Provider Configuration

Add `files.base_path` support to provider spec:

```yaml
# atmos.yaml
auth:
  providers:
    azure-oidc:
      kind: azure/device-code
      spec:
        tenant_id: "00000000-0000-0000-0000-000000000000"
        files:
          # Optional: Override default XDG config directory
          # If not specified, uses XDG_CONFIG_HOME/atmos/azure (default: ~/.config/atmos/azure)
          # Supports tilde expansion: ~/custom/azure
          # Supports environment variables: ${HOME}/custom/azure
          base_path: ""  # Empty = use XDG default (recommended)
```text

## Universal Pattern for All Providers

**This section establishes the convention for GCP, GitHub, and all future identity providers.**

### File Manager Interface

All cloud-specific file managers SHOULD implement these methods:

```go
type FileManager interface {
    // GetProviderDir returns the provider-specific directory.
    GetProviderDir(providerName string) string

    // GetBaseDir returns the base directory path.
    GetBaseDir() string

    // GetDisplayPath returns user-friendly display path.
    GetDisplayPath() string

    // Cleanup removes files for the provider.
    Cleanup(providerName string) error

    // DeleteIdentity removes files for a specific identity.
    DeleteIdentity(ctx context.Context, providerName, identityName string) error

    // CleanupAll removes all provider directories.
    CleanupAll() error
}
```

### Setup Function Pattern

All cloud-specific setup packages SHOULD provide:

```go
// SetupFiles writes credential files to provider directory.
func SetupFiles(providerName, identityName string, creds types.ICredentials, basePath string) error

// SetAuthContext populates cloud-specific AuthContext.
func SetAuthContext(params *SetAuthContextParams) error

// SetEnvironmentVariables derives environment variables from AuthContext.
func SetEnvironmentVariables(authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo) error
```text

### Environment Variable Patterns

Each cloud provider MUST define:

1. **Primary isolation variable(s)** - The environment variable(s) that redirect SDK to Atmos-managed files
2. **Credential variables to clear** - Variables that would conflict with Atmos-managed credentials
3. **Configuration variables to set** - Subscription/project/tenant IDs, regions, etc.
4. **Terraform/tool compatibility variables** - Provider-specific variables for tool integration

**Examples:**

| Provider | Primary Isolation | Credential Clearing | Config Variables | Tool Variables |
|----------|------------------|---------------------|------------------|----------------|
| AWS | `AWS_SHARED_CREDENTIALS_FILE`<br/>`AWS_CONFIG_FILE` | `AWS_ACCESS_KEY_ID`<br/>`AWS_SECRET_ACCESS_KEY` | `AWS_PROFILE`<br/>`AWS_REGION` | `AWS_EC2_METADATA_DISABLED=true` |
| Azure | `AZURE_CONFIG_DIR` | `AZURE_CLIENT_ID`<br/>`AZURE_CLIENT_SECRET`<br/>`ARM_*` | `AZURE_SUBSCRIPTION_ID`<br/>`AZURE_TENANT_ID` | `ARM_USE_CLI=true`<br/>`ARM_SUBSCRIPTION_ID` |
| GCP (future) | `GOOGLE_APPLICATION_CREDENTIALS`<br/>`CLOUDSDK_CONFIG` | `GOOGLE_CLOUD_PROJECT`<br/>`GCP_PROJECT` | `GOOGLE_CLOUD_PROJECT`<br/>`GOOGLE_CLOUD_REGION` | `CLOUDSDK_AUTH_CREDENTIAL_FILE_OVERRIDE` |
| GitHub (future) | `GITHUB_TOKEN_FILE` (custom)<br/>`GH_CONFIG_DIR` | `GITHUB_TOKEN`<br/>`GH_TOKEN` | `GITHUB_ENTERPRISE_URL` | N/A |

## Migration Strategy

### Phase 1: Backward Compatibility (Current)

**Status Quo:**
- Azure continues writing to `~/.azure/` directory
- No XDG compliance
- No isolation from user's manual configuration

### Phase 2: Dual-Path Support (Migration Period)

**Implementation:**
1. **Write to new paths**: Always write credentials to `~/.config/atmos/azure/{provider}/`
2. **Read from both paths**: Check Atmos-managed paths first, fall back to `~/.azure/` if not found
3. **Warn on legacy usage**: Log warning when reading from `~/.azure/`:
   ```
   Using legacy Azure config path ~/.azure - credentials stored in legacy location.
   Atmos now uses XDG-compliant paths for better isolation and security.
   To migrate: Run 'atmos auth login' to re-authenticate and store credentials in the new location.
   Legacy location: ~/.azure/msal_token_cache.json
   New location: ~/.config/atmos/azure/azure-oidc/msal_token_cache.json
   ```

**Migration Command (Optional):**
```bash
$ atmos auth migrate azure

Migrating Azure credentials to XDG-compliant paths...

Found legacy credentials:
  - ~/.azure/msal_token_cache.json
  - ~/.azure/azureProfile.json

Migration options:
  [1] Copy to new location (recommended - preserves existing setup)
  [2] Move to new location (removes from ~/.azure)
  [3] Skip migration (use legacy paths)

Choice: 1

Copying credentials to ~/.config/atmos/azure/azure-oidc/
✓ Copied MSAL cache
✓ Copied Azure profile

Migration complete! Atmos will now use XDG-compliant paths.
Your existing ~/.azure files remain unchanged.
```

### Phase 3: XDG-Only (Future)

**Timeline:** TBD (after sufficient migration period)

**Changes:**
1. Remove fallback to `~/.azure/` paths
2. Remove migration warnings
3. Update documentation to reflect XDG-only behavior

## Testing Strategy

### Unit Tests

**File Manager Tests (`pkg/auth/cloud/azure/files_test.go`):**
```go
func TestNewAzureFileManager_DefaultPath(t *testing.T) {
    tempDir := t.TempDir()
    t.Setenv("XDG_CONFIG_HOME", tempDir)

    mgr, err := NewAzureFileManager("")
    require.NoError(t, err)

    expected := filepath.Join(tempDir, "atmos", "azure")
    assert.Equal(t, expected, mgr.GetBaseDir())
}

func TestNewAzureFileManager_CustomBasePath(t *testing.T) {
    customPath := "/custom/azure"

    mgr, err := NewAzureFileManager(customPath)
    require.NoError(t, err)

    assert.Equal(t, customPath, mgr.GetBaseDir())
}

func TestAzureFileManager_GetProviderDir(t *testing.T) {
    mgr := &AzureFileManager{baseDir: "/home/user/.config/atmos/azure"}

    providerDir := mgr.GetProviderDir("azure-oidc")

    assert.Equal(t, "/home/user/.config/atmos/azure/azure-oidc", providerDir)
}
```text

**Setup Tests (`pkg/auth/cloud/azure/setup_test.go`):**
```go
func TestSetAuthContext_PopulatesAzureContext(t *testing.T) {
    tempDir := t.TempDir()
    t.Setenv("XDG_CONFIG_HOME", tempDir)

    authContext := &schema.AuthContext{}
    azureCreds := &types.AzureCredentials{
        SubscriptionID: "sub-123",
        TenantID:       "tenant-456",
        Location:       "eastus",
    }

    err := SetAuthContext(&SetAuthContextParams{
        AuthContext:  authContext,
        ProviderName: "azure-oidc",
        IdentityName: "dev",
        Credentials:  azureCreds,
        BasePath:     "",
    })

    require.NoError(t, err)
    require.NotNil(t, authContext.Azure)
    assert.Equal(t, "sub-123", authContext.Azure.SubscriptionID)
    assert.Equal(t, "tenant-456", authContext.Azure.TenantID)
    assert.Equal(t, "eastus", authContext.Azure.Location)
    assert.Contains(t, authContext.Azure.ConfigDir, "atmos/azure/azure-oidc")
}
```

**Environment Variable Tests (`pkg/auth/cloud/azure/env_test.go`):**
```go
func TestPrepareEnvironment_SetsAzureConfigDir(t *testing.T) {
    environ := map[string]string{
        "OTHER_VAR": "value",
    }

    result := PrepareEnvironment(PrepareEnvironmentConfig{
        Environ:        environ,
        ConfigDir:      "/custom/azure/config",
        SubscriptionID: "sub-123",
        TenantID:       "tenant-456",
    })

    assert.Equal(t, "/custom/azure/config", result["AZURE_CONFIG_DIR"])
    assert.Equal(t, "sub-123", result["AZURE_SUBSCRIPTION_ID"])
    assert.Equal(t, "sub-123", result["ARM_SUBSCRIPTION_ID"])
    assert.Equal(t, "tenant-456", result["AZURE_TENANT_ID"])
    assert.Equal(t, "tenant-456", result["ARM_TENANT_ID"])
    assert.Equal(t, "true", result["ARM_USE_CLI"])
    assert.Equal(t, "value", result["OTHER_VAR"]) // Preserves existing vars
}

func TestPrepareEnvironment_ClearsConflictingVars(t *testing.T) {
    environ := map[string]string{
        "AZURE_CLIENT_ID":     "conflict",
        "AZURE_CLIENT_SECRET": "secret",
        "ARM_CLIENT_ID":       "arm-conflict",
        "OTHER_VAR":           "value",
    }

    result := PrepareEnvironment(PrepareEnvironmentConfig{
        Environ:        environ,
        ConfigDir:      "/azure/config",
        SubscriptionID: "sub-123",
        TenantID:       "tenant-456",
    })

    // Conflicting vars cleared
    assert.NotContains(t, result, "AZURE_CLIENT_ID")
    assert.NotContains(t, result, "AZURE_CLIENT_SECRET")
    assert.NotContains(t, result, "ARM_CLIENT_ID")

    // Other vars preserved
    assert.Equal(t, "value", result["OTHER_VAR"])
}
```text

### Integration Tests

**Full Auth Flow Test:**
```go
func TestAzureAuth_EndToEnd(t *testing.T) {
    tempDir := t.TempDir()
    t.Setenv("XDG_CONFIG_HOME", tempDir)

    // 1. Setup files
    azureCreds := &types.AzureCredentials{
        AccessToken:    "token",
        SubscriptionID: "sub-123",
        TenantID:       "tenant-456",
    }

    err := SetupFiles("azure-oidc", "dev", azureCreds, "")
    require.NoError(t, err)

    // 2. Verify directory created
    expectedDir := filepath.Join(tempDir, "atmos", "azure", "azure-oidc")
    _, err = os.Stat(expectedDir)
    require.NoError(t, err)

    // 3. Setup auth context
    authContext := &schema.AuthContext{}
    stackInfo := &schema.ConfigAndStacksInfo{}

    err = SetAuthContext(&SetAuthContextParams{
        AuthContext:  authContext,
        StackInfo:    stackInfo,
        ProviderName: "azure-oidc",
        IdentityName: "dev",
        Credentials:  azureCreds,
        BasePath:     "",
    })
    require.NoError(t, err)

    // 4. Setup environment variables
    err = SetEnvironmentVariables(authContext, stackInfo)
    require.NoError(t, err)

    // 5. Verify AZURE_CONFIG_DIR points to Atmos-managed directory
    configDir := stackInfo.ComponentEnvSection["AZURE_CONFIG_DIR"]
    assert.Equal(t, expectedDir, configDir)

    // 6. Cleanup
    mgr, err := NewAzureFileManager("")
    require.NoError(t, err)
    err = mgr.Cleanup("azure-oidc")
    require.NoError(t, err)

    // 7. Verify cleanup removed directory
    _, err = os.Stat(expectedDir)
    assert.True(t, os.IsNotExist(err))
}
```

## Security Considerations

### File Permissions

| File/Directory | Permissions | Rationale |
|----------------|-------------|-----------|
| Config directory | `0o700` | Owner-only access for sensitive credential files |
| MSAL cache | `0o600` | Owner-only read/write for token cache |
| Azure profile | `0o600` | Owner-only read/write for subscription config |

### Credential Isolation Benefits

1. **No cross-contamination**: User's `az login` doesn't affect Atmos credentials
2. **Clean audit trail**: All Atmos-managed credentials in one directory tree
3. **Secure deletion**: Remove entire provider directory for complete logout
4. **Shell session scoping**: Environment variable isolation prevents identity leakage

### Attack Surface Reduction

**Before (Writing to `~/.azure`):**
- ❌ Modifying user's Azure CLI configuration
- ❌ Potential conflicts with user's manual `az` commands
- ❌ Hard to audit which credentials came from Atmos vs manual setup

**After (XDG-compliant isolation):**
- ✅ Separate directory tree for Atmos credentials
- ✅ No modification of user's `~/.azure/` files
- ✅ Clear separation between Atmos and manual credentials
- ✅ Easy to audit: `ls ~/.config/atmos/azure/` shows all Atmos providers

## Documentation Updates

### User Documentation

1. **Azure Authentication Guide** (`website/docs/cli/commands/auth/providers/azure.mdx`):
   ```markdown
   ## File Storage Location

   Atmos stores Azure credentials in XDG-compliant directories:

   - **Linux/macOS**: `~/.config/atmos/azure/{provider-name}/`
   - **Windows**: `%APPDATA%\atmos\azure\{provider-name}\`

   These directories contain:
   - `msal_token_cache.json` - Azure MSAL token cache
   - `azureProfile.json` - Azure subscription profile

   **Important**: Atmos never modifies your `~/.azure/` directory. Your personal
   Azure CLI configuration remains untouched.

   ### Custom Storage Location

   Override the default location with provider configuration:

   ```yaml
   auth:
     providers:
       azure-oidc:
         kind: azure/device-code
         spec:
           tenant_id: "..."
           files:
             base_path: "~/custom/azure"  # Custom location
   ```
   ```

2. **Global Flags Reference** (`website/docs/cli/global-flags.mdx`):
   ```markdown
   ### Azure-Specific Environment Variables

   <dl>
     <dt>`AZURE_CONFIG_DIR`</dt>
     <dd>
       Azure CLI config directory containing MSAL cache and profile.
       Automatically set by Atmos to point to provider-specific directory.
       Example: `~/.config/atmos/azure/azure-oidc`
     </dd>
   </dl>
   ```

3. **Migration Guide** (`website/docs/cli/commands/auth/migration-guides/azure-xdg.mdx`):
   - Document legacy to XDG migration
   - Provide migration command usage
   - Troubleshooting common issues

### Developer Documentation

1. **Update this PRD** with implementation details and lessons learned
2. **Code comments** in new files referencing this PRD
3. **Package documentation** (`pkg/auth/cloud/azure/files.go` godoc)

## Implementation Checklist

### Phase 1: Core Infrastructure
- [ ] Create `pkg/auth/cloud/azure/files.go` - Azure file manager
- [ ] Update `pkg/schema/schema.go` - Extend `AzureAuthContext` with `ConfigDir`
- [ ] Create `pkg/auth/cloud/azure/setup.go` - Setup functions
- [ ] Update `pkg/auth/cloud/azure/env.go` - Add `ConfigDir` to `PrepareEnvironment`
- [ ] Add tests for file manager (`pkg/auth/cloud/azure/files_test.go`)
- [ ] Add tests for setup functions (`pkg/auth/cloud/azure/setup_test.go`)
- [ ] Add tests for environment variables (`pkg/auth/cloud/azure/env_test.go`)

### Phase 2: Provider Integration
- [ ] Update `pkg/auth/providers/azure/device_code.go` - Add `basePath` field
- [ ] Update `pkg/auth/providers/azure/device_code_cache.go` - Use file manager for cache paths
- [ ] Add provider spec parsing for `files.base_path`
- [ ] Update provider tests to verify XDG usage
- [ ] Integration test for full auth flow with XDG paths

### Phase 3: Migration Support
- [ ] Add legacy path detection and warnings
- [ ] Implement fallback to `~/.azure/` during migration period
- [ ] (Optional) Create `atmos auth migrate azure` command
- [ ] Add migration tests

### Phase 4: Documentation
- [ ] Update Azure authentication guide
- [ ] Add migration guide
- [ ] Update global flags reference
- [ ] Add troubleshooting section for path issues
- [ ] Update developer documentation

### Phase 5: Future Providers (Reference Implementation)
- [ ] Document Azure as reference pattern in this PRD
- [ ] Create template for new provider file managers
- [ ] Create template for new provider setup functions
- [ ] Update developer onboarding docs with universal pattern

## Success Metrics

1. **Consistency**: Azure matches AWS file isolation pattern ✅
2. **XDG Compliance**: Uses `~/.config/atmos/azure` on all platforms ✅
3. **No User Impact**: User's `~/.azure/` directory never modified ✅
4. **Clean Logout**: Deleting provider directory removes all traces ✅
5. **Test Coverage**: >80% coverage for new Azure files code ✅
6. **Documentation**: All new behavior documented ✅
7. **Universal Pattern**: Documented pattern for future providers ✅

## Adherence to Universal Pattern

This implementation follows the [Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md):

| Requirement | Azure Implementation | Status |
|-------------|---------------------|--------|
| XDG Compliance | `~/.config/atmos/azure/` | 🚧 Planned |
| Provider Scoping | `{provider-name}/` subdirectories | 🚧 Planned |
| File Permissions | `0o700` dirs, `0o600` files | 🚧 Planned |
| Primary Isolation Var | `AZURE_CONFIG_DIR` | 🚧 Planned |
| Credential Clearing | Clears `AZURE_CLIENT_ID`, `ARM_*`, etc. | ✅ Implemented |
| File Manager | `pkg/auth/cloud/azure/files.go` | 🚧 Planned |
| Setup Functions | `pkg/auth/cloud/azure/setup.go` | 🚧 Planned |
| Environment Prep | `pkg/auth/cloud/azure/env.go` (update) | 🚧 Planned |
| Auth Context | `AzureAuthContext` (extend) | 🚧 Planned |
| Test Coverage | >80% coverage | 🚧 Planned |

## Related Documents

1. **[Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md)** - Canonical pattern (REQUIRED READING)
2. **[AWS Authentication File Isolation](./aws-auth-file-isolation.md)** - Reference implementation
3. **[XDG Base Directory Specification PRD](./xdg-base-directory-specification.md)** - XDG compliance patterns
4. **[Auth Context and Multi-Identity Support PRD](./auth-context-multi-identity.md)** - AuthContext design and usage
5. **[XDG Specification](https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html)** - Official standard

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2025-01-XX | 1.0 | Initial PRD created for Azure file isolation |
