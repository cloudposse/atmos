# Store Providers Reference

Detailed configuration, authentication, key construction, and integration patterns for all five Atmos store providers.

## Store Interface

All store providers implement the `Store` interface with three methods:
- **`Set(stack, component, key, value)`** -- Write a value scoped to a stack and component.
- **`Get(stack, component, key)`** -- Read a value scoped to a stack and component. Used by `!store`.
- **`GetKey(key)`** -- Read a value by arbitrary key path. Used by `!store.get`.

Values are JSON-serialized on write and JSON-deserialized on read. If the stored value is not valid JSON, it is returned as a raw string.

## Store Configuration Schema

Each store entry in `atmos.yaml` follows this schema:

```yaml
stores:
  <store-name>:
    type: <provider-type>       # Required: one of the five provider types
    identity: <identity-name>   # Optional: Atmos auth identity for credential resolution
    options:                    # Required: provider-specific options
      prefix: <string>         # Optional: key prefix for namespace isolation
      stack_delimiter: <string> # Optional: character to split stack names
      # ... provider-specific options
```

The `StoreConfig` struct in code:
```go
type StoreConfig struct {
    Type     string                 `yaml:"type"`
    Identity string                 `yaml:"identity,omitempty"`
    Options  map[string]interface{} `yaml:"options"`
}
```

## AWS SSM Parameter Store

### Type

`aws-ssm-parameter-store`

### Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `region` | string | Yes | -- | AWS region for the SSM endpoint |
| `prefix` | string | No | `""` | Prefix prepended to all parameter paths |
| `stack_delimiter` | string | No | `"-"` | Character used to split stack names into path segments |
| `read_role_arn` | string | No | -- | IAM role ARN to assume for read operations |
| `write_role_arn` | string | No | -- | IAM role ARN to assume for write operations |

### Authentication

1. **Default credential chain** -- AWS SDK default: environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_SESSION_TOKEN`), shared credentials file, EC2 instance profile, ECS task role.
2. **Identity-based** -- Set `identity` at the store level to use Atmos auth identity resolution. Supports profile, credentials file, config file, and region from auth context.
3. **Cross-account role assumption** -- Set `read_role_arn` and/or `write_role_arn`. Atmos calls STS AssumeRole with session name `atmos-ssm-session` and creates a new SSM client with the temporary credentials.

### Key Construction

For `Get`/`Set` (stack/component/key pattern):
```text
/<prefix>/<stack-part-1>/<stack-part-2>/.../<component-parts>/<key>
```

Stack name is split by `stack_delimiter` (default `-`). Component name is split by `/`. All parts are joined with `/`. Double slashes are collapsed.

Example: prefix=`myapp`, stack=`plat-ue2-prod`, component=`vpc`, key=`vpc_id`:
```text
/myapp/plat/ue2/prod/vpc/vpc_id
```

For `GetKey` (arbitrary key):
- If prefix is set: `/<prefix>/<key>`
- If no prefix: `/<key>`
- A leading `/` is always ensured for SSM compatibility.

### Parameters

SSM parameters are stored as `String` type with `Overwrite: true`. Values are JSON-serialized before storage.

### Complete Example

```yaml
stores:
  # Production read-write store
  prod/ssm:
    type: aws-ssm-parameter-store
    options:
      region: us-east-1
      prefix: atmos
      read_role_arn: arn:aws:iam::111111111111:role/AtmosSSMReader
      write_role_arn: arn:aws:iam::111111111111:role/AtmosSSMWriter

  # Development store (same account, no role assumption needed)
  dev/ssm:
    type: aws-ssm-parameter-store
    options:
      region: us-west-2
      prefix: atmos

  # Identity-based store
  shared/ssm:
    type: aws-ssm-parameter-store
    identity: shared-aws
    options:
      region: us-east-1
```

---

## Azure Key Vault

### Type

`azure-key-vault`

### Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `vault_url` | string | Yes | -- | Full URL of the Azure Key Vault (e.g., `https://my-vault.vault.azure.net/`) |
| `prefix` | string | No | `""` | Prefix prepended to secret names |
| `stack_delimiter` | string | No | `"-"` | Character used to split stack names |

### Authentication

1. **Default credential chain** -- Azure SDK `DefaultAzureCredential`: environment variables, managed identity, Azure CLI, Azure PowerShell, and other sources.
2. **Identity-based** -- Set `identity` to use Atmos auth identity resolution. Supports `TenantID` hint for multi-tenant scenarios.

### Key Construction and Normalization

Azure Key Vault secret names must match `^[0-9a-zA-Z-]+$`. Atmos normalizes keys:
1. Build base key from `<prefix>-<stack-parts>-<component-parts>-<key>` (joined by `-`).
2. Replace all non-alphanumeric characters (except `-`) with `-`.
3. Collapse consecutive hyphens into a single `-`.
4. Trim leading and trailing `-`.
5. If the result is empty, use `"default"`.

Example: prefix=`myapp`, stack=`plat-ue2-prod`, component=`vpc/network`, key=`vpc_id`:
```text
myapp-plat-ue2-prod-vpc-network-vpc_id -> myapp-plat-ue2-prod-vpc-network-vpc-id
```

For `GetKey`, the same normalization is applied to the raw key.

### Error Handling

- HTTP 404 -> `ErrResourceNotFound`
- HTTP 403 -> `ErrPermissionDenied`

### Complete Example

```yaml
stores:
  prod/azure:
    type: azure-key-vault
    options:
      vault_url: "https://prod-infra-vault.vault.azure.net/"
      prefix: atmos

  # With identity-based auth
  shared/azure:
    type: azure-key-vault
    identity: azure-shared
    options:
      vault_url: "https://shared-vault.vault.azure.net/"
```

---

## Google Secret Manager

### Type

`google-secret-manager` or `gsm`

### Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `project_id` | string | Yes | -- | GCP project ID |
| `prefix` | string | No | `""` | Prefix prepended to secret names |
| `stack_delimiter` | string | No | `"-"` | Character used to split stack names |
| `credentials` | string | No | -- | Inline JSON service account credentials |
| `locations` | list of strings | No | -- | Replication locations (omit for automatic replication) |

### Authentication

1. **Default credential chain** -- GCP Application Default Credentials (`GOOGLE_APPLICATION_CREDENTIALS` env var, `gcloud auth application-default login`, metadata server).
2. **Inline credentials** -- Set `credentials` option with a JSON service account key string.
3. **Identity-based** -- Set `identity` to use Atmos auth identity resolution. Uses credentials file from GCP auth context if available, otherwise falls back to store credentials.

### Key Construction

For `Get`/`Set`:
1. Build base key from `<prefix>_<stack-parts>_<component-parts>_<key>` (joined by `_`).
2. Replace all `/` with `_`.
3. Collapse consecutive `_` into a single `_`.
4. Trim leading and trailing `_`.

Example: prefix=`myapp`, stack=`plat-ue2-prod`, component=`vpc`, key=`vpc_id`:
```text
myapp_plat_ue2_prod_vpc_vpc_id
```

For `GetKey`:
- If prefix is set: `<prefix>_<key>`
- Access path: `projects/<project_id>/secrets/<key>/versions/latest`

### Replication

- **No locations** -> Automatic replication (Google manages replicas).
- **Locations specified** -> User-managed replication to the listed regions.

### Secret Versioning

On `Set`, Atmos creates the secret if it does not exist (ignores `AlreadyExists` errors), then adds a new version with the JSON-serialized value. On `Get`, Atmos always reads the `latest` version.

### Operation Timeout

All GCP operations have a 30-second context timeout.

### Complete Example

```yaml
stores:
  prod/gcp:
    type: google-secret-manager
    options:
      project_id: my-prod-project
      prefix: atmos
      locations:
        - us-east1
        - us-west1

  # Using short alias
  dev/gcp:
    type: gsm
    options:
      project_id: my-dev-project
```

---

## Redis

### Type

`redis`

### Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `url` | string | Conditional | -- | Redis URL (required if `ATMOS_REDIS_URL` not set) |
| `prefix` | string | No | `""` | Prefix prepended to keys |
| `stack_delimiter` | string | No | `"/"` | Character used to split stack names |

### Authentication

The `url` field supports standard Redis URL format with optional authentication:
- `redis://localhost:6379` -- No auth
- `redis://:password@host:6379` -- Password only
- `redis://user:password@host:6379/0` -- User, password, and database

If `url` is not set in options, Atmos reads the `ATMOS_REDIS_URL` environment variable.

Identity-based authentication is **not supported** for Redis stores. If `identity` is set, a warning is logged and the field is ignored.

### Key Construction

For `Get`/`Set`:
```text
<prefix>/<stack-parts>/<component-parts>/<key>
```

Stack name is split by `stack_delimiter` (default `/`). All parts joined with `/`.

For `GetKey`:
- If prefix is set: `<prefix>:<key>` (note the `:` separator, standard Redis key namespace convention)
- If no prefix: `<key>`

### Storage

Values are stored with no expiration (`TTL: 0`), meaning they persist until explicitly deleted or the Redis instance is flushed.

### Complete Example

```yaml
stores:
  cache:
    type: redis
    options:
      url: "redis://:mypassword@redis.internal:6379/0"
      prefix: atmos
      stack_delimiter: "/"
```

---

## Artifactory

### Type

`artifactory`

### Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `url` | string | Yes | -- | Artifactory server URL |
| `repo_name` | string | Yes | -- | Repository name (must be a Generic repository type) |
| `access_token` | string | Conditional | -- | Access token (required if env vars not set) |
| `prefix` | string | No | `""` | Path prefix within the repository |
| `stack_delimiter` | string | No | `"/"` | Character used to split stack names |

### Authentication

Token resolution order:
1. `access_token` in options (can use `!env ARTIFACTORY_ACCESS_TOKEN`)
2. `ARTIFACTORY_ACCESS_TOKEN` environment variable
3. `JFROG_ACCESS_TOKEN` environment variable

Set `access_token` to `"anonymous"` for unauthenticated access to public repositories.

Identity-based authentication is **not supported** for Artifactory stores. If `identity` is set, a warning is logged and the field is ignored.

### Repository Setup

Create a **Generic** repository type in JFrog Artifactory. Atmos stores data as JSON files, so no specific package type (Maven, npm, Docker, etc.) is required. The repository can be local, remote, or virtual.

### Key Construction

For `Get`/`Set`:
```text
<repo_name>/<prefix>/<stack-parts>/<component-parts>/<key>
```

For `GetKey`:
- If prefix is set: `<repo_name>/<prefix>/<key>.json`
- If no prefix: `<repo_name>/<key>.json`
- A `.json` extension is automatically appended if not present.

### Storage Mechanism

Data is stored as JSON files in Artifactory. On `Set`, Atmos creates a temporary file with JSON content and uploads it. On `Get`, Atmos downloads the file to a temporary directory, reads it, and deserializes the JSON.

### SDK Logging

Artifactory SDK logging is automatically configured based on the Atmos log level:
- Debug/Trace mode: SDK DEBUG logs are enabled.
- All other levels: SDK logging is fully suppressed.

### Connection Settings

- Dial timeout: 180 seconds
- Overall request timeout: 60 seconds
- HTTP retries: 0 (fail fast)

### Complete Example

```yaml
stores:
  artifacts:
    type: artifactory
    options:
      url: https://artifactory.example.com
      repo_name: infra-state
      access_token: !env ARTIFACTORY_ACCESS_TOKEN
      prefix: atmos
```

---

## Hook Integration Patterns

### Basic Store Hook

Write specific Terraform outputs to a store after apply:

```yaml
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
```

### Hook Schema

| Field | Description |
|-------|-------------|
| `hooks.<name>.events` | List of lifecycle events (currently: `after-terraform-apply`) |
| `hooks.<name>.command` | Must be `store` |
| `hooks.<name>.name` | Store name (must match a store in `atmos.yaml`) |
| `hooks.<name>.outputs` | Map of store keys to values. Values starting with `.` are Terraform output names |

### Multi-Store Hooks

Write outputs to multiple stores simultaneously:

```yaml
components:
  terraform:
    vpc:
      hooks:
        store-ssm:
          events:
            - after-terraform-apply
          command: store
          name: prod/ssm
          outputs:
            vpc_id: .vpc_id

        store-redis:
          events:
            - after-terraform-apply
          command: store
          name: cache
          outputs:
            vpc_id: .vpc_id
```

### Layered Hook Configuration

Split hook definitions across inheritance levels for DRY configuration:

**Global level** -- Define the event and command:
```yaml
# stacks/catalog/vpc/_defaults.yaml
hooks:
  store-outputs:
    events:
      - after-terraform-apply
    command: store
```

**Account level** -- Set the store name:
```yaml
# stacks/orgs/acme/plat/prod/_defaults.yaml
terraform:
  hooks:
    store-outputs:
      name: prod/ssm
```

**Component level** -- Define the outputs:
```yaml
# stacks/orgs/acme/plat/prod/us-east-2.yaml
components:
  terraform:
    vpc:
      hooks:
        store-outputs:
          outputs:
            vpc_id: .vpc_id
            subnet_ids: .private_subnet_ids
```

Atmos deep-merges all levels into the final hook configuration.

---

## Troubleshooting

### Common Errors

**`store type not found: <type>`**
The `type` field does not match any registered provider. Valid types: `aws-ssm-parameter-store`, `azure-key-vault`, `google-secret-manager`, `gsm`, `redis`, `artifactory`.

**`region is required in ssm store configuration`**
The `region` option is missing from an AWS SSM store. This is a required field.

**`vault_url is required in azure key vault store configuration`**
The `vault_url` option is missing from an Azure Key Vault store.

**`project_id is required in Google Secret Manager store configuration`**
The `project_id` option is missing from a Google Secret Manager store.

**`failed to parse redis url`**
The Redis URL format is invalid. Use standard Redis URL format: `redis://:password@host:port/db`.

**`either url must be set in options or ATMOS_REDIS_URL environment variable must be set`**
Neither the `url` option nor the `ATMOS_REDIS_URL` env var is available.

**`either access_token must be set in options or one of JFROG_ACCESS_TOKEN or ARTIFACTORY_ACCESS_TOKEN environment variables must be set`**
No Artifactory authentication is configured. Provide a token via options or environment variables.

**`resource not found`**
The key/secret does not exist in the store. This typically happens during cold-start when a component has not been provisioned yet. Use `| default <value>` in `!store` calls to handle this gracefully.

**`permission denied`**
The current credentials do not have access to the requested key/secret. Check IAM policies, Key Vault access policies, or GCP IAM bindings.

**`store identity is configured but auth resolver is not set`**
Identity-based auth is configured but the auth resolver has not been injected. This typically indicates a configuration ordering issue.

**`auth context not available for identity`**
The named identity could not be resolved. Verify the identity name matches an entry in the auth configuration.

### Performance Tips

- Store calls add network latency. Use `!terraform.state` for Terraform outputs when possible.
- All YAML functions and template functions cache results per execution -- repeated references to the same store key only make one network call.
- `!store.get` (direct key access) is slightly faster than `!store` (pattern-based) since no key construction is needed.
- For high-frequency reads, consider Redis as the store backend for lowest latency.
