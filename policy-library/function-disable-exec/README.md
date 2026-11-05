---
title: Deny !exec
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

# Deny !exec

Reject `!exec` before configuration can start a shell.

## Contract

| Field | Value |
|---|---|
| Input | `function` |
| Engines | `cel`, `starlark` |
| Action | `deny` |

## Configuration

No parameters. The rule checks the canonical YAML function name, not shell arguments. It covers configuration loading and lazy YAML resolution, not shell hooks or Go-template functions.

The optional `reason` explains the block and suggests using precomputed environment values with
`!env`. Customize this Markdown text for your organization or omit it. It appears in the CLI error when
the rule denies execution; it is not a template and does not change the rule's behavior.

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

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/function-disable-exec).
The default manifest below uses CEL; the [Starlark variant](https://atmos.tools/policies/library/function-disable-exec/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

[View policy.yaml](https://atmos.tools/policies/library/function-disable-exec/policy.yaml).

## Tests

[View inputs and expected decisions](https://atmos.tools/policies/library/function-disable-exec/tests/cases.json).

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/function-disable-exec$' -v
```

`not_applicable` means this rule did not match, not that every policy passed.
