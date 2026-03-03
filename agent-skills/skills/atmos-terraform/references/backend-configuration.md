# Atmos Terraform Backend Configuration Reference

This reference covers how Atmos configures Terraform backends through stack manifests, including all
supported backend types, configuration patterns, auto-generation, and workspace key management.

## How Backend Configuration Works

When you run any `atmos terraform` command:

1. Atmos reads `backend_type` and `backend` from the resolved stack configuration for the component.
2. Deep-merges settings from all inherited stack manifests (organization, account, environment, component).
3. Generates a `backend.tf.json` file in the component directory.
4. Terraform uses this file to configure state storage during `init`.

This keeps Terraform modules clean -- no hardcoded backend configuration in source code.

## Enabling Auto-Generation

In `atmos.yaml`:

```yaml
components:
  terraform:
    auto_generate_backend_file: true
```

Environment variable: `ATMOS_COMPONENTS_TERRAFORM_AUTO_GENERATE_BACKEND_FILE=true`

## Configuration Hierarchy

Backend settings can be defined at multiple levels, with more specific scopes overriding broader ones:

| Scope | Example File | Effect |
|-------|-------------|--------|
| Organization | `stacks/orgs/acme/_defaults.yaml` | All components inherit |
| Account/Stage | `stacks/orgs/acme/plat/prod/_defaults.yaml` | Override for prod |
| Component-type | Under `terraform:` in any stack | All Terraform components |
| Component | Under `components.terraform.<name>:` | Single component |

### Organization-Level Defaults

```yaml
# stacks/orgs/acme/_defaults.yaml
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: acme-ue1-root-tfstate
      region: us-east-1
      encrypt: true
      use_lockfile: true
```

### Environment-Level Override

```yaml
# stacks/orgs/acme/plat/prod/_defaults.yaml
terraform:
  backend:
    s3:
      bucket: acme-ue1-prod-tfstate
```

### Component-Level Override

```yaml
# stacks/orgs/acme/plat/prod/us-east-1.yaml
components:
  terraform:
    special-component:
      backend_type: s3
      backend:
        s3:
          bucket: acme-ue1-prod-special-tfstate
          key: "special/terraform.tfstate"
```

## S3 Backend (AWS)

The most common backend type for AWS-based infrastructure.

### Configuration

```yaml
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: acme-ue1-root-tfstate        # Required: S3 bucket name
      key: terraform.tfstate                # State file path within bucket
      region: us-east-1                     # Required: AWS region
      encrypt: true                         # Enable server-side encryption
      use_lockfile: true                    # Native S3 locking (Terraform 1.10+)
      acl: bucket-owner-full-control        # Bucket ACL
      workspace_key_prefix: terraform       # Prefix for workspace state paths
```

### Cross-Account Access

```yaml
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: acme-ue1-root-tfstate
      region: us-east-1
      encrypt: true
      use_lockfile: true
      role_arn: arn:aws:iam::999999999999:role/TerraformStateAdmin
      # Or use assume_role block:
      assume_role:
        role_arn: arn:aws:iam::999999999999:role/TerraformStateAdmin
```

### With DynamoDB Locking (Legacy)

For Terraform versions before 1.10 that do not support native S3 locking:

```yaml
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: acme-ue1-root-tfstate
      region: us-east-1
      encrypt: true
      dynamodb_table: acme-ue1-root-tfstate-lock
```

### Generated File Example

```json
{
  "terraform": {
    "backend": {
      "s3": {
        "bucket": "acme-ue1-root-tfstate",
        "region": "us-east-1",
        "encrypt": true,
        "use_lockfile": true,
        "key": "ue1/prod/vpc/terraform.tfstate",
        "workspace_key_prefix": "vpc"
      }
    }
  }
}
```

## GCS Backend (Google Cloud)

```yaml
terraform:
  backend_type: gcs
  backend:
    gcs:
      bucket: my-project-tfstate           # Required: GCS bucket name
      prefix: terraform/state              # Object name prefix
      project: my-gcp-project              # GCP project for the bucket
      location: US                         # Bucket location
      credentials: /path/to/creds.json     # Service account key file (optional)
      encryption_key: base64-encoded-key   # Customer-supplied encryption key (optional)
```

### Generated File Example

```json
{
  "terraform": {
    "backend": {
      "gcs": {
        "bucket": "my-project-tfstate",
        "prefix": "terraform/state"
      }
    }
  }
}
```

## Azure Blob Storage Backend

```yaml
terraform:
  backend_type: azurerm
  backend:
    azurerm:
      resource_group_name: terraform-state-rg      # Required
      storage_account_name: acmetfstate             # Required
      container_name: tfstate                       # Required
      key: terraform.tfstate                        # State blob name
      subscription_id: 00000000-0000-0000-0000-000000000000
      tenant_id: 00000000-0000-0000-0000-000000000000
      use_oidc: true                                # Use OIDC for auth
```

### Generated File Example

```json
{
  "terraform": {
    "backend": {
      "azurerm": {
        "resource_group_name": "terraform-state-rg",
        "storage_account_name": "acmetfstate",
        "container_name": "tfstate",
        "key": "terraform.tfstate"
      }
    }
  }
}
```

## Remote Backend (Terraform Cloud / Enterprise)

```yaml
terraform:
  backend_type: remote
  backend:
    remote:
      hostname: app.terraform.io
      organization: my-org
      workspaces:
        name: my-workspace
        # Or use prefix for multiple workspaces:
        # prefix: my-app-
```

When using `remote` backend, use `--skip-planfile` with plan since Terraform Cloud does not support
local planfiles.

## Other Backends

Atmos supports any Terraform backend type. Set `backend_type` to the backend name and provide
configuration under `backend.<type>`:

### Consul

```yaml
terraform:
  backend_type: consul
  backend:
    consul:
      address: consul.example.com:8500
      scheme: https
      path: terraform/state
```

### PostgreSQL

```yaml
terraform:
  backend_type: pg
  backend:
    pg:
      conn_str: postgres://user:pass@host/dbname
      schema_name: terraform
```

### HTTP

```yaml
terraform:
  backend_type: http
  backend:
    http:
      address: https://state.example.com/terraform
      lock_address: https://state.example.com/terraform/lock
      unlock_address: https://state.example.com/terraform/unlock
```

## Workspace Key Prefix

The `workspace_key_prefix` controls how state paths are organized within the backend. It is part
of the state key path: `<workspace_key_prefix>/<workspace>/terraform.tfstate`.

### Static Configuration

```yaml
components:
  terraform:
    vpc:
      backend:
        s3:
          workspace_key_prefix: vpc
```

### Using metadata.name for Stable Keys

The recommended approach uses `metadata.name` to ensure stable keys even when component
implementations change:

```yaml
components:
  terraform:
    vpc/defaults:
      metadata:
        type: abstract
        name: vpc              # Stable identity for workspace key
        component: vpc/v2      # Implementation can change without affecting state
```

### Dynamic Workspace Key Prefix with Go Templates

For advanced use cases:

```yaml
components:
  terraform:
    vpc:
      backend:
        workspace_key_prefix: "{{.vars.namespace}}-{{.vars.environment}}-{{.vars.stage}}"
```

## Backend Provisioning

Atmos can automatically create backend storage to solve the Terraform bootstrap problem.

### Enable Provisioning

```yaml
components:
  terraform:
    vpc:
      provision:
        backend:
          enabled: true
```

Provisioning can be enabled at any level of the stack hierarchy (organization, environment, component).

### Provisioning Behavior

When `provision.backend.enabled: true`:

- **Automatic**: Backend is provisioned before `terraform init` on plan/apply/deploy
- **Manual**: Use `atmos terraform backend create <component> --stack <stack>`
- **Idempotent**: Safe to run multiple times; skips if backend already exists
- **Secure defaults**: S3 buckets are created with versioning, encryption, and public access blocked

### S3 Backend Provisioning Defaults

- Versioning: Enabled
- Encryption: AES-256 (AWS-managed keys)
- Public access: All four settings blocked
- Locking: Native S3 locking (Terraform 1.10+)
- Tags: `Name`, `ManagedBy=Atmos`

## Manual Generation Commands

Generate backend configuration without running terraform:

```shell
# Generate for one component
atmos terraform generate backend vpc -s plat-ue2-dev

# Generate for all components
atmos terraform generate backends
```

## Remote State Backend

For reading state from other components (cross-component references), configure
`remote_state_backend` separately from the write backend:

```yaml
components:
  terraform:
    vpc:
      remote_state_backend:
        s3:
          bucket: acme-ue1-root-tfstate
          region: us-east-1
          role_arn: arn:aws:iam::999999999999:role/TerraformStateReader
```

This is used by the `remote-state` Terraform module to read outputs from other components.

## Best Practices

1. **Define backend defaults at the organization level** and override per environment as needed.
2. **Use `auto_generate_backend_file: true`** to keep Terraform modules clean of backend config.
3. **Add `backend.tf.json` to `.gitignore`** since it is generated at runtime.
4. **Use `use_lockfile: true`** for S3 backends with Terraform 1.10+ instead of DynamoDB tables.
5. **Use `metadata.name`** for stable workspace key prefixes that survive component version changes.
6. **Enable backend provisioning** in development environments and pre-provision in production.
7. **Use cross-account roles** for centralized state storage with least-privilege access.
