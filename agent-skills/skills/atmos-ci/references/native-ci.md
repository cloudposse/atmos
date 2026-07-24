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
      - has_errors
      - exit_code
      - resources_to_create
      - resources_to_change
      - resources_to_replace
      - resources_to_destroy
      - stack
      - component
      - summary
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

`ci.output.variables` is an allowlist filter over the variables the terraform CI plugin already
builds (an empty list means write all of them); it never invents new names. Only the terraform
plugin implements native output variables today (helm/helmfile/kubernetes plugins do not). Beyond
`has_changes`/`has_errors`/`exit_code`/`stack`/`component`/`command`/`summary`, plan/apply/destroy
add `resources_to_create`/`resources_to_change`/`resources_to_replace`/`resources_to_destroy`,
apply/test add `success`, and test adds `tests_total`/`tests_passed`/`tests_failed`/
`tests_errored`/`tests_skipped`. After a successful `apply`, each Terraform output is also written
as `output_<name>` — those bypass the allowlist and are always included.

| Native CI feature | Atmos config | GitHub permission |
|---|---|---|
| Job summaries | `ci.summary.enabled` | none |
| Output variables | `ci.output.enabled` | none |
| Log groups | `ci.groups.mode` | none |
| Commit statuses/checks | `ci.checks.enabled` | `statuses: write` or `checks: write` |
| PR comments | `ci.comments.enabled` | `pull-requests: write` |

Set `GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}` when checks or comments are enabled.

Native CI writes outputs to `$GITHUB_OUTPUT` when `ci.enabled: true`, `ci.output.enabled: true`, and
GitHub Actions provides the output file. Give Atmos steps an `id`, expose job outputs when another
job needs them, and read those values through `needs.<job>.outputs.*`.

## Log Groups

Configure `ci.groups.mode` to fold Atmos output into collapsible GitHub Actions `::group::` regions:

```yaml
ci:
  enabled: true
  groups:
    mode: auto      # auto (default) | invocation | off
```

- `auto` (default): finest grouping per command — one group per workflow/custom-command step, and
  one group per phase (`terraform init`, `terraform apply`, etc.) of a terraform/tofu invocation.
- `invocation`: one group around the whole top-level `atmos <command>` run; suppresses finer
  step/phase grouping.
- `off`: no grouping.

Modes are mutually exclusive; CI providers do not support nested groups.

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
`dependencies.tools` is the source of truth for tools required by components, workflows, and custom
commands.

Use explicit `atmos toolchain install ...` only for job-level scripted tools that are not already
declared in `dependencies.tools`. In GitHub Actions, `atmos toolchain env --format=github`
automatically appends toolchain paths to `$GITHUB_PATH` when available. Most install steps added
to fix missing tools should instead become `dependencies.tools` on the component, workflow, hook,
or custom command that invokes the tool.

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
      - uses: actions/checkout@v6
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
      - uses: actions/checkout@v6
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

Use this for full deploys, initial bootstraps, or Atmos Pro inventory/drift workflows. Prefer
affected matrices for PRs.

## Atmos Pro Uploads and Drift

Upload affected stack data for PR and merge-queue correlation:

```yaml
- run: atmos describe affected --upload
```

Upload inventory for Atmos Pro:

```yaml
- run: atmos list instances --upload
```

For drift detection, configure `settings.pro.drift_detection.enabled: true` in stack config and
upload plan status from the workflow Atmos Pro dispatches:

```yaml
- run: atmos terraform plan "${{ matrix.component }}" -s "${{ matrix.stack }}" --upload-status
```

Prefer Atmos Pro drift detection over hand-rolled scheduled GitHub Actions.

## Cache

Use `atmos ci cache` for CI cache and `atmos terraform cache` for the Terraform registry cache.
They are different from Terraform's own provider plugin cache.

```yaml
- id: cache
  run: atmos ci cache paths
```

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
| Deprecated `cloudposse/github-action-atmos-terraform-drift-detection` | Atmos Pro drift detection |
| Deprecated `cloudposse/github-action-atmos-terraform-drift-remediation` | Atmos Pro remediation workflow |
| Deprecated `cloudposse/github-action-setup-atmos` | `container: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}` |

Deprecated: do not use `integrations.github.gitops`; model CI behavior directly in workflows and stack settings.
