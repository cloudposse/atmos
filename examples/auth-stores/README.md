# Auth Identity for Stores Example

Demonstrates how stores authenticate using Atmos auth identities instead of the default credential chain.

Each store references a named identity via the `identity` field. When the store is accessed, Atmos authenticates using the referenced identity and passes the resolved credentials to the cloud SDK.

## Configuration

```yaml
stores:
  prod/ssm:
    type: aws-ssm-parameter-store
    identity: prod-admin          # Uses this identity for AWS credentials
    options:
      region: us-east-1
```

## Supported Stores

| Store Type | Identity Kind |
|---|---|
| `aws-ssm-parameter-store` | Any AWS identity |
| `azure-key-vault` | Any Azure identity |
| `google-secret-manager` | Any GCP identity |

## Learn More

See [Stores documentation](https://atmos.tools/core-concepts/stacks/stores/).
