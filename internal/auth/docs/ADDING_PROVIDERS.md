# Adding New Cloud Providers to Atmos Auth

## Overview

This guide explains how to extend the Atmos Auth framework to support new cloud providers (GCP, Azure, Okta, etc.). The framework is designed to be extensible, with clear interfaces and patterns for adding new authentication methods.

## Quick Start Checklist

- [ ] Implement `CloudProvider` interface
- [ ] Create provider-specific implementations
- [ ] Create identity-specific implementations
- [ ] Add factory registration
- [ ] Add validation rules
- [ ] Create tests
- [ ] Update documentation

## Step 1: Implement CloudProvider Interface

Create a new cloud provider by implementing the `CloudProvider` interface:

```go
// internal/auth/cloud/gcp/provider.go
package gcp

import (
    "context"
    "github.com/cloudposse/atmos/internal/auth/types"
    "github.com/cloudposse/atmos/pkg/schema"
)

type gcpProvider struct {
    name string
}

func NewGCPProvider(name string) types.CloudProvider {
    return &gcpProvider{
        name: name,
    }
}

func (p *gcpProvider) GetName() string {
    return "gcp"
}

func (p *gcpProvider) SetupEnvironment(ctx context.Context, providerName, identityName string, credentials *types.Credentials) error {
    // Setup GCP-specific environment (service account keys, etc.)
    if credentials.GCP == nil {
        return fmt.Errorf("no GCP credentials provided")
    }

    // Write service account key file
    keyPath := filepath.Join(os.TempDir(), fmt.Sprintf("gcp-%s-%s.json", providerName, identityName))
    if err := p.writeServiceAccountKey(keyPath, credentials.GCP); err != nil {
        return fmt.Errorf("failed to write service account key: %w", err)
    }

    // Set GOOGLE_APPLICATION_CREDENTIALS
    os.Setenv("GOOGLE_APPLICATION_CREDENTIALS", keyPath)

    return nil
}

func (p *gcpProvider) GetEnvironmentVariables(providerName, identityName string) map[string]string {
    keyPath := filepath.Join(os.TempDir(), fmt.Sprintf("gcp-%s-%s.json", providerName, identityName))
    return map[string]string{
        "GOOGLE_APPLICATION_CREDENTIALS": keyPath,
        "GOOGLE_PROJECT":                 p.getProjectID(providerName, identityName),
    }
}

func (p *gcpProvider) Cleanup(ctx context.Context, providerName, identityName string) error {
    // Remove temporary service account key files
    keyPath := filepath.Join(os.TempDir(), fmt.Sprintf("gcp-%s-%s.json", providerName, identityName))
    return os.Remove(keyPath)
}

func (p *gcpProvider) ValidateCredentials(ctx context.Context, credentials *types.Credentials) error {
    if credentials.GCP == nil {
        return fmt.Errorf("GCP credentials are required")
    }

    // Validate service account key format
    if credentials.GCP.ServiceAccountKey == "" {
        return fmt.Errorf("service account key is required")
    }

    // Test credentials by making a simple API call
    return p.testCredentials(ctx, credentials.GCP)
}

func (p *gcpProvider) GetCredentialFilePaths(providerName string) map[string]string {
    return map[string]string{
        "service_account_key": filepath.Join(os.TempDir(), fmt.Sprintf("gcp-%s.json", providerName)),
    }
}
```

## Step 2: Create Authentication Providers

Implement specific authentication providers for your cloud:

```go
// internal/auth/providers/gcp/workload_identity.go
package gcp

import (
    "context"
    "github.com/cloudposse/atmos/internal/auth/types"
    "github.com/cloudposse/atmos/pkg/schema"
)

type workloadIdentityProvider struct {
    name   string
    config *schema.Provider
}

func NewWorkloadIdentityProvider(name string, config *schema.Provider) (types.Provider, error) {
    if config.Region == "" {
        return nil, fmt.Errorf("region is required for GCP Workload Identity provider")
    }

    return &workloadIdentityProvider{
        name:   name,
        config: config,
    }, nil
}

func (p *workloadIdentityProvider) Kind() string {
    return "gcp/workload-identity"
}

func (p *workloadIdentityProvider) Authenticate(ctx context.Context) (*types.Credentials, error) {
    // Implement GCP Workload Identity authentication
    // 1. Get OIDC token from metadata service
    // 2. Exchange for GCP access token
    // 3. Return credentials

    token, err := p.getWorkloadIdentityToken(ctx)
    if err != nil {
        return nil, fmt.Errorf("failed to get workload identity token: %w", err)
    }

    accessToken, err := p.exchangeToken(ctx, token)
    if err != nil {
        return nil, fmt.Errorf("failed to exchange token: %w", err)
    }

    return &types.Credentials{
        GCP: &schema.GCPCredentials{
            AccessToken: accessToken,
            ProjectID:   p.config.Spec["project_id"].(string),
        },
    }, nil
}

func (p *workloadIdentityProvider) Validate() error {
    if p.config.Spec == nil {
        return fmt.Errorf("spec is required for workload identity provider")
    }

    if _, ok := p.config.Spec["service_account_email"]; !ok {
        return fmt.Errorf("service_account_email is required in spec")
    }

    return nil
}

func (p *workloadIdentityProvider) Environment() (map[string]string, error) {
    return map[string]string{
        "GOOGLE_CLOUD_PROJECT": p.config.Spec["project_id"].(string),
    }, nil
}
```

## Step 3: Create Identity Implementations

Implement identities that use your providers:

```go
// internal/auth/identities/gcp/service_account.go
package gcp

import (
    "context"
    "github.com/cloudposse/atmos/internal/auth/types"
    "github.com/cloudposse/atmos/pkg/schema"
)

type serviceAccountIdentity struct {
    name   string
    config *schema.Identity
}

func NewServiceAccountIdentity(name string, config *schema.Identity) (types.Identity, error) {
    return &serviceAccountIdentity{
        name:   name,
        config: config,
    }, nil
}

func (i *serviceAccountIdentity) Kind() string {
    return "gcp/service-account"
}

func (i *serviceAccountIdentity) Authenticate(ctx context.Context, baseCreds *types.Credentials) (*types.Credentials, error) {
    // Use base credentials to impersonate service account
    if baseCreds.GCP == nil {
        return nil, fmt.Errorf("GCP base credentials required")
    }

    serviceAccountEmail := i.config.Principal["email"].(string)

    // Impersonate service account using base credentials
    impersonatedToken, err := i.impersonateServiceAccount(ctx, baseCreds.GCP, serviceAccountEmail)
    if err != nil {
        return nil, fmt.Errorf("failed to impersonate service account: %w", err)
    }

    return &types.Credentials{
        GCP: &schema.GCPCredentials{
            AccessToken:          impersonatedToken,
            ServiceAccountEmail:  serviceAccountEmail,
            ProjectID:           baseCreds.GCP.ProjectID,
        },
    }, nil
}

func (i *serviceAccountIdentity) Validate() error {
    if i.config.Principal == nil {
        return fmt.Errorf("principal is required for service account identity")
    }

    if _, ok := i.config.Principal["email"]; !ok {
        return fmt.Errorf("service account email is required in principal")
    }

    return nil
}

func (i *serviceAccountIdentity) Environment() (map[string]string, error) {
    return map[string]string{}, nil
}

func (i *serviceAccountIdentity) Merge(component *schema.Identity) types.Identity {
    // Implement merging logic for component overrides
    merged := *i.config
    if component.Principal != nil {
        for k, v := range component.Principal {
            merged.Principal[k] = v
        }
    }

    return &serviceAccountIdentity{
        name:   i.name,
        config: &merged,
    }
}
```

## Step 4: Register in Factory

Add your new providers and identities to the factory:

```go
// internal/auth/factory.go
func NewProvider(name string, config *schema.Provider) (types.Provider, error) {
    switch config.Kind {
    case "aws/iam-identity-center":
        return awsProviders.NewSSOProvider(name, config)
    case "aws/saml":
        return awsProviders.NewSAMLProvider(name, config)
    case "github/oidc":
        return githubProviders.NewOIDCProvider(name, config)
    // Add GCP providers
    case "gcp/workload-identity":
        return gcpProviders.NewWorkloadIdentityProvider(name, config)
    case "gcp/service-account-key":
        return gcpProviders.NewServiceAccountKeyProvider(name, config)
    default:
        return nil, fmt.Errorf("unsupported provider kind: %s", config.Kind)
    }
}

func NewIdentity(name string, config *schema.Identity) (types.Identity, error) {
    switch config.Kind {
    case "aws/permission-set":
        return aws.NewPermissionSetIdentity(name, config)
    case "aws/assume-role":
        return aws.NewAssumeRoleIdentity(name, config)
    case "aws/user":
        return aws.NewUserIdentity(name, config)
    // Add GCP identities
    case "gcp/service-account":
        return gcp.NewServiceAccountIdentity(name, config)
    default:
        return nil, fmt.Errorf("unsupported identity kind: %s", config.Kind)
    }
}
```

## Step 5: Register Cloud Provider

Add your cloud provider to the cloud provider factory:

```go
// internal/auth/cloud/factory.go
func NewCloudProviderFactory() types.CloudProviderFactory {
    factory := &cloudProviderFactory{
        providers: make(map[string]types.CloudProvider),
    }

    // Register built-in providers
    factory.RegisterCloudProvider("aws", aws.NewAWSProvider("aws"))
    factory.RegisterCloudProvider("gcp", gcp.NewGCPProvider("gcp"))
    factory.RegisterCloudProvider("azure", azure.NewAzureProvider("azure"))

    return factory
}
```

## Step 6: Add Validation Rules

Extend the validator to support your new provider types:

```go
// internal/auth/validation/validator.go
func (v *validator) ValidateProvider(name string, provider *schema.Provider) error {
    // ... existing validation ...

    switch provider.Kind {
    // ... existing cases ...
    case "gcp/workload-identity":
        return v.validateGCPWorkloadIdentityProvider(provider)
    case "gcp/service-account-key":
        return v.validateGCPServiceAccountKeyProvider(provider)
    default:
        return fmt.Errorf("unsupported provider kind: %s", provider.Kind)
    }
}

func (v *validator) validateGCPWorkloadIdentityProvider(provider *schema.Provider) error {
    if provider.Region == "" {
        return fmt.Errorf("region is required for GCP Workload Identity provider")
    }

    if provider.Spec == nil {
        return fmt.Errorf("spec is required for GCP Workload Identity provider")
    }

    if _, ok := provider.Spec["service_account_email"]; !ok {
        return fmt.Errorf("service_account_email is required in spec")
    }

    return nil
}
```

## Step 7: Update Schema

Add new credential types to the schema:

```go
// pkg/schema/schema.go
type Credentials struct {
    AWS   *AWSCredentials   `json:"aws,omitempty"`
    GCP   *GCPCredentials   `json:"gcp,omitempty"`
    Azure *AzureCredentials `json:"azure,omitempty"`
}

type GCPCredentials struct {
    AccessToken         string `json:"access_token,omitempty"`
    ServiceAccountKey   string `json:"service_account_key,omitempty"`
    ServiceAccountEmail string `json:"service_account_email,omitempty"`
    ProjectID          string `json:"project_id,omitempty"`
    Expiration         string `json:"expiration,omitempty"`
}

type AzureCredentials struct {
    AccessToken    string `json:"access_token,omitempty"`
    TenantID      string `json:"tenant_id,omitempty"`
    ClientID      string `json:"client_id,omitempty"`
    ClientSecret  string `json:"client_secret,omitempty"`
    SubscriptionID string `json:"subscription_id,omitempty"`
    Expiration    string `json:"expiration,omitempty"`
}
```

## Step 8: Create Tests

Create comprehensive tests for your new providers:

```go
// internal/auth/providers/gcp/workload_identity_test.go
func TestWorkloadIdentityProvider_Authenticate(t *testing.T) {
    tests := []struct {
        name        string
        config      *schema.Provider
        expectError bool
    }{
        {
            name: "valid configuration",
            config: &schema.Provider{
                Kind:   "gcp/workload-identity",
                Region: "us-central1",
                Spec: map[string]interface{}{
                    "service_account_email": "test@project.iam.gserviceaccount.com",
                    "project_id":           "test-project",
                },
            },
            expectError: false,
        },
        // Add more test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            provider, err := NewWorkloadIdentityProvider("test", tt.config)
            if tt.expectError {
                assert.Error(t, err)
                return
            }

            assert.NoError(t, err)
            assert.NotNil(t, provider)

            // Test authentication (may require mocking)
            ctx := context.Background()
            creds, err := provider.Authenticate(ctx)

            // Add assertions based on your implementation
        })
    }
}
```

## Step 9: Add CLI Commands (Optional)

If your provider needs specific CLI commands:

```go
// cmd/auth_gcp.go
var authGCPCmd = &cobra.Command{
    Use:   "gcp",
    Short: "GCP authentication commands",
}

var authGCPLoginCmd = &cobra.Command{
    Use:   "login",
    Short: "Login to GCP using application default credentials",
    RunE: func(cmd *cobra.Command, args []string) error {
        // Implement GCP-specific login flow
        return executeGCPLogin(cmd, args)
    },
}

func init() {
    authGCPCmd.AddCommand(authGCPLoginCmd)
    authCmd.AddCommand(authGCPCmd)
}
```

## Configuration Examples

### GCP Workload Identity Provider

```yaml
providers:
  gcp-workload:
    kind: gcp/workload-identity
    region: us-central1
    spec:
      service_account_email: terraform@my-project.iam.gserviceaccount.com
      project_id: my-project
```

### GCP Service Account Identity

```yaml
identities:
  gcp-admin:
    kind: gcp/service-account
    default: true
    via:
      provider: gcp-workload
    principal:
      email: admin@my-project.iam.gserviceaccount.com
```

## Best Practices

### Security

- Never log credentials or sensitive information
- Use secure credential storage (OS keyring)
- Implement proper credential expiration
- Clean up temporary files and environment variables

### Error Handling

- Provide clear, actionable error messages
- Include context about which step failed
- Handle network timeouts and retries gracefully
- Validate configurations early

### Testing

- Mock external API calls in unit tests
- Test error conditions and edge cases
- Use integration tests for end-to-end flows
- Test credential expiration scenarios

### Documentation

- Document all configuration options
- Provide working examples
- Include troubleshooting guides
- Document any prerequisites or setup requirements

## Common Patterns

### Token Exchange

Many cloud providers use OAuth2 or similar token exchange patterns:

```go
func (p *provider) exchangeToken(ctx context.Context, sourceToken string) (*Credentials, error) {
    // 1. Prepare token exchange request
    // 2. Make HTTP request to token endpoint
    // 3. Parse response and extract credentials
    // 4. Set expiration time
    // 5. Return structured credentials
}
```

### Credential Refresh

Implement automatic credential refresh for long-running processes:

```go
func (p *provider) refreshCredentials(ctx context.Context, creds *Credentials) (*Credentials, error) {
    if !p.isExpired(creds) {
        return creds, nil
    }

    // Refresh logic here
    return p.Authenticate(ctx)
}
```

### Environment Setup

Follow consistent patterns for environment variable management:

```go
func (p *cloudProvider) SetupEnvironment(ctx context.Context, providerName, identityName string, creds *Credentials) error {
    // 1. Write credential files to provider-specific locations
    // 2. Set cloud-specific environment variables
    // 3. Ensure proper file permissions
    // 4. Register cleanup handlers
}
```

This framework provides a solid foundation for adding any cloud provider while maintaining consistency, security, and usability across the Atmos Auth system.
