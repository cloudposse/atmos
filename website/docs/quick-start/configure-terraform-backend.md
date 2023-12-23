---
title: Configure Terraform Backend
sidebar_position: 10
sidebar_label: Configure Terraform Backend
---

In the previous steps, we've configured the `vpc-flow-logs-bucket` and `vpc` Terraform components to be provisioned into three AWS accounts
(`dev`, `staging`, `prod`) in the two AWS regions (`us-east-2` and `us-west-2`).

If we provision the `vpc-flow-logs-bucket` and `vpc` components, by default, Terraform will use a backend
called [local](https://developer.hashicorp.com/terraform/language/settings/backends/local), which stores Terraform state on the local filesystem,
locks that state using system APIs, and performs operations locally. For production use, we'll provision and configure the
Terraform [s3](https://developer.hashicorp.com/terraform/language/settings/backends/s3) backend.

## Terraform Local Backend

Terraform's local backend is designed for development and testing purposes and is generally not recommended for production use. There are several
reasons why using the local backend in a production environment may not be suitable:

- **State Management**: The local backend stores the Terraform state file on the local file system. In a production environment, it's crucial to have
  a robust and scalable solution for managing the Terraform state. Storing state locally can lead to issues with collaboration, concurrency, and
  consistency.

- **Concurrency and Locking**: When multiple users or automation processes are working with Terraform concurrently, it's essential to ensure that only
  one process can modify the infrastructure at a time. The local backend lacks built-in support for locking mechanisms that prevent multiple Terraform
  instances from modifying the state simultaneously. This can lead to race conditions and conflicting changes.

- **Collaboration**: In a production environment with multiple team members, it's important to have a centralized and shared state. The local backend
  does not provide a way to easily share the state across different team members or systems. A remote backend, such as Amazon S3, Azure Storage, or
  HashiCorp Consul, is more suitable for collaboration.

- **Durability and Backup**: The local backend does not provide durability or backup features. If the machine where Terraform is run experiences
  issues, there's a risk of losing the state file, leading to potential data loss. Remote backends offer better durability and often provide features
  for versioning and backup.

- **Remote Execution and Automation**: In production, it's common to use Terraform in automated workflows, such as continuous integration/continuous
  deployment (CI/CD) pipelines. Remote backends are better suited for these scenarios, allowing for seamless integration with automation tools and
  supporting the deployment of infrastructure as code in a reliable and controlled manner.

To address these concerns, it's recommended to use one of the supported remote backends, such as Amazon S3, Azure Storage, Google Cloud Storage,
HashiCorp Consul, or Terraform Cloud, for production environments. Remote backends provide better scalability, collaboration support, and durability,
making them more suitable for managing infrastructure at scale in production environments.

## Terraform S3 Backend

Terraform's S3 backend is a popular remote backend for storing Terraform state files in an Amazon Simple Storage Service (S3) bucket. Using S3 as a
backend offers several advantages over local backends, particularly in production environments. Here's an overview of the key features and benefits of
using the Terraform S3 backend:

- **Remote State Storage**: The Terraform state file is stored remotely in an S3 bucket. This allows multiple users and Terraform instances to access
  and manage the same state file, promoting collaboration and consistency across deployments.

- **Concurrency and Locking**: S3 backend supports state file locking, which prevents multiple Terraform instances from modifying the state file
  simultaneously. This helps avoid conflicts and ensures that changes are applied in a coordinated manner, especially in multi-user or automated
  environments.

- **Durability and Versioning**: S3 provides high durability for object storage, and it automatically replicates data across multiple availability
  zones. Additionally, versioning can be enabled on the S3 bucket, allowing you to track changes to the state file over time. This enhances data
  integrity and provides a safety net in case of accidental changes or deletions.

- **Access Control and Security**: S3 supports fine-grained access control policies, allowing you to restrict access to the state file based on AWS
  Identity and Access Management (IAM) roles and policies. This helps ensure that only authorized users or processes can read or modify the Terraform
  state.

- **Integration with AWS Features**: The S3 backend integrates well with other AWS services. For example, you can use AWS Key Management Service (KMS)
  for server-side encryption of the state file, and you can leverage AWS Identity and Access Management (IAM) roles for secure access to the S3
  bucket.

- **Terraform Remote Operations**: The S3 backend can be used in conjunction with Terraform Remote Operations, allowing you to run Terraform
  commands remotely while keeping the state in S3. This is useful for scenarios where the Terraform client and the infrastructure being managed are
  separated.

To configure Terraform to use an S3 backend, you typically provide the S3 bucket name and an optional key prefix in your Terraform configuration.
Here's a simplified example:

```hcl
terraform {
  backend "s3" {
    acl            = "bucket-owner-full-control"
    bucket         = "your-s3-bucket-name"
    key            = "path/to/terraform.tfstate"
    region         = "your-aws-region"
    encrypt        = true
    dynamodb_table = "terraform_locks"
  }
}
```

In the example, `terraform_locks` is a DynamoDB table used for state locking. DynamoDB is recommended for locking when using the S3 backend to ensure
safe concurrent access.

## Provision Terraform S3 Backend

Before using Terraform S3 backend, a backend S3 bucket and DynamoDB table need to be provisioned.

You can provision them using the [tfstate-backend](https://github.com/cloudposse/terraform-aws-tfstate-backend) Terraform module and
[tfstate-backend](https://github.com/cloudposse/terraform-aws-components/tree/main/modules/tfstate-backend) Terraform component (root module).

Note that the [tfstate-backend](https://github.com/cloudposse/terraform-aws-components/tree/main/modules/tfstate-backend) Terraform component
can be added to the `components/terraform` folder, the configuration for the component can be added to the `stacks`, and the component itself
can be provisioned with Atmos.

Here's an example of an Atmos manifest to configure the `tfstate-backend` Terraform component:

```yaml title="stacks/catalog/tfstate-backend/defaults.yaml"
components:
  terraform:
    tfstate-backend:
      vars:
        enable_server_side_encryption: true
        enabled: true
        force_destroy: false
        name: tfstate
        prevent_unencrypted_uploads: true
        access_roles:
          default:
            write_enabled: true
            allowed_roles:
              core-identity: [ "devops", "developers", "managers", "spacelift" ]
              core-root: [ "admin" ]
            allowed_permission_sets:
              core-identity: [ "AdministratorAccess" ]
```

## Configure Terraform S3 Backend

Once the S3 bucket and DynamoDB table are provisioned, you can start using them to store Terraform state for the Terraform components.
There are two ways of doing this:

- Manually create `backend.tf` file in each component's folder with the following content:

  ```hcl
  terraform {
    backend "s3" {
      acl                  = "bucket-owner-full-control"
      bucket               = "your-s3-bucket-name"
      dynamodb_table       = "your-dynamodb-table-name"
      encrypt              = true
      key                  = "terraform.tfstate"
      region               = "your-aws-region"
      role_arn             = "arn:aws:iam::<your account ID>:role/<IAM Role with permissions to access the Terraform backend>"
      workspace_key_prefix = "<component name, e.g. `vpc` or `vpc-flow-logs-bucket`"
    }
  }
  ```

- Configure Atmos to automatically generate a terraform backend file for each Atmos component. This is the recommended way of configuring Terraform
  state backend since it offers many advantages and will save you from manually creating a backend configuration file for each component

### Configure Terraform S3 Backend with Atmos

Configuring Terraform S3 backend with Atmos consists of the three steps:

- Set `auto_generate_backend_file` to `true` in the `atmos.yaml` CLI config file in the `components.terraform` section:

  ```yaml
  components:
    terraform:
    # Can also be set using 'ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE' ENV var, or '--auto-generate-backend-file' command-line argument
    auto_generate_backend_file: true
  ```

  Refer to [Quick Start: Configure CLI](/quick-start/configure-cli) and [CLI Configurtion](/cli/configuration) for more details.

- Configure the S3 backend in one of the `_defaults.yaml` manifests. You can configure it for the entire Organization, or per OU/tenant, or per
  region, or per account.

  :::note
  The `_defaults.yaml` manifests contain the default settings for the Organization(s), Organizational Units and AWS accounts.
  :::

  To configure the S3 backend for the entire Organization, add the following config in `stacks/orgs/acme/_defaults.yaml`:

  ```yaml title="stacks/orgs/acme/_defaults.yaml"
  terraform:
    backend_type: s3
    backend:
      s3:
        acl: "bucket-owner-full-control"
        encrypt: true
        bucket: "your-s3-bucket-name"
        dynamodb_table: "your-dynamodb-table-name"
        key: "terraform.tfstate"
        region: "your-aws-region"
        role_arn: "arn:aws:iam::<your account ID>:role/<IAM Role with permissions to access the Terraform backend>"
  ```

<br/>

- (This step is optional) For each component, you can add `workspace_key_prefix` similar to the following:

  ```yaml title="stacks/catalog/vpc.yaml"
  components:
    terraform:
      # `vpc` is the Atmos component name
      vpc:
        # Optional backend configuration for the component
        backend:
          s3:
            workspace_key_prefix: vpc
        metadata:
          # Point to the Terraform component
          component: vpc
        settings: {}
        vars: {}
  ```

  Note that this is optional. If you don’t add `backend.s3.workspace_key_prefix` to the component manifest, the Atmos component name will be used
  automatically (which is this example is `vpc`). `/` (slash) in the Atmos component name will get replaced with `-` (dash).

  We usually don’t specify `workspace_key_prefix` for each component and let Atmos use the component name as `workspace_key_prefix`.
