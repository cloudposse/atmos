---
title: Tagging with Null Label
tags: [Stacks, Components]
description: Pass shared stack tags to Null Label and combine them with generated context tags.
cast:
  file: /casts/examples/tagging-null-label/plan-tags.cast
  title: Pass inherited tags to Null Label
related_docs:
  - label: "Tagging strategy tutorial"
    url: /tutorials/tagging-strategy
  - label: "Stack imports"
    url: /stacks/imports
---

# Tagging with Null Label

Pass inherited stack tags to the [Null Label module](https://github.com/cloudposse/terraform-null-label).
The module combines them with tags generated from the component's namespace, stage, and name.

| File | Configuration |
|------|---------------|
| `stacks/catalog/app.yaml` | Shared `namespace`, `name`, and `tags` |
| `stacks/dev.yaml` | Imports the shared configuration and sets `stage: dev` |
| `stacks/prod.yaml` | Imports the shared configuration and sets `stage: prod` |

## Try It

```shell
atmos init examples/tagging-null-label
cd tagging-null-label

atmos describe component app -s dev --query .vars
atmos terraform plan app -s dev
```

The plan's `tags` output contains:

```hcl
Name       = "acme-dev-app"
Namespace  = "acme"
Stage      = "dev"
managed-by = "atmos"
owner      = "platform"
```

Run the same plan with `-s prod` to see `Name = "acme-prod-app"` and `Stage = "prod"`,
while the shared tags stay the same. Run `atmos test` to check both stacks.

The component uses the real `cloudposse/label/null` module. A built-in `terraform_data`
resource displays its tag map without creating cloud resources. No cloud credentials are
needed; the first plan downloads the module from the Terraform registry.

In a cloud component, pass `module.this.tags` to the resource's tags or labels argument.
Account for that cloud and provider's character, casing, length, and tag-count limits:
Null Label preserves the tags you supply and does not normalize them for each provider.

For a larger organizational hierarchy and source-control tags, see the
[tagging strategy tutorial](https://atmos.tools/tutorials/tagging-strategy).
For AWS provider-level tagging, see [tagging-default-tags](../tagging-default-tags).
