---
title: Disable YAML shell execution
description: Reject !exec before configuration can start a shell.
command: atmos describe stacks
tags:
  - Admission
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Policy reference
    url: /policies/admission
---

# Disable YAML shell execution

Reject !exec before configuration can start a shell.

## Contract

| Field | Value |
|---|---|
| Input | `function` |
| Engine | `cel` |
| Action | `deny` |

## Configuration

No parameters. The rule checks the canonical YAML function name, not shell arguments. It covers configuration loading and lazy YAML resolution, not shell hooks or Go-template functions.

## Use

Copy this manifest from a reviewed source checkout into a protected governing directory:

```shell
mkdir -p policy.d
cp policy-library/function-disable-exec/policy.yaml policy.d/function-disable-exec.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Protect the selected policy revision and enforcement workflow independently of the candidate PR.
See [enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Policy

[View policy.yaml](https://atmos.tools/policies/library/function-disable-exec/policy.yaml).

## Tests

[View inputs and expected decisions](https://atmos.tools/policies/library/function-disable-exec/tests/cases.json).

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/function-disable-exec$' -v
```

`not_applicable` means this rule did not match, not that every policy passed.
