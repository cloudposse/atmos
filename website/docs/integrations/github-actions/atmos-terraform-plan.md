---
title: Atmos Terraform Plan
sidebar_position: 40
sidebar_label: Terraform Plan
---

The Cloud Posse GitHub Action for "Atmos Terraform Plan" simplifies provisioning Terraform from within GitHub using workflows. Understand precisely what to expect from running a `terraform plan` from directly within the GitHub UI for any Pull Request.

Given any component and stack in an Atmos supported infrastructure environment, [`github-action-atmos-terraform-plan`](https://github.com/cloudposse/github-action-atmos-terraform-plan) will run `atmos terraform plan`, generate a Terraform [planfile](https://developer.hashicorp.com/terraform/tutorials/automation/automate-terraform), store this planfile in an S3 Bucket with metadata in DynamodDB, and finally format the Terraform Plan result as part of a [GitHub Workflow Job Summary](https://github.blog/2022-05-09-supercharging-github-actions-with-job-summaries/).

This action is intended to be used together with [Atmos Terraform Apply](/integrations/github-actions/atmos-terraform-apply) and [Atmos Affected Stacks](/integrations/github-actions/affected-stacks), as well as integrated into drift detection with [Atmos Terraform Detection and Remediation](/integrations/github-actions/atmos-terraform-drift-detection).

## Features

This GitHub Action incorporates superior GitOps support for Terraform by utilizing the capabilities of Atmos, enabling efficient management of large enterprise-scale environments.

* **Implements Native GitOps** with Atmos and Terraform
* **No hardcoded credentials.** Use GitHub OIDC to assume roles.
* **Compatible with GitHub Cloud & Self-hosted Runners** for maximum flexibility. 
* **Beautiful Job Summaries** don't clutter up pull requests with noisy GitHub comments
* **100% Open Source with Permissive APACHE2 License** means you have no expensive subscriptions or long-term commitments.


## Screenshots

The following screenshot showcases a successful "plan" Job Summary report. The report effectively utilizes badges to clearly indicate success or failure. Furthermore, it specifically highlights any potentially destructive operations, mitigating the risk of unintentional destructive actions. A `diff`-style format is employed to illustrate the creation, recreation, destruction, or modification of resources. Unnecessary details are neatly hidden behind a collapsible `<details/>` block, ensuring a streamlined view. Additionally, developers are provided with a direct link to access the job run, eliminating the need for manual searching to gather information about any potential issues.

![Example Image](/img/github-actions/create.png)

By expanding the "Terraform Plan Summary" block (clicking on the `<details/>` block), the full details of the plan are visible.

![Example Create Extended](/img/github-actions/create-extended.png)

Furthermore, when a resource is marked for deletion, the Plan Summary will include a warning admonition.

![Example Destroy](/img/github-actions/destroy.png)

## Usage

In this example, the action is triggered when certain events occur, such as a manual workflow dispatch or the opening, synchronization, or reopening of a pull request, specifically on the main branch. It specifies specific permissions related to assuming roles in AWS. Within the "plan" job, the "component" and "stack" are hardcoded (`foobar` and `plat-ue2-sandbox`). In practice, these are usually derived from another action. 

:::tip Passing Affected Stacks

We recommend combining this action with the [`affected-stacks`](/integrations/github-actions/affected-stacks) GitHub Action inside a matrix to plan all affected stacks in parallel.

:::


```yaml
  # .github/workflows/atmos-terraform-plan.yaml
  name: "atmos-terraform-plan"
  on:
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
            atmos-gitops-config-path: ./.github/config/atmos-gitops.yaml
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

This GitHub Action depends on a few resources:
* **S3 bucket** for storing planfiles
* **DynamoDB table** for retrieving metadata about planfiles
* **2x IAM roles** for "planning" and accessing the "state" bucket

### S3 Bucket

This action can use any S3 Bucket to keep track of your planfiles. Just ensure the bucket is properly locked down since planfiles may contain secrets.

For example, [vendor in](/core-concepts/components/vendoring) the [`s3-component`](https://docs.cloudposse.com/components/library/aws/s3-bucket/), then using an [Atmos stack configuration](/core-concepts/stacks/), define a bucket using the [`s3-bucket` component](https://github.com/cloudposse/terraform-aws-components/tree/main/modules/s3-bucket) with this catalog configuration:

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

### DynamoDB Table

Similarly, a simple DynamoDB table can be provisioned using our [`dynamodb` component](https://docs.cloudposse.com/components/library/aws/dynamodb/). Set the **Hash Key** and create a **Global Secondary Index** as follows:

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
        # This key (case-sensitive) is required for the cloudposse/github-action-terraform-plan-storage action
        hash_key: id
        range_key: ""
        # Only these 2 attributes are required for creating the GSI, 
        # but there will be several other attributes on the table itself
        dynamodb_attributes:
          - name: 'createdAt'
            type: 'S'
          - name: 'pr'
            type: 'N'
        # This GSI is used to Query the latest plan file for a given PR.
        global_secondary_index_map:
          - name: pr-createdAt-index
            hash_key: pr
            range_key: createdAt
            projection_type: ALL
            non_key_attributes: []
            read_capacity: null
            write_capacity: null
        # Auto delete old entries
        ttl_enabled: true
        ttl_attribute: ttl
```

Pass the ARN of this table as the input to the `terraform-plan-table` of the [`cloudposse/github-action-atmos-terraform-plan`](https://github.com/cloudposse/github-action-atmos-terraform-plan) GitHub Action.

### IAM Access Roles

First create an access role for storing and retrieving planfiles from the S3 Bucket and DynamoDB table. We deploy this role using the [`gitops` component](https://docs.cloudposse.com/components/library/aws/gitops/). Assign this role ARN to the `terraform-state-role` input.

Next, create a role for GitHub workflows to use to plan and apply Terraform. We typically create an "AWS Team" with our [`aws-teams` component](https://docs.cloudposse.com/components/library/aws/aws-teams/), and then allow this team to assume `terraform` in the delegated accounts with our [`aws-team-roles` component](https://docs.cloudposse.com/components/library/aws/aws-team-roles/). Assign this role ARN to the `terraform-plan-role` input
