# YAML Functions Complete Reference

YAML functions are the recommended way to add dynamic behavior to Atmos stack configurations.
They use YAML explicit tags (the `!` prefix) and execute after YAML parsing, making them
type-safe, predictable, and unable to break YAML syntax.

All YAML functions support Go template expressions in their arguments. Atmos processes
templates first, then executes the YAML functions.

## `!terraform.output`

Read Terraform outputs (remote state) by running `terraform output`.

### Syntax

```yaml
# Current stack
!terraform.output <component> <output>
!terraform.output <component> <yq-expression>

# Specific stack
!terraform.output <component> <stack> <output>
!terraform.output <component> <stack> <yq-expression>
```

### Examples

```yaml
vpc_id: !terraform.output vpc vpc_id
vpc_id: !terraform.output vpc plat-ue2-prod vpc_id
vpc_id: !terraform.output vpc {{ .stack }} vpc_id
first_subnet: !terraform.output vpc .private_subnet_ids[0]
username: !terraform.output config .config_map.username
# Default value for unprovisioned components
vpc_id: !terraform.output vpc ".vpc_id // ""fallback-id"""
# YQ string concatenation
url: !terraform.output aurora-postgres ".master_hostname | ""jdbc:postgresql://"" + . + "":5432"""
# Bracket notation for keys with special characters
key: !terraform.output security '.users["github-dependabot"].access_key_id'
```

### Notes

- Requires Terraform initialization (downloads providers) -- can be slow
- Results are cached per execution
- Prefer `!terraform.state` for supported backends (10-100x faster)

## `!terraform.state`

Read Terraform outputs directly from the state backend without initialization.

### Syntax

Same as `!terraform.output`:

```yaml
!terraform.state <component> <output>
!terraform.state <component> <stack> <output>
!terraform.state <component> <yq-expression>
!terraform.state <component> <stack> <yq-expression>
```

### Supported Backends

- `s3` (AWS)
- `local`
- `gcs` (Google Cloud Storage)
- `azurerm` (Azure)

### Examples

```yaml
vpc_id: !terraform.state vpc vpc_id
subnet_ids: !terraform.state vpc plat-ue2-prod private_subnet_ids
first_subnet: !terraform.state vpc .private_subnet_ids[0]
# Default value
vpc_id: !terraform.state vpc ".vpc_id // ""default"""
```

### Notes

- Dramatically faster than `!terraform.output` (no init, no provider download)
- Same caching behavior
- Supports YQ expressions and defaults

## `!store`

Read values from configured stores following the Atmos component/stack/key convention.

### Syntax

```yaml
# Current stack
!store <store_name> <component> <key>
!store <store_name> <component> <key> | default <default-value>
!store <store_name> <component> <key> | query <yq-expression>

# Specific stack
!store <store_name> <stack> <component> <key>
!store <store_name> <stack> <component> <key> | default <default-value>
!store <store_name> <stack> <component> <key> | query <yq-expression>
```

### Examples

```yaml
sg_id: !store prod/ssm security-group/lambda id
sg_id: !store prod/ssm plat-ue2-prod security-group/lambda id
sg_id: !store prod/ssm {{ .stack }} security-group/lambda id
kms_arn: !store prod/ssm kms config | query .arn
api_key: !store prod/ssm config api_key | default "not-set"
```

### Notes

- Constructs store keys from stack/component/key pattern
- Use `!store.get` for arbitrary keys

## `!store.get`

Retrieve arbitrary keys from stores without following the Atmos naming convention.

### Syntax

```yaml
!store.get <store_name> <key>
!store.get <store_name> <key> | default <default-value>
!store.get <store_name> <key> | query <yq-expression>
!store.get <store_name> <key> | default <default-value> | query <yq-expression>
```

### Examples

```yaml
# SSM Parameter Store
db_password: !store.get ssm /myapp/prod/db/password
feature_flag: !store.get ssm /features/new-feature | default "disabled"

# Redis
global_config: !store.get redis global-config
api_version: !store.get redis global-config | query .version
regional_config: !store.get redis "config-{{ .vars.region }}"

# Azure Key Vault
api_secret: !store.get azure-keyvault external-api-key
ssl_cert: !store.get azure-keyvault ssl-certificate | default ""

# Google Secret Manager
client_id: !store.get gsm oauth-config | query .client_id
```

### Key Differences from `!store`

| Feature | `!store` | `!store.get` |
|---------|----------|--------------|
| Key format | Constructs from stack/component/key | Exact key as provided |
| Use case | Atmos component outputs | Arbitrary external values |

## `!env`

Read environment variables from stack manifest `env:` sections or OS environment.

### Syntax

```yaml
# Read env var (null if not found)
!env <env-var-name>

# Read env var with default
!env <env-var-name> <default-value>

# Default with spaces
!env '<ENV_VAR> "default with spaces"'
```

### Resolution Order

1. Stack manifest `env:` sections (merged via inheritance)
2. OS environment variables
3. Default value (if provided)

### Examples

```yaml
api_key: !env API_KEY
app_name: !env APP_NAME my-app
description: !env 'APP_DESC "my application"'
region: !env AWS_REGION us-east-1
```

## `!exec`

Execute shell scripts and assign the output.

### Syntax

```yaml
# Single-line command
!exec <command>

# Multi-line script
!exec
  <line1>
  <line2>
  ...
```

### Examples

```yaml
timestamp: !exec date +%s
result: !exec echo 42
config: !exec get-config.sh --format json

# Multi-line
computed: |
  !exec
    foo=0
    for i in 1 2 3; do
      foo+=$i
    done
    echo $foo

# With template for dynamic args
output: !exec atmos terraform output component1 -s {{ .stack }} --skip-init -- -json test_map
```

### Notes

- Uses the `interp` Go package (POSIX-compatible, Bash-like)
- Complex types (lists, maps) must be returned as JSON strings
- Atmos automatically decodes JSON output into YAML types
- Prefer `!terraform.output` or `!terraform.state` over `!exec atmos terraform output`

## `!include`

Include local or remote files, parsed by extension.

### Syntax

```yaml
# Include entire file
!include <file-path>

# Include with YQ query
!include <file-path> <yq-expression>
```

### Supported Formats

| Extension | Format |
|-----------|--------|
| `.json` | JSON |
| `.yaml`, `.yml` | YAML |
| `.hcl`, `.tf`, `.tfvars` | HCL |
| `.txt`, `.md`, others | Raw text |

### Supported Sources

| Protocol | Example |
|----------|---------|
| Local (relative) | `!include ./config.yaml` |
| Local (absolute) | `!include /path/to/file.yaml` |
| Local (base_path) | `!include stacks/catalog/vpc/defaults.yaml` |
| HTTPS | `!include https://example.com/config.yaml` |
| GitHub | `!include github://org/repo/main/path/file.yaml` |
| S3 | `!include s3::https://bucket.s3.amazonaws.com/file.yaml` |
| GCS | `!include gcs::gs://bucket/file.yaml` |
| SCP/SFTP | `!include scp://user@host:/path/file.yaml` |
| OCI | `!include oci://registry/image:path/file.yaml` |

### Examples

```yaml
# Local files
config: !include ./config.yaml
cidr: !include ./vpc_config.yaml .vars.ipv4_primary_cidr_block
vars: !include config/prod.tfvars
description: !include ./description.md

# Remote files
region_config: !include https://raw.githubusercontent.com/org/repo/main/config.yaml .vars

# Paths with spaces
values: !include '"~/My Documents/config.yaml"'

# YQ with bracket notation
key: !include ./config.yaml '.security.users["github-dependabot"].key'
```

## `!include.raw`

Include files as raw text regardless of file extension. Useful when you want to treat a
`.json` or `.yaml` file as a plain string.

### Syntax

```yaml
!include.raw <file-path>
```

## `!template`

Evaluate Go template expressions and convert JSON results to proper YAML types.

### Syntax

```yaml
!template '<go-template-expression>'
```

### Examples

```yaml
# Complex outputs from atmos.Component
subnet_ids: !template '{{ toJson (atmos.Component "vpc" .stack).outputs.private_subnet_ids }}'
config_map: !template '{{ toJson (atmos.Component "config" .stack).outputs.config_map }}'

# Using settings
cidrs: !template '{{ toJson .settings.allowed_ingress_cidrs }}'

# Appending to lists
all_cidrs: !template '{{ toJson (concat .settings.allowed_ingress_cidrs (list "172.20.0.0/16")) }}'
```

### Notes

- Essential for handling lists and maps from `atmos.Component`
- Converts JSON strings to proper YAML types
- Prefer `!terraform.output` or `!terraform.state` over `!template` + `atmos.Component`

## `!literal`

Preserve values exactly as written, bypassing all template processing.

### Syntax

```yaml
!literal "<value>"
!literal |
  multi-line value
```

### Examples

```yaml
# Helm templates
annotation: !literal "{{ .Values.ingress.class }}"

# Terraform interpolation
user_data: !literal "${var.hostname}"

# ArgoCD
config: !literal "{{external.config_url}}"

# Inline arrays
users: [!literal "{{external.email}}", !literal "{{external.admin}}"]

# Regex patterns
pattern: !literal "^[a-z]+\\d{3}$"
```

## `!random`

Generate cryptographically secure random integers.

### Syntax

```yaml
!random                    # 0 to 65535
!random <max>              # 0 to max
!random <min> <max>        # min to max (inclusive)
```

### Examples

```yaml
port: !random 1024 65535
id: !random 1000 9999
default_random: !random
small: !random 100
```

### Notes

- Values are NOT persisted -- regenerated each time Atmos processes config
- Uses `crypto/rand` for cryptographic security

## `!cwd`

Get the current working directory where Atmos is executed.

### Syntax

```yaml
!cwd
```

## `!repo-root`

Get the root directory of the Atmos repository.

### Syntax

```yaml
!repo-root
```

## `!aws.account_id`

Get the current AWS account ID using STS GetCallerIdentity.

### Syntax

```yaml
!aws.account_id
```

## `!aws.organization_id`

Get the current AWS Organization ID using the AWS Organizations DescribeOrganization API.

### Syntax

```yaml
!aws.organization_id
```

### Notes

- Requires `organizations:DescribeOrganization` IAM permission
- The AWS account must be a member of an AWS Organization
- Results are cached per auth context for the duration of the Atmos execution
- Uses a separate cache from the identity functions (`!aws.account_id`, etc.)

## `!aws.caller_identity_arn`

Get the full ARN of the current AWS caller identity.

### Syntax

```yaml
!aws.caller_identity_arn
```

## `!aws.caller_identity_user_id`

Get the unique user ID of the current AWS caller identity.

### Syntax

```yaml
!aws.caller_identity_user_id
```

## `!aws.region`

Get the current AWS region from SDK configuration.

### Syntax

```yaml
!aws.region
```

## YQ Expression Syntax

Several YAML functions accept YQ expressions for querying complex data:

```yaml
# Array index
!terraform.output vpc .private_subnet_ids[0]

# Map key access
!terraform.output config .config_map.username

# Default values (// operator)
!terraform.output vpc ".vpc_id // ""fallback"""

# String concatenation
!terraform.output db ".hostname | ""jdbc://"" + . + "":5432"""

# Bracket notation for special characters
!terraform.output security '.users["github-dependabot"].key'
```

### Quoting Rules

- Wrap the entire YQ expression in single quotes when it contains double quotes
- Use double quotes inside brackets for string keys
- Escape double quotes inside YQ with two double quotes: `""value""`
- Escape single quotes by doubling: `''`
