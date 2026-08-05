---
name: atmos-pro
description: "Atmos Pro setup and workflows: settings.pro, GitHub OIDC, affected and inventory uploads, stack locks, pro commit, workflow dispatch, merge queues, and drift detection"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Pro

Use this skill when configuring or debugging Atmos Pro, including CI uploads, workflow
dispatch, stack locking, GitHub App commits, merge queues, and drift detection.

Atmos Pro is the control plane for visibility and coordination. Atmos CLI remains the execution
layer for plans, applies, deploys, auth, toolchains, and stack resolution.

## Related Skills

| Need | Load |
|---|---|
| GitHub Actions job structure and Native CI | [atmos-ci](../atmos-ci/SKILL.md) |
| `atmos/pro` provider and `github/sts` integration | [atmos-auth](../atmos-auth/SKILL.md) |
| Managed Git repositories and signed/pushed commits | [atmos-git](../atmos-git/SKILL.md) |
| Migrating old drift/action patterns | [atmos-modernization](../atmos-modernization/SKILL.md) |

## Minimal Setup

Configure the workspace ID in `atmos.yaml` or `ATMOS_PRO_WORKSPACE_ID`. It is not a secret.

```yaml
settings:
  pro:
    workspace_id: "your-workspace-id"
```

In GitHub Actions, grant OIDC permission. Do not use static API keys for normal GitHub Actions
workflows.

```yaml
permissions:
  id-token: write
  contents: read
```

Atmos Pro exchanges the GitHub Actions OIDC token for a short-lived bearer token. The OIDC request
URL/token are normally supplied by GitHub automatically.

## Core Commands

| Command | Purpose |
|---|---|
| `atmos describe affected --upload` | Upload affected stacks/components for PR and merge-queue correlation |
| `atmos list instances --upload` | Upload full component instance inventory |
| `atmos terraform plan <component> -s <stack> --upload-status` | Upload raw plan status for drift detection |
| `atmos pro lock --component <component> --stack <stack>` | Lock a stack/component before mutation |
| `atmos pro unlock --component <component> --stack <stack>` | Release a stack/component lock |
| `atmos pro commit -m "<message>" --all` | Commit CI-generated changes through the Atmos Pro GitHub App |

`atmos pro commit` is for workflows that need committed changes to trigger follow-on CI. Commits
created with `GITHUB_TOKEN` do not trigger subsequent GitHub Actions workflows; commits created by
the Atmos Pro GitHub App do.

## Workflow Dispatch

Atmos Pro reads `settings.pro` from uploaded stack/component payloads and dispatches GitHub
workflows server-side for matching repository events.

```yaml
settings:
  pro:
    enabled: true
    pull_request:
      synchronize:
        workflows:
          atmos-terraform-plan.yaml:
            inputs:
              component: "{{ .atmos_component }}"
              stack: "{{ .atmos_stack }}"
      merged:
        workflows:
          atmos-terraform-apply.yaml:
            inputs:
              component: "{{ .atmos_component }}"
              stack: "{{ .atmos_stack }}"
    merge_group:
      checks_requested:
        workflows:
          atmos-terraform-plan.yaml:
            inputs:
              component: "{{ .atmos_component }}"
              stack: "{{ .atmos_stack }}"
```

Use `pull_request.synchronize` for plan-on-change. Use `pull_request.merged` for the conventional
apply workflow. Configure `merge_group.checks_requested` explicitly for GitHub merge queues rather
than relying on fallback inference.

## Drift Detection

Atmos Pro is the standard product path for drift detection. Do not recommend hand-rolled scheduled
GitHub Actions or deprecated drift wrapper actions for new projects.

Enable drift detection per stack/component:

```yaml
settings:
  pro:
    enabled: true
    drift_detection:
      enabled: true
```

Effective drift detection also requires the instance to be Pro-enabled. `metadata.enabled: false`
or `settings.pro.enabled: false` disables drift dispatch regardless of
`settings.pro.drift_detection.enabled`.

Upload plan status from the workflow that Atmos Pro dispatches:

```bash
atmos terraform plan vpc -s prod-use1 --upload-status
```

Atmos Pro interprets plan/apply status server-side and coordinates drift visibility and remediation.

## GitHub STS

For private GitHub modules, vendoring, component `source`, or remote `import`, configure the
`atmos/pro` auth provider and `github/sts` integration in the `atmos-auth` skill. This lets Atmos
Pro mint short-lived GitHub App installation tokens in CI without storing long-lived tokens.

## Troubleshooting

- If uploads fail authentication, verify `id-token: write`, `settings.pro.workspace_id` or
  `ATMOS_PRO_WORKSPACE_ID`, and that the Atmos Pro GitHub App is installed.
- If uploads return 403, verify the repository is imported into the Atmos Pro workspace.
- If merge queue checks do not resolve, verify `merge_group.checks_requested` and that
  `atmos describe affected --upload` runs on `merge_group` events.
- If drift is not dispatched, verify `settings.pro.enabled`, `settings.pro.drift_detection.enabled`,
  and that the instance appears in `atmos list instances --upload`.
