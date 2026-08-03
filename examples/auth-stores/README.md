---
title: Auth Identities for Stores
tags: [Stacks]
cast:
  file: /casts/examples/auth-stores/identity-backed-stores.cast
  title: atmos auth-backed stores
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

## Manage Store Values with the CLI

Use the `atmos store` command group to read, write, delete, and list values in any of these
stores. Each command uses the identity configured on the target store:

```shell
atmos store list
atmos store set prod/ssm image_tag sha256:abc123 --stack=prod --component=ecs-service
atmos store get prod/ssm image_tag --stack=prod --component=ecs-service
```

## Learn More

See [Stores documentation](https://atmos.tools/core-concepts/stacks/stores/).
