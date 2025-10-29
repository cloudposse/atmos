# Product Requirements Document: Atmos Auth

## Executive Summary

Atmos Auth provides a unified, cloud-agnostic authentication and authorization system that enables secure access to cloud resources across AWS, Azure, GCP, and third-party services. The system implements a provider-identity model where **providers** define _how_ credentials are obtained (SSO, SAML, OIDC) and **identities** define _who_ you become (roles, permission sets, service accounts).

## 1. Problem Statement

### Current Challenges

- **Fragmented Authentication**: Teams use different authentication methods across cloud providers
- **Complex Role Chaining**: No standardized way to chain roles and permissions across clouds
- **Security Risks**: Inconsistent credential management and storage practices
- **Poor Developer Experience**: Multiple CLI tools and authentication flows
- **Compliance Gaps**: Difficulty auditing and managing access across environments

### Success Metrics

- **Reduced Authentication Time**: 80% reduction in time to authenticate across environments
- **Improved Security Posture**: 100% elimination of hardcoded credentials
- **Enhanced Auditability**: Complete audit trail for all authentication events
- **Developer Satisfaction**: 90%+ positive feedback on authentication UX

## 2. Core Requirements

### 2.1 Functional Requirements

#### FR-001: Provider Management

- **Description**: Support multiple authentication providers
- **Acceptance Criteria**:
  - Support AWS IAM Identity Center (SSO), SAML, GitHub Actions OIDC
  - Planned: Azure Entra ID, GCP OIDC, Okta OIDC
  - Allow configuration of provider-specific parameters
  - Enable default provider designation
- Note: This PR implements AWS IAM Identity Center (SSO), SAML, GitHub Actions OIDC, and AWS user credentials. Azure Entra ID, GCP OIDC, and Okta OIDC are planned for a future phase.
- **Priority**: P0 (Must Have)

#### FR-002: Identity Management

- **Description**: Manage cloud identities and role assumptions
- **Acceptance Criteria**:
  - Support AWS permission sets, assume roles, and users
  - Planned: Support Azure roles, GCP service account impersonation
  - Planned: Support Okta applications
  - Enable identity chaining via other identities or providers
  - Support deep identity chaining (provider→identity→identity→identity)
  - Recursive provider resolution for identity chains
  - Identity chains can be arbitrarily deep without limitation
- **Priority**: P0 (Must Have)

#### FR-003: Credential Sourcing

- **Description**: Secure credential retrieval and management
- **Acceptance Criteria**:
  - Support environment variable sourcing via `!env` syntax
  - Integration with OS keychains and external keystores
  - Never store secrets in plain text configuration
  - Support credential refresh and rotation
- **Priority**: P0 (Must Have)

#### FR-004: Schema Validation

- **Description**: Comprehensive configuration validation
- **Acceptance Criteria**:
  - JSON Schema validation for all configuration
  - Runtime validation for chains and cycles
  - Clear error messages for misconfigurations
  - Support for kind-based discriminators
- **Priority**: P0 (Must Have)

#### FR-005: CLI Interface

- **Description**: Intuitive command-line interface
- **Acceptance Criteria**:
  - `atmos auth login` for default identity authentication
  - `atmos auth login -i NAME` for specific identity
  - `atmos auth whoami` to show current effective principal
  - `atmos auth env` to export credentials
  - `atmos auth exec` to run commands under identity
  - `atmos auth validate` for configuration validation
  - `atmos auth user configure` for interactive user credential setup
- **Priority**: P0 (Must Have)

#### FR-006: Terraform Integration

- **Description**: Seamless Terraform workflow integration with pre-hooks
- **Acceptance Criteria**:
  - Terraform prehook runs before any Terraform command
  - Automatic authentication with default profile if found
  - Authentication occurs transparently before Terraform execution
  - Support for component-specific identity overrides
- **Priority**: P0 (Must Have)

#### FR-007: Component Configuration Integration

- **Description**: Auth configuration merges with component configuration
- **Acceptance Criteria**:
  - Components can override identities defined in atmos.yaml
  - Components can override providers defined in atmos.yaml
  - Component-level auth configuration takes precedence
  - Merged configuration is validated at runtime
  - Component-level default identity overrides global default identity
  - Default identity resolution considers merged configuration
  - Component overrides work even when no global default is set
- **Priority**: P0 (Must Have)

#### FR-008: Component Description Enhancement

- **Description**: Enhanced component description with auth information
- **Acceptance Criteria**:
  - `atmos describe component <component> -s <stack>` shows all identities
  - `atmos describe component <component> -s <stack> -q .identities` filters to identities
  - `atmos describe component <component> -s <stack> -q .providers` shows providers
  - Shows merged configuration from atmos.yaml and component overrides
- **Priority**: P0 (Must Have)

#### FR-009: Identity Environment Variables

- **Description**: Support environment variable injection from identities including AWS_PROFILE for proper AWS CLI integration
- **Acceptance Criteria**:
  - Identities support `environment` block with array of key-value objects
  - Environment variables are set before Terraform execution
  - Variables are available to Terraform and other tools
  - Support for case-sensitive environment variable names
  - Environment variables must be merged into component environment section
  - Component environment section takes precedence over process environment
  - AWS identities automatically include `AWS_PROFILE` environment variable set to identity name
  - `AWS_PROFILE` ensures AWS CLI uses correct profile within provider-specific credential files
  - AWS file environment variables (`AWS_SHARED_CREDENTIALS_FILE`, `AWS_CONFIG_FILE`, `AWS_PROFILE`) are automatically injected
- **Priority**: P0 (Must Have)

#### FR-010: AWS Credentials and Config File Management

- **Description**: Manage isolated AWS credentials and config files for Atmos using provider-based directories with identity profiles
- **Acceptance Criteria**:
  - Write AWS credentials to `~/.aws/atmos/<provider>/credentials` using INI format
  - Write AWS config to `~/.aws/atmos/<provider>/config` using INI format
  - Store multiple identity profiles within each provider's credential and config files
  - Use `[identity-name]` sections in credentials file (e.g., `[sandbox-admin]`, `[managers]`)
  - Use `[profile identity-name]` sections in config file (except `[default]` for identity named "default")
  - Set `AWS_SHARED_CREDENTIALS_FILE`, `AWS_CONFIG_FILE`, and `AWS_PROFILE` environment variables
  - Use `gopkg.in/ini.v1` package for robust INI file parsing and generation
  - Credential files are written during Terraform prehook
  - User's existing AWS files remain unmodified
  - Provider-specific isolation prevents credential conflicts
  - AWS file environment variables must be injected into component environment section
  - Environment variables must be available to Terraform execution
- **Priority**: P0 (Must Have)

#### FR-011: SSO Device Authorization Flow

- **Description**: Robust AWS SSO device authorization with proper polling
- **Acceptance Criteria**:
  - Implement proper polling mechanism for device authorization
  - Handle AuthorizationPendingException correctly
  - Handle SlowDownException with exponential backoff
  - Provide user feedback during polling process
  - Support configurable timeout and retry logic
  - Graceful error handling for terminal errors
  - Use AWS SDK v2 error type checking instead of string matching
  - Automatic browser opening when possible
- In CI environments, do not attempt to open a browser; instead, print the verification URI and user code
- **Priority**: P0 (Must Have)

#### FR-012: Credential Store Keyring Integration

- **Description**: Secure credential storage without listing capability dependency
- **Acceptance Criteria**:
  - Store credentials securely in system keyring
  - Retrieve credentials by identity name
  - Check credential expiration without listing all credentials
  - Whoami functionality works without keyring listing capability
  - Iterate through configured identities to find active sessions
  - Handle keyring backend limitations gracefully
  - Find most recent valid credentials across all configured identities
- **Priority**: P0 (Must Have)

#### FR-015: Hierarchical Credential Caching

- **Description**: Implement chain-based credential caching that optimizes authentication across identity hierarchies
- **Acceptance Criteria**:
  - **Chain Discovery**: Auth manager calculates complete identity chain from target identity to root provider
  - **Bottom-Up Validation**: Check cached credentials starting from target identity (bottom of chain)
  - **Selective Re-authentication**: Re-authenticate only invalid credentials in the chain
  - **Cache Key Strategy**: Each identity in chain has unique cache key: `<provider_kind>/<provider_name>/<identity_name>`
  - **Chain Examples**:
    - Simple: `SSO Provider → Permission Set`
    - Complex: `SSO Provider → Permission Set → Assume Role → Nested Assume Role`
  - **Validation Flow**:
    1. Check target identity (assume role) credentials first
    2. If valid (>5min remaining), use cached credentials
    3. If invalid, traverse up chain to find first valid credentials
    4. Re-authenticate from first invalid point down to target
    5. Cache all newly obtained credentials
  - **Performance Optimization**: Minimize authentication requests by reusing valid cached credentials at any chain level
  - **Error Handling**: Clear error messages indicating which level in chain failed authentication
  - **Session Management**: Each chain level respects its configured session duration
- **Priority**: P0 (Must Have)

#### FR-016: SSO Credential Caching

- **Description**: Cache SSO authentication credentials to avoid repeated user prompts
- **Acceptance Criteria**:
  - SSO providers check keyring for cached credentials before prompting user
  - Store SSO credentials in keyring after successful authentication
  - Cache includes access tokens, refresh tokens, and expiration times
  - Automatic cache invalidation when credentials expire
  - Cache key format: `sso/<provider_name>/<identity_name>`
  - Support for credential refresh using cached refresh tokens
  - Graceful fallback to interactive authentication when cache is invalid
  - Cache respects session duration configured in provider
  - Clear error messages when cached credentials are expired or invalid
- **Priority**: P0 (Must Have)

#### FR-013: Interactive User Credential Configuration

- **Description**: Interactive configuration of AWS user credentials with secure storage
- **Acceptance Criteria**:
  - `atmos auth user configure` command available
  - Use Charm Bracelet's huh library for interactive selection
  - Present selector to choose from configured AWS user identity types
  - Prompt for AWS access key ID, secret access key, and optional MFA ARN
  - Store credentials securely in system keyring using go-keyring package
  - Credentials stored per identity with unique keyring keys
  - AWS user provider retrieves credentials from keyring when not specified in spec
  - Spec-based credentials take precedence over keyring-stored credentials
  - Support for updating existing stored credentials
  - Clear error messages for keyring access failures
- **Priority**: P1 (Should Have)

#### FR-014: Deep Identity Chaining

- **Description**: Support arbitrarily deep identity-to-identity chaining
- **Acceptance Criteria**:
  - Identities can chain to other identities via `via.identity`
  - Provider resolution works recursively through identity chains
  - Support chains like: provider → identity1 → identity2 → identity3
  - Each identity in chain can override previous identity's configuration
  - Circular dependency detection prevents infinite loops
  - Clear error messages for broken chains
  - Authentication flows through entire chain to reach final identity
  - Base provider credentials flow through chain transformations
  - `getProviderForIdentity()` function resolves provider recursively
  - Identity chains work seamlessly with component-level overrides
- **Priority**: P0 (Must Have)

#### FR-016: Identity Chaining Credential Flow

- **Description**: Ensure proper credential transformation through identity chains
- **Acceptance Criteria**:
  - Each identity in a chain receives credentials from the previous step in the chain
  - Provider credentials flow to first identity, then each identity transforms credentials for the next
  - AWS SSO → Permission Set → Assume Role chain works correctly:
    - AWS SSO provider authenticates and returns base credentials
    - Permission Set identity uses SSO credentials to assume permission set role
    - Assume Role identity uses Permission Set credentials (not SSO credentials) to assume final role
  - Identity chains resolve the source provider recursively but authenticate sequentially
  - Each identity's `Authenticate()` method receives the output credentials from the previous identity
  - Credential transformation preserves necessary metadata (region, account, etc.)
  - Chain authentication stops at first failure with clear error indicating which step failed
  - Support for any depth of chaining: provider → identity → identity → ... → identity
  - Each identity can validate that incoming credentials are appropriate for its authentication method
- **Priority**: P0 (Must Have)

#### FR-017: Authentication Logging Configuration

- **Description**: Support configurable logging levels specifically for authentication operations
- **Acceptance Criteria**:
  - Support `logs` section in auth configuration with `level` property
  - Log level applies only to authentication operations, not global Atmos logging
  - Supported log levels: `Debug`, `Info`, `Warn`, `Error` (matching Atmos schema)
  - Default log level when not specified should be `Info`
  - Log level configuration is validated at startup
  - Authentication logs use configured level while preserving global log settings
  - Log level automatically reverts to global atmos.yaml log level after authentication completes
  - Example: Global `INFO` + Auth `DEBUG` → Auth operations use `DEBUG`, then revert to `INFO`
  - Log level can ONLY be configured in atmos.yaml, not at component level
  - Integration with Charm Bracelet's logging framework for consistent formatting
- **Priority**: P1 (Should Have)

### 2.2 Non-Functional Requirements

#### NFR-001: Security

- **Description**: Enterprise-grade security standards
- **Requirements**:
  - No credentials stored in configuration files
  - Support for short-lived tokens (15m-1h)
  - Secure credential caching with encryption
  - Audit logging for all authentication events
- **Priority**: P0 (Must Have)

#### NFR-002: Performance

- **Description**: Fast authentication and credential retrieval
- **Requirements**:
  - Authentication completion within 5 seconds
  - Credential caching to avoid repeated API calls
  - Parallel provider initialization where possible
  - SSO credential caching eliminates repeated user prompts
  - Cache hit authentication completes within 1 second
- **Priority**: P1 (Should Have)

#### NFR-003: Reliability

- **Description**: Robust error handling and recovery
- **Requirements**:
  - Graceful degradation when providers are unavailable
  - Clear error messages with remediation guidance
  - Retry logic for transient failures
- **Priority**: P1 (Should Have)

#### NFR-004: Usability

- **Description**: Developer-friendly experience
- **Requirements**:
  - Intuitive YAML configuration structure
  - Comprehensive documentation and examples
  - IDE support with schema validation
  - Use Charm Bracelet's huh library for interactive UI elements
  - Use Charm Bracelet's logging framework throughout
- **Priority**: P1 (Should Have)

#### NFR-005: UI Framework

- **Description**: Consistent and modern CLI user interface
- **Requirements**:
  - Use Charm Bracelet's huh library for all pickers and selections
  - Consistent styling and interaction patterns
  - Accessible and keyboard-friendly interface
  - Support for both interactive and non-interactive modes
- **Priority**: P1 (Should Have)

#### NFR-006: Logging Framework

- **Description**: Structured and consistent logging
- **Requirements**:
  - Use Charm Bracelet's logging framework across all components
  - Structured logging with consistent format
  - Configurable log levels and output formats
  - Integration with audit logging requirements
- **Priority**: P1 (Should Have)

## 3. Use Cases & User Stories

### 3.1 Primary Use Cases

#### UC-001: Developer Daily Workflow

**Actor**: Software Developer
**Goal**: Authenticate to development environments quickly
**Scenario**:

1. Developer runs `atmos auth login`
2. System uses default SSO provider to authenticate
3. Developer gains access to default permission set
4. Developer can now run Terraform/other tools seamlessly

**Acceptance Criteria**:

- Single command authentication
- Automatic credential refresh
- Works across all supported cloud providers

#### UC-002: Multi-Environment Access

**Actor**: DevOps Engineer
**Goal**: Access multiple environments with different permissions
**Scenario**:

1. Engineer authenticates via SSO to base identity
2. Engineer chains to production admin role via `atmos auth login -i prod-admin`
3. Engineer performs production operations
4. Engineer switches to staging environment via `atmos auth login -i staging-dev`

**Acceptance Criteria**:

- Seamless identity switching
- Clear indication of current effective identity
- Proper permission isolation between environments

#### UC-003: CI/CD Pipeline Authentication

**Actor**: CI/CD System
**Goal**: Authenticate using OIDC for automated deployments
**Scenario**:

1. GitHub Actions workflow starts
2. System authenticates using GitHub OIDC provider
3. System assumes appropriate AWS role for deployment
4. Deployment proceeds with proper permissions

**Acceptance Criteria**:

- No long-lived credentials in CI/CD
- Automatic token refresh during long-running jobs
- Clear audit trail of all actions

#### UC-004: Break-Glass Emergency Access

**Actor**: Site Reliability Engineer
**Goal**: Access systems during emergency when SSO is unavailable
**Scenario**:

1. SSO system is down during incident
2. SRE uses break-glass AWS user credentials stored in keyring
3. SRE gains emergency access to critical systems
4. All actions are logged for post-incident review

**Acceptance Criteria**:

- Reliable access when federated systems fail
- Enhanced logging for break-glass usage
- Time-limited emergency access
- Secure credential storage in system keyring

#### UC-007: Interactive User Credential Setup

**Actor**: DevOps Engineer
**Goal**: Configure AWS user credentials for break-glass access
**Scenario**:

1. Engineer runs `atmos auth user configure`
2. System presents interactive selector of configured AWS user identities
3. Engineer selects target identity (e.g., "emergency-admin")
4. System prompts for AWS access key ID, secret access key, and optional MFA ARN
5. Credentials are securely stored in system keyring
6. Engineer can now use the identity for authentication

**Acceptance Criteria**:

- Intuitive interactive interface using Charm Bracelet's huh library
- Secure storage of credentials in system keyring
- Support for updating existing credentials
- Clear feedback on successful configuration
- Graceful error handling for keyring access issues

#### UC-005: Terraform Workflow with Auto-Authentication

**Actor**: DevOps Engineer
**Goal**: Run Terraform commands with automatic authentication
**Scenario**:

1. Engineer runs `atmos terraform plan vpc -s plat-ue2-sandbox`
2. Terraform prehook detects default identity configuration
3. System automatically authenticates using default profile
4. AWS credentials written to `~/.aws/atmos/<provider>/credentials`
5. AWS config written to `~/.aws/atmos/<provider>/config`
6. Environment variables set to point to Atmos-managed AWS files
7. Terraform command executes with proper credentials
8. User's existing AWS files remain unmodified

**Acceptance Criteria**:

- Seamless authentication without manual login
- Component-specific identity overrides work correctly
- Environment variables are properly injected
- AWS credentials isolated from user's existing files
- Clear error messages if authentication fails

#### UC-006: Component-Specific Identity Override

**Actor**: Security Engineer
**Goal**: Use different identity for sensitive components
**Scenario**:

1. Engineer needs to deploy security-critical component
2. Component configuration overrides default identity with elevated permissions
3. System uses component-specific identity for authentication
4. `atmos describe component` shows the effective identity configuration
5. Deployment proceeds with appropriate permissions

**Acceptance Criteria**:

- Component-level auth configuration takes precedence
- Merged configuration is visible via describe command
- Validation ensures component overrides are valid
- Audit trail shows which identity was used

#### UC-008: Deep Identity Chaining Authentication

**Actor**: DevOps Engineer
**Goal**: Authenticate through a complex identity chain for production access
**Scenario**:

1. Engineer runs `atmos auth login -i prod-cross-account-admin`
2. System resolves identity chain: AWS SSO → Permission Set → Assume Role
3. **Step 1**: AWS SSO provider authenticates user via device flow
   - Returns SSO session credentials with temporary access token
4. **Step 2**: Permission Set identity receives SSO credentials
   - Uses SSO credentials to assume `IdentityManagersTeamAccess` permission set
   - Returns permission set role credentials (AccessKeyID, SecretAccessKey, SessionToken)
5. **Step 3**: Assume Role identity receives permission set credentials
   - Uses permission set credentials (not original SSO credentials) to assume cross-account role
   - Returns final cross-account role credentials
6. Engineer now has access to production cross-account resources

**Identity Configuration Example**:

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      start_url: https://company.awsapps.com/start/
      region: us-east-1
      default: true

  identities:
    managers-base:
      kind: aws/permission-set
      via: { provider: company-sso }
      principal:
        name: IdentityManagersTeamAccess
        account:
          name: core-identity

    prod-cross-account-admin:
      kind: aws/assume-role
      via: { identity: managers-base } # Chain from permission set, not provider
      principal:
        assume_role: arn:aws:iam::999999999999:role/CrossAccountProductionAdmin
        session_name: atmos-prod-access
```

**Acceptance Criteria**:

- Each step in the chain receives credentials from the previous step
- SSO credentials are not used directly by the assume role step
- Permission set credentials are properly transformed and passed to assume role
- Chain authentication fails gracefully if any step fails
- Clear error messages indicate which step in the chain failed
- Final credentials provide access to the target cross-account role
- Audit trail shows the complete authentication chain

### 3.2 Edge Cases

#### EC-001: Credential Expiration During Long Operations

**Scenario**: Terraform apply runs longer than token lifetime
**Handling**: Automatic credential refresh with minimal disruption

#### EC-002: Provider Unavailability

**Scenario**: SSO provider is temporarily unavailable
**Handling**: Graceful fallback to cached credentials or alternative providers

#### EC-003: Circular Identity Chains

**Scenario**: Identity A chains to Identity B which chains back to A
**Handling**: Validation prevents circular dependencies at configuration time

#### EC-004: Identity Chain Authentication Failures

**Scenario**: Authentication fails at any step in a deep identity chain
**Handling**:

- Clear error messages indicating which step failed and why
- Audit trail of successful steps before failure
- No partial credential caching for failed chains
- Retry logic only for transient failures (network, temporary service issues)
- Permanent failures (invalid permissions, missing roles) fail immediately

#### EC-005: Identity Chain Credential Mismatch

**Scenario**: Identity receives credentials that are incompatible with its authentication method
**Handling**:

- Each identity validates incoming credential format and type
- Clear error messages about credential compatibility
- Example: Assume Role identity receives non-AWS credentials
- Validation occurs before attempting authentication to fail fast

## 4. Technical Architecture

### 4.1 Core Components

#### 4.1.1 Credential Interfaces

Atmos Auth uses a unified credential interface to standardize provider/identity behavior:

```go
type ICredentials interface {
    IsExpired() bool
    GetExpiration() (*time.Time, error)
    BuildWhoamiInfo(info *WhoamiInfo)
}
```

Concrete implementations:

- AWSCredentials: AccessKeyID, SecretAccessKey, SessionToken, Region, Expiration, MfaArn
  - Implements `IsExpired()`, `GetExpiration()`, and populates region via `BuildWhoamiInfo()`
- OIDCCredentials: Token, Provider, Audience
  - Implements `IsExpired()` (non-expiring), `GetExpiration()` (nil), and no-op `BuildWhoamiInfo()`

The Auth Manager, providers, and identities now exchange `ICredentials` rather than bespoke structs. This simplifies expiration handling and whoami augmentation, and cleanly supports new credential types.

#### Authentication Configuration Structure

```yaml
auth:
  logs:
    level: Debug # Optional: Debug, Info, Warn, Error (default: Info)

  providers:
    aws-sso:
      kind: aws/iam-identity-center
      start_url: https://company.awsapps.com/start/
      region: us-east-1
      default: true

  identities:
    dev-admin:
      kind: aws/permission-set
      default: false # Not default globally
      via: { provider: aws-sso }
      principal:
        name: DeveloperAccess
        account:
          name: development

    superuser:
      kind: aws/user
      # No via provider required - AWS User identities are self-contained
      credentials:
        access_key_id: !env SUPERUSER_AWS_ACCESS_KEY_ID
        secret_access_key: !env SUPERUSER_AWS_SECRET_ACCESS_KEY
        region: us-east-1
```

**Responsibilities**:

- Obtain base credentials from external systems
- Handle provider-specific authentication flows
- Manage session duration and refresh

#### Identity Management

```yaml
identities:
  dev-admin:
    kind: aws/permission-set
    via: { provider: aws-sso }
    principal:
      name: DeveloperAccess
      account:
        name: development
    env:
      - key: AWS_PROFILE
        value: dev-admin
      - key: TEAM_ROLE
        value: dev-admin
      - key: DEPLOYMENT_ENVIRONMENT
        value: development
```

**Responsibilities**:

- Define target identities and permissions
- Handle identity chaining and role assumption
- Manage identity-specific configuration

#### Credential Store

**Responsibilities**:

- Secure storage of temporary credentials
- Encryption of cached tokens
- Automatic cleanup of expired credentials

#### Validation Engine

**Responsibilities**:

- JSON Schema validation
- Runtime constraint checking
- Cycle detection in identity chains

### 4.2 Data Flow

#### Basic Authentication Flow

1. **Configuration Loading**: Parse and validate `atmos.yaml` auth section
2. **Provider Authentication**: Authenticate with configured providers
3. **Identity Resolution**: Resolve target identity through provider or chain
4. **Credential Retrieval**: Obtain and cache temporary credentials
5. **Tool Integration**: Provide credentials to Terraform/other tools

#### Identity Chaining Data Flow

For identity chains (e.g., AWS SSO → Permission Set → Assume Role):

1. **Chain Resolution**: Recursively resolve the provider source for the target identity

   - `prod-admin` chains to `managers-base` identity
   - `managers-base` chains to `company-sso` provider
   - Source provider identified: `company-sso`

2. **Sequential Authentication**: Authenticate through the chain step by step

   - **Step 1**: Authenticate with source provider (`company-sso`)
     - Returns base credentials (SSO session tokens)
   - **Step 2**: First identity (`managers-base`) authenticates using base credentials
     - Permission Set identity receives SSO credentials
     - Uses SSO credentials to assume permission set role
     - Returns permission set credentials (AWS access keys)
   - **Step 3**: Final identity (`prod-admin`) authenticates using previous credentials
     - Assume Role identity receives permission set credentials
     - Uses permission set credentials to assume cross-account role
     - Returns final role credentials

3. **Credential Transformation**: Each step transforms credentials for the next

   - Provider credentials → Identity credentials → Final credentials
   - Metadata (region, account, session info) preserved through chain
   - Each identity validates incoming credentials are appropriate

4. **Error Handling**: Chain stops at first failure with clear error context

   - Indicates which step in the chain failed
   - Provides actionable error messages
   - Maintains audit trail of successful steps

5. **Caching Strategy**: Cache credentials at each step for performance
   - Provider credentials cached separately from identity credentials
   - Each identity's credentials cached with appropriate expiration
   - Cache keys include full chain context to avoid conflicts

### 4.3 Security Model

#### Credential Storage

- **Never in configuration**: Use `!env` variables or keystore integration
- **Encrypted caching**: AES-256 encryption for cached tokens
- **Automatic cleanup**: Remove expired credentials immediately

#### Access Control

- **Principle of least privilege**: Minimal permissions for each identity
- **Time-bounded access**: Short-lived tokens with automatic refresh
- **Audit logging**: Complete record of authentication events

## 5. Configuration Schema

### 5.1 Schema Design Principles

#### Separation of Concerns

The Atmos Auth schema follows a clear separation of concerns:

- **`via` = HOW** you obtain base credentials (SSO/SAML/OIDC or from another identity)
- **`principal` = WHO** you become (permission set, role, service account, app)
- **`credentials` = WHAT** you provide (only for `aws/user` identities)

#### Field Usage by Identity Type

- **Target Identities** (`aws/permission-set`, `aws/assume-role`, `azure/role`, `gcp/impersonate-sa`, `okta/app`): Use `principal` field to specify the target identity to assume
- **AWS User Identities** (`aws/user`): Use `credentials` field to specify access keys and configuration

This design makes the discriminator (`kind`) explicit while keeping the schema clear and type-safe.

### 5.2 Provider Types

**Supported Provider Types:**

- `aws/iam-identity-center` - AWS IAM Identity Center (AWS SSO)
- `aws/saml` - AWS SAML provider

**Note:** `aws/user` and `aws/assume-role` are NOT provider types. These are identity types only.

#### AWS IAM Identity Center

```yaml
providers:
  aws-sso:
    kind: aws/iam-identity-center
    start_url: https://company.awsapps.com/start/
    region: us-east-1
    session: { duration: 15m }
    default: true
```

**Authentication Flow:**

AWS IAM Identity Center uses the OAuth 2.0 device authorization flow. During authentication:

1. Atmos displays a **verification code** (e.g., "WDDD-HRQV") in the terminal
2. A browser window opens automatically to the AWS SSO login page
3. You authenticate in the browser (including any MFA prompts from your identity provider)
4. Atmos polls AWS until authentication completes

**Important Notes:**

- The verification code displayed in the terminal is a **device authorization user code**, NOT an MFA token
- This code is generated by AWS and allows you to visually verify it matches what appears in the browser
- Any MFA prompts (e.g., authenticator app codes, SMS codes) appear in the browser during login
- The device code cannot be pre-configured - it's dynamically generated for each authentication session

#### AWS SAML

The AWS SAML provider uses the `github.com/versent/saml2aws/v2` package with Playwright for browser automation to authenticate against SAML identity providers and assume AWS roles.

**Basic Configuration:**

```yaml
providers:
  aws-saml:
    kind: aws/saml
    url: https://accounts.google.com/o/saml2/initsso?idpid=C01abc23&spid=123456789&forceauthn=false
    region: us-east-1
    username: user@company.com
    provider_type: GoogleApps # Optional: auto-detected if not specified
    download_browser_driver: true # Optional: auto-download Playwright drivers
```

**Advanced Configuration with Authentication Details:**

```yaml
providers:
  okta-saml:
    kind: aws/saml
    url: https://company.okta.com/app/amazon_aws/exk1234567890/sso/saml
    region: us-west-2
    username: john.doe@company.com
    password: "${SAML_PASSWORD}" # Optional: will prompt if not provided
    provider_type: Okta
    download_browser_driver: true
    session:
      duration: 1h
```

**Supported SAML Provider Types:**

- `GoogleApps` - Google Workspace SAML
- `Okta` - Okta SAML
- `ADFS` - Active Directory Federation Services
- `ADFS2` - ADFS 2.0
- `AzureAD` - Azure Active Directory
- `PingFed` - PingFederate
- `KeyCloak` - Keycloak
- `Auth0` - Auth0
- `OneLogin` - OneLogin
- `Browser` - Generic browser-based SAML (auto-detected)

**Environment Variables:**

- `SAML2AWS_AUTO_BROWSER_DOWNLOAD=true` - Auto-download Playwright browser drivers
- `SAML_PASSWORD` - SAML password (if not provided in config)

**Usage Notes:**

- First run will download Playwright browser drivers automatically if `download_browser_driver: true`
- Browser automation handles MFA challenges automatically
- Supports headless browser operation for CI/CD environments
- SAML assertions are cached for session duration to reduce authentication frequency
- Google Apps SAML responses are automatically processed with Base64 decoding and assertion extraction
- Multiple SAML assertion formats are supported (Base64 encoded, raw XML, processed assertions)
- Provider type auto-detection: Google Apps URLs automatically use "Browser" provider for compatibility

#### GitHub OIDC

The GitHub OIDC provider authenticates using GitHub Actions OIDC tokens and is designed to work with AWS assume role identities for CI/CD workflows.

**Basic Configuration:**

```yaml
providers:
  github-oidc:
    kind: github/oidc
```

**Usage with AWS Assume Role Identity:**

```yaml
providers:
  github-oidc:
    kind: github/oidc

identities:
  ci-role:
    kind: aws/assume-role
    via: { provider: github-oidc }
    principal:
      assume_role: arn:aws:iam::123456789012:role/GitHubActionsRole
```

**Environment Variables (GitHub Actions):**

- `ACTIONS_ID_TOKEN_REQUEST_TOKEN` - GitHub Actions OIDC token (automatically set)
- `ACTIONS_ID_TOKEN_REQUEST_URL` - GitHub Actions OIDC endpoint (automatically set)

**Usage Notes:**

- Only works within GitHub Actions environment
- Requires GitHub Actions workflow to have `id-token: write` permission
- OIDC token is automatically retrieved from GitHub Actions environment
- Typically used with AWS assume role identities for temporary AWS credentials
- AWS files are automatically managed with provider-based organization

#### Azure Entra ID

```yaml
providers:
  azure-entra:
    kind: azure/entra
    tenant_id: 11111111-2222-3333-4444-555555555555
```

#### GCP OIDC

```yaml
providers:
  gcp-oidc:
    kind: gcp/oidc
    workload_pool: my-pool
```

#### Okta OIDC

```yaml
providers:
  okta:
    kind: okta/oidc
    org_url: https://company.okta.com
    client_id: okta-client-id
```

### 5.2 Identity Types

#### Environment Variable Support

All identity types support an `env` block for injecting environment variables:

```yaml
identities:
  managers:
    kind: aws/permission-set
    default: true
    via: { provider: cplive-sso }
    principal:
      name: IdentityManagersTeamAccess
      account:
        name: core-identity
    env:
      - key: AWS_PROFILE
        value: managers-core
      - key: TEAM_ROLE
        value: managers
      - key: DEPLOYMENT_ENVIRONMENT
        value: production
```

**Environment Variable Behavior**:

- Variables are set before Terraform execution
- Array format preserves case-sensitive keys (required due to Viper limitations)
- Variables are available to all tools executed under the identity
- Component environment section inherits these variables
- AWS-specific environment variables are automatically set for credential file paths

#### AWS Credentials and Config File Management

For AWS providers, Atmos manages isolated credential and config files:

```yaml
identities:
  prod-admin:
    kind: aws/permission-set
    via: { provider: aws-sso }
    principal:
      name: ProductionAdmin
      account:
        name: production
    env:
      - key: AWS_PROFILE
        value: prod-admin
      - key: ENVIRONMENT
        value: production
    # AWS files automatically managed:
    # ~/.aws/atmos/aws-sso/credentials
    # ~/.aws/atmos/aws-sso/config
```

**AWS File Management Behavior**:

- Credentials written to `~/.aws/atmos/<provider>/credentials` during prehook using INI format
- Config written to `~/.aws/atmos/<provider>/config` during prehook using INI format
- Multiple identity profiles stored within each provider's credential and config files
- Credentials file contains `[identity-name]` sections (e.g., `[sandbox-admin]`, `[managers]`)
- Config file contains `[profile identity-name]` sections (except `[default]` for identity named "default")
- `AWS_SHARED_CREDENTIALS_FILE` points to Atmos-managed credentials file
- `AWS_CONFIG_FILE` points to Atmos-managed config file
- `AWS_PROFILE` set to identity name to select correct profile within files
- User's existing `~/.aws/credentials` and `~/.aws/config` remain untouched
- Provider isolation prevents conflicts between different auth providers
- Uses `gopkg.in/ini.v1` package for robust INI file parsing and generation

#### AWS Permission Set

AWS Permission Set identities assume roles via AWS IAM Identity Center (SSO). The `account` field specifies which AWS account contains the permission set.

**Account Specification Options:**

You can specify the account using either:

- `account.name` - Account name/alias (resolved via SSO ListAccounts API)
- `account.id` - Numeric account ID (used directly, no lookup required)

**Example with Account Name (Recommended):**

```yaml
identities:
  dev-access:
    kind: aws/permission-set
    via: { provider: aws-sso }
    principal:
      name: DeveloperAccess
      account:
        name: dev
```

**Example with Account ID:**

```yaml
identities:
  dev-access:
    kind: aws/permission-set
    via: { provider: aws-sso }
    principal:
      name: DeveloperAccess
      account:
        id: "123456789012"
```

#### AWS Assume Role

```yaml
identities:
  prod-admin:
    kind: aws/assume-role
    via: { identity: dev-access }
    principal:
      assume_role: arn:aws:iam::123456789012:role/ProductionAdmin
```

#### AWS User (Break-glass)

AWS User identities are standalone and do not require a `via` provider configuration. They authenticate directly using AWS access keys with optional multi-factor authentication (MFA) support.

```yaml
identities:
  break-glass:
    kind: aws/user
    credentials:
      access_key_id: !env AWS_ACCESS_KEY_ID
      secret_access_key: !env AWS_SECRET_ACCESS_KEY
      mfa_arn: !env AWS_MFA_ARN
      region: !env AWS_DEFAULT_REGION

  emergency-admin:
    kind: aws/user
    credentials:
      # If not defined, the credentials will try to be pulled from the keyring.
      mfa_arn: arn:aws:iam::123456789012:mfa/username  # Optional: Direct MFA ARN
      region: us-east-1
```

**Key Characteristics:**

- No `via` provider required - AWS User identities are self-contained
- Credentials stored securely in system keyring using `types.Credentials` format with AWS wrapper
- Configure credentials using `atmos auth user configure` interactive command
- MFA ARN support integrated directly into `AWSCredentials` schema
- Primarily used for break-glass scenarios and emergency access
- Direct AWS API authentication using access key pairs
- AWS files written to XDG config directory (e.g., `~/.config/atmos/aws/aws-user/credentials` and `~/.config/atmos/aws/aws-user/config` on Linux)

**Multi-Factor Authentication (MFA) - AWS Implementation:**

AWS User identities support MFA devices for enhanced security. When an MFA device ARN is configured, Atmos prompts for a time-based one-time password (TOTP) during authentication.

> **Note:** This describes the MFA implementation for AWS IAM users. Future implementations will support MFA for Azure (multi-factor authentication via Entra ID), GCP (2-Step Verification), and other cloud providers, each with their provider-specific MFA mechanisms.

**Configuration Options:**

```yaml
# Option 1: Complete credentials in YAML (direct values or environment variables)
identities:
  prod-admin:
    kind: aws/user
    credentials:
      access_key_id: !env AWS_ACCESS_KEY_ID
      secret_access_key: !env AWS_SECRET_ACCESS_KEY
      mfa_arn: arn:aws:iam::123456789012:mfa/username
      region: us-east-1

# Option 2: Credentials in keyring, MFA ARN in YAML (RECOMMENDED)
# Store access keys via 'atmos auth user configure', override MFA ARN in YAML
identities:
  prod-admin:
    kind: aws/user
    credentials:
      mfa_arn: arn:aws:iam::123456789012:mfa/username  # YAML overrides keyring
      region: us-east-1

# Option 3: All credentials in keyring (via atmos auth user configure)
# No credentials in YAML - everything retrieved from keyring
identities:
  prod-admin:
    kind: aws/user
    credentials:
      region: us-east-1
```

**Credential Precedence (Deep Merge):**

Atmos uses per-field precedence with deep merge:

1. **If YAML has complete credentials** (both `access_key_id` and `secret_access_key`):
   - Use YAML entirely (including `mfa_arn` from YAML)
   - Keyring is ignored

2. **If YAML has no credentials** (both keys empty or omitted):
   - Use keyring credentials (access keys + MFA ARN)
   - **Override MFA ARN from YAML if present** (allows version-controlled MFA config)

3. **If YAML has partial credentials** (only one key):
   - Error: Both keys must be provided or both must be empty

This allows flexible configuration:
- **Store sensitive credentials in keyring** (local, secure)
- **Configure MFA ARN in YAML** (version controlled, shared across team)
- **Override any field** from YAML without losing keyring credentials

**Authentication Flow with MFA:**

1. **Credential Resolution:**
   - Atmos retrieves long-lived credentials (access key + secret) from YAML config or keychain
   - MFA ARN is retrieved from YAML config, environment variable, or keychain

2. **Interactive TOTP Prompt:**
   - If MFA ARN is configured, Atmos displays an interactive form
   - User enters 6-digit TOTP code from authenticator app (Google Authenticator, Authy, etc.)
   - TOTP is validated (must be exactly 6 digits, cannot be empty)

3. **Session Token Generation:**
   - Atmos calls AWS STS `GetSessionToken` with:
     - Long-lived credentials (access key + secret)
     - MFA device ARN (`SerialNumber` parameter)
     - TOTP code (`TokenCode` parameter)
     - Session duration (3600 seconds = 1 hour)
   - AWS validates MFA and returns temporary session credentials

4. **Credential Storage:**
   - Temporary session credentials (access key + secret + session token) are written to AWS files
   - Long-lived credentials remain unchanged in keychain
   - Session credentials are valid for 1 hour

5. **Subsequent Operations:**
   - All AWS API calls use temporary session credentials
   - Session credentials expire after 1 hour
   - Re-authentication required after expiration (prompts for new TOTP)

**Security Model:**

- **MFA Device ARN:** Not a secret - safe to store in version-controlled YAML configuration
- **TOTP Codes:** Never stored - ephemeral input only, required for each authentication session
- **Long-lived Credentials:** Stored securely in OS keychain, never in plain text
- **Session Credentials:** Written to AWS files, automatically expire after 1 hour
- **Defense-in-Depth:** Even if long-lived credentials are compromised, attacker needs physical access to MFA device

**Implementation Details:**

- **File:** `pkg/auth/identities/aws/user.go`
- **TOTP Prompt Function:** `promptMfaTokenFunc` (line 238) - uses Charm Bracelet `huh` library for interactive form
- **MFA Form:** `newMfaForm` (line 264) - validates 6-digit TOTP, displays MFA device ARN
- **Session Token Generation:** `generateSessionToken` (line 165) - calls AWS STS with MFA parameters
- **Input Construction:** `buildGetSessionTokenInput` (line 247) - conditionally adds MFA parameters
- **Credential Resolution:** `resolveLongLivedCredentials` (line 86) - prioritizes YAML config over keychain
- **MFA ARN Storage:** `AWSCredentials.MfaArn` field (line 22 in `pkg/auth/types/aws_credentials.go`)

**Use Cases:**

- **Compliance Requirements:** Organizations mandating MFA for privileged access
- **Break-glass Access:** Emergency accounts requiring enhanced security
- **Production Environments:** Critical infrastructure requiring defense-in-depth
- **Regulatory Compliance:** HIPAA, SOC 2, PCI-DSS environments
- **Zero Trust Architecture:** Multi-layered authentication for least-privilege access

#### Azure Role (Not Implemented)

```yaml
identities:
  azure-admin:
    kind: azure/role
    via: { provider: azure-entra }
    principal:
      role_definition_id: b24988ac-6180-42a0-ab88-20f7382dd24c
      scope: /subscriptions/12345678-1234-1234-1234-123456789012
```

#### GCP Service Account Impersonation (Not Implemented)

```yaml
identities:
  gcp-admin:
    kind: gcp/impersonate-sa
    via: { provider: gcp-workload }
    principal:
      service_account: admin@project.iam.gserviceaccount.com
      project: my-project
```

#### Okta Application (Not Implemented)

```yaml
identities:
  monitoring-app:
    kind: okta/app
    via: { provider: okta }
    principal:
      app: datadog-admin
```

### 5.3 Component-Level Auth Configuration

Components can override auth configuration defined in `atmos.yaml`. Here's a complete example:

#### Global Auth Configuration (`atmos.yaml`)

```yaml
auth:
  logs:
    level: Debug # Optional: Debug, Info, Warn, Error (default: Info)

  providers:
    cplive-sso:
      kind: aws/iam-identity-center
      start_url: https://cplive.awsapps.com/start/
      region: us-east-2
      session: { duration: 15m }
      default: true

  identities:
    managers:
      kind: aws/permission-set
      default: false # Not default globally
      via: { provider: cplive-sso }
      principal:
        name: IdentityManagersTeamAccess
        account:
          name: core-identity

    superuser:
      kind: aws/user
      # No via provider required - AWS User identities are self-contained
      credentials:
        access_key_id: !env SUPERUSER_AWS_ACCESS_KEY_ID
        secret_access_key: !env SUPERUSER_AWS_SECRET_ACCESS_KEY
        region: us-east-1
```

#### Component Stack Configuration

```yaml
components:
  terraform:
    vpc:
      # Component-specific auth overrides
      auth:
        identities:
          managers:
            default: true # Override: make this the default for VPC component

      # Regular component configuration
      metadata:
        component: vpc
        inherits:
          - vpc/defaults
      vars:
        enabled: true
```

**Result**: For the VPC component, the `managers` identity becomes the default identity, overriding the global `default: false` setting.

#### Advanced Component Override Example

```yaml
components:
  terraform:
    security-vpc:
      # Component-specific auth overrides
      auth:
        identities:
          security-admin:
            kind: aws/permission-set
            via: { provider: cplive-sso }
            principal:
              name: SecurityAdminAccess
              account:
                name: security
            env:
              - key: SECURITY_MODE
                value: strict
          managers:
            default: false # Disable managers as default for security components

        providers:
          cplive-sso:
            session:
              duration: 30m # Override session duration for security operations

      vars:
        vpc_cidr: "10.1.0.0/16"
```

**Component Auth Merging Rules**:

1. **Identity Property Overrides**: Component identity properties override global identity properties with same name
   - Example: `managers.default: true` in component overrides `managers.default: false` in global config
2. **New Identities**: New identities defined in components are added to available options
3. **Provider Overrides**: Component providers override global providers with same name
4. **Environment Variable Merging**: Component environment variables are merged with identity environment variables
5. **AWS File Management**: AWS credential file paths are automatically set based on active provider
6. **Validation**: Validation occurs on the merged configuration
7. **Precedence**: Component-level configuration always takes precedence over global configuration

### 5.4 Component Description Output

The `atmos describe component` command shows merged auth configuration:

```bash
# Show all component information including auth
atmos describe component vpc -s plat-ue2-sandbox

# Filter to show only auth section
atmos describe component vpc -s plat-ue2-sandbox -q .auth

# Filter to show only identities within auth section
atmos describe component vpc -s plat-ue2-sandbox -q .auth.identities

# Filter to show only providers within auth section
atmos describe component vpc -s plat-ue2-sandbox -q .auth.providers
```

**Expected Output for VPC Component Auth Section**:

```yaml
auth:
  identities:
    managers:
      kind: aws/permission-set
      default: true # Overridden from false to true by component
      via: { provider: cplive-sso }
      principal:
        name: IdentityManagersTeamAccess
        account:
          name: core-identity


      # AWS files automatically managed:
      # ~/.aws/atmos/cplive-sso/credentials
      # ~/.aws/atmos/cplive-sso/config
```

**Expected Output for Security-VPC Component Auth Section**:

```yaml
auth:
  identities:
    managers:
      kind: aws/permission-set
      default: false # Overridden to disable for security components
      via: { provider: cplive-sso }
      principal:
        name: IdentityManagersTeamAccess
        account:
          name: core-identity

    security-admin: # Component-specific identity
      kind: aws/permission-set
      via: { provider: cplive-sso }
      principal:
        name: SecurityAdminAccess
        account:
          name: security
      env:
        - key: SECURITY_MODE
          value: strict
```

## 5. Hierarchical Authentication Flow Examples

### 5.1 Chain-Based Authentication Scenarios

#### Scenario 1: AWS SAML Provider with Assume Role Chain

**Configuration:**

```yaml
providers:
  okta-saml:
    kind: aws/saml
    url: https://company.okta.com/app/amazon_aws/exk1234567890/sso/saml
    region: us-west-2
    username: john.doe@company.com
    provider_type: Okta
    download_browser_driver: true
    session:
      duration: 1h

identities:
  # Direct SAML role assumption
  saml-admin:
    kind: aws/assume-role
    via: { provider: okta-saml }
    principal:
      role_arn: arn:aws:iam::123456789012:role/SAMLAdminRole

  # Chained: SAML → Cross-account role
  prod-deployer:
    kind: aws/assume-role
    via: { identity: saml-admin }
    principal:
      role_arn: arn:aws:iam::987654321098:role/DeployerRole
```

**Authentication Flow:**

1. **Target**: `prod-deployer` identity
2. **Chain**: `okta-saml` → `saml-admin` → `prod-deployer`
3. **Cache Check**: Check `aws-saml/okta-saml/prod-deployer` cache key
4. **SAML Authentication**: Browser opens Okta SAML page, handles MFA
5. **Role Selection**: Extract available roles from SAML assertion
6. **First Assume**: Assume `SAMLAdminRole` using SAML assertion
7. **Second Assume**: Use `SAMLAdminRole` credentials to assume `DeployerRole`
8. **Result**: Final credentials for `DeployerRole` in production account

**Expected Output:**

```bash
$ atmos terraform plan vpc --stack prod-us-west-2
🔐 Starting SAML authentication for provider: okta-saml
🌐 Opening browser for SAML authentication...
✅ SAML authentication successful
🔄 Assuming role: arn:aws:iam::123456789012:role/SAMLAdminRole
🔄 Assuming role: arn:aws:iam::987654321098:role/DeployerRole
✅ Authentication complete for identity: prod-deployer
```

#### Scenario 2: Simple Chain (SSO → Permission Set)

**Configuration:**

```yaml
providers:
  cplive-sso:
    kind: aws/iam-identity-center
    start_url: https://cplive.awsapps.com/start/

identities:
  managers:
    kind: aws/permission-set
    via: { provider: cplive-sso }
    spec:
      name: IdentityManagersTeamAccess
      account: { name: core-identity }
```

**Authentication Flow:**

1. **Target**: `managers` identity
2. **Chain**: `cplive-sso` → `managers`
3. **Cache Check**: Check `aws-iam-identity-center/cplive-sso/managers` cache key
4. **If Valid**: Use cached credentials, skip authentication
5. **If Invalid**:
   - Check `sso/cplive-sso/managers` for SSO tokens
   - If SSO tokens valid, refresh permission set credentials
   - If SSO tokens invalid, prompt for SSO authentication, then get permission set

#### Scenario 2: Complex Chain (SSO → Permission Set → Assume Role)

**Configuration:**

```yaml
providers:
  cplive-sso:
    kind: aws/iam-identity-center
    start_url: https://mock

identities:
  managers:
    kind: aws/permission-set
    via: { provider: cplive-sso }
    principal:
      name: IdentityManagersTeamAccess
      account: { name: core-identity }

  sandbox-admin:
    kind: aws/assume-role
    via: { identity: managers }
    principal:
      role_arn: arn:aws:iam::111111111111:role/ExampleRole
```

**Authentication Flow:**

1. **Target**: `sandbox-admin` identity
2. **Chain**: `cplive-sso` → `managers` → `sandbox-admin`
3. **Bottom-Up Validation**:
   - Check `aws-assume-role/managers/sandbox-admin` (assume role credentials)
   - If valid (>5min), use cached credentials ✅
   - If invalid, check `aws-iam-identity-center/cplive-sso/managers` (permission set credentials)
   - If permission set valid, use it to refresh assume role credentials
   - If permission set invalid, check `sso/cplive-sso/managers` (SSO tokens)
   - Re-authenticate from first invalid point down to target

#### Scenario 3: Deep Chain (SSO → Permission Set → Assume Role → Nested Assume Role)

**Configuration:**

```yaml
identities:
  managers:
    kind: aws/permission-set
    via: { provider: cplive-sso }

  cross-account-admin:
    kind: aws/assume-role
    via: { identity: managers }
    principal:
      assume_role: arn:aws:iam::123456789012:role/CrossAccountAccess

  production-admin:
    kind: aws/assume-role
    via: { identity: cross-account-admin }
    principal:
      assume_role: arn:aws:iam::987654321098:role/ProductionAdmin
```

**Authentication Flow:**

1. **Target**: `production-admin` identity
2. **Chain**: `cplive-sso` → `managers` → `cross-account-admin` → `production-admin`
3. **Optimization Strategy**:
   - Check each level from bottom to top
   - Find first valid cached credentials
   - Re-authenticate only from first invalid point downward
   - Cache all newly obtained credentials

### 5.2 Cache Key Strategy

**Cache Key Format**: `<provider_kind>/<provider_name>/<identity_name>`

**Examples:**

- SSO Tokens: `sso/cplive-sso/managers`
- Permission Set: `aws-iam-identity-center/cplive-sso/managers`
- Assume Role: `aws-assume-role/managers/sandbox-admin`
- Nested Assume Role: `aws-assume-role/cross-account-admin/production-admin`

### 5.3 Performance Benefits

**Without Hierarchical Caching:**

- Every authentication requires full chain re-authentication
- SSO prompts on every expired credential
- No optimization for partially valid chains

**With Hierarchical Caching:**

- 90% reduction in authentication time for valid cached credentials
- Selective re-authentication minimizes user prompts
- Optimal credential reuse across identity chains
- Graceful degradation when partial chain is invalid

## 6. Best Practices

### 6.1 Security Best Practices

#### Credential Management

- **Never hardcode secrets**: Always use `!env` variables or keystore integration
- **Rotate regularly**: Implement automatic credential rotation where possible
- **Monitor usage**: Set up alerts for unusual authentication patterns
- **Audit access**: Maintain comprehensive logs of all authentication events

#### Permission Management

- **Least privilege**: Grant minimal permissions required for each role
- **Time-bound access**: Use shortest practical session durations
- **Regular review**: Periodically audit and clean up unused identities
- **Separation of duties**: Use different identities for different environments

#### Configuration Security

- **Version control**: Store auth configuration in version control
- **Environment separation**: Use different configurations per environment
- **Validation**: Always validate configuration before deployment
- **Documentation**: Document all identities and their intended use

### 6.2 Operational Best Practices

#### Configuration Management

```yaml
# Good: Clear, descriptive names
identities:
  production-terraform-admin:
    kind: aws/permission-set
    via: { provider: company-sso }
    principal:
      name: TerraformAdminAccess
      account.name: production


# Bad: Unclear, generic names
identities:
  identity1:
    kind: aws/permission-set
    via: { provider: sso }
    principal:
      name: Admin
      account.id: "123456789012"
```

#### Error Handling

- **Graceful degradation**: Provide fallback options when providers fail
- **Clear messages**: Give actionable error messages with remediation steps
- **Retry logic**: Implement exponential backoff for transient failures
- **Monitoring**: Set up alerts for authentication failures

#### Performance Optimization

- **Credential caching**: Cache valid credentials to reduce API calls
- **Parallel initialization**: Initialize multiple providers concurrently
- **Lazy loading**: Only authenticate when credentials are needed
- **Connection pooling**: Reuse connections where possible

### 6.3 Development Best Practices

#### Testing Strategy

- **Unit tests**: Test individual provider and identity implementations
- **Integration tests**: Test full authentication flows
- **Security tests**: Validate credential handling and storage
- **Performance tests**: Ensure authentication meets SLA requirements

#### Code Organization

- **Provider plugins**: Implement providers as pluggable modules
- **Interface segregation**: Define clear interfaces between components
- **Error types**: Use typed errors for better error handling
- **Logging**: Implement structured logging throughout

## 7. Testing Strategy

### 7.1 Test Categories

#### Unit Tests

**Scope**: Individual functions and methods
**Coverage**: 90%+ code coverage required
**Examples**:

- Provider configuration parsing
- Identity chain validation
- Credential encryption/decryption
- Schema validation logic

#### Integration Tests

**Scope**: End-to-end authentication flows
**Coverage**: All supported provider-identity combinations
**Examples**:

- AWS SSO → Permission Set flow
- SAML → Assume Role chain
- GitHub OIDC → AWS Role assumption
- Break-glass user authentication

#### Security Tests

**Scope**: Security-critical functionality
**Coverage**: All credential handling paths
**Examples**:

- Credential storage encryption
- Memory cleanup after use
- Token refresh security
- Audit log integrity

#### Performance Tests

**Scope**: Authentication speed and resource usage
**Coverage**: All authentication paths under load
**Examples**:

- Authentication completion time < 5s
- Memory usage during credential caching
- Concurrent authentication handling
- Provider failover performance

### 7.2 Test Data Management

#### Mock Providers

```go
type MockProvider struct {
    kind string
    credentials map[string]string
    failures map[string]error
}

func (m *MockProvider) Authenticate(ctx context.Context) (*Credentials, error) {
    if err, exists := m.failures[m.kind]; exists {
        return nil, err
    }
    return &Credentials{
        AccessToken: m.credentials["access_token"],
        ExpiresAt:   time.Now().Add(15 * time.Minute),
    }, nil
}
```

#### Test Fixtures

- **Valid configurations**: Complete, working auth configurations
- **Invalid configurations**: Various malformed configurations for validation testing
- **Edge cases**: Boundary conditions and error scenarios
- **Performance data**: Large configurations for performance testing

### 7.3 Validation Approaches

#### Schema Validation

```json
{
  "test_name": "valid_aws_sso_provider",
  "input": {
    "auth": {
      "providers": {
        "test-sso": {
          "kind": "aws/iam-identity-center",
          "start_url": "https://test.awsapps.com/start/",
          "region": "us-east-1"
        }
      },
      "identities": {}
    }
  },
  "expected": "valid"
}
```

#### Runtime Validation

```go
func TestIdentityChainValidation(t *testing.T) {
    tests := []struct {
        name        string
        config      *AuthConfig
        expectError bool
        errorType   string
    }{
        {
            name: "valid_chain",
            config: &AuthConfig{
                Providers: map[string]Provider{
                    "sso": {Kind: "aws/iam-identity-center"},
                },
                Identities: map[string]Identity{
                    "base": {Kind: "aws/permission-set", Via: Via{Provider: "sso"}},
                    "admin": {Kind: "aws/assume-role", Via: Via{Identity: "base"}},
                },
            },
            expectError: false,
        },
        {
            name: "circular_chain",
            config: &AuthConfig{
                Identities: map[string]Identity{
                    "a": {Kind: "aws/assume-role", Via: Via{Identity: "b"}},
                    "b": {Kind: "aws/assume-role", Via: Via{Identity: "a"}},
                },
            },
            expectError: true,
            errorType:   "circular_dependency",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateAuthConfig(tt.config)
            if tt.expectError {
                assert.Error(t, err)
                assert.Contains(t, err.Error(), tt.errorType)
            } else {
                assert.NoError(t, err)
            }
        })
    }
}
```

#### End-to-End Testing

```go
func TestE2EAuthentication(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping E2E test in short mode")
    }

    // Setup test environment
    testConfig := loadTestConfig(t)
    authManager := NewAuthManager(testConfig)

    // Test authentication flow
    ctx := context.Background()
    creds, err := authManager.Authenticate(ctx, "test-identity")

    require.NoError(t, err)
    assert.NotEmpty(t, creds.AccessToken)
    assert.True(t, creds.ExpiresAt.After(time.Now()))

    // Verify credentials work
    client := createTestClient(creds)
    err = client.ValidateCredentials(ctx)
    assert.NoError(t, err)
}
```

## 8. Implementation Guidelines

### 8.1 Development Phases

#### Phase 1: Core Infrastructure (4 weeks)

- **Week 1-2**: Configuration parsing and validation
- **Week 3-4**: Provider interface and AWS SSO implementation

**Deliverables**:

- JSON Schema validation
- Provider interface definition
- AWS IAM Identity Center provider
- Basic CLI commands (`login`, `whoami`)
- Charm Bracelet huh and logging integration

#### Phase 2: Identity Management (4 weeks)

- **Week 1**: AWS identity types (permission-set, assume-role, user)
- **Week 2**: Identity chaining and validation
- **Week 3**: Environment variable support and component configuration merging
- **Week 4**: Terraform prehook integration and credential caching

**Deliverables**:

- All AWS identity types with environment variable support
- Chain validation logic
- Component-level auth configuration merging
- Terraform prehook implementation
- Secure credential storage
- Enhanced CLI (`env`, `exec`)
- Component description enhancements

#### Phase 3: Multi-Cloud Support (4 weeks)

- **Week 1**: Azure Entra ID provider and roles
- **Week 2**: GCP OIDC and service account impersonation
- **Week 3**: Additional providers (SAML, GitHub OIDC, Okta)
- **Week 4**: Integration testing and documentation

**Deliverables**:

- All supported providers and identities
- Comprehensive test suite
- Complete documentation
- Migration guides

#### Phase 4: Advanced Features (2 weeks)

- **Week 1**: Performance optimizations and monitoring
- **Week 2**: Advanced CLI features and polish

**Deliverables**:

- Performance benchmarks
- Monitoring and alerting
- Advanced CLI features
- Production readiness

### 8.2 Code Structure

```text
pkg/auth/
├── cloud/                      # Cloud-specific integrations (current)
│   └── aws/
│       ├── files.go            # AWS credentials/config file management (INI)
│       └── setup.go            # AWS file setup during prehook
├── credentials/                # Credential storage (current)
│   └── store.go                # Secure keyring-backed store
├── identities/                 # Identity implementations (current)
│   └── aws/
│       ├── permission_set.go   # AWS SSO Permission Set identity
│       ├── assume_role.go      # AWS STS AssumeRole identity
│       └── user.go             # AWS User (break-glass) identity
├── providers/                  # Authentication providers (current)
│   ├── aws/
│   │   ├── sso.go              # AWS IAM Identity Center (SSO)
│   │   └── saml.go             # AWS SAML provider (saml2aws + Playwright)
│   └── github/
│       └── oidc.go             # GitHub Actions OIDC provider
├── types/                      # Core interfaces and credential types (current)
│   ├── interfaces.go           # AuthManager, Provider, Identity, CredentialStore, Validator
│   ├── whoami.go               # WhoamiInfo model/builder
│   ├── aws_credentials.go      # AWS credential type + helpers
│   └── github_oidc_credentials.go
├── utils/                      # Utilities (current)
│   └── env.go                  # Env helpers and injection utilities
├── validation/                 # Validation (current)
│   └── validator.go            # Config validation logic
├── factory.go                  # Provider/Identity factories (current)
├── hooks.go                    # Terraform prehook + auth logging scope (current)
├── manager.go                  # Main auth manager (current)
└── docs/                       # PRD and supplemental docs (current)

# Planned modules (future phases)
# These directories are not yet present but are planned and referenced by the PRD.
pkg/auth/
├── providers/
│   ├── azure/                  # Azure Entra ID provider (planned)
│   ├── gcp/                    # GCP OIDC provider (planned)
│   ├── okta/                   # Okta OIDC provider (planned)
│   └── oidc/                   # Additional generic OIDC providers (planned)
├── identities/
│   ├── azure/                  # Azure role identities (planned)
│   ├── gcp/                    # GCP service account impersonation (planned)
│   └── okta/                   # Okta app identities (planned)
├── ui/                         # Interactive UI (Charm) for pickers/prompts (planned)
├── cli/                        # Dedicated CLI command package (planned)
└── environment/                # Generic env merging/injection helpers (planned)
```

### 8.3 Interface Definitions

#### Provider Interface

```go
type Provider interface {
    // Kind returns the provider type (e.g., "aws/iam-identity-center")
    Kind() string

    // Authenticate performs the authentication flow and returns base credentials
    Authenticate(ctx context.Context) (*Credentials, error)

    // Refresh refreshes existing credentials if supported
    Refresh(ctx context.Context, creds *Credentials) (*Credentials, error)

    // Validate checks if the provider configuration is valid
    Validate() error
}
```

#### Identity Interface

```go
type Identity interface {
    // Kind returns the identity type (e.g., "aws/permission-set")
    Kind() string

    // Authenticate authenticates this identity using the provided credentials from previous step
    // For identity chains: receives credentials from previous identity in chain
    // For direct provider chains: receives credentials from provider
    Authenticate(ctx context.Context, inputCreds *Credentials) (*Credentials, error)

    // Validate checks if the identity configuration is valid
    Validate() error

    // Chain returns the identity or provider this identity chains from
    Chain() *Via

    // Environment returns the environment variables for this identity
    Environment() []EnvironmentVariable

    // Merge merges this identity with component-level overrides
    Merge(override *Identity) *Identity
}
```

#### Credentials Structure

```go
type Credentials struct {
    // Provider-specific credential data
    AccessToken     string            `json:"access_token,omitempty"`
    RefreshToken    string            `json:"refresh_token,omitempty"`
    AccessKeyID     string            `json:"access_key_id,omitempty"`
    SecretAccessKey string            `json:"secret_access_key,omitempty"`
    SessionToken    string            `json:"session_token,omitempty"`

    // Metadata
    ExpiresAt       time.Time         `json:"expires_at"`
    Region          string            `json:"region,omitempty"`
    Provider        string            `json:"provider"`
    Identity        string            `json:"identity,omitempty"`
    Metadata        map[string]string `json:"metadata,omitempty"`
}

// EnvironmentVariable represents a key-value pair for environment injection
type EnvironmentVariable struct {
    Key   string `json:"key" yaml:"key"`
    Value string `json:"value" yaml:"value"`
}

// AWSFileConfig represents AWS credential and config file paths
type AWSFileConfig struct {
    CredentialsPath string `json:"credentials_path"`
    ConfigPath      string `json:"config_path"`
    ProfileName     string `json:"profile_name"`
}

// AWSFileManager handles AWS credential and config file operations
type AWSFileManager interface {
    // WriteCredentials writes AWS credentials to provider-specific file
    WriteCredentials(provider string, creds *Credentials) error

    // WriteConfig writes AWS config to provider-specific file
    WriteConfig(provider string, config *AWSConfig) error

    // GetFilePaths returns the paths for AWS credentials and config files
    GetFilePaths(provider string) *AWSFileConfig

    // SetEnvironmentVariables sets AWS_SHARED_CREDENTIALS_FILE and AWS_CONFIG_FILE
    SetEnvironmentVariables(provider string) []EnvironmentVariable

    // Cleanup removes temporary AWS files
    Cleanup(provider string) error
}

// LogsConfig represents logging configuration for auth operations
type LogsConfig struct {
    Level string `json:"level,omitempty" yaml:"level,omitempty"` // Debug, Info, Warn, Error
}

// AuthConfig represents the complete auth configuration after merging
type AuthConfig struct {
    Logs       *LogsConfig          `json:"logs,omitempty" yaml:"logs,omitempty"`
    Providers  map[string]Provider  `json:"providers" yaml:"providers"`
    Identities map[string]Identity  `json:"identities" yaml:"identities"`
    Default    string              `json:"default,omitempty" yaml:"default,omitempty"`
}

// ComponentAuthConfig represents component-level auth overrides
type ComponentAuthConfig struct {
    Logs       *LogsConfig          `json:"logs,omitempty" yaml:"logs,omitempty"`
    Providers  map[string]Provider  `json:"providers,omitempty" yaml:"providers,omitempty"`
    Identities map[string]Identity  `json:"identities,omitempty" yaml:"identities,omitempty"`
    Default    string              `json:"default,omitempty" yaml:"default,omitempty"`
}
```

## 9. Migration and Adoption

### 9.1 Migration Strategy

#### Current State Assessment

- **Inventory existing authentication methods**: Document all current auth patterns
- **Identify migration candidates**: Prioritize high-impact, low-risk migrations
- **Plan rollout phases**: Gradual adoption across teams and environments

#### Migration Steps

1. **Pilot deployment**: Start with development environments
2. **Team-by-team rollout**: Migrate teams incrementally
3. **Environment progression**: Dev → Staging → Production
4. **Legacy cleanup**: Remove old authentication methods

#### Backward Compatibility

- **Graceful degradation**: Support existing auth methods during transition
- **Configuration migration**: Provide tools to convert existing configurations
- **Documentation**: Clear migration guides for each auth method

### 9.2 Training and Documentation

#### User Training

- **Quick start guides**: Get users productive quickly
- **Video tutorials**: Visual walkthroughs of common workflows
- **Office hours**: Regular Q&A sessions during rollout
- **Champions program**: Train power users to help others

#### Documentation Strategy

- **Architecture docs**: Technical deep-dive for implementers
- **User guides**: Step-by-step instructions for end users
- **Troubleshooting**: Common issues and solutions
- **Best practices**: Recommended patterns and anti-patterns

## 10. Success Criteria

### 10.1 Functional Success Criteria

#### Authentication Performance

- **Login time**: < 5 seconds for any authentication flow
- **Credential refresh**: < 2 seconds for cached credential refresh
- **Terraform prehook**: < 3 seconds for automatic authentication
- **AWS file operations**: < 1 second for credential/config file writing
- **Component description**: < 1 second for auth information display
- **Availability**: 99.9% uptime for authentication services

#### Security Compliance

- **Zero hardcoded credentials**: No secrets in configuration files
- **Audit coverage**: 100% of authentication events logged
- **Encryption**: All cached credentials encrypted at rest
- **Token lifetime**: Configurable session durations (15m-1h)

#### User Experience

- **Single sign-on**: One authentication for multiple environments
- **Clear error messages**: Actionable guidance for all error conditions
- **Intuitive CLI**: Commands follow established patterns
- **IDE integration**: Schema validation and auto-completion

### 10.2 Adoption Metrics

#### Usage Metrics

- **Active users**: Number of users authenticating weekly
- **Authentication events**: Daily authentication volume
- **Provider distribution**: Usage across different providers
- **Identity chaining**: Frequency of role assumption

#### Quality Metrics

- **Error rate**: < 1% authentication failure rate
- **Support tickets**: < 5 auth-related tickets per week
- **User satisfaction**: > 90% positive feedback
- **Time to productivity**: < 30 minutes for new user onboarding

### 10.3 Business Impact

#### Operational Efficiency

- **Reduced support burden**: Fewer authentication-related issues
- **Faster onboarding**: New team members productive faster
- **Improved compliance**: Better audit trails and access controls
- **Cost reduction**: Reduced operational overhead

#### Security Improvements

- **Eliminated credential sprawl**: Centralized credential management
- **Enhanced monitoring**: Better visibility into access patterns
- **Reduced attack surface**: Shorter-lived credentials
- **Improved incident response**: Faster credential rotation during incidents

## 11. Risk Assessment

### 11.1 Technical Risks

#### High Risk

- **Provider API changes**: External providers may change APIs
  - _Mitigation_: Version provider APIs, implement adapter patterns
- **Credential compromise**: Cached credentials could be exposed
  - _Mitigation_: Strong encryption, automatic cleanup, monitoring
- **Environment variable injection failures**: Component environment not properly updated
  - _Mitigation_: Comprehensive testing, validation of environment merging, fallback mechanisms

#### Medium Risk

- **Performance degradation**: Authentication may become bottleneck
  - _Mitigation_: Caching, parallel processing, performance monitoring
- **Complex debugging**: Multi-provider chains may be hard to troubleshoot
  - _Mitigation_: Comprehensive logging, debugging tools, clear error messages
- **SSO polling failures**: Device authorization flow may fail or timeout
  - _Mitigation_: Robust error handling, proper AWS SDK error types, user feedback
- **Keyring limitations**: System keyring may not support all required operations
  - _Mitigation_: Alternative storage backends, graceful degradation, iteration-based approaches

#### Low Risk

- **Configuration complexity**: Users may misconfigure auth settings
  - _Mitigation_: Schema validation, good defaults, clear documentation
- **Component override conflicts**: Component-level auth config may conflict with global config
  - _Mitigation_: Clear precedence rules, validation, comprehensive testing

### 11.2 Business Risks

#### High Risk

- **Adoption resistance**: Teams may resist changing existing workflows
  - _Mitigation_: Gradual rollout, training, clear benefits communication
- **Compliance gaps**: New system may not meet all compliance requirements
  - _Mitigation_: Early compliance review, audit trail design, security testing

#### Medium Risk

- **Migration complexity**: Moving from existing auth systems may be difficult
  - _Mitigation_: Migration tools, backward compatibility, phased approach
- **Vendor lock-in**: Heavy dependence on specific providers
  - _Mitigation_: Provider abstraction, multi-provider support

## 12. Conclusion

Atmos Auth provides a comprehensive, secure, and user-friendly authentication system that addresses the complex needs of modern cloud-native organizations. By implementing a clear provider-identity model with strong security practices and extensive validation, the system will significantly improve developer productivity while enhancing security posture.

The phased implementation approach ensures manageable development cycles while delivering value incrementally. Comprehensive testing and validation strategies provide confidence in the system's reliability and security.

Success will be measured through improved authentication performance, enhanced security compliance, and high user adoption rates. The risk mitigation strategies address the primary concerns around security, performance, and adoption.

This PRD serves as the foundation for implementing a world-class authentication system that will serve CloudPosse and the broader Atmos community for years to come.
