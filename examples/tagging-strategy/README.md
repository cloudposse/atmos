---
title: Tagging Strategy
tags: [Stacks, Components]
description: >-
  Define resource tags once and inherit them everywhere, using either AWS provider
  default tags or Null Label style tag maps. Both styles run without cloud credentials.
related_docs:
  - label: "Provider configuration"
    url: /stacks/providers
  - label: "Stack names"
    url: /stacks/name
  - label: "!git.repository"
    url: /functions/yaml/git.repository
  - label: "!env"
    url: /functions/yaml/env
  - label: "!labels"
    url: /functions/yaml/labels
---

# Example: Tagging Strategy

Define tags once and inherit them everywhere. This example implements the same tag strategy in
two ways, each in its own stack hierarchy, so you can inspect them independently. Neither needs
cloud credentials. The components are mocks built on the `terraform_data` resource, which ships with
Terraform and OpenTofu.

Tags are defined at three import levels and deep-merged:

| Level | File | Tags |
|-------|------|------|
| Organization | `_defaults.yaml` | `managed-by`, `repository`, `source-branch`, `source-commit`, `stack`, `component` |
| Product | `checkout/_defaults.yaml` | `product`, `owner` |
| Environment | `checkout/{dev,prod}/_defaults.yaml` | `environment`, `tier`, `region` |

## Layout

```text
atmos.yaml                                   # Templating enabled
components/terraform/
  default-tags/                              # Mock component for the default_tags style
  null-label/                                # Mock component for the Null Label style
stacks/
  catalog/                                   # Component definitions
  mixins/ci-source-tags.yaml                 # Pipeline-only source tags using !env
  deploy/
    default-tags/                            # Style 1: stacks default-tags-dev, default-tags-prod
    null-label/                              # Style 2: stacks null-label-dev, null-label-prod
```

## Try It

```shell
cd examples/tagging-strategy

# List the stacks
atmos list stacks
```

### Style 1: AWS provider default tags

Tags live under `terraform.providers.aws.default_tags.tags`. Atmos deep-merges them across the
imports, renders `{{ .atmos_stack }}` and `{{ .atmos_component }}`, and evaluates `!git.*`. The AWS
provider then applies the tags to every resource the component creates, so the component needs no
`tags` variable.

```shell
atmos describe component app --stack=default-tags-dev --query=.providers

# The prod stack overrides the source tags with CI environment variables
atmos describe component app --stack=default-tags-prod --query=.providers
GITHUB_REF_NAME=main GITHUB_SHA=abc123 \
  atmos describe component app --stack=default-tags-prod --query=.providers.aws.default_tags.tags
```

When Atmos runs Terraform for this component, it writes the merged provider configuration to
`providers_override.tf.json` in the component directory. The `default-tags` component deliberately
declares no provider requirements, but the generated `provider "aws"` block means Terraform
needs the AWS provider and credentials to plan it. For that reason the CI test only describes this
component.

### Style 2: Null Label tag maps

Tags live in `vars.tags`, next to the label context (`namespace`, `tenant`, `environment`, `stage`).
`label_key_case: lower` and `label_value_case: lower` normalize the tags. The product tags
`Product: Checkout` and `Owner: Payments-Team` are written in mixed case on purpose.

The real `cloudposse/label/null` module also generates tags from the label context itself
(`namespace`, `tenant`, `environment`, `stage`, and the `Name` tag), so those keys do not need to
appear in `vars.tags`.

```shell
atmos describe component app --stack=null-label-dev --query=.vars
atmos terraform plan app --stack=null-label-dev
```

The `null-label` component mirrors the inputs of the `cloudposse/label/null` module so the example
needs no module download. Real components vendor `context.tf`, which instantiates that module.

### Style 2 with labels: `vars.tags: !labels`

The `app-labeled` component bridges `metadata.labels` into `vars.tags`.

```shell
atmos describe component app-labeled --stack=null-label-dev --query=.vars.tags
```

The inherited organization, product, and environment tags merge into the `!labels` result, and their
templates and Git functions are rendered.

## Source Tags and Git

- `!git.repository`, `!git.branch`, and `!git.sha` need a Git checkout. Without a default
  they fail in a directory that is not a Git repository, or when the repository has no `origin` remote.
- `!git.branch` also fails when `HEAD` is detached, which is typical in CI. The trailing literal
  (`!git.branch detached`, `!git.repository unknown`) is the fallback.
- The pipeline-only variant in `stacks/mixins/ci-source-tags.yaml` reads `GITHUB_REF_NAME`
  and `GITHUB_SHA` with `!env`. It works from any directory and uses `local` when the variables are unset.

## Learn More

- [Provider configuration](https://atmos.tools/stacks/providers)
- [`!git.repository`](https://atmos.tools/functions/yaml/git.repository)
- [`!env`](https://atmos.tools/functions/yaml/env)
- [`!labels`](https://atmos.tools/functions/yaml/labels)
