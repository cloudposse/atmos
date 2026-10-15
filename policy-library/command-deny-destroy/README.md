---
title: Block direct Terraform destroy
description: Deny the direct destroy command without requiring affected analysis.
command: atmos terraform destroy vpc -s prod
tags:
  - Admission
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Policy reference
    url: /policies/admission
---

# Block direct Terraform destroy

Deny the direct destroy command without requiring affected analysis.

## Contract

| Field | Value |
|---|---|
| Input | `command` |
| Engines | `cel`, `starlark` |
| Action | `deny` |

## Configuration

No parameters. This blocks the named command, not deletion through other commands or tools. Pair it with saved-plan deletion policies when resource-level protection is required.

## Use

Copy this manifest from a reviewed source checkout into a protected governing directory:

```shell
mkdir -p policy.d
cp policy-library/command-deny-destroy/policy.yaml policy.d/command-deny-destroy.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Protect the selected policy revision and enforcement workflow independently of the candidate PR.
See [enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Policy

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/command-deny-destroy).
The default manifest below uses CEL; the [Starlark variant](https://atmos.tools/policies/library/command-deny-destroy/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

[View policy.yaml](https://atmos.tools/policies/library/command-deny-destroy/policy.yaml).

## Tests

[View inputs and expected decisions](https://atmos.tools/policies/library/command-deny-destroy/tests/cases.json).

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/command-deny-destroy$' -v
```

`not_applicable` means this rule did not match, not that every policy passed.
