---
title: Require production deployment review
description: Require a designated reviewer before applying or deploying a production instance.
command: atmos helm deploy app -s prod
tags:
  - GitHub Actions
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Policy reference
    url: /policies/approvals
---

# Require production deployment review

Require a designated reviewer before applying or deploying a production instance.

## Contract

| Field | Value |
|---|---|
| Input | `component.command` |
| Engines | `cel`, `starlark` |
| Action | `require_approval` |

## Configuration

The policy covers instances labeled `stage: prod`, so it protects only what carries that label. A production stack without `metadata.labels.stage: prod` is not selected, and `atmos helm deploy app -s prod` then runs without approval. Require the label on every production instance, and review changes to it as you review the policy itself. To select production by an authoritative identifier instead, replace the label selector with exact stack names:

```yaml
match:
  stacks: [prod]
```

Replace `platform-maintainer` with your maintained GitHub user logins and set `minimum`. Requires a current PR checkout and `GITHUB_TOKEN` with `pull-requests: read`. Fixture evaluation returns an outstanding obligation; the Native CI adapter verifies live reviews separately. It does not verify organization teams.

## Use

Copy this manifest from a reviewed source checkout into a protected governing directory:

```shell
mkdir -p policy.d
cp policy-library/github-production-approval/policy.yaml policy.d/github-production-approval.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

Protect the selected policy revision and enforcement workflow independently of the candidate PR.
See [enforcement configuration](https://atmos.tools/policies/configuration#enforcement).

## Policy

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/github-production-approval).
The default manifest below uses CEL; the [Starlark variant](https://atmos.tools/policies/library/github-production-approval/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

[View policy.yaml](https://atmos.tools/policies/library/github-production-approval/policy.yaml).

## Tests

[View inputs and expected decisions](https://atmos.tools/policies/library/github-production-approval/tests/cases.json).

```shell
go test ./pkg/policy -run '^TestPolicyCatalog/github-production-approval$' -v
```

`not_applicable` means this rule did not match, not that every policy passed.
