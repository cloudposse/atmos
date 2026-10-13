---
title: Scan production before plan
description: Require a Checkov scan before planning a production Terraform component.
command: atmos terraform plan <component> -s <stack>
tags:
  - Lifecycle Hooks
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Policy hooks
    url: /policies/hooks
---

# Scan production before plan

Require a Checkov scan before planning a production Terraform component.

## Contract

| Field | Value |
|---|---|
| Input | `operation` |
| Engine | `cel` |
| Action | `hooks` |
| Selects when | `before.terraform.plan` with a stack name starting with `prod-`. |
| Required data | Component lifecycle metadata. Tool and HTTP hooks use their normal execution credentials. |

## Configuration

No rule parameters. Hook arguments: `checkov --directory .`.

Install Checkov or add approved pins to `spec.tools`. Adjust the `prod-` prefix to your stack naming convention. The scan operates on the component directory.

The selected hook must execute and succeed: both `required` and `require_success` are `true`. Dry-run previews it without execution.

## Policy

[View and copy policy.yaml](https://atmos.tools/policies/library/lifecycle-scan-production/policy.yaml).

## Use

From an Atmos source checkout, copy only the selected manifest into your governing directory:

```shell
mkdir -p policy.d
cp policy-library/lifecycle-scan-production/policy.yaml policy.d/lifecycle-scan-production.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Review the settings above before executing a component command. In CI, select policies from a
protected, pinned revision; do not trust definitions from the candidate PR. See
[enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View test inputs and expected results](https://atmos.tools/policies/library/lifecycle-scan-production/tests/cases.json).

| Case | Expected decision |
|---|---|
| selected | `allow` |
| not selected | `not_applicable` |
| unrelated event | `not_applicable` |
| different input kind | `not_applicable` |

Run from the repository root:

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/lifecycle-scan-production$' -v
```

`not_applicable` means this policy did not match, not that all policies passed.
An `allow` with hook requirements selects work; it does not prove the hook executed. Hook execution and failure handling are covered separately by the lifecycle integration tests.
