# Google Cloud Platform (GCP) Cloud Provider

This directory is reserved for Google Cloud Platform cloud provider implementation in the Atmos Auth system.

## Status

**Not Implemented** - This cloud provider is not yet implemented. The GCP cloud provider would enable authentication and credential management for Google Cloud Platform services.

## Planned Features

When implemented, the GCP cloud provider would support:

- **Google Cloud Authentication**

  - Service Account Key authentication
  - Workload Identity Federation
  - Application Default Credentials (ADC)
  - gcloud CLI integration
  - OAuth2 device flow for interactive authentication

- **Environment Management**

  - Service Account key files (`~/.gcp/atmos/<provider>/credentials.json`)
  - gcloud configuration directories
  - Environment variables for Terraform Google provider
  - Project and region management

- **Credential Types**
  - Service Account JSON keys
  - OAuth2 access tokens
  - Workload Identity tokens
  - gcloud cached credentials

## Implementation Guide

To implement GCP support, follow the [Adding Providers Guide](../docs/ADDING_PROVIDERS.md) and implement:

1. **CloudProvider Interface** (`gcp/provider.go`)

   ```go
   type GCPCloudProvider struct{}

   func (p *GCPCloudProvider) SetupEnvironment(ctx context.Context, providerName, identityName string, credentials *types.Credentials) error
   func (p *GCPCloudProvider) GetEnvironmentVariables(providerName, identityName string) map[string]string
   func (p *GCPCloudProvider) ValidateCredentials(ctx context.Context, credentials *types.Credentials) error
   ```

2. **Authentication Providers** (`providers/gcp/`)

   - `service_account_key.go` - Service Account Key authentication
   - `workload_identity.go` - Workload Identity Federation
   - `application_default.go` - Application Default Credentials
   - `oauth2.go` - OAuth2 device flow

3. **Identity Types** (`identities/gcp/`)

   - `service_account.go` - Service Account impersonation
   - `workload_identity.go` - Workload Identity identity

4. **Schema Updates** (`pkg/schema/schema.go`)
   ```go
   type GCPCredentials struct {
       AccessToken         string `json:"access_token,omitempty"`
       ServiceAccountKey   string `json:"service_account_key,omitempty"`
       ServiceAccountEmail string `json:"service_account_email,omitempty"`
       ProjectID          string `json:"project_id,omitempty"`
       Expiration         string `json:"expiration,omitempty"`
   }
   ```

## Configuration Examples

### Service Account Key Provider

```yaml
providers:
  gcp-sa-key:
    kind: gcp/service-account-key
    project_id: "my-gcp-project"
    service_account_key: !env GCP_SERVICE_ACCOUNT_KEY
```

### Workload Identity Provider

```yaml
providers:
  gcp-workload:
    kind: gcp/workload-identity
    project_id: "my-gcp-project"
    spec:
      service_account_email: "terraform@my-project.iam.gserviceaccount.com"
      audience: "//iam.googleapis.com/projects/123456789/locations/global/workloadIdentityPools/my-pool/providers/my-provider"
```

### Service Account Identity

```yaml
identities:
  gcp-admin:
    kind: gcp/service-account
    default: true
    via:
      provider: gcp-workload
    principal:
      email: "admin@my-project.iam.gserviceaccount.com"
      scopes:
        - "https://www.googleapis.com/auth/cloud-platform"
```

### Application Default Credentials

```yaml
providers:
  gcp-adc:
    kind: gcp/application-default
    project_id: "my-gcp-project"
```

## Environment Variables

When implemented, the GCP provider would set:

- `GOOGLE_APPLICATION_CREDENTIALS` - Path to service account key file
- `GOOGLE_CLOUD_PROJECT` - Default GCP project ID
- `GCLOUD_PROJECT` - gcloud CLI project
- `CLOUDSDK_CONFIG` - gcloud configuration directory
- `CLOUDSDK_CORE_PROJECT` - gcloud core project setting
- `GOOGLE_OAUTH_ACCESS_TOKEN` - OAuth2 access token (if applicable)

## File Structure

```
~/.gcp/atmos/<provider>/
├── credentials.json          # Service account key file
├── gcloud/                  # gcloud configuration
│   ├── configurations/
│   │   └── config_<identity>
│   └── credentials.db
└── application_default_credentials.json  # ADC file
```

## Dependencies

Implementation would require:

- `google.golang.org/api` - Google APIs Go client library
- `cloud.google.com/go` - Google Cloud Go SDK
- `golang.org/x/oauth2/google` - Google OAuth2 library
- gcloud CLI integration for token management

## Authentication Flows

### Service Account Key Flow

1. Load service account key from environment or file
2. Create JWT token using private key
3. Exchange JWT for access token
4. Write credentials to provider-specific location

### Workload Identity Flow

1. Get OIDC token from metadata service or external provider
2. Exchange OIDC token for Google access token using STS
3. Optionally impersonate service account
4. Return structured credentials

### Application Default Credentials Flow

1. Check for explicit service account key
2. Check for gcloud credentials
3. Check for metadata service (GCE/Cloud Run/etc.)
4. Use discovered credentials

## Security Considerations

- Service account keys should be rotated regularly
- Use Workload Identity when possible to avoid long-lived keys
- Implement proper scoping for service accounts
- Secure storage of credential files with proper permissions (0600)
- Clean up temporary credential files on exit

## Testing Strategy

- Mock Google Cloud APIs for unit tests
- Test credential validation and refresh
- Test file permission handling
- Integration tests with real GCP resources (optional)
- Test Workload Identity token exchange flows

## Contributing

To contribute GCP support:

1. Review the [Architecture Documentation](../docs/ARCHITECTURE.md)
2. Follow the [Adding Providers Guide](../docs/ADDING_PROVIDERS.md)
3. Implement the CloudProvider interface
4. Add authentication providers and identities
5. Create comprehensive tests
6. Update documentation

For questions or discussions about GCP implementation, please open an issue in the Atmos repository.
