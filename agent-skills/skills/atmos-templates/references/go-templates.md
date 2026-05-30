# Go Template Reference for Atmos

## Template Syntax Basics

Go templates in Atmos use `{{ }}` delimiters (configurable in `atmos.yaml`). Templates are
processed before YAML parsing, so output must produce valid YAML.

```yaml
# Simple value interpolation
name: "{{ .atmos_component }}"

# Function call
upper_name: "{{ .atmos_component | upper }}"

# Conditional (use single quotes to avoid YAML double-quote conflicts)
enabled: '{{ if eq .vars.stage "prod" }}true{{ else }}false{{ end }}'
```

## Available Context Variables

When processing Go templates in stack manifests, Atmos provides the full component configuration
as the template context. All values from `atmos describe component` are available.

### Core Variables

| Variable | Type | Description |
|----------|------|-------------|
| `.atmos_component` | string | The Atmos component name |
| `.atmos_stack` | string | The full Atmos stack name |
| `.stack` | string | Alias for `.atmos_stack` |
| `.atmos_stack_file` | string | Path to the stack manifest file |
| `.workspace` | string | Terraform workspace name |
| `.component` | string | The Terraform component path |

### Section Variables

| Variable | Type | Description |
|----------|------|-------------|
| `.vars` | map | Component variables |
| `.vars.namespace` | string | Organization namespace |
| `.vars.tenant` | string | Organizational unit |
| `.vars.environment` | string | Region/environment |
| `.vars.stage` | string | Account/stage |
| `.vars.name` | string | Component instance name |
| `.vars.region` | string | AWS/cloud region |
| `.vars.tags` | map | Resource tags |
| `.settings` | map | Component settings |
| `.env` | map | Environment variables |
| `.metadata` | map | Component metadata |
| `.metadata.component` | string | Terraform component name |
| `.providers` | map | Provider configuration |
| `.backend` | map | Backend configuration |
| `.backend_type` | string | Backend type (e.g., "s3") |

## Atmos-Specific Template Functions

### `atmos.Component`

Reads any section or attribute from another Atmos component in a stack.

```yaml
{{ (atmos.Component "<component>" "<stack>").<section>.<attribute> }}
```

**Reading outputs (remote state):**

```yaml
vpc_id: '{{ (atmos.Component "vpc" .stack).outputs.vpc_id }}'
vpc_id: '{{ (atmos.Component "vpc" "plat-ue2-prod").outputs.vpc_id }}'
```

**Reading variables:**

```yaml
vpc_name: '{{ (atmos.Component "vpc" .stack).vars.name }}'
```

**Reading settings:**

```yaml
test_flag: '{{ (atmos.Component "test" .stack).settings.test }}'
```

**Reading metadata:**

```yaml
tf_component: '{{ (atmos.Component "test" .stack).metadata.component }}'
```

**Dynamic stack names:**

```yaml
# Using printf for cross-stack references
vpc_id: '{{ (atmos.Component "vpc" (printf "net-%s-%s" .vars.environment .vars.stage)).outputs.vpc_id }}'
```

**Complex types (lists/maps) require `!template` + `toJson`:**

```yaml
# YAML function wrapping template for proper type handling
subnet_ids: !template '{{ toJson (atmos.Component "vpc" .stack).outputs.private_subnet_ids }}'
config_map: !template '{{ toJson (atmos.Component "config" .stack).outputs.config_map }}'
```

Results are cached per execution -- repeated calls to the same component/stack return cached data.

### `atmos.GomplateDatasource`

Fetches data from external sources with automatic caching. Requires datasource configuration in
`atmos.yaml` or stack manifest `settings.templates.settings.gomplate.datasources`.

```yaml
{{ (atmos.GomplateDatasource "<alias>").<attribute> }}
```

**Examples:**

```yaml
# API data
public_ip: '{{ (atmos.GomplateDatasource "ip").ip }}'

# AWS SSM Parameter
db_host: '{{ (atmos.GomplateDatasource "database").host }}'

# Local file
config: '{{ (atmos.GomplateDatasource "config").api_url }}'
```

Supported datasource types: HTTP/HTTPS, file, AWS SSM/Secrets Manager/S3, Azure Key Vault,
Google Cloud Storage, HashiCorp Vault, Consul, environment variables, Git.

### `atmos.Store`

Reads values from configured stores. Same as `!store` YAML function but in template syntax.

```yaml
{{ atmos.Store "<store_name>" "<stack>" "<component>" "<key>" }}
```

**Examples:**

```yaml
# Simple value
cidr: '{{ atmos.Store "redis" "prod" "vpc" "cidr" }}'

# Current stack
count: '{{ atmos.Store "redis" .stack "config" "instance_count" }}'

# Nested access
subnets: '{{ (atmos.Store "redis" .stack "config" "config_map").vpc_config.subnets_count }}'

# In multi-line strings
json_config: |
  {
    "cidr": {{ atmos.Store "redis" "prod" "vpc" "cidr" | quote }}
  }
```

## Sprig Functions

When `templates.settings.sprig.enabled: true`, all Sprig functions are available.

### String Functions

```yaml
# uppercase
name: '{{ upper .vars.name }}'

# lowercase
name: '{{ lower .vars.name }}'

# title case
title: '{{ title .vars.name }}'

# trim whitespace
clean: '{{ trim .vars.name }}'

# replace
slug: '{{ replace "/" "-" .atmos_component }}'

# substring
prefix: '{{ substr 0 3 .vars.name }}'

# quote
quoted: '{{ .vars.name | quote }}'

# default value
name: '{{ .vars.name | default "unnamed" }}'
```

### List Functions

```yaml
# first/last element
first: '{{ first .vars.zones }}'
last: '{{ last .vars.zones }}'

# join
zones_str: '{{ join "," .vars.zones }}'

# list creation
items: '{{ list "a" "b" "c" }}'

# has (check if list contains)
has_zone: '{{ has "us-east-1a" .vars.zones }}'
```

### Map Functions

```yaml
# get key
val: '{{ get .vars.tags "Environment" }}'

# has key
has_env: '{{ hasKey .vars.tags "Environment" }}'

# keys
tag_keys: '{{ keys .vars.tags }}'
```

### Type Conversion

```yaml
# to JSON
json: '{{ toJson .vars.tags }}'
json_raw: '{{ toRawJson .vars.tags }}'

# to string
str: '{{ toString .vars.count }}'

# to int
num: '{{ atoi .vars.port_string }}'
```

### OS Functions

```yaml
# environment variable
user: '{{ env "USER" }}'
home: '{{ env "HOME" }}'

# with default
profile: '{{ env "AWS_PROFILE" | default "default" }}'
```

## Gomplate Functions

When `templates.settings.gomplate.enabled: true`, all Gomplate functions are available.

### String Functions

```yaml
# Title case (Gomplate)
title: '{{ strings.Title .atmos_component }}'

# Quote
quoted: '{{ .vars.name | strings.Quote }}'

# Contains
has_prefix: '{{ strings.HasPrefix "vpc" .atmos_component }}'

# Replace
clean: '{{ strings.ReplaceAll "/" "-" .atmos_component }}'
```

### Environment (Gomplate)

```yaml
# getenv (Gomplate's env function alias - use when both Sprig and Gomplate are enabled)
user: '{{ getenv "USER" }}'
profile: '{{ getenv "AWS_PROFILE" "default" }}'
```

### Data Functions

```yaml
# JSON encode/decode
json: '{{ data.ToJSON .vars.tags }}'
parsed: '{{ data.JSON .vars.json_string }}'

# YAML encode
yaml: '{{ data.ToYAML .vars.config }}'
```

### Datasource Functions

```yaml
# Access configured datasources
ip: '{{ (datasource "ip").ip }}'

# With atmos caching
ip: '{{ (atmos.GomplateDatasource "ip").ip }}'
```

## Control Flow

### Conditionals

```yaml
# if/else
value: '{{ if eq .vars.stage "prod" }}production{{ else }}non-production{{ end }}'

# if/else if/else
tier: '{{ if eq .vars.stage "prod" }}tier1{{ else if eq .vars.stage "staging" }}tier2{{ else }}tier3{{ end }}'

# Boolean check
enabled: '{{ if .vars.enabled }}true{{ else }}false{{ end }}'

# Negation
disabled: '{{ if not .vars.enabled }}true{{ else }}false{{ end }}'

# And/Or
critical: '{{ if and (eq .vars.stage "prod") .vars.high_availability }}true{{ else }}false{{ end }}'
```

### Loops (Range)

```yaml
# Iterate over a list
zones: |
  {{ range .vars.availability_zones }}
  - {{ . }}
  {{ end }}
```

**Warning:** Range in templates can easily break YAML indentation. Use with extreme caution.

### Pipeline Operators

```yaml
# Chain functions with |
name: '{{ .vars.name | upper | quote }}'
description: '{{ .atmos_component | strings.Title }} in {{ .atmos_stack | strings.Quote }}'
```

## Template Delimiters

The default delimiters `{{ }}` can conflict with YAML syntax. To avoid issues with complex
outputs, configure custom delimiters:

```yaml
# atmos.yaml
templates:
  settings:
    delimiters: ["'{{", "}}'"]
```

With custom delimiters, complex types can be safely embedded:

```yaml
subnet_ids: '{{ toRawJson ((atmos.Component "vpc" .stack).outputs.private_subnet_ids) }}'
```

## Escaping Templates

### For External Systems

Prevent Atmos from processing templates intended for other tools:

```yaml
# Backtick escape
annotation: "{{`{{ .Values.ingress.class }}`}}"

# printf function
message: '{{ printf "Application {{ .app.metadata.name }} is running." }}'

# !literal YAML function (preferred)
annotation: !literal "{{ .Values.ingress.class }}"
```

### In Import Files

When using Go templates in both imports and manifests, escape second-pass templates:

```yaml
# In import template file (.tmpl)
tags:
  atmos_component: "{{`{{ .atmos_component }}`}}"
  atmos_stack: "{{`{{ .atmos_stack }}`}}"
```

## Performance Considerations

1. `atmos.Component` requires resolving the full component context and potentially running
   `terraform output`, which initializes Terraform and downloads providers
2. `atmos.GomplateDatasource` caches results per execution (use it instead of `datasource`)
3. `atmos.Store` caches results per store/stack/component/key combination
4. All Atmos template functions cache results within a single CLI command execution
5. Functions like `atmos describe stacks` evaluate all templates, so heavy use of
   `atmos.Component` can significantly slow these commands

## Safety Guidelines

1. Always quote template expressions in YAML values: `'{{ .value }}'` not `{{ .value }}`
2. Use `toJson` or `toRawJson` when embedding complex types (lists, maps)
3. Use `!template` YAML function for complex type output from `atmos.Component`
4. Prefer YAML functions (`!terraform.state`, `!store`) over template functions when possible
5. Test templates with `atmos describe component` to verify output before plan/apply
6. Keep templates simple -- complex templates are hard to debug and maintain
