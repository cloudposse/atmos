# `settings.pro` Dispatch Contract

The `settings.pro` block in `stacks/mixins/atmos-pro.yaml` defines what Atmos Pro dispatches
for each GitHub event. This document captures the full schema verbatim so the skill does not
hallucinate keys.

## Full schema

```yaml
settings:
  spacelift:
    workspace_enabled: false

  pro:
    enabled: true

    drift_detection:
      enabled: true
      detect:
        workflows:
          atmos-terraform-plan.yaml:
            inputs:
              component: "{{ .atmos_component }}"
              stack: "{{ .atmos_stack }}"
              upload: "true"
      remediate:
        workflows:
          atmos-terraform-apply.yaml:
            inputs:
              component: "{{ .atmos_component }}"
              stack: "{{ .atmos_stack }}"

    pull_request:
      opened: &plan-workflow
        workflows:
          atmos-terraform-plan.yaml:
            inputs:
              component: "{{ .atmos_component }}"
              stack: "{{ .atmos_stack }}"
      synchronize: *plan-workflow
      reopened: *plan-workflow
      merged:
        workflows:
          atmos-terraform-apply.yaml:
            inputs:
              component: "{{ .atmos_component }}"
              stack: "{{ .atmos_stack }}"
              github_environment: "{{ .vars.tenant }}-{{ .vars.stage }}"
```

## Key semantics

### `pro.enabled`

Master switch for the stack. If `false`, Atmos Pro ignores the stack/component. Default `true`
in the mixin.

### `pro.drift_detection`

- `enabled: true` — subscribes the stack/component to scheduled drift scans.
- `detect.workflows` — what to run when drift is detected. Always a plan workflow with
  `upload: "true"` so results show up in Atmos Pro.
- `remediate.workflows` — what to run when the user clicks "remediate drift" in Atmos Pro.
  Always an apply workflow.

### `pro.pull_request`

Maps GitHub PR event actions to workflows. Supported keys:

- `opened` — first PR creation
- `synchronize` — new commits pushed to PR branch
- `reopened` — PR reopened after being closed
- `merged` — PR merged into the target branch (not the same as `closed`; fires only when merged)

`closed` without `merged` is intentionally not listed — nothing should run when a PR is
closed without being merged.

### `inputs` template variables

The skill uses Go-template syntax inside the input strings. Atmos Pro renders these server-side
at dispatch time:

| Variable                   | What it resolves to                                    |
|----------------------------|--------------------------------------------------------|
| `{{ .atmos_component }}`   | The component name (e.g., `vpc`, `aws/iam-role/gha-tf-plan`) |
| `{{ .atmos_stack }}`       | The full stack name (e.g., `dev-core-gbl-iam`)         |
| `{{ .vars.tenant }}`       | The `vars.tenant` of the affected stack                |
| `{{ .vars.stage }}`        | The `vars.stage` of the affected stack                 |
| `{{ .sha }}`               | The commit SHA that triggered the dispatch             |

Only use variables the workflow actually declares as inputs. Passing an unrecognized input is
silently ignored by GitHub but causes confusion later when debugging.

### `github_environment` input

The apply dispatch passes `github_environment: "{{ .vars.tenant }}-{{ .vars.stage }}"`. The apply
workflow must declare `github_environment` as a required input and use it as the `environment:`
of the apply job. This routes approvals through GitHub Environments and gives per-account/stage
protection rules.

The skill generates `github_environment` with the `{tenant}-{stage}` pattern by default. If the
user chose Environment-based scoping in Step 4 of the playbook, the same pattern serves both
the OIDC subject claim and the approval gate.

## Why the mixin owns this, not `_defaults.yaml`

The mixin is a single edit point. Adding `mixins/atmos-pro` to an org's `_defaults.yaml`
imports list flips on the entire contract. Removing that import flips it off. This is
preferable to scattering `settings.pro` across multiple files where drift is hard to reason
about.

For single-org repos, a `_defaults.yaml` edit is simpler and the skill may choose it (see
PRD open question #1). The default generator emits the mixin.
