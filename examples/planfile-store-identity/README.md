---
title: Per-Store Identity for Planfile Storage
tags: [Automation]
related_docs:
  - label: "Planfile storage"
    url: /ci/planfile-storage
  - label: "Terraform component configuration"
    url: /cli/configuration/components/terraform
  - label: "Authentication configuration"
    url: /cli/configuration/auth
---

# Per-Store Identity for Planfile Storage

Demonstrates authenticating a [planfile store](https://atmos.tools/ci/planfile-storage) with a specific
[Atmos auth identity](https://atmos.tools/cli/configuration/auth), independent of the component's identity.

In a multi-cloud pipeline, a component often authenticates with one cloud while its planfile store lives in
another. For example, a component authenticates with an `azure/subscription` identity, but the planfiles are
stored in an `aws/s3` bucket. Without a per-store identity, the S3 upload inherits the component's non-AWS
identity and fails to resolve AWS credentials.

The `identity` field on a store fixes this: the store authenticates with its own identity, while the
component keeps using its own.

## Configuration

```yaml
components:
  terraform:
    planfiles:
      default: s3
      stores:
        s3:
          type: aws/s3
          identity: aws-backend    # store authenticates with this identity
          options:
            bucket: example-terraform-planfiles
            region: us-east-1

auth:
  identities:
    azure-plan:                    # component's default identity (non-AWS)
      kind: azure/subscription
      default: true
      # ...
    aws-backend:                   # AWS identity used only by the planfile store
      kind: aws/assume-role
      # ...
```

When `identity` is omitted, the store inherits the component's identity (the prior behavior), so existing
single-cloud setups are unchanged.

## Run

```shell
atmos validate stacks
atmos describe config
```

## Learn More

See the [Planfile Storage documentation](https://atmos.tools/ci/planfile-storage#store-identity).
