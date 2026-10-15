---
title: Require an Owner tag
description: Require a known, nonempty Owner tag on planned EC2 instances.
command: atmos terraform plan <component> -s <stack>
scenarios:
  owned: EC2 instance has an Owner tag
  absent: EC2 instance is missing the Owner tag
  empty: Owner tag is an empty string
  null tags: EC2 instance has a null tag map
  null owner: Owner tag is null
  unknown map: Tags are unknown until apply
  unknown owner: Owner tag is unknown until apply
  missing tags: EC2 instance has no tag map
  deleted: EC2 instance is being deleted
  different input kind: Request is not a Terraform plan
tags:
  - Terraform
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Terraform plans
    url: /policies/terraform-plan
---

# Require an Owner tag

Require a known, nonempty Owner tag on planned EC2 instances.

## Contract

| Field | Value |
|---|---|
| Input | `terraform.plan` |
| Engines | `cel`, `starlark` |
| Action | `deny` |
| Selects when | A managed EC2 instance has a missing, null, empty, or unknown planned `tags.Owner`. |
| Required data | Native saved-plan JSON from Terraform/OpenTofu. No CI token required. |

## Configuration

None. The tag key is `Owner` and is case-sensitive.

Only EC2 instances are selected. Deleting an instance is permitted.

## Policy

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/terraform-require-owner-tag).
The default manifest below uses CEL; the [Starlark variant](https://atmos.tools/policies/library/terraform-require-owner-tag/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

[View and copy policy.yaml](https://atmos.tools/policies/library/terraform-require-owner-tag/policy.yaml).

## Use

From an Atmos source checkout, copy only the selected manifest into your governing directory:

```shell
mkdir -p policy.d
cp policy-library/terraform-require-owner-tag/policy.yaml policy.d/terraform-require-owner-tag.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Review the settings above before executing a component command. In CI, select policies from a
protected, pinned revision; do not trust definitions from the candidate PR. See
[enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View test inputs and expected results](https://atmos.tools/policies/library/terraform-require-owner-tag/tests/cases.json).

| Case | Expected decision |
|---|---|
| owned | `not_applicable` |
| absent | `deny` |
| empty | `deny` |
| null tags | `deny` |
| null owner | `deny` |
| unknown map | `deny` |
| unknown owner | `deny` |
| missing tags | `deny` |
| deleted | `not_applicable` |
| different input kind | `not_applicable` |

Run from the repository root:

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/terraform-require-owner-tag$' -v
```

`not_applicable` means this policy did not match, not that all policies passed.
