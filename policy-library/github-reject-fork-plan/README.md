---
title: Reject fork PR plans
description: Reject saved Terraform plans when the supplied GitHub event identifies a fork pull request.
command: atmos terraform plan <component> -s <stack>
tags:
  - GitHub Actions
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Terraform plans
    url: /policies/terraform-plan
  - label: CI event reference
    url: /policies/ci#github-event-reference
---

# Reject fork PR plans

Reject saved Terraform plans when the supplied GitHub event identifies a fork pull request.

## Contract

| Field | Value |
|---|---|
| Input | `terraform.plan` |
| Engines | `cel`, `starlark` |
| Action | `deny` |
| Selects when | The event contains a pull request whose head and base repository names differ. |
| Required data | Complete `github-actions/v1` event via `required_ci`. Collection requires no API permission; checkout normally requires `contents: read`. |

## Configuration

None.

Runs after planning and before saved-plan apply; it is not a sandbox. Never execute untrusted PR code with privileged credentials in reliance on this rule. Non-PR events do not match.

## Inputs

This example inspects GitHub's native pull-request object, normally supplied by a `pull_request`
event. It tests for that object rather than matching a single event name; other events containing
the same object can also match. The non-PR fixture demonstrates an event without it.

| Field inside `ci.event.data` | Meaning |
|---|---|
| `pull_request` | Pull request associated with the event |
| `pull_request.head.repo.full_name` | Repository containing the proposed changes, such as `contributor/infra` |
| `pull_request.base.repo.full_name` | Repository receiving the changes, such as `acme/infra` |

Different head and base repository names identify a fork. These are provider-owned fields:
see [GitHub's pull-request payload reference](https://docs.github.com/en/webhooks/webhook-events-and-payloads#pull_request).
The example requires complete event collection and assumes a non-null head/base repository when
the PR object exists; missing nested fields cause an evaluation error, not permission to proceed.
Inspect the actual payload with [`atmos ci context`](https://atmos.tools/cli/commands/ci/context).

## Policy

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/github-reject-fork-plan).
The default manifest below uses CEL; the [Starlark variant](https://atmos.tools/policies/library/github-reject-fork-plan/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

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
