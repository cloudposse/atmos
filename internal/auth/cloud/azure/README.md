# Azure Cloud Provider

This directory is reserved for Azure cloud provider implementation in the Atmos Auth system.

## Status

**Not Implemented** - This cloud provider is not yet implemented. The Azure cloud provider would enable authentication and credential management for Microsoft Azure services.

## Planned Features

When implemented, the Azure cloud provider would support:

- **Azure Active Directory (AAD) Authentication**
  - Service Principal authentication
  - Managed Identity support
  - Device code flow for interactive authentication
  - Azure CLI integration

- **Environment Management**
  - Azure CLI configuration files (`~/.azure/atmos/<provider>/config`)
  - Service principal credential files
  - Environment variables for Terraform Azure provider
  - Subscription and tenant management

- **Credential Types**
  - Service Principal (Client ID + Secret)
  - Managed Identity tokens
  - Azure CLI cached tokens
  - Certificate-based authentication

## Implementation Guide

To implement Azure support, follow the [Adding Providers Guide](../docs/ADDING_PROVIDERS.md) and implement:

1. **CloudProvider Interface** (`azure/provider.go`)
   ```go
   type AzureCloudProvider struct{}
   
   func (p *AzureCloudProvider) SetupEnvironment(ctx context.Context, providerName, identityName string, credentials *schema.Credentials) error
   func (p *AzureCloudProvider) GetEnvironmentVariables(providerName, identityName string) map[string]string
   func (p *AzureCloudProvider) ValidateCredentials(ctx context.Context, credentials *schema.Credentials) error
   ```

2. **Authentication Providers** (`providers/azure/`)
   - `service_principal.go` - Service Principal authentication
   - `managed_identity.go` - Managed Identity authentication
   - `device_code.go` - Interactive device code flow

3. **Identity Types** (`identities/azure/`)
   - `service_principal.go` - Service Principal identity
   - `managed_identity.go` - Managed Identity identity

4. **Schema Updates** (`pkg/schema/schema.go`)
   ```go
   type AzureCredentials struct {
       AccessToken    string `json:"access_token,omitempty"`
       TenantID      string `json:"tenant_id,omitempty"`
       ClientID      string `json:"client_id,omitempty"`
       ClientSecret  string `json:"client_secret,omitempty"`
       SubscriptionID string `json:"subscription_id,omitempty"`
       Expiration    string `json:"expiration,omitempty"`
   }
   ```

## Configuration Examples

### Service Principal Provider
```yaml
providers:
  azure-sp:
    kind: azure/service-principal
    tenant_id: "12345678-1234-1234-1234-123456789012"
    client_id: "87654321-4321-4321-4321-210987654321"
    client_secret: !env AZURE_CLIENT_SECRET
```

### Managed Identity Provider
```yaml
providers:
  azure-mi:
    kind: azure/managed-identity
    subscription_id: "12345678-1234-1234-1234-123456789012"
```

### Service Principal Identity
```yaml
identities:
  azure-admin:
    kind: azure/service-principal
    default: true
    via:
      provider: azure-sp
    principal:
      subscription_id: "12345678-1234-1234-1234-123456789012"
      resource_group: "my-resource-group"
```

## Environment Variables

When implemented, the Azure provider would set:

- `AZURE_CLIENT_ID` - Service Principal Client ID
- `AZURE_CLIENT_SECRET` - Service Principal Secret
- `AZURE_TENANT_ID` - Azure AD Tenant ID
- `AZURE_SUBSCRIPTION_ID` - Azure Subscription ID
- `AZURE_CONFIG_DIR` - Azure CLI config directory
- `ARM_CLIENT_ID` - Terraform Azure provider client ID
- `ARM_CLIENT_SECRET` - Terraform Azure provider client secret
- `ARM_TENANT_ID` - Terraform Azure provider tenant ID
- `ARM_SUBSCRIPTION_ID` - Terraform Azure provider subscription ID

## Dependencies

Implementation would require:

- `github.com/Azure/azure-sdk-for-go` - Azure SDK for Go
- `github.com/Azure/go-autorest` - Azure authentication library
- Azure CLI integration for token management

## Contributing

To contribute Azure support:

1. Review the [Architecture Documentation](../docs/ARCHITECTURE.md)
2. Follow the [Adding Providers Guide](../docs/ADDING_PROVIDERS.md)
3. Implement the CloudProvider interface
4. Add authentication providers and identities
5. Create comprehensive tests
6. Update documentation

For questions or discussions about Azure implementation, please open an issue in the Atmos repository.
