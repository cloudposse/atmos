---
title: Weighted plan change budget
description: Use Starlark to limit changes, charging more for deletions and replacements.
command: atmos terraform plan <component> -s <stack>
tags:
  - Terraform
related_docs:
  - label: Starlark policies
    url: /policies/starlark
  - label: Terraform plans
    url: /policies/terraform-plan
---

# Weighted plan change budget

Count managed resource changes against a configurable budget.

## Contract

| Field | Value |
|---|---|
| Input | `terraform.plan` |
| Engine | `starlark` |
| Action | `deny` |
| Selects when | Weighted changes exceed `ctx.params["maximum"]`. |
| Required data | Native saved-plan JSON. No CI token required. |

## Configuration

`maximum`: `5`. `deletion_weight`: `3`.

Each managed resource with a delete action costs three points, including replacement in either
order. Other changed managed resources cost one point. No-op resources and data reads cost zero.
The threshold is inclusive. The budget applies to one plan, not multiple component runs.

## Policy

[View and copy policy.yaml](https://atmos.tools/policies/library/terraform-starlark-change-budget/policy.yaml).

## Use

From a reviewed source checkout:

```shell
mkdir -p policy.d
cp policy-library/terraform-starlark-change-budget/policy.yaml policy.d/change-budget.yaml
atmos terraform plan app -s prod
```

In CI, load governing policies from a protected revision. See
[enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View input snapshots and expected results](https://atmos.tools/policies/library/terraform-starlark-change-budget/tests/cases.json).

Fixtures cover an exceeded budget, the exact threshold, replacement counting, ignored resources,
an empty plan, and an unrelated input kind.

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/terraform-starlark-change-budget$' -v
```
