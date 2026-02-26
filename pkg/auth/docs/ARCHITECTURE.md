# Atmos Auth Package Architecture

## Overview

The Atmos Auth package provides a comprehensive authentication framework for cloud providers, supporting identity chaining, credential management, and environment setup for tools like Terraform. The architecture is designed to be extensible, allowing easy addition of new cloud providers and authentication methods.

## Package Structure

```
pkg/auth/
├── cloud/                    # Cloud-specific helpers
│   ├── aws/                  # AWS-specific implementation
│   │   ├── files.go          # Helpers for AWS credentials/config file management
│   │   └── setup.go          # Setup helpers used by identities
│   └── okta/                 # Okta-specific implementation
│       ├── types.go          # Token types (OktaTokens, DeviceAuthorizationResponse, etc.)
│       ├── files.go          # OktaFileManager for file-locked token caching
│       ├── setup.go          # SetupFiles, SetAuthContext, SetEnvironmentVariables
│       └── env.go            # PrepareEnvironment for Okta env var management
├── credentials/             # Credential storage
│   └── store.go             # Encrypted credential store
├── docs/                    # Documentation
│   ├── Architecture.md      # This file
│   ├── ADDING_PROVIDERS.md  # Guide for adding new providers
│   └── UserGuide.md         # User getting started guide
├── identities/              # Identity implementations
│   ├── aws/                 # AWS identity types
│   │   ├── assume_role.go   # AWS assume role identity (also handles Okta OIDC federation)
│   │   ├── permission_set.go # AWS permission set identity
│   │   └── user.go          # AWS user identity
│   └── okta/                # Okta identity types
│       └── api_access.go    # Okta API access identity (pass-through credentials)
├── providers/               # Provider implementations
│   ├── aws/                 # AWS providers
│   │   ├── saml.go          # AWS SAML provider
│   │   └── sso.go           # AWS SSO provider
│   ├── github/              # GitHub providers
│   │   └── oidc.go          # GitHub OIDC provider
│   └── okta/                # Okta providers
│       ├── device_code.go   # OAuth 2.0 Device Authorization Grant flow
│       └── device_code_ui.go # Bubbletea spinner UI and device code display
├── types/                   # Core interfaces and types
│   ├── interfaces.go        # All auth interfaces
│   └── okta_credentials.go  # OktaCredentials implementing ICredentials
├── ui/                      # User interface components (currently empty)
├── utils/                   # Shared utilities
│   └── env.go               # Helper to append env vars to stack
├── validation/              # Configuration validation
│   └── validator.go         # Auth config validator
├── factory.go               # Provider/Identity factory
├── hooks.go                 # Terraform integration hooks
├── manager.go               # Main auth manager
└── test_helpers.go          # Test utilities
```

## Core Interfaces

### AuthManager

The central orchestrator that manages the entire authentication process:

- **Authenticate()**: Performs authentication for a specified identity
- **Whoami()**: Returns information about identity credentials
- **GetDefaultIdentity()**: Handles multiple defaults with CI detection
- **GetProviderForIdentity()**: Resolves the root provider for an identity
- **GetProviderKindForIdentity()**: Returns provider kind via chain resolution
- **GetChain()**: Exposes the last-built chain for inspection

### Provider

Represents authentication providers (e.g., AWS SSO, SAML, Okta Device Code):

- **Kind()**: Returns provider kind (e.g., "aws/iam-identity-center", "okta/device-code")
- **Name()**: Returns configured provider name
- **PreAuthenticate()**: Inspect chain before auth and set preferences
- **Authenticate()**: Performs provider-level authentication
- **Validate()**: Validates provider configuration
- **Environment()**: Returns provider-level environment variables

### Identity

Represents authentication identities that use provider credentials:

- **Kind()**: Returns identity type (e.g., "aws/permission-set", "okta/api-access")
- **GetProviderName()**: Resolves root provider name for this identity
- **Authenticate()**: Performs identity-level authentication using base credentials
- **Validate()**: Validates identity configuration
- **Environment()**: Returns identity-level environment variables
- **PostAuthenticate()**: Perform file/env setup after successful auth

## Authentication Flow

### 1. Identity Chain Resolution

```
Component Request -> Identity -> Provider Chain -> Root Provider
```

Example: `terraform apply` with identity `sandbox-admin`:

1. Resolve identity chain: `sandbox-admin` -> `managers` -> `cplive-sso`
   a. `sandbox-admin` is the target identity (kind: `aws/assume-role`)
   b. `managers` is the via identity (kind: `aws/permission-set`)
   c. `cplive-sso` is the root provider (kind: `aws/iam-identity-center`)
2. Build authentication chain: `[cplive-sso, managers, sandbox-admin]`
3. Execute sequential authentication

### 2. Sequential Authentication

```go
	finalCreds, err := m.authenticateHierarchical(ctx, identityName)
```

We traverse the chain, checking for cached credentials. We start at the target identity, and if that already has cached credentials, we can use that. Otherwise, we go down until we have cached credentials, and then we refresh the credentials going back up.

If we reach the bottom, which is the provider, then we know that we have to re-authenticate the entire thing. This typically just involves authenticating with the provider again, and then assuming roles.

### 3. Environment Setup

Environment setup is performed by identities via `PostAuthenticate` using the shared cloud helpers:

**AWS example:**
```go
// Inside Identity.PostAuthenticate
aws.SetupFiles(providerName, identityName, creds)
aws.SetEnvironmentVariables(stackInfo, providerName, identityName)
```

**Okta example:**
```go
// Inside Identity.PostAuthenticate
okta.SetupFiles(providerName, identityName, creds, basePath, realm)
okta.SetAuthContext(params)
okta.SetEnvironmentVariables(authContext, stackInfo)
```

The `PostAuthenticate` hook allows each cloud provider or identity to perform cloud-specific file and environment setup.

## Okta Authentication

### Device Code Flow

The Okta device code provider (`okta/device-code`) implements the OAuth 2.0 Device Authorization Grant (RFC 8628):

```
1. Provider.Authenticate()
   |-- tryCachedTokens() -> check ~/.config/atmos/okta/{provider}/tokens.json
   |     |-- If valid: return cached tokens
   |     |-- If expired + refresh token: refreshToken()
   |     |-- Otherwise: proceed to interactive flow
   |
   |-- isInteractive() check (stderr must be TTY)
   |-- startDeviceAuthorization() -> POST /oauth2/{server}/v1/device/authorize
   |-- displayDeviceCodePrompt() -> styled UI with code + URL, opens browser
   |-- pollForTokenWithSpinner() -> Bubbletea spinner + background polling
   |-- Save tokens via OktaFileManager.WriteTokens()
   |-- Return OktaCredentials
```

### Okta API Access

The `okta/api-access` identity is a pass-through that exposes Okta credentials (access token, ID token) for downstream consumers like the Terraform Okta provider.

### AWS OIDC Federation via Okta

The `aws/assume-role` identity natively handles Okta credentials by detecting `OktaCredentials` with an `IDToken` and converting to OIDC credentials for `AssumeRoleWithWebIdentity`:

```
Provider (okta/device-code) -> OktaCredentials (with IDToken)
  |
  v
Identity (aws/assume-role) detects OktaCredentials
  |-- Converts to OIDCCredentials{Token: IDToken, Provider: "okta"}
  |-- Calls assumeRoleWithWebIdentity()
  |-- Returns AWSCredentials
```

### Token File Management

Okta tokens are cached at `~/.config/atmos/{realm}/okta/{provider}/tokens.json` with:
- File locking via `gofrs/flock` for concurrent access safety
- Restricted permissions (0600) for security
- Realm-based isolation for multi-repository credential separation
- 5-minute buffer before access token expiration
- 1-minute buffer before refresh token expiration

### Environment Variables

The Okta cloud helpers set and clear environment variables for spawned processes:

**Set:** `OKTA_ORG_URL`, `OKTA_BASE_URL`, `OKTA_OAUTH2_ACCESS_TOKEN`, `OKTA_CONFIG_DIR`

**Cleared (to prevent conflicts):** `OKTA_API_TOKEN`, `OKTA_API_KEY`, `OKTA_CLIENT_ID`, `OKTA_CLIENT_SECRET`, `OKTA_PRIVATE_KEY`, `OKTA_PRIVATE_KEY_ID`, `OKTA_SCOPES`

## Key Design Patterns

### Factory Pattern

- `NewProvider()` and `NewIdentity()` create instances based on configuration
- Extensible for adding new provider/identity types
- Type-safe construction with validation

### Chain of Responsibility

- Identity chains allow credential transformation through multiple steps
- Each identity receives credentials from the previous step
- Supports complex authentication scenarios (SSO -> Permission Set -> Assume Role)
- Cross-cloud chaining (Okta -> AWS via OIDC federation)

### Strategy Pattern

- Cloud-specific helper packages encapsulate differences (e.g., `pkg/auth/cloud/aws`, `pkg/auth/cloud/okta`).
- Identities and providers call helper functions directly; no shared `CloudProvider` interface.
- Environment setup varies by cloud and is implemented in helper packages.

### Repository Pattern

- `CredentialStore` abstracts credential persistence
- Supports multiple storage backends (keyring, file, etc.)
- Encrypted storage with expiration handling

## Configuration Schema

### AWS Providers

```yaml
providers:
  cplive-sso:
    kind: aws/iam-identity-center
    region: us-east-1
    start_url: https://cplive.awsapps.com/start
```

### Okta Providers

```yaml
providers:
  okta-corp:
    kind: okta/device-code
    spec:
      org_url: https://company.okta.com
      client_id: 0oa1234567890abcdef
      authorization_server: default  # Optional, defaults to "default"
      scopes: openid profile offline_access  # Optional
```

### AWS Identities

```yaml
identities:
  sandbox-admin:
    kind: aws/permission-set
    default: true
    via:
      provider: cplive-sso
    principal:
      name: AdminAccess
      account:
        name: "sandbox"
        # OR use account ID directly:
        # id: "123456789012"
```

### Okta Identities

```yaml
identities:
  # Direct Okta API access (for Terraform Okta provider)
  okta-api:
    kind: okta/api-access
    via:
      provider: okta-corp

  # AWS role assumption via Okta OIDC federation
  aws-via-okta:
    kind: aws/assume-role
    via:
      identity: okta-api
    principal:
      assume_role: arn:aws:iam::123456789012:role/OktaFederatedRole
```

### Identity Chaining

```yaml
identities:
  final-role:
    kind: aws/assume-role
    via:
      identity: sandbox-admin # Chain through another identity
    principal:
      assume_role: arn:aws:iam::999999999999:role/FinalRole
```

## Error Handling

### Validation Errors

- Configuration validation occurs at startup
- Circular dependency detection in identity chains
- Provider/identity reference validation
- Okta-specific validation (org_url, client_id required for device-code provider)

### Authentication Errors

- Graceful handling of expired credentials
- Clear error messages indicating which step failed
- Automatic retry for transient failures
- Okta-specific errors: device code expired, access denied, token refresh failed

### Environment Errors

- File permission handling for credential files
- Environment variable conflict detection
- Cleanup on failure scenarios

## Testing Strategy

### Unit Tests

- Individual component testing with mocks
- Interface compliance testing
- Error condition coverage

### Integration Tests

- End-to-end authentication flows
- CLI command testing with golden snapshots
- Multi-provider scenario testing

### Test Helpers

- Mock implementations for all interfaces
- Test credential generation utilities
- Environment isolation for tests

## Notes on Current Scope

- Cloud-specific behavior is implemented via helper packages (e.g., `pkg/auth/cloud/aws`, `pkg/auth/cloud/okta`) and used by identities in `PostAuthenticate`.
- There is no shared `CloudProvider`, `CloudProviderFactory`, or `CloudProviderManager` abstraction.
- The previous `config/` and `environment/` packages mentioned in older docs do not exist; environment merging is handled via `pkg/auth/utils/env.go` and stack info.

## Performance Considerations

### Credential Caching

- Encrypted credential storage with expiration
- Avoid re-authentication for valid credentials
- Cache invalidation on configuration changes

### File Operations

- Atomic file writes for credential files
- Proper file permissions (0600) for security
- File locking for concurrent access (Okta uses `gofrs/flock`)
- Cleanup of temporary files

### Memory Management

- Secure credential handling in memory
- Zero-out sensitive data after use
- Minimal credential lifetime in memory

## Security Features

### Credential Protection

- Encrypted storage using OS keyring
- Secure memory handling for credentials
- Automatic credential expiration

### File Security

- Restricted file permissions (0600)
- Provider-specific credential isolation
- Realm-based isolation for multi-repository setups
- No modification of user's existing AWS or Okta files

### Environment Isolation

- Provider-specific environment variables
- No global environment pollution
- Clean separation between different auth contexts
- Conflicting credential env vars cleared before setting new ones (Okta)

## Extensibility Points

### Adding New Cloud Providers

1. Create a helper package under `pkg/auth/cloud/<provider>/` (e.g., `aws`, `okta`).
2. Implement file/env helpers (e.g., `files.go`, `setup.go`) for that cloud.
3. Have identities/providers call these helpers in `PostAuthenticate`/`Environment()` as needed.
4. Implement any credential validation helpers and add tests/docs.

### Adding New Providers/Identities

1. Implement `Provider` or `Identity` interface
2. Add to factory functions in `factory.go`
3. Add validation rules in `validator.go`
4. Create tests and documentation

### Adding New Storage Backends

1. Implement `CredentialStore` interface
2. Add configuration options
3. Implement encryption/security requirements
4. Add migration utilities if needed

This architecture provides a solid foundation for multi-cloud authentication while maintaining security, performance, and extensibility requirements.
