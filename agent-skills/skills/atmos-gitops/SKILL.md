---
name: atmos-gitops
description: "CI/CD: GitHub Actions, Spacelift, Atlantis, `atmos describe affected` for change detection"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos GitOps and CI/CD Integrations

## Overview

Atmos provides native CI/CD integration patterns for managing Terraform infrastructure through automated
pipelines. The three primary integration paths are GitHub Actions, Spacelift, and Atlantis. Each approach
leverages Atmos stack configurations and the `atmos describe affected` command to detect changes between
commits or branches, enabling efficient PR-based plan and merge-based apply workflows.

All integrations follow a common GitOps pattern:
1. Detect which stacks and components changed using `atmos describe affected`
2. Run `atmos terraform plan` for each affected component/stack pair
3. Review plan output in the PR or CI/CD UI
4. Apply changes on merge (or via manual trigger / label)

## Change Detection with `atmos describe affected`

The `atmos describe affected` command is the foundation of all CI/CD integrations. It compares two Git
commits to produce a list of affected Atmos components and stacks.

### How It Works

The command performs these steps:
1. Clone or reference the target branch/commit (the comparison baseline)
2. Deep-merge all stack configurations for both the current working branch and the target branch
3. Identify changes in component directories (file-level diffs)
4. Compare each section of the stack configuration to detect differences
5. Output a list of affected components and stacks

When component source directories have changed, all related stacks are marked affected and Atmos skips
further configuration comparison for those stacks, streamlining the process.

### Command Usage

```shell
# Compare current branch against the default branch (main)
atmos describe affected

# Compare against a specific branch
atmos describe affected --ref refs/heads/main

# Compare against a specific commit SHA
atmos describe affected --sha 6a9b2c1

# Use a pre-cloned repo for the target reference
atmos describe affected --repo-path /path/to/cloned/target

# Output as JSON for CI/CD consumption
atmos describe affected --format json
```

### Output Format

The command outputs a JSON array of affected items. Each item contains:

```json
[
  {
    "component": "vpc",
    "component_type": "terraform",
    "stack": "plat-ue2-dev",
    "stack_slug": "plat-ue2-dev-vpc",
    "affected": "stack.vars"
  },
  {
    "component": "eks/cluster",
    "component_type": "terraform",
    "stack": "plat-ue2-prod",
    "stack_slug": "plat-ue2-prod-eks-cluster",
    "affected": "component"
  }
]
```

Fields:
- `component` -- The Atmos component name
- `component_type` -- Always `terraform` for Terraform components
- `stack` -- The Atmos stack name
- `stack_slug` -- A slug combining stack and component (used for matrix grouping)
- `affected` -- What changed: `component` (source files), `stack.vars`, `stack.env`, `stack.settings`, etc.

### Filtering and Grouping for CI/CD Matrices

The output can be filtered and grouped for GitHub Actions matrix strategies using jq expressions
configured in `atmos.yaml`:

```yaml
# atmos.yaml
integrations:
  github:
    gitops:
      matrix:
        sort-by: .stack_slug
        group-by: .stack_slug | split("-") | [.[0], .[2]] | join("-")
```

The `group-by` expression creates groups for parallel execution, which helps work around the
GitHub Actions 256-job matrix limit.

## GitHub Actions Integration

Cloud Posse provides a set of GitHub Actions designed for Atmos-native GitOps workflows. These actions
are open source (Apache 2.0 license), require no subscriptions, and use GitHub OIDC for authentication
(no hardcoded credentials).

### Required Infrastructure

GitHub Actions that manage planfiles require:
- **S3 bucket** for storing Terraform planfiles
- **DynamoDB table** for planfile metadata (hash key: `id`, GSI: `pr-createdAt-index`)
- **Two IAM roles**: one for plan/apply operations, one for accessing the S3/DynamoDB state storage
- **atmos.yaml** configuration with the `integrations.github.gitops` section

### atmos.yaml Configuration for GitHub Actions

```yaml
# atmos.yaml
integrations:
  github:
    gitops:
      terraform-version: 1.5.2
      infracost-enabled: false
      artifact-storage:
        region: us-east-2
        bucket: cptest-core-ue2-auto-gitops
        table: cptest-core-ue2-auto-gitops-plan-storage
        role: arn:aws:iam::xxxxxxxxxxxx:role/cptest-core-ue2-auto-gitops-gha
      role:
        plan: arn:aws:iam::yyyyyyyyyyyy:role/cptest-core-gbl-identity-gitops
        apply: arn:aws:iam::yyyyyyyyyyyy:role/cptest-core-gbl-identity-gitops
      matrix:
        sort-by: .stack_slug
        group-by: .stack_slug | split("-") | [.[0], .[2]] | join("-")
```

### Core GitHub Actions

1. **setup-atmos** -- Installs Atmos in the workflow runner
2. **atmos-terraform-plan** -- Runs `atmos terraform plan`, stores planfiles in S3, posts Job Summaries
3. **atmos-terraform-apply** -- Retrieves planfiles from S3, runs `atmos terraform apply`
4. **affected-stacks** -- Runs `atmos describe affected` and outputs a matrix for parallel jobs
5. **atmos-terraform-drift-detection** -- Scheduled workflow to detect drift and create GitHub Issues
6. **atmos-terraform-drift-remediation** -- Applies fixes for drifted components via IssueOps

### GitOps Workflow Pattern

The standard PR-based workflow:

1. Developer opens a PR with infrastructure changes
2. The plan workflow triggers:
   a. `affected-stacks` action identifies changed components/stacks
   b. Output feeds into a matrix strategy
   c. `atmos-terraform-plan` runs for each affected component/stack pair
   d. Plan summaries appear as GitHub Job Summaries
3. Team reviews plan output in the PR
4. On merge to main, the apply workflow triggers:
   a. `atmos-terraform-apply` retrieves stored planfiles from S3
   b. Applies each affected component/stack pair
   c. Apply summaries appear as Job Summaries

### Drift Detection and Remediation

Drift detection runs on a schedule (cron):

1. The drift detection workflow gathers all components and stacks
2. Runs `atmos terraform plan` for each component/stack pair
3. If drift is detected, creates a GitHub Issue with plan details
4. To remediate, add the `apply` label to the Issue
5. The drift remediation workflow detects the label, runs `atmos terraform apply`, and closes the Issue

The `max-opened-issues` input (default: 10) limits how many drift Issues are created per run.

### 256 Matrix Limitation

GitHub Actions supports at most 256 matrix jobs per workflow. For large environments, use a two-level
workflow pattern:
- Top-level workflow groups affected stacks using the `group-by` jq expression
- Each group is dispatched to a reusable workflow
- The reusable workflow runs the actual plan/apply matrix for its group

## Spacelift Integration

Atmos natively supports Spacelift through stack configuration settings and a Terraform module
that reads YAML stack configs to provision Spacelift resources.

### Stack Configuration for Spacelift

Configure Spacelift behavior in the `settings.spacelift` section of component configurations:

```yaml
components:
  terraform:
    my-component:
      settings:
        spacelift:
          workspace_enabled: true
          administrative: false
          autodeploy: true
          before_init: []
          component_root: components/terraform/my-component
          description: "My component description"
          stack_destructor_enabled: false
          worker_pool_name: null
          policies_enabled: []
          administrative_trigger_policy_enabled: false
          policies_by_id_enabled:
            - my-custom-policy
          terraform_workflow_tool: TERRAFORM  # or OPEN_TOFU
```

Key settings:
- `workspace_enabled` -- Whether this component/stack is managed in Spacelift
- `administrative` -- Whether this is an admin stack that manages other stacks
- `autodeploy` -- Automatically apply on merge without manual confirmation
- `component_root` -- Path to the Terraform component directory
- `policies_by_id_enabled` -- List of Spacelift policy IDs to attach
- `terraform_workflow_tool` -- Use `TERRAFORM` or `OPEN_TOFU`

### Spacelift Stack Dependencies

Define component dependencies using `settings.depends_on`:

```yaml
components:
  terraform:
    my-component:
      settings:
        depends_on:
          1:
            component: "vpc"
          2:
            component: "eks/cluster"
            tenant: "plat"
            environment: "ue2"
            stage: "prod"
```

Each dependency specifies a `component` (required) and optional context variables
(`namespace`, `tenant`, `environment`, `stage`) to reference components in other stacks.

### OpenTofu Support

To make OpenTofu the default for all Spacelift stacks:

```yaml
settings:
  spacelift:
    terraform_workflow_tool: OPEN_TOFU
```

Or configure per-component to override the global setting.

## Atlantis Integration

Atmos supports Atlantis for Terraform Pull Request Automation through three commands:

1. `atmos atlantis generate repo-config` -- Generate `atlantis.yaml` repo-level configuration
2. `atmos terraform generate backends` -- Generate backend configuration for all components
3. `atmos terraform generate varfiles` -- Generate deep-merged varfiles for all stacks

### Atlantis Configuration in atmos.yaml

```yaml
integrations:
  atlantis:
    path: "atlantis.yaml"
    config_templates:
      config-1:
        version: 3
        automerge: true
        delete_source_branch_on_merge: true
        parallel_plan: true
        parallel_apply: true
        allowed_regexp_prefixes:
          - dev/
          - staging/
          - prod/
    project_templates:
      project-1:
        name: "{tenant}-{environment}-{stage}-{component}"
        workspace: "{workspace}"
        dir: "{component-path}"
        terraform_version: v1.8
        delete_source_branch_on_merge: true
        autoplan:
          enabled: true
          when_modified:
            - "**/*.tf"
            - "varfiles/$PROJECT_NAME.tfvars"
          apply_requirements:
            - "approved"
    workflow_templates:
      workflow-1:
        plan:
          steps:
            - run: terraform init -input=false
            - run: terraform workspace select $WORKSPACE
            - run: terraform plan -input=false -refresh -out $PLANFILE -var-file varfiles/$PROJECT_NAME.tfvars
        apply:
          steps:
            - run: terraform apply $PLANFILE
```

### Generating the Atlantis Configuration

```shell
atmos atlantis generate repo-config --config-template config-1 --project-template project-1
```

This generates an `atlantis.yaml` file with a project entry for each Atmos component in every stack,
using the specified config and project templates.

### Stack-Level Atlantis Configuration

Atlantis settings can also be overridden in stack manifests using `settings.atlantis`, which deep-merges
with the `integrations.atlantis` section in `atmos.yaml`.

## Environment Promotion Patterns

Atmos supports environment promotion through its stack inheritance model:

1. **Catalog-based defaults** -- Define common configuration in `stacks/catalog/`
2. **Environment overrides** -- Override values per environment in stack manifests
3. **PR-based promotion** -- Use Git branches and PRs to promote changes through environments
4. **Sequential apply** -- Use CI/CD dependencies to apply to dev first, then staging, then prod

Example promotion workflow:
- Changes merged to `main` auto-apply to `dev`
- Manual approval (or separate PR) promotes to `staging`
- Another manual approval promotes to `prod`

## Best Practices for CI/CD with Atmos

1. **Use `atmos describe affected`** to minimize CI/CD runtime by only planning/applying changed stacks
2. **Store planfiles securely** -- Use encrypted S3 buckets with DynamoDB locking for GitHub Actions
3. **Use GitHub OIDC** -- Never hardcode AWS credentials in workflows
4. **Group matrix jobs** -- Use the `group-by` setting to work around the 256-job GitHub Actions limit
5. **Limit drift detection Issues** -- Set `max-opened-issues` to keep drift manageable
6. **Use separate IAM roles** for plan and apply operations (least privilege)
7. **Pin Atmos and Terraform versions** in CI/CD to ensure reproducibility
8. **Run `atmos validate stacks`** early in CI/CD pipelines to catch configuration errors
9. **Use Job Summaries** instead of PR comments to avoid noisy pull requests
10. **Test infrastructure changes** in lower environments before promoting to production

## Compatibility Notes

GitHub Actions versions must match the Atmos version:
- Atmos >= 1.63.0: Use v2+ of plan, apply, drift-remediation; v1+ of drift-detection and affected-stacks
- Atmos < 1.63.0: Use older action versions (v1 plan/apply, v0 drift-detection, v2 affected-stacks)

The `integrations.github.gitops` configuration in `atmos.yaml` is supported in Atmos >= 1.63.0. Earlier
versions pass the settings directly as GitHub Action inputs.

## Additional Resources

- For complete GitHub Actions inputs/outputs, see [references/github-actions.md](references/github-actions.md)
- For Spacelift stack configuration details, see [references/spacelift.md](references/spacelift.md)
