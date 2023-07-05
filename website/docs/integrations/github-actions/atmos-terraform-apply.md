---
title: Atmos Terraform Apply
sidebar_position: 10
sidebar_label: Terraform Apply
---

The Cloud Posse GitHub Action "Atmos Terraform Apply" runs Terraform entirely from GitHub Action workflows.

Given any component and stack in an Atmos supported infrastructure environment, [`github-action-atmos-terraform-apply`](https://github.com/cloudposse/github-action-atmos-terraform-apply) will retrive an existing Terraform planfile from a given S3 bucket using metadata in a DynamoDB table, run `atmos terraform apply` with that planfile, and format the Terraform Apply result as part of a GitHub Workflow Summary.

This action is intended to be used with [Atmos Terraform Plan](/integrations/github-actions/atmos-terraform-plan)

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

