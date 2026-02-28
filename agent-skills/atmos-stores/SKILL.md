---
name: atmos-stores
description: "Store backends: AWS SSM, Azure Key Vault, Google Secret Manager, Redis, Artifactory configuration, hooks integration, cross-component data sharing"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos External Stores

Stores are external key-value backends configured in `atmos.yaml` that enable components to share data outside of Terraform state. Atmos supports five store providers: AWS SSM Parameter Store, Azure Key Vault, Google Secret Manager, Redis, and JFrog Artifactory.

## When to Use Stores

Use stores when you need to:
- Share data between components that is not managed by Terraform
- Access configuration from external systems (SSM, Vault, Redis)
- Integrate with CI/CD pipelines that write to parameter stores
- Store and retrieve Terraform outputs via hooks for faster cross-component reads
- Share state across accounts, regions, or cloud providers

For Terraform-managed outputs, prefer `!terraform.state` (fastest) or `!terraform.output`. Use stores for external data or when you need a write-back mechanism via hooks.

## Configuring Stores in atmos.yaml

All stores are declared under the top-level `stores:` key in `atmos.yaml`. Each store has a unique name, a `type`, and provider-specific `options`:

```yaml
# atmos.yaml
stores:
  prod/ssm:
    type: aws-ssm-parameter-store
    options:
      region: us-east-1

  prod/azure:
    type: azure-key-vault
    options:
      vault_url: "https://my-keyvault.vault.azure.net/"

  prod/gcp:
    type: google-secret-manager
    options:
      project_id: my-project

  cache:
    type: redis
    options:
      url: "redis://localhost:6379"

  artifacts:
    type: artifactory
    options:
      url: https://artifactory.example.com
      repo_name: my-repo
```

### Store Naming Convention

Store names follow the pattern `<environment>/<type>` by convention:
- `prod/ssm` -- Production SSM Parameter Store
- `dev/secrets` -- Development secrets
- `shared/config` -- Shared configuration store

These names are referenced in `!store` function calls and hook configurations.

### Common Options (All Providers)

All store providers support these optional fields:
- **`prefix`** -- String prepended to all keys (scopes the store namespace)
- **`stack_delimiter`** -- Character used to split stack names into key path segments (defaults vary by provider)

### Identity-Based Authentication

Stores that support identity-based authentication accept an `identity` field at the store level (not inside `options`). This connects the store to an Atmos auth identity for credential resolution:

```yaml
stores:
  prod/ssm:
    type: aws-ssm-parameter-store
    identity: prod-aws  # References an identity defined in the auth section
    options:
      region: us-east-1
```

Identity-based auth is supported by AWS SSM, Azure Key Vault, and Google Secret Manager. It is not supported by Redis or Artifactory (a warning is logged if configured).

## Store Provider Configuration

### AWS SSM Parameter Store

```yaml
stores:
  prod/ssm:
    type: aws-ssm-parameter-store
    options:
      region: us-east-1           # Required
      prefix: myapp               # Optional: prepended to all key paths
      stack_delimiter: "/"         # Optional: default is "-"
      read_role_arn: arn:aws:iam::123456789012:role/SSMReader   # Optional: cross-account read
      write_role_arn: arn:aws:iam::123456789012:role/SSMWriter  # Optional: cross-account write
```

Authentication uses the AWS default credential chain (environment variables, shared credentials, instance profile). Use `read_role_arn`/`write_role_arn` for cross-account access via STS AssumeRole.

Key format: `/<prefix>/<stack-parts>/<component-parts>/<key>` (segments joined by `/`).

### Azure Key Vault

```yaml
stores:
  prod/azure:
    type: azure-key-vault
    options:
      vault_url: "https://my-keyvault.vault.azure.net/"  # Required
      prefix: myapp               # Optional
      stack_delimiter: "-"         # Optional: default is "-"
```

Authentication uses the Azure Default Credential chain (environment variables, managed identity, Azure CLI). Secret names are normalized to comply with Azure Key Vault restrictions: only alphanumeric characters and hyphens are allowed.

Key format: `<prefix>-<stack-parts>-<component-parts>-<key>` (segments joined by `-`, non-alphanumeric characters replaced with `-`).

### Google Secret Manager

```yaml
stores:
  prod/gcp:
    type: google-secret-manager  # Also accepts "gsm"
    options:
      project_id: my-project     # Required
      prefix: myapp              # Optional
      stack_delimiter: "_"       # Optional: default is "-"
      credentials: '{"type":"service_account",...}'  # Optional: inline JSON credentials
      locations:                 # Optional: replication locations
        - us-east1
        - us-west1
```

Authentication uses the GCP default credential chain or the `GOOGLE_APPLICATION_CREDENTIALS` environment variable. Provide `credentials` inline for service account JSON. If `locations` is omitted, automatic replication is used.

Key format: `<prefix>_<stack-parts>_<component-parts>_<key>` (segments joined by `_`, slashes replaced with `_`).

### Redis

```yaml
stores:
  cache:
    type: redis
    options:
      url: "redis://localhost:6379"  # Required (or set ATMOS_REDIS_URL env var)
      prefix: myapp                  # Optional
      stack_delimiter: "/"           # Optional: default is "/"
```

The `url` supports Redis URL format including authentication: `redis://:password@host:port/db`. If `url` is not set, the `ATMOS_REDIS_URL` environment variable is used.

Key format: `<prefix>/<stack-parts>/<component-parts>/<key>` (segments joined by `/`). For `!store.get`, prefix is joined with `:` separator.

### Artifactory

```yaml
stores:
  artifacts:
    type: artifactory
    options:
      url: https://artifactory.example.com   # Required
      repo_name: my-repo                      # Required
      access_token: !env ARTIFACTORY_ACCESS_TOKEN  # Optional (see auth below)
      prefix: myapp                           # Optional
      stack_delimiter: "/"                    # Optional: default is "/"
```

Authentication uses `access_token` from options, or falls back to `ARTIFACTORY_ACCESS_TOKEN` or `JFROG_ACCESS_TOKEN` environment variables. Set token to `"anonymous"` for unauthenticated access.

Create a **Generic** repository type in JFrog Artifactory. Atmos stores data as JSON files, so no specific package type is required.

Key format: `<repo_name>/<prefix>/<stack-parts>/<component-parts>/<key>` (segments joined by `/`).

## Reading from Stores with YAML Functions

### `!store` -- Component-Aware Access

Reads values following the Atmos stack/component/key naming convention. The store constructs the full key path from the stack name, component name, and key:

```yaml
vars:
  # Two-parameter form: store + component + key (current stack implied)
  vpc_id: !store prod/ssm vpc vpc_id

  # Three-parameter form: store + stack + component + key
  vpc_id: !store prod/ssm plat-ue2-prod vpc vpc_id

  # Dynamic stack reference using Go templates
  vpc_id: !store prod/ssm {{ .stack }} vpc vpc_id

  # With default value for cold-start scenarios
  api_key: !store prod/ssm config api_key | default "not-set"

  # With YQ query to extract nested data
  db_host: !store prod/ssm database config | query .host

  # Extract from list
  first_subnet: !store prod/ssm vpc subnet_ids | query .[0]
```

Dynamic stack construction using `printf`:

```yaml
vars:
  # Cross-tenant reference
  vpc_id: !store prod/ssm {{ printf "net-%s-%s" .vars.environment .vars.stage }} vpc vpc_id

  # Full context-based stack name
  config: !store prod/ssm {{ printf "%s-%s-%s" .vars.tenant .vars.environment .vars.stage }} config settings
```

### `!store.get` -- Arbitrary Key Access

Reads arbitrary keys directly from a store without the stack/component/key convention. Use this for values written by external systems or global configuration:

```yaml
vars:
  # Direct key access
  db_password: !store.get prod/ssm /myapp/prod/db/password

  # With default value
  feature_flag: !store.get prod/ssm /features/new-feature \| default "disabled"

  # With YQ query
  api_key: !store.get cache app-config | query .api.key

  # Dynamic key with templates
  config: !store.get cache "config-{{ .vars.region }}"
```

Key differences between `!store` and `!store.get`:

| Feature | `!store` | `!store.get` |
|---------|----------|--------------|
| Key construction | Builds from stack/component/key | Uses exact key as provided |
| Use case | Atmos-managed component outputs | External systems, global config |
| Typical pattern | `prefix/stack/component/key` | Any format the store supports |

### `atmos.Store` -- Go Template Access

Read from stores within Go template expressions:

```yaml
vars:
  vpc_id: '{{ atmos.Store "prod/ssm" .stack "vpc" "vpc_id" }}'
  config: !template '{{ (atmos.Store "redis" .stack "config" "config_map").defaults | toJSON }}'
```

## Writing to Stores with Hooks

Hooks write Terraform outputs to stores after `atmos terraform apply` or `atmos terraform deploy`. Configure hooks at any level (global, terraform-level, component-level) and Atmos deep-merges them:

```yaml
# Full hook definition on a component
components:
  terraform:
    vpc:
      hooks:
        store-outputs:
          events:
            - after-terraform-apply
          command: store
          name: prod/ssm
          outputs:
            vpc_id: .vpc_id
            private_subnet_ids: .private_subnet_ids
            public_subnet_ids: .public_subnet_ids
```

Output values starting with `.` reference Terraform output names. The hook retrieves these from the Terraform state and writes them to the configured store.

### DRY Hook Configuration (Layered)

Split hook configuration across inheritance levels to avoid repetition:

```yaml
# stacks/catalog/vpc/_defaults.yaml -- global level
hooks:
  store-outputs:
    events:
      - after-terraform-apply
    command: store

# stacks/orgs/acme/plat/prod/_defaults.yaml -- account level
terraform:
  hooks:
    store-outputs:
      name: prod/ssm

# stacks/orgs/acme/plat/prod/us-east-2.yaml -- component level
components:
  terraform:
    vpc:
      hooks:
        store-outputs:
          outputs:
            vpc_id: .vpc_id
```

Atmos merges these into a complete hook definition at resolution time.

## Cross-Account and Cross-Region Access

### AWS Cross-Account via Role Assumption

```yaml
stores:
  prod/ssm:
    type: aws-ssm-parameter-store
    options:
      region: us-east-1
      read_role_arn: arn:aws:iam::123456789012:role/SSMReader
      write_role_arn: arn:aws:iam::123456789012:role/SSMWriter
```

Atmos uses STS AssumeRole to obtain temporary credentials for the target account. Separate read and write roles allow least-privilege access.

### Multi-Region Configuration

Define separate stores per region:

```yaml
stores:
  prod-us/ssm:
    type: aws-ssm-parameter-store
    options:
      region: us-east-1

  prod-eu/ssm:
    type: aws-ssm-parameter-store
    options:
      region: eu-west-1
```

Reference the appropriate store in each stack's configuration.

## End-to-End Example: VPC to EKS

1. Configure the store in `atmos.yaml`:

```yaml
stores:
  prod/ssm:
    type: aws-ssm-parameter-store
    options:
      region: us-east-1
```

2. Set up hooks on VPC to write outputs after apply:

```yaml
# stacks/catalog/vpc/_defaults.yaml
hooks:
  store-outputs:
    events:
      - after-terraform-apply
    command: store
    name: prod/ssm
    outputs:
      vpc_id: .vpc_id
      private_subnet_ids: .private_subnet_ids
```

3. Read stored values in EKS component:

```yaml
# stacks/prod/us-east-1.yaml
components:
  terraform:
    eks:
      vars:
        vpc_id: !store prod/ssm vpc vpc_id
        subnet_ids: !store prod/ssm vpc private_subnet_ids
```

## Security Best Practices

- **Secrets exposure**: `!store` values appear in stdout when running `atmos describe stacks` or `atmos describe component`. Avoid storing highly sensitive secrets in stores that are frequently described.
- **Least privilege**: Use `read_role_arn`/`write_role_arn` to separate read and write permissions. Grant only the permissions each operation needs.
- **Environment variables for tokens**: Never hardcode access tokens. Use `!env` or environment variables (`ARTIFACTORY_ACCESS_TOKEN`, `JFROG_ACCESS_TOKEN`, `ATMOS_REDIS_URL`).
- **Cold-start handling**: Always provide `| default` values for store lookups that may reference unprovisioned components.
- **DR implications**: Be cautious with cross-region store references. If a region goes down, stores in that region become unavailable.
- **Permission scoping**: When using `atmos describe affected` with `!store` references, Atmos needs read access to all referenced stores. Limited permissions (e.g., dev-only) will cause failures when referencing production stores.

## Troubleshooting

| Problem | Cause | Solution |
|---------|-------|----------|
| `store type not found` | Invalid `type` in store config | Use one of: `aws-ssm-parameter-store`, `azure-key-vault`, `google-secret-manager`, `gsm`, `redis`, `artifactory` |
| `region is required` | Missing `region` for SSM store | Add `region` to store options |
| `vault_url is required` | Missing `vault_url` for Azure | Add `vault_url` to store options |
| `project_id is required` | Missing `project_id` for GCP | Add `project_id` to store options |
| `failed to parse redis url` | Invalid Redis URL format | Use format `redis://:password@host:port/db` |
| `access_token must be set` | Missing Artifactory token | Set `access_token` in options or `ARTIFACTORY_ACCESS_TOKEN` env var |
| Key not found errors | Component not yet provisioned | Add `\| default` fallback value to the `!store` call |
| Permission denied | Insufficient IAM/RBAC permissions | Check role ARNs, vault policies, or service account permissions |
| Identity warning logged | Identity set on unsupported provider | Remove `identity` from Redis and Artifactory stores |

## Reference

For detailed provider configuration, authentication patterns, and advanced hook integration, see [references/store-providers.md](references/store-providers.md).
