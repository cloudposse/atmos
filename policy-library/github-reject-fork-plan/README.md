---
title: Reject fork PR plans
description: Reject saved Terraform plans when the supplied GitHub event identifies a fork pull request.
tags:
  - GitHub Actions
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Terraform plans
    url: /policies/terraform-plan
---

# Reject fork PR plans

Reject saved Terraform plans when the supplied GitHub event identifies a fork pull request.

## Contract

| Field | Value |
|---|---|
| Input | `terraform.plan` |
| Engine | `cel` |
| Action | `deny` |
| Selects when | The event contains a pull request whose head and base repository names differ. |
| Required data | Complete `github-actions/v1` event via `required_ci`. Collection requires no API permission; checkout normally requires `contents: read`. |

## Configuration

None.

Runs after planning and before saved-plan apply; it is not a sandbox. Never execute untrusted PR code with privileged credentials in reliance on this rule. Non-PR events do not match.

## Policy

[View and copy policy.yaml](https://atmos.tools/policies/library/github-reject-fork-plan/policy.yaml).

## Use

From an Atmos source checkout, copy only the selected manifest into your governing directory:

```shell
mkdir -p policy.d
cp policy-library/github-reject-fork-plan/policy.yaml policy.d/github-reject-fork-plan.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Review the settings above before executing a component command. In CI, select policies from a
protected, pinned revision; do not trust definitions from the candidate PR. See
[enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View test inputs and expected results](https://atmos.tools/policies/library/github-reject-fork-plan/tests/cases.json).

| Case | Expected decision |
|---|---|
| same repository | `not_applicable` |
| fork | `deny` |
| missing context | `error` |
| incomplete event | `error` |
| non PR event | `not_applicable` |
| different input kind | `not_applicable` |

Run from the repository root:

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/github-reject-fork-plan$' -v
```

`not_applicable` means this policy did not match, not that all policies passed.
