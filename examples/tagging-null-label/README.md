---
title: Tagging with Null Label
tags: [Stacks, Components]
description: >-
  Define resource tags once and inherit them in every stack, then apply them with the Null Label
  naming module. Includes bridging metadata.labels into tags with !labels.
related_docs:
  - label: "Tagging strategy tutorial"
    url: /tutorials/tagging-strategy
  - label: "Stack names"
    url: /stacks/name
  - label: "!git.repository"
    url: /functions/yaml/git.repository
  - label: "!labels"
    url: /functions/yaml/labels
---

# Example: Tagging with Null Label

Define tags and the label context (`namespace`, `tenant`, `stage`) once in stack
configuration. The component passes them to the
[`cloudposse/label/null`](https://github.com/cloudposse/terraform-null-label) module through
`context.tf`, and applies `module.this.tags` to its resources. This works on any cloud and with any provider.

The `app` component creates a `terraform_data` resource, which ships with Terraform and OpenTofu, so
planning it needs no provider or credentials. The module itself is downloaded from the Terraform
registry the first time you plan.

Tags are defined at three import levels and deep-merged:

| Level | File | Tags |
|-------|------|------|
| Organization | `orgs/acme/_defaults.yaml` | `managed-by`, `repository`, `source-branch`, `source-commit`, `stack`, `component`, `region` |
| Tenant | `orgs/acme/payments/_defaults.yaml` | `product`, `owner` |
| Stage | `orgs/acme/payments/{dev,prod}.yaml` | `tier` |

## Layout

```text
atmos.yaml                                   # Templating enabled, stack names from name_template
components/terraform/app/
  context.tf                                 # Label context inputs and module.this
  main.tf                                    # Resource tagged with module.this.tags
stacks/
  catalog/app/                               # app and app-labeled component definitions
  orgs/acme/_defaults.yaml                   # Organization defaults
  orgs/acme/payments/_defaults.yaml          # Tenant defaults
  orgs/acme/payments/dev.yaml                # Stack payments-dev
  orgs/acme/payments/prod.yaml               # Stack payments-prod
```

## Try It

```shell
atmos init examples/tagging-null-label
cd tagging-null-label

atmos list stacks
atmos describe component app --stack=payments-dev --query=.vars
atmos terraform plan app --stack=payments-dev
```

Run all the credential-free checks with `atmos test`.

`label_key_case: lower` and `label_value_case: lower` set the case of the tags the module generates
from the label context (`namespace`, `tenant`, `stage`, and the `name` tag), so those
keys do not need to appear in `vars.tags`. The module does not change the keys or values you pass in
`vars.tags`, so the convention's lowercase rule applies to what you write there.

## Labels: `vars.tags: !labels`

The `app-labeled` component bridges `metadata.labels` into `vars.tags`.

```shell
atmos describe component app-labeled --stack=payments-dev --query=.vars.tags
```

The inherited organization, tenant, and stage tags merge into the `!labels` result, and their
templates and Git functions are rendered.

## Source Tags and Git

- `!git.repository`, `!git.branch`, and `!git.sha` need a Git checkout. Without a default
  they fail in a directory that is not a Git repository, or when the repository has no `origin` remote.
- `!git.branch` also fails when `HEAD` is detached, which is typical in CI. The trailing literal
  (`!git.branch detached`, `!git.repository unknown`) is the fallback.

## Learn More

- [Tagging strategy tutorial](https://atmos.tools/tutorials/tagging-strategy)
- [`!labels`](https://atmos.tools/functions/yaml/labels)
- [`!git.repository`](https://atmos.tools/functions/yaml/git.repository)
- The [`tagging-default-tags`](https://github.com/cloudposse/atmos/tree/main/examples/tagging-default-tags)
  example implements the same strategy with the AWS provider's `default_tags`.
