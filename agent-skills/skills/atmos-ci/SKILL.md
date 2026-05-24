---
name: atmos-ci
description: "Atmos CI: Native CI with GitHub Actions containers, native outputs, affected/all matrix workflows, OIDC profiles, toolchain-aware jobs, merge queues, environments, statuses, and Atlantis integration"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/native-ci.md
---

# Atmos CI

## Purpose

Use this skill for CI/CD workflows that run Atmos in GitHub Actions, Atlantis, or other
automation systems. Prefer **Atmos Native CI**: run the Atmos CLI directly in CI, with stack config
as the source of truth and `atmos describe affected`/`atmos list instances` producing matrices.

Do not recommend the deprecated `cloudposse/github-action-atmos*` wrapper actions or
`cloudposse/github-action-setup-atmos`. Replace those with containerized Native CI jobs that run
Atmos commands directly.

## Related Skills

| Need | Load |
|---|---|
| Native CI workflow structure, matrices, outputs, summaries, checks, comments | stay in `atmos-ci` |
| Tool versions, `dependencies.tools`, explicit job tool installs, PATH behavior | [atmos-toolchain](../atmos-toolchain/SKILL.md) |
| OIDC providers, identities, trust policies, cloud auth conventions | [atmos-auth](../atmos-auth/SKILL.md) |
| Profile mechanics for `ATMOS_PROFILE` and `--profile` | [atmos-profiles](../atmos-profiles/SKILL.md) |

## Native CI First

Configure Atmos CI features in `atmos.yaml`; workflow YAML alone is not enough when users want
summaries, outputs, checks, comments, or planfile behavior:

```yaml
ci:
  enabled: true
  output:
    enabled: true
    variables:
      - has_changes
      - has_additions
      - has_destructions
      - artifact_key
      - plan_summary
  summary:
    enabled: true
  checks:
    enabled: true
    context_prefix: atmos
    statuses:
      component: true
      add: true
      change: true
      destroy: true
  comments:
    enabled: true
    behavior: upsert
```

Use the Atmos toolchain for Terraform/OpenTofu and related tools so CI does not depend on runner
images or external setup actions:

```yaml
toolchain:
  aliases:
    terraform: hashicorp/terraform
    opentofu: opentofu/opentofu
    tofu: opentofu/opentofu

terraform:
  dependencies:
    tools:
      terraform: "1.10.3"
      # For OpenTofu projects:
      # opentofu: "1.10.3"
```

Discourage `hashicorp/setup-terraform`, `opentofu/setup-opentofu`, and similar setup actions in Atmos
CI examples. Prefer `dependencies.tools` when the tool is required by a stack, component, workflow,
or custom command; Atmos installs and injects the exact version for that execution context.

Use explicit `atmos toolchain install ...` steps only for job-level scripts that need tools not
declared as component, workflow, or custom command dependencies. In GitHub Actions, run
`atmos toolchain env --format=github`; Atmos appends toolchain paths to `$GITHUB_PATH` when that
file is available, so later steps can call those tools directly.

Primary GitHub Actions pattern:

```yaml
jobs:
  plan:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    permissions:
      contents: read
      id-token: write
      statuses: write
      checks: write
      pull-requests: write
    env:
      ATMOS_PROFILE: github
      GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
    steps:
      - uses: actions/checkout@v6
      - run: atmos terraform plan vpc -s prod
```

For new workflows, use the container image and direct Atmos commands.

## Matrix Patterns

Use affected matrices for pull requests and targeted deploys. When `ci.enabled: true` and
`ci.output.enabled: true` are configured, Atmos writes native outputs to `$GITHUB_OUTPUT`; pass them
between steps and jobs with step `id`, job `outputs`, and `needs.<job>.outputs.*`.

```yaml
jobs:
  affected:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    outputs:
      matrix: ${{ steps.affected.outputs.matrix }}
    steps:
      - uses: actions/checkout@v6
      - id: affected
        run: atmos describe affected --format=matrix

  deploy:
    needs: affected
    if: ${{ needs.affected.outputs.matrix != '' }}
    strategy:
      fail-fast: false
      matrix: ${{ fromJson(needs.affected.outputs.matrix) }}
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    env:
      ATMOS_PROFILE: github
    steps:
      - uses: actions/checkout@v6
      - run: atmos terraform deploy "${{ matrix.component }}" -s "${{ matrix.stack }}"
```

Use all-instance matrices for full estate bootstraps, release deploys, or drift sweeps:

```yaml
- id: instances
  run: atmos list instances --format=matrix
```

For full examples, read [references/native-ci.md](references/native-ci.md).

## Auth and Profiles

Define a CI profile such as `github` and activate it with `ATMOS_PROFILE: github`.
In GitHub Actions OIDC workflows:

- Set `permissions.id-token: write`.
- Configure `auth.providers.<name>.kind: github/oidc`.
- Configure identities such as `aws/assume-role`.
- Let Atmos exchange the OIDC token when the command runs.
- Do not add `atmos auth login` to normal non-interactive OIDC jobs unless a specific integration
  such as Docker/ECR login needs it.

IAM trust policies must constrain GitHub OIDC `sub` claims to the intended repository plus branch
or environment, for example:

```text
repo:ORG/REPO:ref:refs/heads/main
repo:ORG/REPO:environment:prod
```

Use GitHub environments for approval gates and environment-scoped claims. Treat environment names
as GitHub deployment controls; they are independent from Atmos stack names.

## Workflow Guidance

- **Pull request plan**: run `atmos describe affected --format=matrix`, then plan each affected
  component/stack pair.
- **Merge or release deploy**: use `atmos terraform deploy`, not stored wrapper-action planfiles.
- **Affected deploy**: use the affected matrix and optionally `--include-dependents`.
- **All-instance deploy**: use `atmos list instances --format=matrix` when the whole estate is in scope.
- **Merge queue**: run the same plan checks on `merge_group` synthetic commits that are required on PRs.
- **Environment promotion**: use release or manual workflows plus GitHub environments for staging/prod gates.
- **Statuses, checks, comments, and summaries**: configure `ci.summary`, `ci.output`, `ci.checks`,
  and `ci.comments` in `atmos.yaml`; grant only the permissions needed, such as `statuses: write`,
  `checks: write`, or `pull-requests: write`, based on the chosen reporting mode.
- **Step and job outputs**: let Native CI write to `$GITHUB_OUTPUT`, then pass values with step
  `id`, job `outputs`, and `needs.<job>.outputs.*`.
- **Atmos CI creation**: add the `ci` section, configure toolchain aliases and `dependencies.tools`,
  then create containerized workflows that run direct Atmos commands.

## Concurrency Warning

Do not present GitHub Actions `concurrency` groups as a FIFO deployment queue. Default concurrency
allows at most one running and one pending run per group, and a newer pending run replaces an older
pending run. Even queueing modes have ordering caveats. For strict deployment ordering, use GitHub
merge queues, GitHub environments, an explicit promotion workflow, or an external queue/orchestrator.

## Component Dependencies

Use `dependencies.components` for ordering and affected/dependent analysis:

```yaml
components:
  terraform:
    eks/cluster:
      dependencies:
        components:
          - component: vpc
          - component: dns-zone
            stack: plat-ue2-prod
          - kind: file
            path: configs/cluster.yaml
          - kind: folder
            path: src/lambda
```

`settings.depends_on` is legacy. If found, recommend migration to `dependencies.components`.

## Integrations

Atlantis remains a supported integration target, but keep Atmos as the source of truth. For Atlantis,
generate repo configuration with Atmos and keep generated files out of hand-edited skill examples
unless the user is specifically asking about Atlantis.

## Deprecated Patterns

When you see these, recommend replacement with Native CI:

- Deprecated: `cloudposse/github-action-atmos-affected-stacks`
- Deprecated: `cloudposse/github-action-atmos-terraform-plan`
- Deprecated: `cloudposse/github-action-atmos-terraform-apply`
- Deprecated: `cloudposse/github-action-atmos-terraform-drift-detection`
- Deprecated: `cloudposse/github-action-atmos-terraform-drift-remediation`
- Deprecated: `cloudposse/github-action-setup-atmos`
- Deprecated: `integrations.github.gitops`

Do not copy examples that use those patterns into new guidance.
