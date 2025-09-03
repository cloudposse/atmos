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
  - Support AWS IAM Identity Center, SAML, GitHub Actions OIDC
  - Support Azure Entra ID, GCP OIDC, Okta OIDC
  - Allow configuration of provider-specific parameters
  - Enable default provider designation
- **Priority**: P0 (Must Have)

#### FR-002: Identity Management

- **Description**: Manage cloud identities and role assumptions
- **Acceptance Criteria**:
  - Support AWS permission sets, assume roles, and users
  - Support Azure roles, GCP service account impersonation
  - Support Okta applications
  - Enable identity chaining via other identities or providers
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

- **Description**: Support environment variable injection from identities
- **Acceptance Criteria**:
  - Identities support `environment` block with array of key-value objects
  - Environment variables are set before Terraform execution
  - Variables are available to Terraform and other tools
  - Support for case-sensitive environment variable names
- **Priority**: P0 (Must Have)

#### FR-010: AWS Credentials and Config File Management

- **Description**: Manage isolated AWS credentials and config files for Atmos
- **Acceptance Criteria**:
  - Write AWS credentials to `~/.aws/atmos/<provider>/credentials`
  - Write AWS config to `~/.aws/atmos/<provider>/config`
  - Set `AWS_SHARED_CREDENTIALS_FILE` and `AWS_CONFIG_FILE` environment variables
  - Credential files are written during Terraform prehook
  - User's existing AWS files remain unmodified
  - Provider-specific isolation prevents credential conflicts
- **Priority**: P0 (Must Have)

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
2. SRE uses break-glass AWS user credentials
3. SRE gains emergency access to critical systems
4. All actions are logged for post-incident review

**Acceptance Criteria**:

- Reliable access when federated systems fail
- Enhanced logging for break-glass usage
- Time-limited emergency access

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

## 4. Technical Architecture

### 4.1 Core Components

#### Authentication Providers

```yaml
providers:
  aws-sso:
    kind: aws/iam-identity-center
    start_url: https://company.awsapps.com/start/
    region: us-east-1
    default: true
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
    spec:
      name: DeveloperAccess
      account.name: development
    environment:
      - key: AWS_PROFILE
        value: dev-admin
      - key: TERRAFORM_WORKSPACE
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

1. **Configuration Loading**: Parse and validate `atmos.yaml` auth section
2. **Provider Authentication**: Authenticate with configured providers
3. **Identity Resolution**: Resolve target identity through provider or chain
4. **Credential Retrieval**: Obtain and cache temporary credentials
5. **Tool Integration**: Provide credentials to Terraform/other tools

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

### 5.1 Provider Types

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

#### AWS SAML

```yaml
providers:
  aws-saml:
    kind: aws/saml
    url: https://accounts.google.com/o/saml2/initsso?...
    idp_arn: arn:aws:iam::123456789012:saml-provider/GoogleApps
    region: us-east-1
```

#### GitHub Actions OIDC

```yaml
providers:
  github-oidc:
    kind: oidc/github-actions
```

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

All identity types support an `environment` block for injecting environment variables:

```yaml
identities:
  managers:
    kind: aws/permission-set
    default: true
    via: { provider: cplive-sso }
    spec:
      name: IdentityManagersTeamAccess
      account:
        name: core-identity
    alias: managers-core
    environment:
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
    spec:
      name: ProductionAdmin
      account:
        name: production
    environment:
      - key: AWS_PROFILE
        value: prod-admin
      - key: ENVIRONMENT
        value: production
    # AWS files automatically managed:
    # ~/.aws/atmos/aws-sso/credentials
    # ~/.aws/atmos/aws-sso/config
```

**AWS File Management Behavior**:

- Credentials written to `~/.aws/atmos/<provider>/credentials` during prehook
- Config written to `~/.aws/atmos/<provider>/config` during prehook
- `AWS_SHARED_CREDENTIALS_FILE` points to Atmos-managed credentials file
- `AWS_CONFIG_FILE` points to Atmos-managed config file
- User's existing `~/.aws/credentials` and `~/.aws/config` remain untouched
- Provider isolation prevents conflicts between different auth providers

#### AWS Permission Set

```yaml
identities:
  dev-access:
    kind: aws/permission-set
    via: { provider: aws-sso }
    spec:
      name: DeveloperAccess
      account.name: development
    alias: dev
    environment:
      - key: AWS_PROFILE
        value: dev-access
      - key: ENVIRONMENT
        value: development
```

#### AWS Assume Role

```yaml
identities:
  prod-admin:
    kind: aws/assume-role
    via: { identity: dev-access }
    spec:
      assume_role: arn:aws:iam::123456789012:role/ProductionAdmin
    alias: prod
```

#### AWS User (Break-glass)

```yaml
identities:
  emergency:
    kind: aws/user
    spec:
      access_key_id: !env AWS_EMERGENCY_ACCESS_KEY_ID
      secret_access_key: !env AWS_EMERGENCY_SECRET_ACCESS_KEY
      region: us-east-1
    alias: emergency
```

#### Azure Role

```yaml
identities:
  azure-admin:
    kind: azure/role
    via: { provider: azure-entra }
    spec:
      role_definition_id: b24988ac-6180-42a0-ab88-20f7382dd24c
      subscription_id: aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee
```

#### GCP Service Account Impersonation

```yaml
identities:
  gcp-admin:
    kind: gcp/impersonate-sa
    via: { provider: gcp-oidc }
    spec:
      service_account: admin@my-project.iam.gserviceaccount.com
      project_id: my-project
```

#### Okta Application

```yaml
identities:
  monitoring-app:
    kind: okta/app
    via: { provider: okta }
    spec:
      app: datadog-admin
    alias: monitoring
```

### 5.3 Component-Level Auth Configuration

Components can override auth configuration defined in `atmos.yaml`. Here's a complete example:

#### Global Auth Configuration (`atmos.yaml`)

```yaml
auth:
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
      spec:
        name: IdentityManagersTeamAccess
        account:
          name: core-identity
      alias: managers-core
```

#### Component Stack Configuration

```yaml
components:
  terraform:
    vpc:
      # Component-specific auth overrides
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
      identities:
        security-admin:
          kind: aws/permission-set
          via: { provider: cplive-sso }
          spec:
            name: SecurityAdminAccess
            account:
              name: security
          environment:
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

# Filter to show only identities
atmos describe component vpc -s plat-ue2-sandbox -q .identities

# Filter to show only providers
atmos describe component vpc -s plat-ue2-sandbox -q .providers
```

**Expected Output for VPC Component Identities**:

```yaml
identities:
  managers:
    kind: aws/permission-set
    default: true # Overridden from false to true by component
    via: { provider: cplive-sso }
    spec:
      name: IdentityManagersTeamAccess
      account:
        name: core-identity
    alias: managers-core
    # AWS files automatically managed:
    # ~/.aws/atmos/cplive-sso/credentials
    # ~/.aws/atmos/cplive-sso/config
```

**Expected Output for Security-VPC Component Identities**:

```yaml
identities:
  managers:
    kind: aws/permission-set
    default: false # Overridden to disable for security components
    via: { provider: cplive-sso }
    spec:
      name: IdentityManagersTeamAccess
      account:
        name: core-identity
    alias: managers-core
  security-admin: # Component-specific identity
    kind: aws/permission-set
    via: { provider: cplive-sso }
    spec:
      name: SecurityAdminAccess
      account:
        name: security
    environment:
      - key: SECURITY_MODE
        value: strict
```

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
    spec:
      name: TerraformAdminAccess
      account.name: production
    alias: tf-prod

# Bad: Unclear, generic names
identities:
  identity1:
    kind: aws/permission-set
    via: { provider: sso }
    spec:
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

```
internal/auth/
├── providers/           # Authentication providers
│   ├── aws/
│   │   ├── sso.go      # IAM Identity Center
│   │   ├── saml.go     # SAML provider
│   │   └── user.go     # AWS user credentials
│   ├── azure/
│   │   └── entra.go    # Azure Entra ID
│   ├── gcp/
│   │   └── oidc.go     # GCP OIDC
│   ├── oidc/
│   │   └── github.go   # GitHub Actions OIDC
│   ├── okta/
│   │   └── oidc.go     # Okta OIDC
│   └── interface.go    # Provider interface
├── identities/         # Identity implementations
│   ├── aws/
│   │   ├── permission_set.go
│   │   ├── assume_role.go
│   │   └── user.go
│   ├── azure/
│   │   └── role.go
│   ├── gcp/
│   │   └── impersonate.go
│   ├── okta/
│   │   └── app.go
│   └── interface.go    # Identity interface
├── config/             # Configuration management
│   ├── parser.go       # YAML parsing
│   ├── validator.go    # Schema validation
│   ├── merger.go       # Component config merging
│   └── schema.json     # JSON Schema
├── credentials/        # Credential management
│   ├── store.go        # Credential storage
│   ├── cache.go        # Caching logic
│   └── crypto.go       # Encryption/decryption
├── hooks/             # Integration hooks
│   ├── terraform.go   # Terraform prehook
│   ├── aws_setup.go   # AWS file setup during prehook
│   └── interface.go   # Hook interface
├── environment/       # Environment variable management
│   ├── injector.go    # Environment variable injection
│   ├── merger.go      # Environment merging logic
│   └── aws_files.go   # AWS credentials/config file management
├── ui/                # User interface components
│   ├── picker.go      # huh-based pickers
│   ├── prompts.go     # Interactive prompts
│   └── logger.go      # Charm logging setup
├── cli/               # CLI commands
│   ├── login.go
│   ├── whoami.go
│   ├── env.go
│   ├── exec.go
│   └── validate.go
└── manager.go         # Main auth manager
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

    // Assume assumes the identity using the provided credentials
    Assume(ctx context.Context, baseCreds *Credentials) (*Credentials, error)

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

// AuthConfig represents the complete auth configuration after merging
type AuthConfig struct {
    Providers  map[string]Provider  `json:"providers" yaml:"providers"`
    Identities map[string]Identity  `json:"identities" yaml:"identities"`
    Default    string              `json:"default,omitempty" yaml:"default,omitempty"`
}

// ComponentAuthConfig represents component-level auth overrides
type ComponentAuthConfig struct {
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

#### Medium Risk

- **Performance degradation**: Authentication may become bottleneck
  - _Mitigation_: Caching, parallel processing, performance monitoring
- **Complex debugging**: Multi-provider chains may be hard to troubleshoot
  - _Mitigation_: Comprehensive logging, debugging tools, clear error messages

#### Low Risk

- **Configuration complexity**: Users may misconfigure auth settings
  - _Mitigation_: Schema validation, good defaults, clear documentation

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
