# AWS Cloud Package

This package provides AWS-specific functionality for the Atmos authentication system.

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

The resolver is implemented using AWS SDK v2's `EndpointResolverWithOptions`:

```go
resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
    return aws.Endpoint{
        URL:               url,
        HostnameImmutable: true, // prevent SDK from rewriting the host
    }, nil
})
```

This ensures all AWS services (STS, SSO, etc.) use the custom endpoint.
