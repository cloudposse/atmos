---
name: atmos-templates
description: "Templating: Go templates, Sprig/Gomplate functions, YAML functions (!terraform.output, atmos.Component, !store.get), store integration"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Templating System

## Overview

Atmos provides two complementary templating systems for adding dynamic behavior to stack configurations:

1. **YAML Functions** (recommended) -- Native YAML tags (`!terraform.output`, `!store`, `!env`, etc.)
   that execute after YAML parsing, making them type-safe and predictable
2. **Go Templates** -- Text-based templating (`{{ .vars.name }}`, `{{ atmos.Component ... }}`) that
   executes before YAML parsing, offering powerful text manipulation but requiring more caution

YAML functions are always preferred when they can accomplish the task. Go templates should be used
as an escape hatch for complex conditional logic, loops, dynamic key generation, or advanced string
manipulation that YAML functions cannot handle.

## YAML Functions (Recommended)

YAML functions use YAML explicit tags (the `!` prefix) and operate on structured data after YAML
parsing. They cannot break YAML syntax, are type-safe, and produce clear error messages.

### Available YAML Functions

| Function | Purpose |
|----------|---------|
| `!terraform.output` | Read Terraform outputs (remote state) via `terraform output` |
| `!terraform.state` | Read Terraform outputs directly from state backend (fastest) |
| `!store` | Read values from stores (SSM, Redis, etc.) using component/stack/key pattern |
| `!store.get` | Read arbitrary keys from stores (no naming convention required) |
| `!env` | Read environment variables (from stack `env:` sections or OS) |
| `!exec` | Execute shell scripts and use the output |
| `!include` | Include local or remote files (YAML, JSON, HCL, text) |
| `!include.raw` | Include files as raw text regardless of extension |
| `!template` | Evaluate Go template expressions and convert JSON to YAML types |
| `!literal` | Preserve values verbatim, bypassing all template processing |
| `!random` | Generate cryptographically secure random integers |
| `!cwd` | Get the current working directory |
| `!repo-root` | Get the repository root directory |
| `!aws.account_id` | Get the current AWS account ID via STS |
| `!aws.caller_identity_arn` | Get the current AWS caller identity ARN |
| `!aws.caller_identity_user_id` | Get the AWS caller identity user ID |
| `!aws.organization_id` | Get the current AWS Organization ID |
| `!aws.region` | Get the current AWS region from SDK config |

### Supported Sections

YAML functions work in all Atmos stack manifest sections:
- `vars`, `settings`, `env`, `metadata`, `command`, `component`
- `providers`, `overrides`, `backend`, `backend_type`
- `remote_state_backend`, `remote_state_backend_type`

### `!terraform.output` -- Remote State Access

Reads Terraform outputs by running `terraform output` (requires initialization):

```yaml
vars:
  # Two-parameter form: component + output (current stack)
  vpc_id: !terraform.output vpc vpc_id

  # Three-parameter form: component + stack + output
  vpc_id: !terraform.output vpc plat-ue2-prod vpc_id

  # Using Go templates for dynamic stack references
  vpc_id: !terraform.output vpc {{ .stack }} vpc_id

  # YQ expressions for complex outputs
  first_subnet: !terraform.output vpc .private_subnet_ids[0]
  db_host: !terraform.output config .config_map.username

  # Default values for unprovisioned components
  vpc_id: !terraform.output vpc ".vpc_id // ""default-vpc"""
```

**Performance note:** `!terraform.output` requires initializing Terraform for each component
(downloading providers, etc.), which can be slow. Prefer `!terraform.state` or `!store` when possible.

### `!terraform.state` -- Fast State Backend Access

Reads outputs directly from the Terraform state backend without initialization. Supports S3,
local, GCS, and azurerm backends. Same syntax as `!terraform.output`:

```yaml
vars:
  vpc_id: !terraform.state vpc vpc_id
  subnet_ids: !terraform.state vpc plat-ue2-prod private_subnet_ids
  first_az: !terraform.state vpc .availability_zones[0]
```

This is 10-100x faster than `!terraform.output` and should be preferred when the backend type
is supported.

### `!store` -- Component-Aware Store Access

Reads values from configured stores (SSM Parameter Store, Redis, Artifactory, etc.) following
the Atmos stack/component/key naming convention:

```yaml
vars:
  # Two-parameter form: store + component + key (current stack)
  vpc_id: !store prod/ssm vpc vpc_id

  # Three-parameter form: store + stack + component + key
  vpc_id: !store prod/ssm plat-ue2-prod vpc vpc_id

  # With dynamic stack reference
  vpc_id: !store prod/ssm {{ .stack }} vpc vpc_id

  # With default value
  api_key: !store prod/ssm config api_key | default "not-set"

  # With YQ query
  db_host: !store prod/ssm config connection | query .host
```

### `!store.get` -- Arbitrary Key Store Access

Reads arbitrary keys from stores without following the component/stack naming convention:

```yaml
vars:
  # Direct key access
  db_password: !store.get ssm /myapp/prod/db/password

  # With default value
  feature_flag: !store.get ssm /features/new-feature | default "disabled"

  # With YQ query
  api_key: !store.get redis app-config | query .api.key

  # Dynamic key with templates
  config: !store.get redis "config-{{ .vars.region }}"
```

### `!env` -- Environment Variables

Reads from stack manifest `env:` sections (merged via inheritance) or OS environment variables:

```yaml
vars:
  # Read env var, null if not found
  api_key: !env API_KEY

  # Read env var with default
  app_name: !env APP_NAME my-app

  # Default with spaces (use quotes)
  description: !env 'APP_DESC "my application"'
```

Resolution order: stack manifest `env:` sections -> OS environment variables -> default value.

### `!exec` -- Shell Script Execution

Executes shell scripts and assigns the output:

```yaml
vars:
  # Simple command
  timestamp: !exec date +%s

  # Multi-line script
  result: |
    !exec
      foo=0
      for i in 1 2 3; do
        foo+=$i
      done
      echo $foo

  # Complex types must be returned as JSON
  config: !exec get-config.sh --format json
```

### `!include` -- File Inclusion

Includes local or remote files, parsing them based on extension:

```yaml
vars:
  # Local files (relative to current manifest)
  config: !include ./config.yaml

  # Relative to base_path
  vpc_defaults: !include stacks/catalog/vpc/defaults.yaml

  # Remote files
  region_config: !include https://raw.githubusercontent.com/org/repo/main/config.yaml

  # With YQ query
  cidr: !include ./vpc_config.yaml .vars.ipv4_primary_cidr_block

  # HCL/tfvars files
  vars: !include config/prod.tfvars

  # Text/markdown files (returned as strings)
  description: !include ./description.md
```

Supported protocols: local files, HTTP/HTTPS, GitHub (`github://`), S3 (`s3::`), GCS (`gcs::`),
SCP/SFTP, OCI.

### `!template` -- Go Template Evaluation

Evaluates Go template expressions and converts JSON output to proper YAML types. Essential for
handling complex outputs (maps, lists) from `atmos.Component`:

```yaml
vars:
  # Convert list output to YAML list
  subnet_ids: !template '{{ toJson (atmos.Component "vpc" .stack).outputs.private_subnet_ids }}'

  # Convert map output to YAML map
  config: !template '{{ toJson (atmos.Component "config" .stack).outputs.config_map }}'

  # Use settings from the current component
  cidrs: !template '{{ toJson .settings.allowed_ingress_cidrs }}'
```

### `!literal` -- Bypass Template Processing

Preserves values exactly as written, preventing Atmos from evaluating template-like syntax:

```yaml
vars:
  # Pass Helm templates through to Helm
  annotation: !literal "{{ .Values.ingress.class }}"

  # Pass Terraform interpolation through
  user_data: !literal "#!/bin/bash\necho ${hostname}"

  # ArgoCD templates
  config_url: !literal "{{external.config_url}}"
```

### `!random` -- Random Number Generation

Generates cryptographically secure random integers:

```yaml
vars:
  port: !random 1024 65535       # Random port in range
  id: !random 1000 9999          # Random 4-digit ID
  default_random: !random         # 0 to 65535
  small_random: !random 100       # 0 to 100
```

## Go Templates

Go templates use `{{ }}` delimiters and are processed before YAML parsing. They support the full
Go `text/template` syntax plus Sprig and Gomplate function libraries.

### Enabling Go Templates

```yaml
# atmos.yaml
templates:
  settings:
    enabled: true
    evaluations: 1      # Number of processing passes
    delimiters: ["{{", "}}"]  # Default delimiters
    sprig:
      enabled: true     # Enable Sprig functions
    gomplate:
      enabled: true     # Enable Gomplate functions and datasources
      timeout: 5        # Datasource timeout in seconds
```

### Template Context Variables

In Go templates, you can reference any value from the component's configuration (as returned by
`atmos describe component`):

| Variable | Description |
|----------|-------------|
| `.atmos_component` | The Atmos component name |
| `.atmos_stack` | The Atmos stack name |
| `.stack` | Alias for `.atmos_stack` |
| `.atmos_stack_file` | The stack manifest file path |
| `.workspace` | The Terraform workspace name |
| `.vars.*` | Component variables |
| `.settings.*` | Component settings |
| `.env.*` | Environment variables |
| `.metadata.*` | Component metadata |
| `.providers.*` | Provider configuration |
| `.backend.*` | Backend configuration |
| `.backend_type` | Backend type string |

### Template Examples

```yaml
components:
  terraform:
    vpc:
      vars:
        tags:
          atmos_component: "{{ .atmos_component }}"
          atmos_stack: "{{ .atmos_stack }}"
          terraform_workspace: "{{ .workspace }}"
          # Sprig function
          provisioned_by: '{{ env "USER" }}'
          # Gomplate function
          description: "{{ strings.Title .atmos_component }} in {{ .atmos_stack }}"
```

### `atmos.Component` Template Function

Reads any section or attribute from another component's configuration, including Terraform outputs:

```yaml
vars:
  # Read outputs (remote state)
  vpc_id: '{{ (atmos.Component "vpc" .stack).outputs.vpc_id }}'

  # Read variables from another component
  vpc_name: '{{ (atmos.Component "vpc" .stack).vars.name }}'

  # Read settings
  test_setting: '{{ (atmos.Component "test" .stack).settings.test }}'

  # Read metadata
  component_name: '{{ (atmos.Component "test" .stack).metadata.component }}'

  # Complex outputs require !template + toJson
  subnet_ids: !template '{{ toJson (atmos.Component "vpc" .stack).outputs.private_subnet_ids }}'
```

### `atmos.GomplateDatasource` Template Function

Fetches external data with automatic caching:

```yaml
settings:
  templates:
    settings:
      gomplate:
        datasources:
          ip:
            url: "https://api.ipify.org?format=json"
          secret:
            url: "aws+smp:///path/to/secret"

vars:
  public_ip: '{{ (atmos.GomplateDatasource "ip").ip }}'
  db_password: '{{ (atmos.GomplateDatasource "secret").password }}'
```

The function caches results per execution -- multiple references to the same datasource make
only one external call.

### `atmos.Store` Template Function

Reads from stores using Go template syntax (same as `!store` YAML function but in template form):

```yaml
vars:
  vpc_id: '{{ atmos.Store "prod/ssm" .stack "vpc" "vpc_id" }}'
  config: !template '{{ (atmos.Store "redis" .stack "config" "config_map").defaults | toJSON }}'
```

## Template Evaluations (Processing Pipelines)

Atmos supports multiple evaluation passes for template processing:

```yaml
# atmos.yaml
templates:
  settings:
    enabled: true
    evaluations: 2  # Two passes
```

With multiple evaluations, output from the first pass becomes input to the second pass. This is
useful for:
- Combining templates from different sections
- Using templates in datasource URLs
- Multi-stage template resolution

## When to Use Templates vs. YAML Functions

| Scenario | Use |
|----------|-----|
| Reading Terraform outputs | `!terraform.state` or `!terraform.output` |
| Reading store values | `!store` or `!store.get` |
| Environment variables | `!env` |
| Including files | `!include` |
| Simple value interpolation | `!template` or Go templates |
| Complex outputs (lists/maps) | `!template` with `toJson` |
| Conditional logic (`if/else`) | Go templates |
| Loops and iteration | Go templates |
| Dynamic key generation | Go templates |
| Passing syntax to external tools | `!literal` |
| External API data | `atmos.GomplateDatasource` |
| Advanced string manipulation | Go templates with Sprig/Gomplate |

## Excluding Templates from Processing

### Passing Templates to External Tools

Use the backtick escape or `!literal` to prevent Atmos from processing templates intended for
external systems (ArgoCD, Helm, Datadog):

```yaml
# Using !literal (recommended)
annotation: !literal "{{ .Values.ingress.class }}"

# Using backtick escape
annotation: "{{`{{ .Values.ingress.class }}`}}"

# Using printf
annotation: '{{ printf "{{ .Values.ingress.class }}" }}'
```

### Templates in Imports

When using Go templates in both imports and stack manifests, templates intended for the second
pass (stack processing) must be escaped in the import file:

```yaml
# stacks/catalog/eks/eks_cluster.tmpl
components:
  terraform:
    eks/cluster:
      vars:
        # First pass: resolved from import context
        enabled: "{{ .enabled }}"
        name: "{{ .name }}"
        tags:
          # Second pass: escaped for stack processing
          atmos_component: "{{`{{ .atmos_component }}`}}"
          atmos_stack: "{{`{{ .atmos_stack }}`}}"
```

## Template Configuration in Stack Manifests

Template settings can be defined in `settings.templates.settings` in stack manifests, which
deep-merges with `templates.settings` in `atmos.yaml`. Stack manifest settings take precedence.

```yaml
# stacks/orgs/acme/_defaults.yaml
settings:
  templates:
    settings:
      env:
        AWS_PROFILE: "my-profile"
      gomplate:
        timeout: 7
        datasources:
          config:
            url: "./my-config.json"
```

Note: `enabled`, `sprig.enabled`, `gomplate.enabled`, `evaluations`, and `delimiters` settings
are not supported in stack manifests (only in `atmos.yaml`).

## Performance Best Practices

1. **Prefer `!terraform.state` over `!terraform.output`** -- 10-100x faster (no Terraform init)
2. **Prefer `!store` over `atmos.Component` for outputs** -- Avoids Terraform initialization
3. **Use `atmos.GomplateDatasource` instead of `datasource`** -- Built-in caching prevents
   redundant API calls
4. **Minimize `atmos.Component` usage** -- Each call may initialize Terraform
5. **Use Terraform remote state module directly** when possible instead of template functions
6. **All YAML functions and template functions cache results** per execution for repeated calls

## Common Pitfalls

1. **Go templates break YAML** -- Unquoted `{{ }}` can cause YAML parse errors. Always quote
   template expressions.
2. **Type confusion** -- Go templates always return strings. Use `!template` with `toJson` for
   complex types.
3. **Indentation issues** -- Multi-line template output can break YAML indentation.
4. **Sprig/Gomplate conflicts** -- The `env` function exists in both libraries with different
   syntax. Use `getenv` for Gomplate's version when both are enabled.
5. **Cold-start errors** -- `!terraform.output` and `!store` fail if the referenced component
   is not yet provisioned. Use YQ defaults (`//`) or `| default` to handle this.
6. **Performance degradation** -- Overuse of `atmos.Component` or `!terraform.output` across
   many stacks can dramatically slow `atmos describe stacks` and `atmos describe affected`.

## Additional Resources

- For complete Go template context variables and functions, see [references/go-templates.md](references/go-templates.md)
- For the full YAML functions reference with syntax and examples, see [references/yaml-functions-reference.md](references/yaml-functions-reference.md)
