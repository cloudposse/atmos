---
title: Atmos Terraform Apply
sidebar_position: 50
sidebar_label: Terraform Apply
---

The Cloud Posse GitHub Action for "Atmos Terraform Apply" simplifies provisioning Terraform entirely within GitHub Action workflows. It makes it very easy to understand exactly what happened directly within the GitHub UI.

Given any component and stack in an Atmos supported infrastructure environment, [`github-action-atmos-terraform-apply`](https://github.com/cloudposse/github-action-atmos-terraform-apply) will retrieve an existing Terraform [planfile](https://developer.hashicorp.com/terraform/tutorials/automation/automate-terraform) from a given S3 bucket using metadata stored inside a DynamoDB table, run `atmos terraform apply` with that planfile, and format the Terraform Apply result as part of a [GitHub Workflow Job Summary](https://github.blog/2022-05-09-supercharging-github-actions-with-job-summaries/).

This action is intended to be used together with [Atmos Terraform Plan](/integrations/github-actions/atmos-terraform-plan)

## Features

This GitHub Action incorporates superior GitOps support for Terraform by utilizing the capabilities of Atmos, enabling efficient management of large enterprise-scale environments.

* **Implements Native GitOps** with Atmos and Terraform tightly integrated with GitHub UI
* **No hardcoded credentials.** Use GitHub OIDC to assume roles.
* **Compatible with GitHub Cloud & Self-hosted Runners** for maximum flexibility. 
* **Beautiful Job Summaries** don't clutter up pull requests with noisy GitHub comments
* **100% Open Source with Permissive APACHE2 License** means there are no expensive subscriptions or long-term commitments.


## Screenshots

In the following screenshot, we see a successful "apply" Job Summary report. The report utilizes badges to clearly indicate success or failure. Unnecessary details are neatly hidden behind a collapsible `<details/>` block, providing a streamlined view. Additionally, a direct link is provided to view the job run, eliminating the need for developers to search for information about any potential issues.

![Example Image](/img/github-actions/tf_apply.png)

## Usage Example

```yaml
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
  plan:
    runs-on: ubuntu-latest
    steps:
      - name: Plan Atmos Component
        uses: cloudposse/github-action-atmos-terraform-apply@v1
        with:
          component: "foobar"
          stack: "plat-ue2-sandbox"
          component-path: "components/terraform/s3-bucket"
          terraform-apply-role: "arn:aws:iam::111111111111:role/acme-core-gbl-identity-gitops"
          terraform-state-bucket: "acme-core-ue2-auto-gitops"
          terraform-state-role: "arn:aws:iam::999999999999:role/acme-core-ue2-auto-gitops-gha"
          terraform-state-table: "acme-core-ue2-auto-gitops"
          aws-region: "us-east-2"

```

### Requirements

This action has the same requirements as [Atmos Terraform Plan](/integrations/github-actions/atmos-terraform-plan). Use the same S3 Bucket, DynamoDB table, and IAM Roles created with the requirements described there.

