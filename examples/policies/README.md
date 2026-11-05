---
title: Local Policy Walkthrough
description: Inspect and test a policy, then see it block a destructive command without Terraform or cloud credentials.
tags: [Automation]
related_docs:
  - label: Command admission policies
    url: /policies/admission
  - label: Policy commands
    url: /cli/commands/policy
  - label: Saved plan policies
    url: /policies/terraform-plan
  - label: Policy reference
    url: /policies
---

# Policy demonstrations

## Try policies locally

This walkthrough needs only Atmos. It uses [command admission](https://atmos.tools/policies/admission)
to reject destructive Terraform commands before configuration, stacks, or tools are loaded.
No Terraform installation, cloud credentials, or infrastructure is needed.

Start in a directory where you want to create the example:

```shell
atmos init examples/policies
cd policies
```

Atmos discovers the included `policy.d/` directory automatically. If you already set
`ATMOS_POLICY_DIR`, it overrides discovery; point it at this example's `policy.d/`
for the walkthrough. See [policy configuration](https://atmos.tools/policies/configuration).

Validate the policy, inspect its rule, and run the offline fixtures:

```shell
atmos policy check
atmos policy list
atmos policy test tests/cases.json
```

`check` displays a terminal-friendly validation summary with rule and hook counts.
Use `atmos policy check --format json` for machine-readable output on stdout.

The fixtures cover an allowed command, normal plans and applies, direct destroy, and the
`-destroy` flag on plan, apply, and deploy. A matching deny rule returns `deny`; an
unmatched rule returns `not_applicable`, meaning this rule does not block the request.
All fixture expectations should pass. Fixture testing never executes Terraform.

Now run an allowed command with policy diagnostics:

```shell
atmos about --explain
```

The command succeeds and reports `local-safety/deny-destroy` as not applicable. Then
try an intentionally denied command:

```shell
atmos terraform destroy demo -s local --explain
```

Expect exit code **77** and the reason:

> Destructive Terraform operations are disabled in this example. Use a reviewed teardown process instead.

There is deliberately no `demo` component or `local` stack. Admission rejects the command
before component resolution or Terraform execution. The same rule rejects
`atmos terraform plan demo -s local -destroy --explain`. The `--explain` flag reports
policy decisions; it does not bypass enforcement or turn a command into a dry run.

Read `policy.d/safety.yaml` to see the CEL predicate. The policy commands remain available
even when a bundle denies operational commands. Nothing is deployed and no cleanup is needed.
These controls rely on protecting the governing directory and runner; they do not prevent
someone who controls the executable or policy files from bypassing them.

## Preview CI annotations locally

From the initialized example directory, run the intentionally denied command with GitHub provider detection enabled:

```shell
ATMOS_POLICY_DIR=./policy.d GITHUB_ACTIONS=true atmos terraform destroy demo -s local
```

`ATMOS_POLICY_DIR` selects this example's policy, even if your environment already points to another policy
directory. `GITHUB_ACTIONS=true` enables GitHub provider detection for this command. Alongside the normal error
output, Atmos emits exactly one annotation on stderr:

```text
::error title=Policy local-safety/deny-destroy denied execution::Destructive Terraform operations are disabled in this example. Use a reviewed teardown process instead.
```

The terminal displays this raw workflow-command text. In GitHub Actions, the runner renders it as an annotation.
Exit code **77** is intentional: the admission policy blocks the command before Terraform runs or any infrastructure
is accessed. Only Atmos is required; no Terraform installation or cloud credentials are needed. Because this denial
occurs before `atmos.yaml` loads, the preview does not require `ci.enabled` configuration.

## Additional policy bundles

The `terraform/`, `github/`, and `lifecycle/` directories contain parameterized CEL examples that demonstrate [saved-plan policies](https://atmos.tools/policies/terraform-plan)
and lifecycle hooks, not the reusable policy library. They are opt-in controls, not a default compliance baseline. Review the resource
types, limits, event selection, executable paths, tool pins, and credentials before adopting them.

| Directory | Policies | Runtime |
|---|---|---|
| `terraform/` | Protect stateful resources; create-before-destroy; change-count limit; disallowed resource types; allowed instance types; required Owner tag | Saved Terraform plan JSON, checked after plan and before explicit saved-plan apply |
| `github/` | Reject fork PRs in a privileged plan job | Terraform plan plus supplied GitHub Actions native event; no API permission required |
| `lifecycle/` | TFLint after successful plan; production scanner; HTTP callback after Helm apply; cleanup after Kubernetes failure; deny production Helm deletion | Existing Terraform, Helmfile, Helm, and Kubernetes lifecycle hook entry points |

Select the directory explicitly in the protected runner environment:

```shell
export ATMOS_POLICY_DIR=/absolute/protected/path/to/policies/terraform
atmos terraform plan app -s dev
atmos terraform apply app -s dev --from-plan
```

To combine families, copy the desired documents into one protected directory under distinct
filenames. The loader does not recurse. It rejects duplicate rule/hook identities, unknown fields,
missing hook references, and conflicting tool pins. Nothing imports remote policy automatically.
The sample HTTP endpoint is intentionally invalid; replace it with an approved endpoint before use.
The cleanup executable is an example of an organization-owned tool, not a bundled command.
Install or pin TFLint and Checkov before enabling the lifecycle samples. `spec.tools` uses the
existing toolchain installer and does not merge component dependency overrides.

## Repository tests

Contributors can run these checks from the root of the Atmos source repository.
In an initialized copy of this example, use `atmos policy test tests/cases.json`
as shown above instead.

```shell
go test ./tests -run '^TestCLICommands/policies_example_' -v
go test ./pkg/policy -run TestSampleLibrary -v
go test ./pkg/hooks -run 'TestPolicyHooksLifecycle|TestPolicyHookFailuresBlock' -v
go test ./pkg/ci/... -run TestPolicySnapshot -v
```

The CLI tests exercise the local walkthrough, including command denials and their exit codes.
The library test loads the three additional bundles and checks every policy, including allow/block or
select/non-select cases. An added policy without cases fails the suite. Cases exercise thresholds,
replacement ordering, deletion remediation, absent/null/unknown values, different components and
lifecycle outcomes, same-repository/fork PRs, and missing CI evidence. Hook integration tests use
a local HTTP server; they do not install scanners, contact production, or run the cleanup sample.

Predicates evaluate to true when their effect should apply. For `action: deny`, true blocks;
false means not applicable. For `action: hooks`, true selects the hook obligations, not approval.
Terraform unknown values must be handled deliberately; absent values are not evidence of safety.

## Current boundaries

- This is **not yet all-component enforcement**. The pure engine is component-independent;
  lifecycle integration currently follows the four existing hook-enabled component types above.
  Packer, Ansible, container, emulator, and custom-component admission remain unwired.
- A Helm/Kubernetes rendered manifest is not Terraform plan JSON. Native artifact policies require
  their own collectors and capability contracts; Ansible check mode is not a guaranteed full plan.
- Lifecycle rules do not require `--affected`. Affected inventories and semantic before/after
  change policies still need a collector and command integration.
- CI context is supplied-event evidence. Current reviews, changed files, checks, and approval
  verification are not fetched. Required unavailable facts fail; events are not fresh API facts.
- Hooks are static governing definitions. Predicate selection replaces hook `when`/`events`;
  nested skip/background controls are rejected. Hook template/YAML-function rendering is not
  enabled for this slice. Ordinary hook definitions remain unchanged.
- `required: true` ignores `--skip-hooks`; `require_success: true` forces failure propagation.
  Dry-run reports intended hooks without executing them. An HTTP failure after apply can fail
  the command, but cannot undo an apply that already completed.
- Protect the governing directory, workflow, environment, PATH, executable dependencies, scanner
  configuration, and CODEOWNERS with required review. Component arguments and resource metadata
  are untrusted data. No CLI can prevent a caller controlling its executable from bypassing it.
- Each lifecycle invocation executes independently: retrying a command can repeat callbacks.
  Use an application-level idempotency key where needed. There is no durable deduplication,
  exactly-once delivery, approval refresh service, or cross-job aggregation in this library.

See the website's **Policy Library** contributor page for proposed additional examples and their
missing capabilities. Proposals are not mixed into executable policy directories.
