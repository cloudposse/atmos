---
title: Auth Identities for Stores
tags: [Stacks]
---

# Auth Identity for Stores Example

Demonstrates how stores authenticate using Atmos auth identities instead of the default credential chain.

Each store references a named identity via the `identity` field. When the store is accessed, Atmos authenticates using the referenced identity and passes the resolved credentials to the cloud SDK.

## Configuration

```yaml
stores:
  prod/ssm:
    kind: aws/ssm
    identity: prod-admin          # Uses this identity for AWS credentials
    options:
      region: us-east-1
```

## Supported Stores

| Store Kind | Identity Kind |
|---|---|
| `aws/ssm` | Any AWS identity |
| `azure/keyvault` | Any Azure identity |
| `gcp/secretmanager` | Any GCP identity |

## Learn More

See [Stores documentation](https://atmos.tools/core-concepts/stacks/stores/).
