# Okta Authentication Identity PRD

## Executive Summary

This document defines the implementation of native Okta authentication as a first-class identity provider in Atmos. Unlike the existing SAML-based Okta integration (which uses Okta as a SAML IdP for AWS), this PRD introduces a dedicated `okta/*` identity provider that enables direct Okta API access, token management, and integration with Okta-centric workflows.

**Status:** Planned - This PRD defines the implementation plan for native Okta identity support.

**Goal:** Enable organizations using Okta as their primary identity platform to authenticate directly with Okta, obtain access tokens, and use those credentials for downstream operations such as AWS OIDC federation, API access, and Terraform provider authentication.

## Problem Statement

### Background

Many organizations use Okta as their central identity platform for:
- **Single Sign-On (SSO)**: Federated access to AWS, Azure, GCP, and SaaS applications
- **API Access Management**: Okta OAuth/OIDC tokens for API authentication
- **Developer Tooling**: CLI tools that integrate with Okta for authentication
- **Multi-Cloud Federation**: Using Okta as the central IdP for AWS IAM Identity Center, Azure AD, and GCP Workload Identity

### Current Limitations

Atmos currently supports Okta only as a SAML Identity Provider (IdP) through the `aws/saml` provider:

```yaml
# Current: Okta as SAML IdP for AWS
auth:
  providers:
    okta-saml:
      kind: aws/saml
      url: "https://company.okta.com/app/saml"
      region: us-east-1
```

**Limitations:**
1. **SAML-only**: Only supports SAML assertions for AWS, not OAuth/OIDC tokens
2. **AWS-specific**: Cannot use Okta tokens for non-AWS services
3. **No native Okta API access**: Cannot call Okta APIs directly (user info, groups, etc.)
4. **Limited token management**: SAML assertions are short-lived and not cacheable
5. **Browser dependency**: Requires browser automation for SAML flow
6. **No device authorization flow**: Cannot use modern OAuth Device Authorization Grant

### Desired State

Organizations want to:
1. **Authenticate natively with Okta** using Device Authorization Grant (no browser required for CLI)
2. **Obtain Okta access tokens** that can be used for:
   - AWS OIDC federation via `AssumeRoleWithWebIdentity`
   - Azure federated workload identity
   - GCP workload identity federation
   - Direct Okta API calls (user info, groups, applications)
   - Third-party services with Okta OIDC support
3. **Cache and refresh tokens** automatically like AWS SSO and Azure device code providers
4. **Use consistent patterns** across all identity providers (XDG compliance, file isolation)

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

3. **[Azure Authentication File Isolation](./azure-auth-file-isolation.md)** - **Parallel Implementation**
   - Shows Azure device code implementation
   - Demonstrates OAuth/OIDC token caching patterns
   - Similar authentication flow to Okta

4. **[Auth Context and Multi-Identity Support PRD](./auth-context-multi-identity.md)**
   - Defines `AuthContext` as runtime execution context
   - Establishes pattern for passing credentials to in-process SDK calls
   - Explains multi-provider support (AWS + Okta + Azure simultaneously)

5. **[XDG Base Directory Specification PRD](./xdg-base-directory-specification.md)**
   - Establishes XDG compliance across Atmos
   - Defines platform-aware directory resolution
   - CLI tools use `~/.config` on all platforms

## Design Goals

### Primary Goals

1. **Native Okta Authentication**: Implement OAuth 2.0 Device Authorization Grant for CLI authentication
2. **Token Management**: Cache access tokens and refresh tokens with automatic refresh
3. **XDG Compliance**: Follow the [Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md)
4. **Multi-Cloud Federation**: Enable Okta tokens to federate to AWS, Azure, and GCP
5. **API Access**: Enable direct Okta API access using obtained tokens

### Secondary Goals

6. **Okta-Specific Identity Types**: Support `okta/device-code`, `okta/api-token`, and `okta/service-account`
7. **Group-Based Role Mapping**: Map Okta groups to AWS roles, Azure subscriptions, etc.
8. **Session Management**: Support Okta session policies and token lifetimes
9. **Developer Experience**: Seamless authentication flow similar to `az login` or `aws sso login`

## Use Cases

### Use Case 1: AWS OIDC Federation via Okta

**Scenario:** Organization uses Okta as their central IdP and wants to federate to AWS using OIDC.

```yaml
# atmos.yaml
auth:
  providers:
    okta-oidc:
      kind: okta/device-code
      spec:
        org_url: "https://company.okta.com"
        client_id: "0oa1234567890abcdefg"
        scopes:
          - openid
          - profile
          - groups

  identities:
    aws-dev:
      kind: aws/assume-role
      provider: okta-oidc
      principal:
        assume_role: "arn:aws:iam::123456789012:role/OktaFederated"
        # Role trust policy expects Okta OIDC tokens
```

**Flow:**
1. User runs `atmos auth login aws-dev`
2. Atmos initiates Okta Device Authorization Grant
3. User authenticates in browser, approves device
4. Atmos receives Okta access token and ID token
5. Atmos calls `AssumeRoleWithWebIdentity` with Okta ID token
6. AWS returns temporary credentials
7. User runs Terraform with federated AWS credentials

### Use Case 2: Direct Okta API Access

**Scenario:** Terraform provider needs Okta API access for managing Okta resources.

```yaml
auth:
  providers:
    okta-admin:
      kind: okta/device-code
      spec:
        org_url: "https://company.okta.com"
        client_id: "0oa1234567890abcdefg"
        scopes:
          - openid
          - okta.users.read
          - okta.groups.manage

  identities:
    okta-terraform:
      kind: okta/api-access
      provider: okta-admin
```

**Result:**
- `OKTA_ORG_URL` set to organization URL
- `OKTA_API_TOKEN` or `OKTA_OAUTH2_ACCESS_TOKEN` set for Terraform Okta provider

### Use Case 3: Multi-Cloud Federation Hub

**Scenario:** Organization uses Okta as the central identity hub for AWS, Azure, and GCP.

```yaml
auth:
  providers:
    okta-central:
      kind: okta/device-code
      spec:
        org_url: "https://company.okta.com"
        client_id: "0oa1234567890abcdefg"
        scopes:
          - openid
          - profile
          - groups

  identities:
    aws-prod:
      kind: aws/assume-role
      provider: okta-central
      principal:
        assume_role: "arn:aws:iam::111111111111:role/OktaFederated"

    azure-prod:
      kind: azure/federated
      provider: okta-central
      principal:
        subscription_id: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
        tenant_id: "ffffffff-gggg-hhhh-iiii-jjjjjjjjjjjj"

    gcp-prod:
      kind: gcp/workload-identity
      provider: okta-central
      principal:
        project_id: "my-project"
        pool_id: "okta-pool"
        provider_id: "okta-provider"
```

## Technical Specification

### Multi-Cloud Compatibility Design

This section clarifies how the Okta implementation supports future Azure and GCP federation without requiring changes to the Okta provider itself.

#### Abstraction Strategy

The existing `aws/assume-role` identity already supports OIDC federation via the `OIDCCredentials` interface (see `pkg/auth/identities/aws/assume_role.go:152-155`). The same pattern applies for Azure and GCP:

| Cloud | Future Identity Kind | Federation Method | Existing Pattern Reference |
|-------|---------------------|-------------------|---------------------------|
| **AWS** | `aws/assume-role` | `AssumeRoleWithWebIdentity` | ✅ Already implemented |
| **Azure** | `azure/federated` | Federated Identity Credentials | Same `OIDCCredentials` interface |
| **GCP** | `gcp/workload-identity` | Workload Identity Federation | Same `OIDCCredentials` interface |

#### How It Works

1. **Okta provider authenticates** → Returns `OktaCredentials` (with ID token, access token, refresh token)
2. **For cloud federation**: The Okta provider's ID token is passed to cloud identities as `OIDCCredentials`
3. **Cloud identity authenticates**: Uses `AssumeRoleWithWebIdentity` (AWS), federated credentials (Azure), or workload identity (GCP)

The cloud identities detect `OIDCCredentials` and handle the federation automatically. This enables the same Okta configuration to work with any cloud provider that implements OIDC federation support.

#### Future Azure/GCP Implementation

When implementing Azure or GCP federation, the cloud identity (not the Okta provider) will handle the token exchange:

```go
// Future: pkg/auth/identities/azure/federated.go
func (i *federatedIdentity) Authenticate(ctx context.Context, baseCreds types.ICredentials) (types.ICredentials, error) {
    if oidcCreds, ok := baseCreds.(*types.OIDCCredentials); ok {
        // Exchange OIDC token for Azure credentials
        return i.exchangeOIDCForAzureToken(ctx, oidcCreds)
    }
    // ... handle other credential types
}
```

This means the Okta provider implementation in this PRD requires **no changes** to support future Azure/GCP federation.

### Implementation Mapping (AWS/Azure → Okta)

This table shows the direct parallels between existing implementations and the new Okta implementation:

| Aspect | AWS Implementation | Azure Implementation | Okta Implementation |
|--------|-------------------|---------------------|---------------------|
| **Base Directory** | `~/.config/atmos/aws/` | `~/.config/atmos/azure/` | `~/.config/atmos/okta/` |
| **Provider Directory** | `aws-sso/`, `aws-user/` | `azure-oidc/`, `azure-sp/` | `okta-oidc/`, `okta-api/` |
| **Credential Files** | `credentials` (INI)<br/>`config` (INI) | `msal_token_cache.json`<br/>`azureProfile.json` | `tokens.json`<br/>`config.json` |
| **Primary Isolation Env Var** | `AWS_SHARED_CREDENTIALS_FILE`<br/>`AWS_CONFIG_FILE` | `AZURE_CONFIG_DIR` | `OKTA_CONFIG_DIR`<br/>`OKTA_OAUTH2_ACCESS_TOKEN` |
| **Config Env Vars** | `AWS_PROFILE`<br/>`AWS_REGION` | `AZURE_SUBSCRIPTION_ID`<br/>`AZURE_TENANT_ID` | `OKTA_ORG_URL`<br/>`OKTA_BASE_URL` |
| **Cleared Env Vars** | `AWS_ACCESS_KEY_ID`<br/>`AWS_SECRET_ACCESS_KEY` | `AZURE_CLIENT_ID`<br/>`AZURE_CLIENT_SECRET` | `OKTA_API_TOKEN`<br/>`OKTA_CLIENT_ID`<br/>`OKTA_PRIVATE_KEY` |
| **Terraform Provider** | `hashicorp/aws` | `hashicorp/azurerm` | `okta/okta` |
| **Auth Flow** | SSO OIDC | Device Code | Device Code |
| **Token Refresh** | Built into SDK | Manual refresh | Manual refresh via `offline_access` |
| **File Manager** | `pkg/auth/cloud/aws/files.go` | `pkg/auth/cloud/azure/files.go` | `pkg/auth/cloud/okta/files.go` |
| **Setup Functions** | `pkg/auth/cloud/aws/setup.go` | `pkg/auth/cloud/azure/setup.go` | `pkg/auth/cloud/okta/setup.go` |
| **Env Preparation** | `pkg/auth/cloud/aws/env.go` | `pkg/auth/cloud/azure/env.go` | `pkg/auth/cloud/okta/env.go` |
| **Auth Context** | `AWSAuthContext{...}` | `AzureAuthContext{...}` | `OktaAuthContext{...}` |
| **Clean Logout** | `rm -rf ~/.config/atmos/aws/{provider}` | `rm -rf ~/.config/atmos/azure/{provider}` | `rm -rf ~/.config/atmos/okta/{provider}` |
| **User Directory** | `~/.aws/` (never modified) | `~/.azure/` (never modified) | N/A (Okta CLI not common) |

**Key Differences from AWS/Azure:**

1. **No ambient credentials**: Unlike AWS/Azure, Okta doesn't have a standard CLI configuration directory to avoid
2. **OAuth 2.0 native**: Uses standard OAuth 2.0 device flow (similar to Azure device code)
3. **Token-based**: Uses JWT tokens rather than session credentials
4. **Multi-purpose tokens**: Same token can federate to AWS, Azure, GCP, or access Okta APIs directly

### Provider Types

#### `okta/device-code` Provider

Uses OAuth 2.0 Device Authorization Grant for interactive CLI authentication.

**Configuration:**
```yaml
auth:
  providers:
    okta-oidc:
      kind: okta/device-code
      spec:
        # Required: Okta organization URL
        org_url: "https://company.okta.com"

        # Required: OAuth client ID (from Okta application)
        client_id: "0oa1234567890abcdefg"

        # Optional: OAuth scopes (defaults shown)
        scopes:
          - openid
          - profile
          - offline_access  # Required for refresh tokens

        # Optional: Authorization server (default: "default")
        authorization_server: "default"

        # Optional: Custom file storage location
        files:
          base_path: ""  # Empty = use XDG default

        # Optional: Session configuration
        session:
          duration: "8h"  # Token cache duration
```

**Authentication Flow:**
1. Atmos calls Okta's `/oauth2/{server}/v1/device/authorize` endpoint
2. Okta returns `device_code`, `user_code`, and `verification_uri`
3. Atmos displays verification URL and user code to user
4. User opens URL in browser, enters code, authenticates with Okta
5. Atmos polls `/oauth2/{server}/v1/token` until user completes authentication
6. Okta returns `access_token`, `id_token`, and `refresh_token`
7. Atmos caches tokens in XDG-compliant location

#### `okta/api-token` Provider (Future)

Uses pre-generated Okta API tokens for non-interactive authentication.

```yaml
auth:
  providers:
    okta-api:
      kind: okta/api-token
      spec:
        org_url: "https://company.okta.com"
        # Token retrieved from environment or keyring
        token_env: "OKTA_API_TOKEN"
```

#### `okta/service-account` Provider (Future)

Uses Okta service account (OAuth client credentials) for machine-to-machine authentication.

```yaml
auth:
  providers:
    okta-service:
      kind: okta/service-account
      spec:
        org_url: "https://company.okta.com"
        client_id: "0oa1234567890abcdefg"
        # Client secret from keyring or environment
        client_secret_env: "OKTA_CLIENT_SECRET"
```

### Identity Types

#### `okta/api-access` Identity

Provides direct Okta API access using provider tokens.

```yaml
auth:
  identities:
    okta-admin:
      kind: okta/api-access
      provider: okta-oidc
      # No additional principal configuration needed
```

**Environment Variables Set:**
- `OKTA_ORG_URL` - Okta organization URL
- `OKTA_OAUTH2_ACCESS_TOKEN` - OAuth access token (for API calls)
- `OKTA_BASE_URL` - Same as org URL (Terraform provider compatibility)

### File Isolation Pattern

Following the [Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md):

#### Directory Structure

```text
~/.config/atmos/okta/           # XDG_CONFIG_HOME/atmos/okta
├── okta-oidc/                  # Provider name
│   ├── tokens.json             # OAuth tokens (access, refresh, id)
│   ├── config.json             # Provider configuration cache
│   └── cache/                  # Token cache directory
│       └── token_cache.json    # Cached token responses
└── okta-api/                   # Different provider
    └── tokens.json
```

#### Token Storage Format

**`tokens.json`:**
```json
{
  "access_token": "eyJhbGciOiJSUzI1NiIs...",
  "token_type": "Bearer",
  "expires_in": 3600,
  "expires_at": "2025-01-15T12:00:00Z",
  "refresh_token": "abc123...",
  "refresh_token_expires_at": "2025-01-22T10:00:00Z",
  "id_token": "eyJhbGciOiJSUzI1NiIs...",
  "scope": "openid profile offline_access"
}
```

#### File Permissions

| File/Directory | Permissions | Rationale |
|----------------|-------------|-----------|
| Provider directory | `0o700` | Owner-only access |
| `tokens.json` | `0o600` | Sensitive credentials |
| `config.json` | `0o600` | May contain client IDs |
| Cache directory | `0o700` | Owner-only access |

### Environment Variable Strategy

#### Primary Isolation Variables

**`OKTA_CONFIG_DIR`** - Okta configuration directory
- Example: `/home/user/.config/atmos/okta/okta-oidc`
- Purpose: Points Okta SDK/CLI to Atmos-managed tokens
- Note: Custom variable for Atmos (Okta SDK doesn't have standard config dir)

#### Configuration Variables

**`OKTA_ORG_URL`** - Okta organization URL
- Example: `https://company.okta.com`
- Used by: Okta Terraform provider, Okta CLI, Okta SDKs

**`OKTA_OAUTH2_ACCESS_TOKEN`** - OAuth 2.0 access token
- Used by: Okta Terraform provider (OAuth mode)
- Preferred over API tokens for short-lived operations

**`OKTA_API_TOKEN`** - Long-lived API token
- Used by: Okta Terraform provider (API token mode)
- Only set when using `okta/api-token` provider

**`OKTA_BASE_URL`** - Base URL (alias for org URL)
- Used by: Some Okta integrations

#### Variables Cleared

The following are **cleared** to prevent conflicts:
- `OKTA_API_TOKEN` (when using OAuth mode)
- `OKTA_CLIENT_ID` (to prevent ambient credentials)
- `OKTA_PRIVATE_KEY` (to prevent ambient credentials)
- `OKTA_SCOPES` (controlled by provider config)

### Code Architecture

The implementation follows the [Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md). See `pkg/auth/cloud/aws/` and `pkg/auth/cloud/azure/` for reference implementations.

#### Key Types

**Token Storage (`pkg/auth/cloud/okta/types.go`):**
```go
// OktaTokens holds OAuth tokens returned by Okta.
type OktaTokens struct {
	AccessToken           string    `json:"access_token"`
	TokenType             string    `json:"token_type"`
	ExpiresIn             int       `json:"expires_in"`
	ExpiresAt             time.Time `json:"expires_at"`
	RefreshToken          string    `json:"refresh_token,omitempty"`
	RefreshTokenExpiresAt time.Time `json:"refresh_token_expires_at,omitempty"`
	IDToken               string    `json:"id_token,omitempty"`
	Scope                 string    `json:"scope,omitempty"`
}

func (t *OktaTokens) IsExpired() bool
func (t *OktaTokens) CanRefresh() bool
```

**Credentials (`pkg/auth/types/okta_credentials.go`):**
```go
// OktaCredentials implements ICredentials for Okta.
type OktaCredentials struct {
	OrgURL       string    `json:"org_url"`
	AccessToken  string    `json:"access_token"`
	IDToken      string    `json:"id_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
	Scope        string    `json:"scope,omitempty"`
}

// Implements ICredentials interface.
func (c *OktaCredentials) IsExpired() bool
func (c *OktaCredentials) GetExpiration() (*time.Time, error)
func (c *OktaCredentials) BuildWhoamiInfo(info *WhoamiInfo)
func (c *OktaCredentials) Validate(ctx context.Context) (*ValidationInfo, error)
```

**Auth Context (`pkg/schema/schema.go`):**
```go
// OktaAuthContext holds Okta-specific authentication context.
type OktaAuthContext struct {
	ConfigDir   string `json:"config_dir" yaml:"config_dir"`
	TokensFile  string `json:"tokens_file" yaml:"tokens_file"`
	OrgURL      string `json:"org_url" yaml:"org_url"`
	AccessToken string `json:"access_token,omitempty" yaml:"access_token,omitempty"`
	IDToken     string `json:"id_token,omitempty" yaml:"id_token,omitempty"`
}
```

#### File Manager (`pkg/auth/cloud/okta/files.go`)

```go
// OktaFileManager provides helpers to manage Okta config/token files.
// Uses sync.Mutex for thread safety and flock for file locking.
type OktaFileManager struct {
	baseDir string
	mu      sync.Mutex
}

func NewOktaFileManager(basePath string) (*OktaFileManager, error)
func (m *OktaFileManager) GetBaseDir() string
func (m *OktaFileManager) GetDisplayPath() string
func (m *OktaFileManager) GetProviderDir(providerName string) string
func (m *OktaFileManager) GetTokensPath(providerName string) string
func (m *OktaFileManager) WriteTokens(providerName string, tokens *OktaTokens) error
func (m *OktaFileManager) LoadTokens(providerName string) (*OktaTokens, error)
func (m *OktaFileManager) Cleanup(providerName string) error
func (m *OktaFileManager) CleanupAll() error
func (m *OktaFileManager) TokensExist(providerName string) bool
```

#### Device Code Provider (`pkg/auth/providers/okta/device_code.go`)

```go
// deviceCodeProvider implements the Provider interface for Okta device code flow.
type deviceCodeProvider struct {
	name                string
	config              *schema.Provider
	orgURL              string
	clientID            string
	scopes              []string
	authorizationServer string
	fileManager         *oktaCloud.OktaFileManager
}

// Provider interface implementation.
func NewDeviceCodeProvider(name string, config *schema.Provider) (*deviceCodeProvider, error)
func (p *deviceCodeProvider) Kind() string
func (p *deviceCodeProvider) Name() string
func (p *deviceCodeProvider) PreAuthenticate(_ authTypes.AuthManager) error
func (p *deviceCodeProvider) Authenticate(ctx context.Context) (authTypes.ICredentials, error)
func (p *deviceCodeProvider) Validate() error
func (p *deviceCodeProvider) Environment() (map[string]string, error)
func (p *deviceCodeProvider) Paths() ([]authTypes.Path, error)
func (p *deviceCodeProvider) PrepareEnvironment(ctx context.Context, environ map[string]string) (map[string]string, error)
func (p *deviceCodeProvider) Logout(ctx context.Context) error
func (p *deviceCodeProvider) GetFilesDisplayPath() string

// Internal methods.
func (p *deviceCodeProvider) tryCachedTokens(ctx context.Context) (*oktaCloud.OktaTokens, error)
func (p *deviceCodeProvider) startDeviceAuthorization(ctx context.Context) (*deviceAuthorizationResponse, error)
func (p *deviceCodeProvider) pollForToken(ctx context.Context, deviceAuth *deviceAuthorizationResponse, interval time.Duration) (*oktaCloud.OktaTokens, error)
func (p *deviceCodeProvider) refreshToken(ctx context.Context, refreshToken string) (*oktaCloud.OktaTokens, error)
```

#### Setup Functions (`pkg/auth/cloud/okta/setup.go`)

```go
func SetupFiles(params *SetupFilesParams) error
func SetAuthContext(params *SetAuthContextParams) error
func SetEnvironmentVariables(authContext *schema.AuthContext, stackInfo *schema.ConfigAndStacksInfo) error
```

#### Environment Preparation (`pkg/auth/cloud/okta/env.go`)

```go
// problematicOktaEnvVars are cleared to prevent credential conflicts.
var problematicOktaEnvVars = []string{
	"OKTA_API_TOKEN",
	"OKTA_CLIENT_ID",
	"OKTA_PRIVATE_KEY",
	"OKTA_PRIVATE_KEY_ID",
	"OKTA_SCOPES",
}

func PrepareEnvironment(cfg PrepareEnvironmentConfig) map[string]string
```

### AWS OIDC Federation Integration

When using Okta as an OIDC provider for AWS, the identity calls `AssumeRoleWithWebIdentity` with the Okta ID token:

```go
// In pkg/auth/identities/aws/assume_role.go
// When provider is okta/*, use ID token for AssumeRoleWithWebIdentity.
input := &sts.AssumeRoleWithWebIdentityInput{
    RoleArn:          aws.String(roleArn),
    RoleSessionName:  aws.String(sessionName),
    WebIdentityToken: aws.String(oktaCreds.IDToken),  // ID token, not access token
    DurationSeconds:  aws.Int32(sessionDuration),
}
```

## Required Errors

Add the following errors to `errors/errors.go`:

```go
// Okta authentication errors.
var (
	ErrOktaDeviceCodeExpired      = errors.New("Okta device code expired before user completed authentication")
	ErrOktaDeviceCodeDenied       = errors.New("Okta device code authorization was denied by user")
	ErrOktaTokenRefreshFailed     = errors.New("failed to refresh Okta token")
	ErrOktaNoIDToken              = errors.New("Okta response did not include ID token (required for federation)")
	ErrOktaAuthorizationPending   = errors.New("authorization pending - user has not completed authentication")
	ErrOktaSlowDown               = errors.New("polling too frequently - increasing interval")
)
```

**Note:** The following errors are already defined in `errors/errors.go` and should be reused:
- `ErrInvalidProviderConfig` - For invalid provider configuration
- `ErrInvalidProviderKind` - For mismatched provider kinds
- `ErrInvalidCredentials` - For invalid credential types
- `ErrAuthenticationFailed` - For general authentication failures
- `ErrInvalidAuthConfig` - For invalid auth configuration

These errors should be used in the provider implementation for clear error messaging and consistent error handling across all auth providers.

## Implementation Plan

### Phase 1: Core Infrastructure

**Tasks:**
1. Create `pkg/auth/cloud/okta/types.go` - Token types
2. Create `pkg/auth/cloud/okta/files.go` - Okta file manager with locking
3. Create `pkg/auth/cloud/okta/env.go` - Environment preparation
4. Create `pkg/auth/cloud/okta/setup.go` - Setup functions
5. Add `OktaAuthContext` to `pkg/schema/schema.go`
6. Add `OktaCredentials` to `pkg/auth/types/okta_credentials.go`
7. Add Okta-specific errors to `errors/errors.go`
8. Add unit tests for all new files

**Deliverables:**
- File isolation infrastructure for Okta
- AuthContext schema for Okta tokens
- Credential type implementing ICredentials interface

### Phase 2: Device Code Provider

**Tasks:**
1. Create `pkg/auth/providers/okta/device_code.go` - Device code provider implementing full `Provider` interface
2. Create `pkg/auth/providers/okta/device_code_ui.go` - UI helpers for device code display
3. Implement `startDeviceAuthorization()` - Calls Okta `/device/authorize` endpoint
4. Implement `pollForToken()` - Polls `/token` endpoint with proper interval handling
5. Implement `refreshToken()` - Exchanges refresh token for new tokens
6. Implement `tryCachedTokens()` - Checks and refreshes cached tokens
7. Register provider in `pkg/auth/providers/factory.go`
8. Add unit tests with mocked HTTP client
9. Add integration tests (skipped without Okta credentials)

**Deliverables:**
- Working `okta/device-code` provider
- Device authorization flow for CLI
- Token caching and automatic refresh

### Phase 3: AWS OIDC Federation

**Tasks:**
1. Update `pkg/auth/identities/aws/assume_role.go` to detect Okta providers
2. Implement `AssumeRoleWithWebIdentity` for Okta ID tokens
3. Add AWS trust policy documentation
4. Add integration tests with LocalStack

**Deliverables:**
- Okta → AWS OIDC federation working
- Documentation for AWS trust policy configuration

### Phase 4: Okta API Identity

**Tasks:**
1. Create `pkg/auth/identities/okta/api_access.go` - Okta API identity implementing full `Identity` interface:
   - `Kind()`, `GetProviderName()`, `Validate()`
   - `Authenticate()` - Pass-through provider credentials
   - `Environment()` - Returns Okta-specific env vars
   - `Paths()` - Returns token file paths
   - `PrepareEnvironment()` - Sets `OKTA_*` env vars for Terraform
   - `PostAuthenticate()` - Calls `SetAuthContext` and `SetEnvironmentVariables`
   - `Logout()` - Delegates to provider cleanup
   - `CredentialsExist()`, `LoadCredentials()` - File-based credential management
2. Register identity in `pkg/auth/identities/factory.go`
3. Add unit tests with mocked provider
4. Add integration tests for Terraform Okta provider compatibility

**Deliverables:**
- `okta/api-access` identity type
- Terraform Okta provider compatibility
- Environment variables: `OKTA_ORG_URL`, `OKTA_OAUTH2_ACCESS_TOKEN`, `OKTA_BASE_URL`

### Phase 5: Documentation and Testing

**Tasks:**
1. Create Docusaurus documentation at `website/docs/cli/configuration/auth/providers/okta.mdx`
2. Update roadmap at `website/src/data/roadmap.js`
3. Add end-to-end tests
4. Update schema files

**Deliverables:**
- Complete documentation
- Test coverage >80%

## Testing Strategy

### Unit Tests

**File Manager Tests:**
```go
func TestNewOktaFileManager_DefaultPath(t *testing.T) {
    tempDir := t.TempDir()
    t.Setenv("XDG_CONFIG_HOME", tempDir)

    mgr, err := NewOktaFileManager("")
    require.NoError(t, err)

    expected := filepath.Join(tempDir, "atmos", "okta")
    assert.Equal(t, expected, mgr.GetBaseDir())
}

func TestOktaFileManager_GetTokensPath(t *testing.T) {
    mgr := &OktaFileManager{baseDir: "/home/user/.config/atmos/okta"}
    assert.Equal(t, "/home/user/.config/atmos/okta/okta-oidc/tokens.json", mgr.GetTokensPath("okta-oidc"))
}
```

**Environment Tests:**
```go
func TestPrepareEnvironment_SetsOktaOrgURL(t *testing.T) {
    result := PrepareEnvironment(PrepareEnvironmentConfig{
        Environ: map[string]string{"OTHER": "value"},
        OrgURL:  "https://company.okta.com",
    })

    assert.Equal(t, "https://company.okta.com", result["OKTA_ORG_URL"])
    assert.Equal(t, "https://company.okta.com", result["OKTA_BASE_URL"])
    assert.Equal(t, "value", result["OTHER"])
}

func TestPrepareEnvironment_ClearsConflictingVars(t *testing.T) {
    result := PrepareEnvironment(PrepareEnvironmentConfig{
        Environ: map[string]string{
            "OKTA_API_TOKEN":  "should-be-cleared",
            "OKTA_CLIENT_ID":  "should-be-cleared",
            "OTHER":           "should-remain",
        },
        OrgURL: "https://company.okta.com",
    })

    assert.NotContains(t, result, "OKTA_API_TOKEN")
    assert.NotContains(t, result, "OKTA_CLIENT_ID")
    assert.Equal(t, "should-remain", result["OTHER"])
}
```

### Integration Tests

**Device Code Flow Test:**
```go
func TestOktaDeviceCode_EndToEnd(t *testing.T) {
    // Skip in CI without Okta credentials.
    if os.Getenv("OKTA_TEST_ORG_URL") == "" {
        t.Skip("Skipping Okta integration test: OKTA_TEST_ORG_URL not set")
    }

    // ... test device code flow with real Okta
}
```

**AWS Federation Test:**
```go
func TestOktaToAWS_OIDCFederation(t *testing.T) {
    // Use LocalStack for AWS testing.
    // Mock Okta token.
    // Verify AssumeRoleWithWebIdentity is called with correct token.
}
```

## Security Considerations

### Token Storage

1. **Tokens stored with 0o600 permissions** - Only owner can read/write
2. **Refresh tokens cached separately** - Can be revoked independently
3. **ID tokens used for federation** - Short-lived, not stored long-term
4. **No secrets in config files** - Client secrets from keyring or environment

### Attack Surface Reduction

**Before (ambient credentials):**
- `OKTA_API_TOKEN` in shell environment
- API tokens in plain text files
- No credential isolation

**After (Atmos-managed):**
- OAuth tokens in XDG-compliant location
- Automatic token refresh
- Clean logout removes all tokens
- No ambient credentials leak

### Credential Lifecycle

1. **Access tokens**: Short-lived (1 hour default), automatically refreshed
2. **Refresh tokens**: Longer-lived, stored securely, revocable
3. **ID tokens**: Used only for federation, not cached
4. **Logout**: Removes all tokens from disk

## AWS Trust Policy Configuration

For AWS OIDC federation with Okta, configure the role trust policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Federated": "arn:aws:iam::123456789012:oidc-provider/company.okta.com"
      },
      "Action": "sts:AssumeRoleWithWebIdentity",
      "Condition": {
        "StringEquals": {
          "company.okta.com:aud": "0oa1234567890abcdefg"
        }
      }
    }
  ]
}
```

**AWS OIDC Provider Setup:**
```bash
# Create OIDC provider in AWS
aws iam create-open-id-connect-provider \
  --url https://company.okta.com \
  --client-id-list 0oa1234567890abcdefg \
  --thumbprint-list 1234567890abcdef...
```

## Adherence to Universal Pattern

This implementation follows the [Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md):

| Requirement | Okta Implementation | Status |
|-------------|---------------------|--------|
| XDG Compliance | `~/.config/atmos/okta/` | Planned |
| Provider Scoping | `{provider-name}/` subdirectories | Planned |
| File Permissions | `0o700` dirs, `0o600` files | Planned |
| Primary Isolation Var | `OKTA_CONFIG_DIR`, `OKTA_ORG_URL` | Planned |
| Credential Clearing | Clears `OKTA_API_TOKEN`, etc. | Planned |
| File Manager | `pkg/auth/cloud/okta/files.go` | Planned |
| Setup Functions | `pkg/auth/cloud/okta/setup.go` | Planned |
| Environment Prep | `pkg/auth/cloud/okta/env.go` | Planned |
| Auth Context | `OktaAuthContext` | Planned |
| Test Coverage | >80% coverage | Planned |

## Success Metrics

1. **Native Authentication**: Users can authenticate with Okta via Device Authorization Grant
2. **Token Management**: Access tokens cached and refreshed automatically
3. **AWS Federation**: Okta ID tokens successfully federate to AWS via OIDC
4. **Terraform Compatibility**: Okta Terraform provider works with Atmos-managed tokens
5. **XDG Compliance**: All files in `~/.config/atmos/okta/`
6. **Test Coverage**: >80% coverage for Okta auth code
7. **Documentation**: Complete Docusaurus documentation

## Implementation Checklist

### Phase 1: Core Infrastructure
- [ ] Create `pkg/auth/cloud/okta/types.go` - Token types (`OktaTokens`)
- [ ] Create `pkg/auth/cloud/okta/files.go` - Okta file manager with locking
- [ ] Create `pkg/auth/cloud/okta/env.go` - Environment preparation (`PrepareEnvironment`)
- [ ] Create `pkg/auth/cloud/okta/setup.go` - Setup functions (`SetupFiles`, `SetAuthContext`, `SetEnvironmentVariables`)
- [ ] Add `OktaAuthContext` type definition to `pkg/schema/schema.go`
- [ ] Add `Okta *OktaAuthContext` field to `AuthContext` struct in `pkg/schema/schema.go`
- [ ] Create `pkg/auth/types/okta_credentials.go` - Credential type implementing `ICredentials`
- [ ] Add Okta-specific errors to `errors/errors.go`
- [ ] Add tests for file manager (`pkg/auth/cloud/okta/files_test.go`)
- [ ] Add tests for setup functions (`pkg/auth/cloud/okta/setup_test.go`)
- [ ] Add tests for environment variables (`pkg/auth/cloud/okta/env_test.go`)

### Phase 2: Device Code Provider
- [ ] Create `pkg/auth/providers/okta/device_code.go` - Full `Provider` interface implementation
- [ ] Create `pkg/auth/providers/okta/device_code_ui.go` - UI helpers for device code display
- [ ] Implement `startDeviceAuthorization()` - Calls Okta `/device/authorize` endpoint
- [ ] Implement `pollForToken()` - Polls `/token` endpoint with proper interval handling
- [ ] Implement `refreshToken()` - Exchanges refresh token for new tokens
- [ ] Implement `tryCachedTokens()` - Checks and refreshes cached tokens
- [ ] Add provider spec parsing for `files.base_path`
- [ ] Register provider in `pkg/auth/providers/factory.go`
- [ ] Add unit tests with mocked HTTP client
- [ ] Add integration tests (skipped without Okta credentials)

### Phase 3: AWS OIDC Federation
- [ ] Update `pkg/auth/identities/aws/assume_role.go` to detect Okta providers
- [ ] Implement `AssumeRoleWithWebIdentity` for Okta ID tokens
- [ ] Add AWS trust policy documentation
- [ ] Add integration tests with LocalStack

### Phase 4: Okta API Identity
- [ ] Create `pkg/auth/identities/okta/api_access.go` - Full `Identity` interface implementation
- [ ] Implement all identity interface methods
- [ ] Register identity in `pkg/auth/identities/factory.go`
- [ ] Add unit tests with mocked provider
- [ ] Add integration tests for Terraform Okta provider compatibility

### Phase 5: Documentation and Testing
- [ ] Create Docusaurus documentation at `website/docs/cli/configuration/auth/providers/okta.mdx`
- [ ] Update roadmap at `website/src/data/roadmap.js`
- [ ] Add end-to-end tests
- [ ] Update schema files in `pkg/datafetcher/schema/`
- [ ] Create blog post for release
- [ ] Achieve >80% test coverage

## Documentation Updates

### User Documentation

1. **Okta Authentication Guide** (`website/docs/cli/configuration/auth/providers/okta.mdx`):
   ```markdown
   ## File Storage Location

   Atmos stores Okta credentials in XDG-compliant directories:

   - **Linux/macOS**: `~/.config/atmos/okta/{provider-name}/`
   - **Windows**: `%APPDATA%\atmos\okta\{provider-name}\`

   These directories contain:
   - `tokens.json` - OAuth tokens (access, refresh, ID)
   - `config.json` - Provider configuration

   ### Custom Storage Location

   Override the default location with provider configuration:

   ```yaml
   auth:
     providers:
       okta-oidc:
         kind: okta/device-code
         spec:
           org_url: "https://company.okta.com"
           client_id: "..."
           files:
             base_path: "~/custom/okta"  # Custom location
   ```

2. **Global Flags Reference** (`website/docs/cli/global-flags.mdx`):
   ```markdown
   ### Okta-Specific Environment Variables

   <dl>
     <dt>`OKTA_ORG_URL`</dt>
     <dd>
       Okta organization URL. Automatically set by Atmos.
       Example: `https://company.okta.com`
     </dd>

     <dt>`OKTA_OAUTH2_ACCESS_TOKEN`</dt>
     <dd>
       OAuth 2.0 access token for Okta API access.
       Automatically set by Atmos after authentication.
     </dd>
   </dl>
   ```

### Developer Documentation

1. **Update this PRD** with implementation details and lessons learned
2. **Code comments** in new files referencing this PRD
3. **Package documentation** (`pkg/auth/cloud/okta/files.go` godoc)

## Related Documents

1. **[Universal Identity Provider File Isolation Pattern](./auth-file-isolation-pattern.md)** - Canonical pattern (REQUIRED READING)
2. **[AWS Authentication File Isolation](./aws-auth-file-isolation.md)** - Reference implementation
3. **[Azure Authentication File Isolation](./azure-auth-file-isolation.md)** - Azure implementation
4. **[Auth Context and Multi-Identity Support PRD](./auth-context-multi-identity.md)** - AuthContext design
5. **[XDG Base Directory Specification PRD](./xdg-base-directory-specification.md)** - XDG compliance patterns
6. **[Okta OAuth 2.0 Device Authorization](https://developer.okta.com/docs/guides/device-authorization-grant/main/)** - Okta documentation

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2025-12-30 | 1.0 | Initial PRD for Okta authentication identity support |
