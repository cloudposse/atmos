---
title: Block Terraform and Helmfile destroy
description: Deny Terraform and Helmfile destroy, including the Terraform destroy flag on plan, apply, and deploy.
command: atmos terraform destroy vpc -s prod
tags:
  - Admission
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Policy reference
    url: /policies/admission
---

# Block Terraform and Helmfile destroy

Deny Terraform and Helmfile destroy, including the Terraform destroy flag on plan, apply, and deploy.

## Contract

| Field | Value |
|---|---|
| Input | `command` (`command`, `destroy`) |
| Engines | `cel`, `starlark` |
| Action | `deny` |

## Configuration

No parameters. The rule matches the `terraform destroy` command, and every invocation where the
normalized `destroy` input is `true`. Atmos derives `destroy` from the arguments, so the rule does not
parse flags itself.

`destroy` is `true` when:

- the command is `terraform destroy` (also through the `tf` alias) or `helmfile destroy`, or
- a `terraform` command (OpenTofu included) such as `plan`, `apply`, or `deploy` receives the
  `-destroy` or `--destroy` flag, including when it comes after `--`.

A `true` flag value counts: `-destroy=true`, `--destroy=1`, `-destroy=T`. A `false` value does not:
`-destroy=false`, `-destroy=0`. An unparseable value counts as `true`, and any `true` occurrence wins over a
later `false`. Other commands, and other argument values, never set `destroy`.

For example, `atmos terraform apply vpc -s prod -destroy -auto-approve` is denied, while
`atmos terraform apply vpc -s prod -destroy=false` is allowed.

Limits:

- `atmos terraform apply <planfile>` cannot be recognized as a destroy when the saved plan came from
  `plan -destroy`. The plan was already created; govern it with `terraform.plan` policies, such as
  [`terraform-protect-stateful-resources`](https://atmos.tools/policies/library/terraform-protect-stateful-resources).
- Resource removal through other commands, tools, or direct Terraform use outside Atmos is not covered,
  for example `helm delete` or deleting resources from configuration. Pair this policy with saved-plan
  deletion policies when resource-level protection is required.

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
