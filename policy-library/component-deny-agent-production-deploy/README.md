---
title: Block agent deployments to production
description: Deny production apply and deploy commands launched by Claude Code or Codex.
command: atmos terraform apply app -s acme-ue2-prod
tags:
  - Admission
related_docs:
  - label: Policy configuration
    url: /policies/configuration
  - label: Execution admission
    url: /policies/admission
---

# Block agent deployments to production

Deny production apply and deploy commands launched by Claude Code or Codex.

## Contract

| Field | Value |
|---|---|
| Input | `component.command` |
| Engines | `cel`, `starlark` |
| Action | `deny` |
| Selects when | A recognized agent, an `apply` or `deploy` command, and a production stack name. |
| Required data | Captured admission metadata; no cloud credentials required for evaluation. |

## Configuration

`env` is collected from the invoking process before configuration loading:

| Environment marker | Value that detects the agent |
|---|---|
| `CLAUDECODE` | Exactly `1` |
| `CODEX_THREAD_ID` | Any nonempty value |

The shared context exposes environment values to every engine; do not include them in diagnostics.
[Claude Code documents `CLAUDECODE`](https://code.claude.com/docs/en/env-vars).
[Codex sets `CODEX_THREAD_ID` in its shell environment](https://github.com/openai/codex/blob/main/codex-rs/protocol/src/shell_environment.rs).
The presence of a CLI executable, an API key, or a configured home directory does not identify an agent invocation.

The rule treats `prod` or `production` as a case-insensitive name segment separated by `-`, `_`, or `/`.
Examples: `prod`, `acme-ue2-prod`, `production/us`. `product-dev` does not match.
Adapt the stack naming condition to your organization, or use literal inherited component labels in a `match` selector.

The rule covers `apply` and `deploy` on component types that use shared stack admission.
Production plans and diffs, non-production applies, and invocations without either marker do not match.
It deliberately also denies `apply --dry-run` from a recognized agent on a production stack.

These environment markers are an accidental-action guardrail, not authenticated caller identity:
a caller can remove them before launching Atmos, and a tool that does not forward them will not be detected.
Protect governing policy files and enforce cloud authorization independently.
Direct Terraform/cloud CLI execution is outside Atmos admission.

## Policy

Choose CEL or Starlark on the [policy page](https://atmos.tools/policies/library/component-deny-agent-production-deploy).
The default manifest uses CEL; the [Starlark variant](https://atmos.tools/policies/library/component-deny-agent-production-deploy/variants/starlark/policy.yaml)
uses the same configuration and test cases. Install only one implementation.

[View and copy policy.yaml](https://atmos.tools/policies/library/component-deny-agent-production-deploy/policy.yaml)

## Use

From a reviewed, pinned checkout of this repository:

```shell
mkdir -p policy.d
cp policy-library/component-deny-agent-production-deploy/policy.yaml policy.d/component-deny-agent-production-deploy.yaml
export ATMOS_POLICY_DIR="$PWD/policy.d"
```

A denial exits with code `77` before the protected operation. See
[enforcement and trust requirements](https://atmos.tools/policies/configuration#enforcement).

## Tests

[View test cases](https://atmos.tools/policies/library/component-deny-agent-production-deploy/tests/cases.json).
Run the actual distributed manifests in both engines:

```shell
go test ./pkg/policy -run TestPolicyCatalog/component-deny-agent-production-deploy -v
```
