---
title: Limit resource changes
description: Cap the number of changed managed resources in one Terraform plan.
command: atmos terraform plan <component> -s <stack>
tags:
  - Terraform
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Terraform plans
    url: /policies/terraform-plan
---

# Limit resource changes

Cap the number of changed managed resources in one Terraform plan.

## Contract

| Field | Value |
|---|---|
| Input | `terraform.plan` |
| Engines | `cel`, `starlark` |
| Action | `deny` |
| Selects when | The number of managed resources whose actions are not `['no-op']` exceeds `params.maximum`. |
| Required data | Native saved-plan JSON from Terraform/OpenTofu. No CI token required. |

## Configuration

`maximum`: `10`.

The threshold is inclusive. Count is per resource change, not per action; a replacement counts once.

## Policy

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/terraform-limit-change-count).
The default manifest below uses CEL; the [Starlark variant](https://atmos.tools/policies/library/terraform-limit-change-count/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

[View and copy policy.yaml](https://atmos.tools/policies/library/terraform-limit-change-count/policy.yaml).

## Use

From an Atmos source checkout, copy only the selected manifest into your governing directory:

```shell
mkdir -p policy.d
cp policy-library/terraform-limit-change-count/policy.yaml policy.d/terraform-limit-change-count.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Review the settings above before executing a component command. In CI, select policies from a
protected, pinned revision; do not trust definitions from the candidate PR. See
[enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View test inputs and expected results](https://atmos.tools/policies/library/terraform-limit-change-count/tests/cases.json).

| Case | Expected decision |
|---|---|
| at threshold | `not_applicable` |
| over threshold | `deny` |
| empty | `not_applicable` |
| no-op | `not_applicable` |
| different input kind | `not_applicable` |

Run from the repository root:

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/terraform-limit-change-count$' -v
```

`not_applicable` means this policy did not match, not that all policies passed.
