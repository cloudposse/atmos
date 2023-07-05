---
title: Atmos Terraform Plan
sidebar_position: 40
sidebar_label: Terraform Plan
---

The Cloud Posse GitHub Action for "Atmos Terraform Plan" runs Terraform entirely from GitHub Action workflows.

Given any component and stack in an Atmos supported infrastructure environment, [`github-action-atmos-terraform-plan`](https://github.com/cloudposse/github-action-atmos-terraform-plan) will run `atmos terraform plan`, generate a Terraform [planfile](https://developer.hashicorp.com/terraform/tutorials/automation/automate-terraform), store this planfile in an S3 Bucket with metadata in DynamodDB, and finally format the Terraform Plan result as part of a [GitHub Workflow Job Summary](https://github.blog/2022-05-09-supercharging-github-actions-with-job-summaries/).

This action is intended to be used with [Atmos Terraform Apply](/integrations/github-actions/atmos-terraform-apply)

## Usage Example

```yaml
name: "atmos-terraform-plan"

on:
  workflow_dispatch:
  pull_request:
    types:
      - opened
      - synchronize
      - reopened
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
        uses: cloudposse/github-action-atmos-terraform-plan@v1
        with:
          component: "foobar"
          stack: "plat-ue2-sandbox"
          component-path: "components/terraform/s3-bucket"
          terraform-plan-role: "arn:aws:iam::111111111111:role/acme-core-gbl-identity-gitops"
          terraform-state-bucket: "acme-core-ue2-auto-gitops"
          terraform-state-role: "arn:aws:iam::999999999999:role/acme-core-ue2-auto-gitops-gha"
          terraform-state-table: "acme-core-ue2-auto-gitops"
          aws-region: "us-east-2"

```

### Requirements

This GitHub Action expects an S3 bucket, DynamoDB table, and two access roles. 

#### S3 Bucket

This action can use any S3 Bucket to keep track of your planfiles. Just ensure the bucket is properly locked down since planfiles may contain secrets.

For example, [vendor in](/core-concepts/components/vendoring) the [`s3-component`](https://docs.cloudposse.com/components/library/aws/s3-bucket/), then using [Atmos stack configuration](/core-concepts/stacks/), define a bucket using the [`s3-bucket` component](https://github.com/cloudposse/terraform-aws-components/tree/main/modules/s3-bucket) with this catalog configuration:

```yaml
import:
  - catalog/s3-bucket/defaults

components:
  terraform:
    # S3 Bucket for storing Terraform Plans
    gitops/s3-bucket:
      metadata:
        component: s3-bucket
        inherits:
          - s3-bucket/defaults
      vars:
        name: gitops-plan-storage
        allow_encrypted_uploads_only: false
```


Assign this S3 Bucket ARN to the `terraform-plan-bucket` input.

#### DynamoDB Table

Similarly, a simple DynamoDB table can be provisioned using our [`dynamodb` component](https://github.com/cloudposse/terraform-aws-components/tree/main/modules/dynamodb). Set the **Hash Key** and **Range Key** as follows:

```yaml
import:
  - catalog/dynamodb/defaults

components:
  terraform:
    # DynamoDB table used to store metadata for Terraform Plans
    gitops/dynamodb:
      metadata:
        component: dynamodb
        inherits:
          - dynamodb/defaults
      vars:
        name: gitops-plan-storage
        # These keys (case-sensitive) are required for the cloudposse/github-action-terraform-plan-storage action
        hash_key: id
        range_key: createdAt
```


Pass the ARN of this table as the input to the `terraform-plan-table` of the `cloudposse/github-action-atmos-terraform-plan` GitHub Action.

#### IAM Access Roles

First create an access role for storing and retrieving planfiles from the S3 Bucket and DynamoDB table. Assign this role ARN to the `terraform-state-role` input.

Next, create a role for GitHub workflows to use to plan and apply Terraform. We typically create an "AWS Team" with our [`aws-teams` component](https://docs.cloudposse.com/components/library/aws/aws-teams/), and then allow this team to assume `terraform` in the delegated accounts with our [`aws-team-roles` component](https://docs.cloudposse.com/components/library/aws/aws-team-roles/). Assign this role ARN to the `terraform-plan-role` input
