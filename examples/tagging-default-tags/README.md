---
title: Tagging with AWS Default Tags
tags: [Stacks, Components]
description: Define shared AWS provider tags once and add an environment tag in each stack.
cast:
  file: /casts/examples/tagging-default-tags/inherit-tags.cast
  title: Inherit AWS default tags
related_docs:
  - label: "Provider configuration"
    url: /stacks/providers
  - label: "Tagging strategy tutorial"
    url: /tutorials/tagging-strategy
---

# Tagging with AWS Default Tags

Define shared tags once. Each stack inherits them and adds its own environment tag.
Atmos merges the maps into the AWS provider's `default_tags` configuration.

| File | Tags |
|------|------|
| `stacks/catalog/app.yaml` | `managed-by: atmos`, `owner: platform` |
| `stacks/dev.yaml` | `environment: dev` |
| `stacks/prod.yaml` | `environment: prod` |

## Try It

```shell
atmos init examples/tagging-default-tags
cd tagging-default-tags

atmos describe component app -s dev --query .providers.aws.default_tags.tags
atmos describe component app -s prod --query .providers.aws.default_tags.tags
```

The dev stack resolves to:

```yaml
environment: dev
managed-by: atmos
owner: platform
```

The prod stack has the same shared tags, with `environment: prod`.
These commands and `atmos test` need no AWS credentials and create no resources.

## How the Resource Gets Its Tags

The component contains an S3 bucket with no `tags` argument. During Terraform operations,
Atmos writes the merged provider configuration to `providers_override.tf.json`, and the AWS
provider applies the default tags to the bucket. Planning or applying requires the AWS provider
and AWS credentials; applying creates a real bucket.

Default tags apply to supported, taggable AWS resources. Some resources, such as
`aws_autoscaling_group`, need resource-specific tagging configuration.

For source-control tags, CI overrides, and a larger organizational hierarchy, see the
[tagging strategy tutorial](https://atmos.tools/tutorials/tagging-strategy).
For module-based tagging, see [tagging-null-label](../tagging-null-label).
