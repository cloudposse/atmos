---
title: Atmos Terraform Apply
sidebar_position: 50
sidebar_label: Terraform Apply
---

The Cloud Posse GitHub Action for "Atmos Terraform Apply" simplifies provisioning Terraform entirely within GitHub Action workflows. It makes it very easy to understand exactly what happened directly within the GitHub UI.

Given any component and stack in an Atmos supported infrastructure environment, [`github-action-atmos-terraform-apply`](https://github.com/cloudposse/github-action-atmos-terraform-apply) will retrieve an existing Terraform [planfile](https://developer.hashicorp.com/terraform/tutorials/automation/automate-terraform) from a given S3 bucket using metadata stored inside a DynamoDB table, run `atmos terraform apply` with that planfile, and format the Terraform Apply result as part of a [GitHub Workflow Job Summary](https://github.blog/2022-05-09-supercharging-github-actions-with-job-summaries/).

This action is intended to be used together with [Atmos Terraform Plan](/integrations/github-actions/atmos-terraform-plan), as well as integrated into drift detection with [Atmos Terraform Detection and Remediation](/integrations/github-actions/atmos-terraform-drift-detection) GitHub Actions.

## Features

This GitHub Action incorporates superior GitOps support for Terraform by utilizing the capabilities of Atmos, enabling efficient management of large enterprise-scale environments.

* **Implements Native GitOps** with Atmos and Terraform tightly integrated with GitHub UI
* **No hardcoded credentials.** Use GitHub OIDC to assume roles.
* **Compatible with GitHub Cloud & Self-hosted Runners** for maximum flexibility. 
* **Beautiful Job Summaries** don't clutter up pull requests with noisy GitHub comments
* **100% Open Source with Permissive APACHE2 License** means there are no expensive subscriptions or long-term commitments.


## Screenshots

In the following screenshot, we see a successful "apply" Job Summary report. The report utilizes badges to clearly indicate success or failure. Unnecessary details are neatly hidden behind a collapsible `<details/>` block, providing a streamlined view. Additionally, a direct link is provided to view the job run, eliminating the need for developers to search for information about any potential issues.

![Example Image](/img/github-actions/apply.png)

## Usage

In this example, the action is triggered when certain events occur, such as a manual workflow dispatch or the opening, synchronization, or reopening of a pull request, specifically on the main branch. It specifies specific permissions related to assuming roles in AWS. Within the "apply" job, the "component" and "stack" are hardcoded (`foobar` and `plat-ue2-sandbox`). In practice, these are usually derived from another action. 

:::tip Passing Affected Stacks

We recommend combining this action with the [`affected-stacks`](/integrations/github-actions/affected-stacks) GitHub Action inside a matrix to plan all affected stacks in parallel.

:::


```yaml
  # .github/workflows/atmos-terraform-apply.yaml
  name: "atmos-terraform-apply"

  on:
    workflow_dispatch:
    pull_request:
      types:
        - closed
      branches:
        - main

  # These permissions are required for GitHub to assume roles in AWS
  permissions:
    id-token: write
    contents: read

  jobs:
    apply:
      runs-on: ubuntu-latest
      steps:
        - name: Terraform Apply
          uses: cloudposse/github-action-atmos-terraform-apply@v2
          with:
            component: "foobar"
            stack: "plat-ue2-sandbox"
```

with the following configuration as an example:

```yaml
  # .github/config/atmos-gitops.yaml
  atmos-config-path: ./rootfs/usr/local/etc/atmos/
  atmos-version: 1.65.0
  aws-region: us-east-2
  enable-infracost: false
  group-by: .stack_slug | split("-") | [.[0], .[2]] | join("-")
  sort-by: .stack_slug
  terraform-apply-role: arn:aws:iam::yyyyyyyyyyyy:role/cptest-core-gbl-identity-gitops
  terraform-plan-role: arn:aws:iam::yyyyyyyyyyyy:role/cptest-core-gbl-identity-gitops
  terraform-state-bucket: cptest-core-ue2-auto-gitops
  terraform-state-role: arn:aws:iam::xxxxxxxxxxxx:role/cptest-core-ue2-auto-gitops-gha
  terraform-state-table: cptest-core-ue2-auto-gitops
  terraform-version: 1.65.0
```

## Requirements

This action has the same requirements as [Atmos Terraform Plan](/integrations/github-actions/atmos-terraform-plan). Use the same S3 Bucket, DynamoDB table, and IAM Roles created with the requirements described there.
