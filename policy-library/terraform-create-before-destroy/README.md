---
title: Require create-before-destroy
description: Reject Terraform replacements that destroy the old resource first.
tags:
  - Terraform
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Terraform plans
    url: /policies/terraform-plan
---

# Require create-before-destroy

Reject Terraform replacements that destroy the old resource first.

## Contract

| Field | Value |
|---|---|
| Input | `terraform.plan` |
| Engine | `cel` |
| Action | `deny` |
| Selects when | A managed resource has the ordered actions `['delete', 'create']`. |
| Required data | Native saved-plan JSON from Terraform/OpenTofu. No CI token required. |

## Configuration

None.

Pure deletions are outside this rule. Combine with stateful-resource protection when deletion must also be denied.

## Policy

[View and copy policy.yaml](https://atmos.tools/policies/library/terraform-create-before-destroy/policy.yaml).

## Use

From an Atmos source checkout, copy only the selected manifest into your governing directory:

```shell
mkdir -p policy.d
cp policy-library/terraform-create-before-destroy/policy.yaml policy.d/terraform-create-before-destroy.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Review the settings above before executing a component command. In CI, select policies from a
protected, pinned revision; do not trust definitions from the candidate PR. See
[enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View test inputs and expected results](https://atmos.tools/policies/library/terraform-create-before-destroy/tests/cases.json).

| Case | Expected decision |
|---|---|
| create first | `not_applicable` |
| destroy first | `deny` |
| pure deletion | `not_applicable` |
| different input kind | `not_applicable` |

Run from the repository root:

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/terraform-create-before-destroy$' -v
```

`not_applicable` means this policy did not match, not that all policies passed.
