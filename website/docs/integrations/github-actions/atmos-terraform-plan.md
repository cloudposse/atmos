---
title: Atmos Terraform Plan
sidebar_position: 10
sidebar_label: Terraform Plan
---

The Cloud Posse GitHub Action "Atmos Terraform Plan" runs Terraform entirely from GitHub Action workflows.

Given any component and stack in an Atmos supported infrastructure environment, [`github-action-atmos-terraform-plan`](https://github.com/cloudposse/github-action-atmos-terraform-plan) will run `atmos terraform plan`, generate a Terraform planfile, store this planfile in a S3 Bucket with metadata in DynamodDB, and finally format the Terraform Plan result as part of a GitHub Workflow Summary.

This action is intended to be used with [Atmos Terraform Apply](/integrations/github-actions/atmos-terraform-apply)

## Usage Example

```yaml
name: "atmos-terraform-plan"

on:
  workflow_dispatch: {}
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

`
