---
title: Atmos Terraform Drift Detection
sidebar_position: 60
sidebar_label: Terraform Drift Detection
---

The Cloud Posse GitHub Action for "Atmos Terraform Drift Detection" and "Atmos Terraform Drift Remediation" define a scalable pattern for detecting and remediating Terraform drift from within GitHub using workflows and Issues. "Atmos Terraform Drift Detection" will determine drifted Terraform state by running [Atmos Terraform Plan](/integrations/github-actions/atmos-terraform-plan) and creating GitHub Issues for any drifted component and stack. Furthermore, "Atmos Terraform Drift Remediation" will run [Atmos Terraform Apply](/integrations/github-actions/atmos-terraform-apply) for any open Issue if called and close the given Issue. With these two actions, we can fully support drift detection for Terraform directly within the GitHub UI.

This action is intended to be used together with [Atmos Terraform Plan](/integrations/github-actions/atmos-terraform-plan) and [Atmos Terraform Apply](/integrations/github-actions/atmos-terraform-apply).

## Features

This GitHub Action incorporates superior GitOps support for Terraform by utilizing the capabilities of Atmos, enabling efficient management of large enterprise-scale environments.

* **Implements Native GitOps** with Atmos and Terraform.
* **No hardcoded credentials.** Use GitHub OIDC to assume roles.
* **Compatible with GitHub Cloud & Self-hosted Runners** for maximum flexibility. 
* **Beautiful Job Summaries** don't clutter up pull requests with noisy GitHub comments.
* **Automated Drift Detection** Regularly check and track all resources for drift.
* **Free Tier GitHub** Use GitHub Issues to track drifted resources.
* **100% Open Source with Permissive APACHE2 License** means you have no expensive subscriptions or long-term commitments.

## Usage

```mermaid
---
title: Atmos Terraform Drift Detection 
---
stateDiagram-v2
  direction LR
  [*] --> detection : Scheduled Workflow (cron)
  detection --> remediate : Labeled Issue triggers workflow

  state "Atmos Terraform Drift Detection" as detection {
    [*] --> gather
    gather --> plan
    plan --> issue : if changes
    issue --> [*]

    state "Gather every component and stack" as gather
    state "Atmos Terraform Plan" as plan
    state "Create GitHub Issue" as issue 
  }

  state "Atmos Terraform Drift Remediation" as remediate {
    [*] --> fetch
    fetch --> apply
    apply --> close
    close --> [*]

    state "Retrieve Terraform Plan" as fetch
    state "Atmos Terraform Apply" as apply
    state "Close GitHub Issue" as close
  }
```

Drift Detection with Atmos requires two separate workflows. 

### Atmos Terraform Drift Detection

First, we trigger the "Atmos Terraform Drift Detection" workflow on a schedule. This workflow will gather every single component and stack in the repository. Then using that list of components and stacks, run `atmos terraform plan <component> --stack <stack>` for the given component and stack. If there are any changes, the workflow will create a GitHub Issue.

For example in this screenshot, the workflow has gathered two components. Only one has drift, and therefore one new Issue has been created.

![Example Drift Summary](/img/github-actions/drift-summary.png)

Now we can see the new Issue, including a Terraform Plan summary and metadata for applying.

![Example Issue](/img/github-actions/drift-issue.png)

:::tip Limiting the number of Issues

Without a limit, the number of Issues for drifted components can quickly get out-of-hand. In order to create a more manageable developer experience, set a limit for the maximum number of Isuses created by the "Atmos Terraform Drift Detection" action with `max-opened-issues`. 

The default value is `10`.

See [cloudposse/github-action-atmos-terraform-drift-detection](https://github.com/cloudposse/github-action-atmos-terraform-drift-detection/#inputs) for details.

:::

We can quickly see a complete list of all drift components in the "Issues" tab in the GitHub UI.

![Example Issue List](/img/github-actions/drift-issue-list.png)

#### Example Usage

The action expects the atmos configuration file `atmos.yaml` to be present in the repository.
The config should have the following structure:

```yaml
# ./rootfs/usr/local/etc/atmos/atmos.yaml
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

:::tip Important!

**Please note!** This GitHub Action only works with `atmos >= 1.63.0`. If you are using `atmos < 1.63.0` please use [`v0` version](https://github.com/cloudposse/github-action-atmos-terraform-drift-detection/tree/v0).

:::

```yaml
name: 👽 Atmos Terraform Drift Detection

on:
  schedule:
    - cron: "0 * * * *"

permissions:
  id-token: write
  contents: write
  issues: write

jobs:
  select-components:
    name: Select Components
    runs-on: ubuntu-latest
    steps:
      - name: Selected Components
        id: components
        uses: cloudposse/github-action-atmos-terraform-select-components@v2
        with:
          select-filter: '.settings.github.actions_enabled and .metadata.type != "abstract"'
          atmos-config-path: ./rootfs/usr/local/etc/atmos/
          atmos-version: 1.63.0
    outputs:
      stacks: ${{ steps.components.outputs.matrix }}
      has-selected-components: ${{ steps.components.outputs.has-selected-components }}

  plan-atmos-components:
    needs: ["select-components"]
    runs-on: ubuntu-latest
    if: ${{ needs.select-components.outputs.has-selected-components == 'true' }}
    name: Detect Drift (${{ matrix.name }})
    uses: ./.github/workflows/atmos-terraform-plan-matrix.yaml
    strategy:
      max-parallel: 1 # This is important to avoid ddos GHA API
      fail-fast: false # Don't fail fast to avoid locking TF State
      matrix: ${{ fromJson(needs.select-components.outputs.stacks) }}
    with:
      stacks: ${{ matrix.items }}
      sha: ${{ github.sha }}
      drift-detection-mode-enabled: "true"
      continue-on-error: true
      atmos-config-path: ./rootfs/usr/local/etc/atmos/
      atmos-version: 1.63.0
    secrets: inherit

  drift-detection:
    needs: ["plan-atmos-components"]
    if: always()
    name: Reconcile issues
    runs-on: ubuntu-latest
    steps:
      - name: Drift Detection
        uses: cloudposse/github-action-atmos-terraform-drift-detection@v1
        with:
          max-opened-issues: '4'
          process-all: 'true'
```

#### 256 Matrix Limitation

:::warning 

GitHub Actions support 256 matrix jobs in a single workflow at most, [ref](https://docs.github.com/en/actions/using-jobs/using-a-matrix-for-your-jobs#using-a-matrix-strategy). 

:::

When planning all stacks in an Atmos environment, we frequently plan more than 256 component in the stacks at a time. In order to work around this limitation by GitHub, we can add an additional layer of abstraction using reusable workflows. 

For example, the "Atmos Terraform Plan" workflow can call "Atmos Terraform Plan Matrix" workflow which then calls the "Atmos Terraform Plan" Composite Action.

```mermaid
---
title: GitHub Job Matrices
---
stateDiagram-v2
  direction LR
  [*] --> top
  top --> reusable1
  top --> reusable2
  top --> reusable3
  reusable1 --> component1
  reusable1 --> component2
  reusable1 --> component3
  reusable2 --> component4
  reusable2 --> component5
  reusable2 --> component6
  reusable3 --> component7
  reusable3 --> component8
  reusable3 --> component9

  state "Atmos Terraform Plan (Top Level Workflow)" as top
  state "Atmos Terraform Plan Matrix (Reusable Workflow) - Group A" as reusable1
  state "Atmos Terraform Plan Matrix (Reusable Workflow) - Group B" as reusable2
  state "Atmos Terraform Plan Matrix (Reusable Workflow) - Group C" as reusable3
  state "Atmos Terraform Plan (Composite Action) - Component 1" as component1
  state "Atmos Terraform Plan (Composite Action) - Component 2" as component2
  state "Atmos Terraform Plan (Composite Action) - Component 3" as component3
  state "Atmos Terraform Plan (Composite Action) - Component 4" as component4
  state "Atmos Terraform Plan (Composite Action) - Component 5" as component5
  state "Atmos Terraform Plan (Composite Action) - Component 6" as component6
  state "Atmos Terraform Plan (Composite Action) - Component 7" as component7
  state "Atmos Terraform Plan (Composite Action) - Component 8" as component8
  state "Atmos Terraform Plan (Composite Action) - Component 9" as component9
```


### Atmos Terraform Drift Remediation

Once we have an open Issue for a drifted component, we can trigger another workflow to remediate the drifted Terraform resources. When an Issue is labeled with `apply`, the "Atmos Terraform Drift Remediation" workflow will take the component and stack in the given Issue and run `atmos terraform apply <component> --stack <stack>` using the latest Terraform Planfile. If the apply is successful, the workflow will close the given Issue as resolved.

#### Example Usage

:::tip Important!

**Please note!** This GitHub Action only works with `atmos >= 1.63.0`. If you are using `atmos < 1.63.0` please use [`v1` version](https://github.com/cloudposse/github-action-atmos-terraform-drift-remediation/tree/v1).

:::

```yaml
name: 👽 Atmos Terraform Drift Remediation
run-name: 👽 Atmos Terraform Drift Remediation

on:
  issues:
    types:
      - labeled
      - closed

permissions:
  id-token: write
  contents: read

jobs:
  remediate-drift:
    runs-on: ubuntu-latest
    name: Remediate Drift
    if: |
      github.event.action == 'labeled' &&
      contains(join(github.event.issue.labels.*.name, ','), 'apply')
    steps:
      - name: Remediate Drift
        uses: cloudposse/github-action-atmos-terraform-drift-remediation@v2
        with:
          issue-number: ${{ github.event.issue.number }}
          action: remediate
          atmos-config-path: ./rootfs/usr/local/etc/atmos/
          atmos-version: 1.63.0

  discard-drift:
    runs-on: ubuntu-latest
    name: Discard Drift
    if: |
      github.event.action == 'closed' &&
      !contains(join(github.event.issue.labels.*.name, ','), 'remediated')
    steps:
      - name: Discard Drift
        uses: cloudposse/github-action-atmos-terraform-drift-remediation@v2
        with:
          issue-number: ${{ github.event.issue.number }}
          action: discard
          atmos-config-path: ./rootfs/usr/local/etc/atmos/
          atmos-version: 1.63.0
```

## Requirements

This action has the same requirements as [Atmos Terraform Plan](/integrations/github-actions/atmos-terraform-plan). Use the same S3 Bucket, DynamoDB table, and IAM Roles created with the requirements described there.
