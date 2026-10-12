---
title: Clean up after Kubernetes failure
description: Require a cleanup command after a failed Kubernetes apply.
tags:
  - Lifecycle Hooks
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Policy hooks
    url: /policies/hooks
---

# Clean up after Kubernetes failure

Require a cleanup command after a failed Kubernetes apply.

## Contract

| Field | Value |
|---|---|
| Input | `operation` |
| Engine | `cel` |
| Action | `hooks` |
| Selects when | `after.kubernetes.apply` with `input.status == 'failure'`. |
| Required data | Component lifecycle metadata. Tool and HTTP hooks use their normal execution credentials. |

## Configuration

Set `spec.hooks.cleanup.command` to your cleanup executable.

Replace `/opt/platform/bin/cleanup-component` before use. Cleanup does not turn the failed apply into success. Protect the executable and any credentials it uses.

The selected hook must execute and succeed: both `required` and `require_success` are `true`. Dry-run previews it without execution.

## Policy

[View and copy policy.yaml](https://atmos.tools/policies/library/lifecycle-cleanup-failed-apply/policy.yaml).

## Use

From an Atmos source checkout, copy only the selected manifest into your governing directory:

```shell
mkdir -p policy.d
cp policy-library/lifecycle-cleanup-failed-apply/policy.yaml policy.d/lifecycle-cleanup-failed-apply.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Review the settings above before executing a component command. In CI, select policies from a
protected, pinned revision; do not trust definitions from the candidate PR. See
[enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View test inputs and expected results](https://atmos.tools/policies/library/lifecycle-cleanup-failed-apply/tests/cases.json).

| Case | Expected decision |
|---|---|
| selected | `allow` |
| not selected | `not_applicable` |
| unrelated event | `not_applicable` |
| different input kind | `not_applicable` |

Run from the repository root:

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/lifecycle-cleanup-failed-apply$' -v
```

`not_applicable` means this policy did not match, not that all policies passed.
An `allow` with hook requirements selects work; it does not prove the hook executed. Hook execution and failure handling are covered separately by the lifecycle integration tests.
