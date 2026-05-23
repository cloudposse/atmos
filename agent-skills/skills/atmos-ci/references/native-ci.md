# Native CI Reference

## Containerized GitHub Actions

Use the Atmos container for GitHub Actions jobs:

```yaml
container:
  image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
```

Pin `ATMOS_VERSION` with a repository or organization variable. Avoid `latest` for CI.

## Atmos-Side CI Configuration

Create Native CI by configuring the `ci` section in `atmos.yaml`:

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

| Native CI feature | Atmos config | GitHub permission |
|---|---|---|
| Job summaries | `ci.summary.enabled` | none |
| Output variables | `ci.output.enabled` | none |
| Commit statuses/checks | `ci.checks.enabled` | `statuses: write` or `checks: write` |
| PR comments | `ci.comments.enabled` | `pull-requests: write` |

Set `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}` when checks or comments are enabled.

## Terraform/OpenTofu Toolchain

Use the Atmos toolchain to install Terraform or OpenTofu in CI instead of relying on runner images
or setup actions:

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

Atmos installs and resolves declared tools into the command environment before running operations.
Do not recommend `hashicorp/setup-terraform`, `opentofu/setup-opentofu`, or equivalent setup actions
for Atmos Native CI unless the user is explicitly maintaining a non-Atmos workflow. In Atmos workflows,
`dependencies.tools` is the source of truth for Terraform/OpenTofu versions.

## Minimal Permissions

Start with the minimum permissions for the workflow:

```yaml
permissions:
  contents: read
  id-token: write
```

Add reporting permissions only when needed:

```yaml
permissions:
  statuses: write       # commit statuses
  checks: write         # check runs
  pull-requests: write  # PR comments
```

## Pull Request Plan

```yaml
name: plan
on:
  pull_request:
  merge_group:

jobs:
  affected:
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
    outputs:
      matrix: ${{ steps.affected.outputs.matrix }}
    steps:
      - uses: actions/checkout@v4
      - id: affected
        run: atmos describe affected --format=matrix

  plan:
    needs: affected
    if: ${{ needs.affected.outputs.matrix != '' }}
    strategy:
      fail-fast: false
      matrix: ${{ fromJson(needs.affected.outputs.matrix) }}
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
      - uses: actions/checkout@v4
      - run: atmos terraform plan "${{ matrix.component }}" -s "${{ matrix.stack }}"
```

## Deploy Affected

```yaml
- run: atmos terraform deploy "${{ matrix.component }}" -s "${{ matrix.stack }}"
```

Use `deploy` in automation when you want a fresh plan followed by apply with auto-approve. Use
manual gates or GitHub environments for production.

## Deploy All Instances

```yaml
- id: instances
  run: atmos list instances --format=matrix
```

Use this for full deploys, initial bootstraps, or drift sweeps. Prefer affected matrices for PRs.

## Profiles and OIDC

```yaml
env:
  ATMOS_PROFILE: github
permissions:
  id-token: write
```

Example profile content:

```yaml
auth:
  providers:
    github-oidc:
      kind: github/oidc
      region: us-east-2
      spec:
        audience: sts.amazonaws.com
  identities:
    plat-dev/terraform:
      kind: aws/assume-role
      via:
        provider: github-oidc
      principal:
        assume_role: arn:aws:iam::123456789012:role/atmos-ci
```

No routine `atmos auth login` is needed in OIDC jobs. Atmos resolves credentials when commands run.

## Migration From Deprecated Actions

Replace deprecated wrapper actions with direct commands:

| Deprecated action | Native CI replacement |
|---|---|
| Deprecated `cloudposse/github-action-atmos-affected-stacks` | `atmos describe affected --format=matrix` |
| Deprecated `cloudposse/github-action-atmos-terraform-plan` | `atmos terraform plan <component> -s <stack>` |
| Deprecated `cloudposse/github-action-atmos-terraform-apply` | `atmos terraform deploy <component> -s <stack>` |
| Deprecated `cloudposse/github-action-setup-atmos` | `container: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}` |

Deprecated: do not use `integrations.github.gitops`; model CI behavior directly in workflows and stack settings.
