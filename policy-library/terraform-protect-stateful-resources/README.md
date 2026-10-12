---
title: Protect stateful resources
description: Reject deletion or replacement of selected stateful resource types.
tags:
  - Terraform
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Terraform plans
    url: /policies/terraform-plan
---

# Protect stateful resources

Reject deletion or replacement of selected stateful resource types.

## Contract

| Field | Value |
|---|---|
| Input | `terraform.plan` |
| Engine | `cel` |
| Action | `deny` |
| Selects when | Any managed resource in `params.types` has a `delete` action, including replacement. |
| Required data | Native saved-plan JSON from Terraform/OpenTofu. No CI token required. |

## Configuration

`types`: `[aws_db_instance, aws_rds_cluster, aws_s3_bucket]`.

Updates and unrelated types pass. Both replacement orders are denied.

## Policy

[View and copy policy.yaml](/policies/library/terraform-protect-stateful-resources/policy.yaml).

## Use

From an Atmos source checkout, copy only the selected manifest into your governing directory:

```shell
mkdir -p policy.d
cp policy-library/terraform-protect-stateful-resources/policy.yaml policy.d/terraform-protect-stateful-resources.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Review the settings above before executing a component command. In CI, select policies from a
protected, pinned revision; do not trust definitions from the candidate PR. See
[enforcement configuration](/policies/configuration#enforcement).

## Tests

[View test inputs and expected results](/policies/library/terraform-protect-stateful-resources/tests/cases.json).

| Case | Expected decision |
|---|---|
| update | `not_applicable` |
| delete | `deny` |
| create first replacement | `deny` |
| destroy first replacement | `deny` |
| unrelated resource | `not_applicable` |
| empty plan | `not_applicable` |
| different input kind | `not_applicable` |

Run from the repository root:

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/terraform-protect-stateful-resources$' -v
```

`not_applicable` means this policy did not match, not that all policies passed.
