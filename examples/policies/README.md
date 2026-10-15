---
related_docs:
  - label: Saved plan policies
    url: /policies/terraform-plan
  - label: Policy reference
    url: /policies
---

# Policy demonstrations

These parameterized CEL examples demonstrate [saved-plan policies](https://atmos.tools/policies/terraform-plan)
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

## Tests for every sample

```shell
go test ./pkg/policy -run TestSampleLibrary -v
go test ./pkg/hooks -run 'TestPolicyHooksLifecycle|TestPolicyHookFailuresBlock' -v
go test ./pkg/ci/... -run TestPolicySnapshot -v
```

The library test loads these exact files and checks every policy, including allow/block or
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
