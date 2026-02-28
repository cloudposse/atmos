# GitHub Actions Reference for Atmos

## setup-atmos

**Repository:** `cloudposse/github-action-setup-atmos`

Installs Atmos in the GitHub Actions runner.

### Usage

```yaml
- name: Setup Atmos
  uses: cloudposse/github-action-setup-atmos@v2
  with:
    atmos-version: 1.88.0
```

### Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `atmos-version` | No | `latest` | The version of Atmos to install |
| `token` | No | `github.token` | GitHub token for downloading Atmos |

## affected-stacks

**Repository:** `cloudposse/github-action-atmos-affected-stacks`

Runs `atmos describe affected` and outputs a matrix for parallel job execution.

### How It Works

1. Installs Atmos
2. Runs `atmos describe affected` comparing the PR branch to the base branch
3. Outputs a list of affected stacks as raw output and as a GitHub Actions matrix

### Key Inputs

| Input | Required | Description |
|-------|----------|-------------|
| `atmos-config-path` | Yes | Path to the `atmos.yaml` configuration file |
| `atmos-version` | No | Version of Atmos to install (default: `>= 1.63.0`) |
| `head-ref` | No | The head ref to compare (defaults to current branch) |
| `base-ref` | No | The base ref to compare against (defaults to main branch) |

### Outputs

| Output | Description |
|--------|-------------|
| `affected` | JSON array of affected component/stack pairs |
| `has-affected-stacks` | Boolean indicating if any stacks were affected |
| `matrix` | GitHub Actions matrix object for use in strategy.matrix |

### Usage in Matrix

```yaml
jobs:
  affected:
    runs-on: ubuntu-latest
    outputs:
      matrix: ${{ steps.affected.outputs.matrix }}
      has-affected-stacks: ${{ steps.affected.outputs.has-affected-stacks }}
    steps:
      - uses: actions/checkout@v4
      - id: affected
        uses: cloudposse/github-action-atmos-affected-stacks@v3
        with:
          atmos-config-path: ./rootfs/usr/local/etc/atmos/

  plan:
    needs: affected
    if: needs.affected.outputs.has-affected-stacks == 'true'
    strategy:
      matrix: ${{ fromJson(needs.affected.outputs.matrix) }}
    uses: ./.github/workflows/atmos-terraform-plan-matrix.yaml
    with:
      stacks: ${{ matrix.items }}
```

## atmos-terraform-plan

**Repository:** `cloudposse/github-action-atmos-terraform-plan`

Runs `atmos terraform plan` for a component in a stack, stores the planfile in S3 with DynamoDB
metadata, and generates a GitHub Job Summary with the plan output.

### Features

- Native GitOps with Atmos and Terraform
- GitHub OIDC authentication (no hardcoded credentials)
- Planfile storage in S3 with DynamoDB metadata
- Beautiful Job Summaries with diff-style resource changes
- Highlights destructive operations with warnings
- Compatible with GitHub Cloud and self-hosted runners

### Key Inputs

| Input | Required | Description |
|-------|----------|-------------|
| `component` | Yes | The Atmos component to plan |
| `stack` | Yes | The Atmos stack to plan |
| `atmos-config-path` | Yes | Path to the `atmos.yaml` file |
| `atmos-version` | No | Version of Atmos to install |
| `terraform-plan-role` | Yes* | IAM role ARN for planning (*read from atmos.yaml if >= 1.63.0) |
| `terraform-state-role` | Yes* | IAM role ARN for state access |
| `terraform-state-bucket` | Yes* | S3 bucket for planfile storage |
| `terraform-state-table` | Yes* | DynamoDB table for planfile metadata |

*These inputs are read from `atmos.yaml` `integrations.github.gitops` section for Atmos >= 1.63.0.

### Outputs

| Output | Description |
|--------|-------------|
| `summary` | The plan summary text |
| `has-changes` | Whether the plan detected changes |
| `has-no-changes` | Whether the plan detected no changes |
| `is-error` | Whether the plan encountered an error |

## atmos-terraform-apply

**Repository:** `cloudposse/github-action-atmos-terraform-apply`

Retrieves a stored planfile from S3 and runs `atmos terraform apply`. Intended to run on merge
to the main branch.

### Key Inputs

| Input | Required | Description |
|-------|----------|-------------|
| `component` | Yes | The Atmos component to apply |
| `stack` | Yes | The Atmos stack to apply |
| `atmos-config-path` | Yes | Path to the `atmos.yaml` file |
| `atmos-version` | No | Version of Atmos to install |
| `terraform-apply-role` | Yes* | IAM role ARN for applying |
| `terraform-state-role` | Yes* | IAM role ARN for state access |
| `terraform-state-bucket` | Yes* | S3 bucket for planfile storage |
| `terraform-state-table` | Yes* | DynamoDB table for planfile metadata |

### Requirements

Same S3 bucket, DynamoDB table, and IAM roles as the plan action. The apply action retrieves
the planfile created during the plan phase and applies it.

## atmos-terraform-drift-detection

**Repository:** `cloudposse/github-action-atmos-terraform-drift-detection`

Runs on a schedule to detect Terraform drift across all components and stacks.

### How It Works

1. Gathers all components and stacks in the repository
2. Runs `atmos terraform plan` for each component/stack pair
3. Creates a GitHub Issue for each component/stack with detected drift
4. Each Issue contains a plan summary and metadata for remediation

### Key Inputs

| Input | Required | Description |
|-------|----------|-------------|
| `atmos-config-path` | Yes | Path to the `atmos.yaml` file |
| `atmos-version` | No | Version of Atmos to install |
| `max-opened-issues` | No | Maximum number of drift Issues to create (default: 10) |
| `debug` | No | Enable debug mode |

### Typical Trigger

```yaml
on:
  schedule:
    - cron: "0 */4 * * *"  # Every 4 hours
```

## atmos-terraform-drift-remediation

**Repository:** `cloudposse/github-action-atmos-terraform-drift-remediation`

Remediates drift by applying changes when a drift Issue is labeled.

### How It Works

1. Triggered when a label (typically `apply`) is added to a drift Issue
2. Reads the component and stack metadata from the Issue
3. Retrieves the latest planfile and runs `atmos terraform apply`
4. Closes the Issue as resolved on successful apply

### Key Inputs

| Input | Required | Default | Description |
|-------|----------|---------|-------------|
| `action` | Yes | `remediate` | Action to take: `remediate` or `discard` |
| `atmos-config-path` | Yes | - | Path to the `atmos.yaml` file |
| `atmos-version` | No | `>= 1.63.0` | Version of Atmos to install |
| `issue-number` | Yes | - | The GitHub Issue number to process |
| `debug` | No | `false` | Enable debug mode |

### Typical Trigger

```yaml
on:
  issues:
    types:
      - labeled
```

The workflow checks if the label matches (e.g., `apply`) before proceeding.

## Version Compatibility Matrix

| GitHub Action | Atmos < 1.63.0 | Atmos >= 1.63.0 |
|--------------|----------------|-----------------|
| `affected-stacks` | v2 | v1 or later |
| `atmos-terraform-plan` | v1 | v2 or later (v3 for artifact storage update) |
| `atmos-terraform-apply` | v1 | v2 or later |
| `atmos-terraform-drift-remediation` | v1 | v2 or later |
| `atmos-terraform-drift-detection` | v0 | v1 or later (v2 for artifact storage update) |

## Required GitHub Permissions

All workflows need these permissions for AWS OIDC:

```yaml
permissions:
  id-token: write
  contents: read
```

For drift detection/remediation, also add:

```yaml
permissions:
  issues: write
```

## Infrastructure Requirements

### S3 Bucket for Planfile Storage

```yaml
components:
  terraform:
    gitops/s3-bucket:
      metadata:
        component: s3-bucket
      vars:
        name: gitops-plan-storage
        allow_encrypted_uploads_only: true
```

### DynamoDB Table for Planfile Metadata

```yaml
components:
  terraform:
    gitops/dynamodb:
      metadata:
        component: dynamodb
      vars:
        name: gitops-plan-storage
        hash_key: id
        range_key: ""
        dynamodb_attributes:
          - name: 'createdAt'
            type: 'S'
          - name: 'pr'
            type: 'N'
        global_secondary_index_map:
          - name: pr-createdAt-index
            hash_key: pr
            range_key: createdAt
            projection_type: ALL
            non_key_attributes: []
            read_capacity: null
            write_capacity: null
        ttl_enabled: true
        ttl_attribute: ttl
```

### IAM Roles

- **State access role** -- For reading/writing planfiles to S3 and DynamoDB. Deploy with the `gitops` component.
- **Plan/Apply role** -- For running Terraform plan and apply. Deploy with `aws-teams` and `aws-team-roles` components.
