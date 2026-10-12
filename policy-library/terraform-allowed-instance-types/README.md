---
title: Allow approved instance types
description: Require planned EC2 instance types to be known and explicitly approved.
tags:
  - Terraform
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Terraform plans
    url: /policies/terraform-plan
---

# Allow approved instance types

Require planned EC2 instance types to be known and explicitly approved.

## Contract

| Field | Value |
|---|---|
| Input | `terraform.plan` |
| Engine | `cel` |
| Action | `deny` |
| Selects when | A managed EC2 instance's planned type is missing, unknown, or not in `params.allowed`. |
| Required data | Native saved-plan JSON from Terraform/OpenTofu. No CI token required. |

## Configuration

`allowed`: `[t3.micro, t3.small]`.

Deleting an instance is permitted. An unknown-value marker blocks even when `after` contains an approved placeholder.

## Policy

[View and copy policy.yaml](/policies/library/terraform-allowed-instance-types/policy.yaml).

## Use

From an Atmos source checkout, copy only the selected manifest into your governing directory:

```shell
mkdir -p policy.d
cp policy-library/terraform-allowed-instance-types/policy.yaml policy.d/terraform-allowed-instance-types.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Review the settings above before executing a component command. In CI, select policies from a
protected, pinned revision; do not trust definitions from the candidate PR. See
[enforcement configuration](/policies/configuration#enforcement).

## Tests

[View test inputs and expected results](/policies/library/terraform-allowed-instance-types/tests/cases.json).

| Case | Expected decision |
|---|---|
| approved | `not_applicable` |
| unapproved | `deny` |
| missing type | `deny` |
| null type | `deny` |
| unknown mask wins | `deny` |
| deleted | `not_applicable` |
| different input kind | `not_applicable` |

Run from the repository root:

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/terraform-allowed-instance-types$' -v
```

`not_applicable` means this policy did not match, not that all policies passed.
