---
title: Disallow resource types
description: Prevent selected resource types from being created or retained by a change.
command: atmos terraform plan <component> -s <stack>
tags:
  - Terraform
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Terraform plans
    url: /policies/terraform-plan
---

# Disallow resource types

Prevent selected resource types from being created or retained by a change.

## Contract

| Field | Value |
|---|---|
| Input | `terraform.plan` |
| Engines | `cel`, `starlark` |
| Action | `deny` |
| Selects when | A managed resource in `params.types` has a non-null planned `after` value. |
| Required data | Native saved-plan JSON from Terraform/OpenTofu. No CI token required. |

## Configuration

`types`: `[aws_iam_user, aws_iam_access_key]`.

Deletion is permitted as remediation. This checks the plan's resource changes, not an independent inventory of existing resources.

## Policy

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/terraform-disallow-resource-types).
The default manifest below uses CEL; the [Starlark variant](https://atmos.tools/policies/library/terraform-disallow-resource-types/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

[View and copy policy.yaml](https://atmos.tools/policies/library/terraform-disallow-resource-types/policy.yaml).

## Use

From an Atmos source checkout, copy only the selected manifest into your governing directory:

```shell
mkdir -p policy.d
cp policy-library/terraform-disallow-resource-types/policy.yaml policy.d/terraform-disallow-resource-types.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Review the settings above before executing a component command. In CI, select policies from a
protected, pinned revision; do not trust definitions from the candidate PR. See
[enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View test inputs and expected results](https://atmos.tools/policies/library/terraform-disallow-resource-types/tests/cases.json).

| Case | Expected decision |
|---|---|
| allowed resource | `not_applicable` |
| forbidden create | `deny` |
| forbidden update | `deny` |
| deletion remediation | `not_applicable` |
| different input kind | `not_applicable` |

Run from the repository root:

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/terraform-disallow-resource-types$' -v
```

`not_applicable` means this policy did not match, not that all policies passed.
