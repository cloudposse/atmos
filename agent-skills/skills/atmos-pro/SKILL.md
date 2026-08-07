---
name: atmos-pro
description: "Atmos Pro setup and workflows: pro, GitHub OIDC, affected and inventory uploads, stack locks, pro commit, workflow dispatch, merge queues, drift detection, and the CI PR-comment Pro badge"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.1.0"
---

# Atmos Pro

Use this skill when configuring or debugging Atmos Pro, including CI uploads, workflow
dispatch, stack locking, GitHub App commits, merge queues, and drift detection.

Atmos Pro is the control plane for visibility and coordination. Atmos CLI remains the execution
layer for plans, applies, deploys, auth, toolchains, and stack resolution.

`atmos pro` is a top-level CLI command group, so its config lives at the top-level `pro:` key in
`atmos.yaml` and, per-component/stack, at the top-level `pro:` component section (a sibling of
`vars:`/`metadata:`/`settings:`). `settings.pro` is a **deprecated alias** for both — Atmos still
reads it, but an explicit top-level `pro:` block always wins over `settings.pro:` when both are
set. Recommend the top-level form in new configs; don't rewrite working `settings.pro` configs
just to modernize them.

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

Atmos Pro reads the component/stack `pro:` section (or the deprecated `settings.pro:` alias) from
uploaded payloads and dispatches GitHub workflows server-side for matching repository events.

```yaml
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
pro:
  enabled: true
  drift_detection:
    enabled: true
```

Effective drift detection also requires the instance to be Pro-enabled. `metadata.enabled: false`
or `pro.enabled: false` disables drift dispatch regardless of `pro.drift_detection.enabled`.

Upload plan status from the workflow that Atmos Pro dispatches:

```bash
atmos terraform plan vpc -s prod-use1 --upload-status
```

Atmos Pro interprets plan/apply status server-side and coordinates drift visibility and remediation.

## CI PR-Comment Pro Badge

Native CI's `plan`/`apply`/`test` PR comments include a Pro status badge in the same row as the
result badges, reflecting the same effective enabled state as drift dispatch and
`atmos list instances --upload`:

- **Green (`PRO-ENABLED`)** — the component is effectively Pro-enabled. Links to the
  [Atmos Pro dashboard](https://atmos-pro.com/dashboard).
- **Silver (`PRO-DISABLED`)** — the component is not Pro-enabled (no `pro:`/`settings.pro:` block,
  `pro.enabled: false`, or `metadata.enabled: false`). Links to
  [atmos-pro.com](https://atmos-pro.com).

No separate configuration is needed — the badge follows whatever `pro:`/`settings.pro:` the
component already has.

## GitHub STS

For private GitHub modules, vendoring, component `source`, or remote `import`, configure the
`atmos/pro` auth provider and `github/sts` integration in the `atmos-auth` skill. This lets Atmos
Pro mint short-lived GitHub App installation tokens in CI without storing long-lived tokens.

## Troubleshooting

- If uploads fail authentication, verify `id-token: write`, `pro.workspace_id` or
  `ATMOS_PRO_WORKSPACE_ID`, and that the Atmos Pro GitHub App is installed.
- If uploads return 403, verify the repository is imported into the Atmos Pro workspace.
- If merge queue checks do not resolve, verify `merge_group.checks_requested` and that
  `atmos describe affected --upload` runs on `merge_group` events.
- If drift is not dispatched, verify `pro.enabled`, `pro.drift_detection.enabled`,
  and that the instance appears in `atmos list instances --upload`.
- If `atmos.yaml` sets both `pro:` and `settings.pro:`, each field falls back independently: a
  `pro.<field>` set at the top level wins; a field left unset there still falls back to
  `settings.pro.<field>`. A stray `settings.pro:` field left behind after a partial migration can
  still take effect for any field the top-level `pro:` block leaves unset; check both.
- If a stack/component config sets both `pro:` and `settings.pro:`, the top-level `pro:` block wins
  outright as a whole block — it is not merged field-by-field with `settings.pro:`. A stray
  `settings.pro:` left behind after a partial migration is ignored entirely once a local `pro:`
  block exists on the same component; check both.
