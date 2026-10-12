---
title: Protect production Helm releases
description: Deny direct deletion of Helm releases in production stacks.
tags:
  - Lifecycle Hooks
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Policy hooks
    url: /policies/hooks
---

# Protect production Helm releases

Deny direct deletion of Helm releases in production stacks.

## Contract

| Field | Value |
|---|---|
| Input | `operation` |
| Engine | `cel` |
| Action | `deny` |
| Selects when | `before.helm.delete` with a stack name starting with `prod-`. |
| Required data | Component lifecycle metadata. Tool and HTTP hooks use their normal execution credentials. |

## Configuration

None. Adjust the `prod-` prefix in the rule to your stack naming convention.

Other Helm operations and non-production stacks do not match. This is an operation rule, not a rendered Kubernetes resource check.

## Policy

[View and copy policy.yaml](https://atmos.tools/policies/library/lifecycle-deny-production-delete/policy.yaml).

## Use

From an Atmos source checkout, copy only the selected manifest into your governing directory:

```shell
mkdir -p policy.d
cp policy-library/lifecycle-deny-production-delete/policy.yaml policy.d/lifecycle-deny-production-delete.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Review the settings above before executing a component command. In CI, select policies from a
protected, pinned revision; do not trust definitions from the candidate PR. See
[enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View test inputs and expected results](https://atmos.tools/policies/library/lifecycle-deny-production-delete/tests/cases.json).

| Case | Expected decision |
|---|---|
| production | `deny` |
| development | `not_applicable` |
| different operation | `not_applicable` |
| different input kind | `not_applicable` |

Run from the repository root:

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/lifecycle-deny-production-delete$' -v
```

`not_applicable` means this policy did not match, not that all policies passed.
