---
title: Tagging with AWS Default Tags
tags: [Stacks, Components]
description: >-
  Define resource tags once and inherit them in every stack by generating the AWS provider's
  default_tags from stack configuration. Runs without cloud credentials.
related_docs:
  - label: "Tagging strategy tutorial"
    url: /tutorials/tagging-strategy
  - label: "Provider configuration"
    url: /stacks/providers
  - label: "Stack names"
    url: /stacks/name
  - label: "!git.repository"
    url: /functions/yaml/git.repository
  - label: "!env"
    url: /functions/yaml/env
  - label: "Conditional stack imports"
    url: /stacks/imports#conditional-imports
---

# Example: Tagging with AWS Default Tags

Define tags once in stack configuration and have the AWS provider apply them to every resource.
Atmos deep-merges `terraform.providers.aws.default_tags.tags` across the imports, renders
`{{ .atmos_stack }}` and `{{ .atmos_component }}`, evaluates `!git.*`, and writes the result to
`providers_override.tf.json`. The component needs no `tags` variable.

The `app` component is a mock built on the `terraform_data` resource, which ships with Terraform
and OpenTofu. It declares no provider requirements, so Atmos only describes it here. Planning it
would need the AWS provider and credentials, because the generated `provider "aws"` block requires them.

Tags are defined at three import levels and deep-merged:

| Level | File | Tags |
|-------|------|------|
| Organization | `orgs/acme/_defaults.yaml` | `managed-by`, `repository`, `source-branch`, `source-commit`, `stack`, `component`, `region` |
| Tenant | `orgs/acme/payments/_defaults.yaml` | `product`, `owner` |
| Stage | `orgs/acme/payments/{dev,prod}.yaml` | `environment`, `tier` |

## Layout

```text
atmos.yaml                                   # Templating enabled, stack names from name_template
components/terraform/app/                    # Mock component
stacks/
  catalog/app/defaults.yaml                  # Component definition
  mixins/ci-source-tags.yaml                 # Source tags from the pipeline, imported only in CI
  orgs/acme/_defaults.yaml                   # Organization defaults
  orgs/acme/payments/_defaults.yaml          # Tenant defaults
  orgs/acme/payments/dev.yaml                # Stack payments-dev
  orgs/acme/payments/prod.yaml               # Stack payments-prod
```

## Try It

```shell
atmos init examples/tagging-default-tags
cd tagging-default-tags

atmos list stacks
atmos describe component app --stack=payments-dev --query=.providers

# In CI the source tags come from the pipeline environment instead of the checkout
GITHUB_ACTIONS=true GITHUB_REF_NAME=main GITHUB_SHA=abc123 \
  atmos describe component app --stack=payments-dev --query=.providers.aws.default_tags.tags
```

Run all the credential-free checks with `atmos test`.

## Source Tags and Git

- `!git.repository`, `!git.branch`, and `!git.sha` need a Git checkout. Without a default
  they fail in a directory that is not a Git repository, or when the repository has no `origin` remote.
- `!git.branch` also fails when `HEAD` is detached, which is typical in CI. The trailing literal
  (`!git.branch detached`, `!git.repository unknown`) is the fallback.
- `stacks/mixins/ci-source-tags.yaml` reads `GITHUB_REF_NAME` and `GITHUB_SHA` with `!env`. Each stack imports
  it with `when: ci`, so it applies only in a CI pipeline and the Git functions apply everywhere else.

## Learn More

- [Tagging strategy tutorial](https://atmos.tools/tutorials/tagging-strategy)
- [Provider configuration](https://atmos.tools/stacks/providers)
- [`!git.repository`](https://atmos.tools/functions/yaml/git.repository)
- [`!env`](https://atmos.tools/functions/yaml/env)
- [Conditional stack imports](https://atmos.tools/stacks/imports#conditional-imports)
- The [`tagging-null-label`](https://github.com/cloudposse/atmos/tree/main/examples/tagging-null-label)
  example implements the same strategy with the Null Label naming module.
