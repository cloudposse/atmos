# AWS Cloud Package

This package provides AWS-specific functionality for the Atmos authentication system.

## Session Duration Configuration

Session duration can be configured for both providers (SSO, SAML) and identities (IAM users):

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      session:
        duration: "8h"

  identities:
    emergency-admin:
      kind: aws/user
      session:
        duration: "12h"  # Formats: integers (seconds), Go durations ("1h"), or days ("1d")
```

**AWS IAM user limits**: 15m-12h (no MFA) or 15m-36h (with MFA). Default: 12h

## Custom Endpoint Resolver

The AWS cloud package supports custom endpoint resolvers, which is useful for testing with LocalStack or other AWS-compatible services.

### Configuration

You can configure a custom endpoint resolver at either the identity or provider level:

#### Identity-Level Configuration

For identities, add the `aws` configuration to the `credentials` map:

```yaml
auth:
  identities:
    localstack-superuser:
      kind: aws/user
      credentials:
        access_key_id: test
        secret_access_key: test
        region: us-east-1
        aws:
          resolver:
            url: "http://localhost:4566"
```

#### Provider-Level Configuration

For providers, add the `aws` configuration to the `spec` map:

```yaml
auth:
  providers:
    localstack-sso:
      kind: aws/iam-identity-center
      start_url: https://localstack.awsapps.com/start/
      region: us-east-1
      spec:
        aws:
          resolver:
            url: "http://localhost:4566"
```

### Precedence

When both identity and provider have resolver configurations, the **identity resolver takes precedence**.

### Usage

The custom endpoint resolver is automatically applied when:
- AWS identities authenticate (user, assume-role, permission-set)
- AWS providers authenticate (SSO, SAML)

All AWS SDK calls will be directed to the configured endpoint URL.

### LocalStack Example

For a complete LocalStack example, see:
- `/examples/demo-localstack/atmos.yaml`

### Implementation Details

The resolver is implemented using AWS SDK v2's `config.WithBaseEndpoint`:

```go
return config.WithBaseEndpoint(url)
```

This ensures all AWS services (STS, SSO, etc.) use the custom endpoint. The base endpoint approach is the recommended method in AWS SDK v2 for setting custom endpoints.
