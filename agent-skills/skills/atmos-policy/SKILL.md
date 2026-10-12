---
name: atmos-policy
description: "Use when selecting, authoring, or testing Atmos library policies, CEL saved-plan guardrails, policy-selected hooks, or provider-native policy inputs. For existing settings.validation, JSON Schema, and OPA validation commands, use atmos-validation."
---

# Atmos Policy

Use this skill for the experimental policy engine and reusable library, not for general validation
or ordinary lifecycle-hook configuration. Use `atmos-hooks` for non-policy hook definitions and
`atmos-validation` for existing JSON Schema/OPA `settings.validation` behavior.

## Establish capabilities before authoring

Check the installed Atmos version/help and the matching source/docs. Do not infer a command exists
from the PRD: `atmos policy check/test/explain` is proposed, not implemented in this branch.

The current opt-in is a protected, explicit `ATMOS_POLICY_DIR` containing strict `atmos/v1`
`kind: Policy` documents. Only inline CEL runs. The language interface anticipates restricted
Starlark; neither Starlark nor Rego is available through this loader. Existing Rego validation
is separate and remains supported. Do not migrate legacy configuration to proposed `policy:`
attachments yet.

Distinguish input kind, component type, and CI provider:

- `terraform.plan`: native JSON extracted privately from a saved binary plan. Evaluated after
  successful plan, including no-change plans, and again before applying the evaluated private copy.
  Implicit apply/deploy and direct destroy are rejected when plan policies are enabled.
- `operation`: lifecycle event, component type/name, stack, command, status, exit code, dry-run.
  Current integration uses existing Terraform, Helmfile, Helm, and Kubernetes hooks. Do not claim
  enforcement for Packer, Ansible, containers, emulators, or custom components yet.
- GitHub Actions context: `ci.provider == 'github-actions'`, `ci.schema == 'github-actions/v1'`.
  Metadata retains allowlisted environment variable names under `ci.metadata.data`; native event
  fields remain under `ci.event.data`. No tokens or arbitrary environment variables are copied.
  Events are triggering evidence, not current reviews. Missing required data blocks.
- `atmos.affected`, `git.change`, function/source, and approval input types have core contracts or
  design examples, not complete CLI collectors/enforcement. Affected is explicit; ordinary
  lifecycle and saved-plan rules do not depend on `--affected`.

## Author and test

1. Start with `policy-library/<name>/`: each entry has a `README.md`, one `policy.yaml`, and
    `tests/cases.json`. Browse `/policies/library` in the matching-version documentation.
    `examples/policies/{terraform,github,lifecycle}` are demonstration bundles, not the library.
    Copy only selected manifests into a protected, flat `ATMOS_POLICY_DIR`; loading is not recursive.
2. Choose a supported input and effect. A true CEL predicate selects its effect; a false predicate
    is **not applicable**, not denial. Use `action: deny` for a violation predicate, not a safety
    predicate. `action: hooks` is supported on operation inputs.
3. Keep predicates pure. Available roots are `input`, `params`, `ci`, and `request`. Supply
    parameters with `with`. No network, shell, filesystem, clock, or automation host is exposed.
    Include `metadata.description` with the document's plain-English purpose. It is optional
    literal text, separate from the CEL `rule` and denial `reason`; catalog entries require it.
4. Treat missing, null, and unknown separately. Read Terraform `after_unknown` alongside `after`.
    Deletion includes replacement action lists. Decide whether a removal is forbidden or allowed
    remediation. Do not print plan JSON or sensitive values to explain a violation.
5. Add colocated `tests/cases.json` fixtures for each catalog entry: matching and non-matching
    inputs, threshold boundaries, null/missing/unknown data, relevant input kinds, and exact hook
    requirements. `TestPolicyCatalog` loads the actual manifests and checks combined identities.
    Demonstration bundles retain table-driven cases in `pkg/policy/library_test.go`.
6. Run targeted tests, not production applies or callbacks:

```shell
go test ./pkg/policy -run TestPolicyCatalog -v
go test ./pkg/policy -run TestSampleLibrary -v
go test ./pkg/hooks -run 'TestPolicyHooksLifecycle|TestPolicyHookFailuresBlock'
go test ./pkg/ci/... -run TestPolicySnapshot
```

Tests prove only the layers they execute. A supplied-data predicate test does not prove CI
collection, approvals, an artifact binding, or component command admission.

## Policy-selected hooks

Place hook definitions in governing `spec.hooks`, not workspace `hooks:`. Reference them from
an operation rule's `hooks` list with `ref`, `event`, `required`, and `require_success`. Local
references resolve to `metadata.name/ref`. The predicate must select the same lifecycle event
as the hook requirement. Selection happens now, not as scheduling for a later event.

Use existing hook kinds and steps. Keep conditions in the policy: this slice rejects hook-level
`events`/`when` and nested skip/background controls, and does not render hook templates or YAML
functions. Use protected executable dependencies; optional `spec.tools` pins cannot be overridden
by component dependencies. Tool configs and runtime credentials still need protection.

Set both booleans for a mandatory successful check. `required` prevents `--skip-hooks` bypass;
`require_success` forces failure propagation regardless of a hook's weaker `on_failure` setting.
Dry-run reports selections without executing. A denial is never canceled by a successful hook.
After-apply failures do not roll back completed infrastructure. Avoid callbacks without an
idempotency strategy; there is no durable replay ledger or exactly-once guarantee.

## CI and trust boundaries

Reading supplied Actions events requires no API permission. Repository checkout needs
`contents: read`. Native CI status reporting separately needs `statuses: write`. Do not recommend
`write-all`, a PAT, or a GitHub App just to enable OSS policy input. PR reads, current reviews,
check/run facts, review requests, and approval verification are not implemented by this collector.
Live organization-team membership is not a `GITHUB_TOKEN` workflow permission.

Protect the workflow, governing revision/directory, environment, CODEOWNERS, and executables.
The comparison base is not necessarily the governing revision. Never claim an editable checkout
is tamper-proof. Do not suggest privileged execution of fork code under `pull_request_target`.

Keep local/current-job evaluation in OSS. Cross-component/job outcome aggregation, durable
coordination, organization identity enrichment, and webhook orchestration belong to Atmos Pro.
Keep unimplemented designs in `docs/prd/policies.md`, not website docs or runnable catalog entries.
The library governs Atmos operations and consumed artifacts; do not duplicate Kubernetes admission
rules. Required external validators retain ownership of their resource-security rules.
